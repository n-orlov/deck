package tmux

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
)

// OwnershipOption is the window-scoped user option interactive mode claims
// before it acts on a tmux window (PRD phase3b II-14, SPEC.md §11.9). It is
// a WINDOW option, never a global or session one: two different deck
// sessions selecting two different windows must be able to hold ownership
// simultaneously, and scoping to the window is what makes that possible.
const OwnershipOption = "@deck_isize_owner"

// maxOwnershipClaimAttempts bounds the read/compare-and-set loop in
// ClaimWindowOwnership. It is not a retry-until-it-works loop for a live
// contended option -- a live owner is respected on its very first read,
// before this call ever writes anything -- it only bounds how many times a
// compare-and-set can fail because a concurrent writer changed the option
// between this call's read and its compare-and-set (each loss loops back
// to a fresh read, which validates the winning writer's liveness in
// turn). A small fixed bound, not a timer, is enough to make that
// converge; there is deliberately no heartbeat and no TTL anywhere in this
// file (every writer of a tmux socket runs on the socket's own host, so
// liveness is a kill(pid, 0) syscall rather than a lease -- PRD II-14).
const maxOwnershipClaimAttempts = 3

// WindowOwnership is a confirmed claim to OwnershipOption on one tmux
// window, held by this process. It is returned only once
// ClaimWindowOwnership's confirm-read has proven the claim survived any
// concurrent writer; Release is the only way to give it up early (it is
// also implicitly given up, from tmux's point of view, whenever a later
// claimant's confirm-read observes this process's pid as dead).
type WindowOwnership struct {
	client Client
	target string
	claim  string
}

// Target is the tmux target this ownership was claimed against.
func (o *WindowOwnership) Target() string { return o.target }

// ownershipClaimTag returns a fresh, cryptographically random 16 hex
// character tag for one ClaimWindowOwnership call. The pid alone identifies
// which OS process holds a claim, but not which specific call inside that
// process made it; a fresh tag per attempt lets the confirm-read tell "the
// exact value I just wrote is still there" apart from "some other write
// landed here that happens to carry my own pid" (e.g. two goroutines in the
// same deck process racing to enter interactive mode on the same window).
func ownershipClaimTag() (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate ownership claim tag: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// formatOwnershipClaim renders the PRD II-14 `<tag>:<pid>` form.
func formatOwnershipClaim(tag string, pid int) string {
	return fmt.Sprintf("%s:%d", tag, pid)
}

// parseOwnershipClaim splits a `<tag>:<pid>` value on its LAST colon (the
// tag is hex and never contains one, but splitting on the last occurrence
// keeps this robust the same way internal/store/lease.go's parseLeaseOwner
// does for its own not-dissimilar pid-tagged format).
func parseOwnershipClaim(value string) (tag string, pid int, ok bool) {
	at := strings.LastIndex(value, ":")
	if at < 0 {
		return "", 0, false
	}
	tagPart, pidPart := value[:at], value[at+1:]
	parsed, err := strconv.Atoi(pidPart)
	if err != nil || parsed <= 0 || tagPart == "" {
		return "", 0, false
	}
	return tagPart, parsed, true
}

// pidAlive answers tmux's II-14 "kill(pid, 0)" liveness question: does a
// process with this pid exist and belong to us. Signal(syscall.Signal(0))
// asks the kernel whether the process exists without delivering a signal --
// the same probe internal/store/lease.go's leaseOwnerAlive uses for the
// launch lease, minus that lease's extra boot-id check (there is nothing
// here as long-lived as a lease's TTL window for a pid to be reused across,
// and II-14 asks for exactly the syscall, not a lease).
func pidAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	if err == nil {
		return true
	}
	if errors.Is(err, os.ErrProcessDone) {
		return false
	}
	// EPERM means the pid exists but is owned by someone else; deck's own
	// processes always run as the current user, so a pid we cannot signal
	// was never one of ours and its claim is stale.
	return false
}

