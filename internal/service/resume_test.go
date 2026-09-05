package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/store"
)

// writeStubExecutable creates a trivial, real, executable file named name
// in dir (which the caller controls; it does not touch $PATH), so a
// genuine PATH/lookPathIn search can find something without a real agent
// actually being installed (claude/pi are never installed in CI). It
// returns the file's full path so a caller can later remove it to simulate
// the binary vanishing after it was found once.
func writeStubExecutable(t *testing.T, dir, name string) string {
	t.Helper()
	script := "#!/bin/sh\nsleep 5\n"
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write stub %s: %v", name, err)
	}
	return path
}

// stubExecutableOnPath creates a trivial, real, executable file named name
// in a private directory and prepends that directory to $PATH for the
// duration of the test, so an adapter binary that isn't really installed
// (claude/pi are never installed in CI) can still be "found" by a genuine
// PATH search for tests that are not about the not-on-PATH failure itself.
func stubExecutableOnPath(t *testing.T, name string) {
	t.Helper()
	dir := t.TempDir()
	writeStubExecutable(t, dir, name)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// stopSession forces a durable row into "stopped", the only status
// AcquireLaunchLease will treat as leasable, mirroring what reconcile does
// after a tmux server (or its pane) actually goes away.
func stopSession(t *testing.T, db *store.Store, sessionID string) {
	t.Helper()
	if err := db.UpdateSessionStatus(context.Background(), store.StatusUpdateInput{
		SessionID: sessionID, Status: "stopped", Source: "tmux", At: 1,
	}); err != nil {
		t.Fatalf("stop session: %v", err)
	}
}

func TestResumeLaunchesAdapterResumeArgvUnderLease(t *testing.T) {
	cwd := t.TempDir()
	stubExecutableOnPath(t, "claude")
	service, db, logger, socket := newAgentTestService(t, nil, "resume-test")

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
	if err := db.UpdateSessionStatus(context.Background(), store.StatusUpdateInput{
		SessionID: created.ID, Status: "stopped", Source: "user", At: 1,
		EventKind: "killed", KilledByUser: true,
	}); err != nil {
		t.Fatalf("mark original session killed by user: %v", err)
	}

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
	if session.KilledByUser {
		t.Fatal("returned resumed session remains killed_by_user")
	}

	row, err := db.GetSession(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if row.Status != "starting" {
		t.Fatalf("persisted row status = %q, want starting", row.Status)
	}
	if row.KilledByUser {
		t.Fatal("persisted resumed session remains killed_by_user")
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
	wantPrefix := []string{"claude", "--resume", created.ConversationID, "--permission-mode", "acceptEdits"}
	if len(argv) != len(wantPrefix)+2 || strings.Join(argv[:len(wantPrefix)], "\x00") != strings.Join(wantPrefix, "\x00") || argv[len(wantPrefix)] != "--settings" {
		t.Fatalf("resume argv = %#v, want base argv followed by deck --settings", argv)
	}
	if !strings.Contains(argv[len(argv)-1], "'"+service.DeckExecutable+"' _hook") {
		t.Fatalf("resume settings = %q, want absolute deck executable %q", argv[len(argv)-1], service.DeckExecutable)
	}
	assertTMuxEnvironment(t, socket, session.Slug, "DECK_SESSION_ID", session.ID)
	assertTMuxEnvironment(t, socket, session.Slug, "DECK_HOME", service.DeckHome)
	for _, token := range argv {
		if strings.Contains(token, "--session-id") || strings.Contains(token, "--continue") {
			t.Fatalf("resume argv = %#v must not reuse launch/--continue forms", argv)
		}
	}
}

func TestResumeLosingLeaseCreatesNoTMuxSession(t *testing.T) {
	cwd := t.TempDir()
	stubExecutableOnPath(t, "claude")
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
	winner, err := db.AcquireLaunchLease(context.Background(), created.ID, store.CurrentLaunchLeaseOwner(), store.DefaultLaunchLeaseTTL, service.Clock.Now().UnixMilli())
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

func TestResumeNonLeasableReturnsActualStatusAndReason(t *testing.T) {
	cwd := t.TempDir()
	stubExecutableOnPath(t, "claude")
	service, db, _, _ := newAgentTestService(t, nil, "resume-not-leasable")

	created, err := service.CreateAgent(context.Background(), AgentCreateInput{
		Name: "Claude: already waiting", CWD: cwd, Agent: "claude", PermissionProfile: "safe",
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if err := service.TMux.Kill(context.Background(), created.Slug); err != nil {
		t.Fatalf("kill original pane: %v", err)
	}
	if err := db.UpdateSessionStatus(context.Background(), store.StatusUpdateInput{
		SessionID: created.ID, Status: "waiting", Reason: "permission_prompt", Source: "hook",
		At: service.Clock.Now().UnixMilli(), EventKind: "notification",
	}); err != nil {
		t.Fatalf("make row non-leasable: %v", err)
	}

	session, outcome, err := service.Resume(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if outcome != ResumeNotLeasable {
		t.Fatalf("outcome = %v, want ResumeNotLeasable", outcome)
	}
	if session.Status != "waiting" || session.StatusReason != "permission_prompt" {
		t.Fatalf("returned verdict = %q/%q, want waiting/permission_prompt", session.Status, session.StatusReason)
	}

	live, err := service.TMux.List(context.Background())
	if err != nil {
		t.Fatalf("list tmux: %v", err)
	}
	if len(live) != 0 {
		t.Fatalf("live tmux sessions = %#v, want no launch for non-leasable row", live)
	}
}

// TestResumeAdoptsAlreadyRunningTMuxSessionInsteadOfDuplicateError covers
// requirement 46: a durable row can end up marked "stopped" (e.g. an
// external client's stale reconciliation write, or a race) while its tmux
// pane is still genuinely alive. Resume must recognise that BEFORE taking
// the launch lease and report an honest no-op, never attempt `new-session`
// over the live pane (which tmux would refuse as "duplicate session:
// deck_<name>"), and never write an error status for a session that is
// demonstrably running.
func TestResumeAdoptsAlreadyRunningTMuxSessionInsteadOfDuplicateError(t *testing.T) {
	cwd := t.TempDir()
	stubExecutableOnPath(t, "claude")
	service, db, logger, _ := newAgentTestService(t, nil, "resume-already-running")

	created, err := service.CreateAgent(context.Background(), AgentCreateInput{
		Name: "Claude: adopt", CWD: cwd, Agent: "claude", PermissionProfile: "safe",
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	// Deliberately do NOT kill the tmux pane: only the durable row is forced
	// to "stopped", mirroring a row that a stale/racing write marked stopped
	// while the pane itself never went away.
	stopSession(t, db, created.ID)

	session, outcome, err := service.Resume(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if outcome != ResumeAlreadyRunning {
		t.Fatalf("outcome = %v, want ResumeAlreadyRunning", outcome)
	}
	if session.Status != "stopped" {
		t.Fatalf("returned session status = %q, want the untouched stopped row", session.Status)
	}

	row, getErr := db.GetSession(context.Background(), created.ID)
	if getErr != nil {
		t.Fatalf("get session: %v", getErr)
	}
	if row.Status != "stopped" {
		t.Fatalf("persisted row status = %q, want stopped (unchanged, never error)", row.Status)
	}

	live, err := service.TMux.List(context.Background())
	if err != nil {
		t.Fatalf("list tmux: %v", err)
	}
	if len(live) != 1 {
		t.Fatalf("live tmux sessions = %#v, want exactly the one already-running pane (no duplicate attempt)", live)
	}

	// Only the original create-time launch was ever recorded: adopting an
	// already-running session must not itself attempt (and fail) a launch.
	assertOneLaunchRecorded(t, logger.Path())
}

// TestResumeSurfacesAGenuineNonDuplicateTMuxFailure proves requirement 46's
// already-running check does not swallow a real tmux failure: with no
// existing tmux session for the row (Exists reports false), a genuinely
// invalid launch environment must still fail Resume and land the row at
// `error`, exactly as before this task.
func TestResumeSurfacesAGenuineNonDuplicateTMuxFailure(t *testing.T) {
	cwd := t.TempDir()
	stubExecutableOnPath(t, "claude")
	service, db, logger, _ := newAgentTestService(t, nil, "resume-genuine-tmux-failure")

	created, err := service.CreateAgent(context.Background(), AgentCreateInput{
		Name: "Claude: bad env", CWD: cwd, Agent: "claude", PermissionProfile: "safe",
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if err := service.TMux.Kill(context.Background(), created.Slug); err != nil {
		t.Fatalf("kill original pane: %v", err)
	}
	stopSession(t, db, created.ID)

	// A session-layer env key containing "=" is rejected by tmux.Client.Create
	// itself (internal/tmux's environmentArgs), a genuine, non-duplicate tmux
	// failure unrelated to requirement 46's already-running check.
	if _, execErr := db.DB().ExecContext(context.Background(), `UPDATE sessions SET env = ? WHERE id = ?`, `{"BAD=KEY":"x"}`, created.ID); execErr != nil {
		t.Fatalf("force invalid env: %v", execErr)
	}

	_, outcome, err := service.Resume(context.Background(), created.ID)
	if err == nil {
		t.Fatalf("resume: want error for a genuinely invalid launch environment, got none")
	}
	if outcome != ResumeStarted {
		t.Fatalf("outcome = %v, want ResumeStarted (used for launchFailed paths)", outcome)
	}
	if strings.Contains(err.Error(), "duplicate session") {
		t.Fatalf("resume error = %q, must not be tmux's duplicate-session refusal", err.Error())
	}
	if !strings.Contains(err.Error(), "invalid environment variable") {
		t.Fatalf("resume error = %q, want it to name the genuine invalid-environment cause", err.Error())
	}

	row, getErr := db.GetSession(context.Background(), created.ID)
	if getErr != nil {
		t.Fatalf("get session: %v", getErr)
	}
	if row.Status != "error" {
		t.Fatalf("row status = %q, want error", row.Status)
	}

	live, err := service.TMux.List(context.Background())
	if err != nil {
		t.Fatalf("list tmux: %v", err)
	}
	if len(live) != 0 {
		t.Fatalf("live tmux sessions = %#v, want none created by the genuinely failed resume", live)
	}

	assertOneLaunchRecorded(t, logger.Path())
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
		CapturedPath: os.Getenv("PATH"), Status: "stopped", StatusSource: "user", StatusAt: 1, CreatedAt: 1,
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
	// The binary exists at create time (so CreateAgent's own preflight,
	// which probes the same PATH, passes) but is removed from disk before
	// resume: CapturedPath is the create-time string captured onto the row,
	// so a live $PATH never changes it -- only the file backing one of its
	// directories can, which is exactly what "an agent binary uninstalled
	// after create" looks like by resume time.
	dir := t.TempDir()
	stubPath := writeStubExecutable(t, dir, "claude")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
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

	// The binary disappears -- e.g. an uninstall -- while the row's stored
	// CapturedPath string (captured at create time) still names dir.
	if err := os.Remove(stubPath); err != nil {
		t.Fatalf("remove stub claude: %v", err)
	}

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

// TestResumeUsesTheServicesGlobalPreLaunch proves task 005's second call
// site: Resume (internal/service/resume.go), like CreateAgent, wraps its
// pane command with the service's configured global hook ahead of the
// session's own pre_launch, in the same global-first order.
func TestResumeUsesTheServicesGlobalPreLaunch(t *testing.T) {
	cwd := t.TempDir()
	service, db, _, _ := newAgentTestService(t, nil, "resume-global-pre-launch")
	globalMarker := filepath.Join(cwd, "global_marker")
	sessionMarker := filepath.Join(cwd, "session_marker")
	agentMarker := filepath.Join(cwd, "agent_marker")

	created, err := service.CreateAgent(context.Background(), AgentCreateInput{
		Name: "Shell: resume global pre-launch", CWD: cwd, Agent: "shell",
		PreLaunch:  "test -f " + globalMarker + " && touch " + sessionMarker,
		LaunchArgs: []string{"-c", "touch " + agentMarker + " && sleep 5"},
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if err := service.TMux.Kill(context.Background(), created.Slug); err != nil {
		t.Fatalf("kill original pane: %v", err)
	}
	stopSession(t, db, created.ID)

	// Set the service's global hook only now, after create, so the create
	// above proves nothing about it: only the resume call below is under
	// test.
	service.GlobalPreLaunch = "touch " + globalMarker

	_, outcome, err := service.Resume(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if outcome != ResumeStarted {
		t.Fatalf("outcome = %v, want ResumeStarted", outcome)
	}
	if !waitForFile(t, globalMarker, 5*time.Second) {
		t.Fatalf("global pre_launch never ran on resume: %s missing", globalMarker)
	}
	if !waitForFile(t, sessionMarker, 5*time.Second) {
		t.Fatalf("session pre_launch never ran after the global hook on resume: %s missing", sessionMarker)
	}
	if !waitForFile(t, agentMarker, 5*time.Second) {
		t.Fatalf("agent never started on resume after both hooks succeeded: %s missing", agentMarker)
	}
}
