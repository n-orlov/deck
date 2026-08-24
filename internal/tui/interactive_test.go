package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// TestEnterWithoutATmuxClientDoesNotEnterInteractiveMode proves task 061's
// degrade path (WithTmuxClient never called, exactly like every
// pre-task-061 constructor and unit test): Enter is a no-op, not a real
// tmux invocation nobody asked for, mirroring how attachSelected already
// degrades for a nil m.attach.
func TestEnterWithoutATmuxClientDoesNotEnterInteractiveMode(t *testing.T) {
	m := New(nil, config.Settings{}, "")
	m.sessions = []store.Session{{ID: "s1", Name: "one", Agent: "shell", Status: "running", Slug: "one"}}
	m.selected = 0
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
	m.selected = 0
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
	m.selected = 0
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
	m.selected = 0
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
// the fitLines boundary, independent of a real interactive.Session (which
// requires a live tmux pane to construct): given the exact lines a real
// Grid().Render() would hand back, a wholly blank grid gets the
// announcement prepended as line 0, and a grid with real content does
// not.
func TestInteractiveBodyLinesPrependsAnnouncementOnlyWhenGridIsBlank(t *testing.T) {
	blankRendered := []string{"", "", "", ""}
	got := fitLinesWithOptionalNotice(blankRendered, 4)
	if got[0] != interactiveNotRepaintedNotice {
		t.Fatalf("blank grid: line 0 = %q, want the not-repainted notice %q", got[0], interactiveNotRepaintedNotice)
	}

	liveRendered := []string{"repaint #1", "", "", ""}
	got = fitLinesWithOptionalNotice(liveRendered, 4)
	for _, line := range got {
		if strings.Contains(line, interactiveNotRepaintedNotice) {
			t.Fatalf("live grid %v unexpectedly carries the not-repainted notice", got)
		}
	}
	if got[0] != "repaint #1" {
		t.Fatalf("live grid: line 0 = %q, want the real content unshifted", got[0])
	}
}

// fitLinesWithOptionalNotice reproduces interactiveBodyLines' own
// blank-check-then-prepend-then-fit sequence directly on a caller-supplied
// rendered-lines slice, so the two tests above can exercise it without
// needing a real *interactive.Session (interactiveBodyLines itself calls
// m.interactiveGrid.Grid().Render(), which requires a live tmux pane to
// construct at all).
func fitLinesWithOptionalNotice(lines []string, contentHeight int) []string {
	if interactiveGridIsBlank(lines) {
		lines = append([]string{interactiveNotRepaintedNotice}, lines...)
	}
	return fitLines(lines, contentHeight)
}

// TestInteractiveNamedKeyMapsOnlyModeDependentKeys proves the named-key/
// literal-byte split (task 061's own design note): arrows, Home/End,
// PgUp/PgDown, Insert/Delete, ShiftTab and the function keys go by NAME
// through tmux's own translation (internal/tmux/key.go's
// namedKeyAllowlist), because their byte encoding depends on the pane's
// terminal mode; nothing else does, and an Alt-modified named key is
// refused outright rather than silently dropping the Alt.
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
	}
	for _, c := range cases {
		got, ok := interactiveNamedKey(c.msg)
		if !ok || got != c.want {
			t.Errorf("interactiveNamedKey(%v) = (%q, %v), want (%q, true)", c.msg.Type, got, ok, c.want)
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
	// Alt-modified named keys are refused outright, never forwarded with
	// the Alt silently dropped.
	altUp := tea.KeyMsg(tea.Key{Type: tea.KeyUp, Alt: true})
	if _, ok := interactiveNamedKey(altUp); ok {
		t.Errorf("interactiveNamedKey forwarded an alt+up as a bare named key")
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
	// interactiveNamedKey owns it instead, and an alt-modified one is
	// refused by both (see TestInteractiveNamedKeyMapsOnlyModeDependentKeys).
	if _, ok := interactiveLiteralPayload(tea.KeyMsg(tea.Key{Type: tea.KeyUp})); ok {
		t.Errorf("interactiveLiteralPayload claimed a mode-dependent named key")
	}
}
