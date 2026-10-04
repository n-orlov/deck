package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// convertXML runs the converter over one junit file's content and returns
// the results it wrote (ordered by start) plus run's error.
func convertXML(t *testing.T, xml string) ([]map[string]any, error) {
	t.Helper()
	dir := t.TempDir()
	in := filepath.Join(dir, "junit-go.xml")
	if err := os.WriteFile(in, []byte(xml), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "results")
	err := run([]string{"-group", "unit", "-o", out, in}, &bytes.Buffer{})
	files, _ := filepath.Glob(filepath.Join(out, "*-result.json"))
	var all []map[string]any
	for _, f := range files {
		raw, readErr := os.ReadFile(f)
		if readErr != nil {
			t.Fatal(readErr)
		}
		var r map[string]any
		if jsonErr := json.Unmarshal(raw, &r); jsonErr != nil {
			t.Fatal(jsonErr)
		}
		all = append(all, r)
	}
	return all, err
}

func resultNamed(all []map[string]any, name string) []map[string]any {
	var out []map[string]any
	for _, r := range all {
		if r["name"] == name {
			out = append(out, r)
		}
	}
	return out
}

func labelOf(r map[string]any, name string) string {
	for _, l := range r["labels"].([]any) {
		m := l.(map[string]any)
		if m["name"] == name {
			return m["value"].(string)
		}
	}
	return ""
}

