package features

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

// registerCreateSessionCWDPrefillSteps backs requirement 12: the create
// modal's cwd field opens pre-filled with the most recent §11.7 recent_cwds
// entry (labelled "last used"), or with no history the directory deck
// itself was started in, and the first edit replaces that prefill wholesale
// rather than appending to it (task 008).
func registerCreateSessionCWDPrefillSteps(sc *godog.ScenarioContext) {
	sc.Step(`^deck client "([^"]+)" is started in a fresh directory labelled "([^"]+)"$`, clientStartedInFreshDirectoryLabelled)
	sc.Step(`^deck client "([^"]+)" opens the create modal$`, clientOpensCreateModal)
	sc.Step(`^deck client "([^"]+)" screen contains the directory labelled "([^"]+)"$`, clientScreenContainsDirectoryLabelled)
	sc.Step(`^deck client "([^"]+)" screen does not contain the directory labelled "([^"]+)"$`, clientScreenDoesNotContainDirectoryLabelled)
	sc.Step(`^deck client "([^"]+)" creates shell session "([^"]+)" with a fresh working directory labelled "([^"]+)"$`, clientCreatesShellSessionWithFreshCWDLabelled)
	sc.Step(`^deck client "([^"]+)" creates shell session "([^"]+)" typing over the prefilled working directory with the directory labelled "([^"]+)"$`, clientCreatesShellSessionTypingOverPrefillWithLabelled)
	sc.Step(`^the state database session "([^"]+)" has cwd exactly the directory labelled "([^"]+)"$`, sessionHasCWDExactlyLabelled)
	sc.Step(`^deck client "([^"]+)" presses "(up|down|ctrl\+p|ctrl\+n)" in the cwd field (\d+) times?$`, clientPressesArrowInCWDFieldNTimes)
	sc.Step(`^deck client "([^"]+)" tabs to the cwd field$`, clientTabsToCWDField)
	sc.Step(`^the state database has a group named "([^"]+)"$`, ensureGroupNamedExists)
	sc.Step(`^the state database has no group named "([^"]+)"$`, groupNamedIsGone)
	sc.Step(`^deck client "([^"]+)" creates shell session "([^"]+)" into group "([^"]+)" with a fresh working directory labelled "([^"]+)"$`, clientCreatesShellSessionIntoGroupWithFreshCWDLabelled)
	sc.Step(`^the state database session "([^"]+)" was created into group "([^"]+)"$`, sessionWasCreatedIntoGroup)
}

// namedDirectory returns the real path a prior step registered under
// label, in h.namedDirectories -- the same map task 002's fingerprint
// harness populates, reused here for a directory a session's own cwd or a
// client's own process cwd is pinned to, rather than introducing a second,
// parallel bookkeeping map for the same "label -> real path" shape.
func namedDirectory(h *ScenarioHarness, label string) (string, error) {
	path, ok := h.namedDirectories[label]
	if !ok {
		return "", fmt.Errorf("directory %q has not been registered", label)
	}
	return path, nil
}

func registerNamedDirectory(h *ScenarioHarness, label, path string) {
	if h.namedDirectories == nil {
		h.namedDirectories = make(map[string]string)
	}
	h.namedDirectories[label] = path
}

// clientStartedInFreshDirectoryLabelled creates a new, empty directory
// under the scenario's own DECK_HOME (torn down with everything else),
// labels it, and starts a named client with the released binary's own
// process working directory pinned to it -- the "directory deck was
// started in" a no-history create modal falls back to.
func clientStartedInFreshDirectoryLabelled(ctx context.Context, name, label string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	dir := filepath.Join(h.Home, "create-session-"+label)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create directory labelled %q: %w", label, err)
	}
	registerNamedDirectory(h, label, dir)
	client, err := h.StartNamedClientInDir(ctx, name, dir)
	if err != nil {
		return err
	}
	return client.WaitForFrame(ctx, false, "deck - sessions")
}

