package tui

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/store"
)

// R286/R287: Alt+M then Delete clears the chord session's bookmark slots.

func deleteKey() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyDelete} }

const launchChordPrompt = "Bookmark: press a digit 0-9 to set the slot"

func assign(t *testing.T, db *store.Store, slot int, id string) {
	t.Helper()
	if err := db.AssignBookmark(context.Background(), slot, id); err != nil {
		t.Fatalf("AssignBookmark(%d, %s): %v", slot, id, err)
	}
}

// TestBookmarkClearChord: the five finite cases of R286.
func TestBookmarkClearChord(t *testing.T) {
	type row struct {
		name        string
		interactive bool
		on          bool
		slots       map[int]string // slot -> session ordinal key: "cur" or "other"
		wantLeft    map[int]string // slots expected afterwards
		wantNote    string         // substring of the message; "" = no message at all
	}
	rows := []row{
		{"list mode, one slot", false, true, map[int]string{3: "cur"}, map[int]string{}, "3"},
		{"list mode, two slots", false, true, map[int]string{1: "cur", 5: "cur"}, map[int]string{}, "1,5"},
		{"interactive mode, one slot of the attached session", true, true, map[int]string{2: "cur"}, map[int]string{}, "2"},
		{"another session's slot is left alone", false, true, map[int]string{3: "cur", 4: "other"}, map[int]string{4: "other"}, "3"},
		{"bookmarks off binds nothing", false, false, map[int]string{3: "cur"}, map[int]string{3: "cur"}, ""},
	}
	if len(rows) != 5 {
		t.Fatalf("table has %d rows, want 5", len(rows))
	}
	for _, c := range rows {
		t.Run(c.name, func(t *testing.T) {
			var (
				m        Model
				db       *store.Store
				cur, oth string
				press    func(keys ...tea.KeyMsg)
			)
			if c.interactive {
				r := newBookmarkRig(t, c.on)
				db, cur, oth = r.db, r.a.ID, r.b.ID
				press = func(keys ...tea.KeyMsg) { r.press(t, keys...) }
				for slot, who := range c.slots {
					id := cur
					if who == "other" {
						id = oth
					}
					assign(t, db, slot, id)
				}
				press(altRune('m'), deleteKey())
				m = r.m
				if !m.interactive {
					t.Fatal("left interactive mode")
				}
				if got := stripANSI(r.m.footerLine()); !strings.Contains(got, c.wantNote) {
					t.Fatalf("footer = %q, want it to name %q", got, c.wantNote)
				}
				// the Delete must not have reached the pane
				press(key("Z"))
				r.waitReceived(t, cur, "Z")
			} else {
				m, db = bookmarkKeyModel(t, c.on)
				cur, oth = "s0", "s1"
				for slot, who := range c.slots {
					id := cur
					if who == "other" {
						id = oth
					}
					assign(t, db, slot, id)
				}
				m, _ = sendKey(t, m, altRune('m'))
				m, _ = sendKey(t, m, deleteKey())
			}
			want := map[int]string{}
			for slot, who := range c.wantLeft {
				if who == "other" {
					want[slot] = oth
				} else {
					want[slot] = cur
				}
			}
			if got := mustBookmarks(t, db); !reflect.DeepEqual(got, want) {
				t.Fatalf("bookmarks = %v, want %v", got, want)
			}
			if c.wantNote == "" {
				if m.bookmarkNote != "" || m.bookmarkNoteLine() != "" {
					t.Fatalf("message = %q, want none", m.bookmarkNote)
				}
				return
			}
			if !c.interactive && !strings.Contains(m.bookmarkNote, c.wantNote) {
				t.Fatalf("message = %q, want it to name %q", m.bookmarkNote, c.wantNote)
			}
		})
	}
}

// TestBookmarkClearChordMessageNamesSlots reads the message right after the
// Delete, in the interactive case too (the table above presses a key after).
func TestBookmarkClearChordMessageNamesSlots(t *testing.T) {
	r := newBookmarkRig(t, true)
	assign(t, r.db, 2, r.a.ID)
	assign(t, r.db, 7, r.a.ID)
	r.press(t, altRune('m'), deleteKey())
	if got := stripANSI(r.m.footerLine()); !strings.Contains(got, "2,7") {
		t.Fatalf("footer = %q, want it to name slots 2,7", got)
	}
}

