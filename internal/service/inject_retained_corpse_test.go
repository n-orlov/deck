package service

import (
	"context"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// retainedDeadShellPaneFixture builds a shell session whose only tmux pane
// has already exited but is retained by deck's `remain-on-exit failed`
// server -- issue #6's trap, R69's target and R88's own scope. `has-session`
// still succeeds for it forever, while HasLivePane -- R69's "has a live
// pane" liveness notion -- correctly reports false. One env key is left
// dirty so InjectEnv has something to attempt, and the fixture refuses to
// return unless both liveness readings actually disagree, exactly as
// retainedCorpseFixture does for resume/kill in
// retained_corpse_recovery_test.go.
func retainedDeadShellPaneFixture(t *testing.T, svc Service, db *store.Store, id, name, cwd string) (store.Session, string, []byte) {
	t.Helper()
	ctx := context.Background()
	session, err := db.CreateSession(ctx, store.CreateSessionInput{
		ID: id, Name: name, CWD: cwd, Agent: "shell", CapturedPath: "/bin",
		Status: "starting", StatusSource: "tmux", StatusAt: 1, CreatedAt: 1,
	})
	if err != nil {
		t.Fatalf("create fixture row: %v", err)
	}
	if _, err := svc.TMux.Create(ctx, tmux.Launch{
		Slug: session.Slug, CWD: cwd,
		Command: []string{"/bin/sh", "-c", "printf 'shell exited\\n'; exit 7"},
	}); err != nil {
		t.Fatalf("create fixture pane: %v", err)
	}
	waitForDeadPane(t, svc.TMux, session.Slug, 7)

	if err := db.SetSessionEnvValue(ctx, session.ID, "INJECT_TARGET", "should-not-reach-a-corpse", "user", 2); err != nil {
		t.Fatalf("dirty an env key on the fixture: %v", err)
	}
	if err := db.UpdateSessionStatus(ctx, store.StatusUpdateInput{
		SessionID: session.ID, Status: "stopped", Reason: "pane exited", Source: "tmux", At: 3,
	}); err != nil {
		t.Fatalf("mark fixture row stopped: %v", err)
	}
	row, err := db.GetSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("refresh fixture row: %v", err)
	}
	if row.Status != "stopped" || !row.EnvDirty {
		t.Fatalf("fixture row = status %q env_dirty %v, want stopped/dirty", row.Status, row.EnvDirty)
	}

	// The two liveness readings this fixture depends on being discriminating
	// (same pattern as retainedCorpseFixture): has-session still succeeds
	// for the retained corpse, while HasLivePane -- the notion R88 requires
	// InjectEnv to use instead -- correctly reports false.
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
		t.Fatal("fixture is not discriminating: the pane is still live")
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
		t.Fatal("fixture corpse has no pane id to compare against after a refused inject")
	}
	before, err := svc.TMux.CapturePane(ctx, corpsePane, tmux.CaptureOptions{StartLine: "0", EndLine: "-"})
	if err != nil {
		t.Fatalf("capture fixture corpse pane: %v", err)
	}
	return row, corpsePane, before
}

// TestInjectEnvRefusesARetainedDeadShellPane is R88 (finding F3): before the
// fix, InjectEnv decided "can this pane take send-keys?" with Exists, which
// a retained dead pane passes forever, so a stopped/error row with a corpse
// was not refused by this guard at all -- it fell through toward the
// SendKeys loop instead of being turned away up front with a message that
// says what happened and names the way out. This drives exactly that
// fixture and asserts the whole outcome: refused, with the right message;
// no send-keys literal ever reached the corpse's pane; and nothing was
// reported as applied -- env_dirty stays set on the durable row, not just
// on the value InjectEnv happened to return.
func TestInjectEnvRefusesARetainedDeadShellPane(t *testing.T) {
	cwd := t.TempDir()
	svc, db, _, _ := newAgentTestService(t, nil, "inject-retained-corpse")
	ctx := context.Background()

	row, corpsePane, before := retainedDeadShellPaneFixture(t, svc, db, "00000000-0000-4000-8000-000000000900", "dead-shell-pane", cwd)

	returned, keys, err := svc.InjectEnv(ctx, row.ID)
	if err == nil {
		t.Fatal("InjectEnv succeeded against a retained dead pane (issue #6's corpse): it must be refused")
	}
	msg := err.Error()
	if !strings.Contains(msg, "stopped or error row") || !strings.Contains(msg, "cannot take an injection") {
		t.Fatalf("refusal = %q, want it to say a stopped/error row cannot take an injection", msg)
	}
	if !strings.Contains(msg, "restart") || !strings.Contains(msg, "6.4") {
		t.Fatalf("refusal = %q, want it to name SPEC \u00a76.4's restart-to-apply route", msg)
	}
	if keys != nil {
		t.Fatalf("keys reported = %#v, want nil: nothing was applied", keys)
	}
	if !returned.EnvDirty {
		t.Fatal("InjectEnv's own returned session lost env_dirty for a refused injection")
	}

	after, err := db.GetSession(ctx, row.ID)
	if err != nil {
		t.Fatalf("get session after refused inject: %v", err)
	}
	if !after.EnvDirty {
		t.Fatal("the durable row's env_dirty flag was cleared even though the injection was refused -- nothing was applied")
	}

	afterCapture, err := svc.TMux.CapturePane(ctx, corpsePane, tmux.CaptureOptions{StartLine: "0", EndLine: "-"})
	if err != nil {
		t.Fatalf("capture corpse pane after refused inject: %v", err)
	}
	if string(afterCapture) != string(before) {
		t.Fatalf("corpse pane content changed after a refused inject: before %q, after %q", before, afterCapture)
	}
	if strings.Contains(string(afterCapture), "export INJECT_TARGET") {
		t.Fatal("a send-keys literal reached a retained dead pane")
	}
}
