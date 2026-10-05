package features

import (
	"reflect"
	"testing"
)

// TestDeckBuildArgsFollowsTheUnitPassCoverMode pins the nightly's coverage
// merge: the black-box deck binary is built with the unit pass's covermode
// (atomic under -race), because `go tool covdata textfmt` refuses a merge of
// set and atomic counter data and the merged profile came out empty.
func TestDeckBuildArgsFollowsTheUnitPassCoverMode(t *testing.T) {
	cases := []struct {
		name                string
		coverDir, coverMode string
		want                []string
	}{
		{"no coverage: the plain build", "", "", []string{"build", "-o", "/b", "/r/cmd/deck"}},
		{"cover dir only: -cover, default mode", "/cov", "", []string{"build", "-o", "/b", "-cover", "/r/cmd/deck"}},
		{"cover dir and set", "/cov", "set", []string{"build", "-o", "/b", "-cover", "-covermode=set", "/r/cmd/deck"}},
		{"cover dir and atomic (the -race nightly)", "/cov", "atomic", []string{"build", "-o", "/b", "-cover", "-covermode=atomic", "/r/cmd/deck"}},
		{"a mode without a cover dir is ignored", "", "atomic", []string{"build", "-o", "/b", "/r/cmd/deck"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := deckBuildArgs("/b", "/r", tc.coverDir, tc.coverMode)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("deckBuildArgs = %q, want %q", got, tc.want)
			}
		})
	}
}
