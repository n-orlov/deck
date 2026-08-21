package features

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/x/vt"
)

// TestTokenFormAssertionsResolveThroughPinnedThemeAndCanFail is task 013's
// explicit negative proof for the token-named requirement-1 steps
// (cellHasForegroundToken and its siblings in cell_attributes_test.go): a
// step given the WRONG token name must fail rather than pass by accident,
// exactly as TestCellAttributeAssertionsCanFail already proves for the
// hex-literal steps. It builds a minimal ScenarioHarness (a temp DECK_HOME
// carrying a config.toml that pins a scenario-written user theme, and a
// fake named client backed by a bare vt.Emulator painted with one token's
// colour) directly, without starting a real deck process or going through
// godog, so the assertion functions themselves -- not the theme package,
// already covered by internal/theme's own tests -- are what gets exercised.
func TestTokenFormAssertionsResolveThroughPinnedThemeAndCanFail(t *testing.T) {
	home := t.TempDir()

	// A user theme with two tokens deliberately set to different, easily
	// distinguished colours, so a step naming the wrong token is provably
	// checking against the wrong expectation rather than coincidentally
	// matching.
	themeTOML := `name = "token-proof"
appearance = "dark"

[colors]
background        = "#000000"
surface           = "#000000"
border            = "#000000"
border_focus      = "#000000"
selection         = "#000000"
selection_idle    = "#000000"
title             = "#000000"
text              = "#111111"
dimmed            = "#000000"
hint              = "#000000"
key               = "#000000"
accent            = "#00ff00"
group             = "#000000"
search_match      = "#000000"
badge             = "#000000"
badge_warn        = "#000000"
waiting           = "#000000"
running           = "#000000"
idle              = "#000000"
starting          = "#000000"
stopped           = "#000000"
error             = "#000000"
archived          = "#000000"
`
	cfgPath := filepath.Join(home, "config.toml")
	if err := os.WriteFile(cfgPath, []byte(`[ui]
theme = "token-proof"
`), 0o600); err != nil {
		t.Fatalf("write config.toml: %v", err)
	}
	themesDir := filepath.Join(home, "themes")
	if err := os.MkdirAll(themesDir, 0o700); err != nil {
		t.Fatalf("create themes dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(themesDir, "token-proof.toml"), []byte(themeTOML), 0o600); err != nil {
		t.Fatalf("write user theme: %v", err)
	}

	// The fake client's grid carries the "accent" token's colour
	// (#00ff00) at column 0 -- exactly what a real deck client would
	// carry if it painted that cell from the "accent" token, once tasks
	// 014-018 wire theme rendering in.
	terminal := vt.NewEmulator(4, 1)
	if _, err := terminal.Write([]byte("\x1b[38;2;0;255;0mX\x1b[0m")); err != nil {
		t.Fatalf("paint fake client cell: %v", err)
	}

	h := &ScenarioHarness{
		Home:         home,
		namedClients: map[string]*ScreenDriver{"fake": {screen: terminal}},
	}
	ctx := context.WithValue(context.Background(), scenarioHarnessKey{}, h)

	// The correct token resolves and matches: the positive assertion
	// passes and the negative assertion (naming that SAME token) fails.
	if err := cellHasForegroundToken(ctx, "fake", 0, 0, "accent"); err != nil {
		t.Fatalf("cellHasForegroundToken(accent) on the accent-coloured cell: %v", err)
	}
	if err := cellDoesNotHaveForegroundToken(ctx, "fake", 0, 0, "accent"); err == nil {
		t.Fatal("want cellDoesNotHaveForegroundToken(accent) to fail on the accent-coloured cell, got nil")
	}

	// The wrong token ("text", #111111) resolves to a DIFFERENT colour
	// than the cell actually carries: the positive assertion must fail
	// (never pass by falling back to some default), and the negative
	// assertion naming the wrong token correctly passes.
	if err := cellHasForegroundToken(ctx, "fake", 0, 0, "text"); err == nil {
		t.Fatal("want cellHasForegroundToken(text) to fail on the accent-coloured cell, got nil")
	}
	if err := cellDoesNotHaveForegroundToken(ctx, "fake", 0, 0, "text"); err != nil {
		t.Fatalf("cellDoesNotHaveForegroundToken(text) on the accent-coloured cell: %v", err)
	}

	// An unknown token name is a resolution error, not a fabricated
	// default colour to compare against.
	if err := cellHasForegroundToken(ctx, "fake", 0, 0, "not-a-real-token"); err == nil {
		t.Fatal("want cellHasForegroundToken(not-a-real-token) to fail with a resolution error, got nil")
	}

	// The same discrimination holds through the text-matching form.
	if err := textHasForegroundToken(ctx, "fake", "X", "accent"); err != nil {
		t.Fatalf("textHasForegroundToken(accent) on text \"X\": %v", err)
	}
	if err := textHasForegroundToken(ctx, "fake", "X", "text"); err == nil {
		t.Fatal("want textHasForegroundToken(text) to fail on text \"X\" coloured by accent, got nil")
	}

	// Sanity: resolveScenarioTokenHex itself returns the theme's authored
	// value, not some quantised or otherwise transformed one, confirming
	// the comparison above is against the real per-token colour and not
	// an accident of the two tokens' particular hex values.
	got, err := resolveScenarioTokenHex(ctx, "accent")
	if err != nil {
		t.Fatalf("resolveScenarioTokenHex(accent): %v", err)
	}
	if want := "#00ff00"; got != want {
		t.Fatalf("resolveScenarioTokenHex(accent) = %s, want %s", got, want)
	}
	fmt.Println("token-form assertions discriminate: accent=" + got)
}
