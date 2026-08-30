package tui

import (
	"os"
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/theme"
)

// task016CreateTestModel builds a colour-enabled create-modal Model with
// every field populated (mirroring create_validation_test.go's
// newCreatingModel, which this file must not disturb -- see that test's
// own TestCreateModalSlugCollisionMessage) at deck's documented 80x24
// minimum.
func task016CreateTestModel(t *testing.T) Model {
	t.Helper()
	m := New(nil, config.Settings{Color: true}, "")
	m.width, m.height = 80, 24
	m.creating = true
	m.createName = "my session"
	m.createCWD = t.TempDir()
	m.createAgent = "shell"
	m.createProfile = "safe"
	m.createField = 0
	return m
}

// TestCreateFieldRowsSourceContainsNoColorToken pins the success
// criterion literally: createFieldRows must go on returning plain
// strings, colour applied only in the view (createView/styledCreateBody),
// never baked into the field table itself. Grepping the function's own
// source text (rather than merely trusting styledCreateBody's existence)
// is what would catch a future edit that colours a value or help string
// directly inside the table.
func TestCreateFieldRowsSourceContainsNoColorToken(t *testing.T) {
	data, err := os.ReadFile("tui.go")
	if err != nil {
		t.Fatalf("ReadFile(tui.go): %v", err)
	}
	src := string(data)

	start := strings.Index(src, "func (m Model) createFieldRows()")
	if start == -1 {
		t.Fatal("createFieldRows function not found in tui.go")
	}
	rest := src[start+1:]
	nextFuncRe := regexp.MustCompile(`(?m)^func `)
	loc := nextFuncRe.FindStringIndex(rest)
	if loc == nil {
		t.Fatal("no function follows createFieldRows -- cannot bound its body")
	}
	body := src[start : start+1+loc[0]]

	if strings.Contains(body, "colorToken") {
		t.Fatalf("createFieldRows' own source mentions colorToken -- colour must be applied in the view, not the field table:\n%s", body)
	}
	if strings.Contains(body, "\x1b[") {
		t.Fatalf("createFieldRows' own source contains a raw escape byte:\n%q", body)
	}
}

// TestCreateBodyMeasurementsCarryNoSGRBytes is task 016's own success
// criterion: wrapDialogLines/dialogContentBudget must measure the create
// modal's PLAIN body, never the coloured one, so a theme change can never
// move where a page boundary falls (the same reason updateCreate's new
// pgup/pgdown cases pass createBody(), not styledCreateBody(), to
// dialogScrollByPage). Every physical line wrapDialogLines(createBody())
// returns is checked for a raw escape byte -- with Color enabled, so a
// regression that started colouring before wrapping would be caught here,
// not hidden by NO_COLOR.
func TestCreateBodyMeasurementsCarryNoSGRBytes(t *testing.T) {
	m := task016CreateTestModel(t)
	m.createLaunchArgs = `["--flag"]`
	m.createEnv = "KEY=value"
	m.createPreLaunch = "echo hi"
	m.createError = "working directory is required"

	plain := m.createBody()
	if strings.ContainsRune(plain, 0x1b) {
		t.Fatalf("createBody() (the plain body scroll math measures) contains an escape byte:\n%q", plain)
	}
	for i, line := range m.wrapDialogLines(plain) {
		if strings.ContainsRune(line, 0x1b) {
			t.Fatalf("wrapDialogLines(createBody())[%d] contains an escape byte: %q", i, line)
		}
	}
	if budget := m.dialogContentBudget(); budget != m.height-2 {
		t.Fatalf("dialogContentBudget() = %d, want frameHeight-2 = %d (must not itself depend on colour)", budget, m.height-2)
	}

	// styledCreateBody, by contrast, is the coloured render body and is
	// expected to carry SGR bytes -- proving the split is real, not that
	// colour vanished everywhere.
	if styled := m.styledCreateBody(); !strings.ContainsRune(styled, 0x1b) {
		t.Fatalf("styledCreateBody() carries no escape byte at all with Color enabled -- theming did not apply:\n%q", styled)
	}
}

