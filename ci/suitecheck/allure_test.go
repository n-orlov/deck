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

// counts is one side's view of a run: Allure statuses folded the way the job
// summary reports them (failed and broken are both "fail").
type counts struct{ pass, fail, skip, flaky, other int }

func (c counts) String() string {
	return fmt.Sprintf("pass=%d fail=%d skip=%d flaky=%d other=%d", c.pass, c.fail, c.skip, c.flaky, c.other)
}

// allureCounts counts the way the Allure report's own summary widget does: one
// test per historyId, its status the latest result's, flaky when the latest
// passed after an earlier attempt that did not. Each status is counted as
// itself -- never derived from a total minus the failures.
func allureCounts(results []allureResult) counts {
	byHistory := map[string][]allureResult{}
	for _, r := range results {
		byHistory[r.HistoryID] = append(byHistory[r.HistoryID], r)
	}
	var c counts
	for _, attempts := range byHistory {
		sort.Slice(attempts, func(i, j int) bool { return attempts[i].Start < attempts[j].Start })
		switch attempts[len(attempts)-1].Status {
		case "passed":
			c.pass++
			if len(attempts) > 1 {
				c.flaky++
			}
		case "failed", "broken":
			c.fail++
		case "skipped":
			c.skip++
		default:
			c.other++
		}
	}
	return c
}

var summaryRow = regexp.MustCompile(`(?m)^\| (pass|fail|skip|flaky) \| (\d+) \|$`)

// summaryCounts runs the real ci/summary.sh over a results directory.
func summaryCounts(t *testing.T, root, outdir string) counts {
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
	if len(got) != 4 {
		t.Fatalf("summary has no pass/fail/skip/flaky rows:\n%s", out)
	}
	return counts{pass: got["pass"], fail: got["fail"], skip: got["skip"], flaky: got["flaky"]}
}

// summaryAllureDisagreement returns "" when ci/summary.sh and the Allure
// results of the same directory report the same pass, fail, skip and flaky
// counts, row by row.
func summaryAllureDisagreement(t *testing.T, root, outdir string) string {
	t.Helper()
	summary := summaryCounts(t, root, outdir)
	allure := allureCounts(readAllureResults(t, filepath.Join(outdir, "allure-results")))
	if summary != allure {
		return fmt.Sprintf("summary says %v; Allure results say %v", summary, allure)
	}
	return ""
}

const agreementUnitJUnit = `<?xml version="1.0" encoding="UTF-8"?>
<testsuites tests="6" failures="2" errors="0" time="1">
	<testsuite name="example.com/m/a" tests="6" failures="2" errors="0" time="1">
		<testcase classname="example.com/m/a" name="TestOne" time="0.1"></testcase>
		<testcase classname="example.com/m/a" name="TestTwo" time="0.1"></testcase>
		<testcase classname="example.com/m/a" name="TestSkipped" time="0"><skipped message="skip"></skipped></testcase>
		<testcase classname="example.com/m/a" name="TestFlaky" time="0.2"><rerunFailure message="first" type="">first</rerunFailure></testcase>
		<testcase classname="example.com/m/a" name="TestFails" time="0.1"><failure message="boom" type="">boom</failure></testcase>
		<testcase classname="example.com/m/a" name="TestFails" time="0.1"><failure message="boom again" type="">boom again</failure></testcase>
	</testsuite>
</testsuites>
`

const agreementFeaturesJUnit = `<?xml version="1.0" encoding="UTF-8"?>
<testsuites tests="4" skipped="0" failures="2" errors="0" time="1">
	<testsuite name="Fixture" tests="4" skipped="0" failures="2" errors="0" time="1">
		<testcase name="steady" status="passed" time="0.1"></testcase>
		<testcase name="wobbly" status="passed" time="0.1"><rerunFailure message="first" type="">first</rerunFailure></testcase>
		<testcase name="broken" status="failed" time="0.1"><failure message="Step boom"></failure><error message="Step after" type="skipped"></error></testcase>
		<testcase name="broken" status="failed" time="0.1"><failure message="Step boom"></failure><error message="Step after" type="skipped"></error></testcase>
	</testsuite>
</testsuites>
`

// rewriteMergedJUnit replaces the fixture's merged Go JUnit file under outdir.
// The write goes through an os.Root, so the name can only ever resolve inside
// the fixture directory.
func rewriteMergedJUnit(outdir, body string) error {
	scope, err := os.OpenRoot(outdir)
	if err != nil {
		return err
	}
	defer func() { _ = scope.Close() }() // nothing is read back through this handle
	return scope.WriteFile("junit-merged/junit-go.xml", []byte(body), 0o600)
}

