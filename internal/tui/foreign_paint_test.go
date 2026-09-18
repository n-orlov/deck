package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/theme"
)

// paintModel builds a Model whose ui.preview_paint is the SHIPPED default,
// taken from the schema rather than written out here, so a change to that
// default cannot leave these tests asserting a mode deck no longer uses.
func paintModel(t *testing.T, name string) Model {
	t.Helper()
	return paintModelMode(t, name, shippedPreviewPaint(t))
}

// paintModelMode builds a Model in one named ui.preview_paint mode, for the
// tests that are about a specific mode rather than the default.
func paintModelMode(t *testing.T, name, mode string) Model {
	t.Helper()
	th, ok := theme.Builtin(name)
	if !ok {
		t.Fatalf("theme.Builtin(%q): not found", name)
	}
	return Model{settings: config.Settings{Color: true, Theme: th, PreviewPaint: mode}}
}

// shippedPreviewPaint is ui.preview_paint's schema default: what a deck
// built from this tree does for a user who has never touched the key.
func shippedPreviewPaint(t *testing.T) string {
	t.Helper()
	field, ok := config.FieldByFullKey("ui.preview_paint")
	if !ok {
		t.Fatal("config.Schema has no ui.preview_paint field")
	}
	mode, ok := field.Default.(string)
	if !ok {
		t.Fatalf("ui.preview_paint Default = %#v, want a string", field.Default)
	}
	return mode
}

// hexSGRFg is the truecolour foreground sequence for a hex colour, for
// asserting what repaintForeignDefaults emitted.
func hexSGRFg(t *testing.T, hex string) string {
	t.Helper()
	r, g, b, err := theme.HexRGB(hex)
	if err != nil {
		t.Fatalf("HexRGB(%q): %v", hex, err)
	}
	return fmt.Sprintf("\x1b[38;2;%d;%d;%dm", r, g, b)
}

func hexSGRBg(t *testing.T, hex string) string {
	t.Helper()
	r, g, b, err := theme.HexRGB(hex)
	if err != nil {
		t.Fatalf("HexRGB(%q): %v", hex, err)
	}
	return fmt.Sprintf("\x1b[48;2;%d;%d;%dm", r, g, b)
}

// TestForeignPaintOffIsByteIdentical: `preview_paint = "off"` must hand the
// captured bytes back untouched, byte for byte. This is the escape hatch
// SPEC §11.3 promises a user who would rather see an agent's colours
// exactly as the agent chose them, so "off" being ALMOST unchanged would
// not honour it.
func TestForeignPaintOffIsByteIdentical(t *testing.T) {
	m := paintModelMode(t, "parchment", "off")
	for _, row := range []string{
		"plain text",
		"\x1b[34mblue\x1b[0m plain",
		"\x1b[48;2;0;39;0m\x1b[38;5;226myellow on green\x1b[m",
		"",
	} {
		if got := m.repaintForeignDefaults(theme.Background, row); got != row {
			t.Errorf("preview_paint=off changed %q to %q", row, got)
		}
	}
}

// TestForeignPaintEmitsNothingWithoutColour mirrors canvasBackground's own
// colour-disabled contract: NO_COLOR/DECK_COLOR=0 means no escape bytes at
// all, not "escape bytes with the default colour".
func TestForeignPaintEmitsNothingWithoutColour(t *testing.T) {
	m := Model{settings: config.Settings{Color: false}}
	const row = "\x1b[34mblue\x1b[0m plain"
	if got := m.repaintForeignDefaults(theme.Background, row); got != row {
		t.Errorf("with Color=false, got %q, want the row unchanged", got)
	}
}

// TestForeignPaintOpensTheCanvasPairOnAPlainRow: the overwhelmingly common
// case -- an agent that emitted no SGR at all -- must come back carrying
// deck's background AND foreground. Background alone is GH #24 all over
// again: the text would fall back to the TERMINAL's default foreground,
// whose contrast against a light theme is undefined.
func TestForeignPaintOpensTheCanvasPairOnAPlainRow(t *testing.T) {
	m := paintModel(t, "parchment")
	th := m.activeTheme()
	bg, _ := th.Color(theme.Background)
	text, _ := th.Color(theme.Text)

	got := m.repaintForeignDefaults(theme.Background, "hello")
	if !strings.Contains(got, hexSGRBg(t, bg)) {
		t.Errorf("row %q carries no %s background (%s)", got, theme.Background, bg)
	}
	if !strings.Contains(got, hexSGRFg(t, text)) {
		t.Errorf("row %q carries no %s foreground (%s)", got, theme.Text, text)
	}
	if !strings.HasSuffix(got, "hello") {
		t.Errorf("row %q does not end in the agent's own text", got)
	}
}

