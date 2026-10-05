package tmux

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// fakeTmuxClient returns a Client whose tmux binary is a shell script running
// body, so a test drives Client's own error and parsing branches without a
// real tmux server. The script sees tmux's arguments as "$@".
func fakeTmuxClient(t *testing.T, body string) Client {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tmux")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return Client{Binary: path, Socket: "fake-socket", Timeout: 5 * time.Second}
}
