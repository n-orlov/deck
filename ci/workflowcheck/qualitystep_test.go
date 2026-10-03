// qualitystep_test.go is task 086's own probe (R187): a YAML structural
// check over ci.yml that fails the moment ci/quality.sh stops running as
// a step inside the existing `lint`/`suite` jobs -- whether the step is
// deleted outright, or its `run:` stops invoking ci/quality.sh. It never
// checks for a brand-new job: R187 (and the standing rule it comes from)
// is explicit that the quality gates ride inside the existing jobs, never
// a lane of their own.
package workflowcheck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// ciStep models only the fields this probe reads from a step node: its
// `run:` script (the thing that actually invokes ci/quality.sh) and its
// `uses:` (left unread, but present so a `uses:` step unmarshals cleanly
// instead of erroring on an unexpected key).
type ciStep struct {
	Name string `yaml:"name"`
	Run  string `yaml:"run"`
	Uses string `yaml:"uses"`
}

type ciJob struct {
	Steps []ciStep `yaml:"steps"`
}

type ciWorkflow struct {
	Jobs map[string]ciJob `yaml:"jobs"`
}

func loadCIWorkflow(t *testing.T) ciWorkflow {
	t.Helper()
	root, err := repositoryRoot()
	if err != nil {
		t.Fatalf("repositoryRoot: %v", err)
	}
	path := filepath.Join(root, ".github", "workflows", "ci.yml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	var wf ciWorkflow
	if err := yaml.Unmarshal(raw, &wf); err != nil {
		t.Fatalf("%s: parse YAML: %v", path, err)
	}
	return wf
}

// stepInvokingQualitySh reports whether any step in the given job's
// `steps:` list runs ci/quality.sh (directly, or wrapped in ci/run.sh --
// either way the substring "ci/quality.sh" appears in the step's `run:`
// script).
func stepInvokingQualitySh(j ciJob) bool {
	for _, s := range j.Steps {
		if strings.Contains(s.Run, "ci/quality.sh") {
			return true
		}
	}
	return false
}

// TestQualityGatesRunAsStepInLintOrSuiteNotNewJob is task 086's success
// criterion: ci/quality.sh runs as a step inside the existing `lint`
// and/or `suite` jobs (at least one of the two), and no job outside that
// pair invokes it -- a brand-new job wired in instead would pass the
// "runs somewhere" half of this check but fail the "no new job" half.
//
// Demonstrated failing against a scratch mutation deleting the quality
// step from both jobs: see
// /run/ralphd/artifacts/task086/qualitystep-removed-fail.log.
func TestQualityGatesRunAsStepInLintOrSuiteNotNewJob(t *testing.T) {
	wf := loadCIWorkflow(t)

	lint, lintOK := wf.Jobs["lint"]
	suite, suiteOK := wf.Jobs["suite"]
	if !lintOK && !suiteOK {
		t.Fatalf("ci.yml: neither \"lint\" nor \"suite\" job found -- the probe cannot locate either existing job")
	}

	inLint := lintOK && stepInvokingQualitySh(lint)
	inSuite := suiteOK && stepInvokingQualitySh(suite)

	if !inLint && !inSuite {
		t.Errorf("ci.yml: no step in \"lint\" or \"suite\" invokes ci/quality.sh")
	}

	for name, j := range wf.Jobs {
		if name == "lint" || name == "suite" {
			continue
		}
		if stepInvokingQualitySh(j) {
			t.Errorf("ci.yml: job %q invokes ci/quality.sh -- R187 requires the quality gates to run as a step inside the existing lint/suite jobs, adding no new job", name)
		}
	}
}
