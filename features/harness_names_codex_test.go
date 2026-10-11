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

// registerHarnessNameCodexSteps backs the Codex @harness-names scenarios
// (R264). Codex names a thread in its own state_5.sqlite (threads.name),
// appends the naming to session_index.jsonl and puts "<name> | <cwd>" in the
// pane title; the steps assert through those records and deck's store and
// screen, polling each with a deadline and never pausing for a fixed time.
func registerHarnessNameCodexSteps(sc *godog.ScenarioContext) {
	sc.Step(`^deck client "([^"]+)" creates codex session with a blank name expecting the derived name "([^"]+)"$`, clientCreatesCodexSessionWithBlankName)
	sc.Step(`^fake Codex session "([^"]+)" finishes its turn with "([^"]*)"$`, fakeCodexFinishesTurn)
	sc.Step(`^fake Codex session "([^"]+)" generates the title "([^"]+)"$`, fakeCodexGeneratesTitle)
	sc.Step(`^fake Codex session "([^"]+)" is asked to generate the title "([^"]+)"$`, fakeCodexIsAskedToGenerateTitle)
	sc.Step(`^fake Codex session "([^"]+)" is renamed inside Codex to "([^"]+)"$`, fakeCodexIsRenamedInside)
	sc.Step(`^the fake Codex pane of session "([^"]+)" is titled "([^"]+)"$`, fakeCodexPaneIsTitled)
	sc.Step(`^the fake Codex thread of session "([^"]+)" is named "([^"]+)"$`, fakeCodexThreadIsNamed)
	sc.Step(`^the fake Codex thread of session "([^"]+)" has no name$`, fakeCodexThreadHasNoName)
	sc.Step(`^the fake Codex session index of session "([^"]+)" ends with the name "([^"]+)"$`, fakeCodexIndexEndsWith)
}

func clientCreatesCodexSessionWithBlankName(ctx context.Context, clientName, derived string) error {
	return clientCreatesAgentSessionWithBlankName(ctx, clientName, "codex", derived)
}

// sendFakeCodexPaneLine types one line into the fake Codex pane: a JSON pane
// command, or the "/rename <name>" a human types inside Codex.
func sendFakeCodexPaneLine(ctx context.Context, name, line string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	slug, err := sessionSlugByName(h, name)
	if err != nil {
		return err
	}
	target := "deck_" + slug
	if _, err := tmuxOutput(ctx, h, "send-keys", "-t", target, "-l", line); err != nil {
		return fmt.Errorf("send %q to fake Codex pane %q: %w", line, target, err)
	}
	if _, err := tmuxOutput(ctx, h, "send-keys", "-t", target, "Enter"); err != nil {
		return fmt.Errorf("submit %q to fake Codex pane %q: %w", line, target, err)
	}
	return nil
}

func sendFakeCodexCommand(ctx context.Context, name string, command map[string]any) error {
	request, err := json.Marshal(command)
	if err != nil {
		return err
	}
	return sendFakeCodexPaneLine(ctx, name, string(request))
}

// fakeCodexFinishesTurn fires the Stop hook through the fake's own "stop"
// pane command and waits for the fake to print the message, so the hook has
// been delivered by the time the step returns.
func fakeCodexFinishesTurn(ctx context.Context, name, message string) error {
	if err := sendFakeCodexCommand(ctx, name, map[string]any{"command": "stop", "message": message}); err != nil {
		return err
	}
	return fakeCodexPaneShows(ctx, name, "• "+message)
}

