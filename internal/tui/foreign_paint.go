package tui

import (
	"fmt"
	"strings"

	"github.com/n-orlov/deck/internal/theme"
)

// Preview paint (SPEC §11.3, `[ui] preview_paint`).
//
// Deck paints its own canvas UNDER a previewed pane's captured cells,
// keeps the agent's own HUES, and moves only what has to move for the
// pair to be legible:
//
//  1. A cell the agent left at the terminal's DEFAULT colour gets deck's
//     canvas pair. An unnamed foreground has UNDEFINED contrast, so
//     without this the pane region's readability depends on the user's
//     terminal profile, which deck cannot inspect and the theme cannot
//     control -- GH #24's defect one layer in.
//  2. A cell the agent coloured EXPLICITLY keeps its hue -- Claude Code's
//     blue stays blue, Pi's yellow stays yellow, which is how one agent's
//     output is told from another's at a glance.
//  3. ...but an explicit colour chosen for a dark terminal is not
//     automatically legible on parchment, so its LIGHTNESS is moved just
//     far enough to clear theme.AAFloor against whatever deck now paints
//     underneath it (theme.FitForeground).
//
// The two preview modes start from different states, and both are handled
// here because both converge on this function:
//
//   - Unattached (passive capture): the pane region carries theme
//     background EVERYWHERE EXCEPT under the agent's own text, because
//     cropRow hands the captured bytes back untouched and only paints the
//     pad/marker columns it added itself.
//   - Attached (interactive grid): the WHOLE pane region carries the
//     agent's default background, because ultraviolet's renderLine emits a
//     full ResetStyle for every EmptyCell (buffer.go:150-153), so even the
//     blanks actively reset deck's canvas.
//
// That difference is load-bearing -- it is how a user sees at a glance
// whether keystrokes go to the pane -- so painting both must NOT collapse
// them into one colour. See foreignCanvasToken.
//
// `[ui] preview_paint` selects how far the paint reaches, and is read live
// on every row (see foreignPaint):
//
//	fit      paint both channels and fit explicit colours (the default)
//	nofit    paint both, but never adjust an agent's explicit colour
//	bg       paint the background only; touch no foreground
//	off      repaint nothing: captured bytes reach the panel untouched
type foreignPaintMode int

const (
	foreignPaintFit            foreignPaintMode = iota // background + foreground + fit
	foreignPaintOff                                    // SPEC.md:1595 as it stands
	foreignPaintBackgroundOnly                         // background only
	foreignPaintNoFit                                  // background + foreground, no fitting
)

// foreignPaint resolves the mode from the running Model's own settings,
// read fresh on every call rather than memoised, so a ctrl+s in the
// settings takeover is live on the next preview tick (the ScopeGlobal
// claim ui.preview_paint's schema entry makes). The cost is one map-free
// string switch per previewed row, against a bisection that only runs for
// a colour that actually fails its floor.
//
// config validates the key at parse time and the DECK_PREVIEW_PAINT
// override errors on anything unrecognised, so an unknown value cannot
// reach here from either source; the default arm exists because a Model
// built in a test may carry a zero-valued Settings, and the shipped
// default is the honest thing to give it.
func (m Model) foreignPaint() foreignPaintMode {
	return parseForeignPaint(m.settings.PreviewPaint)
}

// parseForeignPaint maps one of ui.preview_paint's four declared values to
// its mode. Only those four are accepted: aliases ("no", "background",
// "keep") would let a config.toml read as if it had selected a mode it did
// not, and the schema's EnumValues are what the settings takeover offers.
func parseForeignPaint(v string) foreignPaintMode {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "off":
		return foreignPaintOff
	case "bg":
		return foreignPaintBackgroundOnly
	case "nofit":
		return foreignPaintNoFit
	default:
		return foreignPaintFit
	}
}

// foreignCanvasToken is the background token deck paints under a captured
// pane's own cells. The two preview modes deliberately differ: losing the
// ability to tell an attached preview from an unattached one at a glance
// would be a worse regression than the contrast problem this fixes, and
// the border colour alone is a one-cell cue on two edges. `surface` is
// §11.6's "elevated" tone and is a real step away from `background` in
// every built-in -- TestForeignCanvasTokenDistinguishesAttachedFromUnattached
// asserts the two resolve to different colours for every one of them, so
// this cue cannot be quietly lost by a theme edit -- which makes an
// attached pane read as a raised region rather than as more canvas.
func (m Model) foreignCanvasToken() theme.Token {
	if m.interactive {
		return attachedCanvasToken()
	}
	return theme.Background
}

