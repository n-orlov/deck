# Task 035 — stop the no-live-pane return from spending previewFit's one coalesced attempt

Fixes the bug task 034 derived counts against (`docs/reports/phase3g-034-previewfit-derivation/README.md`):
`previewFit`'s closure (`internal/tui/tui.go`) reports the identical
`previewFitDone{sessionID}` message from all three of its return paths, and
the handler (`case previewFitDone` in `Model.Update`) latched
`previewFitSessionID = msg.sessionID` unconditionally on every one of them —
including the no-live-pane path, which never touched the window at all. Once
latched, `previewFit`'s own scheduling guard
(`session.ID == m.previewFitSessionID`) refuses every future fit for that
session ID for the rest of the model's lifetime, even after the pane later
becomes live again while the row stays (or is re-)selected.

## The fix

`internal/tui/tui.go`:
- `previewFitDone` gained a `noLivePane bool` field. Zero value is `false`
  (`noLivePane` unset), deliberately chosen so every pre-existing
  `previewFitDone{sessionID: "..."}` literal in `preview_fit_overlap_test.go`
  keeps its old "settled" behaviour unchanged.
- The `PreviewPane` `err != nil || !ok` return (no live pane: stopped,
  starting, archived, or reaped between `List` and capture) now returns
  `previewFitDone{sessionID: sessionID, noLivePane: true}`.
- The `tmux.SessionName` failure path and the real `FitWindowToPane` path are
  both **unchanged** — out of this task's scope, matching task 034's
  derivation, which only ever discusses the no-live-pane branch.
- The `previewFitDone` handler now only latches when the return was NOT a
  no-live-pane skip: `if !msg.noLivePane { m.previewFitSessionID = msg.sessionID }`.
  `previewFitInFlight` is still cleared unconditionally either way, per the
  existing comment there.

Diff: [`tui.go.diff`](tui.go.diff).

## Count check against task 034's pre-derived prediction

Task 034 enumerated all 7 exact-count assertions in the suite and predicted
**zero deltas** (none of them exercise the no-live-pane branch). Re-running
both files after the fix:

```
$ ci/run.sh env DECK_GODOG_PATHS=preview.feature,interactive_sigwinch_budget.feature go test ./features/ -run TestFeatures -count=1 -v
...
15 scenarios (15 passed)
152 steps (152 passed)
```

Full log: [`features-preview-sigwinch-green.log`](features-preview-sigwinch-green.log).
Every one of the 7 counts task 034 listed is exactly what it predicted (no
mismatch to explain):

| # | File:line | Asserted today (034) | Predicted post-fix (034) | Observed post-fix |
|---|---|---|---|---|
| 1 | `interactive_sigwinch_budget.feature:33` | 2 | 2 (unchanged) | 2 |
| 2 | `interactive_sigwinch_budget.feature:46` | 2 | 2 (unchanged) | 2 |
| 3 | `preview.feature:95` | 1 | 1 (unchanged) | 1 |
| 4 | `preview.feature:107` | 1 | 1 (unchanged) | 1 |
| 5 | `preview.feature:130` | 0 | 0 (unchanged) | 0 |
| 6 | `preview.feature:171` | 0 | 0 (unchanged) | 0 |
| 7 | `preview.feature:173` | 1 | 1 (unchanged) | 1 |

No scenario in either file was deleted, skipped, or tagged out to get this
result — `git diff --stat features/` for this task is empty; only
`internal/tui/tui.go` changed.

## `internal/tui` unit suite

```
$ ci/run.sh go test -count=1 ./internal/tui/
ok  	github.com/n-orlov/deck/internal/tui	1.023s
```
Full `-v` log: [`internal-tui-tests.log`](internal-tui-tests.log). Includes
the three pre-existing R63 tests in `preview_fit_overlap_test.go`
(`TestPreviewFitDoesNotOverlapItself`,
`TestPreviewFitResumesAfterItsDoneLands`,
`TestPreviewFitDoneForUnselectedSessionClearsTheMarker`), all unaffected
because none of their `previewFitDone{sessionID: ...}` literals set
`noLivePane`.

## Demonstrating the fit-floor `has never been taller than` assertion still fails before the count it protects

`features/preview.feature:170`'s `Then deck client "solo" has never been
taller than 9 rows` sits immediately before the `exactly 0 SIGWINCH` count
assertion it exists to protect (task 028, finding F1: a taller frame would
have licensed the very fit that assertion checks never happened, turning it
into a race rather than a deterministic check). To prove this ordering is
still load-bearing after this task's fix — not merely unexercised — a
temporary, throwaway edit inserted a genuine height violation into the
scenario's own steps (a `resized to 100x20` step right after the client
starts at `100x9`, **never committed** — reverted immediately after this run,
confirmed by `git diff features/preview.feature` coming back empty):

```
$ ci/run.sh env DECK_GODOG_PATHS=preview.feature go test ./features/ -run TestFeatures -count=1 -v
...
    Then deck client "solo" has never been taller than 9 rows
    after scenario hook failed: deck client "solo" has been 20 rows tall at
    some point, want never more than 9: ... (task 028, finding F1)
[... surviving-client goroutine dump elided; not relevant to this demonstration ...]
--- FAIL: TestFeatures/a_fit_is_skipped_below_the_7-inner-row_floor,...
```

Full trimmed log: [`preview-floor-assertion-forced-red.log`](preview-floor-assertion-forced-red.log)
(the huge killed-client goroutine dump godog prints on a scenario failure was
elided as noise; the scenario steps and the failing assertion itself are kept
verbatim). The `has never been taller than 9 rows` step is the one reported
red; the two `SIGWINCH` count steps after it in the scenario are never
reached (godog stops the scenario at the first failing step) — proving the
height assertion catches a floor violation before the count assertion would
have to, exactly as task 028 designed it to. This scratch edit was made only
in the working tree, run once, and reverted before any other work in this
task; it is not part of the committed diff.

## What this task deliberately did not change

- The `tmux.SessionName` failure return path — same class of bug (nothing
  resized, yet it still latches), but outside "the no-live-pane early
  return" this task's title and task 034's derivation name. Left as a
  candidate for a follow-up task, not folded in here.
- The two scenarios task 034 flagged as a coverage gap
  (`preview.feature:239` "doomed" crash tail, `preview.feature:251`
  "retiring" stopped session) — neither asserts a SIGWINCH count and neither
  reselects the session after its pane becomes live again, so neither one is
  evidence for or against this fix. Adding a regression case that actually
  exercises "pane goes live again after a no-live-pane skip, and the session
  stays/gets reselected" is out of this task's scope per 034's own note.