// TestBookmarkChordPromptConditional: Delete is offered only on a bookmarked
// session; the unbookmarked prompt is byte-identical to the launch prompt.
func TestBookmarkChordPromptConditional(t *testing.T) {
	for _, c := range []struct {
		name  string
		slots []int
		want  string // "" = the launch prompt, exactly
	}{
		{"unbookmarked", nil, ""},
		{"one slot", []int{4}, "Press Delete to remove bookmark 4"},
		{"two slots", []int{1, 8}, "Press Delete to remove bookmark 1,8"},
	} {
		t.Run("list "+c.name, func(t *testing.T) {
			m, db := bookmarkKeyModel(t, true)
			for _, s := range c.slots {
				assign(t, db, s, "s0")
			}
			assign(t, db, 9, "s1") // another session's slot never shows
			m, _ = sendKey(t, m, altRune('m'))
			checkPrompt(t, m.bookmarkNote, c.want)
		})
		t.Run("interactive "+c.name, func(t *testing.T) {
			r := newBookmarkRig(t, true)
			for _, s := range c.slots {
				assign(t, r.db, s, r.a.ID)
			}
			assign(t, r.db, 9, r.b.ID)
			r.press(t, altRune('m'))
			checkPrompt(t, r.m.bookmarkNote, c.want)
			if c.want != "" && !strings.Contains(stripANSI(r.m.footerLine()), c.want) {
				t.Fatalf("footer = %q, want it to show %q", stripANSI(r.m.footerLine()), c.want)
			}
		})
	}
}

func checkPrompt(t *testing.T, got, want string) {
	t.Helper()
	if want == "" {
		if got != launchChordPrompt {
			t.Fatalf("prompt = %q, want the launch prompt %q", got, launchChordPrompt)
		}
		return
	}
	if !strings.Contains(got, want) || strings.Contains(got, want+",") {
		t.Fatalf("prompt = %q, want it to contain %q", got, want)
	}
}

// TestBookmarkChordDeleteOnUnbookmarkedFallsThrough: the chord is cancelled
// and Delete is handled exactly as a Delete with no chord before it.
func TestBookmarkChordDeleteOnUnbookmarkedFallsThrough(t *testing.T) {
	m, db := bookmarkKeyModel(t, true)
	assign(t, db, 4, "s1") // another session's slot must survive
	chorded, _ := sendKey(t, m, altRune('m'))
	chorded, _ = sendKey(t, chorded, deleteKey())
	plain, _ := sendKey(t, m, deleteKey())
	if chorded.bookmarkChord {
		t.Fatal("the chord is still armed")
	}
	if chorded.bookmarkNote != "" {
		t.Fatalf("message = %q, want none", chorded.bookmarkNote)
	}
	if chorded.selected != plain.selected || chorded.pendingDelete != plain.pendingDelete ||
		chorded.help != plain.help || chorded.filtering != plain.filtering {
		t.Fatal("Delete after Alt+M was not handled like a plain Delete")
	}
	if got := mustBookmarks(t, db); !reflect.DeepEqual(got, map[int]string{4: "s1"}) {
		t.Fatalf("bookmarks = %v, want only slot 4 -> s1", got)
	}
	// the chord is over: a digit now sets nothing
	chorded, _ = sendKey(t, chorded, key("5"))
	if got := mustBookmarks(t, db); len(got) != 1 {
		t.Fatalf("bookmarks = %v after a later digit, want unchanged", got)
	}
}

// TestBookmarkChordOtherKeyCancels: every non-digit, non-Delete key still
// cancels the chord and leaves the slots alone.
func TestBookmarkChordOtherKeyCancels(t *testing.T) {
	for _, k := range []string{"esc", "x", "j", "tab"} {
		t.Run(k, func(t *testing.T) {
			m, db := bookmarkKeyModel(t, true)
			assign(t, db, 4, "s0")
			m, _ = sendKey(t, m, altRune('m'))
			m, _ = sendKey(t, m, key(k))
			if m.bookmarkChord {
				t.Fatal("the chord is still armed")
			}
			m, _ = sendKey(t, m, deleteKey())
			m, _ = sendKey(t, m, key("5"))
			if got := mustBookmarks(t, db); !reflect.DeepEqual(got, map[int]string{4: "s0"}) {
				t.Fatalf("bookmarks = %v, want only slot 4 -> s0", got)
			}
		})
	}
}

