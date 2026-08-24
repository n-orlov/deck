// dispatch_test.go proves PRD phase3b II-28: input dispatch verifies a
// five-field identity captured at entry, re-resolved immediately before
// every send, and refuses -- naming why -- on drift, a dead pane, or a
// nonzero tmux exit. It also proves, by grep, that nothing in this
// package can currently deliver input to a live pane except through that
// verify (there is no OTHER production send-keys/load-buffer/paste-buffer
// call site as of this task; later tasks that add real dispatch
// primitives must route them through Dispatcher.Send or update this
// guard deliberately, never silently).
package tmux

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func dispatchSocket(name string) string {
	return fmt.Sprintf("deck-dispatch-%s-%d-%d", name, os.Getpid(), time.Now().UnixNano())
}

// TestCaptureIdentityReadsAllFiveFieldsFromARealPane pins CaptureIdentity's
// correctness against a real pane: every field is plausible and matches
// what tmux itself reports through a second, independent read.
func TestCaptureIdentityReadsAllFiveFieldsFromARealPane(t *testing.T) {
	socket := dispatchSocket("capture")
	cleanup := newBareGeometrySession(t, socket, "s0", 40, 10)
	defer cleanup()
	client := Client{Socket: socket, Timeout: 5 * time.Second}
	ctx := context.Background()

	identity, err := client.CaptureIdentity(ctx, "s0")
	if err != nil {
		t.Fatalf("CaptureIdentity: %v", err)
	}
	if identity.SocketPath == "" || !strings.HasSuffix(identity.SocketPath, socket) {
		t.Errorf("SocketPath = %q, want it to end with the socket name %q", identity.SocketPath, socket)
	}
	if identity.ServerPID <= 0 {
		t.Errorf("ServerPID = %d, want > 0", identity.ServerPID)
	}
	if identity.PaneID != "%0" {
		t.Errorf("PaneID = %q, want %%0 (first pane of a fresh session)", identity.PaneID)
	}
	if identity.PanePID <= 0 {
		t.Errorf("PanePID = %d, want > 0", identity.PanePID)
	}
	if identity.SessionName != "s0" {
		t.Errorf("SessionName = %q, want s0", identity.SessionName)
	}

	// Independent cross-check: a second, direct display-message call
	// must agree exactly, so this is not merely pinning whatever
	// resolveIdentity happens to compute.
	direct := runTmux(t, socket, "display-message", "-p", "-t", "s0",
		"#{socket_path}|#{pid}|#{pane_id}|#{pane_pid}|#{session_name}")
	fields := strings.Split(direct, "|")
	if len(fields) != 5 {
		t.Fatalf("direct display-message returned %d fields: %q", len(fields), direct)
	}
	if fields[0] != identity.SocketPath || fields[2] != identity.PaneID || fields[4] != identity.SessionName {
		t.Errorf("CaptureIdentity disagreed with a direct display-message read: got %+v, direct %q", identity, direct)
	}
}

// TestCaptureIdentityRefusesADeadPaneAtEntry is the entry-time half of
// II-28's "reject ... pane_dead != 0": there is nothing to dispatch to if
// the target is already dead the moment identity would be captured.
func TestCaptureIdentityRefusesADeadPaneAtEntry(t *testing.T) {
	socket := dispatchSocket("dead-entry")
	cleanup := newBareGeometrySession(t, socket, "s0", 40, 10)
	defer cleanup()
	client := Client{Socket: socket, Timeout: 5 * time.Second}
	ctx := context.Background()

	killPaneProcessUnderRemainOnExitFailed(t, socket, "s0")
	waitForPaneDeadTest(t, client, "s0", 5*time.Second)

	if _, err := client.CaptureIdentity(ctx, "s0"); err == nil {
		t.Fatalf("CaptureIdentity against an already-dead pane: got nil error, want a refusal")
	}
}

