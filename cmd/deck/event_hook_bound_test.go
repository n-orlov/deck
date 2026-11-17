package main

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/agent"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/racebuild"
	"github.com/n-orlov/deck/internal/store"
)

// agentHookBound is the limit the agent's installed hook entry gives `deck
// _hook` (the Copilot plugin declares it; the other agents' hooks are no
// looser). It is the existing overall latency bound this file holds _hook to.
const agentHookBound = time.Duration(agent.CopilotHookTimeoutSec) * time.Second

const waitingPromptPayload = `{"hook_event_name":"Notification","session_id":"conv-1","notification_type":"permission_prompt"}`

// eventResults maps every event row to the hook result stored against it.
func eventResults(t *testing.T, paths config.Paths) (events []store.Event, results map[int64]store.EventHookResult) {
	t.Helper()
	db, err := store.Open(paths)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if events, err = db.ListEvents(context.Background(), 200); err != nil {
		t.Fatal(err)
	}
	results = map[int64]store.EventHookResult{}
	for _, event := range events {
		res, ok, err := db.EventHookResultOf(context.Background(), event.Seq)
		if err != nil {
			t.Fatal(err)
		}
		if ok {
			results[event.Seq] = res
		}
	}
	return events, results
}

// SPEC §3.1/§10.4: at the production event_hook_timeout, an incoming event
// whose script hangs AND five reconcile-detected deaths whose scripts hang
// hold `deck _hook` to one timeout plus the reconcile budget, inside the
// agent's own 5 s hook bound — never the incoming timeout then a second full
// death-batch timeout. Every event is durable and every offer has its result.
func TestHookWithHungIncomingAndDeathScriptsStaysInsideTheAgentBoundAtDefaultTimeout(t *testing.T) {
	f := newEventHookFixture(t, "claude", "running")
	f.settings.EventHookTimeout = config.DefaultEventHookTimeout
	f.script(t, fmt.Sprintf("printf '%%s %%s\\n' \"$1\" \"$DECK_SESSION_ID\" >> %q\nsleep 30\n", filepath.Join(f.out, "calls")))
	f.seedExtraRows(t, 4)
	if err := exec.Command("tmux", "-L", f.settings.Socket, "kill-server").Run(); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if code, stderr := f.run(waitingPromptPayload); code != 0 || stderr != "" {
		t.Fatalf("hook exit = %d, stderr %q", code, stderr)
	}
	elapsed := time.Since(start)
	if limit := f.settings.EventHookTimeout + f.settings.Reconcile + time.Second; elapsed > limit || elapsed >= agentHookBound {
		t.Fatalf("_hook took %s with a hung incoming script and five hung death scripts at the default %s timeout, want <= %s and inside the agent's %s",
			elapsed, f.settings.EventHookTimeout, limit, agentHookBound)
	}
	if elapsed < f.settings.EventHookTimeout {
		t.Fatalf("_hook took %s, less than the %s timeout: the hung scripts were not waited for at all", elapsed, f.settings.EventHookTimeout)
	}
	calls := strings.Fields(f.read(t, "calls"))
	if len(calls) != 12 || calls[0] != "waiting" || calls[1] != "row-1" {
		t.Fatalf("event hook calls = %q, want the hook's own waiting first then five ended offers", calls)
	}
	events, results := eventResults(t, f.paths)
	var waiting, gone int
	for _, event := range events {
		switch event.Kind {
		case "notification":
			waiting++
		case "tmux.session_gone":
			gone++
		default:
			continue
		}
		if res, ok := results[event.Seq]; !ok || !res.TimedOut {
			t.Errorf("event %d (%s on %s) result = %+v ok=%v, want a recorded timeout", event.Seq, event.Kind, event.SessionID, res, ok)
		}
	}
	if waiting != 1 || gone != 5 {
		t.Fatalf("durable events: %d notification, %d tmux.session_gone, want 1 and 5", waiting, gone)
	}
}

