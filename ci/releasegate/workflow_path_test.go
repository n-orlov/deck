package main

import (
	"fmt"
	"strings"
	"testing"
)

// suiteCheckFrom builds a check-runs body with one green "suite" check run in
// check_suite 7, and the actions-runs body saying which workflow file (path)
// and event produced that suite.
func suiteCheckFrom(path, event string) (checkRuns, actionsRuns []byte) {
	checkRuns = []byte(`{"total_count":1,"check_runs":[{"name":"suite","status":"completed","conclusion":"success","started_at":"2026-01-01T10:00:00Z","check_suite":{"id":7}}]}`)
	actionsRuns = []byte(fmt.Sprintf(`{"workflow_runs":[{"event":%q,"check_suite_id":7,"path":%q}]}`, event, path))
	return checkRuns, actionsRuns
}

// TestEvaluate_SuiteCheckFromAnotherWorkflowDoesNotGate: a green "suite"
// check published by pages-pr-publish.yml (R205, #61) never lets a release
// through, even on a push event.
func TestEvaluate_SuiteCheckFromAnotherWorkflowDoesNotGate(t *testing.T) {
	for _, event := range []string{"push", "pull_request"} {
		checks, runs := suiteCheckFrom(".github/workflows/pages-pr-publish.yml", event)
		err := evaluate(checks, runs, shaUnderTest, "suite")
		if err == nil {
			t.Fatalf("%s: a green suite check from pages-pr-publish.yml passed the gate", event)
		}
		if !strings.Contains(err.Error(), "ci.yml") || !strings.Contains(err.Error(), shaUnderTest) {
			t.Errorf("%s: error %q does not name the sha and ci.yml", event, err)
		}
	}
}

// TestEvaluate_SuiteCheckFromCIWorkflowGates: the same check from ci.yml
// passes, whether the path is bare or carries GitHub's "@<ref>" suffix.
func TestEvaluate_SuiteCheckFromCIWorkflowGates(t *testing.T) {
	for _, path := range []string{".github/workflows/ci.yml", ".github/workflows/ci.yml@refs/pull/9/merge"} {
		checks, runs := suiteCheckFrom(path, "push")
		if err := evaluate(checks, runs, shaUnderTest, "suite"); err != nil {
			t.Errorf("path %q: green suite check from ci.yml refused: %v", path, err)
		}
	}
}

// TestEvaluate_OtherWorkflowGreenDoesNotMaskCIFailure: ci.yml's own red suite
// still refuses when another workflow publishes a later green "suite".
func TestEvaluate_OtherWorkflowGreenDoesNotMaskCIFailure(t *testing.T) {
	checks := []byte(`{"check_runs":[
		{"name":"suite","status":"completed","conclusion":"failure","started_at":"2026-01-01T10:00:00Z","check_suite":{"id":1}},
		{"name":"suite","status":"completed","conclusion":"success","started_at":"2026-01-01T11:00:00Z","check_suite":{"id":2}}]}`)
	runs := []byte(`{"workflow_runs":[
		{"event":"push","check_suite_id":1,"path":".github/workflows/ci.yml"},
		{"event":"push","check_suite_id":2,"path":".github/workflows/pages-pr-publish.yml"}]}`)
	err := evaluate(checks, runs, shaUnderTest, "suite")
	if err == nil || !strings.Contains(err.Error(), "failure") {
		t.Fatalf("got %v, want refusal naming the ci.yml failure", err)
	}
}

// runGateOverHTTP drives the production run() path against a fake GitHub
// server whose single green "suite" check run (check_suite 7) is associated
// with the given raw workflow-run JSON object text.
func runGateOverHTTP(t *testing.T, workflowRun string) error {
	t.Helper()
	checks := `{"total_count":1,"check_runs":[{"name":"suite","status":"completed","conclusion":"success","started_at":"2026-01-01T10:00:00Z","check_suite":{"id":7}}]}`
	s := newPageServer(t, []string{checks}, []string{`{"workflow_runs":[` + workflowRun + `]}`})
	withAPIBase(t, s.baseURL)
	return run([]string{"-repo", "o/r", "-sha", "deadbeef"}, noTokenEnv)
}

