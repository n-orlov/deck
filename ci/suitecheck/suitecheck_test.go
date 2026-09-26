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
	"regexp"
	"strconv"
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

// minuteDurationFlag finds the FIRST `<N>m` shell-duration literal inside
// content and returns N, or ok=false if none appears. It deliberately does
// not accept other Go duration suffixes (h, s) because every value this
// repo has ever set here is a whole number of minutes; a future caller
// writing `-timeout=1h` would fail this parse loudly rather than silently
// being read as zero.
func minuteDurationFlag(content string) (minutes int, ok bool) {
	re := regexp.MustCompile(`(\d+)m`)
	m := re.FindStringSubmatch(content)
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, false
	}
	return n, true
}

// TestFeaturesTestInvocationsCarryAGenerousExplicitTimeoutBudget is the
// cure-01-07 (R145 nightly completion) regression: `go test`'s own default
// per-test-binary alarm is an UNSET 10 minutes (testing.(*M).startAlarm),
// never one of this repo's own scenario deadlines -- every scenario/step
// assertion in features/ already carries its own, much shorter, explicit
// deadline. features/TestFeatures fans out into 300+ Godog scenarios in one
// process, and nightly run 36222292303 hit `panic: test timed out after
// 10m0s` at 600.033s/600.065s in BOTH the -race suite step and a plain
// (non-race) ci/stability.sh repetition -- the harness's own unset budget,
// not any scenario's own assertion, was too tight (see
// artifacts/review/nightly-timeout-stack.log, nightly-analysis.log). This
// fails the moment any of the three `go test` invocations that can run
// features/TestFeatures loses its own explicit `-timeout=` flag, or the
// duration it defaults to shrinks back down near the 10-minute default that
// already proved insufficient once under ordinary CI contention.
func TestFeaturesTestInvocationsCarryAGenerousExplicitTimeoutBudget(t *testing.T) {
	root, err := repositoryRoot()
	if err != nil {
		t.Fatalf("repositoryRoot: %v", err)
	}

	const minMinutes = 15 // comfortably above the 10m default that failed

	suitePath := filepath.Join(root, "ci", "suite.sh")
	raw, err := os.ReadFile(suitePath)
	if err != nil {
		t.Fatalf("read %s: %v", suitePath, err)
	}
	suiteContent := stripCommentLines(string(raw))

	// run_test_features is the function both the original features/
	// TestFeatures pass and every per-scenario solo rerun go through; it
	// must pass an explicit -timeout, not rely on go test's unset default.
	fnStart := strings.Index(suiteContent, "run_test_features()")
	if fnStart < 0 {
		t.Fatalf("%s: run_test_features() function not found", suitePath)
	}
	fnEnd := strings.Index(suiteContent[fnStart:], "\n}")
	if fnEnd < 0 {
		t.Fatalf("%s: run_test_features() function body has no closing brace", suitePath)
	}
	fnBody := suiteContent[fnStart : fnStart+fnEnd]
	featuresVarRe := regexp.MustCompile(`-timeout=\$(\w+)`)
	featuresVar := featuresVarRe.FindStringSubmatch(fnBody)
	if featuresVar == nil {
		t.Errorf("%s: run_test_features() carries no `-timeout=$<var>` flag; go test falls back to its unset 10-minute default, which already panicked TestFeatures on nightly run 36222292303", suitePath)
	} else if minutes, ok := minuteDurationFlag(assignmentDefault(suiteContent, featuresVar[1])); !ok {
		t.Errorf("%s: %s's default carries no <N>m duration to check", suitePath, featuresVar[1])
	} else if minutes < minMinutes {
		t.Errorf("%s: %s defaults to %dm, not comfortably above the 10-minute default that already failed; want >= %dm", suitePath, featuresVar[1], minutes, minMinutes)
	}

	// The unit pass (`-skip '^TestFeatures$'`) never runs TestFeatures
	// itself, but under -race it still shares the same default per-binary
	// alarm across every OTHER package in ./..., so it gets its own
	// explicit budget too.
	unitLineRe := regexp.MustCompile(`-skip '\^TestFeatures\$' "-timeout=\$(\w+)"`)
	unitVar := unitLineRe.FindStringSubmatch(suiteContent)
	if unitVar == nil {
		t.Errorf("%s: the unit pass (-skip '^TestFeatures$') carries no `-timeout=$<var>` flag immediately after it", suitePath)
	} else if minutes, ok := minuteDurationFlag(assignmentDefault(suiteContent, unitVar[1])); !ok {
		t.Errorf("%s: %s's default carries no <N>m duration to check", suitePath, unitVar[1])
	} else if minutes < minMinutes {
		t.Errorf("%s: %s defaults to %dm, not comfortably above the 10-minute default; want >= %dm", suitePath, unitVar[1], minutes, minMinutes)
	}

	// ci/stability.sh drives the whole ./... suite (features/ included)
	// directly through ci/run.sh, never through ci/suite.sh -- it needs its
	// own independent -timeout for exactly the same reason. Nightly run
	// 36222292303's third repetition is the one that actually hit the
	// panic without -race at all.
	stabilityPath := filepath.Join(root, "ci", "stability.sh")
	raw, err = os.ReadFile(stabilityPath)
	if err != nil {
		t.Fatalf("read %s: %v", stabilityPath, err)
	}
	stabilityContent := stripCommentLines(string(raw))
	stabilityLineRe := regexp.MustCompile(`-timeout=\$\{[A-Za-z0-9_]+:-(\d+)m\}`)
	stabilityMatch := stabilityLineRe.FindStringSubmatch(stabilityContent)
	if stabilityMatch == nil {
		t.Errorf("%s: the ci/run.sh go test invocation carries no explicit -timeout=${VAR:-<N>m} flag; nightly run 36222292303's third repetition hit the unset 10-minute default's panic without -race at all", stabilityPath)
	} else if minutes, _ := strconv.Atoi(stabilityMatch[1]); minutes < minMinutes {
		t.Errorf("%s: -timeout defaults to %dm, not comfortably above the 10-minute default that already failed; want >= %dm", stabilityPath, minutes, minMinutes)
	}
}

