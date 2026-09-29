package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// This file is task 011's own acceptance test obligation (SPEC §11's pin
// rule, R159's detail-dialog leg): the `i` detail dialog carries its own
// "Pinned:" field and its own "p" binding, distinct from (but agreeing
// with) the top-level sidebar pin (task 010) -- the detail dialog is the
// one place that states the pin as plain "yes"/"no" text rather than the
// sidebar's ✦ glyph.

// TestDetailPTogglesPinAndPinnedLineFollows proves R159's detail-dialog
// leg end to end against a real store (never a stub): opening `i` on an
// unpinned session shows "pinned: no"; pressing "p" pins it (the store's
// own pinned_at column becomes non-zero once the returned tea.Cmd
// actually runs, exactly like the top-level `p`'s own contract,
// TestPTogglesPin) and the still-open dialog -- m.detail stays true the
// whole time, never closing for the pin/unpin round trip or its reload --
// now shows "pinned: yes"; a second "p" returns both the store and the
// dialog to "no"/0.
func TestDetailPTogglesPinAndPinnedLineFollows(t *testing.T) {
	db := sidebarPinTestStore(t)
	sidebarPinCreateSession(t, db, "s1", "alpha", 100)

	clock, err := config.NewClock("2025-01-02T03:04:05Z", "")
	if err != nil {
		t.Fatal(err)
	}
	model := NewWithShellCreator(db, config.Settings{Clock: clock}, "", nil)
	updated, _ := model.Update(model.loadSessions())
	model = updated.(Model)
	model.selected = rowCursor(0)

	got, _ := model.Update(key("i"))
	model = got.(Model)
	if !model.detail {
		t.Fatal("\"i\" did not open the detail dialog")
	}

	pinnedLine := detailLineWithPrefix(t, model.detailBody(), "Pinned:")
	if !strings.Contains(pinnedLine, "no") || strings.Contains(pinnedLine, "yes") {
		t.Fatalf("unpinned session's detail dialog line = %q, want it to say \"no\", not \"yes\"", pinnedLine)
	}

	got, cmd := model.Update(key("p"))
	model = got.(Model)
	if !model.detail {
		t.Fatal("\"p\" inside detail closed the dialog; it must stay open")
	}
	if cmd == nil {
		t.Fatal("\"p\" inside detail returned no tea.Cmd")
	}
	msg := cmd()
	pinnedMsg, ok := msg.(sessionsPinned)
	if !ok {
		t.Fatalf("cmd() returned %T, want sessionsPinned", msg)
	}
	if pinnedMsg.err != nil {
		t.Fatalf("detail pin cmd errored: %v", pinnedMsg.err)
	}

	row, err := db.GetSession(context.Background(), "s1")
	if err != nil {
		t.Fatal(err)
	}
	if row.PinnedAt == 0 {
		t.Fatal("running detail \"p\"'s own tea.Cmd did not pin s1 in the store")
	}

	got, reloadCmd := model.Update(pinnedMsg)
	model = got.(Model)
	if !model.detail {
		t.Fatal("applying sessionsPinned closed the detail dialog; it must stay open")
	}
	if reloadCmd == nil {
		t.Fatal("sessionsPinned scheduled no reload")
	}
	got, _ = model.Update(reloadCmd())
	model = got.(Model)
	if !model.detail {
		t.Fatal("the reload triggered by sessionsPinned closed the detail dialog; it must stay open")
	}

	pinnedLine = detailLineWithPrefix(t, model.detailBody(), "Pinned:")
	if !strings.Contains(pinnedLine, "yes") || strings.Contains(pinnedLine, "no") {
		t.Fatalf("now-pinned session's detail dialog line = %q, want it to say \"yes\", not \"no\"", pinnedLine)
	}

	// Second "p" returns both the store and the dialog to no/0.
	got, cmd = model.Update(key("p"))
	model = got.(Model)
	if !model.detail {
		t.Fatal("second \"p\" inside detail closed the dialog; it must stay open")
	}
	if cmd == nil {
		t.Fatal("second \"p\" inside detail returned no tea.Cmd")
	}
	msg = cmd()
	pinnedMsg, ok = msg.(sessionsPinned)
	if !ok {
		t.Fatalf("second cmd() returned %T, want sessionsPinned", msg)
	}
	if pinnedMsg.err != nil {
		t.Fatalf("second detail pin cmd errored: %v", pinnedMsg.err)
	}

	row, err = db.GetSession(context.Background(), "s1")
	if err != nil {
		t.Fatal(err)
	}
	if row.PinnedAt != 0 {
		t.Fatalf("second detail \"p\" did not unpin s1, pinned_at = %d", row.PinnedAt)
	}

	got, reloadCmd = model.Update(pinnedMsg)
	model = got.(Model)
	got, _ = model.Update(reloadCmd())
	model = got.(Model)
	if !model.detail {
		t.Fatal("the second reload closed the detail dialog; it must stay open")
	}

	pinnedLine = detailLineWithPrefix(t, model.detailBody(), "Pinned:")
	if !strings.Contains(pinnedLine, "no") || strings.Contains(pinnedLine, "yes") {
		t.Fatalf("re-unpinned session's detail dialog line = %q, want it to say \"no\", not \"yes\"", pinnedLine)
	}
}

// TestDetailKeyLineNamesPcp proves the detail dialog's own key line (the
// last non-empty line of detailBody, detailFooterLine's own contract)
// names "P" (task 007's permission-profile picker), "c" (task 008's
// resume-mode lock chooser) and "p" (this task's pin toggle) alongside
// its three pre-existing load-bearing keys "r", "l" and "g" -- every key
// inside this dialog SPEC §11.4 calls load-bearing must be named where it
// applies, exactly like TestDetailFooterAndHelpBothNameGForGroupMove
// already proves for "g" alone.
func TestDetailKeyLineNamesPcp(t *testing.T) {
	m := New(nil, config.Settings{}, "")
	m.width, m.height = 80, 24
	m.sessions = []store.Session{{ID: "s1", Name: "alpha", Agent: "shell", Status: "stopped", Slug: "alpha"}}
	m.selected = rowCursor(0)

	footer := detailFooterLine(m.detailBody())
	for _, want := range []string{"P switches", "c changes", "p toggles", "r renames", "l edits launch inputs", "g moves group", "i or Esc closes detail"} {
		if !strings.Contains(footer, want) {
			t.Fatalf("detail key line %q does not name %q", footer, want)
		}
	}
}
