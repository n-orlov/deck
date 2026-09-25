package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// sessionRowRenderGoldenPath is the checked-in, byte-exact rendering of
// every sidebar SESSION ROW (sidebarLineRow entries only -- headers and
// the socket line are deliberately excluded) for a fixed fixture, in
// colour, NO_COLOR and ascii modes, with the cursor on a session row and
// then on a group header.
//
// Task 003/R141 (GH #39) moved group headers flush left and changed only
// the HEADER's cursor cue; session rows must render exactly as before. The
// golden was generated on the tree before that change (5611c2b) and is
// compared byte-for-byte here, raw SGR escapes included, so any drift in
// row gutter, indentation, background or text fails this test.
//
// Regenerate only after a deliberate, reviewed row-rendering change:
//
//	ci/run.sh env UPDATE_GOLDEN=1 go test -count=1 -run TestSessionRowRenderGoldenIsByteIdentical ./internal/tui/
const sessionRowRenderGoldenPath = "testdata/golden/session_rows.golden"

func renderSessionRowGolden(t *testing.T) string {
	t.Helper()
	// A frozen wall clock (no step) pins every row's relative age, so the
	// golden never drifts with the date it is run on.
	clock, err := config.NewClock("2026-01-02T03:04:05Z", "")
	if err != nil {
		t.Fatalf("frozen clock: %v", err)
	}
	now := clock.Now()
	ago := func(d time.Duration) int64 { return now.Add(-d).UnixMilli() }
	idA, idB := int64(1), int64(2)
	sessions := []store.Session{
		{ID: "a0", Name: "alpha zero", Status: "idle", GroupName: "a", GroupID: &idA, CreatedAt: ago(0)},
		{ID: "a1", Name: "alpha one", Status: "working", GroupName: "a", GroupID: &idA, CreatedAt: ago(5 * time.Minute)},
		{ID: "b0", Name: "bravo zero", Status: "waiting", GroupName: "b", GroupID: &idB, CreatedAt: ago(3 * time.Hour)},
		{ID: "d0", Name: "default row", Status: "idle", CreatedAt: ago(50 * time.Hour)},
	}
	modes := []struct {
		name     string
		settings config.Settings
	}{
		{"colour", config.Settings{Socket: "deck", Color: true, Clock: clock}},
		{"NO_COLOR", config.Settings{Socket: "deck", Color: false, Clock: clock}},
		{"ascii", config.Settings{Socket: "deck", Color: true, ASCII: true, Clock: clock}},
	}
	cursors := []struct {
		name   string
		cursor func(m Model) sidebarCursor
	}{
		{"cursor-on-row", func(m Model) sidebarCursor { return m.selected }},
		{"cursor-on-header", func(Model) sidebarCursor { return headerCursor(idB) }},
	}
	var b strings.Builder
	for _, mode := range modes {
		for _, c := range cursors {
			m := New(nil, mode.settings, "")
			m.sessions = append([]store.Session(nil), sessions...)
			m.selected = c.cursor(m)
			fmt.Fprintf(&b, "== %s / %s ==\n", mode.name, c.name)
			for _, e := range m.sidebarEntries(40) {
				if e.kind != sidebarLineRow {
					continue
				}
				fmt.Fprintf(&b, "%q\n", m.sidebarContentLine(44, e.gutter, e.text, e.bg))
			}
		}
	}
	return b.String()
}

// TestSessionRowRenderGoldenIsByteIdentical proves task 003/R141 success
// criterion 3's last clause: the session-row render golden is
// byte-identical to its pre-change rendering (see
// sessionRowRenderGoldenPath's comment).
func TestSessionRowRenderGoldenIsByteIdentical(t *testing.T) {
	got := renderSessionRowGolden(t)
	if got != renderSessionRowGolden(t) {
		t.Fatalf("session-row rendering is not deterministic across two renders")
	}
	if !strings.Contains(got, "alpha zero") || !strings.Contains(got, "default row") {
		t.Fatalf("fixture rendered no session rows:\n%s", got)
	}
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll(filepath.Dir(sessionRowRenderGoldenPath), 0o700); err != nil {
			t.Fatalf("create golden directory: %v", err)
		}
		if err := os.WriteFile(sessionRowRenderGoldenPath, []byte(got), 0o600); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		t.Logf("wrote %s (%d bytes)", sessionRowRenderGoldenPath, len(got))
		return
	}
	want, err := os.ReadFile(sessionRowRenderGoldenPath)
	if err != nil {
		t.Fatalf("read golden %s: %v", sessionRowRenderGoldenPath, err)
	}
	if got != string(want) {
		t.Fatalf("session-row rendering drifted from golden %s\n--- got ---\n%s\n--- want ---\n%s", sessionRowRenderGoldenPath, got, string(want))
	}
}
