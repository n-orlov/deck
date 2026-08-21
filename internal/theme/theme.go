package theme

import "fmt"

// Theme is one loaded theme: a name, an appearance ("dark" or "light",
// driving contrast direction per §11.6), and a colour for every token in
// AllTokens. Colors always holds hex strings ("#rrggbb", lower-case, as
// written in the TOML source) — quantisation to the 16-colour floor
// (task 010) is applied by the renderer, not stored here, so a truecolour
// terminal always sees the theme's exact authored values.
type Theme struct {
	Name       string
	Appearance string
	Colors     map[Token]string
}

// Color returns the hex string for tok, or an error if the theme somehow
// lacks it. Themes constructed via Parse are guaranteed complete, so this
// only defends against a Theme built by hand elsewhere.
func (t *Theme) Color(tok Token) (string, error) {
	v, ok := t.Colors[tok]
	if !ok {
		return "", fmt.Errorf("theme %q: missing token %q", t.Name, tok)
	}
	return v, nil
}

// Validate checks that Colors carries exactly AllTokens: no missing
// token, no invented one.
func (t *Theme) Validate() error {
	seen := make(map[Token]bool, len(t.Colors))
	for tok := range t.Colors {
		if !isKnownToken(tok) {
			return fmt.Errorf("theme %q: unknown token %q", t.Name, tok)
		}
		seen[tok] = true
	}
	for _, tok := range AllTokens {
		if !seen[tok] {
			return fmt.Errorf("theme %q: missing token %q", t.Name, tok)
		}
	}
	if t.Appearance != "dark" && t.Appearance != "light" {
		return fmt.Errorf("theme %q: appearance must be \"dark\" or \"light\", got %q", t.Name, t.Appearance)
	}
	return nil
}
