package service

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// retainedCorpseFixture builds issue #6's wedged state and refuses to return
// unless it is genuinely discriminating: a durable row reading `stopped` from
// a hook (Claude's SessionEnd, reason `other`, in the same millisecond as the
// exit) WHILE tmux still retains the dead pane, because deck's server runs
// `remain-on-exit failed`. It returns the refreshed row and the pane id of the
// corpse, so a caller can prove a later launch created a NEW pane rather than
// adopting this one.
func retainedCorpseFixture(t *testing.T, svc Service, db *store.Store, id, name, cwd string, exitStatus int) (store.Session, string) {
	t.Helper()
	ctx := context.Background()
	session, err := db.CreateSession(ctx, store.CreateSessionInput{
		ID: id, Name: name, CWD: cwd, Agent: "claude", CapturedPath: os.Getenv("PATH"),
		PermissionProfile: "safe", Status: "running", StatusSource: "hook", StatusAt: 1, CreatedAt: 1,
	})
	if err != nil {
		t.Fatalf("create fixture row: %v", err)
	}
	if err := db.SetConversationID(ctx, session.ID, "11111111-2222-4333-8444-555555555555", "test", 1); err != nil {
		t.Fatalf("assign fixture conversation id: %v", err)
	}
	command := `printf 'claude: resume failed\n'; exit ` + strconv.Itoa(exitStatus)
	if _, err := svc.TMux.Create(ctx, tmux.Launch{Slug: session.Slug, CWD: cwd, Command: []string{"/bin/sh", "-c", command}}); err != nil {
		t.Fatalf("create fixture pane: %v", err)
	}
	waitForDeadPane(t, svc.TMux, session.Slug, exitStatus)

	if err := db.UpdateSessionStatus(ctx, store.StatusUpdateInput{
		SessionID: session.ID, Status: "stopped", Reason: "SessionEnd (other)",
		Source: "hook", At: svc.Clock.Now().UnixMilli(), EventKind: "hook.SessionEnd",
	}); err != nil {
		t.Fatalf("hook write of stopped: %v", err)
	}
	row, err := db.GetSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("refresh fixture row: %v", err)
	}
	if row.Status != "stopped" || row.StatusSource != "hook" || row.PaneExitStatus != nil {
		t.Fatalf("fixture is not discriminating: status=%q source=%q pane_exit_status=%v",
			row.Status, row.StatusSource, row.PaneExitStatus)
	}
	// The two liveness readings that make this fixture the trap issue #6
	// described: `has-session` still succeeds, so the OLD resume decision
	// (TMux.Exists) would have adopted a corpse, while HasLivePane -- the new
	// decision -- correctly sees nothing to adopt.
	exists, err := svc.TMux.Exists(ctx, row.Slug)
	if err != nil {
		t.Fatalf("fixture Exists: %v", err)
	}
	if !exists {
		t.Fatal("fixture is not discriminating: tmux no longer has a session for the row, so nothing here distinguishes a corpse from an absent session")
	}
	live, err := svc.TMux.HasLivePane(ctx, row.Slug)
	if err != nil {
		t.Fatalf("fixture HasLivePane: %v", err)
	}
	if live {
		t.Fatal("fixture is not discriminating: the pane is still live, so this is requirement 46's genuine already-running case, not a corpse")
	}
	sessions, err := svc.TMux.List(ctx)
	if err != nil {
		t.Fatalf("fixture List: %v", err)
	}
	var corpsePane string
	for _, listed := range sessions {
		if listed.Name == "deck_"+row.Slug && len(listed.Panes) > 0 {
			corpsePane = listed.Panes[0].ID
		}
	}
	if corpsePane == "" {
		t.Fatal("fixture corpse has no pane id to compare a later launch against")
	}
	return row, corpsePane
}

