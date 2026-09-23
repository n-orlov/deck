package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// This file is R73's own test obligation (issue #7): the three scrollable
// overlays -- `?` help, `i` detail and `E` event log -- must scroll by
// exactly ONE line for up/down and their j/k sidebar aliases, while
// PgUp/PgDn keep the whole-page step.
//
// Why the assertions below compare rendered ROWS rather than "the view
// changed": binding up/down to the existing page step (the mistake the
// pre-R73 dialogScrollBy made easy, since a page was its only step size)
// also changes the view, so a changed-view test passes on the wrong
// implementation. Instead each test captures the overlay's content rows,
// presses the key, and requires the new first row to be the row that was
// SECOND before the press -- an offset of exactly 1 wrapped line, which a
// page step cannot satisfy at any frame size where scrolling engages.

// dialogContentRows returns a rendered scrollable overlay's content rows:
// everything between the box's own top and bottom border lines. The rows
// are returned verbatim (border glyphs and padding included) because every
// content row is formatted identically by fullBoxContentLine, so comparing
// one render's row against another's is a comparison of body text alone.
func dialogContentRows(t *testing.T, view string) []string {
	t.Helper()
	lines := strings.Split(view, "\n")
	if len(lines) < 3 {
		t.Fatalf("overlay render has only %d lines, expected a box with content:\n%s", len(lines), view)
	}
	return lines[1 : len(lines)-1]
}

// assertScrolledByOneLine requires after's first content row to be before's
// SECOND content row (dir > 0, scrolled down by one line) or before's first
// content row to be after's second (dir < 0, scrolled up by one line). The
// failure message names how far the view actually moved, so a page-step
// regression reads as "advanced by 20 lines", not merely "differs".
func assertScrolledByOneLine(t *testing.T, overlay, keyName string, before, after []string, dir int) {
	t.Helper()
	if dir < 0 {
		// Retreating by one line is advancing by one line, read the
		// other way round.
		assertScrolledByOneLine(t, overlay, keyName, after, before, 1)
		return
	}
	if len(before) < 2 || len(after) < 2 {
		t.Fatalf("%s: fewer than 2 content rows (%d before, %d after) -- the fixture does not overflow the frame budget, so this test would be vacuous", overlay, len(before), len(after))
	}
	if after[0] == before[1] {
		return
	}
	moved := -1
	for i, row := range before {
		if row == after[0] {
			moved = i
			break
		}
	}
	if moved < 0 {
		t.Fatalf("%s: after %q the first visible row is not any row of the previous view -- it moved by more than one page, want exactly 1 line\nfirst row after: %q\nprevious rows:\n%s",
			overlay, keyName, after[0], strings.Join(before, "\n"))
	}
	t.Fatalf("%s: after %q the first visible row advanced by %d lines, want exactly 1\nfirst row after: %q\nprevious first two rows: %q / %q",
		overlay, keyName, moved, after[0], before[0], before[1])
}

// scrollableOverlay is one fixture: an already-open overlay whose content
// overflows the frame budget, plus the accessors needed to state what a
// whole-page step would be for THAT overlay's own body (the three overlays
// have different bodies, so their max scroll differs).
type scrollableOverlay struct {
	name   string
	model  Model
	scroll func(Model) int
	body   func(Model) string
}

// scrollableOverlayFixtures builds one over-flowing model per scrollable
// overlay, each already open, so the three key bindings are exercised
// through the real Update/View path rather than against the scroll field.
func scrollableOverlayFixtures(t *testing.T) []scrollableOverlay {
	t.Helper()

	help := New(nil, config.Settings{}, "")
	help.width, help.height = 80, 24
	help.help = true

	detail := New(nil, config.Settings{}, "")
	detail.width, detail.height = 80, 24
	detail.sessions = []store.Session{heightBoundTestSession()}
	detail.selected = rowCursor(0)
	detail.detail = true

	db := openEventLogTestStore(t)
	ctx := context.Background()
	for i := 0; i < 30; i++ {
		if err := db.RecordOrphanEvent(ctx, store.EventInput{
			At: int64(i + 1), Kind: "note", Reason: "user",
			Payload: fmt.Sprintf(`{"note":"event-number-%02d"}`, i),
		}); err != nil {
			t.Fatal(err)
		}
	}
	eventLog := New(db, config.Settings{}, "")
	eventLog.width, eventLog.height = 80, 24
	eventLog.eventLogOpen = true
	loaded := eventLog.loadEventLog().(eventLogLoaded)
	eventLog.eventLogRows = loaded.events
	eventLog.eventLogErr = loaded.err

	return []scrollableOverlay{
		{
			name:   "help overlay",
			model:  help,
			scroll: func(m Model) int { return m.helpScroll },
			body:   func(m Model) string { return helpText(m.settings.ASCII) },
		},
		{
			name:   "detail view",
			model:  detail,
			scroll: func(m Model) int { return m.detailScroll },
			body:   func(m Model) string { return m.detailBody() },
		},
		{
			name:   "event log",
			model:  eventLog,
			scroll: func(m Model) int { return m.eventLogScroll },
			body:   func(m Model) string { return m.eventLogBody() },
		},
	}
}

