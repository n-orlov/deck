# Task 034 -- pre-derived SIGWINCH/resize counts for the `previewFit` no-live-pane fix

Written and committed **before** any change to `internal/tui/tui.go`'s
`previewFit`, at HEAD `ca43907`. Every number below comes from reading
`internal/tui/tui.go`, `internal/tmux/tmux.go` and the two feature files that
assert exact SIGWINCH/resize counts -- no test was run to obtain any of them,
per the standing rule "SIGWINCH/resize counts may be re-baselined only against
a derivation written and committed before the code change, never read off a
failure." Task 035 checks its own post-fix counts against this document, not
the other way around; any mismatch belongs in 035's own evidence dir, not
folded back into this file.

## The bug, in the code's own terms

`previewFit` (`internal/tui/tui.go:1471`) schedules at most one outstanding
fit at a time, guarded by two fields:

- `previewFitInFlight` (set at line 1498, cleared unconditionally at line
  2202 when any `previewFitDone` lands) -- "a fit for *some* session is
  currently running".
- `previewFitSessionID` (written only at line 2193, from `previewFitDone`)
  -- "this session ID has already settled and been fit (or found not
  applicable)"; line 1482's guard (`session.ID == m.previewFitSessionID`)
  refuses to schedule anything further for that ID once set, and nothing in
  the file ever clears or overwrites it back for the *same* ID (confirmed:
  `previewFitSessionID =` appears exactly once in the whole file, at line
  2193).

The closure `previewFit` hands to the event loop has three return paths,
all of which report the identical message, `previewFitDone{sessionID:
sessionID}`:

