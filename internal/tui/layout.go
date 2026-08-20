package tui

// §11.2 layout modes: one pure, unit-tested geometry function.
//
// ComputeLayout maps (terminal width, terminal rows, a pinned mode, and the
// persisted sidebar_width) to the sidebar and preview panel rectangles. It
// reads nothing from the store and writes nothing anywhere — callers own
// persistence, this function only computes shape. In particular it never
// "overwrites" a pinned mode: LayoutResult.Mode is the *pin as requested*,
// unchanged, and LayoutResult.Effective is what actually gets rendered this
// frame when the pin cannot hold its floors at the current size.

const (
	// LayoutAuto lets the viewport width choose side-by-side or stacked.
	LayoutAuto = "auto"
	// LayoutSideBySide pins the sidebar-beside-preview layout.
	LayoutSideBySide = "side-by-side"
	// LayoutStacked pins the sidebar-above-preview layout.
	LayoutStacked = "stacked"
	// LayoutCollapsed pins the 3-column strip; never chosen by auto.
	LayoutCollapsed = "collapsed"

	// SidebarWidthFloor is the narrowest sidebar that still shows
	// glyph+name+status with nothing elided (§11.2).
	SidebarWidthFloor = 24
	// PreviewWidthFloor is the narrowest crop that still carries meaning
	// in side-by-side mode (§11.2).
	PreviewWidthFloor = 40
	// CollapsedStripWidth is the `»` glyph plus its two borders (§11.2).
	CollapsedStripWidth = 3
	// StackedPreviewFloor is the preview's minimum height when stacked
	// below the list (§11.2).
	StackedPreviewFloor = 8
	// StackedListMin and StackedListMax bound the stacked list's height:
	// min(max(rows/3, 5), 12) (§11.2).
	StackedListMin = 5
	StackedListMax = 12

	// AutoSideBySideWidth is deck's own supported minimum column count;
	// auto chooses side-by-side at or above it (§11.2).
	AutoSideBySideWidth = 80
	// MinRows is deck's own supported minimum row count.
	MinRows = 24
)

// Rect is a panel rectangle in terminal cells. Width and Height are total
// columns/rows for that panel, including its own borders and padding
// (§11.2's floors are stated the same way).
type Rect struct {
	X, Y, Width, Height int
}

// LayoutResult is the outcome of one ComputeLayout call.
type LayoutResult struct {
	// Requested is the pin passed in, verbatim (LayoutAuto if empty).
	Requested string
	// Effective is the mode actually rendered this frame: it differs
	// from Requested only when Requested pinned a mode that cannot hold
	// its floors at the current size, in which case it is the auto
	// choice for that size. The caller must not persist Effective over
	// Requested — the pin itself is unchanged by this fallback.
	Effective string
	// Fallback is true only when Requested pinned an explicit mode
	// (not auto) that could not hold its floors, so Effective differs
	// from the pin for this frame. Auto choosing a mode based on width
	// is not a fallback, so Fallback is always false when Requested is
	// LayoutAuto.
	Fallback bool
	// BelowMinimum is true when the viewport is narrower than 80 columns
	// or shorter than 24 rows, deck's own supported minimum.
	BelowMinimum bool
	// SidebarWidth is the effective sidebar width used for this frame's
	// Sidebar rectangle in side-by-side mode (clamped into
	// [SidebarWidthFloor, width-PreviewWidthFloor]). In stacked and
	// collapsed modes it plays no role in the geometry and is reported
	// as the clamped column-adjustment value used by `<`/`>` (task 015)
	// so callers have one place to read it from regardless of mode.
	SidebarWidth int
	// Sidebar and Preview are the two panel rectangles for this frame.
	Sidebar Rect
	Preview Rect
}

// nextLayoutMode is `|`'s cycle (§11.2/§11.8 requirement 9): auto →
// side-by-side → stacked → collapsed → auto. It always returns an explicit
// pin (never "") except when cycling off collapsed back to auto, at which
// point it returns LayoutAuto itself — the caller stores this verbatim as
// the new pin, so unlike ComputeLayout's fallback this is never silently
// overwritten later; the user asked for auto by cycling all the way around.
func nextLayoutMode(current string) string {
	switch current {
	case LayoutSideBySide:
		return LayoutStacked
	case LayoutStacked:
		return LayoutCollapsed
	case LayoutCollapsed:
		return LayoutAuto
	default: // "" or LayoutAuto (or any unknown value)
		return LayoutSideBySide
	}
}

