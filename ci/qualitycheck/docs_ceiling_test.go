package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
)

// docsCeilingRE matches the sentence in docs/ci.md that states the CRAP
// ceiling ci/quality.json holds.
var docsCeilingRE = regexp.MustCompile(`\*\*CRAP ceiling (\d+)\*\*`)

// statedCrapCeiling extracts the ceiling docs/ci.md states.
func statedCrapCeiling(doc string) (int, bool) {
	m := docsCeilingRE.FindStringSubmatch(doc)
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	return n, err == nil
}

// TestDocsCiMdStatesTheCheckedInCrapCeiling fails when the CRAP ceiling
// stated in docs/ci.md differs from ci/quality.json.
func TestDocsCiMdStatesTheCheckedInCrapCeiling(t *testing.T) {
	root := repoRoot(t)
	cfg, err := loadConfig(filepath.Join(root, "ci", "quality.json"))
	if err != nil {
		t.Fatal(err)
	}
	doc, err := os.ReadFile(filepath.Join(root, "docs", "ci.md"))
	if err != nil {
		t.Fatal(err)
	}
	got, ok := statedCrapCeiling(string(doc))
	if !ok {
		t.Fatalf("docs/ci.md does not state the ceiling as **CRAP ceiling N**")
	}
	if float64(got) != cfg.Crap.Ceiling {
		t.Errorf("docs/ci.md states CRAP ceiling %d, ci/quality.json holds %v", got, cfg.Crap.Ceiling)
	}
}

// TestStatedCrapCeilingDetectsADifferentNumber seeds a stale document and
// requires the extracted number to differ from the real ceiling, proving
// the doc check above can fail.
func TestStatedCrapCeilingDetectsADifferentNumber(t *testing.T) {
	got, ok := statedCrapCeiling("holds at this commit: **CRAP ceiling 15**")
	if !ok || got != 15 {
		t.Fatalf("statedCrapCeiling = %d, %v; want 15, true", got, ok)
	}
	if _, ok := statedCrapCeiling("no ceiling stated"); ok {
		t.Fatal("a document without the statement must report not-found")
	}
}
