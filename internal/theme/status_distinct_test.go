package theme

import (
	"fmt"
	"strings"
	"testing"
)

// minStatusDeltaE is SPEC §11.6's status-distinguishability floor: the
// CIEDE2000 distance between any two of a theme's seven §7 status colours.
const minStatusDeltaE = 10.0

// statusDistinctnessFailures returns one message per pair of the theme's
// StatusTokens whose CIEDE2000 ΔE is below minStatusDeltaE, each naming the
// theme, the two tokens and their colours.
func statusDistinctnessFailures(th *Theme) ([]string, error) {
	var out []string
	for i := 0; i < len(StatusTokens); i++ {
		for j := i + 1; j < len(StatusTokens); j++ {
			hi, err := th.Color(StatusTokens[i])
			if err != nil {
				return nil, err
			}
			hj, err := th.Color(StatusTokens[j])
			if err != nil {
				return nil, err
			}
			d, err := hexDeltaE(hi, hj)
			if err != nil {
				return nil, err
			}
			if d < minStatusDeltaE {
				out = append(out, fmt.Sprintf("theme %q: status %s (%s) and %s (%s) have CIEDE2000 ΔE %.2f < %.0f",
					th.Name, StatusTokens[i], hi, StatusTokens[j], hj, d, minStatusDeltaE))
			}
		}
	}
	return out, nil
}

// TestBuiltinStatusColoursAreDistinguishable is R211b: in every registered
// built-in, the seven §7 status colours are pairwise at least
// minStatusDeltaE apart in CIEDE2000, because the colour is the fastest
// thing a human reads in the list. It iterates the registry, so a later
// built-in is held to the same floor with no new test.
func TestBuiltinStatusColoursAreDistinguishable(t *testing.T) {
	all := Builtins()
	if len(all) == 0 {
		t.Fatal("registry holds no built-ins")
	}
	for _, th := range all {
		fails, err := statusDistinctnessFailures(th)
		if err != nil {
			t.Fatalf("theme %q: %v", th.Name, err)
		}
		for _, f := range fails {
			t.Error(f)
		}
	}
}

// collapsedTheme is a test-local copy of base whose idle and stopped status
// colours are set to the given values.
func collapsedTheme(base *Theme, name, idle, stopped string) *Theme {
	th := &Theme{Name: name, Appearance: base.Appearance, Colors: map[Token]string{}}
	for k, v := range base.Colors {
		th.Colors[k] = v
	}
	th.Colors[Idle] = idle
	th.Colors[Stopped] = stopped
	return th
}

// TestStatusDistinctnessRejectsCollapsedPalette proves the check bites: a
// test-local theme whose idle and stopped colours are equal, and another
// whose two colours are merely close, are both reported by name and pair,
// while a well-separated pair is not.
func TestStatusDistinctnessRejectsCollapsedPalette(t *testing.T) {
	base := Builtins()[0]
	for _, tc := range []struct {
		name, idle, stopped string
		wantFailure         bool
	}{
		{"equal", "#808080", "#808080", true},
		{"close", "#808080", "#838383", true},
		{"far", "#808080", "#d0d0d0", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			th := collapsedTheme(base, "collapsed-"+tc.name, tc.idle, tc.stopped)
			fails, err := statusDistinctnessFailures(th)
			if err != nil {
				t.Fatal(err)
			}
			if !tc.wantFailure {
				for _, f := range fails {
					if strings.Contains(f, "idle") && strings.Contains(f, "stopped") {
						t.Errorf("well-separated idle/stopped reported: %s", f)
					}
				}
				return
			}
			found := false
			for _, f := range fails {
				if strings.Contains(f, th.Name) && strings.Contains(f, "idle") && strings.Contains(f, "stopped") {
					found = true
				}
			}
			if !found {
				t.Errorf("collapsed idle/stopped pair not reported for %s; got %q", th.Name, fails)
			}
		})
	}
}
