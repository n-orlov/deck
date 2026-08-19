package features

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/cucumber/godog"
)

// registerAgentSessionSteps extends the black-box assertion surface with the
// facts a real coding-agent session needs (task 022): creating one with a
// named agent/profile/cwd through the real TUI, reading its conversation_id
// back from the store, asserting launch-audit argv content, counting launch
// records per session, counting tmux sessions matching a slug, and pressing
// `r` on a named row from a named client. It deliberately reuses the
// existing harness (ScenarioHarness, tmuxOutput, readAudit, sql.Open) rather
// than growing a second one.
func registerAgentSessionSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a fake "claude" binary is on PATH for future deck clients$`, fakeClaudeOnPATHForFutureClients)
	sc.Step(`^a long-running fake "claude" binary is on PATH for future deck clients$`, longRunningFakeClaudeOnPATHForFutureClients)
	sc.Step(`^the deck config allows yolo$`, deckConfigAllowsYolo)
	sc.Step(`^deck client "([^"]+)" opens the create modal for agent "([^"]+)"$`, clientOpensCreateModalForAgent)
	sc.Step(`^deck client "([^"]+)" screen does not contain "([^"]+)"$`, clientScreenDoesNotContain)
	sc.Step(`^deck client "([^"]+)" creates ([a-z]+) session "([^"]+)" with permission profile "yolo" confirming yolo$`, clientCreatesAgentSessionConfirmingYolo)
	sc.Step(`^deck client "([^"]+)" attempts ([a-z]+) session "([^"]+)" with permission profile "yolo" without confirming$`, clientAttemptsAgentSessionWithYoloWithoutConfirming)
	sc.Step(`^deck client "([^"]+)" opens detail for session "([^"]+)"$`, clientOpensDetailForSession)
	sc.Step(`^the state database session "([^"]+)" is marked degraded from requesting permission profile "([^"]+)" on agent "([^"]+)"$`, sessionMarkedDegraded)
	sc.Step(`^the state database session "([^"]+)" has permission profile "([^"]+)"$`, sessionHasPermissionProfile)
	sc.Step(`^the state database does not contain session "([^"]+)"$`, stateDatabaseDoesNotContainSession)
	sc.Step(`^deck client "([^"]+)" creates ([a-z]+) session "([^"]+)" with permission profile "([^"]+)"$`, clientCreatesAgentSessionWithProfile)
	sc.Step(`^deck client "([^"]+)" creates ([a-z]+) session "([^"]+)" with permission profile "([^"]+)" and message "([^"]+)"$`, clientCreatesAgentSessionWithProfileAndMessage)
	sc.Step(`^deck client "([^"]+)" presses r on session "([^"]+)"$`, clientPressesResumeOnNamedSession)
	sc.Step(`^the state database session "([^"]+)" has a non-empty conversation id$`, sessionHasNonEmptyConversationID)
	sc.Step(`^the state database sessions "([^"]+)" and "([^"]+)" have different conversation ids$`, sessionsHaveDifferentConversationIDs)
	sc.Step(`^the audit log's most recent launch argv for session "([^"]+)" contains "([^"]+)"$`, launchArgvForSessionContains)
	sc.Step(`^the audit log's most recent launch argv for session "([^"]+)" does not contain "([^"]+)"$`, launchArgvForSessionDoesNotContain)
	sc.Step(`^the audit log's most recent launch argv for session "([^"]+)" does not contain session "([^"]+)"'s conversation id$`, launchArgvForSessionDoesNotContainOthersConversationID)
	sc.Step(`^the audit log's most recent launch argv for session "([^"]+)" contains session "([^"]+)"'s conversation id$`, launchArgvForSessionContainsOwnConversationID)
	sc.Step(`^the audit log has ([0-9]+) launch record(?:s)? for session "([^"]+)"$`, auditHasLaunchRecordCountForSession)
	sc.Step(`^exactly ([0-9]+) private tmux session(?:s)? match(?:es)? slug "([^"]+)"$`, exactlyNPrivateSessionsMatchSlug)
	sc.Step(`^no private tmux session exists$`, noPrivateTMuxSessionExists)
	sc.Step(`^session "([^"]+)" replays its own last message, not session "([^"]+)"'s$`, sessionReplaysOwnMessageNotAnothers)
	sc.Step(`^the state database session "([^"]+)"'s status reason contains "([^"]+)"$`, sessionStatusReasonContains)
	sc.Step(`^the working directory shared by this scenario's sessions no longer exists$`, workingDirectoryRemoved)
	sc.Step(`^the state database session "([^"]+)"'s captured_path no longer contains the fake "claude" binary$`, sessionCapturedPathStrippedOfAgent)
	sc.Step(`^the state database session "([^"]+)"'s conversation id is cleared$`, sessionConversationIDCleared)
	sc.Step(`^deck clients "([^"]+)", "([^"]+)" and "([^"]+)" race pressing r on session "([^"]+)"$`, clientsRacePressingResumeOnNamedSession)
	sc.Step(`^at least one of deck clients "([^"]+)", "([^"]+)" and "([^"]+)" screen contains "starting elsewhere"$`, atLeastOneOfClientsShowsStartingElsewhere)
	sc.Step(`^the state database session "([^"]+)" has a launch lease held by a live process$`, sessionHasLiveLaunchLease)
	sc.Step(`^the state database session "([^"]+)" has a launch lease owned by a dead process$`, sessionHasDeadLaunchLease)
	sc.Step(`^the state database session "([^"]+)" has an expired launch lease$`, sessionHasExpiredLaunchLease)
	sc.Step(`^the state database session "([^"]+)"'s launch lease is cleared$`, sessionLaunchLeaseIsCleared)
}

