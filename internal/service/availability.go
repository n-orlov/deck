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
// A candidate must also be a regular file (info.Mode().IsRegular()) in both
// branches: a FIFO, socket, device or other non-regular node with the
// executable bits set (mode 0755, say) is rejected even though its mode bits
// alone would pass isExecutable. This is a deliberate tightening beyond what
// exec.LookPath itself guarantees on some Go releases (older stdlib
// implementations accept non-regular-but-executable-mode files) -- it is not
// a restoration of stdlib parity. os.Stat follows symlinks, so a symlink to
// a real regular executable still passes IsRegular.
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
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%s: not a regular file", file)
		}
		if !isExecutable(info) {
			return fmt.Errorf("%s: not executable", file)
		}
		return nil
	}
	for _, dir := range filepath.SplitList(pathEnv) {
		if dir == "" {
			dir = "."
		}
		candidate := filepath.Join(dir, file)
		if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() && isExecutable(info) {
			return nil
		}
	}
	return fmt.Errorf("%s: executable file not found in $PATH", file)
}

// isExecutable reports whether info's mode has at least one of the POSIX
// executable bits (owner, group or other) set, mirroring the check
// exec.LookPath performs on the file it finds: a regular file that merely
// exists and is not a directory (mode 0644, say) is not itself sufficient
// -- R111 requires resolving an executable, not any same-named file.
func isExecutable(info os.FileInfo) bool {
	return info.Mode()&0111 != 0
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
