package features

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

// registerHarnessNameSteps backs the @harness-names scenarios (R264): names
// travel between deck and a harness through the fake agent's own persisted
// state (its transcript records and the hooks it fires) and deck's store and
// screen. Every wait is a poll on one of those observables with a deadline;
// no step relies on a fixed sleep to be synchronised.
func registerHarnessNameSteps(sc *godog.ScenarioContext) {
	registerHarnessNameCodexSteps(sc)
	registerHarnessNameCopilotSteps(sc)
	registerHarnessNamePiSteps(sc)
	sc.Step(`^deck client "([^"]+)" creates claude session with a blank name expecting the derived name "([^"]+)"$`, clientCreatesClaudeSessionWithBlankName)
	sc.Step(`^deck client "([^"]+)" is started with the fake agents in a fresh directory labelled "([^"]+)" with the clock frozen at "([^"]+)"$`, clientStartedWithFakeAgentsInFreshDirectory)
	sc.Step(`^fake Claude session "([^"]+)" generates the title "([^"]+)"$`, fakeClaudeGeneratesTitle)
	sc.Step(`^fake Claude session "([^"]+)" is asked to generate the title "([^"]+)"$`, fakeClaudeIsAskedToGenerateTitle)
	sc.Step(`^fake Claude session "([^"]+)" is renamed inside Claude to "([^"]+)"$`, fakeClaudeIsRenamedInside)
	sc.Step(`^the fake Claude transcript of session "([^"]+)" has a "(custom-title|agent-name|ai-title)" record titled "([^"]+)"$`, fakeClaudeTranscriptHasRecord)
	sc.Step(`^the fake Claude transcript of session "([^"]+)" has no "(custom-title|agent-name|ai-title)" record$`, fakeClaudeTranscriptHasNoRecord)
	sc.Step(`^the fake Claude transcript of session "([^"]+)" has no record titled "([^"]+)"$`, fakeClaudeTranscriptHasNoTitled)
	sc.Step(`^deck client "([^"]+)" renames session "([^"]+)" to "([^"]+)" from its detail dialog$`, clientRenamesSessionFromDetail)
	sc.Step(`^the state database session "([^"]+)" has name source "(user|derived|harness)"$`, sessionHasNameSource)
	sc.Step(`^the state database session "([^"]+)" has no pending harness name$`, sessionHasNoPendingHarnessName)
	sc.Step(`^the state database session "([^"]+)" has no "([^"]+)" event with reason containing "([^"]+)"$`, sessionHasNoEventWithReasonContaining)
}

// clientCreatesClaudeSessionWithBlankName submits the create modal for Claude
// with the name left blank, so the row is created name_source=derived with the
// clock-derived default name the step names.
func clientCreatesClaudeSessionWithBlankName(ctx context.Context, clientName, derived string) error {
	return clientCreatesAgentSessionWithBlankName(ctx, clientName, "claude", derived)
}

// clientCreatesAgentSessionWithBlankName is the blank-name create for any
// agent the create modal offers.
func clientCreatesAgentSessionWithBlankName(ctx context.Context, clientName, agentName, derived string) error {
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
	if err := ensureCreateModalAgent(ctx, client, agentName); err != nil {
		return err
	}
	offsets := auditOffsets(h)
	if err := client.Send("\r"); err != nil {
		return err
	}
	return waitForAgentCreateRecorded(ctx, h, client, derived, offsets)
}

// claudeTranscriptFile is the path of the session's transcript in the fake
// agent's HOME, derived from the row's own cwd and conversation id.
func claudeTranscriptFile(h *ScenarioHarness, name string) (string, error) {
	if h.agentHOMEDir == "" {
		return "", fmt.Errorf("fixture HOME directory was not configured (call the fake claude PATH step first)")
	}
	db, err := openObservedDatabase(h)
	if err != nil {
		return "", err
	}
	defer db.Close()
	var cwd, conversation string
	if err := db.QueryRow(`SELECT cwd, COALESCE(conversation_id, '') FROM sessions WHERE name = ?`, name).Scan(&cwd, &conversation); err != nil {
		return "", fmt.Errorf("observe session %q cwd: %w", name, err)
	}
	if conversation == "" {
		return "", fmt.Errorf("session %q has no conversation id", name)
	}
	project := strings.ReplaceAll(cwd, string(os.PathSeparator), "-")
	return filepath.Join(h.agentHOMEDir, ".claude", "projects", project, conversation+".jsonl"), nil
}

