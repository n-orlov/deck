// lintgate_test.go is task 028's probe (R188): golangci-lint is a gate,
// not an advisory. It fails the moment the Lint stage stops running the
// linter before the suite, the two entry points (ci/lint.sh and
// ci/quality.sh) stop sharing one invocation, the quality gate is
// switched off, or gofmt/go vet/go mod tidy turn up in a second place.
package repocheck

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
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

// TestLintJobRunsGolangciLintBeforeTheSuite: the Jenkinsfile's Lint stage
// runs ci/lint.sh, ci/lint.sh runs the shared golangci-lint invocation, and
// the Lint stage comes before the Suite stage, so the linter runs first.
func TestLintJobRunsGolangciLintBeforeTheSuite(t *testing.T) {
	jf := readRepoFile(t, "Jenkinsfile")
	lint, ok := jenkinsStage(jf, "Lint")
	if !ok {
		t.Fatal("the Jenkinsfile has no Lint stage")
	}
	if !strings.Contains(lint, "ci/run.sh ci/lint.sh") {
		t.Error("the Lint stage does not run ci/run.sh ci/lint.sh")
	}
	if !stageBefore(jf, "Lint", "Suite") {
		t.Error("the Lint stage does not come before the Suite stage, so the linter would not run first")
	}

	lintSh := shellLines(readRepoFile(t, "ci", "lint.sh"))
	if !anyLineContains(lintSh, "ci/golangci.sh") {
		t.Error("ci/lint.sh does not run ci/golangci.sh, so golangci-lint never gates the Lint stage")
	}
	if !strings.Contains(strings.Join(shellLines(readRepoFile(t, "ci", "golangci.sh")), "\n"), "golangci-lint run ./...") {
		t.Error("ci/golangci.sh does not run `golangci-lint run ./...` over the whole tree")
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
	for _, l := range jenkinsfileLines(readRepoFile(t, "Jenkinsfile")) {
		for _, banned := range []string{"gofmt", "go vet", "go mod tidy"} {
			if strings.Contains(l, banned) {
				t.Errorf("the Jenkinsfile runs %q directly (%q), a second place besides ci/lint.sh", banned, l)
			}
		}
	}
}