// attachedCanvasToken is the attached pane's fill. Measured contrast
// against `background` across the built-ins:
//
//	surface         ~1.07 - 1.14:1   the §11.6 "elevated" tone
//	selection_idle  ~1.23 - 1.76:1
//	selection       ~1.39 - 1.69:1   the largest step, but see below
//
// `surface` is chosen despite being the SMALLEST step, on the operator's
// call (2026-09-18), and the reason is that a big step is not the same as
// a good cue. `selection` is §11.6's SELECTED-ROW colour, and filling a
// whole pane with it devalues the thing it exists to mark: with the
// attached pane painted `selection`, the selected row in the list is the
// same colour as a pane, and the row highlight reads as invisible. A cue
// that costs another cue is not a win. `surface` is already semantically
// "elevated region", which is what an attached pane is, and it leaves
// `selection` to mean one thing only.
//
// This is not a setting: it is one token, chosen on the merits above, and
// what it must never be is `background` --
// TestForeignCanvasTokenDistinguishesAttachedFromUnattached enforces that
// for every built-in with a 1.05:1 floor, so the step cannot silently
// shrink to nothing under a theme edit.
func attachedCanvasToken() theme.Token {
	return theme.Surface
}

// sgrColor is one channel's colour state within a captured row: either the
// terminal's default (deck's to fill) or a resolved 24-bit value the agent
// asked for explicitly (deck's to preserve, and to fit).
type sgrColor struct {
	explicit bool
	hex      string
}

// repaintForeignDefaults rewrites one captured row's SGR stream so that
// deck's canvas shows through wherever the agent expressed no preference,
// and the agent's own colours survive -- legibly -- wherever it did.
//
// It works on the SGR stream rather than on emulator cells because both
// preview paths converge here as strings, and the string form carries
// everything needed: a pane's colour state changes only via SGR, so
// tracking that state and correcting it at each boundary is sufficient and
// complete. The alternative -- walking ultraviolet cells in
// internal/interactive and substituting nil Fg/Bg -- would fix the
// attached half only, and would push deck's theme into a package that is
// deliberately theme-blind.
//
// The four fg/bg combinations are each handled differently, and the reason
// is contrast, not symmetry:
//
//	default fg, default bg   -> deck's canvas pair. Nothing is lost: the
//	                            agent expressed no preference at all.
//	explicit fg, default bg  -> deck's background, and the agent's colour
//	                            fitted against it (hue preserved).
//	default fg, explicit bg  -> the agent's background, and DECK's text
//	                            colour fitted against THAT. The mirror of
//	                            the case above, and for the same reason:
//	                            deck's own text is already open on the row
//	                            (opened for the default/default cells
//	                            before this run began), so "leave the
//	                            foreground alone" is not an option that
//	                            exists here -- the only question is whether
//	                            the pair deck has already half-composed is
//	                            legible. parchment's near-black on an
//	                            agent's dark blue is what fitting prevents.
//	explicit fg, explicit bg -> untouched entirely. The agent chose BOTH
//	                            halves of this pair; it owns the result,
//	                            and deck has no cell to contribute. This
//	                            includes UNDOING a fit deck made a moment
//	                            earlier: an agent that sets its foreground
//	                            and then its background in two separate
//	                            sequences passes through the explicit-fg/
//	                            default-bg case on the way, and the fitted
//	                            colour that case emits must not survive
//	                            into a pair the agent has since completed.
//
// SGR 7 (reverse video) is tracked but deliberately not compensated for:
// with both channels supplied by deck, a reverse run swaps deck's own pair
// and stays exactly as readable (the ratio is symmetric). Fitting is
// skipped while reverse is active, because there the "foreground" is what
// will actually be painted as the background.
func (m Model) repaintForeignDefaults(tok theme.Token, text string) string {
	mode := m.foreignPaint()
	if mode == foreignPaintOff {
		return text
	}
	bgSeq, ok := m.backgroundSGR(tok)
	if !ok {
		// NO_COLOR / DECK_COLOR=0: emit no escape bytes at all, exactly
		// like canvasBackground's own colour-disabled path.
		return text
	}
	canvasBg, bgHexOK := m.tokenHex(tok)
	fgSeq := ""
	if mode != foreignPaintBackgroundOnly {
		if seq, fgOK := m.foregroundSGR(theme.Text); fgOK {
			fgSeq = seq
		}
	}

	// The common row: the agent emitted no SGR at all, so one span covers
	// it and no state machine is needed.
	if !strings.Contains(text, "\x1b") {
		return bgSeq + fgSeq + text
	}

	var (
		b       strings.Builder
		fg, bg  sgrColor
		reverse bool
		// deckFitted records that the foreground currently in effect is
		// one DECK chose (a fitted agent colour, or a fitted canvas text
		// colour), not one the agent asked for -- so a later sequence that
		// hands the pair back to the agent knows there is something to
		// undo.
		deckFitted bool
	)
	b.Grow(len(text) + 64)

	// correct emits whatever deck needs to add on top of the state the
	// agent's own sequence just established.
	correct := func() {
		if !bg.explicit {
			b.WriteString(bgSeq)
		}
		switch {
		case fg.explicit && bg.explicit:
			// The agent owns this pair. If deck fitted this foreground
			// while the background was still deck's, put the agent's own
			// colour back: the fit was measured against a background that
			// is no longer underneath it.
			if deckFitted && fg.hex != "" {
				b.WriteString(m.fgSGRForHex(fg.hex))
				deckFitted = false
			}
		case fg.explicit:
			if mode != foreignPaintFit || reverse || !bgHexOK {
				return
			}
			if adjusted, changed, err := theme.FitForeground(fg.hex, canvasBg, theme.AAFloor); err == nil && changed {
				b.WriteString(m.fgSGRForHex(adjusted))
				deckFitted = true
			}
		case bg.explicit:
			if mode != foreignPaintFit || reverse || fgSeq == "" || bg.hex == "" {
				return
			}
			textHex, ok := m.tokenHex(theme.Text)
			if !ok {
				return
			}
			if adjusted, changed, err := theme.FitForeground(textHex, bg.hex, theme.AAFloor); err == nil && changed {
				b.WriteString(m.fgSGRForHex(adjusted))
				deckFitted = true
			}
		default:
			b.WriteString(fgSeq)
			deckFitted = false
		}
	}

	b.WriteString(bgSeq)
	if fgSeq != "" {
		b.WriteString(fgSeq)
	}
	for i := 0; i < len(text); {
		if text[i] != 0x1b || i+1 >= len(text) || text[i+1] != '[' {
			b.WriteByte(text[i])
			i++
			continue
		}
		// CSI: parameter/intermediate bytes, then one final byte in
		// 0x40..0x7e. Anything that is not SGR ("m") passes through
		// untouched -- deck has no business rewriting a pane's cursor
		// moves or mode changes.
		j := i + 2
		for j < len(text) && (text[j] < 0x40 || text[j] > 0x7e) {
			j++
		}
		if j >= len(text) {
			b.WriteString(text[i:]) // truncated sequence at end of row
			break
		}
		b.WriteString(text[i : j+1])
		if text[j] == 'm' {
			applySGR(text[i+2:j], &fg, &bg, &reverse)
			correct()
		}
		i = j + 1
	}
	return b.String()
}

