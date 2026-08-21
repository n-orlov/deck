package tui

import (
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/theme"
)

// detailColorTestModel builds a colour-enabled Model with the `i` detail
// dialog already open on a session with enough fields set (agent, cwd,
// status reason, verdict source/age, permission profile, conversation id)
// that every detailField call site in detailView actually renders (task
// 022, SPEC requirement 34).
func detailColorTestModel(t *testing.T) Model {
	t.Helper()
	m := New(nil, config.Settings{Color: true}, "")
	m.width, m.height = 120, 30
	m.sessions = []store.Session{{
		ID:                "s1",
		Name:              "alpha",
		Agent:             "claude",
		CWD:               "/repo/alpha",
		Status:            "waiting",
		StatusReason:      "needs input",
		StatusSource:      "hook",
		StatusAt:          1,
		PermissionProfile: "safe",
		ConversationID:    "conv-123",
	}}
	m.selected = 0
	m.detail = true
	return m
}

// TestDetailFieldLabelsAreHintValuesAreText proves SPEC requirement 34:
// every `label: value` pair in the `i` detail dialog renders its label in
// the `hint` token and its value in `text`, distinctly, read per cell off
// a real vt.Emulator grid rather than merely inspecting the source.
func TestDetailFieldLabelsAreHintValuesAreText(t *testing.T) {
	m := detailColorTestModel(t)
	hintHex := tokenHex(t, m, theme.Hint)
	textHex := tokenHex(t, m, theme.Text)
	if hintHex == textHex {
		t.Skip("this theme's hint and text tokens happen to share a colour; the distinctness assertion below would be vacuous")
	}

	view := m.View()
	term := renderSettingsToEmulator(t, view, m.width, m.height)

	row := findRowContaining(t, term, "Agent:")
	labelCol := findCol(t, term, row, "Agent:")
	labelFg, ok := cellFgHex(t, term, labelCol, row)
	if !ok {
		t.Fatalf("detail label %q has no foreground colour", "Agent:")
	}
	if labelFg != hintHex {
		t.Fatalf("detail label foreground = %s, want hint token %s", labelFg, hintHex)
	}

	valueCol := findCol(t, term, row, "claude")
	valueFg, ok := cellFgHex(t, term, valueCol, row)
	if !ok {
		t.Fatalf("detail value %q has no foreground colour", "claude")
	}
	if valueFg != textHex {
		t.Fatalf("detail value foreground = %s, want text token %s", valueFg, textHex)
	}

	// A second label:value pair (Conversation id) confirms the pattern
	// holds across the dialog, not just its first line.
	convRow := findRowContaining(t, term, "Conversation id:")
	convLabelCol := findCol(t, term, convRow, "Conversation id:")
	convLabelFg, ok := cellFgHex(t, term, convLabelCol, convRow)
	if !ok {
		t.Fatalf("detail label %q has no foreground colour", "Conversation id:")
	}
	if convLabelFg != hintHex {
		t.Fatalf("detail label foreground = %s, want hint token %s", convLabelFg, hintHex)
	}
	convValueCol := findCol(t, term, convRow, "conv-123")
	convValueFg, ok := cellFgHex(t, term, convValueCol, convRow)
	if !ok {
		t.Fatalf("detail value %q has no foreground colour", "conv-123")
	}
	if convValueFg != textHex {
		t.Fatalf("detail value foreground = %s, want text token %s", convValueFg, textHex)
	}
}
