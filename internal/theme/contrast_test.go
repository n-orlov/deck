package theme

import (
	"math"
	"testing"
)

// minContrastRatio is the floor a theme's declared colours (and their
// 16-colour quantisation) must clear against their background/selection
// per task 011: 3:1, not the stricter 4.5:1 WCAG AA text threshold —
// deck's chrome includes non-text glyphs and short status words rendered
// with bold/reverse attributes that the SPEC does not hold to full AA.
const minContrastRatio = 3.0

// contrastChecks lists the loader-level pairs task 011 requires: text,
// hint, title and each of the seven §7 status tokens against background,
// plus text against selection. This is deliberately NOT StatusTokens
// alone — Text/Hint/Title are checked explicitly, once, regardless of
// how the status list evolves.
func contrastChecks() []struct {
	label string
	fg    Token
	bg    Token
} {
	checks := []struct {
		label string
		fg    Token
		bg    Token
	}{
		{"text/background", Text, Background},
		{"hint/background", Hint, Background},
		{"title/background", Title, Background},
	}
	for _, st := range StatusTokens {
		checks = append(checks, struct {
			label string
			fg    Token
			bg    Token
		}{string(st) + "/background", st, Background})
	}
	checks = append(checks, struct {
		label string
		fg    Token
		bg    Token
	}{"text/selection", Text, Selection})
	return checks
}

// TestBuiltinContrastFloor is the loader-level WCAG contrast golden test
// (requirement 30): every built-in theme, text/hint/title and each of
// the seven §7 status tokens against background, plus text against
// selection, computed over BOTH the theme's authored hex palette and its
// §11.6 16-colour quantisation (task 010) — failing below 3:1 on either.
// The full ratio table is logged (via t.Log, visible with `go test -v`)
// so it can be pasted into the phase report per task 054.
func TestBuiltinContrastFloor(t *testing.T) {
	for _, th := range Builtins() {
		th := th
		t.Run(th.Name, func(t *testing.T) {
			for _, chk := range contrastChecks() {
				fgHex, err := th.Color(chk.fg)
				if err != nil {
					t.Fatalf("Color(%q): %v", chk.fg, err)
				}
				bgHex, err := th.Color(chk.bg)
				if err != nil {
					t.Fatalf("Color(%q): %v", chk.bg, err)
				}
				ratioHex, err := contrastRatio(fgHex, bgHex)
				if err != nil {
					t.Fatalf("contrastRatio(%q, %q): %v", fgHex, bgHex, err)
				}

				fgQ, err := th.QuantizedColor(chk.fg)
				if err != nil {
					t.Fatalf("QuantizedColor(%q): %v", chk.fg, err)
				}
				bgQ, err := th.QuantizedColor(chk.bg)
				if err != nil {
					t.Fatalf("QuantizedColor(%q): %v", chk.bg, err)
				}
				ratioQuant, err := contrastRatio(fgQ, bgQ)
				if err != nil {
					t.Fatalf("contrastRatio(%q, %q): %v", fgQ, bgQ, err)
				}

				t.Logf("%-8s %-20s hex %s/%s = %.2f:1   quant %s/%s = %.2f:1",
					th.Name, chk.label, fgHex, bgHex, ratioHex, fgQ, bgQ, ratioQuant)

				if ratioHex < minContrastRatio {
					t.Errorf("theme %q %s: hex contrast %.2f:1 < %.1f:1 (fg=%s bg=%s)",
						th.Name, chk.label, ratioHex, minContrastRatio, fgHex, bgHex)
				}
				if ratioQuant < minContrastRatio {
					t.Errorf("theme %q %s: quantised contrast %.2f:1 < %.1f:1 (fg=%s bg=%s)",
						th.Name, chk.label, ratioQuant, minContrastRatio, fgQ, bgQ)
				}
			}
		})
	}
}

