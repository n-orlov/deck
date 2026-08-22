package tui

import (
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
)

// TestCreateFieldRowsEveryFieldHasANonEmptyLabelAndHelp covers task 016
// (SPEC requirement 7): the create modal's field set (createFieldRows) is
// meant to be self-describing entirely through the field's own on-screen
// label and help text, so this enumerates every row rather than asserting
// a fixed count, and fails loudly (naming the offending row index) if any
// future field addition to that table forgets to give itself a
// description. pre_launch, login_shell and launch_args get an additional,
// stronger assertion below: their help text must state *what the field
// does*, not just be non-empty.
func TestCreateFieldRowsEveryFieldHasANonEmptyLabelAndHelp(t *testing.T) {
	model := New(nil, config.Settings{Socket: "test-socket"}, "")
	rows := model.createFieldRows()
	if len(rows) != createFieldCount {
		t.Fatalf("createFieldRows() returned %d rows, want createFieldCount=%d", len(rows), createFieldCount)
	}
	for i, row := range rows {
		if strings.TrimSpace(row.label) == "" {
			t.Errorf("createFieldRows()[%d] has an empty label", i)
		}
		if strings.TrimSpace(row.help) == "" {
			t.Errorf("createFieldRows()[%d] (%q) has an empty help/description", i, row.label)
		}
	}
}

// TestCreateFieldRowsPreLaunchLoginShellLaunchArgsDescribeWhatTheyDo pins the
// three fields task 016 names explicitly: their help text must say what
// the field actually does (not just exist), phrased so a user reading only
// the dialog -- never SPEC.md -- can tell.
func TestCreateFieldRowsPreLaunchLoginShellLaunchArgsDescribeWhatTheyDo(t *testing.T) {
	model := New(nil, config.Settings{Socket: "test-socket"}, "")
	rows := model.createFieldRows()
	want := map[string][]string{
		"Launch args (JSON array)": {
			"extra arguments", "appended", "argv",
		},
		"Pre-launch command": {
			"command run in the pane", "before the agent starts",
		},
		"Login shell": {
			"$SHELL -lc", "instead of the agent argv",
		},
	}
	byLabel := make(map[string]string, len(rows))
	for _, row := range rows {
		byLabel[row.label] = row.help
	}
	for label, phrases := range want {
		help, ok := byLabel[label]
		if !ok {
			t.Errorf("createFieldRows() has no field labelled %q", label)
			continue
		}
		for _, phrase := range phrases {
			if !strings.Contains(help, phrase) {
				t.Errorf("field %q help = %q, missing expected phrase %q describing what it does", label, help, phrase)
			}
		}
	}
}
