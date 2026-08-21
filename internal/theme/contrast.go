package theme

import "math"

// relativeLuminance computes the WCAG 2.x relative luminance of a
// "#rrggbb" hex colour: https://www.w3.org/TR/WCAG21/#dfn-relative-luminance
func relativeLuminance(hex string) (float64, error) {
	r, g, b, err := hexRGB(hex)
	if err != nil {
		return 0, err
	}
	chan_ := func(c int) float64 {
		v := float64(c) / 255.0
		if v <= 0.03928 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	R, G, B := chan_(r), chan_(g), chan_(b)
	return 0.2126*R + 0.7152*G + 0.0722*B, nil
}

// contrastRatio is the WCAG contrast ratio between two "#rrggbb" colours:
// (L1+0.05)/(L2+0.05) with L1 the lighter of the two relative
// luminances. Always >= 1.0.
func contrastRatio(a, b string) (float64, error) {
	la, err := relativeLuminance(a)
	if err != nil {
		return 0, err
	}
	lb, err := relativeLuminance(b)
	if err != nil {
		return 0, err
	}
	lighter, darker := la, lb
	if lb > la {
		lighter, darker = lb, la
	}
	return (lighter + 0.05) / (darker + 0.05), nil
}
