package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/store"
)

// stubExecutableOnPath creates a trivial, real, executable file named name
// in a private directory and prepends that directory to $PATH for the
// duration of the test, so an adapter binary that isn't really installed
// (claude/pi are never installed in CI) can still be "found" by a genuine
// PATH search for tests that are not about the not-on-PATH failure itself.
func stubExecutableOnPath(t *testing.T, name string) {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\nsleep 5\n"
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write stub %s: %v", name, err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

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
	stubExecutableOnPath(t, "claude")
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

func TestResumeFailsOnUnknownConversationID(t *testing.T) {
	cwd := t.TempDir()
	service, db, logger, _ := newAgentTestService(t, nil, "resume-unknown-id")

	// A row with no conversation id assigned at all: the adapter requires
	// one to resume (AssignsConversationID), so this stands in for an
	// unknown/rejected conversation id — there is nothing valid for the
	// adapter's --resume flag to name.
	session, err := db.CreateSession(context.Background(), store.CreateSessionInput{
		ID: "unknown-id-session", Name: "Claude: no id", CWD: cwd, Agent: "claude",
		CapturedPath: os.Getenv("PATH"), Status: "stopped", StatusSource: "user",
	})
	if err != nil {
		t.Fatalf("create durable session directly: %v", err)
	}

	_, outcome, err := service.Resume(context.Background(), session.ID)
	if err == nil {
		t.Fatalf("resume: want error for a session with no conversation id, got none")
	}
	if outcome != ResumeStarted {
		t.Fatalf("outcome = %v, want ResumeStarted (used for launchFailed paths)", outcome)
	}
	if !strings.Contains(err.Error(), "conversation id") {
		t.Fatalf("resume error = %q, want it to name the conversation id cause", err.Error())
	}

	row, getErr := db.GetSession(context.Background(), session.ID)
	if getErr != nil {
		t.Fatalf("get session: %v", getErr)
	}
	if row.Status != "error" {
		t.Fatalf("row status = %q, want error", row.Status)
	}
	if !strings.Contains(row.StatusReason, "conversation id") {
		t.Fatalf("row status reason = %q, want it to name the conversation id cause", row.StatusReason)
	}

	assertNoFreshLaunchRecorded(t, logger.Path())
}

func TestResumeFailsOnMissingCWD(t *testing.T) {
	cwd := t.TempDir()
	stubExecutableOnPath(t, "claude")
	service, db, logger, _ := newAgentTestService(t, nil, "resume-missing-cwd")

	created, err := service.CreateAgent(context.Background(), AgentCreateInput{
		Name: "Claude: gone cwd", CWD: cwd, Agent: "claude", PermissionProfile: "safe",
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if err := service.TMux.Kill(context.Background(), created.Slug); err != nil {
		t.Fatalf("kill original pane: %v", err)
	}
	stopSession(t, db, created.ID)
	if err := os.RemoveAll(cwd); err != nil {
		t.Fatalf("remove cwd: %v", err)
	}

	_, outcome, err := service.Resume(context.Background(), created.ID)
	if err == nil {
		t.Fatalf("resume: want error for a missing cwd, got none")
	}
	if outcome != ResumeStarted {
		t.Fatalf("outcome = %v, want ResumeStarted", outcome)
	}
	if !strings.Contains(err.Error(), "cwd") {
		t.Fatalf("resume error = %q, want it to name the cwd cause", err.Error())
	}

	row, getErr := db.GetSession(context.Background(), created.ID)
	if getErr != nil {
		t.Fatalf("get session: %v", getErr)
	}
	if row.Status != "error" {
		t.Fatalf("row status = %q, want error", row.Status)
	}
	if !strings.Contains(row.StatusReason, "cwd") {
		t.Fatalf("row status reason = %q, want it to name the cwd cause", row.StatusReason)
	}

	assertOneLaunchRecorded(t, logger.Path())
}

func TestResumeFailsOnAgentBinaryNotOnPath(t *testing.T) {
	cwd := t.TempDir()
	// Deliberately no stubExecutableOnPath: the CI toolchain never
	// installs claude/pi, so PATH genuinely lacks the "claude" binary.
	service, db, logger, _ := newAgentTestService(t, nil, "resume-missing-binary")

	created, err := service.CreateAgent(context.Background(), AgentCreateInput{
		Name: "Claude: no binary", CWD: cwd, Agent: "claude", PermissionProfile: "safe",
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if err := service.TMux.Kill(context.Background(), created.Slug); err != nil {
		t.Fatalf("kill original pane: %v", err)
	}
	stopSession(t, db, created.ID)

	_, outcome, err := service.Resume(context.Background(), created.ID)
	if err == nil {
		t.Fatalf("resume: want error for an agent binary not on PATH, got none")
	}
	if outcome != ResumeStarted {
		t.Fatalf("outcome = %v, want ResumeStarted", outcome)
	}
	if !strings.Contains(err.Error(), "PATH") {
		t.Fatalf("resume error = %q, want it to name the PATH cause", err.Error())
	}

	row, getErr := db.GetSession(context.Background(), created.ID)
	if getErr != nil {
		t.Fatalf("get session: %v", getErr)
	}
	if row.Status != "error" {
		t.Fatalf("row status = %q, want error", row.Status)
	}
	if !strings.Contains(row.StatusReason, "PATH") {
		t.Fatalf("row status reason = %q, want it to name the PATH cause", row.StatusReason)
	}

	live, err := service.TMux.List(context.Background())
	if err != nil {
		t.Fatalf("list tmux: %v", err)
	}
	if len(live) != 0 {
		t.Fatalf("live tmux sessions = %#v, want none created by the failed resume", live)
	}

	assertOneLaunchRecorded(t, logger.Path())
}

// assertNoFreshLaunchRecorded fails the test if any "launch" audit record
// exists at all, for a session that never had a successful create-time
// launch (it was inserted directly into the store).
func assertNoFreshLaunchRecorded(t *testing.T, path string) {
	t.Helper()
	launches := 0
	for _, record := range auditRecords(t, path) {
		if record["event"] == "launch" {
			launches++
		}
	}
	if launches != 0 {
		t.Fatalf("launch records = %d, want 0 (resume must never record a fresh-conversation launch)", launches)
	}
}

// assertOneLaunchRecorded fails the test unless exactly one "launch" audit
// record exists (the original successful create), proving the failed
// resume attempt never itself recorded a (fresh-conversation) launch.
func assertOneLaunchRecorded(t *testing.T, path string) {
	t.Helper()
	launches := 0
	for _, record := range auditRecords(t, path) {
		if record["event"] == "launch" {
			launches++
		}
	}
	if launches != 1 {
		t.Fatalf("launch records = %d, want 1 (create only; resume must never record a fresh-conversation launch)", launches)
	}
}
