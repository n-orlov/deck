package service

import (
	"context"
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

// TestClearRecentCwdsIsObservableAndNeverLeaksAPathIntoTheAuditLog covers
// task 013 (requirement 17, §11.5): the settings takeover's "clear
// recent_cwds" action is a real, observable store mutation --
// store.RecentCwds returns nothing after store.ClearRecentCwds, even
// though CreateShell just promoted a real cwd into it -- and that cwd
// path, promoted or cleared, never once appears in the launch audit log.
// There is no notification payload construction anywhere in this tree to
// assert against either (grep `internal/notify` outside the deliberately
// untouched doc.go, per the standing rule, finds nothing: no phase before
// this one implements notification delivery), so the audit log is the
// full extent of "no recent path is ever written" this tree can prove.
func TestClearRecentCwdsIsObservableAndNeverLeaksAPathIntoTheAuditLog(t *testing.T) {
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
	socket := "deck-service-clear-recent-" + strings.ReplaceAll(time.Now().Format("150405.000000000"), ".", "")
	t.Cleanup(func() { _ = exec.Command("tmux", "-L", socket, "kill-server").Run() })
	service := Service{
		Store: db, TMux: tmux.Client{Socket: socket}, Audit: logger, Clock: clock,
		IDs: config.NewIDGenerator("service-clear-recent-test"), Shell: "/bin/sh", RecentCwdLimit: 5,
	}

	if _, err := service.CreateShell(context.Background(), ShellCreateInput{Name: "clear-recent-cwds", CWD: cwd}); err != nil {
		t.Fatalf("create shell: %v", err)
	}
	before, err := db.RecentCwds(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != 1 || before[0].Path != cwd {
		t.Fatalf("recent cwds before clear = %+v, want exactly [%q]", before, cwd)
	}

	if err := db.ClearRecentCwds(context.Background()); err != nil {
		t.Fatalf("clear recent cwds: %v", err)
	}
	after, err := db.RecentCwds(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 0 {
		t.Fatalf("recent cwds after clear = %+v, want none", after)
	}

	contents, err := os.ReadFile(logger.Path())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(contents), cwd) {
		t.Fatalf("audit log leaked the promoted/cleared recent cwd path %q:\n%s", cwd, contents)
	}
}
