package main

import (
	"errors"
	"strings"
	"testing"
)

// TestEvaluate_MalformedResponses asserts that a body that is not JSON is
// refused with an error naming the sha and the response that could not be
// parsed, whichever of the two API responses is the malformed one.
func TestEvaluate_MalformedResponses(t *testing.T) {
	const notJSON = `<html>502 Bad Gateway</html>`
	cases := []struct {
		name        string
		checkRuns   string
		actionsRuns string
		wantCall    string
	}{
		{"check-runs body is not JSON", notJSON, noWorkflowRuns, "check-runs"},
		{"actions-runs body is not JSON", `{"total_count":0,"check_runs":[]}`, notJSON, "actions-runs"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := evaluate([]byte(tc.checkRuns), []byte(tc.actionsRuns), shaUnderTest, "suite")
			if err == nil {
				t.Fatalf("evaluate: expected refusal for a non-JSON %s body, got nil", tc.wantCall)
			}
			if !strings.Contains(err.Error(), shaUnderTest) {
				t.Errorf("evaluate error %q does not name the sha %q", err.Error(), shaUnderTest)
			}
			if !strings.Contains(err.Error(), tc.wantCall) {
				t.Errorf("evaluate error %q does not name the failing call %q", err.Error(), tc.wantCall)
			}
			var other interface{ Unwrap() error }
			if !errors.As(err, &other) || other.Unwrap() == nil {
				t.Errorf("evaluate error %q does not wrap the underlying JSON error", err.Error())
			}
		})
	}
}