// sessionRowSurfaceChecks lists every foreground token task 084's own
// success criteria names as "can appear on a session row" -- title
// (session name), dimmed (starting-row name override, line 2's created
// line, quality badge -- the token task 083 gave the most new work),
// text (plain segments), badge/badge_warn (marked/profile badges) and
// each of the seven §7 status tokens (the status word, the unseen
// glyph's fallback, and archived_at's own token) -- each checked against
// theme.Surface, the alternating stripe's own background (task 084), on
// top of contrastChecks()'s existing theme.Background checks. Surface is
// deliberately close to Background in every builtin theme (an elevated
// row, not a loud one), so a token that clears the floor against
// Background is not guaranteed to also clear it against Surface --
// this is the check that actually proves it does.
func sessionRowSurfaceChecks() []struct {
	label string
	fg    Token
	bg    Token
} {
	checks := []struct {
		label string
		fg    Token
		bg    Token
	}{
		{"title/surface", Title, Surface},
		{"dimmed/surface", Dimmed, Surface},
		{"text/surface", Text, Surface},
		{"badge/surface", Badge, Surface},
		{"badge_warn/surface", BadgeWarn, Surface},
	}
	for _, st := range StatusTokens {
		checks = append(checks, struct {
			label string
			fg    Token
			bg    Token
		}{string(st) + "/surface", st, Surface})
	}
	return checks
}

// TestSessionRowTokensClearContrastFloorOnSurface is task 084's own
// contrast obligation: every foreground token that can appear on a
// sidebar session row must clear minContrastRatio against theme.Surface
// (the alternating stripe's background), not only against
// theme.Background as TestBuiltinContrastFloor already checks -- over
// both the theme's authored hex palette and its 16-colour quantisation,
// exactly like TestBuiltinContrastFloor does for Background.
func TestSessionRowTokensClearContrastFloorOnSurface(t *testing.T) {
	for _, th := range Builtins() {
		th := th
		t.Run(th.Name, func(t *testing.T) {
			for _, chk := range sessionRowSurfaceChecks() {
				fgHex, err := th.Color(chk.fg)
				if err != nil {
					t.Fatalf("Color(%q): %v", chk.fg, err)
				}
				bgHex, err := th.Color(chk.bg)
				if err != nil {
					t.Fatalf("Color(%q): %v", chk.bg, err)
				}
				ratioHex, err := contrastRatio(fgHex, bgHex)
				if err != nil {
					t.Fatalf("contrastRatio(%q, %q): %v", fgHex, bgHex, err)
				}

				fgQ, err := th.QuantizedColor(chk.fg)
				if err != nil {
					t.Fatalf("QuantizedColor(%q): %v", chk.fg, err)
				}
				bgQ, err := th.QuantizedColor(chk.bg)
				if err != nil {
					t.Fatalf("QuantizedColor(%q): %v", chk.bg, err)
				}
				ratioQuant, err := contrastRatio(fgQ, bgQ)
				if err != nil {
					t.Fatalf("contrastRatio(%q, %q): %v", fgQ, bgQ, err)
				}

				t.Logf("%-8s %-20s hex %s/%s = %.2f:1   quant %s/%s = %.2f:1",
					th.Name, chk.label, fgHex, bgHex, ratioHex, fgQ, bgQ, ratioQuant)

				if ratioHex < minContrastRatio {
					t.Errorf("theme %q %s: hex contrast %.2f:1 < %.1f:1 (fg=%s bg=%s)",
						th.Name, chk.label, ratioHex, minContrastRatio, fgHex, bgHex)
				}
				if ratioQuant < minContrastRatio {
					t.Errorf("theme %q %s: quantised contrast %.2f:1 < %.1f:1 (fg=%s bg=%s)",
						th.Name, chk.label, ratioQuant, minContrastRatio, fgQ, bgQ)
				}
			}
		})
	}
}

// dialogSurfaceChecks lists the three token/Surface pairs R84 adds beyond
// sessionRowSurfaceChecks's own set: hint (a dialog field's label), key
// (a dialog footer's keycap) and error (a dialog's validation message) —
// none of §11.6's dialog chrome, all newly drawn on theme.Surface once
// R82 themes a dialog body.
func dialogSurfaceChecks() []struct {
	label string
	fg    Token
	bg    Token
} {
	return []struct {
		label string
		fg    Token
		bg    Token
	}{
		{"hint/surface", Hint, Surface},
		{"key/surface", Key, Surface},
		{"error/surface", Error, Surface},
	}
}

