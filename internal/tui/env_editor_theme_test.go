package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/theme"
)

// task017EnvTestModel builds a colour-enabled `e` env editor Model at
// deck's documented 80x24 minimum, open on a session whose env map
// supplies a handful of non-secret-shaped keys -- mirroring
// task016CreateTestModel one file over (create_view_theme_test.go), same
// reason: every theming test in this file needs the identical minimum
// terminal size the criteria are stated against.
func task017EnvTestModel(t *testing.T) Model {
	t.Helper()
	m := New(nil, config.Settings{Color: true}, "")
	m.width, m.height = 80, 24
	m.sessions = []store.Session{{
		ID:   "s1",
		Name: "theme session",
		Env: map[string]string{
			"ALPHA_VAR": "alpha-value",
			"BETA_VAR":  "beta-value",
		},
	}}
	m.selected = rowCursor(0)
	m.envEditing = true
	m.envCursor = 0
	return m
}

// task017ManyEnvKeysModel is task017EnvTestModel with enough resolved keys
// (task 014's own height probe: from 16 up) that the env editor's
// unbounded framedDialog used to overflow an 80x24 frame with no way to
// reach the closing hint line -- the gap this task closes.
func task017ManyEnvKeysModel(t *testing.T) Model {
	t.Helper()
	m := New(nil, config.Settings{Color: true}, "")
	m.width, m.height = 80, 24
	env := make(map[string]string, 20)
	for i := 0; i < 20; i++ {
		env[task017EnvKeyName(i)] = "value"
	}
	m.sessions = []store.Session{{ID: "s1", Name: "overflow session", Env: env}}
	m.selected = rowCursor(0)
	m.envEditing = true
	m.envCursor = 0
	return m
}

// task017EnvKeyName names env key N deterministically ("VAR_00",
// "VAR_01", ...) so the 20-key model above sorts predictably and never
// collides with a secret-shaped substring.
func task017EnvKeyName(i int) string {
	digits := "0123456789"
	tens, ones := i/10, i%10
	return "VAR_" + string(digits[tens]) + string(digits[ones])
}

// TestEnvBodyMeasurementsCarryNoSGRBytes is task 017's own success
// criterion (mirroring TestCreateBodyMeasurementsCarryNoSGRBytes): envBody
// -- the plain body wrapDialogLines/dialogMaxScroll/PgUp/PgDn measure --
// must never carry an escape byte, with Color enabled, so a theme change
// can never move where a page boundary falls. styledEnvBody, by contrast,
// is expected to carry SGR bytes, proving the split is real.
func TestEnvBodyMeasurementsCarryNoSGRBytes(t *testing.T) {
	m := task017EnvTestModel(t)
	m.envNote = "Cannot edit environment: boom"

	plain := m.envBody()
	if strings.ContainsRune(plain, 0x1b) {
		t.Fatalf("envBody() (the plain body scroll math measures) contains an escape byte:\n%q", plain)
	}
	for i, line := range m.wrapDialogLines(plain) {
		if strings.ContainsRune(line, 0x1b) {
			t.Fatalf("wrapDialogLines(envBody())[%d] contains an escape byte: %q", i, line)
		}
	}

	if styled := m.styledEnvBody(); !strings.ContainsRune(styled, 0x1b) {
		t.Fatalf("styledEnvBody() carries no escape byte at all with Color enabled -- theming did not apply:\n%q", styled)
	}
}

// TestStyledEnvBodyMatchesPlainBodyLineForLine proves styledEnvBody never
// adds or removes a physical line relative to wrapDialogLines(envBody())
// and, once every escape sequence is stripped back out, is byte-identical
// to it -- the general form of task 016's own plain-vs-styled split,
// applied to the env editor (task 017).
func TestStyledEnvBodyMatchesPlainBodyLineForLine(t *testing.T) {
	browsing := task017EnvTestModel(t)

	editing := task017EnvTestModel(t)
	editing.envEditKey, editing.envEditValue, editing.envEditPrefilled = "ALPHA_VAR", "typed-value", false

	withNote := task017EnvTestModel(t)
	withNote.envNote = "Cannot edit environment: boom"

	revealed := task017EnvTestModel(t)
	revealed.envReveal = true

	for _, tc := range []struct {
		name string
		m    Model
	}{
		{"browsing", browsing},
		{"editing a row", editing},
		{"with a note", withNote},
		{"revealed", revealed},
	} {
		plainBody := tc.m.envBody()
		if strings.ContainsRune(plainBody, 0x1b) {
			t.Fatalf("%s: envBody() contains an escape byte:\n%q", tc.name, plainBody)
		}
		plain := tc.m.wrapDialogLines(plainBody)
		styled := strings.Split(tc.m.styledEnvBody(), "\n")
		if len(styled) != len(plain) {
			t.Fatalf("%s: styledEnvBody has %d physical lines, wrapDialogLines(envBody()) has %d:\nplain: %q\nstyled: %q", tc.name, len(styled), len(plain), plain, styled)
		}
		for i := range plain {
			if got := stripANSI(styled[i]); got != plain[i] {
				t.Fatalf("%s: line %d differs once escapes are stripped:\n styled: %q\n  plain: %q", tc.name, i, got, plain[i])
			}
		}
	}
}

