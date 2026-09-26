package main

import (
	"strings"
	"testing"
)

const shaUnderTest = "deadbeefcafef00dfeedface1234567890abcde"

// TestEvaluate_NoRun asserts refusal when the check-runs API reports no
// run for the "suite" check at all (an empty check_runs list) -- the sha
// simply has no CI result yet.
func TestEvaluate_NoRun(t *testing.T) {
	body := []byte(`{"total_count":0,"check_runs":[]}`)

	err := evaluate(body, shaUnderTest, "suite")
	if err == nil {
		t.Fatalf("evaluate: expected refusal for no matching check run, got nil")
	}
	if !strings.Contains(err.Error(), shaUnderTest) {
		t.Errorf("evaluate error %q does not name the sha %q", err.Error(), shaUnderTest)
	}
	if !strings.Contains(err.Error(), "no") || !strings.Contains(err.Error(), `"suite"`) {
		t.Errorf("evaluate error %q does not say what it found (no matching \"suite\" run)", err.Error())
	}
}

// TestEvaluate_Failure asserts refusal when the "suite" check completed
// with conclusion "failure".
func TestEvaluate_Failure(t *testing.T) {
	body := []byte(`{
		"total_count": 1,
		"check_runs": [
			{"name": "suite", "status": "completed", "conclusion": "failure", "started_at": "2026-01-01T00:00:00Z"}
		]
	}`)

	err := evaluate(body, shaUnderTest, "suite")
	if err == nil {
		t.Fatalf("evaluate: expected refusal for conclusion=failure, got nil")
	}
	if !strings.Contains(err.Error(), shaUnderTest) {
		t.Errorf("evaluate error %q does not name the sha %q", err.Error(), shaUnderTest)
	}
	if !strings.Contains(err.Error(), "failure") {
		t.Errorf("evaluate error %q does not name the failure conclusion it found", err.Error())
	}
}

// TestEvaluate_InProgress asserts refusal when the "suite" check has
// started but not yet completed (status=in_progress, conclusion=null).
func TestEvaluate_InProgress(t *testing.T) {
	body := []byte(`{
		"total_count": 1,
		"check_runs": [
			{"name": "suite", "status": "in_progress", "conclusion": null, "started_at": "2026-01-01T00:00:00Z"}
		]
	}`)

	err := evaluate(body, shaUnderTest, "suite")
	if err == nil {
		t.Fatalf("evaluate: expected refusal for status=in_progress, got nil")
	}
	if !strings.Contains(err.Error(), shaUnderTest) {
		t.Errorf("evaluate error %q does not name the sha %q", err.Error(), shaUnderTest)
	}
	if !strings.Contains(err.Error(), "in_progress") {
		t.Errorf("evaluate error %q does not name the in_progress status it found", err.Error())
	}
}

// TestEvaluate_Success asserts a pass when the "suite" check's most
// recently started run completed with conclusion "success", even amid
// other, older or differently-named check runs (an earlier failed attempt
// on the same sha, plus an unrelated "lint" check) -- evaluate must pick
// the latest "suite" run by started_at, not merely the first one it sees.
func TestEvaluate_Success(t *testing.T) {
	body := []byte(`{
		"total_count": 3,
		"check_runs": [
			{"name": "lint", "status": "completed", "conclusion": "success", "started_at": "2026-01-01T00:00:00Z"},
			{"name": "suite", "status": "completed", "conclusion": "failure", "started_at": "2026-01-01T00:01:00Z"},
			{"name": "suite", "status": "completed", "conclusion": "success", "started_at": "2026-01-01T00:05:00Z"}
		]
	}`)

	if err := evaluate(body, shaUnderTest, "suite"); err != nil {
		t.Fatalf("evaluate: expected pass for conclusion=success, got error: %v", err)
	}
}

// deliberateCIVerifyFailure forces the suite check red for task 023's
// throwaway v0.0.0-ci-verify-red probe; removed before the branch is deleted.
func TestDeliberateCIVerifyFailure(t *testing.T) {
	t.Fatal("deliberate failure for task 023 ci-verify red-path probe")
}
