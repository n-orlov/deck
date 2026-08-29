package tmux

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// TestCreateRacesConcurrentReconcilerKill is task 501's red-before repro of
// the REAL trigger for crash.feature:46: internal/service/reconcile.go runs
// concurrently with any in-flight Create call, and the moment its own List
// observes a dead pane it calls Kill on that same session (reconcile.go
// around line 183) -- entirely independent of tmux's own remain-on-exit
// handling, which only decides whether the PANE is retained, never whether
// deck's own reconciler leaves the SESSION alone. Before the fix, Create ran
// new-session and then a separate, later loop of set-environment calls; if
// the reconciler's Kill lands in the gap between new-session returning and
// that loop finishing, set-environment fails with "no such session" and
// Create aborts the whole create. This test plays the reconciler's exact
// List-then-Kill-on-Dead shape in a tight polling goroutine racing a real
// Create call whose command exits instantly, without any artificial CPU
// load -- the two real deck actors are enough on their own.
func TestCreateRacesConcurrentReconcilerKill(t *testing.T) {
	socket := fmt.Sprintf("deck-reconciler-race-%d-%d", os.Getpid(), time.Now().UnixNano())
	client := Client{Socket: socket, Timeout: 3 * time.Second}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = client.command(ctx, "kill-server").Run()
	})

	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		ctx := context.Background()
		for {
			select {
			case <-stop:
				return
			default:
			}
			sessions, err := client.List(ctx)
			if err != nil {
				continue
			}
			for _, session := range sessions {
				for _, pane := range session.Panes {
					if pane.Dead {
						// Mirrors reconcile.go's own Kill(ctx, session.Slug)
						// once it has observed the dead pane -- session.Name
						// is "deck_<slug>", so strip the prefix exactly as
						// reconcile.go's own session.Slug field already
						// holds it.
						slug := strings.TrimPrefix(session.Name, "deck_")
						_ = client.Kill(ctx, slug)
					}
				}
			}
		}
	}()

	_, err := client.Create(context.Background(), Launch{
		Slug:    "reconciler_race",
		CWD:     t.TempDir(),
		Command: []string{"sh", "-c", "exit 1"},
		Env:     map[string]string{"DECK_RACE_A": "one", "DECK_RACE_B": "two"},
	})
	close(stop)
	<-done

	if err != nil {
		t.Fatalf("create session racing a concurrent reconciler kill: %v", err)
	}
}
