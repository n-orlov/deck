package features

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/vt"
	"github.com/cucumber/godog"

	"github.com/n-orlov/deck/internal/theme"
)

// registerThemePinningSteps exposes requirement 4's theme-pinning harness
// prerequisite: a scenario can pin a named BUILT-IN theme via a config.toml
// it writes itself, or discover a USER theme file it writes into
// $XDG_CONFIG_HOME/deck/themes (here, the scenario's own DECK_HOME/themes --
// see theme.ThemesDir), and then have that resolution's actual colours
// painted, one token per column, onto a named bare vt emulator so the
// existing per-cell foreground/background/attribute steps (requirement 1)
// can assert against them directly.
//
// This deliberately does not start a real deck client: nothing in
// internal/tui consumes internal/theme yet (that lands in later tasks), so
// asserting against a live client's own render would currently prove
// nothing either way. What requirement 4 asks to be "drivable" is the
// DISCOVERY path -- config.toml naming a built-in, and a scenario-written
// file under themes/ naming a user theme -- and this is that path, exercised
// with the exact loader (theme.DiscoverUserThemes, theme.Resolve) production
// code will call once wired.
func registerThemePinningSteps(sc *godog.ScenarioContext) {
	sc.Step(`^the scenario's config\.toml selects theme "([^"]*)"$`, scenarioConfigSelectsTheme)
	sc.Step(`^the scenario writes user theme file "([^"]+)" into its themes directory with:$`, scenarioWritesUserThemeFile)
	sc.Step(`^the scenario's config-selected theme is painted onto a fresh swatch emulator sized (\d+)x(\d+) named "([^"]+)"$`, paintConfigSelectedThemeSwatch)
	sc.Step(`^the "([^"]+)" swatch emulator has foreground "(#[0-9a-fA-F]{6})" for token "([^"]+)"$`, swatchTokenHasForeground)
	sc.Step(`^the "([^"]+)" swatch emulator does not have foreground "(#[0-9a-fA-F]{6})" for token "([^"]+)"$`, swatchTokenDoesNotHaveForeground)
}

// scenarioConfigSelectsTheme writes (or overwrites) the scenario's
// config.toml with nothing but a `[ui] theme = "<name>"` declaration. This
// is deliberately the same file task 003's writer targets
// (filepath.Join(h.Home, "config.toml")), so a real client started
// afterwards would see the identical file a settings-schema-driven parser
// (task 027/028) will one day read theme from.
func scenarioConfigSelectsTheme(ctx context.Context, name string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	content := fmt.Sprintf("[ui]\ntheme = %q\n", name)
	path := filepath.Join(h.Home, "config.toml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return fmt.Errorf("write scenario config.toml selecting theme %q: %w", name, err)
	}
	return nil
}

// scenarioWritesUserThemeFile writes doc's raw content verbatim to
// filename under the scenario's themes directory (theme.ThemesDir of the
// scenario's own config.toml path), creating the directory if it does not
// exist yet -- exactly the layout DiscoverUserThemes scans at start-up.
func scenarioWritesUserThemeFile(ctx context.Context, filename string, doc *godog.DocString) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	dir := theme.ThemesDir(filepath.Join(h.Home, "config.toml"))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create scenario themes directory: %w", err)
	}
	content := doc.Content
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return fmt.Errorf("write scenario user theme file %q: %w", filename, err)
	}
	return nil
}

// configSelectedThemeName re-reads the scenario's config.toml and extracts
// the bare `theme = "..."` value out of its `[ui]` table -- a minimal,
// harness-local stand-in for the schema-driven config parser task 027/028
// will add, scoped deliberately to test code because production wiring is
// not this task's job. An absent file or an absent [ui] theme line both
// yield "" (the "nothing configured" case theme.Resolve already documents),
// never an error, so a scenario that never calls scenarioConfigSelectsTheme
// still gets the default theme rather than a spurious failure.
func configSelectedThemeName(h *ScenarioHarness) (string, error) {
	path := filepath.Join(h.Home, "config.toml")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read scenario config.toml: %w", err)
	}
	section := ""
	for _, raw := range strings.Split(string(data), "\n") {
		text := strings.TrimSpace(raw)
		if idx := strings.Index(text, "#"); idx >= 0 {
			text = strings.TrimSpace(text[:idx])
		}
		if text == "" {
			continue
		}
		if strings.HasPrefix(text, "[") && strings.HasSuffix(text, "]") {
			section = strings.TrimSpace(text[1 : len(text)-1])
			continue
		}
		if section != "ui" {
			continue
		}
		idx := strings.Index(text, "=")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(text[:idx])
		if key != "theme" {
			continue
		}
		value := strings.TrimSpace(text[idx+1:])
		unquoted, err := strconv.Unquote(value)
		if err != nil {
			return "", fmt.Errorf("scenario config.toml [ui] theme value %q is not a quoted string: %w", value, err)
		}
		return unquoted, nil
	}
	return "", nil
}