// fakeClaudeOnPATHForFutureClients builds the repository's fake-claude
// fixture into its own directory named exactly "claude" and records that
// directory on the harness so every subsequently started named client gets
// it prepended to a real PATH (real tmux/go remain reachable; see
// clientCreatesAgentSessionWithProfile / startNamedClient for how it is
// consumed). It must run before the client it is meant to affect is started:
// a deck client's environment is fixed at process start.
func fakeClaudeOnPATHForFutureClients(ctx context.Context) error {
	return installFakeClaudeOnPATH(ctx, false)
}

func longRunningFakeClaudeOnPATHForFutureClients(ctx context.Context) error {
	return installFakeClaudeOnPATH(ctx, true)
}

func installFakeClaudeOnPATH(ctx context.Context, longRunning bool) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	root, err := repositoryRoot()
	if err != nil {
		return err
	}
	dir := filepath.Join(h.Home, "fake-agent-path")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create fake agent PATH directory: %w", err)
	}
	realBinary := filepath.Join(dir, "fake-claude-real")
	build := exec.CommandContext(ctx, "go", "build", "-o", realBinary, "./cmd/fake-claude")
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		return fmt.Errorf("build fake claude fixture: %w\n%s", err, output)
	}
	// deck's own CreateAgent/Resume mirror the launch env into the tmux
	// session via a second round-trip tmux command AFTER the pane's process
	// has already started (internal/tmux.Client.Create's "mirrored into the
	// session for any future pane" set-environment loop), which is safe for
	// a real, long-lived coding agent but races an instant-exit fixture: the
	// fixture's own banner/argv/replay output happens synchronously before
	// exit, so it is unaffected, but the pane (and, if it was the session's
	// only pane, the whole tmux session) can already be gone by the time
	// that second command runs, which deck correctly treats as a launch
	// failure. Wrapping the fixture so it lingers briefly after doing its
	// own observable work keeps this fixture faithful to a real agent's
	// actual lifetime for exactly that window, without changing anything
	// about deck's own contract.
	claudeWrapper := filepath.Join(dir, "claude")
	script := "#!/bin/sh\n\"" + realBinary + "\" \"$@\"\ncode=$?\nsleep 0.5\nexit \"$code\"\n"
	if longRunning {
		// Command mode reads from the pane until it receives a command or EOF,
		// providing an unsignalled live agent without sleeps or network access.
		script = "#!/bin/sh\nFAKE_CLAUDE_COMMANDS=1 exec \"" + realBinary + "\" \"$@\"\n"
	}
	if err := os.WriteFile(claudeWrapper, []byte(script), 0o700); err != nil {
		return fmt.Errorf("write claude fixture wrapper: %w", err)
	}
	h.agentPATHDir = dir
	homeDir := filepath.Join(h.Home, "agent-fixture-home")
	if err := os.MkdirAll(homeDir, 0o700); err != nil {
		return fmt.Errorf("create fixture HOME directory: %w", err)
	}
	h.agentHOMEDir = homeDir
	return nil
}

// clientCreatesAgentSessionWithProfile drives the real create modal by
// keystrokes only: it types the name and a fresh working directory, cycles
// the Agent field to kind, cycles the Permission profile field to profile,
// then submits. It reuses the same working-directory/sentinel setup as
// clientCreatesShellSession so the created row is comparable to a shell
// row's contract.
func clientCreatesAgentSessionWithProfile(ctx context.Context, clientName, kind, name, profile string) error {
	return clientCreatesAgentSessionWithProfileAndOptionalMessage(ctx, clientName, kind, name, profile, "")
}

// clientCreatesAgentSessionWithProfileAndMessage is the same keystroke-only
// creation flow as clientCreatesAgentSessionWithProfile, but also fills the
// Launch args (JSON array) field with a single-element JSON array holding
// message. The claude adapter appends launch_args verbatim after its own
// argv (internal/agent.Claude.Launch/Resume), and the fake-claude fixture
// records any trailing positional text as that conversation's message and
// replays it on --resume (cmd/fake-claude's replayAndRecord) — this is how
// this package proves distinct conversations replay their own message and
// never another session's (task 023).
func clientCreatesAgentSessionWithProfileAndMessage(ctx context.Context, clientName, kind, name, profile, message string) error {
	return clientCreatesAgentSessionWithProfileAndOptionalMessage(ctx, clientName, kind, name, profile, message)
}

func clientCreatesAgentSessionWithProfileAndOptionalMessage(ctx context.Context, clientName, kind, name, profile, message string) error {
	_, client, err := positionCreateModalOnProfileField(ctx, clientName, kind, name, profile)
	if err != nil {
		return err
	}
	if message != "" {
		// Tab onto the Launch args (JSON array) field, right after Permission
		// profile, and type a one-element JSON array holding message verbatim
		// (internal/tui.createFieldRows field order).
		encoded, err := json.Marshal([]string{message})
		if err != nil {
			return fmt.Errorf("encode launch_args message %q: %w", message, err)
		}
		if err := client.Send("\t" + string(encoded)); err != nil {
			return err
		}
		time.Sleep(75 * time.Millisecond)
		if err := client.WaitForFrame(ctx, false, string(encoded)); err != nil {
			return fmt.Errorf("type Launch args field with message %q: %w", message, err)
		}
	}
	if err := client.Send("\r"); err != nil {
		return err
	}
	return client.WaitForFrame(ctx, false, "starting")
}

