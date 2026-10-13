package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func repositoryRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal(errors.New("no go.mod above the test directory"))
		}
		dir = parent
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func goModVersion(t *testing.T) string {
	t.Helper()
	v, err := GoVersion(readFile(t, filepath.Join(repositoryRoot(t), "go.mod")))
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// TestReadmeBadgeBlockMatchesGenerator fails when the block committed in
// README.md differs, by one character, from what the generator emits, and
// when the README does not open with the block.
func TestReadmeBadgeBlockMatchesGenerator(t *testing.T) {
	readme := readFile(t, filepath.Join(repositoryRoot(t), "README.md"))
	if !strings.HasPrefix(readme, StartMarker+"\n") {
		t.Fatalf("README.md must open with %s", StartMarker)
	}
	got, err := Extract(readme)
	if err != nil {
		t.Fatal(err)
	}
	if want := Block(goModVersion(t)); got != want {
		t.Fatalf("README.md badge block differs from the generator; run `go run ./ci/readmebadges`\n--- committed\n%s\n--- generated\n%s", got, want)
	}
}

// TestReadmeBadgeBlockDetectsOneCharacterEdit is the red side of the drift
// test: the same comparison, over a copy with one character changed.
func TestReadmeBadgeBlockDetectsOneCharacterEdit(t *testing.T) {
	block := Block("1.25.0")
	i := strings.Index(block, "shields.io")
	edited := block[:i] + "shield5.io" + block[i+len("shields.io"):]
	if edited == block {
		t.Fatal("edit did not change the block")
	}
	readme, err := Replace("intro\n"+block+"\nrest\n", edited)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Extract(readme)
	if err != nil {
		t.Fatal(err)
	}
	if got == block {
		t.Fatal("a one-character edit inside the block compares equal to the generator output")
	}
}

// TestReadmeGoVersionBadgeMatchesGoMod asserts the Go-version badge in the
// committed README carries the go.mod "go" directive.
func TestReadmeGoVersionBadgeMatchesGoMod(t *testing.T) {
	want := goModVersion(t)
	readme := readFile(t, filepath.Join(repositoryRoot(t), "README.md"))
	block, err := Extract(readme)
	if err != nil {
		t.Fatal(err)
	}
	var found string
	for _, line := range strings.Split(block, "\n") {
		if strings.HasPrefix(line, "[![Go version](") {
			found = line
		}
	}
	if found == "" {
		t.Fatal("no Go version badge in the README block")
	}
	image, _, _ := strings.Cut(strings.SplitN(found, "](", 3)[1], ")")
	const prefix = "https://img.shields.io/badge/go-"
	if !strings.HasPrefix(image, prefix) {
		t.Fatalf("Go version badge image %q does not start with %s", image, prefix)
	}
	rest := strings.TrimPrefix(image, prefix)
	version := rest[:strings.LastIndex(rest, "-")]
	if version != want {
		t.Fatalf("Go version badge says %q, go.mod says %q", version, want)
	}
}

// TestBadgeBlockContent pins the block's make-up: the workflow badge on
// main, the five gate endpoints, and the three static badges, each linked.
func TestBadgeBlockContent(t *testing.T) {
	block := Block("1.25.0")
	lines := strings.Split(block, "\n")
	if len(lines) != 11 || lines[0] != StartMarker || lines[10] != EndMarker {
		t.Fatalf("unexpected block shape:\n%s", block)
	}
	for _, want := range []string{
		"actions/workflows/ci.yml/badge.svg?branch=main",
		"https%3A%2F%2Fn-orlov.github.io%2Fdeck%2Fbadges%2Flint.json",
		"badges%2Funit-tests.json", "badges%2Ffeatures.json",
		"badges%2Fcoverage.json", "badges%2Fvulnerabilities.json",
		"img.shields.io/badge/license-", "img.shields.io/badge/go-1.25.0-",
		"img.shields.io/badge/report-",
	} {
		if !strings.Contains(block, want) {
			t.Errorf("block lacks %q", want)
		}
	}
	for _, line := range lines[1:10] {
		if !strings.HasPrefix(line, "[![") || !strings.HasSuffix(line, ")") || strings.Count(line, "](") != 2 {
			t.Errorf("badge line is not an image wrapped in a link: %q", line)
		}
	}
}

// TestEveryBadgeLinksToWorkflowOrReport pins R291's link rule: each badge in
// the block links either to the ci.yml workflow's runs on main or to the
// published report, and nowhere else.
func TestEveryBadgeLinksToWorkflowOrReport(t *testing.T) {
	block := Block("1.25.0")
	lines := strings.Split(block, "\n")
	allowed := map[string]bool{
		"https://github.com/n-orlov/deck/actions/workflows/ci.yml?query=branch%3Amain": true,
		"https://n-orlov.github.io/deck/":                                              true,
	}
	for _, line := range lines[1 : len(lines)-1] {
		i := strings.LastIndex(line, "](")
		if i < 0 || !strings.HasSuffix(line, ")") {
			t.Fatalf("badge line has no link: %q", line)
		}
		link := line[i+2 : len(line)-1]
		if !allowed[link] {
			t.Errorf("badge links to %q, neither the ci.yml workflow nor the report: %q", link, line)
		}
	}
}

func TestGoVersionParsing(t *testing.T) {
	if v, err := GoVersion("module x\n\ngo 1.25.0\n\ntoolchain go1.26.9\n"); err != nil || v != "1.25.0" {
		t.Fatalf("got %q, %v", v, err)
	}
	if _, err := GoVersion("module x\n"); err == nil {
		t.Fatal("missing go directive accepted")
	}
}

func TestRunRewritesAndInserts(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module x\n\ngo 1.22.1\n")
	write("README.md", "# x\n")
	for i := 0; i < 2; i++ { // second run must be a no-op
		if err := run(dir, false); err != nil {
			t.Fatal(err)
		}
	}
	got := readFile(t, filepath.Join(dir, "README.md"))
	if want := Block("1.22.1") + "\n\n# x\n"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	write("go.mod", "module x\n\ngo 1.23.0\n")
	if err := run(dir, false); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(dir, "README.md")); !strings.Contains(got, "go-1.23.0-") || strings.Contains(got, "1.22.1") {
		t.Fatalf("block not regenerated: %q", got)
	}
	if err := run(dir, true); err != nil {
		t.Fatal(err)
	}
	if err := run(filepath.Join(dir, "missing"), false); err == nil {
		t.Fatal("missing go.mod accepted")
	}
	write("go.mod", "module x\n")
	if err := run(dir, false); err == nil {
		t.Fatal("go.mod without go directive accepted")
	}
	if _, err := Replace("no markers", "x"); err == nil {
		t.Fatal("Replace without markers accepted")
	}
}
