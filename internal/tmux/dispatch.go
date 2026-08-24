package tmux

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Identity is the five-field tuple PRD phase3b II-28 requires interactive
// mode's input dispatch to verify before every send: the tmux socket
// path, the server's own pid, the pane id, the pane's process id and the
// owning session's name. All five are read together in a single
// display-message invocation (dispatchIdentityFormat) alongside
// #{pane_dead}, never from separately-timed calls tmux could process pane
// output in between.
//
// Identity is deliberately a plain comparable struct (no slices/maps): a
// Dispatcher compares two Identity values with ==, which is what lets
// Send's drift check be exactly "did any of these five fields change",
// with no field-by-field bookkeeping to keep in sync as fields are added.
type Identity struct {
	// SocketPath is #{socket_path}: the real filesystem path of the tmux
	// socket target's server is listening on -- carrying the socket
	// itself in the identity is what PRD II-31 needs to reject a
	// cross-socket pane-id collision.
	SocketPath string
	// ServerPID is #{pid}: the tmux server process's own pid. Pane ids
	// are only guaranteed unique within one server's lifetime (PRD
	// II-31); a restarted server reissuing %0 changes this even when
	// SocketPath happens to be reused.
	ServerPID int
	// PaneID is #{pane_id}, e.g. "%3".
	PaneID string
	// PanePID is #{pane_pid}: the pid of the process tmux itself spawned
	// in the pane. PRD II-29 is the reason this is here at all: a
	// respawn-pane replaces this process while leaving PaneID and
	// SessionName unchanged, so PanePID is the only one of the five that
	// actually moves when the program behind the pane is swapped out
	// from under a dispatcher.
	PanePID int
	// SessionName is #{session_name}. PRD II-30 forbids ever dispatching
	// BY this value (a renamed/reused session name can point at an
	// impostor); it is carried here purely as one more thing that must
	// still match, never as a targeting mechanism.
	SessionName string
}

// dispatchIdentityFormat is read as one display-message call so the five
// identity fields and the pane_dead liveness check that gates every send
// (PRD II-23, II-28) are measured at the same instant, not across two
// invocations tmux could process pane output between.
const dispatchIdentityFormat = "#{socket_path}|#{pid}|#{pane_id}|#{pane_pid}|#{session_name}|#{pane_dead}"

// dispatchIdentityFieldCount is len(strings.Split(dispatchIdentityFormat, "|")).
const dispatchIdentityFieldCount = 6

// resolveIdentity reads target's current five-field Identity plus
// #{pane_dead} in one tmux invocation. It is the ONLY place in this
// package that ever reads dispatchIdentityFormat, so both CaptureIdentity
// (the entry read) and every Dispatcher.Send's re-resolution (PRD II-28's
// "re-resolved immediately before EVERY send") go through the identical
// code path and can never silently drift apart in what they consider the
// identity to be.
func (c Client) resolveIdentity(ctx context.Context, target string) (Identity, bool, error) {
	if c.Socket == "" {
		return Identity{}, false, errors.New("tmux socket name is required")
	}
	if target == "" {
		return Identity{}, false, errors.New("tmux dispatch target is required")
	}
	output, err := c.run(ctx, "display-message", "-p", "-t", target, dispatchIdentityFormat)
	if err != nil {
		return Identity{}, false, fmt.Errorf("resolve dispatch identity for %q: %w", target, err)
	}
	fields := strings.Split(strings.TrimRight(string(output), "\n"), "|")
	if len(fields) != dispatchIdentityFieldCount {
		return Identity{}, false, fmt.Errorf("resolve dispatch identity for %q: got %d fields, want %d: %q", target, len(fields), dispatchIdentityFieldCount, string(output))
	}
	serverPID, err := strconv.Atoi(strings.TrimSpace(fields[1]))
	if err != nil {
		return Identity{}, false, fmt.Errorf("resolve dispatch identity for %q: parse server pid %q: %w", target, fields[1], err)
	}
	panePID, err := strconv.Atoi(strings.TrimSpace(fields[3]))
	if err != nil {
		return Identity{}, false, fmt.Errorf("resolve dispatch identity for %q: parse pane_pid %q: %w", target, fields[3], err)
	}
	identity := Identity{
		SocketPath:  fields[0],
		ServerPID:   serverPID,
		PaneID:      fields[2],
		PanePID:     panePID,
		SessionName: fields[4],
	}
	dead := strings.TrimSpace(fields[5]) == "1"
	return identity, dead, nil
}

// CaptureIdentity reads target's dispatch identity once, at interactive
// mode's entry (PRD II-28: "captured at entry"). The returned Identity is
// exactly what every later Dispatcher.Send call re-resolves and compares
// against; it refuses (rather than returning a half-formed Identity) if
// target is already dead at capture time, since there would then be
// nothing live to dispatch to in the first place.
func (c Client) CaptureIdentity(ctx context.Context, target string) (Identity, error) {
	identity, dead, err := c.resolveIdentity(ctx, target)
	if err != nil {
		return Identity{}, err
	}
	if dead {
		return Identity{}, fmt.Errorf("capture dispatch identity for %q: pane is already dead", target)
	}
	return identity, nil
}