// writeAgreementFixture builds a results directory the way ci/suite.sh leaves
// one: merged JUnit and flaky lists for ci/summary.sh, and allure-results/
// with the unit tests converted by the real ci/junit2allure and features/'s
// native results. Each group holds a pass, a test that failed then passed on
// retry, and a test that failed on both attempts (two JUnit testcases, two
// Allure results under one historyId); the unit group also holds a skip.
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
		{"f5", "Fixture:broken", "failed", 50},
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
// ci/summary.sh's pass/fail/skip/flaky counts and the Allure results come
// from the same tests: on a fixture holding passes, a skip, retried-then-passed
// tests and tests that failed on every attempt in both groups the two agree
// row by row, and they stop agreeing the moment a result goes missing from one
// side.
func TestSummaryCountsAgreeWithTheAllureResults(t *testing.T) {
	root, err := repositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	outdir := writeAgreementFixture(t, root)

	want := counts{pass: 5, fail: 2, skip: 1, flaky: 2}
	if got := summaryCounts(t, root, outdir); got != want {
		t.Fatalf("summary of the fixture = %v, want %v", got, want)
	}
	if got := allureCounts(readAllureResults(t, filepath.Join(outdir, "allure-results"))); got != want {
		t.Fatalf("Allure results of the fixture = %v, want %v", got, want)
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
	for _, f := range []string{"f4-result.json", "f5-result.json"} {
		if err := os.Remove(filepath.Join(outdir, "allure-results", f)); err != nil {
			t.Fatal(err)
		}
	}
	if diff := summaryAllureDisagreement(t, root, outdir); diff == "" {
		t.Fatal("a missing Allure result went unnoticed: summary and report would disagree silently")
	}
}

// TestSummaryAllureCheckSeesASkipCountedAsAPass is the shape push run
// 37164009078 had: one skipped unit test that Allure reported as skipped while
// the summary's pass row counted it as a pass. With the skip gone from the
// JUnit side only, the pass and skip rows disagree and the check says so.
func TestSummaryAllureCheckSeesASkipCountedAsAPass(t *testing.T) {
	root, err := repositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	outdir := writeAgreementFixture(t, root)
	junit := filepath.Join(outdir, "junit-merged", "junit-go.xml")
	raw, err := os.ReadFile(junit)
	if err != nil {
		t.Fatal(err)
	}
	stripped := strings.Replace(string(raw), `<skipped message="skip"></skipped>`, "", 1)
	if stripped == string(raw) {
		t.Fatal("fixture has no <skipped> element to strip")
	}
	if err := rewriteMergedJUnit(outdir, stripped); err != nil {
		t.Fatal(err)
	}
	diff := summaryAllureDisagreement(t, root, outdir)
	if !strings.Contains(diff, "summary says pass=6 ") || !strings.Contains(diff, "Allure results say pass=5 ") {
		t.Fatalf("disagreement = %q, want summary pass=6 against Allure pass=5", diff)
	}
}

// TestSummaryAllureCheckSeesAFailurePerAttempt: a test that failed on every
// attempt is one failed test in Allure (one historyId), so a third failed
// attempt of TestFails must leave both sides at the same fail count -- a
// summary counting one failure per JUnit testcase would say 3 -- while a
// genuinely different failed test present only on the JUnit side is flagged.
func TestSummaryAllureCheckSeesAFailurePerAttempt(t *testing.T) {
	root, err := repositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	outdir := writeAgreementFixture(t, root)
	junit := filepath.Join(outdir, "junit-merged", "junit-go.xml")
	raw, err := os.ReadFile(junit)
	if err != nil {
		t.Fatal(err)
	}
	const attempt = `<testcase classname="example.com/m/a" name="TestFails" time="0.1"><failure message="boom" type="">boom</failure></testcase>`
	if !strings.Contains(string(raw), attempt) {
		t.Fatal("fixture has no TestFails attempt to repeat")
	}
	third := strings.Replace(string(raw), attempt, attempt+attempt, 1)
	if err := rewriteMergedJUnit(outdir, third); err != nil {
		t.Fatal(err)
	}
	if diff := summaryAllureDisagreement(t, root, outdir); diff != "" {
		t.Fatalf("a third failed attempt of one test must stay one failure on both sides: %s", diff)
	}
	other := strings.Replace(string(raw), attempt, attempt+strings.Replace(attempt, "TestFails", "TestFailsToo", 1), 1)
	if err := rewriteMergedJUnit(outdir, other); err != nil {
		t.Fatal(err)
	}
	if diff := summaryAllureDisagreement(t, root, outdir); !strings.Contains(diff, "summary says pass=5 fail=3 ") {
		t.Fatalf("a failed test missing from the Allure side went unnoticed: %q", diff)
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
