package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// R324: the unseen marker is one glyph, distinct from the `●` waiting status
// glyph, with an ASCII form; the sidebar header carries "N unseen"; and
// setting or clearing the marker never moves a row in any sort order.

func unseenModel(ascii bool) Model {
	m := New(nil, config.Settings{Socket: "deck", ASCII: ascii}, "")
	m.sessions = []store.Session{
		{ID: "quiet-out", Name: "quiet-out", Status: "idle", LastOutputAt: 500, LastSeenAt: 100},
		{ID: "viewed", Name: "viewed", Status: "idle", LastOutputAt: 500, LastSeenAt: 500},
		{ID: "busy", Name: "busy", Status: "running", LastOutputAt: 900, LastSeenAt: 0},
		{ID: "ask", Name: "ask", Status: "waiting", StatusAt: 50},
	}
	return m
}

func unseenRowText(m Model, name string) string {
	for _, e := range m.sidebarEntries(60) {
		if e.kind == sidebarLineRow && strings.Contains(e.text, name) {
			return e.text
		}
	}
	return ""
}

func TestUnseenMarkerIsDistinctFromTheWaitingGlyphAndHasAnASCIIForm(t *testing.T) {
	m := unseenModel(false)
	if m.unseenMarker() == m.sidebarStatusGlyph("waiting") {
		t.Fatalf("the unseen marker %q collides with the waiting status glyph", m.unseenMarker())
	}
	for _, status := range []string{"waiting", "running", "idle", "starting", "stopped", "error"} {
		if m.unseenMarker() == m.sidebarStatusGlyph(status) {
			t.Fatalf("the unseen marker %q collides with the %s glyph", m.unseenMarker(), status)
		}
	}
	a := unseenModel(true)
	if a.unseenMarker() != "!" {
		t.Fatalf("ascii unseen marker = %q, want %q", a.unseenMarker(), "!")
	}
	for _, status := range []string{"waiting", "running", "idle", "starting", "stopped", "error"} {
		if a.unseenMarker() == a.sidebarStatusGlyph(status) {
			t.Fatalf("the ascii unseen marker collides with the ascii %s glyph", status)
		}
	}
	if got := unseenRowText(m, "quiet-out"); !strings.Contains(got, m.unseenMarker()) {
		t.Fatalf("an idle row with unviewed output lacks the marker: %q", got)
	}
	if got := unseenRowText(m, "ask"); !strings.Contains(got, m.unseenMarker()) {
		t.Fatalf("an unseen waiting row lacks the marker: %q", got)
	}
	if got := unseenRowText(m, "viewed"); strings.Contains(got, m.unseenMarker()) {
		t.Fatalf("a viewed row carries the marker: %q", got)
	}
	if got := unseenRowText(m, "busy"); strings.Contains(got, m.unseenMarker()) {
		t.Fatalf("a running row carries an output-unseen marker: %q", got)
	}
}

func TestHeaderShowsTheUnseenCountAndTheCountFollowsTheMarker(t *testing.T) {
	m := unseenModel(false)
	if got := m.sidebarTitleText(); !strings.Contains(got, "2 unseen") {
		t.Fatalf("header %q lacks %q", got, "2 unseen")
	}
	if got := unseenModel(true).sidebarTitleText(); !strings.Contains(got, "2 unseen") || !strings.Contains(got, " - ") {
		t.Fatalf("ascii header %q lacks the count or the ascii separator", got)
	}
	m.sessions[0].LastSeenAt = 500 // the view clears one
	if got := m.sidebarTitleText(); !strings.Contains(got, "1 unseen") {
		t.Fatalf("header %q after a view lacks %q", got, "1 unseen")
	}
	m.sessions[3].SeenStatusAt = m.sessions[3].StatusAt
	if got := m.sidebarTitleText(); strings.Contains(got, "unseen") {
		t.Fatalf("header %q still counts after everything was seen", got)
	}
}

func unseenSortRows() []store.Session {
	return []store.Session{
		{ID: "a", Name: "Alpha", Status: "waiting", StatusAt: 40, CreatedAt: 1000, LastOutputAt: 300_000, LastSeenAt: 300_000, SeenStatusAt: 40},
		{ID: "b", Name: "bravo", Status: "idle", StatusAt: 30, CreatedAt: 3000, LastOutputAt: 200_000, LastSeenAt: 200_000},
		{ID: "c", Name: "Charlie", Status: "error", StatusAt: 20, CreatedAt: 2000, LastOutputAt: 100_000, LastSeenAt: 100_000, SeenStatusAt: 20},
		{ID: "d", Name: "delta", Status: "running", StatusAt: 10, CreatedAt: 4000, LastOutputAt: 400_000, LastSeenAt: 400_000},
		{ID: "e", Name: "echo", Status: "stopped", StatusAt: 5, CreatedAt: 500, LastOutputAt: 50_000, LastSeenAt: 50_000},
	}
}

// flipMarkers sets and clears the marker on every row that can show it: output
// unviewed (last_seen_at behind last_output_at), and for a waiting row an
// unseen status. (A seen/unseen *error* moves between tiers by R319's own
// rule; that is a status-seen change, not this requirement's marker.)
func flipMarkers(rows []store.Session, on bool) []store.Session {
	out := append([]store.Session(nil), rows...)
	for i := range out {
		if on {
			out[i].LastSeenAt = 0
			if out[i].Status == "waiting" {
				out[i].SeenStatusAt = 0
			}
		} else {
			out[i].LastSeenAt = out[i].LastOutputAt
			if out[i].Status == "waiting" {
				out[i].SeenStatusAt = out[i].StatusAt
			}
		}
	}
	return out
}

func assertMarkerFlipMovesNoRow(t *testing.T, order string) {
	t.Helper()
	width := 10 * time.Second
	base := sortSessionsByOrder(nil, flipMarkers(unseenSortRows(), false), order, width)
	want := idsOf(base)
	// Marker on: some rows now unseen, nothing else changed.
	on := flipMarkers(base, true)
	nUnseen := 0
	for _, s := range on {
		if s.Unseen() {
			nUnseen++
		}
	}
	if nUnseen == 0 {
		t.Fatal("fixture: flipping the markers on set no marker")
	}
	got := sortSessionsByOrder(base, on, order, width)
	assertIDOrder(t, got, want)
	// And with the previous frame unknown (a fresh load), the order is the same.
	assertIDOrder(t, sortSessionsByOrder(nil, on, order, width), want)
	// Marker off again: back to the same order.
	assertIDOrder(t, sortSessionsByOrder(got, flipMarkers(got, false), order, width), want)
}

func TestMarkerFlipMovesNoRowAttentionOrder(t *testing.T) {
	assertMarkerFlipMovesNoRow(t, SortOrderAttention)
}
func TestMarkerFlipMovesNoRowCreatedOrder(t *testing.T) {
	assertMarkerFlipMovesNoRow(t, SortOrderCreated)
}
func TestMarkerFlipMovesNoRowActivityOrder(t *testing.T) {
	assertMarkerFlipMovesNoRow(t, SortOrderActivity)
}
func TestMarkerFlipMovesNoRowNameOrder(t *testing.T) {
	assertMarkerFlipMovesNoRow(t, SortOrderName)
}
