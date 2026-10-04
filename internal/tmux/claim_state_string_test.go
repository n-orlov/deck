package tmux

import "testing"

// TestClaimStateStringNamesEveryState pins the text test failures and log
// lines use for each ownership classification, and the numeric fallback for a
// value outside the defined states.
func TestClaimStateStringNamesEveryState(t *testing.T) {
	cases := []struct {
		state ClaimState
		want  string
	}{
		{ClaimUnset, "unset"},
		{ClaimForeignLive, "foreign-live"},
		{ClaimStillMine, "still-mine"},
		{ClaimState(99), "ClaimState(99)"},
	}
	for _, tc := range cases {
		if got := tc.state.String(); got != tc.want {
			t.Errorf("ClaimState(%d).String() = %q, want %q", int(tc.state), got, tc.want)
		}
	}
}
