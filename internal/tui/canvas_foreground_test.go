package tui

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"testing"

	"github.com/charmbracelet/x/vt"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/theme"
)

// This file is GH #24's SECOND report -- "light themes still show white
// session text on light background, almost unreadable", filed against
// parchment AFTER R118 (deck paints its own canvas) and R119 (the
// selection gutter) shipped -- reduced to a per-cell property and proved
// red-first.
//
// R118's own success criteria only ever named the BACKGROUND half: "a
// rendered frame's every cell that deck owns carries the theme's
// background". It says nothing about the foreground, and before this task
// canvasBackground (panel.go) opened only a background. So every run of
// deck's own copy that never opened a foreground of its own -- and there
// are many, because the row/header/footer/placeholder builders correctly
// leave colour to whoever paints the line -- rendered in the TERMINAL's
// default foreground on top of the theme's own canvas. On a terminal whose
// default foreground is white or near-white (the common case, and the
// exact case for an operator who then picks a LIGHT theme), that is
// white-on-cream: R118 painted the canvas and left the text behind.
//
// Measured on a real parchment frame at HEAD before the fix (deck driven
// under a pty at TERM=tmux-256color/COLORTERM=truecolor against a copy of
// a live state DB, the raw byte stream walked with an SGR state machine):
// 30 deck-owned runs carried NO explicit foreground at all --
//
//	x3   "socket: deck"                                (sidebarEntries, tui.go)
//	x1   the preview placeholder's cwd line            (previewPlaceholderLines)
//	x1   "No live preview captured for this row yet."  (previewPlaceholderLines)
//	x1   "77x40 of 116x44"                             (cropPreviewBottomLeft's geometry line)
//	x16  the crop marker "»"                            (cropRow -> paintForeignFill)
//	x1   the footer's status-reason line                (footerLine)
//	x7   the footer legend's " · " separators           (footerLegendWithin)
//
// -- every one of them inside a cell already carrying theme.Background
// (#f5ecd7). Their contrast ratio is not "low", it is UNDEFINED: it
// depends entirely on the terminal's own default foreground, which is
// exactly the dependency R118 set out to remove.
//
// The tests below state the property in the only place it cannot be
// side-stepped -- the rendered cell grid, per built-in theme -- rather
// than enumerating those seven strings, so a builder added later that
// forgets to colour its text is caught the same way.

// canvasForegroundFrame is one Model state whose EVERY rendered cell is
// deck's own composition: no captured tmux pane bytes reach the frame at
// all, so "every non-blank cell carries an explicit foreground" holds
// without a single carve-out. A state that DOES embed a capture is tested
// separately (TestCropMarkerAndGeometryLineCarryExplicitForeground below),
// where the foreign rows are named and skipped by provenance rather than
// by guesswork.
type canvasForegroundFrame struct {
	name  string
	build func(t *testing.T, bt *theme.Theme) Model
}

// canvasForegroundSessions is a deliberately mixed session list: several
// §7 statuses (so the status-word token varies per row), a workspace so
// grouping draws headers, a long name that forces padTrunc to truncate,
// and enough rows for the alternating `surface` stripe to appear on some
// and not others.
func canvasForegroundSessions() []store.Session {
	return []store.Session{
		{ID: "s1", Name: "api-refactor", Agent: "claude", Status: "waiting", CWD: "/home/op/work/api", CreatedAt: 1000, StatusReason: "idle_prompt"},
		{ID: "s2", Name: "a-very-long-session-name-that-will-not-fit-in-the-sidebar", Agent: "codex", Status: "running", CWD: "/home/op/work/web", CreatedAt: 2000},
		{ID: "s3", Name: "starting-one", Agent: "shell", Status: "starting", CWD: "/home/op/work/ops", CreatedAt: 3000},
		{ID: "s4", Name: "stopped-one", Agent: "claude", Status: "stopped", CWD: "/home/op/work/ops", CreatedAt: 4000},
		{ID: "s5", Name: "broken-one", Agent: "codex", Status: "error", CWD: "/home/op/work/ops", CreatedAt: 5000, StatusReason: "pane failed after the stale frame"},
	}
}