// dialogSelectionTokens is the exact token set a themed dialog (R82)
// actually composes over theme.Selection today: renderCreateRowSegments
// (tui.go) and renderRenameFieldRow (rename.go) are the ONLY two sites in
// internal/tui that call bgColorToken(theme.Selection, ...) -- see
// TestDialogSelectionRenderersComposeOnlyFloorTokens in internal/tui,
// which enumerates both by AST and fails if either is ever made to
// compose a token this slice does not list, and its render-level twin
// TestDialogSelectionCellsRenderOnlyFloorTokens, which renders every
// themed dialog on every built-in into a terminal emulator and fails if
// any CELL on the selection background carries a foreground this slice
// does not list -- so a token that reaches a focused row by a route no
// static pass follows (a helper, a later `segs[i].Tok = ...`, a run-time
// value) is caught too. Both tests read THIS declaration out of this
// file's own AST rather than keeping a second copy of the set, so the
// floor table and its completeness proofs cannot drift: renaming or
// emptying this variable fails them outright.
// Both sites compose only a field's label (hint) and its value (text)
// onto that background; no
// current dialog puts dimmed (per-field help), key (a footer keycap) or
// error (a validation line) on theme.Selection, so R84's "every text
// token" is, in practice, these two.
//
// text/selection is already hard-enforced (no allowlist) by
// TestBuiltinContrastFloor's own contrastChecks; it is repeated here so
// this table is self-contained for the pairs a themed dialog draws.
//
// theme.SelectionIdle is deliberately OUT of scope: per panel.go:134-139
// and settings.go:1318, SelectionIdle only ever backs the sidebar's own
// unfocused-panel selection marker and settings' own rows -- no dialog
// draws any token over it, so a sub-floor SelectionIdle cell (several
// exist, e.g. empire's 16-colour quantisation collapsing several
// foreground tokens onto SelectionIdle's own #7f7f7f reference colour)
// is not a pair this floor needs to hold, and is reported instead as a
// finding (task 1208), alongside the sidebar's own pre-existing sub-floor
// dimmed/selection marker (tui.go:4292) which sits outside R84's dialog
// scope entirely.
var dialogSelectionTokens = []Token{Hint, Text}

// dialogSelectionChecks pairs every dialogSelectionTokens entry against
// theme.Selection, the one background a dialog's focused field row
// actually carries (see dialogSelectionTokens's own doc comment for why
// SelectionIdle is excluded).
func dialogSelectionChecks() []struct {
	label string
	fg    Token
	bg    Token
} {
	checks := make([]struct {
		label string
		fg    Token
		bg    Token
	}, 0, len(dialogSelectionTokens))
	for _, tok := range dialogSelectionTokens {
		checks = append(checks, struct {
			label string
			fg    Token
			bg    Token
		}{string(tok) + "/selection", tok, Selection})
	}
	return checks
}