// windowOwnershipState is a read of OwnershipOption in the window scope.
// Both of tmux's "unset" shapes for a "@"-prefixed user option collapse to
// Set == false here: `show-options -w` on a window where it was never set
// exits non-zero with "invalid option: <name>" (unlike an unset BUILTIN
// window option, which prints an empty line and exits 0 -- task 028's
// distinction; OwnershipOption is a user option, so only the error shape
// ever applies to it, but this function does not assume that and treats an
// empty successful read as unset too).
//
// Present is narrower than "holds a claim": it reports whether the window
// scope holds the option at all, a value explicitly set to the empty string
// (which show-options prints as an empty quoted pair) included. tmux's
// format lookup stops at a present window option whatever it holds, so only
// an absent one lets the global-window, session or global scope through.
type windowOwnershipState struct {
	Value   string
	Set     bool
	Present bool
}

func (c Client) readWindowOwnership(ctx context.Context, target string) (windowOwnershipState, error) {
	commandCtx, cancel := context.WithTimeout(ctx, c.timeout())
	defer cancel()
	output, err := c.command(commandCtx, "show-options", "-w", "-t", target, OwnershipOption).CombinedOutput()
	return c.parseWindowOwnershipRead(output, err, "show-options -w -t "+target+" "+OwnershipOption)
}

// parseWindowOwnershipRead turns the output of a tmux invocation whose last
// (or only) command is `show-options -w -t <target> OwnershipOption` into a
// windowOwnershipState, mapping tmux's "invalid option" error shape to unset.
//
// The value is read verbatim: `show-options -wv` and `display-message` print a
// control character (a newline included) as `_`, so the read uses the
// escaped `name value` form and decodes it (decodeShowOptionsValue). Only the
// one newline tmux prints after that line is removed; a newline the stored
// value itself ends in is part of the value.
func (c Client) parseWindowOwnershipRead(output []byte, err error, what string) (windowOwnershipState, error) {
	if err != nil {
		trimmed := strings.TrimRight(string(output), "\n")
		if strings.Contains(trimmed, "invalid option") {
			return windowOwnershipState{}, nil
		}
		return windowOwnershipState{}, fmt.Errorf("tmux -L %s %s: %w: %s", c.Socket, what, err, trimmed)
	}
	if len(output) == 0 {
		return windowOwnershipState{}, nil
	}
	value, decodeErr := decodeShowOptionsValue(strings.TrimSuffix(string(output), "\n"), OwnershipOption)
	if decodeErr != nil {
		return windowOwnershipState{}, fmt.Errorf("tmux -L %s %s: %w", c.Socket, what, decodeErr)
	}
	if value == "" {
		return windowOwnershipState{Present: true}, nil
	}
	return windowOwnershipState{Value: value, Set: true, Present: true}, nil
}

// decodeShowOptionsValue recovers the stored string from one line of tmux's
// `show-options` output (`<name> <escaped value>`, the terminating newline
// already removed). tmux renders the value with its args_escape: the empty
// string is `”`; a value holding a space, `#`, `'`, `;`, `$`, `{`, `}` or `%`
// is wrapped in double quotes, one holding only a space or `"` in single
// quotes; a leading `~` and a one-character value that needs quoting are
// backslash-escaped; and inside, vis(3)'s C-style escapes stand for control
// characters (`\n`, `\t`, `\\`, `\ooo` octal, ...), so no raw newline reaches the line.
func decodeShowOptionsValue(line, name string) (string, error) {
	body, found := strings.CutPrefix(line, name+" ")
	if !found {
		return "", fmt.Errorf("unexpected show-options output %q for %s", line, name)
	}
	if len(body) >= 2 && (body[0] == '"' || body[0] == '\'') && body[len(body)-1] == body[0] {
		body = body[1 : len(body)-1]
	}
	return unvisCStyle(body), nil
}

// visSimpleEscapes maps the letter after a backslash to the control character
// vis(3)'s C-style escaping wrote it for.
var visSimpleEscapes = map[byte]byte{
	'n': '\n', 'r': '\r', 't': '\t', 'a': '\a', 'b': '\b', 'f': '\f', 'v': '\v', 's': ' ', 'E': 0x1b,
}

// unvisCStyle undoes vis(3)'s VIS_CSTYLE|VIS_OCTAL escaping as tmux applies it.
func unvisCStyle(escaped string) string {
	var out strings.Builder
	for i := 0; i < len(escaped); i++ {
		ch := escaped[i]
		if ch != '\\' || i+1 == len(escaped) {
			out.WriteByte(ch)
			continue
		}
		i++
		if isOctalDigit(escaped[i]) {
			value, consumed := parseOctalEscape(escaped[i:])
			out.WriteByte(value)
			i += consumed - 1
			continue
		}
		if mapped, ok := visSimpleEscapes[escaped[i]]; ok {
			out.WriteByte(mapped)
			continue
		}
		out.WriteByte(escaped[i])
	}
	return out.String()
}

