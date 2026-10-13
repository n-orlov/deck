// badgerefresh_test.go: R290's badges refresh on every non-cancelled push or
// workflow_dispatch run on main, including the run whose lint failed (so
// `suite` was skipped and left no results) and the run whose suite failed.
package workflowcheck

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func reportJobIf(t *testing.T) string {
	t.Helper()
	_, jobs := loadCIWorkflowRaw(t)
	n, ok := jobs["report"]["if"]
	if !ok || n.Kind != yaml.ScalarNode || n.Value == "" {
		t.Fatal("ci.yml: report job has no job-level if:")
	}
	return n.Value
}

// TestReportJobRunsForBadgesWhenLintFailedAndSuiteSkipped: the report job (the
// one that writes badges) runs for a failed lint with a skipped suite on
// push/dispatch to main, and for no other skipped-suite case.
func TestReportJobRunsForBadgesWhenLintFailedAndSuiteSkipped(t *testing.T) {
	cond := reportJobIf(t)
	eval := func(event, ref, lint, suite string) bool {
		return evalGhBoolTemplate(cond, map[string]string{
			"github.event_name": event,
			"github.ref":        ref,
			"github.repository": "n-orlov/deck",
			"github.event.pull_request.head.repo.full_name": "n-orlov/deck",
			"needs.lint.result":                             lint,
			"needs.suite.result":                            suite,
		})
	}
	const main, feat = "refs/heads/main", "refs/heads/feature"
	for _, c := range []struct {
		name                    string
		event, ref, lint, suite string
		want                    bool
	}{
		{"push main, lint failed, suite skipped", "push", main, "failure", "skipped", true},
		{"dispatch main, lint failed, suite skipped", "workflow_dispatch", main, "failure", "skipped", true},
		{"push main, lint ok, suite failed", "push", main, "success", "failure", true},
		{"dispatch main, lint ok, suite failed", "workflow_dispatch", main, "success", "failure", true},
		{"push main, all green", "push", main, "success", "success", true},
		{"pull_request, lint failed, suite skipped", "pull_request", "refs/pull/7/merge", "failure", "skipped", false},
		{"schedule, lint failed, suite skipped", "schedule", main, "failure", "skipped", false},
		{"push feature, lint failed, suite skipped", "push", feat, "failure", "skipped", false},
		{"push main, lint cancelled (superseded)", "push", main, "cancelled", "skipped", false},
		{"push main, suite cancelled (superseded)", "push", main, "success", "cancelled", false},
		{"dispatch main, lint skipped, suite skipped", "workflow_dispatch", main, "skipped", "skipped", false},
	} {
		if got := eval(c.event, c.ref, c.lint, c.suite); got != c.want {
			t.Errorf("report job for %s = %v, want %v", c.name, got, c.want)
		}
	}
}

// TestBadgeStepsSurviveAFailedGate: the badge steps do not hide behind
// success(), so a failed earlier step in report (or a failed gate) still
// refreshes them, and the generator receives the lint job's own result.
func TestBadgeStepsSurviveAFailedGate(t *testing.T) {
	wf := loadCIWorkflow(t)
	steps := badgeSteps(wf)
	if len(steps) < 2 {
		t.Fatalf("expected generate and publish badge steps, found %d", len(steps))
	}
	for _, b := range steps {
		if strings.Contains(b.step.If, "success()") {
			t.Errorf("badge step %q is gated on success(): a failed gate would leave the previous green badge", b.step.Name)
		}
		if !strings.Contains(b.step.If, "!cancelled()") {
			t.Errorf("badge step %q if: %q does not use !cancelled()", b.step.Name, b.step.If)
		}
		if strings.Contains(b.step.Run, "ci/badgegen") && !strings.Contains(b.step.Run, "-lint-result") {
			t.Errorf("badge generate step does not pass -lint-result, so a skipped suite cannot render lint red")
		}
	}
}

// TestReportJobStepsNeedingResultsAreGated: with no results artifact (lint
// failed, suite skipped) the Allure steps and the results summary are
// skipped, so the badge steps are reached.
func TestReportJobStepsNeedingResultsAreGated(t *testing.T) {
	wf := loadCIWorkflow(t)
	for _, s := range wf.Jobs["report"].Steps {
		needsResults := strings.Contains(s.Run, "allure-report.sh") ||
			strings.Contains(s.Run, "allure-smoke") || strings.Contains(s.Run, "ci/summary.sh") ||
			strings.Contains(s.Run, "TestAllureFormatter") || strings.Contains(s.Run, "docker build") ||
			strings.Contains(s.Run, "allure-report ") || strings.Contains(s.Run, "pages-persist.sh site allure-report")
		if needsResults && !strings.Contains(s.If, "steps.results.outcome == 'success'") {
			t.Errorf("report step %q needs the results artifact but is not gated on steps.results.outcome", s.Name)
		}
	}
}