func canvasForegroundFrames() []canvasForegroundFrame {
	return []canvasForegroundFrame{
		{
			// The main side-by-side frame with the placeholder preview: the
			// sidebar's socket line, its session rows (selected, marked and
			// plain), the preview placeholder's cwd + "No live preview
			// captured..." copy, and the footer's reason + legend -- five of
			// the seven measured no-foreground runs live here.
			name: "main side-by-side, placeholder preview",
			build: func(t *testing.T, bt *theme.Theme) Model {
				m := New(nil, config.Settings{Color: true, Theme: bt, Socket: "deck"}, "")
				m.width, m.height = 110, 32
				m.sessions = canvasForegroundSessions()
				m.selected = rowCursor(0)
				m.marked = map[string]bool{"s2": true}
				if got := m.computeLayout().Effective; got != LayoutSideBySide {
					t.Fatalf("theme %q: frame computed as %q, want %q", bt.Name, got, LayoutSideBySide)
				}
				return m
			},
		},
		{
			// The empty state: "No sessions yet. Press n to create a
			// session." is composed by sidebarEntries as plain text too.
			name: "empty store",
			build: func(t *testing.T, bt *theme.Theme) Model {
				m := New(nil, config.Settings{Color: true, Theme: bt, Socket: "deck"}, "")
				m.width, m.height = 100, 30
				return m
			},
		},
		{
			// The stacked frame, whose rows go through fullBoxContentLine
			// rather than sidebarContentLine -- a separate builder, and the
			// one task 322 found had been missed entirely once before.
			name: "stacked layout",
			build: func(t *testing.T, bt *theme.Theme) Model {
				m := New(nil, config.Settings{Color: true, Theme: bt, Socket: "deck"}, "")
				m.width, m.height = 70, 34
				m.layoutMode = LayoutStacked
				m.sessions = canvasForegroundSessions()
				m.selected = rowCursor(1)
				if got := m.computeLayout().Effective; got != LayoutStacked {
					t.Fatalf("theme %q: frame computed as %q, want %q", bt.Name, got, LayoutStacked)
				}
				return m
			},
		},
		{
			// A filter in force: filterStatusLine goes through
			// canvasWrapText, a third composition path (unpadded banner
			// lines outside any bordered panel).
			name: "filter in force",
			build: func(t *testing.T, bt *theme.Theme) Model {
				m := New(nil, config.Settings{Color: true, Theme: bt, Socket: "deck"}, "")
				m.width, m.height = 110, 32
				m.sessions = canvasForegroundSessions()[:2]
				m.selected = rowCursor(0)
				m.filterQuery = "api"
				return m
			},
		},
		{
			// The settings takeover: the six settingsLeft*/settingsRight*
			// builders plus its own footer line.
			name: "settings takeover",
			build: func(t *testing.T, bt *theme.Theme) Model {
				m := New(nil, config.Settings{Color: true, Theme: bt, Socket: "deck"}, "")
				m.width, m.height = 140, 32
				m.settingsOpen = true
				m.settingsFocus = settingsFocusFields
				m.settingsEdits = config.FileConfig{}
				return m
			},
		},
		{
			// The `?` help overlay: framedDialogScrollable's own frame plus
			// help_style.go's per-entry keycap styling, and (the reason it is
			// here) every body line it does NOT style.
			name: "help overlay",
			build: func(t *testing.T, bt *theme.Theme) Model {
				m := New(nil, config.Settings{Color: true, Theme: bt, Socket: "deck"}, "")
				m.width, m.height = 110, 40
				m.help = true
				return m
			},
		},
		{
			// The `n` create dialog: renderCreateRowSegments composes a
			// focused row over theme.Selection via bgColorToken, a path that
			// does not go through canvasBackground at all.
			name: "create dialog",
			build: func(t *testing.T, bt *theme.Theme) Model {
				m := New(nil, config.Settings{Color: true, Theme: bt, Socket: "deck"}, "")
				m.width, m.height = 110, 40
				m.creating = true
				return m
			},
		},
		{
			// The `i` detail dialog.
			name: "detail dialog",
			build: func(t *testing.T, bt *theme.Theme) Model {
				m := New(nil, config.Settings{Color: true, Theme: bt, Socket: "deck"}, "")
				m.width, m.height = 110, 40
				m.sessions = canvasForegroundSessions()
				m.selected = rowCursor(0)
				m.detail = true
				return m
			},
		},
		{
			// The `t` theme picker, which renders live OVER the real sidebar
			// and preview (theme_picker.go) through canvasWrapText.
			name: "theme picker over the live list",
			build: func(t *testing.T, bt *theme.Theme) Model {
				m := New(nil, config.Settings{Color: true, Theme: bt, Socket: "deck"}, "")
				m.width, m.height = 110, 32
				m.sessions = canvasForegroundSessions()
				m.themePicking = true
				return m
			},
		},
		{
			// SPEC requirement 14's below-minimum frame: a different footer
			// line (belowMinimumNotice) and a truncateToWidth path of its own.
			name: "below deck's supported minimum",
			build: func(t *testing.T, bt *theme.Theme) Model {
				m := New(nil, config.Settings{Color: true, Theme: bt, Socket: "deck"}, "")
				m.width, m.height = 60, 18
				m.sessions = canvasForegroundSessions()
				m.selected = rowCursor(0)
				return m
			},
		},
		{
			// The 3-column collapsed strip (SPEC requirement 15), whose rows
			// go through collapsedStripContentLine -- the one content-line
			// builder with no trailing pad column.
			name: "collapsed sidebar strip",
			build: func(t *testing.T, bt *theme.Theme) Model {
				m := New(nil, config.Settings{Color: true, Theme: bt, Socket: "deck"}, "")
				m.width, m.height = 110, 30
				m.sessions = canvasForegroundSessions()
				m.selected = rowCursor(0)
				m.sidebarWidth = 3
				return m
			},
		},
		{
			// mainView's startup banner (the tmux-unavailable note), another
			// canvasWrapText caller drawn outside both bordered panels.
			name: "startup banner",
			build: func(t *testing.T, bt *theme.Theme) Model {
				m := New(nil, config.Settings{Color: true, Theme: bt, Socket: "deck"}, "tmux 2.9 is older than deck's supported minimum")
				m.width, m.height = 110, 30
				return m
			},
		},
	}
}

