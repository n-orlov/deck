package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/agent"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// copilotProbeRow creates a live row of kind whose pane prints the named
// copilot probe fixture, last written by a hook (status running, reason
// "prompt", the userPromptSubmitted mapping) one probe window ago.
func copilotProbeRow(t *testing.T, svc Service, db *store.Store, kind, profile, fixture string, n int) store.Session {
	t.Helper()
	cwd := t.TempDir()
	now := svc.Clock.Now().UnixMilli()
	stale := now - config.DefaultStaleAfter.Milliseconds()
	session, err := db.CreateSession(context.Background(), store.CreateSessionInput{
		ID: fmt.Sprintf("00000000-0000-4000-8000-%012d", 700+n), Name: kind + " " + fixture, CWD: cwd,
		Agent: kind, CapturedPath: "/bin", Status: "running", StatusSource: "hook",
		StatusAt: stale, CreatedAt: stale, PermissionProfile: profile,
	})
	if err != nil {
		t.Fatal(err)
	}
	pane, err := os.ReadFile(filepath.Join("..", "agent", "testdata", "probes", "copilot", fixture))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.TMux.Create(context.Background(), tmux.Launch{
		Slug: session.Slug, CWD: cwd,
		Command: []string{"/bin/sh", "-c", `printf '%s' "$1"; sleep 30`, "probe-fixture", string(pane)},
	}); err != nil {
		t.Fatal(err)
	}
	return session
}