// swatchScenarioKey stores every named swatch emulator a scenario has
// painted, so several can coexist (e.g. one per pinned theme) and be
// compared against each other within one scenario -- see harness.feature's
// "proving the step is not a no-op" scenario.
type swatchScenarioKey struct{}

func swatchEmulators(ctx context.Context) map[string]vt.Terminal {
	m, _ := ctx.Value(swatchScenarioKey{}).(map[string]vt.Terminal)
	return m
}

// paintConfigSelectedThemeSwatch resolves the scenario's currently
// config-selected theme name (via configSelectedThemeName) against the
// scenario's own themes directory (via theme.DiscoverUserThemes) exactly as
// theme.Resolve documents -- user theme wins on a name collision, an
// unknown/unparseable name falls back to the default -- then paints every
// token in theme.AllTokens order, one token per column, as a truecolour SGR
// foreground on a distinguishing character, onto a freshly created bare vt
// emulator registered under name. This exercises BOTH discovery paths
// requirement 4 asks for: a built-in reached by name alone, and a user
// theme file reached by DiscoverUserThemes scanning the themes directory.
func paintConfigSelectedThemeSwatch(ctx context.Context, width, height int, name string) (context.Context, error) {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return ctx, err
	}
	themeName, err := configSelectedThemeName(h)
	if err != nil {
		return ctx, err
	}
	dir := theme.ThemesDir(filepath.Join(h.Home, "config.toml"))
	userThemes, userErrs := theme.DiscoverUserThemes(dir)
	resolved, _ := theme.Resolve(userThemes, userErrs, themeName)

	terminal := vt.NewEmulator(width, height)
	var b strings.Builder
	for _, tok := range theme.AllTokens {
		hex, err := resolved.Color(tok)
		if err != nil {
			return ctx, fmt.Errorf("paint swatch %q: %w", name, err)
		}
		r, g, bl, err := hexToRGB(hex)
		if err != nil {
			return ctx, fmt.Errorf("paint swatch %q token %q: %w", name, tok, err)
		}
		fmt.Fprintf(&b, "\x1b[38;2;%d;%d;%dm#\x1b[0m", r, g, bl)
	}
	if _, err := terminal.Write([]byte(b.String())); err != nil {
		return ctx, fmt.Errorf("paint swatch %q: %w", name, err)
	}

	m := swatchEmulators(ctx)
	if m == nil {
		m = make(map[string]vt.Terminal)
	}
	m[name] = terminal
	return context.WithValue(ctx, swatchScenarioKey{}, m), nil
}

// hexToRGB parses a "#rrggbb" string as produced by theme.Parse (always
// lower-case, always six hex digits -- see internal/theme/parse.go).
func hexToRGB(hex string) (r, g, b int, err error) {
	if len(hex) != 7 || hex[0] != '#' {
		return 0, 0, 0, fmt.Errorf("not a #rrggbb colour: %q", hex)
	}
	v, err := strconv.ParseInt(hex[1:], 16, 32)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("not a #rrggbb colour: %q: %w", hex, err)
	}
	return int(v >> 16 & 0xff), int(v >> 8 & 0xff), int(v & 0xff), nil
}

// tokenColumn returns the fixed column paintConfigSelectedThemeSwatch wrote
// tok to: its index in theme.AllTokens, since the swatch writes exactly one
// cell per token in that fixed order.
func tokenColumn(tok theme.Token) (int, error) {
	for i, t := range theme.AllTokens {
		if t == tok {
			return i, nil
		}
	}
	return 0, fmt.Errorf("unknown token %q", tok)
}

// swatchCellForToken looks up the named swatch and the fixed column
// paintConfigSelectedThemeSwatch wrote tok's colour to (via tokenColumn),
// so feature files name a token rather than a magic column number.
func swatchCellForToken(ctx context.Context, name, tokenName string) (*uv.Cell, error) {
	m := swatchEmulators(ctx)
	terminal, ok := m[name]
	if !ok {
		return nil, fmt.Errorf("no swatch emulator named %q painted in this scenario", name)
	}
	column, err := tokenColumn(theme.Token(tokenName))
	if err != nil {
		return nil, err
	}
	return terminal.CellAt(column, 0), nil
}

func swatchTokenHasForeground(ctx context.Context, name, want, tokenName string) error {
	cell, err := swatchCellForToken(ctx, name, tokenName)
	if err != nil {
		return err
	}
	got, err := cellForegroundHex(cell)
	if err != nil {
		return fmt.Errorf("swatch %q token %q: %w", name, tokenName, err)
	}
	if got != want {
		return fmt.Errorf("swatch %q token %q has foreground %s, want %s", name, tokenName, got, want)
	}
	return nil
}

func swatchTokenDoesNotHaveForeground(ctx context.Context, name, unwanted, tokenName string) error {
	cell, err := swatchCellForToken(ctx, name, tokenName)
	if err != nil {
		return err
	}
	got, err := cellForegroundHex(cell)
	if err != nil {
		// No foreground set at all trivially satisfies "does not have".
		return nil
	}
	if got == unwanted {
		return fmt.Errorf("swatch %q token %q has foreground %s, want anything else", name, tokenName, got)
	}
	return nil
}
