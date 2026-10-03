package main

import (
	"math"
	"testing"
)

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

// TestCrapOf_Formula pins PRD R187's locked formula:
// CRAP = cc^2 * (1 - cov)^3 + cc.
func TestCrapOf_Formula(t *testing.T) {
	cases := []struct {
		name string
		cc   int
		cov  float64
		want float64
	}{
		{"zero-statement function at 100% coverage: CRAP collapses to cc", 1, 1.0, 1},
		{"cc=10 at 0% coverage", 10, 0.0, 110}, // 10^2*1^3+10 = 110
		{"cc=5 at 100% coverage: CRAP == cc", 5, 1.0, 5},
		{"cc=5 at 80% coverage", 5, 0.8, 5.2},   // 25*0.2^3 + 5 = 25*0.008+5 = 5.2
		{"cc=3 at 50% coverage", 3, 0.5, 4.125}, // 9*0.5^3 + 3 = 9*0.125+3 = 4.125
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := crapOf(tc.cc, tc.cov)
			if !almostEqual(got, tc.want) {
				t.Errorf("crapOf(cc=%d, cov=%v) = %v, want %v", tc.cc, tc.cov, got, tc.want)
			}
		})
	}
}

// TestCoverageOf_ZeroStatementFunctionIsAlways100Percent pins PRD R187's
// "a zero-statement function scored at 100% coverage" rule: it holds
// even when the profile has NO blocks at all for that file (which, for
// any function with actual statements, is the "absent from profile"
// 0%-coverage case instead -- the empty-body rule must win for an empty
// function, not that one).
func TestCoverageOf_ZeroStatementFunctionIsAlways100Percent(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir+"/go.mod", "module example.com/zerostmt\n\ngo 1.21\n")
	writeFile(t, dir+"/pkg.go", "package zerostmt\n\nfunc Empty() {\n}\n")

	fns, err := scanFunctions(dir, "")
	if err != nil {
		t.Fatalf("scanFunctions: %v", err)
	}
	if len(fns) != 1 {
		t.Fatalf("scanFunctions found %d functions, want 1", len(fns))
	}

	emptyProfile := &profile{blocks: map[string][]coverBlock{}}
	cov := coverageOf(fns[0], "example.com/zerostmt", dir, emptyProfile)
	if cov != 1.0 {
		t.Errorf("coverageOf(zero-statement function, empty profile) = %v, want 1.0 (100%%)", cov)
	}

	cc := cyclomaticComplexity(fns[0].Decl)
	crap := crapOf(cc, cov)
	if crap != float64(cc) {
		t.Errorf("CRAP of a zero-statement function = %v, want exactly cc (%d) at 100%% coverage", crap, cc)
	}
}