// task016GhostModel builds a colour-enabled create modal focused on the
// cwd field, whose text is the prefix of exactly ONE directory -- so
// createCWDGhostCompletion has a ghost to offer and the cwd row's value
// really does end in completion text the user never typed. It returns the
// model and the ghost suffix the field is expected to show.
// The parent directory is a SHORT scratch path (os.MkdirTemp, not
// t.TempDir, whose name embeds this test's own long name): the cwd row's
// rendered value must fit one physical line inside the 80x24 dialog for the
// per-cell assertions below to read the ghost off a single grid row.
func task016GhostModel(t *testing.T) (Model, string) {
	t.Helper()
	m := task016CreateTestModel(t)
	parent, err := os.MkdirTemp("", "g")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(parent) })
	if err := os.Mkdir(parent+"/unique-directory", 0o755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	m.createField = 1
	m.createCWD = parent + "/uni"
	ghost, ok := createCWDGhostCompletion(m.createCWD)
	if !ok || ghost != "que-directory/" {
		t.Fatalf("createCWDGhostCompletion(%q) = %q ok=%v, want %q -- this test needs a live ghost to be non-vacuous", m.createCWD, ghost, ok, "que-directory/")
	}
	return m, ghost
}

// TestCreateCWDGhostValueCarriesNoSGRBytes closes task 016's own
// validation finding: the cwd field's ghost completion used to be coloured
// inside createCWDDisplayValue, so createFieldRows()[1].value came back as
// `.../uni\x1b[38;2;...mque-directory/\x1b[0m` and createBody -- the body
// wrapDialogLines/dialogMaxScroll measure -- carried SGR bytes that count
// as display width. The ghost path is the ONE field value produced by a
// helper rather than held verbatim in the model, which is why it needs its
// own no-escape assertion beyond
// TestCreateBodyMeasurementsCarryNoSGRBytes' fully-typed model.
func TestCreateCWDGhostValueCarriesNoSGRBytes(t *testing.T) {
	m, ghost := task016GhostModel(t)

	rows := m.createFieldRows()
	value := rows[1].value
	if strings.ContainsRune(value, 0x1b) {
		t.Fatalf("createFieldRows()[1].value contains an escape byte: %q", value)
	}
	if want := m.createCWD + ghost; value != want {
		t.Fatalf("createFieldRows()[1].value = %q, want the plain typed text plus its plain ghost %q", value, want)
	}
	for i, row := range rows {
		for _, field := range []struct{ what, s string }{{"label", row.label}, {"value", row.value}, {"help", row.help}} {
			if strings.ContainsRune(field.s, 0x1b) {
				t.Fatalf("createFieldRows()[%d].%s contains an escape byte: %q", i, field.what, field.s)
			}
		}
	}

	plain := m.createBody()
	if strings.ContainsRune(plain, 0x1b) {
		t.Fatalf("createBody() with a live ghost contains an escape byte:\n%q", plain)
	}
	for i, line := range m.wrapDialogLines(plain) {
		if strings.ContainsRune(line, 0x1b) {
			t.Fatalf("wrapDialogLines(createBody())[%d] contains an escape byte: %q", i, line)
		}
	}
	// The measurements themselves must be identical to those of the same
	// body with the ghost's bytes typed out by hand: proof no invisible
	// byte is being counted as width anywhere in the scroll math.
	typed := m
	typed.createCWD = m.createCWD + ghost
	typed.createField = 0 // no ghost of its own to add on top
	if got, want := len(m.wrapDialogLines(plain)), len(typed.wrapDialogLines(typed.createBody())); got != want {
		t.Fatalf("ghosted body wraps to %d lines, the same text typed out wraps to %d -- a hidden byte is being measured", got, want)
	}
}

