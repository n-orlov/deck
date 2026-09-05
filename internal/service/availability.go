package service

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/n-orlov/deck/internal/agent"
)

// lookPathIn reports whether file (an adapter's declared launch executable,
// e.g. "claude") is executable under pathEnv (a colon-separated PATH value,
// not the current process's own environment), mirroring exec.LookPath's
// search rules but against an arbitrary PATH string rather than
// os.Getenv("PATH"). A file containing a path separator is checked directly
// instead of searched.
//
// This is the one probe R111 requires: resume's preflight (resume.go),
// CreateAgent's preflight and AvailableKinds below all call this same
// function so two copies can never disagree about whether a binary is on
// PATH.
func lookPathIn(file, pathEnv string) error {
	if file == "" {
		return errors.New("empty command")
	}
	if strings.ContainsRune(file, os.PathSeparator) || strings.Contains(file, "/") {
		info, err := os.Stat(file)
		if err != nil {
			return err
		}
		if info.IsDir() {
			return fmt.Errorf("%s is a directory", file)
		}
		return nil
	}
	for _, dir := range filepath.SplitList(pathEnv) {
		if dir == "" {
			dir = "."
		}
		candidate := filepath.Join(dir, file)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return nil
		}
	}
	return fmt.Errorf("%s: executable file not found in $PATH", file)
}

// kindAvailable reports whether kind's declared executable is available
// under pathEnv: an adapter with no executable to probe (the empty string,
// e.g. shell) is always available (SPEC R111); every other adapter is
// available exactly when lookPathIn finds its declared executable on
// pathEnv.
func kindAvailable(a agent.Adapter, pathEnv string) bool {
	executable := a.Capabilities().Executable
	if executable == "" {
		return true
	}
	return lookPathIn(executable, pathEnv) == nil
}

// AvailableKinds returns the stable, sorted set of kinds in s.Agents whose
// declared executable (internal/agent's Caps.Executable) is available under
// the PATH a new pane would get at list time (SPEC §6.3): deck's own
// os.Getenv("PATH") as captured_path, merged with config [env]'s PATH if it
// sets one -- exactly resolveLaunchEnv(os.Getenv("PATH"), nil)["PATH"]. No
// session env exists at list time, so it never participates. An adapter
// whose declared executable is empty (shell) is always available, since
// there is nothing to look up on PATH.
func (s Service) AvailableKinds() []string {
	if s.Agents == nil {
		return nil
	}
	pathEnv := s.resolveLaunchEnv(os.Getenv("PATH"), nil)["PATH"]
	kinds := s.Agents.Kinds()
	available := make([]string, 0, len(kinds))
	for _, kind := range kinds {
		a, ok := s.Agents.Lookup(kind)
		if !ok {
			continue
		}
		if kindAvailable(a, pathEnv) {
			available = append(available, kind)
		}
	}
	return available
}
