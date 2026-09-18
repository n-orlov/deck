package theme

import (
	"math"
	"testing"
)

// hueSat is rgbToHSL's hue/saturation for a hex colour, for the
// hue-preservation assertions below.
func hueSat(t *testing.T, hex string) (h, s float64) {
	t.Helper()
	r, g, b, err := hexRGB(hex)
	if err != nil {
		t.Fatalf("hexRGB(%q): %v", hex, err)
	}
	h, s, _ = rgbToHSL(r, g, b)
	return h, s
}

// The colours the operator named: Claude Code's blue and Pi's yellow, in
// both their normal and bright ANSI forms, plus the rest of the palette an
// agent actually uses.
var agentColors = map[string]string{
	"blue (SGR 34)":          ReferencePalette[4],
	"bright blue (SGR 94)":   ReferencePalette[12],
	"yellow (SGR 33)":        ReferencePalette[3],
	"bright yellow (SGR 93)": ReferencePalette[11],
	"green (SGR 32)":         ReferencePalette[2],
	"red (SGR 31)":           ReferencePalette[1],
	"cyan (SGR 36)":          ReferencePalette[6],
	"grey (SGR 90)":          ReferencePalette[8],
}

// TestFitForegroundPreservesHueAndClearsFloor is the operator's own
// requirement in assertion form: "preserve agent text colour highlights
// (eg claude code uses blue text often, pi - yellow)" AND "adjust these
// colours to fit theme background". Both at once is the whole point --
// either alone is easy and useless.
func TestFitForegroundPreservesHueAndClearsFloor(t *testing.T) {
	// Every built-in's canvas pair: `background` (the unattached preview's
	// token) and `surface` (the attached one's), since a fitted colour has
	// to work over both.
	for _, th := range Builtins() {
		name := th.Name
		for _, tok := range []Token{Background, Surface} {
			bg, err := th.Color(tok)
			if err != nil {
				t.Fatalf("%s.Color(%s): %v", name, tok, err)
			}
			for label, fg := range agentColors {
				fitted, changed, err := FitForeground(fg, bg, AAFloor)
				if err != nil {
					t.Fatalf("FitForeground(%s, %s): %v", fg, bg, err)
				}
				after, err := ContrastRatio(fitted, bg)
				if err != nil {
					t.Fatal(err)
				}
				before, err := ContrastRatio(fg, bg)
				if err != nil {
					t.Fatal(err)
				}

				// 1. The floor is actually reached.
				if after < AAFloor-0.01 {
					t.Errorf("%s/%s %s: fitted %s is %.2f:1 against %s, below the %.1f floor -- the whole point of fitting is that the operator can read it",
						name, tok, label, fitted, after, bg, AAFloor)
				}

				// 2. The HUE is preserved: blue is still blue, yellow is
				// still yellow. This is what keeps "which agent is this"
				// legible, and it is why fitting cannot just blend toward
				// the theme's text colour.
				h0, s0 := hueSat(t, fg)
				h1, s1 := hueSat(t, fitted)
				if s0 > 0.01 { // hue is meaningless for greys
					if d := math.Abs(h1 - h0); d > 1.0 && math.Abs(d-360) > 1.0 {
						t.Errorf("%s/%s %s: hue moved %.1f deg (%s -> %s); only lightness may move",
							name, tok, label, d, fg, fitted)
					}
					if math.Abs(s1-s0) > 0.02 {
						t.Errorf("%s/%s %s: saturation moved %.2f -> %.2f (%s -> %s); only lightness may move",
							name, tok, label, s0, s1, fg, fitted)
					}
				}

				// 3. A colour that already clears the floor is returned
				// byte-identical: an agent whose palette already suits the
				// theme pays nothing at all.
				if before >= AAFloor {
					if changed || fitted != fg {
						t.Errorf("%s/%s %s: %s already cleared the floor at %.2f:1 but was changed to %s",
							name, tok, label, fg, before, fitted)
					}
				} else if !changed {
					t.Errorf("%s/%s %s: %s was %.2f:1, below the floor, but reported unchanged",
						name, tok, label, fg, before)
				}
			}
		}
	}
}