- line 1508-1509: `PreviewPane` returned `err != nil || !ok` -- **no live
  pane** (stopped, starting-with-no-pane-yet, archived, or reaped between
  `List` and capture; see `internal/tmux/tmux.go:444`'s own doc comment).
  Nothing was resized.
- line 1511-1512: `tmux.SessionName` failed. Nothing was resized.
- line 1520: the real path -- `FitWindowToPane` was actually called (a
  no-op resize, i.e. 0 further SIGWINCH, if the pane already matches, per
  that call's own doc comment two lines above).

Because the outer handler at line 2193 does not distinguish these three
cases, a **no-live-pane skip is recorded exactly like a real, successful
fit**: `previewFitSessionID` is set to that session's ID either way. The
guard at line 1482 then refuses every future fit attempt for that same
session ID for the rest of the model's lifetime -- including a legitimate
one after the pane later becomes live (session resumed, agent restarted)
while the row stays selected or is reselected. That is "the bounded early
return" this task's title names, and task 035's fix is expected to stop the
no-live-pane closure path from writing `previewFitSessionID`, so a later
live pane is still eligible for its own coalesced fit.

## Every scenario asserting an exact SIGWINCH/resize count today

Exhaustive: `grep -rn "received exactly\|has never been taller than"
features/*.feature` (run as a read-only search, not a test) returns exactly
these lines, in exactly these two files:

| # | File:line | Scenario | Count asserted today | Path previewFit takes here |
|---|---|---|---|---|
| 1 | `interactive_sigwinch_budget.feature:33` | "a full enter/exit cycle costs exactly two SIGWINCH with nobody attached" | 2 | **None.** This scenario drives `internal/tmux`'s `CaptureWindowGeometry`/`FitWindowToPane`/`RestoreWindowGeometry` directly against a bare tmux session (`features/interactive_sigwinch_budget_test.go`'s `deckEntersInteractiveModeFittingTo`/`deckExitsInteractiveModeOnTmuxSession`); it never starts a deck `Model` at all, so `previewFit` never runs. |
| 2 | `interactive_sigwinch_budget.feature:46` | "...with a client attached throughout" | 2 | Same as #1 -- no `Model`, no `previewFit`. |
| 3 | `preview.feature:95` | "a settled selection fits the session's window..." (first assertion) | 1 | `session.ID` ("beacon") is selected only *after* the scenario waits for `screen contains "running"`, so the tmux window already exists and the pane is live; `previewFit` takes the real-fit path (line 1520), never the no-live-pane path. |
| 4 | `preview.feature:107` | same scenario, second assertion (repeated revisits) | 1 | Revisits of the same already-settled, still-live session; guarded by line 1482's `session.ID == m.previewFitSessionID` (now genuinely "already fitted"), never re-enters the closure at all. |
| 5 | `preview.feature:130` | "preview_fit = false restores the wholly passive preview..." | 0 | `Given the deck config disables preview fit` sets `m.settings.PreviewFit = false`; `previewFit` returns `nil` at its very first line (`!m.settings.PreviewFit`), before ever reaching `PreviewPane`. The closure never runs, so the no-live-pane branch is never reached either. |
| 6 | `preview.feature:171` | "a fit is skipped below the 7-inner-row floor..." (first assertion) | 0 | `solo` is born at 100x9; `m.previewContentSize()` is below `interactiveMinInnerRows`, so line 1486's floor guard returns `nil` *before* `previewFitInFlight` is ever set and *before* the closure (hence `PreviewPane`) ever runs. This is a different early return from the one this task is about. |
| 7 | `preview.feature:173` | same scenario, after resizing to 100x30 | 1 | Above the floor now; `beacon` is a claude session created by "maker" and never killed/stopped, so its tmux pane has stayed live the whole scenario. `previewFit` takes the real-fit path (line 1520), never the no-live-pane path. |

## Predicted post-fix counts

**All seven counts above are predicted to stay exactly as they are today.**
None of the seven exercises `previewFit`'s no-live-pane closure branch
(lines 1502-1509) at all, for the reasons in the table's rightmost column:
two never construct a `Model` (interactive_sigwinch_budget.feature, rows
1-2), three are blocked from ever reaching that branch by an earlier,
different guard (`PreviewFit` off, or the floor check -- rows 5-6), and the
remaining two involve a session whose pane is live for the scenario's
entire lifetime (rows 3-4, 7), so every closure invocation they do trigger
takes the real-fit path, not the no-live-pane one. Fixing the no-live-pane
branch to stop writing `previewFitSessionID` therefore changes nothing these
seven assertions observe -- task 035's own count check against this
document should find zero deltas, not a coincidental zero it has to explain
away.

## Coverage gap this derivation surfaces (informational, not a prediction)

Two other scenarios in `preview.feature` *do* reach the no-live-pane closure
branch under today's code (`PreviewFit` left at its true default, a session
selected with no live pane), but neither asserts any SIGWINCH/resize count,
so there is no number for the fix to change:

- `preview.feature:239` ("an error row's preview shows the durable crash
  tail...") -- session "doomed" is SIGKILLed; its pane is dead/reaped by the
  time it is captured. Assertions are all on screen text.
- `preview.feature:251` ("a stopped session's preview names its own
  state...") -- session "retiring" exits with status zero and is never
  resumed within the scenario. Assertions are all on screen text.

Neither scenario keeps the session selected across a later transition back
to a live pane (the one condition that actually exposes the bug this task
describes), so neither one is evidence either for or against the fix --
they simply have nothing in this document to predict. That absence is a
gap task 035 (or a follow-up) may want to close with a new
regression case; it is out of this task's own scope to add one.

## Method

Static reading only, at HEAD `ca43907`:
- `internal/tui/tui.go` (`previewFit`, its `previewFitDone` handler, and
  every assignment to `previewFitSessionID`/`previewFitInFlight`).
- `internal/tmux/tmux.go` (`PreviewPane`'s liveness contract).
- `features/preview.feature` and `features/interactive_sigwinch_budget.feature`
  in full, plus `features/interactive_sigwinch_budget_test.go` to confirm the
  latter file never constructs a `tui.Model`.
- `grep -rn "received exactly\|has never been taller than" features/*.feature`
  to enumerate every exact-count assertion in the suite exhaustively (no
  other feature file matched).

No `go test`, `ci/run.sh`, or godog run of any kind was used to produce or
check any number in this document.
