package features

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
	sc.Step(`^deck client "([^"]+)" creates ([a-z]+) session "([^"]+)" with permission profile "([^"]+)"$`, clientCreatesAgentSessionWithProfile)
	sc.Step(`^deck client "([^"]+)" presses r on session "([^"]+)"$`, clientPressesResumeOnNamedSession)
	sc.Step(`^the state database session "([^"]+)" has a non-empty conversation id$`, sessionHasNonEmptyConversationID)
	sc.Step(`^the state database sessions "([^"]+)" and "([^"]+)" have different conversation ids$`, sessionsHaveDifferentConversationIDs)
	sc.Step(`^the audit log's most recent launch argv for session "([^"]+)" contains "([^"]+)"$`, launchArgvForSessionContains)
	sc.Step(`^the audit log's most recent launch argv for session "([^"]+)" does not contain "([^"]+)"$`, launchArgvForSessionDoesNotContain)
	sc.Step(`^the audit log has ([0-9]+) launch record(?:s)? for session "([^"]+)"$`, auditHasLaunchRecordCountForSession)
	sc.Step(`^exactly ([0-9]+) private tmux session(?:s)? match(?:es)? slug "([^"]+)"$`, exactlyNPrivateSessionsMatchSlug)
}

// fakeClaudeOnPATHForFutureClients builds the repository's fake-claude
// fixture into its own directory named exactly "claude" and records that
// directory on the harness so every subsequently started named client gets
// it prepended to a real PATH (real tmux/go remain reachable; see
// clientCreatesAgentSessionWithProfile / startNamedClient for how it is
// consumed). It must run before the client it is meant to affect is started:
// a deck client's environment is fixed at process start.
func fakeClaudeOnPATHForFutureClients(ctx context.Context) error {
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
	if err := os.WriteFile(claudeWrapper, []byte(script), 0o700); err != nil {
		return fmt.Errorf("write claude fixture wrapper: %w", err)
	}
	h.agentPATHDir = dir
	return nil
}

// clientCreatesAgentSessionWithProfile drives the real create modal by
// keystrokes only: it types the name and a fresh working directory, cycles
// the Agent field to kind, cycles the Permission profile field to profile,
// then submits. It reuses the same working-directory/sentinel setup as
// clientCreatesShellSession so the created row is comparable to a shell
// row's contract.
func clientCreatesAgentSessionWithProfile(ctx context.Context, clientName, kind, name, profile string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(clientName)
	if err != nil {
		return err
	}
	if h.workingDir == "" {
		cwd := filepath.Join(h.Home, "agent-session-cwd")
		if err := os.MkdirAll(cwd, 0o700); err != nil {
			return fmt.Errorf("create scenario working directory: %w", err)
		}
		h.workingDir, h.sentinel = cwd, []byte("deck must not alter user cwd\n")
		if err := os.WriteFile(filepath.Join(cwd, "sentinel"), h.sentinel, 0o600); err != nil {
			return fmt.Errorf("write working-directory sentinel: %w", err)
		}
	}
	if err := client.Send("n"); err != nil {
		return err
	}
	if err := client.WaitForFrame(ctx, false, "Create shell session"); err != nil {
		return err
	}
	if err := client.Send(name); err != nil {
		return err
	}
	time.Sleep(75 * time.Millisecond)
	if err := client.Send("\t" + h.workingDir); err != nil {
		return err
	}
	time.Sleep(75 * time.Millisecond)
	// Tab onto the Agent field, then cycle right until it reads kind; the
	// field order is fixed (name, cwd, agent, profile, ...), see
	// internal/tui.createFieldRows.
	if err := client.Send("\t"); err != nil {
		return err
	}
	time.Sleep(50 * time.Millisecond)
	for _, want := range createAgentOptionsOrder {
		if want == kind {
			break
		}
		if err := client.Send("\x1b[C"); err != nil { // right arrow
			return err
		}
		time.Sleep(25 * time.Millisecond)
	}
	if err := client.WaitForFrame(ctx, false, kind+" (left/right cycles"); err != nil {
		return fmt.Errorf("cycle Agent field to %q: %w", kind, err)
	}
	// Tab onto the Permission profile field, then cycle right until it
	// reads profile. Options depend on the now-selected agent, so read them
	// from the current frame rather than hard-coding claude/pi's lists here.
	if err := client.Send("\t"); err != nil {
		return err
	}
	time.Sleep(50 * time.Millisecond)
	for attempt := 0; attempt < 4; attempt++ {
		frame := client.Frame(false)
		if strings.Contains(frame, profile+" (left/right cycles") {
			break
		}
		if err := client.Send("\x1b[C"); err != nil {
			return err
		}
		time.Sleep(25 * time.Millisecond)
	}
	if err := client.WaitForFrame(ctx, false, profile+" (left/right cycles"); err != nil {
		return fmt.Errorf("cycle Permission profile field to %q: %w", profile, err)
	}
	if err := client.Send("\r"); err != nil {
		return err
	}
	return client.WaitForFrame(ctx, false, "starting")
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

func launchArgvForSessionContains(ctx context.Context, name, want string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	argv, err := mostRecentLaunchArgvForSession(h, name)
	if err != nil {
		return err
	}
	if !argvContains(argv, want) {
		return fmt.Errorf("most recent launch argv for session %q = %q, does not contain %q", name, argv, want)
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