// TestSuiteScriptRetainsFeaturesReportDataAndAnExplicitAbortOnTimeout is the
// cure-01-08 (R146) regression: final-sha nightly 36222292303 left
// junit-features.xml empty after features/TestFeatures was killed by go
// test's own -timeout alarm mid-scenario (Godog's own JUnit formatter never
// got to flush). ci/junitflaky reported EOF parsing that empty file, and
// ci/suite.sh's merge step used to treat that failure as fatal for the
// whole features/ report input: it deleted junit-merged/junit-features.xml
// outright, silently dropping every gotestsum scenario result the run DID
// produce and leaving the report/summary looking like an all-passing
// subset (junit-go.xml only) even though the run had failed
// (artifacts/review/abort-report-{probe,assertion}.log, nightly-junit-
// error.log). This fails the moment any of the following goes missing
// from ci/suite.sh's own text:
//
//  1. the primary godog-JUnit branch checks the file is non-empty (`-s`),
//     not merely present (`-f`) -- an empty file is exactly what a killed
//     process leaves behind, and `-f` alone would keep feeding it to
//     ci/junitflaky, which errors on it (EOF) with nothing to fall back to;
//  2. merge_junit itself reports failure to its caller (`return 1`) instead
//     of always returning success, which is what let the EOF failure above
//     go unnoticed by the caller in the first place;
//  3. a merge failure/empty/malformed Godog JUnit falls back to gotestsum's
//     own view of the same pass (`junit-features-gotestsum.xml`); and
//  4. a features/ pass that aborted with no scenario location parseable at
//     all still gets an explicit, synthesized failed/aborted TestFeatures
//     outcome folded into the report input (`write_aborted_testfeatures_
//     junit`), so the merged file can never read as an all-passing subset
//     for a run that actually failed.
//
// Demonstrated fixing the defect against a deterministic timeout fixture
// (a scenario that sleeps past a short DECK_CI_FEATURES_TEST_TIMEOUT) run
// through the real, unmodified ci/suite.sh in a throwaway worktree: see
// /run/ralphd/artifacts/cure-01-08/.
func TestSuiteScriptRetainsFeaturesReportDataAndAnExplicitAbortOnTimeout(t *testing.T) {
	root, err := repositoryRoot()
	if err != nil {
		t.Fatalf("repositoryRoot: %v", err)
	}
	path := filepath.Join(root, "ci", "suite.sh")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	content := stripCommentLines(string(raw))

	if !strings.Contains(content, `[ -s "$features_junit" ]`) {
		t.Errorf("%s: the primary godog-JUnit branch no longer checks the file is non-empty (`-s`) before trusting it -- an empty file left by a killed process would be fed to ci/junitflaky again", path)
	}
	if !strings.Contains(content, "return 1") {
		t.Errorf("%s: merge_junit no longer reports failure (`return 1`) to its caller -- a merge failure would go unnoticed again, exactly as it did for final-sha nightly 36222292303", path)
	}
	if !strings.Contains(content, "junit-features-gotestsum.xml") || !strings.Contains(content, "falling back to gotestsum") {
		t.Errorf("%s: missing the fallback to gotestsum's own scenario JUnit when Godog's own file is missing, empty or malformed", path)
	}
	if !strings.Contains(content, "write_aborted_testfeatures_junit") {
		t.Errorf("%s: missing write_aborted_testfeatures_junit, the synthesized explicit failed/aborted TestFeatures outcome for a pass that aborted with no scenario location parseable", path)
	}
	if !strings.Contains(content, "features_aborted_without_locations") {
		t.Errorf("%s: missing the features_aborted_without_locations flag that ties step 2's whole-process-abort detection to step 3's synthesized outcome", path)
	}
}

// assignmentDefault returns the raw text of the shell default expression a
// `name=${ENV_VAR:-<default>}` (or `name=<literal>`) assignment for the
// given variable name gives it, or "" if no such assignment line exists.
func assignmentDefault(content, name string) string {
	re := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(name) + `=(.+)$`)
	m := re.FindStringSubmatch(content)
	if m == nil {
		return ""
	}
	return m[1]
}
