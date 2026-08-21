package theme

import (
	"fmt"
	"strconv"
)

// ReferencePalette is deck's declared 16-colour reference palette (§11.6):
// the fixed xterm defaults quantisation measures distance against, spelled
// exactly as the SPEC states them ("000000 cd0000 00cd00 cdcd00 0000ee
// cd00cd 00cdcd e5e5e5" for 0-7, "7f7f7f ff0000 00ff00 ffff00 5c5cff
// ff00ff 00ffff ffffff" for 8-15). This is fixed here — not read from the
// terminal — because terminals do not agree on what their 16 ANSI slots
// actually render, and "nearest" plus any contrast ratio computed over the
// quantised palette (task 011) are undefined without a fixed target.
var ReferencePalette = [16]string{
	"#000000", "#cd0000", "#00cd00", "#cdcd00",
	"#0000ee", "#cd00cd", "#00cdcd", "#e5e5e5",
	"#7f7f7f", "#ff0000", "#00ff00", "#ffff00",
	"#5c5cff", "#ff00ff", "#00ffff", "#ffffff",
}

// quantize returns the ReferencePalette entry nearest hex by Euclidean
// RGB distance. Ties resolve to the lower index (ReferencePalette's
// declared order), so quantisation is deterministic and reproducible.
func quantize(hex string) (string, error) {
	r, g, b, err := hexRGB(hex)
	if err != nil {
		return "", err
	}
	best := ReferencePalette[0]
	bestDist := -1
	for _, ref := range ReferencePalette {
		rr, rg, rb, err := hexRGB(ref)
		if err != nil {
			return "", fmt.Errorf("theme: internal error: reference palette entry %q: %w", ref, err)
		}
		dr := r - rr
		dg := g - rg
		db := b - rb
		dist := dr*dr + dg*dg + db*db
		if bestDist == -1 || dist < bestDist {
			bestDist = dist
			best = ref
		}
	}
	return best, nil
}

// quantizeColors quantises every entry of colors independently, returning
// a new map (colors itself is never mutated — a truecolour terminal must
// still see the theme's exact authored values via Theme.Colors).
func quantizeColors(colors map[Token]string) (map[Token]string, error) {
	out := make(map[Token]string, len(colors))
	for tok, hex := range colors {
		q, err := quantize(hex)
		if err != nil {
			return nil, fmt.Errorf("token %q: %w", tok, err)
		}
		out[tok] = q
	}
	return out, nil
}

// HexRGB parses a "#rrggbb" hex colour string into its three byte
// components (0-255 each). Exported so a renderer (internal/tui) can turn
// an authored/quantised colour into an SGR truecolour escape without this
// package growing a rendering dependency of its own.
func HexRGB(hex string) (r, g, b int, err error) {
	return hexRGB(hex)
}

// ANSI16Code returns the SGR foreground colour code (30-37 for the eight
// low-intensity slots, 90-97 for the eight high-intensity ones) for hex,
// provided hex is EXACTLY one of ReferencePalette's 16 entries (i.e. an
// already-quantised colour, as QuantizedColor always returns) -- the ANSI
// 16-colour floor rendering path is authorised to say more only about
// exactly those 16 colours, per SS11.6. ok is false for anything else,
// including an unquantised authored hex that happens not to collide with
// the palette.
func ANSI16Code(hex string) (code int, ok bool) {
	for i, ref := range ReferencePalette {
		if ref == hex {
			if i < 8 {
				return 30 + i, true
			}
			return 90 + (i - 8), true
		}
	}
	return 0, false
}

// hexRGB parses a "#rrggbb" string into its three byte components.
func hexRGB(hex string) (r, g, b int, err error) {
	if len(hex) != 7 || hex[0] != '#' {
		return 0, 0, 0, fmt.Errorf("theme: not a #rrggbb hex colour: %q", hex)
	}
	r, err = hexByte(hex[1:3])
	if err != nil {
		return 0, 0, 0, err
	}
	g, err = hexByte(hex[3:5])
	if err != nil {
		return 0, 0, 0, err
	}
	b, err = hexByte(hex[5:7])
	if err != nil {
		return 0, 0, 0, err
	}
	return r, g, b, nil
}

func hexByte(s string) (int, error) {
	v, err := strconv.ParseInt(s, 16, 32)
	if err != nil {
		return 0, fmt.Errorf("theme: invalid hex byte %q: %w", s, err)
	}
	return int(v), nil
}
