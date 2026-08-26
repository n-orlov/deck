package tui

import (
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/theme"
)

// This file is task 318's own tests (R57, SPEC amendment 6584299): the
// seam between the sidebar and the preview panel — and the ┬/┴ junctions
// where it meets their top/bottom borders — reads as `border_focus`
// whenever EITHER panel is focused, and plain `border` only when neither
// is (some overlay atop mainView, e.g. the `t` theme picker or the `/`
// filter, owns the keyboard instead). Before this task the seam followed
// previewBorderToken alone, so it read as plain `border` whenever the
// sidebar — not the preview — held focus; TestSeamIsFocusedWhenSidebarHasFocus
// is this task's fixture trap: it is RED against that hard-coded
// previewBorderToken implementation (revert panel.go's previewTopLine/
// previewBottomLine/previewContentLine and seamBorderToken to see it fail).

// seamColumnFor returns the frame column the shared seam renders at for a
// side-by-side (or collapsed-strip, which is still side-by-side) layout.
func seamColumnFor(t *testing.T, m Model) int {
	t.Helper()
	layout := m.computeLayout()
	if layout.Effective == LayoutStacked {
		t.Fatalf("frame computed as stacked, want side-by-side/collapsed (test assumes a shared seam)")
	}
	return layout.Sidebar.Width
}

// frameTopRow locates the row the sidebar's own top border (and thus the
// seam's top T-junction one column to its right) renders on within view.
// Banners the theme picker/filter/theme-fallback/etc. grow above the
// panels (tui.go's `reserved` budget) push the frame down by one row per
// banner line, so row 0 is only ever the frame's top border when no banner
// is showing -- this scans for the actual row instead of assuming that.
func frameTopRow(t *testing.T, m Model, view string) int {
	t.Helper()
	left := m.box().topLeft
	for i, line := range strings.Split(stripANSI(view), "\n") {
		if strings.HasPrefix(line, left) {
			return i
		}
	}
	t.Fatalf("no frame top border (%q) found in view", left)
	return -1
}

// frameBottomRow is frameTopRow's mirror: it locates the row the frame's
// own bottom border (and thus the seam's bottom T-junction) renders on.
func frameBottomRow(t *testing.T, m Model, view string) int {
	t.Helper()
	left := m.box().bottomLeft
	lines := strings.Split(stripANSI(view), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.HasPrefix(lines[i], left) {
			return i
		}
	}
	t.Fatalf("no frame bottom border (%q) found in view", left)
	return -1
}

// frameBodyRow returns a row strictly between the frame's top and bottom
// border rows -- a BODY row, as opposed to either T-junction -- so tests
// can assert the seam's plain content-line colour (previewContentLine's
// seamBorderToken call, panel.go:544-550) rather than only the corner
// glyphs previewTopLine/previewBottomLine draw. The row immediately below
// the top border is always a content row for any box at least 3 rows
// tall (top border + >=1 content row + bottom border), which every
// scenario this helper is used from satisfies.
func frameBodyRow(t *testing.T, m Model, view string) int {
	t.Helper()
	top := frameTopRow(t, m, view)
	bottom := frameBottomRow(t, m, view)
	body := top + 1
	if body >= bottom {
		t.Fatalf("frame too short to have a body row: top=%d bottom=%d", top, bottom)
	}
	return body
}

// TestSeamBodyRowIsFocusedWhenSidebarHasFocus, TestSeamBodyRowStaysFocusedInInteractiveMode
// and TestSeamBodyRowIsPlainBorderWhenNeitherPanelFocused are task 404's own
// tests (R57 coverage gap): the three T-junction tests above only sample
// the seam's TOP corner glyph, which previewTopLine draws via its own
// leftTok := m.seamBorderToken() branch (panel.go:514-524) -- a body row's
// plain vertical bar comes from a SEPARATE call site, previewContentLine
// (panel.go:544-550), that could regress independently (e.g. reverted to
// the hard-coded m.previewBorderToken() task 318's own comment says the
// seam used before that task) while every T-junction test above kept
// passing. These three mirror the T-junction tests' three focus states
// but read column sw at a BODY row instead of row 0.
func TestSeamBodyRowIsFocusedWhenSidebarHasFocus(t *testing.T) {
	m := mainViewColorTestModel(t)
	focusHex := tokenHex(t, m, theme.BorderFocus)
	borderHex := tokenHex(t, m, theme.Border)
	if focusHex == borderHex {
		t.Skip("this theme's border_focus and border tokens happen to share a colour; the distinctness assertion below would be vacuous")
	}
	sw := seamColumnFor(t, m)

	view := m.View()
	body := frameBodyRow(t, m, view)
	term := renderSettingsToEmulator(t, view, m.width, m.height)
	seamFg, ok := cellFgHex(t, term, sw, body)
	if !ok {
		t.Fatalf("seam body cell has no foreground colour")
	}
	if seamFg != focusHex {
		t.Fatalf("sidebar focused: seam body row %d = %s, want border_focus token %s (either-panel-focused rule)", body, seamFg, focusHex)
	}
}

