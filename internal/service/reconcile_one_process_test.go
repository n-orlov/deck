package service

import (
	"context"
	"fmt"
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

// TestReconcilePassIsOneTmuxProcessWhateverTheSessionCount pins R184 (GH #49):
// a reconcile pass with no status probe due reads tmux once, so it spawns
// exactly one tmux process over 12 live shell sessions, over 1 and over 0.
func TestReconcilePassIsOneTmuxProcessWhateverTheSessionCount(t *testing.T) {
	for _, count := range []int{12, 1, 0} {
		t.Run(fmt.Sprintf("%d_sessions", count), func(t *testing.T) {
			home, cwd := t.TempDir(), t.TempDir()
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
			socket := fmt.Sprintf("priv-reconcile-one-%d-%d", os.Getpid(), time.Now().UnixNano())
			t.Cleanup(func() { _ = exec.Command("tmux", "-L", socket, "kill-server").Run() })
			client := tmux.Client{Socket: socket, Timeout: 10 * time.Second}
			for i := 0; i < count; i++ {
				name := fmt.Sprintf("shell-%02d", i)
				reconcileSession(t, db, fmt.Sprintf("00000000-0000-4000-8000-0000000001%02d", i), name, cwd, "shell", "running", "user")
				if _, err := client.Create(context.Background(), tmux.Launch{Slug: name, CWD: cwd, Command: []string{"/bin/sh", "-c", "exec sleep 120"}}); err != nil {
					t.Fatalf("create %s: %v", name, err)
				}
			}

			realTmux, err := exec.LookPath("tmux")
			if err != nil {
				t.Fatal(err)
			}
			shimDir := t.TempDir()
			invocationLog := filepath.Join(shimDir, "invocations")
			shim := filepath.Join(shimDir, "tmux")
			script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"" + invocationLog + "\"\nexec " + realTmux + " \"$@\"\n"
			if err := os.WriteFile(shim, []byte(script), 0o700); err != nil {
				t.Fatal(err)
			}
			client.Binary = shim
			service := Service{Store: db, TMux: client, Audit: logger, Clock: clock}

			if err := service.Reconcile(context.Background()); err != nil {
				t.Fatalf("reconcile: %v", err)
			}
			var invocations []string
			if data, err := os.ReadFile(invocationLog); err == nil {
				invocations = strings.Split(strings.TrimRight(string(data), "\n"), "\n")
			}
			if len(invocations) != 1 {
				t.Fatalf("one reconcile pass over %d live sessions spawned %d tmux processes, want exactly 1:\n%s", count, len(invocations), strings.Join(invocations, "\n"))
			}
			if !strings.HasPrefix(invocations[0], "-L "+socket+" ") || !strings.Contains(invocations[0], "list-panes -a") {
				t.Fatalf("the single process is %q, want one `list-panes -a` read", invocations[0])
			}
		})
	}
}
