package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// R301: an update leaves live agent sessions undisturbed, in both directions.
// These tests drive real binaries: a TUI under a pty over two live fake-agent
// sessions (tmux panes holding `sleep`), and the hook command each session's
// agent would run, with a configured event hook that records which deck
// version ran the hook.

const (
	liveOldVersion = "v1.0.0"
	liveNewVersion = "v1.1.0"
)

var liveSessions = []struct{ id, conversation, name string }{
	{"live-a", "conv-a", "alpha agent"},
	{"live-b", "conv-b", "bravo agent"},
}

// liveFixture is one state home with two live sessions, an event hook that
// records the sending deck's version, and a TUI over them.
type liveFixture struct {
	home, socket, events string
	paths                config.Paths
}

func buildLiveDeck(t *testing.T, name, version string, schema int) string {
	t.Helper()
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), name)
	args := []string{"build"}
	ldflags := "-X main.version=" + version
	if schema != 0 {
		args = append(args, "-tags", "deckoldschema")
		ldflags += fmt.Sprintf(" -X github.com/n-orlov/deck/internal/store.oldSchemaVersion=%d", schema)
	}
	args = append(args, "-ldflags", ldflags, "-o", binary, ".")
	build := exec.Command("go", args...)
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build %s: %v: %s", name, err, output)
	}
	return binary
}

func newLiveFixture(t *testing.T) *liveFixture {
	t.Helper()
	home := t.TempDir()
	f := &liveFixture{
		home:   home,
		socket: "priv-live-" + strings.ReplaceAll(filepath.Base(home), "_", ""),
		events: t.TempDir(),
		paths:  config.Paths{Home: home, DataDir: home, ConfigFile: filepath.Join(home, "config.toml"), LogDir: filepath.Join(home, "log"), StateDB: filepath.Join(home, "state.db")},
	}
	t.Cleanup(func() { _ = exec.Command("tmux", "-L", f.socket, "kill-server").Run() })

	// The event hook writes its stdin payload (which names the sending deck's
	// version) to one file per run.
	script := filepath.Join(f.events, "hook.sh")
	body := fmt.Sprintf("#!/bin/sh\nPATH=/usr/bin:/bin\ncat > \"%s/ev.$(date +%%s%%N)\"\n", f.events)
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := fmt.Sprintf("event_hook = %q\nevent_hook_default = true\nevent_hook_events = [\"waiting\"]\n", script)
	if err := os.WriteFile(f.paths.ConfigFile, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}

	db, err := store.Open(f.paths)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	client := tmux.Client{Socket: f.socket}
	for _, s := range liveSessions {
		row, err := db.CreateSession(context.Background(), store.CreateSessionInput{
			ID: s.id, Name: s.name, CWD: home, Agent: "claude", CapturedPath: "/bin",
			Status: "running", StatusSource: "hook", StatusAt: 1000, CreatedAt: 1000, ConversationID: s.conversation,
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := client.Create(context.Background(), tmux.Launch{Slug: row.Slug, CWD: home, Command: []string{"/bin/sh", "-c", "sleep 600"}}); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

func (f *liveFixture) env() []string {
	return append(os.Environ(), "DECK_HOME="+f.home, "DECK_TMUX_SOCKET="+f.socket, "DECK_RECONCILE_MS=200")
}

// startTUI runs the binary as a TUI under a pty and waits for both sessions to
// render. The returned func reports whether it is still running.
func (f *liveFixture) startTUI(t *testing.T, binary string) (stillRunning func() bool) {
	t.Helper()
	cmd := exec.Command(binary)
	cmd.Env = append(f.env(), "NO_COLOR=1", "DECK_ASCII=1", "DECK_ANIM=0", "TERM=xterm-256color", "SHELL=/bin/sh")
	terminal, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 30, Cols: 120})
	if err != nil {
		t.Fatalf("start TUI under pty: %v", err)
	}
	output := newPTYOutput()
	go pumpTerminal(output, terminal)
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		<-done
		_ = terminal.Close()
	})
	waitForScreen(t, output, done, "\x1b[6n")
	if _, err := terminal.Write([]byte("\x1b]11;rgb:0000/0000/0000\x1b\\\x1b[1;1R")); err != nil {
		t.Fatal(err)
	}
	for _, s := range liveSessions {
		waitForScreenWithin(t, output, done, s.name, 10*time.Second)
	}
	exited := false
	return func() bool {
		select {
		case <-done:
			exited = true
		default:
		}
		return !exited
	}
}

