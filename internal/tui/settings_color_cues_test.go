package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/vt"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/theme"
)

// renderSettingsToEmulator feeds a settings view string ("\n"-joined, like
// every other view in this package) into a fresh vt.Emulator, so this test
// reads the SAME per-cell foreground/background attributes the harness's
// own token-form steps (features/cell_attributes_test.go's
// cellHasForegroundToken and siblings) read off a real running client's
// grid, rather than grepping the raw escape bytes. "\n" becomes "\r\n"
// because a bare terminal emulator, unlike tmux's own capture-pane (see
// panel.go's splitPreviewLines doc), does not return the cursor to column 0
// on a lone linefeed.
func renderSettingsToEmulator(t *testing.T, view string, width, height int) *vt.Emulator {
	t.Helper()
	term := vt.NewEmulator(width, height)
	payload := strings.ReplaceAll(view, "\n", "\r\n")
	if _, err := term.Write([]byte(payload)); err != nil {
		t.Fatalf("write settings view into emulator: %v", err)
	}
	return term
}

// cellFgHex/cellBgHex mirror features/cell_attributes_test.go's
// cellForegroundHex/cellBackgroundHex exactly (same colour extraction),
// duplicated here rather than imported since that package is test-only and
// internal/tui must not depend on it.
func cellFgHex(t *testing.T, term *vt.Emulator, col, row int) (string, bool) {
	t.Helper()
	cell := term.CellAt(col, row)
	if cell == nil || cell.Style.Fg == nil {
		return "", false
	}
	r, g, b, _ := cell.Style.Fg.RGBA()
	return fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, b>>8), true
}

func cellBgHex(t *testing.T, term *vt.Emulator, col, row int) (string, bool) {
	t.Helper()
	cell := term.CellAt(col, row)
	if cell == nil || cell.Style.Bg == nil {
		return "", false
	}
	r, g, b, _ := cell.Style.Bg.RGBA()
	return fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, b>>8), true
}

// findCol locates the first column on row whose visible text (ignoring
// escapes -- the emulator has already applied them as attributes, not
// literal cells) starts with want, by scanning cell content directly.
func findCol(t *testing.T, term *vt.Emulator, row int, want string) int {
	t.Helper()
	target := []rune(want)
	width := term.Width()
	for col := 0; col+len(target) <= width; col++ {
		match := true
		for i, r := range target {
			cell := term.CellAt(col+i, row)
			if cell == nil || cell.Content != string(r) {
				match = false
				break
			}
		}
		if match {
			return col
		}
	}
	t.Fatalf("row %d has no column starting with %q", row, want)
	return -1
}

// findRowContaining returns the first row containing want anywhere, by
// concatenating each row's cell contents.
func findRowContaining(t *testing.T, term *vt.Emulator, want string) int {
	t.Helper()
	width, height := term.Width(), term.Height()
	for row := 0; row < height; row++ {
		var b strings.Builder
		for col := 0; col < width; col++ {
			if cell := term.CellAt(col, row); cell != nil {
				b.WriteString(cell.Content)
			}
		}
		if strings.Contains(b.String(), want) {
			return row
		}
	}
	t.Fatalf("no row contains %q", want)
	return -1
}

// settingsColorTestModel builds a colour-enabled Model with the takeover open on
// a generous frame (160x30, per the notes.md gotcha: the default 80x24
// truncates a field's env-override label before the assertion below can
// reach it) using the default theme, so hint/text/border_focus/border/
// selection/selection_idle all resolve to the real, distinct hex values
// task 019 requires colouring FROM (SPEC requirement 33: colour only from
// theme tokens).
func settingsColorTestModel(t *testing.T) Model {
	t.Helper()
	m := New(nil, config.Settings{Color: true}, "")
	m.settingsOpen = true
	m.width, m.height = 160, 30
	return m
}

