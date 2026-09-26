// Package suitecheck is the R145 probe for task 015/ci/suite.sh: a static
// structural check over the script's own text that fails the moment its
// retry-once/flaky-recording behaviour, for either the ordinary Go package
// matrix or features/'s per-scenario rerun, goes missing. Tier 3 probe
// audit (task 025) added this alongside the ci/lint.sh, ci.yml fork-guard
// and ci/releasegate probes.
package suitecheck

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stripCommentLines drops every line whose first non-whitespace character
// is `#` (a POSIX sh full-line comment), so a marker mentioned only in
// prose above the real code is never mistaken for the real invocation.
func stripCommentLines(s string) string {
	lines := strings.Split(s, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

func repositoryRoot() (string, error) {
	directory, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(directory, "go.mod")); err == nil {
			return directory, nil
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return "", errors.New("locate repository root containing go.mod")
		}
		directory = parent
	}
}

// TestSuiteScriptRetriesOnceAndRecordsFlakyForBothPasses is the probe for
// task 015's ci/suite.sh: it fails if any of the following goes missing
// from the script's own text:
//
//  1. the ordinary Go package matrix is driven with gotestsum's own
//     `--rerun-fails=1` (fail once, retry once) fed by
//     `--rerun-fails-report`, and the post-filter that keeps only entries
//     where `failures < runs` -- a test that fails on every attempt is not
//     flaky, it is a real failure, and must not be written to the flaky
//     list;
//  2. a failed features/ scenario is rerun by its own `<file>:<line>`
//     selector (`DECK_GODOG_PATHS=`), never the whole package -- the
//     rerun call for features/TestFeatures never omits `-run
//     '^TestFeatures$'` scoped alone without that selector present in the
//     same invocation family; and
//  3. a scenario that fails again on that solo rerun still makes the
//     overall script exit non-zero (an `all_recovered`-style guard that is
//     never unconditionally reset to success).
//
// Demonstrated failing against a scratch mutation removing
// `--rerun-fails=1` from a disposable worktree's own copy of ci/suite.sh:
// see /run/ralphd/artifacts/probes/tier3/015-suite-rerun-fails-removed-fail.log.
func TestSuiteScriptRetriesOnceAndRecordsFlakyForBothPasses(t *testing.T) {
	root, err := repositoryRoot()
	if err != nil {
		t.Fatalf("repositoryRoot: %v", err)
	}
	path := filepath.Join(root, "ci", "suite.sh")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	// The script's own header comment mentions several of these markers in
	// prose (explaining *why* the design works the way it does), so a
	// mutation removing the real invocation but leaving the comment intact
	// must not still read as present. Strip full-line comments first.
	content := stripCommentLines(string(raw))

	if !strings.Contains(content, "--rerun-fails=1") {
		t.Errorf("%s: missing gotestsum's `--rerun-fails=1` (fail-once-retry-once) on the Go package matrix", path)
	}
	if !strings.Contains(content, "--rerun-fails-report") {
		t.Errorf("%s: missing `--rerun-fails-report`, the source the flaky-list post-filter reads", path)
	}
	if !strings.Contains(content, `"$fails" -lt "$runs"`) {
		t.Errorf(`%s: missing the flaky post-filter (%q) -- a test that failed on every attempt would be written to the flaky list instead of failing the run`, path, `"$fails" -lt "$runs"`)
	}

	if !strings.Contains(content, "DECK_GODOG_PATHS=$loc") {
		t.Errorf("%s: missing the per-scenario rerun selector `DECK_GODOG_PATHS=$loc` -- a failed features/ scenario would be rerun some other way, e.g. the whole package", path)
	}
	if !strings.Contains(content, "all_recovered=0") {
		t.Errorf("%s: missing the guard that keeps a scenario failing again on its solo rerun from being reported as recovered", path)
	}
	if !strings.Contains(content, `if [ "$all_recovered" -eq 1 ]`) {
		t.Errorf("%s: missing the conditional that only clears features_status when every rerun scenario actually recovered", path)
	}
}
