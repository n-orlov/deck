// hardening_test.go is task 013's probe (R197, GH #61): ci.yml and
// pages-pr-publish.yml must (1) hand github.ref_name to the shell through
// env, never ${{ }} interpolation inside a run script, (2) pin every
// third-party action by a 40-hex commit SHA with the tag in a trailing
// comment, and (3) drop the GITHUB_TOKEN extraheader from site/.git/config
// once the report job's push is done.
package workflowcheck

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type hardeningStep struct {
	Name string            `yaml:"name"`
	If   string            `yaml:"if"`
	Run  string            `yaml:"run"`
	Env  map[string]string `yaml:"env"`
	With map[string]any    `yaml:"with"`
	Uses string            `yaml:"uses"`
}

type hardeningWorkflow struct {
	Jobs map[string]struct {
		Steps []hardeningStep `yaml:"steps"`
	} `yaml:"jobs"`
}

func readWorkflowFile(t *testing.T, name string) []byte {
	t.Helper()
	root, err := repositoryRoot()
	if err != nil {
		t.Fatalf("repositoryRoot: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(root, ".github", "workflows", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return raw
}

func parseHardening(t *testing.T, name string) hardeningWorkflow {
	t.Helper()
	var wf hardeningWorkflow
	if err := yaml.Unmarshal(readWorkflowFile(t, name), &wf); err != nil {
		t.Fatalf("%s: parse YAML: %v", name, err)
	}
	return wf
}

var refNameExprRe = regexp.MustCompile(`\$\{\{[^}]*github\.ref_name[^}]*\}\}`)

// TestCIRunBlocksNeverInterpolateRefName: criterion (1). The notify step
// must still carry the ref name, via an env entry.
func TestCIRunBlocksNeverInterpolateRefName(t *testing.T) {
	wf := parseHardening(t, "ci.yml")
	envRouted := false
	for jobID, job := range wf.Jobs {
		for _, step := range job.Steps {
			if refNameExprRe.MatchString(step.Run) {
				t.Errorf("ci.yml job %q step %q: run script interpolates github.ref_name; route it through env:", jobID, step.Name)
			}
			if jobID != "notify" {
				continue
			}
			for _, value := range step.Env {
				if refNameExprRe.MatchString(value) {
					envRouted = true
				}
			}
		}
	}
	if !envRouted {
		t.Errorf("ci.yml notify job: no step reads ${{ github.ref_name }} through env:")
	}
}

var (
	usesLineRe   = regexp.MustCompile(`(?m)^\s*(?:-\s+)?uses:\s*(\S+)(.*)$`)
	pinnedUsesRe = regexp.MustCompile(`^[A-Za-z0-9._-]+/[A-Za-z0-9._/-]+@[0-9a-f]{40}$`)
	tagCommentRe = regexp.MustCompile(`^\s+#\s*v?[0-9][A-Za-z0-9._-]*\s*$`)
)

// TestCIAndPagesPublishPinActionsBySHA: criterion (2); release.yml is pinned
// the same way (R205).
func TestCIAndPagesPublishPinActionsBySHA(t *testing.T) {
	for _, name := range []string{"ci.yml", "pages-pr-publish.yml", "release.yml"} {
		matches := usesLineRe.FindAllStringSubmatch(string(readWorkflowFile(t, name)), -1)
		if len(matches) == 0 {
			t.Fatalf("%s: no uses: line found, the probe would pass vacuously", name)
		}
		for _, m := range matches {
			if !pinnedUsesRe.MatchString(m[1]) {
				t.Errorf("%s: uses: %s is not pinned by a 40-hex commit SHA", name, m[1])
			}
			if !tagCommentRe.MatchString(m[2]) {
				t.Errorf("%s: uses: %s lacks a trailing `# <tag>` comment (got %q)", name, m[1], m[2])
			}
		}
	}
}

// TestReportJobLeavesNoExtraheaderInSiteGitConfig: criterion (3). The
// report job copies the extraheader into site/.git/config so the push
// authenticates; a later always-run step must unset it, before the site
// tree is uploaded, unless every checkout there sets persist-credentials
// false (in which case the copy step could not have run at all).
func TestReportJobLeavesNoExtraheaderInSiteGitConfig(t *testing.T) {
	steps := parseHardening(t, "ci.yml").Jobs["report"].Steps
	if len(steps) == 0 {
		t.Fatal("ci.yml: report job has no steps")
	}
	copyAt, pushAt, dropAt, uploadAt := -1, -1, -1, -1
	for i, s := range steps {
		switch {
		case strings.Contains(s.Run, "-C site config") && strings.Contains(s.Run, "extraheader") && !strings.Contains(s.Run, "unset"):
			copyAt = i
		case strings.Contains(s.Run, "ci/pages-persist.sh"):
			pushAt = i
		case strings.Contains(s.Run, "-C site config") && strings.Contains(s.Run, "--unset") && strings.Contains(s.Run, "extraheader"):
			dropAt = i
			if s.If != "always()" {
				t.Errorf("step %q must run `if: always()` so a failed push still drops the token, got %q", s.Name, s.If)
			}
		case strings.HasPrefix(s.Uses, "actions/upload-pages-artifact@"):
			uploadAt = i
		}
	}
	if copyAt < 0 || pushAt < 0 {
		t.Fatalf("report job: copy step (%d) or push step (%d) not found; update this probe with the workflow", copyAt, pushAt)
	}
	if dropAt < 0 {
		t.Fatal("report job: no step unsets http.*.extraheader in site/.git/config after the push")
	}
	if dropAt < pushAt {
		t.Errorf("report job: the extraheader is dropped (step %d) before the push finishes (step %d)", dropAt, pushAt)
	}
	if uploadAt >= 0 && dropAt > uploadAt {
		t.Errorf("report job: the extraheader is dropped (step %d) after the Pages artifact upload (step %d)", dropAt, uploadAt)
	}
}
