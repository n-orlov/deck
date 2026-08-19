package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/service"
	"github.com/n-orlov/deck/internal/store"
)

// TestResumeKeyRendersAwaitingSignalNeverRunning proves `r` on a stopped row
// calls the wired resume function, and that a successfully resumed row
// renders exactly "starting · awaiting signal" and never "running" until
// an agent hook or sampled probe supplies a real readiness verdict.
func TestResumeKeyRendersAwaitingSignalNeverRunning(t *testing.T) {
	resumed := store.Session{ID: "s1", Name: "alpha", Agent: "claude", Status: "starting"}
	model := NewWithShellCreatorAttacherKillerReconcilerAndResumer(
		nil, config.Settings{}, "", nil, nil, nil, nil,
		func(ctx context.Context, id string) (store.Session, service.ResumeOutcome, error) {
			if id != "s1" {
				t.Fatalf("resume called with unexpected id %q", id)
			}
			return resumed, service.ResumeStarted, nil
		},
	)
	model.sessions = []store.Session{{ID: "s1", Name: "alpha", Agent: "claude", Status: "stopped"}}
	model.selected = 0

	updated, cmd := model.Update(key("r"))
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("r did not dispatch a resume command")
	}
	msg := cmd()
	updated, loadCmd := model.Update(msg)
	model = updated.(Model)
	if loadCmd == nil {
		t.Fatal("a successful resume did not trigger a reload")
	}

	// Simulate the reload landing with the resumed (starting) row.
	updated, _ = model.Update(sessionsLoaded{sessions: []store.Session{resumed}})
	model = updated.(Model)

	view := model.View()
	if !strings.Contains(view, "starting \u00b7 awaiting signal") {
		t.Fatalf("resumed row did not render 'starting · awaiting signal':\n%s", view)
	}
	if strings.Contains(view, "running") {
		t.Fatalf("resumed agent row rendered 'running' without a readiness verdict:\n%s", view)
	}
}

// TestResumeStartingElsewhereIsNotAnError proves a caller that loses the
// launch-lease race renders "starting elsewhere" rather than an error
// message (SPEC §9.3).
func TestResumeStartingElsewhereIsNotAnError(t *testing.T) {
	model := NewWithShellCreatorAttacherKillerReconcilerAndResumer(
		nil, config.Settings{}, "", nil, nil, nil, nil,
		func(ctx context.Context, id string) (store.Session, service.ResumeOutcome, error) {
			return store.Session{}, service.ResumeStartingElsewhere, nil
		},
	)
	model.sessions = []store.Session{{ID: "s1", Name: "alpha", Agent: "claude", Status: "stopped"}}
	model.selected = 0

	updated, cmd := model.Update(key("r"))
	model = updated.(Model)
	msg := cmd()
	updated, _ = model.Update(msg)
	model = updated.(Model)

	view := model.View()
	if !strings.Contains(view, "starting elsewhere") {
		t.Fatalf("losing-lease resume did not render 'starting elsewhere':\n%s", view)
	}
	if model.attachError != "" {
		t.Fatalf("losing-lease resume was rendered as an error: %q", model.attachError)
	}
	if strings.Contains(view, "Cannot resume") {
		t.Fatalf("losing-lease resume rendered as an error:\n%s", view)
	}
}

func TestResumeNonLeasableRendersActualStatusAndReason(t *testing.T) {
	actual := store.Session{
		ID: "s1", Name: "alpha", Agent: "claude", Status: "waiting",
		StatusReason: "permission_prompt", StatusSource: "hook",
	}
	model := NewWithShellCreatorAttacherKillerReconcilerAndResumer(
		nil, config.Settings{}, "", nil, nil, nil, nil,
		func(ctx context.Context, id string) (store.Session, service.ResumeOutcome, error) {
			return actual, service.ResumeNotLeasable, nil
		},
	)
	// This stopped row is deliberately stale: it is what made r available.
	model.sessions = []store.Session{{ID: "s1", Name: "alpha", Agent: "claude", Status: "stopped"}}
	model.selected = 0

	updated, cmd := model.Update(key("r"))
	model = updated.(Model)
	updated, _ = model.Update(cmd())
	model = updated.(Model)

	view := model.View()
	if !strings.Contains(view, "waiting") {
		t.Fatalf("non-leasable resume did not render actual status:\n%s", view)
	}
	if strings.Contains(view, "starting elsewhere") {
		t.Fatalf("non-leasable row was misreported as starting elsewhere:\n%s", view)
	}
	model.detail = true
	detail := model.View()
	if !strings.Contains(detail, "Status reason:      permission_prompt") {
		t.Fatalf("detail did not render actual status reason:\n%s", detail)
	}
}

// TestResumeFailureRendersAsError proves a genuine resume failure (e.g. the
// three SPEC-named causes from task 012) is still surfaced as an error,
// distinct from losing the lease race.
func TestResumeFailureRendersAsError(t *testing.T) {
	model := NewWithShellCreatorAttacherKillerReconcilerAndResumer(
		nil, config.Settings{}, "", nil, nil, nil, nil,
		func(ctx context.Context, id string) (store.Session, service.ResumeOutcome, error) {
			return store.Session{}, service.ResumeStarted, errors.New("boom")
		},
	)
	model.sessions = []store.Session{{ID: "s1", Name: "alpha", Agent: "claude", Status: "stopped"}}
	model.selected = 0

	updated, cmd := model.Update(key("r"))
	model = updated.(Model)
	msg := cmd()
	updated, _ = model.Update(msg)
	model = updated.(Model)

	if !strings.Contains(model.attachError, "boom") {
		t.Fatalf("resume failure was not surfaced as an error: %q", model.attachError)
	}
}

// TestResumeKeyRequiresStoppedRow proves `r` is a no-op (no resume call, no
// error) on a row that is not stopped.
func TestResumeKeyRequiresStoppedRow(t *testing.T) {
	called := false
	model := NewWithShellCreatorAttacherKillerReconcilerAndResumer(
		nil, config.Settings{}, "", nil, nil, nil, nil,
		func(ctx context.Context, id string) (store.Session, service.ResumeOutcome, error) {
			called = true
			return store.Session{}, service.ResumeStarted, nil
		},
	)
	model.sessions = []store.Session{{ID: "s1", Name: "alpha", Agent: "claude", Status: "starting"}}
	model.selected = 0

	updated, _ := model.Update(key("r"))
	model = updated.(Model)
	if called {
		t.Fatal("r called resume on a non-stopped row")
	}
	if model.attachError == "" {
		t.Fatal("r on a non-stopped row did not explain why nothing happened")
	}
}