// TestResumeRelaunchesARowWhoseOnlyPaneIsARetainedCorpse is issue #6's leg 2:
// resume's liveness decision must mean "has a live pane", not "has-session
// succeeds". Against the unfixed decision (TMux.Exists) this row returns
// ResumeAlreadyRunning -- the silent no-op the operator hit, with no way out
// of the UI -- because `remain-on-exit failed` keeps the dead pane and its
// session on the socket. The assertions go further than the outcome: a brand
// new pane must exist under the same session name, so the corpse cannot have
// been adopted and cannot still be holding the name.
func TestResumeRelaunchesARowWhoseOnlyPaneIsARetainedCorpse(t *testing.T) {
	cwd := t.TempDir()
	stubExecutableOnPath(t, "claude")
	svc, db, _, _ := newAgentTestService(t, nil, "resume-retained-corpse")
	ctx := context.Background()

	row, corpsePane := retainedCorpseFixture(t, svc, db, "00000000-0000-4000-8000-000000000692", "backlog-ideas", cwd, 7)

	session, outcome, err := svc.Resume(ctx, row.ID)
	if err != nil {
		t.Fatalf("resume a row whose only pane is dead: %v", err)
	}
	if outcome == ResumeAlreadyRunning {
		t.Fatal("resume reported ResumeAlreadyRunning for a session whose only pane is a corpse (#6): the liveness decision is still `has-session`, not `has a live pane`")
	}
	if outcome != ResumeStarted {
		t.Fatalf("outcome = %v, want ResumeStarted", outcome)
	}
	if session.Status != "starting" {
		t.Fatalf("returned session status = %q, want starting", session.Status)
	}

	live, err := svc.TMux.HasLivePane(ctx, row.Slug)
	if err != nil {
		t.Fatalf("HasLivePane after resume: %v", err)
	}
	if !live {
		t.Fatal("resume returned ResumeStarted but the session has no live pane")
	}
	listed, err := svc.TMux.List(ctx)
	if err != nil {
		t.Fatalf("list tmux: %v", err)
	}
	if len(listed) != 1 || listed[0].Name != "deck_"+row.Slug || len(listed[0].Panes) != 1 {
		t.Fatalf("tmux sessions after resume = %#v, want exactly the one relaunched session with one pane", listed)
	}
	pane := listed[0].Panes[0]
	if pane.Dead {
		t.Fatalf("the pane after resume is still the dead one: %#v", pane)
	}
	if pane.ID == corpsePane {
		t.Fatalf("resume adopted the corpse's own pane %q instead of creating a new one", pane.ID)
	}

	after, err := db.GetSession(ctx, row.ID)
	if err != nil {
		t.Fatalf("get session after resume: %v", err)
	}
	if after.Status != "starting" {
		t.Fatalf("row status after resume = %q (source %q), want starting", after.Status, after.StatusSource)
	}
}

// TestKillRemovesARetainedCorpseAndStillRefusesAGenuinelyGoneSession is
// issue #6's leg 3. Kill's guard is now "already stopped AND no tmux session
// exists": the first kill below is refused by the unfixed guard
// ("session is already stopped") even though there is a live tmux session
// name to free, and the second -- the same row once its corpse really is
// gone -- must still be refused, so the guard is narrowed rather than
// removed. Kill deliberately consults Exists, not HasLivePane: a corpse is
// exactly what this call is for removing.
func TestKillRemovesARetainedCorpseAndStillRefusesAGenuinelyGoneSession(t *testing.T) {
	cwd := t.TempDir()
	svc, db, _, _ := newAgentTestService(t, nil, "kill-retained-corpse")
	ctx := context.Background()

	row, _ := retainedCorpseFixture(t, svc, db, "00000000-0000-4000-8000-000000000693", "wedged-row", cwd, 7)

	if err := svc.Kill(ctx, row); err != nil {
		t.Fatalf("kill a stopped row that still has a retained corpse: %v", err)
	}
	listed, err := svc.TMux.List(ctx)
	if err != nil {
		t.Fatalf("list tmux: %v", err)
	}
	if len(listed) != 0 {
		t.Fatalf("tmux sessions after kill = %#v, want the corpse collected", listed)
	}
	after, err := db.GetSession(ctx, row.ID)
	if err != nil {
		t.Fatalf("get session after kill: %v", err)
	}
	if after.Status != "stopped" || after.StatusSource != "user" || !after.KilledByUser {
		t.Fatalf("row after kill = status %q source %q killed_by_user %v, want stopped/user/true",
			after.Status, after.StatusSource, after.KilledByUser)
	}
	if !auditContains(t, svc.Audit.Path(), row.ID, "killed") {
		t.Fatalf("audit lacks the killed transition for %q", row.ID)
	}

	// The narrowed guard still refuses the case it was written for: stopped
	// with nothing left on the socket.
	err = svc.Kill(ctx, after)
	if err == nil {
		t.Fatal("kill succeeded on a stopped row with no tmux session at all: the guard was removed instead of narrowed")
	}
	if !strings.Contains(err.Error(), "already stopped") {
		t.Fatalf("refusal = %q, want it to still name the already-stopped cause", err.Error())
	}
}
