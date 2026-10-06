package theme

import (
	"fmt"
	"math"
	"testing"
)

// minPairwiseDeltaE is SPEC §11.6's distinctness floor: the mean CIEDE2000
// distance between any two built-in themes over pairwiseDistanceTokens.
const minPairwiseDeltaE = 15.0

// pairwiseDistanceTokens is the fixed token set the distance is averaged
// over: the canvas and elevated surface, the title, text and key colours,
// the focused border and the seven §7 status colours.
func pairwiseDistanceTokens() []Token {
	toks := []Token{Background, Surface, Title, Text, Key, BorderFocus}
	return append(toks, StatusTokens...)
}

// hexToLab converts a "#rrggbb" colour to CIE L*a*b* (D65), reusing
// contrast.go's sRGB parsing.
func hexToLab(hex string) (l, a, b float64, err error) {
	r8, g8, b8, err := hexRGB(hex)
	if err != nil {
		return 0, 0, 0, err
	}
	lin := func(c int) float64 {
		v := float64(c) / 255.0
		if v <= 0.04045 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	R, G, B := lin(r8), lin(g8), lin(b8)
	x := (0.4124564*R + 0.3575761*G + 0.1804375*B) / 0.95047
	y := 0.2126729*R + 0.7151522*G + 0.0721750*B
	z := (0.0193339*R + 0.1191920*G + 0.9503041*B) / 1.08883
	f := func(t float64) float64 {
		if t > 216.0/24389.0 {
			return math.Cbrt(t)
		}
		return (24389.0/27.0*t + 16) / 116
	}
	fx, fy, fz := f(x), f(y), f(z)
	return 116*fy - 16, 500 * (fx - fy), 200 * (fy - fz), nil
}

func degrees(rad float64) float64 { return rad * 180 / math.Pi }
func radians(deg float64) float64 { return deg * math.Pi / 180 }

// hueAngle is atan2(b, a) in degrees mapped to [0, 360).
func hueAngle(b, a float64) float64 {
	if a == 0 && b == 0 {
		return 0
	}
	h := degrees(math.Atan2(b, a))
	if h < 0 {
		h += 360
	}
	return h
}

// ciede2000 is the CIEDE2000 colour difference (Sharma, Wu, Dalal 2005),
// kL = kC = kH = 1.
func ciede2000(l1, a1, b1, l2, a2, b2 float64) float64 {
	c1 := math.Hypot(a1, b1)
	c2 := math.Hypot(a2, b2)
	cBar7 := math.Pow((c1+c2)/2, 7)
	g := 0.5 * (1 - math.Sqrt(cBar7/(cBar7+math.Pow(25, 7))))
	a1p, a2p := (1+g)*a1, (1+g)*a2
	c1p, c2p := math.Hypot(a1p, b1), math.Hypot(a2p, b2)
	h1p, h2p := hueAngle(b1, a1p), hueAngle(b2, a2p)

	dLp := l2 - l1
	dCp := c2p - c1p
	var dhp float64
	if c1p*c2p != 0 {
		dhp = h2p - h1p
		if dhp > 180 {
			dhp -= 360
		} else if dhp < -180 {
			dhp += 360
		}
	}
	dHp := 2 * math.Sqrt(c1p*c2p) * math.Sin(radians(dhp/2))

	lBarP := (l1 + l2) / 2
	cBarP := (c1p + c2p) / 2
	hBarP := h1p + h2p
	if c1p*c2p != 0 {
		switch {
		case math.Abs(h1p-h2p) <= 180:
			hBarP /= 2
		case h1p+h2p < 360:
			hBarP = (hBarP + 360) / 2
		default:
			hBarP = (hBarP - 360) / 2
		}
	}
	t := 1 - 0.17*math.Cos(radians(hBarP-30)) + 0.24*math.Cos(radians(2*hBarP)) +
		0.32*math.Cos(radians(3*hBarP+6)) - 0.20*math.Cos(radians(4*hBarP-63))
	dTheta := 30 * math.Exp(-math.Pow((hBarP-275)/25, 2))
	rC := 2 * math.Sqrt(math.Pow(cBarP, 7)/(math.Pow(cBarP, 7)+math.Pow(25, 7)))
	sL := 1 + 0.015*math.Pow(lBarP-50, 2)/math.Sqrt(20+math.Pow(lBarP-50, 2))
	sC := 1 + 0.045*cBarP
	sH := 1 + 0.015*cBarP*t
	rT := -math.Sin(radians(2*dTheta)) * rC
	tl, tc, th := dLp/sL, dCp/sC, dHp/sH
	return math.Sqrt(tl*tl + tc*tc + th*th + rT*tc*th)
}

func hexDeltaE(x, y string) (float64, error) {
	l1, a1, b1, err := hexToLab(x)
	if err != nil {
		return 0, err
	}
	l2, a2, b2, err := hexToLab(y)
	if err != nil {
		return 0, err
	}
	return ciede2000(l1, a1, b1, l2, a2, b2), nil
}

// themeDistance is the mean CIEDE2000 ΔE between two themes, token by token.
func themeDistance(x, y *Theme) (float64, error) {
	toks := pairwiseDistanceTokens()
	sum := 0.0
	for _, tok := range toks {
		hx, err := x.Color(tok)
		if err != nil {
			return 0, err
		}
		hy, err := y.Color(tok)
		if err != nil {
			return 0, err
		}
		d, err := hexDeltaE(hx, hy)
		if err != nil {
			return 0, err
		}
		sum += d
	}
	return sum / float64(len(toks)), nil
}

// TestCIEDE2000KnownValues pins the implementation to published reference
// pairs from Sharma, Wu & Dalal (2005), Table 1.
func TestCIEDE2000KnownValues(t *testing.T) {
	cases := []struct {
		l1, a1, b1, l2, a2, b2, want float64
	}{
		{50, 2.6772, -79.7751, 50, 0, -82.7485, 2.0425},
		{50, 3.1571, -77.2803, 50, 0, -82.7485, 2.8615},
		{50, 2.5, 0, 50, 0, -2.5, 4.3065},
		{50, 2.5, 0, 73, 25, -18, 27.1492},
		{60.2574, -34.0099, 36.2677, 60.4626, -34.1751, 39.4387, 1.2644},
		{22.7233, 20.0904, -46.694, 23.0331, 14.973, -42.5619, 2.0373},
	}
	for _, c := range cases {
		got := ciede2000(c.l1, c.a1, c.b1, c.l2, c.a2, c.b2)
		if math.Abs(got-c.want) > 1e-3 {
			t.Errorf("ciede2000(%v,%v,%v | %v,%v,%v) = %.4f, want %.4f", c.l1, c.a1, c.b1, c.l2, c.a2, c.b2, got, c.want)
		}
	}
	if d, err := hexDeltaE("#336699", "#336699"); err != nil || d != 0 {
		t.Errorf("identical colours: ΔE = %v, err %v, want 0", d, err)
	}
	if _, err := hexDeltaE("nope", "#000000"); err == nil {
		t.Error("hexDeltaE accepted an invalid hex colour")
	}
}

// TestBuiltinThemesArePairwiseDistinct is R210: for every unordered pair of
// registered built-ins the mean CIEDE2000 ΔE over pairwiseDistanceTokens is
// at least minPairwiseDeltaE. It iterates the registry, so a later built-in
// is covered with no new test, and a failure names the closest pair.
func TestBuiltinThemesArePairwiseDistinct(t *testing.T) {
	all := Builtins()
	if len(all) < 2 {
		t.Fatalf("registry holds %d built-ins, need at least 2 for a pairwise check", len(all))
	}
	closest, closestD := "", math.Inf(1)
	for i := 0; i < len(all); i++ {
		for j := i + 1; j < len(all); j++ {
			d, err := themeDistance(all[i], all[j])
			if err != nil {
				t.Fatalf("distance %s/%s: %v", all[i].Name, all[j].Name, err)
			}
			t.Logf("%-10s %-10s mean ΔE00 = %.2f", all[i].Name, all[j].Name, d)
			if d < closestD {
				closest, closestD = fmt.Sprintf("%s/%s", all[i].Name, all[j].Name), d
			}
		}
	}
	if closestD < minPairwiseDeltaE {
		t.Errorf("closest built-in pair %s has mean CIEDE2000 ΔE %.2f < %.0f over %v",
			closest, closestD, minPairwiseDeltaE, pairwiseDistanceTokens())
	}
}
