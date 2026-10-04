package suitecheck

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// allureResult is the part of one Allure *-result.json these tests read.
type allureResult struct {
	HistoryID string `json:"historyId"`
	Status    string `json:"status"`
	Start     int64  `json:"start"`
	Labels    []struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	} `json:"labels"`
}

func (r allureResult) group() string {
	for _, l := range r.Labels {
		if l.Name == "parentSuite" {
			return l.Value
		}
	}
	return ""
}

func readAllureResults(t *testing.T, dir string) []allureResult {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*-result.json"))
	if err != nil {
		t.Fatal(err)
	}
	var all []allureResult
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var r allureResult
		if err := json.Unmarshal(raw, &r); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		all = append(all, r)
	}
	return all
}

// allureCounts counts the way Allure itself does: one test per historyId, its
// outcome the latest result's, flaky when earlier attempts failed and the
// latest passed.
func allureCounts(results []allureResult) (tests, failed, flaky int) {
	byHistory := map[string][]allureResult{}
	for _, r := range results {
		byHistory[r.HistoryID] = append(byHistory[r.HistoryID], r)
	}
	for _, attempts := range byHistory {
		sort.Slice(attempts, func(i, j int) bool { return attempts[i].Start < attempts[j].Start })
		last := attempts[len(attempts)-1]
		tests++
		switch last.Status {
		case "failed", "broken":
			failed++
		case "passed":
			if len(attempts) > 1 {
				flaky++
			}
		}
	}
	return tests, failed, flaky
}

var summaryRow = regexp.MustCompile(`(?m)^\| (pass|fail|flaky) \| (\d+) \|$`)

// summaryCounts runs the real ci/summary.sh over a results directory.
func summaryCounts(t *testing.T, root, outdir string) (pass, fail, flaky int) {
	t.Helper()
	out, err := exec.Command("sh", filepath.Join(root, "ci", "summary.sh"), outdir).CombinedOutput()
	if err != nil {
		t.Fatalf("ci/summary.sh: %v\n%s", err, out)
	}
	got := map[string]int{}
	for _, m := range summaryRow.FindAllStringSubmatch(string(out), -1) {
		n, _ := strconv.Atoi(m[2])
		got[m[1]] = n
	}
	if len(got) != 3 {
		t.Fatalf("summary has no pass/fail/flaky rows:\n%s", out)
	}
	return got["pass"], got["fail"], got["flaky"]
}

// summaryAllureDisagreement returns "" when ci/summary.sh and the Allure
// results of the same directory count the same tests, failures and flaky tests.
func summaryAllureDisagreement(t *testing.T, root, outdir string) string {
	t.Helper()
	pass, fail, flaky := summaryCounts(t, root, outdir)
	tests, failed, allureFlaky := allureCounts(readAllureResults(t, filepath.Join(outdir, "allure-results")))
	if pass != tests-failed || fail != failed || flaky != allureFlaky {
		return fmt.Sprintf("summary says pass=%d fail=%d flaky=%d; Allure results say pass=%d fail=%d flaky=%d", pass, fail, flaky, tests-failed, failed, allureFlaky)
	}
	return ""
}

const agreementUnitJUnit = `<?xml version="1.0" encoding="UTF-8"?>
<testsuites tests="5" failures="1" errors="0" time="1">
	<testsuite name="example.com/m/a" tests="5" failures="1" errors="0" time="1">
		<testcase classname="example.com/m/a" name="TestOne" time="0.1"></testcase>
		<testcase classname="example.com/m/a" name="TestTwo" time="0.1"></testcase>
		<testcase classname="example.com/m/a" name="TestSkipped" time="0"><skipped message="skip"></skipped></testcase>
		<testcase classname="example.com/m/a" name="TestFlaky" time="0.2"><rerunFailure message="first" type="">first</rerunFailure></testcase>
		<testcase classname="example.com/m/a" name="TestFails" time="0.1"><failure message="boom" type="">boom</failure></testcase>
	</testsuite>
</testsuites>
`

