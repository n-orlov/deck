package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// TestEnterWithoutATmuxClientDoesNotEnterInteractiveMode proves task 061's
// degrade path (WithTmuxClient never called, exactly like every
// pre-task-061 constructor and unit test): Enter is a no-op, not a real
// tmux invocation nobody asked for, mirroring how attachSelected already
// degrades for a nil m.attach.
func TestEnterWithoutATmuxClientDoesNotEnterInteractiveMode(t *testing.T) {
	m := New(nil, config.Settings{}, "")
	m.sessions = []store.Session{{ID: "s1", Name: "one", Agent: "shell", Status: "running", Slug: "one"}}
	m.selected = rowCursor(0)
	next, cmd := m.enterInteractive()
	got := next.(Model)
	if got.interactive {
		t.Fatalf("enterInteractive entered interactive mode with a zero tmux.Client")
	}
	if cmd != nil {
		t.Fatalf("enterInteractive returned a non-nil cmd with a zero tmux.Client")
	}
	if got.attachError != "" {
		t.Fatalf("enterInteractive with a zero tmux.Client set attachError %q, want none (mirrors attachSelected's nil-m.attach silence)", got.attachError)
	}
}

// TestEnterKeyRoutesToEnterInteractiveNotAttachSelected proves the actual
// keymap rebind (PRD II-41): pressing Enter in list mode no longer calls
// attachSelected (which requires m.attach, unset here) -- it is a no-op
// exactly like enterInteractive's own degrade path, not a crash or a
// fallback to the old behaviour.
func TestEnterKeyRoutesToEnterInteractiveNotAttachSelected(t *testing.T) {
	m := New(nil, config.Settings{}, "")
	m.sessions = []store.Session{{ID: "s1", Name: "one", Agent: "shell", Status: "running", Slug: "one"}}
	m.selected = rowCursor(0)
	next, cmd := m.Update(tea.KeyMsg(tea.Key{Type: tea.KeyEnter}))
	got := next.(Model)
	if got.interactive {
		t.Fatalf("Enter entered interactive mode with a zero tmux.Client")
	}
	if cmd != nil {
		t.Fatalf("Enter returned a non-nil cmd; attachSelected would have (m.attach is unset, so this proves Enter did not call it)")
	}
}

// TestCtrlQExitsInteractiveModeAndClearsEveryField proves exitInteractive's
// own contract in isolation from any real tmux call: with no window
// target/ownership/grid claimed (the shape enterInteractive leaves fields
// in in this scenario -- a hand-built one purely to exercise
// updateInteractive/exitInteractive without spawning tmux), Ctrl+Q still
// flips m.interactive off and resets every interactive* field to its zero
// value, so a later Enter cannot observe stale state from a previous
// session.
func TestCtrlQExitsInteractiveModeAndClearsEveryField(t *testing.T) {
	m := New(nil, config.Settings{}, "")
	m.interactive = true
	next, cmd := m.updateInteractive(tea.KeyMsg(tea.Key{Type: tea.KeyCtrlQ}))
	got := next.(Model)
	if got.interactive {
		t.Fatalf("ctrl+q did not leave interactive mode")
	}
	if cmd != nil {
		t.Fatalf("ctrl+q returned a non-nil cmd, want nil")
	}
	if got.interactiveWindowTarget != "" || got.interactiveOwnership != nil || got.interactiveGrid != nil || got.interactiveDispatcher != nil {
		t.Fatalf("ctrl+q left interactive state behind: %+v", got)
	}
}

// TestUpdateInteractiveWithoutADispatcherIsANoOp proves updateInteractive
// never panics on a nil interactiveDispatcher (the shape between
// enterInteractive succeeding up to claiming ownership but failing to
// build a Dispatcher, and the shape of any hand-built test Model): every
// non-ctrl+q key is silently dropped rather than forwarded to nothing.
func TestUpdateInteractiveWithoutADispatcherIsANoOp(t *testing.T) {
	m := New(nil, config.Settings{}, "")
	m.interactive = true
	next, cmd := m.updateInteractive(tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune("x")}))
	got := next.(Model)
	if !got.interactive {
		t.Fatalf("a non-ctrl+q key left interactive mode")
	}
	if cmd != nil {
		t.Fatalf("updateInteractive returned a non-nil cmd, want nil")
	}
}

// TestInteractiveFooterAdvertisesTheExitChordAndForwarding is PRD Part II
// requirement 43's other half: interactive mode's own footer line is
// DIFFERENT from list mode's (footerKeyLegend, still exercised by
// TestEmptyAndHelpViewsAreDiscoverable and the golden frame elsewhere) --
// it states that keystrokes forward rather than list one of list mode's
// now-inert bindings, and names Ctrl+Q as the one bound exit chord.
func TestInteractiveFooterAdvertisesTheExitChordAndForwarding(t *testing.T) {
	m := New(nil, config.Settings{}, "")
	m.interactive = true
	footer := m.footerLine()
	if !strings.Contains(footer, "Ctrl+Q") {
		t.Fatalf("interactive footer %q does not name the Ctrl+Q exit chord", footer)
	}
	if !strings.Contains(footer, "forward") {
		t.Fatalf("interactive footer %q does not state that keystrokes forward", footer)
	}
	for _, listOnly := range []string{"acknowledge", "attach", " a ", "resume", "relaunch"} {
		if strings.Contains(footer, listOnly) {
			t.Fatalf("interactive footer %q advertises list mode's %q, which interactive mode does not bind", footer, listOnly)
		}
	}
}

// TestPreviewTitleNamesInteractiveModeAndTheExitChord proves the preview
// panel's own border title (previewTitle) changes the same way while
// m.interactive is true, matching the footer's own statement of how to
// leave rather than contradicting it.
func TestPreviewTitleNamesInteractiveModeAndTheExitChord(t *testing.T) {
	m := New(nil, config.Settings{}, "")
	if got := m.previewTitle(); got != "" {
		t.Fatalf("list mode's previewTitle is %q, want empty", got)
	}
	m.interactive = true
	got := m.previewTitle()
	if !strings.Contains(got, "interactive") || !strings.Contains(got, "Ctrl+Q") {
		t.Fatalf("interactive previewTitle %q does not name interactive mode and Ctrl+Q", got)
	}
}

