package features

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/x/vt"
)

// TestRowStepsReadTheSessionsOwnSidebarRowNotTheFirstLineNamingIt (R421,
// status_attach.feature:51, extended build 71) pins the two per-row
// assertion steps `... row "<name>" still contains "<want>"` and
// `... row "<name>" does not contain "<unwanted>"`. Both used to inspect
// the first terminal LINE containing the session name. The preview panel
// shares every line with the sidebar, and the fake agent's pane echoes its
// launch command (`--name=<name>`) near the top, so that line decided the
// verdict instead of the row: the row showed "! live error" and the step
// said it "no longer contains \"!\"". Each case is one frame; side and pane
// are the two panels of one terminal line.
func TestRowStepsReadTheSessionsOwnSidebarRowNotTheFirstLineNamingIt(t *testing.T) {
	type line struct{ side, pane string }
	steps := []struct {
		name string
		fn   func(context.Context, string, string, string) error
		// positive: the step wants the badge present (still contains)
		positive bool
	}{
		{"still_contains", clientRowStillContains, true},
		{"does_not_contain", clientRowDoesNotContain, false},
	}
	cases := []struct {
		name   string
		lines  []line
		names  []string
		row    string
		badge  string
		onRow  bool // the row's own badge run holds badge
		noRow  bool // the sidebar has no row of that name at all
		reason string
	}{
		{
			// The finding's own frame: the pane's launch command line is
			// ABOVE the row and has no "!" at its sidebar column.
			name: "pane_echo_above_the_row_with_the_badge_on_the_row",
			lines: []line{
				{"socket: deck_test", `a54206d41f","--name=failed inline","--settings"`},
				{"v default (1)", "fake-claude hook fired"},
				{"> x failed inline ! live error", ""},
			},
			names: []string{"failed inline"}, row: "failed inline", badge: "!", onRow: true,
		},
		{
			// Unlisted: the pane echoes name AND the badge above a row that
			// lost it; the old first-line read passed this wrongly.
			name: "pane_echo_with_the_badge_above_a_row_that_lacks_it",
			lines: []line{
				{"socket: deck_test", `--name=failed inline ! seen`},
				{"v default (1)", ""},
				{"> x failed inline live error", ""},
			},
			names: []string{"failed inline"}, row: "failed inline", badge: "!", onRow: false,
		},
		{
			// Unlisted: status word echoed by the pane above a starting row.
			name: "pane_echo_of_a_status_word_above_a_row_in_another_status",
			lines: []line{
				{"socket: deck_test", `unsignalled agent running`},
				{"v default (1)", ""},
				{"> o unsignalled agent starting", ""},
			},
			names: []string{"unsignalled agent"}, row: "unsignalled agent", badge: "running", onRow: false,
		},
		{
			// Unlisted: another row sharing the name's prefix is not this row.
			name: "a_longer_named_sibling_does_not_own_the_row",
			lines: []line{
				{"v default (2)", ""},
				{"> x two live error", "pane two ! text"},
				{"  o two sampled waiting", ""},
			},
			names: []string{"two", "two sampled"}, row: "two", badge: "waiting", onRow: false,
		},
		{
			name: "the_name_only_in_the_pane_is_no_row",
			lines: []line{
				{"v default (0)", `--name=failed inline ! live error`},
			},
			names: []string{"failed inline"}, row: "failed inline", badge: "!", noRow: true,
		},
	}
	for _, st := range steps {
		for _, tc := range cases {
			t.Run(st.name+"/"+tc.name, func(t *testing.T) {
				var out []string
				for _, l := range tc.lines {
					out = append(out, "| "+l.side+strings.Repeat(" ", 34-len([]rune(l.side)))+"| "+l.pane+" |")
				}
				screen := vt.NewEmulator(120, len(out)+1)
				if _, err := screen.Write([]byte(strings.Join(out, "\r\n"))); err != nil {
					t.Fatal(err)
				}
				driver := &ScreenDriver{screen: screen}
				h := &ScenarioHarness{Home: t.TempDir(), namedClients: map[string]*ScreenDriver{"A": driver}}
				writeSyntheticSessionNames(t, h.Home, tc.names...)
				ctx := context.WithValue(context.Background(), scenarioHarnessKey{}, h)
				err := st.fn(ctx, "A", tc.row, tc.badge)
				wantErr := tc.noRow || (st.positive && !tc.onRow) || (!st.positive && tc.onRow)
				if wantErr && err == nil {
					t.Fatalf("%s accepted row %q for badge %q on frame:\n%s", st.name, tc.row, tc.badge, driver.Frame(false))
				}
				if !wantErr && err != nil {
					t.Fatalf("%s rejected row %q for badge %q: %v", st.name, tc.row, tc.badge, err)
				}
			})
		}
	}
}
