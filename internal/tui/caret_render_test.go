package tui

import (
	"context"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/store"
	"os"
	"strings"
	"testing"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/vt"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/theme"
)

// R181 (SPEC §11.11): only the focused field draws the reverse-video caret,
// there is no `_` stand-in, and an offered value is drawn as a selection. These
// tests read the REAL rendered frame (Model.View) per cell off a vt.Emulator,
// the grid a terminal would show, rather than grepping escape bytes.

// The helpers below are this file's own so that it stands alone: it compiles and
// runs against a tree that has none of the later tests' helpers.

// caretPress sends each key through Update, one keystroke at a time.
func caretPress(m Model, keys ...string) Model {
	for _, k := range keys {
		updated, _ := m.Update(key(k))
		m = updated.(Model)
	}
	return m
}

// caretType types text one rune at a time, as a terminal delivers it.
func caretType(m Model, text string) Model {
	for _, r := range text {
		m = caretPress(m, string(r))
	}
	return m
}

// caretRenameModel opens the rename dialog on a session named name, through
// the `r` key inside detail.
func caretRenameModel(t *testing.T, name string) Model {
	t.Helper()
	m := New(nil, config.Settings{}, "")
	m.width, m.height = 80, 24
	m.sessions = []store.Session{{ID: "s1", Name: name, Agent: "shell", Status: "running", Slug: "alpha"}}
	m.selected = rowCursor(0)
	m.detail = true
	m = caretPress(m, "r")
	if !m.renaming {
		t.Fatal("r did not open the rename dialog")
	}
	return m
}

// caretFilterModel opens the `/` filter.
func caretFilterModel() Model {
	m := newFilterTestModel(filterTestSessions())
	m.width, m.height = 80, 24
	return caretPress(m, "/")
}