// positionCreateModalOnProfileField drives the real create modal by
// keystrokes only through name, cwd, Agent (cycled to kind) and Permission
// profile (cycled to profile), leaving the modal open with the cursor on
// the Permission profile field and not yet submitted. It is the shared
// prefix behind every create-flow step in this file, including the ones
// exercising the yolo double-gate (task 025), which need to act (or
// deliberately not act) between reaching the profile field and pressing
// enter.
func positionCreateModalOnProfileField(ctx context.Context, clientName, kind, name, profile string) (*ScenarioHarness, *ScreenDriver, error) {
	h, err := assertionHarness(ctx)
	if err != nil {
		return nil, nil, err
	}
	client, err := h.Client(clientName)
	if err != nil {
		return nil, nil, err
	}
	if h.workingDir == "" {
		cwd := filepath.Join(h.Home, "agent-session-cwd")
		if err := os.MkdirAll(cwd, 0o700); err != nil {
			return nil, nil, fmt.Errorf("create scenario working directory: %w", err)
		}
		h.workingDir, h.sentinel = cwd, []byte("deck must not alter user cwd\n")
		if err := os.WriteFile(filepath.Join(cwd, "sentinel"), h.sentinel, 0o600); err != nil {
			return nil, nil, fmt.Errorf("write working-directory sentinel: %w", err)
		}
	}
	if err := client.Send("n"); err != nil {
		return nil, nil, err
	}
	if err := client.WaitForFrame(ctx, false, "Create shell session"); err != nil {
		return nil, nil, err
	}
	if err := client.Send(name); err != nil {
		return nil, nil, err
	}
	time.Sleep(75 * time.Millisecond)
	if err := client.Send("\t" + h.workingDir); err != nil {
		return nil, nil, err
	}
	time.Sleep(75 * time.Millisecond)
	if err := cycleCreateFieldToValue(ctx, client, kind, createAgentOptionsOrder); err != nil {
		return nil, nil, fmt.Errorf("cycle Agent field to %q: %w", kind, err)
	}
	// Tab onto the Permission profile field, then cycle right until it
	// reads profile. Options depend on the now-selected agent, so read them
	// from the current frame rather than hard-coding claude/pi's lists here.
	if err := client.Send("\t"); err != nil {
		return nil, nil, err
	}
	time.Sleep(50 * time.Millisecond)
	for attempt := 0; attempt < 4; attempt++ {
		frame := client.Frame(false)
		if strings.Contains(frame, profile+" (left/right cycles") {
			break
		}
		if err := client.Send("\x1b[C"); err != nil {
			return nil, nil, err
		}
		time.Sleep(25 * time.Millisecond)
	}
	if err := client.WaitForFrame(ctx, false, profile+" (left/right cycles"); err != nil {
		return nil, nil, fmt.Errorf("cycle Permission profile field to %q: %w", profile, err)
	}
	return h, client, nil
}

// cycleCreateFieldToValue tabs onto the field the caller is currently
// positioned before (the Agent field, in every current caller) and cycles
// right until want is on screen, matching one entry of order.
func cycleCreateFieldToValue(ctx context.Context, client *ScreenDriver, want string, order []string) error {
	if err := client.Send("\t"); err != nil {
		return err
	}
	time.Sleep(50 * time.Millisecond)
	for _, candidate := range order {
		if candidate == want {
			break
		}
		if err := client.Send("\x1b[C"); err != nil { // right arrow
			return err
		}
		time.Sleep(25 * time.Millisecond)
	}
	return client.WaitForFrame(ctx, false, want+" (left/right cycles")
}

// clientCreatesAgentSessionConfirmingYolo drives the create modal to the
// yolo permission profile, presses the yolo double-gate's explicit confirm
// keystroke ("y", only meaningful while focused on the Permission profile
// field, see internal/tui.Model's "y" handling), then submits. It requires
// allow_yolo=true in the scenario's config (SPEC §5); use
// deckConfigAllowsYolo before starting the client.
func clientCreatesAgentSessionConfirmingYolo(ctx context.Context, clientName, kind, name string) error {
	_, client, err := positionCreateModalOnProfileField(ctx, clientName, kind, name, "yolo")
	if err != nil {
		return err
	}
	if err := client.Send("y"); err != nil {
		return err
	}
	if err := client.Send("\r"); err != nil {
		return err
	}
	return client.WaitForFrame(ctx, false, "starting")
}

// clientAttemptsAgentSessionWithYoloWithoutConfirming drives the create
// modal to the yolo permission profile and presses enter without the "y"
// confirm keystroke, asserting the double-gate refuses to create anything
// (SPEC §5, task 017/036): the modal states why and stays open rather than
// silently creating a yolo session.
func clientAttemptsAgentSessionWithYoloWithoutConfirming(ctx context.Context, clientName, kind, name string) error {
	_, client, err := positionCreateModalOnProfileField(ctx, clientName, kind, name, "yolo")
	if err != nil {
		return err
	}
	if err := client.Send("\r"); err != nil {
		return err
	}
	return client.WaitForFrame(ctx, false, "yolo requires confirmation")
}

// clientOpensCreateModalForAgent opens the create modal and cycles only the
// Agent field to kind, leaving the modal open without filling name/cwd or
// submitting. internal/tui.createFieldRows renders every field's help text
// unconditionally, so this is enough for a caller to assert the Permission
// profile field's yolo-availability explanation (task 025) without needing
// a fully valid, submittable form.
func clientOpensCreateModalForAgent(ctx context.Context, clientName, kind string) error {
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
	// One tab: Name -> Working directory. cycleCreateFieldToValue's own
	// leading tab then moves Working directory -> Agent.
	if err := client.Send("\t"); err != nil {
		return err
	}
	time.Sleep(50 * time.Millisecond)
	return cycleCreateFieldToValue(ctx, client, kind, createAgentOptionsOrder)
}

// createAgentOptionsOrder mirrors internal/tui.createAgentOptions. It is
// duplicated here, not imported, because this file is deliberately a
// black-box observer of the released binary (see registerBlackBoxAssertionSteps).
var createAgentOptionsOrder = []string{"shell", "claude", "pi"}