// fakeCodexPaneShows polls the pane's own scrollback for text.
func fakeCodexPaneShows(ctx context.Context, name, text string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	slug, err := sessionSlugByName(h, name)
	if err != nil {
		return err
	}
	ctx, cancel := withDefaultWaitDeadline(ctx)
	defer cancel()
	var capture []byte
	for {
		capture, _ = tmuxOutput(ctx, h, "capture-pane", "-p", "-J", "-S", "-", "-t", "deck_"+slug)
		if strings.Contains(string(capture), text) {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("fake Codex pane of session %q never showed %q; last capture:\n%s", name, text, capture)
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func fakeCodexGeneratesTitle(ctx context.Context, name, title string) error {
	if err := sendFakeCodexCommand(ctx, name, map[string]any{"command": "ai-title", "name": title}); err != nil {
		return err
	}
	return fakeCodexThreadIsNamed(ctx, name, title)
}

// fakeCodexIsAskedToGenerateTitle sends the auto-title command without waiting
// for a result: the caller proves, after a later hook the fake processes in
// order, that Codex ignored it for a thread that was already named.
func fakeCodexIsAskedToGenerateTitle(ctx context.Context, name, title string) error {
	return sendFakeCodexCommand(ctx, name, map[string]any{"command": "ai-title", "name": title})
}

func fakeCodexIsRenamedInside(ctx context.Context, name, title string) error {
	if err := sendFakeCodexPaneLine(ctx, name, "/rename "+title); err != nil {
		return err
	}
	return fakeCodexThreadIsNamed(ctx, name, title)
}

// fakeCodexPaneIsTitled polls tmux's pane title, which the fake sets with
// OSC 0 as "<name> | <cwd basename>" the moment a /rename arrives.
func fakeCodexPaneIsTitled(ctx context.Context, name, title string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	slug, err := sessionSlugByName(h, name)
	if err != nil {
		return err
	}
	ctx, cancel := withDefaultWaitDeadline(ctx)
	defer cancel()
	var got string
	for {
		out, _ := tmuxOutput(ctx, h, "display-message", "-p", "-t", "deck_"+slug, "#{pane_title}")
		got = strings.TrimSpace(string(out))
		if strings.HasPrefix(got, title+" | ") {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("fake Codex pane of session %q is titled %q, want %q followed by \" | <cwd>\"", name, got, title)
		case <-time.After(20 * time.Millisecond):
		}
	}
}

// codexConversation is the conversation id deck holds for the row.
func codexConversation(h *ScenarioHarness, name string) (string, error) {
	db, err := openObservedDatabase(h)
	if err != nil {
		return "", err
	}
	defer db.Close()
	var id string
	if err := db.QueryRow(`SELECT COALESCE(conversation_id, '') FROM sessions WHERE name = ?`, name).Scan(&id); err != nil {
		return "", fmt.Errorf("observe session %q conversation id: %w", name, err)
	}
	if id == "" {
		return "", fmt.Errorf("session %q has no conversation id", name)
	}
	return id, nil
}

// codexThreadRow reads the fake's own threads row; found is false while the
// database or the row does not exist yet.
func codexThreadRow(h *ScenarioHarness, id string) (name sql.NullString, found bool, err error) {
	path := filepath.Join(h.agentHOMEDir, ".codex", "state_5.sqlite")
	if _, statErr := os.Stat(path); statErr != nil {
		return name, false, nil
	}
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro&_pragma=busy_timeout(2000)")
	if err != nil {
		return name, false, err
	}
	defer db.Close()
	switch err := db.QueryRow(`SELECT name FROM threads WHERE id = ?`, id).Scan(&name); err {
	case nil:
		return name, true, nil
	case sql.ErrNoRows:
		return name, false, nil
	default:
		return name, false, err
	}
}

// pollCodexThread polls the thread row of the named session until ok holds.
func pollCodexThread(ctx context.Context, session, what string, ok func(sql.NullString) bool) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	ctx, cancel := withDefaultWaitDeadline(ctx)
	defer cancel()
	var last sql.NullString
	for {
		id, idErr := codexConversation(h, session)
		if idErr == nil {
			name, found, rowErr := codexThreadRow(h, id)
			if rowErr == nil && found && ok(name) {
				return nil
			}
			last = name
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("fake Codex thread of session %q never showed %s (conversation id error: %w; last name %v)", session, what, idErr, last)
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func fakeCodexThreadIsNamed(ctx context.Context, session, want string) error {
	return pollCodexThread(ctx, session, fmt.Sprintf("threads.name %q", want), func(name sql.NullString) bool {
		return name.Valid && name.String == want
	})
}

// fakeCodexThreadHasNoName is a point-in-time check: callers order it after a
// step that already proved the fake processed its input.
func fakeCodexThreadHasNoName(ctx context.Context, session string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	id, err := codexConversation(h, session)
	if err != nil {
		return err
	}
	name, found, err := codexThreadRow(h, id)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("fake Codex has no thread %s for session %q", id, session)
	}
	if name.Valid {
		return fmt.Errorf("fake Codex thread of session %q is named %q, want NULL", session, name.String)
	}
	return nil
}

// fakeCodexIndexEndsWith polls session_index.jsonl for the thread's latest
// line (the latest line per id wins, as in the real file).
func fakeCodexIndexEndsWith(ctx context.Context, session, want string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	ctx, cancel := withDefaultWaitDeadline(ctx)
	defer cancel()
	var latest string
	for {
		if id, idErr := codexConversation(h, session); idErr == nil {
			latest = latestCodexIndexName(filepath.Join(h.agentHOMEDir, ".codex", "session_index.jsonl"), id)
			if latest == want {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("latest session_index.jsonl name of session %q is %q, want %q", session, latest, want)
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func latestCodexIndexName(path, id string) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()
	var latest string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var rec struct {
			ID         string `json:"id"`
			ThreadName string `json:"thread_name"`
		}
		if json.Unmarshal(scanner.Bytes(), &rec) == nil && rec.ID == id {
			latest = rec.ThreadName
		}
	}
	return latest
}