// TestForeignPaintFitsAnExplicitForegroundOverDecksBackground is the
// operator's requirement: an explicit agent colour is PRESERVED (its hue)
// and ADJUSTED (its lightness) against whatever deck now paints under it.
func TestForeignPaintFitsAnExplicitForegroundOverDecksBackground(t *testing.T) {
	m := paintModel(t, "parchment")
	bg, _ := m.activeTheme().Color(theme.Background)

	// SGR 93 is bright yellow (#ffff00): 1.09:1 on parchment, the worst
	// case the operator would actually hit.
	got := m.repaintForeignDefaults(theme.Background, "\x1b[93magent\x1b[0m")

	want, changed, err := theme.FitForeground(theme.ReferencePalette[11], bg, theme.AAFloor)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatalf("expected bright yellow to need fitting on parchment")
	}
	if !strings.Contains(got, hexSGRFg(t, want)) {
		t.Errorf("row %q does not carry the fitted colour %s", got, want)
	}
	// And the agent's own sequence is still there: deck ADDS a correction
	// after it rather than rewriting the pane's bytes.
	if !strings.Contains(got, "\x1b[93m") {
		t.Errorf("row %q dropped the agent's own \\x1b[93m", got)
	}
	ratio, err := theme.ContrastRatio(want, bg)
	if err != nil {
		t.Fatal(err)
	}
	if ratio < theme.AAFloor-0.01 {
		t.Errorf("fitted %s is only %.2f:1 on %s", want, ratio, bg)
	}
}

// TestForeignPaintNoFitKeepsTheAgentsExactColour: `preview_paint = "nofit"`
// paints the canvas but never touches a colour the agent chose, for a user
// who wants deck's background under the pane and the agent's own hues
// exactly as chosen -- illegible pairings included, which is the trade the
// mode exists to allow.
func TestForeignPaintNoFitKeepsTheAgentsExactColour(t *testing.T) {
	m := paintModelMode(t, "parchment", "nofit")
	got := m.repaintForeignDefaults(theme.Background, "\x1b[93magent\x1b[0m")
	fitted, _, err := theme.FitForeground(theme.ReferencePalette[11], mustColor(t, m, theme.Background), theme.AAFloor)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, hexSGRFg(t, fitted)) {
		t.Errorf("nofit mode still emitted the fitted colour: %q", got)
	}
}

// TestForeignPaintLeavesAnExplicitPairAlone: when the agent chose BOTH the
// foreground and the background, deck has no cell to contribute and must
// contribute nothing -- painting there would destroy a deliberate pairing
// (a diff hunk, a selected line, a status bar).
func TestForeignPaintLeavesAnExplicitPairAlone(t *testing.T) {
	m := paintModel(t, "parchment")
	bg, _ := m.activeTheme().Color(theme.Background)
	got := m.repaintForeignDefaults(theme.Background, "\x1b[44;97mstatus\x1b[0m")

	// Between the agent's own pair sequence and its text, deck must add
	// nothing at all.
	idx := strings.Index(got, "\x1b[44;97m")
	if idx < 0 {
		t.Fatalf("row %q lost the agent's pair sequence", got)
	}
	after := got[idx+len("\x1b[44;97m"):]
	if !strings.HasPrefix(after, "status") {
		t.Errorf("deck inserted %q between the agent's explicit pair and its text",
			after[:min(len(after), 40)])
	}
	// The canvas background must NOT be re-asserted over the agent's own.
	if strings.Contains(after[:min(len(after), 20)], hexSGRBg(t, bg)) {
		t.Errorf("deck overpainted the agent's explicit background: %q", got)
	}
}