// pressKey sends one keystroke through the real Update and returns the model.
func pressKey(t *testing.T, m Model, name string) Model {
	t.Helper()
	updated, _ := m.Update(key(name))
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("Update(%q) returned %T, not a tui.Model", name, updated)
	}
	return next
}

// TestScrollableOverlaysScrollOneLineWithArrows proves the down/up arrows
// move each of the three scrollable overlays by exactly one wrapped body
// line: down advances the first visible row by one, a second down by one
// more, and up retreats it by exactly one again.
func TestScrollableOverlaysScrollOneLineWithArrows(t *testing.T) {
	for _, fixture := range scrollableOverlayFixtures(t) {
		t.Run(fixture.name, func(t *testing.T) {
			m := fixture.model
			rows0 := dialogContentRows(t, m.View())

			m1 := pressKey(t, m, "down")
			rows1 := dialogContentRows(t, m1.View())
			assertScrolledByOneLine(t, fixture.name, "down", rows0, rows1, 1)

			m2 := pressKey(t, m1, "down")
			rows2 := dialogContentRows(t, m2.View())
			assertScrolledByOneLine(t, fixture.name, "down (second press)", rows1, rows2, 1)

			m3 := pressKey(t, m2, "up")
			rows3 := dialogContentRows(t, m3.View())
			assertScrolledByOneLine(t, fixture.name, "up", rows2, rows3, -1)
			if rows3[0] != rows1[0] {
				t.Fatalf("%s: up did not return to the row two downs had reached one line earlier\nwant first row: %q\ngot:            %q", fixture.name, rows1[0], rows3[0])
			}
		})
	}
}

// TestScrollableOverlaysScrollOneLineWithJK proves j/k -- the sidebar's own
// aliases for down/up -- do exactly the same one-line step in all three
// overlays, so a user navigating with them does not find them dead once an
// overlay is open.
func TestScrollableOverlaysScrollOneLineWithJK(t *testing.T) {
	for _, fixture := range scrollableOverlayFixtures(t) {
		t.Run(fixture.name, func(t *testing.T) {
			m := fixture.model
			rows0 := dialogContentRows(t, m.View())

			m1 := pressKey(t, m, "j")
			rows1 := dialogContentRows(t, m1.View())
			assertScrolledByOneLine(t, fixture.name, "j", rows0, rows1, 1)

			m2 := pressKey(t, m1, "j")
			rows2 := dialogContentRows(t, m2.View())
			assertScrolledByOneLine(t, fixture.name, "j (second press)", rows1, rows2, 1)

			m3 := pressKey(t, m2, "k")
			rows3 := dialogContentRows(t, m3.View())
			assertScrolledByOneLine(t, fixture.name, "k", rows2, rows3, -1)
			if rows3[0] != rows1[0] {
				t.Fatalf("%s: k did not return to the row two js had reached one line earlier\nwant first row: %q\ngot:            %q", fixture.name, rows1[0], rows3[0])
			}
		})
	}
}

// TestScrollableOverlaysStillPageWithPgDn proves R73 did not turn PgUp/PgDn
// into the one-line step: one PgDn still moves the first visible row by a
// whole content budget (or to the very bottom, when the body has less than
// a page left below the first screen -- the event log's case), and PgUp
// brings the same first row back.
func TestScrollableOverlaysStillPageWithPgDn(t *testing.T) {
	for _, fixture := range scrollableOverlayFixtures(t) {
		t.Run(fixture.name, func(t *testing.T) {
			m := fixture.model
			rows0 := dialogContentRows(t, m.View())

			// What a page step means for this body: the whole content
			// budget, clamped by the body's own remaining length.
			wantOffset := m.dialogContentBudget()
			if max := m.dialogMaxScroll(fixture.body(m)); wantOffset > max {
				wantOffset = max
			}
			if wantOffset < 2 {
				t.Fatalf("%s: a page step here is only %d lines, so this test could not tell a page from a line -- the fixture needs more content", fixture.name, wantOffset)
			}

			paged := pressKey(t, m, "pgdown")
			if got := fixture.scroll(paged); got != wantOffset {
				t.Fatalf("%s: pgdown moved the scroll offset to %d, want %d (one full page, clamped)", fixture.name, got, wantOffset)
			}
			rowsPaged := dialogContentRows(t, paged.View())
			if wantOffset < len(rows0) {
				if rowsPaged[0] != rows0[wantOffset] {
					t.Fatalf("%s: after pgdown the first visible row is not the previous view's row %d\nwant: %q\ngot:  %q", fixture.name, wantOffset, rows0[wantOffset], rowsPaged[0])
				}
			} else {
				for i, row := range rows0 {
					if row == rowsPaged[0] {
						t.Fatalf("%s: after pgdown the first visible row was row %d of the previous page -- pgdown no longer steps a whole page\nrow: %q", fixture.name, i, row)
					}
				}
			}

			back := pressKey(t, paged, "pgup")
			rowsBack := dialogContentRows(t, back.View())
			if rowsBack[0] != rows0[0] {
				t.Fatalf("%s: pgup did not page back to the top row\nwant: %q\ngot:  %q", fixture.name, rows0[0], rowsBack[0])
			}
		})
	}
}
