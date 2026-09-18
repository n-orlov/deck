package theme

import (
	"fmt"
	"math"
)

// ContrastRatio is contrastRatio exported for internal/tui, which needs to
// judge a CAPTURED PANE's own colours against deck's canvas at render time
// -- a question that did not exist while SPEC.md:1595 left pane output
// untouched, and which no in-package caller can answer for it.
func ContrastRatio(a, b string) (float64, error) { return contrastRatio(a, b) }

// RelativeLuminance is relativeLuminance exported for the same reason:
// deciding whether a canvas is light or dark decides which direction a
// foreground has to move to become readable on it.
func RelativeLuminance(hex string) (float64, error) { return relativeLuminance(hex) }

// QuantizeHex snaps an arbitrary colour to ReferencePalette, exported for
// internal/tui: a fitted agent colour has to be expressible at the active
// colour depth, and at depth 16 that means finding its nearest ANSI slot
// the same way a theme's own tokens are quantised.
func QuantizeHex(hex string) (string, error) { return quantize(hex) }

// AAFloor is WCAG 2.x AA for normal-size body text. It is the floor
// internal/theme's own built-in palettes are held to
// (TestBuiltinContrastFloors), so it is the floor an agent's own colour is
// held to as well once deck paints a canvas underneath it.
const AAFloor = 4.5

// FitForeground returns fg adjusted just far enough to reach floor:1
// contrast against bg, MOVING ONLY LIGHTNESS and leaving hue and
// saturation exactly as they were.
//
// This exists because deck painting its own canvas under a captured pane
// (the SPEC.md:1595 experiment) changes what the agent's own colours are
// composed against. An agent picks its palette for the terminal it thinks
// it is on: Claude Code's blue and Pi's yellow are chosen against a dark
// terminal, and dropping them onto parchment's #f5ecd7 unmodified is how
// "preserve the agent's highlights" turns into "the highlights are
// unreadable". Preserving the HUE is what preserves the agent's meaning --
// blue stays blue and yellow stays yellow, so the user can still tell
// Claude's output from Pi's at a glance -- while lightness is the one
// dimension that has to move for the pair to be legible at all.
//
// The direction is decided by the canvas, not by the colour: on a light
// background a colour must get darker, on a dark background lighter.
// Saturated hues are the hard case and are why this cannot just blend
// toward the theme's text colour -- pure yellow (#ffff00) on parchment is
// 1.19:1 and blending it toward parchment's near-black text would turn it
// grey-brown, losing the very signal the operator asked to keep. Dropping
// its lightness instead yields a dark olive: still recognisably the same
// hue, and readable.
//
// Returns the (possibly unchanged) colour and whether it was adjusted. A
// colour that already clears the floor is returned untouched, so an agent
// on a terminal whose palette already suits the theme pays nothing.
func FitForeground(fg, bg string, floor float64) (string, bool, error) {
	ratio, err := ContrastRatio(fg, bg)
	if err != nil {
		return "", false, err
	}
	if ratio >= floor {
		return fg, false, nil
	}
	bgLum, err := relativeLuminance(bg)
	if err != nil {
		return "", false, err
	}
	r, g, b, err := hexRGB(fg)
	if err != nil {
		return "", false, err
	}
	h, s, l := rgbToHSL(r, g, b)

	// Move lightness away from the canvas: darker on a light canvas,
	// lighter on a dark one. bgLum is WCAG relative luminance, whose
	// midpoint for "is this light or dark" is ~0.18 (the luminance of
	// mid-grey #777), not 0.5.
	darken := bgLum > 0.18

	// The extreme in the chosen direction is the most contrast this hue
	// can possibly reach. If even that fails the floor, no lightness
	// works and there is nothing to search for: return it, since it is
	// still the most readable version available. (A canvas that cannot
	// carry any colour of this hue at the floor is a theme problem, not a
	// per-cell one, so this reports success rather than an error.)
	limit := 0.0
	if !darken {
		limit = 1.0
	}
	extreme := hslToHex(h, s, limit)
	extremeRatio, err := ContrastRatio(extreme, bg)
	if err != nil {
		return "", false, err
	}
	if extremeRatio < floor {
		return extreme, true, nil
	}

	// Bisect for the lightness CLOSEST TO THE AGENT'S OWN that still
	// clears the floor, so the colour moves as little as it has to.
	// clears() is monotonic in this direction: `limit` clears, `l` does
	// not, so the invariant "clearing end / failing end" holds throughout.
	// 24 iterations resolve L to ~6e-8, far finer than the 1/255 the
	// result is quantised to.
	clearing, failing := limit, l
	for range 24 {
		mid := (clearing + failing) / 2
		cr, err := ContrastRatio(hslToHex(h, s, mid), bg)
		if err != nil {
			return "", false, err
		}
		if cr >= floor {
			clearing = mid
		} else {
			failing = mid
		}
	}
	return hslToHex(h, s, clearing), true, nil
}

// rgbToHSL converts 0-255 RGB to hue (degrees), saturation and lightness
// in [0,1].
func rgbToHSL(r, g, b int) (h, s, l float64) {
	rf, gf, bf := float64(r)/255, float64(g)/255, float64(b)/255
	maxc := math.Max(rf, math.Max(gf, bf))
	minc := math.Min(rf, math.Min(gf, bf))
	l = (maxc + minc) / 2
	if maxc == minc {
		return 0, 0, l // achromatic
	}
	d := maxc - minc
	if l > 0.5 {
		s = d / (2 - maxc - minc)
	} else {
		s = d / (maxc + minc)
	}
	switch maxc {
	case rf:
		h = (gf - bf) / d
		if gf < bf {
			h += 6
		}
	case gf:
		h = (bf-rf)/d + 2
	default:
		h = (rf-gf)/d + 4
	}
	return h * 60, s, l
}

// hslToHex is rgbToHSL's inverse, rendered as "#rrggbb".
func hslToHex(h, s, l float64) string {
	if l < 0 {
		l = 0
	}
	if l > 1 {
		l = 1
	}
	var r, g, b float64
	if s == 0 {
		r, g, b = l, l, l
	} else {
		q := l * (1 + s)
		if l >= 0.5 {
			q = l + s - l*s
		}
		p := 2*l - q
		hk := h / 360
		r = hueToRGB(p, q, hk+1.0/3.0)
		g = hueToRGB(p, q, hk)
		b = hueToRGB(p, q, hk-1.0/3.0)
	}
	return fmt.Sprintf("#%02x%02x%02x", round255(r), round255(g), round255(b))
}

func hueToRGB(p, q, t float64) float64 {
	if t < 0 {
		t++
	}
	if t > 1 {
		t--
	}
	switch {
	case t < 1.0/6.0:
		return p + (q-p)*6*t
	case t < 1.0/2.0:
		return q
	case t < 2.0/3.0:
		return p + (q-p)*(2.0/3.0-t)*6
	default:
		return p
	}
}

func round255(v float64) int {
	n := int(math.Round(v * 255))
	if n < 0 {
		return 0
	}
	if n > 255 {
		return 255
	}
	return n
}