// TestPreviewTitleNamesTheTargetSessionWhileInteractive is task 063/II-44's
// own requirement: because NO_COLOR drops deck to monochrome and its own
// golden frames are captured that way, a colour-only focus indicator would
// pass whether or not focus actually moved, so the preview's top border
// must carry the TARGET SESSION'S NAME as plain screen text while
// interactive -- not just say "interactive" generically.
func TestPreviewTitleNamesTheTargetSessionWhileInteractive(t *testing.T) {
	m := New(nil, config.Settings{}, "")
	m.sessions = []store.Session{{ID: "s1", Name: "focus-target", Agent: "shell", Status: "running", Slug: "focus-target"}}
	m.selected = rowCursor(0)
	m.interactive = true
	got := m.previewTitle()
	if !strings.Contains(got, "focus-target") {
		t.Fatalf("interactive previewTitle %q does not name the target session", got)
	}
	if !strings.Contains(got, "interactive") || !strings.Contains(got, "Ctrl+Q") {
		t.Fatalf("interactive previewTitle %q dropped the mode name or exit chord while adding the session name", got)
	}
}

// TestPreviewTitleStatesFittedGeometryDifferentlyFromACrop is PRD Part II
// requirement 46: interactive mode's enterInteractive fits the tmux window
// to exactly the panel's own content box (previewContentSize), so the real
// pane size and the panel's content size are always the same number --
// stating that in cropPreviewBottomLeft's own "WxH of realWxrealH" form
// would degenerate to the misleading "45x22 of 45x22" (a crop statement
// about a pane that was never cropped). previewTitle must state it
// differently: "WxH fitted", never containing " of " and never the
// degenerate doubled form.
func TestPreviewTitleStatesFittedGeometryDifferentlyFromACrop(t *testing.T) {
	m := New(nil, config.Settings{}, "")
	m.width, m.height = 100, 30
	m.sessions = []store.Session{{ID: "s1", Name: "focus-target", Agent: "shell", Status: "running", Slug: "focus-target"}}
	m.selected = rowCursor(0)
	m.interactive = true
	got := m.previewTitle()
	width, height := m.previewContentSize()
	wantGeom := fmt.Sprintf("%dx%d fitted", width, height)
	if !strings.Contains(got, wantGeom) {
		t.Fatalf("interactive previewTitle %q does not contain the fitted geometry %q", got, wantGeom)
	}
	if strings.Contains(got, " of ") {
		t.Fatalf("interactive previewTitle %q states fitted geometry in the crop's \"of\" form", got)
	}
	degenerate := fmt.Sprintf("%dx%d of %dx%d", width, height, width, height)
	if strings.Contains(got, degenerate) {
		t.Fatalf("interactive previewTitle %q contains the degenerate crop form %q", got, degenerate)
	}
}

// TestInteractiveGridIsBlankStripsANSIAndDetectsVisibleText proves
// interactiveGridIsBlank's own two claims directly (PRD II-49, task 067):
// escape-only rows (an SGR run with no visible glyph -- e.g. a coloured
// background left behind by a cursor cell) still read as blank, and a
// row carrying real text -- plain or bracketed by real SGR escapes, not a
// fixture that happens to already be plain -- reads as non-blank. The
// escape-stripping is confirmed non-vacuous by asserting the styled-word
// case would misreport blank if the escapes were left in (a literal ESC
// byte is never whitespace, so strings.TrimSpace alone cannot already be
// what makes this pass).
func TestInteractiveGridIsBlankStripsANSIAndDetectsVisibleText(t *testing.T) {
	blank := []string{"", "   ", "\x1b[48;5;236m\x1b[0m", "\x1b[38;5;7m   \x1b[0m"}
	if !interactiveGridIsBlank(blank) {
		t.Fatalf("interactiveGridIsBlank(%q) = false, want true (every row is empty or escape-only)", blank)
	}

	plainWord := []string{"", "hello", ""}
	if interactiveGridIsBlank(plainWord) {
		t.Fatalf("interactiveGridIsBlank(%q) = true, want false (a plain visible word is present)", plainWord)
	}

	styledWord := []string{"", "\x1b[1mrepaint #1\x1b[0m", ""}
	if interactiveGridIsBlank(styledWord) {
		t.Fatalf("interactiveGridIsBlank(%q) = true, want false (an SGR-bracketed visible word is present)", styledWord)
	}
	// Non-vacuousness: without stripping the escapes first, the raw row
	// still contains non-whitespace bytes (the ESC/CSI bytes themselves),
	// so a version of this check that forgot to strip would ALSO report
	// non-blank here -- the real proof that stripping matters is the
	// opposite case above: an escape-only row containing nothing BUT
	// escapes and whitespace must read as blank, which it would not if
	// the ESC bytes were left in (they are not whitespace either).
	rawEscapeOnly := blank[2]
	if strings.TrimSpace(rawEscapeOnly) == "" {
		t.Fatalf("escape-only fixture %q is already whitespace-only unstripped; the fixture must contain a real escape sequence to prove stripping matters", rawEscapeOnly)
	}
}