// tokensByHex inverts a theme's palette: hex -> every token declaring it,
// sorted, so a failure message can name WHICH token a cell's foreground
// is (or report that it is a colour no token in the theme declares, which
// is SPEC requirement 33's "colour only from theme tokens" broken).
func tokensByHex(t *testing.T, bt *theme.Theme) map[string][]string {
	t.Helper()
	out := make(map[string][]string, len(theme.AllTokens))
	for _, tok := range theme.AllTokens {
		hex, err := bt.Color(tok)
		if err != nil {
			t.Fatalf("theme %q lacks token %q: %v", bt.Name, tok, err)
		}
		out[hex] = append(out[hex], string(tok))
	}
	for hex := range out {
		sort.Strings(out[hex])
	}
	return out
}

// cellGlyph returns a cell's visible content, treating both the empty
// string and a lone space as blank -- a blank cell needs no foreground
// (there is no glyph for one to colour), which is why the assertions below
// only ever look at cells with a real glyph in them.
func cellGlyph(term *vt.Emulator, col, row int) string {
	cell := term.CellAt(col, row)
	if cell == nil {
		return ""
	}
	return strings.TrimSpace(cell.Content)
}

// TestDeckOwnedCellsCarryExplicitForegroundOnEveryBuiltin is this task's
// red-first core: for every built-in theme and every frame state above,
// EVERY cell holding a visible glyph must carry an explicit foreground
// colour, and that colour must be one the theme itself declares.
//
// Against HEAD before the fix this fails on the very first case for every
// built-in -- the socket line's own cells report no foreground at all --
// and it fails on the LIGHT built-ins for exactly the operator's reason.
// It is deliberately not restricted to the two light themes: a missing
// foreground is a bug on a dark theme too (it just happens to be invisible
// there, because most terminals' default foreground is light and most
// terminal users' background is dark), and pinning it for all five is what
// stops the same regression reappearing behind a dark default.
func TestDeckOwnedCellsCarryExplicitForegroundOnEveryBuiltin(t *testing.T) {
	for _, bt := range theme.Builtins() {
		bt := bt
		t.Run(bt.Name, func(t *testing.T) {
			byHex := tokensByHex(t, bt)
			for _, frame := range canvasForegroundFrames() {
				frame := frame
				t.Run(frame.name, func(t *testing.T) {
					m := frame.build(t, bt)
					term := renderSettingsToEmulator(t, m.View(), m.width, m.height)
					for row := 0; row < m.height; row++ {
						for col := 0; col < m.width; col++ {
							glyph := cellGlyph(term, col, row)
							if glyph == "" {
								continue
							}
							hex, ok := cellFgHex(t, term, col, row)
							if !ok {
								t.Fatalf("theme %q %s: cell (%d,%d) shows glyph %q with NO explicit foreground -- it renders in the TERMINAL's default foreground on deck's own painted canvas (GH #24: white text on a light theme's canvas)",
									bt.Name, frame.name, col, row, glyph)
							}
							if _, declared := byHex[hex]; !declared {
								t.Fatalf("theme %q %s: cell (%d,%d) glyph %q has foreground %s, which no token in this theme declares (SPEC requirement 33: colour only from theme tokens)",
									bt.Name, frame.name, col, row, glyph, hex)
							}
						}
					}
				})
			}
		})
	}
}