// TestRun_UnidentifiableWorkflowPathNeverGates (R205): a run whose workflow
// path is missing, null, empty or anything but ci.yml never lets a green
// suite check through, for either gating event.
func TestRun_UnidentifiableWorkflowPathNeverGates(t *testing.T) {
	paths := map[string]string{
		"absent":              ``,
		"null":                `,"path":null`,
		"empty":               `,"path":""`,
		"only a ref suffix":   `,"path":"@refs/heads/main"`,
		"bare file name":      `,"path":"ci.yml"`,
		"suffix on file name": `,"path":".github/workflows/ci.yml.bak"`,
		"prefixed file name":  `,"path":".github/workflows/xci.yml"`,
		"different directory": `,"path":"other/.github/workflows/ci.yml"`,
		"different case":      `,"path":".github/workflows/CI.yml"`,
		"surrounding space":   `,"path":" .github/workflows/ci.yml"`,
		"yaml extension":      `,"path":".github/workflows/ci.yaml"`,
		"another workflow":    `,"path":".github/workflows/pages-pr-publish.yml"`,
		"ci.yml in later ref": `,"path":".github/workflows/pages-pr-publish.yml@.github/workflows/ci.yml"`,
	}
	for name, pathField := range paths {
		for _, event := range []string{"push", "pull_request"} {
			t.Run(name+"/"+event, func(t *testing.T) {
				err := runGateOverHTTP(t, fmt.Sprintf(`{"event":%q,"check_suite_id":7%s}`, event, pathField))
				if err == nil {
					t.Fatalf("a green suite check whose workflow path is %s passed the gate", name)
				}
				if !strings.Contains(err.Error(), "ci.yml") {
					t.Errorf("error %q does not name ci.yml", err)
				}
			})
		}
	}
}

// TestRun_CIWorkflowPathGatesOverHTTP: the positive control for the cases
// above, with the documented "@<ref>" suffix, over the same HTTP path.
func TestRun_CIWorkflowPathGatesOverHTTP(t *testing.T) {
	for _, path := range []string{".github/workflows/ci.yml", ".github/workflows/ci.yml@refs/heads/main", ".github/workflows/ci.yml@refs/pull/9/merge"} {
		for _, event := range []string{"push", "pull_request"} {
			if err := runGateOverHTTP(t, fmt.Sprintf(`{"event":%q,"check_suite_id":7,"path":%q}`, event, path)); err != nil {
				t.Errorf("%s %q: green ci.yml suite refused: %v", event, path, err)
			}
		}
	}
}

// TestRun_PathlessRunDoesNotMaskIdentifiedFailure: a pathless later green
// suite never hides ci.yml's own red one.
func TestRun_PathlessRunDoesNotMaskIdentifiedFailure(t *testing.T) {
	checks := `{"check_runs":[
		{"name":"suite","status":"completed","conclusion":"failure","started_at":"2026-01-01T10:00:00Z","check_suite":{"id":1}},
		{"name":"suite","status":"completed","conclusion":"success","started_at":"2026-01-01T11:00:00Z","check_suite":{"id":2}}]}`
	runs := `{"workflow_runs":[{"event":"push","check_suite_id":1,"path":".github/workflows/ci.yml"},{"event":"push","check_suite_id":2}]}`
	s := newPageServer(t, []string{checks}, []string{runs})
	withAPIBase(t, s.baseURL)
	err := run([]string{"-repo", "o/r", "-sha", "deadbeef"}, noTokenEnv)
	if err == nil || !strings.Contains(err.Error(), "failure") {
		t.Fatalf("got %v, want refusal naming the ci.yml failure", err)
	}
}
