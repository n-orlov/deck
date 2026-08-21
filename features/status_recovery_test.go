package features

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/cucumber/godog"
)

// registerStatusRecoverySteps backs features/status_recovery.feature
// (requirements 43-47): the status-recovery chain re-derived in tasks
// 001-004. Every step stays black-box, exactly like registerClaudeHookStatusSteps:
// commands enter a real fake-agent pane via tmux send-keys, and assertions
// observe only tmux, SQLite and released TUI frames.
func registerStatusRecoverySteps(sc *godog.ScenarioContext) {
	sc.Step(`^fake Claude session "([^"]+)" resumes in-session into a new conversation$`, fakeClaudeResumesInSession)
	sc.Step(`^the state database session "([^"]+)" now has a different, non-empty conversation id than before the resume$`, sessionConversationIDChangedSinceResume)
	sc.Step(`^the state database session "([^"]+)"'s status is forced to "([^"]+)" from tmux as a stale launch-failure verdict$`, forceSessionStatusToStaleTMuxVerdict)
}

// fakeClaudeResumesInSession captures the session's current conversation_id
// (so a later step can prove it actually changed), then sends fake-claude's
// "resume" pane command for that id, letting fake-claude mint the new id
// itself. This fires SessionEnd reason=resume immediately followed by
// SessionStart reason=resume in the same pane -- the in-session resume pair
// task 001/002 must leave the row running with the new conversation_id.
func fakeClaudeResumesInSession(ctx context.Context, name string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	oldID, err := sessionConversationID(h, name)
	if err != nil {
		return err
	}
	if oldID == "" {
		return fmt.Errorf("session %q has no conversation id to resume from", name)
	}
	if h.preResumeConversationIDs == nil {
		h.preResumeConversationIDs = make(map[string]string)
	}
	h.preResumeConversationIDs[name] = oldID

	slug, err := sessionSlugByName(h, name)
	if err != nil {
		return err
	}
	sessionID, err := sessionIDByName(h, name)
	if err != nil {
		return err
	}
	db, err := openObservedDatabase(h)
	if err != nil {
		return err
	}
	defer db.Close()
	var before int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM events WHERE session_id = ?`, sessionID).Scan(&before); err != nil {
		return err
	}

	request, err := json.Marshal(map[string]any{"command": "resume", "old_session_id": oldID})
	if err != nil {
		return err
	}
	paneTarget := "deck_" + slug
	if _, err := tmuxOutput(ctx, h, "send-keys", "-t", paneTarget, "-l", string(request)); err != nil {
		return fmt.Errorf("send resume command to fake Claude pane %q: %w", paneTarget, err)
	}
	if _, err := tmuxOutput(ctx, h, "send-keys", "-t", paneTarget, "Enter"); err != nil {
		return fmt.Errorf("submit resume command to fake Claude pane %q: %w", paneTarget, err)
	}

	// fireResumePair (cmd/fake-claude) records a SessionEnd then a
	// SessionStart event: wait for both to land before returning.
	deadline := time.Now().Add(3 * time.Second)
	for {
		var count int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM events WHERE session_id = ?`, sessionID).Scan(&count); err != nil {
			return err
		}
		if count >= before+2 {
			return nil
		}
		if time.Now().After(deadline) {
			output, _ := tmuxOutput(ctx, h, "capture-pane", "-p", "-S", "-", "-t", paneTarget)
			return fmt.Errorf("fake Claude pane %q did not persist the resume end/start pair; pane:\n%s", paneTarget, output)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// sessionConversationIDChangedSinceResume asserts the durable conversation_id
// is non-empty and differs from the value fakeClaudeResumesInSession captured
// immediately before firing the resume command, proving requirement 44 (the
// stored conversation_id follows the live conversation on SessionStart)
// rather than merely re-observing whatever value happens to be present now.
func sessionConversationIDChangedSinceResume(ctx context.Context, name string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	oldID, ok := h.preResumeConversationIDs[name]
	if !ok {
		return fmt.Errorf("session %q's conversation id was never captured before a resume", name)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		newID, err := sessionConversationID(h, name)
		if err != nil {
			return err
		}
		if newID != "" && newID != oldID {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("session %q conversation id = %q, want non-empty and different from pre-resume value %q", name, newID, oldID)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// forceSessionStatusToStaleTMuxVerdict writes a tmux-sourced status directly
// into the state database, the same precondition-setup discipline as this
// package's existing conversation_id/captured_path clear steps. It stands in
// for the magpie-row case task 003's finding names: a tmux-sourced launch
// failure (status "error", status_source "tmux", status_reason naming a
// duplicate-session collision) parked on a row whose conversation is, in
// truth, live and about to fire a real hook.
func forceSessionStatusToStaleTMuxVerdict(ctx context.Context, name, status string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	db, err := openObservedDatabase(h)
	if err != nil {
		return err
	}
	defer db.Close()
	result, err := db.ExecContext(ctx, `UPDATE sessions SET status = ?, status_source = 'tmux', status_reason = 'duplicate session: deck_x', status_at = 1000 WHERE name = ?`, status, name)
	if err != nil {
		return fmt.Errorf("force session %q status to %q from tmux: %w", name, status, err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return fmt.Errorf("force session %q status affected %d rows, want 1", name, affected)
	}
	return nil
}