// The same rule for a script that fails fast instead of hanging, and for a
// slow incoming script with no death at all: the incoming offer alone is one
// timeout, the death batch rides along with it.
func TestHookBoundHoldsForFailingDeathScriptsAndAHungIncomingScriptAlone(t *testing.T) {
	t.Run("hung incoming only", func(t *testing.T) {
		f := newEventHookFixture(t, "claude", "running")
		f.settings.EventHookTimeout = config.DefaultEventHookTimeout
		f.script(t, "sleep 30\n")
		start := time.Now()
		if code, stderr := f.run(waitingPromptPayload); code != 0 || stderr != "" {
			t.Fatalf("hook exit = %d, stderr %q", code, stderr)
		}
		if elapsed := time.Since(start); elapsed > f.settings.EventHookTimeout+f.settings.Reconcile+time.Second {
			t.Fatalf("_hook took %s with one hung script", elapsed)
		}
	})
	t.Run("failing incoming and failing deaths", func(t *testing.T) {
		f := newEventHookFixture(t, "claude", "running")
		f.settings.EventHookTimeout = config.DefaultEventHookTimeout
		// The liveness pass's budget is a hard deadline on the pass's own work
		// (list rows, list tmux, one status write per death). Running out of it
		// skips the rest of the pass and still exits the hook 0 with one line
		// on stderr (SPEC §3.1, R416; TestReleasedHookExitsZeroWhenTheLiveness-
		// Pass...), but this subtest asserts the hook's silence and that every
		// death was recorded and offered, so the pass must finish. Under -race
		// the in-process SQLite (modernc, pure Go) is itself race-instrumented:
		// on a loaded agent the pass's median ~30 ms reaches several hundred ms,
		// and nightly #49 crossed the 500 ms production default here (a death's
		// "begin session status update: context deadline exceeded"). A deadline
		// costs nothing when the pass is quick, so a race build gets a wider one.
		if racebuild.Enabled {
			f.settings.Reconcile = 2 * time.Second
		}
		// Each run records when it started and ended, so the overlap is read
		// from the scripts themselves rather than inferred from wall time.
		f.script(t, fmt.Sprintf(`s=$(date +%%s%%N)
sleep 2
printf '%%s %%s %%s %%s\n' "$DECK_EVENT_KIND" "$DECK_SESSION_ID" "$s" "$(date +%%s%%N)" >> %q
echo boom >&2
exit 7
`, filepath.Join(f.out, "spans")))
		f.seedExtraRows(t, 3)
		if err := exec.Command("tmux", "-L", f.settings.Socket, "kill-server").Run(); err != nil {
			t.Fatal(err)
		}
		if code, stderr := f.run(waitingPromptPayload); code != 0 || stderr != "" {
			t.Fatalf("hook exit = %d, stderr %q", code, stderr)
		}
		// Serial would be the incoming 2 s script and then the death batch's;
		// overlapped, every death script starts while the incoming one still
		// runs. Asserted on the scripts' own clocks, so it holds however slow
		// the agent is around them.
		spans := scriptSpans(t, f.read(t, "spans"))
		incoming, ok := spans["waiting row-1"]
		if !ok || len(spans) != 5 {
			t.Fatalf("scripts recorded %v, want the incoming waiting offer and four ended offers", spans)
		}
		for key, span := range spans {
			if key != "waiting row-1" && span[0] >= incoming[1] {
				t.Errorf("%s started %s after the incoming script ended: the death batch followed the incoming script instead of overlapping it",
					key, time.Duration(span[0]-incoming[1]))
			}
		}
		events, results := eventResults(t, f.paths)
		var offered int
		for _, event := range events {
			if event.Kind != "notification" && event.Kind != "tmux.session_gone" {
				continue
			}
			offered++
			if res, ok := results[event.Seq]; !ok || res.ExitCode != 7 || res.TimedOut {
				t.Errorf("event %d (%s) result = %+v ok=%v, want exit 7", event.Seq, event.Kind, res, ok)
			}
		}
		if offered != 5 {
			t.Fatalf("%d offered events, want the incoming one and four deaths", offered)
		}
	})
}

// The Copilot plugin's installed hook limit is 5 s: at the default timeout a
// copilot _hook that finds one more row gone stays inside it. The incoming
// offer is claimed first and its script started first.
func TestCopilotHookWithADeathStaysInsideItsInstalledBoundAtDefaultTimeout(t *testing.T) {
	f := newEventHookFixture(t, "copilot", "running")
	t.Setenv(agent.CopilotHookEventEnv, "agentStop")
	f.settings.EventHookTimeout = config.DefaultEventHookTimeout
	f.script(t, fmt.Sprintf("printf '%%s %%s\\n' \"$1\" \"$DECK_SESSION_ID\" >> %q\nsleep 30\n", filepath.Join(f.out, "calls")))
	f.seedExtraRows(t, 1)
	start := time.Now()
	if code, stderr := f.run(`{"sessionId":"conv-1","stopReason":"end_turn"}`); code != 0 || stderr != "" {
		t.Fatalf("hook exit = %d, stderr %q", code, stderr)
	}
	if elapsed := time.Since(start); elapsed >= agentHookBound {
		t.Fatalf("copilot _hook took %s at the default %s timeout, past the installed %s bound", elapsed, f.settings.EventHookTimeout, agentHookBound)
	}
	calls := strings.Split(strings.TrimSpace(f.read(t, "calls")), "\n")
	if len(calls) != 2 || calls[0] != "idle row-1" || calls[1] != "ended row-2" {
		t.Fatalf("event hook calls = %q, want idle row-1 then ended row-2", calls)
	}
}
