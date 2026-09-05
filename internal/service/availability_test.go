package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/n-orlov/deck/internal/agent"
)

// TestAvailShellAlwaysAvailableUnderEmptyPath asserts the R111 floor:
// shell declares no executable, so it is available even when the probed
// PATH is empty (kindAvailable never has anything to look up for it).
func TestAvailShellAlwaysAvailableUnderEmptyPath(t *testing.T) {
	if !kindAvailable(agent.NewShell(), "") {
		t.Fatalf("kindAvailable(shell, \"\") = false, want true (shell has no executable to probe)")
	}
}

// TestAvailPiUnavailableWhenAbsentFromPath asserts that pi is reported
// unavailable when its declared executable ("pi") is nowhere on the
// probed PATH.
func TestAvailPiUnavailableWhenAbsentFromPath(t *testing.T) {
	dir := t.TempDir() // empty: no "pi" written into it
	if kindAvailable(agent.NewPi(), dir) {
		t.Fatalf("kindAvailable(pi, %q) = true, want false (no pi executable on that PATH)", dir)
	}
}

// TestAvailPiAvailableWhenPresentOnPath asserts that pi is reported
// available once an executable named pi exists in a directory on the
// probed PATH.
func TestAvailPiAvailableWhenPresentOnPath(t *testing.T) {
	dir := t.TempDir()
	piPath := filepath.Join(dir, "pi")
	if err := os.WriteFile(piPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake pi executable: %v", err)
	}
	if !kindAvailable(agent.NewPi(), dir) {
		t.Fatalf("kindAvailable(pi, %q) = false, want true (pi executable is on that PATH)", dir)
	}
}
