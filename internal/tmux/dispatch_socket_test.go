// dispatch_socket_test.go proves PRD phase3b II-31: "carry the socket and
// a server-lifetime discriminator". Pane ids are only unique WITHIN one
// tmux server's lifetime -- they collide across independent sockets, and
// a restarted server on the SAME socket path reissues the exact same
// ids. Two halves:
//
//   - the raw hazard, demonstrated against bare tmux with no deck code
//     involved at all: a bare send-keys naming only a pane id ("%0")
//     against the WRONG server (a different -L socket) succeeds with
//     exit=0 and silently lands in that server's own local pane, which
//     has nothing to do with the pane the caller actually meant.
//   - deck's discriminator: Identity carries SocketPath (#{socket_path})
//     and ServerPID (#{pid}) alongside PaneID, so two identities with an
//     identical PaneID from different servers -- or the same server
//     across a restart -- are never equal, and Dispatcher.Send refuses
//     (ErrIdentityDrifted) rather than silently dispatching to whichever
//     local pane happens to hold that id now.
package tmux

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func dispatchSocketTestName(name string) string {
	return fmt.Sprintf("deck-dispatch-socket-%s-%d-%d", name, os.Getpid(), time.Now().UnixNano())
}

// killServerOnSocket kills whatever tmux server (if any) is listening on
// socket, ignoring the error -- used both for test cleanup and, in the
// restart test, deliberately mid-test to force a genuinely NEW server
// process onto the same socket path.
func killServerOnSocket(socket string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = exec.CommandContext(ctx, "tmux", "-L", socket, "kill-server").Run()
}