// TestInteractiveBodyLinesAnnouncesNotRepaintedWhileGridIsBlank and its
// sibling below prove interactiveBodyLines' own contract (PRD II-49) at
// the fitInteractiveBodyLines boundary, independent of a real
// interactive.Session (which requires a live tmux pane to construct):
// given the exact lines a real Grid().Render() would hand back, a wholly
// blank grid gets the announcement prepended as line 0, and a grid with
// real content does not.
func TestInteractiveBodyLinesPrependsAnnouncementOnlyWhenGridIsBlank(t *testing.T) {
	blankRendered := []string{"", "", "", ""}
	got, _ := fitInteractiveBodyLines(blankRendered, 4, interactiveGridIsBlank(blankRendered))
	if got[0] != interactiveNotRepaintedNotice {
		t.Fatalf("blank grid: line 0 = %q, want the not-repainted notice %q", got[0], interactiveNotRepaintedNotice)
	}

	liveRendered := []string{"repaint #1", "", "", ""}
	got, _ = fitInteractiveBodyLines(liveRendered, 4, interactiveGridIsBlank(liveRendered))
	for _, line := range got {
		if strings.Contains(line, interactiveNotRepaintedNotice) {
			t.Fatalf("live grid %v unexpectedly carries the not-repainted notice", got)
		}
	}
	if got[0] != "repaint #1" {
		t.Fatalf("live grid: line 0 = %q, want the real content unshifted", got[0])
	}
}

// TestFitInteractiveBodyLinesOwnership is this task's (002/B1) own
// red-first proof of fitInteractiveBodyLines' per-row provenance,
// independent of a real *interactive.Session for the same reason the test
// above is: interactiveBodyLines itself calls m.interactiveGrid.RenderRows,
// which requires a live tmux pane to construct at all, but the
// notice-then-pad/truncate sequence this task moved out to
// fitInteractiveBodyLines takes plain lines and needs none of that.
//
// Two cases, matching the task's own two named scenarios:
//   - the blank-grid notice case: a wholly blank grid (contentHeight rows,
//     as RenderRows always returns) gets interactiveNotRepaintedNotice
//     prepended as a deck-owned line 0, pushing the slice one row over
//     contentHeight -- fitInteractiveBodyLines' truncation branch then
//     drops the LAST row, which is one of the grid's own (foreign) blank
//     rows, never the notice itself.
//   - the short-grid pad case: a grid slice shorter than contentHeight
//     (never produced by the real RenderRows today, but the provenance
//     must still be correct symmetrically with cropPreviewBottomLeft's own
//     vertical blank-fill) gets deck-owned pad rows appended to reach
//     contentHeight, while every real grid row already present stays
//     foreign.
func TestFitInteractiveBodyLinesOwnership(t *testing.T) {
	t.Run("blank-grid notice case", func(t *testing.T) {
		blankRendered := []string{"", "", "", ""}
		lines, owners := fitInteractiveBodyLines(blankRendered, 4, true)
		if len(lines) != 4 || len(owners) != 4 {
			t.Fatalf("len(lines)=%d len(owners)=%d, want 4/4", len(lines), len(owners))
		}
		if lines[0] != interactiveNotRepaintedNotice {
			t.Fatalf("lines[0] = %q, want the not-repainted notice %q", lines[0], interactiveNotRepaintedNotice)
		}
		if owners[0] != previewLineDeckOwned {
			t.Fatalf("owners[0] (notice line) = %v, want previewLineDeckOwned", owners[0])
		}
		for i := 1; i < 4; i++ {
			if owners[i] != previewLineForeign {
				t.Fatalf("owners[%d] (grid row) = %v, want previewLineForeign", i, owners[i])
			}
		}
	})

	t.Run("short-grid pad case", func(t *testing.T) {
		shortGrid := []string{"repaint #1", "repaint #2"}
		lines, owners := fitInteractiveBodyLines(shortGrid, 5, false)
		if len(lines) != 5 || len(owners) != 5 {
			t.Fatalf("len(lines)=%d len(owners)=%d, want 5/5", len(lines), len(owners))
		}
		for i := 0; i < 2; i++ {
			if lines[i] != shortGrid[i] {
				t.Fatalf("lines[%d] = %q, want the grid's own %q unshifted", i, lines[i], shortGrid[i])
			}
			if owners[i] != previewLineForeign {
				t.Fatalf("owners[%d] (real grid row) = %v, want previewLineForeign", i, owners[i])
			}
		}
		for i := 2; i < 5; i++ {
			if lines[i] != "" {
				t.Fatalf("lines[%d] = %q, want a blank pad row", i, lines[i])
			}
			if owners[i] != previewLineDeckOwned {
				t.Fatalf("owners[%d] (synthesized pad row) = %v, want previewLineDeckOwned", i, owners[i])
			}
		}
	})
}

