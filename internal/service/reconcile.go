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
	return s.reconcile(ctx, 0)
}

// ReconcileWithProbes is the TUI-only reconciliation path. In addition to the
// same liveness observations as Reconcile, it samples eligible live agent panes
// whose last accepted verdict is at least staleAfter old. Hook callers must use
// Reconcile or ReconcileWithin instead so pane heuristics never enter the hook
// critical path.
func (s Service) ReconcileWithProbes(ctx context.Context, staleAfter time.Duration) error {
	if staleAfter <= 0 {
		return errors.New("probe stale_after must be positive")
	}
	if s.Agents == nil {
		return errors.New("probe reconciliation requires an agent registry")
	}
	return s.reconcile(ctx, staleAfter)
}

func (s Service) reconcile(ctx context.Context, staleAfter time.Duration) error {
	if s.Store == nil || s.Audit == nil || s.Clock == nil {
		return errors.New("reconciliation requires store, audit logger, and clock")
	}
	// Read durable intent before observing tmux. A resume atomically changes a
	// row to starting before it creates the pane; this ordering means a pass
	// racing that launch sees either the old stopped row (which is skipped) or
	// the newly-created pane. Listing tmux first could combine an old "absent"
	// snapshot with the later launch.ready row and falsely stop a successful
	// agent launch on another client.
	rows, err := s.Store.ListSessions(ctx)
	if err != nil {
		return fmt.Errorf("list durable sessions for reconciliation: %w", err)
	}
	live, err := s.TMux.List(ctx)
	if err != nil {
		return fmt.Errorf("list tmux sessions for reconciliation: %w", err)
	}
	liveByName := make(map[string]tmux.Session, len(live))
	for _, session := range live {
		liveByName[session.Name] = session
	}
	for _, session := range rows {
		// terminal marks the rows this pass takes no *liveness verdict* from. A
		// user-sourced starting row is between the durable create and tmux
		// launch; once launch observes tmux it changes the source to tmux. A
		// stopped row has no return edge here, and a stored crash is terminal:
		// collection deliberately removes its tmux session, but that absence
		// must not turn the error into a clean stop.
		//
		// It deliberately no longer suppresses crashed-pane COLLECTION (#6).
		// deck's server runs `remain-on-exit failed`, so a non-zero exit RETAINS
		// the pane and its session; when a SessionEnd hook writes `stopped` in
		// the same millisecond, a status-first short-circuit skipped the row
		// before tmux was ever consulted, so the corpse was never captured,
		// never killed, and held the session name against every later resume --
		// exactly the retention SPEC.md:547 forbids. A dead pane is therefore
		// collected on sight whatever the row says. Only the *status write*
		// stays guarded, and it is guarded where it always was, inside
		// UpdateSessionStatus: first-writer-wins on pane_exit_status keeps an
		// already-stored crash verdict and tail intact, so collecting a corpse
		// for an already-terminal row tears tmux down without rewriting history.
		terminal := session.Status == "stopped" || session.PaneExitStatus != nil ||
			(session.Status == "starting" && session.StatusSource == "user")
		observed, present := liveByName["deck_"+session.Slug]
		if present {
			pane, crashed := crashedPane(observed)
			if !crashed {
				// A live, non-dead pane paired with a stopped row, or with an
				// error row that itself carries a pane-exit or tmux/user-sourced
				// verdict, is SPEC §7's invariant violation regardless of
				// whether terminal (below) also holds for this row: the pane is
				// the part that is right, and the stored verdict is either
				// spent (a crash the live pane now contradicts) or was never
				// more than tmux's own liveness guess in the first place. A
				// hook- or probe-sourced error with no pane-exit verdict is
				// deliberately excluded: SPEC §7's transition table allows
				// running --turn or API failure--> error with no pane death at
				// all, so that row is the agent's own considered verdict, not a
				// contradiction tmux liveness gets to overrule (finding F40,
				// task 901); it still owns :188's session-absent branch,
				// unmodified.
				if session.Status == "stopped" ||
					(session.Status == "error" && (session.PaneExitStatus != nil || session.StatusSource == "tmux" || session.StatusSource == "user")) {
					if err := s.repairTerminalRowWithLivePane(ctx, session); err != nil {
						return err
					}
					continue
				}
				if terminal {
					// A user-sourced starting row (between the durable create and
					// the tmux launch) is not an invariant violation, only a
					// transient window this pass takes no verdict from; it
					// resolves on its own via tmuxLaunchObservation once the
					// launch is observed.
					continue
				}
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
				if staleAfter > 0 && session.Agent != "shell" && probeEligible(session, s.Clock.Now(), staleAfter) {
					if len(observed.Panes) == 0 {
						continue
					}
					captured, err := s.TMux.CapturePane(ctx, observed.Panes[0].ID, tmux.CaptureOptions{StartLine: "-200", EndLine: "-"})
					if err != nil {
						if tmux.IsTargetAbsent(err) {
							continue
						}
						return fmt.Errorf("capture probe pane for session %q: %w", session.ID, err)
					}
					adapter, ok := s.Agents.Lookup(session.Agent)
					if !ok {
						return fmt.Errorf("probe session %q: unknown agent %q", session.ID, session.Agent)
					}
					status, reason := adapter.Probe(string(captured))
					now := s.Clock.Now().UnixMilli()
					if status != "" {
						if err := s.Store.UpdateSessionStatus(ctx, store.StatusUpdateInput{
							SessionID: session.ID, Status: status, Reason: reason, Source: "probe", At: now,
							StaleAfter: staleAfter.Milliseconds(), EventKind: "probe." + status,
						}); err != nil {
							return fmt.Errorf("record probe for session %q: %w", session.ID, err)
						}
					} else {
						// A total miss (no probeRule matched at all) is diagnostic
						// evidence, not a verdict: it must never touch status,
						// status_source or status_at (SPEC §7 is untouched), but is
						// still worth recording so the `i` detail dialog can tell
						// "sampled, no rule matched" apart from "never sampled"
						// (task 009).
						if err := s.Store.RecordProbeMiss(ctx, session.ID, now); err != nil {
							return fmt.Errorf("record probe miss for session %q: %w", session.ID, err)
						}
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
		if terminal {
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

// repairTerminalRowWithLivePane is SPEC §7's one self-healing rule: a
// terminal status (stopped, error) paired with a live, non-dead pane is an
// invariant violation, not evidence to act on -- the pane is the part that
// is right. It corrects the row from what liveness alone can observe: for a
// shell row that is exactly the §7 shell-liveness rule (a live pane always
// means running, because a shell has no other signal, ever); for an agent
// row liveness supplies no verdict at all, so the row is reset to the
// neutral "starting" a fresh pane always begins at, leaving the hook/probe
// rules to take it from there on a later pass. The two terminal verdicts the
// row may be carrying are spent by the same observation and are cleared with
// it: killed_by_user exists so an in-flight hook cannot undo an explicit
// kill, and a stored pane_exit_status/crash tail describes a pane that died
// -- a live pane is direct evidence against both. Left set, either one
// re-freezes the row the moment it is repaired (the status reads live while
// every hook is outranked forever, and PaneExitStatus alone keeps the row
// terminal for every later pass), which is SPEC §9.1's "spent verdict
// outranks everything forever" bug and the very unrecoverable row §7 sends
// this repair to fix. Either way the pane itself is touched not at all -- no
// kill, no respawn, no send-keys -- and the correction is recorded as an
// event, the same way every other reconcile verdict is. It is unleased and
// idempotent: once applied, the row is no longer terminal, so a repeat pass
// takes the ordinary (non-terminal, no-op) path above instead of repairing it
// again.
func (s Service) repairTerminalRowWithLivePane(ctx context.Context, session store.Session) error {
	status, reason, eventKind := "starting", "tmux pane is alive; terminal row corrected", "tmux.terminal_pane_alive"
	if session.Agent == "shell" {
		status, reason, eventKind = "running", "tmux pane is alive", "tmux.shell_live"
	}
	if err := s.Store.UpdateSessionStatus(ctx, store.StatusUpdateInput{
		SessionID:         session.ID,
		Status:            status,
		Reason:            reason,
		Source:            "tmux",
		At:                s.Clock.Now().UnixMilli(),
		EventKind:         eventKind,
		ClearKilledByUser: true,
		ClearCrashVerdict: true,
	}); err != nil {
		return fmt.Errorf("repair terminal row with live pane for session %q: %w", session.ID, err)
	}
	if err := s.Audit.Transition(session.ID, eventKind); err != nil {
		return fmt.Errorf("audit terminal-row repair for session %q: %w", session.ID, err)
	}
	return nil
}

func probeEligible(session store.Session, now time.Time, staleAfter time.Duration) bool {
	switch session.Status {
	case "starting", "running", "waiting":
		return now.UnixMilli()-session.StatusAt >= staleAfter.Milliseconds()
	default:
		return false
	}
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
	if err := s.ReconcileWithin(ctx, interval); err != nil {
		return err
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := s.ReconcileWithin(ctx, interval); err != nil {
				return err
			}
		}
	}
}

// ReconcileWithin runs one liveness-only pass with a hard budget. It is shared
// by the TUI loop and the post-hook path: neither caller may be held forever by
// a stalled tmux command, and this surface deliberately contains no probing.
func (s Service) ReconcileWithin(ctx context.Context, budget time.Duration) error {
	if budget <= 0 {
		return errors.New("reconciliation budget must be positive")
	}
	passCtx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	return s.Reconcile(passCtx)
}