// hook runs the hook command line an agent would run (`sh -c`) for the
// session with a permission request for tool, and fails the test on a non-zero exit.
func (f *liveFixture) hook(t *testing.T, command, conversation, tool string) {
	t.Helper()
	payload := fmt.Sprintf(`{"hook_event_name":"PermissionRequest","session_id":%q,"tool_name":%q}`, conversation, tool)
	cmd := exec.Command("sh", "-c", command)
	cmd.Stdin = strings.NewReader(payload)
	cmd.Env = f.env()
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("hook for %s (%s): %v: %s", conversation, tool, err, out)
	}
}

// requestPermission fires one hook per session. Each run names a different
// tool, so the event hook (one spawn per kind and reason) is offered every
// time, and every run leaves the sessions waiting.
func (f *liveFixture) requestPermission(t *testing.T, command, tool string) {
	t.Helper()
	for _, s := range liveSessions {
		f.hook(t, command, s.conversation, tool)
	}
}

func (f *liveFixture) statuses(t *testing.T) map[string]string {
	t.Helper()
	db, err := sql.Open("sqlite", f.paths.StateDB)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.Query(`SELECT id, status FROM sessions`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var id, status string
		if err := rows.Scan(&id, &status); err != nil {
			t.Fatal(err)
		}
		out[id] = status
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// writerBinary reads the meta row without opening the store, which would
// record the test binary as the writer.
func (f *liveFixture) writerBinary(t *testing.T) string {
	t.Helper()
	db, err := sql.Open("sqlite", f.paths.StateDB)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var path string
	if err := db.QueryRow(`SELECT CAST(version AS TEXT) FROM meta WHERE key = 'writer_binary'`).Scan(&path); err != nil {
		t.Fatal(err)
	}
	return path
}

func (f *liveFixture) assertIntegrity(t *testing.T) {
	t.Helper()
	db, err := sql.Open("sqlite", f.paths.StateDB)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var result string
	if err := db.QueryRow(`PRAGMA integrity_check`).Scan(&result); err != nil || result != "ok" {
		t.Fatalf("PRAGMA integrity_check = %q, %v; want ok", result, err)
	}
}

// versions returns, per session id, the deck versions the event hook was run
// by, in the order the runs happened.
func (f *liveFixture) versions(t *testing.T) map[string][]string {
	t.Helper()
	entries, err := filepath.Glob(filepath.Join(f.events, "ev.*"))
	if err != nil {
		t.Fatal(err)
	}
	type run struct {
		session, version, name string
	}
	var runs []run
	for _, path := range entries {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var payload struct {
			Deck    struct{ Version string }
			Session struct{ ID string }
		}
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatalf("event payload %s: %v: %s", path, err, raw)
		}
		runs = append(runs, run{payload.Session.ID, payload.Deck.Version, filepath.Base(path)})
	}
	// File names carry the nanosecond clock, so name order is run order.
	sort.Slice(runs, func(i, j int) bool { return runs[i].name < runs[j].name })
	out := map[string][]string{}
	for _, r := range runs {
		out[r.session] = append(out[r.session], r.version)
	}
	return out
}

func installLive(t *testing.T, src, dst string) {
	t.Helper()
	copyFile(t, src, dst)
}

// replaceBinary is the atomic step of an update: the new file is written
// beside the installed one and renamed over it.
func replaceBinary(t *testing.T, src, dst string) {
	t.Helper()
	staged := dst + ".new"
	copyFile(t, src, staged)
	if err := os.Rename(staged, dst); err != nil {
		t.Fatal(err)
	}
}

func shellQuote(path string) string { return "'" + strings.ReplaceAll(path, "'", `'"'"'`) + "' _hook" }

// R301, old TUI and new hooks: the installed binary is renamed over while an
// old-version TUI runs over two live sessions whose agents keep the hook
// command naming that path. The next hook run is the new version, the sessions
// hold the statuses they held before, and the database is consistent.
func TestUpdateOldTUINewHooks(t *testing.T) {
	oldBin := buildLiveDeck(t, "deck-old", liveOldVersion, 0)
	newBin := buildLiveDeck(t, "deck-new", liveNewVersion, 0)
	installed := filepath.Join(t.TempDir(), "bin", "deck")
	installLive(t, oldBin, installed)
	command := shellQuote(installed)

	f := newLiveFixture(t)
	running := f.startTUI(t, installed)

	// Before the update: the old hooks leave both sessions waiting.
	f.requestPermission(t, command, "before")
	before := f.statuses(t)
	for _, s := range liveSessions {
		if before[s.id] != "waiting" {
			t.Fatalf("pre-update status of %s = %q, want waiting (all %v)", s.id, before[s.id], before)
		}
		if got := f.versions(t)[s.id]; len(got) != 1 || got[0] != liveOldVersion {
			t.Fatalf("pre-update event hook versions for %s = %v, want [%s]", s.id, got, liveOldVersion)
		}
	}

	replaceBinary(t, newBin, installed)
	if !running() {
		t.Fatal("the old TUI exited when the binary was replaced")
	}

	// After: the very next hook run is the new version.
	f.requestPermission(t, command, "after")
	for _, s := range liveSessions {
		got := f.versions(t)[s.id]
		if len(got) != 2 || got[1] != liveNewVersion {
			t.Errorf("event hook versions for %s = %v, want the next run to report %s", s.id, got, liveNewVersion)
		}
	}
	after := f.statuses(t)
	for id, want := range before {
		if after[id] != want {
			t.Errorf("status of %s = %q after the update, want its pre-update status %q", id, after[id], want)
		}
	}
	if len(after) != len(before) {
		t.Errorf("sessions after the update = %v, before = %v", after, before)
	}
	f.assertIntegrity(t)
	if !running() {
		t.Fatal("the old TUI exited while the new hooks ran")
	}
}

// R301, new TUI and old hooks: the TUI is the new version, and the sessions'
// agents still run hook commands naming the old binary, whose schema is older
// than the database the new TUI keeps. The old hook finds the writer binary
// recorded in the database, re-execs it, and the new version lands the update.
func TestUpdateNewTUIOldHooks(t *testing.T) {
	oldHook := buildLiveDeck(t, "deck-old", liveOldVersion, oldHookSchema)
	newBin := buildLiveDeck(t, "deck-new", liveNewVersion, 0)
	oldPath := filepath.Join(t.TempDir(), "old", "deck")
	newPath := filepath.Join(t.TempDir(), "new", "deck")
	installLive(t, oldHook, oldPath)
	installLive(t, newBin, newPath)
	command := shellQuote(oldPath)

	f := newLiveFixture(t)
	running := f.startTUI(t, newPath)

	f.requestPermission(t, command, "before")
	before := f.statuses(t)
	for _, s := range liveSessions {
		if before[s.id] != "waiting" {
			t.Fatalf("status of %s = %q after the old hook, want waiting (all %v)", s.id, before[s.id], before)
		}
	}
	f.requestPermission(t, command, "after")

	for _, s := range liveSessions {
		got := f.versions(t)[s.id]
		if len(got) != 2 || got[0] != liveNewVersion || got[1] != liveNewVersion {
			t.Errorf("event hook versions for %s = %v, want the re-exec'd %s both times", s.id, got, liveNewVersion)
		}
	}
	after := f.statuses(t)
	for id, want := range before {
		if after[id] != want {
			t.Errorf("status of %s = %q, want %q", id, after[id], want)
		}
	}
	if got := f.writerBinary(t); got != newPath {
		t.Errorf("writer_binary = %q, want the new TUI's path %q", got, newPath)
	}
	f.assertIntegrity(t)
	if !running() {
		t.Fatal("the new TUI exited while the old hooks ran")
	}
}