func isOctalDigit(ch byte) bool { return ch >= '0' && ch <= '7' }

// parseOctalEscape reads up to three octal digits from the start of s and
// returns their value and how many digits it consumed.
func parseOctalEscape(s string) (value byte, consumed int) {
	for consumed < 3 && consumed < len(s) && isOctalDigit(s[consumed]) {
		value = value*8 + (s[consumed] - '0')
		consumed++
	}
	return value, consumed
}

// ownershipShadowScopes are the scopes, in format-lookup order, at which a
// value of OwnershipOption can sit in front of the window's own option (or in
// front of its absence): the read that finds one is naming where it is.
var ownershipShadowScopes = []struct {
	name string
	args []string
}{
	{"pane-scope", []string{"-p"}},
	{"global-window-scope", []string{"-wg"}},
	{"session-scope", nil},
	{"global-scope", []string{"-g"}},
}

// findOwnershipShadow names the scope whose OwnershipOption value the
// compare-and-set condition would read instead of the window's. A pane value
// always shadows; with the window option absent from the window scope (not
// merely empty: a present empty value ends the lookup) a value at the global
// window, session or global scope is what the lookup falls back to.
func (c Client) findOwnershipShadow(ctx context.Context, target string, window windowOwnershipState) (string, error) {
	for _, scope := range ownershipShadowScopes {
		if window.Present && scope.name != "pane-scope" {
			continue
		}
		commandCtx, cancel := context.WithTimeout(ctx, c.timeout())
		args := append([]string{"show-options"}, scope.args...)
		args = append(args, "-v", "-t", target, OwnershipOption)
		output, err := c.command(commandCtx, args...).CombinedOutput()
		cancel()
		trimmed := strings.TrimRight(string(output), "\n")
		if err == nil {
			return scope.name, nil
		}
		if !strings.Contains(trimmed, "invalid option") {
			return "", fmt.Errorf("tmux -L %s %s: %w: %s", c.Socket, strings.Join(args, " "), err, trimmed)
		}
	}
	return "", nil
}

// ownershipShadow reports the scope shadowing the window's OwnershipOption
// for target ("" when none does). It asks tmux which scope holds the option
// (findOwnershipShadow) and never infers it from comparing values: a shadow
// holding the very value the window holds, or a value tmux's display forms
// print differently (a control character reads `_`), shadows just the same.
func (c Client) ownershipShadow(ctx context.Context, target string, window windowOwnershipState) (string, error) {
	return c.findOwnershipShadow(ctx, target, window)
}

func shadowedOwnershipError(verb, target, scope string) error {
	return fmt.Errorf("%s %s on %q: a %s %s value shadows the window option, so the compare-and-set could not match it (unset it with `set-option -u`)", verb, OwnershipOption, target, scope, OwnershipOption)
}

func (c Client) writeWindowOwnership(ctx context.Context, target, value string) error {
	if _, err := c.run(ctx, "set-option", "-w", "-t", target, OwnershipOption, value); err != nil {
		return fmt.Errorf("claim %s on %q: %w", OwnershipOption, target, err)
	}
	return nil
}

func (c Client) unsetWindowOwnership(ctx context.Context, target string) error {
	if _, err := c.run(ctx, "set-option", "-w", "-u", "-t", target, OwnershipOption); err != nil {
		return fmt.Errorf("release %s on %q: %w", OwnershipOption, target, err)
	}
	return nil
}

// formatLiteralEscaper escapes a literal so it can stand as one operand of a
// tmux format comparison (`#{==:a,b}`): `#` would start an expansion, `,`
// would end the operand and `}` would close the conditional, so each gets
// tmux's own `#` escape.
var formatLiteralEscaper = strings.NewReplacer("#", "##", ",", "#,", "}", "#}")

// quoteTmuxCommandArg single-quotes one argument of a command string that
// tmux's own command parser will parse (if-shell's command argument). Like a
// POSIX shell, tmux's parser joins adjacent quoted and escaped pieces into
// one token, so each embedded single quote closes the quoted piece, is
// emitted backslash-escaped, and reopens it.
func quoteTmuxCommandArg(arg string) string {
	return "'" + strings.ReplaceAll(arg, "'", `'\''`) + "'"
}