// TestForeignPaintLeavesTheForegroundAloneOverAnAgentBackground: the agent
// set a background and left the text default, expecting the terminal's
// default foreground on it. Substituting theme.Text there could put
// parchment's near-black onto the agent's dark blue -- worse than leaving
// it, so this is deliberately NOT painted.
func TestForeignPaintLeavesTheForegroundAloneOverAnAgentBackground(t *testing.T) {
	m := paintModel(t, "parchment")
	text, _ := m.activeTheme().Color(theme.Text)
	got := m.repaintForeignDefaults(theme.Background, "\x1b[44mon blue\x1b[0m")

	idx := strings.Index(got, "\x1b[44m")
	if idx < 0 {
		t.Fatalf("row %q lost the agent's background sequence", got)
	}
	after := got[idx+len("\x1b[44m"):]
	if strings.HasPrefix(after, hexSGRFg(t, text)) {
		t.Errorf("deck asserted theme.Text over the agent's own background: %q", got)
	}
}

// TestForeignPaintDoesNotMisreadASubParameterAsADefault is the trap this
// whole state machine exists to avoid: 39 and 49 are "default foreground"
// and "default background" as top-level SGR parameters, but they also
// occur as ordinary colour components. "\x1b[48;2;0;39;0m" sets an
// explicit green background and says nothing about the foreground; a naive
// scanner reads the 39 and wrongly concludes the foreground went default.
func TestForeignPaintDoesNotMisreadASubParameterAsADefault(t *testing.T) {
	var fg, bg sgrColor
	var reverse bool
	applySGR("48;2;0;39;0", &fg, &bg, &reverse)
	if fg.explicit {
		t.Errorf("foreground became explicit from a background's sub-parameters: %+v", fg)
	}
	if !bg.explicit || bg.hex != "#002700" {
		t.Errorf("background = %+v, want explicit #002700", bg)
	}

	// The same shape for a foreground carrying a 49.
	fg, bg, reverse = sgrColor{}, sgrColor{}, false
	applySGR("38;2;0;49;0", &fg, &bg, &reverse)
	if bg.explicit {
		t.Errorf("background became explicit from a foreground's sub-parameters: %+v", bg)
	}
	if !fg.explicit || fg.hex != "#003100" {
		t.Errorf("foreground = %+v, want explicit #003100", fg)
	}
}

func TestApplySGRTracksState(t *testing.T) {
	cases := []struct {
		params      string
		wantFg      sgrColor
		wantBg      sgrColor
		wantReverse bool
	}{
		{"", sgrColor{}, sgrColor{}, false},
		{"0", sgrColor{}, sgrColor{}, false},
		{"34", sgrColor{true, theme.ReferencePalette[4]}, sgrColor{}, false},
		{"94", sgrColor{true, theme.ReferencePalette[12]}, sgrColor{}, false},
		{"33", sgrColor{true, theme.ReferencePalette[3]}, sgrColor{}, false},
		{"41", sgrColor{}, sgrColor{true, theme.ReferencePalette[1]}, false},
		{"104", sgrColor{}, sgrColor{true, theme.ReferencePalette[12]}, false},
		{"39", sgrColor{}, sgrColor{}, false},
		{"49", sgrColor{}, sgrColor{}, false},
		// Last state wins WITHIN one sequence: reset then explicit red is
		// an explicit foreground, not a default one.
		{"0;31", sgrColor{true, theme.ReferencePalette[1]}, sgrColor{}, false},
		// ...and the reverse order is a default foreground.
		{"31;39", sgrColor{}, sgrColor{}, false},
		{"7", sgrColor{}, sgrColor{}, true},
		{"7;27", sgrColor{}, sgrColor{}, false},
		{"38;5;226", sgrColor{true, "#ffff00"}, sgrColor{}, false},
		{"38;5;4", sgrColor{true, theme.ReferencePalette[4]}, sgrColor{}, false},
		{"38;5;232", sgrColor{true, "#080808"}, sgrColor{}, false},
		{"1;38;2;12;34;56;4", sgrColor{true, "#0c2238"}, sgrColor{}, false},
		// Bold/underline/italic are the pane's business and change nothing.
		{"1;3;4", sgrColor{}, sgrColor{}, false},
	}
	for _, c := range cases {
		var fg, bg sgrColor
		var reverse bool
		applySGR(c.params, &fg, &bg, &reverse)
		if fg != c.wantFg || bg != c.wantBg || reverse != c.wantReverse {
			t.Errorf("applySGR(%q) = fg%+v bg%+v rev=%v, want fg%+v bg%+v rev=%v",
				c.params, fg, bg, reverse, c.wantFg, c.wantBg, c.wantReverse)
		}
	}
}

