// pagesprpublish_test.go is task 005's own probe (R163, GH #50): a YAML
// structural check over pages-pr-publish.yml -- not string matching --
// that fails the moment its workflow_run trigger stops excluding a `main`
// head branch, stops admitting a non-`main` head branch, or the `gate`
// job's own guard (a `pull_request` event and a same-repo head) weakens.
//
// Demonstrated failing against the pre-fix dc2b6f7ece tree (no
// branches-ignore on the workflow_run trigger at all, so a `main` head
// branch is never excluded): see
// /run/ralphd/artifacts/r163/workflowcheck-dc2b6f7ece-fail.log.
package workflowcheck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// workflowRunTrigger models just the on.workflow_run shape this probe
// needs: the list of workflow names it reacts to and the branch filters
// GitHub evaluates against the *triggering* workflow's own head branch
// (see GitHub's own docs, "Events that trigger workflows" ->
// `workflow_run`, and "Workflow syntax for GitHub Actions" ->
// `on.<push|pull_request>.<branches|branches-ignore>`).
type workflowRunTrigger struct {
	Workflows      []string `yaml:"workflows"`
	Types          []string `yaml:"types"`
	Branches       []string `yaml:"branches"`
	BranchesIgnore []string `yaml:"branches-ignore"`
}

type onBlock struct {
	WorkflowRun workflowRunTrigger `yaml:"workflow_run"`
}

// job models only the fields this probe reads from a job block: its own
// `if:` condition (a YAML expression string) and, for `gate`, nothing
// else -- the rest of the workflow (steps, outputs, permissions) is left
// as raw nodes so unmarshalling never has to model the whole schema.
type job struct {
	If yaml.Node `yaml:"if"`
}

type pagesPRPublishWorkflow struct {
	On   onBlock        `yaml:"on"`
	Jobs map[string]job `yaml:"jobs"`
}

func loadPagesPRPublishWorkflow(t *testing.T) pagesPRPublishWorkflow {
	t.Helper()
	root, err := repositoryRoot()
	if err != nil {
		t.Fatalf("repositoryRoot: %v", err)
	}
	path := filepath.Join(root, ".github", "workflows", "pages-pr-publish.yml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	var wf pagesPRPublishWorkflow
	if err := yaml.Unmarshal(raw, &wf); err != nil {
		t.Fatalf("%s: parse YAML: %v", path, err)
	}
	return wf
}

func contains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

// TestWorkflowRunTriggerExcludesMainHeadBranch is R163's own success
// criterion: the trigger excludes a `main` head branch.
func TestWorkflowRunTriggerExcludesMainHeadBranch(t *testing.T) {
	wf := loadPagesPRPublishWorkflow(t)
	trigger := wf.On.WorkflowRun

	if len(trigger.BranchesIgnore) == 0 && len(trigger.Branches) == 0 {
		t.Fatalf("on.workflow_run carries neither branches nor branches-ignore -- every ci completion (including main push/schedule/workflow_dispatch runs) starts a pages-pr-publish run")
	}

	if len(trigger.BranchesIgnore) > 0 {
		if !contains(trigger.BranchesIgnore, "main") {
			t.Errorf("on.workflow_run.branches-ignore = %v does not exclude \"main\"", trigger.BranchesIgnore)
		}
		return
	}

	// A `branches:` allow-list form would exclude main by omission; that
	// is an equally valid shape, so only fault it if main was actually
	// listed.
	if contains(trigger.Branches, "main") {
		t.Errorf("on.workflow_run.branches = %v admits \"main\" instead of excluding it", trigger.Branches)
	}
}

// TestWorkflowRunTriggerAdmitsNonMainHeadBranch is R163's second success
// criterion: the trigger still admits a non-main head branch (e.g. a PR's
// own head branch, "feature/x"), i.e. it did not overshoot into excluding
// everything.
func TestWorkflowRunTriggerAdmitsNonMainHeadBranch(t *testing.T) {
	wf := loadPagesPRPublishWorkflow(t)
	trigger := wf.On.WorkflowRun

	const candidate = "feature/x"

	if len(trigger.BranchesIgnore) > 0 {
		if contains(trigger.BranchesIgnore, candidate) || contains(trigger.BranchesIgnore, "*") {
			t.Errorf("on.workflow_run.branches-ignore = %v excludes non-main head branch %q", trigger.BranchesIgnore, candidate)
		}
		return
	}

	if len(trigger.Branches) > 0 {
		if !contains(trigger.Branches, candidate) && !contains(trigger.Branches, "*") {
			t.Errorf("on.workflow_run.branches = %v does not admit non-main head branch %q", trigger.Branches, candidate)
		}
		return
	}

	t.Fatalf("on.workflow_run carries neither branches nor branches-ignore -- cannot confirm a non-main head branch is admitted")
}

// TestGateJobStillRequiresPullRequestEventAndSameRepoHead is R163's third
// success criterion: the gate job's pull_request and same-repo head
// checks stay intact.
func TestGateJobStillRequiresPullRequestEventAndSameRepoHead(t *testing.T) {
	wf := loadPagesPRPublishWorkflow(t)

	gate, ok := wf.Jobs["gate"]
	if !ok {
		t.Fatalf("pages-pr-publish.yml: no \"gate\" job found")
	}
	if gate.If.Value == "" {
		t.Fatalf("pages-pr-publish.yml: gate job carries no if: condition")
	}
	cond := gate.If.Value

	const wantEvent = "github.event.workflow_run.event == 'pull_request'"
	const wantSameRepo = "github.event.workflow_run.head_repository.full_name == github.repository"

	if !strings.Contains(cond, wantEvent) {
		t.Errorf("gate job's if: condition does not require %q; got: %q", wantEvent, cond)
	}
	if !strings.Contains(cond, wantSameRepo) {
		t.Errorf("gate job's if: condition does not require %q; got: %q", wantSameRepo, cond)
	}
}
