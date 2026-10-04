package features

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/vt"
	"github.com/n-orlov/deck/internal/racebuild"
)

// TestR150RowCallbackIgnoresGroupHeaderText (R150, cure-01-01-3) pins the
// runtime half of a review finding on cure-01-01-2's own fix: naming a
// group "alpha-running" while session "alpha" itself is still "starting"
// let the shipped named-wait callback (clientRowContainsWithinReconcile,
// and the two sibling callbacks that share its one frameSidebarRowContains
// helper -- clientRowContainsWithinThreeSeconds,
// clientRowContainsAcrossSeveralProbeCycles) accept the group HEADER's own
// rendered text ("alpha-running  (1)", groupHeaderText) as durable proof
// of alpha's own row, because the helper searched every sidebar cell
// without excluding header lines. A named wait must verify the NAMED
// SESSION's own status, never a group name, a header, footer or preview
// text, or another session's name (R150's whole guard, cure-01-01-2/3).
//
// Two controls isolate what actually decides the outcome, matching the
// review's own probe:
//   - rename_only_control renames the group to something neutral while
//     alpha stays "starting": the wait still fails, proving the rejection
//     above is not a side effect of some OTHER text the header happened to
//     carry -- the header is never consulted at all, whatever it says.
//   - actual_status_control leaves the misleading group name in place but
//     gives alpha its own real "running" status: the wait now passes,
//     proving the callback tracks alpha's own row, not the header.
func TestR150RowCallbackIgnoresGroupHeaderText(t *testing.T) {
	cases := []struct {
		name       string
		header     string
		row        string
		wantAccept bool
	}{
		{
			name:   "misleading_group_name_does_not_settle_a_starting_row",
			header: "v alpha-running (1)",
			row:    "> alpha starting",
		},
		{
			name:   "rename_only_control_still_rejects_the_starting_row",
			header: "v neutral-group (1)",
			row:    "> alpha starting",
		},
		{
			name:       "actual_status_control_accepts_the_running_row",
			header:     "v alpha-running (1)",
			row:        "> alpha running",
			wantAccept: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			frame := "| " + tc.header + " | preview |\r\n| " + tc.row + " | pane |"
			screen := vt.NewEmulator(100, 5)
			if _, err := screen.Write([]byte(frame)); err != nil {
				t.Fatalf("write frame to emulator: %v", err)
			}
			driver := &ScreenDriver{screen: screen}
			h := &ScenarioHarness{Home: t.TempDir(), namedClients: map[string]*ScreenDriver{"A": driver}}
			writeSyntheticSessionNames(t, h.Home, "alpha")
			ctx := context.WithValue(context.Background(), scenarioHarnessKey{}, h)
			err := clientRowContainsWithinReconcile(ctx, "A", "alpha", "running")
			t.Logf("frame=%q err=%v", driver.Frame(false), err)
			if tc.wantAccept && err != nil {
				t.Fatalf("named wait on alpha's own actual running row must pass: %v", err)
			}
			if !tc.wantAccept && err == nil {
				t.Fatalf("named wait must not be settled by group header/row text unrelated to alpha's own status")
			}
		})
	}
}

// TestR150LiveGroupedSidebarRowSettleProvesActualStatus (R150,
// cure-01-01-3) is TestR150RowCallbackIgnoresGroupHeaderText's live
// counterpart against the actual released binary, a real private tmux
// server and a real PTY, rather than a synthetic frame: a non-hooking
// fake "claude" agent holds "starting" durably (longRunningFakeClaude
// OnPATHForFutureClients) so the probe does not depend on a shell's own
// fast reconcile-timed promotion, and the row callback takes no agent-kind
// input anyway. Group "alpha-running" is assigned directly in the state
// database (setSessionGroup) so the real sidebar renders the exact
// collision under test.
func TestR150LiveGroupedSidebarRowSettleProvesActualStatus(t *testing.T) {
	h, err := newScenarioHarness(buildDeckBinary(t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Remove(h.Binary); err != nil {
			t.Logf("remove scenario binary: %v", err)
		}
	}()
	defer func() {
		if err := h.Close(); err != nil {
			t.Error(err)
		}
	}()
	// R164 cure (cure-01-01): under -race, the base and rename_only_control
	// calls to clientRowContainsWithinReconcile below are each expected to
	// REJECT, burning the full race-widened reconcileIntervalPollDeadline
	// (task 016) before returning; widen this outer context to accommodate
	// both, on top of the unchanged 15s normal-build budget.
	ctx, cancel := context.WithTimeout(context.Background(), r150LiveOuterDeadline(15*time.Second, 2, racebuild.Enabled))
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
	must(setSessionGroup(ctx, "alpha", "alpha-running"))
	must(client.WaitForFrame(ctx, false, "alpha-running"))

	frame := client.Frame(false)
	actualStarting := false
	for _, line := range strings.Split(frame, "\n") {
		if cell, ok := sidebarCell(line); ok && strings.Contains(cell, "alpha") && strings.Contains(cell, "starting") {
			actualStarting = true
		}
	}
	if !actualStarting {
		t.Fatalf("probe needs alpha actually starting, frame:\n%s", frame)
	}

	db, err := openObservedDatabase(h)
	must(err)
	defer db.Close()

	// Base case: the misleading group header must not settle the wait
	// while alpha's own row still reads "starting".
	err = clientRowContainsWithinReconcile(ctx, "A", "alpha", "running")
	var status string
	must(db.QueryRowContext(ctx, `SELECT status FROM sessions WHERE name = 'alpha'`).Scan(&status))
	t.Logf("real grouped TUI frame before wait:\n%s\ncallback err=%v actual store status=%s", frame, err, status)
	if err == nil && status != "running" {
		t.Fatal("named running wait passed on the real group header, not alpha's own actually-starting row")
	}
	if status != "starting" {
		t.Fatalf("probe lost its durable starting control: %s", status)
	}

	// rename_only_control: renaming the group away from the misleading
	// name changes nothing about alpha's own status, so the wait must
	// still fail -- proving the base case's rejection is not a side
	// effect of some other header text, because the header is never
	// consulted in the first place.
	_, err = db.ExecContext(ctx, `UPDATE groups SET name='neutral' WHERE name='alpha-running'`)
	must(err)
	must(client.WaitForFrameGone(ctx, false, "alpha-running"))
	if e := clientRowContainsWithinReconcile(ctx, "A", "alpha", "running"); e == nil {
		t.Fatal("rename_only_control: named wait must still fail with a neutral header and alpha's row still starting")
	}
	t.Log("rename_only_control: renaming the group to neutral leaves the named-running wait failing, exactly like the misleading name did")

	// actual_status_control: giving alpha its own real "running" status
	// (misleading group name restored is unnecessary; the neutral one
	// already proves the header carries no weight) makes the wait pass.
	_, err = db.ExecContext(ctx, `UPDATE sessions SET status='running' WHERE name='alpha'`)
	must(err)
	must(clientRowContainsWithinReconcile(ctx, "A", "alpha", "running"))
	t.Log("actual_status_control: alpha's own real running status makes the named-running wait pass")
}