// TestThemedDialogTokensClearContrastFloor is R84's own contrast
// obligation (task 106, tightened by task 1204): the pairs a themed
// dialog (R82) actually draws that neither TestBuiltinContrastFloor nor
// TestSessionRowTokensClearContrastFloorOnSurface cover -- hint/surface,
// key/surface, error/surface (dialogSurfaceChecks), and hint/text over
// theme.Selection (dialogSelectionChecks, see dialogSelectionTokens's own
// doc comment for exactly which tokens and why SelectionIdle is out of
// scope) -- over both the theme's authored hex palette and its 16-colour
// quantisation, exactly like the two existing tests.
//
// The floor is hard-enforced for EVERY built-in, not just the reference
// theme (matrix), with NO allowlist, tolerance table or log-only branch:
// every cell in this table fails the suite (t.Errorf) the instant it
// drops below minContrastRatio, full stop. A failing pair is a finding to
// report (docs/reports/phase3g-findings.md, task 1208), never a licence
// to recolour a built-in theme file.
func TestThemedDialogTokensClearContrastFloor(t *testing.T) {
	checks := dialogSurfaceChecks()
	checks = append(checks, dialogSelectionChecks()...)

	thinnest := make(map[string]float64, len(Builtins()))
	thinnestLabel := make(map[string]string, len(Builtins()))

	for _, th := range Builtins() {
		th := th
		min := math.Inf(1)
		minLabel := ""
		t.Run(th.Name, func(t *testing.T) {
			for _, chk := range checks {
				fgHex, err := th.Color(chk.fg)
				if err != nil {
					t.Fatalf("Color(%q): %v", chk.fg, err)
				}
				bgHex, err := th.Color(chk.bg)
				if err != nil {
					t.Fatalf("Color(%q): %v", chk.bg, err)
				}
				ratioHex, err := contrastRatio(fgHex, bgHex)
				if err != nil {
					t.Fatalf("contrastRatio(%q, %q): %v", fgHex, bgHex, err)
				}

				fgQ, err := th.QuantizedColor(chk.fg)
				if err != nil {
					t.Fatalf("QuantizedColor(%q): %v", chk.fg, err)
				}
				bgQ, err := th.QuantizedColor(chk.bg)
				if err != nil {
					t.Fatalf("QuantizedColor(%q): %v", chk.bg, err)
				}
				ratioQuant, err := contrastRatio(fgQ, bgQ)
				if err != nil {
					t.Fatalf("contrastRatio(%q, %q): %v", fgQ, bgQ, err)
				}

				t.Logf("%-8s %-20s hex %s/%s = %.2f:1   quant %s/%s = %.2f:1",
					th.Name, chk.label, fgHex, bgHex, ratioHex, fgQ, bgQ, ratioQuant)

				if ratioHex < min {
					min, minLabel = ratioHex, chk.label+" (hex)"
				}
				if ratioQuant < min {
					min, minLabel = ratioQuant, chk.label+" (quant)"
				}

				if ratioHex < minContrastRatio {
					t.Errorf("theme %q %s: hex contrast %.2f:1 < %.1f:1 (fg=%s bg=%s)",
						th.Name, chk.label, ratioHex, minContrastRatio, fgHex, bgHex)
				}
				if ratioQuant < minContrastRatio {
					t.Errorf("theme %q %s: quantised contrast %.2f:1 < %.1f:1 (fg=%s bg=%s)",
						th.Name, chk.label, ratioQuant, minContrastRatio, fgQ, bgQ)
				}
			}
		})
		thinnest[th.Name] = min
		thinnestLabel[th.Name] = minLabel
	}

	for _, th := range Builtins() {
		status := "clears"
		if thinnest[th.Name] < minContrastRatio {
			status = "BELOW"
		}
		t.Logf("SUMMARY %-10s thinnest newly-covered pair %-24s = %.2f:1 (%s floor %.1f:1)",
			th.Name, thinnestLabel[th.Name], thinnest[th.Name], status, minContrastRatio)
	}
}

// gutterBarChecks lists the two pairs sidebarGutterBar (tui.go) actually
// paints: the gutter glyph is drawn with `background` as its own
// FOREGROUND on top of `accent` (row selected) or `badge` (row marked but
// not selected) as the bar's background -- the one place in the whole
// chrome where `background` is a foreground rather than the thing
// everything else sits on, so neither TestBuiltinContrastFloor's own
// contrastChecks (which only ever puts background/selection/surface on
// the BACKGROUND side) nor either of the other two tables above cover it.
func gutterBarChecks() []struct {
	label string
	fg    Token
	bg    Token
} {
	return []struct {
		label string
		fg    Token
		bg    Token
	}{
		{"background/accent", Background, Accent},
		{"background/badge", Background, Badge},
	}
}

