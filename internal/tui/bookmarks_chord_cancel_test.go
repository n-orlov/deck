package tui

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// R275: the key after Alt+M cancels the chord whichever layer consumes it, so
// a later bare digit never writes a bookmark. These drive Model.Update.

func requireNoBookmarks(t *testing.T, m Model, why string) {
	t.Helper()
	if m.bookmarkChord {
		t.Fatalf("%s: the chord is still armed", why)
	}
	if got, err := m.store.Bookmarks(context.Background()); err != nil || len(got) != 0 {
		t.Fatalf("%s: bookmarks = %v (err %v), want none", why, got, err)
	}
}

// TestBookmarkChordIsCancelledByTheRefusalBannerEsc: the banner-dismissing Esc
// is consumed ahead of the bookmark layer; it still cancels the chord.
func TestBookmarkChordIsCancelledByTheRefusalBannerEsc(t *testing.T) {
	m, _ := bookmarkKeyModel(t, true)
	m.setEntryRefusal("s0", entryRefusalStopped, "stopped")
	m, _ = sendKey(t, m, altRune('m'))
	if !m.bookmarkChord {
		t.Fatal("alt+m did not arm the chord")
	}
	m, _ = sendKey(t, m, key("esc"))
	if m.entryRefusal.active {
		t.Fatal("esc did not dismiss the refusal banner")
	}
	requireNoBookmarks(t, m, "after the banner esc")
	m, _ = sendKey(t, m, key("5"))
	requireNoBookmarks(t, m, "bare digit after the banner esc")
}

// TestBookmarkChordIsCancelledByKeysOfHigherLayers: keys taken by the pending
// delete intercept, the help view and the filter prompt cancel the chord too.
func TestBookmarkChordIsCancelledByKeysOfHigherLayers(t *testing.T) {
	cases := []struct {
		name string
		key  string
		open func(m Model) bool
	}{
		{"help", "?", func(m Model) bool { return m.help }},
		{"filter", "/", func(m Model) bool { return m.filtering }},
		{"pending delete", "d", func(m Model) bool { return m.pendingDelete }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m, _ := bookmarkKeyModel(t, true)
			m, _ = sendKey(t, m, altRune('m'))
			m, _ = sendKey(t, m, key(c.key))
			if !c.open(m) {
				t.Fatalf("%s did not take its normal effect", c.key)
			}
			requireNoBookmarks(t, m, "after "+c.key)
			if c.name == "pending delete" {
				// the digit is the pendingDelete layer's own key
				m, _ = sendKey(t, m, key("5"))
				requireNoBookmarks(t, m, "digit after the cancelled chord")
			}
		})
	}
}

// TestBookmarkChordAltMThenAltMStaysOneChord: a second Alt+M re-arms rather
// than stacking; one digit sets exactly one slot and the next digit none.
func TestBookmarkChordAltMThenAltMStaysOneChord(t *testing.T) {
	m, db := bookmarkKeyModel(t, true)
	m, _ = sendKey(t, m, altRune('m'))
	m, _ = sendKey(t, m, altRune('m'))
	m, _ = sendKey(t, m, key("2"))
	sendKey(t, m, key("3"))
	if got := mustBookmarks(t, db); len(got) != 1 || got[2] != "s0" {
		t.Fatalf("bookmarks = %v, want only slot 2 -> s0", got)
	}
}

// TestInteractiveBookmarkChordIsCancelledByCtrlQ: attached Ctrl+Q leaves
// interactive mode as usual and leaves no chord for a later digit.
func TestInteractiveBookmarkChordIsCancelledByCtrlQ(t *testing.T) {
	r := newBookmarkRig(t, true)
	r.press(t, altRune('m'))
	if !r.m.bookmarkChord {
		t.Fatal("alt+m did not arm the chord")
	}
	r.press(t, tea.KeyMsg{Type: tea.KeyCtrlQ})
	if r.m.interactive {
		t.Fatal("ctrl+q did not leave interactive mode")
	}
	requireNoBookmarks(t, r.m, "after ctrl+q")
	m, _ := sendKey(t, r.m, key("5"))
	requireNoBookmarks(t, m, "bare digit after ctrl+q")
}

// TestInteractiveBookmarkChordIsCancelledByNonBookmarkKeys: Ctrl+C is
// forwarded to the pane and a jump key is consumed by the bookmark layer;
// both cancel the chord, so the next bare digit is plain forwarded input.
func TestInteractiveBookmarkChordIsCancelledByNonBookmarkKeys(t *testing.T) {
	for name, k := range map[string]tea.KeyMsg{"ctrl+c": tea.KeyMsg{Type: tea.KeyCtrlC}, "alt+1 jump": altRune('1')} {
		t.Run(name, func(t *testing.T) {
			r := newBookmarkRig(t, true)
			r.press(t, altRune('m'), k, key("5"))
			requireNoBookmarks(t, r.m, name)
		})
	}
}

// TestInteractiveBookmarkChordIsDroppedByAKeylessExit: leaving interactive
// mode some other way (the pane going away) also drops the chord.
func TestInteractiveBookmarkChordIsDroppedByAKeylessExit(t *testing.T) {
	r := newBookmarkRig(t, true)
	r.press(t, altRune('m'))
	left, _ := r.m.exitInteractive()
	m, _ := sendKey(t, left.(Model), key("5"))
	requireNoBookmarks(t, m, "digit after a keyless exit")
}