// clientOpensCreateModal sends "n" and waits for the create modal's own
// Agent row -- rendered unconditionally by createFieldRows regardless of
// which agent is pre-selected (task 024's "(last used)"), unlike the
// modal's title, which reads "Create shell session" only while shell is
// selected and a plain "Create session" otherwise (F19) -- without
// asserting anything about which fields are pre-filled: callers state what
// they expect separately.
func clientOpensCreateModal(ctx context.Context, name string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(name)
	if err != nil {
		return err
	}
	if err := client.Send("n"); err != nil {
		return err
	}
	return client.WaitForFrame(ctx, false, "Agent: ")
}

// clientScreenContainsDirectoryLabelled asserts the currently rendered
// frame contains the real path a prior step registered under label,
// reusing clientScreenContains rather than re-implementing its polling.
func clientScreenContainsDirectoryLabelled(ctx context.Context, name, label string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	dir, err := namedDirectory(h, label)
	if err != nil {
		return err
	}
	return clientScreenContains(ctx, name, dir)
}

// clientScreenDoesNotContainDirectoryLabelled is
// clientScreenContainsDirectoryLabelled's negative counterpart (task 013,
// requirement 17): asserts the real path registered under label is absent
// from the currently rendered frame, reusing clientScreenDoesNotContain
// rather than re-implementing its polling.
func clientScreenDoesNotContainDirectoryLabelled(ctx context.Context, name, label string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	dir, err := namedDirectory(h, label)
	if err != nil {
		return err
	}
	return clientScreenDoesNotContain(ctx, name, dir)
}

// createShellSessionInLabelledCWD drives the real create modal to a
// successful shell-session creation in a brand-new, existing directory
// under the scenario's DECK_HOME: send "n", type the session name, move
// down (↑/↓, task 025) to
// the cwd field, type dir over whatever the field already held (its own
// §11.7 prefill included -- typing never clears first, by design; see
// task 008) and submit. Shared by both create-session steps below, which
// differ only in *why* the scenario calls it (seeding recent_cwds history
// vs. proving the wholesale-replace rule), not in what it does.
func createShellSessionInLabelledCWD(ctx context.Context, h *ScenarioHarness, clientName, sessionName, label string) error {
	client, err := h.Client(clientName)
	if err != nil {
		return err
	}
	dir := filepath.Join(h.Home, "create-session-"+label)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create directory labelled %q: %w", label, err)
	}
	registerNamedDirectory(h, label, dir)
	if err := client.Send("n"); err != nil {
		return err
	}
	// Title-independent (F19): ensureCreateModalAgent forces the Agent
	// field to shell rather than a literal wait for the shell-only
	// "Create shell session" title, which hangs once a non-shell agent
	// was pre-selected by an earlier create in the same scenario (proven
	// by create_session.feature's "converges on the shell agent even
	// after a non-shell session was created earlier" scenario).
	if err := ensureCreateModalAgent(ctx, client, "shell"); err != nil {
		return err
	}
	if err := client.Send(sessionName); err != nil {
		return err
	}
	time.Sleep(75 * time.Millisecond)
	if err := client.Send("\x1b[B" + dir + "\r"); err != nil {
		return err
	}
	return client.WaitForFrame(ctx, false, "starting")
}

// clientCreatesShellSessionWithFreshCWDLabelled seeds §11.7's recent_cwds
// history: per task 007, a successful create promotes the resolved
// absolute path to the front of it, so a later "opens the create modal"
// step can assert it is the one that pre-fills.
func clientCreatesShellSessionWithFreshCWDLabelled(ctx context.Context, clientName, sessionName, label string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	return createShellSessionInLabelledCWD(ctx, h, clientName, sessionName, label)
}

// clientCreatesShellSessionTypingOverPrefillWithLabelled opens the create
// modal (so the cwd field carries whatever prefill task 008 gave it -- a
// recent_cwds entry if an earlier step seeded one) and types a second,
// different directory over it without ever clearing the field by hand. If
// the wholesale-replace rule regressed to appending instead, the field
// would hold the prefill concatenated with this directory -- a path that
// does not exist -- and the create would be rejected rather than reaching
// "starting", so a passing scenario here already proves the replace, not
// only the later cwd-value assertion.
func clientCreatesShellSessionTypingOverPrefillWithLabelled(ctx context.Context, clientName, sessionName, label string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	return createShellSessionInLabelledCWD(ctx, h, clientName, sessionName, label)
}