func TestSeamBodyRowStaysFocusedInInteractiveMode(t *testing.T) {
	m := mainViewColorTestModel(t)
	focusHex := tokenHex(t, m, theme.BorderFocus)
	borderHex := tokenHex(t, m, theme.Border)
	if focusHex == borderHex {
		t.Skip("this theme's border_focus and border tokens happen to share a colour; the distinctness assertion below would be vacuous")
	}
	m.interactive = true
	sw := seamColumnFor(t, m)

	view := m.View()
	body := frameBodyRow(t, m, view)
	term := renderSettingsToEmulator(t, view, m.width, m.height)
	seamFg, ok := cellFgHex(t, term, sw, body)
	if !ok {
		t.Fatalf("seam body cell has no foreground colour")
	}
	if seamFg != focusHex {
		t.Fatalf("interactive (preview focused): seam body row %d = %s, want border_focus token %s", body, seamFg, focusHex)
	}
}

func TestSeamBodyRowIsPlainBorderWhenNeitherPanelFocused(t *testing.T) {
	for _, tc := range []struct {
		name string
		set  func(*Model)
	}{
		{"theme picker open", func(m *Model) { m.themePicking = true }},
		{"filter input focused", func(m *Model) { m.filtering = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := mainViewColorTestModel(t)
			focusHex := tokenHex(t, m, theme.BorderFocus)
			borderHex := tokenHex(t, m, theme.Border)
			if focusHex == borderHex {
				t.Skip("this theme's border_focus and border tokens happen to share a colour; the distinctness assertion below would be vacuous")
			}
			sw := seamColumnFor(t, m)
			tc.set(&m)

			view := m.View()
			body := frameBodyRow(t, m, view)
			term := renderSettingsToEmulator(t, view, m.width, m.height)
			seamFg, ok := cellFgHex(t, term, sw, body)
			if !ok {
				t.Fatalf("seam body cell has no foreground colour")
			}
			if seamFg != borderHex {
				t.Fatalf("%s: seam body row %d = %s, want plain border token %s (neither panel focused)", tc.name, body, seamFg, focusHex)
			}
		})
	}
}

func TestSeamIsFocusedWhenSidebarHasFocus(t *testing.T) {
	m := mainViewColorTestModel(t)
	focusHex := tokenHex(t, m, theme.BorderFocus)
	borderHex := tokenHex(t, m, theme.Border)
	if focusHex == borderHex {
		t.Skip("this theme's border_focus and border tokens happen to share a colour; the distinctness assertion below would be vacuous")
	}
	sw := seamColumnFor(t, m)

	view := m.View()
	row := frameTopRow(t, m, view)
	term := renderSettingsToEmulator(t, view, m.width, m.height)
	seamFg, ok := cellFgHex(t, term, sw, row)
	if !ok {
		t.Fatalf("seam corner has no foreground colour")
	}
	if seamFg != focusHex {
		t.Fatalf("sidebar focused: seam = %s, want border_focus token %s (either-panel-focused rule)", seamFg, focusHex)
	}
}

func TestSeamStaysFocusedInInteractiveMode(t *testing.T) {
	m := mainViewColorTestModel(t)
	focusHex := tokenHex(t, m, theme.BorderFocus)
	borderHex := tokenHex(t, m, theme.Border)
	if focusHex == borderHex {
		t.Skip("this theme's border_focus and border tokens happen to share a colour; the distinctness assertion below would be vacuous")
	}
	m.interactive = true
	sw := seamColumnFor(t, m)

	view := m.View()
	row := frameTopRow(t, m, view)
	term := renderSettingsToEmulator(t, view, m.width, m.height)
	seamFg, ok := cellFgHex(t, term, sw, row)
	if !ok {
		t.Fatalf("seam corner has no foreground colour")
	}
	if seamFg != focusHex {
		t.Fatalf("interactive (preview focused): seam = %s, want border_focus token %s", seamFg, focusHex)
	}
}

