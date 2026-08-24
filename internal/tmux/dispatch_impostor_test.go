// dispatch_impostor_test.go proves PRD phase3b II-30: "never dispatch by
// session name". It has two halves.
//
// The red control (TestNameDispatchedPayloadLandsInTheImpostorPane) shows
// the raw hazard PRD II-30 names, entirely outside deck's own code: rename
// a session, create a NEW session reusing the old name, then issue a bare
// tmux send-keys targeted with "-t" at the reused NAME (never through
// Dispatcher). tmux resolves "-t <name>" dynamically at call time, so the
// payload lands in the impostor's pane -- exit=0, empty stderr, no
// indication anything went wrong.
//
// The green half (TestDispatcherRefusesAfterSessionRenameEvenThoughPaneIDIsUnchanged)
// re-runs the identical rename+reuse setup against a Dispatcher captured on
// the ORIGINAL pane's id before the rename, and shows Send refuses -- the
// captured SessionName ("orig") no longer matches the renamed pane's fresh
// SessionName ("orig-renamed"), so the drift check (task 050) catches it,
// and the marker never reaches either pane.
//
// TestNoSendPathUsesSessionNameAsTarget is the grep proof the task's
// success criteria names directly: production code in this package never
// builds a dispatch command's own "-t" argument from a SessionName field.
// This is enforced structurally by dispatch.go's Send (task 052): Send
// refuses outright if a caller's args contain "-t" at all, always
// supplying the pane id captured and re-verified at entry instead -- so
// the grep is a proof the mechanism exists and stays that way, not the
// mechanism itself.
package tmux

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
	"time"
)

func dispatchImpostorSocket(name string) string {
	return fmt.Sprintf("deck-impostor-%s-%d-%d", name, os.Getpid(), time.Now().UnixNano())
}

// runTmuxRaw runs a bare tmux command against socket and reports its
// stdout, stderr and exit code separately -- unlike this package's
// runTmux (geometry_test.go), which calls t.Fatalf on any nonzero exit
// and cannot be used to assert that a command "succeeded" in the
// dangerous sense PRD II-30 describes (exit=0, empty stderr, wrong pane).
func runTmuxRaw(t *testing.T, socket string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	full := append([]string{"-L", socket}, args...)
	cmd := exec.CommandContext(ctx, "tmux", full...)
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	code := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			code = exitErr.ExitCode()
		} else {
			t.Fatalf("tmux -L %s %s: %v", socket, strings.Join(args, " "), err)
		}
	}
	return outBuf.String(), errBuf.String(), code
}

// TestNameDispatchedPayloadLandsInTheImpostorPane is the raw hazard PRD
// II-30 asks to be shown: rename a session, create a new one reusing the
// name, dispatch by that name, and watch the payload land in the
// impostor -- silently, exit=0, empty stderr.
func TestNameDispatchedPayloadLandsInTheImpostorPane(t *testing.T) {
	socket := dispatchImpostorSocket("raw")
	origCleanup := newBareGeometrySession(t, socket, "orig", 40, 10)
	defer origCleanup()

	runTmux(t, socket, "rename-session", "-t", "orig", "orig-renamed")

	impostorCleanup := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = exec.CommandContext(ctx, "tmux", "-L", socket, "kill-session", "-t", "orig").Run()
	}
	runTmux(t, socket, "new-session", "-d", "-s", "orig", "-x", "40", "-y", "10")
	defer impostorCleanup()

	marker := "impostor-marker-90210"
	stdout, stderr, code := runTmuxRaw(t, socket, "send-keys", "-t", "orig", "-l", "--", marker)
	if code != 0 {
		t.Fatalf("send-keys -t orig (the reused name): exit=%d, want 0; stdout=%q stderr=%q", code, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("send-keys -t orig (the reused name): stderr=%q, want empty (PRD II-30: this hazard is SILENT)", stderr)
	}

	// The marker must land in the IMPOSTOR's pane (the new session that
	// reused the name), and must NOT reach the renamed original.
	impostorCapture := runTmux(t, socket, "capture-pane", "-p", "-t", "orig")
	if !strings.Contains(impostorCapture, marker) {
		t.Fatalf("impostor pane capture = %q, want it to contain the marker %q -- the name-dispatched payload should have landed here", impostorCapture, marker)
	}
	originalCapture := runTmux(t, socket, "capture-pane", "-p", "-t", "orig-renamed")
	if strings.Contains(originalCapture, marker) {
		t.Fatalf("renamed original pane capture = %q, contains the marker %q -- it should never have been reachable via the reused name", originalCapture, marker)
	}
}

