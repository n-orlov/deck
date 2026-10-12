package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/theme"
	"github.com/n-orlov/deck/internal/tmux"
	"github.com/n-orlov/deck/internal/tui/lineedit"
)

// bookmarkSetChord is the key that arms the slot-setting chord: Alt+M, then a
// digit 0..9 (layout-independent, R275).
const bookmarkSetChord = "alt+m"

// bookmarkShiftedSlots maps the Alt+Shift+digit convenience keys of a US
// layout to the slot each one sets. The keys arrive as `alt+<symbol>` because
// Bubble Tea v1 reports the shifted rune, not the digit.
var bookmarkShiftedSlots = map[string]int{
	"alt+!": 1, "alt+@": 2, "alt+#": 3, "alt+$": 4, "alt+%": 5,
	"alt+^": 6, "alt+&": 7, "alt+*": 8, "alt+(": 9, "alt+)": 0,
}

// bookmarkJumpSlot reports the slot an `alt+<digit>` key names.
func bookmarkJumpSlot(key string) (int, bool) {
	if len(key) == 5 && key[:4] == "alt+" && key[4] >= '0' && key[4] <= '9' {
		return int(key[4] - '0'), true
	}
	return 0, false
}

// bookmarkChordSlot reports the slot a bare digit completing a pending
// Alt+M chord names.
func bookmarkChordSlot(key string) (int, bool) {
	if len(key) == 1 && key[0] >= '0' && key[0] <= '9' {
		return int(key[0] - '0'), true
	}
	return 0, false
}

// handleBookmarkKey is the list-mode bookmark layer (R275). It binds nothing
// while the bookmarks setting is off, so every key reaches the rest of the
// list untouched. handled reports that the key was consumed here; a pending
// chord is always cancelled by the next key, and a key that does not complete
// it falls through (handled false) to be handled normally.
func (m Model) handleBookmarkKey(msg tea.KeyMsg) (next Model, handled bool) {
	if !m.settings.Bookmarks {
		m.bookmarkChord = false
		return m, false
	}
	key := msg.String()
	if m.bookmarkChord {
		m.bookmarkChord = false
		if slot, ok := bookmarkChordSlot(key); ok {
			m.setBookmark(slot)
			return m, true
		}
	}
	if key == bookmarkSetChord {
		m.bookmarkChord = true
		m.setBookmarkNote("Bookmark: press a digit 0-9 to set the slot")
		return m, true
	}
	if slot, ok := bookmarkShiftedSlots[key]; ok {
		m.setBookmark(slot)
		return m, true
	}
	if slot, ok := bookmarkJumpSlot(key); ok {
		m.jumpToBookmark(slot)
		return m, true
	}
	return m, false
}

// setBookmark points slot at the session under the cursor.
func (m *Model) setBookmark(slot int) {
	session, ok := m.selectedSession()
	m.setBookmarkOn(slot, session, ok)
}

// setBookmarkOn points slot at session; ok false means there is none.
func (m *Model) setBookmarkOn(slot int, session store.Session, ok bool) {
	if !ok {
		m.setBookmarkNote(fmt.Sprintf("Bookmark %d: select a session first", slot))
		return
	}
	if m.store == nil {
		m.setBookmarkNote("Bookmarks are unavailable")
		return
	}
	if err := m.store.AssignBookmark(context.Background(), slot, session.ID); err != nil {
		m.setBookmarkNote(fmt.Sprintf("Bookmark %d: %v", slot, err))
		return
	}
	m.setBookmarkNote(fmt.Sprintf("Bookmark %d set to %s", slot, session.Name))
}

// jumpToBookmark moves the selection (never attaches) to the session held in
// slot. An empty slot is a no-op with a one-line status message.
func (m *Model) jumpToBookmark(slot int) {
	idx, note := m.bookmarkSlotIndex(slot)
	if idx < 0 {
		m.setBookmarkNote(note)
		return
	}
	m.setSelection(rowCursor(idx))
	m.setBookmarkNote(fmt.Sprintf("Bookmark %d: %s", slot, m.sessions[idx].Name))
}

// bookmarkSlotIndex resolves slot to an index into m.sessions, first
// revealing a target the list hides (R278): an active filter that hides it is
// cleared and a collapsed group holding it is expanded, so the jump always
// lands on a visible row. A slot that names no session returns -1 and the
// one-line message to show.
func (m *Model) bookmarkSlotIndex(slot int) (int, string) {
	if m.store == nil {
		return -1, "Bookmarks are unavailable"
	}
	slots, err := m.store.Bookmarks(context.Background())
	if err != nil {
		return -1, fmt.Sprintf("Bookmark %d: %v", slot, err)
	}
	id, ok := slots[slot]
	if !ok {
		return -1, fmt.Sprintf("Bookmark %d is empty", slot)
	}
	idx := indexOfSessionID(m.sessions, id)
	if idx < 0 && m.filterQuery != "" && indexOfSessionID(m.baseSessions, id) >= 0 {
		m.filtering = false
		m.filterQuery = ""
		m.filterEdit = lineedit.Editor{}
		m.sessions = m.filteredSessions()
		idx = indexOfSessionID(m.sessions, id)
	}
	if idx < 0 {
		return -1, fmt.Sprintf("Bookmark %d: the session is not in the list", slot)
	}
	if gid := sessionGroupID(m.sessions[idx]); m.isGroupCollapsed(gid) {
		m.setGroupCollapsed(gid, false)
	}
	return idx, ""
}

