package service

import (
	"context"
	"github.com/n-orlov/deck/internal/agent"
	"strings"
	"testing"
)

// screenRecorder is a fake pane that shows a fixed screen and records every
// key plan sent to it.
type screenRecorder struct {
	screen string
	plans  [][]string
}

func (r *screenRecorder) Screen(context.Context, string) (string, error) { return r.screen, nil }

func (r *screenRecorder) SendPlan(_ context.Context, _ string, plan []string) error {
	r.plans = append(r.plans, plan)
	return nil
}

const claudeRule = "────────────────────────────────────────────────────────────\n"

// claudeReadyScreen is Claude idle at an empty composer: what a fake pane shows
// in the tests that push a rename and are not about the gate.
const claudeReadyScreen = "● done\n\n" + claudeRule + "❯\n" + claudeRule + "  ? for shortcuts\n"

// TestClaudePushGate (R267): the real Claude adapter's rename push is sent only
// into an empty, ready composer with no dialog focused, whatever the stored
// status says. A refused push reaches the pane with no key at all and leaves
// one harness_rename_refused event.
func TestClaudePushGate(t *testing.T) {
	permission := "● Bash(rm -rf build)\n\n Bash command\n   rm -rf build\n\n Do you want to proceed?\n ❯ 1. Yes\n   2. No\n\n Esc to cancel\n"
	for _, tc := range []struct {
		name     string
		status   string
		screen   string
		wantSent bool
	}{
		{name: "permission dialog up, stored status running", status: "running", screen: permission},
		{name: "permission dialog up, stored status idle", status: "idle", screen: permission},
		{name: "composer holding typed, unsent text", status: "idle", screen: "● done\n\n" + claudeRule + "❯ half a prompt I was typing\n" + claudeRule + "  ? for shortcuts\n"},
		{name: "empty ready composer", status: "idle", screen: "❯ earlier prompt\n\n● done\n\n" + claudeRule + "❯\n" + claudeRule + "  ? for shortcuts\n", wantSent: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rig := newPushRig(t, true)
			pane := &screenRecorder{screen: tc.screen}
			rig.svc.NamePane = pane
			// The real Claude adapter, not the scripted fake the rig registers.
			rig.svc.Agents.Register(agent.NewClaude())
			row := rig.row(t, "row-a", tc.status)

			_, result, err := rig.svc.RenameAndPush(context.Background(), row.ID, "new name")
			if err != nil {
				t.Fatal(err)
			}
			if tc.wantSent {
				if len(pane.plans) != 1 || !result.Sent {
					t.Fatalf("plans = %q, sent = %v; want the push sent once", pane.plans, result.Sent)
				}
				if got := rig.eventCount(t, row.ID, EventHarnessRenameRefused); got != 0 {
					t.Fatalf("refused events = %d, want 0", got)
				}
				return
			}
			if len(pane.plans) != 0 || result.Sent {
				t.Fatalf("keys reached the pane: %q (sent = %v)", pane.plans, result.Sent)
			}
			if got := rig.eventCount(t, row.ID, EventHarnessRenameRefused); got != 1 {
				t.Fatalf("refused events = %d, want 1 (note %q)", got, result.Note)
			}
			if !strings.Contains(result.Note, "not pushed") {
				t.Fatalf("note %q does not say the rename was not pushed", result.Note)
			}
		})
	}
}
