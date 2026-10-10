// uploadpagesbudget_test.go: the report job's Pages artifact upload keeps its
// pre-cure attempt budget of exactly ONE attempt. A transient failure of the
// artifact service is not papered over with extra attempts or back-off: the
// upload fails the job, and the pinned action is retained.
package workflowcheck

import (
	"regexp"
	"strings"
	"testing"
)

var pinnedSHA = regexp.MustCompile(`@[0-9a-f]{40}$`)

func TestCIWorkflowReportPagesUploadIsOneUnretriedAttempt(t *testing.T) {
	const path = ".github/workflows/ci.yml"
	job := findPublishJob(t, loadPublishWorkflow(t, path), path, "report (Allure)")
	var uploads []deployStep
	for _, s := range job.Steps {
		if strings.HasPrefix(s.Uses, "actions/upload-pages-artifact@") {
			uploads = append(uploads, s)
		}
	}
	if len(uploads) != 1 {
		t.Fatalf("%s report job: %d actions/upload-pages-artifact steps, want exactly 1 (no retry attempts)", path, len(uploads))
	}
	up := uploads[0]
	if up.ContinueOnError != nil && *up.ContinueOnError {
		t.Errorf("the upload carries continue-on-error: true -- a failure would pass the job")
	}
	if up.If != "" {
		t.Errorf("the upload is gated by if: %q -- it must run unconditionally as the single attempt", up.If)
	}
	if !pinnedSHA.MatchString(up.Uses) {
		t.Errorf("the upload action %q is not pinned to a full commit sha", up.Uses)
	}
}

func TestCIWorkflowReportHasNoBackoffOrRetryStepsAroundTransport(t *testing.T) {
	const path = ".github/workflows/ci.yml"
	job := findPublishJob(t, loadPublishWorkflow(t, path), path, "report (Allure)")
	for _, s := range job.Steps {
		if strings.Contains(s.Run, "sleep ") {
			t.Errorf("report job step %q sleeps (%q): a back-off between attempts is a raised budget", s.Name, s.Run)
		}
		if strings.Contains(s.Run, "--retry") {
			t.Errorf("report job step %q passes a retry flag: %q", s.Name, s.Run)
		}
		if s.ID != "" && strings.HasPrefix(s.ID, "pages_upload") {
			t.Errorf("report job step %q has retry-chain id %q", s.Name, s.ID)
		}
		if s.Uses != "" && !pinnedSHA.MatchString(s.Uses) {
			t.Errorf("report job step %q uses %q, not pinned to a full commit sha", s.Name, s.Uses)
		}
	}
}