// tokenHex resolves tok's hex colour in m's active theme, failing the test
// (never fabricating a default) if the theme somehow lacks it.
func tokenHex(t *testing.T, m Model, tok theme.Token) string {
	t.Helper()
	hex, err := m.activeTheme().Color(tok)
	if err != nil {
		t.Fatalf("active theme lacks token %q: %v", tok, err)
	}
	return hex
}

// TestSettingsFieldRowLabelIsHintValueIsText proves SPEC requirement 34 for
// the settings field list: the selected field row's label renders in the
// `hint` token and its value renders in `text`, read per-cell off a real
// vt.Emulator grid rather than grepped from raw escape bytes -- and the
// two colours are asserted DISTINCT first, so a theme whose hint and text
// happen to coincide could never make this pass by accident.
func TestSettingsFieldRowLabelIsHintValueIsText(t *testing.T) {
	m := settingsColorTestModel(t)
	hintHex := tokenHex(t, m, theme.Hint)
	textHex := tokenHex(t, m, theme.Text)
	if hintHex == textHex {
		t.Skip("this theme's hint and text tokens happen to share a colour; the distinctness assertion below would be vacuous")
	}

	// allow_yolo is General's first field (category 0, field 0) --
	// opened as the takeover's own default selection.
	view := m.settingsView()
	term := renderSettingsToEmulator(t, view, m.width, m.height)

	label := settingsFieldLabel(config.Field{Key: "allow_yolo"})
	row := findRowContaining(t, term, label)
	labelCol := findCol(t, term, row, label)
	labelFg, ok := cellFgHex(t, term, labelCol, row)
	if !ok {
		t.Fatalf("label %q at row %d col %d has no foreground colour", label, row, labelCol)
	}
	if labelFg != hintHex {
		t.Fatalf("field label %q foreground = %s, want hint token %s", label, labelFg, hintHex)
	}

	value := "On"
	if !m.settingsToggleValueForTest("allow_yolo") {
		value = "Off"
	}
	valueCol := findCol(t, term, row, ": "+value)
	valueCol += len(": ") // skip the ": " separator itself (rendered in `text` too, but the field's own value column is the unambiguous proof point)
	valueFg, ok := cellFgHex(t, term, valueCol, row)
	if !ok {
		t.Fatalf("value %q at row %d col %d has no foreground colour", value, row, valueCol)
	}
	if valueFg != textHex {
		t.Fatalf("field value %q foreground = %s, want text token %s", value, valueFg, textHex)
	}
	if valueFg == hintHex {
		t.Fatalf("field value %q foreground %s matches the LABEL's hint colour -- label/value are not distinguished", value, valueFg)
	}
}

// settingsToggleValueForTest reads a KindToggle field's current staged
// value straight from the schema's own get function, so the test above
// does not hand-guess "On"/"Off" independently of settingsToggleValue's
// real dispatch.
func (m Model) settingsToggleValueForTest(fullKey string) bool {
	for _, f := range config.Schema {
		if f.FullKey() == fullKey {
			return settingsToggleValue(f, m.settingsEdits)
		}
	}
	return false
}

// TestSettingsBorderFocusCueSwapsWithTabAndReverts proves SPEC requirement
// 42's border half for the settings takeover's two lists: the category
// panel's top-left corner renders in `border_focus` while categories have
// focus (the takeover's own default) and `border` once focus moves to
// fields via tab -- and the field panel's border does the exact opposite
// at the same moment, never both or neither.
func TestSettingsBorderFocusCueSwapsWithTabAndReverts(t *testing.T) {
	m := settingsColorTestModel(t)
	focusHex := tokenHex(t, m, theme.BorderFocus)
	borderHex := tokenHex(t, m, theme.Border)
	if focusHex == borderHex {
		t.Skip("this theme's border_focus and border tokens happen to share a colour; the distinctness assertion below would be vacuous")
	}

	assertCorners := func(m Model, wantLeftFocused bool) {
		view := m.settingsView()
		term := renderSettingsToEmulator(t, view, m.width, m.height)
		leftFg, ok := cellFgHex(t, term, 0, 0)
		if !ok {
			t.Fatalf("left panel's top-left corner has no foreground colour")
		}
		leftWidth := settingsCategoryWidth(m.width)
		rightFg, ok := cellFgHex(t, term, leftWidth, 0)
		if !ok {
			t.Fatalf("right panel's top-left (seam) corner has no foreground colour")
		}
		wantLeft, wantRight := borderHex, focusHex
		if wantLeftFocused {
			wantLeft, wantRight = focusHex, borderHex
		}
		if leftFg != wantLeft {
			t.Errorf("left panel corner = %s, want %s (focused=%v)", leftFg, wantLeft, wantLeftFocused)
		}
		if rightFg != wantRight {
			t.Errorf("right panel (seam) corner = %s, want %s (focused=%v)", rightFg, wantRight, !wantLeftFocused)
		}
	}

	// Categories has focus by default.
	assertCorners(m, true)

	updated, _ := m.Update(key("tab"))
	m2 := updated.(Model)
	assertCorners(m2, false)

	updated, _ = m2.Update(key("tab"))
	m3 := updated.(Model)
	assertCorners(m3, true)
}

