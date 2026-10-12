package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/tmux"
)

func scriptedPanePort(t *testing.T, body string) PanePort {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tmux")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return Service{TMux: tmux.Client{Binary: path, Socket: "pane-port", Timeout: 5 * time.Second}}.panePort()
}

// R256: the real pane port reads the live screen through tmux, reports a
// vanished pane as an empty screen without an error, and wraps other failures.
func TestPushPanePortScreen(t *testing.T) {
	ctx := context.Background()
	live := scriptedPanePort(t, `printf '0|90|30\nhello\n'`)
	if screen, err := live.Screen(ctx, "a"); err != nil || screen != "hello\n" {
		t.Fatalf("live Screen = %q, %v; want the pane text", screen, err)
	}
	gone := scriptedPanePort(t, `echo "can't find session: deck_a" >&2; exit 1`)
	if screen, err := gone.Screen(ctx, "a"); err != nil || screen != "" {
		t.Fatalf("vanished Screen = %q, %v; want empty and no error", screen, err)
	}
	broken := scriptedPanePort(t, `echo "protocol error" >&2; exit 1`)
	if _, err := broken.Screen(ctx, "a"); err == nil || !strings.Contains(err.Error(), "protocol error") {
		t.Fatalf("broken Screen error = %v, want the tmux failure", err)
	}
}
