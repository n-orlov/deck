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

// dialogFocusedFieldTextTokens is the exact token set R82 draws inside a
// dialog's focused field row, over that row's selection background:
// label -> hint, value -> text, per-field help -> dimmed, footer keys ->
// key, validation -> error. This is deliberately NOT StatusTokens or
// AllTokens minus the structural ones -- R84 names "every text token"
// meaning the five roles a themed dialog's own field row actually
// composes, matching task 038's absence check
// (docs/reports/phase3g-038-r84-contrast-floor-absent/).
var dialogFocusedFieldTextTokens = []Token{Text, Dimmed, Hint, Key, Error}

// dialogSelectionChecks pairs every dialogFocusedFieldTextTokens entry
// against bg (theme.Selection or theme.SelectionIdle -- the two
// backgrounds a dialog's focused field row can carry, active vs. an
// idle/unfocused panel still showing its own selection).
func dialogSelectionChecks(label string, bg Token) []struct {
	label string
	fg    Token
	bg    Token
} {
	checks := make([]struct {
		label string
		fg    Token
		bg    Token
	}, 0, len(dialogFocusedFieldTextTokens))
	for _, tok := range dialogFocusedFieldTextTokens {
		checks = append(checks, struct {
			label string
			fg    Token
			bg    Token
		}{string(tok) + "/" + label, tok, bg})
	}
	return checks
}

// dialogPairKey names one (built-in theme, pair, colour space) cell of
// TestThemedDialogTokensClearContrastFloor's table, so a sub-floor cell
// can be allowlisted individually instead of a whole theme being exempted.
func dialogPairKey(theme, label, space string) string {
	return theme + " " + label + " " + space
}

// dialogPairAllowlist is the explicit, measured list of the cells that
// already sat below minContrastRatio when R84's coverage was added (task
// 106) -- authored palette values this plan is forbidden to change (a
// sub-floor pair is a finding for the operator, not a licence to recolour
// a theme file: finding F23 in docs/reports/phase3g-findings.md and
// docs/reports/phase3g-106-contrast-floor/).
//
// The value is the ratio measured then, to two decimals. It is pinned,
// not merely tolerated, and every cell NOT listed here is hard-enforced
// for every built-in, so this table cannot hide a regression:
//   - an unlisted cell that drops below the floor fails;
//   - a listed cell that drifts (worse OR better) by more than 0.01 fails,
//     because the recorded ratio no longer describes the palette;
//   - a listed cell that has reached the floor fails as a stale entry, so
//     the allowlist shrinks only deliberately;
//   - a listed cell naming a theme/pair/space this test does not cover
//     fails as unmatched, so a typo cannot silently exempt a real cell.
var dialogPairAllowlist = map[string]float64{
	// cobalt: dimmed (a field's own help text) over the active selection.
	"cobalt dimmed/selection hex": 2.59,
	// empire: dimmed over both selection backgrounds, plus the 16-colour
	// quantisation collapsing dimmed/hint/key/error onto SelectionIdle's
	// own reference colour (#7f7f7f).
	"empire dimmed/selection hex":       2.69,
	"empire dimmed/selectionidle hex":   2.13,
	"empire dimmed/selectionidle quant": 1.00,
	"empire hint/selectionidle quant":   1.00,
	"empire key/selectionidle quant":    2.35,
	"empire error/selectionidle hex":    2.69,
	"empire error/selectionidle quant":  1.00,
	// parchment: dimmed over both selection backgrounds.
	"parchment dimmed/selection hex":     2.51,
	"parchment dimmed/selectionidle hex": 2.87,
}

// checkDialogPair enforces minContrastRatio for one cell of R84's table,
// honouring dialogPairAllowlist exactly as that variable's comment
// describes. It returns the allowlist key it covered, so the caller can
// prove every allowlist entry matched a real cell.
func checkDialogPair(t *testing.T, theme, label, space string, ratio float64, fg, bg string) string {
	t.Helper()
	key := dialogPairKey(theme, label, space)
	want, known := dialogPairAllowlist[key]

	switch {
	case known && ratio >= minContrastRatio:
		t.Errorf("theme %q %s (%s): %.2f:1 now clears the %.1f:1 floor -- stale dialogPairAllowlist entry (recorded %.2f:1), delete it",
			theme, label, space, ratio, minContrastRatio, want)
	case known && math.Abs(ratio-want) > 0.01:
		t.Errorf("theme %q %s (%s): %.2f:1 (fg=%s bg=%s) drifted from the recorded %.2f:1 -- the palette moved; re-measure and update dialogPairAllowlist (never recolour a built-in to pass)",
			theme, label, space, ratio, fg, bg, want)
	case known:
		t.Logf("FINDING theme %q %s: %s contrast %.2f:1 < %.1f:1 (fg=%s bg=%s) -- known sub-floor pair, allowlisted at its measured ratio and reported (F23), not silenced or recoloured, per R84",
			theme, label, space, ratio, minContrastRatio, fg, bg)
	case ratio < minContrastRatio:
		t.Errorf("theme %q %s: %s contrast %.2f:1 < %.1f:1 (fg=%s bg=%s)",
			theme, label, space, ratio, minContrastRatio, fg, bg)
	}
	return key
}

// TestThemedDialogTokensClearContrastFloor is R84's own contrast
// obligation (task 106): the pairs a themed dialog (R82) actually draws
// that neither TestBuiltinContrastFloor nor
// TestSessionRowTokensClearContrastFloorOnSurface cover --
// hint/surface, key/surface, error/surface, and every one of a dialog's
// focused-field text tokens (text, dimmed, hint, key, error) over both
// theme.Selection and theme.SelectionIdle -- over both the theme's
// authored hex palette and its 16-colour quantisation, exactly like the
// two existing tests. This requirement pins what the PRD measured as
// already true; a failing pair here is a finding to report, not a licence
// to recolour a built-in theme.
//
// The floor is hard-enforced for EVERY built-in, not just the reference
// theme (matrix): the cells that were already sub-floor when this
// coverage landed are listed individually, with their measured ratios, in
// dialogPairAllowlist, and every other cell fails the suite the moment it
// drops. See that variable's comment for what the allowlist can and
// cannot absorb, docs/reports/phase3g-106-contrast-floor/ for the
// measurements, and docs/reports/phase3g-findings.md (F23) for the
// finding those sub-floor cells were reported as.
func TestThemedDialogTokensClearContrastFloor(t *testing.T) {
	checks := dialogSurfaceChecks()
	checks = append(checks, dialogSelectionChecks("selection", Selection)...)
	checks = append(checks, dialogSelectionChecks("selectionidle", SelectionIdle)...)

	thinnest := make(map[string]float64, len(Builtins()))
	thinnestLabel := make(map[string]string, len(Builtins()))
	covered := make(map[string]bool, len(Builtins())*len(checks)*2)

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

				covered[checkDialogPair(t, th.Name, chk.label, "hex", ratioHex, fgHex, bgHex)] = true
				covered[checkDialogPair(t, th.Name, chk.label, "quant", ratioQuant, fgQ, bgQ)] = true
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

	// An allowlist entry that matches no cell this test walks would be a
	// silent exemption of whatever it was meant to name.
	for key := range dialogPairAllowlist {
		if !covered[key] {
			t.Errorf("dialogPairAllowlist entry %q matches no (theme, pair, colour space) this test covers -- typo, or the pair/theme was renamed", key)
		}
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