// clientPressesResumeOnNamedSession moves the list selection to the row
// whose name is want (by repeated down-arrows from the top of the list,
// matching the selection marker rendered by internal/tui.listView) and then
// sends the `r` resume key.
func clientPressesResumeOnNamedSession(ctx context.Context, clientName, want string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(clientName)
	if err != nil {
		return err
	}
	marker := "> " + want
	for attempt := 0; attempt < 50; attempt++ {
		if strings.Contains(client.Frame(false), marker) {
			return client.Send("r")
		}
		if err := client.Send("\x1b[B"); err != nil { // down arrow
			return err
		}
		time.Sleep(25 * time.Millisecond)
	}
	return fmt.Errorf("deck client %q never selected session %q (marker %q not found):\n%s", clientName, want, marker, client.Frame(false))
}

func sessionIDByName(h *ScenarioHarness, name string) (string, error) {
	db, err := openObservedDatabase(h)
	if err != nil {
		return "", err
	}
	defer db.Close()
	var id string
	if err := db.QueryRow(`SELECT id FROM sessions WHERE name = ?`, name).Scan(&id); err != nil {
		return "", fmt.Errorf("observe session %q id: %w", name, err)
	}
	return id, nil
}

func sessionConversationID(h *ScenarioHarness, name string) (string, error) {
	db, err := openObservedDatabase(h)
	if err != nil {
		return "", err
	}
	defer db.Close()
	var conversationID sql.NullString
	if err := db.QueryRow(`SELECT conversation_id FROM sessions WHERE name = ?`, name).Scan(&conversationID); err != nil {
		return "", fmt.Errorf("observe session %q conversation id: %w", name, err)
	}
	return conversationID.String, nil
}

func sessionHasNonEmptyConversationID(ctx context.Context, name string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	id, err := sessionConversationID(h, name)
	if err != nil {
		return err
	}
	if id == "" {
		return fmt.Errorf("session %q has no conversation id", name)
	}
	return nil
}

func sessionsHaveDifferentConversationIDs(ctx context.Context, first, second string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	firstID, err := sessionConversationID(h, first)
	if err != nil {
		return err
	}
	secondID, err := sessionConversationID(h, second)
	if err != nil {
		return err
	}
	if firstID == "" || secondID == "" {
		return fmt.Errorf("session %q or %q has no conversation id (%q, %q)", first, second, firstID, secondID)
	}
	if firstID == secondID {
		return fmt.Errorf("sessions %q and %q share conversation id %q", first, second, firstID)
	}
	return nil
}

// launchArgvRecordsForSession returns every launch record's argv for the
// named session, oldest first, by joining the audit log's session_id
// against the store's own id for name. It never imports internal/audit or
// internal/store: everything is observed through the released JSONL file
// and SQLite database, matching this package's black-box discipline.
func launchArgvRecordsForSession(h *ScenarioHarness, name string) ([][]string, error) {
	sessionID, err := sessionIDByName(h, name)
	if err != nil {
		return nil, err
	}
	records, err := readAudit(h)
	if err != nil {
		return nil, err
	}
	var argvRecords [][]string
	for _, record := range records {
		var event, recordSessionID string
		_ = json.Unmarshal(record["event"], &event)
		_ = json.Unmarshal(record["session_id"], &recordSessionID)
		if event != "launch" || recordSessionID != sessionID {
			continue
		}
		var argv []string
		if raw, ok := record["argv"]; ok {
			if err := json.Unmarshal(raw, &argv); err != nil {
				return nil, fmt.Errorf("parse launch argv for session %q: %w", name, err)
			}
		}
		argvRecords = append(argvRecords, argv)
	}
	return argvRecords, nil
}

func mostRecentLaunchArgvForSession(h *ScenarioHarness, name string) ([]string, error) {
	records, err := launchArgvRecordsForSession(h, name)
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("audit log has no launch record for session %q", name)
	}
	return records[len(records)-1], nil
}

