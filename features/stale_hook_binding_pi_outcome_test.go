package features

import "testing"

// The fake pi prints "hook failed" and its "notify" line separately, so a pane
// capture between the two must not count as a finished hook: the step would
// otherwise record a failure with an empty message (dispatch run 37744422828).
func TestFakePiHookOutcomeWaitsForTheNotifyLineOfAFailedHook(t *testing.T) {
	const fired, failed = "fake-pi hook fired: SessionStart", "fake-pi hook failed: SessionStart"
	before := "fake-pi extension: x\n"
	cases := []struct {
		name, output, notice string
		done, hookFailed     bool
	}{
		{"nothing yet", before, "", false, false},
		{"failed line only", before + failed + "\n", "", false, true},
		{"failed with notify", before + failed + "\nfake-pi notify: restart it\n\n", "restart it", true, true},
		{"fired", before + fired + "\n", "", true, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			done, notice, hookFailed := fakePiHookOutcome(before, c.output, fired, failed)
			if done != c.done || notice != c.notice || hookFailed != c.hookFailed {
				t.Fatalf("outcome = (%v, %q, %v), want (%v, %q, %v)", done, notice, hookFailed, c.done, c.notice, c.hookFailed)
			}
		})
	}
}