// TestInteractiveNamedKeyMapsOnlyModeDependentKeys proves the named-key/
// literal-byte split (task 061's own design note): arrows, Home/End,
// PgUp/PgDown, Insert/Delete, ShiftTab and the function keys go by NAME
// through tmux's own translation (internal/tmux/key.go's
// namedKeyAllowlist), because their byte encoding depends on the pane's
// terminal mode; nothing else does.
//
// The Alt-modified forms of these are TestInteractiveNamedKeyForwardsEveryAltModifiedSpecialKey's
// subject below (issue #28). This test used to close by asserting the
// opposite -- that Alt+Up resolved to nothing at all -- which is the
// assertion that encoded the bug; its replacement at the bottom of this
// function now pins the fixed behaviour instead.
func TestInteractiveNamedKeyMapsOnlyModeDependentKeys(t *testing.T) {
	cases := []struct {
		msg  tea.KeyMsg
		want string
	}{
		{tea.KeyMsg(tea.Key{Type: tea.KeyUp}), "Up"},
		{tea.KeyMsg(tea.Key{Type: tea.KeyDown}), "Down"},
		{tea.KeyMsg(tea.Key{Type: tea.KeyLeft}), "Left"},
		{tea.KeyMsg(tea.Key{Type: tea.KeyRight}), "Right"},
		{tea.KeyMsg(tea.Key{Type: tea.KeyHome}), "Home"},
		{tea.KeyMsg(tea.Key{Type: tea.KeyEnd}), "End"},
		{tea.KeyMsg(tea.Key{Type: tea.KeyPgUp}), "PageUp"},
		{tea.KeyMsg(tea.Key{Type: tea.KeyPgDown}), "PageDown"},
		{tea.KeyMsg(tea.Key{Type: tea.KeyDelete}), "Delete"},
		{tea.KeyMsg(tea.Key{Type: tea.KeyInsert}), "Insert"},
		{tea.KeyMsg(tea.Key{Type: tea.KeyShiftTab}), "BTab"},
		{tea.KeyMsg(tea.Key{Type: tea.KeyF1}), "F1"},
		{tea.KeyMsg(tea.Key{Type: tea.KeyF12}), "F12"},
		// Modified navigation (steer 017 item 1 / SPEC.md §11.9): each of
		// these is bubbletea's OWN distinct KeyType, never KeyLeft/KeyUp/...
		// with a modifier flag set, so a switch that only matched the bare
		// types (as this one did before) drops every one of these silently
		// -- see internal/tmux/key_test.go for the real-tmux half proving
		// the delivered bytes match.
		{tea.KeyMsg(tea.Key{Type: tea.KeyCtrlUp}), "C-Up"},
		{tea.KeyMsg(tea.Key{Type: tea.KeyCtrlDown}), "C-Down"},
		{tea.KeyMsg(tea.Key{Type: tea.KeyCtrlLeft}), "C-Left"},
		{tea.KeyMsg(tea.Key{Type: tea.KeyCtrlRight}), "C-Right"},
		{tea.KeyMsg(tea.Key{Type: tea.KeyCtrlHome}), "C-Home"},
		{tea.KeyMsg(tea.Key{Type: tea.KeyCtrlEnd}), "C-End"},
		{tea.KeyMsg(tea.Key{Type: tea.KeyCtrlPgUp}), "C-PgUp"},
		{tea.KeyMsg(tea.Key{Type: tea.KeyCtrlPgDown}), "C-PgDn"},
		{tea.KeyMsg(tea.Key{Type: tea.KeyShiftUp}), "S-Up"},
		{tea.KeyMsg(tea.Key{Type: tea.KeyShiftDown}), "S-Down"},
		{tea.KeyMsg(tea.Key{Type: tea.KeyShiftLeft}), "S-Left"},
		{tea.KeyMsg(tea.Key{Type: tea.KeyShiftRight}), "S-Right"},
		{tea.KeyMsg(tea.Key{Type: tea.KeyShiftHome}), "S-Home"},
		{tea.KeyMsg(tea.Key{Type: tea.KeyShiftEnd}), "S-End"},
		{tea.KeyMsg(tea.Key{Type: tea.KeyCtrlShiftUp}), "C-S-Up"},
		{tea.KeyMsg(tea.Key{Type: tea.KeyCtrlShiftDown}), "C-S-Down"},
		{tea.KeyMsg(tea.Key{Type: tea.KeyCtrlShiftLeft}), "C-S-Left"},
		{tea.KeyMsg(tea.Key{Type: tea.KeyCtrlShiftRight}), "C-S-Right"},
		{tea.KeyMsg(tea.Key{Type: tea.KeyCtrlShiftHome}), "C-S-Home"},
		{tea.KeyMsg(tea.Key{Type: tea.KeyCtrlShiftEnd}), "C-S-End"},
	}
	for _, c := range cases {
		got, ok := interactiveNamedKey(c.msg)
		if !ok || got != c.want {
			t.Errorf("interactiveNamedKey(%v) = (%q, %v), want (%q, true)", c.msg.Type, got, ok, c.want)
		}
		// The exact hazard steer 017 item 1 warns about: an unlisted or
		// misspelled name is typed into the agent as literal text, exit
		// 0, no error (key_test.go's own
		// TestSendNamedKeyRejectsAnUnknownNameWithoutSpawningTmux is the
		// other half). So every name this function can ever produce must
		// already be on internal/tmux's compiled-in allowlist -- checked
		// here, not assumed, so a future name added to one map and not
		// the other fails a test instead of shipping.
		if !tmux.IsNamedKeyAllowed(c.want) {
			t.Errorf("interactiveNamedKey(%v) returned %q, which is NOT on internal/tmux's namedKeyAllowlist -- an unlisted name is typed into the agent as literal text by tmux, not refused", c.msg.Type, c.want)
		}
	}
	// A fixed-byte key (Enter/Tab/Escape/Backspace/runes) is NOT a named
	// key -- interactiveLiteralPayload owns it instead.
	for _, msg := range []tea.KeyMsg{
		tea.KeyMsg(tea.Key{Type: tea.KeyEnter}),
		tea.KeyMsg(tea.Key{Type: tea.KeyTab}),
		tea.KeyMsg(tea.Key{Type: tea.KeyEscape}),
		tea.KeyMsg(tea.Key{Type: tea.KeyBackspace}),
		tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune("x")}),
	} {
		if _, ok := interactiveNamedKey(msg); ok {
			t.Errorf("interactiveNamedKey(%v) claimed a fixed-byte key", msg.Type)
		}
	}
	// Issue #28: an Alt-modified named key resolves to the M--prefixed
	// tmux name, NEVER to the bare one (which would forward Up for a
	// physical Alt+Up, dropping the modifier) and never to nothing at all
	// (which is the silent drop issue #28 reports). This assertion is the
	// inversion of the one that used to stand here.
	altUp := tea.KeyMsg(tea.Key{Type: tea.KeyUp, Alt: true})
	got, ok := interactiveNamedKey(altUp)
	if !ok || got != "M-Up" {
		t.Errorf("interactiveNamedKey(alt+up) = (%q, %v), want (\"M-Up\", true) -- issue #28: the blanket Alt refusal dropped the keystroke with no bytes written and no error", got, ok)
	}
}

