package tui

import (
	"strings"
	"testing"

	"github.com/mattn/go-runewidth"
	"github.com/n-orlov/deck/internal/theme"
)

// floorTokenFor maps a Go identifier from internal/theme's own token
// constants (as it appears in R84's floor table, e.g. `Hint`,
// `SelectionIdle`) onto the theme.Token value it names, by matching
// against theme.AllTokens with underscores removed and case folded --
// never by rebuilding the string by hand. An identifier that matches no
// token is a hard failure: silently dropping it would shrink the set of
// foregrounds this test accepts on the selection background, i.e. make
// the test stricter in a way whose diagnosis is confusing, or (if the
// table were read differently) let a cell through unchecked.
func floorTokenFor(t *testing.T, ident string) theme.Token {
	t.Helper()
	want := strings.ToLower(ident)
	for _, tok := range theme.AllTokens {
		if strings.ReplaceAll(string(tok), "_", "") == want {
			return tok
		}
	}
	t.Fatalf("R84's floor table lists %q, which is not one of internal/theme's own tokens (%v) -- the table and the token set have drifted", ident, theme.AllTokens)
	return theme.Token("")
}

// TestDialogSelectionCellsRenderOnlyFloorTokens is the render-level half
// of task 1204's completeness proof, and the half that holds no matter
// HOW a token reaches a focused row.
//
// TestDialogSelectionRenderersComposeOnlyFloorTokens (the static, AST
// half) reasons about the source: which tokens the two
// bgColorToken(theme.Selection, ...) sites can be given. This test
// instead renders every themed dialog for real -- reusing
// dialogDegradationCases(), the tracked table that already enumerates
// every dialog this phase themed together with the model builder each
// dialog's own theme test uses -- once per built-in theme, feeds the body
// through a vt.Emulator, and inspects the resulting GRID: every cell
// whose background is that theme's `selection` colour must carry a
// foreground that is one of the colours R84's floor table
// (internal/theme/contrast_test.go's dialogSelectionTokens, read live out
// of that file's AST by loadThemeSelectionFloorTokens -- never copied
// here) actually holds against `selection`.
//
// Because it reads the finished cells, no source-level trick evades it:
// a `segs[i].Tok = theme.Dimmed` written after the composite literal, a
// token handed in by a helper the static pass does not walk, a token
// computed at run time -- all of them land on the grid as a foreground
// colour that is not in the floor table's set, and fail here. Paired with
// internal/theme's TestThemedDialogTokensClearContrastFloor (which holds
// every listed token to >= 3.0:1 on every built-in, hex and 16-colour
// quantisation alike, with no allowlist), the two together mean a cell
// drawn on a dialog's focused row is either held to the floor or red.
//
// theme.SelectionIdle is out of scope here for the same reason as in the
// floor table itself: no dialog draws on it (panel.go:134-139 and
// settings.go:1318 are the only backgrounds that use it -- the sidebar's
// unfocused selection marker and settings' own rows), so it is a finding
// (task 1208), not a pair this floor must hold.
func TestDialogSelectionCellsRenderOnlyFloorTokens(t *testing.T) {
	floorIdents := sortedKeys(loadThemeSelectionFloorTokens(t))
	if len(floorIdents) == 0 {
		t.Fatal("R84's floor table resolved to zero tokens -- refusing to accept any foreground on the selection background")
	}
	cases := dialogDegradationCases()
	if len(cases) == 0 {
		t.Fatal("dialogDegradationCases() is empty -- this test would render nothing and prove nothing")
	}

	builtins := theme.Builtins()
	if len(builtins) == 0 {
		t.Fatal("theme.Builtins() is empty -- nothing to check")
	}

	selectionSeen := false
	for _, th := range builtins {
		th := th
		t.Run(th.Name, func(t *testing.T) {
			selHex, err := th.Color(theme.Selection)
			if err != nil {
				t.Fatalf("theme %q lacks selection: %v", th.Name, err)
			}
			allowed := map[string]string{}
			for _, ident := range floorIdents {
				tok := floorTokenFor(t, ident)
				hex, err := th.Color(tok)
				if err != nil {
					t.Fatalf("theme %q lacks %q: %v", th.Name, tok, err)
				}
				allowed[hex] = string(tok)
			}

			for _, tc := range cases {
				for _, mtc := range tc.mutations {
					m := tc.build(t)
					m.settings.Theme = th
					mtc.mut(&m)
					body := tc.styled(m)
					lines := strings.Split(body, "\n")
					width := m.width
					for _, line := range lines {
						if w := runewidth.StringWidth(stripANSI(line)) + 1; w > width {
							width = w
						}
					}
					term := renderSettingsToEmulator(t, body, width, len(lines)+1)
					for row := 0; row < len(lines); row++ {
						for col := 0; col < width; col++ {
							cell := term.CellAt(col, row)
							if cell == nil || strings.TrimSpace(cell.Content) == "" {
								continue
							}
							bg, ok := cellBgHex(t, term, col, row)
							if !ok || bg != selHex {
								continue
							}
							selectionSeen = true
							fg, ok := cellFgHex(t, term, col, row)
							if !ok {
								t.Errorf("theme %q, %s (%s): cell (%d,%d) %q is drawn on the selection background with no explicit foreground -- its colour comes from the terminal's default, which R84's floor cannot hold; render it through a floor token",
									th.Name, tc.name, mtc.name, col, row, cell.Content)
								continue
							}
							if _, ok := allowed[fg]; !ok {
								t.Errorf("theme %q, %s (%s): cell (%d,%d) %q renders foreground %s on the selection background %s, which is not one of R84's floor tokens for that background (%v = %v) -- either draw it in a listed token or add its token to %s's %s so the floor actually holds the pair",
									th.Name, tc.name, mtc.name, col, row, cell.Content, fg, selHex, floorIdents, sortedKeys(hexKeys(allowed)), themeContrastTestPath, themeFloorTokensVar)
							}
						}
					}
				}
			}
		})
	}
	if !selectionSeen {
		t.Fatalf("no dialog rendered a single cell on the selection background across %d built-in theme(s) -- the focused-row treatment SPEC.md:1355 requires is not reaching the grid, so this proof is vacuous", len(builtins))
	}
}

// hexKeys turns allowed's hex->token map into a token-name set, for the
// failure message above.
func hexKeys(m map[string]string) map[string]bool {
	out := map[string]bool{}
	for _, v := range m {
		out[v] = true
	}
	return out
}
