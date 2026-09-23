package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/theme"
)

// This file is task 018's own test obligation, the same shape
// create_view_theme_test.go (016) and env_editor_theme_test.go (017)
// already carry for their dialogs: the bulk `dd` confirm must render in
// SPEC.md:1355's §11.6 tokens at render time (never by baking escapes into
// the strings the scroll math measures) and must be bounded by
// framedDialogScrollable, so it fits deck's documented 80x24 minimum with
// its submit line reachable rather than clipped away.

// task018MarkName names marked session N deterministically, with no space
// in it, so no name can straddle a wrapDialogLines word-wrap boundary and
// make a substring assertion below flaky.
func task018MarkName(i int) string {
	return fmt.Sprintf("marked-%02d", i)
}

// task018BulkDeleteModel builds a colour-enabled Model sitting in the bulk
// `dd` confirm over `marks` marked sessions at 80x24 -- deck's documented
// minimum, the size every criterion here is stated against.
func task018BulkDeleteModel(t *testing.T, marks int) Model {
	t.Helper()
	m := New(nil, config.Settings{Color: true}, "")
	m.width, m.height = 80, 24
	m.marked = make(map[string]bool, marks)
	for i := 0; i < marks; i++ {
		id := fmt.Sprintf("s%02d", i)
		m.sessions = append(m.sessions, store.Session{
			ID:     id,
			Name:   task018MarkName(i),
			Agent:  "claude",
			CWD:    "/repo/deck",
			Status: "idle",
		})
		m.marked[id] = true
	}
	m.selected = rowCursor(0)
	m.deleteConfirming = true
	return m
}

// TestBulkDeleteConfirmBodyMeasurementsCarryNoSGRBytes is task 018's own
// plain-vs-styled split, mirroring the create modal's and the env
// editor's: bulkDeleteConfirmBody -- what wrapDialogLines/dialogMaxScroll/
// PgUp/PgDn measure -- must never carry an escape byte even with Color
// enabled, so a theme change can never move where a page boundary falls,
// while styledBulkDeleteConfirmBody must carry them, proving the split is
// real rather than nominal.
func TestBulkDeleteConfirmBodyMeasurementsCarryNoSGRBytes(t *testing.T) {
	m := task018BulkDeleteModel(t, 3)
	m.deleteNote = "Cannot delete marked-01: boom"

	plain := m.bulkDeleteConfirmBody()
	if strings.ContainsRune(plain, 0x1b) {
		t.Fatalf("bulkDeleteConfirmBody() (the plain body scroll math measures) contains an escape byte:\n%q", plain)
	}
	for i, line := range m.wrapDialogLines(plain) {
		if strings.ContainsRune(line, 0x1b) {
			t.Fatalf("wrapDialogLines(bulkDeleteConfirmBody())[%d] contains an escape byte: %q", i, line)
		}
	}
	if styled := m.styledBulkDeleteConfirmBody(); !strings.ContainsRune(styled, 0x1b) {
		t.Fatalf("styledBulkDeleteConfirmBody() carries no escape byte at all with Color enabled -- theming did not apply:\n%q", styled)
	}
}

// TestStyledBulkDeleteConfirmBodyMatchesPlainBodyLineForLine proves the
// styled body never adds or removes a physical line relative to
// wrapDialogLines(bulkDeleteConfirmBody()) and, once every escape sequence
// is stripped back out, is byte-identical to it -- the property that makes
// scrolling measured off the plain body correct for the coloured one.
func TestStyledBulkDeleteConfirmBodyMatchesPlainBodyLineForLine(t *testing.T) {
	withNote := task018BulkDeleteModel(t, 3)
	withNote.deleteNote = "Cannot delete marked-02: unavailable"

	for _, tc := range []struct {
		name string
		m    Model
	}{
		{"one mark", task018BulkDeleteModel(t, 1)},
		{"a few marks", task018BulkDeleteModel(t, 3)},
		{"enough marks to overflow the frame", task018BulkDeleteModel(t, 20)},
		{"with a note", withNote},
	} {
		plainBody := tc.m.bulkDeleteConfirmBody()
		if strings.ContainsRune(plainBody, 0x1b) {
			t.Fatalf("%s: bulkDeleteConfirmBody() contains an escape byte:\n%q", tc.name, plainBody)
		}
		plain := tc.m.wrapDialogLines(plainBody)
		styled := strings.Split(tc.m.styledBulkDeleteConfirmBody(), "\n")
		if len(styled) != len(plain) {
			t.Fatalf("%s: styled body has %d physical lines, wrapDialogLines(plain) has %d:\nplain: %q\nstyled: %q", tc.name, len(styled), len(plain), plain, styled)
		}
		for i := range plain {
			if got := stripANSI(styled[i]); got != plain[i] {
				t.Fatalf("%s: line %d differs once escapes are stripped:\n styled: %q\n  plain: %q", tc.name, i, got, plain[i])
			}
		}
	}
}