// TestInteractiveNamedKeyForwardsEveryAltModifiedSpecialKey is issue #28's
// regression pin: the interactive preview used to drop Alt plus every
// SPECIAL key (Alt+Up/Down/Left/Right, Alt+Home/End/PgUp/PgDn/Delete,
// Alt+F1-F12 and every Ctrl/Shift/Ctrl+Shift combination of those with Alt
// added) silently -- no bytes, no error, no beep -- while Alt+letter kept
// working, which is why it read as intermittent in live use. SPEC.md §11.9
// names `Alt` alongside `Ctrl` and `Shift` ("Modified navigation keys
// forward, like the unmodified ones, by tmux key name") and forbids
// exactly this failure ("a keystroke that does nothing and reports
// nothing"), so the drop was a SPEC violation rather than a documented
// gap.
//
// The root cause was that both halves of the forwarding split refused
// these keys: interactiveNamedKey opened with a blanket
// `if msg.Alt { return "", false }`, and interactiveLiteralPayload cannot
// reach them either because every special key's tea.KeyType is NEGATIVE
// (charmbracelet/bubbletea@v1.3.10/key.go:205,
// `KeyRunes KeyType = -(iota + 1)`), so tea.KeyUp fails that function's
// `msg.Type >= 0 && msg.Type <= 31` guard and lands in its default
// branch. This test therefore asserts BOTH halves for every key: the
// named half returns the surveyed M- name, and the literal half still
// declines it (there is no overlap between the two, and a key that both
// declined is precisely the bug).
//
// The want names are the ones internal/tui/interactive.go's
// interactiveAltNamedKeys comment records the tmux 3.6b survey for;
// internal/tmux/key_test.go's
// TestSendNamedKeyDeliversAltModifiedNavigationKeysByTmuxsOwnTranslation
// is the real-tmux half that proves each of them delivers the CSI
// sequence bubbletea's own `sequences` table decodes straight back into
// the {KeyType, Alt: true} listed here.
func TestInteractiveNamedKeyForwardsEveryAltModifiedSpecialKey(t *testing.T) {
	cases := []struct {
		keyType tea.KeyType
		want    string
	}{
		{tea.KeyUp, "M-Up"},
		{tea.KeyDown, "M-Down"},
		{tea.KeyLeft, "M-Left"},
		{tea.KeyRight, "M-Right"},
		{tea.KeyHome, "M-Home"},
		{tea.KeyEnd, "M-End"},
		{tea.KeyPgUp, "M-PageUp"},
		{tea.KeyPgDown, "M-PageDown"},
		{tea.KeyDelete, "M-Delete"},
		{tea.KeyCtrlUp, "C-M-Up"},
		{tea.KeyCtrlDown, "C-M-Down"},
		{tea.KeyCtrlLeft, "C-M-Left"},
		{tea.KeyCtrlRight, "C-M-Right"},
		{tea.KeyCtrlHome, "C-M-Home"},
		{tea.KeyCtrlEnd, "C-M-End"},
		{tea.KeyCtrlPgUp, "C-M-PgUp"},
		{tea.KeyCtrlPgDown, "C-M-PgDn"},
		{tea.KeyShiftUp, "S-M-Up"},
		{tea.KeyShiftDown, "S-M-Down"},
		{tea.KeyShiftLeft, "S-M-Left"},
		{tea.KeyShiftRight, "S-M-Right"},
		{tea.KeyShiftHome, "S-M-Home"},
		{tea.KeyShiftEnd, "S-M-End"},
		{tea.KeyCtrlShiftUp, "C-M-S-Up"},
		{tea.KeyCtrlShiftDown, "C-M-S-Down"},
		{tea.KeyCtrlShiftLeft, "C-M-S-Left"},
		{tea.KeyCtrlShiftRight, "C-M-S-Right"},
		{tea.KeyCtrlShiftHome, "C-M-S-Home"},
		{tea.KeyCtrlShiftEnd, "C-M-S-End"},
		{tea.KeyF1, "M-F1"},
		{tea.KeyF2, "M-F2"},
		{tea.KeyF3, "M-F3"},
		{tea.KeyF4, "M-F4"},
		{tea.KeyF5, "M-F5"},
		{tea.KeyF6, "M-F6"},
		{tea.KeyF7, "M-F7"},
		{tea.KeyF8, "M-F8"},
		{tea.KeyF9, "M-F9"},
		{tea.KeyF10, "M-F10"},
		{tea.KeyF11, "M-F11"},
		{tea.KeyF12, "M-F12"},
	}
	// Counted against the map rather than hard-coded, so a name added to
	// interactiveAltNamedKeys without a case here fails as an incomplete
	// table instead of going untested (the gaps, whose value is "", are
	// excluded -- TestEveryBareNamedKeyHasAnExplicitAltVerdict owns those).
	forwardable := 0
	for _, altName := range interactiveAltNamedKeys {
		if altName != "" {
			forwardable++
		}
	}
	if len(cases) != forwardable {
		t.Fatalf("table has %d cases but interactiveAltNamedKeys forwards %d names -- keep the two in step", len(cases), forwardable)
	}
	for _, c := range cases {
		msg := tea.KeyMsg(tea.Key{Type: c.keyType, Alt: true})
		got, ok := interactiveNamedKey(msg)
		if !ok || got != c.want {
			t.Errorf("interactiveNamedKey(alt+%v) = (%q, %v), want (%q, true) -- issue #28's silent drop", c.keyType, got, ok, c.want)
			continue
		}
		// The same hazard the unmodified half already checks for (steer
		// 017 item 1): a name that is not on internal/tmux's compiled-in
		// allowlist is typed into the agent as literal text by tmux, exit
		// 0, no error. Every new M- name has to be on it, checked here
		// rather than assumed, so adding one to this package and not to
		// internal/tmux fails a test instead of shipping.
		if !tmux.IsNamedKeyAllowed(c.want) {
			t.Errorf("interactiveNamedKey(alt+%v) returned %q, which is NOT on internal/tmux's namedKeyAllowlist -- an unlisted name is typed into the agent as literal text by tmux, not refused", c.keyType, c.want)
		}
		// The other half of the split stays closed for these: they are
		// mode-dependent, so deck must never hand-encode them as literal
		// bytes. (Before issue #28's fix BOTH halves refused them, which
		// is what made the drop silent -- this assertion on its own is
		// therefore not enough, it only holds meaning next to the one
		// above.)
		if payload, ok := interactiveLiteralPayload(msg); ok {
			t.Errorf("interactiveLiteralPayload(alt+%v) = (%q, true), want declined -- an Alt-modified special key goes by tmux name, never as hand-built bytes", c.keyType, payload)
		}
	}
}

