package features

import (
	"context"
	"fmt"

	"github.com/cucumber/godog"
)

// registerLaunchInputsEditorSteps exposes task 023's `l` launch-inputs
// editor (SPEC §6.2/§11.4, PRD R108) for task 027's own scenario: opening
// it for a named live session (reachable only from inside `i` detail,
// exactly like rename), typing into its pre-launch field, submitting it
// (which returns to detailView, not all the way out, mirroring rename's
// own submit), closing detail back to the main list, and reading the
// sessions.launch_dirty column directly -- the same durable flag the
// sidebar's `launch↻` badge (task 025) and `R` restart's apply-and-clear
// path (internal/service/restart.go) both read and write.
func registerLaunchInputsEditorSteps(sc *godog.ScenarioContext) {
	sc.Step(`^deck client "([^"]+)" opens the launch inputs editor for session "([^"]+)"$`, clientOpensLaunchInputsEditorForSession)
	sc.Step(`^deck client "([^"]+)" types "([^"]*)" into the pre-launch field$`, clientTypesIntoLaunchInputsPreLaunchField)
	sc.Step(`^deck client "([^"]+)" submits the launch inputs editor$`, clientSubmitsLaunchInputsEditor)
	sc.Step(`^deck client "([^"]+)" closes detail$`, clientClosesDetail)
	sc.Step(`^the state database session "([^"]+)" is marked launch_dirty$`, sessionIsMarkedLaunchDirty)
	sc.Step(`^the state database session "([^"]+)" is not marked launch_dirty$`, sessionIsNotMarkedLaunchDirty)
}

// clientOpensLaunchInputsEditorForSession opens `i` detail for the named
// session (clientOpensDetailForSession, navigation_settle_test.go) and
// then sends "l" (rename.go's own load-bearing key, the one and only
// place that ever sets m.launchInputsEditing), waiting for
// launchInputsBody's own "Launch inputs for <name>" title to render.
func clientOpensLaunchInputsEditorForSession(ctx context.Context, clientName, sessionName string) error {
	if err := clientOpensDetailForSession(ctx, clientName, sessionName); err != nil {
		return err
	}
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(clientName)
	if err != nil {
		return err
	}
	if err := client.Send("l"); err != nil {
		return err
	}
	return client.WaitForFrame(ctx, false, "Launch inputs for "+sessionName)
}

// clientTypesIntoLaunchInputsPreLaunchField sends value's runes straight
// into the currently-focused field. The editor always opens with field 0
// (Pre-launch command) focused (rename.go's "l" case sets
// m.launchInputsField = 0), and unlike the env editor's per-row
// enter-then-type-then-enter, updateLaunchInputsDialog appends typed
// runes to whichever field is focused directly -- there is no separate
// "open this row for editing" step here.
func clientTypesIntoLaunchInputsPreLaunchField(ctx context.Context, clientName, value string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(clientName)
	if err != nil {
		return err
	}
	if err := client.Send(value); err != nil {
		return err
	}
	return client.WaitForFrame(ctx, false, value)
}

// clientSubmitsLaunchInputsEditor sends enter (dialogContract's Submit,
// applyDialogContract) and waits for launchInputsBody's own title to
// leave the frame -- a successful submit returns to detailView
// (m.detail stays true underneath, exactly like rename's own submit),
// never all the way out to the main list.
func clientSubmitsLaunchInputsEditor(ctx context.Context, clientName string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(clientName)
	if err != nil {
		return err
	}
	if err := client.Send("\r"); err != nil {
		return err
	}
	return client.WaitForFrameGone(ctx, false, "Launch inputs for")
}

// clientClosesDetail sends "i" (detailView's own footer: "i or Esc closes
// detail") and waits for the main list's own header to reappear, so a
// scenario can go on to assert the sidebar's own launch↻/launch* badge,
// which detailView never renders (only sidebarRowLines does).
func clientClosesDetail(ctx context.Context, clientName string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(clientName)
	if err != nil {
		return err
	}
	if err := client.Send("i"); err != nil {
		return err
	}
	return client.WaitForFrame(ctx, false, "deck - sessions")
}

// sessionIsMarkedLaunchDirty proves a committed launch-inputs edit (task
// 023's store write, SetLaunchInputs) set the sessions.launch_dirty
// column -- the same flag the sidebar's launch↻ badge (task 025) and
// `R` restart's apply-and-clear path (internal/service/restart.go) both
// read, mirroring sessionIsMarkedEnvDirty exactly for the launch-side flag.
func sessionIsMarkedLaunchDirty(ctx context.Context, name string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	db, err := openObservedDatabase(h)
	if err != nil {
		return err
	}
	defer db.Close()
	var dirty int
	if err := db.QueryRowContext(ctx, `SELECT launch_dirty FROM sessions WHERE name = ?`, name).Scan(&dirty); err != nil {
		return fmt.Errorf("observe session %q launch_dirty: %w", name, err)
	}
	if dirty == 0 {
		return fmt.Errorf("session %q launch_dirty = 0, want 1 (nonzero) after a committed launch-inputs edit", name)
	}
	return nil
}

// sessionIsNotMarkedLaunchDirty is sessionIsMarkedLaunchDirty's inverse:
// only `R` restart (internal/service/restart.go) ever clears launch_dirty
// back to false, once a relaunch has actually carried the edit to the new
// pane.
func sessionIsNotMarkedLaunchDirty(ctx context.Context, name string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	db, err := openObservedDatabase(h)
	if err != nil {
		return err
	}
	defer db.Close()
	var dirty int
	if err := db.QueryRowContext(ctx, `SELECT launch_dirty FROM sessions WHERE name = ?`, name).Scan(&dirty); err != nil {
		return fmt.Errorf("observe session %q launch_dirty: %w", name, err)
	}
	if dirty != 0 {
		return fmt.Errorf("session %q launch_dirty = %d, want 0 after a restart applies the edit", name, dirty)
	}
	return nil
}
