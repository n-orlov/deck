package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

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
	_, err = db.CreateSession(context.Background(), store.CreateSessionInput{
		ID: "row-1", Name: "hook target", CWD: t.TempDir(), Agent: "claude", CapturedPath: "/bin",
		Status: "starting", StatusSource: "user", StatusAt: 1000, CreatedAt: 1000,
		ConversationID: "conversation-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	run := func(input string, extraEnv ...string) (string, error) {
		t.Helper()
		cmd := exec.Command(binary, "_hook")
		cmd.Stdin = strings.NewReader(input)
		cmd.Env = append(os.Environ(), append([]string{"DECK_HOME=" + home}, extraEnv...)...)
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
	if err != nil || len(rows) != 1 {
		t.Fatalf("sessions after hook = %#v, %v", rows, err)
	}
	if rows[0].Status != "waiting" || rows[0].StatusSource != "hook" || rows[0].StatusReason != "permission_prompt" {
		t.Fatalf("hook status = %#v", rows[0])
	}
	var eventCount int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM events`).Scan(&eventCount); err != nil {
		t.Fatal(err)
	}
	if eventCount != 1 {
		t.Fatalf("event count after one hook = %d, want 1", eventCount)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	log, err := os.ReadFile(filepath.Join(home, "log", "deck.jsonl"))
	if err != nil || bytes.Count(log, []byte(`"event":"hook.store_write"`)) != 1 {
		t.Fatalf("hook store audit = %q, %v", log, err)
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
	if eventCount != 1 {
		t.Fatalf("invalid input performed a write; event count = %d", eventCount)
	}

	if stderr, err := run(`{"hook_event_name":"Stop"}`, "DECK_SESSION_ID=stale-row"); err == nil || !strings.Contains(stderr, "could not be resolved") {
		t.Fatalf("stale hook err=%v stderr=%q", err, stderr)
	}

	missingHome := filepath.Join(t.TempDir(), "must-not-exist")
	socket := "deck-hook-missing-" + filepath.Base(t.TempDir())
	cmd := exec.Command(binary, "_hook")
	cmd.Stdin = strings.NewReader(payload)
	cmd.Env = append(os.Environ(), "DECK_HOME="+missingHome, "DECK_TMUX_SOCKET="+socket)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err == nil || !strings.Contains(stderr.String(), "state database does not exist") {
		t.Fatalf("missing database hook err=%v stderr=%q", err, stderr.String())
	}
	if _, err := os.Stat(missingHome); !os.IsNotExist(err) {
		t.Fatalf("missing hook created DECK_HOME: %v", err)
	}
	if err := exec.Command("tmux", "-L", socket, "has-session").Run(); err == nil {
		t.Fatal("missing hook bootstrapped a tmux server/session")
	}
}
