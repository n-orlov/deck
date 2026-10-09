package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/n-orlov/deck/internal/agent"
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
		if err := s.reconcileRow(ctx, session, liveByName, staleAfter); err != nil {
			return err
		}
	}
	return nil
}

// reconcileRow takes one durable row's verdict from the tmux snapshot: an
// absent session is recorded as gone, a dead non-zero pane is collected, and a
// live pane is repaired, promoted or probed.
func (s Service) reconcileRow(ctx context.Context, session store.Session, liveByName map[string]tmux.Session, staleAfter time.Duration) error {
	observed, present := liveByName["deck_"+session.Slug]
	if !present {
		return s.recordSessionGone(ctx, session)
	}
	if pane, crashed := crashedPane(observed); crashed {
		return s.collectCrashedPane(ctx, session, pane)
	}
	return s.reconcileLivePane(ctx, session, observed, staleAfter)
}

// rowIsTerminal marks the rows this pass takes no *liveness verdict* from. A
// user-sourced starting row is between the durable create and tmux launch;
// once launch observes tmux it changes the source to tmux. A stopped row has
// no return edge here, and a stored crash is terminal: collection deliberately
// removes its tmux session, but that absence must not turn the error into a
// clean stop.
//
// It deliberately does not suppress crashed-pane COLLECTION (#6). deck's
// server runs `remain-on-exit failed`, so a non-zero exit RETAINS the pane and
// its session; when a SessionEnd hook writes `stopped` in the same
// millisecond, a status-first short-circuit skipped the row before tmux was
// ever consulted, so the corpse was never captured, never killed, and held the
// session name against every later resume -- exactly the retention SPEC.md:547
// forbids. A dead pane is therefore collected on sight whatever the row says.
// Only the *status write* stays guarded, inside UpdateSessionStatus:
// first-writer-wins on pane_exit_status keeps an already-stored crash verdict
// and tail intact.
//
// An `error` row whose source is `tmux` or `user` (task 011, M9) is ALSO
// terminal here, for the absent-pane case specifically: launchFailed
// (resume.go/shell.go) writes exactly this shape for every one of SPEC §9.3's
// three named resume failures, none of which ever create a tmux session for
// this attempt -- so there is nothing for this pass to observe as "gone" that
// the row does not already claim; without it the specific, SPEC-mandated
// reason would be replaced by the generic "tmux session disappeared". A hook-
// or probe-sourced error (a turn/API failure with the pane still presumably
// alive) is deliberately excluded: it still owns the write if its pane later
// genuinely disappears (SPEC §7's "any -> stopped" on clean exit).
func rowIsTerminal(session store.Session) bool {
	return session.Status == "stopped" || session.PaneExitStatus != nil ||
		(session.Status == "error" && (session.StatusSource == "tmux" || session.StatusSource == "user")) ||
		(session.Status == "starting" && session.StatusSource == "user")
}

// recordSessionGone writes the stopped verdict for a row whose tmux session is
// absent, unless the row already claims the process is gone.
func (s Service) recordSessionGone(ctx context.Context, session store.Session) error {
	if rowIsTerminal(session) {
		return nil
	}
	var seq int64
	at := s.Clock.Now()
	const reason = "tmux session disappeared"
	if err := s.Store.UpdateSessionStatus(ctx, store.StatusUpdateInput{
		SessionID: session.ID,
		Status:    "stopped",
		Reason:    reason,
		Source:    "tmux",
		At:        at.UnixMilli(),
		EventKind: "tmux.session_gone",
		EventSeq:  &seq,
	}); err != nil {
		return fmt.Errorf("mark session %q stopped: %w", session.ID, err)
	}
	if err := s.Audit.Transition(session.ID, "tmux.session_gone"); err != nil {
		return fmt.Errorf("audit disappeared tmux session %q: %w", session.ID, err)
	}
	// A clean disappearance is a process death too (SPEC §10.4): it offers
	// `ended`, after the event row is durable.
	s.offerHook(ctx, HookEvent{SessionID: session.ID, StoredKind: "tmux.session_gone", Reason: reason,
		At: at, AppliedStatus: "stopped", EventSeq: seq})
	return nil
}

