// lintgate_test.go is task 028's probe (R188): golangci-lint is a gate,
// not an advisory. It fails the moment the lint job stops running the
// linter before the suite, the two entry points (ci/lint.sh and
// ci/quality.sh) stop sharing one invocation, the quality gate is
// switched off, or gofmt/go vet/go mod tidy turn up in a second place.
package workflowcheck

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// shellLines returns the executable lines of a shell script: comments and
// blank lines dropped.
func shellLines(script string) []string {
	var out []string
	for _, l := range strings.Split(script, "\n") {
		if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "#") {
			out = append(out, l)
		}
	}
	return out
}

func anyLineContains(lines []string, sub string) bool {
	for _, l := range lines {
		if strings.Contains(l, sub) {
			return true
		}
	}
	return false
}

// TestLintJobRunsGolangciLintBeforeTheSuite: the `lint` job runs
// ci/lint.sh, ci/lint.sh runs the shared golangci-lint invocation, and the
// `suite` job needs `lint` so the linter runs first.
func TestLintJobRunsGolangciLintBeforeTheSuite(t *testing.T) {
	wf := loadCIWorkflow(t)
	lint, ok := wf.Jobs["lint"]
	if !ok {
		t.Fatal("ci.yml has no lint job")
	}
	runsLintSh := false
	for _, s := range lint.Steps {
		if strings.Contains(s.Run, "ci/lint.sh") {
			runsLintSh = true
		}
	}
	if !runsLintSh {
		t.Error("the lint job has no step running ci/lint.sh")
	}

	lintSh := shellLines(readRepoFile(t, "ci", "lint.sh"))
	if !anyLineContains(lintSh, "ci/golangci.sh") {
		t.Error("ci/lint.sh does not run ci/golangci.sh, so golangci-lint never gates the lint job")
	}
	if !strings.Contains(strings.Join(shellLines(readRepoFile(t, "ci", "golangci.sh")), "\n"), "golangci-lint run ./...") {
		t.Error("ci/golangci.sh does not run `golangci-lint run ./...` over the whole tree")
	}

	var raw struct {
		Jobs map[string]struct {
			Needs any `yaml:"needs"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal([]byte(readRepoFile(t, ".github", "workflows", "ci.yml")), &raw); err != nil {
		t.Fatalf("parse ci.yml: %v", err)
	}
	needs := raw.Jobs["suite"].Needs
	if s, isStr := needs.(string); !isStr || s != "lint" {
		list, _ := needs.([]any)
		found := false
		for _, n := range list {
			if n == "lint" {
				found = true
			}
		}
		if !found {
			t.Errorf("the suite job does not need lint (needs: %v), so the linter would not run before it", needs)
		}
	}
}

// TestQualityGateRunsTheSameGolangciInvocation: ci/quality.json switches the
// golangci gate on, and ci/qualitycheck runs the very script ci/lint.sh
// runs, so the two entry points cannot drift apart.
func TestQualityGateRunsTheSameGolangciInvocation(t *testing.T) {
	var cfg struct {
		Golangci struct {
			Enabled bool `json:"enabled"`
		} `json:"golangci"`
	}
	if err := json.Unmarshal([]byte(readRepoFile(t, "ci", "quality.json")), &cfg); err != nil {
		t.Fatalf("parse ci/quality.json: %v", err)
	}
	if !cfg.Golangci.Enabled {
		t.Error("ci/quality.json has no enabled golangci gate")
	}
	if !strings.Contains(readRepoFile(t, "ci", "qualitycheck", "golangci.go"), `"ci", "golangci.sh"`) {
		t.Error("ci/qualitycheck's golangci gate does not run ci/golangci.sh")
	}
	if !anyLineContains(shellLines(readRepoFile(t, "ci", "lint.sh")), "ci/golangci.sh") {
		t.Error("ci/lint.sh does not run ci/golangci.sh")
	}
}

// TestFormattingVetAndTidyChecksLiveOnlyInLintSh: R188 "one place".
func TestFormattingVetAndTidyChecksLiveOnlyInLintSh(t *testing.T) {
	if !anyLineContains(shellLines(readRepoFile(t, "ci", "lint.sh")), "go vet ./...") {
		t.Fatal("ci/lint.sh no longer runs go vet ./...")
	}
	for _, rel := range [][]string{{"ci", "quality.sh"}, {"ci", "suite.sh"}, {"ci", "golangci.sh"}} {
		lines := shellLines(readRepoFile(t, rel...))
		for _, banned := range []string{"gofmt", "go vet", "go mod tidy"} {
			if anyLineContains(lines, banned) {
				t.Errorf("%s runs %q, which belongs only in ci/lint.sh", filepath.Join(rel...), banned)
			}
		}
	}
	for name, job := range loadCIWorkflow(t).Jobs {
		for _, step := range job.Steps {
			for _, banned := range []string{"gofmt", "go vet", "go mod tidy"} {
				if strings.Contains(step.Run, banned) {
					t.Errorf("ci.yml job %q runs %q directly, a second place besides ci/lint.sh", name, banned)
				}
			}
		}
	}
}
