package tui

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// This file is task 078's own test obligation (requirement 39 residual,
// widened by operator steer 004 to include the `i` detail view): the help
// overlay, `E` event log and `i` detail view must each render at most 24
// lines at 80x24 for content that genuinely overflows that budget, and a
// user must still be able to reach every line via PgUp/PgDn rather than
// having content silently truncated away.

// realisticLastMessage matches steer 004's own measurement against the
// operator's live state DB (three real sessions at 2032-2040 characters) --
// a short fixture that happens to already fit the frame budget would pass
// whether the height-bounding fix landed or not, so this is deliberately
// long enough to overflow it.
func realisticLastMessage() string {
	var b strings.Builder
	for i := 0; b.Len() < 2036; i++ {
		fmt.Fprintf(&b, "The agent reported progress on step chunk%04d of the requested change, describing the files it touched and the tests it ran. ", i)
	}
	return strings.TrimRight(b.String(), " ")
}

func heightBoundTestSession() store.Session {
	return store.Session{
		ID:                "s1",
		Name:              "alpha",
		Agent:             "claude",
		CWD:               "/repo/alpha",
		Status:            "waiting",
		StatusReason:      "needs input",
		StatusSource:      "hook",
		StatusAt:          1,
		PermissionProfile: "safe",
		ConversationID:    "conv-123",
		LastMessage:       realisticLastMessage(),
	}
}

// countViewLines is this file's own line-count helper: strings.Split's
// "\n" count, matching how every line-budget assertion elsewhere in this
// package already counts a rendered View().
func countViewLines(view string) int {
	return len(strings.Split(view, "\n"))
}

// TestHelpOverlayStaysWithinFrameBudgetAt80x24 proves the `?` help
// overlay -- 273 lines unbounded at this width (per help_keymap_parity_
// test.go's own measurement) -- never exceeds 24 rendered lines at deck's
// documented 80x24 minimum once framedDialogScrollable clips it.
func TestHelpOverlayStaysWithinFrameBudgetAt80x24(t *testing.T) {
	model := New(nil, config.Settings{}, "")
	model.width, model.height = 80, 24
	model.help = true
	view := model.View()
	if n := countViewLines(view); n > 24 {
		t.Fatalf("help view is %d lines at 80x24, want <= 24:\n%s", n, view)
	}
}

// TestEventLogStaysWithinFrameBudgetAt80x24 proves the `E` event log never
// exceeds 24 rendered lines at 80x24 even with far more events than the
// frame could ever show at once (30, one line per event, comfortably past
// the ~20-line content budget the box leaves once its own header/footer
// text and border are accounted for).
func TestEventLogStaysWithinFrameBudgetAt80x24(t *testing.T) {
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
	model := New(db, config.Settings{}, "")
	model.width, model.height = 80, 24
	model.eventLogOpen = true
	loaded := model.loadEventLog().(eventLogLoaded)
	model.eventLogRows = loaded.events
	model.eventLogErr = loaded.err
	view := model.View()
	if n := countViewLines(view); n > 24 {
		t.Fatalf("event log view is %d lines at 80x24, want <= 24:\n%s", n, view)
	}
}

// TestDetailViewStaysWithinFrameBudgetAt80x24 proves the `i` detail view
// never exceeds 24 rendered lines at 80x24 for a session whose Last
// message is a realistic ~2036-character hook payload (steer 004's own
// measurement) -- ~25 lines of labelled fields plus ~27 wrapped Last
// message lines plus 2 border lines, all unbounded before this task's fix.
func TestDetailViewStaysWithinFrameBudgetAt80x24(t *testing.T) {
	model := New(nil, config.Settings{}, "")
	model.width, model.height = 80, 24
	model.sessions = []store.Session{heightBoundTestSession()}
	model.selected = 0
	model.detail = true
	view := model.View()
	if n := countViewLines(view); n > 24 {
		t.Fatalf("detail view is %d lines at 80x24, want <= 24:\n%s", n, view)
	}
}

