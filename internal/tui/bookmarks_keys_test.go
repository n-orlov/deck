package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// altRune is the key Bubble Tea v1 delivers for Alt held with one rune.
func altRune(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}, Alt: true}
}

// bookmarkKeyModel is a list model over a real store holding four sessions
// (b0..b3), the cursor on b0, with the bookmarks setting as given.
func bookmarkKeyModel(t *testing.T, on bool) (Model, *store.Store) {
	t.Helper()
	home := t.TempDir()
	db, err := store.OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatalf("OpenPath: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	var sessions []store.Session
	for i := 0; i < 4; i++ {
		s, err := db.CreateSession(context.Background(), store.CreateSessionInput{
			ID: fmt.Sprintf("s%d", i), Name: fmt.Sprintf("b%d", i), CWD: t.TempDir(), Agent: "claude",
			CapturedPath: "/bin/true", Status: "running", StatusAt: 1, CreatedAt: int64(i + 1),
		})
		if err != nil {
			t.Fatalf("CreateSession: %v", err)
		}
		sessions = append(sessions, s)
	}
	m := New(db, config.Settings{Bookmarks: on}, "")
	m.width, m.height = 120, 30
	m.baseSessions = sessions
	m.sessions = m.filteredSessions()
	m.selected = rowCursor(0)
	return m, db
}

func sendKey(t *testing.T, m Model, msg tea.KeyMsg) (Model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(msg)
	return next.(Model), cmd
}

func selectedID(t *testing.T, m Model) string {
	t.Helper()
	s, ok := m.selectedSession()
	if !ok {
		t.Fatal("the cursor names no session")
	}
	return s.ID
}

func mustBookmarks(t *testing.T, db *store.Store) map[int]string {
	t.Helper()
	got, err := db.Bookmarks(context.Background())
	if err != nil {
		t.Fatalf("Bookmarks: %v", err)
	}
	return got
}

// TestBookmarkAltDigitJumpsSelectOnly drives alt+0..alt+9 over a filled slot
// table: the cursor lands on the bookmarked session, nothing attaches.
func TestBookmarkAltDigitJumpsSelectOnly(t *testing.T) {
	for slot := 0; slot <= 9; slot++ {
		t.Run(fmt.Sprintf("alt+%d", slot), func(t *testing.T) {
			m, db := bookmarkKeyModel(t, true)
			target := fmt.Sprintf("s%d", 1+slot%3)
			if err := db.AssignBookmark(context.Background(), slot, target); err != nil {
				t.Fatalf("AssignBookmark: %v", err)
			}
			got, cmd := sendKey(t, m, altRune([]rune(fmt.Sprint(slot))[0]))
			if id := selectedID(t, got); id != target {
				t.Fatalf("alt+%d selected %s, want %s", slot, id, target)
			}
			if cmd != nil || got.interactive || got.lostAttach {
				t.Fatalf("alt+%d is selection only; got cmd=%v interactive=%v", slot, cmd != nil, got.interactive)
			}
		})
	}
}

// TestBookmarkSetChordAndShiftedKeys covers every key shape that sets a slot:
// alt+m then a bare digit, and the ten Alt+Shift+digit keys of a US layout.
func TestBookmarkSetChordAndShiftedKeys(t *testing.T) {
	for slot := 0; slot <= 9; slot++ {
		t.Run(fmt.Sprintf("alt+m %d", slot), func(t *testing.T) {
			m, db := bookmarkKeyModel(t, true)
			m, _ = sendKey(t, m, altRune('m'))
			if !m.bookmarkChord {
				t.Fatal("alt+m did not arm the chord")
			}
			m, _ = sendKey(t, m, key(fmt.Sprint(slot)))
			if m.bookmarkChord {
				t.Fatal("the digit did not consume the chord")
			}
			if got := mustBookmarks(t, db); len(got) != 1 || got[slot] != "s0" {
				t.Fatalf("bookmarks = %v, want slot %d -> s0", got, slot)
			}
		})
	}
	shifted := map[string]int{"!": 1, "@": 2, "#": 3, "$": 4, "%": 5, "^": 6, "&": 7, "*": 8, "(": 9, ")": 0}
	for sym, slot := range shifted {
		t.Run("alt+"+sym, func(t *testing.T) {
			m, db := bookmarkKeyModel(t, true)
			m, _ = sendKey(t, m, key("j"))
			sendKey(t, m, altRune([]rune(sym)[0]))
			if got := mustBookmarks(t, db); len(got) != 1 || got[slot] != "s1" {
				t.Fatalf("alt+%s: bookmarks = %v, want slot %d -> s1", sym, got, slot)
			}
		})
	}
}

// TestBookmarkChordIsCancelledByEscAndByAnyOtherKey: esc and an unrelated key
// after alt+m cancel the chord and are then handled normally, so the digit
// that follows sets nothing.
func TestBookmarkChordIsCancelledByEscAndByAnyOtherKey(t *testing.T) {
	m, db := bookmarkKeyModel(t, true)
	m, _ = sendKey(t, m, key("m"))
	if len(m.marked) != 1 {
		t.Fatalf("m did not mark the row: %v", m.marked)
	}
	m, _ = sendKey(t, m, altRune('m'))
	m, _ = sendKey(t, m, key("esc"))
	if m.bookmarkChord {
		t.Fatal("esc left the chord armed")
	}
	if len(m.marked) != 0 {
		t.Fatal("esc after alt+m was swallowed: the marks were not cleared as esc normally does")
	}
	m, _ = sendKey(t, m, key("5"))

	m, _ = sendKey(t, m, altRune('m'))
	m, _ = sendKey(t, m, key("j"))
	if m.bookmarkChord {
		t.Fatal("j left the chord armed")
	}
	if id := selectedID(t, m); id != "s1" {
		t.Fatalf("j after alt+m selected %s, want s1 (handled normally)", id)
	}
	m, _ = sendKey(t, m, key("7"))
	if got := mustBookmarks(t, db); len(got) != 0 {
		t.Fatalf("a cancelled chord set slots: %v", got)
	}
}

// TestBookmarkJumpToEmptySlotIsANoOpWithAStatusLine pins the empty-slot rule.
func TestBookmarkJumpToEmptySlotIsANoOpWithAStatusLine(t *testing.T) {
	m, db := bookmarkKeyModel(t, true)
	m, _ = sendKey(t, m, key("j"))
	before := selectedID(t, m)
	got, cmd := sendKey(t, m, altRune('4'))
	if cmd != nil {
		t.Fatal("a jump to an empty slot returned a command")
	}
	if id := selectedID(t, got); id != before {
		t.Fatalf("the selection moved from %s to %s", before, id)
	}
	if len(mustBookmarks(t, db)) != 0 {
		t.Fatal("a jump wrote bookmarks")
	}
	line := got.footerLineContent()
	if !strings.Contains(line, "Bookmark 4 is empty") || strings.Contains(line, "\n") {
		t.Fatalf("footer = %q, want a one-line empty-slot message", line)
	}
	if after, _ := sendKey(t, got, key("j")); after.bookmarkNote != "" {
		t.Fatal("the status message outlived the next key")
	}
}

// TestBookmarkKeysBindNothingWhileTheSettingIsOff: with bookmarks off every
// key of the on-state tests leaves the model, the store and the screen as
// they were. Binding the keys regardless of the setting turns this red.
func TestBookmarkKeysBindNothingWhileTheSettingIsOff(t *testing.T) {
	var keys []tea.KeyMsg
	for r := '0'; r <= '9'; r++ {
		keys = append(keys, altRune(r))
	}
	for _, r := range "!@#$%^&*()" {
		keys = append(keys, altRune(r))
	}
	keys = append(keys, altRune('m'))
	for _, k := range keys {
		t.Run(k.String(), func(t *testing.T) {
			m, db := bookmarkKeyModel(t, false)
			if err := db.AssignBookmark(context.Background(), 3, "s2"); err != nil {
				t.Fatal(err)
			}
			m, _ = sendKey(t, m, key("j"))
			wantView, wantSel := m.View(), m.selected
			got, cmd := sendKey(t, m, k)
			if cmd != nil || got.selected != wantSel || got.bookmarkChord || got.bookmarkNote != "" {
				t.Fatalf("%s changed the model: cmd=%v selected=%v chord=%v note=%q", k, cmd != nil, got.selected, got.bookmarkChord, got.bookmarkNote)
			}
			if got.View() != wantView {
				t.Fatalf("%s changed the screen", k)
			}
			// A digit after alt+m must not set a slot either.
			got, _ = sendKey(t, got, key("5"))
			if s := mustBookmarks(t, db); len(s) != 1 || s[3] != "s2" {
				t.Fatalf("%s wrote bookmarks: %v", k, s)
			}
		})
	}
}
