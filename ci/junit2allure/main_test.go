package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const fixture = `<?xml version="1.0" encoding="UTF-8"?>
<testsuites tests="5" failures="1" errors="0" time="1">
	<testsuite name="example.com/m/a" tests="5" failures="1" errors="0" time="1">
		<testcase classname="example.com/m/a" name="TestPasses" time="0.250"></testcase>
		<testcase classname="example.com/m/a" name="TestFails" time="0.1">
			<failure message="boom" type="">trace of the boom</failure>
		</testcase>
		<testcase classname="example.com/m/a" name="TestSkipped" time="0">
			<skipped message="not here"></skipped>
		</testcase>
		<testcase classname="example.com/m/a" name="TestFlaky" time="0.2">
			<rerunFailure message="first try" type="">first try body</rerunFailure>
		</testcase>
		<testcase classname="example.com/m/a" name="TestBroken" time="0">
			<error message="oops" type="panic">panic body</error>
		</testcase>
	</testsuite>
</testsuites>
`

func convertFixture(t *testing.T) []map[string]any {
	t.Helper()
	dir := t.TempDir()
	in := filepath.Join(dir, "junit.xml")
	if err := os.WriteFile(in, []byte(fixture), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "results")
	if err := run([]string{"-group", "unit", "-o", out, in}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob(filepath.Join(out, "*-result.json"))
	if err != nil {
		t.Fatal(err)
	}
	var all []map[string]any
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var r map[string]any
		if err := json.Unmarshal(raw, &r); err != nil {
			t.Fatal(err)
		}
		all = append(all, r)
	}
	sort.Slice(all, func(i, j int) bool { return all[i]["start"].(float64) < all[j]["start"].(float64) })
	return all
}

func TestEveryTestCaseBecomesAResultInTheUnitGroup(t *testing.T) {
	all := convertFixture(t)
	if len(all) != 6 {
		t.Fatalf("got %d results, want 6 (5 test cases + the flaky test's earlier attempt)", len(all))
	}
	want := map[string]string{"TestPasses": "passed", "TestFails": "failed", "TestSkipped": "skipped", "TestBroken": "broken"}
	for _, r := range all {
		parent := ""
		for _, l := range r["labels"].([]any) {
			m := l.(map[string]any)
			if m["name"] == "parentSuite" {
				parent = m["value"].(string)
			}
		}
		if parent != "unit" {
			t.Fatalf("%v: parentSuite = %q, want unit", r["name"], parent)
		}
		if w, ok := want[r["name"].(string)]; ok && r["status"] != w {
			t.Fatalf("%v: status %v, want %v", r["name"], r["status"], w)
		}
		if r["name"] == "TestFails" {
			d := r["statusDetails"].(map[string]any)
			if d["message"] != "boom" || !strings.Contains(d["trace"].(string), "trace of the boom") {
				t.Fatalf("failure details = %v", d)
			}
		}
	}
}

func TestAFlakyTestIsOneHistoryIDWithAFailedAttemptBeforeThePassingOne(t *testing.T) {
	var flaky []map[string]any
	for _, r := range convertFixture(t) {
		if r["name"] == "TestFlaky" {
			flaky = append(flaky, r)
		}
	}
	if len(flaky) != 2 {
		t.Fatalf("got %d TestFlaky results, want 2", len(flaky))
	}
	if flaky[0]["historyId"] != flaky[1]["historyId"] {
		t.Fatalf("historyIds differ: %v vs %v", flaky[0]["historyId"], flaky[1]["historyId"])
	}
	if flaky[0]["historyId"] != "example.com/m/a:example.com/m/a#TestFlaky" {
		t.Fatalf("historyId = %v, want the JUnit plugin's suite:classname#name so trends carry over", flaky[0]["historyId"])
	}
	if flaky[0]["status"] != "failed" || flaky[1]["status"] != "passed" {
		t.Fatalf("attempt order = %v then %v, want failed then passed", flaky[0]["status"], flaky[1]["status"])
	}
	if d, _ := flaky[1]["statusDetails"].(map[string]any); d["flaky"] != true {
		t.Fatalf("passing attempt statusDetails = %v, want flaky:true (the JUnit plugin's convention)", flaky[1]["statusDetails"])
	}
	if flaky[0]["uuid"] == flaky[1]["uuid"] {
		t.Fatal("two attempts share a uuid")
	}
}

func TestAnInputWithoutTestCasesIsAnError(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "empty.xml")
	if err := os.WriteFile(in, []byte(`<testsuites tests="0"></testsuites>`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"-group", "unit", "-o", filepath.Join(dir, "r"), in}, &bytes.Buffer{}); err == nil {
		t.Fatal("want an error for an input holding no test cases")
	}
}