// TestBulkDeleteConfirmStaysWithinFrameBudgetAt80x24 proves the bounding
// half of task 018: framedDialogScrollable's own doc comment names "a bulk
// dd confirm over ~13+ marks" as a dialog that used to draw past an 80x24
// frame through the unbounded framedDialog path, so a 20-mark confirm must
// now clip to the frame -- and every rendered line must be exactly one
// dialog wide, never a short line the terminal would leave ragged.
func TestBulkDeleteConfirmStaysWithinFrameBudgetAt80x24(t *testing.T) {
	m := task018BulkDeleteModel(t, 20)
	if max := m.dialogMaxScroll(m.bulkDeleteConfirmBody()); max == 0 {
		t.Fatal("a 20-mark confirm body already fits the frame (maxScroll 0) -- this test needs an overflowing body to be non-vacuous")
	}
	view := m.deleteConfirmView()
	if n := countViewLines(view); n > 24 {
		t.Fatalf("bulk delete confirm is %d lines at 80x24, want <= 24:\n%s", n, view)
	}
	for i, line := range strings.Split(view, "\n") {
		if w := stringWidth(line); w != m.dialogWidth() {
			t.Fatalf("bulk delete confirm line %d width = %d, want dialogWidth() = %d: %q", i, w, m.dialogWidth(), line)
		}
	}
}

// TestBulkDeleteConfirmSubmitLineVisibleForAnOrdinaryMarkSet proves the
// submit line is on screen with no keystroke at all for the mark sets the
// keyboard-only PTY scenarios actually exercise (kill_delete_undo.feature
// marks two sessions and reads "Delete 2 marked sessions" straight off the
// grid), so bounding the dialog never cost those scenarios their submit
// legend.
func TestBulkDeleteConfirmSubmitLineVisibleForAnOrdinaryMarkSet(t *testing.T) {
	m := task018BulkDeleteModel(t, 2)
	// stripANSI first: the legend is coloured word by word, so the raw
	// view string carries an escape run between "Enter" and "deletes".
	// What a PTY step matches is the rendered grid, which is what
	// stripANSI reproduces here.
	view := stripANSI(m.deleteConfirmView())
	for _, want := range []string{"Delete 2 marked sessions", task018MarkName(0), task018MarkName(1), "Enter deletes all · Esc cancels"} {
		if !strings.Contains(view, want) {
			t.Fatalf("the confirm does not show %q with two marks and no scrolling:\n%s", want, view)
		}
	}
	if n := countViewLines(view); n > 24 {
		t.Fatalf("two-mark confirm is %d lines at 80x24, want <= 24:\n%s", n, view)
	}
}

