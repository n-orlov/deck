package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/n-orlov/deck/internal/agent"
	"github.com/n-orlov/deck/internal/store"
)

// Event kinds a rename push writes (SPEC §8 "Pushing a rename").
const (
	// EventHarnessRenameRefused records a push deck declined to send, with the reason.
	EventHarnessRenameRefused = "harness_rename_refused"
	// EventHarnessRenameFailed records a push that was sent or attempted and not confirmed.
	EventHarnessRenameFailed = "harness_rename_failed"
)

// pushConfirmTimeout is how long a pushed name may stay pending before the
// push is declared failed.
const pushConfirmTimeout = 30 * time.Second

// PanePort is the pane access a rename push needs: the visible screen (to
// look for a focused dialog) and the keystroke plan send. nil on Service
// means the real tmux client.
type PanePort interface {
	Screen(ctx context.Context, slug string) (string, error)
	SendPlan(ctx context.Context, slug string, plan []string) error
}

type tmuxPanePort struct{ s Service }

func (p tmuxPanePort) Screen(ctx context.Context, slug string) (string, error) {
	capture, err := p.s.TMux.CapturePreview(ctx, slug)
	if err != nil || !capture.Live {
		return "", err
	}
	return string(capture.Bytes), nil
}

func (p tmuxPanePort) SendPlan(ctx context.Context, slug string, plan []string) error {
	return p.s.TMux.SendKeyPlan(ctx, slug, plan)
}

func (s Service) panePort() PanePort {
	if s.NamePane != nil {
		return s.NamePane
	}
	return tmuxPanePort{s}
}

// PushResult is what a rename push did, for the caller to surface. Note is
// the footer note: empty when there is nothing to say.
type PushResult struct {
	// Sent reports whether the keystroke plan was sent to the running agent.
	Sent bool
	// Note names the reason a push was refused or failed, or says it was sent.
	Note string
}

// RenameAndPush is Rename followed by the push of the new name into a running
// harness whose adapter declares PushRename=sendkeys. The rename itself
// always stands; a refused or failed push is reported in the result and never
// as an error.
func (s Service) RenameAndPush(ctx context.Context, sessionID, newName string) (store.Session, PushResult, error) {
	session, err := s.Rename(ctx, sessionID, newName)
	if err != nil {
		return session, PushResult{}, err
	}
	result := s.pushRename(ctx, session)
	updated, err := s.Store.GetSession(ctx, sessionID)
	if err != nil {
		return session, result, nil
	}
	return updated, result, nil
}

// pushStatusRefusals names why a status other than idle or running cannot
// take a push.
var pushStatusRefusals = map[string]string{
	"waiting":  "the harness is waiting for an answer, so a keystroke would land in its prompt",
	"starting": "the harness is still starting, so it would swallow the keystrokes",
	"error":    "the harness is in an error state",
}

// pushRename decides and performs the push for a just-renamed row. A shell, an
// adapter whose caps do not claim PushRename=sendkeys and a stopped row send
// nothing and say nothing: a stopped row takes the name at its next launch.
func (s Service) pushRename(ctx context.Context, session store.Session) PushResult {
	adapter, ok := s.pushAdapter(session)
	if !ok || session.Status == "stopped" {
		return PushResult{}
	}
	if reason := s.pushRefusal(ctx, adapter, session); reason != "" {
		return s.refusePush(ctx, session, reason)
	}
	keys, ok := agent.PushName(adapter, agent.NamePushInput{Name: strings.TrimSpace(session.Name)})
	if !ok {
		return s.refusePush(ctx, session, "the "+session.Agent+" adapter declined to build a rename")
	}
	if err := s.panePort().SendPlan(ctx, session.Slug, keys); err != nil {
		return s.failPush(ctx, session, fmt.Sprintf("the keystrokes could not be sent: %v", err))
	}
	s.markPending(ctx, adapter, session)
	return PushResult{Sent: true, Note: "rename sent to " + session.Agent}
}

