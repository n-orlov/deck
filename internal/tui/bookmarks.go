package tui

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
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
		m.bookmarkNote = "Bookmark: press a digit 0-9 to set the slot"
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
	if !ok {
		m.bookmarkNote = fmt.Sprintf("Bookmark %d: select a session first", slot)
		return
	}
	if m.store == nil {
		m.bookmarkNote = "Bookmarks are unavailable"
		return
	}
	if err := m.store.AssignBookmark(context.Background(), slot, session.ID); err != nil {
		m.bookmarkNote = fmt.Sprintf("Bookmark %d: %v", slot, err)
		return
	}
	m.bookmarkNote = fmt.Sprintf("Bookmark %d set to %s", slot, session.Name)
}

// jumpToBookmark moves the selection (never attaches) to the session held in
// slot. An empty slot is a no-op with a one-line status message.
func (m *Model) jumpToBookmark(slot int) {
	if m.store == nil {
		m.bookmarkNote = "Bookmarks are unavailable"
		return
	}
	slots, err := m.store.Bookmarks(context.Background())
	if err != nil {
		m.bookmarkNote = fmt.Sprintf("Bookmark %d: %v", slot, err)
		return
	}
	id, ok := slots[slot]
	if !ok {
		m.bookmarkNote = fmt.Sprintf("Bookmark %d is empty", slot)
		return
	}
	idx := indexOfSessionID(m.sessions, id)
	if idx < 0 {
		m.bookmarkNote = fmt.Sprintf("Bookmark %d: the session is not in the list", slot)
		return
	}
	m.setSelection(rowCursor(idx))
	m.bookmarkNote = fmt.Sprintf("Bookmark %d: %s", slot, m.sessions[idx].Name)
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