// TestEnvViewStaysWithinFrameBudgetAt80x24 mirrors
// TestCreateViewStaysWithinFrameBudgetAt80x24: task 014's own height probe
// found the env editor overflows an 80x24 frame from 16 resolved keys up
// through the unbounded framedDialog path -- this proves
// framedDialogScrollable actually clips it (task 017) rather than the
// dialog drawing past the frame.
func TestEnvViewStaysWithinFrameBudgetAt80x24(t *testing.T) {
	m := task017ManyEnvKeysModel(t)
	view := m.envView()
	if n := countViewLines(view); n > 24 {
		t.Fatalf("env view is %d lines at 80x24, want <= 24:\n%s", n, view)
	}
	for i, line := range strings.Split(view, "\n") {
		if w := stringWidth(line); w != m.dialogWidth() {
			t.Fatalf("env view line %d width = %d, want dialogWidth() = %d: %q", i, w, m.dialogWidth(), line)
		}
	}
}

// TestEnvViewClosingHintReachableViaPgDown proves the env editor's own
// closing legend line ("j/k select a key ... Esc closes.") -- absent from
// the first 80x24 page once the resolved-key list overflows it -- becomes
// visible after paging down, and paging back up returns to the top (task
// 017's own "submit line stays reachable" success criterion, mirroring
// TestCreateViewSubmitLineReachableViaPgDown).
func TestEnvViewClosingHintReachableViaPgDown(t *testing.T) {
	m := task017ManyEnvKeysModel(t)

	first := m.envView()
	if strings.Contains(first, "closes.") {
		t.Fatalf("the closing hint is already visible on the first page -- this test needs more overflowing content to be non-vacuous:\n%s", first)
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	m2 := updated.(Model)
	bottom := m2.envView()
	if !strings.Contains(bottom, "closes.") {
		t.Fatalf("paging down never reached the closing hint line:\n%s", bottom)
	}

	updated, _ = m2.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	m3 := updated.(Model)
	top := m3.envView()
	if !strings.Contains(top, "Environment for") {
		t.Fatalf("paging back up did not return to the top of the dialog:\n%s", top)
	}
	if m3.envScroll != 0 {
		t.Fatalf("envScroll = %d after paging fully back up, want 0", m3.envScroll)
	}
}

// TestEnvViewLabelHintValueTextTitleDimmed proves SPEC.md:1355's token
// mapping for the env editor (task 017): the title in `title`, a row's key
// in `hint`, its value in `text`, and the order line's explanatory text in
// `dimmed`, read per-cell off a real vt.Emulator grid.
func TestEnvViewLabelHintValueTextTitleDimmed(t *testing.T) {
	m := task017EnvTestModel(t)
	titleHex := tokenHex(t, m, theme.Title)
	hintHex := tokenHex(t, m, theme.Hint)
	textHex := tokenHex(t, m, theme.Text)
	dimmedHex := tokenHex(t, m, theme.Dimmed)

	view := m.envView()
	term := renderSettingsToEmulator(t, view, m.width, m.height)

	titleRow := findRowContaining(t, term, "Environment for theme session")
	titleCol := findCol(t, term, titleRow, "Environment")
	if fg, ok := cellFgHex(t, term, titleCol, titleRow); !ok || fg != titleHex {
		t.Fatalf("title foreground = %q ok=%v, want title token %s", fg, ok, titleHex)
	}

	// BETA_VAR is never the cursor's row (m.envCursor == 0, on ALPHA_VAR)
	// in task017EnvTestModel, so it exercises the plain (non-selection)
	// label/value colouring.
	betaRow := findRowContaining(t, term, "BETA_VAR")
	labelCol := findCol(t, term, betaRow, "BETA_VAR")
	if fg, ok := cellFgHex(t, term, labelCol, betaRow); !ok || fg != hintHex {
		t.Fatalf("BETA_VAR label foreground = %q ok=%v, want hint token %s", fg, ok, hintHex)
	}
	valueCol := findCol(t, term, betaRow, "beta-value")
	if fg, ok := cellFgHex(t, term, valueCol, betaRow); !ok || fg != textHex {
		t.Fatalf("BETA_VAR value foreground = %q ok=%v, want text token %s", fg, ok, textHex)
	}

	orderRow := findRowContaining(t, term, "Order, lowest to highest")
	orderCol := findCol(t, term, orderRow, "Order,")
	if fg, ok := cellFgHex(t, term, orderCol, orderRow); !ok || fg != dimmedHex {
		t.Fatalf("order line foreground = %q ok=%v, want dimmed token %s", fg, ok, dimmedHex)
	}
}

// TestEnvViewFocusedRowGetsSelectionBackground proves the remaining half
// of SPEC.md:1355's mapping: "the focused field carrying the same
// selection treatment a selected list row does". m.envCursor names
// ALPHA_VAR in task017EnvTestModel; BETA_VAR is not the cursor's row and
// must carry no selection background at all.
func TestEnvViewFocusedRowGetsSelectionBackground(t *testing.T) {
	m := task017EnvTestModel(t)
	selectionHex := tokenHex(t, m, theme.Selection)

	view := m.envView()
	term := renderSettingsToEmulator(t, view, m.width, m.height)

	alphaRow := findRowContaining(t, term, "ALPHA_VAR")
	alphaCol := findCol(t, term, alphaRow, "ALPHA_VAR")
	if bg, ok := cellBgHex(t, term, alphaCol, alphaRow); !ok || bg != selectionHex {
		t.Fatalf("cursor row (ALPHA_VAR) background = %q ok=%v, want selection token %s", bg, ok, selectionHex)
	}

	betaRow := findRowContaining(t, term, "BETA_VAR")
	betaCol := findCol(t, term, betaRow, "BETA_VAR")
	if bg, ok := cellBgHex(t, term, betaCol, betaRow); ok && bg == selectionHex {
		t.Fatalf("non-cursor row (BETA_VAR) unexpectedly carries the selection background %s", bg)
	}
}

// TestEnvViewNoteIsError proves envNote (always an in-dialog failure
// reason -- see submitEnvEdit/envEdited) renders in SPEC.md:1355's `error`
// token, matching every other §11.4 dialog's own validation-message
// colouring.
func TestEnvViewNoteIsError(t *testing.T) {
	m := task017EnvTestModel(t)
	m.envNote = "Cannot edit environment: unavailable"
	errorHex := tokenHex(t, m, theme.Error)

	view := m.envView()
	term := renderSettingsToEmulator(t, view, m.width, m.height)

	row := findRowContaining(t, term, "Cannot edit environment")
	col := findCol(t, term, row, "Cannot")
	if fg, ok := cellFgHex(t, term, col, row); !ok || fg != errorHex {
		t.Fatalf("envNote foreground = %q ok=%v, want error token %s", fg, ok, errorHex)
	}
}

// TestEnvViewClosingHintKeysAreKeyToken proves the closing legend line's
// own bound-key words (SPEC.md:1355's "the keys in a footer legend in
// `key`") render distinctly from the surrounding prose, and that the prose
// itself is `hint` -- R82's "footer keys -> key with the rest in hint",
// the same split styledCreateBody's colorFooterLine already applies -- in
// both the browsing and the editing variant of that line.
func TestEnvViewClosingHintKeysAreKeyToken(t *testing.T) {
	m := task017EnvTestModel(t)
	keyHex := tokenHex(t, m, theme.Key)
	hintHex := tokenHex(t, m, theme.Hint)
	if keyHex == hintHex {
		t.Skip("this theme's key and hint tokens happen to share a colour; the distinctness assertion below would be vacuous")
	}

	view := m.envView()
	term := renderSettingsToEmulator(t, view, m.width, m.height)

	row := findRowContaining(t, term, "j/k select a key")
	jkCol := findCol(t, term, row, "j/k")
	if fg, ok := cellFgHex(t, term, jkCol, row); !ok || fg != keyHex {
		t.Fatalf("j/k foreground = %q ok=%v, want key token %s", fg, ok, keyHex)
	}
	selectCol := findCol(t, term, row, "select")
	if fg, ok := cellFgHex(t, term, selectCol, row); !ok || fg != hintHex {
		t.Fatalf("surrounding prose foreground = %q ok=%v, want hint token %s", fg, ok, hintHex)
	}

	editing := task017EnvTestModel(t)
	editing.envEditKey, editing.envEditValue = "ALPHA_VAR", "typed"
	editView := editing.envView()
	editTerm := renderSettingsToEmulator(t, editView, editing.width, editing.height)
	editRow := findRowContaining(t, editTerm, "Enter saves this key")
	enterCol := findCol(t, editTerm, editRow, "Enter")
	if fg, ok := cellFgHex(t, editTerm, enterCol, editRow); !ok || fg != keyHex {
		t.Fatalf("edit-mode Enter foreground = %q ok=%v, want key token %s", fg, ok, keyHex)
	}
}

// TestEnvViewEditPromptAndSubmitLineVisibleWhenListOverflows closes the
// gap task 017's first pass left: with enough resolved keys to overflow an
// 80x24 frame, pressing enter on the cursor's row opened an edit whose
// submit legend ("Enter saves this key") and typed value both sat off the
// bottom of the visible page -- and PgDn could not reach them either,
// because updateEnvDialog returned early for every key while a row was
// open. features/env_editor_test.go's keyboard-only PTY step reads exactly
// those two strings out of this view with no scroll keystroke in between,
// so both must be on screen the instant the edit opens.
func TestEnvViewEditPromptAndSubmitLineVisibleWhenListOverflows(t *testing.T) {
	m := task017ManyEnvKeysModel(t)

	browsing := m.envView()
	if strings.Contains(browsing, "closes.") {
		t.Fatalf("the closing legend is already visible while browsing -- this test needs an overflowing list to be non-vacuous:\n%s", browsing)
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	editing := updated.(Model)
	if editing.envEditKey != task017EnvKeyName(0) {
		t.Fatalf("enter opened edit on key %q, want %q", editing.envEditKey, task017EnvKeyName(0))
	}

	// stripANSI first: colorLegendLine colours the legend word by word, so
	// the raw view string carries an escape run between "Enter" and
	// "saves". What the PTY step matches is the rendered GRID, which is
	// what stripANSI reproduces here.
	view := stripANSI(editing.envView())
	if !strings.Contains(view, "Enter saves this key") {
		t.Fatalf("the edit-mode submit legend is not on screen when the edit opens:\n%s", view)
	}
	if !strings.Contains(view, "Editing "+task017EnvKeyName(0)) {
		t.Fatalf("the edit prompt naming the open key is not on screen:\n%s", view)
	}
	if n := countViewLines(view); n > 24 {
		t.Fatalf("editing env view is %d lines at 80x24, want <= 24:\n%s", n, view)
	}

	typed, _ := editing.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("typed-here")})
	typing := typed.(Model)
	typedView := stripANSI(typing.envView())
	if !strings.Contains(typedView, "typed-here") {
		t.Fatalf("the typed value is not on screen while editing an overflowing list:\n%s", typedView)
	}
	if !strings.Contains(typedView, "Enter saves this key") {
		t.Fatalf("the submit legend fell off screen once typing started:\n%s", typedView)
	}
}

