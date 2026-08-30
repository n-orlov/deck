package features

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
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
	sc.Step(`^fake Claude session "([^"]+)" exits its pane cleanly$`, fakeClaudeExitsPaneCleanly)
	sc.Step(`^the released deck _hook receives "([^"]+)" for session "([^"]+)" using (injected|conversation) identity:$`, releasedHookFiresForSession)
	sc.Step(`^the state database session "([^"]+)" has hook status "([^"]+)", reason "([^"]*)", message "([^"]*)", acknowledged ([01]), and notify_epoch ([0-9]+)$`, databaseSessionHasHookStatus)
	sc.Step(`^the state database session "([^"]+)" is repaired to "([^"]+)" from "([^"]+)" with reason "([^"]*)", message "([^"]*)", acknowledged ([01]), and notify_epoch ([0-9]+)$`, databaseSessionIsRepairedTo)
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

// fakeClaudeExitsPaneCleanly sends this fixture's "exit" pane command, which
// ends its runCommands loop and, via the long-running wrapper's exec, this
// process, with status 0. Under deck's server-wide remain-on-exit=failed a
// zero-exit pane is destroyed rather than retained, and since a scenario
// session always occupies its tmux session's only window/pane, the whole
// "deck_<slug>" session disappears with it. It polls for that disappearance
// rather than returning as soon as send-keys completes, so a caller that
// immediately fires a hook afterward is guaranteed a genuinely dead pane
// underneath it, not a race against the exit still landing.
func fakeClaudeExitsPaneCleanly(ctx context.Context, name string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	slug, err := sessionSlugByName(h, name)
	if err != nil {
		return err
	}
	target := "deck_" + slug
	request, err := json.Marshal(map[string]any{"command": "exit"})
	if err != nil {
		return err
	}
	if _, err := tmuxOutput(ctx, h, "send-keys", "-t", target, "-l", string(request)); err != nil {
		return fmt.Errorf("send exit command to fake Claude pane %q: %w", target, err)
	}
	if _, err := tmuxOutput(ctx, h, "send-keys", "-t", target, "Enter"); err != nil {
		return fmt.Errorf("submit exit command to fake Claude pane %q: %w", target, err)
	}
	return privateSessionDoesNotExist(ctx, target)
}

// releasedHookFiresForSession delivers a hook straight to the released deck
// _hook subcommand, exactly as a real Claude hook subprocess does: it is a
// one-shot invocation independent of whatever the interactive pane is doing
// (or whether it still exists at all), never a send-keys into any pane. This
// is the correct route once a scenario has driven the target's own pane to a
// genuine, confirmed death (fakeClaudeExitsPaneCleanly above) -- delivering
// through the dead pane is impossible, and delivering through some other
// still-live pane would misrepresent which process the hook came from.
// Identity is supplied the same way releasedHookForSession (assertions_test.go)
// already does for the single-purpose "released running/waiting hook" steps:
// DECK_SESSION_ID (and, when the row's own launch took a lease, its current
// DECK_LAUNCH_GENERATION) stand in for the pane environment a real hook
// subprocess would have inherited.
func releasedHookFiresForSession(ctx context.Context, event, target, identity string, table *godog.Table) error {
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
	} else if identity != "injected" {
		return fmt.Errorf("invalid hook identity %q", identity)
	}
	if _, exists := payload["hook_event_name"]; !exists {
		payload["hook_event_name"] = event
	}
	request, err := json.Marshal(payload)
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
	var leaseOwner string
	err = db.QueryRowContext(ctx, `SELECT COALESCE(launch_lease_owner, '') FROM sessions WHERE id = ?`, targetID).Scan(&leaseOwner)
	db.Close()
	if err != nil {
		return fmt.Errorf("resolve launch lease owner for session %q: %w", target, err)
	}
	env := []string{"DECK_SESSION_ID=" + targetID}
	if _, generation, found := strings.Cut(leaseOwner, "#"); found && generation != "" {
		env = append(env, "DECK_LAUNCH_GENERATION="+generation)
	}
	commandCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(commandCtx, h.Binary, "_hook")
	cmd.Env = append(os.Environ(), h.Environment(env...)...)
	cmd.Stdin = bytes.NewReader(request)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("released deck _hook for session %q event %q: %w: %s", target, event, err, strings.TrimSpace(string(output)))
	}
	return nil
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

// databaseSessionIsRepairedTo asserts the state SPEC §7's self-heal
// (internal/service.Service.repairTerminalRowWithLivePane, R76) leaves behind
// once it has fired: the tmux-sourced correction is applied synchronously by
// the post-hook reconcile pass that runs inside the same "deck _hook"
// subprocess invocation (cmd/deck/main.go's runHook), before the hook ever
// returns to its caller. Currently unused: task 902 (per task 901's finding
// F40) narrowed the repair so a hook's own bare "error" write -- this step's
// original use, when task 701 still made the repair reach it unconditionally
// -- is no longer touched by it at all, and task 903 reverted
// status_claude_hooks.feature's StopFailure assertion back onto that
// unrepaired hook verdict; no scenario in this file calls this step today.
// Unlike databaseSessionHasHookStatus,
// this does not require status_source "hook": the repair is tmux-sourced by
// definition, and asserting that source is the whole point of this step.
func databaseSessionIsRepairedTo(ctx context.Context, name, wantStatus, wantSource, wantReason, wantMessage string, acknowledgedText, epochText string) error {
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
			return fmt.Errorf("observe repaired status for session %q: %w", name, err)
		}
		if status == wantStatus && source == wantSource && reason == wantReason && message == wantMessage && acknowledged == wantAcknowledged && epoch == wantEpoch {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("session %q = status %q source %q reason %q message %q acknowledged=%d epoch=%d; want %q from %q %q %q %d %d", name, status, source, reason, message, acknowledged, epoch, wantStatus, wantSource, wantReason, wantMessage, wantAcknowledged, wantEpoch)
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
	if err := selectRowByName(ctx, client, sessionName); err != nil {
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
