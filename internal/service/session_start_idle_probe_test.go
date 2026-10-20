package service

import (
	"context"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/hookrecv"
)

// R307: a session resumed with --resume/--continue and given no prompt stays
// idle across a probe tick whose pane matches no probe rule. The row starts
// from a stale running hook verdict (so the hook is a real transition and
// dates status_at a full window ago), takes SessionStart through the real
// receiver, and is then probed by ReconcileWithProbes over a live pane.
func TestResumedSessionWithNoPromptStaysIdleAcrossAProbeTickThatMatchesNoRule(t *testing.T) {
	svc, db := newCopilotProbeService(t, "session-start-idle")
	const pane = "Resumed conversation. Nothing on this screen names a status.\n"
	row := copilotProbeRowWithPane(t, svc, db, "claude", "safe", "resumed", pane, 7)

	adapter, ok := svc.Agents.Lookup("claude")
	if !ok {
		t.Fatal("claude adapter is not registered")
	}
	if status, reason := adapter.Probe(pane); status != "" || reason != "" {
		t.Fatalf("fixture is not discriminating: the pane matches a probe rule (%q, %q)", status, reason)
	}

	stale := svc.Clock.Now().UnixMilli() - config.DefaultStaleAfter.Milliseconds()
	raw := []byte(`{"hook_event_name":"SessionStart","source":"resume"}`)
	if _, err := hookrecv.Receive(context.Background(), db, raw, row.ID, "", stale); err != nil {
		t.Fatal(err)
	}
	before := sessionStatus(t, db, row.ID)
	if before.Status != "idle" || before.StatusSource != "hook" || before.StatusReason != "resume" {
		t.Fatalf("after SessionStart(resume) the row is %q/%q/%q, want idle/hook/resume", before.Status, before.StatusSource, before.StatusReason)
	}

	got := probeCopilotRow(t, svc, db, row)
	if got.Status != "idle" || got.StatusSource != "hook" || got.StatusAt != before.StatusAt {
		t.Fatalf("a probe tick that matched no rule moved the resumed row: %q/%q at %d, was idle/hook at %d",
			got.Status, got.StatusSource, got.StatusAt, before.StatusAt)
	}
}