// applySGR advances one row's colour state by one SGR sequence's
// parameters. Only the colour and reverse-video parameters matter here;
// bold/underline/italic are the pane's business and pass through
// untouched.
//
// The subtlety this must not get wrong is that 39 (default foreground) and
// 49 (default background) also occur as ordinary SUB-parameters of an
// explicit colour: an SGR of 48;2;0;39;0 sets an explicit green background
// and says nothing whatever about the foreground. Each 38/48/58 spec's own
// sub-parameters are therefore consumed, not scanned.
func applySGR(params string, fg, bg *sgrColor, reverse *bool) {
	if strings.TrimSpace(params) == "" {
		*fg, *bg, *reverse = sgrColor{}, sgrColor{}, false // ESC[m == ESC[0m
		return
	}
	fields := strings.Split(params, ";")
	for i := 0; i < len(fields); i++ {
		f := fields[i]
		if strings.ContainsRune(f, ':') {
			// Colon sub-parameter form (38:2::r:g:b) is self-contained:
			// one field, no lookahead to consume. Treat it as explicit
			// without resolving it -- an unresolvable explicit colour is
			// preserved as-is, never fitted, which is the safe direction.
			switch {
			case strings.HasPrefix(f, "38:"):
				*fg = sgrColor{explicit: true}
			case strings.HasPrefix(f, "48:"):
				*bg = sgrColor{explicit: true}
			}
			continue
		}
		n, ok := atoiSGR(f)
		if !ok {
			continue
		}
		switch {
		case n == 0:
			*fg, *bg, *reverse = sgrColor{}, sgrColor{}, false
		case n == 7:
			*reverse = true
		case n == 27:
			*reverse = false
		case n == 39:
			*fg = sgrColor{}
		case n == 49:
			*bg = sgrColor{}
		case n >= 30 && n <= 37:
			*fg = sgrColor{explicit: true, hex: theme.ReferencePalette[n-30]}
		case n >= 90 && n <= 97:
			*fg = sgrColor{explicit: true, hex: theme.ReferencePalette[n-90+8]}
		case n >= 40 && n <= 47:
			*bg = sgrColor{explicit: true, hex: theme.ReferencePalette[n-40]}
		case n >= 100 && n <= 107:
			*bg = sgrColor{explicit: true, hex: theme.ReferencePalette[n-100+8]}
		case n == 38 || n == 48 || n == 58:
			hex, consumed := extendedColor(fields[i+1:])
			c := sgrColor{explicit: hex != "", hex: hex}
			if n == 38 {
				*fg = c
			} else if n == 48 {
				*bg = c
			}
			i += consumed
		}
	}
}

