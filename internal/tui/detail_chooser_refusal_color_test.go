package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/theme"
)

// TestDetailChooserRefusalUsesErrorToken is cure-01-01-2's cell-color
// regression proof: detailBody used to append m.attachError via a plain
// fmt.Fprintf, so the refusal rendered in whatever foreground the
// surrounding text carries (theme.Text, detailField's own default) rather
// than theme.Error -- the same token every other in-dialog validation note
// (profileSwitchNote, pinNote, renameNote, moveGroupNote, envNote,
// launchInputsNote, ...) already renders in via each dialog's own
// colorWhole, and the one SPEC §11.4 names explicitly for validation
// messages. This reads the actual per-cell foreground through a real
// vt.Emulator (renderSettingsToEmulator/cellFgHex, settings_color_cues_
// test.go), the same way every other §11.6 token-cue regression in this
// package does, rather than grepping for a raw escape sequence.
func TestDetailChooserRefusalUsesErrorToken(t *testing.T) {
	for _, tc := range []struct {
		key  string
		want string
	}{
		{"P", "Cannot change"},
		{"c", "Cannot change"},
	} {
		t.Run(tc.key, func(t *testing.T) {
			noop := func(context.Context, string, string) (store.Session, error) {
				return store.Session{}, nil
			}
			m := NewWithShellCreatorAttacherKillerResumerProfileSwitcherAndResumeModer(
				nil, config.Settings{Color: true}, "", nil, nil, nil, nil, nil, noop, noop,
			)
			m.width, m.height = 80, 24
			m.sessions = []store.Session{{ID: "s", Name: "shell", Agent: "shell", CWD: "/tmp", Status: "running"}}
			m.selected = rowCursor(0)

			got, _ := m.Update(key("i"))
			m = got.(Model)
			got, _ = m.Update(key(tc.key))
			m = got.(Model)
			if !m.detail {
				t.Fatalf("%s on an ineligible session closed the detail dialog", tc.key)
			}
			if m.attachError == "" {
				t.Fatalf("%s set no attachError, this test proves nothing", tc.key)
			}

			term := renderSettingsToEmulator(t, m.View(), m.width, m.height)
			row := findRowContainingInSidebar(t, term, m.width, tc.want)
			col := findCol(t, term, row, tc.want)
			got2, ok := cellFgHex(t, term, col, row)
			want := tokenHex(t, m, theme.Error)
			if !ok || got2 != want {
				t.Fatalf("refusal foreground=(%q,%v), want error token %q", got2, ok, want)
			}
		})
	}
}

// TestDetailChooserRefusalReadableUnderNoColor is the NO_COLOR fallback
// this same fix must not break: colorToken (theme_color.go) already
// returns text unmodified whenever m.settings.Color is false, so the
// refusal must still render as its own plain text -- never an unresolved
// escape sequence, and never dropped -- once colour is disabled.
func TestDetailChooserRefusalReadableUnderNoColor(t *testing.T) {
	noop := func(context.Context, string, string) (store.Session, error) {
		return store.Session{}, nil
	}
	m := NewWithShellCreatorAttacherKillerResumerProfileSwitcherAndResumeModer(
		nil, config.Settings{Color: false}, "", nil, nil, nil, nil, nil, noop, noop,
	)
	m.width, m.height = 80, 24
	m.sessions = []store.Session{{ID: "s", Name: "shell", Agent: "shell", CWD: "/tmp", Status: "running"}}
	m.selected = rowCursor(0)

	got, _ := m.Update(key("i"))
	m = got.(Model)
	got, _ = m.Update(key("P"))
	m = got.(Model)

	frame := m.View()
	if frame != stripANSI(frame) {
		t.Fatalf("NO_COLOR view still carries escape bytes:\n%q", frame)
	}
	if !strings.Contains(stripANSI(frame), "Cannot change permission profile") {
		t.Fatalf("refusal not readable under NO_COLOR:\n%s", frame)
	}
}