// caretEnvModel opens the env editor on key K (value "before") with enter
// pressed on that row, so the value is being edited.
func caretEnvModel() Model {
	m := New(nil, config.Settings{}, "")
	m.sessions = []store.Session{{ID: "s1", Name: "sess", Agent: "claude", Env: map[string]string{"K": "before"}}}
	m.envEditing = true
	next, _ := m.updateEnvDialog(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	m.width, m.height = 100, 30
	return m
}

func caretEnvPress(m Model, keys ...string) Model {
	for _, k := range keys {
		next, _ := m.updateEnvDialog(key(k))
		m = next.(Model)
	}
	return m
}

// caretGroupRenameModel opens the settings Groups panel with one group named
// name and presses r on it.
func caretGroupRenameModel(t *testing.T, name string) Model {
	t.Helper()
	db := openStoreForLastCreateGroup(t)
	if _, err := db.CreateGroup(context.Background(), name); err != nil {
		t.Fatal(err)
	}
	m := settingsOpenOnGroupsFields(t, db)
	m.width, m.height = 100, 30
	return caretPress(m, "r")
}

// caretCreateModel opens the create modal with `n`, started in dir.
func caretCreateModel(t *testing.T, dir string) Model {
	t.Helper()
	t.Chdir(dir)
	m := New(nil, config.Settings{}, "")
	m.width, m.height = 120, 40
	m = caretPress(m, "n")
	if !m.creating {
		t.Fatal("n did not open the create modal")
	}
	return m
}

// reverseCells lists every (col,row) of the emulator whose cell is reverse video.
func reverseCells(term *vt.Emulator) [][2]int {
	var out [][2]int
	for y := 0; y < term.Height(); y++ {
		for x := 0; x < term.Width(); x++ {
			if c := term.CellAt(x, y); c != nil && c.Style.Attrs&uv.AttrReverse != 0 {
				out = append(out, [2]int{x, y})
			}
		}
	}
	return out
}

func frameOf(t *testing.T, m Model) *vt.Emulator {
	t.Helper()
	return renderSettingsToEmulator(t, m.View(), m.width, m.height)
}

// colourModes runs fn once under colour and once under NO_COLOR (Settings.Color
// false), the two states §11.11 requires the caret to survive.
func colourModes(t *testing.T, fn func(t *testing.T, color bool)) {
	t.Helper()
	for _, mode := range []struct {
		name  string
		color bool
	}{{"colour", true}, {"NO_COLOR", false}} {
		t.Run(mode.name, func(t *testing.T) { fn(t, mode.color) })
	}
}

// caretCase is one text field of one handler, with its caret in the middle of
// the text: prefix is the visible text on the field's row up to the caret
// (label included) and under is the character the caret sits on.
type caretCase struct {
	name   string
	build  func(t *testing.T, color bool) Model
	prefix string
	under  string
}

func caretCases() []caretCase {
	left := func(m Model, n int) Model {
		for i := 0; i < n; i++ {
			m = caretPress(m, "left")
		}
		return m
	}
	return []caretCase{
		{"rename", func(t *testing.T, color bool) Model {
			m := caretRenameModel(t, "alpha")
			m.settings.Color = color
			return left(m, 2)
		}, "New name:  alp", "h"},
		{"filter", func(t *testing.T, color bool) Model {
			m := caretFilterModel()
			m.settings.Color = color
			return left(caretType(m, "alpha"), 2)
		}, "Filter: alp", "h"},
		{"launch inputs", func(t *testing.T, color bool) Model {
			db, id := newLaunchInputsTestStore(t)
			m := launchInputsTestModel(t, db, id)
			m.settings.Color = color
			m.width, m.height = 100, 40
			return left(caretType(m, "abc"), 2)
		}, "Pre-launch command: a", "b"},
		{"env editor", func(t *testing.T, color bool) Model {
			m := caretEnvModel()
			m.settings.Color = color
			return caretEnvPress(m, "x", "y", "z", "left", "left")
		}, "Editing K: x", "y"},
		{"create", func(t *testing.T, color bool) Model {
			m := task016CreateTestModel(t)
			m.settings.Color = color
			return left(m, 2)
		}, "Name: my sessi", "o"},
		{"settings group", func(t *testing.T, color bool) Model {
			m := caretGroupRenameModel(t, "sprint")
			m.settings.Color = color
			return left(m, 3)
		}, "New name:  spr", "i"},
		{"settings env", func(t *testing.T, color bool) Model {
			m := settingsOpenOnEnvField(t, config.Settings{Env: map[string]string{"A": "value"}})
			m.settings.Color = color
			m.width, m.height = 100, 30
			return caretPress(m, "enter", "enter", "left", "left")
		}, "Value: val", "u"},
		{"settings string", func(t *testing.T, color bool) Model {
			m := settingsOpenOnStringField(t, config.Settings{}, "pre_launch")
			m.settings.Color = color
			m.width, m.height = 120, 40
			m = caretPress(m, "enter")
			got, _ := m.Update(key("echo hi"))
			return left(got.(Model), 2)
		}, "> echo ", "h"},
		{"settings search", func(t *testing.T, color bool) Model {
			m := New(nil, config.Settings{}, "")
			m.settings.Color = color
			m.width, m.height = 100, 30
			m.settingsOpen = true
			return left(caretPress(m, "/", "verdict"), 3)
		}, "Search: verd", "i"},
	}
}

// TestFocusedFieldDrawsTheCaretCellAtTheRightColumn is R181's first clause: in
// every handler's focused text field the caret cell is SGR 7 on the character
// the caret is on, at the column that character is drawn at, it is the only
// reverse-video cell of the frame, and no `_` stands in for it. Under NO_COLOR
// the caret is still there (it is not colour) and the frame carries no colour.
func TestFocusedFieldDrawsTheCaretCellAtTheRightColumn(t *testing.T) {
	for _, tc := range caretCases() {
		t.Run(tc.name, func(t *testing.T) {
			colourModes(t, func(t *testing.T, color bool) {
				m := tc.build(t, color)
				view := m.View()
				term := renderSettingsToEmulator(t, view, m.width, m.height)
				row := findRowContaining(t, term, tc.prefix)
				col := findCol(t, term, row, tc.prefix) + len([]rune(tc.prefix))
				cell := term.CellAt(col, row)
				if cell == nil || cell.Style.Attrs&uv.AttrReverse == 0 {
					t.Fatalf("cell at row %d column %d (after %q) is not reverse video; reverse cells: %v\n%s", row, col, tc.prefix, reverseCells(term), stripANSI(view))
				}
				if cell.Content != tc.under {
					t.Fatalf("caret cell at row %d column %d shows %q, want %q", row, col, cell.Content, tc.under)
				}
				if got := reverseCells(term); len(got) != 1 {
					t.Fatalf("frame has %d reverse-video cells %v, want exactly the caret", len(got), got)
				}
				if strings.Contains(stripANSI(view), tc.prefix+"_") {
					t.Fatalf("the field draws a `_` stand-in caret:\n%s", stripANSI(view))
				}
				if !color && (strings.Contains(view, "\x1b[48") || strings.Contains(view, "\x1b[38")) {
					t.Fatalf("NO_COLOR frame carries colour:\n%q", view)
				}
			})
		})
	}
}

// TestCaretMovesOneCellWithLeft: the caret is a cell on the field's own row, so
// each left moves it exactly one column and right moves it back.
func TestCaretMovesOneCellWithLeft(t *testing.T) {
	for _, tc := range caretCases() {
		t.Run(tc.name, func(t *testing.T) {
			m := tc.build(t, true)
			at := func(m Model) [2]int {
				cells := reverseCells(frameOf(t, m))
				if len(cells) != 1 {
					t.Fatalf("frame has %d reverse cells, want 1", len(cells))
				}
				return cells[0]
			}
			before := at(m)
			got, _ := m.Update(key("left"))
			after := at(got.(Model))
			if after[1] != before[1] || after[0] != before[0]-1 {
				t.Fatalf("left moved the caret from %v to %v, want one column left on the same row", before, after)
			}
		})
	}
}

// TestUnfocusedFieldDrawsNoCaret: a field that does not have focus draws no
// caret, whatever it holds, and a frame with no focused text field has none.
func TestUnfocusedFieldDrawsNoCaret(t *testing.T) {
	colourModes(t, func(t *testing.T, color bool) {
		// create: the cwd holds text but Name has focus.
		c := task016CreateTestModel(t)
		c.settings.Color = color
		term := frameOf(t, c)
		cwdRow := findRowContaining(t, term, "Working directory:")
		for _, rc := range reverseCells(term) {
			if rc[1] == cwdRow {
				t.Fatalf("create: the unfocused Working directory row draws a caret at %v", rc)
			}
		}
		if got := reverseCells(term); len(got) != 1 {
			t.Fatalf("create: %d carets drawn, want exactly the Name field's", len(got))
		}
		// create: focus moved to a selection field: no text field draws one.
		c.createField = 2 // Agent, a selection field
		if got := reverseCells(frameOf(t, c)); len(got) != 0 {
			t.Fatalf("create: caret drawn with a selection field focused: %v", got)
		}

		// launch inputs: type in field 0, then focus the next text field; field 0
		// keeps its text and loses the caret.
		db, id := newLaunchInputsTestStore(t)
		l := launchInputsTestModel(t, db, id)
		l.settings.Color = color
		l.width, l.height = 100, 40
		l = caretType(l, "ab")
		got, _ := l.Update(key("down"))
		l = got.(Model)
		term = frameOf(t, l)
		preRow := findRowContaining(t, term, "Pre-launch command: ab")
		postRow := findRowContaining(t, term, "Post-destroy command:")
		cells := reverseCells(term)
		if len(cells) != 1 || cells[0][1] != postRow || cells[0][1] == preRow {
			t.Fatalf("launch inputs: carets %v, want exactly one, on the focused Post-destroy row %d (not Pre-launch row %d)", cells, postRow, preRow)
		}

		// settings env: the Key field holds "A" while the Value field has focus.
		s := settingsOpenOnEnvField(t, config.Settings{Env: map[string]string{"A": "value"}})
		s.settings.Color = color
		s.width, s.height = 100, 30
		s = caretPress(s, "enter", "enter")
		term = frameOf(t, s)
		keyRow := findRowContaining(t, term, "Key: A")
		valRow := findRowContaining(t, term, "Value: value")
		cells = reverseCells(term)
		if len(cells) != 1 || cells[0][1] != valRow || cells[0][1] == keyRow {
			t.Fatalf("settings env: carets %v, want exactly one, on the Value row %d (not Key row %d)", cells, valRow, keyRow)
		}
	})
}

// TestOfferedValueCarriesTheSelectionBackground: an offered value (§11.11) is
// drawn as a selection, every cell of its text in the theme's `selection`
// background under colour, and plain under NO_COLOR. Only the create modal can
// tell it from a focused row (the dialogs paint the focused row in `selection`
// anyway), so there the offered cwd sits on an unfocused row and is compared with
// the same row holding the user's own text.
func TestOfferedValueCarriesTheSelectionBackground(t *testing.T) {
	type offered struct {
		name  string
		build func(t *testing.T, color bool) (Model, string) // the model and the offered text
		label string                                         // drawn before it
	}
	cases := []offered{
		{"rename", func(t *testing.T, color bool) (Model, string) {
			m := caretRenameModel(t, "alpha")
			m.settings.Color = color
			return m, "alpha"
		}, "New name:  "},
		{"env editor", func(t *testing.T, color bool) (Model, string) {
			m := caretEnvModel()
			m.settings.Color = color
			m.width, m.height = 100, 30
			return m, "before"
		}, "Editing K: "},
		{"create cwd prefill", func(t *testing.T, color bool) (Model, string) {
			// opened with `n`: the cwd is the start directory, offered, and the
			// Name field has focus, so the offered cwd is not the focused row.
			dir, err := os.MkdirTemp("", "o")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { os.RemoveAll(dir) })
			m := caretCreateModel(t, dir)
			m.settings.Color = color
			return m, dir
		}, "Working directory: "},
		{"settings group", func(t *testing.T, color bool) (Model, string) {
			m := caretGroupRenameModel(t, "sprint")
			m.settings.Color = color
			return m, "sprint"
		}, "New name:  "},
		{"settings env", func(t *testing.T, color bool) (Model, string) {
			m := settingsOpenOnEnvField(t, config.Settings{Env: map[string]string{"A": "value"}})
			m.settings.Color = color
			m.width, m.height = 100, 30
			return caretPress(m, "enter", "enter"), "value"
		}, "Value: "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			colourModes(t, func(t *testing.T, color bool) {
				m, value := tc.build(t, color)
				term := frameOf(t, m)
				selection := tokenHex(t, m, theme.Selection)
				row := findRowContaining(t, term, tc.label+value)
				first := findCol(t, term, row, tc.label+value) + len([]rune(tc.label))
				for i := range []rune(value) {
					bg, ok := cellBgHex(t, term, first+i, row)
					if color && (!ok || bg != selection) {
						t.Fatalf("offered value cell %d of %q has background %q (set %v), want the selection token %s", i, value, bg, ok, selection)
					}
					if !color && ok {
						t.Fatalf("NO_COLOR: offered value cell %d carries background %q", i, bg)
					}
				}
				if tc.name != "create cwd prefill" {
					return
				}
				// The same unfocused cwd holding the user's own text is not a
				// selection: only an offer is drawn as one.
				own := task016CreateTestModel(t)
				own.settings.Color = color
				term = frameOf(t, own)
				row = findRowContaining(t, term, "Working directory: ")
				col := findCol(t, term, row, "Working directory: ") + len([]rune("Working directory: "))
				if bg, ok := cellBgHex(t, term, col, row); ok && bg == selection {
					t.Fatalf("the user's own cwd text carries the selection background %s", bg)
				}
			})
		})
	}
}

