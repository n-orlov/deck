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
)

// ProbeScript checks that the configured event hook can be spawned (SPEC
// §10.3, §11.4): command[0] is a path that exists and is an executable
// regular file, or a bare name that resolves on PATH. It returns nil when it
// can, and otherwise one of the Err values above. It never runs the script.
func ProbeScript(command []string) error {
	if len(command) == 0 || strings.TrimSpace(command[0]) == "" {
		return ErrScriptNotSet
	}
	name := command[0]
	if !strings.ContainsRune(name, '/') {
		if _, err := exec.LookPath(name); err != nil {
			if errors.Is(err, exec.ErrNotFound) {
				return ErrScriptMissing
			}
			return ErrScriptNotExecutable
		}
		return nil
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