func probeCopilotRow(t *testing.T, svc Service, db *store.Store, session store.Session) store.Session {
	t.Helper()
	if err := svc.ReconcileWithProbes(context.Background(), config.DefaultStaleAfter); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func newCopilotProbeService(t *testing.T, seed string) (Service, *store.Store) {
	t.Helper()
	isolateAgentHome(t)
	svc, db, _, _ := newAgentTestService(t, nil, seed)
	svc.Agents.Register(agent.NewCopilot())
	return svc, db
}

// R219c: a Ctrl-C abort fires no hook, so a row a userPromptSubmitted hook left
// running is corrected by the ordinary stale-verdict probe, with the window the
// other hook kinds use (config.DefaultStaleAfter), once the pane shows the idle footer.
func TestCopilotAbortDemotesAHookRunningRowWhoseIdleFooterIsShown(t *testing.T) {
	svc, db := newCopilotProbeService(t, "copilot-abort")
	row := copilotProbeRow(t, svc, db, "copilot", "safe", "idle.txt", 1)
	// Younger than the window, the same pane changes nothing.
	if err := db.UpdateSessionStatus(context.Background(), store.StatusUpdateInput{
		SessionID: row.ID, Status: "running", Reason: "prompt", Source: "hook", At: svc.Clock.Now().UnixMilli(), EventKind: "user_prompt_submitted",
	}); err != nil {
		t.Fatal(err)
	}
	if got := probeCopilotRow(t, svc, db, row); got.Status != "running" || got.StatusSource != "hook" {
		t.Fatalf("a fresh hook verdict was probed: %#v", got)
	}
	if err := db.UpdateSessionStatus(context.Background(), store.StatusUpdateInput{
		SessionID: row.ID, Status: "running", Reason: "prompt", Source: "hook",
		At: svc.Clock.Now().UnixMilli() - config.DefaultStaleAfter.Milliseconds(), EventKind: "user_prompt_submitted",
	}); err != nil {
		t.Fatal(err)
	}
	got := probeCopilotRow(t, svc, db, row)
	if got.Status != "idle" || got.StatusSource != "probe" || got.StatusReason != "ready" {
		t.Fatalf("aborted copilot row = status %q source %q reason %q, want idle/probe/ready", got.Status, got.StatusSource, got.StatusReason)
	}
}

func TestCopilotWorkingFooterIsNotDemotedByTheProbe(t *testing.T) {
	svc, db := newCopilotProbeService(t, "copilot-working")
	row := copilotProbeRow(t, svc, db, "copilot", "safe", "working.txt", 2)
	got := probeCopilotRow(t, svc, db, row)
	if got.Status != "running" {
		t.Fatalf("working copilot row = status %q source %q, want running", got.Status, got.StatusSource)
	}
}

// Profile honesty: Copilot's own settings can start it in Allow All whatever
// profile deck launched; the stored permission-profile reason (the detail
// pane's `degraded:` line) says so for safe and edits.
func TestCopilotAllowAllFooterOnASafeOrEditsLaunchIsReportedAsElevated(t *testing.T) {
	for i, profile := range []string{"safe", "edits"} {
		t.Run(profile, func(t *testing.T) {
			svc, db := newCopilotProbeService(t, "copilot-elevated-"+profile)
			row := copilotProbeRow(t, svc, db, "copilot", profile, "allow-all-footer.txt", 3+i)
			got := probeCopilotRow(t, svc, db, row)
			if !strings.Contains(got.PermissionProfileReason, "Copilot's own settings elevated the profile") {
				t.Fatalf("permission profile reason = %q, want it to say Copilot's own settings elevated the profile", got.PermissionProfileReason)
			}
			if got.PermissionProfile != profile {
				t.Fatalf("the launched profile was rewritten to %q", got.PermissionProfile)
			}
			again := probeCopilotRow(t, svc, db, row)
			if again.PermissionProfileReason != got.PermissionProfileReason {
				t.Fatalf("a second pass rewrote the reason: %q", again.PermissionProfileReason)
			}
		})
	}
}

func TestCopilotElevationReasonExtendsAnExistingDegradationReason(t *testing.T) {
	svc, db := newCopilotProbeService(t, "copilot-elevated-degraded")
	row := copilotProbeRow(t, svc, db, "copilot", "safe", "allow-all-footer.txt", 5)
	if err := db.SetPermissionProfileReason(context.Background(), row.ID, "copilot does not support permission profile \"plan\"; falling back to safe", "test", 1); err != nil {
		t.Fatal(err)
	}
	got := probeCopilotRow(t, svc, db, row)
	if !strings.HasPrefix(got.PermissionProfileReason, "copilot does not support") || !strings.Contains(got.PermissionProfileReason, "elevated the profile") {
		t.Fatalf("reason = %q, want the degradation reason extended by the elevation", got.PermissionProfileReason)
	}
}

func TestCopilotYoloShowingManualApprovalOrAllowAllIsNotFlagged(t *testing.T) {
	for i, fixture := range []string{"idle.txt", "allow-all-footer.txt"} {
		svc, db := newCopilotProbeService(t, "copilot-yolo-"+fixture)
		row := copilotProbeRow(t, svc, db, "copilot", "yolo", fixture, 6+i)
		if got := probeCopilotRow(t, svc, db, row); got.PermissionProfileReason != "" {
			t.Fatalf("yolo row with %s flagged: %q", fixture, got.PermissionProfileReason)
		}
	}
}

func TestCopilotSafeShowingManualApprovalIsNotFlagged(t *testing.T) {
	svc, db := newCopilotProbeService(t, "copilot-safe-manual")
	row := copilotProbeRow(t, svc, db, "copilot", "safe", "idle.txt", 8)
	if got := probeCopilotRow(t, svc, db, row); got.PermissionProfileReason != "" {
		t.Fatalf("safe row showing Manual Approval flagged: %q", got.PermissionProfileReason)
	}
}

func TestClaudeCodexAndPiRowsAreNeverFlaggedByTheCopilotProfileAudit(t *testing.T) {
	for i, kind := range []string{"claude", "codex", "pi"} {
		svc, db := newCopilotProbeService(t, "copilot-audit-other-"+kind)
		row := copilotProbeRow(t, svc, db, kind, "safe", "allow-all-footer.txt", 9+i)
		if got := probeCopilotRow(t, svc, db, row); got.PermissionProfileReason != "" {
			t.Fatalf("%s row flagged: %q", kind, got.PermissionProfileReason)
		}
	}
}
