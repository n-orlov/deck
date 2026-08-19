package service

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/audit"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

func TestCreateShellPersistsLaunchesAndAudits(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()
	clock, err := config.NewClock("2025-01-02T03:04:05Z", "")
	if err != nil {
		t.Fatal(err)
	}
	paths := config.Paths{Home: home, LogDir: filepath.Join(home, "log"), StateDB: filepath.Join(home, "state.db")}
	db, err := store.Open(paths)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	logger, err := audit.New(paths, clock)
	if err != nil {
		t.Fatal(err)
	}
	socket := "deck-service-" + strings.ReplaceAll(time.Now().Format("150405.000000000"), ".", "")
	t.Cleanup(func() { _ = exec.Command("tmux", "-L", socket, "kill-server").Run() })
	service := Service{
		Store: db, TMux: tmux.Client{Socket: socket}, Audit: logger, Clock: clock,
		IDs: config.NewIDGenerator("service-test"), Shell: "/bin/sh",
	}

	session, err := service.CreateShell(context.Background(), ShellCreateInput{
		Name: "Shell: session", CWD: cwd, Env: map[string]string{"VISIBLE": "yes", "SECRET_TOKEN": "not-in-audit"},
	})
	if err != nil {
		t.Fatalf("create shell: %v", err)
	}
	if session.Agent != "shell" || session.Slug != "shell-session" || session.CWD != cwd {
		t.Fatalf("durable session = %#v", session)
	}
	live, err := service.TMux.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(live) != 1 || live[0].Name != "deck_shell-session" || len(live[0].Panes) != 1 || live[0].Panes[0].CurrentPath != cwd {
		t.Fatalf("live tmux sessions = %#v", live)
	}
	rows, err := db.ListSessions(context.Background())
	if err != nil || len(rows) != 1 || rows[0].ID != session.ID || rows[0].StatusSource != "tmux" {
		t.Fatalf("durable rows = %#v, %v; want tmux-observed ready row", rows, err)
	}

	records := auditRecords(t, logger.Path())
	if len(records) != 3 {
		t.Fatalf("audit record count = %d, want starting, launch, and ready records", len(records))
	}
	starting, launch, ready := records[0], records[1], records[2]
	if starting["event"] != "starting" || starting["session_id"] != session.ID || starting["duration_ms"].(float64) < 1 {
		t.Fatalf("starting transition = %#v", starting)
	}
	if launch["event"] != "launch" || launch["session_id"] != session.ID || strings.Join(jsonStrings(launch["argv"]), "\x00") != "/bin/sh" || strings.Join(jsonStrings(launch["env_keys"]), ",") != "SECRET_TOKEN,VISIBLE" {
		t.Fatalf("launch audit = %#v", launch)
	}
	if ready["event"] != "launch.ready" || ready["session_id"] != session.ID || ready["duration_ms"].(float64) < 1 {
		t.Fatalf("ready transition = %#v", ready)
	}
	contents, err := os.ReadFile(logger.Path())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(contents), "not-in-audit") {
		t.Fatalf("audit leaked environment value: %s", contents)
	}
}