// TestSeamIsPlainBorderWhenNeitherPanelFocused covers the negative: while
// the theme picker (`t`) or the filter (`/`) owns the keyboard, mainView
// keeps rendering underneath (theme_picker.go/filter.go's own file
// comments) but neither panel is the thing actually holding focus, so the
// seam must NOT read as border_focus the way a hard-coded "always focused
// unless interactive-swaps-it" implementation would.
func TestSeamIsPlainBorderWhenNeitherPanelFocused(t *testing.T) {
	for _, tc := range []struct {
		name string
		set  func(*Model)
	}{
		{"theme picker open", func(m *Model) { m.themePicking = true }},
		{"filter input focused", func(m *Model) { m.filtering = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := mainViewColorTestModel(t)
			focusHex := tokenHex(t, m, theme.BorderFocus)
			borderHex := tokenHex(t, m, theme.Border)
			if focusHex == borderHex {
				t.Skip("this theme's border_focus and border tokens happen to share a colour; the distinctness assertion below would be vacuous")
			}
			sw := seamColumnFor(t, m)
			tc.set(&m)

			view := m.View()
			row := frameTopRow(t, m, view)
			term := renderSettingsToEmulator(t, view, m.width, m.height)
			seamFg, ok := cellFgHex(t, term, sw, row)
			if !ok {
				t.Fatalf("seam corner has no foreground colour")
			}
			if seamFg != borderHex {
				t.Fatalf("%s: seam = %s, want plain border token %s (neither panel focused)", tc.name, seamFg, focusHex)
			}
		})
	}
}

// TestStackedModeUnaffectedBySeamRule is task 318's own regression guard:
// the stacked layout ("each keeps all four of its own borders" per
// renderStackedFrame's comment) has no seam at all, so it must keep
// splitting border_focus/border per panel via fullBoxTop/fullBoxBottom/
// fullBoxContentLine's own explicit focused bool exactly as before —
// seamBorderToken/mainViewOverlayActive are never consulted on this path.
func TestStackedModeUnaffectedBySeamRule(t *testing.T) {
	m := mainViewColorTestModel(t)
	focusHex := tokenHex(t, m, theme.BorderFocus)
	borderHex := tokenHex(t, m, theme.Border)
	if focusHex == borderHex {
		t.Skip("this theme's border_focus and border tokens happen to share a colour; the distinctness assertion below would be vacuous")
	}
	m.width, m.height = 60, 30
	layout := m.computeLayout()
	if layout.Effective != LayoutStacked {
		t.Fatalf("60x30 frame computed as %s, want stacked", layout.Effective)
	}

	view := m.View()
	topRow := frameTopRow(t, m, view)
	term := renderSettingsToEmulator(t, view, m.width, m.height)
	sidebarFg, ok := cellFgHex(t, term, 0, topRow)
	if !ok {
		t.Fatalf("sidebar box's top-left corner has no foreground colour")
	}
	if sidebarFg != focusHex {
		t.Fatalf("stacked sidebar (focused) corner = %s, want border_focus token %s", sidebarFg, focusHex)
	}
	previewTopRow := topRow + layout.Sidebar.Height
	previewFg, ok := cellFgHex(t, term, 0, previewTopRow)
	if !ok {
		t.Fatalf("preview box's top-left corner has no foreground colour")
	}
	if previewFg != borderHex {
		t.Fatalf("stacked preview (unfocused) corner = %s, want border token %s", previewFg, borderHex)
	}
}

// TestSeamRuleAppliesWhenCollapsed proves the same either-panel-focused
// seam rule holds when the sidebar is pinned to the 3-column collapsed
// strip (still side-by-side with the preview, renderSideBySideFrame's own
// seam=true call site is unconditional on layout.Effective): focused ->
// border_focus, and an overlay atop mainView (neither panel focused) ->
// plain border, exactly as the full-width sidebar case above.
func TestSeamRuleAppliesWhenCollapsed(t *testing.T) {
	m := mainViewColorTestModel(t)
	focusHex := tokenHex(t, m, theme.BorderFocus)
	borderHex := tokenHex(t, m, theme.Border)
	if focusHex == borderHex {
		t.Skip("this theme's border_focus and border tokens happen to share a colour; the distinctness assertion below would be vacuous")
	}
	m.layoutMode = LayoutCollapsed
	sw := seamColumnFor(t, m)

	view := m.View()
	row := frameTopRow(t, m, view)
	term := renderSettingsToEmulator(t, view, m.width, m.height)
	seamFg, ok := cellFgHex(t, term, sw, row)
	if !ok {
		t.Fatalf("collapsed strip's seam corner has no foreground colour")
	}
	if seamFg != focusHex {
		t.Fatalf("collapsed, sidebar focused: seam = %s, want border_focus token %s", seamFg, focusHex)
	}

	m.themePicking = true
	view = m.View()
	row = frameTopRow(t, m, view)
	term = renderSettingsToEmulator(t, view, m.width, m.height)
	seamFg, ok = cellFgHex(t, term, sw, row)
	if !ok {
		t.Fatalf("collapsed strip's seam corner has no foreground colour (theme picker open)")
	}
	if seamFg != borderHex {
		t.Fatalf("collapsed, theme picker open (neither focused): seam = %s, want border token %s", seamFg, borderHex)
	}
}
