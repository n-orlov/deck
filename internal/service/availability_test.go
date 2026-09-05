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

// TestAvailConfigEnvPathWinsOverProcessPath asserts that config [env]'s
// PATH (Service.ConfigEnv) participates in the probe AvailableKinds runs
// and takes priority over the process's own PATH (SPEC §6.3,
// resolveLaunchEnv): pi installed only on the config PATH is available,
// and pi installed only on the process PATH (with a different config PATH
// set) is not, because resolveLaunchEnv's config layer wins.
func TestAvailConfigEnvPathWinsOverProcessPath(t *testing.T) {
	configDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(configDir, "pi"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake pi executable on config PATH: %v", err)
	}
	processDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(processDir, "pi"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake pi executable on process PATH: %v", err)
	}

	registry := agent.NewRegistry()
	registry.Register(agent.NewPi())

	t.Setenv("PATH", "")
	availableViaConfig := Service{Agents: registry, ConfigEnv: map[string]string{"PATH": configDir}}.AvailableKinds()
	if len(availableViaConfig) != 1 || availableViaConfig[0] != "pi" {
		t.Fatalf("AvailableKinds() with pi only on config PATH = %v, want [pi]", availableViaConfig)
	}

	t.Setenv("PATH", processDir)
	unconfiguredProcessOnly := Service{Agents: registry}.AvailableKinds()
	if len(unconfiguredProcessOnly) != 1 || unconfiguredProcessOnly[0] != "pi" {
		t.Fatalf("AvailableKinds() with pi on process PATH and no config PATH = %v, want [pi] (sanity check)", unconfiguredProcessOnly)
	}

	availableWithConfigOverride := Service{Agents: registry, ConfigEnv: map[string]string{"PATH": configDir}}.AvailableKinds()
	if len(availableWithConfigOverride) != 1 || availableWithConfigOverride[0] != "pi" {
		t.Fatalf("AvailableKinds() with process PATH=%q and config PATH=%q = %v, want [pi] (config PATH wins, process-only pi must not leak availability)", processDir, configDir, availableWithConfigOverride)
	}
}
