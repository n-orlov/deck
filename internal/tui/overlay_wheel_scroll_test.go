package tui

import (
	"context"
	"os/exec"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// This file is the second half of R73's test obligation (issue #7): the
// wheel is routed to EXACTLY the three scrollable overlays, and clicks and
// drags stay suppressed for all fifteen.
//
// Both halves matter, and each catches a different wrong implementation:
//
//   - "the wheel scrolls the three" fails on the pre-R73 tree, where one
//     blanket early return over fifteen overlay flags discarded every
//     MouseMsg before handleMouse ever saw it.
//   - "a click inside a dialog still does nothing" fails on the obvious
//     naive fix -- moving the whole mouse-handling path inside the guard for
//     scrollable overlays, or deleting the guard's help/detail/eventLogOpen
//     flags -- which lets a press fall through to the sidebar underneath and
//     re-enter interactive mode from behind a modal dialog.
//
// The scroll assertions reuse overlay_line_scroll_test.go's fixtures and its
// exactly-one-line comparison (dialogContentRows/assertScrolledByOneLine),
// so a wheel wired to the PAGE step is a failure here for the same reason it
// is a failure for `down`.

// wheelOverlayFixtures is scrollableOverlayFixtures with mouse reporting
// enabled, since Update's own DECK_MOUSE gate (requirement 3/37) drops every
// MouseMsg before anything else when m.settings.Mouse is false.
func wheelOverlayFixtures(t *testing.T) []scrollableOverlay {
	t.Helper()
	fixtures := scrollableOverlayFixtures(t)
	for i := range fixtures {
		fixtures[i].model.settings.Mouse = true
	}
	return fixtures
}

// sendMouse pushes one mouse event through the real Update and returns the
// model together with the command Update produced, so a test can assert
// "and nothing was dispatched either".
func sendMouse(t *testing.T, m Model, e tea.MouseMsg) (Model, tea.Cmd) {
	t.Helper()
	updated, cmd := m.Update(e)
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("Update(mouse %v) returned %T, not a tui.Model", e, updated)
	}
	return next, cmd
}

// TestWheelScrollsScrollableOverlaysByOneLine proves a wheel notch moves
// each of the three scrollable overlays by exactly one wrapped body line --
// down advances the first visible row by one, a second notch by one more,
// and up retreats it by exactly one again -- which is both that the wheel is
// routed at all and that it uses the line step rather than the page step.
func TestWheelScrollsScrollableOverlaysByOneLine(t *testing.T) {
	for _, fixture := range wheelOverlayFixtures(t) {
		t.Run(fixture.name, func(t *testing.T) {
			m := fixture.model
			rows0 := dialogContentRows(t, m.View())

			m1, cmd := sendMouse(t, m, wheelDown(10, 5))
			if cmd != nil {
				t.Fatalf("%s: a wheel notch dispatched a command; scrolling a viewport must reach no action", fixture.name)
			}
			rows1 := dialogContentRows(t, m1.View())
			assertScrolledByOneLine(t, fixture.name, "wheel down", rows0, rows1, 1)
			if got := fixture.scroll(m1); got != 1 {
				t.Fatalf("%s: one wheel notch left the scroll offset at %d, want 1", fixture.name, got)
			}

			m2, _ := sendMouse(t, m1, wheelDown(10, 5))
			rows2 := dialogContentRows(t, m2.View())
			assertScrolledByOneLine(t, fixture.name, "wheel down (second notch)", rows1, rows2, 1)

			m3, _ := sendMouse(t, m2, wheelUp(10, 5))
			rows3 := dialogContentRows(t, m3.View())
			assertScrolledByOneLine(t, fixture.name, "wheel up", rows2, rows3, -1)
			if rows3[0] != rows1[0] {
				t.Fatalf("%s: wheel up did not return to the row two notches had reached one line earlier\nwant first row: %q\ngot:            %q", fixture.name, rows1[0], rows3[0])
			}
		})
	}
}

