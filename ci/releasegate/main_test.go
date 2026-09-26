package main

import (
	"strings"
	"testing"
)

const shaUnderTest = "deadbeefcafef00dfeedface1234567890abcde"

// noWorkflowRuns is the actions/runs?head_sha=<sha> body for a sha that
// has no workflow runs recorded at all -- used by the pre-existing cases
// below, none of which cares about the push/pull_request distinction
// itself, only about needing SOME actionsRunsBody argument now that
// evaluate takes one.
const noWorkflowRuns = `{"workflow_runs":[]}`

// TestEvaluate_NoRun asserts refusal when the check-runs API reports no
// run for the "suite" check at all (an empty check_runs list) -- the sha
// simply has no CI result yet.
func TestEvaluate_NoRun(t *testing.T) {
	body := []byte(`{"total_count":0,"check_runs":[]}`)

	err := evaluate(body, []byte(noWorkflowRuns), shaUnderTest, "suite")
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
// with conclusion "failure", on a run whose workflow event is "push" (so
// the push/pull_request-only filter does not itself hide this run).
func TestEvaluate_Failure(t *testing.T) {
	body := []byte(`{
		"total_count": 1,
		"check_runs": [
			{"name": "suite", "status": "completed", "conclusion": "failure", "started_at": "2026-01-01T00:00:00Z", "check_suite": {"id": 501}}
		]
	}`)
	actionsRuns := []byte(`{"workflow_runs":[{"event":"push","check_suite_id":501}]}`)

	err := evaluate(body, actionsRuns, shaUnderTest, "suite")
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
// started but not yet completed (status=in_progress, conclusion=null),
// on a push run.
func TestEvaluate_InProgress(t *testing.T) {
	body := []byte(`{
		"total_count": 1,
		"check_runs": [
			{"name": "suite", "status": "in_progress", "conclusion": null, "started_at": "2026-01-01T00:00:00Z", "check_suite": {"id": 502}}
		]
	}`)
	actionsRuns := []byte(`{"workflow_runs":[{"event":"push","check_suite_id":502}]}`)

	err := evaluate(body, actionsRuns, shaUnderTest, "suite")
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
// recently started push run completed with conclusion "success", even
// amid other, older or differently-named check runs (an earlier failed
// attempt on the same sha, plus an unrelated "lint" check) -- evaluate
// must pick the latest "suite" run by started_at, not merely the first
// one it sees. All three check runs here belong to push workflow runs,
// so the push/pull_request filter passes all of them through unchanged.
func TestEvaluate_Success(t *testing.T) {
	body := []byte(`{
		"total_count": 3,
		"check_runs": [
			{"name": "lint", "status": "completed", "conclusion": "success", "started_at": "2026-01-01T00:00:00Z", "check_suite": {"id": 601}},
			{"name": "suite", "status": "completed", "conclusion": "failure", "started_at": "2026-01-01T00:01:00Z", "check_suite": {"id": 602}},
			{"name": "suite", "status": "completed", "conclusion": "success", "started_at": "2026-01-01T00:05:00Z", "check_suite": {"id": 603}}
		]
	}`)
	actionsRuns := []byte(`{"workflow_runs":[
		{"event":"push","check_suite_id":601},
		{"event":"push","check_suite_id":602},
		{"event":"push","check_suite_id":603}
	]}`)

	if err := evaluate(body, actionsRuns, shaUnderTest, "suite"); err != nil {
		t.Fatalf("evaluate: expected pass for conclusion=success, got error: %v", err)
	}
}

// TestEvaluate_PushSuccess_LaterDispatchFailure_Accepted is R147/steer
// 001's own acceptance case: a push suite run went green, and a later-
// started manual workflow_dispatch run of the same check on the same sha
// went red. Per SPEC §13.2, a dispatch run alerts but never gates, so
// evaluate must ignore it entirely and accept on the push run alone --
// even though the dispatch run's started_at is later, so a naive
// "most recently started, full stop" rule (the pre-change behaviour)
// would refuse here.
func TestEvaluate_PushSuccess_LaterDispatchFailure_Accepted(t *testing.T) {
	body := []byte(`{
		"total_count": 2,
		"check_runs": [
			{"name": "suite", "status": "completed", "conclusion": "success", "started_at": "2026-01-01T00:00:00Z", "check_suite": {"id": 701}},
			{"name": "suite", "status": "completed", "conclusion": "failure", "started_at": "2026-01-01T01:00:00Z", "check_suite": {"id": 702}}
		]
	}`)
	actionsRuns := []byte(`{"workflow_runs":[
		{"event":"push","check_suite_id":701},
		{"event":"workflow_dispatch","check_suite_id":702}
	]}`)

	if err := evaluate(body, actionsRuns, shaUnderTest, "suite"); err != nil {
		t.Fatalf("evaluate: expected acceptance (push success, later dispatch failure ignored), got error: %v", err)
	}
}

// TestEvaluate_PushSuccess_LaterScheduleFailure_Accepted is the same
// acceptance case as above but for a nightly "schedule" run instead of a
// manual "workflow_dispatch" one -- SPEC §13.2 names both as non-gating.
func TestEvaluate_PushSuccess_LaterScheduleFailure_Accepted(t *testing.T) {
	body := []byte(`{
		"total_count": 2,
		"check_runs": [
			{"name": "suite", "status": "completed", "conclusion": "success", "started_at": "2026-01-01T00:00:00Z", "check_suite": {"id": 801}},
			{"name": "suite", "status": "completed", "conclusion": "failure", "started_at": "2026-01-01T02:00:00Z", "check_suite": {"id": 802}}
		]
	}`)
	actionsRuns := []byte(`{"workflow_runs":[
		{"event":"push","check_suite_id":801},
		{"event":"schedule","check_suite_id":802}
	]}`)

	if err := evaluate(body, actionsRuns, shaUnderTest, "suite"); err != nil {
		t.Fatalf("evaluate: expected acceptance (push success, later schedule failure ignored), got error: %v", err)
	}
}

// TestEvaluate_OnlyDispatchOrScheduleSuccess_Refused asserts refusal when
// the only "suite" run(s) on the sha belong to non-gating (workflow_
// dispatch/schedule) workflow runs, even though that run's own conclusion
// is "success" -- a green nightly/dispatch run must never substitute for
// a missing push/pull_request one.
func TestEvaluate_OnlyDispatchOrScheduleSuccess_Refused(t *testing.T) {
	body := []byte(`{
		"total_count": 1,
		"check_runs": [
			{"name": "suite", "status": "completed", "conclusion": "success", "started_at": "2026-01-01T00:00:00Z", "check_suite": {"id": 901}}
		]
	}`)
	actionsRuns := []byte(`{"workflow_runs":[{"event":"workflow_dispatch","check_suite_id":901}]}`)

	err := evaluate(body, actionsRuns, shaUnderTest, "suite")
	if err == nil {
		t.Fatalf("evaluate: expected refusal when only a non-gating run exists, got nil")
	}
	if !strings.Contains(err.Error(), shaUnderTest) {
		t.Errorf("evaluate error %q does not name the sha %q", err.Error(), shaUnderTest)
	}
	if !strings.Contains(err.Error(), "non-gating") {
		t.Errorf("evaluate error %q does not say that only non-gating runs exist", err.Error())
	}
}