// TestHelpOverlayScrollReachesEveryLine proves PgDn/PgUp actually move a
// user through the WHOLE overlay rather than merely clipping the tail
// away: the closing "q quits deck." sentence -- absent from the first
// 80x24 page -- becomes visible after paging all the way down, and paging
// back up returns to a view containing the very first Keys-section entry
// again.
func TestHelpOverlayScrollReachesEveryLine(t *testing.T) {
	model := New(nil, config.Settings{}, "")
	model.width, model.height = 80, 24
	model.help = true

	first := model.View()
	if strings.Contains(first, "q quits deck.") {
		t.Fatalf("the closing sentence is already visible on the first page -- this test needs a shorter/taller frame to be non-vacuous:\n%s", first)
	}

	m := model
	for i := 0; i < 40; i++ { // comfortably more presses than dialogMaxScroll needs
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
		m = updated.(Model)
	}
	bottom := m.View()
	if !strings.Contains(bottom, "q quits deck.") {
		t.Fatalf("paging all the way down never reached the closing sentence:\n%s", bottom)
	}

	for i := 0; i < 40; i++ {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyPgUp})
		m = updated.(Model)
	}
	top := m.View()
	if !strings.Contains(top, "select a session") {
		t.Fatalf("paging all the way back up did not return to the top of the Keys section:\n%s", top)
	}
	if m.helpScroll != 0 {
		t.Fatalf("helpScroll = %d after paging fully back up, want 0", m.helpScroll)
	}
}

// TestEventLogScrollReachesEveryLine mirrors the help overlay's own
// scroll-reaches-every-line proof for the `E` event log: the oldest (and
// therefore last-rendered, newest-first) event's payload is absent from
// the first 80x24 page and becomes visible only after paging down.
func TestEventLogScrollReachesEveryLine(t *testing.T) {
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
	model := New(db, config.Settings{}, "")
	model.width, model.height = 80, 24
	model.eventLogOpen = true
	loaded := model.loadEventLog().(eventLogLoaded)
	model.eventLogRows = loaded.events
	model.eventLogErr = loaded.err

	first := model.View()
	if strings.Contains(first, "event-number-00") {
		t.Fatalf("the oldest event is already visible on the first page -- this test needs more events to be non-vacuous:\n%s", first)
	}

	m := model
	for i := 0; i < 40; i++ {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
		m = updated.(Model)
	}
	bottom := m.View()
	if !strings.Contains(bottom, "event-number-00") {
		t.Fatalf("paging all the way down never reached the oldest event:\n%s", bottom)
	}
}

// TestDetailViewScrollReachesEveryLineAndFieldsScrollTogether proves task
// 078's whole-dialog-scrolls-uniformly requirement: paging down moves the
// EARLIER fields (e.g. "Agent:") out of view exactly as the Last message
// text scrolls into it -- there is no separate, per-field scroll window
// for Last message -- and paging all the way down eventually reaches the
// tail of the (realistic, ~2036-character) Last message text that the
// first page cannot show.
func TestDetailViewScrollReachesEveryLineAndFieldsScrollTogether(t *testing.T) {
	session := heightBoundTestSession()
	// The message's own last chunk marker (a single unbroken token,
	// guaranteed never split across two wrapped lines by wrapText's
	// word-boundary wrapping) stands in for "the tail of the message":
	// unlike an arbitrary raw substring of the original text, this can
	// never straddle a wrap-inserted newline and produce a false negative.
	matches := regexp.MustCompile(`chunk\d{4}`).FindAllString(session.LastMessage, -1)
	if len(matches) == 0 {
		t.Fatalf("test fixture has no chunk marker to look for:\n%s", session.LastMessage)
	}
	tail := matches[len(matches)-1]

	model := New(nil, config.Settings{}, "")
	model.width, model.height = 80, 24
	model.sessions = []store.Session{session}
	model.selected = 0
	model.detail = true

	first := model.View()
	if strings.Contains(first, tail) {
		t.Fatalf("the Last message's own tail is already visible on the first page -- this test needs a longer message to be non-vacuous:\n%s", first)
	}
	if !strings.Contains(first, "Agent:") {
		t.Fatalf("the first page does not show the Agent field:\n%s", first)
	}

	m := model
	for i := 0; i < 40; i++ {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
		m = updated.(Model)
	}
	bottom := m.View()
	if !strings.Contains(bottom, tail) {
		t.Fatalf("paging all the way down never reached the Last message's own tail:\n%s", bottom)
	}
	// Uniform scroll, not a per-field window: once scrolled far enough to
	// show the Last message's tail, the header fields at the very top of
	// the dialog (rendered long before Last message in detailBody) are no
	// longer on screen.
	if strings.Contains(bottom, "Agent:") {
		t.Fatalf("the Agent field is still visible at the bottom of the scroll -- detail is not scrolling as one dialog:\n%s", bottom)
	}
	// No per-field truncation indicator (task 078's explicit prohibition):
	// neither page ever states a line count for the Last message field.
	for _, forbidden := range []string{"of 27 lines", "lines of", "6 of 27"} {
		if strings.Contains(first, forbidden) || strings.Contains(bottom, forbidden) {
			t.Fatalf("detail view carries a per-field line-count indicator %q, which task 078 forbids:\n%s\n%s", forbidden, first, bottom)
		}
	}
}
