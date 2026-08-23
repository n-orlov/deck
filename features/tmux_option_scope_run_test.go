package features

import (
	"context"
	"os/exec"
	"testing"
	"time"
)

// newBareTmuxSession starts a single-window tmux session directly on a
// throwaway harness socket, without the deck binary or any of deck's own
// bootstrap options, so option-scope reads can be asserted against tmux's
// own unmodified defaults. The caller must call cleanup once done.
func newBareTmuxSession(t *testing.T, session string) (socket string, cleanup func()) {
	t.Helper()
	h, err := newScenarioHarness("")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if output, err := exec.CommandContext(ctx, "tmux", "-L", h.Socket, "new-session", "-d", "-s", session, "-x", "80", "-y", "24").CombinedOutput(); err != nil {
		_ = h.KillTMuxServer(ctx)
		t.Fatalf("start bare tmux session: %v: %s", err, output)
	}
	return h.Socket, func() {
		closeCtx, closeCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer closeCancel()
		_ = h.KillTMuxServer(closeCtx)
	}
}

func setTmuxOption(t *testing.T, socket string, args ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	full := append([]string{"-L", socket, "set-option"}, args...)
	if output, err := exec.CommandContext(ctx, "tmux", full...).CombinedOutput(); err != nil {
		t.Fatalf("tmux %v: %v: %s", full, err, output)
	}
}

// TestReadTmuxOptionInScopeTreatsBothUnsetShapesAsNone proves both of tmux's
// distinct "unset" exit shapes -- an unset builtin option's empty
// line/exit-0, and an unset user option's non-zero "invalid option" exit --
// are read identically as tmuxOptionState{Set: false}, exactly what the PRD
// asks the step to do rather than trust exit code or output emptiness alone.
func TestReadTmuxOptionInScopeTreatsBothUnsetShapesAsNone(t *testing.T) {
	socket, cleanup := newBareTmuxSession(t, "s0")
	defer cleanup()
	ctx := context.Background()

	got, err := readTmuxOptionInScope(ctx, socket, "s0", tmuxOptionScopeWindow, "window-size")
	if err != nil {
		t.Fatalf("unset builtin window option: %v", err)
	}
	if got.Set {
		t.Fatalf("unset builtin window option read as set: %+v", got)
	}

	got, err = readTmuxOptionInScope(ctx, socket, "s0", tmuxOptionScopeWindow, "@deck_isize_owner")
	if err != nil {
		t.Fatalf("unset user option: %v", err)
	}
	if got.Set {
		t.Fatalf("unset user option read as set: %+v", got)
	}

	got, err = readTmuxOptionInScope(ctx, socket, "s0", tmuxOptionScopeGlobal, "@deck_isize_owner")
	if err != nil {
		t.Fatalf("unset user option in global scope: %v", err)
	}
	if got.Set {
		t.Fatalf("unset user option in global scope read as set: %+v", got)
	}
}

// TestAssertTmuxOptionInScopeDistinguishesGlobalFromWindow is the "test
// against real tmux" II-3 asks for that a value set in one scope is not
// found when a different value is read from the other -- distinguishing
// `show -gv` from `show -wv` the way a merged/effective read never could.
func TestAssertTmuxOptionInScopeDistinguishesGlobalFromWindow(t *testing.T) {
	socket, cleanup := newBareTmuxSession(t, "s0")
	defer cleanup()
	ctx := context.Background()

	setTmuxOption(t, socket, "-g", "window-size", "latest")
	setTmuxOption(t, socket, "-w", "-t", "s0", "window-size", "manual")

	if err := assertTmuxOptionInScope(ctx, socket, "s0", tmuxOptionScopeWindow, "window-size", "manual"); err != nil {
		t.Fatalf("window scope should read the window-local value: %v", err)
	}
	if err := assertTmuxOptionInScope(ctx, socket, "s0", tmuxOptionScopeGlobal, "window-size", "latest"); err != nil {
		t.Fatalf("global scope should still read the untouched global value: %v", err)
	}
}

// TestAssertTmuxOptionInScopeFailsWhenValueIsInTheWrongScope is the negative
// control the criteria names by name: the assertion must fail, not merely
// happen to pass, when the value it wants only exists in the other scope.
func TestAssertTmuxOptionInScopeFailsWhenValueIsInTheWrongScope(t *testing.T) {
	socket, cleanup := newBareTmuxSession(t, "s0")
	defer cleanup()
	ctx := context.Background()

	setTmuxOption(t, socket, "-g", "window-size", "latest")
	setTmuxOption(t, socket, "-w", "-t", "s0", "window-size", "manual")

	if err := assertTmuxOptionInScope(ctx, socket, "s0", tmuxOptionScopeGlobal, "window-size", "manual"); err == nil {
		t.Fatal("expected failure asserting the window-local value against the global scope, got nil")
	}
	if err := assertTmuxOptionInScope(ctx, socket, "s0", tmuxOptionScopeWindow, "window-size", "latest"); err == nil {
		t.Fatal("expected failure asserting the global value against the window scope, got nil")
	}

	// The same wrong-scope failure for a user option, whose "unset" shape is
	// the non-zero exit rather than the builtin's empty line: set only in
	// the window scope, the global read must not be satisfied by it.
	setTmuxOption(t, socket, "-w", "-t", "s0", "@deck_isize_owner", "tag:123")
	if err := assertTmuxOptionInScope(ctx, socket, "s0", tmuxOptionScopeGlobal, "@deck_isize_owner", "tag:123"); err == nil {
		t.Fatal("expected failure asserting a window-only user option against the global scope, got nil")
	}
	if err := assertTmuxOptionInScope(ctx, socket, "s0", tmuxOptionScopeWindow, "@deck_isize_owner", "tag:123"); err != nil {
		t.Fatalf("window scope should read the user option it was actually set in: %v", err)
	}
}

// TestAssertTmuxOptionUnsetInScope proves the unset-assertion direction:
// green while genuinely unset, red once a value lands in that exact scope.
func TestAssertTmuxOptionUnsetInScope(t *testing.T) {
	socket, cleanup := newBareTmuxSession(t, "s0")
	defer cleanup()
	ctx := context.Background()

	if err := assertTmuxOptionUnsetInScope(ctx, socket, "s0", tmuxOptionScopeWindow, "@deck_isize_owner"); err != nil {
		t.Fatalf("genuinely unset user option: %v", err)
	}

	setTmuxOption(t, socket, "-w", "-t", "s0", "@deck_isize_owner", "tag:123")
	if err := assertTmuxOptionUnsetInScope(ctx, socket, "s0", tmuxOptionScopeWindow, "@deck_isize_owner"); err == nil {
		t.Fatal("expected failure once the option was set in the exact scope asserted unset, got nil")
	}
	// It is still genuinely unset in the OTHER scope.
	if err := assertTmuxOptionUnsetInScope(ctx, socket, "s0", tmuxOptionScopeGlobal, "@deck_isize_owner"); err != nil {
		t.Fatalf("global scope should still read unset: %v", err)
	}
}