// extendedColor resolves the sub-parameters following a 38/48/58 and
// reports how many fields it consumed. An unrecognised form consumes
// nothing beyond the selector, which is the conservative choice: a
// mis-consumed field could turn a later 39 into a phantom default.
func extendedColor(rest []string) (hex string, consumed int) {
	if len(rest) == 0 {
		return "", 0
	}
	switch rest[0] {
	case "5":
		if len(rest) < 2 {
			return "", 1
		}
		n, ok := atoiSGR(rest[1])
		if !ok {
			return "", 2
		}
		return xterm256Hex(n), 2
	case "2":
		if len(rest) < 4 {
			return "", len(rest)
		}
		r, rOK := atoiSGR(rest[1])
		g, gOK := atoiSGR(rest[2])
		bl, bOK := atoiSGR(rest[3])
		if !rOK || !gOK || !bOK {
			return "", 4
		}
		return fmt.Sprintf("#%02x%02x%02x", clamp255(r), clamp255(g), clamp255(bl)), 4
	case "3", "4":
		// CMY / CMYK, implementation-defined and vanishingly rare. Consume
		// the fields so nothing after them is misread; do not resolve.
		return "", min(4, len(rest))
	default:
		return "", 1
	}
}

// xterm256Hex resolves an xterm 256-colour index: 0-15 the ANSI slots
// (deck's own ReferencePalette, for the same reason quantize.go fixes it
// -- terminals do not agree, and a ratio over an unfixed palette is
// undefined), 16-231 the 6x6x6 cube, 232-255 the 24-step grey ramp.
func xterm256Hex(n int) string {
	switch {
	case n < 0 || n > 255:
		return ""
	case n < 16:
		return theme.ReferencePalette[n]
	case n < 232:
		n -= 16
		steps := [6]int{0, 95, 135, 175, 215, 255}
		return fmt.Sprintf("#%02x%02x%02x", steps[n/36], steps[(n/6)%6], steps[n%6])
	default:
		v := 8 + (n-232)*10
		return fmt.Sprintf("#%02x%02x%02x", v, v, v)
	}
}

// tokenHex is the canvas colour a fitted agent foreground is measured
// against, taken at the ACTIVE colour depth so the measurement matches
// what the terminal will actually show -- the same split sgrForToken makes
// between Color and QuantizedColor.
func (m Model) tokenHex(tok theme.Token) (string, bool) {
	th := m.activeTheme()
	if th == nil {
		return "", false
	}
	var (
		hex string
		err error
	)
	if m.settings.ColorDepth == "16" {
		hex, err = th.QuantizedColor(tok)
	} else {
		hex, err = th.Color(tok)
	}
	if err != nil {
		return "", false
	}
	return hex, true
}

// fgSGRForHex renders a fitted agent colour as a foreground SGR at the
// active colour depth. Emitting 38;2 truecolour unconditionally would be
// wrong on a 16-colour terminal, which is exactly where deck's own tokens
// go through ANSI16Code instead.
func (m Model) fgSGRForHex(hex string) string {
	if m.settings.ColorDepth == "16" {
		q, err := theme.QuantizeHex(hex)
		if err != nil {
			return ""
		}
		code, ok := theme.ANSI16Code(q)
		if !ok {
			return ""
		}
		return fmt.Sprintf("\x1b[%dm", code)
	}
	r, g, b, err := theme.HexRGB(hex)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("\x1b[38;2;%d;%d;%dm", r, g, b)
}

func clamp255(n int) int {
	if n < 0 {
		return 0
	}
	if n > 255 {
		return 255
	}
	return n
}

func atoiSGR(f string) (int, bool) {
	if f == "" {
		return 0, false
	}
	n := 0
	for i := 0; i < len(f); i++ {
		if f[i] < '0' || f[i] > '9' {
			return 0, false
		}
		n = n*10 + int(f[i]-'0')
	}
	return n, true
}
