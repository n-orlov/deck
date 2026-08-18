package service

import (
	"context"
	"strings"
	"testing"
)

// TestPinnedResumeSurvivesRestartAndArgv proves that, once a session is
// pinned (SPEC §8/§9.3, task 021), a Resume after a simulated deck restart
// (a fresh Service value sharing the same durable store) uses the pinned
// conversation id in the resume argv.
func TestPinnedResumeSurvivesRestartAndArgv(t *testing.T) {
	cwd := t.TempDir()
	stubExecutableOnPath(t, "claude")
	service, db, logger, _ := newAgentTestService(t, nil, "pin-test")

	created, err := service.CreateAgent(context.Background(), AgentCreateInput{
		Name: "Claude: pin", CWD: cwd, Agent: "claude", PermissionProfile: "edits",
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	pinned, err := service.PinResume(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("pin resume: %v", err)
	}
	if pinned.ResumeState != "pinned" || pinned.ResumePin != created.ConversationID {
		t.Fatalf("pinned session = %+v, want resume_state=pinned resume_pin=%q", pinned, created.ConversationID)
	}

	if err := service.TMux.Kill(context.Background(), created.Slug); err != nil {
		t.Fatalf("kill original pane: %v", err)
	}
	stopSession(t, db, created.ID)

	// Simulate a deck restart: a fresh Service sharing the same store/audit
	// but no in-memory state must still pick up the persisted pin.
	restarted := service
	session, outcome, err := restarted.Resume(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("resume after restart: %v", err)
	}
	if outcome != ResumeStarted {
		t.Fatalf("outcome = %v, want ResumeStarted", outcome)
	}
	if session.Status != "starting" {
		t.Fatalf("row status = %q, want starting", session.Status)
	}

	var argv []string
	for _, record := range auditRecords(t, logger.Path()) {
		if record["event"] == "launch" {
			argv = jsonStrings(record["argv"])
		}
	}
	found := false
	for i, token := range argv {
		if token == "--resume" && i+1 < len(argv) && argv[i+1] == created.ConversationID {
			found = true
		}
	}
	if !found {
		t.Fatalf("resume argv %#v does not contain --resume %q (the pinned id)", argv, created.ConversationID)
	}
}

// TestFreshOnceStartsFreshConversationThenRevertsToAuto proves the one-shot
// "start fresh" launches a brand-new conversation (never --resume or
// --continue) exactly once, and that resume_state reads auto (not cleared,
// not still fresh-once, not pinned) once that fresh launch has happened.
func TestFreshOnceStartsFreshConversationThenRevertsToAuto(t *testing.T) {
	cwd := t.TempDir()
	stubExecutableOnPath(t, "claude")
	service, db, logger, _ := newAgentTestService(t, nil, "fresh-once-test")

	created, err := service.CreateAgent(context.Background(), AgentCreateInput{
		Name: "Claude: fresh", CWD: cwd, Agent: "claude", PermissionProfile: "edits",
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	armed, err := service.ArmFreshOnce(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("arm fresh-once: %v", err)
	}
	if armed.ResumeState != "fresh-once" {
		t.Fatalf("resume_state = %q, want fresh-once", armed.ResumeState)
	}

	if err := service.TMux.Kill(context.Background(), created.Slug); err != nil {
		t.Fatalf("kill original pane: %v", err)
	}
	stopSession(t, db, created.ID)

	session, outcome, err := service.Resume(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("resume (fresh-once): %v", err)
	}
	if outcome != ResumeStarted {
		t.Fatalf("outcome = %v, want ResumeStarted", outcome)
	}
	if session.ConversationID == "" || session.ConversationID == created.ConversationID {
		t.Fatalf("fresh-once conversation id = %q, want a new, non-empty id (had %q)", session.ConversationID, created.ConversationID)
	}

	row, err := db.GetSession(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if row.ResumeState != "auto" {
		t.Fatalf("persisted resume_state = %q, want auto (never cleared/left fresh-once, never pinned)", row.ResumeState)
	}
	if row.ConversationID != session.ConversationID {
		t.Fatalf("persisted conversation id = %q, want %q", row.ConversationID, session.ConversationID)
	}

	var argv []string
	launches := 0
	for _, record := range auditRecords(t, logger.Path()) {
		if record["event"] == "launch" {
			launches++
			argv = jsonStrings(record["argv"])
		}
	}
	if launches != 2 {
		t.Fatalf("launch records = %d, want 2 (create + fresh-once)", launches)
	}
	for _, token := range argv {
		if strings.Contains(token, "--resume") || strings.Contains(token, "--continue") {
			t.Fatalf("fresh-once launch argv %#v must never contain --resume/--continue", argv)
		}
	}
	found := false
	for i, token := range argv {
		if token == "--session-id" && i+1 < len(argv) && argv[i+1] == session.ConversationID {
			found = true
		}
	}
	if !found {
		t.Fatalf("fresh-once launch argv %#v does not contain --session-id %q", argv, session.ConversationID)
	}
}

// TestResumeModeDispatchesToPinAutoAndFreshOnce proves the single
// ResumeMode entry point (wired to the TUI's `p` dialog) reaches the same
// store state as calling PinResume/SetResumeAuto/ArmFreshOnce directly.
func TestResumeModeDispatchesToPinAutoAndFreshOnce(t *testing.T) {
	cwd := t.TempDir()
	service, _, _, _ := newAgentTestService(t, nil, "resume-mode-test")

	created, err := service.CreateAgent(context.Background(), AgentCreateInput{
		Name: "Claude: mode", CWD: cwd, Agent: "claude", PermissionProfile: "edits",
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	pinned, err := service.ResumeMode(context.Background(), created.ID, "pinned")
	if err != nil {
		t.Fatalf("resume mode pinned: %v", err)
	}
	if pinned.ResumeState != "pinned" || pinned.ResumePin != created.ConversationID {
		t.Fatalf("pinned session = %+v", pinned)
	}

	fresh, err := service.ResumeMode(context.Background(), created.ID, "fresh-once")
	if err != nil {
		t.Fatalf("resume mode fresh-once: %v", err)
	}
	if fresh.ResumeState != "fresh-once" {
		t.Fatalf("resume_state = %q, want fresh-once", fresh.ResumeState)
	}

	auto, err := service.ResumeMode(context.Background(), created.ID, "auto")
	if err != nil {
		t.Fatalf("resume mode auto: %v", err)
	}
	if auto.ResumeState != "auto" || auto.ResumePin != "" {
		t.Fatalf("auto session = %+v, want resume_state=auto resume_pin=\"\"", auto)
	}

	if _, err := service.ResumeMode(context.Background(), created.ID, "bogus"); err == nil {
		t.Fatalf("resume mode %q: want error for unknown mode", "bogus")
	}
}