// compareAndSetWindowOwnership is the single atomic step every non-forced
// change to OwnershipOption goes through: in ONE tmux invocation, set
// OwnershipOption on target to next (or unset it when next is "") if and
// only if it currently reads exactly expected, then read it back.
//
// The atomicity comes from the tmux server, not from this process. The
// invocation is one command list (`if-shell -F <cond> <set> ; show-options`)
// queued on one client; the server is single-threaded, and `if-shell -F`
// evaluates its format condition synchronously and inserts its command
// directly after itself in that same queue, which the server drains before
// it runs any other client's command. So no other writer can land between
// the comparison and the set -- which a read and a write issued as two
// separate tmux processes (the protocol this replaced) cannot promise.
//
// The comparison reads `#{@deck_isize_owner}`, which expands to "" when the
// option is unset, so an unset expected compares against "". (A format
// lookup reads a pane value first and falls back to the global-window,
// session and global scopes when the window option is absent, where
// show-options -w does not; callers ask findOwnershipShadow first and refuse
// rather than compare against such a value.) The trailing
// show-options is the confirm-read: it reports what the option holds after
// the conditional, which is how the caller learns whether its set took
// effect (the option reads exactly next) or the comparison failed because
// somebody else changed the option first.
func (c Client) compareAndSetWindowOwnership(ctx context.Context, target string, expected windowOwnershipState, next string) (windowOwnershipState, error) {
	condition := "#{==:#{" + OwnershipOption + "}," + formatLiteralEscaper.Replace(expected.Value) + "}"
	set := "set-option -w -t " + quoteTmuxCommandArg(target) + " " + OwnershipOption + " " + quoteTmuxCommandArg(next)
	if next == "" {
		set = "set-option -w -u -t " + quoteTmuxCommandArg(target) + " " + OwnershipOption
	}
	commandCtx, cancel := context.WithTimeout(ctx, c.timeout())
	defer cancel()
	output, err := c.command(commandCtx,
		"if-shell", "-F", "-t", target, condition, set,
		";", "show-options", "-w", "-t", target, OwnershipOption,
	).CombinedOutput()
	return c.parseWindowOwnershipRead(output, err, "compare-and-set "+OwnershipOption+" on "+target)
}

// ClaimWindowOwnership implements PRD phase3b II-14 / SPEC.md §11.9's
// interactive-mode ownership protocol for one tmux window:
//
//  1. Read the option first. If it names a LIVE owner (kill(pid, 0)
//     succeeds), stand down without writing anything -- a live owner is
//     respected untouched, never raced against.
//  2. Otherwise (unset, or a DEAD owner) replace exactly the value just read
//     with this process's fresh pid-tagged `<tag>:<pid>` claim, as ONE
//     atomic tmux compare-and-set that also confirm-reads the result
//     (compareAndSetWindowOwnership). Of any number of concurrent claimants
//     that all read the same unset or dead value, the tmux server applies
//     exactly one compare-and-set; every other comparison fails and leaves
//     the winner's value untouched.
//  3. If the confirm-read shows exactly the value just written, the claim
//     is acquired. If it shows something else, the option changed between
//     this call's read and its compare-and-set; loop back to step 1, which
//     will validate THAT writer's liveness in turn -- a live winner is
//     respected on the very next iteration, not thrashed against.
//
// The three returns are: the held ownership (nil unless acquired), whether
// it was acquired at all, and an error only for a genuine tmux/transport
// failure (never for losing the claim to a live owner -- that is a normal,
// error-free stand-down).
func (c Client) ClaimWindowOwnership(ctx context.Context, target string) (*WindowOwnership, bool, error) {
	tag, err := ownershipClaimTag()
	if err != nil {
		return nil, false, err
	}
	mine := formatOwnershipClaim(tag, os.Getpid())
	for attempt := 0; attempt < maxOwnershipClaimAttempts; attempt++ {
		outcome, err := c.attemptWindowClaim(ctx, target, mine)
		if err != nil {
			return nil, false, err
		}
		switch outcome {
		case claimAcquired:
			return &WindowOwnership{client: c, target: target, claim: mine}, true, nil
		case claimStoodDown:
			return nil, false, nil
		}
		// claimRaceLost: loop back to step 1 and let the next iteration's
		// read validate that writer's liveness instead of trusting our own
		// just-issued write.
	}
	return nil, false, fmt.Errorf("claim %s on %q: gave up after %d attempts racing a concurrent writer", OwnershipOption, target, maxOwnershipClaimAttempts)
}