// TestInteractiveAltForwardingLeavesTheFixedByteKeysAlone is issue #28's
// no-collateral-damage guard. The keys below already worked before the fix
// and take a different path from the M- names above, so the fix must not
// have moved any of them: Alt+<rune> and the Alt-modified POSITIVE C0/DEL
// types (Enter=13, Tab=9, Escape=27, Backspace=127) still go out as an ESC
// prefix plus their own fixed byte through interactiveLiteralPayload, and
// bare Ctrl+<letter> is untouched by anything Alt-related at all.
func TestInteractiveAltForwardingLeavesTheFixedByteKeysAlone(t *testing.T) {
	literal := []struct {
		name string
		msg  tea.KeyMsg
		want string
	}{
		// Alt+rune: the case that kept working through issue #28 and is
		// why the bug read as intermittent rather than as "Alt is dead".
		{"alt+a", tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune("a"), Alt: true}), "\x1ba"},
		{"alt+space", tea.KeyMsg(tea.Key{Type: tea.KeySpace, Runes: []rune(" "), Alt: true}), "\x1b "},
		{"alt+enter", tea.KeyMsg(tea.Key{Type: tea.KeyEnter, Alt: true}), "\x1b\r"},
		{"alt+tab", tea.KeyMsg(tea.Key{Type: tea.KeyTab, Alt: true}), "\x1b\t"},
		{"alt+escape", tea.KeyMsg(tea.Key{Type: tea.KeyEscape, Alt: true}), "\x1b\x1b"},
		{"alt+backspace", tea.KeyMsg(tea.Key{Type: tea.KeyBackspace, Alt: true}), "\x1b\x7f"},
		{"alt+ctrl+a", tea.KeyMsg(tea.Key{Type: tea.KeyCtrlA, Alt: true}), "\x1b\x01"},
		// Unmodified, for the plain "nothing else moved" half.
		{"enter", tea.KeyMsg(tea.Key{Type: tea.KeyEnter}), "\r"},
		{"backspace", tea.KeyMsg(tea.Key{Type: tea.KeyBackspace}), "\x7f"},
		{"ctrl+a", tea.KeyMsg(tea.Key{Type: tea.KeyCtrlA}), "\x01"},
		{"ctrl+c", tea.KeyMsg(tea.Key{Type: tea.KeyCtrlC}), "\x03"},
		{"rune", tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune("hi")}), "hi"},
	}
	for _, c := range literal {
		got, ok := interactiveLiteralPayload(c.msg)
		if !ok || got != c.want {
			t.Errorf("interactiveLiteralPayload(%s) = (%q, %v), want (%q, true)", c.name, got, ok, c.want)
		}
		// None of these is a named key: if the fix had reached them, they
		// would be resolving to a tmux name instead of to their own bytes,
		// and interactiveNamedKey winning first in updateInteractive means
		// interactiveLiteralPayload would never even be consulted.
		if name, ok := interactiveNamedKey(c.msg); ok {
			t.Errorf("interactiveNamedKey(%s) = (%q, true), want declined -- a fixed-byte key must not have become a named key", c.name, name)
		}
	}
}

// TestEveryBareNamedKeyHasAnExplicitAltVerdict is the structural guard
// that keeps issue #28 from coming back one key at a time. interactiveNamedKey
// resolves Alt by looking the bare name up in interactiveAltNamedKeys, so
// a bare name that is missing from that map reads the zero value and is
// refused -- indistinguishable, at runtime, from the two deliberate gaps.
// This test makes the difference a compile-time-ish fact instead: every
// name interactiveBareNamedKey can return must have an entry, and the only
// entries allowed to be empty are the two gaps SPEC.md §11.9 requires to
// be "known, listed" ones. Adding a KeyType to the bare switch without
// deciding what Alt means for it therefore fails here rather than silently
// re-creating the original silent drop for that one key.
//
// The two gaps and their evidence (both recorded in full in
// interactiveAltNamedKeys' own comment):
//
//   - Insert: tmux translates "M-Insert" correctly, but bubbletea v1.3.10
//     has no entry for those bytes at all -- it carries the transposed
//     "\x1b[3;2~" (xterm's Shift+Delete) as {KeyInsert, Alt: true}
//     instead -- so deck can never receive a genuine Alt+Insert to
//     forward, and forwarding the one KeyMsg that does arrive would type
//     Alt+Insert for a physical Shift+Delete.
//   - BTab: `send-keys M-BTab` on real tmux 3.6b delivers bytes identical
//     to plain `BTab`; tmux itself discards the Alt, so naming it would
//     forward Shift+Tab for a physical Alt+Shift+Tab.
func TestEveryBareNamedKeyHasAnExplicitAltVerdict(t *testing.T) {
	// bubbletea's special KeyTypes are negative and its C0/DEL types are
	// 0..127, so this range covers every value interactiveBareNamedKey can
	// ever be handed.
	knownGaps := map[string]string{
		"Insert": "bubbletea v1.3.10 cannot decode tmux's (correct) M-Insert bytes -- upstream parameter transposition at key.go:407",
		"BTab":   "tmux discards the Alt: send-keys M-BTab delivers the same bytes as BTab",
	}
	seen := map[string]bool{}
	for raw := -256; raw <= 255; raw++ {
		keyType := tea.KeyType(raw)
		name, ok := interactiveBareNamedKey(keyType)
		if !ok {
			continue
		}
		seen[name] = true
		altName, listed := interactiveAltNamedKeys[name]
		if !listed {
			t.Errorf("interactiveBareNamedKey(%d) = %q, which has NO entry in interactiveAltNamedKeys -- decide what Alt+%s forwards as (and allowlist the name in internal/tmux), or record it as a listed gap with an empty value; a missing entry is issue #28's silent drop for that key", raw, name, name)
			continue
		}
		if altName == "" {
			if _, expected := knownGaps[name]; !expected {
				t.Errorf("interactiveAltNamedKeys[%q] is empty, but %q is not one of the two gaps SPEC.md §11.9's enumeration records -- a new gap needs its reason written down in both places, not just an empty string", name, name)
			}
			continue
		}
		if !tmux.IsNamedKeyAllowed(altName) {
			t.Errorf("interactiveAltNamedKeys[%q] = %q, which is NOT on internal/tmux's namedKeyAllowlist", name, altName)
		}
	}
	// The reverse direction: nothing in interactiveAltNamedKeys may be
	// keyed by a name interactiveBareNamedKey cannot actually produce,
	// which would be exactly the "untested dead weight" internal/tmux/key.go's
	// allowlist comment refuses.
	for name := range interactiveAltNamedKeys {
		if !seen[name] {
			t.Errorf("interactiveAltNamedKeys has an entry for %q, which interactiveBareNamedKey never returns -- an unreachable Alt mapping cannot be tested and must not exist", name)
		}
	}
	for name := range knownGaps {
		if altName, listed := interactiveAltNamedKeys[name]; !listed || altName != "" {
			t.Errorf("interactiveAltNamedKeys[%q] = (%q, %v), want a listed gap (\"\", true) -- if this key's Alt form became forwardable, move it out of this test's knownGaps and out of the gap list in SPEC.md §11.9", name, altName, listed)
		}
	}
}

