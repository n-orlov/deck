package service

import (
	"context"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/store"
)

// stopSession forces a durable row into "stopped", the only status
// AcquireLaunchLease will treat as leasable, mirroring what reconcile does
// after a tmux server (or its pane) actually goes away.
func stopSession(t *testing.T, db *store.Store, sessionID string) {
	t.Helper()
	if err := db.UpdateSessionStatus(context.Background(), store.StatusUpdateInput{
		SessionID: sessionID, Status: "stopped", Source: "tmux",
	}); err != nil {
		t.Fatalf("stop session: %v", err)
	}
}

func TestResumeLaunchesAdapterResumeArgvUnderLease(t *testing.T) {
	cwd := t.TempDir()
	service, db, logger, _ := newAgentTestService(t, nil, "resume-test")

	created, err := service.CreateAgent(context.Background(), AgentCreateInput{
		Name: "Claude: resume", CWD: cwd, Agent: "claude", PermissionProfile: "edits",
		Env: map[string]string{"FROM_SESSION": "1"},
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if err := service.TMux.Kill(context.Background(), created.Slug); err != nil {
		t.Fatalf("kill original pane: %v", err)
	}
	stopSession(t, db, created.ID)

	session, outcome, err := service.Resume(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if outcome != ResumeStarted {
		t.Fatalf("outcome = %v, want ResumeStarted", outcome)
	}
	if session.Status != "starting" {
		t.Fatalf("row status = %q, want starting", session.Status)
	}

	row, err := db.GetSession(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if row.Status != "starting" {
		t.Fatalf("persisted row status = %q, want starting", row.Status)
	}

	live, err := service.TMux.List(context.Background())
	if err != nil || len(live) != 1 {
		t.Fatalf("live tmux sessions = %#v, %v", live, err)
	}

	records := auditRecords(t, logger.Path())
	var argv []string
	launches := 0
	for _, record := range records {
		if record["event"] == "launch" {
			launches++
			argv = jsonStrings(record["argv"])
		}
	}
	if launches != 2 {
		t.Fatalf("launch records = %d, want 2 (create + resume)", launches)
	}
	wantArgv := []string{"claude", "--resume", created.ConversationID, "--permission-mode", "acceptEdits"}
	if strings.Join(argv, "\x00") != strings.Join(wantArgv, "\x00") {
		t.Fatalf("resume argv = %#v, want %#v", argv, wantArgv)
	}
	for _, token := range argv {
		if strings.Contains(token, "--session-id") || strings.Contains(token, "--continue") {
			t.Fatalf("resume argv = %#v must not reuse launch/--continue forms", argv)
		}
	}
}

func TestResumeLosingLeaseCreatesNoTMuxSession(t *testing.T) {
	cwd := t.TempDir()
	service, db, _, _ := newAgentTestService(t, nil, "resume-race")

	created, err := service.CreateAgent(context.Background(), AgentCreateInput{
		Name: "Claude: race", CWD: cwd, Agent: "claude", PermissionProfile: "safe",
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if err := service.TMux.Kill(context.Background(), created.Slug); err != nil {
		t.Fatalf("kill original pane: %v", err)
	}
	stopSession(t, db, created.ID)

	// Acquire the lease ourselves first, simulating a concurrent winner:
	// AcquireLaunchLease treats our own live pid as unbreakable, so the
	// service's own Resume call below must lose the race.
	winner, err := db.AcquireLaunchLease(context.Background(), created.ID, store.CurrentLaunchLeaseOwner(), store.DefaultLaunchLeaseTTL)
	if err != nil {
		t.Fatalf("pre-acquire lease: %v", err)
	}
	if winner.Outcome != store.LaunchLeaseAcquired {
		t.Fatalf("pre-acquire outcome = %v, want acquired", winner.Outcome)
	}

	session, outcome, err := service.Resume(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if outcome != ResumeStartingElsewhere {
		t.Fatalf("outcome = %v, want ResumeStartingElsewhere", outcome)
	}
	_ = session

	live, err := service.TMux.List(context.Background())
	if err != nil {
		t.Fatalf("list tmux: %v", err)
	}
	if len(live) != 0 {
		t.Fatalf("live tmux sessions = %#v, want none created by the losing resume", live)
	}
}
