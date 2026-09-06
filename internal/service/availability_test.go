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

// TestAvailDirectoryNamedPiDoesNotCount asserts that a directory entry
// named "pi" on the probed PATH does not satisfy the probe: lookPathIn
// (and therefore kindAvailable) must find an executable *file*, not merely
// a same-named directory sitting in one of PATH's directories.
func TestAvailDirectoryNamedPiDoesNotCount(t *testing.T) {
	dir := t.TempDir()
	piDir := filepath.Join(dir, "pi")
	if err := os.Mkdir(piDir, 0o755); err != nil {
		t.Fatalf("mkdir fake pi directory: %v", err)
	}
	if kindAvailable(agent.NewPi(), dir) {
		t.Fatalf("kindAvailable(pi, %q) = true, want false (PATH entry named pi is a directory, not an executable)", dir)
	}
}

// TestAvailPiUnavailableWhenModeNonExecutable asserts the R111 gap an
// independent review caught (docs/reports/phase3k-findings.md finding 1):
// a mode-0644 regular file named "pi" on the probed PATH exists and is not
// a directory, but it is not executable, so kindAvailable must report it
// unavailable rather than treating mere existence as availability.
func TestAvailPiUnavailableWhenModeNonExecutable(t *testing.T) {
	dir := t.TempDir()
	piPath := filepath.Join(dir, "pi")
	if err := os.WriteFile(piPath, []byte("#!/bin/sh\nexit 0\n"), 0o644); err != nil {
		t.Fatalf("write non-executable fake pi file: %v", err)
	}
	if kindAvailable(agent.NewPi(), dir) {
		t.Fatalf("kindAvailable(pi, %q) = true, want false (pi on that PATH is mode 0644, not executable)", dir)
	}
}

// TestLookPathInFindsExecutableOnPath asserts the direct, package-private
// contract of lookPathIn itself: given a name that resolves to an
// executable file in one of pathEnv's directories, it returns a nil error.
func TestLookPathInFindsExecutableOnPath(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pi"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake pi executable: %v", err)
	}
	if err := lookPathIn("pi", dir); err != nil {
		t.Fatalf("lookPathIn(\"pi\", %q) = %v, want nil (pi executable is on that PATH)", dir, err)
	}
}

// TestLookPathInErrorsWhenNameAbsentFromPath asserts that lookPathIn
// returns a non-nil error when the requested name is nowhere on pathEnv.
func TestLookPathInErrorsWhenNameAbsentFromPath(t *testing.T) {
	dir := t.TempDir() // empty: no "pi" written into it
	if err := lookPathIn("pi", dir); err == nil {
		t.Fatalf("lookPathIn(\"pi\", %q) = nil, want a non-nil error (no pi on that PATH)", dir)
	}
}

// TestLookPathInErrorsWhenNameIsADirectory asserts that lookPathIn returns
// a non-nil error when the requested name matches a *directory* entry in
// one of pathEnv's directories, rather than an executable file.
func TestLookPathInErrorsWhenNameIsADirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "pi"), 0o755); err != nil {
		t.Fatalf("mkdir fake pi directory: %v", err)
	}
	if err := lookPathIn("pi", dir); err == nil {
		t.Fatalf("lookPathIn(\"pi\", %q) = nil, want a non-nil error (PATH entry named pi is a directory, not an executable)", dir)
	}
}

// TestLookPathInErrorsWhenNameIsModeNonExecutable asserts that lookPathIn's
// PATH-search branch rejects a mode-0644 regular file: existing and not a
// directory is not enough, it must also carry an executable bit (R111).
func TestLookPathInErrorsWhenNameIsModeNonExecutable(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pi"), []byte("#!/bin/sh\nexit 0\n"), 0o644); err != nil {
		t.Fatalf("write non-executable fake pi file: %v", err)
	}
	if err := lookPathIn("pi", dir); err == nil {
		t.Fatalf("lookPathIn(\"pi\", %q) = nil, want a non-nil error (pi on that PATH is mode 0644, not executable)", dir)
	}
}

