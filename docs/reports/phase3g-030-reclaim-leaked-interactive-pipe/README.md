# Task 030 evidence: reclaim a leaked interactive pipe at the next start (R89)

SIGKILL cannot be handled at all, so a deck client killed mid-interactive
always leaves its armed `pipe-pane`, its FIFO temp dir and its window
ownership claim behind. The guarantee this task implements is "the next
deck start reclaims it", never "the exit cleans it".

## What was reverted to produce "red"

The fix lives in `internal/tmux/reclaim.go`'s `reclaimOne`, called from
`tmux.ReclaimLeakedInteractivePipes` at every `cmd/deck` start
(`cmd/deck/main.go`). To reproduce "red" without breaking compilation of
the tests that already depend on this file's own new types
(`InteractiveClaimRecord`, `SaveInteractiveClaimRecord`, `PanePipe.TempDir`,
etc.), `reclaimOne` was temporarily given an unconditional
`return false, nil` as its very first statement -- i.e. "reclaim never
actually finds anything to reclaim", which is exactly the pre-fix
behaviour (the function/scan did not exist at all before this task; a
permanent no-op is behaviourally identical for these tests' purposes).
Reverted immediately after capturing the logs below; `git diff
internal/tmux/reclaim.go` against the committed tree is empty.

## Red (before the fix)

- `red-before-fix-unit.log` --
  `ci/run.sh go test -count=1 ./internal/tmux/ -run TestReclaimLeakedInteractivePipesDisarmsRestoresAndRemovesAStaleDeadOwnerClaim -v`
  fails: `reclaimed = [], want exactly [...]`.
- `red-before-fix-feature.log` (trimmed; see the note inline for what was
  cut) --
  `ci/run.sh env DECK_GODOG_PATHS=interactive_pipe_leak.feature go test ./features/ -run TestFeatures -count=1 -v`
  fails at `Then tmux pane pipe is not armed for session "leaky"`:
  `#{pane_pipe} still reads 1 for "deck_leaky", want 0 (disarmed)` -- i.e.
  the leak from the SIGKILL is real and still present after the next
  client starts, proving both the positive control (the leak exists) and
  that an unpatched next-start does not touch it.

## Green (after the fix)

- `green-after-fix-unit.log` -- the same unit test passes.
- `green-after-fix-feature.log` -- the same feature scenario passes end to
  end: pipe-pane disarmed, temp dir/FIFO gone, `@deck_isize_owner` unset,
  and the window's geometry unchanged (`still matches "before-interactive"`),
  all observed from a second client that never itself attached to the
  reclaimed window.

## Required commands (final state, fix applied)

- `ci/run.sh go test -count=1 ./internal/tmux/ ./internal/interactive/ ./cmd/deck/`
  -- green (`criterion-packages-run1.log` + `criterion-interactive-rerun.log`).
  Two failures seen on the first run of this command were both stale test
  drivers predating this task, fixed here because they blocked the criterion:
  - `cmd/deck` `TestDeckBinaryEmptyHelpAndQuitThroughPTY` asserted the
    empty-list footer literal `up/down - n new - ? help - q quit`, stale since
    task 014 (`7dbe5c5`) put `,` settings into the fixed set (SPEC.md:1307).
  - `cmd/deck` `TestAbandonedDDIsReapedAtNextStoreOpen` still drove the create
    modal's field navigation with tab, stale since task 025 moved it to
    down-arrow: the cwd was typed into the Name field, so the session was
    created under a long path-shaped name and the delete confirm's
    "press d again to confirm" wrapped, timing the wait out. Now sends the
    name, settles 75ms, then `\x1b[B` + cwd, the way features/*_test.go do.
  - `internal/interactive`'s `TestSessionRendersAreCoalescedAgainstAKnownByte...`
    failed once under three-package parallel load (5s pipe-pane fifo connect
    timeout) and passes on its own re-run -- a pre-existing timing flake, not
    touched.
- `ci/run.sh env DECK_GODOG_PATHS=<the nine interactive_*.feature files>,no_leak_scan.feature go test ./features/ -run TestFeatures -count=1`
  -- green (`criterion-feature-files.log`).