// TestDeckOwnedCellContrastTable is the measured evidence half: it logs
// every distinct (foreground token, background token) pair the frames
// above actually put a GLYPH on, with its WCAG ratio, and hard-fails on a
// pair whose ratio is below 1.5:1 -- the "effectively invisible"
// threshold, well under internal/theme's own 3:1 chrome floor.
//
// The threshold here is deliberately NOT 3:1, and the `border` token is
// deliberately exempt from it, because two sub-floor pairs are
// pre-existing, deliberate, already-recorded findings that this task
// neither introduces nor is scoped to change --
//
//	`border` on `background`: 1.42:1 (daylight), 1.66:1 (parchment),
//	   1.72:1 (empire), 1.93:1 (cobalt), 2.08:1 (matrix) -- sub-floor in
//	   ALL FIVE built-ins, including the three the operator says "look
//	   really well", and on `selection_idle` it reaches 1.02:1 (empire).
//	   Panel borders are intentionally quiet chrome: §11.6's contrast
//	   tables in internal/theme/contrast_test.go have never listed
//	   `border`, in any theme, dark or light, and that file's own doc
//	   comments state the rule for a sub-floor pair ("a finding to report,
//	   never a licence to recolour a built-in theme file"). Retuning it in
//	   the two LIGHT themes alone would make them the only built-ins with a
//	   loud frame; retuning all five is a deliberate restyle of every
//	   built-in and the operator's call, not this task's. Hence: logged,
//	   reported, exempt from the threshold.
//	`dimmed` on `selection`: 2.51:1 (parchment), 2.59:1 (cobalt) -- the
//	   selected row's line-2 age. contrast_test.go's own
//	   dialogSelectionTokens doc comment already records this exact pair as
//	   a finding ("the sidebar's own pre-existing sub-floor dimmed/selection
//	   marker (tui.go:4292) which sits outside R84's dialog scope entirely").
//	   It clears 1.5:1, so it needs no exemption here.
//
// internal/theme is where deck holds token PAIRS to a floor, and where
// this task added the pair it actually introduces
// (TestCanvasDefaultForegroundClearsFloor: `text` on every background
// canvasBackground can open under a glyph). This test's job is to prove
// there is no NEW invisible pair on screen and to leave the numbers in the
// record; run it with -v to read the table.
func TestDeckOwnedCellContrastTable(t *testing.T) {
	const invisibleRatio = 1.5

	for _, bt := range theme.Builtins() {
		bt := bt
		t.Run(bt.Name, func(t *testing.T) {
			byHex := tokensByHex(t, bt)
			backgroundHex, err := bt.Color(theme.Background)
			if err != nil {
				t.Fatalf("theme %q lacks background: %v", bt.Name, err)
			}
			borderHex, err := bt.Color(theme.Border)
			if err != nil {
				t.Fatalf("theme %q lacks border: %v", bt.Name, err)
			}

			type pair struct{ fg, bg string }
			counts := make(map[pair]int)
			for _, frame := range canvasForegroundFrames() {
				m := frame.build(t, bt)
				term := renderSettingsToEmulator(t, m.View(), m.width, m.height)
				for row := 0; row < m.height; row++ {
					for col := 0; col < m.width; col++ {
						if cellGlyph(term, col, row) == "" {
							continue
						}
						fg, ok := cellFgHex(t, term, col, row)
						if !ok {
							continue // TestDeckOwnedCells... above is the assertion for this
						}
						bg, ok := cellBgHex(t, term, col, row)
						if !ok {
							bg = backgroundHex
						}
						counts[pair{fg, bg}]++
					}
				}
			}

			keys := make([]pair, 0, len(counts))
			for p := range counts {
				keys = append(keys, p)
			}
			sort.Slice(keys, func(i, j int) bool {
				ri, _ := wcagRatio(keys[i].fg, keys[i].bg)
				rj, _ := wcagRatio(keys[j].fg, keys[j].bg)
				return ri < rj
			})
			for _, p := range keys {
				ratio, err := wcagRatio(p.fg, p.bg)
				if err != nil {
					t.Fatalf("theme %q: ratio(%s, %s): %v", bt.Name, p.fg, p.bg, err)
				}
				exempt := ""
				if p.fg == borderHex {
					exempt = "  [border: pre-existing finding, exempt]"
				}
				t.Logf("%-10s %5.2f:1  fg %s (%s) on bg %s (%s) -- %d cells%s",
					bt.Name, ratio, p.fg, strings.Join(byHex[p.fg], "|"), p.bg, strings.Join(byHex[p.bg], "|"), counts[p], exempt)
				if p.fg == borderHex {
					continue
				}
				if ratio < invisibleRatio {
					t.Errorf("theme %q: %d cells render fg %s (%s) on bg %s (%s) at %.2f:1, below the %.1f:1 effectively-invisible threshold",
						bt.Name, counts[p], p.fg, strings.Join(byHex[p.fg], "|"), p.bg, strings.Join(byHex[p.bg], "|"), ratio, invisibleRatio)
				}
			}
		})
	}
}