// TestCreateViewGhostRendersHintAtRenderTime is the other half: moving
// the colour out of createFieldRows must not lose it. The cwd row is the
// one this task (1203) focuses -- task016GhostModel leaves it focused
// (m.createField == 1), so this row's background is theme.Selection
// (renderCreateRowSegments), exactly the pair R84's contrast floor holds
// every dialogSelectionTokens entry (internal/theme/contrast_test.go) to.
// The ghost's cells render in
// the `hint` token (not the sub-floor `dimmed` this test used to assert,
// before task 1203: `dimmed` measures 2.59:1 on cobalt, 2.69:1 on empire
// and 2.51:1 on parchment over theme.Selection, all below the 3.0:1 floor,
// while `hint` clears it on every built-in), and the typed part of the
// same value still renders in `text` -- distinct from the ghost's `hint`,
// read per-cell off a real emulator grid, so the ghost stays visually
// distinguishable from what the user actually typed even though both now
// clear the floor.
func TestCreateViewGhostRendersHintAtRenderTime(t *testing.T) {
	m, _ := task016GhostModel(t)
	hintHex := tokenHex(t, m, theme.Hint)
	textHex := tokenHex(t, m, theme.Text)
	if hintHex == textHex {
		t.Fatalf("theme.Hint and theme.Text resolve to the same colour %s -- this test needs them visually distinct to be non-vacuous", hintHex)
	}

	view := m.createView()
	term := renderSettingsToEmulator(t, view, m.width, m.height)

	row := findRowContaining(t, term, "que-directory/")
	ghostCol := findCol(t, term, row, "que-directory/")
	ghostFg, ok := cellFgHex(t, term, ghostCol, row)
	if !ok || ghostFg != hintHex {
		t.Fatalf("ghost completion foreground = %q ok=%v, want hint token %s", ghostFg, ok, hintHex)
	}
	typedCol := ghostCol - 1 // the "i" of the typed ".../uni"
	typedFg, ok := cellFgHex(t, term, typedCol, row)
	if !ok || typedFg != textHex {
		t.Fatalf("typed cwd text foreground = %q ok=%v, want text token %s -- the hint span must cover the ghost only", typedFg, ok, textHex)
	}
	if ghostFg == typedFg {
		t.Fatalf("ghost segment foreground %s equals the typed segment's foreground %s -- the ghost must stay visually distinct from typed text", ghostFg, typedFg)
	}
}

// TestStyledCreateBodyMatchesPlainBodyLineForLine is the general form of
// task 016's plain-vs-styled split: strip every escape sequence back out of
// styledCreateBody and what remains must be wrapDialogLines(createBody())
// line for line -- same count, same bytes. That catches both directions of
// the bug validation found on the cwd ghost path (a colour baked into a
// value shifts the measured width, and a colour applied before wrapping
// shifts where a line breaks) for EVERY row of the modal at once, including
// the cases where the ghost text itself contains a space and so is subject
// to word-wrap.
func TestStyledCreateBodyMatchesPlainBodyLineForLine(t *testing.T) {
	spaced, spacedGhost := task016SpacedGhostModel(t)
	ghosted, _ := task016GhostModel(t)

	full := task016CreateTestModel(t)
	full.createLaunchArgs = `["--flag"]`
	full.createEnv = "KEY=value"
	full.createPreLaunch = "echo hi"
	full.createError = "working directory is required"

	candidates := task016CreateTestModel(t)
	candidates.createField = 1
	candidates.createCWDCandidates = []string{"alpha", "beta"}
	candidates.createCWDCandidateIndex = 1

	for _, tc := range []struct {
		name string
		m    Model
	}{
		{"fully typed with an error", full},
		{"unique-match ghost", ghosted},
		{"ghost containing a space (" + spacedGhost + ")", spaced},
		{"tab-completion candidate list", candidates},
	} {
		plain := m0WrapPlain(t, tc.m)
		styled := strings.Split(tc.m.styledCreateBody(), "\n")
		if len(styled) != len(plain) {
			t.Fatalf("%s: styledCreateBody has %d physical lines, wrapDialogLines(createBody()) has %d", tc.name, len(styled), len(plain))
		}
		for i := range plain {
			if got := stripANSI(styled[i]); got != plain[i] {
				t.Fatalf("%s: line %d differs once escapes are stripped:\n styled: %q\n  plain: %q", tc.name, i, got, plain[i])
			}
		}
	}
}

// m0WrapPlain is wrapDialogLines(createBody()) with the no-escape check the
// whole split exists for, so every case above asserts it too.
func m0WrapPlain(t *testing.T, m Model) []string {
	t.Helper()
	body := m.createBody()
	if strings.ContainsRune(body, 0x1b) {
		t.Fatalf("createBody() contains an escape byte:\n%q", body)
	}
	return m.wrapDialogLines(body)
}

