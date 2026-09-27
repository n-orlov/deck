package workflowcheck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPRCommentJobUsesPaginatedGoCommand is task 003's own probe (R146,
// review finding B2): the previous `pr-comment` job was an inline
// actions/github-script step whose octokit call
// `github.rest.issues.listComments(...)` carried no pagination
// arguments, which only ever returns page one of a PR's comments
// (GitHub's own default per_page is 30). Once a PR held more ordinary
// comments than that -- or the marker comment itself ended up beyond
// whatever page one held -- every run appended a fresh marker comment
// instead of updating the existing one in place. ci/prcomment
// (main.go/main_test.go in this same directory tree) replaces that call
// with a paginated Go lookup; this test guards the workflow wiring: the
// `pr-comment` job must invoke `go run ./ci/prcomment`, and no file
// under .github/workflows/ may call `issues.listComments` directly ever
// again.
//
// Demonstrated failing against f86a1ad59's own .github/workflows/ci.yml
// (the pre-fix pr-comment job, an inline actions/github-script step
// calling issues.listComments with no pagination arguments and no
// `go run ./ci/prcomment` anywhere in the workflow): see
// /run/ralphd/artifacts/003/workflowcheck-f86a1ad59-fail.log.
func TestPRCommentJobUsesPaginatedGoCommand(t *testing.T) {
	root, err := repositoryRoot()
	if err != nil {
		t.Fatalf("repositoryRoot: %v", err)
	}
	workflowsDir := filepath.Join(root, ".github", "workflows")
	entries, err := os.ReadDir(workflowsDir)
	if err != nil {
		t.Fatalf("read %s: %v", workflowsDir, err)
	}

	var ciYML string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		path := filepath.Join(workflowsDir, e.Name())
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		content := string(raw)
		if strings.Contains(content, "issues.listComments") {
			t.Errorf("%s: calls issues.listComments directly -- this only ever reads page one (default per_page=30) of a PR's comments and can silently duplicate the marker comment; use the paginated ci/prcomment instead", path)
		}
		if e.Name() == "ci.yml" {
			ciYML = content
		}
	}
	if ciYML == "" {
		t.Fatalf("%s: ci.yml not found", workflowsDir)
	}

	headers := jobHeaderRe.FindAllStringSubmatchIndex(ciYML, -1)
	if len(headers) == 0 {
		t.Fatalf("ci.yml: no top-level job header matched (jobHeaderRe) -- the probe cannot locate any job block")
	}

	var prCommentBlock string
	for i, h := range headers {
		if ciYML[h[2]:h[3]] != "pr-comment" {
			continue
		}
		blockStart := h[0]
		blockEnd := len(ciYML)
		if i+1 < len(headers) {
			blockEnd = headers[i+1][0]
		}
		prCommentBlock = ciYML[blockStart:blockEnd]
	}
	if prCommentBlock == "" {
		t.Fatalf("ci.yml: no pr-comment job found at all -- the probe cannot exercise the check it is meant to run")
	}

	const wantCommand = "go run ./ci/prcomment"
	if !strings.Contains(prCommentBlock, wantCommand) {
		t.Errorf("ci.yml: pr-comment job does not run %q; job block:\n%s", wantCommand, prCommentBlock)
	}
}