// TestDispatcherRefusesAfterSessionRenameEvenThoughPaneIDIsUnchanged is
// the green half: a Dispatcher constructed on the ORIGINAL pane's id
// (never on the session name) refuses to deliver the identical marker
// into the identical rename+reuse setup, because the pane's own
// session_name field drifted from "orig" to "orig-renamed" -- and the
// marker never reaches either pane, not even the one the Dispatcher still
// points at.
func TestDispatcherRefusesAfterSessionRenameEvenThoughPaneIDIsUnchanged(t *testing.T) {
	socket := dispatchImpostorSocket("refuse")
	origCleanup := newBareGeometrySession(t, socket, "orig", 40, 10)
	defer origCleanup()
	client := Client{Socket: socket, Timeout: 5 * time.Second}
	ctx := context.Background()

	dispatcher, err := NewDispatcher(ctx, client, "%0")
	if err != nil {
		t.Fatalf("NewDispatcher: %v", err)
	}
	if dispatcher.Identity().SessionName != "orig" {
		t.Fatalf("captured SessionName = %q, want orig; test setup invalid", dispatcher.Identity().SessionName)
	}

	runTmux(t, socket, "rename-session", "-t", "orig", "orig-renamed")

	impostorCleanup := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = exec.CommandContext(ctx, "tmux", "-L", socket, "kill-session", "-t", "orig").Run()
	}
	runTmux(t, socket, "new-session", "-d", "-s", "orig", "-x", "40", "-y", "10")
	defer impostorCleanup()

	marker := "deck-refuses-marker-13579"
	err = dispatcher.Send(ctx, "send-keys", "-l", "--", marker)
	if err == nil {
		t.Fatalf("Send after session rename+reuse: got nil error, want a refusal for identity drift")
	}
	if !errors.Is(err, ErrIdentityDrifted) {
		t.Fatalf("Send after session rename+reuse: err = %v, want it to wrap ErrIdentityDrifted", err)
	}

	// The Dispatcher's own target (the original pane's id) still exists
	// (it was only renamed, not killed) -- confirm the marker reached
	// NEITHER the pane the Dispatcher still points at, nor the impostor.
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		originalCapture := runTmux(t, socket, "capture-pane", "-p", "-t", "orig-renamed")
		if strings.Contains(originalCapture, marker) {
			t.Fatalf("original (renamed) pane capture = %q, contains the marker %q -- refused Send must not have delivered it anyway", originalCapture, marker)
		}
		impostorCapture := runTmux(t, socket, "capture-pane", "-p", "-t", "orig")
		if strings.Contains(impostorCapture, marker) {
			t.Fatalf("impostor pane capture = %q, contains the marker %q -- refused Send must not have delivered it anywhere", impostorCapture, marker)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// TestNewDispatcherRejectsASessionNameTarget is the direct structural
// proof that NewDispatcher itself refuses a session-name-shaped target,
// not merely tolerates it and catches the danger later via drift: PRD
// II-30 is enforced at construction, before any Send is ever possible.
func TestNewDispatcherRejectsASessionNameTarget(t *testing.T) {
	socket := dispatchImpostorSocket("reject")
	cleanup := newBareGeometrySession(t, socket, "s0", 40, 10)
	defer cleanup()
	client := Client{Socket: socket, Timeout: 5 * time.Second}
	ctx := context.Background()

	if _, err := NewDispatcher(ctx, client, "s0"); err == nil {
		t.Fatalf("NewDispatcher(ctx, client, %q): got nil error, want a refusal -- %q is a session name, not a pane id", "s0", "s0")
	}
}

// sessionNameAsDispatchTarget is the pattern TestNoSendPathUsesSessionNameAsTarget
// hunts for: a "-t" tmux flag built directly from a SessionName field,
// which is exactly what would let a renamed-then-reused session's
// impostor receive a dispatch meant for the original pane.
var sessionNameAsDispatchTarget = regexp.MustCompile(`"-t"[^\n]*SessionName|SessionName[^\n]*"-t"`)

// TestNoSendPathUsesSessionNameAsTarget is the grep proof PRD II-30's
// success criteria names directly: no production .go file in this
// package ever builds a dispatch command's "-t" argument from a
// SessionName field. dispatch.go's Send (task 052) makes this
// structurally true -- args may not contain "-t" at all, so there is no
// place left for a SessionName-derived value to be smuggled in as a
// target -- but this test is what keeps that property honest if the code
// ever changes.
func TestNoSendPathUsesSessionNameAsTarget(t *testing.T) {
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
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("ReadFile %s: %v", name, err)
		}
		for i, line := range strings.Split(string(data), "\n") {
			if sessionNameAsDispatchTarget.MatchString(line) {
				t.Fatalf("%s:%d: dispatch target built from SessionName: %q -- PRD II-30 forbids ever dispatching by session name", name, i+1, strings.TrimSpace(line))
			}
		}
	}
}

// TestNoSendPathUsesSessionNameAsTargetIsNonVacuous proves the guard
// above actually fires against a realistic violation, the same
// demonstrate-then-revert discipline dispatch_test.go's own grep guard
// (TestNoSendPathBypassesTheDispatcherVerifyIsNonVacuous) uses.
func TestNoSendPathUsesSessionNameAsTargetIsNonVacuous(t *testing.T) {
	violation := `	if _, err := d.client.run(ctx, "send-keys", "-t", identity.SessionName, "-l", "--", payload); err != nil {`
	if !sessionNameAsDispatchTarget.MatchString(violation) {
		t.Fatalf("the guard's own pattern does not match a realistic session-name-as-target violation -- it would not have caught it")
	}
}