// TestStrayDeleteOutsideChordUnchanged: a Delete with no pending chord clears
// nothing, in the list and in interactive mode (where it reaches the pane).
func TestStrayDeleteOutsideChordUnchanged(t *testing.T) {
	t.Run("list", func(t *testing.T) {
		m, db := bookmarkKeyModel(t, true)
		assign(t, db, 4, "s0")
		m, _ = sendKey(t, m, deleteKey())
		if m.bookmarkNote != "" {
			t.Fatalf("message = %q, want none", m.bookmarkNote)
		}
		if got := mustBookmarks(t, db); !reflect.DeepEqual(got, map[int]string{4: "s0"}) {
			t.Fatalf("bookmarks = %v, want slot 4 kept", got)
		}
	})
	t.Run("interactive", func(t *testing.T) {
		r := newBookmarkRig(t, true)
		assign(t, r.db, 4, r.a.ID)
		r.press(t, deleteKey())
		waitNonEmpty(t, r, r.a.ID)
		if got := mustBookmarks(t, r.db); !reflect.DeepEqual(got, map[int]string{4: r.a.ID}) {
			t.Fatalf("bookmarks = %v, want slot 4 kept", got)
		}
	})
}

func waitNonEmpty(t *testing.T, r *bookmarkRig, id string) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for r.received(id) == "" {
		if time.Now().After(deadline) {
			t.Fatalf("pane %s received nothing", id)
		}
		time.Sleep(20 * time.Millisecond)
	}
	return r.received(id)
}

// TestInteractiveChordDeleteNotForwarded: the Delete that completes the chord
// sends zero bytes to the pane.
func TestInteractiveChordDeleteNotForwarded(t *testing.T) {
	r := newBookmarkRig(t, true)
	assign(t, r.db, 6, r.a.ID)
	r.press(t, altRune('m'), deleteKey(), key("Z"))
	// Anything the Delete leaked would precede the Z, in order.
	r.waitReceived(t, r.a.ID, "Z")
	if got := r.received(r.b.ID); got != "" {
		t.Fatalf("the other pane received %q", got)
	}
	if got := mustBookmarks(t, r.db); len(got) != 0 {
		t.Fatalf("bookmarks = %v, want none", got)
	}
}

// TestInteractiveChordDeleteUnbookmarkedForwarded: a Delete that does not
// complete the chord reaches the pane with the bytes a stray Delete sent at
// the launch sha.
func TestInteractiveChordDeleteUnbookmarkedForwarded(t *testing.T) {
	stray := newBookmarkRig(t, true)
	stray.press(t, deleteKey())
	want := waitNonEmpty(t, stray, stray.a.ID)

	r := newBookmarkRig(t, true)
	assign(t, r.db, 6, r.b.ID) // the other session's slot is not this chord's
	r.press(t, altRune('m'), deleteKey())
	r.waitReceived(t, r.a.ID, want)
	if got := mustBookmarks(t, r.db); !reflect.DeepEqual(got, map[int]string{6: r.b.ID}) {
		t.Fatalf("bookmarks = %v, want slot 6 kept", got)
	}
}

// R286: the clear chord belongs to the session Alt+M started on. A background
// reload between the two keys (sessionsLoaded, as the poller delivers it) may
// move the selection or drop the session; Delete never clears another
// session's slots.

// reloadWith delivers a background session reload that holds only keep, in order.
func reloadWith(t *testing.T, m Model, keep ...store.Session) Model {
	t.Helper()
	next, _ := m.Update(sessionsLoaded{sessions: keep})
	return next.(Model)
}