// task016SpacedGhostModel is the one ghost shape word-wrap can actually
// split: a directory whose NAME contains a space, so the ghost suffix is
// two words and wrapDialogLines may break between them. wrapText never
// hard-breaks a single long word (panel.go), so a space-free path -- however
// long -- always stays on one physical line.
func task016SpacedGhostModel(t *testing.T) (Model, string) {
	t.Helper()
	m := task016CreateTestModel(t)
	parent, err := os.MkdirTemp("", "g")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(parent) })
	// Pad so the first word of the value nearly fills the dialog's inner
	// width and the ghost's second word is pushed onto the next line.
	inner := m.dialogWidth() - 4
	padLen := inner - 4 - len(parent) - 1 - len("uni")
	if padLen < 1 {
		t.Fatalf("scratch parent %q leaves no room to pad within inner width %d", parent, inner)
	}
	segment := "uni" + strings.Repeat("x", padLen)
	if err := os.Mkdir(parent+"/"+segment+" tail", 0o755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	m.createField = 1
	m.createCWD = parent + "/" + segment
	ghost, ok := createCWDGhostCompletion(m.createCWD)
	if !ok || ghost != " tail/" {
		t.Fatalf("createCWDGhostCompletion(%q) = %q ok=%v, want %q", m.createCWD, ghost, ok, " tail/")
	}
	return m, ghost
}

// TestCreateViewStaysWithinFrameBudgetAt80x24 mirrors
// height_bound_test.go's own three overlay assertions: the create modal's
// field set alone reaches well past 24 lines untouched
// (framedDialogScrollable's own doc comment measured 29), so this proves
// framedDialogScrollable actually clips it at deck's documented minimum
// rather than the modal drawing past the frame.
func TestCreateViewStaysWithinFrameBudgetAt80x24(t *testing.T) {
	m := task016CreateTestModel(t)
	m.createLaunchArgs = `["--flag"]`
	m.createEnv = "KEY=value"
	m.createPreLaunch = "echo hi"
	view := m.createView()
	if n := countViewLines(view); n > 24 {
		t.Fatalf("create view is %d lines at 80x24, want <= 24:\n%s", n, view)
	}
	for i, line := range strings.Split(view, "\n") {
		if w := stringWidth(line); w != m.dialogWidth() {
			t.Fatalf("create view line %d width = %d, want dialogWidth() = %d: %q", i, w, m.dialogWidth(), line)
		}
	}
}