// TestSettingsSelectionCueUsesSelectionWhenFocusedAndIdleOtherwise proves
// SPEC requirement 42's selection half: the category list's currently
// highlighted row carries a `selection` BACKGROUND while categories has
// focus, and `selection_idle` once focus moves to fields -- both lists
// always show their own highlighted row, distinguished only by which
// token, never by one disappearing outright.
func TestSettingsSelectionCueUsesSelectionWhenFocusedAndIdleOtherwise(t *testing.T) {
	m := settingsColorTestModel(t)
	selHex := tokenHex(t, m, theme.Selection)
	idleHex := tokenHex(t, m, theme.SelectionIdle)
	if selHex == idleHex {
		t.Skip("this theme's selection and selection_idle tokens happen to share a colour; the distinctness assertion below would be vacuous")
	}

	categories := settingsCategories()
	if len(categories) == 0 {
		t.Fatal("no settings categories to select")
	}
	selectedCategory := categories[0].Name

	// Categories focused (the default): the selected category row's
	// marker/name renders on `selection`.
	view := m.settingsView()
	term := renderSettingsToEmulator(t, view, m.width, m.height)
	row := findRowContaining(t, term, selectedCategory)
	col := findCol(t, term, row, selectedCategory)
	bg, ok := cellBgHex(t, term, col, row)
	if !ok {
		t.Fatalf("selected category row has no background colour while categories are focused")
	}
	if bg != selHex {
		t.Fatalf("selected category background = %s while focused, want selection token %s", bg, selHex)
	}

	// Move focus to fields: the SAME category row must now show
	// selection_idle, not vanish.
	updated, _ := m.Update(key("tab"))
	m2 := updated.(Model)
	view2 := m2.settingsView()
	term2 := renderSettingsToEmulator(t, view2, m2.width, m2.height)
	row2 := findRowContaining(t, term2, selectedCategory)
	col2 := findCol(t, term2, row2, selectedCategory)
	bg2, ok := cellBgHex(t, term2, col2, row2)
	if !ok {
		t.Fatalf("selected category row has no background colour once fields are focused")
	}
	if bg2 != idleHex {
		t.Fatalf("selected category background = %s once fields are focused, want selection_idle token %s", bg2, idleHex)
	}

	// The field list's own selected row must show the OPPOSITE pairing:
	// selection now that fields have focus.
	fields := categories[0].Fields
	if len(fields) == 0 {
		t.Fatal("first category has no fields to select")
	}
	fieldLabel := settingsFieldLabel(fields[0])
	frow := findRowContaining(t, term2, fieldLabel)
	fcol := findCol(t, term2, frow, fieldLabel)
	fbg, ok := cellBgHex(t, term2, fcol, frow)
	if !ok {
		t.Fatalf("selected field row has no background colour while fields are focused")
	}
	if fbg != selHex {
		t.Fatalf("selected field background = %s while focused, want selection token %s", fbg, selHex)
	}
}
