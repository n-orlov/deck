# Task 031: clean up the interactive pipe on SIGTERM and on panic (PRD R89)

## What was missing

Task 030 gave deck the "next start reclaims it" half of R89 (SIGKILL cannot
be handled at all). SIGTERM and a panic both still run Go code in this same
process before it exits, so R89 also names a stronger guarantee for them:
"the exit itself cleans it up", proven directly against the process that was
interactive, never a later rescuer.

Two gaps, both in `cmd/deck`, not in `internal/tui`'s own `exitInteractive`
(Ctrl+Q's teardown, unchanged and already correct):

- **SIGTERM**: Bubble Tea's own signal handler (`tea.Program.handleSignals`)
  turns SIGTERM into a `QuitMsg`, and `eventLoop`'s own `case QuitMsg: return
  model, nil` returns straight through **without ever calling `Update`
  again**. `exitInteractive` never runs.
- **Panic**: Bubble Tea's own top-level recover (`Program.Run`) restores the
  terminal and reports the panic, but discards the model outright -- its
  named return `returnModel` never gets assigned once the panic unwinds past
  `model, err := p.eventLoop(...)`.

## The fix

- `internal/tui/interactive.go`: factored `exitInteractive`'s own
  disarm/restore/release sequence into `teardownInteractive`, and exported it
  as `Model.ShutdownInteractive(ctx)` -- a no-op unless interactive mode is
  armed, exactly like `exitInteractive`'s own guard.
- `cmd/deck/interactive_shutdown.go` (new, always-on, no build tag):
  `interactiveShutdownGuard` wraps the OUTERMOST layer of every wrap
  `main.go` composes onto the real model, so its own `recover` sees a panic
  from any of them (including `testpanic_hook.go`'s deliberate-panic hook,
  which panics *before* delegating to the model it wraps). It unwraps down
  to whichever wrapped value implements `ShutdownInteractive` via a small
  `Unwrap() tea.Model` interface, so it works uniformly regardless of how
  many test-only wrappers (`decktestpanic`, `deckinputcount`) are present.
- `cmd/deck/main.go`: after `tea.Program.Run()` returns, calls the same
  `shutdownArmedInteractiveClaim` helper on the **returned** model -- this is
  the only remaining chance for the SIGTERM route, since QuitMsg never lets
  a panic-style recover fire at all.
- `testpanic_hook.go` / `inputcount_hook.go`: one-line `Unwrap()` method each
  so the guard above can see through them.

## Evidence (revert-and-reproduce)

Both new tests fail red against the pre-fix tree (production files reverted,
test file kept) and pass green against the fix:

- `red.log` -- `cmd/deck/main.go`, `internal/tui/interactive.go`,
  `testpanic_hook.go`, `inputcount_hook.go` reverted to HEAD
  (`8ddf869`, task 030's own commit) and `interactive_shutdown.go` removed;
  both `TestDeckBinarySIGTERMMidInteractiveDisarmsPipeAndRestoresWindow` and
  `TestDeckBinaryPanicMidInteractiveDisarmsPipeAndRestoresWindow` fail with
  "pipe-pane is still armed ... after deck exited, want disarmed".
- `green.log` -- same two tests, fix restored: both pass.
- `full-target-suite.log` -- the task's own required command,
  `ci/run.sh go test -count=1 ./internal/tmux/ ./cmd/deck/`, green.
- `no_leak_scan.log` -- `no_leak_scan.feature` (unrelated masking scan,
  named explicitly in the task's own success criteria as a regression
  check), green, 2/2 scenarios.

## Success criteria checklist

- [x] SIGTERM mid-interactive disarms the pipe-pane, removes the FIFO/temp
      dir and restores window ownership/geometry byte-exact before exit
      (`TestDeckBinarySIGTERMMidInteractiveDisarmsPipeAndRestoresWindow`).
- [x] A panic on the same path does the same
      (`TestDeckBinaryPanicMidInteractiveDisarmsPipeAndRestoresWindow`, reusing
      the existing `DECK_TEST_PANIC_KEY`/`decktestpanic` mechanism
      `features/mouse_exit_paths_test.go` already relies on for R36).
- [x] Separate tests per exit route, each asserting no armed pipe-pane and no
      `/tmp/deck-interactive-pipe-*` remains.
- [x] `ci/run.sh go test -count=1 ./internal/tmux/ ./cmd/deck/` green.
- [x] `no_leak_scan.feature` green.