// claimAttemptOutcome is how one read/write/confirm-read pass of
// ClaimWindowOwnership ended.
type claimAttemptOutcome int

const (
	// claimRaceLost: this pass wrote its claim but the confirm-read showed
	// another value; the caller loops back to a fresh read.
	claimRaceLost claimAttemptOutcome = iota
	// claimAcquired: the confirm-read showed exactly the value written.
	claimAcquired
	// claimStoodDown: a live owner holds the option; nothing was written.
	claimStoodDown
)

// attemptWindowClaim is steps 1-3 of ClaimWindowOwnership's protocol, once:
// read, stand down for a live owner, otherwise atomically replace exactly the
// value read with mine and confirm-read the result.
func (c Client) attemptWindowClaim(ctx context.Context, target, mine string) (claimAttemptOutcome, error) {
	got, err := c.readWindowOwnership(ctx, target)
	if err != nil {
		return claimRaceLost, err
	}
	// An unset option, an unparseable value (a bug or a hand-edited option,
	// never something formatOwnershipClaim itself produces) and a
	// confirmed-dead owner are all stolen from by falling through to the
	// compare-and-set below, which only takes effect if the option still
	// holds exactly the value whose deadness was just judged.
	// A value at another scope is what the condition below would compare, not
	// what was just read: refuse to judge or write over it.
	scope, err := c.ownershipShadow(ctx, target, got)
	if err != nil {
		return claimRaceLost, err
	}
	if scope != "" {
		return claimRaceLost, shadowedOwnershipError("claim", target, scope)
	}
	if holdsLiveClaim(got) {
		return claimStoodDown, nil
	}
	confirm, err := c.compareAndSetWindowOwnership(ctx, target, got, mine)
	if err != nil {
		return claimRaceLost, err
	}
	if confirm.Set && confirm.Value == mine {
		return claimAcquired, nil
	}
	return claimRaceLost, nil
}

// holdsLiveClaim reports whether the option names a parseable claim whose pid
// is alive.
func holdsLiveClaim(state windowOwnershipState) bool {
	if !state.Set {
		return false
	}
	_, pid, ok := parseOwnershipClaim(state.Value)
	return ok && pidAlive(pid)
}

// ForceClaimWindowOwnership implements a single-shot variant of
// ClaimWindowOwnership for a force-attach path (SPEC.md §11.9's force
// override): it never reads the option first and never consults pidAlive
// -- a live owner is not respected here by design, because the caller has
// already decided to override whatever is there. It writes this process's
// fresh pid-tagged `<tag>:<pid>` claim exactly once and confirm-reads it
// exactly once, with no retry loop: if the confirm-read shows exactly the
// value just written, the claim is acquired; if it shows anything else
// (a concurrent writer landed between the write and the confirm-read),
// this call simply lost that single race and returns acquired=false with
// a nil error -- never an error, because losing a race to another writer
// is not a transport failure. An error return is reserved for a genuine
// tmux/transport failure from the write or the confirm-read themselves.
func (c Client) ForceClaimWindowOwnership(ctx context.Context, target string) (*WindowOwnership, bool, error) {
	tag, err := ownershipClaimTag()
	if err != nil {
		return nil, false, err
	}
	mine := formatOwnershipClaim(tag, os.Getpid())
	if err := c.writeWindowOwnership(ctx, target, mine); err != nil {
		return nil, false, err
	}
	confirm, err := c.readWindowOwnership(ctx, target)
	if err != nil {
		return nil, false, err
	}
	if confirm.Set && confirm.Value == mine {
		return &WindowOwnership{client: c, target: target, claim: mine}, true, nil
	}
	return nil, false, nil
}

// ClaimState is the tri-state answer to "what does OwnershipOption read on
// this window RIGHT NOW", from the point of view of a caller who may or
// may not hold a claim there (SPEC.md §11.9's R100 teardown gating, R101
// tick poll and R102 previewFit stand-down all classify a window this way
// before deciding whether to act on it).
type ClaimState int

