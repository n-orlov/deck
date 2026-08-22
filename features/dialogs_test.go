package features

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

// registerDialogsSteps backs features/dialogs.feature (task 031, SPEC §11.4,
// requirements 8, 9, 10, 11, 50): the shared dialog contract
// (internal/tui/dialog_contract.go, task 029) asserted per dialog from
// outside the released binary, on state rather than on screen text, for
// esc's "changes nothing", enter's submit, tab's field navigation, the
// [26,80] width clamp (task 030) at both ends, the mouse's inability to
// cancel or confirm a dialog, and in-dialog validation retaining what was
// typed.
func registerDialogsSteps(sc *godog.ScenarioContext) {
	sc.Step(`^deck client "([^"]+)" opens the permission profile dialog for session "([^"]+)"$`, clientOpensProfileSwitchDialogForSession)
	sc.Step(`^deck client "([^"]+)" opens the pin dialog for session "([^"]+)"$`, clientOpensPinDialogForSession)
	sc.Step(`^deck client "([^"]+)" opens help$`, clientOpensHelp)
	sc.Step(`^deck client "([^"]+)" closes the dialog with escape$`, clientClosesDialogWithEscape)
	sc.Step(`^deck client "([^"]+)" cycles the open dialog's field right$`, clientCyclesOpenDialogFieldRight)
	sc.Step(`^deck client "([^"]+)" submits the open dialog$`, clientSubmitsOpenDialog)
	sc.Step(`^deck client "([^"]+)" opens the create modal and alters every field$`, clientOpensCreateModalAndAltersEveryField)
	sc.Step(`^deck client "([^"]+)" walks and edits every create modal field by keyboard, asserting each change is visible$`, clientWalksAndEditsEveryCreateFieldAssertingVisibility)
	sc.Step(`^deck client "([^"]+)" tabs (\d+) times in the open dialog$`, clientTabsInOpenDialog)
	sc.Step(`^deck client "([^"]+)" dialog box width is (\d+)$`, clientDialogBoxWidthIs)
	sc.Step(`^deck client "([^"]+)" attempts to create a shell session named "([^"]+)" with working directory "([^"]+)"$`, clientAttemptsCreateModalWithCWD)
	sc.Step(`^the state database session "([^"]+)" has resume mode "([^"]+)"$`, sessionHasResumeMode)
}