// TestInteractiveLiteralPayloadCoversEveryFixedByteKey proves the other
// half of the split: runes/space forward verbatim, every C0 control byte
// (including Ctrl+A..Ctrl+Z, Enter=13, Tab=9, Escape=27) and DEL/127
// forward as that literal byte, and Alt prefixes a plain ESC byte -- all
// without ever touching tmux's own named-key translation, since these
// values never depend on the pane's terminal mode.
func TestInteractiveLiteralPayloadCoversEveryFixedByteKey(t *testing.T) {
	cases := []struct {
		msg  tea.KeyMsg
		want string
	}{
		{tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune("hi")}), "hi"},
		{tea.KeyMsg(tea.Key{Type: tea.KeySpace, Runes: []rune(" ")}), " "},
		{tea.KeyMsg(tea.Key{Type: tea.KeyEnter}), "\r"},
		{tea.KeyMsg(tea.Key{Type: tea.KeyTab}), "\t"},
		{tea.KeyMsg(tea.Key{Type: tea.KeyEscape}), "\x1b"},
		{tea.KeyMsg(tea.Key{Type: tea.KeyBackspace}), "\x7f"},
		{tea.KeyMsg(tea.Key{Type: tea.KeyCtrlA}), "\x01"},
		{tea.KeyMsg(tea.Key{Type: tea.KeyCtrlB}), "\x02"},
		{tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune("a"), Alt: true}), "\x1ba"},
	}
	for _, c := range cases {
		got, ok := interactiveLiteralPayload(c.msg)
		if !ok || got != c.want {
			t.Errorf("interactiveLiteralPayload(%v) = (%q, %v), want (%q, true)", c.msg.Type, got, ok, c.want)
		}
	}
	// A mode-dependent named key is NOT a literal payload --
	// interactiveNamedKey owns it instead, with or without Alt (issue #28:
	// the Alt-modified form used to be refused by BOTH functions, which is
	// what made the keystroke vanish; see
	// TestInteractiveNamedKeyForwardsEveryAltModifiedSpecialKey).
	if _, ok := interactiveLiteralPayload(tea.KeyMsg(tea.Key{Type: tea.KeyUp})); ok {
		t.Errorf("interactiveLiteralPayload claimed a mode-dependent named key")
	}
	if _, ok := interactiveLiteralPayload(tea.KeyMsg(tea.Key{Type: tea.KeyUp, Alt: true})); ok {
		t.Errorf("interactiveLiteralPayload claimed an alt-modified mode-dependent named key")
	}
}

// TestEnterInteractiveRefusesBelowTheSevenRowFloorWithoutAnyTmuxCall is PRD
// Part II requirement 48's own deterministic backstop (task 203, review
// finding F3): F3 found that the ONLY coverage of the 7-row-floor refusal
// was a real-tmux godog scenario that flakes under load, and that the
// refusal's own published root cause was wrong (deck was observed entering
// interactive mode at 41x6 rather than refusing -- see
// docs/reports/phase3d-201-req48-degrade-rootcause.md). enterInteractive's
// floor check (interactive.go's "Refusal case 1") now runs before the
// attached-client check and before every other tmux call in the function,
// specifically so this can be proved with NO live tmux server at all: the
// tmux.Client below carries a non-empty Socket (so the zero-client degrade
// path at the top of enterInteractive does not fire) that resolves to
// nothing real, and the test still passes -- the moment any tmux call
// actually ran, it would return a plain "exec: tmux: not found"/dial error
// instead of the floor message, and this test would fail on the assertion
// below. 80x9 is requirement 48's own godog fixture size
// (features/interactive_refusals.feature's @requirement-48 scenario),
// which this test independently confirms fits the exact 41x6 frame F3's
// root-cause report captured (previewContentSize below).
func TestEnterInteractiveRefusesBelowTheSevenRowFloorWithoutAnyTmuxCall(t *testing.T) {
	m := New(nil, config.Settings{}, "")
	m = m.WithTmuxClient(tmux.Client{Socket: "no-such-tmux-server-203"})
	m.sessions = []store.Session{{ID: "s1", Name: "squeezed", Agent: "shell", Status: "running", Slug: "squeezed"}}
	m.selected = rowCursor(0)
	m.width, m.height = 80, 9

	width, height := m.previewContentSize()
	if width != 41 || height != 6 {
		t.Fatalf("previewContentSize() at 80x9 = %dx%d, want 41x6 (requirement 48's own godog fixture frame); the fixture below no longer matches the scenario it stands in for", width, height)
	}
	if height >= interactiveMinInnerRows {
		t.Fatalf("fixture height %d is not below interactiveMinInnerRows (%d); this test would be vacuous", height, interactiveMinInnerRows)
	}

	next, cmd := m.enterInteractive()
	got := next.(Model)

	if got.interactive {
		t.Fatalf("enterInteractive entered interactive mode at %dx%d inner rows, below the %d-row floor -- this is exactly F3's degrade defect, not a refusal", width, height, interactiveMinInnerRows)
	}
	if cmd != nil {
		t.Fatalf("enterInteractive returned a non-nil cmd on the floor refusal path, want nil")
	}
	if !strings.Contains(got.attachError, "7-row floor") {
		t.Fatalf("attachError %q does not name the 7-row floor", got.attachError)
	}
	if !strings.Contains(got.attachError, "press a to attach") {
		t.Fatalf("attachError %q does not offer the a-to-attach alternative (PRD II-47)", got.attachError)
	}
	if !strings.Contains(got.attachError, fmt.Sprintf("%d inner rows", height)) {
		t.Fatalf("attachError %q does not name the measured inner-row count %d", got.attachError, height)
	}
}