// launchArgvForSessionContains is the one entry point that must poll rather
// than read once: it is always the FIRST audit-log assertion right after a
// screen-based wait for "starting" following a `r` keypress. "screen
// contains starting" only proves the deck client's OWN terminal grid shows
// that string; it gives no cross-process ordering guarantee with the audit
// JSONL file being written by that same deck subprocess's Resume() call.
// Program order inside deck does guarantee the audit.Launch write completes
// before the store row (and therefore the rendered status) ever reaches
// "starting", but nothing here synchronizes THIS test process's read of the
// file with that write actually having been scheduled and become visible;
// the two are only loosely ordered by wall-clock proximity. Polling for up
// to 2s (the same pattern databaseSessionStatus above already uses for the
// analogous DB-vs-screen gap) closes that gap without weakening what is
// asserted: this still fails hard if --resume never appears. Every OTHER
// launch-argv assertion in this file runs strictly after this one has
// already observed the record, so they stay single-shot reads.
func launchArgvForSessionContains(ctx context.Context, name, want string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	deadline := time.Now().Add(2 * time.Second)
	var lastArgv []string
	var lastErr error
	for {
		argv, argvErr := mostRecentLaunchArgvForSession(h, name)
		if argvErr == nil && argvContains(argv, want) {
			return nil
		}
		lastArgv, lastErr = argv, argvErr
		if time.Now().After(deadline) {
			if lastErr != nil {
				return lastErr
			}
			return fmt.Errorf("most recent launch argv for session %q = %q, does not contain %q", name, lastArgv, want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// launchArgvForSessionContainsOwnConversationID asserts the named session's
// most recent launch argv contains that session's own persisted conversation
// id, resolved from the state database rather than hard-coded, so a resume
// argv's `--resume <id>` is proven to carry the session's own id and not
// merely the literal `--resume` flag (PRD requirement 22 / operator
// steering 003).
func launchArgvForSessionContainsOwnConversationID(ctx context.Context, name, self string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	if name != self {
		return fmt.Errorf("expected session name %q to match itself %q", name, self)
	}
	selfID, err := sessionConversationID(h, self)
	if err != nil {
		return err
	}
	if selfID == "" {
		return fmt.Errorf("session %q has no conversation id", self)
	}
	argv, err := mostRecentLaunchArgvForSession(h, name)
	if err != nil {
		return err
	}
	if !argvContains(argv, selfID) {
		return fmt.Errorf("most recent launch argv for session %q = %q, does not contain session %q's own conversation id %q", name, argv, self, selfID)
	}
	return nil
}

// launchArgvForSessionDoesNotContainOthersConversationID asserts the named
// session's most recent launch argv never contains other's conversation id,
// proving two sessions sharing one cwd cannot leak each other's id into a
// resume argv (task 024).
func launchArgvForSessionDoesNotContainOthersConversationID(ctx context.Context, name, other string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	otherID, err := sessionConversationID(h, other)
	if err != nil {
		return err
	}
	if otherID == "" {
		return fmt.Errorf("session %q has no conversation id", other)
	}
	argv, err := mostRecentLaunchArgvForSession(h, name)
	if err != nil {
		return err
	}
	if argvContains(argv, otherID) {
		return fmt.Errorf("most recent launch argv for session %q = %q, unexpectedly contains session %q's conversation id %q", name, argv, other, otherID)
	}
	return nil
}

func launchArgvForSessionDoesNotContain(ctx context.Context, name, unwanted string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	argv, err := mostRecentLaunchArgvForSession(h, name)
	if err != nil {
		return err
	}
	if argvContains(argv, unwanted) {
		return fmt.Errorf("most recent launch argv for session %q = %q, unexpectedly contains %q", name, argv, unwanted)
	}
	return nil
}

// argvContains reports whether any argv token itself contains want, so a
// step can assert against a substring of one token (e.g. a UUID embedded
// after a flag) as well as an exact token.
func argvContains(argv []string, want string) bool {
	for _, token := range argv {
		if strings.Contains(token, want) {
			return true
		}
	}
	return false
}

func auditHasLaunchRecordCountForSession(ctx context.Context, want int, name string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	records, err := launchArgvRecordsForSession(h, name)
	if err != nil {
		return err
	}
	if len(records) != want {
		return fmt.Errorf("audit log has %d launch records for session %q, want %d", len(records), name, want)
	}
	return nil
}

func exactlyNPrivateSessionsMatchSlug(ctx context.Context, want int, slug string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	commandCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(commandCtx, "tmux", "-L", h.Socket, "list-sessions", "-F", "#{session_name}").CombinedOutput()
	if err != nil {
		if strings.Contains(string(output), "no server running") || strings.Contains(string(output), "no sessions") {
			if want != 0 {
				return fmt.Errorf("private tmux server has no sessions, want %d matching slug %q", want, slug)
			}
			return nil
		}
		return fmt.Errorf("tmux -L %s list-sessions: %w: %s", h.Socket, err, strings.TrimSpace(string(output)))
	}
	got := 0
	for _, line := range strings.Fields(string(output)) {
		if line == slug {
			got++
		}
	}
	if got != want {
		return fmt.Errorf("private tmux sessions matching slug %q = %d, want %d", slug, got, want)
	}
	return nil
}

// noPrivateTMuxSessionExists asserts the scenario's private tmux server has
// no live session at all, the black-box fact SPEC §13.4's T1 scenario names
// ("no tmux session exists"): after a reboot stand-in (kill-server) and a
// deck restart, nothing is auto-started.
func noPrivateTMuxSessionExists(ctx context.Context) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	commandCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(commandCtx, "tmux", "-L", h.Socket, "list-sessions", "-F", "#{session_name}").CombinedOutput()
	if err != nil {
		if strings.Contains(string(output), "no server running") || strings.Contains(string(output), "no sessions") {
			return nil
		}
		return fmt.Errorf("tmux -L %s list-sessions: %w: %s", h.Socket, err, strings.TrimSpace(string(output)))
	}
	names := strings.Fields(string(output))
	if len(names) != 0 {
		return fmt.Errorf("private tmux server has %d live session(s), want none: %v", len(names), names)
	}
	return nil
}

// sessionSlugByName reads the store's own slug column for name (see
// internal/store.Slug), rather than reimplementing that conversion here, so
// this file stays a pure observer of what deck itself decided.
func sessionSlugByName(h *ScenarioHarness, name string) (string, error) {
	db, err := openObservedDatabase(h)
	if err != nil {
		return "", err
	}
	defer db.Close()
	var slug string
	if err := db.QueryRow(`SELECT slug FROM sessions WHERE name = ?`, name).Scan(&slug); err != nil {
		return "", fmt.Errorf("observe session %q slug: %w", name, err)
	}
	return slug, nil
}

// sessionReplaysOwnMessageNotAnothers polls the resumed session's own live
// tmux pane (fixture wrapper lingers ~0.5s after the fixture's own exit, see
// fakeClaudeOnPATHForFutureClients) for the fixture's "fake-claude replay: "
// banner, and asserts it carries this session's own last recorded message,
// never other's. It must run promptly after a resume/press-r step, before
// the pane exits and deck's private server (remain-on-exit failed) tears
// the pane, and with it the session, down.
func sessionReplaysOwnMessageNotAnothers(ctx context.Context, name, other string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	slug, err := sessionSlugByName(h, name)
	if err != nil {
		return err
	}
	ownMessage, err := sessionLastMessage(h, name)
	if err != nil {
		return err
	}
	if ownMessage == "" {
		return fmt.Errorf("session %q has no recorded message to replay", name)
	}
	othersMessage, err := sessionLastMessage(h, other)
	if err != nil {
		return err
	}
	tmuxSession := "deck_" + slug
	deadline := time.Now().Add(3 * time.Second)
	var text string
	for {
		output, captureErr := tmuxOutput(ctx, h, "capture-pane", "-p", "-S", "-", "-t", tmuxSession)
		if captureErr == nil {
			text = string(output)
			if strings.Contains(text, "fake-claude replay: "+ownMessage) {
				break
			}
		}
		if time.Now().After(deadline) {
			if captureErr != nil {
				return fmt.Errorf("capture resumed pane %q: %w", tmuxSession, captureErr)
			}
			return fmt.Errorf("resumed pane %q never showed replay of session %q's own message %q:\n%s", tmuxSession, name, ownMessage, text)
		}
		time.Sleep(25 * time.Millisecond)
	}
	if othersMessage != "" && strings.Contains(text, "fake-claude replay: "+othersMessage) {
		return fmt.Errorf("resumed pane %q for session %q unexpectedly replayed session %q's message %q:\n%s", tmuxSession, name, other, othersMessage, text)
	}
	return nil
}

// sessionLastMessage reads the fake-claude transcript fixture path/format
// directly (cmd/fake-claude's transcriptPath/appendMessage), keyed by the
// session's own conversation id and the scenario's fixture HOME/cwd, and
// returns the last recorded message line, or "" if the transcript does not
// exist yet.
func sessionLastMessage(h *ScenarioHarness, name string) (string, error) {
	if h.agentHOMEDir == "" {
		return "", fmt.Errorf("fixture HOME directory was not configured (call the fake claude PATH step first)")
	}
	conversationID, err := sessionConversationID(h, name)
	if err != nil {
		return "", err
	}
	if conversationID == "" {
		return "", fmt.Errorf("session %q has no conversation id", name)
	}
	project := strings.ReplaceAll(h.workingDir, string(os.PathSeparator), "-")
	path := filepath.Join(h.agentHOMEDir, ".claude", "projects", project, conversationID+".jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read transcript %q for session %q: %w", path, name, err)
	}
	var last string
	for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
		if line == "" {
			continue
		}
		var entry struct {
			Message string `json:"message"`
		}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			return "", fmt.Errorf("decode transcript entry %q: %w", line, err)
		}
		last = entry.Message
	}
	return last, nil
}