// TestWheelInOverlayAgreesWithArrowKeys proves the wheel and `down` are the
// same step over the same body in all three overlays: after one notch and
// after one keypress the rendered overlay is byte-identical. A wheel wired
// to its own private arithmetic (a second scroller, which R73 explicitly
// must not grow) would drift from the keyboard here.
func TestWheelInOverlayAgreesWithArrowKeys(t *testing.T) {
	for _, fixture := range wheelOverlayFixtures(t) {
		t.Run(fixture.name, func(t *testing.T) {
			wheeled, _ := sendMouse(t, fixture.model, wheelDown(10, 5))
			pressed := pressKey(t, fixture.model, "down")
			if wheeled.View() != pressed.View() {
				t.Fatalf("%s: one wheel notch and one `down` render differently -- the wheel is not using dialogScrollByLines\nwheel:\n%s\n\ndown:\n%s", fixture.name, wheeled.View(), pressed.View())
			}
			if got, want := fixture.scroll(wheeled), fixture.scroll(pressed); got != want {
				t.Fatalf("%s: wheel left the offset at %d, `down` at %d", fixture.name, got, want)
			}
		})
	}
}

// TestWheelInOverlayHonoursMouseOptOut proves R73 did not open a hole in the
// DECK_MOUSE opt-out (requirement 3/37): with mouse reporting off, a wheel
// report that arrives anyway still scrolls nothing.
func TestWheelInOverlayHonoursMouseOptOut(t *testing.T) {
	for _, fixture := range scrollableOverlayFixtures(t) { // mouse deliberately NOT enabled
		t.Run(fixture.name, func(t *testing.T) {
			before := fixture.model.View()
			after, _ := sendMouse(t, fixture.model, wheelDown(10, 5))
			if got := fixture.scroll(after); got != 0 {
				t.Fatalf("%s: with mouse reporting disabled a wheel notch moved the offset to %d, want 0", fixture.name, got)
			}
			if after.View() != before {
				t.Fatalf("%s: with mouse reporting disabled a wheel notch changed the frame", fixture.name)
			}
		})
	}
}

// overlayClickFixture is one open overlay plus the reason it is here: a
// scrollable one (so the wheel routing cannot be mistaken for permission to
// click) and an unscrollable one (so the fifteen-flag guard is still proven
// for the twelve overlays R73 does not touch).
type overlayClickFixture struct {
	name       string
	model      Model
	scrollable bool
}

// clickTestSessions is a list long enough that the sidebar itself is
// scrollable, so a wheel event leaking past an unscrollable overlay would
// move m.sidebarScroll and be caught rather than silently absorbed.
func clickTestSessions() []store.Session {
	sessions := make([]store.Session, 0, 40)
	for i := 0; i < 40; i++ {
		s := heightBoundTestSession()
		s.ID = "s" + string(rune('a'+i%26)) + string(rune('0'+i/26))
		s.Name = "row-" + s.ID
		sessions = append(sessions, s)
	}
	return sessions
}

// overlayClickFixtures opens one scrollable and two unscrollable overlays
// over the same populated session list, each with mouse reporting on.
func overlayClickFixtures(t *testing.T) []overlayClickFixture {
	t.Helper()
	base := func() Model {
		m := New(nil, config.Settings{Mouse: true}, "")
		m.width, m.height = 80, 24
		m.sessions = clickTestSessions()
		m.selected = rowCursor(0)
		m.attach = func(context.Context, string) (*exec.Cmd, error) { return exec.Command("true"), nil }
		return m
	}

	detail := base()
	detail.detail = true

	help := base()
	help.help = true

	// The delete confirm is reached through its real `dd` chord so the
	// fixture is the dialog the product actually opens.
	deleteConfirm := pressKey(t, pressKey(t, base(), "d"), "d")
	if !deleteConfirm.deleteConfirming {
		t.Fatal("fixture: dd did not open the delete confirm dialog")
	}

	rename := base()
	rename.renaming = true

	return []overlayClickFixture{
		{name: "detail view (scrollable)", model: detail, scrollable: true},
		{name: "help overlay (scrollable)", model: help, scrollable: true},
		{name: "delete confirm (not scrollable)", model: deleteConfirm},
		{name: "rename dialog (not scrollable)", model: rename},
	}
}