// TestCreateViewSubmitLineReachableViaPgDown proves the modal's own footer
// legend ("...Enter submits...") -- absent from the first 80x24 page once
// the field set overflows it -- becomes visible after paging down, and
// paging back up returns to the top (task 016's own "submit line
// reachable" success criterion).
func TestCreateViewSubmitLineReachableViaPgDown(t *testing.T) {
	m := task016CreateTestModel(t)
	m.createLaunchArgs = `["--flag"]`
	m.createEnv = "KEY=value"
	m.createPreLaunch = "echo hi"

	first := m.createView()
	if strings.Contains(first, "Enter") && strings.Contains(first, "submits") {
		t.Fatalf("the footer legend is already visible on the first page -- this test needs more overflowing content to be non-vacuous:\n%s", first)
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	m2 := updated.(Model)
	bottom := m2.createView()
	// The footer legend line itself can land right on a word-wrap
	// boundary ("...Enter" ends one physical line, "submits..." starts
	// the next -- createBody's own wrapping, unrelated to this task's
	// colouring), so both words are checked independently rather than as
	// one contiguous substring.
	if !strings.Contains(bottom, "Enter") || !strings.Contains(bottom, "submits") {
		t.Fatalf("paging down never reached the footer legend:\n%s", bottom)
	}

	updated, _ = m2.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	m3 := updated.(Model)
	top := m3.createView()
	if !strings.Contains(top, "Create shell session") {
		t.Fatalf("paging back up did not return to the top of the modal:\n%s", top)
	}
	if m3.createScroll != 0 {
		t.Fatalf("createScroll = %d after paging fully back up, want 0", m3.createScroll)
	}
}

// TestCreateViewLabelHintValueTextTitleDimmed proves SPEC.md:1355's token
// mapping for the create modal (task 016): the title in `title`, a
// field's label in `hint`, its value in `text`, and its help line in
// `dimmed`, read per-cell off a real vt.Emulator grid.
func TestCreateViewLabelHintValueTextTitleDimmed(t *testing.T) {
	m := task016CreateTestModel(t)
	titleHex := tokenHex(t, m, theme.Title)
	hintHex := tokenHex(t, m, theme.Hint)
	textHex := tokenHex(t, m, theme.Text)
	dimmedHex := tokenHex(t, m, theme.Dimmed)

	view := m.createView()
	term := renderSettingsToEmulator(t, view, m.width, m.height)

	titleRow := findRowContaining(t, term, "Create shell session")
	titleCol := findCol(t, term, titleRow, "Create")
	if fg, ok := cellFgHex(t, term, titleCol, titleRow); !ok || fg != titleHex {
		t.Fatalf("title foreground = %q ok=%v, want title token %s", fg, ok, titleHex)
	}

	// The Agent row is never the focused field (field 0, Name, is) in
	// task016CreateTestModel, so it exercises the plain (non-selection)
	// label/value colouring.
	agentRow := findRowContaining(t, term, "Agent:")
	labelCol := findCol(t, term, agentRow, "Agent:")
	if fg, ok := cellFgHex(t, term, labelCol, agentRow); !ok || fg != hintHex {
		t.Fatalf("Agent label foreground = %q ok=%v, want hint token %s", fg, ok, hintHex)
	}
	valueCol := findCol(t, term, agentRow, "shell")
	if fg, ok := cellFgHex(t, term, valueCol, agentRow); !ok || fg != textHex {
		t.Fatalf("Agent value foreground = %q ok=%v, want text token %s", fg, ok, textHex)
	}

	helpRow := findRowContaining(t, term, "which coding agent adapter")
	helpCol := findCol(t, term, helpRow, "which")
	if fg, ok := cellFgHex(t, term, helpCol, helpRow); !ok || fg != dimmedHex {
		t.Fatalf("Agent help foreground = %q ok=%v, want dimmed token %s", fg, ok, dimmedHex)
	}
}

// TestCreateViewFocusedFieldGetsSelectionBackground proves the one
// remaining half of SPEC.md:1355's mapping: "the focused field carrying
// the same selection treatment a selected list row does". Field 0 (Name)
// is focused in task016CreateTestModel; field 2 (Agent) is not, and must
// carry no selection background at all.
func TestCreateViewFocusedFieldGetsSelectionBackground(t *testing.T) {
	m := task016CreateTestModel(t)
	selectionHex := tokenHex(t, m, theme.Selection)

	view := m.createView()
	term := renderSettingsToEmulator(t, view, m.width, m.height)

	nameRow := findRowContaining(t, term, "Name:")
	nameCol := findCol(t, term, nameRow, "Name:")
	bg, ok := cellBgHex(t, term, nameCol, nameRow)
	if !ok || bg != selectionHex {
		t.Fatalf("focused Name row background = %q ok=%v, want selection token %s", bg, ok, selectionHex)
	}

	agentRow := findRowContaining(t, term, "Agent:")
	agentCol := findCol(t, term, agentRow, "Agent:")
	if bg, ok := cellBgHex(t, term, agentCol, agentRow); ok && bg == selectionHex {
		t.Fatalf("unfocused Agent row carries the selection background %s -- selection must not leak past the focused field", bg)
	}
}

// TestCreateViewValidationMessageIsError proves the error half of
// SPEC.md:1355's mapping: a validation/collision message renders in
// `error`.
func TestCreateViewValidationMessageIsError(t *testing.T) {
	m := task016CreateTestModel(t)
	m.createError = "working directory is required"
	errorHex := tokenHex(t, m, theme.Error)

	view := m.createView()
	term := renderSettingsToEmulator(t, view, m.width, m.height)

	row := findRowContaining(t, term, "Cannot create session")
	col := findCol(t, term, row, "Cannot")
	if fg, ok := cellFgHex(t, term, col, row); !ok || fg != errorHex {
		t.Fatalf("validation message foreground = %q ok=%v, want error token %s", fg, ok, errorHex)
	}
}
