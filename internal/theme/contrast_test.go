package theme

import (
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