// clientOpensProfileSwitchDialogForSession selects the named row and sends
// `P` (SPEC §5/§11.4), the only way profileSwitchView opens.
func clientOpensProfileSwitchDialogForSession(ctx context.Context, clientName, sessionName string) error {
	if err := clientSelectsSessionByName(ctx, clientName, sessionName); err != nil {
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
	if err := client.Send("P"); err != nil {
		return err
	}
	return client.WaitForFrame(ctx, false, "Change permission profile")
}

// clientOpensPinDialogForSession selects the named row and sends `p` (SPEC
// §8/§9.3/§11.4), the only way pinView opens.
func clientOpensPinDialogForSession(ctx context.Context, clientName, sessionName string) error {
	if err := clientSelectsSessionByName(ctx, clientName, sessionName); err != nil {
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
	if err := client.Send("p"); err != nil {
		return err
	}
	return client.WaitForFrame(ctx, false, "Change resume mode")
}

// clientOpensHelp sends `?`, the only way helpView opens.
func clientOpensHelp(ctx context.Context, clientName string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(clientName)
	if err != nil {
		return err
	}
	if err := client.Send("?"); err != nil {
		return err
	}
	// helpText is longer than the harness's default terminal height, so its
	// own "deck help" title line (the very first line) can already have
	// scrolled off screen by the time this returns -- wait for a phrase from
	// its last line instead, which is always the most recently written and
	// therefore always on screen regardless of terminal height.
	return client.WaitForFrame(ctx, false, "closes help")
}

// clientClosesDialogWithEscape sends the shared §11.4 esc key and waits for
// the main session list to reappear, which every one of the five dialogs'
// own Cancel closures (dialog_contract.go's applyDialogContract) produces.
func clientClosesDialogWithEscape(ctx context.Context, clientName string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(clientName)
	if err != nil {
		return err
	}
	if err := client.Send("\x1b"); err != nil {
		return err
	}
	return client.WaitForFrame(ctx, false, "deck - sessions")
}

// clientCyclesOpenDialogFieldRight sends the shared §11.4 right-arrow key,
// which profileSwitchView and pinView's single Cycle field (dialog_contract.go)
// both bind to change the candidate value one step.
func clientCyclesOpenDialogFieldRight(ctx context.Context, clientName string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(clientName)
	if err != nil {
		return err
	}
	if err := client.Send("\x1b[C"); err != nil {
		return err
	}
	time.Sleep(30 * time.Millisecond)
	return nil
}

// clientSubmitsOpenDialog sends the shared §11.4 enter key and waits for the
// main session list to reappear, exactly the outcome a successful
// createView/profileSwitchView/pinView Submit (dialog_contract.go) produces.
func clientSubmitsOpenDialog(ctx context.Context, clientName string) error {
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
	return client.WaitForFrame(ctx, false, "deck - sessions")
}

// clientTabsInOpenDialog sends the shared §11.4 tab key n times, which
// createView's Fields.Count/Index (dialog_contract.go) advances the
// focused-field marker by.
func clientTabsInOpenDialog(ctx context.Context, clientName string, n int) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(clientName)
	if err != nil {
		return err
	}
	for i := 0; i < n; i++ {
		if err := client.Send("\t"); err != nil {
			return err
		}
		time.Sleep(25 * time.Millisecond)
	}
	return nil
}

// clientOpensCreateModalAndAltersEveryField opens the create modal and
// edits all eight of createFieldRows' fields (Name, Working directory,
// Agent, Permission profile, Launch args, Env, Pre-launch command, Login
// shell) in turn, tabbing onto each one before changing it -- text fields
// get typed characters, the two selection fields (Agent, Permission
// profile) get a right-arrow cycle, and the toggle field (Login shell)
// gets a space. It never submits: requirement 8 wants every field altered
// and then esc, not a valid, submittable form.
func clientOpensCreateModalAndAltersEveryField(ctx context.Context, clientName string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(clientName)
	if err != nil {
		return err
	}
	if err := client.Send("n"); err != nil {
		return err
	}
	if err := client.WaitForFrame(ctx, false, "Create shell session"); err != nil {
		return err
	}
	edits := []string{
		"dc-name",                // field 0: Name (already focused on open)
		"\t/dialog/contract/cwd", // field 1: Working directory
		"\t\x1b[C",               // field 2: Agent -- cycle right
		"\t\x1b[C",               // field 3: Permission profile -- cycle right
		"\tlaunch-args-typed",    // field 4: Launch args (JSON array)
		"\tENV_TYPED=1",          // field 5: Env
		"\tpre-launch-typed",     // field 6: Pre-launch command
		"\t ",                    // field 7: Login shell -- space toggles
	}
	for _, edit := range edits {
		if err := client.Send(edit); err != nil {
			return err
		}
		time.Sleep(30 * time.Millisecond)
	}
	return nil
}

// clientWalksAndEditsEveryCreateFieldAssertingVisibility covers task 016
// (SPEC requirement 7): every one of createFieldRows' eight fields is
// reachable by tab/shift+tab alone, editable with its own stated per-field
// key (typing for the free-text fields, right-arrow for the two selection
// fields, space for the toggle), and each edit is asserted VISIBLE on
// screen -- not merely performed and left unobserved, unlike the sibling
// esc-changes-nothing scenario driven by clientOpensCreateModalAndAltersEveryField
// above, which never looks at the screen mid-walk. It never submits (esc is
// left to the caller), matching that sibling's own contract.
func clientWalksAndEditsEveryCreateFieldAssertingVisibility(ctx context.Context, clientName string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(clientName)
	if err != nil {
		return err
	}
	if err := client.Send("n"); err != nil {
		return err
	}
	if err := client.WaitForFrame(ctx, false, "Create shell session"); err != nil {
		return err
	}
	assertVisible := func(step, want string) error {
		time.Sleep(30 * time.Millisecond)
		frame := client.Frame(false)
		if !strings.Contains(frame, want) {
			return fmt.Errorf("after %s, deck client %q screen does not contain %q:\n%s", step, clientName, want, frame)
		}
		return nil
	}
	// Field 0 (Name): already focused on open; typing appends, per
	// createFieldIsText.
	if err := client.Send("dc-walk-name"); err != nil {
		return err
	}
	if err := assertVisible("typing into Name", "dc-walk-name"); err != nil {
		return err
	}
	// Field 1 (Working directory): tab onto it, then type -- the first
	// keystroke replaces the prefill wholesale (task 008/009).
	if err := client.Send("\t/dc-walk/cwd"); err != nil {
		return err
	}
	if err := assertVisible("typing into Working directory", "/dc-walk/cwd"); err != nil {
		return err
	}
	// Field 2 (Agent): tab onto it, then right-arrow cycles §5's kinds
	// (sorted: claude, pi, shell); the default open value is "shell" (the
	// last, alphabetically), so one right-arrow wraps to "claude".
	if err := client.Send("\t\x1b[C"); err != nil {
		return err
	}
	if err := assertVisible("cycling Agent right", "Agent: claude"); err != nil {
		return err
	}
	// Field 3 (Permission profile): tab onto it, then right-arrow cycles
	// claude's declared profiles (safe, plan, edits[, yolo if allowed]);
	// the default settings this harness starts with have allow_yolo off,
	// and the field opened on "safe", so one right-arrow lands on "plan".
	if err := client.Send("\t\x1b[C"); err != nil {
		return err
	}
	if err := assertVisible("cycling Permission profile right", "Permission profile: plan"); err != nil {
		return err
	}
	// Field 4 (Launch args): tab onto it, then type.
	if err := client.Send("\twalk-launch-args"); err != nil {
		return err
	}
	if err := assertVisible("typing into Launch args", "walk-launch-args"); err != nil {
		return err
	}
	// Field 5 (Env): tab onto it, then type.
	if err := client.Send("\tWALK_ENV=1"); err != nil {
		return err
	}
	if err := assertVisible("typing into Env", "WALK_ENV=1"); err != nil {
		return err
	}
	// Field 6 (Pre-launch command): tab onto it, then type.
	if err := client.Send("\twalk-pre-launch"); err != nil {
		return err
	}
	if err := assertVisible("typing into Pre-launch command", "walk-pre-launch"); err != nil {
		return err
	}
	// Field 7 (Login shell): tab onto it, then space toggles off -> on.
	if err := client.Send("\t "); err != nil {
		return err
	}
	if err := assertVisible("toggling Login shell", "Login shell: on"); err != nil {
		return err
	}
	return nil
}

// clientDialogBoxWidthIs asserts the currently rendered dialog's top border
// -- the whole frame, since a §11.4 dialog's View() replaces the main view
// entirely rather than compositing over it -- is exactly want columns wide
// (task 030's dialogWidth: 80% of the viewport clamped to [26, 80]).
func clientDialogBoxWidthIs(ctx context.Context, clientName string, want int) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(clientName)
	if err != nil {
		return err
	}
	frame := client.Frame(false)
	lines := strings.Split(frame, "\n")
	if len(lines) == 0 || lines[0] == "" {
		return fmt.Errorf("deck client %q has no rendered dialog frame:\n%s", clientName, frame)
	}
	top := lines[0]
	got := len([]rune(top))
	if got != want {
		return fmt.Errorf("deck client %q dialog top border width = %d (%q), want %d", clientName, got, top, want)
	}
	return nil
}

