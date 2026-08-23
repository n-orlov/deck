package features

import (
	"context"
	"testing"
)

// TestReadPaneHistoryLimitDistinguishesTmuxDefaultFromADeckValue is the
// test task 029's (II-6) criteria names by name: the read must tell tmux's
// own unmodified default (2000) apart from a value something else (task 032
// will be deck's own Client.Bootstrap) explicitly set, against a real tmux
// server rather than a fixture.
func TestReadPaneHistoryLimitDistinguishesTmuxDefaultFromADeckValue(t *testing.T) {
	socket, cleanup := newBareTmuxSession(t, "s0")
	defer cleanup()
	ctx := context.Background()

	got, err := readPaneHistoryLimit(ctx, socket, "s0")
	if err != nil {
		t.Fatalf("read effective history-limit before any override: %v", err)
	}
	if got != 2000 {
		t.Fatalf("tmux's own unmodified default read as %d, want 2000 (if this ever legitimately changes, the test fixture's tmux version did, not the read)", got)
	}

	// A window-local override is enough to change what the pane's effective
	// value resolves to -- exercising the same "some scope now shadows the
	// default" mechanism task 032's Bootstrap change and task 028's scope
	// step both care about, without depending on either.
	setTmuxOption(t, socket, "-w", "-t", "s0", "history-limit", "5000")

	got, err = readPaneHistoryLimit(ctx, socket, "s0")
	if err != nil {
		t.Fatalf("read effective history-limit after override: %v", err)
	}
	if got != 5000 {
		t.Fatalf("effective history-limit after a window-local override = %d, want 5000", got)
	}
	if got == 2000 {
		t.Fatal("read still reports tmux's default after an explicit override -- distinguishing power is exactly what requirement 15 needs")
	}
}

// TestAssertPaneHistoryLimitFailsAgainstTheWrongValue proves the assertion
// itself is a real check, not a no-op: it must fail when the pane's
// effective value is not the one asserted, in both directions (asserting
// the default when an override is live, and asserting the override's value
// when it is not).
func TestAssertPaneHistoryLimitFailsAgainstTheWrongValue(t *testing.T) {
	socket, cleanup := newBareTmuxSession(t, "s0")
	defer cleanup()
	ctx := context.Background()

	if err := assertPaneHistoryLimit(ctx, socket, "s0", 2000); err != nil {
		t.Fatalf("expected the untouched default to satisfy the assertion: %v", err)
	}
	if err := assertPaneHistoryLimit(ctx, socket, "s0", 5000); err == nil {
		t.Fatal("expected failure asserting 5000 against the untouched default of 2000, got nil")
	}

	setTmuxOption(t, socket, "-w", "-t", "s0", "history-limit", "5000")

	if err := assertPaneHistoryLimit(ctx, socket, "s0", 5000); err != nil {
		t.Fatalf("expected the overridden value to satisfy the assertion: %v", err)
	}
	if err := assertPaneHistoryLimit(ctx, socket, "s0", 2000); err == nil {
		t.Fatal("expected failure asserting the stale default of 2000 against a pane now overridden to 5000, got nil")
	}
}
