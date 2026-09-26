// Package workflowcheck is the R145 probe for task 017/.github/workflows/ci.yml:
// a static structural check over the workflow's own text that fails the
// moment any self-hosted job loses its job-level fork guard, or
// `pull_request_target` reappears anywhere in the workflow. Tier 3 probe
// audit (task 025) added this alongside the ci/lint.sh, ci/suite.sh retry
// and ci/releasegate probes.
package workflowcheck

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func repositoryRoot() (string, error) {
	directory, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(directory, "go.mod")); err == nil {
			return directory, nil
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return "", errors.New("locate repository root containing go.mod")
		}
		directory = parent
	}
}

// headRepoGuard is the exact substring ci.yml repeats on every self-hosted
// job (job-level `if:` cannot read the workflow's own top-level `env:`, so
// it is inlined rather than centralised -- see ci.yml's own comment).
const headRepoGuard = "github.event.pull_request.head.repo.full_name == github.repository"

// jobHeaderRe matches a top-level job id line under `jobs:` (2-space
// indent, e.g. "  lint:"), which is how this probe splits the workflow
// into one block of text per job without a full YAML parse.
var jobHeaderRe = regexp.MustCompile(`(?m)^  ([A-Za-z][A-Za-z0-9_-]*):[ \t]*$`)

// runsOnSelfHostedRe matches a `runs-on:` value naming the self-hosted
// `deck` slots (a YAML flow-sequence, e.g. "[self-hosted, linux, x64, deck]").
var runsOnSelfHostedRe = regexp.MustCompile(`runs-on:\s*\[[^]\n]*self-hosted[^]\n]*\]`)

// TestSelfHostedJobsCarryHeadRepoGuardAndNeverPullRequestTarget is the
// probe for task 017's ci.yml: it fails if (1) `pull_request_target`
// appears anywhere in the workflow -- the one trigger that checks out and
// evaluates workflow YAML from a fork PR's own head, which would defeat
// the guard entirely -- or (2) any job whose `runs-on:` names the
// self-hosted `deck` slots has a job block that does not contain the
// head-repo guard string.
//
// Demonstrated failing against a scratch mutation deleting the `lint` job's
// `if:` line from a disposable worktree's own copy of ci.yml: see
// /run/ralphd/artifacts/probes/tier3/017-forkguard-if-removed-fail.log.
func TestSelfHostedJobsCarryHeadRepoGuardAndNeverPullRequestTarget(t *testing.T) {
	root, err := repositoryRoot()
	if err != nil {
		t.Fatalf("repositoryRoot: %v", err)
	}
	path := filepath.Join(root, ".github", "workflows", "ci.yml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	content := string(raw)

	if strings.Contains(content, "pull_request_target") {
		t.Errorf("%s: found pull_request_target -- this trigger checks out and evaluates workflow YAML from a fork PR's own head, defeating the head-repo guard entirely", path)
	}

	headers := jobHeaderRe.FindAllStringSubmatchIndex(content, -1)
	if len(headers) == 0 {
		t.Fatalf("%s: no top-level job header matched (jobHeaderRe) -- the probe cannot locate any job block", path)
	}

	checkedSelfHosted := 0
	for i, h := range headers {
		name := content[h[2]:h[3]]
		blockStart := h[0]
		blockEnd := len(content)
		if i+1 < len(headers) {
			blockEnd = headers[i+1][0]
		}
		block := content[blockStart:blockEnd]

		if !runsOnSelfHostedRe.MatchString(block) {
			continue // e.g. publish/pr-comment run on ubuntu-latest, no guard required
		}
		checkedSelfHosted++
		if !strings.Contains(block, headRepoGuard) {
			t.Errorf("job %q runs on the self-hosted deck slots but its block does not contain the head-repo guard %q", name, headRepoGuard)
		}
	}
	if checkedSelfHosted == 0 {
		t.Fatalf("%s: no self-hosted job found at all -- the probe cannot exercise the guard it is meant to check", path)
	}
}
