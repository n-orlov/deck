package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/n-orlov/deck/internal/agent"
)

// TestLookPathInRejectsWhatCannotBeLaunched: an empty command, a missing
// path, a directory, a non-regular node and a non-executable file are each
// refused with their own reason, and an executable on a relative PATH entry
// ("" meaning the current directory) is accepted.
func TestLookPathInRejectsWhatCannotBeLaunched(t *testing.T) {
	dir := t.TempDir()
	plain := filepath.Join(dir, "plain")
	if err := os.WriteFile(plain, []byte("#!/bin/sh\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(dir, "tool")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	fifo := filepath.Join(dir, "fifo")
	if err := mkfifo(fifo); err != nil {
		t.Skipf("no FIFO support: %v", err)
	}
	if err := os.Chmod(fifo, 0o755); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name, file, pathEnv, want string
	}{
		{"empty command", "", dir, "empty command"},
		{"directory", dir, dir, "is a directory"},
		{"not executable", plain, dir, "not executable"},
		{"fifo", fifo, dir, "not a regular file"},
		{"missing", filepath.Join(dir, "absent"), dir, "no such file"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { wantErr(t, tc.name, lookPathIn(tc.file, tc.pathEnv), tc.want) })
	}

	if err := lookPathIn(exe, ""); err != nil {
		t.Fatalf("an absolute executable was refused: %v", err)
	}
	t.Chdir(dir)
	if err := lookPathIn("tool", ":/nonexistent"); err != nil {
		t.Fatalf("an executable in the current directory (empty PATH entry) was refused: %v", err)
	}
	wantErr(t, "not on PATH", lookPathIn("tool", "/nonexistent"), "")
}

// TestAvailableKindsWithoutARegistryIsEmpty: no registry means no kinds, and
// a registry listing only the shell adapter (no executable to look up) reports it.
func TestAvailableKindsWithoutARegistryIsEmpty(t *testing.T) {
	if got := (Service{}).AvailableKinds(); len(got) != 0 {
		t.Fatalf("AvailableKinds without a registry = %v, want none", got)
	}
	registry := agent.NewRegistry()
	registry.Register(agent.NewShell())
	got := Service{Agents: registry}.AvailableKinds()
	if len(got) != 1 || got[0] != "shell" {
		t.Fatalf("AvailableKinds = %v, want [shell]", got)
	}
}
