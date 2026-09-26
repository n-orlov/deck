package suitecheck

import (
	"encoding/xml"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// copyFile copies src to dst, creating dst's parent directories.
func copyFile(t *testing.T, src, dst string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(dst), err)
	}
	in, err := os.Open(src)
	if err != nil {
		t.Fatalf("open %s: %v", src, err)
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		t.Fatalf("create %s: %v", dst, err)
	}
	if _, err := io.Copy(out, in); err != nil {
		t.Fatalf("copy %s -> %s: %v", src, dst, err)
	}
	if err := out.Close(); err != nil {
		t.Fatalf("close %s: %v", dst, err)
	}
}

type fixtureJUnitFailure struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
}

type fixtureJUnitCase struct {
	Name    string               `xml:"name,attr"`
	Failure *fixtureJUnitFailure `xml:"failure"`
	Error   *fixtureJUnitFailure `xml:"error"`
}

type fixtureJUnitSuites struct {
	Tests    int `xml:"tests,attr"`
	Failures int `xml:"failures,attr"`
	Errors   int `xml:"errors,attr"`
	Suites   []struct {
		Cases []fixtureJUnitCase `xml:"testcase"`
	} `xml:"testsuite"`
}

// TestSuiteScriptTimeoutFixtureRetainsScenarioResultsAndAbortInReport is the
// runnable cure-01-08 (R146) regression: it runs the repository's real
// ci/suite.sh, ci/junitflaky and ci/summary.sh, unmodified, against the
// deterministic timeout fixture in testdata/timeoutfixture/ (a TestFeatures
// that leaves Godog's JUnit file empty, completes one scenario subtest and
// then blocks until a 3s DECK_CI_FEATURES_TEST_TIMEOUT kills the binary --
// the shape final-sha nightly 36222292303 took). It requires that:
//
//  1. the suite exits non-zero;
//  2. the raw Godog JUnit really is empty (so the fallback path is what is
//     under test);
//  3. the merged report input junit-merged/junit-features.xml -- the file
//     ci/allure-report.sh and ci/summary.sh read -- exists, still carries
//     gotestsum's passing completed_scenario result, and carries an
//     explicit failed/aborted TestFeatures outcome; and
//  4. the rendered ci/summary.sh report counts at least one failure, so
//     the run can never be rendered as an all-passing subset.
//
// Against the pre-fix ci/suite.sh (184d98617) this fails at (3): the merged
// features file is deleted and only junit-go.xml's passing test survives.
func TestSuiteScriptTimeoutFixtureRetainsScenarioResultsAndAbortInReport(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Fatalf("go toolchain not on PATH: %v", err)
	}
	root, err := repositoryRoot()
	if err != nil {
		t.Fatalf("repositoryRoot: %v", err)
	}
	fixture := filepath.Join(root, "ci", "suitecheck", "testdata", "timeoutfixture")
	module := t.TempDir()
	copyFile(t, filepath.Join(fixture, "go.mod.fixture"), filepath.Join(module, "go.mod"), 0o644)
	copyFile(t, filepath.Join(fixture, "unit", "unit_test.go"), filepath.Join(module, "unit", "unit_test.go"), 0o644)
	copyFile(t, filepath.Join(fixture, "features", "fixture_test.go"), filepath.Join(module, "features", "fixture_test.go"), 0o644)
	copyFile(t, filepath.Join(root, "ci", "suite.sh"), filepath.Join(module, "ci", "suite.sh"), 0o755)
	copyFile(t, filepath.Join(root, "ci", "summary.sh"), filepath.Join(module, "ci", "summary.sh"), 0o755)
	copyFile(t, filepath.Join(root, "ci", "junitflaky", "main.go"), filepath.Join(module, "ci", "junitflaky", "main.go"), 0o644)

	outdir := filepath.Join(module, "out")
	env := make([]string, 0, len(os.Environ())+6)
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "DECK_") || strings.HasPrefix(kv, "GOWORK=") || strings.HasPrefix(kv, "GOCOVERDIR=") {
			continue
		}
		env = append(env, kv)
	}
	env = append(env,
		"GOWORK=off",
		"DECK_CI_OUT="+outdir,
		"DECK_CI_GO_PACKAGES=./unit/",
		"DECK_CI_FEATURES_TEST_TIMEOUT=3s",
	)

	cmd := exec.Command("sh", filepath.Join(module, "ci", "suite.sh"))
	cmd.Dir = module
	cmd.Env = env
	suiteOut, suiteErr := cmd.CombinedOutput()
	t.Logf("ci/suite.sh output:\n%s", suiteOut)
	if suiteErr == nil {
		t.Fatalf("ci/suite.sh exited 0 for a features/ pass killed by its -timeout alarm")
	}

	rawGodog := filepath.Join(outdir, "junit-features.xml")
	if fi, err := os.Stat(rawGodog); err != nil || fi.Size() != 0 {
		t.Fatalf("fixture precondition: raw Godog JUnit %s should exist and be empty (stat err %v)", rawGodog, err)
	}

	mergedPath := filepath.Join(outdir, "junit-merged", "junit-features.xml")
	raw, err := os.ReadFile(mergedPath)
	if err != nil {
		t.Fatalf("report input %s was not written: the aborted features/ pass was dropped from the report (%v)", mergedPath, err)
	}
	var merged fixtureJUnitSuites
	if err := xml.Unmarshal(raw, &merged); err != nil {
		t.Fatalf("parse %s: %v\n%s", mergedPath, err, raw)
	}
	if merged.Failures+merged.Errors == 0 {
		t.Errorf("%s reports no failures (tests=%d) for an aborted features/ pass", mergedPath, merged.Tests)
	}
	var completedPassed, abortedFailed bool
	for _, suite := range merged.Suites {
		for _, c := range suite.Cases {
			failed := c.Failure != nil || c.Error != nil
			if strings.HasSuffix(c.Name, "completed_scenario") && !failed {
				completedPassed = true
			}
			if c.Name == "TestFeatures" && failed {
				abortedFailed = true
			}
		}
	}
	if !completedPassed {
		t.Errorf("%s lost gotestsum's passing completed_scenario result:\n%s", mergedPath, raw)
	}
	if !abortedFailed {
		t.Errorf("%s has no explicit failed/aborted TestFeatures outcome:\n%s", mergedPath, raw)
	}

	summary := exec.Command("sh", filepath.Join(module, "ci", "summary.sh"), outdir)
	summary.Dir = module
	summary.Env = env
	rendered, err := summary.CombinedOutput()
	if err != nil {
		t.Fatalf("ci/summary.sh: %v\n%s", err, rendered)
	}
	m := regexp.MustCompile(`\| fail \| (\d+) \|`).FindSubmatch(rendered)
	if m == nil {
		t.Fatalf("ci/summary.sh rendered no fail row:\n%s", rendered)
	}
	if n, _ := strconv.Atoi(string(m[1])); n == 0 {
		t.Errorf("ci/summary.sh rendered the aborted run as all-passing (fail=0):\n%s", rendered)
	}
}
