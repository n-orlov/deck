// deploypagesretry_test.go is task 006's own probe (R166, GH #50): a YAML
// structural check over both `publish (Pages)` in ci.yml and
// `publish (Pages, PR)` in pages-pr-publish.yml that fails the moment
// either job's actions/deploy-pages retry chain loses an attempt, an
// attempt loses its continue-on-error (except the last), a later attempt
// (or its preceding back-off step) stops gating on the previous attempt's
// own failure, or the final attempt gains continue-on-error (which would
// let a persistent failure pass the job).
//
// Demonstrated failing against the pre-fix dc2b6f7ece tree (each job
// calls actions/deploy-pages exactly once, with no retry at all): see
// /run/ralphd/artifacts/r166/workflowcheck-dc2b6f7ece-fail.log.
package workflowcheck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// deployStep models just the step fields this probe needs. `ContinueOnError`
// is a *bool (not bool) so "the key is absent" is distinguishable from
// "the key is present and false" -- both mean "does not tolerate
// failure", but the distinction matters if a future edit writes
// `continue-on-error: false` explicitly on the final attempt.
type deployStep struct {
	ID              string `yaml:"id"`
	Name            string `yaml:"name"`
	Uses            string `yaml:"uses"`
	Run             string `yaml:"run"`
	If              string `yaml:"if"`
	ContinueOnError *bool  `yaml:"continue-on-error"`
}

type environmentBlock struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
}

type publishJob struct {
	Name        string           `yaml:"name"`
	Environment environmentBlock `yaml:"environment"`
	Steps       []deployStep     `yaml:"steps"`
}

type publishWorkflow struct {
	Jobs map[string]publishJob `yaml:"jobs"`
}

func loadPublishWorkflow(t *testing.T, relPath string) publishWorkflow {
	t.Helper()
	root, err := repositoryRoot()
	if err != nil {
		t.Fatalf("repositoryRoot: %v", err)
	}
	path := filepath.Join(root, relPath)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	var wf publishWorkflow
	if err := yaml.Unmarshal(raw, &wf); err != nil {
		t.Fatalf("%s: parse YAML: %v", path, err)
	}
	return wf
}

// findPublishJob locates the job in wf.Jobs whose own `name:` equals
// wantName (job ids are free-form; the success criteria name jobs by
// their display name, not their YAML key).
func findPublishJob(t *testing.T, wf publishWorkflow, path, wantName string) publishJob {
	t.Helper()
	for _, j := range wf.Jobs {
		if j.Name == wantName {
			return j
		}
	}
	t.Fatalf("%s: no job with name %q found", path, wantName)
	return publishJob{}
}

// deployPagesStepIndices returns the indices, within job.Steps, of every
// step invoking actions/deploy-pages.
func deployPagesStepIndices(job publishJob) []int {
	var indices []int
	for i, s := range job.Steps {
		if strings.HasPrefix(s.Uses, "actions/deploy-pages@") {
			indices = append(indices, i)
		}
	}
	return indices
}

// checkDeployPagesRetryChain is the shared body of R166's three success
// criteria (>= 3 attempts; later attempts and their preceding back-off
// step gated on the previous attempt's own failure; the final attempt
// not continue-on-error), applied to one job in one workflow file.
func checkDeployPagesRetryChain(t *testing.T, path string, job publishJob) {
	t.Helper()

	deployIdx := deployPagesStepIndices(job)
	if len(deployIdx) < 3 {
		t.Fatalf("%s job %q: found %d actions/deploy-pages step(s), want >= 3", path, job.Name, len(deployIdx))
	}

	last := len(deployIdx) - 1
	for attempt, idx := range deployIdx {
		step := job.Steps[idx]

		isLast := attempt == last
		hasContinueOnError := step.ContinueOnError != nil && *step.ContinueOnError

		if isLast {
			if hasContinueOnError {
				t.Errorf("%s job %q: final deploy-pages attempt (step %q) carries continue-on-error: true -- a persistent failure would not fail the job", path, job.Name, step.Name)
			}
		} else {
			if !hasContinueOnError {
				t.Errorf("%s job %q: deploy-pages attempt %d of %d (step %q) does not carry continue-on-error: true -- its own failure would fail the job outright instead of falling through to the next attempt", path, job.Name, attempt+1, len(deployIdx), step.Name)
			}
		}

		if attempt == 0 {
			continue
		}

		prevStep := job.Steps[deployIdx[attempt-1]]
		if prevStep.ID == "" {
			t.Fatalf("%s job %q: deploy-pages attempt %d (step %q) carries no id -- cannot express a following attempt's own gate on its outcome", path, job.Name, attempt, prevStep.Name)
		}
		wantGate := "steps." + prevStep.ID + ".outcome == 'failure'"

		if !strings.Contains(step.If, wantGate) {
			t.Errorf("%s job %q: deploy-pages attempt %d of %d (step %q) has if: %q, want it to contain %q so it runs only when the previous attempt failed", path, job.Name, attempt+1, len(deployIdx), step.Name, step.If, wantGate)
		}

		// The step immediately preceding this attempt (the back-off) must
		// gate on the same condition, and must not itself be a
		// deploy-pages call (i.e. there really is a back-off step between
		// consecutive attempts, not two deploy-pages calls back to back).
		backoffIdx := idx - 1
		if backoffIdx < 0 || backoffIdx == deployIdx[attempt-1] {
			t.Errorf("%s job %q: no back-off step found immediately before deploy-pages attempt %d of %d (step %q)", path, job.Name, attempt+1, len(deployIdx), step.Name)
			continue
		}
		backoff := job.Steps[backoffIdx]
		if strings.HasPrefix(backoff.Uses, "actions/deploy-pages@") {
			t.Errorf("%s job %q: step immediately before deploy-pages attempt %d of %d is itself a deploy-pages call (step %q) -- no back-off step between them", path, job.Name, attempt+1, len(deployIdx), backoff.Name)
			continue
		}
		if !strings.Contains(backoff.If, wantGate) {
			t.Errorf("%s job %q: back-off step (%q) preceding deploy-pages attempt %d of %d has if: %q, want it to contain %q", path, job.Name, backoff.Name, attempt+1, len(deployIdx), backoff.If, wantGate)
		}
	}

	// environment.url must resolve from whichever attempt actually
	// produced a page_url output -- i.e. it must reference every
	// attempt's own step id's outputs.page_url, not just the first or
	// last.
	for _, idx := range deployIdx {
		step := job.Steps[idx]
		if step.ID == "" {
			continue
		}
		wantRef := "steps." + step.ID + ".outputs.page_url"
		if !strings.Contains(job.Environment.URL, wantRef) {
			t.Errorf("%s job %q: environment.url = %q does not reference %q -- a deploy that succeeds on this attempt would not resolve the environment URL", path, job.Name, job.Environment.URL, wantRef)
		}
	}
}

func TestCIWorkflowPublishPagesRetriesDeployPages(t *testing.T) {
	const path = ".github/workflows/ci.yml"
	wf := loadPublishWorkflow(t, path)
	job := findPublishJob(t, wf, path, "publish (Pages)")
	checkDeployPagesRetryChain(t, path, job)
}

func TestPagesPRPublishWorkflowPublishPagesRetriesDeployPages(t *testing.T) {
	const path = ".github/workflows/pages-pr-publish.yml"
	wf := loadPublishWorkflow(t, path)
	job := findPublishJob(t, wf, path, "publish (Pages, PR)")
	checkDeployPagesRetryChain(t, path, job)
}