// TestForeignPaintPassesNonSGRSequencesThrough: deck rewrites colour and
// nothing else. A pane's cursor moves, mode changes and hyperlinks are its
// own business, and a row carrying them must come back with them intact.
func TestForeignPaintPassesNonSGRSequencesThrough(t *testing.T) {
	m := paintModel(t, "parchment")
	for _, seq := range []string{"\x1b[2K", "\x1b[1;5H", "\x1b[?25l", "\x1b[3J"} {
		row := "a" + seq + "b"
		got := m.repaintForeignDefaults(theme.Background, row)
		if !strings.Contains(got, seq) {
			t.Errorf("row %q lost the non-SGR sequence %q (got %q)", row, seq, got)
		}
	}
}

// TestForeignPaintSurvivesATruncatedSequence: a captured row can be cut
// mid-escape by cropRow's own truncation, and a renderer that panics or
// drops the tail on one is worse than one that leaves it alone.
func TestForeignPaintSurvivesATruncatedSequence(t *testing.T) {
	m := paintModel(t, "parchment")
	for _, row := range []string{"text\x1b", "text\x1b[", "text\x1b[38;2;1", "\x1b[0"} {
		got := m.repaintForeignDefaults(theme.Background, row)
		if !strings.HasSuffix(got, row) && !strings.Contains(got, row) {
			t.Errorf("row %q came back as %q, losing its own tail", row, got)
		}
	}
}

// TestForeignPaintReopensTheCanvasAfterEveryReset is the same invariant
// canvasBackground carries for deck's own chrome (SPEC.md:1593): a reset
// clears BOTH channels, so both have to be re-asserted after it, or the
// remainder of the row falls back to the terminal's own colours.
func TestForeignPaintReopensTheCanvasAfterEveryReset(t *testing.T) {
	m := paintModel(t, "parchment")
	th := m.activeTheme()
	bg, _ := th.Color(theme.Background)
	text, _ := th.Color(theme.Text)
	bgSeq, fgSeq := hexSGRBg(t, bg), hexSGRFg(t, text)

	for _, reset := range []string{"\x1b[0m", "\x1b[m", "\x1b[00m"} {
		row := "\x1b[34mblue" + reset + "after"
		got := m.repaintForeignDefaults(theme.Background, row)
		idx := strings.Index(got, reset+bgSeq)
		if idx < 0 {
			t.Errorf("reset %q was not followed by the canvas background in %q", reset, got)
			continue
		}
		rest := got[idx+len(reset+bgSeq):]
		if !strings.HasPrefix(rest, fgSeq) {
			t.Errorf("reset %q re-opened the background but not the foreground in %q", reset, got)
		}
	}

	// 39 and 49 alone each re-open only their own channel.
	got := m.repaintForeignDefaults(theme.Background, "\x1b[34mblue\x1b[39mafter")
	if !strings.Contains(got, "\x1b[39m"+bgSeq+fgSeq) {
		t.Errorf("a bare 39 did not restore the canvas foreground: %q", got)
	}
}

// TestForeignCanvasTokenDistinguishesAttachedFromUnattached: the operator
// called this out as extremely important -- if painting both preview modes
// made them the same colour, "are my keystrokes going to the pane?" would
// be answerable only from the border. The two tokens must differ, and must
// differ in EVERY built-in.
func TestForeignCanvasTokenDistinguishesAttachedFromUnattached(t *testing.T) {
	for _, th := range theme.Builtins() {
		passive := Model{settings: config.Settings{Color: true, Theme: th}}
		attached := Model{settings: config.Settings{Color: true, Theme: th}, interactive: true}

		pTok, aTok := passive.foreignCanvasToken(), attached.foreignCanvasToken()
		if pTok == aTok {
			t.Errorf("%s: attached and unattached previews both paint %s", th.Name, pTok)
			continue
		}
		pHex, _ := th.Color(pTok)
		aHex, _ := th.Color(aTok)
		if pHex == aHex {
			t.Errorf("%s: %s and %s are both %s, so the two modes are indistinguishable",
				th.Name, pTok, aTok, pHex)
		}
		// And the step has to be big enough to actually see. `surface` is
		// the weakest step of the tokens considered, at ~1.07:1; anything
		// below that is not a cue at all.
		ratio, err := theme.ContrastRatio(pHex, aHex)
		if err != nil {
			t.Fatal(err)
		}
		if ratio < 1.05 {
			t.Errorf("%s: %s vs %s is only %.3f:1 -- too close to read as a different region",
				th.Name, pTok, aTok, ratio)
		}
	}
}

