package features

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

// registerCopilotHookStatusSteps backs features/status_copilot_hooks.feature
// (R218, R224): fake-copilot's pane commands fire Copilot's real hook shapes
// through deck's own plugin entries, and the assertions read the state database.
func registerCopilotHookStatusSteps(sc *godog.ScenarioContext) {
	sc.Step(`^fake Copilot session "([^"]+)" is prompted with "([^"]*)" and finishes its turn$`, fakeCopilotPromptedAndFinishes)
	sc.Step(`^fake Copilot session "([^"]+)" starts working on "([^"]*)"$`, fakeCopilotStartsWorking)
	sc.Step(`^fake Copilot session "([^"]+)" finishes its turn$`, fakeCopilotFinishesTurn)
	sc.Step(`^fake Copilot session "([^"]+)" raises a "([^"]+)" notification$`, fakeCopilotRaisesNotification)
	sc.Step(`^fake Copilot session "([^"]+)" exits$`, fakeCopilotExits)
	sc.Step(`^the state database session "([^"]+)" has hook status "([^"]+)" with reason "([^"]*)"$`, sessionHasHookStatusWithReason)
	sc.Step(`^session "([^"]+)" has a "([^"]+)" event$`, sessionHasAnEventOfKind)
}

// copilotEventCounts reads how many events of each kind the row has.
func copilotEventCounts(ctx context.Context, h *ScenarioHarness, sessionID string, kinds []string) (map[string]int, error) {
	db, err := openObservedDatabase(h)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	counts := make(map[string]int, len(kinds))
	for _, kind := range kinds {
		var n int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM events WHERE session_id = ? AND kind = ?`, sessionID, kind).Scan(&n); err != nil {
			return nil, err
		}
		counts[kind] = n
	}
	return counts, nil
}

// sendCopilotCommand writes one JSON pane command to the named fake-copilot
// session and returns once the row's event log holds a new event of every kind
// in wait: the hook has then written its status through the real receiver.
func sendCopilotCommand(ctx context.Context, name string, command map[string]any, wait ...string) error {
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
	before, err := copilotEventCounts(ctx, h, sessionID, wait)
	if err != nil {
		return err
	}
	request, err := json.Marshal(command)
	if err != nil {
		return err
	}
	target := "deck_" + slug
	if _, err := tmuxOutput(ctx, h, "send-keys", "-t", target, "-l", string(request)); err != nil {
		return fmt.Errorf("send %v command to fake Copilot pane %q: %w", command["command"], target, err)
	}
	if _, err := tmuxOutput(ctx, h, "send-keys", "-t", target, "Enter"); err != nil {
		return fmt.Errorf("submit %v command to fake Copilot pane %q: %w", command["command"], target, err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		after, err := copilotEventCounts(ctx, h, sessionID, wait)
		if err != nil {
			return err
		}
		var missing []string
		for _, kind := range wait {
			if after[kind] <= before[kind] {
				missing = append(missing, kind)
			}
		}
		if len(missing) == 0 {
			return nil
		}
		if time.Now().After(deadline) {
			output, _ := tmuxOutput(ctx, h, "capture-pane", "-p", "-S", "-", "-t", target)
			return fmt.Errorf("session %q never recorded %s after %v; pane:\n%s", name, strings.Join(missing, ", "), command["command"], output)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// firstPromptKinds is what a held prompt records: sessionStart only on the
// process's first prompt, so a repeat prompt waits for userPromptSubmitted alone.
func promptKinds(ctx context.Context, name string, kinds ...string) ([]string, error) {
	h, err := assertionHarness(ctx)
	if err != nil {
		return nil, err
	}
	sessionID, err := sessionIDByName(h, name)
	if err != nil {
		return nil, err
	}
	counts, err := copilotEventCounts(ctx, h, sessionID, []string{"session_start"})
	if err != nil {
		return nil, err
	}
	if counts["session_start"] == 0 {
		kinds = append(kinds, "session_start")
	}
	return kinds, nil
}

func fakeCopilotPromptedAndFinishes(ctx context.Context, name, text string) error {
	kinds, err := promptKinds(ctx, name, "user_prompt_submitted", "stop")
	if err != nil {
		return err
	}
	return sendCopilotCommand(ctx, name, map[string]any{"command": "prompt", "text": text}, kinds...)
}

func fakeCopilotStartsWorking(ctx context.Context, name, text string) error {
	kinds, err := promptKinds(ctx, name, "user_prompt_submitted")
	if err != nil {
		return err
	}
	return sendCopilotCommand(ctx, name, map[string]any{"command": "prompt", "text": text, "hold": true}, kinds...)
}

func fakeCopilotFinishesTurn(ctx context.Context, name string) error {
	return sendCopilotCommand(ctx, name, map[string]any{"command": "stop"}, "stop")
}

func fakeCopilotRaisesNotification(ctx context.Context, name, notificationType string) error {
	return sendCopilotCommand(ctx, name, map[string]any{
		"command": "notification", "notification_type": notificationType,
		"title": "Attention", "message": "fake " + notificationType,
	}, "notification")
}

func fakeCopilotExits(ctx context.Context, name string) error {
	return sendCopilotCommand(ctx, name, map[string]any{"command": "exit"}, "session_end")
}

// sessionHasHookStatusWithReason polls for a hook-sourced status and reason.
func sessionHasHookStatusWithReason(ctx context.Context, name, status, reason string) error {
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
	for {
		var gotStatus, gotReason, gotSource string
		if err := db.QueryRowContext(ctx, `SELECT status, COALESCE(status_reason, ''), status_source FROM sessions WHERE name = ?`, name).Scan(&gotStatus, &gotReason, &gotSource); err != nil {
			return fmt.Errorf("observe session %q status: %w", name, err)
		}
		if gotStatus == status && gotReason == reason && gotSource == "hook" {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("session %q is %q/%q from %q, want %q/%q from \"hook\"", name, gotStatus, gotReason, gotSource, status, reason)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func sessionHasAnEventOfKind(ctx context.Context, name, kind string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	sessionID, err := sessionIDByName(h, name)
	if err != nil {
		return err
	}
	counts, err := copilotEventCounts(ctx, h, sessionID, []string{kind})
	if err != nil {
		return err
	}
	if counts[kind] == 0 {
		return fmt.Errorf("session %q has no %q event", name, kind)
	}
	return nil
}