// ErrIdentityDrifted is the error Send/Verify wrap when target's
// re-resolved identity no longer equals the one captured at entry. PRD
// II-29's respawn-pane scenario is the case this exists to catch:
// PaneID and SessionName survive a respawn unchanged, so it is PanePID
// moving, and nothing else, that makes this fire -- which is the entire
// reason PanePID belongs in the tuple at all.
var ErrIdentityDrifted = errors.New("dispatch target identity drifted since entry")

// ErrPaneDead is the error Send/Verify wrap when target's #{pane_dead}
// reads nonzero at verify time. A dead target is refused, never sent to
// (PRD II-28).
var ErrPaneDead = errors.New("dispatch target pane is dead")

// Dispatcher is the only path PRD II-28 permits interactive-mode input to
// reach a tmux pane through: Send re-resolves the SAME five fields
// captured at construction time immediately before every send, and
// refuses -- returning a distinct, named error instead of silently
// dropping the payload -- on any drift, a dead pane, or the dispatch
// command's own nonzero tmux exit. Every later send primitive (`send-keys
// -l --`, `load-buffer`+`paste-buffer`, etc., tasks 054-060) must build
// its argv and hand it to THIS Send, never call Client.run directly to
// deliver input to a live pane; that single bottleneck is what makes "no
// send path bypasses the verify" (dispatch_test.go's
// TestNoSendPathBypassesTheDispatcherVerify) a grep-checkable property
// rather than a convention nobody enforces.
type Dispatcher struct {
	client   Client
	target   string
	identity Identity

	verifications int
}

// NewDispatcher captures target's identity NOW (PRD II-28: "at entry")
// and returns a Dispatcher that only ever sends to that exact
// socket+server+pane+session tuple for the rest of its life -- a later
// Send never re-derives target from anything but the argv the caller
// itself supplies for the dispatch command; the identity fields are used
// solely to verify, never to build the send's own -t argument.
func NewDispatcher(ctx context.Context, client Client, target string) (*Dispatcher, error) {
	identity, err := client.CaptureIdentity(ctx, target)
	if err != nil {
		return nil, fmt.Errorf("construct dispatcher for %q: %w", target, err)
	}
	return &Dispatcher{client: client, target: target, identity: identity}, nil
}

// Identity is the tuple captured at entry, unchanged for this
// Dispatcher's whole life (Send never updates it -- only ever compares a
// fresh read against it).
func (d *Dispatcher) Identity() Identity { return d.identity }

// Target is the tmux target this Dispatcher verifies and sends against.
func (d *Dispatcher) Target() string { return d.target }

// Verifications is the number of times this Dispatcher has re-resolved
// target's identity, i.e. the number of Send calls it has ever been
// asked to make, refused or not -- exposed so a test can count the
// re-resolutions against the sends directly (PRD II-28's own success
// criterion), rather than intercepting tmux invocations.
func (d *Dispatcher) Verifications() int { return d.verifications }

// Verify re-resolves target's identity and pane_dead state and reports
// whether a Send right now would be refused, without actually running
// any dispatch command. It counts as one re-resolution, exactly like a
// verify performed inside Send -- there is only ever one verify
// implementation (verifyLocked) and both Verify and Send call it.
func (d *Dispatcher) Verify(ctx context.Context) error {
	_, err := d.verify(ctx)
	return err
}

func (d *Dispatcher) verify(ctx context.Context) (Identity, error) {
	d.verifications++
	fresh, dead, err := d.client.resolveIdentity(ctx, d.target)
	if err != nil {
		return Identity{}, err
	}
	if dead {
		return fresh, ErrPaneDead
	}
	if fresh != d.identity {
		return fresh, ErrIdentityDrifted
	}
	return fresh, nil
}

// Send re-resolves target's identity immediately before running args
// against tmux (PRD II-28: "re-resolved immediately before EVERY send"),
// refusing -- and saying why, via a wrapped ErrIdentityDrifted/ErrPaneDead
// or the tmux command's own error -- instead of sending on any drift, a
// dead pane, or the underlying tmux invocation's nonzero exit. args is
// the complete tmux argv for the dispatch command (e.g.
// []string{"send-keys", "-t", target, "-l", "--", payload}); Send runs it
// unmodified through the same Client.run every other tmux invocation in
// this package uses, so an ordinary nonzero exit is reported exactly as
// Client.run already reports one everywhere else.
func (d *Dispatcher) Send(ctx context.Context, args ...string) error {
	if _, err := d.verify(ctx); err != nil {
		return fmt.Errorf("refuse dispatch to %q: %w", d.target, err)
	}
	if _, err := d.client.run(ctx, args...); err != nil {
		return fmt.Errorf("dispatch to %q: %w", d.target, err)
	}
	return nil
}
