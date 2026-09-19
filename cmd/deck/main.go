// Command deck starts the deck terminal user interface.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/agent"
	"github.com/n-orlov/deck/internal/audit"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/hookrecv"
	"github.com/n-orlov/deck/internal/service"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
	"github.com/n-orlov/deck/internal/tui"
)

func main() { os.Exit(run(os.Args, os.Stdin, os.Stdout, os.Stderr)) }

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if isVersionRequest(args) {
		printVersion(stdout)
		return 0
	}
	isHook := len(args) == 2 && args[1] == "_hook"
	settings, err := config.Load()
	if err != nil {
		fmt.Fprintln(stderr, "deck configuration:", err)
		if isHook {
			return 1
		}
		return 0
	}
	if isHook {
		if err := runHook(context.Background(), settings, stdin); err != nil {
			fmt.Fprintln(stderr, "deck hook:", err)
			return 1
		}
		return 0
	}

	stopClockStep := startClockStepTrigger(settings.Clock, stderr)
	defer stopClockStep()

	db, err := store.Open(settings.Paths)
	if err != nil {
		fmt.Fprintln(stderr, "deck state:", err)
		return 0
	}
	defer db.Close()

	// R62 (steer 3e-001 §6.4/§7): the first EnforceEventRetention call --
	// right here, on store open -- always performs its deletion pass (no
	// ui_state row yet records a last run); every later call, from
	// tuiReconcile below, is throttled internally to at most once an hour.
	// The hidden hook command's own store.Open (below, in runHook) does NOT
	// call this: hooks share the same tight, measured budget that already
	// keeps pane-text probing out of their critical path (see tuiReconcile's
	// own comment), and the throttle above already guarantees this main
	// process's own next reconcile tick will catch up within the hour.
	if err := db.EnforceEventRetention(context.Background(), settings.EventRetentionDays, settings.Clock.Now().UnixMilli()); err != nil {
		fmt.Fprintln(stderr, "deck event retention:", err)
		return 0
	}
	// Task 010's store-open call site, deliberately right beside R62's above
	// and throttled by the same ui_state mechanism: a row tombstoned by `dd`
	// and then abandoned (deck quit, was killed, or crashed before its
	// tea.Tick deleteGraceExpired could fire) has nothing left in-process to
	// reap it, so the next open does it. A sweep failure must not stop deck
	// from starting -- unlike the event-retention call above, this is a
	// best-effort backlog catch-up, not part of any user-visible promise made
	// at open time -- so it is reported and stepped over. It reaps at most one
	// bounded batch before returning, so the work here cannot grow with the
	// size of the backlog; tuiReconcile's very first tick below (a fraction of
	// a second later, per settings.Reconcile) then calls db.DrainExpiredTombstones
	// instead of a single bounded pass, so a backlog bigger than one batch
	// finishes draining in that SAME open/startup cycle (review finding 3,
	// R79) rather than needing an extra real hour per leftover batch.
	preFrameTombstoneSweep(context.Background(), db, settings, stderr)

	logger, err := audit.New(settings.Paths, settings.Clock)
	if err != nil {
		fmt.Fprintln(stderr, "deck audit:", err)
		return 0
	}
	executable, err := os.Executable()
	if err != nil {
		fmt.Fprintln(stderr, "deck executable:", err)
		return 0
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		fmt.Fprintln(stderr, "deck executable:", err)
		return 0
	}
	client := tmux.Client{Socket: settings.Socket, Mouse: settings.TmuxMouse}
	// PRD R89/task 030: reclaim any interactive pipe a PRIOR deck process
	// leaked by dying somewhere SIGKILL cannot be caught -- disarms the
	// stale `pipe-pane`, restores the claimed window's geometry byte-exact
	// (SPEC §11.9) and releases ownership, then removes the leaked FIFO/temp
	// dir. Same best-effort shape as the tombstone sweep just below: a
	// failure here must never stop deck from starting, since this is a
	// backlog catch-up, not part of any promise made at open time.
	if _, err := tmux.ReclaimLeakedInteractivePipes(context.Background()); err != nil {
		fmt.Fprintln(stderr, "deck interactive pipe reclaim:", err)
	}
	registry := agent.NewRegistry()
	registry.Register(agent.NewShell())
	registry.Register(agent.NewClaude())
	registry.Register(agent.NewPi())
	registry.Register(agent.NewCodex())
	sessions := service.Service{
		Store: db, TMux: client, Audit: logger,
		Clock: settings.Clock, IDs: settings.IDs, Agents: registry,
		ConfigEnv: settings.Env, DeckExecutable: executable, DeckHome: settings.Paths.Home,
		GlobalPreLaunch:   settings.PreLaunch,
		GlobalPostDestroy: settings.PostDestroy,
		RecentCwdLimit:    settings.RecentCwdLimit,
	}
	// The TUI owns pane-text sampling. Its reconcile callback performs liveness
	// first and then probes stale eligible agents; the hidden hook command below
	// deliberately wires only ReconcileWithin and can therefore never probe. It
	// also enforces R62's event retention window (steer 3e-001 §6.4/§7) on every
	// tick, throttled internally by Store.EnforceEventRetention to at most once
	// an hour -- settings is captured once here, so a sort_order-shaped
	// restart-to-apply: a save changes config.toml immediately, but this already
	// running client keeps enforcing the OLD window until deck restarts.
	tuiReconcile := newTUIReconcile(db, sessions, settings)
	// Archive and Delete now also return SPEC §9.2's teardown-hook toast
	// message (task 013). WithTeardownHookReporters below wires that message
	// through to an on-screen note (task 042); the constructor's own
	// archiver/deleter params below still need the pre-task-013
	// `func(context.Context, store.Session) error` shape (see
	// footer_handler_agreement_test.go, which must keep passing unedited) so
	// that Submit's nil-check knows archiving/deleting is available at all --
	// these discard-adapters exist ONLY to satisfy that fallback signature.
	// Once WithTeardownHookReporters is set, the model's submit handlers call
	// exclusively the matching reporter and never these adapters, so
	// sessions.Archive/Delete each still run exactly once per submit.
	archiveAdapter := func(ctx context.Context, session store.Session) error {
		_, callErr := sessions.Archive(ctx, session)
		return callErr
	}
	deleteAdapter := func(ctx context.Context, session store.Session) error {
		_, callErr := sessions.Delete(ctx, session)
		return callErr
	}
	model := tui.NewWithShellCreatorAttacherKillerResumerProfileSwitcherResumeModerAgentCreatorRegistryPreviewCapturerEnvSetterRestarterInjectorDeleterRestorerReaperPurgerArchiverRenamerAndUnarchiver(db, settings, tui.TmuxHealth(settings), sessions.CreateShell, client.AttachCommand, sessions.Kill, tuiReconcile, sessions.Resume, sessions.SetPermissionProfile, sessions.ResumeMode, sessions.CreateAgent, registry, client.CapturePreview, sessions.SetSessionEnv, sessions.Restart, sessions.InjectEnv, deleteAdapter, sessions.Restore, sessions.Reap, sessions.Purge, archiveAdapter, sessions.Rename, sessions.Unarchive)
	// task 043: wire the message-carrying reporters themselves so the toast
	// added by task 042 actually shows -- sessions.Archive/Delete already have
	// the exact `func(context.Context, store.Session) (string, error)` shape
	// WithTeardownHookReporters wants, so they are passed directly (no
	// adapter, no discarding).
	model = model.WithTeardownHookReporters(sessions.Archive, sessions.Delete)
	// §11.9 interactive mode (task 061, PRD Part II onward) is the one Model
	// dependency that needs the raw tmux.Client itself rather than one more
	// narrow func field: window geometry, ownership and dispatcher/transport
	// construction all take a live Client value directly (see
	// internal/tui/interactive.go). Wiring it through WithTmuxClient rather
	// than growing the NewWith...And-chain above keeps that already-absurd
	// parameter list from growing a 22nd entry for a dependency of a
	// genuinely different shape.
	model = model.WithTmuxClient(client)
	// The `i` detail dialog's launch-inputs editor (task 023, SPEC §6.2/§11.4,
	// PRD R108) is wired the same way and for the same reason: one narrow
	// dependency, added without growing the positional chain above any
	// further. Without this line the dialog renders but every submit reports
	// "editing launch inputs is unavailable"; with it, Enter persists all four
	// editable launch inputs and marks the row launch_dirty, which only `R`
	// clears once a relaunch has actually carried them into a live pane.
	model = model.WithLaunchInputsSetter(sessions.SetLaunchInputs)
	// R130 part 2's `i`-dialog-only `g` move-group picker (SPEC §11) is
	// wired the same way and for the same reason: one narrow dependency,
	// added without growing the positional chain above any further.
	// Without this line the picker renders but every submit reports
	// "moving a session's group is unavailable"; with it, Enter persists
	// the chosen group_id (or clears it back to the structural default).
	model = model.WithGroupMover(sessions.SetSessionGroup)
	// Task 006/PRD R111-R112: wire task 002's shared availability probe
	// (sessions.AvailableKinds, itself backed by internal/service's
	// lookPathIn against the launch PATH) into task 005's seam, so the
	// create dialog's Agent field cycles only kinds actually on PATH
	// instead of the full registry. Every other caller of the seam
	// (registry_guard_test.go, TestBlackBoxRegistrySwapNeedsNoTUIEdit) still
	// injects its own prober in setup; this is the one production wiring.
	model = model.WithAvailableAgentKindsProber(sessions.AvailableKinds)
	programOptions := []tea.ProgramOption{tea.WithAltScreen()}
	// [ui] mouse / DECK_MOUSE (requirement 3) gates SGR mouse reporting for the
	// whole program lifetime; §11.8's hit-testing and gesture handling land in
	// later phase-2b1 tasks, but the on/off control itself is honoured here.
	if settings.Mouse {
		programOptions = append(programOptions, tea.WithMouseCellMotion())
	}
	programOptions = append(programOptions, rawByteCountingProgramOptions()...)
	// wrapForInteractiveShutdownOnPanic is deliberately the OUTERMOST wrap
	// (applied last, around everything else): its own recover must see a
	// panic thrown by any of the layers below it too, not only one from
	// inside the real tui.Model's own Update (see cmd/deck/interactive_shutdown.go).
	finalModel, runErr := tea.NewProgram(wrapForInteractiveShutdownOnPanic(wrapForInputCounting(wrapForDeliberateTestPanic(model))), programOptions...).Run()
	// PRD R89/task 031: SIGTERM's QuitMsg (Bubble Tea's own signal handler)
	// returns the model completely unchanged, without ever calling Update --
	// so unlike a panic (already handled inside the wrapper above, before
	// this point), a SIGTERM mid-interactive never reaches any recover at
	// all. finalModel is exactly what QuitMsg's own early return in
	// eventLoop handed back, so this is deck's only remaining chance to tear
	// an armed claim down before the process exits. A no-op whenever
	// interactive mode was not armed (ShutdownInteractive's own guard), so
	// this is safe to call unconditionally on every other exit route too.
	shutdownArmedInteractiveClaim(finalModel)
	if runErr != nil {
		fmt.Fprintln(stderr, "deck:", runErr)
		return 0
	}
	return 0
}