// TestFitForegroundMovesAwayFromTheCanvas pins the DIRECTION: a light
// canvas darkens the agent's colour, a dark canvas lightens it. Getting
// this backwards still clears the floor in some cases (by overshooting
// past the canvas) while looking obviously wrong.
func TestFitForegroundMovesAwayFromTheCanvas(t *testing.T) {
	cases := []struct {
		name     string
		bg       string
		fg       string
		wantDark bool // want the fitted colour DARKER than the original
	}{
		{"parchment vs bright yellow", "#f5ecd7", ReferencePalette[11], true},
		{"daylight vs bright yellow", "#f8fafc", ReferencePalette[11], true},
		{"empire vs blue", "#0f172a", ReferencePalette[4], false},
		{"matrix vs blue", "#000000", ReferencePalette[4], false},
	}
	for _, c := range cases {
		fitted, changed, err := FitForeground(c.fg, c.bg, AAFloor)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if !changed {
			t.Fatalf("%s: expected %s to need fitting against %s", c.name, c.fg, c.bg)
		}
		l0, err := RelativeLuminance(c.fg)
		if err != nil {
			t.Fatal(err)
		}
		l1, err := RelativeLuminance(fitted)
		if err != nil {
			t.Fatal(err)
		}
		if c.wantDark && l1 >= l0 {
			t.Errorf("%s: %s -> %s got LIGHTER (%.4f -> %.4f) on a light canvas", c.name, c.fg, fitted, l0, l1)
		}
		if !c.wantDark && l1 <= l0 {
			t.Errorf("%s: %s -> %s got DARKER (%.4f -> %.4f) on a dark canvas", c.name, c.fg, fitted, l0, l1)
		}
	}
}

// TestFitForegroundMovesAsLittleAsPossible: the fitted colour must sit
// just past the floor, not slammed to black or white. Overshooting would
// throw away the agent's own shade for no readability gain -- the
// difference between "adjusted to fit" and "flattened".
func TestFitForegroundMovesAsLittleAsPossible(t *testing.T) {
	for _, bg := range []string{"#f5ecd7", "#f8fafc", "#0f172a", "#000000", "#0a1a2e"} {
		for label, fg := range agentColors {
			fitted, changed, err := FitForeground(fg, bg, AAFloor)
			if err != nil {
				t.Fatal(err)
			}
			if !changed {
				continue
			}
			got, err := ContrastRatio(fitted, bg)
			if err != nil {
				t.Fatal(err)
			}
			// Bisection to ~6e-8 in lightness, quantised to 1/255, lands
			// within a few hundredths of the floor. A full slam to
			// black/white against these backgrounds would be >= 8:1.
			if got > AAFloor+0.5 {
				t.Errorf("%s on %s: fitted %s overshot to %.2f:1 (floor %.1f) -- the agent's own shade is being discarded",
					label, bg, fitted, got, AAFloor)
			}
		}
	}
}

// TestFitForegroundReportsTheBestAvailableWhenTheFloorIsUnreachable: a hue
// that cannot clear the floor at ANY lightness (a saturated yellow against
// a mid-grey canvas) must still come back as the most readable version
// rather than an error or the unchanged original -- a theme problem is not
// a per-cell one.
func TestFitForegroundReportsTheBestAvailableWhenTheFloorIsUnreachable(t *testing.T) {
	const bg = "#808080" // mid grey: nothing clears 4.5:1 against it
	fitted, changed, err := FitForeground(ReferencePalette[11], bg, AAFloor)
	if err != nil {
		t.Fatalf("FitForeground: %v", err)
	}
	if !changed {
		t.Fatalf("expected a change: bright yellow on mid grey cannot clear the floor unadjusted")
	}
	got, err := ContrastRatio(fitted, bg)
	if err != nil {
		t.Fatal(err)
	}
	unfitted, err := ContrastRatio(ReferencePalette[11], bg)
	if err != nil {
		t.Fatal(err)
	}
	if got <= unfitted {
		t.Errorf("fitted %s is %.2f:1, no better than the original's %.2f:1", fitted, got, unfitted)
	}
	h0, s0 := hueSat(t, ReferencePalette[11])
	h1, _ := hueSat(t, fitted)
	if s0 > 0.01 && math.Abs(h1-h0) > 1.0 {
		t.Errorf("hue moved %.1f -> %.1f even in the unreachable case", h0, h1)
	}
}

func TestHSLRoundTrip(t *testing.T) {
	for _, hex := range append(ReferencePalette[:], "#f5ecd7", "#3b2a1a", "#0d9488", "#123456", "#abcdef") {
		r, g, b, err := hexRGB(hex)
		if err != nil {
			t.Fatalf("hexRGB(%q): %v", hex, err)
		}
		h, s, l := rgbToHSL(r, g, b)
		if got := hslToHex(h, s, l); got != hex {
			t.Errorf("round trip %s -> hsl(%.2f, %.2f, %.2f) -> %s", hex, h, s, l, got)
		}
	}
}

func TestQuantizeHexIsExported(t *testing.T) {
	// The 16-colour path in internal/tui depends on this: a fitted
	// truecolour value has to be snappable to an ANSI slot.
	got, err := QuantizeHex("#6f6f00")
	if err != nil {
		t.Fatalf("QuantizeHex: %v", err)
	}
	if _, ok := ANSI16Code(got); !ok {
		t.Errorf("QuantizeHex returned %q, which ANSI16Code cannot render", got)
	}
}