func TestKillStopsSessionPreservesCWDAndRecordsTransition(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()
	sentinel := filepath.Join(cwd, "deck-kill-sentinel")
	contents := []byte("user files are never owned by deck\n")
	if err := os.WriteFile(sentinel, contents, 0o600); err != nil {
		t.Fatal(err)
	}
	clock, _ := config.NewClock("2025-01-02T03:04:05Z", "")
	paths := config.Paths{Home: home, LogDir: filepath.Join(home, "log"), StateDB: filepath.Join(home, "state.db")}
	db, err := store.Open(paths)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	logger, err := audit.New(paths, clock)
	if err != nil {
		t.Fatal(err)
	}
	socket := "deck-kill-" + strings.ReplaceAll(filepath.Base(home), "_", "")
	t.Cleanup(func() { _ = exec.Command("tmux", "-L", socket, "kill-server").Run() })
	svc := Service{Store: db, TMux: tmux.Client{Socket: socket}, Audit: logger, Clock: clock, IDs: config.NewIDGenerator("kill-test"), Shell: "/bin/sh"}
	session, err := svc.CreateShell(context.Background(), ShellCreateInput{Name: "keep cwd", CWD: cwd})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Kill(context.Background(), session); err != nil {
		t.Fatal(err)
	}
	live, err := svc.TMux.List(context.Background())
	if err != nil || len(live) != 0 {
		t.Fatalf("live sessions after kill = %#v, %v", live, err)
	}
	rows, err := db.ListSessions(context.Background())
	if err != nil || len(rows) != 1 || rows[0].Status != "stopped" || rows[0].StatusSource != "user" || !rows[0].KilledByUser {
		t.Fatalf("durable rows after kill = %#v, %v", rows, err)
	}
	var events int
	if err := db.DB().QueryRow(`SELECT count(*) FROM events WHERE session_id = ? AND kind = 'killed'`, session.ID).Scan(&events); err != nil || events != 1 {
		t.Fatalf("killed event count = %d, %v", events, err)
	}
	got, err := os.ReadFile(sentinel)
	if err != nil || string(got) != string(contents) {
		t.Fatalf("sentinel after kill = %q, %v", got, err)
	}
	log, err := os.ReadFile(logger.Path())
	if err != nil || !strings.Contains(string(log), `"event":"killed"`) || !strings.Contains(string(log), session.ID) {
		t.Fatalf("kill audit = %q, %v", log, err)
	}
}

func TestCreateShellRecordsCoherentFailureWhenTmuxCannotLaunch(t *testing.T) {
	home := t.TempDir()
	clock, _ := config.NewClock("2025-01-02T03:04:05Z", "")
	paths := config.Paths{Home: home, LogDir: filepath.Join(home, "log"), StateDB: filepath.Join(home, "state.db")}
	db, err := store.Open(paths)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	logger, err := audit.New(paths, clock)
	if err != nil {
		t.Fatal(err)
	}
	service := Service{Store: db, TMux: tmux.Client{Socket: "invalid/socket"}, Audit: logger, Clock: clock, IDs: config.NewIDGenerator("failure"), Shell: "/bin/sh"}
	_, err = service.CreateShell(context.Background(), ShellCreateInput{Name: "broken", CWD: filepath.Join(home, "does-not-exist")})
	if err == nil || !strings.Contains(err.Error(), "launch shell session") {
		t.Fatalf("launch error = %v", err)
	}
	rows, err := db.ListSessions(context.Background())
	if err != nil || len(rows) != 1 || rows[0].Status != "error" || !strings.Contains(rows[0].StatusReason, "launch shell session") {
		t.Fatalf("failed session row = %#v, %v", rows, err)
	}
	var events int
	if err := db.DB().QueryRow(`SELECT count(*) FROM events WHERE session_id = ? AND kind = 'launch.failed'`, rows[0].ID).Scan(&events); err != nil || events != 1 {
		t.Fatalf("launch failure event count = %d, %v", events, err)
	}
	records := auditRecords(t, logger.Path())
	if len(records) != 2 {
		t.Fatalf("audit record count = %d, want starting and failure transitions", len(records))
	}
	for index, want := range []string{"starting", "launch.failed"} {
		record := records[index]
		if record["event"] != want || record["session_id"] != rows[0].ID || record["duration_ms"].(float64) < 1 {
			t.Fatalf("audit transition %d = %#v, want %q session-scoped positive-duration record", index, record, want)
		}
	}
}

func auditRecords(t *testing.T, path string) []map[string]any {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	var records []map[string]any
	for {
		var record map[string]any
		err := decoder.Decode(&record)
		if err == io.EOF {
			return records
		}
		if err != nil {
			t.Fatalf("decode JSONL audit record: %v", err)
		}
		records = append(records, record)
	}
}

func jsonStrings(value any) []string {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	strings := make([]string, 0, len(items))
	for _, item := range items {
		value, ok := item.(string)
		if !ok {
			return nil
		}
		strings = append(strings, value)
	}
	return strings
}