// TestBulkDeleteConfirmSubmitLinePinnedWhileTheMarkListScrolls proves task
// 018's residual criterion: at 80x24 the submit line is visible WITH NO
// KEYSTROKE even for a mark set that overflows the frame -- only the list of
// marked names scrolls (PgUp/PgDn), the title and the legend are pinned --
// and no marked name is dropped: paging down reaches the last one.
func TestBulkDeleteConfirmSubmitLinePinnedWhileTheMarkListScrolls(t *testing.T) {
	m := task018BulkDeleteModel(t, 20)
	if !m.bulkDeleteConfirmScrolls() {
		t.Fatal("a 20-mark confirm already fits the frame -- this test needs an overflowing mark list to be non-vacuous")
	}

	first := stripANSI(m.deleteConfirmView())
	for _, want := range []string{"Delete 20 marked sessions", "Enter deletes all", "Esc cancels", "PgUp/PgDn scrolls", task018MarkName(0)} {
		if !strings.Contains(first, want) {
			t.Fatalf("the first page of a 20-mark confirm does not show %q with no keystroke:\n%s", want, first)
		}
	}
	if strings.Contains(first, task018MarkName(19)) {
		t.Fatalf("the first page already shows the last marked name -- nothing is scrolling:\n%s", first)
	}
	if n := countViewLines(first); n > 24 {
		t.Fatalf("20-mark confirm is %d lines at 80x24, want <= 24:\n%s", n, first)
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	down := updated.(Model)
	if down.deleteScroll == 0 {
		t.Fatal("PgDn in the bulk delete confirm left deleteScroll at 0")
	}
	if !down.deleteConfirming {
		t.Fatal("PgDn closed the bulk delete confirm")
	}
	bottom := stripANSI(down.deleteConfirmView())
	for _, want := range []string{task018MarkName(19), "Delete 20 marked sessions", "Enter deletes all"} {
		if !strings.Contains(bottom, want) {
			t.Fatalf("after PgDn the confirm does not show %q (the head and tail are pinned, the list scrolls):\n%s", want, bottom)
		}
	}
	if n := countViewLines(bottom); n > 24 {
		t.Fatalf("scrolled 20-mark confirm is %d lines at 80x24, want <= 24:\n%s", n, bottom)
	}

	back, _ := down.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	up := back.(Model)
	if up.deleteScroll != 0 {
		t.Fatalf("deleteScroll = %d after paging back up, want 0", up.deleteScroll)
	}
	top := stripANSI(up.deleteConfirmView())
	if !strings.Contains(top, task018MarkName(0)) || !strings.Contains(top, "Enter deletes all") {
		t.Fatalf("paging back up did not return to the top of the mark list with the legend still pinned:\n%s", top)
	}
}

// TestBulkDeleteConfirmBodyKeepsEveryMarkedName proves the pinning above is
// a WINDOW, not a truncation: the plain body -- what an assertion or a copy
// of the dialog's text reads -- still names all 20 marked sessions, and the
// scroll keys are advertised on the submit line whenever some of them are
// off screen (SPEC requirement 39: paginate, never silently drop).
func TestBulkDeleteConfirmBodyKeepsEveryMarkedName(t *testing.T) {
	m := task018BulkDeleteModel(t, 20)
	body := m.bulkDeleteConfirmBody()
	for i := 0; i < 20; i++ {
		if !strings.Contains(body, task018MarkName(i)) {
			t.Fatalf("bulkDeleteConfirmBody() dropped marked name %q:\n%s", task018MarkName(i), body)
		}
	}
	if !strings.Contains(body, "PgUp/PgDn scrolls") {
		t.Fatalf("an overflowing confirm does not advertise the scroll keys:\n%s", body)
	}

	small := task018BulkDeleteModel(t, 2)
	if small.bulkDeleteConfirmScrolls() {
		t.Fatal("a two-mark confirm reports itself as scrolling at 80x24")
	}
	if got := small.bulkDeleteConfirmBody(); strings.Contains(got, "PgUp/PgDn") {
		t.Fatalf("a confirm that fits advertises scroll keys it does not need:\n%s", got)
	}
}

// TestBulkDeleteConfirmReopenStartsUnscrolled proves a reopened confirm
// never starts scrolled from wherever a previous visit left it: the `dd`
// chord on a marked set resets deleteScroll to 0.
func TestBulkDeleteConfirmReopenStartsUnscrolled(t *testing.T) {
	m := task018BulkDeleteModel(t, 20)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	scrolled := updated.(Model)
	if scrolled.deleteScroll == 0 {
		t.Fatal("PgDn left deleteScroll at 0 -- nothing to prove a reset against")
	}

	closed := scrolled
	closed.deleteConfirming = false
	firstD, _ := closed.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	pending := firstD.(Model)
	if !pending.pendingDelete {
		t.Fatal("a lone `d` did not arm the dd chord")
	}
	reopened, _ := pending.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	again := reopened.(Model)
	if !again.deleteConfirming {
		t.Fatal("the `dd` chord on a marked set did not reopen the confirm")
	}
	if again.deleteScroll != 0 {
		t.Fatalf("reopened confirm starts at deleteScroll = %d, want 0", again.deleteScroll)
	}
}

// TestBulkDeleteConfirmTokensMatchSpec proves SPEC.md:1355's token mapping
// for this dialog, read per-cell off a real vt.Emulator grid rather than
// from the view string: the title in `title`, the explanatory
// survives-text in `dimmed`, each marked session's own name in `text`, the
// legend's bound keys in `key` with the surrounding prose in `hint`.
func TestBulkDeleteConfirmTokensMatchSpec(t *testing.T) {
	m := task018BulkDeleteModel(t, 3)
	titleHex := tokenHex(t, m, theme.Title)
	dimmedHex := tokenHex(t, m, theme.Dimmed)
	textHex := tokenHex(t, m, theme.Text)
	keyHex := tokenHex(t, m, theme.Key)
	hintHex := tokenHex(t, m, theme.Hint)

	view := m.deleteConfirmView()
	term := renderSettingsToEmulator(t, view, m.width, m.height)

	titleRow := findRowContaining(t, term, "Delete 3 marked sessions")
	titleCol := findCol(t, term, titleRow, "Delete")
	if fg, ok := cellFgHex(t, term, titleCol, titleRow); !ok || fg != titleHex {
		t.Fatalf("title foreground = %q ok=%v, want title token %s", fg, ok, titleHex)
	}

	dimRow := findRowContaining(t, term, "This kills each live pane")
	dimCol := findCol(t, term, dimRow, "This")
	if fg, ok := cellFgHex(t, term, dimCol, dimRow); !ok || fg != dimmedHex {
		t.Fatalf("explanation foreground = %q ok=%v, want dimmed token %s", fg, ok, dimmedHex)
	}

	nameRow := findRowContaining(t, term, task018MarkName(1))
	nameCol := findCol(t, term, nameRow, task018MarkName(1))
	if fg, ok := cellFgHex(t, term, nameCol, nameRow); !ok || fg != textHex {
		t.Fatalf("marked session name foreground = %q ok=%v, want text token %s", fg, ok, textHex)
	}

	legendRow := findRowContaining(t, term, "Enter deletes all")
	enterCol := findCol(t, term, legendRow, "Enter")
	if fg, ok := cellFgHex(t, term, enterCol, legendRow); !ok || fg != keyHex {
		t.Fatalf("legend Enter foreground = %q ok=%v, want key token %s", fg, ok, keyHex)
	}
	if keyHex == hintHex {
		t.Skip("this theme's key and hint tokens share a colour; the prose-vs-key distinctness assertion below would be vacuous")
	}
	proseCol := findCol(t, term, legendRow, "deletes")
	if fg, ok := cellFgHex(t, term, proseCol, legendRow); !ok || fg != hintHex {
		t.Fatalf("legend prose foreground = %q ok=%v, want hint token %s", fg, ok, hintHex)
	}
}

// TestBulkDeleteConfirmNoteIsError proves deleteNote -- always an
// in-dialog failure reason -- renders in SPEC.md:1355's `error` token,
// matching every other §11.4 dialog's validation message.
func TestBulkDeleteConfirmNoteIsError(t *testing.T) {
	m := task018BulkDeleteModel(t, 2)
	m.deleteNote = "Cannot delete marked-00: unavailable"
	errorHex := tokenHex(t, m, theme.Error)

	view := m.deleteConfirmView()
	term := renderSettingsToEmulator(t, view, m.width, m.height)

	row := findRowContaining(t, term, "Cannot delete marked-00")
	col := findCol(t, term, row, "Cannot")
	if fg, ok := cellFgHex(t, term, col, row); !ok || fg != errorHex {
		t.Fatalf("deleteNote foreground = %q ok=%v, want error token %s", fg, ok, errorHex)
	}
}
