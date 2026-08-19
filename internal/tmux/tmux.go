package tmux

import (
	"context"
	"errors"
	"fmt"
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
// Binary defaults to "tmux" and Timeout defaults to five seconds.
type Client struct {
	Binary  string
	Socket  string
	Timeout time.Duration
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

func sessionName(slug string) (string, error) {
	if !regexp.MustCompile(`^[a-z0-9_-]+$`).MatchString(slug) {
		return "", fmt.Errorf("invalid session slug %q; use only lowercase letters, digits, _ and -", slug)
	}
	return "deck_" + slug, nil
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
	args := []string{"new-session", "-d", "-s", name, "-c", launch.CWD, "--", "env"}
	for key, value := range pairs(env) {
		args = append(args, key+"="+value)
	}
	args = append(args, launch.Command...)
	if _, err := c.run(ctx, args...); err != nil {
		return Session{}, fmt.Errorf("create session %q: %w", name, err)
	}
	for key, value := range pairs(env) {
		if _, err := c.run(ctx, "set-environment", "-t", name, key, value); err != nil {
			_ = c.Kill(ctx, launch.Slug)
			return Session{}, fmt.Errorf("set environment for session %q: %w", name, err)
		}
	}
	return c.session(ctx, name)
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
	args = append(args, "-S", options.StartLine, "-E", options.EndLine, "-t", paneID)
	output, err := c.run(ctx, args...)
	if err != nil {
		return nil, fmt.Errorf("capture pane %q: %w", paneID, err)
	}
	return output, nil
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
		if tmuxTargetAbsent(err) {
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

func tmuxTargetAbsent(err error) bool {
	if sessionDisappeared(err) {
		return true
	}
	message := err.Error()
	return strings.Contains(message, "no server running") ||
		strings.Contains(message, "no sessions") ||
		strings.Contains(message, "no current target") ||
		strings.Contains(message, "error connecting to") && strings.Contains(message, "No such file or directory")
}

func (c Client) session(ctx context.Context, name string) (Session, error) {
	output, err := c.run(ctx, "list-panes", "-t", name, "-F", "#{pane_id}|#{pane_current_path}|#{pane_pid}|#{pane_dead}|#{pane_dead_status}|#{pane_current_command}")
	if err != nil {
		return Session{}, fmt.Errorf("list panes for session %q: %w", name, err)
	}
	session := Session{Name: name}
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Split(line, "|")
		if len(fields) != 6 {
			return Session{}, fmt.Errorf("parse pane facts for session %q: %q", name, line)
		}
		pid, err := strconv.Atoi(fields[2])
		if err != nil {
			return Session{}, fmt.Errorf("parse pane PID for session %q: %w", name, err)
		}
		pane := Pane{ID: fields[0], CurrentPath: fields[1], PID: pid, Dead: fields[3] == "1", Command: fields[5]}
		if fields[4] != "" {
			status, err := strconv.Atoi(fields[4])
			if err != nil {
				return Session{}, fmt.Errorf("parse pane exit status for session %q: %w", name, err)
			}
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

func (c Client) Bootstrap(ctx context.Context) error {
	if c.Socket == "" {
		return errors.New("tmux socket name is required")
	}
	bootstrapCtx, cancel := context.WithTimeout(ctx, c.timeout())
	defer cancel()
	output, err := c.command(bootstrapCtx,
		"start-server", ";",
		"set-option", "-s", "exit-empty", "off", ";",
		"set-option", "-g", "remain-on-exit", "failed", ";",
		"set-option", "-g", "window-size", "latest", ";",
		"set-window-option", "-g", "aggressive-resize", "on",
	).CombinedOutput()
	if err != nil {
		return fmt.Errorf("bootstrap tmux server on socket %q: %w: %s", c.Socket, err, strings.TrimSpace(string(output)))
	}
	return nil
}
