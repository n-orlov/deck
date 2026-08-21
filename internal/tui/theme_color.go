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
// NO_COLOR/DECK_COLOR (m.settings.Color, exactly like m.color()) and
// DECK_COLOR_DEPTH: "16" reads the theme's already-quantised colour and
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