// TestGutterBarContrastFloor is task 011's own contrast obligation: the
// sidebar gutter bar's background-on-accent (selected) and
// background-on-badge (marked) pairs must clear minContrastRatio too,
// over both a theme's authored hex palette and its 16-colour
// quantisation, for every built-in theme registry.go's builtinFiles list
// embeds -- no allowlist, no theme recoloured to make a pair clear the
// floor. A failing pair is a finding to report, never a licence to
// recolour a built-in theme file; see sidebarGutterBar's own doc comment
// in internal/tui/tui.go for the one case this floor already knows is
// thin -- parchment quantises accent and badge to the very same #7f7f7f
// reference colour, so both of parchment's quantised pairs here land on
// an identical ratio.
func TestGutterBarContrastFloor(t *testing.T) {
	if len(Builtins()) != len(builtinFiles) {
		t.Fatalf("Builtins() returned %d themes, want exactly len(builtinFiles) = %d -- every registry.go entry must be checked, no allowlist", len(Builtins()), len(builtinFiles))
	}
	for _, th := range Builtins() {
		th := th
		t.Run(th.Name, func(t *testing.T) {
			for _, chk := range gutterBarChecks() {
				fgHex, err := th.Color(chk.fg)
				if err != nil {
					t.Fatalf("Color(%q): %v", chk.fg, err)
				}
				bgHex, err := th.Color(chk.bg)
				if err != nil {
					t.Fatalf("Color(%q): %v", chk.bg, err)
				}
				ratioHex, err := contrastRatio(fgHex, bgHex)
				if err != nil {
					t.Fatalf("contrastRatio(%q, %q): %v", fgHex, bgHex, err)
				}

				fgQ, err := th.QuantizedColor(chk.fg)
				if err != nil {
					t.Fatalf("QuantizedColor(%q): %v", chk.fg, err)
				}
				bgQ, err := th.QuantizedColor(chk.bg)
				if err != nil {
					t.Fatalf("QuantizedColor(%q): %v", chk.bg, err)
				}
				ratioQuant, err := contrastRatio(fgQ, bgQ)
				if err != nil {
					t.Fatalf("contrastRatio(%q, %q): %v", fgQ, bgQ, err)
				}

				t.Logf("%-8s %-20s hex %s/%s = %.2f:1   quant %s/%s = %.2f:1",
					th.Name, chk.label, fgHex, bgHex, ratioHex, fgQ, bgQ, ratioQuant)

				if ratioHex < minContrastRatio {
					t.Errorf("theme %q %s: hex contrast %.2f:1 < %.1f:1 (fg=%s bg=%s)",
						th.Name, chk.label, ratioHex, minContrastRatio, fgHex, bgHex)
				}
				if ratioQuant < minContrastRatio {
					t.Errorf("theme %q %s: quantised contrast %.2f:1 < %.1f:1 (fg=%s bg=%s)",
						th.Name, chk.label, ratioQuant, minContrastRatio, fgQ, bgQ)
				}
			}
		})
	}
}

// canvasForegroundBackgrounds is every background token internal/tui's
// canvasBackground (panel.go) is actually called with AND that can carry a
// deck-drawn GLYPH whose own run never opened a foreground of its own.
//
// GH #24's second report is what makes this table necessary. R118 made
// canvasBackground open the `background` token under every cell deck draws,
// but naming only a background moves the readability problem instead of
// fixing it: a run of deck's own copy that never opened a foreground still
// renders in the TERMINAL's default one, and on a terminal whose default
// foreground is white/light that is white-on-cream under a LIGHT theme --
// measured on a real parchment frame as 30 unforegrounded deck-owned runs
// (the sidebar's `socket:` line, the preview placeholder's cwd + "No live
// preview captured..." copy, the crop geometry line, 16 crop markers, the
// footer's reason line and its " · " legend separators). canvasBackground
// therefore opens `text` -- §11.6's body-text token -- as the canvas
// FOREGROUND alongside whichever background token it is given, and this is
// the table that holds that new pair to the §11.6 floor for every built-in.
//
// The four entries are exactly canvasBackground's glyph-bearing call sites'
// tokens: Background (every chrome line -- borders, padding, headers, the
// socket line, banners, notes, both footers, the preview placeholder, the
// crop geometry line and crop marker, every dialog frame), Surface (the
// alternating session-row stripe, and a dialog body), Selection and
// SelectionIdle (the selected row in a focused/unfocused list).
//
// DELIBERATELY ABSENT, and they must stay absent: Accent and Badge,
// sidebarGutterBar's two bar colours. `text` is unreadable on both in every
// built-in (parchment 2.67:1, empire 2.15:1, cobalt 2.02:1, daylight
// 2.06:1, matrix 1.00:1), and that is not a defect, because every glyph the
// gutter bar draws carries an explicit `background` foreground of its own
// (TestGutterBarContrastFloor above is that pair's floor) so no bar cell
// ever falls back to the canvas default. internal/tui's
// TestSidebarGutterCellsNeverFallBackToCanvasForeground is the render-level
// proof that they do not. Adding Accent/Badge here would not describe
// anything on screen; it would only force a recolour of every built-in's
// gutter, which §11.6's own rule in this file forbids ("a failing pair is a
// finding to report, never a licence to recolour a built-in theme file").
var canvasForegroundBackgrounds = []Token{Background, Surface, Selection, SelectionIdle}

