package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// Reconcile compares durable rows with deck's private tmux server. A missing
// tmux session is a stopped (and therefore resumable) session; reconciliation
// never launches a replacement. Each observed disappearance is recorded both
// in the store's event log and in the JSONL audit log.
func (s Service) Reconcile(ctx context.Context) error {
	if s.Store == nil || s.Audit == nil || s.Clock == nil {
		return errors.New("reconciliation requires store, audit logger, and clock")
	}
	live, err := s.TMux.List(ctx)
	if err != nil {
		return fmt.Errorf("list tmux sessions for reconciliation: %w", err)
	}
	liveByName := make(map[string]tmux.Session, len(live))
	for _, session := range live {
		liveByName[session.Name] = session
	}
	rows, err := s.Store.ListSessions(ctx)
	if err != nil {
		return fmt.Errorf("list durable sessions for reconciliation: %w", err)
	}
	for _, session := range rows {
		if session.Status == "stopped" || session.PaneExitStatus != nil || (session.Status == "starting" && session.StatusSource == "user") {
			// A user-sourced row is between the durable create and tmux launch.
			// Once launch observes tmux it changes the source to tmux. A stored
			// crash is also terminal: collection deliberately removes its tmux
			// session, but that must not turn the error into a clean stop.
			continue
		}
		observed, present := liveByName["deck_"+session.Slug]
		if present {
			pane, crashed := crashedPane(observed)
			if !crashed {
				// Shells have no higher-quality signal or probe, so a live pane is
				// their sound starting → running transition. For agents it remains
				// liveness evidence only and must never fabricate working state.
				if session.Agent == "shell" && session.Status == "starting" {
					if err := s.Store.UpdateSessionStatus(ctx, store.StatusUpdateInput{
						SessionID: session.ID,
						Status:    "running",
						Reason:    "tmux pane is alive",
						Source:    "tmux",
						At:        s.Clock.Now().UnixMilli(),
						EventKind: "tmux.shell_live",
					}); err != nil {
						return fmt.Errorf("promote live shell session %q: %w", session.ID, err)
					}
					if err := s.Audit.Transition(session.ID, "tmux.shell_live"); err != nil {
						return fmt.Errorf("audit live shell session %q: %w", session.ID, err)
					}
				}
				continue
			}
			captured, err := s.TMux.CapturePane(ctx, pane.ID, tmux.CaptureOptions{StartLine: "-", EndLine: "-"})
			if err != nil {
				if tmux.IsTargetAbsent(err) {
					// Another unleased reconciler collected this corpse after our
					// List. Its atomic first-writer update owns the artifact.
					continue
				}
				return fmt.Errorf("capture crashed pane for session %q: %w", session.ID, err)
			}
			exitStatus := *pane.DeadStatus
			if err := s.Store.UpdateSessionStatus(ctx, store.StatusUpdateInput{
				SessionID:      session.ID,
				Status:         "error",
				Reason:         fmt.Sprintf("tmux pane exited with status %d", exitStatus),
				Source:         "tmux",
				At:             s.Clock.Now().UnixMilli(),
				EventKind:      "tmux.pane_dead",
				PaneExitStatus: &exitStatus,
				CrashTail:      crashTail(captured, 200),
			}); err != nil {
				return fmt.Errorf("record crashed pane for session %q: %w", session.ID, err)
			}
			if err := s.Audit.Transition(session.ID, "tmux.pane_dead"); err != nil {
				return fmt.Errorf("audit crashed tmux pane %q: %w", session.ID, err)
			}
			// Capture and the atomic store write must precede teardown. Kill is
			// idempotent, so racing observers need no collection lease.
			if err := s.TMux.Kill(ctx, session.Slug); err != nil {
				return fmt.Errorf("collect crashed tmux session %q: %w", session.ID, err)
			}
			continue
		}
		if err := s.Store.UpdateSessionStatus(ctx, store.StatusUpdateInput{
			SessionID: session.ID,
			Status:    "stopped",
			Reason:    "tmux session disappeared",
			Source:    "tmux",
			At:        s.Clock.Now().UnixMilli(),
			EventKind: "tmux.session_gone",
		}); err != nil {
			return fmt.Errorf("mark session %q stopped: %w", session.ID, err)
		}
		if err := s.Audit.Transition(session.ID, "tmux.session_gone"); err != nil {
			return fmt.Errorf("audit disappeared tmux session %q: %w", session.ID, err)
		}
	}
	return nil
}

func crashedPane(session tmux.Session) (tmux.Pane, bool) {
	for _, pane := range session.Panes {
		if pane.Dead && pane.DeadStatus != nil && *pane.DeadStatus != 0 {
			return pane, true
		}
	}
	return tmux.Pane{}, false
}

// crashTail converts captured terminal contents into inert UTF-8 text and
// keeps only the last maxLines. tmux capture without -e already omits terminal
// rendition escapes; the sanitizer is a second boundary against controls in
// malformed or synthetic captures before the bytes enter deck's own chrome.
func crashTail(captured []byte, maxLines int) string {
	plain := stripTerminalControls(strings.ToValidUTF8(string(captured), "�"))
	plain = strings.TrimSuffix(plain, "\n")
	if plain == "" || maxLines <= 0 {
		return ""
	}
	lines := strings.Split(plain, "\n")
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	return strings.Join(lines, "\n")
}

func stripTerminalControls(text string) string {
	var out strings.Builder
	for i := 0; i < len(text); {
		switch text[i] {
		case 0x1b:
			i++
			if i >= len(text) {
				continue
			}
			switch text[i] {
			case '[': // CSI: consume through its final byte.
				i++
				for i < len(text) {
					b := text[i]
					i++
					if b >= 0x40 && b <= 0x7e {
						break
					}
				}
			case ']': // OSC: consume through BEL or ST.
				i++
				for i < len(text) {
					if text[i] == 0x07 {
						i++
						break
					}
					if i+1 < len(text) && text[i] == 0x1b && text[i+1] == '\\' {
						i += 2
						break
					}
					i++
				}
			default: // A two-byte escape sequence.
				i++
			}
		default:
			r, size := utf8.DecodeRuneInString(text[i:])
			i += size
			if r == '\n' || r == '\t' || r >= 0x20 && r != 0x7f && !(r >= 0x80 && r <= 0x9f) {
				out.WriteRune(r)
			}
		}
	}
	return out.String()
}

// RunReconciler performs an immediate reconciliation and repeats it at the
// configured interval until ctx is cancelled. It does not create or bootstrap
// a tmux server, so a killed server is observed as an empty server rather than
// being relaunched.
func (s Service) RunReconciler(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		return errors.New("reconciliation interval must be positive")
	}
	if err := s.reconcileWithin(ctx, interval); err != nil {
		return err
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := s.reconcileWithin(ctx, interval); err != nil {
				return err
			}
		}
	}
}

// reconcileWithin gives every liveness pass the same bounded budget as the
// configured cadence. In particular, a stalled tmux command cannot consume
// many reconcile intervals and make an otherwise healthy client report stale
// liveness indefinitely.
func (s Service) reconcileWithin(ctx context.Context, interval time.Duration) error {
	passCtx, cancel := context.WithTimeout(ctx, interval)
	defer cancel()
	return s.Reconcile(passCtx)
}