// collectCrashedPane records the crash verdict and tail of a retained dead
// pane, then tears its tmux session down. Capture and the atomic store write
// must precede teardown. Kill is idempotent, so racing observers need no
// collection lease.
func (s Service) collectCrashedPane(ctx context.Context, session store.Session, pane tmux.Pane) error {
	captured, err := s.TMux.CapturePane(ctx, pane.ID, tmux.CaptureOptions{StartLine: "-", EndLine: "-"})
	if err != nil {
		if tmux.IsTargetAbsent(err) {
			// Another unleased reconciler collected this corpse after our
			// List. Its atomic first-writer update owns the artifact.
			return nil
		}
		return fmt.Errorf("capture crashed pane for session %q: %w", session.ID, err)
	}
	if err := s.recordCrashedPane(ctx, session, pane, captured); err != nil {
		return err
	}
	if err := s.TMux.Kill(ctx, session.Slug); err != nil {
		return fmt.Errorf("collect crashed tmux session %q: %w", session.ID, err)
	}
	return nil
}

// recordCrashedPane writes the first-writer crash verdict, with the captured
// tail, for a dead pane and audits it.
func (s Service) recordCrashedPane(ctx context.Context, session store.Session, pane tmux.Pane, captured []byte) error {
	exitStatus := *pane.DeadStatus
	var seq int64
	at := s.Clock.Now()
	reason := fmt.Sprintf("tmux pane exited with status %d", exitStatus)
	tail := crashTail(captured, 200)
	if err := s.Store.UpdateSessionStatus(ctx, store.StatusUpdateInput{
		SessionID:      session.ID,
		Status:         "error",
		Reason:         reason,
		Source:         "tmux",
		At:             at.UnixMilli(),
		EventKind:      "tmux.pane_dead",
		PaneExitStatus: &exitStatus,
		CrashTail:      tail,
		EventSeq:       &seq,
	}); err != nil {
		return fmt.Errorf("record crashed pane for session %q: %w", session.ID, err)
	}
	if err := s.Audit.Transition(session.ID, "tmux.pane_dead"); err != nil {
		return fmt.Errorf("audit crashed tmux pane %q: %w", session.ID, err)
	}
	s.offerHook(ctx, HookEvent{SessionID: session.ID, StoredKind: "tmux.pane_dead", Reason: reason,
		Message: tail, At: at, AppliedStatus: "error", EventSeq: seq})
	return nil
}

// repairsWithLivePane reports whether a live, non-dead pane contradicts the
// row's stored verdict: a stopped row, or an error row that itself carries a
// pane-exit or tmux/user-sourced verdict, is SPEC §7's invariant violation
// regardless of whether rowIsTerminal also holds -- the pane is the part that
// is right. A hook- or probe-sourced error with no pane-exit verdict is
// deliberately excluded: SPEC §7's transition table allows running --turn or
// API failure--> error with no pane death at all, so that row is the agent's
// own considered verdict, not a contradiction tmux liveness gets to overrule
// (finding F40, task 901).
func repairsWithLivePane(session store.Session) bool {
	return session.Status == "stopped" ||
		(session.Status == "error" && (session.PaneExitStatus != nil || session.StatusSource == "tmux" || session.StatusSource == "user"))
}

// reconcileLivePane handles a row whose tmux session is present with no
// crashed pane.
func (s Service) reconcileLivePane(ctx context.Context, session store.Session, observed tmux.Session, staleAfter time.Duration) error {
	if repairsWithLivePane(session) {
		return s.repairTerminalRowWithLivePane(ctx, session)
	}
	if rowIsTerminal(session) {
		// A user-sourced starting row (between the durable create and the tmux
		// launch) is not an invariant violation, only a transient window this
		// pass takes no verdict from; it resolves on its own via
		// tmuxLaunchObservation once the launch is observed.
		return nil
	}
	if err := s.promoteLiveShell(ctx, session); err != nil {
		return err
	}
	if staleAfter > 0 && session.Agent != "shell" && probeEligible(session, s.Clock.Now(), staleAfter) {
		return s.probeLivePane(ctx, session, observed, staleAfter)
	}
	return nil
}