// canvasForegroundChecks pairs Text against every canvasForegroundBackgrounds
// entry -- the "an uncoloured deck glyph is readable wherever deck paints"
// table.
func canvasForegroundChecks() []struct {
	label string
	fg    Token
	bg    Token
} {
	checks := make([]struct {
		label string
		fg    Token
		bg    Token
	}, 0, len(canvasForegroundBackgrounds))
	for _, bg := range canvasForegroundBackgrounds {
		checks = append(checks, struct {
			label string
			fg    Token
			bg    Token
		}{"text/" + string(bg), Text, bg})
	}
	return checks
}

// TestCanvasDefaultForegroundClearsFloor is GH #24's foreground half held
// to §11.6's floor: internal/tui's canvasBackground opens `text` as the
// canvas foreground alongside every background token it paints, so `text`
// must clear minContrastRatio against each of them -- over BOTH the
// theme's authored hex palette and its 16-colour quantisation, for every
// built-in registry.go embeds, with no allowlist and no theme recoloured
// to make a pair clear it.
//
// Both LIGHT built-ins are the reason this exists (`parchment` is the theme
// the operator filed against), so the two are asserted by name as well as
// by iteration: a future edit that drops one from builtinFiles must not
// silently take its coverage with it.
func TestCanvasDefaultForegroundClearsFloor(t *testing.T) {
	if len(Builtins()) != len(builtinFiles) {
		t.Fatalf("Builtins() returned %d themes, want exactly len(builtinFiles) = %d -- every registry.go entry must be checked, no allowlist", len(Builtins()), len(builtinFiles))
	}
	seenLight := map[string]bool{}
	for _, th := range Builtins() {
		th := th
		if th.Appearance == "light" {
			seenLight[th.Name] = true
		}
		t.Run(th.Name, func(t *testing.T) {
			for _, chk := range canvasForegroundChecks() {
				fgHex, err := th.Color(chk.fg)
				if err != nil {
					t.Fatalf("Color(%q): %v", chk.fg, err)
				}
				bgHex, err := th.Color(chk.bg)
				if err != nil {
					t.Fatalf("Color(%q): %v", chk.bg, err)
				}
				ratioHex, err := contrastRatio(fgHex, bgHex)
				if err != nil {
					t.Fatalf("contrastRatio(%q, %q): %v", fgHex, bgHex, err)
				}

				fgQ, err := th.QuantizedColor(chk.fg)
				if err != nil {
					t.Fatalf("QuantizedColor(%q): %v", chk.fg, err)
				}
				bgQ, err := th.QuantizedColor(chk.bg)
				if err != nil {
					t.Fatalf("QuantizedColor(%q): %v", chk.bg, err)
				}
				ratioQuant, err := contrastRatio(fgQ, bgQ)
				if err != nil {
					t.Fatalf("contrastRatio(%q, %q): %v", fgQ, bgQ, err)
				}

				t.Logf("%-10s %-8s %-22s hex %s/%s = %.2f:1   quant %s/%s = %.2f:1",
					th.Name, th.Appearance, chk.label, fgHex, bgHex, ratioHex, fgQ, bgQ, ratioQuant)

				if ratioHex < minContrastRatio {
					t.Errorf("theme %q %s: hex contrast %.2f:1 < %.1f:1 (fg=%s bg=%s)",
						th.Name, chk.label, ratioHex, minContrastRatio, fgHex, bgHex)
				}
				if ratioQuant < minContrastRatio {
					t.Errorf("theme %q %s: quantised contrast %.2f:1 < %.1f:1 (fg=%s bg=%s)",
						th.Name, chk.label, ratioQuant, minContrastRatio, fgQ, bgQ)
				}
			}
		})
	}
	for _, want := range []string{"parchment", "daylight"} {
		if !seenLight[want] {
			t.Errorf("built-in %q was not covered as a light theme -- GH #24 is about the light built-ins, so losing either one silently loses this test's whole point", want)
		}
	}
}

