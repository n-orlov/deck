// haslivepane_test.go pins the distinction issue #6 turned on: under deck's
// server-wide `remain-on-exit failed`, `has-session` (Exists) keeps
// succeeding for a session whose only pane is a corpse, so a caller that
// wants to know whether there is anything left to adopt must ask
// HasLivePane instead.
package tmux

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"
)

func hasLivePaneSocket(name string) string {
	return fmt.Sprintf("deck-haslivepane-%s-%d-%d", name, os.Getpid(), time.Now().UnixNano())
}

// waitForRetainedDeadPane polls List until slug's session reports a dead
// pane with the given exit status, which is what `remain-on-exit failed`
// leaves behind and is not instantaneous after the process exits.
func waitForRetainedDeadPane(t *testing.T, client Client, slug string, status int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		sessions, err := client.List(context.Background())
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		for _, session := range sessions {
			if session.Name != "deck_"+slug {
				continue
			}
			for _, pane := range session.Panes {
				if pane.Dead && pane.DeadStatus != nil && *pane.DeadStatus == status {
					return
				}
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("session deck_%s never retained a dead pane with status %d", slug, status)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestHasLivePaneSeparatesALiveSessionFromARetainedCorpse is the tmux-layer
// half of issue #6, leg 2. The load-bearing assertion is the PAIR taken on
// the same corpse: Exists reports true (tmux's `has-session` cannot tell a
// dead pane from a live one) while HasLivePane reports false. A test that
// only checked HasLivePane on a live session and on an absent one would pass
// against Exists itself and prove nothing.
func TestHasLivePaneSeparatesALiveSessionFromARetainedCorpse(t *testing.T) {
	socket := hasLivePaneSocket("corpse")
	t.Cleanup(func() { _ = exec.Command("tmux", "-L", socket, "kill-server").Run() })
	client := Client{Socket: socket, Timeout: 10 * time.Second}
	ctx := context.Background()

	// An absent private server is a normal "no live pane", not an error --
	// the same answer Exists gives, and asked before anything bootstraps it.
	if live, err := client.HasLivePane(ctx, "never-created"); err != nil || live {
		t.Fatalf("HasLivePane against a server that was never bootstrapped = %v, %v; want false, nil", live, err)
	}

	cwd := t.TempDir()
	if _, err := client.Create(ctx, Launch{Slug: "alive", CWD: cwd, Command: []string{"/bin/sh", "-c", "sleep 300"}}); err != nil {
		t.Fatalf("create live session: %v", err)
	}
	if live, err := client.HasLivePane(ctx, "alive"); err != nil || !live {
		t.Fatalf("HasLivePane on a session with a running pane = %v, %v; want true, nil", live, err)
	}

	if _, err := client.Create(ctx, Launch{Slug: "corpse", CWD: cwd, Command: []string{"/bin/sh", "-c", "exit 7"}}); err != nil {
		t.Fatalf("create crashing session: %v", err)
	}
	waitForRetainedDeadPane(t, client, "corpse", 7)

	exists, err := client.Exists(ctx, "corpse")
	if err != nil {
		t.Fatalf("Exists on a retained corpse: %v", err)
	}
	if !exists {
		t.Fatal("Exists reported false for a retained dead pane: the fixture is not discriminating, since HasLivePane returning false would then prove nothing new")
	}
	live, err := client.HasLivePane(ctx, "corpse")
	if err != nil {
		t.Fatalf("HasLivePane on a retained corpse: %v", err)
	}
	if live {
		t.Fatal("HasLivePane reported true for a session whose only pane is dead: a corpse is not a pane to adopt (#6)")
	}

	// Killing the corpse leaves the same false, still without an error.
	if err := client.Kill(ctx, "corpse"); err != nil {
		t.Fatalf("kill corpse: %v", err)
	}
	if live, err := client.HasLivePane(ctx, "corpse"); err != nil || live {
		t.Fatalf("HasLivePane after the corpse was collected = %v, %v; want false, nil", live, err)
	}
	// ...and the untouched neighbour is still live: the predicate is
	// per-session, never a server-wide answer.
	if live, err := client.HasLivePane(ctx, "alive"); err != nil || !live {
		t.Fatalf("HasLivePane on the untouched live session = %v, %v; want true, nil", live, err)
	}
}
