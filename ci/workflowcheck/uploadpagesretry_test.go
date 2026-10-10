// uploadpagesretry_test.go: the report job's Pages artifact upload
// (actions/upload-pages-artifact) is retried the way publish's deploy-pages
// is: a transient network failure of one upload must not fail the run,
// while a persistent one still does.
package workflowcheck

import (
	"strings"
	"testing"
)

// uploadPagesAttempts returns the indices of every actions/upload-pages-artifact
// step in steps.
func uploadPagesAttempts(steps []deployStep) []int {
	var indices []int
	for i, s := range steps {
		if strings.HasPrefix(s.Uses, "actions/upload-pages-artifact@") {
			indices = append(indices, i)
		}
	}
	return indices
}

// checkUploadAttempt checks one attempt's continue-on-error (every attempt
// but the last tolerates its own failure, the last never does) and, for a
// later attempt, that both it and the back-off step right before it run
// only when the previous attempt failed.
func checkUploadAttempt(t *testing.T, steps []deployStep, attempts []int, n int) {
	t.Helper()
	step := steps[attempts[n]]
	tolerant := step.ContinueOnError != nil && *step.ContinueOnError
	if last := n == len(attempts)-1; last && tolerant {
		t.Errorf("final upload attempt %q carries continue-on-error: true -- a persistent failure would pass the job", step.Name)
	} else if !last && !tolerant {
		t.Errorf("upload attempt %d %q lacks continue-on-error: true -- its failure would fail the job before the retry", n+1, step.Name)
	}
	if n == 0 {
		return
	}
	prev := steps[attempts[n-1]]
	if prev.ID == "" {
		t.Fatalf("upload attempt %d %q has no id to gate the next attempt on", n, prev.Name)
	}
	gate := "steps." + prev.ID + ".outcome == 'failure'"
	if !strings.Contains(step.If, gate) {
		t.Errorf("upload attempt %d %q has if: %q, want it to contain %q", n+1, step.Name, step.If, gate)
	}
	backoff := attempts[n] - 1
	if backoff == attempts[n-1] || !strings.Contains(steps[backoff].If, gate) || !strings.Contains(steps[backoff].Run, "sleep") {
		t.Errorf("no back-off step gated on %q right before upload attempt %d %q", gate, n+1, step.Name)
	}
}

func TestCIWorkflowReportRetriesThePagesArtifactUpload(t *testing.T) {
	const path = ".github/workflows/ci.yml"
	job := findPublishJob(t, loadPublishWorkflow(t, path), path, "report (Allure)")
	attempts := uploadPagesAttempts(job.Steps)
	if len(attempts) < 3 {
		t.Fatalf("%s report job: %d actions/upload-pages-artifact step(s), want >= 3 attempts", path, len(attempts))
	}
	for n := range attempts {
		checkUploadAttempt(t, job.Steps, attempts, n)
	}
}
