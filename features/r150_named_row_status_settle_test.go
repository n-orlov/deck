package features

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/vt"
)

// r150NamedStatusCallbacks is every registered named status callback -- the
// three step texts ending `row "<name>" contains "<want>"` -- all of which
// must settle on the requested session's OWN status and nothing else.
var r150NamedStatusCallbacks = []struct {
	name string
	fn   func(context.Context, string, string, string) error
}{
	{"within_reconcile", clientRowContainsWithinReconcile},
	{"within_three_seconds", clientRowContainsWithinThreeSeconds},
	{"across_probe_cycles", clientRowContainsAcrossSeveralProbeCycles},
}

// writeSyntheticSessionNames creates a minimal state database under home
// holding just the sessions.name column the named callbacks read to
// disambiguate rows, so a synthetic-frame callback test exercises the same
// store lookup a live scenario does.
func writeSyntheticSessionNames(t *testing.T, home string, names ...string) {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE sessions(name TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	for _, n := range names {
		if _, err := db.Exec(`INSERT INTO sessions(name) VALUES(?)`, n); err != nil {
			t.Fatal(err)
		}
	}
}

// TestR150NamedCallbacksVerifyTheRequestedSessionsOwnStatus (R150,
// cure-01-01-3) pins the review's negative probe on cure-01-01-3's first
// attempt: every named callback matched the requested name and the wanted
// word as two independent SUBSTRINGS of one sidebar cell, so
//   - another session's name: row "alpha" (still starting) was settled as
//     running by the neighbouring row "alpha-helper running";
//   - the name itself: row "running-alpha" (still starting) was settled as
//     running by its own name's text;
//   - a space-bearing name: session "alpha sampled" (status running)
//     renders exactly like session "alpha" with quality sampled and status
//     running, so only the store's names can say whose row it is.
//
// Each negative has two controls, matching the review's own shape:
// rename_only renames the misleading OTHER text to something neutral while
// the requested session keeps its real "starting" status (the wait still
// fails -- the misleading text was never what decided it), and
// actual_status gives the requested session its own real target status
// (the wait passes -- the callback tracks that row's own status).
func TestR150NamedCallbacksVerifyTheRequestedSessionsOwnStatus(t *testing.T) {
	cases := []struct {
		name       string
		rows       []string
		names      []string
		rowName    string
		want       string
		wantAccept bool
	}{
		{
			name:    "another_session_name_does_not_settle_a_starting_row",
			rows:    []string{"v default (2)", "> alpha starting", "  1s ago", "  alpha-helper running", "  1s ago"},
			names:   []string{"alpha", "alpha-helper"},
			rowName: "alpha", want: "running",
		},
		{
			name:    "another_session_name_rename_only_control",
			rows:    []string{"v default (2)", "> alpha starting", "  1s ago", "  neutral-helper running", "  1s ago"},
			names:   []string{"alpha", "neutral-helper"},
			rowName: "alpha", want: "running",
		},
		{
			name:    "another_session_name_actual_status_control",
			rows:    []string{"v default (2)", "> alpha running", "  1s ago", "  alpha-helper running", "  1s ago"},
			names:   []string{"alpha", "alpha-helper"},
			rowName: "alpha", want: "running", wantAccept: true,
		},
		{
			name:    "name_itself_containing_the_status_does_not_settle",
			rows:    []string{"v default (1)", "> running-alpha starting", "  1s ago"},
			names:   []string{"running-alpha"},
			rowName: "running-alpha", want: "running",
		},
		{
			name:    "name_itself_rename_only_control",
			rows:    []string{"v default (1)", "> neutral-alpha starting", "  1s ago"},
			names:   []string{"neutral-alpha"},
			rowName: "neutral-alpha", want: "running",
		},
		{
			name:    "name_itself_actual_status_control",
			rows:    []string{"v default (1)", "> running-alpha running", "  1s ago"},
			names:   []string{"running-alpha"},
			rowName: "running-alpha", want: "running", wantAccept: true,
		},
		{
			name:    "space_bearing_other_name_does_not_settle_a_starting_row",
			rows:    []string{"v default (2)", "> alpha starting", "  1s ago", "  alpha sampled running", "  1s ago"},
			names:   []string{"alpha", "alpha sampled"},
			rowName: "alpha", want: "running",
		},
		{
			name:    "space_bearing_other_name_rename_only_control",
			rows:    []string{"v default (2)", "> alpha starting", "  1s ago", "  neutral sampled running", "  1s ago"},
			names:   []string{"alpha", "neutral sampled"},
			rowName: "alpha", want: "running",
		},
		{
			name:    "space_bearing_other_name_actual_status_control",
			rows:    []string{"v default (2)", "> alpha sampled running", "  1s ago", "  alpha sampled running", "  1s ago"},
			names:   []string{"alpha", "alpha sampled"},
			rowName: "alpha sampled", want: "running", wantAccept: true,
		},
		{
			name:    "quality_word_on_the_requested_row_is_its_own_badge",
			rows:    []string{"v default (1)", "> alpha live running", "  1s ago"},
			names:   []string{"alpha"},
			rowName: "alpha", want: "live", wantAccept: true,
		},
		{
			name:    "ellipsised_row_keeps_its_complete_quality_badge",
			rows:    []string{"v default (1)", "> codex-purge-remove live run...", "  1s ago"},
			names:   []string{"codex-purge-remove"},
			rowName: "codex-purge-remove", want: "live", wantAccept: true,
		},
		{
			name:    "ellipsised_status_never_settles_a_status_wait",
			rows:    []string{"v default (1)", "> codex-purge-remove live run...", "  1s ago"},
			names:   []string{"codex-purge-remove"},
			rowName: "codex-purge-remove", want: "running",
		},
		{
			name:    "unseen_glyph_on_the_requested_row_is_its_own_badge",
			rows:    []string{"v default (1)", "> failed inline ! error", "  1s ago"},
			names:   []string{"failed inline"},
			rowName: "failed inline", want: "!", wantAccept: true,
		},
	}

	for _, cb := range r150NamedStatusCallbacks {
		cb := cb
		t.Run(cb.name, func(t *testing.T) {
			t.Parallel()
			for _, tc := range cases {
				tc := tc
				t.Run(tc.name, func(t *testing.T) {
					t.Parallel()
					var lines []string
					for _, r := range tc.rows {
						lines = append(lines, "| "+r+strings.Repeat(" ", 34-len([]rune(r)))+"| pane running |")
					}
					frame := strings.Join(lines, "\r\n")
					screen := vt.NewEmulator(80, len(lines)+1)
					screen.Write([]byte(frame))
					driver := &ScreenDriver{screen: screen}
					h := &ScenarioHarness{Home: t.TempDir(), namedClients: map[string]*ScreenDriver{"A": driver}}
					writeSyntheticSessionNames(t, h.Home, tc.names...)
					ctx := context.WithValue(context.Background(), scenarioHarnessKey{}, h)
					err := cb.fn(ctx, "A", tc.rowName, tc.want)
					if tc.wantAccept && err != nil {
						t.Fatalf("callback %s must settle on row %q's own %q badge: %v", cb.name, tc.rowName, tc.want, err)
					}
					if !tc.wantAccept && err == nil {
						t.Fatalf("FALSE SETTLE: callback %s accepted row %q as %q from text that is not that session's own status; frame:\n%s", cb.name, tc.rowName, tc.want, driver.Frame(false))
					}
				})
			}
		})
	}
}

// TestR150SidebarRowBadgesParsesOnlyTheRenderedBadgeRun (R150,
// cure-01-01-3) pins the grammar frameSessionRowShows relies on: a row's
// first line is "<name> [unseen] [quality] <status> [archived]" and
// nothing else, so a name's own words, a truncated row, or extra text
// never parse as a status.
func TestR150SidebarRowBadgesParsesOnlyTheRenderedBadgeRun(t *testing.T) {
	cases := []struct {
		text, name string
		ok         bool
	}{
		{"alpha running", "alpha", true},
		{"alpha live running", "alpha", true},
		{"alpha \u25cf sampled waiting \u25a3", "alpha", true},
		{"alpha ! error [archived]", "alpha", true},
		{"alpha-helper running", "alpha", false},
		{"running-alpha starting", "alpha", false},
		{"alpha running starting", "alpha", false},
		{"alpha runn\u2026", "alpha", true},
		{"alpha live run...", "alpha", true},
		{"alpha xyz...", "alpha", false},
		{"alpha running-alpha...", "alpha", false},
		{"alpha running extra", "alpha", false},
		{"alpha live", "alpha", false},
		{"alpha", "alpha", false},
	}
	for _, tc := range cases {
		if _, ok := sidebarRowBadges(tc.text, tc.name); ok != tc.ok {
			t.Errorf("sidebarRowBadges(%q, %q) ok=%v, want %v", tc.text, tc.name, ok, tc.ok)
		}
	}
	// A truncated row's cut field is never a badge: an ellipsised status
	// cannot settle a status wait, while the complete quality badge before
	// it still reads as the row's own.
	if badges, _ := sidebarRowBadges("alpha live run...", "alpha"); len(badges) != 1 || badges[0] != "live" {
		t.Errorf("truncated row badges = %q, want [live]", badges)
	}
	if badges, _ := sidebarRowBadges("alpha runn\u2026", "alpha"); len(badges) != 0 {
		t.Errorf("truncated status must yield no badge, got %q", badges)
	}
}

// TestR150LiveNamedRowSettleIgnoresAnotherSessionsName (R150,
// cure-01-01-3) is the negative probe's live counterpart against the real
// binary, a private tmux server and a real PTY: session "alpha" (a
// non-hooking fake claude, so it stays "starting" durably) beside session
// "alpha-helper" whose own row reads "running". The named running wait on
// alpha must fail (base), still fail once alpha-helper is renamed to
// something neutral (rename_only_control), and pass once alpha's own
// status is running (actual_status_control).
func TestR150LiveNamedRowSettleIgnoresAnotherSessionsName(t *testing.T) {
	h, err := newScenarioHarness(buildDeckBinary(t))
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(h.Binary)
	defer func() {
		if err := h.Close(); err != nil {
			t.Error(err)
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	ctx = context.WithValue(ctx, scenarioHarnessKey{}, h)
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}

	must(longRunningFakeClaudeOnPATHForFutureClients(ctx))
	client, err := h.StartNamedClient(ctx, "A")
	must(err)
	defer func() {
		_ = client.Send("q")
		if err := client.Stop(3 * time.Second); err != nil {
			t.Error(err)
		}
	}()
	must(client.WaitForFrame(ctx, false, "No sessions"))
	must(clientCreatesAgentSessionWithProfile(ctx, "A", "claude", "alpha", "safe"))
	must(clientCreatesAgentSessionWithProfile(ctx, "A", "claude", "alpha-helper", "safe"))

	db, err := openObservedDatabase(h)
	must(err)
	defer db.Close()
	_, err = db.ExecContext(ctx, `UPDATE sessions SET status='running' WHERE name='alpha-helper'`)
	must(err)
	must(client.WaitForFrame(ctx, false, "alpha-helper running"))

	status := func() string {
		t.Helper()
		var s string
		must(db.QueryRowContext(ctx, `SELECT status FROM sessions WHERE name = 'alpha'`).Scan(&s))
		return s
	}
	if s := status(); s != "starting" {
		t.Fatalf("probe needs alpha durably starting, store says %q; frame:\n%s", s, client.Frame(false))
	}

	err = clientRowContainsWithinReconcile(ctx, "A", "alpha", "running")
	t.Logf("real frame:\n%s\nbase callback err=%v", client.Frame(false), err)
	if err == nil {
		t.Fatalf("FALSE SETTLE: named running wait on alpha passed on alpha-helper's row while alpha's own status is %q", status())
	}
	if s := status(); s != "starting" {
		t.Fatalf("probe lost its durable starting control: %s", s)
	}

	_, err = db.ExecContext(ctx, `UPDATE sessions SET name='neutral-helper' WHERE name='alpha-helper'`)
	must(err)
	must(client.WaitForFrame(ctx, false, "neutral-helper running"))
	if e := clientRowContainsWithinReconcile(ctx, "A", "alpha", "running"); e == nil {
		t.Fatal("rename_only_control: named wait must still fail while alpha's own row is starting")
	}

	_, err = db.ExecContext(ctx, `UPDATE sessions SET status='running' WHERE name='alpha'`)
	must(err)
	must(clientRowContainsWithinReconcile(ctx, "A", "alpha", "running"))
	t.Log("actual_status_control: alpha's own running status settles the named wait")
}