// transcriptTitles returns the (type, title) of every title record in path; a
// missing file is no records.
func transcriptTitles(path string) ([][2]string, error) {
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var titles [][2]string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var rec struct {
			Type        string `json:"type"`
			CustomTitle string `json:"customTitle"`
			AgentName   string `json:"agentName"`
			AITitle     string `json:"aiTitle"`
		}
		if json.Unmarshal(scanner.Bytes(), &rec) != nil {
			continue
		}
		switch rec.Type {
		case "custom-title":
			titles = append(titles, [2]string{rec.Type, rec.CustomTitle})
		case "agent-name":
			titles = append(titles, [2]string{rec.Type, rec.AgentName})
		case "ai-title":
			titles = append(titles, [2]string{rec.Type, rec.AITitle})
		}
	}
	return titles, scanner.Err()
}

// pollTranscript polls the session's transcript until ok(titles) holds or the
// deadline passes. The file is resolved through the session's name on every
// pass, so it suits a check on a row nothing is about to rename.
func pollTranscript(ctx context.Context, name, what string, ok func([][2]string) bool) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	return pollTranscriptVia(ctx, name, what, ok, func() (string, error) { return claudeTranscriptFile(h, name) })
}

// pollTranscriptAt polls a transcript whose path was resolved before the step
// acted. A step that makes Claude write a title record can race deck's own
// tick adopting that title: the row is renamed, and a lookup by its old name
// then finds no row for the rest of the deadline, although the record is
// there. The path does not change with the name, so it is resolved first.
func pollTranscriptAt(ctx context.Context, name, path, what string, ok func([][2]string) bool) error {
	return pollTranscriptVia(ctx, name, what, ok, func() (string, error) { return path, nil })
}