// TestLookPathInErrorsWhenDirectPathIsModeNonExecutable asserts that
// lookPathIn's direct-path branch (file containing a path separator) also
// rejects a mode-0644 regular file, not just the PATH-search branch above.
func TestLookPathInErrorsWhenDirectPathIsModeNonExecutable(t *testing.T) {
	dir := t.TempDir()
	claudePath := filepath.Join(dir, "claude")
	if err := os.WriteFile(claudePath, []byte("#!/bin/sh\nexit 0\n"), 0o644); err != nil {
		t.Fatalf("write non-executable fake claude file: %v", err)
	}
	if err := lookPathIn(claudePath, ""); err == nil {
		t.Fatalf("lookPathIn(%q, \"\") = nil, want a non-nil error (direct path is mode 0644, not executable)", claudePath)
	}
}

// TestAvailConfigEnvPathWinsOverProcessPath asserts that config [env]'s
// PATH (Service.ConfigEnv) participates in the probe AvailableKinds runs
// and takes priority over the process's own PATH (SPEC §6.3,
// resolveLaunchEnv): pi installed only on the config PATH is available,
// and pi installed only on the process PATH is NOT available once config
// [env] sets a PATH that does not contain it, because resolveLaunchEnv's
// config layer replaces captured_path's PATH outright.
func TestAvailConfigEnvPathWinsOverProcessPath(t *testing.T) {
	// configDirWithPi holds the only pi executable for the positive half;
	// processDirWithPi holds the only one for the negative half;
	// configDirWithoutPi never holds one at all.
	configDirWithPi := t.TempDir()
	if err := os.WriteFile(filepath.Join(configDirWithPi, "pi"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake pi executable on config PATH: %v", err)
	}
	processDirWithPi := t.TempDir()
	if err := os.WriteFile(filepath.Join(processDirWithPi, "pi"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake pi executable on process PATH: %v", err)
	}
	configDirWithoutPi := t.TempDir() // empty: no pi is ever written here

	registry := agent.NewRegistry()
	registry.Register(agent.NewPi())

	// (a) pi installed ONLY on the config [env] PATH -- the process PATH
	// holds no pi -- is available: the config PATH participates in the probe.
	t.Setenv("PATH", configDirWithoutPi)
	availableViaConfig := Service{Agents: registry, ConfigEnv: map[string]string{"PATH": configDirWithPi}}.AvailableKinds()
	if len(availableViaConfig) != 1 || availableViaConfig[0] != "pi" {
		t.Fatalf("AvailableKinds() with pi only on config PATH %q (process PATH %q has none) = %v, want [pi]", configDirWithPi, configDirWithoutPi, availableViaConfig)
	}

	// (b) Sanity: with no config [env] PATH at all, the process PATH is what
	// the probe sees, so this same process-PATH-only pi IS available. Without
	// this, (c) could pass for the wrong reason (an unfindable fake pi).
	t.Setenv("PATH", processDirWithPi)
	unconfiguredProcessOnly := Service{Agents: registry}.AvailableKinds()
	if len(unconfiguredProcessOnly) != 1 || unconfiguredProcessOnly[0] != "pi" {
		t.Fatalf("AvailableKinds() with pi on process PATH %q and no config PATH = %v, want [pi] (sanity check)", processDirWithPi, unconfiguredProcessOnly)
	}

	// (c) The discriminating case: the same process PATH still holds the only
	// pi, but config [env] sets a PATH without one. Config wins, so pi
	// installed only on the process PATH must NOT be reported available.
	processOnlyUnderConfigOverride := Service{Agents: registry, ConfigEnv: map[string]string{"PATH": configDirWithoutPi}}.AvailableKinds()
	if len(processOnlyUnderConfigOverride) != 0 {
		t.Fatalf("AvailableKinds() with pi only on process PATH %q and config PATH=%q (no pi there) = %v, want [] (config PATH wins; a process-only pi must not leak availability)", processDirWithPi, configDirWithoutPi, processOnlyUnderConfigOverride)
	}
}