// TestCanvasForegroundTableExcludesTheGutterBarTokens is
// canvasForegroundBackgrounds' own non-vacuousness guard, in the direction
// that actually matters: it proves Accent and Badge are absent from that
// table for a REASON (text is genuinely unreadable on both in at least one
// built-in) rather than by oversight, so a future edit that adds them --
// and thereby forces a recolour of every built-in's gutter bar to satisfy
// a pair no cell on screen ever shows -- fails here with the numbers in
// front of it.
func TestCanvasForegroundTableExcludesTheGutterBarTokens(t *testing.T) {
	for _, tok := range canvasForegroundBackgrounds {
		if tok == Accent || tok == Badge {
			t.Fatalf("canvasForegroundBackgrounds must not list %q: see its own doc comment -- the gutter bar's glyphs carry an explicit `background` foreground, they never fall back to the canvas `text` default", tok)
		}
	}
	worst := 21.0
	worstTheme := ""
	for _, th := range Builtins() {
		for _, tok := range []Token{Accent, Badge} {
			fg, err := th.Color(Text)
			if err != nil {
				t.Fatalf("theme %q: Color(text): %v", th.Name, err)
			}
			bg, err := th.Color(tok)
			if err != nil {
				t.Fatalf("theme %q: Color(%q): %v", th.Name, tok, err)
			}
			ratio, err := contrastRatio(fg, bg)
			if err != nil {
				t.Fatalf("theme %q: contrastRatio(%q, %q): %v", th.Name, fg, bg, err)
			}
			t.Logf("%-10s text/%-6s hex %s/%s = %.2f:1 (excluded from the canvas-foreground floor by design)", th.Name, tok, fg, bg, ratio)
			if ratio < worst {
				worst, worstTheme = ratio, th.Name+" text/"+string(tok)
			}
		}
	}
	if worst >= minContrastRatio {
		t.Errorf("text now clears %.1f:1 on accent AND badge in every built-in (thinnest %s at %.2f:1) -- the exclusion in canvasForegroundBackgrounds' doc comment is no longer justified by the numbers and should be revisited", minContrastRatio, worstTheme, worst)
	}
}

// TestContrastRatioKnownValues pins contrastRatio/relativeLuminance
// against a handful of independently-computable values so the golden
// test above is not the only thing exercising the maths.
func TestContrastRatioKnownValues(t *testing.T) {
	cases := []struct {
		a, b string
		want float64
	}{
		{"#000000", "#ffffff", 21.0},
		{"#ffffff", "#ffffff", 1.0},
		{"#000000", "#000000", 1.0},
	}
	for _, c := range cases {
		got, err := contrastRatio(c.a, c.b)
		if err != nil {
			t.Fatalf("contrastRatio(%q, %q): %v", c.a, c.b, err)
		}
		if diff := got - c.want; diff > 0.01 || diff < -0.01 {
			t.Errorf("contrastRatio(%q, %q) = %.4f, want %.4f", c.a, c.b, got, c.want)
		}
	}
}

// TestContrastRatioRejectsInvalidHex proves contrastRatio errors rather
// than defaulting on malformed input, matching quantize's contract.
func TestContrastRatioRejectsInvalidHex(t *testing.T) {
	if _, err := contrastRatio("not-a-colour", "#000000"); err == nil {
		t.Error("contrastRatio(bad, good): want error, got nil")
	}
	if _, err := contrastRatio("#000000", "not-a-colour"); err == nil {
		t.Error("contrastRatio(good, bad): want error, got nil")
	}
}