// deckConfigAllowsYolo writes a config.toml with allow_yolo = true into the
// scenario's DECK_HOME. It must run before the client(s) it is meant to
// affect are started: config.toml is read once at process start
// (internal/config.Load).
func deckConfigAllowsYolo(ctx context.Context) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	path := filepath.Join(h.Home, "config.toml")
	if err := os.WriteFile(path, []byte("allow_yolo = true\n"), 0o600); err != nil {
		return fmt.Errorf("write scenario config.toml: %w", err)
	}
	return nil
}

// clientScreenDoesNotContain is the negative counterpart to "deck client
// screen contains": it takes the current frame as-is, without polling,
// since every caller in this package first waits for the frame that
// establishes the state being asserted about (e.g. having just cycled the
// Agent field to a specific value) before checking an absence within it.
func clientScreenDoesNotContain(ctx context.Context, name, unwanted string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(name)
	if err != nil {
		return err
	}
	if frame := client.Frame(false); strings.Contains(frame, unwanted) {
		return fmt.Errorf("deck client %q screen unexpectedly contains %q:\n%s", name, unwanted, frame)
	}
	return nil
}

// clientOpensDetailForSession selects the named row (reusing
// clientPressesResumeOnNamedSession's marker-matching down-arrow search) and
// then presses "i" (internal/tui's detail-toggle key) instead of "r".
func clientOpensDetailForSession(ctx context.Context, clientName, want string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(clientName)
	if err != nil {
		return err
	}
	marker := "> " + want
	for attempt := 0; attempt < 50; attempt++ {
		if strings.Contains(client.Frame(false), marker) {
			return client.Send("i")
		}
		if err := client.Send("\x1b[B"); err != nil { // down arrow
			return err
		}
		time.Sleep(25 * time.Millisecond)
	}
	return fmt.Errorf("deck client %q never selected session %q (marker %q not found):\n%s", clientName, want, marker, client.Frame(false))
}