// clientAttemptsCreateModalWithCWD opens the create modal, types name into
// the Name field and cwd into the Working directory field, then submits --
// proving requirement 10's in-dialog validation: a rejected value is
// re-presented with the reason, and the typed value is neither cleared nor
// silently corrected.
func clientAttemptsCreateModalWithCWD(ctx context.Context, clientName, name, cwd string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(clientName)
	if err != nil {
		return err
	}
	if err := client.Send("n"); err != nil {
		return err
	}
	if err := client.WaitForFrame(ctx, false, "Create shell session"); err != nil {
		return err
	}
	if err := client.Send(name + "\t" + cwd); err != nil {
		return err
	}
	time.Sleep(75 * time.Millisecond)
	if err := client.Send("\r"); err != nil {
		return err
	}
	return client.WaitForFrame(ctx, false, "does not exist")
}

// sessionHasResumeMode asserts the store's own persisted resume_state column
// for the named session equals want, the same durable-proof pattern
// sessionHasPermissionProfile already uses for `P` (SPEC §8/§9.3).
func sessionHasResumeMode(ctx context.Context, name, want string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	db, err := openObservedDatabase(h)
	if err != nil {
		return err
	}
	defer db.Close()
	var got string
	if err := db.QueryRowContext(ctx, `SELECT resume_state FROM sessions WHERE name = ?`, name).Scan(&got); err != nil {
		return fmt.Errorf("observe session %q resume_state: %w", name, err)
	}
	if got != want {
		return fmt.Errorf("session %q resume_state = %q, want %q", name, got, want)
	}
	return nil
}
