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

// R416 (F2): the post-hook liveness pass is bounded by DECK_RECONCILE_MS. When
// tmux's `list-panes` hangs past that bound, the pass is skipped (the next
// reconcile or hook repairs it); the released hook still exits with the status
// its own event handling earned, inside event_hook_timeout plus the reconcile
// budget, and leaves exactly one line naming the call that ran out. The stub
// prints a canary the way a tmux command prints pane content before it is
// killed; the line must never carry it.
func TestReleasedHookExitsZeroWhenTheLivenessPassRunsOutOfItsBoundOnListPanes(t *testing.T) {
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
	if _, err := db.CreateSession(context.Background(), store.CreateSessionInput{
		ID: "notifying", Name: "notifying", CWD: home, Agent: "claude", CapturedPath: "/bin",
		Status: "running", StatusSource: "hook", StatusAt: 1000, CreatedAt: 1000,
		ConversationID: "conversation-notifying",
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	const canary = "PANE-TEXT-CANARY"
	fakeBin := t.TempDir()
	marker := filepath.Join(home, "tmux-list-panes")
	script := "#!/bin/sh\n" +
		"case \" $* \" in *\" list-panes \"*) ;; *) exit 0;; esac\n" +
		"printf invoked > \"$DECK_TEST_TMUX_MARKER\"\n" +
		"printf '" + canary + "\\n'\n" +
		"exec /bin/sleep 10\n"
	if err := os.WriteFile(filepath.Join(fakeBin, "tmux"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}

	const reconcileBudget = 30 * time.Millisecond
	cmd := exec.Command(binary, "_hook")
	cmd.Stdin = strings.NewReader(`{"hook_event_name":"Notification","session_id":"conversation-notifying","notification_type":"permission_prompt"}`)
	cmd.Env = append(os.Environ(),
		"DECK_HOME="+home,
		"DECK_TMUX_SOCKET=priv-hook-list-panes",
		"DECK_RECONCILE_MS=30",
		"DECK_TEST_TMUX_MARKER="+marker,
		"PATH="+fakeBin,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	started := time.Now()
	err = cmd.Run()
	elapsed := time.Since(started)

	if err != nil {
		t.Fatalf("hook whose liveness pass ran out of its bound exited non-zero: %v, stderr=%q", err, stderr.String())
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("hook never reached the stalled list-panes call: %v", err)
	}
	// The documented total bound: one event_hook_timeout plus the reconcile
	// budget. The 10 s stub sleep would blow it; the pass's own cut-off keeps
	// the hook far inside it.
	if bound := config.DefaultEventHookTimeout + reconcileBudget; elapsed >= bound {
		t.Fatalf("hook took %s, want < %s (event_hook_timeout + reconcile budget)", elapsed, bound)
	}
	lines := strings.Split(strings.TrimSpace(stderr.String()), "\n")
	if len(lines) != 1 || !strings.Contains(lines[0], "list-panes") || !strings.Contains(lines[0], "ran out") {
		t.Fatalf("want exactly one line naming the list-panes call that ran out, got %q", stderr.String())
	}
	if strings.Contains(stderr.String(), canary) {
		t.Fatalf("log line carries tmux output: %q", stderr.String())
	}

	// The hook's own event handling is durable: the skipped pass is the only
	// thing that did not happen.
	db, err = store.Open(paths)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	row, err := db.GetSession(context.Background(), "notifying")
	if err != nil {
		t.Fatal(err)
	}
	if row.Status != "waiting" || row.StatusSource != "hook" {
		t.Fatalf("notification not applied: status=%q source=%q", row.Status, row.StatusSource)
	}
}