func TestMillisParsesSecondsAndIgnoresGarbage(t *testing.T) {
	for in, want := range map[string]int64{"0.250": 250, " 1.5 ": 1500, "": 0, "abc": 0, "-3": 0, "2": 2000} {
		if got := millis(in); got != want {
			t.Errorf("millis(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestDetailsOfFallsBackToTextAndNilStaysNil(t *testing.T) {
	if detailsOf(nil) != nil {
		t.Error("detailsOf(nil) must be nil: a passed test carries no status details")
	}
	d := detailsOf(&problem{Text: "  only a body \n"})
	if d == nil || d.Message != "only a body" || d.Trace != "only a body" {
		t.Errorf("detailsOf(text only) = %+v, want message and trace both 'only a body'", d)
	}
	d = detailsOf(&problem{Message: "short", Text: "long trace"})
	if d.Message != "short" || d.Trace != "long trace" {
		t.Errorf("detailsOf(message+text) = %+v", d)
	}
}

func TestRerunErrorAttemptsAreBrokenAndThePassingRetryIsFlaky(t *testing.T) {
	all, err := convertXML(t, `<testsuites><testsuite name="s">
		<testcase classname="p" name="TestRetried" time="0.1">
			<rerunError message="e1" type="panic">e1 body</rerunError>
			<rerunFailure message="f1" type="">f1 body</rerunFailure>
		</testcase></testsuite></testsuites>`)
	if err != nil {
		t.Fatal(err)
	}
	rs := resultNamed(all, "TestRetried")
	if len(rs) != 3 {
		t.Fatalf("want 3 results (two failed attempts + the final pass), got %d: %v", len(rs), rs)
	}
	statuses := map[string]int{}
	var final map[string]any
	for _, r := range rs {
		statuses[r["status"].(string)]++
		if r["status"] == "passed" {
			final = r
		}
	}
	if statuses["failed"] != 1 || statuses["broken"] != 1 || statuses["passed"] != 1 {
		t.Fatalf("statuses = %v, want one failed, one broken, one passed", statuses)
	}
	if d, _ := final["statusDetails"].(map[string]any); d == nil || d["flaky"] != true {
		t.Errorf("a pass after failed attempts must be flagged flaky; statusDetails = %v", final["statusDetails"])
	}
	hist := rs[0]["historyId"]
	for _, r := range rs {
		if r["historyId"] != hist {
			t.Errorf("attempts of one test must share a historyId: %v vs %v", r["historyId"], hist)
		}
	}
}

func TestSingleTestsuiteRootAndNestedSuitesAndUnnamedSuite(t *testing.T) {
	all, err := convertXML(t, `<testsuite>
		<testcase classname="p" name="TestTop"></testcase>
		<testsuite name="child"><testcase classname="p" name="TestChild"></testcase></testsuite>
	</testsuite>`)
	if err != nil {
		t.Fatal(err)
	}
	top, child := resultNamed(all, "TestTop"), resultNamed(all, "TestChild")
	if len(top) != 1 || len(child) != 1 {
		t.Fatalf("want one result each, got %d and %d (%v)", len(top), len(child), all)
	}
	if got := labelOf(top[0], "suite"); got != "junit-go.xml" {
		t.Errorf("an unnamed suite takes the input file's name, got %q", got)
	}
	if got := labelOf(child[0], "suite"); got != "child" {
		t.Errorf("nested suite label = %q, want child", got)
	}
	if got := labelOf(child[0], "parentSuite"); got != "unit" {
		t.Errorf("parentSuite label = %q, want the -group value 'unit'", got)
	}
}

func TestSkippedAndErroredCasesCarryDetails(t *testing.T) {
	all, err := convertXML(t, `<testsuites><testsuite name="s">
		<testcase classname="p" name="TestSkip"><skipped message="why"></skipped></testcase>
		<testcase classname="p" name="TestErr"><error message="bad" type="t">bad body</error></testcase>
	</testsuite></testsuites>`)
	if err != nil {
		t.Fatal(err)
	}
	if s := resultNamed(all, "TestSkip"); len(s) != 1 || s[0]["status"] != "skipped" {
		t.Errorf("TestSkip = %v, want one skipped result", s)
	}
	e := resultNamed(all, "TestErr")
	if len(e) != 1 || e[0]["status"] != "broken" || e[0]["statusDetails"].(map[string]any)["message"] != "bad" {
		t.Errorf("TestErr = %v, want one broken result with message 'bad'", e)
	}
}

func TestRunRejectsBadInvocationsAndInputs(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good.xml")
	if err := os.WriteFile(good, []byte(`<testsuites><testsuite name="s"><testcase classname="p" name="T"></testcase></testsuite></testsuites>`), 0o644); err != nil {
		t.Fatal(err)
	}
	empty := filepath.Join(dir, "empty.xml")
	if err := os.WriteFile(empty, []byte(`<testsuites></testsuites>`), 0o644); err != nil {
		t.Fatal(err)
	}
	garbage := filepath.Join(dir, "garbage.xml")
	if err := os.WriteFile(garbage, []byte(`this is not xml`), 0o644); err != nil {
		t.Fatal(err)
	}
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out")
	cases := map[string]struct {
		args []string
		want string
	}{
		"no flags":             {nil, "usage"},
		"no group":             {[]string{"-o", out, good}, "usage"},
		"no input":             {[]string{"-group", "unit", "-o", out}, "usage"},
		"unknown flag":         {[]string{"-nope"}, "flag provided but not defined"},
		"missing input":        {[]string{"-group", "unit", "-o", out, filepath.Join(dir, "absent.xml")}, "absent.xml"},
		"not xml":              {[]string{"-group", "unit", "-o", out, garbage}, "garbage.xml"},
		"no testcase anywhere": {[]string{"-group", "unit", "-o", out, empty}, "no <testcase>"},
		"output dir is a file": {[]string{"-group", "unit", "-o", filepath.Join(blocker, "sub"), good}, "not a directory"},
	}
	for name, c := range cases {
		var stderr bytes.Buffer
		err := run(c.args, &stderr)
		if err == nil || !strings.Contains(err.Error()+stderr.String(), c.want) {
			t.Errorf("%s: err = %v (stderr %q), want an error mentioning %q", name, err, stderr.String(), c.want)
		}
	}
}

func TestRunReportsAWriteFailure(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "j.xml")
	if err := os.WriteFile(in, []byte(`<testsuites><testsuite name="s"><testcase classname="p" name="T"></testcase></testsuite></testsuites>`), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "ro")
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(out, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(out, 0o755) })
	if f, err := os.Create(filepath.Join(out, "probe")); err == nil {
		f.Close()
		t.Skip("the process can write into a 0500 directory (running as root); the write-failure path cannot be provoked")
	}
	if err := run([]string{"-group", "unit", "-o", out, in}, &bytes.Buffer{}); err == nil {
		t.Fatal("run into a read-only output directory must fail, not report success")
	}
}