// ClampSidebarWidth clamps a requested sidebar_width to
// [SidebarWidthFloor, width-PreviewWidthFloor], the same bound `<`/`>`
// (task 015) must respect. When width is too narrow to hold both floors,
// the upper bound collapses to SidebarWidthFloor so the result is always
// well-defined.
func ClampSidebarWidth(width, sidebarWidth int) int {
	hi := width - PreviewWidthFloor
	if hi < SidebarWidthFloor {
		hi = SidebarWidthFloor
	}
	if sidebarWidth < SidebarWidthFloor {
		return SidebarWidthFloor
	}
	if sidebarWidth > hi {
		return hi
	}
	return sidebarWidth
}

// autoMode picks side-by-side at deck's own supported minimum width and
// stacked below it. Collapsed is never chosen automatically (§11.2).
func autoMode(width int) string {
	if width >= AutoSideBySideWidth {
		return LayoutSideBySide
	}
	return LayoutStacked
}

// sideBySideFits reports whether side-by-side can hold both its floors
// (sidebar 24 + preview 40 = 64 total columns) at the given width.
func sideBySideFits(width int) bool {
	return width >= SidebarWidthFloor+PreviewWidthFloor
}

// stackedListHeight is §11.2's min(max(rows/3, 5), 12).
func stackedListHeight(rows int) int {
	h := rows / 3
	if h < StackedListMin {
		h = StackedListMin
	}
	if h > StackedListMax {
		h = StackedListMax
	}
	return h
}

// ComputeLayout is the single §11.2 geometry function. width and rows are
// the full terminal size; pinned is "" (meaning auto), "side-by-side",
// "stacked" or "collapsed"; sidebarWidth is the persisted sidebar_width
// (0 or negative means "use the default").
func ComputeLayout(width, rows int, pinned string, sidebarWidth int) LayoutResult {
	requested := pinned
	if requested == "" {
		requested = LayoutAuto
	}
	if sidebarWidth <= 0 {
		sidebarWidth = 35 // §11.2 default; internal/store.DefaultSidebarWidth
	}

	effective := requested
	switch requested {
	case LayoutAuto:
		effective = autoMode(width)
	case LayoutSideBySide:
		if !sideBySideFits(width) {
			effective = autoMode(width)
		}
	case LayoutStacked, LayoutCollapsed:
		// Stacked is itself the degrade-as-far-as-it-fits mode (§11.2's
		// "below 80×24 ... renders the stacked mode as far as it
		// fits"), and the collapsed strip has no floor to violate at
		// any width ≥ its own 3 columns, so neither pin ever falls
		// back to auto.
		effective = requested
	default:
		// Unknown value: behave as auto rather than render nothing.
		effective = autoMode(width)
	}

	result := LayoutResult{
		Requested: requested,
		Effective: effective,
		// Fallback only means something for an explicit pin: auto is
		// *supposed* to change with width, so that is not a fallback.
		Fallback:     requested != LayoutAuto && effective != requested,
		BelowMinimum: width < AutoSideBySideWidth || rows < MinRows,
	}

	switch effective {
	case LayoutSideBySide:
		sw := ClampSidebarWidth(width, sidebarWidth)
		result.SidebarWidth = sw
		result.Sidebar = Rect{X: 0, Y: 0, Width: sw, Height: rows}
		result.Preview = Rect{X: sw, Y: 0, Width: width - sw, Height: rows}
	case LayoutCollapsed:
		result.SidebarWidth = ClampSidebarWidth(width, sidebarWidth)
		result.Sidebar = Rect{X: 0, Y: 0, Width: CollapsedStripWidth, Height: rows}
		result.Preview = Rect{X: CollapsedStripWidth, Y: 0, Width: width - CollapsedStripWidth, Height: rows}
	default: // LayoutStacked
		result.SidebarWidth = ClampSidebarWidth(width, sidebarWidth)
		listHeight := stackedListHeight(rows)
		if listHeight > rows {
			listHeight = rows
		}
		previewHeight := rows - listHeight
		if previewHeight < 0 {
			previewHeight = 0
		}
		result.Sidebar = Rect{X: 0, Y: 0, Width: width, Height: listHeight}
		result.Preview = Rect{X: 0, Y: listHeight, Width: width, Height: previewHeight}
	}

	return result
}
