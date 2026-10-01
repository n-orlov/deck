package features

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// This file holds the durable observable an AGENT session's own entry into
// "starting" is synchronised on (task 026, steer 002), replacing every
// frame wait on the lone transient word "starting" an agent create or
// resume used to end with.
//
// SPEC §7 promotes an agent row out of "starting" on its first agent signal
// and moves ANY row to "stopped" on a clean pane exit (or "error" on a
// non-zero one). The plain fake-claude fixture exits 0 about half a second
// after its own banner (installFakeClaudeOnPATH's lingering wrapper), and a
// failing pre_launch kills its pane at once, so the sidebar's "starting"
// render of a just-created or just-resumed agent row is transient exactly
// the way a shell row's is (inventory mechanism M1): ScreenDriver only
// samples the frame when d.updated fires, so a "starting" render that the
// next reconcile overwrites inside one pty read is never observed, and a
// wait for it alone runs out the full default deadline although the
// session behaved correctly. The audit log's own "starting" transition
// (internal/service CreateAgent/Resume write it right after the durable
// row, before the pane is launched) is never overwritten, so it is the
// durable observable used here instead.

// auditRecordCount is the number of records the audit log holds right now,
// 0 when it cannot be read yet. A create helper takes it BEFORE submitting,
// so the wait below only accepts a "starting" record written by that
// submission -- never an earlier record of a tombstoned row that held the
// same name (SPEC §9.2 name reuse).
func auditRecordCount(h *ScenarioHarness) int {
	records, err := readAudit(h)
	if err != nil {
		return 0
	}
	return len(records)
}

// auditSessionEventCount counts records[from:] whose event is event and
// whose session_id is sessionID.
func auditSessionEventCount(records []map[string]json.RawMessage, from int, sessionID, event string) int {
	if from < 0 {
		from = 0
	}
	count := 0
	for i := from; i < len(records); i++ {
		var got, id string
		_ = json.Unmarshal(records[i]["event"], &got)
		_ = json.Unmarshal(records[i]["session_id"], &id)
		if got == event && id == sessionID {
			count++
		}
	}
	return count
}

// waitForAgentCreateRecorded is the agent create helpers' durable sync
// point: it waits until the audit log holds a "starting" transition for the
// session now named name, written after auditOffset (the create this step
// just submitted entered "starting"), and then until client's sidebar shows
// that session's row in ANY status glyph (the TUI has caught up with the
// create, whatever the row has moved on to since).
func waitForAgentCreateRecorded(ctx context.Context, h *ScenarioHarness, client *ScreenDriver, name string, auditOffset int) error {
	ctx, cancel := withDefaultWaitDeadline(ctx)
	defer cancel()
	for {
		id, err := sessionIDByName(h, name)
		if err == nil {
			if records, rerr := readAudit(h); rerr == nil && auditSessionEventCount(records, auditOffset, id, "starting") > 0 {
				break
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("audit log never recorded agent session %q entering starting after the create was submitted: %w", name, ctx.Err())
		case <-time.After(20 * time.Millisecond):
		}
	}
	rowShown := func(frame string) bool {
		_, ok := frameSidebarRowGlyph(frame, name)
		return ok
	}
	frame, err := client.WaitForFrameFunc(ctx, false, rowShown)
	if err != nil {
		return fmt.Errorf("session %q recorded starting but its sidebar row never rendered: %w\nframe:\n%s", name, err, frame)
	}
	return nil
}

// auditRecordsSessionEnteringStartingNTimes is the Gherkin form of the same
// observable, for a scenario that used to follow a create/resume with
// `screen contains "starting"`: it polls (inside the default UI deadline)
// until the audit log holds exactly want "starting" transitions for the
// session, and as many "launch.ready" ones -- the launch each of those
// starts began was recorded too (CreateAgent/Resume write the launch record
// before launch.ready), so a following one-shot launch-record assertion
// reads a settled log. It fails at once if either count overshoots want.
func auditRecordsSessionEnteringStartingNTimes(ctx context.Context, name string, want int) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	ctx, cancel := withDefaultWaitDeadline(ctx)
	defer cancel()
	starting, ready := -1, -1
	var lastErr error
	for {
		id, err := sessionIDByName(h, name)
		if err == nil {
			var records []map[string]json.RawMessage
			records, err = readAudit(h)
			if err == nil {
				starting = auditSessionEventCount(records, 0, id, "starting")
				ready = auditSessionEventCount(records, 0, id, "launch.ready")
				if starting > want || ready > want {
					return fmt.Errorf("audit log records session %q entering starting %d times (launch.ready %d), want exactly %d", name, starting, ready, want)
				}
				if starting == want && ready == want {
					return nil
				}
			}
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return fmt.Errorf("audit log records session %q entering starting %d times (launch.ready %d), want %d (last read error: %v): %w", name, starting, ready, want, lastErr, ctx.Err())
		case <-time.After(20 * time.Millisecond):
		}
	}
}
