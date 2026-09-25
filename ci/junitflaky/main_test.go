package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mustParse(t *testing.T, name, body string) *node {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	n, err := parse(p)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	return n
}

func render(t *testing.T, n *node) string {
	t.Helper()
	var buf bytes.Buffer
	if err := write(&buf, n); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func testcases(root *node, suite string) []*node {
	var out []*node
	for _, s := range root.Nodes {
		if s.XMLName.Local != "testsuite" || s.attr("name") != suite {
			continue
		}
		for _, c := range s.Nodes {
			if c.XMLName.Local == "testcase" {
				out = append(out, c)
			}
		}
	}
	return out
}

func named(tcs []*node, name string) []*node {
	var out []*node
	for _, tc := range tcs {
		if tc.attr("name") == name {
			out = append(out, tc)
		}
	}
	return out
}

// gotestsum --rerun-fails appends the rerun to the same testsuite, after
// the original failed attempt: the merged file must hold ONE passing
// testcase for it carrying the original failure as <rerunFailure>, which
// is what Allure's junit-xml plugin reports as passed and flaky.
func TestFoldsGotestsumSameFileRerunIntoOnePassingFlakyTestcase(t *testing.T) {
	in := mustParse(t, "junit-go.xml", `<?xml version="1.0" encoding="UTF-8"?>
<testsuites tests="4" failures="1" errors="0" time="1.0">
	<testsuite tests="4" failures="1" time="0.5" name="example.com/p" timestamp="2026-09-25T23:03:36Z">
		<properties><property name="go.version" value="go1.25"></property></properties>
		<testcase classname="example.com/p" name="TestStable" time="0.1"></testcase>
		<testcase classname="example.com/p" name="TestFlaky" time="0.0">
			<failure message="Failed" type="">first-run failure body</failure>
		</testcase>
		<testcase classname="example.com/p" name="TestOther" time="0.1"></testcase>
		<testcase classname="example.com/p" name="TestFlaky" time="0.2"></testcase>
	</testsuite>
</testsuites>`)
	out := merge([]*node{in})
	tcs := testcases(out, "example.com/p")
	if len(tcs) != 3 {
		t.Fatalf("merged suite has %d testcases, want 3 (the rerun folded into the original):\n%s", len(tcs), render(t, out))
	}
	flaky := named(tcs, "TestFlaky")
	if len(flaky) != 1 {
		t.Fatalf("TestFlaky appears %d times, want exactly once", len(flaky))
	}
	tc := flaky[0]
	if outcomeOf(tc) != passed {
		t.Fatalf("TestFlaky outcome = %v, want passed:\n%s", outcomeOf(tc), render(t, out))
	}
	if tc.child("failure") != nil {
		t.Fatalf("TestFlaky still carries a <failure>, Allure would report it failed")
	}
	rf := tc.child("rerunFailure")
	if rf == nil {
		t.Fatalf("TestFlaky has no <rerunFailure>, Allure would not mark it flaky:\n%s", render(t, out))
	}
	if rf.attr("message") != "Failed" || !strings.Contains(rf.Text, "first-run failure body") {
		t.Fatalf("rerunFailure lost the first attempt's message/body: message=%q text=%q", rf.attr("message"), rf.Text)
	}
	if tc.attr("time") != "0.2" {
		t.Fatalf("TestFlaky time = %q, want the passing attempt's own 0.2", tc.attr("time"))
	}
	if tcs[1].attr("name") != "TestFlaky" {
		t.Fatalf("the folded testcase moved: position 1 is %q, want TestFlaky kept at its first occurrence", tcs[1].attr("name"))
	}
	if got := out.attr("failures"); got != "0" {
		t.Fatalf("root failures = %q, want 0 after the fold", got)
	}
	if got := out.attr("tests"); got != "3" {
		t.Fatalf("root tests = %q, want 3", got)
	}
	if s := out.Nodes[0]; s.attr("failures") != "0" || s.attr("tests") != "3" || s.child("properties") == nil {
		t.Fatalf("suite counts/properties wrong: tests=%q failures=%q properties=%v", s.attr("tests"), s.attr("failures"), s.child("properties") != nil)
	}
}

// ci/suite.sh's per-scenario features/ rerun writes its own godog JUnit
// file; the merged file keeps the original's layout and folds the rerun's
// passing scenario into the original failed one.
func TestFoldsGodogRerunFileIntoTheOriginalScenario(t *testing.T) {
	first := mustParse(t, "junit-features.xml", `<?xml version="1.0" encoding="UTF-8"?>
<testsuites name="" tests="3" skipped="0" failures="1" errors="0" time="3.0">
  <testsuite name="Feature A" tests="1" skipped="0" failures="0" errors="0" time="1.0">
    <testcase name="a passes" status="passed" time="1.0"></testcase>
  </testsuite>
  <testsuite name="CI flaky" tests="2" skipped="0" failures="1" errors="0" time="2.0">
    <testcase name="fail once then pass" status="failed" time="0.4">
      <failure message="Step x: intentional first-run failure"></failure>
    </testcase>
    <testcase name="stays green" status="passed" time="1.6"></testcase>
  </testsuite>
</testsuites>`)
	rerun := mustParse(t, "junit-features-rerun-1.xml", `<?xml version="1.0" encoding="UTF-8"?>
<testsuites name="" tests="1" skipped="0" failures="0" errors="0" time="0.5">
  <testsuite name="CI flaky" tests="1" skipped="0" failures="0" errors="0" time="0.5">
    <testcase name="fail once then pass" status="passed" time="0.5"></testcase>
  </testsuite>
</testsuites>`)
	out := merge([]*node{first, rerun})
	tcs := testcases(out, "CI flaky")
	if len(tcs) != 2 {
		t.Fatalf("CI flaky suite has %d testcases, want 2:\n%s", len(tcs), render(t, out))
	}
	tc := named(tcs, "fail once then pass")
	if len(tc) != 1 {
		t.Fatalf("scenario appears %d times, want once:\n%s", len(tc), render(t, out))
	}
	if outcomeOf(tc[0]) != passed || tc[0].attr("status") != "passed" {
		t.Fatalf("scenario outcome=%v status=%q, want passed", outcomeOf(tc[0]), tc[0].attr("status"))
	}
	rf := tc[0].child("rerunFailure")
	if rf == nil || rf.attr("message") != "Step x: intentional first-run failure" {
		t.Fatalf("scenario lacks the first attempt as <rerunFailure>:\n%s", render(t, out))
	}
	if len(testcases(out, "Feature A")) != 1 {
		t.Fatalf("an untouched feature lost its scenario")
	}
	if out.attr("failures") != "0" || out.attr("tests") != "3" || out.attr("skipped") != "0" {
		t.Fatalf("root counts tests=%q failures=%q skipped=%q, want 3/0/0", out.attr("tests"), out.attr("failures"), out.attr("skipped"))
	}
}

// A test that failed on every attempt is a real failure, not a flake: it
// must stay failed, keep every attempt, and carry no rerunFailure (which
// Allure would turn into a flaky mark).
func TestLeavesATestThatFailedEveryAttemptFailedAndNotFlaky(t *testing.T) {
	first := mustParse(t, "junit-features.xml", `<testsuites tests="1" skipped="0" failures="1" errors="0">
  <testsuite name="F" tests="1" skipped="0" failures="1" errors="0">
    <testcase name="always red" status="failed"><failure message="boom 1"></failure></testcase>
  </testsuite>
</testsuites>`)
	rerun := mustParse(t, "junit-features-rerun-1.xml", `<testsuites tests="1" skipped="0" failures="1" errors="0">
  <testsuite name="F" tests="1" skipped="0" failures="1" errors="0">
    <testcase name="always red" status="failed"><failure message="boom 2"></failure></testcase>
  </testsuite>
</testsuites>`)
	out := merge([]*node{first, rerun})
	tcs := named(testcases(out, "F"), "always red")
	if len(tcs) != 2 {
		t.Fatalf("got %d attempts, want both failures kept:\n%s", len(tcs), render(t, out))
	}
	for _, tc := range tcs {
		if outcomeOf(tc) != failed {
			t.Fatalf("an attempt is %v, want failed", outcomeOf(tc))
		}
		if tc.child("rerunFailure") != nil || tc.child("rerunError") != nil {
			t.Fatalf("a hard failure carries a rerun element, Allure would call it flaky:\n%s", render(t, out))
		}
	}
	if out.attr("failures") != "2" {
		t.Fatalf("root failures = %q, want 2", out.attr("failures"))
	}
}

// An earlier <error> attempt folds as <rerunError>, the plugin's other
// flaky marker.
func TestFoldsAnEarlierErrorAsRerunError(t *testing.T) {
	in := mustParse(t, "j.xml", `<testsuite name="s" tests="2" failures="0" errors="1">
  <testcase classname="c" name="T"><error message="panic" type="E">trace</error></testcase>
  <testcase classname="c" name="T"></testcase>
</testsuite>`)
	out := merge([]*node{in})
	tcs := testcases(out, "s")
	if len(tcs) != 1 {
		t.Fatalf("got %d testcases, want 1:\n%s", len(tcs), render(t, out))
	}
	re := tcs[0].child("rerunError")
	if re == nil || re.attr("message") != "panic" || re.attr("type") != "E" || re.Text != "trace" {
		t.Fatalf("missing/incomplete <rerunError>:\n%s", render(t, out))
	}
	if out.attr("errors") != "0" {
		t.Fatalf("root errors = %q, want 0", out.attr("errors"))
	}
}

// End to end through run(): the written file re-parses as JUnit and holds
// the folded testcase.
func TestRunWritesAReparseableMergedFile(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.xml")
	if err := os.WriteFile(in, []byte(`<testsuites><testsuite name="s">
<testcase classname="c" name="T"><failure message="m">b &amp; c</failure></testcase>
<testcase classname="c" name="T"></testcase>
</testsuite></testsuites>`), 0o644); err != nil {
		t.Fatal(err)
	}
	outPath := filepath.Join(dir, "out.xml")
	var stderr bytes.Buffer
	if err := run([]string{"-o", outPath, in}, &stderr); err != nil {
		t.Fatalf("run: %v (%s)", err, stderr.String())
	}
	back, err := parse(outPath)
	if err != nil {
		t.Fatalf("merged file does not re-parse: %v", err)
	}
	tcs := testcases(back, "s")
	if len(tcs) != 1 || tcs[0].child("rerunFailure") == nil || tcs[0].child("rerunFailure").Text != "b & c" {
		data, _ := os.ReadFile(outPath)
		t.Fatalf("re-parsed merged file wrong:\n%s", data)
	}
	if err := run([]string{in}, &stderr); err == nil {
		t.Fatalf("run without -o succeeded, want a usage error")
	}
}
