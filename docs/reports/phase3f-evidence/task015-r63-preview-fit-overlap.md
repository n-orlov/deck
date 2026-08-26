# Task 015 / R63 — a passive preview fit can never overlap itself

## The defect

`previewFitSessionID` (tui.go) was the only coalescer for steer-018's passive
preview fit, and it is written **only when an attempt completes**
(`previewFitDone`). `previewTick` fires every `DECK_PREVIEW_MS` (300ms by
default) regardless of what is still running, so two ticks that both land
inside one `PreviewPane` + `FitWindowToPane` tmux round trip both saw the
selection as unsettled and both scheduled a fit **for the same session**: two
`resize-window` calls against one window, hence an extra SIGWINCH the
scenarios that count them (`features/preview.feature`'s "exactly 1") never
asked for.

## The fix (commit: see `git log --oneline -1` for this task)

- New model field `previewFitInFlight` (tui.go, documented beside
  `previewFitSessionID`): the session ID of the ONE outstanding passive fit,
  set at **scheduling** time inside `previewFit`, cleared when
  `previewFitDone` lands.
- `previewFit`'s guard now consults both halves:
  `if session.ID == m.previewFitSessionID || m.previewFitInFlight != ""`.
  While a fit is in flight nothing is scheduled for any session, so at most
  one passive fit is ever outstanding.
- `previewFit`'s receiver became a pointer (`func (m *Model) previewFit()`)
  because scheduling is itself a state change; its single caller is the
  `previewTick` case, whose `m` is addressable.
- A comment at the marker records that **every early return inside the
  closure owes a `previewFitDone` for the same sessionID** — that message is
  the only thing that clears the marker. All four return paths in the closure
  do deliver it.
- `previewFitDone` clears the marker **unconditionally** (comment explains):
  the reported session may no longer be selected, and an unconditional clear
  cannot wedge the mechanism.
- `exitInteractive` deliberately does NOT clear `previewFitInFlight` (comment
  added in interactive.go): its clearing of `previewFitSessionID` must not
  license a second, overlapping fit for a session whose first attempt is
  still outstanding.

## Tests (`internal/tui/preview_fit_overlap_test.go`)

Three tests driving `Model.Update` directly — no pty, no tmux, no socket
dialled — each asserting on the **tea.Cmd Update returned** (the fit closure
is never run). `previewCapture` is left nil, so the only commands a
`previewTick` can batch are the unconditional reschedule and the passive fit;
`previewTickCmds` flattens the result (tea.Batch collapses a single command,
so a no-fit tick returns the bare reschedule, which is verified to produce a
`previewTick` message rather than a `tea.BatchMsg`).

- (a) `TestPreviewFitDoesNotOverlapItself` — two `previewTick`s with no
  intervening `previewFitDone`: first returns 2 commands, second returns 1.
- (b) `TestPreviewFitResumesAfterItsDoneLands` — tick, `previewFitDone{s1}`,
  selection moves to s2, tick: 2 commands again, marker now `s2`.
- (c) `TestPreviewFitDoneForUnselectedSessionClearsTheMarker` — the fit for s1
  reports after the user navigated to s2: the marker clears and the next tick
  fits s2 (the wedge case).

## Evidence

Host load recorded with every run (`/proc/loadavg` printed in the transcript;
2.0-4.9 across these runs).

- `ci/run.sh go test -count=1 ./internal/tui/` → `ok ... 0.844s`
  (`task015-r63-tui-package.log`). Verbose confirmation that all three new
  tests ran and passed is in the iteration transcript.
- **Revert-and-reproduce** (`task015-r63-reverted-guard.log`): the product
  file was copied to /tmp and the guard mutated in place back to the plausible
  naive form `if session.ID == m.previewFitSessionID {` (marker and clearing
  left in place, so only the overlap half is undone), then restored and
  `diff`ed clean. Output:

  ```
  --- FAIL: TestPreviewFitDoesNotOverlapItself (0.00s)
      preview_fit_overlap_test.go:96: second previewTick with the first fit still in flight returned 2 commands, want 1 (the reschedule alone): a second overlapping fit resizes the same window again and costs an extra SIGWINCH
  FAIL
  FAIL	github.com/n-orlov/deck/internal/tui	0.003s
  ```

  Tests (b) and (c) still pass under the reverted guard — (a) is the one that
  pins the fix.
- `ci/run.sh env DECK_GODOG_TAGS='@requirement-21,@requirement-22,@steer-018-preview-fit-on-navigation' go test -count=1 ./features/`
  → `ok ... 23.064s` (`task015-r63-features-criteria-tags.log`).
  Note the vacuity caveat from the handoff notes: godog matches tags exactly,
  so the bare `@requirement-21` / `@requirement-22` select nothing; the
  selection that actually ran is the four
  `@steer-018-preview-fit-on-navigation` scenarios.
- Because of that, the same command was also run with the **exact** tags
  (`@requirement-21-preview-no-side-effects,@requirement-22-24-preview-colour-border-integrity,@steer-018-preview-fit-on-navigation`):
  green (`task015-r63-features-exacttags-green.log`), and green on 3/3 repeats
  at loadavg up to 4.91.
- `git diff --name-only` = `internal/tui/interactive.go`,
  `internal/tui/tui.go` only: `features/preview.feature` and
  `features/interactive_sigwinch_budget.feature` are UNMODIFIED, and no
  expected SIGWINCH count was touched.

## Observed flake (NOT caused by this change) — for R65 / task 017

`task015-r63-features-exacttags-flake.log` records one failure of
`features/preview.feature`'s scenario *"a fit is skipped below the
7-inner-row floor, and retried once the panel grows back above it"*:

```
suite.go:640: after scenario hook failed: fake "claude" agent received 1 SIGWINCH signals, want exactly 0
```

(the log also contains the client's SIGQUIT goroutine dump, hence its size).
This is the same shape as the pre-existing flake the handoff notes record for
`features/preview.feature:134` (the `preview_fit = false` scenario) — a
0-expected SIGWINCH assertion read before the client has settled — but in a
*different* scenario of the same feature. A/B evidence that it is not this
change: 3/3 green on the `@steer-018-preview-fit-on-navigation` tag with the
fix applied, 2/2 green on the exact-tag set with the two product files
reverted to HEAD, and 3/3 green on the exact-tag set with the fix applied
(loadavg 3.0-4.9). Logically the fix can only ever *suppress* fits, never add
one, so it cannot raise a SIGWINCH count. R65 (task 017) owns settling before
the counter is read; it should cover this scenario too, not only line 134.