// sessionMarkedDegraded writes the exact permission_profile_reason string
// internal/service.CreateAgent would have stored had this session's agent
// been asked to honour requested at create time (internal/agent.Caps's
// ResolveProfile phrasing), directly into the store's own column. This
// package never imports internal/agent (see registerBlackBoxAssertionSteps'
// black-box discipline), so the wording is reproduced here deliberately, as
// a fixture standing in for an adapter capability drift after create time —
// the only way a stored profile can go stale, since the create modal only
// ever offers profiles the currently selected adapter declares (task 017),
// and therefore never lets a caller request an unsupported one to begin
// with.
func sessionMarkedDegraded(ctx context.Context, name, requested, agentKind string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	db, err := openObservedDatabase(h)
	if err != nil {
		return err
	}
	defer db.Close()
	reason := fmt.Sprintf("%s does not support permission profile %q; falling back to safe", agentKind, requested)
	result, err := db.ExecContext(ctx, `UPDATE sessions SET permission_profile_reason = ? WHERE name = ?`, reason, name)
	if err != nil {
		return fmt.Errorf("mark session %q degraded: %w", name, err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("mark session %q degraded: %w", name, err)
	}
	if affected != 1 {
		return fmt.Errorf("mark session %q degraded: %d rows affected, want 1", name, affected)
	}
	return nil
}

// sessionHasPermissionProfile asserts the store's own persisted
// permission_profile column for the named session equals want, e.g. proving
// yolo survived being persisted at create time (SPEC \u00a75: "Persisted, so a
// yolo session comes back yolo on resume").
func sessionHasPermissionProfile(ctx context.Context, name, want string) error {
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
	if err := db.QueryRowContext(ctx, `SELECT permission_profile FROM sessions WHERE name = ?`, name).Scan(&got); err != nil {
		return fmt.Errorf("observe session %q permission profile: %w", name, err)
	}
	if got != want {
		return fmt.Errorf("session %q permission profile = %q, want %q", name, got, want)
	}
	return nil
}

// stateDatabaseDoesNotContainSession asserts the yolo double-gate's refusal
// (task 017/036) never left a row behind: an enter without the "y" confirm
// closes nothing and creates nothing.
func stateDatabaseDoesNotContainSession(ctx context.Context, name string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	db, err := openObservedDatabase(h)
	if err != nil {
		return err
	}
	defer db.Close()
	var count int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM sessions WHERE name = ?`, name).Scan(&count); err != nil {
		return fmt.Errorf("observe session %q count: %w", name, err)
	}
	if count != 0 {
		return fmt.Errorf("state database unexpectedly contains session %q", name)
	}
	return nil
}

// sessionStatusReasonContains asserts the store's own persisted
// status_reason column for the named session contains want, e.g. proving a
// failed resume's row names its specific cause (unknown conversation id,
// missing cwd, agent binary not on PATH — task 026) rather than a generic
// message. It polls briefly since the row is updated asynchronously from
// the resuming client's own request/response cycle.
func sessionStatusReasonContains(ctx context.Context, name, want string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	db, err := openObservedDatabase(h)
	if err != nil {
		return err
	}
	defer db.Close()
	deadline := time.Now().Add(5 * time.Second)
	var got string
	for {
		if err := db.QueryRowContext(ctx, `SELECT COALESCE(status_reason, '') FROM sessions WHERE name = ?`, name).Scan(&got); err != nil {
			return fmt.Errorf("observe session %q status reason: %w", name, err)
		}
		if strings.Contains(got, want) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("session %q status reason = %q, want it to contain %q", name, got, want)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

// workingDirectoryRemoved deletes the entire directory every session
// created so far in this scenario shares as its cwd (positionCreateModalOnProfileField
// creates it lazily on first use and stores it on the harness), proving the
// resume-failure cause "missing or non-directory cwd" (SPEC §9.3 / task
// 012) black-box: a session that was perfectly launchable at create time can
// still fail resume later if its cwd has since vanished.
func workingDirectoryRemoved(ctx context.Context) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	if h.workingDir == "" {
		return errors.New("no working directory has been created yet for this scenario")
	}
	if err := os.RemoveAll(h.workingDir); err != nil {
		return fmt.Errorf("remove scenario working directory %q: %w", h.workingDir, err)
	}
	return nil
}

// sessionCapturedPathStrippedOfAgent overwrites the named session's own
// persisted captured_path (SPEC §6.3's PATH-resolution layer recorded at
// create time) with a freshly created, genuinely empty directory that
// cannot contain the fake claude binary fakeClaudeOnPATHForFutureClients put
// on the real PATH, proving the resume-failure cause "agent binary not on
// PATH" (task 012) black-box: the session was launchable when created, and
// only its own recorded PATH layer has since gone stale (its session-level
// env and any config [env] carry no PATH override in these scenarios, so
// this is the layer that decides resume's PATH).
func sessionCapturedPathStrippedOfAgent(ctx context.Context, name string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	empty := filepath.Join(h.Home, "empty-path-"+name)
	if err := os.MkdirAll(empty, 0o700); err != nil {
		return fmt.Errorf("create empty PATH directory: %w", err)
	}
	db, err := openObservedDatabase(h)
	if err != nil {
		return err
	}
	defer db.Close()
	result, err := db.ExecContext(ctx, `UPDATE sessions SET captured_path = ? WHERE name = ?`, empty, name)
	if err != nil {
		return fmt.Errorf("strip captured_path for session %q: %w", name, err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("strip captured_path for session %q: %w", name, err)
	}
	if affected != 1 {
		return fmt.Errorf("strip captured_path for session %q: %d rows affected, want 1", name, affected)
	}
	return nil
}

// sessionConversationIDCleared blanks the named session's own persisted
// conversation_id, proving the resume-failure cause "unknown/rejected
// conversation id" (task 012) black-box: a session that was assigned one
// perfectly well at create time can still have nothing valid to resume if
// its own recorded id has since gone missing (e.g. rejected upstream).
// internal/service.Resume treats an empty conversation id exactly the same
// as one the adapter never accepted, since deck itself never validates the
// id's acceptance with the agent ahead of the resume attempt.
func sessionConversationIDCleared(ctx context.Context, name string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	db, err := openObservedDatabase(h)
	if err != nil {
		return err
	}
	defer db.Close()
	result, err := db.ExecContext(ctx, `UPDATE sessions SET conversation_id = '' WHERE name = ?`, name)
	if err != nil {
		return fmt.Errorf("clear conversation id for session %q: %w", name, err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("clear conversation id for session %q: %w", name, err)
	}
	if affected != 1 {
		return fmt.Errorf("clear conversation id for session %q: %d rows affected, want 1", name, affected)
	}
	return nil
}

// selectRowByName moves client's selection to the row whose name is want by
// repeated down-arrows from wherever the cursor currently is, matching the
// same "> name" marker clientPressesResumeOnNamedSession relies on. It is
// factored out so the launch-lease race step (task 027) can position every
// racing client on the same row BEFORE firing `r` concurrently, since the
// positioning itself must stay sequential (each keystroke is a real PTY
// write) while only the final `r` needs to land within the race window.
func selectRowByName(client *ScreenDriver, want string) error {
	marker := "> " + want
	for attempt := 0; attempt < 50; attempt++ {
		if strings.Contains(client.Frame(false), marker) {
			return nil
		}
		if err := client.Send("\x1b[B"); err != nil { // down arrow
			return err
		}
		time.Sleep(25 * time.Millisecond)
	}
	return fmt.Errorf("never selected session %q (marker %q not found):\n%s", want, marker, client.Frame(false))
}

// clientsRacePressingResumeOnNamedSession positions all three named clients
// on the same stopped row and then fires `r` on all three concurrently, from
// separate goroutines, so their real process-level keystrokes land within a
// tight (well under 100ms) window rather than godog's normal one-step-at-a-
// time sequencing (task 027, SPEC §9.3's launch-lease race). Positioning
// happens first, one client at a time, since only the final `r` needs to
// race; a shared row selection is required for the race to mean anything.
func clientsRacePressingResumeOnNamedSession(ctx context.Context, first, second, third, sessionName string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	names := []string{first, second, third}
	clients := make([]*ScreenDriver, len(names))
	for i, name := range names {
		client, err := h.Client(name)
		if err != nil {
			return err
		}
		if err := selectRowByName(client, sessionName); err != nil {
			return fmt.Errorf("deck client %q: %w", name, err)
		}
		clients[i] = client
	}
	var wg sync.WaitGroup
	errs := make([]error, len(clients))
	for i, client := range clients {
		wg.Add(1)
		go func(i int, client *ScreenDriver) {
			defer wg.Done()
			errs[i] = client.Send("r")
		}(i, client)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			return fmt.Errorf("deck client %q send r: %w", names[i], err)
		}
	}
	return nil
}

// atLeastOneOfClientsShowsStartingElsewhere polls the three named clients'
// screens for the losing side of the launch-lease race (internal/tui's
// resumeNote == "starting elsewhere", SPEC §9.3), asserting at least one of
// them ends up showing it — the other two are the winner (whose own row
// reloads to "starting · awaiting signal") and any client whose own `r`
// simply never won a lease slot before the winner's launch already
// completed the acquisition. This is deliberately >= 1, not == 1: in a real
// 3-way race both losers can each independently observe and report
// "starting elsewhere" before the winner's reconcile lands, so asserting
// exactly one would flake.
func atLeastOneOfClientsShowsStartingElsewhere(ctx context.Context, first, second, third string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	names := []string{first, second, third}
	deadline := time.Now().Add(5 * time.Second)
	for {
		count := 0
		var frames []string
		for _, name := range names {
			client, err := h.Client(name)
			if err != nil {
				return err
			}
			frame := client.Frame(false)
			frames = append(frames, frame)
			if strings.Contains(frame, "starting elsewhere") {
				count++
			}
		}
		if count >= 1 {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("no losing client among %v showed 'starting elsewhere':\n%s", names, strings.Join(frames, "\n---\n"))
		}
		time.Sleep(25 * time.Millisecond)
	}
}

// currentBootID replicates internal/store.bootID's read of the kernel boot
// id (empty when unreadable) so this deliberately black-box package can
// construct a launch-lease owner string ("pid@boot_id", SPEC §9.3) that the
// released binary's own liveness check (internal/store.leaseOwnerAlive)
// recognizes as belonging to the current boot, without importing
// internal/store itself.
func currentBootID() string {
	data, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// setRawLaunchLease writes launch_lease_owner/launch_lease_until directly,
// bypassing internal/store.AcquireLaunchLease entirely, so a scenario can
// plant a lease in a state the released binary's own CAS acquisition path
// can never itself produce from the outside (task 028, mirroring the
// sessionConversationIDCleared/sessionMarkedDegraded fixture-only pattern).
func setRawLaunchLease(ctx context.Context, name, owner string, untilMillis int64) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	db, err := openObservedDatabase(h)
	if err != nil {
		return err
	}
	defer db.Close()
	result, err := db.ExecContext(ctx,
		`UPDATE sessions SET launch_lease_owner = ?, launch_lease_until = ? WHERE name = ?`,
		owner, untilMillis, name)
	if err != nil {
		return fmt.Errorf("set launch lease for session %q: %w", name, err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("set launch lease for session %q: %w", name, err)
	}
	if affected != 1 {
		return fmt.Errorf("set launch lease for session %q: %d rows affected, want 1", name, affected)
	}
	return nil
}

// sessionHasLiveLaunchLease plants a lease owned by this very test process
// (guaranteed alive, and on the current boot by construction) with a
// far-future TTL: SPEC §9.3's "live owner within TTL" case, which
// AcquireLaunchLease must refuse to break.
func sessionHasLiveLaunchLease(ctx context.Context, name string) error {
	owner := fmt.Sprintf("%d@%s", os.Getpid(), currentBootID())
	return setRawLaunchLease(ctx, name, owner, time.Now().Add(time.Hour).UnixMilli())
}

// sessionHasDeadLaunchLease plants a lease naming a pid that (barring an
// implausible collision) does not exist, still on the current boot and
// still well within its TTL, so only the dead-pid liveness check — not the
// TTL — is what makes it breakable (SPEC §9.3's "dead-owner-pid" case).
func sessionHasDeadLaunchLease(ctx context.Context, name string) error {
	owner := fmt.Sprintf("999999999@%s", currentBootID())
	return setRawLaunchLease(ctx, name, owner, time.Now().Add(time.Hour).UnixMilli())
}

// sessionHasExpiredLaunchLease plants a lease owned by this live, in-boot
// test process but whose TTL already elapsed, so only the TTL check — not
// liveness — is what makes it breakable (SPEC §9.3's "expired-TTL" case).
func sessionHasExpiredLaunchLease(ctx context.Context, name string) error {
	owner := fmt.Sprintf("%d@%s", os.Getpid(), currentBootID())
	return setRawLaunchLease(ctx, name, owner, time.Now().Add(-time.Minute).UnixMilli())
}

// sessionLaunchLeaseIsCleared removes a planted lease so a following `r`
// exercises a plain fresh acquisition, proving the row was never left
// wedged by whichever case the scenario just proved breakable/unbreakable.
func sessionLaunchLeaseIsCleared(ctx context.Context, name string) error {
	return setRawLaunchLease(ctx, name, "", 0)
}
