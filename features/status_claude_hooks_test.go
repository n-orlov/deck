package features

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

// registerClaudeHookStatusSteps keeps the Phase 2 status scenarios black-box:
// commands enter the fake agent's real tmux pane, while assertions observe only
// tmux, SQLite, the filesystem, and released TUI frames.
func registerClaudeHookStatusSteps(sc *godog.ScenarioContext) {
	sc.Step(`^session "([^"]+)"'s pane has the scenario hook environment$`, paneHasScenarioHookEnvironment)
	sc.Step(`^the scenario working directory contains no deck state$`, scenarioWorkingDirectoryContainsNoDeckState)
	sc.Step(`^fake Claude session "([^"]+)" fires "([^"]+)" for itself using (injected|conversation) identity:$`, fakeClaudeFiresForSelf)
	sc.Step(`^fake Claude session "([^"]+)" fires "([^"]+)" for session "([^"]+)" using conversation identity:$`, fakeClaudeFiresForSession)
	sc.Step(`^the state database session "([^"]+)" has hook status "([^"]+)", reason "([^"]*)", message "([^"]*)", acknowledged ([01]), and notify_epoch ([0-9]+)$`, databaseSessionHasHookStatus)
	sc.Step(`^session "([^"]+)" has one "([^"]+)" event with payload field "([^"]+)" equal to "([^"]*)"$`, sessionHasOneEventPayloadField)
	sc.Step(`^session "([^"]+)" has an audited "([^"]+)" event with payload field "([^"]+)" equal to "([^"]*)"$`, sessionHasAuditedEventPayloadField)
	sc.Step(`^deck client "([^"]+)" kills session "([^"]+)"$`, clientKillsNamedSession)
	sc.Step(`^deck client "([^"]+)" closes the session detail$`, clientClosesSessionDetail)
}

func paneHasScenarioHookEnvironment(ctx context.Context, name string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	slug, err := sessionSlugByName(h, name)
	if err != nil {
		return err
	}
	sessionID, err := sessionIDByName(h, name)
	if err != nil {
		return err
	}
	target := "deck_" + slug
	for key, want := range map[string]string{"DECK_HOME": h.Home, "DECK_SESSION_ID": sessionID} {
		output, err := tmuxOutput(ctx, h, "show-environment", "-t", target, key)
		if err != nil {
			return fmt.Errorf("read %s from pane session %q: %w", key, target, err)
		}
		if got := strings.TrimSpace(string(output)); got != key+"="+want {
			return fmt.Errorf("pane session %q environment %s = %q, want %q", target, key, got, key+"="+want)
		}
	}
	return nil
}

func scenarioWorkingDirectoryContainsNoDeckState(ctx context.Context) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	if h.workingDir == "" {
		return fmt.Errorf("scenario has no agent working directory")
	}
	forbidden := map[string]bool{"state.db": true, "clock.now": true, "config.toml": true, "deck.jsonl": true}
	return filepath.WalkDir(h.workingDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if forbidden[entry.Name()] {
			return fmt.Errorf("deck state escaped scenario DECK_HOME into working directory: %s", path)
		}
		return nil
	})
}

func fakeClaudeFiresForSelf(ctx context.Context, emitter, event, identity string, table *godog.Table) error {
	return fakeClaudeFires(ctx, emitter, event, emitter, identity, table)
}

func fakeClaudeFiresForSession(ctx context.Context, emitter, event, target string, table *godog.Table) error {
	return fakeClaudeFires(ctx, emitter, event, target, "conversation", table)
}