// preFrameTombstoneSweep is task 010's store-open call site, extracted so
// task 214's cmd/deck integration test can invoke this EXACT production
// pre-first-frame startup sweep rather than duplicating it or calling
// Store.SweepTombstones from a test-owned loop. It reaps at most one bounded
// batch before returning, so the work here cannot grow with the size of the
// backlog; newTUIReconcile's reconcile callback (the first tick after this
// call, per settings.Reconcile) then drains any remainder within the same
// open/startup cycle (review finding 3, R79). A sweep failure must not stop
// deck from starting -- this is a best-effort backlog catch-up, not part of
// any user-visible promise made at open time -- so it is reported and
// stepped over.
func preFrameTombstoneSweep(ctx context.Context, db *store.Store, settings config.Settings, stderr io.Writer) {
	if _, err := db.SweepTombstones(ctx, settings.DeleteGrace, settings.Clock.Now().UnixMilli()); err != nil {
		fmt.Fprintln(stderr, "deck tombstone sweep:", err)
	}
}

// newTUIReconcile builds the exact reconcile callback/helper cmd/deck wires
// into the TUI, extracted (task 214) so the same integration test can drive
// it directly instead of calling Store.DrainExpiredTombstones on its own.
// It performs liveness+probe reconciliation, then R62's throttled event
// retention (steer 3e-001 §6.4/§7), then task 010's tombstone sweep: Store.
// SweepTombstones throttles internally, so all but one call an hour is a
// single ui_state SELECT. DrainExpiredTombstones (review finding 3, R79)
// chains as many SweepTombstones batches as the store itself reports remain,
// so a backlog abandoned across a prior crash or long-dead process finishes
// draining on the FIRST tick after store open -- still the same open/startup
// cycle -- instead of one bounded batch per real hour; once caught up, the
// hourly throttle re-engages exactly as before for anything that arrives
// afterwards.
func newTUIReconcile(db *store.Store, sessions service.Service, settings config.Settings) func(context.Context) error {
	return func(ctx context.Context) error {
		if err := sessions.ReconcileWithProbes(ctx, settings.StaleAfter); err != nil {
			return err
		}
		if err := db.EnforceEventRetention(ctx, settings.EventRetentionDays, settings.Clock.Now().UnixMilli()); err != nil {
			return err
		}
		return db.DrainExpiredTombstones(ctx, settings.DeleteGrace, settings.Clock.Now().UnixMilli())
	}
}