// TestEnvEditModePagingScrollsAndKeepsTheEditOpen proves PgUp/PgDn stay
// live while a row is open for editing (they were silent no-ops before,
// swallowed by updateEnvDialog's edit-mode early return) and that paging
// never abandons the edit in progress.
func TestEnvEditModePagingScrollsAndKeepsTheEditOpen(t *testing.T) {
	m := task017ManyEnvKeysModel(t)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	editing := updated.(Model)
	if max := editing.dialogMaxScroll(editing.envBody()); max == 0 {
		t.Fatalf("the editing body already fits the frame (maxScroll 0) -- this test needs an overflowing list to be non-vacuous")
	}

	paged, _ := editing.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	down := paged.(Model)
	if down.envScroll == 0 {
		t.Fatalf("PgDn while editing left envScroll at 0 (the paging keys are still being swallowed by edit mode)")
	}
	if down.envEditKey != editing.envEditKey {
		t.Fatalf("PgDn while editing changed the open key: %q -> %q", editing.envEditKey, down.envEditKey)
	}
	if !down.envEditing {
		t.Fatal("PgDn while editing closed the env editor")
	}

	back, _ := down.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	up := back.(Model)
	if up.envScroll >= down.envScroll {
		t.Fatalf("PgUp while editing did not scroll back: %d -> %d", down.envScroll, up.envScroll)
	}
	if up.envEditKey != editing.envEditKey {
		t.Fatalf("PgUp while editing changed the open key: %q -> %q", editing.envEditKey, up.envEditKey)
	}
}
