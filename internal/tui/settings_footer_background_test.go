package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/vt"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/theme"
)

// This file is cure-03-01's red-first proof for review pass 234's B1
// finding on the settings takeover's own footer: settingsFooterLine
// (settings.go) used to return its composed text directly, never routed
// through canvasBackground the way mainView's own footerLine (tui.go,
// task 004/R118) already was, so the footer row leaked the terminal's own
// background in every settings mode instead of carrying deck's own
// `background` token -- exactly the review probe's
// TestReviewSettingsFooterCanvas found (settings footer cell (0,29)
// bg="" present=false). Fixed by wrapping settingsFooterLineContent's
// existing composition in a canvasBackground(theme.Background, ...) call,
// mirroring footerLine/footerLineContent's own split.
//
// Six modes are checked, one per settingsFooterLine branch: the plain
// takeover ("normal"), the discard-confirm prompt, `/` search, the [env]
// list, the [env] key/value editor and the free-text string editor --
// every branch settingsFooterLine can return, across every built-in
// theme, through a real Model.View() render fed into a vt.Emulator
// (renderSettingsToEmulator, settings_color_cues_test.go) exactly as the
// review probe did, so painting only the six settings view builders'
// callers (rather than settingsFooterLine itself) could not satisfy this
// test by accident. This was RED before the cure (bg missing on every
// cell) and is GREEN after it.
func TestSettingsFooterCarriesDeckBackground(t *testing.T) {
	for _, bt := range theme.Builtins() {
		bt := bt
		t.Run(bt.Name, func(t *testing.T) {
			for _, mode := range []string{"normal", "discard", "search", "env-list", "env-edit", "string-edit"} {
				t.Run(mode, func(t *testing.T) {
					m := New(nil, config.Settings{Color: true, Theme: bt}, "")
					m.width, m.height, m.settingsOpen = 100, 30, true
					switch mode {
					case "discard":
						m.settingsDiscardConfirm = true
					case "search":
						m.settingsSearchActive = true
					case "env-list":
						m.settingsEnvOpen = true
					case "env-edit":
						m.settingsEnvOpen, m.settingsEnvEditing = true, true
					case "string-edit":
						m.settingsStringEditing = true
					}

					wantText := m.settingsFooterLineContent()

					view := m.View()
					term := renderSettingsToEmulator(t, view, m.width, m.height)
					want := tokenHex(t, m, theme.Background)
					footerRow := m.height - 1

					// The footer's own literal text must survive the paint
					// wrapping unchanged -- read straight off the
					// emulator's own cell content, the same idiom
					// findRowContaining (settings_color_cues_test.go)
					// already uses, rather than re-deriving it from the
					// escape bytes.
					gotText := rowText(term, footerRow, stringWidth(wantText))
					if gotText != wantText {
						t.Fatalf("theme %q mode %q: footer row text = %q, want %q (geometry must be preserved)", bt.Name, mode, gotText, wantText)
					}

					// Every cell across the footer's own text must carry
					// deck's background token -- not merely column 0, the
					// single cell the review probe happened to check.
					for col := 0; col < stringWidth(wantText); col++ {
						got, ok := cellBgHex(t, term, col, footerRow)
						if !ok || got != want {
							t.Fatalf("theme %q mode %q: settings footer cell (%d,%d) bg=%q present=%v want=%q; footer=%q", bt.Name, mode, col, footerRow, got, ok, want, wantText)
						}
					}
				})
			}
		})
	}
}

// TestSettingsFooterUnderNoColorCarriesNoEscapes proves NO_COLOR (Color:
// false) leaves the settings footer's own text byte-for-byte unchanged --
// canvasBackground's own "tok==” leaves parts untouched" branch -- rather
// than emitting an SGR sequence nobody asked for.
func TestSettingsFooterUnderNoColorCarriesNoEscapes(t *testing.T) {
	m := New(nil, config.Settings{Color: false}, "")
	m.width, m.height, m.settingsOpen = 100, 30, true
	content := m.settingsFooterLineContent()
	painted := m.settingsFooterLine()
	if painted != content {
		t.Fatalf("settings footer under Color:false = %q, want unpainted content %q unchanged", painted, content)
	}
}

// rowText reads width cells of visible text off row, starting at column 0,
// mirroring findRowContaining's cell-content concatenation
// (settings_color_cues_test.go).
func rowText(term *vt.Emulator, row, width int) string {
	var b strings.Builder
	for col := 0; col < width; col++ {
		if cell := term.CellAt(col, row); cell != nil {
			b.WriteString(cell.Content)
		}
	}
	return b.String()
}
