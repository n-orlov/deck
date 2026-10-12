package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// TestDeckNewBlankNameIsCwdBasenameStamp (PRD "Auto", R248): a blank `deck new`
// name is `<cwd-basename>-<MMDD-HHMM>` with derived provenance, a repeat in the
// same minute takes the first free -2, -3 suffix, another directory gets its
// own basename, and explicit names are stored verbatim as user.
func TestDeckNewBlankNameIsCwdBasenameStamp(t *testing.T) {
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "copilot"), []byte("#!/bin/sh\nexec sleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	env := append(tempDeckEnv(t), "DECK_CLOCK=2025-08-20T14:43:00Z")
	socket := "priv-blank-" + strings.ReplaceAll(filepath.Base(deckEnvValue(t, env, "XDG_STATE_HOME")), "_", "")
	defer killTmuxServer(socket)
	env = append(env, "DECK_TMUX_SOCKET="+socket)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	dirA := filepath.Join(t.TempDir(), "alpha")
	dirB := filepath.Join(t.TempDir(), "beta.v2")
	for _, d := range []string{dirA, dirB} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	run := func(args ...string) string {
		t.Helper()
		code, stdout, stderr := runWithEnv(t, env, append([]string{"new", "--agent", "copilot"}, args...)...)
		if code != 0 {
			t.Fatalf("deck new %v: exit %d %q", args, code, stderr)
		}
		return strings.TrimSpace(stdout)
	}
	cases := []struct {
		args []string
		want string
		src  string
	}{
		{[]string{"--cwd", dirA}, "alpha-0820-1443", "derived"},
		{[]string{"--cwd", dirA}, "alpha-0820-1443-2", "derived"},
		{[]string{"--cwd", dirA, "--name", "   "}, "alpha-0820-1443-3", "derived"},
		{[]string{"--cwd", dirB + "/"}, "beta.v2-0820-1443", "derived"},
		{[]string{"--cwd", dirA, "--name", "alpha-0820-1443"}, "", "user"}, // collides: explicit names keep their refusal
		{[]string{"--cwd", dirA, "--name", "mine"}, "mine", "user"},
	}
	for _, tc := range cases {
		if tc.want == "" {
			code, _, _ := runWithEnv(t, env, append([]string{"new", "--agent", "copilot"}, tc.args...)...)
			if code == 0 {
				t.Fatalf("explicit colliding name %v was accepted", tc.args)
			}
			continue
		}
		if got := run(tc.args...); got != tc.want {
			t.Fatalf("deck new %v named %q, want %q", tc.args, got, tc.want)
		}
	}
	settings, err := config.LoadFromProfile(os.Getenv, os.UserHomeDir, "")
	if err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(settings.Paths)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	sources := map[string]string{}
	sessions, err := db.ListSessions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range sessions {
		sources[s.Name] = s.NameSource
	}
	for i, tc := range cases {
		if tc.want == "" {
			continue
		}
		if sources[tc.want] != tc.src {
			t.Errorf("case %d: %q stored name_source %q, want %q", i, tc.want, sources[tc.want], tc.src)
		}
	}
}