func TestBookmarkClearChordKeepsItsSessionAcrossReload(t *testing.T) {
	cases := []struct {
		name string
		// slots held when the chord starts (s0 is selected); after is what
		// the store holds when Delete arrives.
		before, after map[int]string
		keep          []int // indexes of the base sessions the reload keeps
		wantSlots     map[int]string
		wantNote      string
	}{
		{ // the finding: s0 vanishes, s1 becomes the selection
			name:      "session dropped, slots already cleared, neighbour keeps its slot",
			before:    map[int]string{3: "s0", 9: "s1"},
			after:     map[int]string{9: "s1"},
			keep:      []int{1, 2, 3},
			wantSlots: map[int]string{9: "s1"},
			wantNote:  "no longer holds",
		},
		{ // availability changed but the session still owns its slot: that slot is the chord's
			name:      "session dropped from the list, its slots still cleared, neighbour kept",
			before:    map[int]string{3: "s0", 9: "s1"},
			after:     map[int]string{3: "s0", 9: "s1"},
			keep:      []int{1, 2, 3},
			wantSlots: map[int]string{9: "s1"},
			wantNote:  "Removed bookmark 3",
		},
		{ // slots added to the old session meanwhile are also its own
			name:      "slot added to the chord session before Delete",
			before:    map[int]string{2: "s0", 5: "s1"},
			after:     map[int]string{2: "s0", 4: "s0", 5: "s1"},
			keep:      []int{1, 2, 3},
			wantSlots: map[int]string{5: "s1"},
			wantNote:  "Removed bookmark 2,4",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m, db := bookmarkKeyModel(t, true)
			base := append([]store.Session(nil), m.baseSessions...)
			for slot, id := range c.before {
				assign(t, db, slot, id)
			}
			m, _ = sendKey(t, m, altRune('m'))
			for slot := 0; slot <= 9; slot++ {
				if err := db.ClearBookmark(context.Background(), slot); err != nil {
					t.Fatal(err)
				}
			}
			for slot, id := range c.after {
				assign(t, db, slot, id)
			}
			var keep []store.Session
			for _, i := range c.keep {
				keep = append(keep, base[i])
			}
			m = reloadWith(t, m, keep...)
			if got := selectedID(t, m); got == "s0" {
				t.Fatalf("test setup: selection stayed on s0 after the reload")
			}
			m, _ = sendKey(t, m, deleteKey())
			want := map[int]string{}
			for slot, id := range c.wantSlots {
				want[slot] = id
			}
			if got := mustBookmarks(t, db); !reflect.DeepEqual(got, want) {
				t.Fatalf("bookmarks = %v, want %v", got, want)
			}
			if !strings.Contains(m.bookmarkNote, c.wantNote) {
				t.Fatalf("message = %q, want it to contain %q", m.bookmarkNote, c.wantNote)
			}
		})
	}
}

// A chord that began on an unbookmarked session keeps its fall-through even
// when a reload selects a bookmarked one: the Delete clears nothing.
func TestBookmarkClearChordUnbookmarkedStartIgnoresReloadedSelection(t *testing.T) {
	m, db := bookmarkKeyModel(t, true)
	assign(t, db, 7, "s1")
	base := append([]store.Session(nil), m.baseSessions...)
	m, _ = sendKey(t, m, altRune('m'))
	m = reloadWith(t, m, base[1], base[2], base[3])
	m, _ = sendKey(t, m, deleteKey())
	if got := mustBookmarks(t, db); !reflect.DeepEqual(got, map[int]string{7: "s1"}) {
		t.Fatalf("bookmarks = %v, want slot 7 kept", got)
	}
}

// Interactive mode: the attached session is the chord's session; a reload
// that drops it between Alt+M and Delete never clears the other session.
func TestInteractiveClearChordKeepsItsSessionAcrossReload(t *testing.T) {
	t.Run("attached session dropped, slots already cleared", func(t *testing.T) {
		r := newBookmarkRig(t, true)
		assign(t, r.db, 3, r.a.ID)
		assign(t, r.db, 9, r.b.ID)
		r.press(t, altRune('m'))
		if err := r.db.ClearBookmark(context.Background(), 3); err != nil {
			t.Fatal(err)
		}
		next, _ := r.m.Update(sessionsLoaded{sessions: []store.Session{r.b}})
		r.m = next.(Model)
		r.press(t, deleteKey())
		if got := mustBookmarks(t, r.db); !reflect.DeepEqual(got, map[int]string{9: r.b.ID}) {
			t.Fatalf("bookmarks = %v, want only slot 9 of the other session", got)
		}
	})
	t.Run("attached session dropped from the list, slots still owned", func(t *testing.T) {
		r := newBookmarkRig(t, true)
		assign(t, r.db, 3, r.a.ID)
		assign(t, r.db, 9, r.b.ID)
		r.press(t, altRune('m'))
		next, _ := r.m.Update(sessionsLoaded{sessions: []store.Session{r.b}})
		r.m = next.(Model)
		r.press(t, deleteKey())
		if got := mustBookmarks(t, r.db); !reflect.DeepEqual(got, map[int]string{9: r.b.ID}) {
			t.Fatalf("bookmarks = %v, want the chord session's slot 3 cleared and slot 9 kept", got)
		}
	})
	t.Run("attached session still owns its slots", func(t *testing.T) {
		r := newBookmarkRig(t, true)
		assign(t, r.db, 3, r.a.ID)
		assign(t, r.db, 9, r.b.ID)
		r.press(t, altRune('m'))
		next, _ := r.m.Update(sessionsLoaded{sessions: []store.Session{r.b, r.a}})
		r.m = next.(Model)
		r.press(t, deleteKey())
		if got := mustBookmarks(t, r.db); !reflect.DeepEqual(got, map[int]string{9: r.b.ID}) {
			t.Fatalf("bookmarks = %v, want only slot 9 kept", got)
		}
	})
}
