package tui

import (
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/theme"
)

// TestStatusTokenIsExhaustiveOverSevenStatuses pins statusToken against
// theme.StatusTokens' own list rather than a second hand-written table
// here: every one of §7's seven statuses must resolve to its own,
// distinct token (never another status's — "no borrowing between
// statuses", task 014's own success criterion), and an unrecognised
// status word must come back ok=false rather than a default colour, so a
// status added anywhere in store/tui without a matching theme.StatusTokens
// entry is visibly uncoloured rather than silently wrong.
func TestStatusTokenIsExhaustiveOverSevenStatuses(t *testing.T) {
	want := map[string]theme.Token{
		"starting": theme.Starting,
		"running":  theme.Running,
		"waiting":  theme.Waiting,
		"idle":     theme.Idle,
		"error":    theme.Error,
		"stopped":  theme.Stopped,
		"archived": theme.Archived,
	}
	if len(want) != len(theme.StatusTokens) {
		t.Fatalf("test's status table has %d entries, theme.StatusTokens has %d -- keep these in lockstep", len(want), len(theme.StatusTokens))
	}
	for _, tok := range theme.StatusTokens {
		status := string(tok)
		wantTok, known := want[status]
		if !known {
			t.Fatalf("theme.StatusTokens has %q with no entry in this test's status table -- add one", status)
		}
		gotTok, ok := statusToken(status)
		if !ok {
			t.Fatalf("statusToken(%q) = (_, false), want (%q, true)", status, wantTok)
		}
		if gotTok != wantTok {
			t.Fatalf("statusToken(%q) = %q, want %q", status, gotTok, wantTok)
		}
	}
	// No status borrows another's token: every resolved token above is
	// distinct (theme.StatusTokens itself has no duplicates, and the map
	// literal's keys guarantee status uniqueness, so this is really
	// checking the identity-cast mapping never collapses two statuses onto
	// the same token).
	seen := make(map[theme.Token]string, len(want))
	for status, tok := range want {
		if other, dup := seen[tok]; dup {
			t.Fatalf("statuses %q and %q both map to token %q", status, other, tok)
		}
		seen[tok] = status
	}
	for _, unknown := range []string{"", "paused", "STOPPED", "waiting "} {
		if _, ok := statusToken(unknown); ok {
			t.Fatalf("statusToken(%q) = (_, true), want ok=false for an unrecognised status", unknown)
		}
	}
}

// TestColorTokenRespectsNoColor asserts colorToken's NO_COLOR gate: with
// Color=false, text is returned byte-for-byte unmodified -- no SGR escape
// sneaks in through the theme path any more than it does through the
// existing m.color().
func TestColorTokenRespectsNoColor(t *testing.T) {
	m := Model{settings: config.Settings{Color: false}}
	got := m.colorToken(theme.Running, "running")
	if got != "running" {
		t.Fatalf("colorToken with Color=false = %q, want unmodified %q", got, "running")
	}
}

// TestColorTokenEmitsTruecolorByDefault asserts that with colour on and no
// DECK_COLOR_DEPTH override, colorToken emits a 38;2;r;g;b truecolour SGR
// escape carrying the active theme's exact authored hex for the token, and
// that two different tokens the active theme colours differently produce
// visibly different escapes (proving this is reading the theme, not a
// constant).
func TestColorTokenEmitsTruecolorByDefault(t *testing.T) {
	m := Model{settings: config.Settings{Color: true}}
	th := m.activeTheme()
	runningHex, err := th.Color(theme.Running)
	if err != nil {
		t.Fatalf("default theme lacks token %q: %v", theme.Running, err)
	}
	r, g, b, err := theme.HexRGB(runningHex)
	if err != nil {
		t.Fatalf("theme.HexRGB(%q): %v", runningHex, err)
	}
	got := m.colorToken(theme.Running, "running")
	wantEscape := sgrTruecolor(r, g, b)
	if !strings.Contains(got, wantEscape) {
		t.Fatalf("colorToken(running, ...) = %q, want it to contain %q (the active theme's running colour)", got, wantEscape)
	}
	if !strings.Contains(got, "running") {
		t.Fatalf("colorToken(running, ...) = %q lost the original text", got)
	}
	errorHex, err := th.Color(theme.Error)
	if err != nil {
		t.Fatalf("default theme lacks token %q: %v", theme.Error, err)
	}
	if errorHex == runningHex {
		t.Skip("this theme's running and error tokens happen to share a colour; the distinctness assertion below would be vacuous")
	}
	gotError := m.colorToken(theme.Error, "error")
	if strings.Contains(gotError, wantEscape) {
		t.Fatalf("colorToken(error, ...) = %q used running's colour escape %q -- statuses must not borrow each other's colour", gotError, wantEscape)
	}
}

// TestColorTokenUsesQuantizedPaletteAtDepth16 asserts DECK_COLOR_DEPTH=16
// (m.settings.ColorDepth == "16") makes colorToken read the theme's
// QUANTIZED colour and emit the matching bare ANSI 16-colour SGR code
// (never a truecolour 38;2 escape), so a 16-colour-floor render never
// claims a colour outside the reference palette.
func TestColorTokenUsesQuantizedPaletteAtDepth16(t *testing.T) {
	m := Model{settings: config.Settings{Color: true, ColorDepth: "16"}}
	th := m.activeTheme()
	quantHex, err := th.QuantizedColor(theme.Waiting)
	if err != nil {
		t.Fatalf("default theme lacks quantized token %q: %v", theme.Waiting, err)
	}
	code, ok := theme.ANSI16Code(quantHex)
	if !ok {
		t.Fatalf("quantized colour %q is not one of ReferencePalette's 16 entries", quantHex)
	}
	got := m.colorToken(theme.Waiting, "waiting")
	wantEscape := sgrCode16(code)
	if !strings.Contains(got, wantEscape) {
		t.Fatalf("colorToken at depth 16 = %q, want it to contain %q", got, wantEscape)
	}
	if strings.Contains(got, "38;2;") {
		t.Fatalf("colorToken at depth 16 = %q emitted a truecolour escape, want only the bare ANSI code", got)
	}
}

func sgrTruecolor(r, g, b int) string {
	return "\x1b[38;2;" + itoa(r) + ";" + itoa(g) + ";" + itoa(b) + "m"
}

func sgrCode16(code int) string {
	return "\x1b[" + itoa(code) + "m"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [8]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