// promoteLiveShell: shells have no higher-quality signal or probe, so a live
// pane is their sound starting → running transition. For agents it remains
// liveness evidence only and must never fabricate working state.
func (s Service) promoteLiveShell(ctx context.Context, session store.Session) error {
	if session.Agent != "shell" || session.Status != "starting" {
		return nil
	}
	if err := s.Store.UpdateSessionStatus(ctx, store.StatusUpdateInput{
		SessionID:              session.ID,
		Status:                 "running",
		Reason:                 "tmux pane is alive",
		Source:                 "tmux",
		At:                     s.Clock.Now().UnixMilli(),
		AllowedCurrentStatuses: []string{"starting"},
		EventKind:              "tmux.shell_live",
	}); err != nil {
		return fmt.Errorf("promote live shell session %q: %w", session.ID, err)
	}
	if err := s.Audit.Transition(session.ID, "tmux.shell_live"); err != nil {
		return fmt.Errorf("audit live shell session %q: %w", session.ID, err)
	}
	return nil
}

// probeLivePane samples an eligible live agent pane and records the verdict.
func (s Service) probeLivePane(ctx context.Context, session store.Session, observed tmux.Session, staleAfter time.Duration) error {
	if len(observed.Panes) == 0 {
		return nil
	}
	captured, err := s.TMux.CapturePane(ctx, observed.Panes[0].ID, tmux.CaptureOptions{StartLine: "-200", EndLine: "-"})
	if err != nil {
		if tmux.IsTargetAbsent(err) {
			return nil
		}
		return fmt.Errorf("capture probe pane for session %q: %w", session.ID, err)
	}
	adapter, ok := s.Agents.Lookup(session.Agent)
	if !ok {
		return fmt.Errorf("probe session %q: unknown agent %q", session.ID, session.Agent)
	}
	if err := s.auditProfile(ctx, session, adapter, string(captured)); err != nil {
		return err
	}
	status, reason := adapter.Probe(string(captured))
	return s.recordProbe(ctx, session, status, reason, staleAfter)
}

// auditProfile reports, through the stored permission-profile reason the
// detail pane shows as `degraded:`, a live pane that is more permissive than
// the profile deck launched. An existing reason (an unsupported profile that
// fell back to safe) is kept and extended, and an already-reported pane is not
// rewritten on every pass.
func (s Service) auditProfile(ctx context.Context, session store.Session, adapter agent.Adapter, pane string) error {
	auditor, ok := adapter.(agent.ProfileAuditor)
	if !ok {
		return nil
	}
	reason := auditor.AuditProfile(session.PermissionProfile, pane)
	if reason == "" || strings.Contains(session.PermissionProfileReason, reason) {
		return nil
	}
	if session.PermissionProfileReason != "" {
		reason = session.PermissionProfileReason + "; " + reason
	}
	if err := s.Store.SetPermissionProfileReason(ctx, session.ID, reason, "probe", s.Clock.Now().UnixMilli()); err != nil {
		return fmt.Errorf("record elevated profile for session %q: %w", session.ID, err)
	}
	return nil
}

// recordProbe stores a probe verdict. A total miss (no probeRule matched at
// all) is diagnostic evidence, not a verdict: it must never touch status,
// status_source or status_at (SPEC §7 is untouched), but is still worth
// recording so the `i` detail dialog can tell "sampled, no rule matched" apart
// from "never sampled" (task 009).
func (s Service) recordProbe(ctx context.Context, session store.Session, status, reason string, staleAfter time.Duration) error {
	now := s.Clock.Now().UnixMilli()
	if status == "" {
		if err := s.Store.RecordProbeMiss(ctx, session.ID, now); err != nil {
			return fmt.Errorf("record probe miss for session %q: %w", session.ID, err)
		}
		return nil
	}
	var seq int64
	if err := s.Store.UpdateSessionStatus(ctx, store.StatusUpdateInput{
		SessionID: session.ID, Status: status, Reason: reason, Source: "probe", At: now,
		StaleAfter: staleAfter.Milliseconds(), EventKind: "probe." + status, EventSeq: &seq,
	}); err != nil {
		return fmt.Errorf("record probe for session %q: %w", session.ID, err)
	}
	// Only a probe that CHANGED the status is an event of its own; a verdict
	// that repeats what a hook (or an earlier probe) already put on the row is
	// the same attention episode, not a second event to ping about.
	if session.Status != status {
		s.offerHook(ctx, HookEvent{SessionID: session.ID, StoredKind: "probe." + status, Reason: reason,
			At: s.Clock.Now(), AppliedStatus: status, EventSeq: seq})
	}
	return nil
}

