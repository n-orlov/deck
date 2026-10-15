package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// R315: a running row with no event for 3 x stale_after and three consecutive
// probe misses shows `◔` (ASCII `-`) in the running glyph's column. Status
// text and sort position are an ordinary running row's.

const unverifiedStale = 45 * time.Second

func unverifiedModel(ascii bool, rows ...store.Session) Model {
	clock, err := config.NewClock(time.UnixMilli(10_000_000).UTC().Format(time.RFC3339Nano), "")
	if err != nil {
		panic(err)
	}
	m := New(nil, config.Settings{ASCII: ascii, StaleAfter: unverifiedStale, Clock: clock}, "")
	m.width, m.height = 80, 24
	m.sessions = rows
	m.selected = rowCursor(-1)
	return m
}

func quietRunning(id string, misses int) store.Session {
	return store.Session{ID: id, Name: id, Agent: "claude", Status: "running", CreatedAt: 1000,
		StatusAt: 10_000_000 - 3*unverifiedStale.Milliseconds(), ProbeMissStreak: misses}
}

func unverifiedRowLine(t *testing.T, view, name string) string {
	t.Helper()
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, name) {
			return line
		}
	}
	t.Fatalf("no row for %q in:\n%s", name, view)
	return ""
}

func TestUnverifiedGlyphAppearsAtThreeMissesNotTwo(t *testing.T) {
	for _, mode := range []struct {
		name, plain, marked string
		ascii               bool
	}{
		{"unicode", "◐", "◔", false},
		{"ascii", "~", "-", true},
	} {
		t.Run(mode.name, func(t *testing.T) {
			m := unverifiedModel(mode.ascii, quietRunning("twomiss", 2), quietRunning("threemiss", 3))
			view := m.View()
			two, three := unverifiedRowLine(t, view, "twomiss"), unverifiedRowLine(t, view, "threemiss")
			if !strings.Contains(two, mode.plain+" twomiss") || strings.Contains(two, mode.marked+" ") {
				t.Fatalf("two misses should keep the running glyph %q: %q", mode.plain, two)
			}
			if !strings.Contains(three, mode.marked+" threemiss") || strings.Contains(three, mode.plain+" threemiss") {
				t.Fatalf("three misses should show %q in place of %q: %q", mode.marked, mode.plain, three)
			}
		})
	}
}

func TestUnverifiedNeedsTheQuietWindowToo(t *testing.T) {
	row := quietRunning("recent", 3)
	row.StatusAt = 10_000_000 - unverifiedStale.Milliseconds()
	view := unverifiedModel(false, row).View()
	if line := unverifiedRowLine(t, view, "recent"); strings.Contains(line, "◔") {
		t.Fatalf("a row with an event 1 x stale_after ago was marked unverified: %q", line)
	}
}

func TestUnverifiedLeavesTheStatusTextUnchanged(t *testing.T) {
	plain := unverifiedRowLine(t, unverifiedModel(false, quietRunning("same", 0)).View(), "same")
	marked := unverifiedRowLine(t, unverifiedModel(false, quietRunning("same", 3)).View(), "same")
	if !strings.Contains(marked, "running") {
		t.Fatalf("unverified row lost its status word: %q", marked)
	}
	if strings.Contains(strings.ToLower(marked), "unverified") {
		t.Fatalf("unverified row states new text: %q", marked)
	}
	// Only the one glyph column differs.
	if strings.Replace(marked, "◔", "◐", 1) != plain {
		t.Fatalf("rows differ beyond the glyph:\nplain:  %q\nmarked: %q", plain, marked)
	}
}

func TestUnverifiedLeavesTheSortPositionUnchanged(t *testing.T) {
	base := func(misses int) []store.Session {
		a, b, c := quietRunning("a", 0), quietRunning("b", misses), quietRunning("c", 0)
		a.CreatedAt, b.CreatedAt, c.CreatedAt = 100, 200, 300
		a.LastOutputAt, b.LastOutputAt, c.LastOutputAt = 100_000, 300_000, 200_000
		return []store.Session{a, b, c}
	}
	for _, order := range []string{SortOrderAttention, SortOrderCreated, SortOrderActivity, SortOrderName} {
		ids := func(rows []store.Session) string {
			var out []string
			for _, row := range sortSessionsByOrder(nil, rows, order, 0) {
				out = append(out, row.ID)
			}
			return strings.Join(out, ",")
		}
		if got, want := ids(base(3)), ids(base(0)); got != want {
			t.Fatalf("order %s: unverified row moved the list: %s, want %s", order, got, want)
		}
	}
}

// TestRecentEventKeepsAnOldRunningRowVerified: the quiet window counts from
// the newest event, not status_at. A repeat verdict 1.5 s ago (status_at left
// ancient by R303) keeps the plain running glyph despite three misses.
func TestRecentEventKeepsAnOldRunningRowVerified(t *testing.T) {
	row := quietRunning("recentevent", 3)
	row.StatusAt = 1000
	row.LastEventAt = 10_000_000 - 1500
	line := unverifiedRowLine(t, unverifiedModel(false, row).View(), "recentevent")
	if !strings.Contains(line, "◐ recentevent") || strings.Contains(line, "◔") {
		t.Fatalf("a row with an event 1.5 s ago was decorated unverified: %q", line)
	}
}