// loadBookmarkSlots reads the slot table once and returns, per session id,
// the slots it holds ascending. It is nil while the setting is off or the
// store is unavailable, so nothing downstream draws or lists a slot.
func (m Model) loadBookmarkSlots() map[string][]int {
	if !m.settings.Bookmarks || m.store == nil {
		return nil
	}
	all, err := m.store.Bookmarks(context.Background())
	if err != nil || len(all) == 0 {
		return nil
	}
	out := map[string][]int{}
	for slot := 0; slot <= 9; slot++ {
		if id, ok := all[slot]; ok {
			out[id] = append(out[id], slot)
		}
	}
	return out
}

// bookmarkRowSegment is the slot marker a bookmarked row draws before its
// name, `[3]` or `[1,5]` for a session holding several slots; empty when the
// session holds none or the setting is off.
func (m Model) bookmarkRowSegment(sessionID string) (settingsRowSegment, bool) {
	slots := m.bookmarkSlots[sessionID]
	if len(slots) == 0 || !m.settings.Bookmarks {
		return settingsRowSegment{}, false
	}
	digits := make([]string, len(slots))
	for i, slot := range slots {
		digits[i] = fmt.Sprint(slot)
	}
	return settingsRowSegment{Text: "[" + strings.Join(digits, ",") + "] ", Tok: theme.Badge}, true
}

// bookmarkNoteLine is the footer's one-line bookmark status, empty when there
// is none.
func (m Model) bookmarkNoteLine() string {
	if m.bookmarkNote == "" || !m.settings.Bookmarks {
		return ""
	}
	width, _ := m.frameSize()
	return m.elideToWidth(m.bookmarkNote, width)
}

// handleInteractiveBookmarkKey is the attached-mode bookmark layer (R276).
// It intercepts the same keys the list binds, next to Ctrl+Q, so none of them
// reaches the pane, and binds nothing while the setting is off. A pending
// Alt+M chord is cancelled by the next key; only a digit completes it, and
// any other key (Esc included) is not handled here and is forwarded as usual.
// A slot is set on the session interactive mode is bound to, and a jump
// retargets interactive mode onto the slot's session without a list round trip.
// Its one-line message shows in the footer's note slot until the next key.
func (m Model) handleInteractiveBookmarkKey(msg tea.KeyMsg) (next Model, handled bool) {
	// The message a bookmark key left in the footer lasts until the next
	// key, as in the list.
	m.setBookmarkNote("")
	if !m.settings.Bookmarks {
		m.bookmarkChord = false
		return m, false
	}
	key := msg.String()
	if m.bookmarkChord {
		m.bookmarkChord = false
		if slot, ok := bookmarkChordSlot(key); ok {
			target, found := m.interactiveTargetSession()
			m.setBookmarkOn(slot, target, found)
			return m, true
		}
	}
	if key == bookmarkSetChord {
		m.bookmarkChord = true
		m.setBookmarkNote("Bookmark: press a digit 0-9 to set the slot")
		return m, true
	}
	if slot, ok := bookmarkShiftedSlots[key]; ok {
		target, found := m.interactiveTargetSession()
		m.setBookmarkOn(slot, target, found)
		return m, true
	}
	if slot, ok := bookmarkJumpSlot(key); ok {
		return m.jumpInteractive(slot), true
	}
	return m, false
}

// jumpInteractive switches the attached view to the session held in slot. An
// empty slot, a session missing from the list or one that cannot be attached
// leaves the current view as it is and shows a one-line message in the
// interactive footer's note slot (noteBookmarkAttached).
func (m Model) jumpInteractive(slot int) Model {
	idx, note := m.bookmarkSlotIndex(slot)
	if idx < 0 {
		m.setBookmarkNote(note)
		return m
	}
	target := m.sessions[idx]
	m.setBookmarkNote(fmt.Sprintf("Bookmark %d: %s", slot, target.Name))
	if !canReachPane(target) {
		m.setBookmarkNote(fmt.Sprintf("Bookmark %d: %s is stopped", slot, target.Name))
		return m
	}
	if name, err := tmux.SessionName(target.Slug); err == nil && name == m.interactiveWindowTarget {
		return m
	}
	left, _ := m.exitInteractive()
	next := left.(Model)
	next.setSelection(rowCursor(idx))
	entered, _ := next.enterInteractive()
	next = entered.(Model)
	next.setBookmarkNote(fmt.Sprintf("Bookmark %d: %s", slot, target.Name))
	return next
}

// bookmarkFooterHints are the footer legend's bookmark entries: none while the
// setting is off, so no footer text mentions bookmarks then. They trail the
// fixed set, so a narrow footer drops them before any other key.
func (m Model) bookmarkFooterHints() []footerKeyHint {
	if !m.settings.Bookmarks {
		return nil
	}
	return []footerKeyHint{
		{"alt+0-9/m", "alt+0-9/m", "bookmarks", nil},
	}
}

// bookmarkHelpAnchor is the helpText line the bookmark keys are listed before,
// and bookmarkHelpLines the lines themselves (R277).
const bookmarkHelpAnchor = "  , open/close settings"

const bookmarkHelpLines = `  alt+0 … alt+9 jump to the session bookmarked in that slot: the selection
    moves, nothing attaches; a hidden target is revealed (the filter clears,
    a folded group unfolds). alt+m then a digit 0-9 bookmarks the selected
    session in that slot (alt+Shift+digit does the same on a US layout);
    shown only while the bookmarks setting is on
`
