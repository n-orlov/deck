package notify

import (
	"errors"
	"os"
	"os/exec"
	"strings"
)

// Script problems ProbeScript reports, each a state the health view names.
var (
	// ErrScriptNotSet means event_hook names no script: the feature is inert.
	ErrScriptNotSet = errors.New("event_hook is not set")
	// ErrScriptMissing means the configured script does not exist.
	ErrScriptMissing = errors.New("script does not exist")
	// ErrScriptNotExecutable means the script exists but cannot be run.
	ErrScriptNotExecutable = errors.New("script is not executable")
	// ErrScriptRelative means the script is named by a relative path
	// (`./hook.sh`, `scripts/hook.sh`, `~/hook.sh`). deck _hook runs in the
	// agent's directory, so such a path means a different file there, or none.
	ErrScriptRelative = errors.New("script path is relative")
)

// RelativeScript reports whether command[0] is a path that is not absolute:
// it contains a slash and does not start with one. A bare name is not a path;
// it resolves on PATH like an agent binary (SPEC §10.1).
func RelativeScript(command []string) bool {
	if len(command) == 0 {
		return false
	}
	return strings.ContainsRune(command[0], '/') && !strings.HasPrefix(command[0], "/")
}

// ProbeScript checks that the configured event hook can be spawned (SPEC
// §10.3, §11.4): command[0] is a path that exists and is an executable
// regular file given by an absolute path (a relative path is
// ErrScriptRelative), or a bare name that resolves on PATH. It returns nil when it
// can, and otherwise one of the Err values above. It never runs the script.
func ProbeScript(command []string) error {
	if len(command) == 0 || strings.TrimSpace(command[0]) == "" {
		return ErrScriptNotSet
	}
	name := command[0]
	switch {
	case RelativeScript(command):
		return ErrScriptRelative
	case !strings.ContainsRune(name, '/'):
		return probeBareName(name)
	}
	info, err := os.Stat(name)
	if err != nil {
		return ErrScriptMissing
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return ErrScriptNotExecutable
	}
	return nil
}

// probeBareName reports whether a bare name resolves on PATH.
func probeBareName(name string) error {
	if _, err := exec.LookPath(name); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return ErrScriptMissing
		}
		return ErrScriptNotExecutable
	}
	return nil
}