// TestForeignPaintDefaultsToTheShippedContract pins what a user who has
// never touched config.toml gets: ui.preview_paint's schema default, and
// deck actually painting under a captured row rather than merely being
// configured to. Both halves matter -- a default of "fit" that some
// unresolved zero value quietly turned back into "off" would look exactly
// like this test's first assertion passing on its own.
func TestForeignPaintDefaultsToTheShippedContract(t *testing.T) {
	if got := shippedPreviewPaint(t); got != "fit" {
		t.Fatalf("ui.preview_paint default = %q, want \"fit\" (SPEC §11.3)", got)
	}
	m := paintModel(t, "parchment")
	if got := m.foreignPaint(); got != foreignPaintFit {
		t.Errorf("a Model on the shipped default resolves to mode %v, want fit", got)
	}
	const row = "\x1b[34mblue\x1b[0m plain"
	painted := m.repaintForeignDefaults(theme.Background, row)
	if painted == row {
		t.Errorf("the shipped default left a captured row untouched: %q", row)
	}
	bg := mustColor(t, m, theme.Background)
	if !strings.Contains(painted, hexSGRBg(t, bg)) {
		t.Errorf("the shipped default did not open deck's canvas background %s: %q", bg, painted)
	}
}

// TestForeignPaintZeroSettingsFallsBackToTheDefault covers the one caller
// config validation cannot reach: a Model built in a test (or any future
// code path) whose Settings were never loaded carries PreviewPaint == "".
// That must resolve to the shipped mode, not to a fifth silent behaviour.
func TestForeignPaintZeroSettingsFallsBackToTheDefault(t *testing.T) {
	var m Model
	if got := m.foreignPaint(); got != foreignPaintFit {
		t.Errorf("an unloaded Settings resolves ui.preview_paint to %v, want fit", got)
	}
}

// TestParseForeignPaint covers exactly ui.preview_paint's declared values,
// read from the schema so a value added there without a mode here fails
// loudly rather than falling into the default arm. Case and surrounding
// space are tolerated because a hand-edited config.toml is the normal way
// this key gets set.
func TestParseForeignPaint(t *testing.T) {
	field, ok := config.FieldByFullKey("ui.preview_paint")
	if !ok {
		t.Fatal("config.Schema has no ui.preview_paint field")
	}
	want := map[string]foreignPaintMode{
		"fit":   foreignPaintFit,
		"nofit": foreignPaintNoFit,
		"bg":    foreignPaintBackgroundOnly,
		"off":   foreignPaintOff,
	}
	if len(field.EnumValues) != len(want) {
		t.Fatalf("ui.preview_paint EnumValues = %v, but this test maps %d values", field.EnumValues, len(want))
	}
	for _, value := range field.EnumValues {
		mode, ok := want[value]
		if !ok {
			t.Fatalf("ui.preview_paint declares %q, which parseForeignPaint has no mode for", value)
		}
		for _, spelling := range []string{value, strings.ToUpper(value), " " + value + " "} {
			if got := parseForeignPaint(spelling); got != mode {
				t.Errorf("parseForeignPaint(%q) = %v, want %v", spelling, got, mode)
			}
		}
	}
	// An alias is NOT accepted: "no" reading as off would mean a
	// config.toml that says one thing and a deck that does another, and
	// config rejects the value at parse time anyway.
	if got := parseForeignPaint("no"); got != foreignPaintFit {
		t.Errorf("parseForeignPaint(%q) = %v, want the default arm (aliases are not accepted)", "no", got)
	}
}