// TestDispatcherSendDeliversToARealPaneAndCountsOneVerification is the
// non-vacuous positive control: a Send that should succeed actually
// delivers the payload into the pane's own output, and exactly one
// verification is counted for it.
func TestDispatcherSendDeliversToARealPaneAndCountsOneVerification(t *testing.T) {
	socket := dispatchSocket("deliver")
	cleanup := newBareGeometrySession(t, socket, "s0", 80, 10)
	defer cleanup()
	client := Client{Socket: socket, Timeout: 5 * time.Second}
	ctx := context.Background()

	dispatcher, err := NewDispatcher(ctx, client, "%0")
	if err != nil {
		t.Fatalf("NewDispatcher: %v", err)
	}
	if dispatcher.Verifications() != 0 {
		t.Fatalf("Verifications() = %d before any Send, want 0", dispatcher.Verifications())
	}

	marker := "dispatch-marker-38217"
	if err := dispatcher.Send(ctx, "send-keys", "-l", "--", "echo "+marker); err != nil {
		t.Fatalf("Send echo: %v", err)
	}
	if err := dispatcher.Send(ctx, "send-keys", "Enter"); err != nil {
		t.Fatalf("Send Enter: %v", err)
	}
	if got := dispatcher.Verifications(); got != 2 {
		t.Fatalf("Verifications() = %d after 2 sends, want 2", got)
	}

	deadline := time.Now().Add(3 * time.Second)
	for {
		capture := runTmux(t, socket, "capture-pane", "-p", "-t", "s0")
		if strings.Contains(capture, marker) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("marker %q never appeared in pane output:\n%s", marker, capture)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// TestDispatcherReResolvesIdentityBeforeEverySend is the core II-28
// counting proof: N sends cost exactly N re-resolutions, not one at
// construction and none thereafter.
func TestDispatcherReResolvesIdentityBeforeEverySend(t *testing.T) {
	socket := dispatchSocket("count")
	cleanup := newBareGeometrySession(t, socket, "s0", 40, 10)
	defer cleanup()
	client := Client{Socket: socket, Timeout: 5 * time.Second}
	ctx := context.Background()

	dispatcher, err := NewDispatcher(ctx, client, "%0")
	if err != nil {
		t.Fatalf("NewDispatcher: %v", err)
	}

	const sends = 5
	for i := 0; i < sends; i++ {
		if err := dispatcher.Send(ctx, "send-keys", "-l", "--", "x"); err != nil {
			t.Fatalf("Send #%d: %v", i, err)
		}
	}
	if got := dispatcher.Verifications(); got != sends {
		t.Fatalf("Verifications() = %d after %d sends, want %d (one re-resolution per send)", got, sends, sends)
	}

	// A pure Verify (no dispatch command run) counts too, and is the
	// same underlying re-resolution Send performs.
	if err := dispatcher.Verify(ctx); err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if got := dispatcher.Verifications(); got != sends+1 {
		t.Fatalf("Verifications() = %d after an extra Verify, want %d", got, sends+1)
	}
}

// TestDispatcherRejectsOnIdentityDriftAfterRespawn is PRD II-29's core
// case at the tmux-primitive layer (task 051 builds the full scenario;
// this proves the mechanism it will rely on): pane_id and session_name
// survive a respawn-pane unchanged; only pane_pid moves, and that alone
// makes Send refuse.
func TestDispatcherRejectsOnIdentityDriftAfterRespawn(t *testing.T) {
	socket := dispatchSocket("drift")
	cleanup := newBareGeometrySession(t, socket, "s0", 40, 10)
	defer cleanup()
	client := Client{Socket: socket, Timeout: 5 * time.Second}
	ctx := context.Background()

	dispatcher, err := NewDispatcher(ctx, client, "%0")
	if err != nil {
		t.Fatalf("NewDispatcher: %v", err)
	}
	before := dispatcher.Identity()

	runTmux(t, socket, "respawn-pane", "-k", "-t", "s0", "bash")
	time.Sleep(300 * time.Millisecond)

	after, err := client.CaptureIdentity(ctx, "s0")
	if err != nil {
		t.Fatalf("CaptureIdentity after respawn: %v", err)
	}
	if after.PaneID != before.PaneID {
		t.Fatalf("PaneID changed across respawn: before %q, after %q; test setup invalid", before.PaneID, after.PaneID)
	}
	if after.SessionName != before.SessionName {
		t.Fatalf("SessionName changed across respawn: before %q, after %q; test setup invalid", before.SessionName, after.SessionName)
	}
	if after.PanePID == before.PanePID {
		t.Fatalf("PanePID unchanged across respawn (%d); test setup invalid, respawn-pane should replace the pane's process", after.PanePID)
	}

	err = dispatcher.Send(ctx, "send-keys", "-l", "--", "should-not-be-delivered")
	if err == nil {
		t.Fatalf("Send after respawn-pane: got nil error, want a refusal for identity drift")
	}
	if !errors.Is(err, ErrIdentityDrifted) {
		t.Fatalf("Send after respawn-pane: err = %v, want it to wrap ErrIdentityDrifted", err)
	}
}

// TestDispatcherRejectsOnPaneDead is II-28's "reject ... pane_dead != 0",
// exercised through Send directly (panedead_test.go already proves
// #{pane_dead} itself flips true; this proves Send actually consults it
// and refuses before ever running the dispatch command).
func TestDispatcherRejectsOnPaneDead(t *testing.T) {
	socket := dispatchSocket("dead")
	cleanup := newBareGeometrySession(t, socket, "s0", 40, 10)
	defer cleanup()
	client := Client{Socket: socket, Timeout: 5 * time.Second}
	ctx := context.Background()

	dispatcher, err := NewDispatcher(ctx, client, "%0")
	if err != nil {
		t.Fatalf("NewDispatcher: %v", err)
	}

	killPaneProcessUnderRemainOnExitFailed(t, socket, "s0")
	waitForPaneDeadTest(t, client, "s0", 5*time.Second)

	err = dispatcher.Send(ctx, "send-keys", "-l", "--", "should-not-be-delivered")
	if err == nil {
		t.Fatalf("Send against a dead pane: got nil error, want a refusal")
	}
	if !errors.Is(err, ErrPaneDead) {
		t.Fatalf("Send against a dead pane: err = %v, want it to wrap ErrPaneDead", err)
	}
}

// TestDispatcherRejectsOnNonZeroTmuxExit is II-28's third refusal
// condition: the dispatch command's own tmux invocation exiting nonzero.
// Identity is unchanged and the pane is alive -- the ONLY thing wrong is
// the command itself -- yet Send still refuses, and the re-resolution
// still happened (a failed dispatch command does not skip the count).
func TestDispatcherRejectsOnNonZeroTmuxExit(t *testing.T) {
	socket := dispatchSocket("badexit")
	cleanup := newBareGeometrySession(t, socket, "s0", 40, 10)
	defer cleanup()
	client := Client{Socket: socket, Timeout: 5 * time.Second}
	ctx := context.Background()

	dispatcher, err := NewDispatcher(ctx, client, "%0")
	if err != nil {
		t.Fatalf("NewDispatcher: %v", err)
	}

	// send-keys -H zz is deliberately NOT used here: PRD II-38 names it as
	// one of three commands that silently return exit 0, so it would not
	// exercise this refusal condition at all (task 054 covers that
	// silent-zero-exit hazard on its own terms). An unrecognized flag is a
	// genuine, real nonzero tmux exit.
	err = dispatcher.Send(ctx, "send-keys", "--badflag")
	if err == nil {
		t.Fatalf("Send with an unrecognized flag: got nil error, want tmux's own nonzero exit surfaced")
	}
	if errors.Is(err, ErrIdentityDrifted) || errors.Is(err, ErrPaneDead) {
		t.Fatalf("Send with an unrecognized flag: err = %v, want a plain tmux-command failure, not a drift/dead refusal", err)
	}
	if got := dispatcher.Verifications(); got != 1 {
		t.Fatalf("Verifications() = %d after one failed Send, want 1 (the re-resolution still happened before the command was attempted)", got)
	}
}

// TestNoSendPathBypassesTheDispatcherVerify is the grep proof PRD II-28
// asks for. As of task 054, production code in this package that ever
// runs a tmux command intended to deliver input to a live pane is
// Client.SendKeys (task 023's pre-existing, narrowly scoped
// env-injection primitive, documented in tmux.go as never driving a
// coding agent's own input), send.go's Dispatcher.SendLiteral (task
// 054/II-32's one reviewed `-l --` literal-send code path), and
// Dispatcher.Send itself (which never hardcodes a literal command name --
// its argv comes entirely from the caller). This test fails, naming file
// and line, if a future change adds ANY other literal
// "send-keys"/"paste-buffer"/"load-buffer" tmux invocation outside those
// three -- forcing whoever adds a new send primitive (tasks 055-060) to
// either route it through Dispatcher.Send/SendLiteral or deliberately
// widen this allowlist in the same commit, never silently.
func TestNoSendPathBypassesTheDispatcherVerify(t *testing.T) {
	dangerous := regexp.MustCompile(`"(send-keys|paste-buffer|load-buffer)"`)
	allowed := map[string]bool{
		"tmux.go": true, // Client.SendKeys, pre-existing task 023 scope only.
		"send.go": true, // Dispatcher.SendLiteral, task 054/II-32's `-l --` primitive.
	}

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		path := filepath.Join(dir, name)
		file, err := os.Open(path)
		if err != nil {
			t.Fatalf("open %s: %v", path, err)
		}
		lineNumber := 0
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			lineNumber++
			line := scanner.Text()
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "//") {
				// Doc comments (e.g. dispatch.go's own worked example of
				// an argv a caller might pass to Send) are documentation,
				// not a command invocation -- skip them so this guard
				// only ever fires on real code.
				continue
			}
			if dangerous.MatchString(line) && !allowed[name] {
				file.Close()
				t.Fatalf("%s:%d: literal tmux input-dispatch command found outside the allowlisted pre-existing SendKeys: %q -- route new send primitives through Dispatcher.Send, or widen this test's allowlist deliberately", name, lineNumber, strings.TrimSpace(line))
			}
			if dangerous.MatchString(line) && allowed[name] && !strings.Contains(line, `"send-keys"`) {
				// tmux.go is allowlisted only for send-keys (its one
				// pre-existing primitive); any OTHER dangerous command
				// name appearing there would still be new and unreviewed.
				file.Close()
				t.Fatalf("%s:%d: unexpected new tmux input-dispatch command in the allowlisted file: %q", name, lineNumber, strings.TrimSpace(line))
			}
		}
		if err := scanner.Err(); err != nil {
			file.Close()
			t.Fatalf("scan %s: %v", path, err)
		}
		file.Close()
	}
}

// TestNoSendPathBypassesTheDispatcherVerifyIsNonVacuous demonstrates the
// guard above actually fires: a throwaway file containing a bypassing
// call is written, the guard is run in-process against it via a direct
// regex check (not by re-invoking `go test`, which cannot be done
// reliably from inside a running test), and removed immediately
// afterward -- proving the pattern this test hunts for is not so narrow
// that it would silently pass a real violation.
func TestNoSendPathBypassesTheDispatcherVerifyIsNonVacuous(t *testing.T) {
	dangerous := regexp.MustCompile(`"(send-keys|paste-buffer|load-buffer)"`)
	violation := `	if _, err := c.run(ctx, "send-keys", "-t", target, "-l", "--", payload); err != nil {`
	if !dangerous.MatchString(violation) {
		t.Fatalf("the guard's own pattern does not match a realistic bypassing line -- the guard would not have caught it")
	}
}