// clientTabsToCWDField moves create-modal focus from the name field (0,
// where "n" leaves it) to the cwd field (1) via a single ↑/↓ press (task
// 025 moved field navigation off tab onto ↑/↓; the step's own name is
// kept since every scenario using it still reads as "get onto the cwd
// field", regardless of which key does it) --
// clientPressesArrowInCWDFieldNTimes below needs the cwd field actually
// focused, since up/down are a no-op on every other create-modal field
// once ANY dialog's own list/recent-cwd per-field key set (§11.4) doesn't
// claim them first.
func clientTabsToCWDField(ctx context.Context, name string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(name)
	if err != nil {
		return err
	}
	if err := client.Send("\x1b[B"); err != nil {
		return err
	}
	time.Sleep(60 * time.Millisecond)
	return nil
}

// clientPressesArrowInCWDFieldNTimes drives ↑/↓ field navigation or (task
// 025 moved recent_cwds cycling off ↑/↓ onto shell-history-style Ctrl+P
// (older) / Ctrl+N (newer)) recent-cwd cycling on the cwd field: the
// caller must already have tabbed focus to the cwd field
// (clientTabsToCWDField), since ↑/↓ move to a different field, and
// Ctrl+P/Ctrl+N are a no-op, everywhere else in the create modal. Sends
// the real terminal escape/control byte for up (\x1b[A), down (\x1b[B),
// ctrl+p (\x10) or ctrl+n (\x0e) n times, pausing between presses exactly
// as clientPressesKeyNTimes (features/layout_modes_test.go) does for any
// other repeated-keystroke step, since the same pty-coalescing gotcha
// applies to any raw byte sent back to back.
func clientPressesArrowInCWDFieldNTimes(ctx context.Context, name, direction string, n int) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(name)
	if err != nil {
		return err
	}
	seq, ok := map[string]string{
		"up":     "\x1b[A",
		"down":   "\x1b[B",
		"ctrl+p": "\x10",
		"ctrl+n": "\x0e",
	}[direction]
	if !ok {
		return fmt.Errorf("clientPressesArrowInCWDFieldNTimes: unrecognised direction %q", direction)
	}
	for i := 0; i < n; i++ {
		if err := client.Send(seq); err != nil {
			return err
		}
		time.Sleep(60 * time.Millisecond)
	}
	return nil
}

// sessionHasCWDExactlyLabelled asserts the sessions table's cwd column for
// name is byte-identical to the real path registered under label -- a
// stricter check than substring-on-screen, catching a leftover prefill
// prefix or suffix a screen assertion alone would miss.
func sessionHasCWDExactlyLabelled(ctx context.Context, name, label string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	want, err := namedDirectory(h, label)
	if err != nil {
		return err
	}
	db, err := openObservedDatabase(h)
	if err != nil {
		return err
	}
	defer db.Close()
	var got string
	if err := db.QueryRowContext(ctx, "SELECT cwd FROM sessions WHERE name = ?", name).Scan(&got); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("no session named %q in the state database", name)
		}
		return fmt.Errorf("observe cwd for session %q: %w", name, err)
	}
	if got != want {
		return fmt.Errorf("session %q cwd = %q, want exactly %q", name, got, want)
	}
	return nil
}

// ensureGroupNamedExists backs "the state database has a group named"
// (R130): creates the groups row directly, mirroring
// attention_sort_test.go's own setSessionGroup precedent for seeding a
// group ahead of the create modal's own "n" open, since R131's settings
// section (which will eventually offer an in-dialog "n" to create one) is
// a later task. ON CONFLICT DO NOTHING makes a second call for the same
// name in the same scenario harmless.
func ensureGroupNamedExists(ctx context.Context, name string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	db, err := openObservedDatabase(h)
	if err != nil {
		return err
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, `INSERT INTO groups(name) VALUES(?) ON CONFLICT(name) DO NOTHING`, name); err != nil {
		return fmt.Errorf("create group %q: %w", name, err)
	}
	return nil
}

