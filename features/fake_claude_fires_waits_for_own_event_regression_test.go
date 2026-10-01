package features

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/cucumber/godog"
	"github.com/n-orlov/deck/internal/store"
)

// TestFakeClaudeFiresWaitsForTheHooksOwnEvent (task 026) pins the sync point
// of the `fake Claude session "E" fires "Event" ...` steps: they may report
// the hook persisted only once the hook's OWN event row exists for the
// target, never because some other event for the target appeared meanwhile.
// status_recovery.feature:88 failed solo ("has 0 session_start events, want
// one") because a late reconcile/kill event for the just-killed target --
// UpdateSessionStatus records an event even for a write it declines --
// satisfied the old "event count went up" wait before the hook landed.
//
// The emitter's pane here is a plain `cat`, so no hook ever fires; an
// unrelated "tmux.session_gone" event is written for the target while the
// step waits. The step must therefore fail (its own deadline), and must
// succeed once a real "session_start" event is written instead.
func TestFakeClaudeFiresWaitsForTheHooksOwnEvent(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not on PATH")
	}
	home := t.TempDir()
	socket := fmt.Sprintf("deck_test_fires_own_event_%d", os.Getpid())
	h := &ScenarioHarness{Home: home, Socket: socket}
	ctx := context.WithValue(context.Background(), scenarioHarnessKey{}, h)
	defer func() { _ = exec.Command("tmux", "-L", socket, "kill-server").Run() }()

	db, err := store.OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, s := range []struct{ name, conversation string }{{"emitter", "conv-emitter"}, {"target", "conv-target"}} {
		if _, err := db.CreateSession(ctx, store.CreateSessionInput{
			ID: s.name + "-id", Name: s.name, CWD: home, Agent: "claude", CapturedPath: "/bin",
			Status: "running", StatusSource: "hook", StatusAt: 10, CreatedAt: 1, ConversationID: s.conversation,
		}); err != nil {
			t.Fatal(err)
		}
	}
	slug, err := sessionSlugByName(h, "emitter")
	if err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("tmux", "-L", socket, "new-session", "-d", "-s", "deck_"+slug, "cat >/dev/null").CombinedOutput(); err != nil {
		t.Fatalf("start emitter pane: %v: %s", err, out)
	}

	writeEventSoon := func(kind string) {
		go func() {
			time.Sleep(300 * time.Millisecond)
			if _, err := db.DB().Exec(`INSERT INTO events (session_id, at, kind, reason, payload) VALUES (?, ?, ?, '', '')`, "target-id", 50, kind); err != nil {
				t.Error(err)
			}
		}()
	}

	writeEventSoon("tmux.session_gone")
	if err := fakeClaudeFires(ctx, "emitter", "SessionStart", "target", "conversation", &godog.Table{}); err == nil {
		t.Fatal(`fakeClaudeFires reported the SessionStart hook persisted although only an unrelated "tmux.session_gone" event was written for the target`)
	}

	writeEventSoon("session_start")
	if err := fakeClaudeFires(ctx, "emitter", "SessionStart", "target", "conversation", &godog.Table{}); err != nil {
		t.Fatalf("fakeClaudeFires did not see the hook's own session_start event: %v", err)
	}
}
