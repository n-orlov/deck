// uploadpagesretry_test.go: the report-pipeline attempt budgets stay at their
// pre-cure value of ONE attempt each (task cure-01-02-2, re-taken by
// retake-01-02-02-2). It pins what the tree must NOT contain -- a retry chain
// around the Pages artifact upload, a retry flag or loop around the Allure
// download -- and what it must keep: the pinned upload action, the checksum
// verification and failure propagation (no masking of an upload failure).
package workflowcheck

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// pagesUploadWorkflows are every workflow that uploads a Pages artifact; each
// job in them may hold at most one upload step.
var pagesUploadWorkflows = []string{
	".github/workflows/ci.yml",
	".github/workflows/pages-pr-publish.yml",
}

func TestNoJobHoldsMoreThanOnePagesUploadAttempt(t *testing.T) {
	for _, path := range pagesUploadWorkflows {
		for key, job := range loadPublishWorkflow(t, path).Jobs {
			n := 0
			for _, s := range job.Steps {
				if strings.HasPrefix(s.Uses, "actions/upload-pages-artifact@") {
					n++
				}
			}
			if n > 1 {
				t.Errorf("%s job %q: %d upload-pages-artifact steps, want at most 1 (no retry attempts)", path, key, n)
			}
		}
	}
}

func TestPagesUploadFailureIsNotMaskedByLaterSteps(t *testing.T) {
	const path = ".github/workflows/ci.yml"
	job := findPublishJob(t, loadPublishWorkflow(t, path), path, "report (Allure)")
	upload := -1
	for i, s := range job.Steps {
		if strings.HasPrefix(s.Uses, "actions/upload-pages-artifact@") {
			upload = i
		}
	}
	if upload < 0 {
		t.Fatalf("%s report job has no upload-pages-artifact step", path)
	}
	for i, s := range job.Steps {
		text := s.If + " " + s.Run
		if strings.Contains(text, "outcome") && strings.Contains(text, "upload") {
			t.Errorf("step %q inspects an upload outcome (%q): a failed upload would be recovered, not propagated", s.Name, text)
		}
		if i > upload && s.ContinueOnError != nil && *s.ContinueOnError {
			t.Errorf("step %q after the upload tolerates failure", s.Name)
		}
	}
	if job.Steps[upload].ID != "" {
		t.Errorf("the upload step carries id %q: an id only exists to be referenced by a retry gate", job.Steps[upload].ID)
	}
}

func TestPagesPRPublishUploadIsAlsoASingleUnretriedAttempt(t *testing.T) {
	const path = ".github/workflows/pages-pr-publish.yml"
	found := false
	for _, job := range loadPublishWorkflow(t, path).Jobs {
		for _, s := range job.Steps {
			if !strings.HasPrefix(s.Uses, "actions/upload-pages-artifact@") {
				continue
			}
			found = true
			if s.ContinueOnError != nil && *s.ContinueOnError {
				t.Errorf("%s: the upload carries continue-on-error: true", path)
			}
			if !pinnedSHA.MatchString(s.Uses) {
				t.Errorf("%s: upload action %q is not pinned to a full commit sha", path, s.Uses)
			}
		}
	}
	if !found {
		t.Fatalf("%s holds no upload-pages-artifact step", path)
	}
}

func TestAllureDownloadIsOneCurlAttemptWithChecksumAbort(t *testing.T) {
	root, err := repositoryRoot()
	if err != nil {
		t.Fatalf("repositoryRoot: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(root, "ci", "allure-report.sh"))
	if err != nil {
		t.Fatalf("read ci/allure-report.sh: %v", err)
	}
	var code []string
	for _, line := range strings.Split(string(raw), "\n") {
		if trimmed := strings.TrimSpace(line); !strings.HasPrefix(trimmed, "#") {
			code = append(code, trimmed)
		}
	}
	script := strings.Join(code, "\n")
	if n := len(regexp.MustCompile(`\bcurl\b`).FindAllString(script, -1)); n != 1 {
		t.Errorf("ci/allure-report.sh runs curl %d times, want exactly 1 attempt", n)
	}
	for _, banned := range []string{"--retry", "--retry-all-errors", "--retry-delay", "sleep ", "until ", "while "} {
		if strings.Contains(script, banned) {
			t.Errorf("ci/allure-report.sh contains %q: a retry/back-off wrapper raises the attempt budget", banned)
		}
	}
	for _, kept := range []string{"curl -fsSL", `checksum mismatch`, "exit 1"} {
		if !strings.Contains(script, kept) {
			t.Errorf("ci/allure-report.sh lost %q: failure propagation / integrity check", kept)
		}
	}
}
