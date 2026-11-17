package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// R416 (F4): the post-hook liveness pass is bounded by DECK_RECONCILE_MS on
// its store calls as well as its tmux calls. A death the pass must record
// whose status write cannot get the state database's write lock inside that
// bound (another writer holds it) skips the pass: the released hook still
// exits 0, inside event_hook_timeout plus the reconcile budget, and leaves
// exactly one line naming the store call that ran out. The lock is taken
// deterministically: the stub tmux reports "invoked" when the pass reaches its
// list-panes call (the hook's own write is already durable by then), the test
// takes an immediate transaction on a second connection, and only then lets
// list-panes answer with an empty server, so the pass finds its death and
// meets the held lock. The lock is released after the hook returns.
func TestReleasedHookExitsZeroWhenTheLivenessPassCannotGetTheWriteLockInTime(t *testing.T) {
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "deck")
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build released deck: %v: %s", err, output)
	}

	home := t.TempDir()
	paths := config.Paths{Home: home, DataDir: home, ConfigFile: filepath.Join(home, "config.toml"), LogDir: filepath.Join(home, "log"), StateDB: filepath.Join(home, "state.db")}
	db, err := store.Open(paths)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, id := range []string{"notifying", "died"} {
		if _, err := db.CreateSession(context.Background(), store.CreateSessionInput{
			ID: id, Name: id, CWD: home, Agent: "claude", CapturedPath: "/bin",
			Status: "running", StatusSource: "hook", StatusAt: 1000, CreatedAt: 1000,
			ConversationID: "conversation-" + id,
		}); err != nil {
			t.Fatal(err)
		}
	}

	fakeBin := t.TempDir()
	invoked := filepath.Join(home, "tmux-list-panes")
	proceed := filepath.Join(home, "tmux-proceed")
	script := "#!/bin/sh\n" +
		"case \" $* \" in *\" list-panes \"*) ;; *) exit 0;; esac\n" +
		"printf invoked > \"$DECK_TEST_TMUX_INVOKED\"\n" +
		"while [ ! -e \"$DECK_TEST_TMUX_PROCEED\" ]; do /bin/sleep 0.01; done\n" +
		"exit 0\n"
	if err := os.WriteFile(filepath.Join(fakeBin, "tmux"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}

	// Generous enough that the pass reaches its status write before the
	// bound runs out, however loaded the machine is; the write then waits on
	// the lock for what is left of it.
	const reconcileBudget = 1500 * time.Millisecond
	cmd := exec.Command(binary, "_hook")
	cmd.Stdin = strings.NewReader(`{"hook_event_name":"Notification","session_id":"conversation-notifying","notification_type":"permission_prompt"}`)
	cmd.Env = append(os.Environ(),
		"DECK_HOME="+home,
		"DECK_TMUX_SOCKET=priv-hook-store-lock",
		"DECK_RECONCILE_MS=1500",
		"DECK_TEST_TMUX_INVOKED="+invoked,
		"DECK_TEST_TMUX_PROCEED="+proceed,
		"PATH="+fakeBin,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	started := time.Now()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	for deadline := time.Now().Add(30 * time.Second); ; {
		if _, err := os.Stat(invoked); err == nil {
			break
		}
		if time.Now().After(deadline) {
			_ = cmd.Process.Kill()
			t.Fatalf("hook never reached its liveness pass's list-panes call; stderr=%q", stderr.String())
		}
		time.Sleep(5 * time.Millisecond)
	}
	lock, err := db.DB().BeginTx(context.Background(), nil)
	if err != nil {
		_ = cmd.Process.Kill()
		t.Fatalf("take the write lock: %v", err)
	}
	released := false
	release := func() {
		if !released {
			released = true
			_ = lock.Rollback()
		}
	}
	defer release()
	if err := os.WriteFile(proceed, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	runErr := <-done
	elapsed := time.Since(started)
	release()

	if runErr != nil {
		t.Fatalf("hook whose liveness pass could not get the write lock in time exited non-zero: %v, stderr=%q", runErr, stderr.String())
	}
	if bound := config.DefaultEventHookTimeout + reconcileBudget; elapsed >= bound {
		t.Fatalf("hook took %s, want < %s (event_hook_timeout + reconcile budget); stderr=%q", elapsed, bound, stderr.String())
	}
	lines := strings.Split(strings.TrimSpace(stderr.String()), "\n")
	if len(lines) != 1 || !strings.Contains(lines[0], `mark session "`) || !strings.Contains(lines[0], "begin session status update") || !strings.Contains(lines[0], "ran out") {
		t.Fatalf("want exactly one line naming the status-update store call that ran out, got %q", stderr.String())
	}

	// The hook's own event handling is durable and the pass wrote nothing:
	// the lock held the death's status write off until the bound ran out.
	for id, want := range map[string]string{"notifying": "waiting", "died": "running"} {
		row, err := db.GetSession(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		if row.Status != want {
			t.Fatalf("session %q status = %q, want %q", id, row.Status, want)
		}
	}
}