func TestXterm256Hex(t *testing.T) {
	cases := map[int]string{
		0: theme.ReferencePalette[0], 15: theme.ReferencePalette[15],
		16: "#000000", 226: "#ffff00", 231: "#ffffff",
		232: "#080808", 255: "#eeeeee",
	}
	for in, want := range cases {
		if got := xterm256Hex(in); got != want {
			t.Errorf("xterm256Hex(%d) = %q, want %q", in, got, want)
		}
	}
	for _, bad := range []int{-1, 256, 999} {
		if got := xterm256Hex(bad); got != "" {
			t.Errorf("xterm256Hex(%d) = %q, want \"\"", bad, got)
		}
	}
}

// TestFgSGRForHexRespectsColourDepth: emitting 38;2 truecolour on a
// 16-colour terminal is exactly the mistake deck's own tokens avoid by
// going through ANSI16Code, so a fitted agent colour must avoid it too.
func TestFgSGRForHexRespectsColourDepth(t *testing.T) {
	th, _ := theme.Builtin("parchment")
	truecolor := Model{settings: config.Settings{Color: true, Theme: th}}
	if got := truecolor.fgSGRForHex("#6f6f00"); got != "\x1b[38;2;111;111;0m" {
		t.Errorf("truecolour depth emitted %q", got)
	}
	sixteen := Model{settings: config.Settings{Color: true, Theme: th, ColorDepth: "16"}}
	got := sixteen.fgSGRForHex("#6f6f00")
	if strings.Contains(got, "38;2") {
		t.Errorf("16-colour depth emitted truecolour: %q", got)
	}
	if got == "" {
		t.Errorf("16-colour depth emitted nothing for #6f6f00")
	}
}

func mustColor(t *testing.T, m Model, tok theme.Token) string {
	t.Helper()
	hex, err := m.activeTheme().Color(tok)
	if err != nil {
		t.Fatalf("Color(%s): %v", tok, err)
	}
	return hex
}

// TestPreviewContentLineDistinguishesAttachedFromUnattachedEndToEnd is the
// operator's "extremely important" requirement asserted where it actually
// matters -- on the composed row, not just on the token helper. Once deck
// paints under BOTH preview modes, the only thing left saying "your
// keystrokes are going here" is this colour step plus the border, so a
// change that made the two rows identical has to fail a test.
func TestPreviewContentLineDistinguishesAttachedFromUnattachedEndToEnd(t *testing.T) {
	for _, th := range theme.Builtins() {
		passive := Model{settings: config.Settings{Color: true, Theme: th}}
		attached := Model{settings: config.Settings{Color: true, Theme: th}, interactive: true}

		const row = "agent output"
		pLine := passive.previewContentLine(40, row, false)
		aLine := attached.previewContentLine(40, row, false)
		if pLine == aLine {
			t.Errorf("%s: attached and unattached preview rows are byte-identical -- the attach cue is gone", th.Name)
			continue
		}

		bgHex, err := th.Color(theme.Background)
		if err != nil {
			t.Fatal(err)
		}
		attachedTok := attachedCanvasToken()
		attachedHex, err := th.Color(attachedTok)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(pLine, hexSGRBg(t, bgHex)) {
			t.Errorf("%s: unattached row does not carry `background` (%s)", th.Name, bgHex)
		}
		if !strings.Contains(aLine, hexSGRBg(t, attachedHex)) {
			t.Errorf("%s: attached row does not carry `%s` (%s)", th.Name, attachedTok, attachedHex)
		}
		if strings.Contains(aLine, hexSGRBg(t, bgHex)) {
			t.Errorf("%s: attached row still carries `background` (%s), muddying the cue", th.Name, bgHex)
		}
	}
}

// TestFullBoxPreviewContentLineDistinguishesAttachedFromUnattached is the
// same requirement for the stacked/full-box layout, which is a separate
// composition function and so a separate way to lose the cue.
func TestFullBoxPreviewContentLineDistinguishesAttachedFromUnattached(t *testing.T) {
	for _, th := range theme.Builtins() {
		passive := Model{settings: config.Settings{Color: true, Theme: th}}
		attached := Model{settings: config.Settings{Color: true, Theme: th}, interactive: true}
		p := passive.fullBoxPreviewContentLine(40, "agent output", true, false)
		a := attached.fullBoxPreviewContentLine(40, "agent output", true, false)
		if p == a {
			t.Errorf("%s: attached and unattached full-box rows are byte-identical", th.Name)
		}
	}
}