// wcagRatio is internal/theme's contrastRatio, duplicated here for exactly
// the reason cellFgHex/cellBgHex are duplicated from
// features/cell_attributes_test.go (see their own doc comment): the
// function it mirrors is unexported in another package, and internal/tui
// must not grow a dependency on a test-only surface to read a number it
// only ever logs and threshold-checks.
func wcagRatio(a, b string) (float64, error) {
	la, err := wcagLuminance(a)
	if err != nil {
		return 0, err
	}
	lb, err := wcagLuminance(b)
	if err != nil {
		return 0, err
	}
	lighter, darker := la, lb
	if lb > la {
		lighter, darker = lb, la
	}
	return (lighter + 0.05) / (darker + 0.05), nil
}

func wcagLuminance(hex string) (float64, error) {
	r, g, b, err := theme.HexRGB(hex)
	if err != nil {
		return 0, err
	}
	lin := func(c int) float64 {
		v := float64(c) / 255.0
		if v <= 0.03928 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(r) + 0.7152*lin(g) + 0.0722*lin(b), nil
}

// TestCropMarkerAndGeometryLineCarryExplicitForeground covers the two
// deck-owned runs the frames above cannot reach, because both only exist
// when a LIVE capture is cropped: cropPreviewBottomLeft's geometry line
// ("40x21 of 116x43") and cropRow's crop marker (`»`, R23), 16 of which
// appeared unforegrounded in the parchment measurement -- one per cropped
// row.
//
// The captured rows themselves are the boundary: deck's canvas foreground
// must not simply be stamped over them. Under SPEC §11.3 a capture cell
// whose foreground the agent chose (here an explicit 38;2;10;20;30) keeps
// that colour's HUE, moved in lightness only as far as the 4.5:1 floor
// against deck's canvas requires -- never replaced by `text`, which is what
// this test's own deck-owned columns must carry. So this is simultaneously
// the fix's proof and its boundary.
func TestCropMarkerAndGeometryLineCarryExplicitForeground(t *testing.T) {
	const paneFg = "#0a141e" // 10;20;30, the pane's own foreground below

	for _, bt := range theme.Builtins() {
		bt := bt
		t.Run(bt.Name, func(t *testing.T) {
			m := New(nil, config.Settings{Color: true, Theme: bt, Socket: "deck"}, "")
			m.width, m.height = 110, 32
			m.sessions = []store.Session{{ID: "s1", Name: "live-one", Agent: "codex", Status: "running", CWD: "/home/op/w", CreatedAt: 1000}}
			m.selected = rowCursor(0)
			m.previewLive = true
			m.previewSessionID = "s1"

			layout := m.computeLayout()
			if layout.Effective != LayoutSideBySide {
				t.Fatalf("theme %q: frame computed as %q, want side-by-side", bt.Name, layout.Effective)
			}
			sw, pw := layout.Sidebar.Width, layout.Preview.Width
			contentWidth := pw - 4
			contentHeight := layout.Sidebar.Height - 2

			// A pane WIDER and TALLER than the panel, so cropped is true:
			// cropPreviewBottomLeft prepends its geometry line and cropRow
			// marks every row it had to cut.
			m.previewPaneWidth = contentWidth + 40
			m.previewPaneHeight = contentHeight + 5
			rows := make([]string, m.previewPaneHeight)
			for i := range rows {
				rows[i] = "\x1b[38;2;10;20;30m" + strings.Repeat("x", m.previewPaneWidth) + "\x1b[0m"
			}
			m.previewBytes = []byte(strings.Join(rows, "\n"))

			textHex := tokenHex(t, m, theme.Text)
			backgroundHex := tokenHex(t, m, theme.Background)
			term := renderSettingsToEmulator(t, m.View(), m.width, m.height)

			// Row 1 (the first preview content row) is the geometry line;
			// the interior span runs from sw+2 to sw+1+contentWidth.
			geomRow := 1
			geom := fmt.Sprintf("%dx%d of %dx%d", contentWidth, contentHeight, m.previewPaneWidth, m.previewPaneHeight)
			var got strings.Builder
			for col := sw + 2; col <= sw+1+contentWidth; col++ {
				if cell := term.CellAt(col, geomRow); cell != nil {
					got.WriteString(cell.Content)
				}
			}
			if !strings.HasPrefix(got.String(), geom) {
				t.Fatalf("theme %q: preview row %d = %q, want it to start with the geometry line %q", bt.Name, geomRow, got.String(), geom)
			}
			for i := 0; i < len(geom); i++ {
				col := sw + 2 + i
				hex, ok := cellFgHex(t, term, col, geomRow)
				if !ok {
					t.Fatalf("theme %q: geometry line cell (%d,%d) has NO explicit foreground", bt.Name, col, geomRow)
				}
				if hex != textHex {
					t.Fatalf("theme %q: geometry line cell (%d,%d) foreground = %s, want the canvas's `text` token %s", bt.Name, col, geomRow, hex, textHex)
				}
			}

			// Row 2 onward are cropped captured rows: the last interior
			// column carries deck's own crop marker, every column before it
			// the pane's own bytes.
			markerCol := sw + 1 + contentWidth
			marker := m.cropMarker()
			for row := 2; row < 1+contentHeight; row++ {
				if cell := term.CellAt(markerCol, row); cell == nil || cell.Content != marker {
					content := ""
					if cell != nil {
						content = cell.Content
					}
					t.Fatalf("theme %q: cell (%d,%d) = %q, want the crop marker %q", bt.Name, markerCol, row, content, marker)
				}
				hex, ok := cellFgHex(t, term, markerCol, row)
				if !ok {
					t.Fatalf("theme %q: crop marker cell (%d,%d) has NO explicit foreground -- it renders in the terminal's default foreground on deck's own canvas", bt.Name, markerCol, row)
				}
				if hex != textHex {
					t.Fatalf("theme %q: crop marker cell (%d,%d) foreground = %s, want the canvas's `text` token %s", bt.Name, markerCol, row, hex, textHex)
				}
				if bg, ok := cellBgHex(t, term, markerCol, row); !ok || bg != backgroundHex {
					t.Fatalf("theme %q: crop marker cell (%d,%d) background = %q/%v, want deck's `background` %s", bt.Name, markerCol, row, bg, ok, backgroundHex)
				}

				// The pane's own cells keep the pane's own HUE, fitted
				// against deck's canvas: `text` must stop at deck's own
				// columns and never be stamped over the agent's choice.
				assertPaneCellFitted(t, term, sw+2, row, paneFg, backgroundHex)
			}
		})
	}
}

// TestSidebarGutterCellsNeverFallBackToCanvasForeground is the boundary
// canvasBackground's own doc comment promises: the gutter bar's two
// background tokens (`accent` when selected, `badge` when marked) are the
// only tokens canvasBackground is ever called with on which `text` -- the
// canvas foreground this task added -- is NOT readable (parchment 2.67:1,
// matrix 1.00:1; see internal/theme's canvasForegroundChecks, which
// deliberately excludes both). That is safe only because every glyph the
// bar draws carries an explicit `background` foreground of its own, so no
// bar cell ever falls back to the canvas default. This pins that: if a
// later change drops sidebarGutterBar's colorToken(theme.Background, ...)
// and lets the bar's glyphs inherit the canvas foreground instead, the
// `>` and `✓` cues become near-invisible on the very themes GH #24 is
// about, and this fails.
func TestSidebarGutterCellsNeverFallBackToCanvasForeground(t *testing.T) {
	for _, bt := range theme.Builtins() {
		bt := bt
		t.Run(bt.Name, func(t *testing.T) {
			m := New(nil, config.Settings{Color: true, Theme: bt, Socket: "deck"}, "")
			m.width, m.height = 110, 32
			m.sessions = canvasForegroundSessions()
			m.selected = rowCursor(0)
			m.marked = map[string]bool{"s2": true}
			if got := m.computeLayout().Effective; got != LayoutSideBySide {
				t.Fatalf("theme %q: frame computed as %q, want side-by-side", bt.Name, got)
			}
			backgroundHex := tokenHex(t, m, theme.Background)
			accentHex := tokenHex(t, m, theme.Accent)
			badgeHex := tokenHex(t, m, theme.Badge)

			term := renderSettingsToEmulator(t, m.View(), m.width, m.height)

			// The selected row's `>` and the marked row's mark glyph are the
			// two glyph-bearing gutter cells; find each by its own content
			// on the row the emulator put it on, then assert the pair.
			for _, want := range []struct {
				glyph  string
				barHex string
				label  string
			}{
				{">", accentHex, "selected row's arrow"},
				{m.glyph("✓", "*"), badgeHex, "marked row's mark cue"},
			} {
				found := false
				for row := 1; row < m.height && !found; row++ {
					for col := 1; col < 6; col++ {
						cell := term.CellAt(col, row)
						if cell == nil || cell.Content != want.glyph {
							continue
						}
						bg, ok := cellBgHex(t, term, col, row)
						if !ok || bg != want.barHex {
							continue // a same-glyph cell elsewhere, not the bar
						}
						found = true
						fg, ok := cellFgHex(t, term, col, row)
						if !ok {
							t.Fatalf("theme %q: %s at (%d,%d) has no foreground at all", bt.Name, want.label, col, row)
						}
						if fg != backgroundHex {
							t.Fatalf("theme %q: %s at (%d,%d) foreground = %s, want the `background` token %s (never the canvas's `text` default, which is unreadable on the bar)",
								bt.Name, want.label, col, row, fg, backgroundHex)
						}
						break
					}
				}
				if !found {
					t.Fatalf("theme %q: no gutter cell showing %q on its bar background %s was rendered at all", bt.Name, want.glyph, want.barHex)
				}
			}
		})
	}
}

// TestCanvasBackgroundAddsNoForegroundWithoutColour is the NO_COLOR /
// DECK_COLOR=0 half: with colour disabled, canvasBackground must hand its
// parts back byte-for-byte -- no background escape, and (this task's new
// obligation) no foreground escape either.
func TestCanvasBackgroundAddsNoForegroundWithoutColour(t *testing.T) {
	m := New(nil, config.Settings{Color: false, Theme: theme.Default()}, "")
	const plain = "socket: deck"
	if got := m.canvasBackground(theme.Background, plain); got != plain {
		t.Fatalf("canvasBackground with colour disabled = %q, want %q byte-for-byte", got, plain)
	}
	view := m.View()
	if strings.Contains(view, "\x1b[38;2;") {
		t.Fatalf("colour-disabled View() emitted a truecolour foreground escape:\n%q", view)
	}
}

// TestCanvasBackgroundOpensBothCanvasAttributesAndReopensAfterResets is
// the unit-level shape of the fix, independent of any rendered frame: the
// composed string opens the background AND the `text` foreground, and
// re-opens BOTH after an inner self-resetting run -- because an SGR reset
// clears every attribute, so re-opening only the background would leave
// everything after the first coloured glyph back on the terminal's default
// foreground, which is precisely the bug.
func TestCanvasBackgroundOpensBothCanvasAttributesAndReopensAfterResets(t *testing.T) {
	bt, ok := theme.Builtin("parchment")
	if !ok {
		t.Fatal("built-in theme parchment is not embedded")
	}
	m := New(nil, config.Settings{Color: true, Theme: bt}, "")

	bgSeq, ok := m.backgroundSGR(theme.Background)
	if !ok {
		t.Fatal("backgroundSGR(background) not available on a colour-enabled model")
	}
	fgSeq, ok := m.foregroundSGR(theme.Text)
	if !ok {
		t.Fatal("foregroundSGR(text) not available on a colour-enabled model")
	}

	got := m.canvasBackground(theme.Background, m.colorToken(theme.Title, "titled"), " plain tail")
	wantOpen := bgSeq + fgSeq
	if !strings.HasPrefix(got, wantOpen) {
		t.Fatalf("canvasBackground = %q, want it to open with background+text %q", got, wantOpen)
	}
	// colorToken's own trailing reset is the inner reset; the canvas pair
	// must be re-opened immediately after it, before " plain tail".
	if !strings.Contains(got, "\x1b[0m"+wantOpen+" plain tail") {
		t.Fatalf("canvasBackground = %q, want the canvas pair %q re-opened right after the inner reset and before the plain tail", got, wantOpen)
	}
	if !strings.HasSuffix(got, "\x1b[0m") {
		t.Fatalf("canvasBackground = %q, want exactly one closing reset", got)
	}
}
