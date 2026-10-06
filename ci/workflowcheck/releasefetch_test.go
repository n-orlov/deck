// releasefetch_test.go: release.yml's gate runs `git merge-base --is-ancestor`
// against origin/main (R214), which needs the full history of every branch;
// actions/checkout's default of one commit on the tag's ref would make that
// check unanswerable.
package workflowcheck

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestReleaseCheckoutFetchesFullHistoryForTheReachabilityGate(t *testing.T) {
	var wf hardeningWorkflow
	if err := yaml.Unmarshal(readWorkflowFile(t, "release.yml"), &wf); err != nil {
		t.Fatalf("release.yml: parse YAML: %v", err)
	}
	var checkouts int
	for _, step := range wf.Jobs["release"].Steps {
		if !strings.HasPrefix(step.Uses, "actions/checkout@") {
			continue
		}
		checkouts++
		if depth, ok := step.With["fetch-depth"].(int); !ok || depth != 0 {
			t.Errorf("checkout step %q has fetch-depth %v, want 0 (full history incl. origin/main)", step.Uses, step.With["fetch-depth"])
		}
	}
	if checkouts != 1 {
		t.Fatalf("release.yml has %d checkout steps, want 1", checkouts)
	}
}