// TestWindowShrinkBelowTheFloorLeavesInteractiveModeAndRestoresTheList is
// PRD Part II requirement 48/task 204's own backstop (review finding F3's
// second half): 203 fixed enterInteractive's floor check, but that check
// only ever ran AT ENTRY -- previewTitle/previewContentSize (tui.go:3092)
// recompute the preview box's inner size on every render, so nothing
// re-checked the floor after a shrink WHILE ALREADY interactive, and deck
// simply kept rendering the live grid into a box below the measured
// floor. This proves the fix directly against Model.Update's own
// tea.WindowSizeMsg case, with no live tmux server at all: m.interactive
// starts true with every tmux-owning field already at ITS zero value
// (interactiveWindowTarget/interactiveOwnership/interactiveGrid/
// interactiveDispatcher), the exact shape
// TestCtrlQExitsInteractiveModeAndClearsEveryField already uses to drive
// exitInteractive without spawning tmux -- so if this test's WindowSizeMsg
// reaches any real tmux call at all (it must not: the fix's whole point is
// that the ordinary exit path, which DOES release ownership/restore
// geometry when those fields are non-zero, is reused unconditionally), it
// would be a no-op regardless, and the assertions below are what actually
// prove the mode was left.
func TestWindowShrinkBelowTheFloorLeavesInteractiveModeAndRestoresTheList(t *testing.T) {
	m := New(nil, config.Settings{}, "")
	m.sessions = []store.Session{{ID: "s1", Name: "squeezed", Agent: "shell", Status: "running", Slug: "squeezed"}}
	m.selected = rowCursor(0)
	m.width, m.height = 80, 24
	m.interactive = true

	// Sanity: 80x24 is comfortably above the floor, and a shrink that
	// STAYS above the floor must not leave interactive mode -- otherwise
	// every ordinary resize would kick the user out.
	next, cmd := m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	stillIn := next.(Model)
	if !stillIn.interactive {
		t.Fatalf("a resize that stays above the %d-row floor left interactive mode; only a resize BELOW the floor may do that", interactiveMinInnerRows)
	}
	if cmd != nil {
		t.Fatalf("a resize that stays above the floor returned a non-nil cmd, want nil")
	}

	// The real case: 80x9 is requirement 48's own godog fixture size
	// (features/interactive_refusals.feature's @requirement-48 scenario),
	// reached here via a resize WHILE already interactive rather than at
	// Enter time. Its expected geometry is measured on a probe BEFORE the
	// resize (attachError still "" there, same as on stillIn), matching
	// exactly what the product code's own previewContentSize call sees
	// before it sets attachError -- previewContentSize's reserved-rows
	// budget grows once attachError is populated (attachErrorLines), so
	// measuring it on the POST-shrink model would silently disagree with
	// the number the product actually put in the message.
	probe := stillIn
	probe.width, probe.height = 80, 9
	wantWidth, wantHeight := probe.previewContentSize()
	if wantWidth != 41 || wantHeight != 6 {
		t.Fatalf("previewContentSize() at 80x9 = %dx%d, want 41x6; the fixture no longer matches requirement 48's own scenario", wantWidth, wantHeight)
	}
	if wantHeight >= interactiveMinInnerRows {
		t.Fatalf("fixture height %d is not below interactiveMinInnerRows (%d); this test would be vacuous", wantHeight, interactiveMinInnerRows)
	}

	next, cmd = stillIn.Update(tea.WindowSizeMsg{Width: 80, Height: 9})
	got := next.(Model)

	if got.interactive {
		t.Fatalf("deck remained interactive at %dx%d inner rows, below the %d-row floor, after a mid-session shrink -- this is F3's degrade defect, now reached by resize instead of entry", wantWidth, wantHeight, interactiveMinInnerRows)
	}
	if cmd != nil {
		t.Fatalf("the below-floor shrink returned a non-nil cmd, want nil (exitInteractive never returns one)")
	}
	if got.interactiveWindowTarget != "" || got.interactiveOwnership != nil || got.interactiveGrid != nil || got.interactiveDispatcher != nil {
		t.Fatalf("the below-floor shrink left interactive state behind: %+v", got)
	}
	if !strings.Contains(got.attachError, "7-row floor") {
		t.Fatalf("attachError %q does not name the 7-row floor", got.attachError)
	}
	if !strings.Contains(got.attachError, "press a to attach") {
		t.Fatalf("attachError %q does not offer the a-to-attach alternative (PRD II-47)", got.attachError)
	}
	if !strings.Contains(got.attachError, fmt.Sprintf("%d inner rows", wantHeight)) {
		t.Fatalf("attachError %q does not name the measured inner-row count %d", got.attachError, wantHeight)
	}
}