func fakeClaudeFires(ctx context.Context, emitter, event, target, identity string, table *godog.Table) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	payload := make(map[string]any)
	for _, row := range table.Rows {
		if len(row.Cells) != 2 {
			return fmt.Errorf("hook payload row has %d cells, want key and value", len(row.Cells))
		}
		payload[row.Cells[0].Value] = row.Cells[1].Value
	}
	if identity == "conversation" {
		conversationID, err := sessionConversationID(h, target)
		if err != nil {
			return err
		}
		if conversationID == "" {
			return fmt.Errorf("target session %q has no conversation identity", target)
		}
		payload["session_id"] = conversationID
	} else if identity != "injected" || emitter != target {
		return fmt.Errorf("invalid hook identity %q from %q to %q", identity, emitter, target)
	}
	request, err := json.Marshal(map[string]any{"command": "hook", "event": event, "payload": payload})
	if err != nil {
		return err
	}
	emitterSlug, err := sessionSlugByName(h, emitter)
	if err != nil {
		return err
	}
	targetID, err := sessionIDByName(h, target)
	if err != nil {
		return err
	}
	db, err := openObservedDatabase(h)
	if err != nil {
		return err
	}
	defer db.Close()
	var before int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM events WHERE session_id = ?`, targetID).Scan(&before); err != nil {
		return err
	}
	paneTarget := "deck_" + emitterSlug
	if _, err := tmuxOutput(ctx, h, "send-keys", "-t", paneTarget, "-l", string(request)); err != nil {
		return fmt.Errorf("send %s command to fake Claude pane %q: %w", event, paneTarget, err)
	}
	if _, err := tmuxOutput(ctx, h, "send-keys", "-t", paneTarget, "Enter"); err != nil {
		return fmt.Errorf("submit %s command to fake Claude pane %q: %w", event, paneTarget, err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		var count int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM events WHERE session_id = ?`, targetID).Scan(&count); err != nil {
			return err
		}
		if count > before {
			return nil
		}
		if time.Now().After(deadline) {
			output, _ := tmuxOutput(ctx, h, "capture-pane", "-p", "-S", "-", "-t", paneTarget)
			return fmt.Errorf("fake Claude pane %q did not persist %s hook; pane:\n%s", paneTarget, event, output)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func databaseSessionHasHookStatus(ctx context.Context, name, wantStatus, wantReason, wantMessage, acknowledgedText, epochText string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	wantAcknowledged, _ := strconv.Atoi(acknowledgedText)
	wantEpoch, _ := strconv.Atoi(epochText)
	db, err := openObservedDatabase(h)
	if err != nil {
		return err
	}
	defer db.Close()
	deadline := time.Now().Add(3 * time.Second)
	for {
		var status, source, reason, message string
		var acknowledged, epoch int
		err := db.QueryRowContext(ctx, `SELECT status, status_source, COALESCE(status_reason, ''), COALESCE(last_message, ''), acknowledged, notify_epoch FROM sessions WHERE name = ?`, name).Scan(&status, &source, &reason, &message, &acknowledged, &epoch)
		if err != nil {
			return fmt.Errorf("observe hook status for session %q: %w", name, err)
		}
		if status == wantStatus && source == "hook" && reason == wantReason && message == wantMessage && acknowledged == wantAcknowledged && epoch == wantEpoch {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("session %q = status %q source %q reason %q message %q acknowledged=%d epoch=%d; want %q hook %q %q %d %d", name, status, source, reason, message, acknowledged, epoch, wantStatus, wantReason, wantMessage, wantAcknowledged, wantEpoch)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func sessionHasOneEventPayloadField(ctx context.Context, name, kind, field, want string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	db, err := openObservedDatabase(h)
	if err != nil {
		return err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, `SELECT payload FROM events WHERE session_id = (SELECT id FROM sessions WHERE name = ?) AND kind = ?`, name, kind)
	if err != nil {
		return err
	}
	defer rows.Close()
	var payloads []string
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return err
		}
		payloads = append(payloads, raw)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(payloads) != 1 {
		return fmt.Errorf("session %q has %d %q events, want one", name, len(payloads), kind)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(payloads[0]), &payload); err != nil {
		return fmt.Errorf("decode preserved %s payload: %w", kind, err)
	}
	if got := fmt.Sprint(payload[field]); got != want {
		return fmt.Errorf("session %q %s payload field %q = %q, want %q", name, kind, field, got, want)
	}
	return nil
}

func sessionHasAuditedEventPayloadField(ctx context.Context, name, kind, field, want string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	db, err := openObservedDatabase(h)
	if err != nil {
		return err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, `SELECT payload FROM events WHERE session_id = (SELECT id FROM sessions WHERE name = ?) AND kind = ?`, name, kind)
	if err != nil {
		return err
	}
	defer rows.Close()
	matches := 0
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return err
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(raw), &payload); err != nil {
			return fmt.Errorf("decode preserved %s payload: %w", kind, err)
		}
		if fmt.Sprint(payload[field]) == want {
			matches++
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if matches != 1 {
		return fmt.Errorf("session %q has %d %q events with payload field %q equal to %q, want one", name, matches, kind, field, want)
	}
	return nil
}

func clientClosesSessionDetail(ctx context.Context, clientName string) error {
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

func clientKillsNamedSession(ctx context.Context, clientName, sessionName string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(clientName)
	if err != nil {
		return err
	}
	if err := selectRowByName(client, sessionName); err != nil {
		return err
	}
	if err := client.Send("x"); err != nil {
		return err
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		db, err := sql.Open("sqlite", filepath.Join(h.Home, "state.db"))
		if err != nil {
			return err
		}
		var status, source string
		var killed int
		err = db.QueryRowContext(ctx, `SELECT status, status_source, killed_by_user FROM sessions WHERE name = ?`, sessionName).Scan(&status, &source, &killed)
		_ = db.Close()
		if err == nil && status == "stopped" && source == "user" && killed == 1 {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("session %q was not durably killed by client %q: status=%q source=%q killed=%d err=%v", sessionName, clientName, status, source, killed, err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
