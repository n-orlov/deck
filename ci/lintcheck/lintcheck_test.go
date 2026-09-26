// Package lintcheck is the R145 probe for task 013/ci/lint.sh: a static
// structural check over the script's own text that fails the moment any of
// the three fail-fast lint stages, their exit-on-drift handling, or their
// ordering, goes missing -- without needing a live Go toolchain, git tree
// or gofmt binary to execute the script itself. Tier 3 probe audit (task
// 025) added this alongside the ci.yml fork-guard, ci/suite.sh retry, and
// ci/releasegate probes.
package lintcheck

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stripCommentLines drops every line whose first non-whitespace character
// is `#` (a POSIX sh full-line comment), so a marker or exit statement
// mentioned only in prose above the real code is never mistaken for the
// real thing.
func stripCommentLines(s string) string {
	lines := strings.Split(s, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

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

// TestLintScriptRunsAllThreeStagesFailFastAndExitsOnDrift is the probe for
// task 013's ci/lint.sh: it fails if the script's own text no longer shows
// (1) `set -eu` (or `set -e`) so any unguarded failing command aborts the
// script, (2) all three stages present -- `gofmt -l`, `go vet ./...`,
// `go mod tidy` -- in that exact order, so a formatting problem is reported
// before vet or tidy ever run, and (3) an explicit non-zero exit on the
// gofmt-drift branch and on the go-mod-tidy-drift branch (neither `gofmt -l`
// nor a masked `diff` alone makes the script fail without one).
//
// Demonstrated failing against a scratch mutation removing the `go vet
// ./...` stage from a disposable worktree's own copy of ci/lint.sh: see
// /run/ralphd/artifacts/probes/tier3/013-lint-vet-stage-removed-fail.log.
func TestLintScriptRunsAllThreeStagesFailFastAndExitsOnDrift(t *testing.T) {
	root, err := repositoryRoot()
	if err != nil {
		t.Fatalf("repositoryRoot: %v", err)
	}
	path := filepath.Join(root, "ci", "lint.sh")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	fullContent := string(raw)
	// The three stage markers ("gofmt -l", "go vet ./...", "go mod tidy")
	// also appear, in that same order, in the script's own leading comment
	// block -- so an ordering/exit check against the raw file text would
	// spuriously pass (or, worse, spuriously fail) on the comment's own
	// occurrences rather than the real invocations below it. Strip every
	// full-line `#` comment before locating them.
	content := stripCommentLines(fullContent)

	if !strings.Contains(content, "set -eu") && !strings.Contains(content, "set -e") {
		t.Errorf("%s: missing `set -eu`/`set -e` -- an unguarded failing command (e.g. go vet) would no longer abort the script", path)
	}

	idxGofmt := strings.Index(content, "gofmt -l")
	idxVet := strings.Index(content, "go vet ./...")
	idxTidy := strings.Index(content, "go mod tidy")

	if idxGofmt < 0 {
		t.Errorf("%s: missing the `gofmt -l` stage", path)
	}
	if idxVet < 0 {
		t.Errorf("%s: missing the `go vet ./...` stage", path)
	}
	if idxTidy < 0 {
		t.Errorf("%s: missing the `go mod tidy` stage", path)
	}
	if idxGofmt >= 0 && idxVet >= 0 && idxTidy >= 0 {
		if !(idxGofmt < idxVet && idxVet < idxTidy) {
			t.Errorf("%s: the three stages are not in fail-fast order gofmt(%d) < vet(%d) < tidy(%d)", path, idxGofmt, idxVet, idxTidy)
		}
	}

	// gofmt -l never returns non-zero merely for listing files, so the
	// drift branch needs its own explicit non-zero exit between the gofmt
	// stage and the vet stage.
	if idxGofmt >= 0 && idxVet > idxGofmt {
		gofmtBlock := content[idxGofmt:idxVet]
		if !strings.Contains(gofmtBlock, "exit 1") && !strings.Contains(gofmtBlock, "exit 2") {
			t.Errorf("%s: the gofmt-drift branch (between the gofmt and vet stages) has no explicit non-zero exit", path)
		}
	}

	// The `go mod tidy` diff check is masked behind `||`/a captured status
	// variable in this script, so it likewise needs its own explicit
	// non-zero exit on drift, appearing after the tidy stage begins.
	if idxTidy >= 0 {
		tidyBlock := content[idxTidy:]
		if !strings.Contains(tidyBlock, "exit 1") && !strings.Contains(tidyBlock, "exit 2") {
			t.Errorf("%s: the go-mod-tidy-drift branch (after the tidy stage) has no explicit non-zero exit", path)
		}
	}
}
