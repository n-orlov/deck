# Task 403: replace the three fixed-duration waits in the R54 scenarios with observable-consequence settles

`grep -n 'milliseconds pass' features/mouse.feature` now returns nothing (verified below). All three
sites were in `features/mouse.feature`'s `@requirement-54-sidebar-click-on-interactive-row-is-a-no-op`
scenario alone — the other two `@requirement-54-*` scenarios never had one.

## Per-site consequence and replacement

| # | Old line (mouse.feature, pre-fix) | What it stood in for | Replacement |
|---|---|---|---|
| 1 | `And 200 milliseconds pass` after `enters interactive mode`, before `Then ... preview top border contains "retarget-noop-a"` | The render triggered by entering interactive mode (border text change) landing before the read. | `clientPreviewTopBorderContains` (`features/interactive_retarget_test.go`) now polls via new `ScreenDriver.WaitForFrameFunc` (`features/pty_driver_test.go`) instead of reading `client.Frame(false)` exactly once — it retries against the same `d.updated` channel `WaitForFrame`/`WaitForFrameGone` already use until `previewTopBorderText(frame)` contains the wanted text, bounded by the same `defaultWaitDeadline`. |
| 2 | `And 200 milliseconds pass` after the retarget click, before `Then ... has session "retarget-noop-b" selected` | The render triggered by the retarget click (selection + border change) landing before the read. | Deleted outright — the very next step, `clientHasSessionSelected` (`features/attention_sort_test.go`), **already polls** `WaitForFrame` for the `"> "+name` marker (unrelated to this task; pre-existing). That existing polling Then step is the "step that polls the specific observable consequence" for this site; no new code was needed once the redundant explicit wait was removed. `clientPreviewTopBorderContains`'s own new poll (site 1's fix) covers the following `And ... preview top border contains` line too. |
| 3 | `And 200 milliseconds pass` after the SECOND (no-op) click, before `Then ... has session "retarget-noop-b" selected` / `... still matches ...` | Time for the no-op click to be read and `Update`d by deck before reading the ownership claim. **This site has no content-level observable at all**: `standard_renderer.flush` (`charmbracelet/bubbletea@v1.3.10/standard_renderer.go:165`, confirmed by reading the vendored source in the `ci` sibling) returns before writing *any* bytes when the rendered buffer is byte-identical to the last render, so a genuine no-op produces zero pty output and therefore never fires `d.updated` — there is nothing for `WaitForFrame`/`WaitForFrameFunc` to poll for. `features/mouse_synthesis_test.go`'s pre-existing `clientFrameStillMatchesCaptured` documents the identical defect class ("a mouse report deck does not react to produces no output at all... success is the absence of a new frame"). | `privateTMuxWindowOwnershipClaimForSessionStillMatches` (`features/interactive_retarget_test.go`) no longer reads `tmux show-options` once after a fixed sleep; it now **polls the ownership option across a 200ms window** (`ownershipStillMatchesSettleWindow`, 20ms interval), failing the instant the value ever changes rather than only at one arbitrarily-timed sample point. This is strictly more sensitive than the old fixed-sleep-then-single-read shape (a late leave+re-enter landing near the end of the window is still caught) and removes the literal `milliseconds pass` feature step. It is the same idiom `clientFrameStillMatchesCaptured` already uses for this exact class of assertion, adapted to poll the *specific value under test* across the window instead of the deck frame (there is nothing frame-side to poll here). |

## Guard test

`features/mouse_feature_no_sleep_guard_test.go`'s `TestMouseFeatureNeverRegainsMillisecondsPass` scans
`features/mouse.feature`'s source text for `"milliseconds pass"` and fails if it ever reappears.

- `guard-test-red.log`: temporarily re-inserted `And 200 milliseconds pass` after line 125 (`enters
  interactive mode`) — FAILs, naming `mouse.feature:126` and quoting the offending line.
- `guard-test-green.log`: reverted (`git checkout -- features/mouse.feature`, then the real fix
  re-applied) — PASSes.

`millisecondsPass` (`features/kill_delete_undo_test.go`) stays available and unmodified — its
legitimate timeout-expiry users are:

- `features/kill_delete_undo.feature` (many scenarios): the undo toast's real expiry, scheduled via
  `tea.Tick` against `DECK_UNDO_MS`, never against `m.settings.Clock` — a scenario proving the window
  has expired must wait it out for real (see `millisecondsPass`'s own doc comment).
- `features/interactive_sigwinch_budget.feature` (both scenarios): pacing around the fake fixture's
  own SIGWINCH counter, whose channel buffer is exactly 1 (Go's `os/signal` contract) — a real
  wall-clock pause is required to avoid coalescing two real kernel SIGWINCH into one count (see the
  feature file's own header comment).
- `features/preview.feature:74`: settling a selection's own fit before taking the "before" baseline
  snapshot the rest of that scenario compares against.

None of these three files were touched by this task.

## Proof

- `clean-tag-alone-3x/r54-noop-run-{1,2,3}.log`: `ci/run.sh sh -c 'DECK_GODOG_TAGS=@requirement-54-sidebar-click-on-interactive-row-is-a-no-op go test -count=1 -run TestFeatures -v ./features/'` — 3 consecutive green runs (1.38-1.42s each), loadavg 10.6-13.3 (this sibling image shares the host with the rest of this run's contention).
- `mutation.diff` (401's own guard-deletion mutant, reapplied and re-verified here): `if !m.interactive || index == m.selected` → `if !m.interactive`.
- `mutant-tag-alone-red.log`: same tag-alone run under the mutant — FAILs. The failure is at
  `mouse.feature:133`'s `still matches` assertion (not a precondition step), and the concrete error is
  `tmux ... show-options ... @deck_isize_owner: exit status 1: invalid option: @deck_isize_owner` —
  the mutant's `exitInteractive` call *unsets* the window option while releasing ownership, and the
  poll caught that transient unset state directly, which is an even more direct proof of the
  leave-then-re-enter than a value mismatch would have been.
- `mutant-reverted-clean-tag-alone-green.log`: mutation reverted (`git checkout -- internal/tui/mouse.go`,
  `git diff` empty), same tag-alone run — green again.
- `retarget-tag-alone-3x/r54-retarget-run-{1,2,3}.log`: the sibling
  `@requirement-54-sidebar-click-retargets-interactive-mode` scenario (no waits, never had any) run tag-alone
  3 consecutive times — confirms the new `clientPreviewTopBorderContains` poll (site 1's fix, which this
  scenario also exercises) didn't regress it and isn't flaky either. Both `@requirement-54-*` tags now each
  have 3-consecutive-green tag-alone evidence, per the criterion.
- `internal-tui-package.log`: `ci/run.sh go test -count=1 ./internal/tui/` — `ok`.

## Verification

- `ci/run.sh go build ./...`, `ci/run.sh go vet ./...`, `ci/run.sh gofmt -l features/ internal/ cmd/`: all
  clean (the three pre-existing `.spike-preview/...` gofmt hits are untouched by this task and predate
  it).
- `grep -n 'milliseconds pass' features/mouse.feature`: no output.
- `git status --short` at the end of this task: only this report directory,
  `features/mouse_feature_no_sleep_guard_test.go`,
  `features/interactive_retarget_test.go`, `features/mouse.feature` and `features/pty_driver_test.go`
  changed/added — no scenario deleted, skipped or retagged, no product code touched.
