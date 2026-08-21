package tui

import (
	"fmt"

	"github.com/n-orlov/deck/internal/theme"
)

// activeTheme is the theme the current render uses: whatever config.Load
// resolved (never nil — see config.Settings.Theme's doc), or theme.Default()
// for a Settings value built by hand (e.g. many existing tests construct
// config.Settings{...} literals without a Theme field, and must still get a
// real, complete theme rather than a nil-map panic).
func (m Model) activeTheme() *theme.Theme {
	if m.settings.Theme != nil {
		return m.settings.Theme
	}
	return theme.Default()
}

// statusToken resolves a session status word to its matching §7 status
// token (task 014, SPEC §11.6). Token values are spelled identically to
// their status strings (see internal/theme/token.go's Waiting = "waiting"
// etc — theme's own TestStatusTokensMatchSevenStatuses pins this), so this
// is a validated identity cast rather than a second hand-written table:
// checking status against theme.StatusTokens is what makes it exhaustive
// over exactly §7's seven statuses — a status string with no matching
// entry in theme.StatusTokens (a typo, or a new status added to store/tui
// without updating theme.StatusTokens) comes back ok=false, and callers
// must not borrow another status's colour for it.
func statusToken(status string) (theme.Token, bool) {
	tok := theme.Token(status)
	for _, want := range theme.StatusTokens {
		if want == tok {
			return tok, true
		}
	}
	return "", false
}

// colorToken renders text in tok's colour from the active theme, honouring
// NO_COLOR/DECK_COLOR (m.settings.Color, the same gate the removed
// placeholder m.color() used) and DECK_COLOR_DEPTH: "16" reads the theme's already-quantised colour and
// emits the matching ANSI 16-colour SGR code (§11.6's 16-colour floor),
// anything else (including the unset auto-detect case) reads the theme's
// authored hex and emits a truecolour SGR escape. A token the active theme
// somehow lacks, or a quantised colour that is not exactly one of
// ReferencePalette's 16 entries (defensive only — Theme.Quantized is
// always exactly that), returns text unmodified rather than guessing a
// colour.
func (m Model) colorToken(tok theme.Token, text string) string {
	if !m.settings.Color {
		return text
	}
	th := m.activeTheme()
	sgr, ok := m.sgrForToken(th, tok)
	if !ok {
		return text
	}
	return sgr + text + "\x1b[0m"
}

// sgrForToken looks up tok's colour in th at the active colour depth and
// renders the matching SGR escape. Factored out of colorToken so it can be
// unit-tested independently of NO_COLOR gating.
func (m Model) sgrForToken(th *theme.Theme, tok theme.Token) (string, bool) {
	if m.settings.ColorDepth == "16" {
		hex, err := th.QuantizedColor(tok)
		if err != nil {
			return "", false
		}
		code, ok := theme.ANSI16Code(hex)
		if !ok {
			return "", false
		}
		return fmt.Sprintf("\x1b[%dm", code), true
	}
	hex, err := th.Color(tok)
	if err != nil {
		return "", false
	}
	r, g, b, err := theme.HexRGB(hex)
	if err != nil {
		return "", false
	}
	return fmt.Sprintf("\x1b[38;2;%d;%d;%dm", r, g, b), true
}

// bgSgrForToken mirrors sgrForToken exactly but renders tok's colour as a
// BACKGROUND SGR (48;2;r;g;b truecolour, or the bare 16-colour background
// code -- 10 above the matching foreground code, e.g. 30->40, 90->100,
// standard ANSI) rather than a foreground one. Task 019 is the first
// caller: the settings takeover's selection/selection_idle focus cue
// (SPEC requirement 42) highlights a row's background, not its text
// colour.
func (m Model) bgSgrForToken(th *theme.Theme, tok theme.Token) (string, bool) {
	if m.settings.ColorDepth == "16" {
		hex, err := th.QuantizedColor(tok)
		if err != nil {
			return "", false
		}
		code, ok := theme.ANSI16Code(hex)
		if !ok {
			return "", false
		}
		return fmt.Sprintf("\x1b[%dm", code+10), true
	}
	hex, err := th.Color(tok)
	if err != nil {
		return "", false
	}
	r, g, b, err := theme.HexRGB(hex)
	if err != nil {
		return "", false
	}
	return fmt.Sprintf("\x1b[48;2;%d;%d;%dm", r, g, b), true
}

// foregroundSGR/backgroundSGR are colorToken/bgColorToken's underlying
// "just the open sequence, honouring NO_COLOR" halves, exposed so a caller
// composing several tokens onto one row (settingsRenderRow) can open more
// than one attribute and close them all with a SINGLE trailing reset,
// rather than stacking colorToken's own self-resetting calls -- an SGR
// \x1b[0m reset clears every attribute, background included, so nesting
// resets would silently cancel an outer row background the moment an
// inner foreground segment ended.
func (m Model) foregroundSGR(tok theme.Token) (string, bool) {
	if !m.settings.Color {
		return "", false
	}
	return m.sgrForToken(m.activeTheme(), tok)
}

func (m Model) backgroundSGR(tok theme.Token) (string, bool) {
	if !m.settings.Color {
		return "", false
	}
	return m.bgSgrForToken(m.activeTheme(), tok)
}

// bgColorToken renders text with tok's colour as a BACKGROUND SGR (see
// bgSgrForToken) rather than colorToken's foreground -- the standalone,
// self-resetting form for a caller that only needs one token on one run of
// text, mirroring colorToken's own shape exactly.
func (m Model) bgColorToken(tok theme.Token, text string) string {
	seq, ok := m.backgroundSGR(tok)
	if !ok {
		return text
	}
	return seq + text + "\x1b[0m"
}
