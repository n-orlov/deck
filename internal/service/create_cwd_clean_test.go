package service

import (
	"context"
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

// TestCreateStoresCleanedCWD holds SPEC §4/§11.7: a created session's stored
// cwd is the cleaned path, whatever trailing-separator form the caller typed,
// and the recent-cwd entry the create promotes is that same path.
func TestCreateStoresCleanedCWD(t *testing.T) {
	forms := map[string]func(dir string) string{
		"trailing-slash": func(d string) string { return d + "/" },
		"double-slash":   func(d string) string { return d + "//" },
		"dot-segment":    func(d string) string { return d + "/./" },
	}
	for name, form := range forms {
		t.Run("agent/"+name, func(t *testing.T) {
			dir := t.TempDir()
			stubExecutableOnPath(t, "claude")
			service, db, _, _ := newAgentTestService(t, nil, "create-clean-agent-"+name)
			service.RecentCwdLimit = 5
			session, err := service.CreateAgent(context.Background(), AgentCreateInput{
				Name: "Claude: clean", CWD: form(dir), Agent: "claude", PermissionProfile: "edits",
			})
			if err != nil {
				t.Fatalf("create agent: %v", err)
			}
			assertCleanedCWD(t, db, session, dir)
		})
		t.Run("shell/"+name, func(t *testing.T) {
			dir := t.TempDir()
			home := t.TempDir()
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
			socket := "priv-service-clean-" + strings.ReplaceAll(time.Now().Format("150405.000000000"), ".", "")
			t.Cleanup(func() { _ = exec.Command("tmux", "-L", socket, "kill-server").Run() })
			service := Service{
				Store: db, TMux: tmux.Client{Socket: socket}, Audit: logger, Clock: clock,
				IDs: config.NewIDGenerator("service-clean-test"), Shell: "/bin/sh", RecentCwdLimit: 5,
			}
			session, err := service.CreateShell(context.Background(), ShellCreateInput{Name: "clean-cwd", CWD: form(dir)})
			if err != nil {
				t.Fatalf("create shell: %v", err)
			}
			assertCleanedCWD(t, db, session, dir)
		})
	}
}

func assertCleanedCWD(t *testing.T, db *store.Store, created store.Session, dir string) {
	t.Helper()
	if created.CWD != dir {
		t.Fatalf("returned cwd = %q, want %q", created.CWD, dir)
	}
	row, err := db.GetSession(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("read stored row: %v", err)
	}
	if row.CWD != dir {
		t.Fatalf("stored cwd = %q, want the cleaned %q", row.CWD, dir)
	}
	recent, err := db.RecentCwds(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(recent) != 1 || recent[0].Path != dir {
		t.Fatalf("recent cwds = %+v, want exactly [%q]", recent, dir)
	}
}
