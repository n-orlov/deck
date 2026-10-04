// allureSmokeStep_test.go is task 006's probe (R196): the Allure smoke check
// (ci/allure-smoke.sh) must run as a step of the existing `report` job, never
// a job of its own, and before the report is generated for publishing.
package workflowcheck

import (
	"strings"
	"testing"
)

func smokeStepIndex(j ciJob) int {
	for i, s := range j.Steps {
		if strings.Contains(s.Run, "ci/allure-smoke.sh") {
			return i
		}
	}
	return -1
}

func TestAllureSmokeRunsAsAStepOfTheReportJobBeforeThePublishedReport(t *testing.T) {
	wf := loadCIWorkflow(t)

	report, ok := wf.Jobs["report"]
	if !ok {
		t.Fatal("ci.yml: no \"report\" job")
	}
	smoke := smokeStepIndex(report)
	if smoke < 0 {
		t.Fatal("ci.yml: no step of the \"report\" job runs ci/allure-smoke.sh")
	}
	generate := -1
	for i, s := range report.Steps {
		if strings.Contains(s.Run, "ci/allure-report.sh") {
			generate = i
		}
	}
	if generate < 0 || smoke > generate {
		t.Errorf("ci.yml: the smoke step (index %d) must come before the real report generation (index %d)", smoke, generate)
	}

	for name, j := range wf.Jobs {
		if name != "report" && smokeStepIndex(j) >= 0 {
			t.Errorf("ci.yml: job %q runs ci/allure-smoke.sh -- R196 wants it inside the existing report job, adding no new job", name)
		}
	}
}