// startClockStepTrigger makes SIGUSR1 the on-demand DECK_CLOCK_STEP trigger for
// a running client. AdvanceShared's file lock serializes signals handled by
// different clients that share DECK_HOME, and clock.now publishes the result to
// every already-running client and later deck subprocess. No signal is claimed
// unless both DECK_CLOCK and DECK_CLOCK_STEP configure a step-capable clock.
func startClockStepTrigger(clock *config.Clock, stderr io.Writer) func() {
	if !clock.StepEnabled() {
		return func() {}
	}
	requests := make(chan os.Signal, 16)
	done := make(chan struct{})
	signal.Notify(requests, syscall.SIGUSR1)
	go func() {
		for {
			select {
			case <-requests:
				if _, err := clock.AdvanceShared(); err != nil {
					fmt.Fprintln(stderr, "deck clock step:", err)
				}
			case <-done:
				return
			}
		}
	}()
	return func() {
		signal.Stop(requests)
		close(done)
	}
}

// runHook is intentionally selected before opening the normal application
// store or constructing a tmux client. A late hook must not recreate deleted
// state or bootstrap a tmux server.
func runHook(ctx context.Context, settings config.Settings, stdin io.Reader) error {
	var raw json.RawMessage
	decoder := json.NewDecoder(stdin)
	if err := decoder.Decode(&raw); err != nil {
		return fmt.Errorf("read one JSON object: %w", err)
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return errors.New("stdin must contain one JSON object")
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("stdin contains more than one JSON value")
		}
		return fmt.Errorf("reject trailing stdin: %w", err)
	}

	if info, err := os.Stat(settings.Paths.StateDB); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("state database does not exist: %s", settings.Paths.StateDB)
		}
		return fmt.Errorf("inspect state database: %w", err)
	} else if !info.Mode().IsRegular() {
		return fmt.Errorf("state database is not a regular file: %s", settings.Paths.StateDB)
	}
	db, err := store.Open(settings.Paths)
	if err != nil {
		return fmt.Errorf("open existing state database: %w", err)
	}
	defer db.Close()
	logger, err := audit.New(settings.Paths, settings.Clock)
	if err != nil {
		return fmt.Errorf("open audit log: %w", err)
	}
	timed := timedHookStore{Store: db, Audit: logger}
	// The pane's own launch generation travels with the row identity so the
	// receiver can tell a hook from the current launch apart from a late hook
	// from a launch deck has already replaced (issue #11, R74). Absent when
	// this pane's launch took no lease, which Receive reads as "no token".
	result, err := hookrecv.Receive(ctx, timed, trimmed, os.Getenv("DECK_SESSION_ID"), os.Getenv(agent.LaunchGenerationEnv), settings.Clock.Now().UnixMilli())
	if err != nil {
		return err
	}
	// SessionEnd shares the agent's tight shutdown budget and is deliberately
	// one store write and exit. Every other hook pays for one bounded liveness
	// pass after (and therefore outside) the measured store callback. This is
	// the unattended path that notices and collects a different crashed pane;
	// ReconcileWithin is liveness-only and never runs pane-text probes.
	if result.Kind == "session_end" {
		return nil
	}
	liveness := service.Service{
		Store: db,
		TMux:  tmux.Client{Socket: settings.Socket},
		Audit: logger,
		Clock: settings.Clock,
	}
	if err := liveness.ReconcileWithin(ctx, settings.Reconcile); err != nil {
		return fmt.Errorf("post-hook liveness pass: %w", err)
	}
	return nil
}

// timedHookStore leaves resolution reads outside the measured span and wraps
// exactly the receiver's single durable mutation (resolved or orphan).
type timedHookStore struct {
	*store.Store
	Audit *audit.Logger
}

func (s timedHookStore) UpdateSessionStatus(ctx context.Context, input store.StatusUpdateInput) error {
	return s.Audit.HookStoreWrite(input.SessionID, func() error {
		return s.Store.UpdateSessionStatus(ctx, input)
	})
}

func (s timedHookStore) RecordOrphanEvent(ctx context.Context, input store.EventInput) error {
	return s.Audit.HookStoreWrite("", func() error {
		return s.Store.RecordOrphanEvent(ctx, input)
	})
}
