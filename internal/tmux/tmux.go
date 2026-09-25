package tmux

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	// MinimumMajor and MinimumMinor are the oldest supported tmux release.
	MinimumMajor = 3
	MinimumMinor = 2
)

var (
	versionPattern     = regexp.MustCompile(`^tmux ([0-9]+)\.([0-9]+)`) // tmux 3.5a, tmux 3.2
	paneIDPattern      = regexp.MustCompile(`^%[0-9]+$`)
	captureLinePattern = regexp.MustCompile(`^(?:-|[-]?[0-9]+)$`)
)

// Client runs tmux exclusively against deck's configured private socket.
// Binary defaults to "tmux" and Timeout defaults to five seconds. Mouse
// mirrors config.toml's top-level tmux_mouse key (SPEC §6.5/§11.8): whether
// Bootstrap turns on tmux's own `mouse` server option on deck's private
// socket. It defaults to Go's zero value (false, i.e. tmux's own default)
// like every other Client field -- callers that want it on must set it
// explicitly from a resolved config.Settings, the same way Socket is never
// implicitly config.DefaultSocket here either.
type Client struct {
	Binary  string
	Socket  string
	Timeout time.Duration
	Mouse   bool
}

// Version is the parsed tmux version reported by `tmux -V`.
type Version struct {
	Major int
	Minor int
	Raw   string
}

// Launch describes the one-pane tmux session deck creates for a durable row.
// Command is an argv, not a shell string; tmux receives it after a literal --.
type Launch struct {
	Slug    string
	CWD     string
	Command []string
	Env     map[string]string
}

// Session is the liveness information returned by List.
type Session struct {
	Name  string
	Panes []Pane
}

// Pane is a deliberately small set of facts that reconciliation needs from
// tmux. All fields are read from tmux format strings, never inferred locally.
type Pane struct {
	ID          string
	CurrentPath string
	PID         int
	Dead        bool
	DeadStatus  *int
	Command     string
	// Width and Height are the pane's real terminal geometry (tmux's
	// pane_width/pane_height), independent of how much of it capture-pane
	// actually returns for a given row (trailing blank lines/columns are
	// trimmed by tmux). The §11.7 crop (task 018) needs this to state the
	// real geometry ("45×22 of 120×40") rather than inferring it from
	// however much non-blank content happened to be captured.
	Width, Height int
}

// CaptureOptions describes an explicit tmux pane range. Line positions use
// tmux's capture-pane notation: integers are relative to the top of the visible
// pane, negative integers address history, and "-" means the beginning (for
// StartLine) or end (for EndLine) of the available pane contents. Including
// escape sequences is useful for replay; crash tails should leave it false.
type CaptureOptions struct {
	StartLine              string
	EndLine                string
	IncludeEscapeSequences bool
	// PreserveTrailingBlankLines is tmux's `-N`: without it, tmux trims
	// trailing spaces from each line and the pane's own trailing blank
	// lines before returning them, which silently discards
	// background-styled blank cells (a filled row whose only content is
	// its own background colour looks, character-for-character, exactly
	// like an empty one). PRD phase3b II-18's seed capture always sets
	// this; ordinary crash-tail/preview reads normally leave it unset.
	PreserveTrailingBlankLines bool
}

func (v Version) String() string { return v.Raw }

// Supported reports whether this release meets deck's tmux contract.
func (v Version) Supported() bool {
	return v.Major > MinimumMajor || (v.Major == MinimumMajor && v.Minor >= MinimumMinor)
}

func (c Client) binary() string {
	if c.Binary == "" {
		return "tmux"
	}
	return c.Binary
}

func (c Client) timeout() time.Duration {
	if c.Timeout <= 0 {
		return 5 * time.Second
	}
	return c.Timeout
}

func (c Client) command(ctx context.Context, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, c.binary(), append([]string{"-L", c.Socket}, args...)...)
}