// TestClickInsideOverlayStillDoesNothing is the other half of R73's routing
// criterion: with the wheel now let through, a PRESS, a drag and a release
// must still be discarded for every overlay -- scrollable or not -- so no
// dialog action becomes reachable by mouse (SPEC.md:1250, §11.4/§11.8).
// The assertion is on state and on the frame: had the press fallen through
// to the sidebar underneath, it would have selected a row and entered
// interactive mode (requirement 33's own click gesture).
func TestClickInsideOverlayStillDoesNothing(t *testing.T) {
	for _, fixture := range overlayClickFixtures(t) {
		t.Run(fixture.name, func(t *testing.T) {
			m := fixture.model
			before := m.View()
			gestures := []struct {
				what string
				msg  tea.MouseMsg
			}{
				{"press on the dialog's own top-left border", press(1, 1)},
				{"press over the dialog's body text", press(40, 5)},
				{"press over the hidden sidebar row underneath", press(5, 4)},
				{"press outside the dialog box", press(78, 22)},
				{"drag across the dialog", motion(30, 10)},
				{"release inside the dialog", release(30, 10)},
				{"second press (the double-click attach gesture)", press(5, 4)},
			}
			for _, g := range gestures {
				next, cmd := sendMouse(t, m, g.msg)
				if cmd != nil {
					t.Fatalf("%s: %s dispatched a command -- a dialog action became reachable by mouse", fixture.name, g.what)
				}
				if next.interactive {
					t.Fatalf("%s: %s entered interactive mode from behind the overlay", fixture.name, g.what)
				}
				if next.selected != m.selected {
					t.Fatalf("%s: %s moved the selection from %d to %d", fixture.name, g.what, m.selected, next.selected)
				}
				if next.sidebarScroll != m.sidebarScroll {
					t.Fatalf("%s: %s scrolled the sidebar underneath (%d -> %d)", fixture.name, g.what, m.sidebarScroll, next.sidebarScroll)
				}
				if next.detailScroll != m.detailScroll || next.helpScroll != m.helpScroll || next.eventLogScroll != m.eventLogScroll {
					t.Fatalf("%s: %s scrolled an overlay viewport -- only the wheel may do that", fixture.name, g.what)
				}
				if next.detail != m.detail || next.help != m.help || next.renaming != m.renaming || next.deleteConfirming != m.deleteConfirming || next.eventLogOpen != m.eventLogOpen {
					t.Fatalf("%s: %s cancelled or opened an overlay", fixture.name, g.what)
				}
				if got := next.View(); got != before {
					t.Fatalf("%s: %s changed the frame\nbefore:\n%s\n\nafter:\n%s", fixture.name, g.what, before, got)
				}
				m = next
			}
		})
	}
}

// TestWheelOverUnscrollableOverlayDoesNothing proves the routing is "the
// three scrollable overlays", not "any overlay": a wheel notch while an
// unscrollable dialog is open must neither scroll the dialog (it has no
// viewport) nor leak through to the sidebar list underneath it, which is
// what the blanket guard prevented before R73 and must still prevent.
func TestWheelOverUnscrollableOverlayDoesNothing(t *testing.T) {
	for _, fixture := range overlayClickFixtures(t) {
		if fixture.scrollable {
			continue
		}
		t.Run(fixture.name, func(t *testing.T) {
			m := fixture.model
			before := m.View()
			for _, e := range []tea.MouseMsg{wheelDown(5, 4), wheelDown(40, 12), wheelUp(5, 4)} {
				next, cmd := sendMouse(t, m, e)
				if cmd != nil {
					t.Fatalf("%s: a wheel notch dispatched a command", fixture.name)
				}
				if next.sidebarScroll != m.sidebarScroll {
					t.Fatalf("%s: a wheel notch scrolled the sidebar underneath the dialog (%d -> %d)", fixture.name, m.sidebarScroll, next.sidebarScroll)
				}
				if got := next.View(); got != before {
					t.Fatalf("%s: a wheel notch changed the frame\nbefore:\n%s\n\nafter:\n%s", fixture.name, before, got)
				}
				m = next
			}
		})
	}
}

// TestWheelStillScrollsTheSidebarWithNoOverlayOpen guards the fall-through:
// R73's new early return must fire ONLY while a scrollable overlay is open,
// leaving requirement 34's own wheel binding (wheel over the sidebar scrolls
// the list without changing selection) exactly as it was.
func TestWheelStillScrollsTheSidebarWithNoOverlayOpen(t *testing.T) {
	m := New(nil, config.Settings{Mouse: true}, "")
	m.width, m.height = 80, 24
	m.sessions = clickTestSessions()
	before := m.selected

	scrolled, _ := sendMouse(t, m, wheelDown(5, 4))
	if scrolled.sidebarScroll != 1 {
		t.Fatalf("wheel down over the sidebar left sidebarScroll at %d, want 1", scrolled.sidebarScroll)
	}
	if scrolled.selected != before {
		t.Fatalf("wheel down over the sidebar changed the selection from %d to %d", before, scrolled.selected)
	}
}