// A bracketed paste that joins the clusters on either side of the caret
// (R177, §11.11) leaves the caret on a boundary of the joined text, so the
// focused field still draws its reverse-video caret and one backspace deletes
// the whole joined grapheme.
func TestRenameJoiningPasteKeepsAVisibleCaretOnABoundary(t *testing.T) {
	const woman, laptop, zwj = "\U0001F469", "\U0001F4BB", "\u200d"
	for _, c := range []struct{ name, text, paste, afterBackspace string }{
		{"zwj between emoji", woman + laptop, zwj, ""},
		{"combining mark after a letter", "ab", "\u0301", "b"},
	} {
		t.Run(c.name, func(t *testing.T) {
			m := caretRenameModel(t, c.text)
			m = caretPress(m, "left")
			next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(c.paste), Paste: true})
			m = next.(Model)
			if got := reverseCells(frameOf(t, m)); len(got) == 0 {
				t.Fatalf("no reverse-video caret after a joining paste: value=%q caret=%d", m.renameEdit.Value(), m.renameEdit.Caret())
			}
			m = caretPress(m, "backspace")
			if got := m.renameEdit.Value(); got != c.afterBackspace {
				t.Fatalf("backspace left %q, want %q", got, c.afterBackspace)
			}
		})
	}
}