func (c Client) run(ctx context.Context, args ...string) ([]byte, error) {
	commandCtx, cancel := context.WithTimeout(ctx, c.timeout())
	defer cancel()
	output, err := c.command(commandCtx, args...).CombinedOutput()
	if err != nil {
		return output, fmt.Errorf("tmux -L %s %s: %w: %s", c.Socket, strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return output, nil
}

// runWithStdin is run's twin for a command that reads its payload from
// stdin instead of argv -- PRD II-36's whole point is that ONLY this
// shape (used by send.go's load-buffer streaming) has no length ceiling
// at all, unlike every argv-based command tmux accepts, which is capped
// by tmux's own ~16 KiB internal command-string limit (see send.go's
// literalChunkBytes/hexChunkArgs doc comments) long before the OS's much
// larger ARG_MAX would ever matter. This helper is deliberately generic
// (it never hardcodes a command name), so it does not itself need to be
// added to dispatch_test.go's per-file send-primitive allowlist -- only
// the call site that actually names "load-buffer" does, and that call
// site lives in send.go, which is already allowlisted.
func (c Client) runWithStdin(ctx context.Context, stdin io.Reader, args ...string) ([]byte, error) {
	commandCtx, cancel := context.WithTimeout(ctx, c.timeout())
	defer cancel()
	cmd := c.command(commandCtx, args...)
	cmd.Stdin = stdin
	output, err := cmd.CombinedOutput()
	if err != nil {
		return output, fmt.Errorf("tmux -L %s %s: %w: %s", c.Socket, strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return output, nil
}

func sessionName(slug string) (string, error) {
	if !regexp.MustCompile(`^[a-z0-9_-]+$`).MatchString(slug) {
		return "", fmt.Errorf("invalid session slug %q; use only lowercase letters, digits, _ and -", slug)
	}
	return "deck_" + slug, nil
}

// SessionName is sessionName exported: task 061 (PRD Part II §11.9 onward)
// needs the WINDOW-scoped target -- the deck_<slug> session name -- for
// every geometry/ownership call (CaptureWindowGeometry, FitWindowToPane,
// ClaimWindowOwnership, RestoreWindowGeometry all address the window this
// names, never the pane id PreviewPane returns), and internal/tui is
// outside this package.
func SessionName(slug string) (string, error) {
	return sessionName(slug)
}

func environmentArgs(environment map[string]string) ([]string, error) {
	keys := make([]string, 0, len(environment))
	for key := range environment {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	args := make([]string, 0, len(keys)*2)
	for _, key := range keys {
		value := environment[key]
		if key == "" || strings.ContainsAny(key, "=\x00") || strings.Contains(value, "\x00") {
			return nil, fmt.Errorf("invalid environment variable %q", key)
		}
		args = append(args, key, value)
	}
	return args, nil
}

// Discover verifies that tmux is installed and that its version supports the
// server options deck needs. It intentionally does not include -L: `tmux -V`
// never connects to or creates a server.
func (c Client) Discover(ctx context.Context) (Version, error) {
	checkCtx, cancel := context.WithTimeout(ctx, c.timeout())
	defer cancel()
	output, err := exec.CommandContext(checkCtx, c.binary(), "-V").CombinedOutput()
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
			return Version{}, fmt.Errorf("tmux is required but was not found on PATH; install tmux %d.%d or newer: %w", MinimumMajor, MinimumMinor, err)
		}
		return Version{}, fmt.Errorf("run tmux -V: %w: %s", err, strings.TrimSpace(string(output)))
	}
	text := strings.TrimSpace(string(output))
	match := versionPattern.FindStringSubmatch(text)
	if match == nil {
		return Version{}, fmt.Errorf("unrecognized tmux version %q; deck requires tmux %d.%d or newer", text, MinimumMajor, MinimumMinor)
	}
	major, _ := strconv.Atoi(match[1])
	minor, _ := strconv.Atoi(match[2])
	version := Version{Major: major, Minor: minor, Raw: text}
	if !version.Supported() {
		return Version{}, fmt.Errorf("tmux %s is too old; deck requires tmux %d.%d or newer", text, MinimumMajor, MinimumMinor)
	}
	return version, nil
}

// Bootstrap creates deck's private server if necessary and configures every
// contract option in one tmux client invocation. Passing -L on the sole command
// is deliberate: deck must never initialize or alter the user's default server.
// Create makes exactly one detached tmux session named deck_<slug>, with one
// pane in CWD running Command. Environment is passed to that initial process
// and mirrored into the session for any future pane.
func (c Client) Create(ctx context.Context, launch Launch) (Session, error) {
	if c.Socket == "" {
		return Session{}, errors.New("tmux socket name is required")
	}
	name, err := sessionName(launch.Slug)
	if err != nil {
		return Session{}, err
	}
	if launch.CWD == "" {
		return Session{}, errors.New("session working directory is required")
	}
	if len(launch.Command) == 0 || launch.Command[0] == "" {
		return Session{}, errors.New("session command is required")
	}
	env, err := environmentArgs(launch.Env)
	if err != nil {
		return Session{}, err
	}
	if err := c.Bootstrap(ctx); err != nil {
		return Session{}, err
	}
	// The session's own environment table (task 501) is mirrored via
	// new-session's own -e, one flag per variable, so it lands as part of
	// this single new-session call -- atomically with session creation --
	// rather than via a separate set-environment call issued afterwards.
	// The previous code ran set-environment in a follow-up loop once
	// new-session had already returned; when Command exits essentially
	// instantly (e.g. a failing pre_launch, crash.feature:46), the
	// reconciler goroutine (reconcile.go) can observe the dead pane and
	// Kill the very same session on its next tick before that follow-up
	// loop finishes -- independent of tmux's own remain-on-exit, which
	// only decides whether the pane is retained, never whether deck's own
	// reconciler leaves the session alone. That surfaced as "no such
	// session" and aborted the whole create; see
	// docs/reports/phase3g-501-create-env-race/ for the red/green repro.
	// -e is available because deck's documented minimum tmux is 3.2 and -e
	// was added in 3.0. There is deliberately no fallback/tolerance here:
	// an -e failure still fails Create exactly as before, it just cannot
	// lose the environment-mirroring race against a concurrent reconciler
	// anymore because there is no longer a second, later call for that
	// race to have a window in.
	args := []string{"new-session", "-d", "-s", name}
	for key, value := range pairs(env) {
		args = append(args, "-e", key+"="+value)
	}
	args = append(args, "-c", launch.CWD, "--", "env")
	for key, value := range pairs(env) {
		args = append(args, key+"="+value)
	}
	args = append(args, launch.Command...)
	if _, err := c.run(ctx, args...); err != nil {
		return Session{}, fmt.Errorf("create session %q: %w", name, err)
	}
	return c.session(ctx, name)
}

// SetEnvironment mirrors exactly one key/value into a deck-owned tmux
// session's own environment table via `set-environment -t`. This updates
// what tmux itself remembers for the session (visible via
// `show-environment -t`, and inherited by any FUTURE pane tmux starts in
// it) -- it never reaches into an already-running pane's process, which
// keeps whatever environment it inherited when it started. Callers that
// need the change to actually take effect in the live pane must kill and
// relaunch it with the new value (task 022's `R`); this method by itself
// makes no such claim.
func (c Client) SetEnvironment(ctx context.Context, slug, key, value string) error {
	if c.Socket == "" {
		return errors.New("tmux socket name is required")
	}
	name, err := sessionName(slug)
	if err != nil {
		return err
	}
	if key == "" || strings.ContainsAny(key, "=\x00") || strings.Contains(value, "\x00") {
		return fmt.Errorf("invalid environment variable %q", key)
	}
	if _, err := c.run(ctx, "set-environment", "-t", name, key, value); err != nil {
		return fmt.Errorf("set environment for session %q: %w", name, err)
	}
	return nil
}

// SendKeys types literal text into a deck-owned session's single pane,
// exactly as a user typing at the keyboard would, followed by Enter to
// submit it as a command line. `-l` sends the text literally (no key-name
// translation, so `=`/`$`/quotes in an exported value reach the shell
// unmodified) and Enter is sent as its own key name in a second call,
// mirroring how a real keystroke sequence separates typed text from the
// key that submits it. It is used only by task 023's inject-instead path
// to run `export KEY=value` in an already-running shell session's pane --
// never to drive any coding agent's own input, and never anything but
// this one line.
//
// PRD II-30 (task 052) forbids ever dispatching BY the session name --
// this is exactly the hazard a renamed-then-reused session's impostor
// pane can silently absorb. SendKeys therefore never builds its own
// "-t" from the session name: it resolves the session's live pane id
// once (PreviewPane, the same lookup the read-only preview path already
// uses) and sends through a Dispatcher constructed on that pane id, so
// the real tmux command's target is always a verified pane id and any
// rename between resolution and send is caught as identity drift
// (ErrIdentityDrifted) rather than silently landing in an impostor.
//
// The literal payload itself goes through Dispatcher.SendLiteral (task
// 054/II-32), never a bare Send("send-keys", ...) built here -- that is
// the one place in this package permitted to assemble a literal
// send-keys argv, so this method and every later dispatch primitive stay
// on the single reviewed `-l --` code path.
func (c Client) SendKeys(ctx context.Context, slug, literal string) error {
	if c.Socket == "" {
		return errors.New("tmux socket name is required")
	}
	pane, ok, err := c.PreviewPane(ctx, slug)
	if err != nil {
		return fmt.Errorf("resolve pane for session %q: %w", slug, err)
	}
	if !ok {
		return fmt.Errorf("send keys to session %q: no live pane", slug)
	}
	dispatcher, err := NewDispatcher(ctx, c, pane.ID)
	if err != nil {
		return fmt.Errorf("send keys to session %q: %w", slug, err)
	}
	if err := dispatcher.SendLiteral(ctx, literal); err != nil {
		return fmt.Errorf("send keys to session %q: %w", slug, err)
	}
	if err := dispatcher.Send(ctx, "send-keys", "Enter"); err != nil {
		return fmt.Errorf("send Enter to session %q: %w", slug, err)
	}
	return nil
}

// List returns only deck-owned sessions and their pane facts. A server with no
// sessions is a normal empty result.
func (c Client) List(ctx context.Context) ([]Session, error) {
	if c.Socket == "" {
		return nil, errors.New("tmux socket name is required")
	}
	output, err := c.run(ctx, "list-sessions", "-F", "#{session_name}")
	if err != nil {
		// A private server that was killed (or has never been bootstrapped) is
		// an empty liveness view, not a reason to start a replacement server.
		message := err.Error()
		if strings.Contains(message, "no server running") || strings.Contains(message, "no sessions") ||
			strings.Contains(message, "error connecting to") && strings.Contains(message, "No such file or directory") {
			return []Session{}, nil
		}
		return nil, err
	}
	var sessions []Session
	for _, name := range strings.Fields(string(output)) {
		if !strings.HasPrefix(name, "deck_") {
			continue
		}
		session, err := c.session(ctx, name)
		if err != nil {
			// A session can disappear between list-sessions and list-panes.
			// Treat that narrow race as an absent session so reconciliation can
			// record the durable transition instead of aborting its whole pass.
			if sessionDisappeared(err) {
				continue
			}
			return nil, err
		}
		sessions = append(sessions, session)
	}
	return sessions, nil
}

// SeedCaptureOptions is the exact capture-pane options PRD phase3b II-18
// requires for an interactive-preview seed: the VISIBLE PANE ONLY
// (StartLine "0" through EndLine "-", i.e. no scrollback history at all),
// escape sequences preserved so the body carries its own SGR verbatim,
// and `-N` so trailing background-styled blank cells survive instead of
// being trimmed (the second mandatory negative control task 042
// demonstrates).
//
// "Visible only" is now a DELIBERATE, cost-driven choice rather than the
// only shape that exists (issue #29): this is the range the two periodic
// reseed loops use -- internal/interactive's captureLoop (TransportCapture)
// and fallbackLoop (post-pipe-displacement) -- and both of them rebuild
// the grid WHOLESALE every 200ms. A history-inclusive capture at those
// loops' cadence was measured at ~37ms and ~21MB of garbage per tick for
// a 2040-row seed against ~0.7ms for a 40-row one, i.e. roughly a fifth
// of a core at test geometry and half a core at the operator's real
// 166x66 panes; the visible-only range keeps them exactly as cheap as
// they have always been. The ONE-OFF entry seed, which is what issue #29
// is about, uses the history-inclusive variant instead -- see
// SeedCaptureOptionsWithHistory.
func SeedCaptureOptions() CaptureOptions {
	return CaptureOptions{StartLine: "0", EndLine: "-", IncludeEscapeSequences: true, PreserveTrailingBlankLines: true}
}

// SeedCaptureOptionsWithHistory is SeedCaptureOptions widened to include
// up to historyLines rows of the pane's tmux-side scrollback ABOVE the
// visible screen ("-S -<historyLines>"), which is what issue #29 needs:
// a pane that has been running for a while before it is ever previewed
// interactively has all of its earlier output in tmux's own history and
// none of it in deck's grid, so Shift+PgUp in a freshly entered
// interactive preview reaches nothing at all. Only the ENTRY seed uses
// this range (internal/interactive.CaptureSeedWithHistory, called from
// internal/tui's enterInteractiveBody); the periodic reseed loops
// deliberately stay on SeedCaptureOptions' visible-only range for the
// cost reasons documented there.
//
// tmux CLAMPS this range gracefully rather than erroring: a pane whose
// history is shorter than historyLines returns
// min(history_size, historyLines) + pane_height rows, and a pane with no
// history at all returns exactly pane_height rows -- pinned against a
// real tmux by TestSeedCaptureOptionsWithHistoryClampsToAvailableHistory,
// which is what protects every caller here from having to pre-read
// #{history_size} and pick a range from it (a read that would race the
// capture anyway).
//
// historyLines <= 0 returns exactly SeedCaptureOptions(), so "no history"
// is expressible without a caller having to know that "0" is the
// visible-screen start line.
func SeedCaptureOptionsWithHistory(historyLines int) CaptureOptions {
	if historyLines <= 0 {
		return SeedCaptureOptions()
	}
	options := SeedCaptureOptions()
	options.StartLine = "-" + strconv.Itoa(historyLines)
	return options
}

// CapturePane returns exactly the requested range from a pane previously
// obtained from List or Create. Requiring both bounds keeps capture ownership
// explicit and allows the same primitive to serve bounded crash tails and
// escape-preserving replay.
func (c Client) CapturePane(ctx context.Context, paneID string, options CaptureOptions) ([]byte, error) {
	if c.Socket == "" {
		return nil, errors.New("tmux socket name is required")
	}
	if !paneIDPattern.MatchString(paneID) {
		return nil, fmt.Errorf("invalid tmux pane id %q", paneID)
	}
	if !captureLinePattern.MatchString(options.StartLine) {
		return nil, fmt.Errorf("invalid capture start line %q", options.StartLine)
	}
	if !captureLinePattern.MatchString(options.EndLine) {
		return nil, fmt.Errorf("invalid capture end line %q", options.EndLine)
	}
	args := []string{"capture-pane", "-p"}
	if options.IncludeEscapeSequences {
		args = append(args, "-e")
	}
	if options.PreserveTrailingBlankLines {
		args = append(args, "-N")
	}
	args = append(args, "-S", options.StartLine, "-E", options.EndLine, "-t", paneID)
	output, err := c.run(ctx, args...)
	if err != nil {
		return nil, fmt.Errorf("capture pane %q: %w", paneID, err)
	}
	return output, nil
}

// PreviewCapture is one point-in-time, read-only snapshot of a session's
// live pane for the §11.6/§11.7 preview panel (SPEC requirements 21, 22).
// Live is false whenever the session currently has no live pane — stopped,
// archived, a starting row with no pane yet, or a pane tmux has already
// reaped between List and capture-pane — and Bytes is then nil; nothing
// past that point should treat an inert PreviewCapture as an error. Bytes,
// when Live, is the exact escape-preserving contents of the pane's visible
// screen only, never scrollback history. Width and Height are the pane's
// real geometry at capture time (task 018, SPEC requirement 23's "45×22 of
// 120×40" line) — independent of however many of Bytes' lines happen to be
// non-blank, since tmux trims trailing blank lines/columns from a capture.
type PreviewCapture struct {
	Live          bool
	Bytes         []byte
	Width, Height int
}

// PreviewPane resolves the live tmux pane for a deck-owned session by its
// slug, for read-only preview capture only — callers must never attach to,
// resize, or write input to the pane this returns. ok is false when no live
// pane exists; a dead pane (tmux has observed its process exit but
// reconciliation has not yet collected it) is reported the same as absent,
// since a corpse is never a legitimate preview target.
func (c Client) PreviewPane(ctx context.Context, slug string) (pane Pane, ok bool, err error) {
	name, err := sessionName(slug)
	if err != nil {
		return Pane{}, false, err
	}
	sessions, err := c.List(ctx)
	if err != nil {
		return Pane{}, false, err
	}
	for _, session := range sessions {
		if session.Name != name || len(session.Panes) == 0 {
			continue
		}
		candidate := session.Panes[0]
		if candidate.Dead {
			return Pane{}, false, nil
		}
		return candidate, true, nil
	}
	return Pane{}, false, nil
}

// CapturePreview is the whole read-only preview capture engine (SPEC
// requirements 21, 22): it resolves the session's live pane and, if one
// exists, captures exactly its visible screen (StartLine "0", the top of
// the visible pane, through EndLine "-", the bottom of the visible pane —
// deliberately excluding scrollback history) with escape sequences
// preserved, so the preview can reproduce colour and cursor state exactly.
// It performs a single capture-pane invocation per call and nothing else:
// no client ever attaches, no control-mode server ever spawns, pipe-pane is
// never used, and the pane is never resized. A session with no live pane,
// or one tmux has already reaped between the two commands, is reported as
// Live: false rather than an error.
func (c Client) CapturePreview(ctx context.Context, slug string) (PreviewCapture, error) {
	pane, ok, err := c.PreviewPane(ctx, slug)
	if err != nil {
		return PreviewCapture{}, err
	}
	if !ok {
		return PreviewCapture{}, nil
	}
	data, err := c.CapturePane(ctx, pane.ID, CaptureOptions{StartLine: "0", EndLine: "-", IncludeEscapeSequences: true})
	if err != nil {
		if IsTargetAbsent(err) {
			return PreviewCapture{}, nil
		}
		return PreviewCapture{}, err
	}
	return PreviewCapture{Live: true, Bytes: data, Width: pane.Width, Height: pane.Height}, nil
}

// Exists reports whether a tmux session named deck_<slug> is already
// running on this client's private server, so a caller can adopt an
// already-launched pane (SPEC requirement 46) instead of attempting
// `new-session` over it and misreporting tmux's own "duplicate session"
// refusal as a launch failure. An absent private server (never bootstrapped,
// or killed) is a normal "does not exist" answer, not an error; any other
// tmux failure (a socket permission problem, a corrupt server, etc.) is
// returned so a genuine, non-duplicate tmux failure still surfaces.
func (c Client) Exists(ctx context.Context, slug string) (bool, error) {
	if c.Socket == "" {
		return false, errors.New("tmux socket name is required")
	}
	name, err := sessionName(slug)
	if err != nil {
		return false, err
	}
	if _, err := c.run(ctx, "has-session", "-t", name); err != nil {
		if IsTargetAbsent(err) {
			return false, nil
		}
		return false, fmt.Errorf("check session %q: %w", name, err)
	}
	return true, nil
}

// HasLivePane reports whether a tmux session named deck_<slug> exists on
// this client's private server AND still has at least one pane whose own
// process has not exited. It is the liveness predicate a caller that wants
// to LAUNCH must use, and it deliberately differs from Exists: deck's
// server runs `remain-on-exit failed` (see Bootstrap), so a pane exiting
// non-zero is RETAINED as a dead pane together with its session, and
// `has-session` keeps succeeding for it forever. Reporting such a corpse as
// "already running" is exactly issue #6: requirement 46's adoption fired on
// a session with nothing left to adopt, so `r` became a silent no-op and the
// row was unrecoverable from the UI. A session that is absent, has no panes
// at all, or whose every pane is dead is therefore reported false — with an
// absent target (never bootstrapped, killed, or removed between two commands)
// a normal false rather than an error, as in Exists. Any other tmux failure is
// returned so a genuine problem still surfaces.
func (c Client) HasLivePane(ctx context.Context, slug string) (bool, error) {
	if c.Socket == "" {
		return false, errors.New("tmux socket name is required")
	}
	name, err := sessionName(slug)
	if err != nil {
		return false, err
	}
	output, err := c.run(ctx, "list-panes", "-t", name, "-F", "#{pane_dead}")
	if err != nil {
		if IsTargetAbsent(err) {
			return false, nil
		}
		return false, fmt.Errorf("check live pane for session %q: %w", name, err)
	}
	for _, field := range strings.Fields(string(output)) {
		switch field {
		case "0":
			return true, nil
		case "1":
			continue
		default:
			return false, fmt.Errorf("check live pane for session %q: unexpected pane_dead value %q", name, field)
		}
	}
	return false, nil
}

// Kill removes a deck-owned tmux session without touching a similarly named
// user session on the default tmux socket. A concurrently removed session (or
// private server) is already in the desired state and therefore succeeds.
func (c Client) Kill(ctx context.Context, slug string) error {
	name, err := sessionName(slug)
	if err != nil {
		return err
	}
	if _, err := c.run(ctx, "kill-session", "-t", name); err != nil {
		if IsTargetAbsent(err) {
			return nil
		}
		return fmt.Errorf("kill session %q: %w", name, err)
	}
	return nil
}

// AttachCommand returns the interactive tmux command for a deck-owned session.
// It deliberately clears TMUX: attaching from a nested tmux is unsupported,
// and leaving that variable set makes tmux attempt to switch clients instead
// of creating the required direct attachment. The caller owns its terminal
// streams; Bubble Tea uses this with tea.Exec so it can restore its UI after a
// detach.
func (c Client) AttachCommand(ctx context.Context, slug string) (*exec.Cmd, error) {
	if c.Socket == "" {
		return nil, errors.New("tmux socket name is required")
	}
	name, err := sessionName(slug)
	if err != nil {
		return nil, err
	}
	// An interactive attachment is intentionally governed by the caller's
	// context, not Client.Timeout (which only bounds noninteractive commands).
	cmd := c.command(ctx, "attach-session", "-t", name)
	cmd.Env = withoutTMUX(os.Environ())
	return cmd, nil
}

// Attach connects the caller's terminal directly to a deck-owned session.
// Detaching returns nil.
func (c Client) Attach(ctx context.Context, slug string) error {
	cmd, err := c.AttachCommand(ctx, slug)
	if err != nil {
		return err
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("attach session %q: %w", slug, err)
	}
	return nil
}

func withoutTMUX(environment []string) []string {
	result := make([]string, 0, len(environment))
	for _, item := range environment {
		if !strings.HasPrefix(item, "TMUX=") {
			result = append(result, item)
		}
	}
	return result
}

func sessionDisappeared(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	return strings.Contains(message, "can't find session") ||
		strings.Contains(message, "can't find window") ||
		strings.Contains(message, "no such session")
}

// IsTargetAbsent reports that an operation lost a race with removal of its
// target session, pane, or private server. Callers use it to make unleased
// collection idempotent without hiding unrelated tmux command failures.
//
// "can't find pane" covers a pane that vanished between a pane list and a
// later capture-pane against the pane id that list returned (GH #36): the
// pane id is server-global, so a session/window teardown that removed it
// concurrently is reported by tmux against the pane, not the session, and is
// exactly as much a removal race as "can't find session"/"can't find window"
// already are.
func IsTargetAbsent(err error) bool {
	if sessionDisappeared(err) {
		return true
	}
	message := err.Error()
	return strings.Contains(message, "can't find pane") ||
		strings.Contains(message, "no server running") ||
		strings.Contains(message, "no sessions") ||
		strings.Contains(message, "no current target") ||
		strings.Contains(message, "error connecting to") && strings.Contains(message, "No such file or directory")
}

func (c Client) session(ctx context.Context, name string) (Session, error) {
	output, err := c.run(ctx, "list-panes", "-t", name, "-F", "#{pane_id}|#{pane_current_path}|#{pane_pid}|#{pane_dead}|#{pane_dead_status}|#{pane_dead_signal}|#{pane_current_command}|#{pane_width}|#{pane_height}")
	if err != nil {
		return Session{}, fmt.Errorf("list panes for session %q: %w", name, err)
	}
	session := Session{Name: name}
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Split(line, "|")
		if len(fields) != 9 {
			return Session{}, fmt.Errorf("parse pane facts for session %q: %q", name, line)
		}
		pid, err := strconv.Atoi(fields[2])
		if err != nil {
			return Session{}, fmt.Errorf("parse pane PID for session %q: %w", name, err)
		}
		width, err := strconv.Atoi(fields[7])
		if err != nil {
			return Session{}, fmt.Errorf("parse pane width for session %q: %w", name, err)
		}
		height, err := strconv.Atoi(fields[8])
		if err != nil {
			return Session{}, fmt.Errorf("parse pane height for session %q: %w", name, err)
		}
		pane := Pane{ID: fields[0], CurrentPath: fields[1], PID: pid, Dead: fields[3] == "1", Command: fields[6], Width: width, Height: height}
		if fields[4] != "" {
			status, err := strconv.Atoi(fields[4])
			if err != nil {
				return Session{}, fmt.Errorf("parse pane exit status for session %q: %w", name, err)
			}
			pane.DeadStatus = &status
		} else if fields[5] != "" {
			// tmux reports signal deaths separately from ordinary exit status.
			// Preserve the conventional shell status (128 + signal) so SIGKILL
			// remains a nonzero crash observation instead of an unclassified corpse.
			signal, err := strconv.Atoi(fields[5])
			if err != nil {
				return Session{}, fmt.Errorf("parse pane death signal for session %q: %w", name, err)
			}
			status := 128 + signal
			pane.DeadStatus = &status
		}
		session.Panes = append(session.Panes, pane)
	}
	return session, nil
}

func pairs(values []string) func(func(string, string) bool) {
	return func(yield func(string, string) bool) {
		for index := 0; index < len(values); index += 2 {
			if !yield(values[index], values[index+1]) {
				return
			}
		}
	}
}

// historyLimit is the scrollback depth (in lines) deck sets on its own tmux
// server via Bootstrap, replacing tmux's own default of 2000. SPEC.md:177
// (II-15/requirement 15) requires an explicit, non-default value because
// §11.9's interactive mode narrows the window, and output produced while
// narrow consumes history rows roughly 2.7x faster than at full width --
// rows evicted past the limit never return (measured: 23 of 40 logical
// lines destroyed at history-limit 100 where an unnarrowed control lost
// none). 10000 is comfortably larger than that failure mode's regime while
// staying far short of the per-line memory cost becoming a real budget
// concern; it is not itself a spike-measured number, and that is recorded
// in docs/reports/phase3b.md.
const historyLimit = 10000

func (c Client) Bootstrap(ctx context.Context) error {
	if c.Socket == "" {
		return errors.New("tmux socket name is required")
	}
	bootstrapCtx, cancel := context.WithTimeout(ctx, c.timeout())
	defer cancel()
	// mouse is set in this same single invocation, on deck's own -L socket
	// only, alongside the other server options -- never touching the user's
	// default socket or ~/.tmux.conf (c.command always carries -L c.Socket).
	mouseState := "off"
	if c.Mouse {
		mouseState = "on"
	}
	// history-limit is set here, in this same single invocation, alongside
	// exit-empty/remain-on-exit/window-size/mouse -- and nowhere else (grep
	// for "history-limit" in internal/tmux proves it). A pane's history-limit
	// is fixed at the moment the pane is created, so this must land before
	// Create's new-session call; Bootstrap is always invoked from Create
	// before that new-session, which satisfies the ordering requirement.
	output, err := c.command(bootstrapCtx,
		"start-server", ";",
		"set-option", "-s", "exit-empty", "off", ";",
		"set-option", "-g", "remain-on-exit", "failed", ";",
		"set-option", "-g", "window-size", "latest", ";",
		"set-option", "-g", "mouse", mouseState, ";",
		"set-option", "-g", "history-limit", strconv.Itoa(historyLimit), ";",
		"set-window-option", "-g", "aggressive-resize", "on",
	).CombinedOutput()
	if err != nil {
		return fmt.Errorf("bootstrap tmux server on socket %q: %w: %s", c.Socket, err, strings.TrimSpace(string(output)))
	}
	return nil
}