const (
	// ClaimUnset means OwnershipOption carries no live claim: either it
	// was never set (windowOwnershipState.Set == false, tmux's "invalid
	// option" or an empty successful read) or it names a pid that is no
	// longer alive -- the same "unset or dead owner" grouping
	// ClaimWindowOwnership's own doc comment already treats as
	// equivalent (both fall through to a write there; here, both fall
	// through to this answer).
	ClaimUnset ClaimState = iota
	// ClaimForeignLive means OwnershipOption is set to a value this call
	// does not recognise as its own, naming a pid that answers
	// pidAlive -- some other live process holds the window now (most
	// obviously a ForceClaimWindowOwnership steal, but also an ordinary
	// ClaimWindowOwnership by another process).
	ClaimForeignLive
	// ClaimStillMine means OwnershipOption reads exactly the value a
	// specific WindowOwnership claimed -- nobody has stolen it. Only
	// WindowOwnership.Probe, which knows that value, can ever return
	// this; the target-only Client.ProbeWindowOwnership never does.
	ClaimStillMine
)

// String renders a ClaimState the way test failures and any future log
// line should name it.
func (s ClaimState) String() string {
	switch s {
	case ClaimUnset:
		return "unset"
	case ClaimForeignLive:
		return "foreign-live"
	case ClaimStillMine:
		return "still-mine"
	default:
		return fmt.Sprintf("ClaimState(%d)", int(s))
	}
}

// classifyWindowOwnership reads OwnershipOption on target and classifies
// it against mine (the empty string when the caller holds no claim of its
// own to compare against -- Client.ProbeWindowOwnership's case): unset (or
// a dead owner), a live claim that is not mine, or mine exactly. This is
// the one implementation shared by both exported probes below; neither
// adds a branch of its own.
func (c Client) classifyWindowOwnership(ctx context.Context, target, mine string) (ClaimState, error) {
	got, err := c.readWindowOwnership(ctx, target)
	if err != nil {
		return ClaimUnset, err
	}
	return classifyOwnershipState(got, mine), nil
}

// classifyOwnershipState is the pure half of classifyWindowOwnership: the
// classification of an already-read OwnershipOption value, shared with the
// interactive tick's combined read (Client.InteractiveTickRead), which
// obtains the value together with pane_dead and session_attached in one
// tmux process instead of a show-options of its own.
func classifyOwnershipState(got windowOwnershipState, mine string) ClaimState {
	if !got.Set {
		return ClaimUnset
	}
	if mine != "" && got.Value == mine {
		return ClaimStillMine
	}
	_, pid, ok := parseOwnershipClaim(got.Value)
	if !ok || !pidAlive(pid) {
		return ClaimUnset
	}
	return ClaimForeignLive
}

// ProbeWindowOwnership answers whether OwnershipOption on target is unset
// or held by a live foreign pid, for a caller that holds no claim of its
// own on target to compare against (SPEC.md §11.9's R102: previewFit
// checks a session's window it has never claimed). It never returns
// ClaimStillMine -- there is nothing here to be "mine" -- so a caller that
// DOES hold a claim wants WindowOwnership.Probe instead.
func (c Client) ProbeWindowOwnership(ctx context.Context, target string) (ClaimState, error) {
	return c.classifyWindowOwnership(ctx, target, "")
}

// Probe answers, for this WindowOwnership's own target, which of the
// three claim states currently holds: OwnershipOption unset, held by a
// live foreign pid, or still exactly this WindowOwnership's own confirmed
// claim. It is the read-only counterpart to
// ClaimWindowOwnership/ForceClaimWindowOwnership that R100's teardown
// gating and R101's tick poll both need before acting: neither may
// restore geometry, clear @deck_isize_geometry or release ownership
// unless this reads ClaimStillMine first.
func (o *WindowOwnership) Probe(ctx context.Context) (ClaimState, error) {
	return o.client.classifyWindowOwnership(ctx, o.target, o.claim)
}

