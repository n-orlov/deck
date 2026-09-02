package tmux

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// IsizeGeometryOption is the window-scoped user option that carries the
// window's pre-entry geometry across an arbitrary chain of interactive-mode
// steals (PRD phase3b/3i R100, SPEC.md §11.9). Like OwnershipOption it is
// scoped to the WINDOW, never global or session: two different windows
// each remember their own pre-entry geometry independently, and a steal on
// one window must never disturb what another window has recorded.
//
// R100's hazard, restated: a stealer's own CaptureWindowGeometry reads the
// PREVIOUS holder's already-fitted size, not the size the window had
// before any deck process ever touched it. Recording the ORIGINAL geometry
// once, in this option, and having every later claimant read it back
// rather than re-capture, is what lets an arbitrary chain of steals still
// restore byte-exactly to the window's pre-deck state when the last
// holder lets go legitimately.
const IsizeGeometryOption = "@deck_isize_geometry"

// EncodeIsizeGeometry renders a WindowGeometry in the fixed R100 wire
// form: `<width>x<height>:<window-size-value>`, with an empty final field
// when WindowSizeSet is false. This is the ONLY encoding this option is
// ever written in -- fixed by the PRD so nothing drifts across the
// multiple call sites (first-claimant capture, reclaim's resolved-original
// carry, a future rework) that will eventually write it.
func EncodeIsizeGeometry(geometry WindowGeometry) string {
	value := ""
	if geometry.WindowSizeSet {
		value = geometry.WindowSizeValue
	}
	return fmt.Sprintf("%dx%d:%s", geometry.Width, geometry.Height, value)
}

// ParseIsizeGeometry parses the fixed R100 wire form back into a
// WindowGeometry. A value that does not parse -- missing the `x` split,
// missing the `:` field separator, or a non-integer width/height -- is
// treated as ABSENT (ok=false), never as an error: the PRD states this
// explicitly ("A value that does not parse is treated as absent"), so a
// hand-edited or foreign-written option value never wedges a claimant that
// only wants to know whether a usable geometry is already recorded there.
//
// An empty final field parses back to WindowSizeSet=false, the exact
// inverse of EncodeIsizeGeometry's own empty-field case; any other final
// field (including an empty-looking but present window-size value, which
// tmux's own window-size option never actually produces) parses to
// WindowSizeSet=true with that field verbatim as WindowSizeValue.
func ParseIsizeGeometry(value string) (geometry WindowGeometry, ok bool) {
	dimensions, sizeField, found := strings.Cut(value, ":")
	if !found {
		return WindowGeometry{}, false
	}
	widthPart, heightPart, found := strings.Cut(dimensions, "x")
	if !found {
		return WindowGeometry{}, false
	}
	width, err := strconv.Atoi(widthPart)
	if err != nil {
		return WindowGeometry{}, false
	}
	height, err := strconv.Atoi(heightPart)
	if err != nil {
		return WindowGeometry{}, false
	}
	if sizeField == "" {
		return WindowGeometry{Width: width, Height: height, WindowSizeSet: false}, true
	}
	return WindowGeometry{Width: width, Height: height, WindowSizeSet: true, WindowSizeValue: sizeField}, true
}

// readWindowIsizeGeometry reads IsizeGeometryOption in the window scope
// and parses it. Like OwnershipOption, this is a "@"-prefixed USER option,
// so an unset window reports tmux's "invalid option" error shape (task
// 028's distinction), not a builtin option's empty-line-exit-0 shape --
// readWindowOwnership's own doc comment covers the same distinction for
// its sibling option, and this function treats it identically. An
// unparseable stored value (should never arise from EncodeIsizeGeometry
// itself, but could from a hand-edited option or a foreign writer) is
// reported as absent (ok=false) rather than an error, per R100's "a value
// that does not parse is treated as absent".
func (c Client) readWindowIsizeGeometry(ctx context.Context, target string) (geometry WindowGeometry, ok bool, err error) {
	commandCtx, cancel := context.WithTimeout(ctx, c.timeout())
	defer cancel()
	output, cmdErr := c.command(commandCtx, "show-options", "-wv", "-t", target, IsizeGeometryOption).CombinedOutput()
	trimmed := strings.TrimRight(string(output), "\n")
	if cmdErr != nil {
		if strings.Contains(trimmed, "invalid option") {
			return WindowGeometry{}, false, nil
		}
		return WindowGeometry{}, false, fmt.Errorf("tmux -L %s show-options -wv -t %s %s: %w: %s", c.Socket, target, IsizeGeometryOption, cmdErr, trimmed)
	}
	if trimmed == "" {
		return WindowGeometry{}, false, nil
	}
	geometry, parsed := ParseIsizeGeometry(trimmed)
	if !parsed {
		return WindowGeometry{}, false, nil
	}
	return geometry, true, nil
}

// writeWindowIsizeGeometry writes IsizeGeometryOption in the window scope
// to geometry's fixed R100 encoding.
func (c Client) writeWindowIsizeGeometry(ctx context.Context, target string, geometry WindowGeometry) error {
	if _, err := c.run(ctx, "set-option", "-w", "-t", target, IsizeGeometryOption, EncodeIsizeGeometry(geometry)); err != nil {
		return fmt.Errorf("record %s on %q: %w", IsizeGeometryOption, target, err)
	}
	return nil
}

// ResolveIsizeGeometry implements SPEC.md §11.9's "written by whoever
// finds none there and read -- never rewritten -- by whoever takes the
// claim afterwards": if IsizeGeometryOption is absent on target, it
// writes captured (the caller's own, just-taken CaptureWindowGeometry)
// and returns it unchanged, exactly the first-claimant case; if the
// option is already present -- a steal, by force or otherwise -- it reads
// the value already recorded there and returns THAT instead, never
// writing anything at all. captured is consulted only on the absent
// branch; on the present branch it is ignored entirely, which is what
// keeps the original pre-entry geometry from drifting at each successive
// steal (a stealer's own CaptureWindowGeometry sees the previous holder's
// already-fitted size, never the window's true pre-deck size).
//
// The caller is expected to call this only once it has actually taken
// the claim (ClaimWindowOwnership/ForceClaimWindowOwnership acquired);
// this function itself performs no ownership check and has no opinion on
// whether the caller is entitled to write -- the same separation the
// other helpers in this file keep from their own callers' gating.
func (c Client) ResolveIsizeGeometry(ctx context.Context, target string, captured WindowGeometry) (WindowGeometry, error) {
	existing, ok, err := c.readWindowIsizeGeometry(ctx, target)
	if err != nil {
		return WindowGeometry{}, err
	}
	if ok {
		return existing, nil
	}
	if err := c.writeWindowIsizeGeometry(ctx, target, captured); err != nil {
		return WindowGeometry{}, err
	}
	return captured, nil
}

// unsetWindowIsizeGeometry unsets IsizeGeometryOption in the window scope
// -- the "restored and cleared" half of R100's "restored and cleared by
// the last holder to let go legitimately, and by nobody else". Whether
// this call is reached at all is exactly that gating, decided entirely by
// the caller (a later task wires the claim-still-mine check this option's
// own encode/parse pair does not need to know about); this helper itself
// performs no such check, the same separation ownership.go's own
// unsetWindowOwnership keeps from ClaimWindowOwnership's caller-side
// gating.
func (c Client) unsetWindowIsizeGeometry(ctx context.Context, target string) error {
	if _, err := c.run(ctx, "set-option", "-w", "-u", "-t", target, IsizeGeometryOption); err != nil {
		return fmt.Errorf("clear %s on %q: %w", IsizeGeometryOption, target, err)
	}
	return nil
}