// TestCrossSocketPaneIDCollisionSilentlyHitsTheLocalPane is the raw
// hazard: two entirely independent tmux servers (different -L sockets),
// each freshly started, each assign their own first pane the same id,
// "%0" -- tmux's ids are only unique within one server's life, never
// across servers. A bare send-keys that names only the pane id, aimed at
// the WRONG socket, succeeds (exit=0, no stderr) and delivers the
// payload into that OTHER server's own "%0" -- a pane the caller never
// intended to touch -- while the intended pane never sees it at all.
// This is demonstrated with no deck code in the loop whatsoever: it is a
// property of tmux itself, which is exactly why deck's Identity must
// carry the socket (PRD II-31).
func TestCrossSocketPaneIDCollisionSilentlyHitsTheLocalPane(t *testing.T) {
	socketA := dispatchSocketTestName("cross-a")
	socketB := dispatchSocketTestName("cross-b")
	cleanupA := newBareGeometrySession(t, socketA, "s0", 40, 10)
	defer cleanupA()
	cleanupB := newBareGeometrySession(t, socketB, "s0", 40, 10)
	defer cleanupB()

	paneA := runTmux(t, socketA, "display-message", "-p", "-t", "s0", "#{pane_id}")
	paneB := runTmux(t, socketB, "display-message", "-p", "-t", "s0", "#{pane_id}")
	if paneA != "%0" || paneB != "%0" {
		t.Fatalf("paneA=%q paneB=%q, want both %%0 (first pane of a fresh server) -- test setup invalid, the collision this test relies on didn't happen", paneA, paneB)
	}

	marker := "deck-cross-socket-marker-24680"

	// A caller that remembered only the pane id "%0" (not which socket
	// it came from) and, by mistake, ends up dispatching through
	// socket B's server instead of socket A's -- the server they
	// actually meant -- gets no error at all.
	stdout, stderr, code := runTmuxRaw(t, socketB, "send-keys", "-t", "%0", "-l", "--", marker)
	if code != 0 {
		t.Fatalf("cross-socket send-keys exit=%d stdout=%q stderr=%q, want exit=0 -- the whole hazard is that tmux never complains", code, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("cross-socket send-keys stderr=%q, want empty -- tmux gives no warning that %%0 means something different on this socket", stderr)
	}

	deadline := time.Now().Add(1 * time.Second)
	var landedOnB string
	for time.Now().Before(deadline) {
		landedOnB = runTmux(t, socketB, "capture-pane", "-p", "-t", "s0")
		if strings.Contains(landedOnB, marker) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !strings.Contains(landedOnB, marker) {
		t.Fatalf("socket B pane capture = %q, want it to contain the marker %q -- the cross-socket send should have landed here silently", landedOnB, marker)
	}
	landedOnA := runTmux(t, socketA, "capture-pane", "-p", "-t", "s0")
	if strings.Contains(landedOnA, marker) {
		t.Fatalf("socket A pane capture = %q, contains the marker %q -- it must never have reached the pane the caller actually meant", landedOnA, marker)
	}
}

// TestIdentityDistinguishesSameLooksLikePaneIDAcrossSockets is the
// direct proof that Identity's extra fields, not PaneID alone, are what
// makes two pane-id-collided panes on different servers distinguishable:
// PaneID is identical for both, yet the full Identity values differ
// (SocketPath differs, and so does ServerPID), so a comparison against a
// previously captured Identity -- exactly what Dispatcher.Send does on
// every send -- would never mistake one for the other.
func TestIdentityDistinguishesSameLooksLikePaneIDAcrossSockets(t *testing.T) {
	socketA := dispatchSocketTestName("identity-a")
	socketB := dispatchSocketTestName("identity-b")
	cleanupA := newBareGeometrySession(t, socketA, "s0", 40, 10)
	defer cleanupA()
	cleanupB := newBareGeometrySession(t, socketB, "s0", 40, 10)
	defer cleanupB()

	clientA := Client{Socket: socketA, Timeout: 5 * time.Second}
	clientB := Client{Socket: socketB, Timeout: 5 * time.Second}
	ctx := context.Background()

	identityA, err := clientA.CaptureIdentity(ctx, "%0")
	if err != nil {
		t.Fatalf("CaptureIdentity(socket A): %v", err)
	}
	identityB, err := clientB.CaptureIdentity(ctx, "%0")
	if err != nil {
		t.Fatalf("CaptureIdentity(socket B): %v", err)
	}

	if identityA.PaneID != "%0" || identityB.PaneID != "%0" {
		t.Fatalf("identityA.PaneID=%q identityB.PaneID=%q, want both %%0 -- test setup invalid, no collision to distinguish", identityA.PaneID, identityB.PaneID)
	}
	if identityA.PaneID != identityB.PaneID {
		t.Fatalf("identityA.PaneID=%q != identityB.PaneID=%q, want them EQUAL -- that equality is the whole hazard this test exists to show is not enough", identityA.PaneID, identityB.PaneID)
	}
	if identityA.SocketPath == identityB.SocketPath {
		t.Fatalf("identityA.SocketPath=%q == identityB.SocketPath=%q, want them to differ -- two independent -L sockets must report different #{socket_path} or this test proves nothing", identityA.SocketPath, identityB.SocketPath)
	}
	if identityA.ServerPID == identityB.ServerPID {
		t.Fatalf("identityA.ServerPID=%d == identityB.ServerPID=%d, want them to differ -- two independent tmux server processes must have different pids", identityA.ServerPID, identityB.ServerPID)
	}
	if identityA == identityB {
		t.Fatalf("identityA == identityB, want them to differ despite the identical PaneID -- SocketPath/ServerPID are exactly what breaks the tie")
	}
}

// TestDispatcherRefusesWhenTargetIdentityBelongsToTheWrongSocket is
// deck's discriminator actually rejecting the cross-socket case at the
// Dispatcher level, not just the raw Identity comparison. It constructs
// a Dispatcher normally against socket A (its identity captured there,
// exactly as NewDispatcher always does), then -- to model the
// misdirection a caller who only carried the bare pane id string could
// fall into -- makes the SAME Dispatcher value read through socket B's
// client for its "before every send" re-resolution. Socket B has its
// own unrelated %0, so Send must refuse with ErrIdentityDrifted, and the
// marker must reach neither pane.
func TestDispatcherRefusesWhenTargetIdentityBelongsToTheWrongSocket(t *testing.T) {
	socketA := dispatchSocketTestName("wrong-a")
	socketB := dispatchSocketTestName("wrong-b")
	cleanupA := newBareGeometrySession(t, socketA, "s0", 40, 10)
	defer cleanupA()
	cleanupB := newBareGeometrySession(t, socketB, "s0", 40, 10)
	defer cleanupB()

	clientA := Client{Socket: socketA, Timeout: 5 * time.Second}
	clientB := Client{Socket: socketB, Timeout: 5 * time.Second}
	ctx := context.Background()

	dispatcher, err := NewDispatcher(ctx, clientA, "%0")
	if err != nil {
		t.Fatalf("NewDispatcher(socket A): %v", err)
	}
	if dispatcher.Identity().SocketPath == "" {
		t.Fatalf("captured Identity has empty SocketPath; test setup invalid")
	}

	// Model the misdirection: the exact same Dispatcher value, but its
	// re-resolution now goes through client B's server instead of the
	// one its identity was actually captured on. This is the only way
	// to exercise the discriminator's REFUSAL from inside this package
	// without a second, separately-plumbed caller-error path to build
	// just for this test -- the identity comparison inside verify() is
	// identical either way.
	dispatcher.client = clientB

	marker := "deck-wrong-socket-marker-97531"
	err = dispatcher.Send(ctx, "send-keys", "-l", "--", marker)
	if err == nil {
		t.Fatalf("Send across a socket mismatch: got nil error, want a refusal for identity drift")
	}
	if !errors.Is(err, ErrIdentityDrifted) {
		t.Fatalf("Send across a socket mismatch: err = %v, want it to wrap ErrIdentityDrifted", err)
	}

	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		capturedA := runTmux(t, socketA, "capture-pane", "-p", "-t", "s0")
		if strings.Contains(capturedA, marker) {
			t.Fatalf("socket A pane capture = %q, contains the marker %q -- refused Send must not have delivered it anywhere", capturedA, marker)
		}
		capturedB := runTmux(t, socketB, "capture-pane", "-p", "-t", "s0")
		if strings.Contains(capturedB, marker) {
			t.Fatalf("socket B pane capture = %q, contains the marker %q -- refused Send must not have delivered it anywhere", capturedB, marker)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// TestRestartedServerReissuesPaneIDAndDispatcherRefuses is the second
// case PRD II-31 names explicitly: a restarted tmux server, on the exact
// SAME socket path, reissues the exact same pane id ("%0", the first
// pane of any fresh server). SocketPath alone is unchanged by a restart
// -- it names a filesystem path, not a server instance -- and, to rule
// that field out as an accidental discriminator, this test even keeps
// the session NAME identical across the restart too ("s0" both times),
// leaving ServerPID (#{pid}) as the only field that actually moves. A
// Dispatcher captured before the restart must refuse to send after it.
func TestRestartedServerReissuesPaneIDAndDispatcherRefuses(t *testing.T) {
	socket := dispatchSocketTestName("restart")
	cleanup := newBareGeometrySession(t, socket, "s0", 40, 10)
	defer cleanup()
	client := Client{Socket: socket, Timeout: 5 * time.Second}
	ctx := context.Background()

	dispatcher, err := NewDispatcher(ctx, client, "%0")
	if err != nil {
		t.Fatalf("NewDispatcher: %v", err)
	}
	beforeIdentity := dispatcher.Identity()
	if beforeIdentity.SessionName != "s0" {
		t.Fatalf("captured SessionName = %q, want s0; test setup invalid", beforeIdentity.SessionName)
	}

	killServerOnSocket(socket)
	// A brand new server process, on the exact same socket path, with
	// a session of the exact same name -- everything a caller could
	// plausibly have remembered about the original target is identical
	// except the server itself.
	newCleanup := newBareGeometrySession(t, socket, "s0", 40, 10)
	defer newCleanup()

	afterIdentity, err := client.CaptureIdentity(ctx, "%0")
	if err != nil {
		t.Fatalf("CaptureIdentity after restart: %v", err)
	}
	if afterIdentity.PaneID != beforeIdentity.PaneID {
		t.Fatalf("PaneID after restart = %q, want it UNCHANGED (%q) -- a fresh server's first pane is always %%0, that reissue is the whole point of this test", afterIdentity.PaneID, beforeIdentity.PaneID)
	}
	if afterIdentity.SessionName != beforeIdentity.SessionName {
		t.Fatalf("SessionName after restart = %q, want it UNCHANGED (%q) -- kept identical deliberately so it cannot be what catches the restart", afterIdentity.SessionName, beforeIdentity.SessionName)
	}
	if afterIdentity.SocketPath != beforeIdentity.SocketPath {
		t.Fatalf("SocketPath after restart = %q, want it UNCHANGED (%q) -- it names a filesystem path, not a server instance, so it must not be what catches the restart either", afterIdentity.SocketPath, beforeIdentity.SocketPath)
	}
	if afterIdentity.ServerPID == beforeIdentity.ServerPID {
		t.Fatalf("ServerPID after restart = %d, want it DIFFERENT from the pre-restart value %d -- a genuinely new server process must have a new pid, or this test proves nothing", afterIdentity.ServerPID, beforeIdentity.ServerPID)
	}

	marker := "deck-restarted-server-marker-86420"
	err = dispatcher.Send(ctx, "send-keys", "-l", "--", marker)
	if err == nil {
		t.Fatalf("Send after server restart: got nil error, want a refusal for identity drift")
	}
	if !errors.Is(err, ErrIdentityDrifted) {
		t.Fatalf("Send after server restart: err = %v, want it to wrap ErrIdentityDrifted", err)
	}

	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		captured := runTmux(t, socket, "capture-pane", "-p", "-t", "s0")
		if strings.Contains(captured, marker) {
			t.Fatalf("post-restart pane capture = %q, contains the marker %q -- refused Send must not have delivered it", captured, marker)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