// repairTerminalRowWithLivePane is SPEC §7's one self-healing rule: a
// stopped row, or an error row that itself already carries a pane-exit or
// tmux/user-sourced verdict, paired with a live, non-dead pane is an
// invariant violation, not evidence to act on -- the pane is the part that
// is right. A hook- or probe-sourced error row with no pane-exit verdict is
// deliberately excluded: SPEC §7's transition table allows running --turn or
// API failure--> error with no pane death at all, so that row is the
// agent's own considered verdict, not a contradiction tmux liveness gets to
// overrule (finding F40, task 901); the caller (reconcile.go's terminal-row
// branch above) is what enforces that narrower trigger before ever calling
// this function. It corrects the row from what liveness alone can observe: for a
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
		if text[i] == 0x1b {
			i = skipEscapeSequence(text, i)
			continue
		}
		r, size := utf8.DecodeRuneInString(text[i:])
		i += size
		if keepTerminalRune(r) {
			out.WriteRune(r)
		}
	}
	return out.String()
}

// skipEscapeSequence returns the index just past the escape sequence that
// starts at text[i] (an ESC byte): a CSI, an OSC, or a two-byte escape.
func skipEscapeSequence(text string, i int) int {
	i++
	if i >= len(text) {
		return i
	}
	switch text[i] {
	case '[':
		return skipCSI(text, i+1)
	case ']':
		return skipOSC(text, i+1)
	default: // A two-byte escape sequence.
		return i + 1
	}
}

// skipCSI consumes a CSI sequence's parameters through its final byte.
func skipCSI(text string, i int) int {
	for i < len(text) {
		b := text[i]
		i++
		if b >= 0x40 && b <= 0x7e {
			break
		}
	}
	return i
}

// skipOSC consumes an OSC sequence through BEL or ST.
func skipOSC(text string, i int) int {
	for i < len(text) {
		if text[i] == 0x07 {
			return i + 1
		}
		if i+1 < len(text) && text[i] == 0x1b && text[i+1] == '\\' {
			return i + 2
		}
		i++
	}
	return i
}

// keepTerminalRune reports whether a rune survives control stripping:
// newline, tab, and printable runes outside DEL and the C1 range.
func keepTerminalRune(r rune) bool {
	return r == '\n' || r == '\t' || r >= 0x20 && r != 0x7f && (r < 0x80 || r > 0x9f)
}

// RunReconciler performs an immediate reconciliation and repeats it at the
// configured interval until ctx is cancelled. It does not create or bootstrap
// a tmux server, so a killed server is observed as an empty server rather than
// being relaunched.
//
// The cadence is anchored at the loop's start, not at the end of the first
// pass: the ticker is created before the immediate pass, so a first snapshot
// that is slow to return (and may have been taken just before a session
// vanished) cannot push the detecting second pass a whole extra interval out.
func (s Service) RunReconciler(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		return errors.New("reconciliation interval must be positive")
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	if err := s.ReconcileWithin(ctx, interval); err != nil {
		return err
	}
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
// The event hook for a process death the pass records is spawned after the
// pass returns (SPEC §10.4): its event_hook_timeout is its own bound, never
// charged against the pass's budget.
func (s Service) ReconcileWithin(ctx context.Context, budget time.Duration) error {
	if budget <= 0 {
		return errors.New("reconciliation budget must be positive")
	}
	queued, hooks := withDeferredHooks(ctx)
	passCtx, cancel := context.WithTimeout(queued, budget)
	err := s.Reconcile(passCtx)
	cancel()
	s.dispatchDeferred(ctx, hooks)
	return err
}
