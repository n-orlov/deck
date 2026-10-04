package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// checkedInCrap reads the CRAP gate's switch and ceilings from the real,
// checked-in ci/quality.json (the one place thresholds live).
func checkedInCrap(t *testing.T) (enabled bool, ceiling, fixtureCeiling float64) {
	t.Helper()
	raw, err := os.ReadFile("../quality.json")
	if err != nil {
		t.Fatalf("read ci/quality.json: %v", err)
	}
	var cfg struct {
		Crap struct {
			Enabled        bool    `json:"enabled"`
			Ceiling        float64 `json:"ceiling"`
			FixtureCeiling float64 `json:"fixture_ceiling"`
		} `json:"crap"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("parse ci/quality.json: %v", err)
	}
	return cfg.Crap.Enabled, cfg.Crap.Ceiling, cfg.Crap.FixtureCeiling
}

// TestCheckedInCrapGateIsOnAtThirty: R191's first stage -- the gate is on,
// at 30 for product and for the cmd/fake-* fixtures alike.
func TestCheckedInCrapGateIsOnAtThirty(t *testing.T) {
	enabled, ceiling, fixtureCeiling := checkedInCrap(t)
	if !enabled {
		t.Error("ci/quality.json: crap.enabled is false, want the gate on")
	}
	if ceiling != 30 || fixtureCeiling != 30 {
		t.Errorf("ci/quality.json: crap ceiling=%v fixture_ceiling=%v, want 30 and 30", ceiling, fixtureCeiling)
	}
}

// TestRun_SeededFunctionOverTheCheckedInCeilingFails seeds a function whose
// CRAP is over the checked-in ceiling (cc 6 at 0% coverage scores
// 6^2*1 + 6 = 42) beside a trivial one, and requires the gate at that
// ceiling to fail and name only the seeded function.
func TestRun_SeededFunctionOverTheCheckedInCeilingFails(t *testing.T) {
	_, ceiling, _ := checkedInCrap(t)
	dir := t.TempDir()
	writeFile(t, dir+"/go.mod", "module example.com/seeded\n\ngo 1.21\n")
	writeFile(t, dir+"/pkg/pkg.go", `package pkg

// Seeded has cc 6 and no test coverage: CRAP 42.
func Seeded(x int) int {
	if x == 1 {
		return 1
	}
	if x == 2 {
		return 2
	}
	if x == 3 {
		return 3
	}
	if x == 4 {
		return 4
	}
	if x == 5 {
		return 5
	}
	return 0
}

// Trivial has cc 1: CRAP 2 even uncovered.
func Trivial() int { return 1 }
`)
	profile := dir + "/cover.out"
	// One block elsewhere keeps the profile non-empty; every pkg function
	// is then absent from it and scores 0%.
	writeFile(t, profile, "mode: set\nexample.com/seeded/other/o.go:1.1,1.1 0 0\n")

	report, exitCode, err := runInDir(t, dir, profile, ceiling, "pkg")
	if err != nil {
		t.Fatalf("run: unexpected error: %v", err)
	}
	if exitCode != 1 {
		t.Fatalf("exitCode = %d, want 1 (Seeded scores 42 against a ceiling of %v):\n%s", exitCode, ceiling, report)
	}
	if !strings.Contains(report, "Seeded") {
		t.Errorf("report does not name Seeded:\n%s", report)
	}
	if strings.Contains(report, "Trivial") {
		t.Errorf("report names Trivial, which is under the ceiling:\n%s", report)
	}
}