func (s Service) pushAdapter(session store.Session) (agent.Adapter, bool) {
	if s.Agents == nil || s.Store == nil || s.Clock == nil || session.Agent == "shell" {
		return nil, false
	}
	adapter, ok := s.Agents.Lookup(session.Agent)
	if !ok || !adapter.Capabilities().Name.ClaimsPush() {
		return nil, false
	}
	return adapter, true
}

// pushRefusal is the reason a push must not be sent now, or "" when it may be:
// status idle or running, and no harness dialog focused.
func (s Service) pushRefusal(ctx context.Context, adapter agent.Adapter, session store.Session) string {
	if session.Status != "idle" && session.Status != "running" {
		if reason, known := pushStatusRefusals[session.Status]; known {
			return fmt.Sprintf("status is %s: %s", session.Status, reason)
		}
		return fmt.Sprintf("status is %s, not idle or running", session.Status)
	}
	if !agent.IsDialogDetector(adapter) {
		return ""
	}
	screen, err := s.panePort().Screen(ctx, session.Slug)
	if err != nil {
		return "the pane could not be read to check for a focused dialog"
	}
	if agent.DialogFocused(adapter, screen) {
		return "a harness dialog is focused and would swallow the keystrokes"
	}
	return ""
}

func (s Service) refusePush(ctx context.Context, session store.Session, reason string) PushResult {
	_ = s.Store.RecordSessionEvent(ctx, session.ID, EventHarnessRenameRefused, reason, s.Clock.Now().UnixMilli())
	return PushResult{Note: fmt.Sprintf("rename not pushed to %s: %s", session.Agent, reason)}
}

func (s Service) failPush(ctx context.Context, session store.Session, reason string) PushResult {
	_ = s.Store.RecordSessionEvent(ctx, session.ID, EventHarnessRenameFailed, reason, s.Clock.Now().UnixMilli())
	return PushResult{Note: fmt.Sprintf("rename of %s in %s failed: %s", session.Name, session.Agent, reason)}
}

// markPending holds the pushed name in pending_harness_name when the adapter
// can report a name back, so the observe path can recognise the echo. An
// adapter with no observation source can never confirm, so nothing is held.
func (s Service) markPending(ctx context.Context, adapter agent.Adapter, session store.Session) {
	if !adapter.Capabilities().Name.ClaimsRead() {
		return
	}
	_ = s.Store.SetPendingHarnessName(ctx, session.ID, strings.TrimSpace(session.Name), s.Clock.Now().UnixMilli())
}

// settlePush resolves a row's pending pushed name on a tick or hook. The
// observed echo of the pushed name confirms the push: the field clears and
// nothing is adopted. A pending name older than the timeout is a failed
// confirmation: one harness_rename_failed event, the field clears, and the
// keystrokes are never sent again. A row that has stopped just drops it.
func (s Service) settlePush(ctx context.Context, session store.Session) {
	if rowIsTerminal(session) {
		_ = s.Store.ClearPendingHarnessName(ctx, session.ID)
		return
	}
	if s.observedPending(ctx, session) {
		_ = s.Store.ClearPendingHarnessName(ctx, session.ID)
		return
	}
	age := s.Clock.Now().UnixMilli() - session.PendingHarnessNameAt
	if age < pushConfirmTimeout.Milliseconds() {
		return
	}
	reason := fmt.Sprintf("the harness did not report the name %q within %s; it was not retried", session.PendingHarnessName, pushConfirmTimeout)
	_ = s.Store.ClearPendingHarnessName(ctx, session.ID)
	_ = s.Store.RecordSessionEvent(ctx, session.ID, EventHarnessRenameFailed, reason, s.Clock.Now().UnixMilli())
}

// observedPending reports whether the harness's own current name is the one
// deck pushed.
func (s Service) observedPending(ctx context.Context, session store.Session) bool {
	adapter, ok := s.Agents.Lookup(session.Agent)
	if !ok || session.ConversationID == "" {
		return false
	}
	name, _, ok := agent.ReadName(adapter, s.nameProbe(ctx, adapter, session))
	return ok && strings.TrimSpace(name) == session.PendingHarnessName
}