// Release gives up this ownership, unsetting OwnershipOption on its target
// -- but only if the option still reads exactly the claim this call made.
// If it does not (a later claimant already validated this process as dead
// and stole it, or Release raced a steal in progress), unsetting would
// clobber that later claim instead of merely tidying up this one, so
// Release leaves it alone. The check and the unset are one atomic tmux
// compare-and-set, so a steal can never land between them.
func (o *WindowOwnership) Release(ctx context.Context) error {
	// A value at another scope the format lookup reads before the window's
	// decides the comparison whatever it holds -- and whatever the window
	// holds: a shadow equal to this claim would let the compare-and-set unset
	// a window option a later claimant replaced it with. So a release that
	// finds one refuses before writing, the window option left untouched.
	current, err := o.client.readWindowOwnership(ctx, o.target)
	if err != nil {
		return err
	}
	if err := o.refuseShadowedRelease(ctx, current); err != nil {
		return err
	}
	if !current.Set || current.Value != o.claim {
		// Displaced (or already released): the window holds no claim of
		// this call's, so there is nothing of ours to unset.
		return nil
	}
	confirm, err := o.client.compareAndSetWindowOwnership(ctx, o.target, windowOwnershipState{Value: o.claim, Set: true}, "")
	if err != nil {
		return err
	}
	if confirm.Set && confirm.Value == o.claim {
		// The claim is still there: the comparison failed although the window
		// holds it, which only a value at another scope can cause. Report it.
		return o.refuseShadowedRelease(ctx, confirm)
	}
	return nil
}

// refuseShadowedRelease returns the release refusal naming the scope that
// shadows the window option as read in window (nil when none does).
func (o *WindowOwnership) refuseShadowedRelease(ctx context.Context, window windowOwnershipState) error {
	scope, err := o.client.findOwnershipShadow(ctx, o.target, window)
	if err != nil {
		return err
	}
	if scope != "" {
		return shadowedOwnershipError("release", o.target, scope)
	}
	return nil
}

// InteractiveTick is one interactive preview tick's whole tmux read
// (SPEC §2, R185): the pane's liveness, the session's attached-client count
// and the window's OwnershipOption value, obtained together from a single
// `display-message`.
type InteractiveTick struct {
	PaneDead bool
	Attached int
	// Owner is OwnershipOption's value on the target's window, and
	// OwnerSet is false when the option is unset (tmux prints it empty).
	Owner    string
	OwnerSet bool
}

// InteractiveTickRead reads `#{pane_dead}`, `#{session_attached}` and the
// window's `#{@deck_isize_owner}` for target in ONE tmux process. It is the
// whole of interactive mode's per-tick tmux cost: the displacement backstop
// (claim still ours, no other client attached) and the dead-pane check read
// the same answer, so neither needs a process of its own. A vanished target
// is an error, which callers treat as terminal for the pane exactly as
// PaneDead did.
func (c Client) InteractiveTickRead(ctx context.Context, target string) (InteractiveTick, error) {
	commandCtx, cancel := context.WithTimeout(ctx, c.timeout())
	defer cancel()
	format := "#{pane_dead}|#{session_attached}|#{" + OwnershipOption + "}"
	output, err := c.command(commandCtx, "display-message", "-p", "-t", target, format).CombinedOutput()
	if err != nil {
		return InteractiveTick{}, fmt.Errorf("tmux -L %s display-message -p -t %s interactive tick: %w: %s", c.Socket, target, err, strings.TrimSpace(string(output)))
	}
	line := strings.TrimRight(string(output), "\n")
	parts := strings.SplitN(line, "|", 3)
	if len(parts) != 3 {
		return InteractiveTick{}, fmt.Errorf("tmux -L %s display-message -p -t %s interactive tick: unexpected output %q", c.Socket, target, line)
	}
	var tick InteractiveTick
	switch parts[0] {
	case "1":
		tick.PaneDead = true
	case "0":
	default:
		return InteractiveTick{}, fmt.Errorf("tmux -L %s display-message -p -t %s interactive tick: unexpected pane_dead %q", c.Socket, target, parts[0])
	}
	attached, convErr := strconv.Atoi(parts[1])
	if convErr != nil {
		return InteractiveTick{}, fmt.Errorf("tmux -L %s display-message -p -t %s interactive tick: parse session_attached %q: %w", c.Socket, target, parts[1], convErr)
	}
	tick.Attached = attached
	tick.Owner = parts[2]
	tick.OwnerSet = parts[2] != ""
	return tick, nil
}

// StillMine reports whether tick's Owner is exactly this WindowOwnership's
// own confirmed claim: the same answer Probe gives as ClaimStillMine, read
// from a tick that already carries the option instead of a read of its own.
func (o *WindowOwnership) StillMine(tick InteractiveTick) bool {
	return classifyOwnershipState(windowOwnershipState{Value: tick.Owner, Set: tick.OwnerSet}, o.claim) == ClaimStillMine
}
