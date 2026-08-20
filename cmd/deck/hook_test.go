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
	"github.com/n-orlov/deck/internal/tmux"
)

func TestReleasedHookBoundsStalledTmuxAndSkipsItForSessionEnd(t *testing.T) {
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
	for _, fixture := range []struct{ id, conversation string }{
		{id: "ending", conversation: "conversation-ending"},
		{id: "notifying", conversation: "conversation-notifying"},
		{id: "sentinel"},
	} {
		status, source := "starting", "user"
		if fixture.id == "sentinel" {
			status, source = "running", "hook"
		}
		if _, err := db.CreateSession(context.Background(), store.CreateSessionInput{
			ID: fixture.id, Name: fixture.id, CWD: home, Agent: "claude", CapturedPath: "/bin",
			Status: status, StatusSource: source, StatusAt: 1000, CreatedAt: 1000,
			ConversationID: fixture.conversation,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	fakeBin := t.TempDir()
	marker := filepath.Join(home, "tmux-invoked")
	fakeTmux := filepath.Join(fakeBin, "tmux")
	if err := os.WriteFile(fakeTmux, []byte("#!/bin/sh\nprintf invoked > \"$DECK_TEST_TMUX_MARKER\"\nexec /bin/sleep 10\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	run := func(payload string) (time.Duration, string, error) {
		t.Helper()
		cmd := exec.Command(binary, "_hook")
		cmd.Stdin = strings.NewReader(payload)
		cmd.Env = append(os.Environ(),
			"DECK_HOME="+home,
			"DECK_TMUX_SOCKET=deck-hook-stalled",
			"DECK_RECONCILE_MS=30",
			"DECK_TEST_TMUX_MARKER="+marker,
			"PATH="+fakeBin,
		)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		started := time.Now()
		err := cmd.Run()
		return time.Since(started), stderr.String(), err
	}

	elapsed, stderr, err := run(`{"hook_event_name":"SessionEnd","session_id":"conversation-ending","reason":"logout"}`)
	if err != nil {
		t.Fatalf("released SessionEnd failed: %v, stderr=%q", err, stderr)
	}
	if elapsed >= time.Second {
		t.Fatalf("SessionEnd took %s despite its no-liveness contract", elapsed)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("SessionEnd invoked tmux: %v", err)
	}
	db, err = store.Open(paths)
	if err != nil {
		t.Fatal(err)
	}
	sentinel, err := db.GetSession(context.Background(), "sentinel")
	if err != nil {
		t.Fatal(err)
	}
	if sentinel.Status != "running" || sentinel.StatusSource != "hook" {
		t.Fatalf("SessionEnd reconciled unrelated sentinel: %#v", sentinel)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	elapsed, stderr, err = run(`{"hook_event_name":"Notification","session_id":"conversation-notifying","notification_type":"permission_prompt"}`)
	if err == nil || !strings.Contains(stderr, "post-hook liveness pass") {
		t.Fatalf("stalled released hook err=%v stderr=%q", err, stderr)
	}
	if elapsed >= time.Second {
		t.Fatalf("stalled tmux held released hook for %s, want < 1s", elapsed)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("nonterminal hook did not invoke stalled tmux fixture: %v", err)
	}
}

func TestReleasedDeckHookIsOneShotAndDoesNotBootstrapStateOrTmux(t *testing.T) {
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
	target, err := db.CreateSession(context.Background(), store.CreateSessionInput{
		ID: "row-1", Name: "hook target", CWD: t.TempDir(), Agent: "claude", CapturedPath: "/bin",
		Status: "running", StatusSource: "hook", StatusAt: 1000, CreatedAt: 1000,
		ConversationID: "conversation-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	crashed, err := db.CreateSession(context.Background(), store.CreateSessionInput{
		ID: "row-2", Name: "unattended crash", CWD: t.TempDir(), Agent: "claude", CapturedPath: "/bin",
		Status: "running", StatusSource: "hook", StatusAt: 1000, CreatedAt: 1001,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	// No TUI is running. A live pane for the hook target and a retained dead
	// pane for a different row make the released _hook binary the only possible
	// observer and collector of the crash.
	hookSocket := "deck-hook-live-" + filepath.Base(t.TempDir())
	tmuxClient := tmux.Client{Socket: hookSocket}
	t.Cleanup(func() { _ = exec.Command("tmux", "-L", hookSocket, "kill-server").Run() })
	if _, err := tmuxClient.Create(context.Background(), tmux.Launch{Slug: target.Slug, CWD: target.CWD, Command: []string{"/bin/sh", "-c", "sleep 30"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := tmuxClient.Create(context.Background(), tmux.Launch{Slug: crashed.Slug, CWD: crashed.CWD, Command: []string{"/bin/sh", "-c", "printf 'unattended-tail\\n'; exit 23"}}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		live, err := tmuxClient.List(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		dead := false
		for _, session := range live {
			if session.Name == "deck_"+crashed.Slug && len(session.Panes) == 1 && session.Panes[0].Dead {
				dead = true
			}
		}
		if dead {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("crash fixture did not leave a retained dead pane")
		}
		time.Sleep(5 * time.Millisecond)
	}

	run := func(input string, extraEnv ...string) (string, error) {
		t.Helper()
		cmd := exec.Command(binary, "_hook")
		cmd.Stdin = strings.NewReader(input)
		base := []string{"DECK_HOME=" + home, "DECK_TMUX_SOCKET=" + hookSocket, "DECK_RECONCILE_MS=1000"}
		cmd.Env = append(os.Environ(), append(base, extraEnv...)...)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		err := cmd.Run()
		return stderr.String(), err
	}

	payload := `{"hook_event_name":"Notification","session_id":"conversation-1","notification_type":"permission_prompt"}`
	if stderr, err := run(payload); err != nil {
		t.Fatalf("released _hook failed: %v, stderr=%q", err, stderr)
	}
	db, err = store.Open(paths)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := db.ListSessions(context.Background())
	if err != nil || len(rows) != 2 {
		t.Fatalf("sessions after hook = %#v, %v", rows, err)
	}
	var hooked, collected store.Session
	for _, row := range rows {
		switch row.ID {
		case target.ID:
			hooked = row
		case crashed.ID:
			collected = row
		}
	}
	if hooked.Status != "waiting" || hooked.StatusSource != "hook" || hooked.StatusReason != "permission_prompt" {
		t.Fatalf("hook status = %#v", hooked)
	}
	if collected.Status != "error" || collected.PaneExitStatus == nil || *collected.PaneExitStatus != 23 || !strings.Contains(collected.CrashTail, "unattended-tail") {
		t.Fatalf("unattended crash verdict = %#v", collected)
	}
	live, err := tmuxClient.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, session := range live {
		if session.Name == "deck_"+crashed.Slug {
			t.Fatalf("unattended crash was not collected: %#v", session)
		}
	}
	var eventCount int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM events`).Scan(&eventCount); err != nil {
		t.Fatal(err)
	}
	if eventCount != 2 {
		t.Fatalf("event count after hook and liveness pass = %d, want 2", eventCount)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	log, err := os.ReadFile(filepath.Join(home, "log", "deck.jsonl"))
	if err != nil || bytes.Count(log, []byte(`"event":"hook.store_write"`)) != 1 {
		t.Fatalf("hook store audit = %q, %v", log, err)
	}
	writeAt, crashAt := bytes.Index(log, []byte(`"event":"hook.store_write"`)), bytes.Index(log, []byte(`"event":"tmux.pane_dead"`))
	if writeAt < 0 || crashAt <= writeAt || bytes.Contains(log, []byte("probe")) {
		t.Fatalf("post-hook liveness audit order/probe isolation = %q", log)
	}

	for name, input := range map[string]string{
		"malformed": `{"hook_event_name":`,
		"array":     `[]`,
		"extra":     payload + ` {"hook_event_name":"Stop"}`,
	} {
		t.Run(name, func(t *testing.T) {
			if stderr, err := run(input); err == nil || stderr == "" {
				t.Fatalf("invalid hook err=%v stderr=%q", err, stderr)
			}
		})
	}
	db, err = store.Open(paths)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM events`).Scan(&eventCount); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if eventCount != 2 {
		t.Fatalf("invalid input performed a write; event count = %d", eventCount)
	}

	if stderr, err := run(`{"hook_event_name":"Stop"}`, "DECK_SESSION_ID=stale-row"); err == nil || !strings.Contains(stderr, "could not be resolved") {
		t.Fatalf("stale hook err=%v stderr=%q", err, stderr)
	}

	missingHome := filepath.Join(t.TempDir(), "must-not-exist")
	missingSocket := "deck-hook-missing-" + filepath.Base(t.TempDir())
	cmd := exec.Command(binary, "_hook")
	cmd.Stdin = strings.NewReader(payload)
	cmd.Env = append(os.Environ(), "DECK_HOME="+missingHome, "DECK_TMUX_SOCKET="+missingSocket)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err == nil || !strings.Contains(stderr.String(), "state database does not exist") {
		t.Fatalf("missing database hook err=%v stderr=%q", err, stderr.String())
	}
	if _, err := os.Stat(missingHome); !os.IsNotExist(err) {
		t.Fatalf("missing hook created DECK_HOME: %v", err)
	}
	if err := exec.Command("tmux", "-L", missingSocket, "has-session").Run(); err == nil {
		t.Fatal("missing hook bootstrapped a tmux server/session")
	}
}