func pollTranscriptVia(ctx context.Context, name, what string, ok func([][2]string) bool, resolve func() (string, error)) error {
	ctx, cancel := withDefaultWaitDeadline(ctx)
	defer cancel()
	for {
		path, pathErr := resolve()
		var titles [][2]string
		if pathErr == nil {
			var err error
			if titles, err = transcriptTitles(path); err != nil {
				return err
			}
			if ok(titles) {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			if pathErr != nil {
				return fmt.Errorf("fake Claude transcript of session %q never showed %s: %w", name, what, pathErr)
			}
			return fmt.Errorf("fake Claude transcript of session %q never showed %s; records %v", name, what, titles)
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func hasTitleRecord(titles [][2]string, kind, title string) bool {
	for _, t := range titles {
		if t[0] == kind && t[1] == title {
			return true
		}
	}
	return false
}

func fakeClaudeTranscriptHasRecord(ctx context.Context, name, kind, title string) error {
	return pollTranscript(ctx, name, fmt.Sprintf("a %s record titled %q", kind, title), func(titles [][2]string) bool {
		return hasTitleRecord(titles, kind, title)
	})
}

// fakeClaudeTranscriptHasNoRecord is a point-in-time absence check: callers
// order it after a step that already proved the fake processed its input.
func fakeClaudeTranscriptHasNoRecord(ctx context.Context, name, kind string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	path, err := claudeTranscriptFile(h, name)
	if err != nil {
		return err
	}
	titles, err := transcriptTitles(path)
	if err != nil {
		return err
	}
	for _, t := range titles {
		if t[0] == kind {
			return fmt.Errorf("fake Claude transcript of session %q holds a %s record %q, want none", name, kind, t[1])
		}
	}
	return nil
}

func fakeClaudeTranscriptHasNoTitled(ctx context.Context, name, title string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	path, err := claudeTranscriptFile(h, name)
	if err != nil {
		return err
	}
	titles, err := transcriptTitles(path)
	if err != nil {
		return err
	}
	for _, t := range titles {
		if t[1] == title {
			return fmt.Errorf("fake Claude transcript of session %q holds a %s record titled %q, want none", name, t[0], title)
		}
	}
	return nil
}

// sendFakeClaudeNameCommand types a pane command into the fake's own pane.
func sendFakeClaudeNameCommand(ctx context.Context, name string, command map[string]any) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	slug, err := sessionSlugByName(h, name)
	if err != nil {
		return err
	}
	request, err := json.Marshal(command)
	if err != nil {
		return err
	}
	target := "deck_" + slug
	if _, err := tmuxOutput(ctx, h, "send-keys", "-t", target, "-l", string(request)); err != nil {
		return fmt.Errorf("send %v to fake Claude pane %q: %w", command["command"], target, err)
	}
	if _, err := tmuxOutput(ctx, h, "send-keys", "-t", target, "Enter"); err != nil {
		return fmt.Errorf("submit %v to fake Claude pane %q: %w", command["command"], target, err)
	}
	return nil
}

func fakeClaudeGeneratesTitle(ctx context.Context, name, title string) error {
	return fakeClaudeWritesTitleRecord(ctx, name, "ai-title", "ai-title", title)
}

// fakeClaudeWritesTitleRecord sends the fake the command that appends a kind
// record and waits for it in the transcript. The transcript path is resolved
// before sending (see pollTranscriptAt): deck may adopt the title and rename
// the row before the wait begins.
func fakeClaudeWritesTitleRecord(ctx context.Context, name, command, kind, title string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	path, pathErr := claudeTranscriptFile(h, name)
	if err := sendFakeClaudeNameCommand(ctx, name, map[string]any{"command": command, "name": title}); err != nil {
		return err
	}
	what := fmt.Sprintf("a %s record titled %q", kind, title)
	has := func(titles [][2]string) bool { return hasTitleRecord(titles, kind, title) }
	if pathErr != nil {
		return pollTranscript(ctx, name, what, has)
	}
	return pollTranscriptAt(ctx, name, path, what, has)
}

// fakeClaudeIsAskedToGenerateTitle sends the auto-title command without
// waiting for a record: the caller proves, after a later hook the fake has
// already processed in order, that Claude suppressed it.
func fakeClaudeIsAskedToGenerateTitle(ctx context.Context, name, title string) error {
	return sendFakeClaudeNameCommand(ctx, name, map[string]any{"command": "ai-title", "name": title})
}

// clientStartedWithFakeAgentsInFreshDirectory is the frozen-clock fresh-directory
// start (so a blank name derives a deterministic <dir>-<MMDD-HHMM>) with the
// scenario's fake-agent PATH and HOME, which StartClientInDir does not apply.
func clientStartedWithFakeAgentsInFreshDirectory(ctx context.Context, name, label, iso string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	if h.agentPATHDir == "" || h.agentHOMEDir == "" {
		return fmt.Errorf("a fake agent binary must be put on PATH before the client starts")
	}
	h.clientEnv = append(h.clientEnv, "DECK_CLOCK="+iso)
	dir := filepath.Join(h.Home, label) // a short basename keeps the derived name inside the sidebar row
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create directory labelled %q: %w", label, err)
	}
	registerNamedDirectory(h, label, dir)
	client, err := h.StartNamedClientInDir(ctx, name, dir,
		"PATH="+h.agentPATHDir+string(os.PathListSeparator)+os.Getenv("PATH"), "HOME="+h.agentHOMEDir)
	if err != nil {
		return err
	}
	return client.WaitForFrame(ctx, false, "deck - sessions")
}

func fakeClaudeIsRenamedInside(ctx context.Context, name, title string) error {
	return fakeClaudeWritesTitleRecord(ctx, name, "rename", "custom-title", title)
}

// clientRenamesSessionFromDetail is the user's own rename: the `i` detail
// dialog, `r`, the new name, enter, then back to the list.
func clientRenamesSessionFromDetail(ctx context.Context, clientName, name, newName string) error {
	for _, step := range []func(context.Context) error{
		func(ctx context.Context) error { return clientOpensDetailForSession(ctx, clientName, name) },
		func(ctx context.Context) error { return clientOpensRenameDialog(ctx, clientName) },
		func(ctx context.Context) error { return clientTypesNameIntoRenameField(ctx, clientName, newName) },
		func(ctx context.Context) error { return clientSubmitsRenameDialog(ctx, clientName) },
		func(ctx context.Context) error { return clientClosesSessionDetail(ctx, clientName) },
	} {
		if err := step(ctx); err != nil {
			return err
		}
	}
	return nil
}

// clientTypesNameIntoRenameField types the new name and waits until the typed
// text is on the screen, rather than pausing for a fixed time.
func clientTypesNameIntoRenameField(ctx context.Context, clientName, newName string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(clientName)
	if err != nil {
		return err
	}
	return client.SendAwaitingVisible(ctx, newName, newName)
}

// pollSessionColumn polls one text column of the named row until it equals want.
func pollSessionColumn(ctx context.Context, name, column, want string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	db, err := openObservedDatabase(h)
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, cancel := withDefaultWaitDeadline(ctx)
	defer cancel()
	var got string
	for {
		err := db.QueryRowContext(ctx, `SELECT COALESCE(CAST(`+column+` AS TEXT), '') FROM sessions WHERE name = ?`, name).Scan(&got)
		if err == nil && got == want {
			return nil
		}
		if err != nil && err != sql.ErrNoRows {
			return fmt.Errorf("observe %s of session %q: %w", column, name, err)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("session %q %s = %q (row found: %v), want %q", name, column, got, err == nil, want)
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func sessionHasNameSource(ctx context.Context, name, want string) error {
	return pollSessionColumn(ctx, name, "name_source", want)
}

func sessionHasNoPendingHarnessName(ctx context.Context, name string) error {
	return pollSessionColumn(ctx, name, "pending_harness_name", "")
}

func sessionHasNoEventWithReasonContaining(ctx context.Context, name, kind, fragment string) error {
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
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE session_id = (SELECT id FROM sessions WHERE name = ?) AND kind = ? AND COALESCE(reason, '') LIKE '%' || ? || '%'`, name, kind, fragment).Scan(&count); err != nil {
		return err
	}
	if count != 0 {
		return fmt.Errorf("session %q has %d %q events with reason containing %q, want none", name, count, kind, fragment)
	}
	return nil
}
