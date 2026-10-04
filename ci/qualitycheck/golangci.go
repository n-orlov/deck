// R188's golangci-lint gate: it runs ci/golangci.sh, the SAME single
// invocation ci/lint.sh runs in the `lint` job (the whole tree, tests
// included, under the checked-in .golangci.yml), so the two entry points
// can never disagree about what "lint is clean" means. There is no
// threshold: any finding, any linter failure and any missing script all
// fail the gate. gofmt, go vet and go mod tidy are NOT run here; they live
// only in ci/lint.sh.
package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// golangciConfig is the "golangci" section of ci/quality.json.
type golangciConfig struct {
	Enabled bool `json:"enabled"`
}

// golangciOptions is everything one gate run needs.
type golangciOptions struct {
	Script string // the shared invocation script (default ci/golangci.sh under Target)
	Target string // repository root the script runs from
}

// runGolangciGate runs the gate. ok=false with a nil error is a lint
// failure; a non-nil error is a tooling failure (exit 2): the script is
// missing or cannot be started at all.
func runGolangciGate(o golangciOptions) (ok bool, output string, err error) {
	target := o.Target
	if target == "" {
		target = "."
	}
	script := o.Script
	if script == "" {
		script = filepath.Join(target, "ci", "golangci.sh")
	}
	if _, serr := os.Stat(script); serr != nil {
		return false, "", fmt.Errorf("golangci gate: the shared invocation %s is missing: %w", script, serr)
	}
	cmd := exec.Command("sh", script) //nolint:gosec // G204: the script is the gate's own flag-supplied shared invocation, never external input
	cmd.Dir = target
	out, runErr := cmd.CombinedOutput()
	text := string(out)
	if runErr == nil {
		return true, text + "what to do: nothing -- golangci-lint reports zero findings on the whole tree.\n", nil
	}
	var ee *exec.ExitError
	if errors.As(runErr, &ee) {
		return false, text + "what to do: fix each finding at its root (handle the error, rename the parameter, add the doc comment); " +
			"a //nolint:<linter> // <reason> is allowed only for one genuine false positive, never a bulk or baseline exclusion.\n", nil
	}
	return false, text, fmt.Errorf("running %s: %w", script, runErr)
}