// groupNamedIsGone backs "the state database has no group named" (R131
// part 2's "the group row goes once the batch commits"): it POLLS, exactly
// as stateDatabaseSessionIsTombstoned does, because the row is dropped by
// the TUI on the routed batch's own result message rather than by the
// keypress that submitted it -- a single immediate read would be a race,
// and a negative assertion that races is a test that passes for the wrong
// reason.
func groupNamedIsGone(ctx context.Context, name string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		count, err := groupRowCount(ctx, h, name)
		if err != nil {
			return err
		}
		if count == 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("group %q is still in the state database after 3s, want its row gone", name)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// groupRowCount is groupNamedIsGone's one read, kept separate so the
// observed database handle is opened and closed per poll rather than held
// open across the wait (every other assertion here opens its own handle
// too -- the released binary owns the file).
func groupRowCount(ctx context.Context, h *ScenarioHarness, name string) (int, error) {
	db, err := openObservedDatabase(h)
	if err != nil {
		return 0, err
	}
	defer db.Close()
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM groups WHERE name = ?`, name).Scan(&count); err != nil {
		return 0, fmt.Errorf("count group %q: %w", name, err)
	}
	return count, nil
}

// createModalCWDToGroupFieldDowns is the number of ↓ presses that move
// create-modal focus from the cwd field (1) to the Group field task 016
// appended last (createFieldRows' Agent/Permission profile/Launch args/
// Env/Pre-launch/Post-destroy/Login shell rows sit in between, in that
// order) -- mirroring ensureCreateModalAgent's own hardcoded 2-down
// Name->Agent precedent, since this file is deliberately a black-box
// observer of the released binary (see registerBlackBoxAssertionSteps)
// and cannot import internal/tui's own createFieldCount.
const createModalCWDToGroupFieldDowns = 8

// clientCreatesShellSessionIntoGroupWithFreshCWDLabelled is
// createShellSessionInLabelledCWD's R130 counterpart: it additionally
// ensures the target group exists (ensureGroupNamedExists, since R131's
// in-dialog "n" does not exist yet) and cycles the create modal's Group
// field to it before submitting, so the persisted group_id can be
// asserted against a session actually created through the dialog rather
// than set directly on the row afterwards.
func clientCreatesShellSessionIntoGroupWithFreshCWDLabelled(ctx context.Context, clientName, sessionName, group, label string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	if err := ensureGroupNamedExists(ctx, group); err != nil {
		return err
	}
	client, err := h.Client(clientName)
	if err != nil {
		return err
	}
	dir := filepath.Join(h.Home, "create-session-"+label)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create directory labelled %q: %w", label, err)
	}
	registerNamedDirectory(h, label, dir)
	if err := client.Send("n"); err != nil {
		return err
	}
	// Title-independent (F19), exactly like createShellSessionInLabelledCWD.
	if err := ensureCreateModalAgent(ctx, client, "shell"); err != nil {
		return err
	}
	if err := client.Send(sessionName); err != nil {
		return err
	}
	time.Sleep(75 * time.Millisecond)
	if err := client.Send("\x1b[B" + dir); err != nil {
		return err
	}
	time.Sleep(75 * time.Millisecond)
	if err := client.Send(strings.Repeat("\x1b[B", createModalCWDToGroupFieldDowns)); err != nil {
		return err
	}
	time.Sleep(75 * time.Millisecond)
	if err := cycleCreateModalGroupFieldTo(ctx, client, group); err != nil {
		return err
	}
	if err := client.Send("\r"); err != nil {
		return err
	}
	return waitForCreatedSessionPastStarting(ctx, client, sessionName)
}

// waitForCreatedSessionPastStarting waits for the just-submitted create
// modal to close and the new session's row to render, accepting either
// the transient "starting" frame or, if the shell fast-path in
// internal/service/reconcile.go already promoted the row to "running"
// before any frame happened to show "starting", the row reporting
// sessionName as running instead. A plain
// client.WaitForFrame(ctx, false, "starting") races that reconcile loop:
// task 021-cure-01's gate run (docs/reports/phase4b-final-suite/
// fullsuite-baf92ed.log) timed out on exactly this step with the frame
// already showing "groups-delete-dd-one running" and "starting" nowhere
// on screen -- the create had already succeeded, the wait was just still
// looking for a label the client had already left.
func waitForCreatedSessionPastStarting(ctx context.Context, client *ScreenDriver, sessionName string) error {
	running := sessionName + " running"
	_, err := client.WaitForFrameFunc(ctx, false, func(frame string) bool {
		return strings.Contains(frame, "starting") || strings.Contains(frame, running)
	})
	return err
}

// cycleCreateModalGroupFieldTo assumes focus is already on the create
// modal's Group field and presses right until its value reads want,
// mirroring cycleCreateFieldToValue's identical Agent-field precedent
// (features/agent_steps_test.go) but reading the frame rather than
// counting presses against a known option order, since the Group cycle
// order depends on which real groups a scenario created.
func cycleCreateModalGroupFieldTo(ctx context.Context, client *ScreenDriver, want string) error {
	marker := "Group: " + want + " (left/right cycles"
	matchesMarker := func(frame string) bool {
		return strings.Contains(dewrapCreateModalRowLabelled(frame, "Group: "), marker)
	}
	if matchesMarker(client.Frame(false)) {
		return nil
	}
	for attempt := 0; attempt < 8; attempt++ {
		if err := client.Send("\x1b[C"); err != nil { // right arrow
			return err
		}
		time.Sleep(25 * time.Millisecond)
		if matchesMarker(client.Frame(false)) {
			break
		}
	}
	if _, err := client.WaitForFrameFunc(ctx, false, matchesMarker); err != nil {
		return fmt.Errorf("cycle the create modal's Group field to %q: %w", want, err)
	}
	return nil
}

// dewrapCreateModalRowLabelled is dewrapCreateModalAgentRow's generic
// counterpart (features/agent_steps_test.go): the create modal's dialog
// box word-wraps a field's "value (left/right cycles: ...)" hint onto the
// dialog's fixed content width, which can split "Group: " and its value
// across two grid rows exactly like the Agent row -- see that function's
// own doc comment for why a plain substring match across the joined frame
// can never match there.
func dewrapCreateModalRowLabelled(frame, label string) string {
	lines := strings.Split(frame, "\n")
	for i, line := range lines {
		if !strings.Contains(line, label) {
			continue
		}
		joined := stripDialogBoxBorder(line)
		if i+1 < len(lines) {
			joined += " " + stripDialogBoxBorder(lines[i+1])
		}
		return joined
	}
	return frame
}

// sessionWasCreatedIntoGroup asserts the sessions table's group_id column
// for name resolves (via the groups table) to exactly the named group --
// not merely that some group_id is set -- proving the create modal's
// Group field selection reached the persisted row (R130).
func sessionWasCreatedIntoGroup(ctx context.Context, name, group string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	db, err := openObservedDatabase(h)
	if err != nil {
		return err
	}
	defer db.Close()
	var got sql.NullInt64
	if err := db.QueryRowContext(ctx, "SELECT group_id FROM sessions WHERE name = ?", name).Scan(&got); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("no session named %q in the state database", name)
		}
		return fmt.Errorf("observe group_id for session %q: %w", name, err)
	}
	if !got.Valid {
		return fmt.Errorf("session %q group_id is NULL, want it to name group %q", name, group)
	}
	var wantID int64
	if err := db.QueryRowContext(ctx, "SELECT id FROM groups WHERE name = ?", group).Scan(&wantID); err != nil {
		return fmt.Errorf("look up group %q: %w", group, err)
	}
	if got.Int64 != wantID {
		return fmt.Errorf("session %q group_id = %d, want %d (group %q)", name, got.Int64, wantID, group)
	}
	return nil
}