const agreementFeaturesJUnit = `<?xml version="1.0" encoding="UTF-8"?>
<testsuites tests="3" failures="1" errors="0" time="1">
	<testsuite name="Fixture" tests="3" failures="1" errors="0" time="1">
		<testcase classname="Fixture" name="steady" time="0.1"></testcase>
		<testcase classname="Fixture" name="wobbly" time="0.1"><rerunFailure message="first" type="">first</rerunFailure></testcase>
		<testcase classname="Fixture" name="broken" time="0.1"><failure message="boom" type="">boom</failure></testcase>
	</testsuite>
</testsuites>
`

// writeAgreementFixture builds a results directory the way ci/suite.sh leaves
// one: merged JUnit and flaky lists for ci/summary.sh, and allure-results/
// with the unit tests converted by the real ci/junit2allure and features/'s
// native results (one retried scenario: a failed attempt, then a passing one).
func writeAgreementFixture(t *testing.T, root string) string {
	t.Helper()
	outdir := t.TempDir()
	merged := filepath.Join(outdir, "junit-merged")
	if err := os.MkdirAll(merged, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"junit-merged/junit-go.xml":       agreementUnitJUnit,
		"junit-merged/junit-features.xml": agreementFeaturesJUnit,
		"flaky-go.txt":                    "example.com/m/a TestFlaky: 2 runs, 1 failures\n",
		"flaky-features.txt":              "fixture.feature:5\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(outdir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	allure := filepath.Join(outdir, "allure-results")
	cmd := exec.Command("go", "run", "./ci/junit2allure", "-group", "unit", "-o", allure, filepath.Join(merged, "junit-go.xml"))
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ci/junit2allure: %v\n%s", err, out)
	}
	native := []struct {
		uuid, history, status string
		start                 int
	}{
		{"f1", "Fixture:steady", "passed", 10},
		{"f2", "Fixture:wobbly", "failed", 20},
		{"f3", "Fixture:wobbly", "passed", 30},
		{"f4", "Fixture:broken", "failed", 40},
	}
	for _, n := range native {
		body := fmt.Sprintf(`{"uuid":%q,"historyId":%q,"name":%q,"status":%q,"stage":"finished","start":%d,"stop":%d,"labels":[{"name":"parentSuite","value":"features"}],"steps":[],"attachments":[]}`,
			n.uuid, n.history, n.history, n.status, n.start, n.start+5)
		if err := os.WriteFile(filepath.Join(allure, n.uuid+"-result.json"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return outdir
}

// TestSummaryCountsAgreeWithTheAllureResults is task 005's (R195) proof that
// ci/summary.sh's pass/fail/flaky counts and the Allure results come from the
// same tests: on a fixture holding passes, a skip, retried-then-passed tests
// and failures in both groups the two agree, and they stop agreeing the moment
// a result goes missing from one side.
func TestSummaryCountsAgreeWithTheAllureResults(t *testing.T) {
	root, err := repositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	outdir := writeAgreementFixture(t, root)

	pass, fail, flaky := summaryCounts(t, root, outdir)
	if pass != 6 || fail != 2 || flaky != 2 {
		t.Fatalf("summary of the fixture = pass %d fail %d flaky %d, want 6/2/2", pass, fail, flaky)
	}
	if diff := summaryAllureDisagreement(t, root, outdir); diff != "" {
		t.Fatalf("fixture must agree: %s", diff)
	}
	groups := map[string]bool{}
	for _, r := range readAllureResults(t, filepath.Join(outdir, "allure-results")) {
		groups[r.group()] = true
	}
	if !groups["unit"] || !groups["features"] || len(groups) != 2 {
		t.Fatalf("result groups = %v, want exactly unit and features", groups)
	}

	// A features result dropped from the Allure side must be caught...
	if err := os.Remove(filepath.Join(outdir, "allure-results", "f4-result.json")); err != nil {
		t.Fatal(err)
	}
	if diff := summaryAllureDisagreement(t, root, outdir); diff == "" {
		t.Fatal("a missing Allure result went unnoticed: summary and report would disagree silently")
	}
}

// TestSummaryAllureCheckSeesAFlakyMarkerDisagreement: the same fixture with the
// retry's failed attempt gone makes the wobbly scenario look steady in Allure
// while ci/summary.sh still counts it flaky.
func TestSummaryAllureCheckSeesAFlakyMarkerDisagreement(t *testing.T) {
	root, err := repositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	outdir := writeAgreementFixture(t, root)
	if err := os.Remove(filepath.Join(outdir, "allure-results", "f2-result.json")); err != nil {
		t.Fatal(err)
	}
	if diff := summaryAllureDisagreement(t, root, outdir); !strings.Contains(diff, "flaky=") {
		t.Fatalf("disagreement = %q, want it to name the flaky counts", diff)
	}
}

// TestSuiteScriptWritesOneAllureResultsDirWithEnvironmentAndExecutor runs the
// real ci/suite.sh over the covfixture and reads back what R195/R196 ask of
// the results: unit and features groups side by side, environment.properties
// (Go version, tmux version, sha, race or not) and executor.json (the run link).
func TestSuiteScriptWritesOneAllureResultsDirWithEnvironmentAndExecutor(t *testing.T) {
	const runURL = "https://github.com/example/repo/actions/runs/4242"
	run := runCovFixture(t, "DECK_CI_SHA=0123abcd", "DECK_CI_RUN_URL="+runURL)
	dir := filepath.Join(run.outdir, "allure-results")

	groups := map[string]int{}
	for _, r := range readAllureResults(t, dir) {
		groups[r.group()]++
	}
	if groups["unit"] == 0 || groups["features"] == 0 || len(groups) != 2 {
		t.Fatalf("result groups = %v in %s, want unit and features only", groups, dir)
	}

	env, err := os.ReadFile(filepath.Join(dir, "environment.properties"))
	if err != nil {
		t.Fatalf("environment.properties: %v", err)
	}
	props := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(env)), "\n") {
		k, v, _ := strings.Cut(line, "=")
		props[k] = v
	}
	if !strings.HasPrefix(props["go.version"], "go1.") {
		t.Errorf("go.version = %q, want a go1.x version", props["go.version"])
	}
	if props["tmux.version"] == "" {
		t.Errorf("tmux.version missing (tmux itself or the word unknown): %q", env)
	}
	if props["git.sha"] != "0123abcd" {
		t.Errorf("git.sha = %q, want the DECK_CI_SHA value", props["git.sha"])
	}
	if props["race"] != "no" {
		t.Errorf("race = %q, want no for a run without -race", props["race"])
	}

	raw, err := os.ReadFile(filepath.Join(dir, "executor.json"))
	if err != nil {
		t.Fatalf("executor.json: %v", err)
	}
	var executor struct {
		Name     string `json:"name"`
		Type     string `json:"type"`
		BuildURL string `json:"buildUrl"`
	}
	if err := json.Unmarshal(raw, &executor); err != nil {
		t.Fatalf("executor.json is not JSON: %v\n%s", err, raw)
	}
	if executor.BuildURL != runURL || executor.Type != "github" {
		t.Errorf("executor = %+v, want the Actions run link", executor)
	}
}

// TestSuiteScriptFeedsTheAllureFormatterToEveryFeaturesRun: the initial
// TestFeatures run and each solo rerun write native results into the same
// directory (same historyId, so the rerun is a retry), and -race reaches
// environment.properties.
func TestSuiteScriptFeedsTheAllureFormatterToEveryFeaturesRun(t *testing.T) {
	root, err := repositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(root, "ci", "suite.sh"))
	if err != nil {
		t.Fatal(err)
	}
	script := stripCommentLines(string(raw))
	calls := 0
	for _, line := range strings.Split(script, "\n") {
		if strings.Contains(line, "run_test_features ") && !strings.Contains(line, "run_test_features()") {
			calls++
			if !strings.Contains(line, `"DECK_GODOG_ALLURE=$allure_dir"`) {
				t.Errorf("a TestFeatures run does not set DECK_GODOG_ALLURE=$allure_dir: %s", strings.TrimSpace(line))
			}
		}
	}
	if calls != 2 {
		t.Errorf("found %d run_test_features calls, want 2 (the initial run and the solo rerun)", calls)
	}
	if !strings.Contains(script, `*' -race '*) race_mode=yes`) {
		t.Error("environment.properties no longer records race=yes for a -race run")
	}
}
