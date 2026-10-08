package tui

import (
	"reflect"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// TestZIsUnboundAndNeverAdvertised covers R233c: snooze is retired, so `z`
// is bound to nothing (on a session row and on a header: no model change, no
// command), is not a session-scoped key, and neither the footer nor the help
// view mentions it or snooze.
func TestZIsUnboundAndNeverAdvertised(t *testing.T) {
	if sessionScopedKeys["z"] {
		t.Fatal(`"z" is still in sessionScopedKeys`)
	}
	rowModel := New(nil, config.Settings{}, "")
	rowModel.sessions = []store.Session{{ID: "s1", Name: "alpha", Agent: "shell", Status: "running"}}
	rowModel.selected = rowCursor(0)
	for name, m := range map[string]Model{"session row": rowModel, "header": sessionScopedKeyGuardFixture()} {
		before := modelSnapshotForEquality(m)
		updated, cmd := m.Update(key("z"))
		if cmd != nil {
			t.Errorf("%s: pressing z returned a command", name)
		}
		out, ok := updated.(Model)
		if !ok {
			t.Fatalf("%s: Update returned %T", name, updated)
		}
		if got := modelSnapshotForEquality(out); !reflect.DeepEqual(got, before) {
			t.Errorf("%s: pressing z changed the model: %s", name, strings.Join(differingModelFields(before, got), "; "))
		}
	}

	footer := strings.ToLower(rowModel.footerKeyLegend())
	if strings.Contains(footer, "snooze") || strings.Contains(footer, "z ") {
		t.Errorf("footer mentions z or snooze: %q", footer)
	}
	rowModel.help = true
	help := rowModel.helpView()
	if strings.Contains(strings.ToLower(help), "snooze") {
		t.Errorf("help mentions snooze:\n%s", help)
	}
	for _, line := range strings.Split(help, "\n") {
		if f := strings.Fields(line); len(f) > 0 && f[0] == "z" {
			t.Errorf("help lists a z binding: %q", line)
		}
	}
}
