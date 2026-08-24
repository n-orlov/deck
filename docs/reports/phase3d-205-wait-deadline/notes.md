# Task 205 — bound ScreenDriver waits, in-progress evidence

## Change

`features/pty_driver_test.go`: `WaitForFrame`/`WaitForFrameGone` now call a new
`withDefaultWaitDeadline(ctx)` helper first. If the caller's `ctx` already carries
a deadline (every existing `context.WithTimeout(ctx, ...)` checkpoint in
`features/*.go` — 2s/3s/5s/20s/`resizeAwaitTimeout`), it is returned unchanged. If
it does not (the bare scenario `ctx` from `registerScenarioLifecycle`'s
`sc.Before`, which is what most `features/*.go` call sites hand these two
methods), a child bounded by `defaultWaitDeadline` (var, currently 45s) is
returned instead. This is exactly the gap that let run-2/run-5 of
`docs/reports/phase3d-i20-stability-logs` block forever in
`clientExitsCopyModeOnAttachedPane`'s `WaitForFrameGone` until Go's hard 10m test
timeout (goroutine 259).

`git diff features/pty_driver_test.go` (see the task 205 commit) shows only
additions: no existing `context.WithTimeout` call site's duration changed.

## Non-vacuousness / targeted test

New `TestWaitForFrameAppliesADefaultDeadlineWhenTheCallersContextHasNone`
(`features/pty_driver_test.go`) shrinks `defaultWaitDeadline` to 200ms for its own
duration, drives both `WaitForFrame` and `WaitForFrameGone` with a bare
`context.Background()` against a `ScreenDriver` whose `done` channel never closes
(no real process, so this cannot be "deck exited") and a frame/wanted-text pair
that can never resolve either way — proving only the new deadline stops the call.
Asserts `errors.Is(err, context.DeadlineExceeded)`, elapsed bounded near the
(shrunk) deadline, and the frame diagnostic present in the error text.

```
$ ci/run.sh sh -c 'go test -count=1 -run TestWaitForFrameAppliesADefaultDeadlineWhenTheCallersContextHasNone -v ./features/'
=== RUN   TestWaitForFrameAppliesADefaultDeadlineWhenTheCallersContextHasNone
--- PASS: TestWaitForFrameAppliesADefaultDeadlineWhenTheCallersContextHasNone (0.40s)
PASS
ok  	github.com/n-orlov/deck/features	0.408s
```

`ci/run.sh go build ./...`, `ci/run.sh go vet ./...`, `ci/run.sh gofmt -l $(git ls-files '*.go')`
all clean (empty output).

## Full features package: NOT green in this iteration's one run

`ci/run.sh go test -count=1 ./features/` (full log:
`full-suite-run-with-known-206-flake.log`, 306.986s) failed on exactly ONE
scenario: `@requirement-48-wheel-scrolls-attached-pane-without-typing`
("a wheel notch scrolls an attached pane's scrollback and leaves the shell's
input line untouched", `attach_scroll.feature:11`) — this is task 206's own
target bug (the copy-mode-exit hang: the `ATTACH_SCROLL_TOP_MARKER` frame marker
never clears). Critically, **this is the fix working as intended**: instead of
hanging to Go's 10-minute test-binary timeout (the old failure mode this task
exists to close), the scenario failed in 46.94s with the exact bounded
diagnostic (`context deadline exceeded` + the frame). No other scenario failed.

Because this iteration already spent its one whole-suite run, criterion "Full
features package still green in one run after the change" is NOT yet
satisfied and task 205 is left `in-progress`. The known cause is task 206's
target bug, which is flaky (not 100% reproduction), so a second full run in a
future iteration may well come back green even before 206 lands; if it does not,
206 needs to land first (fixing the copy-mode-exit hang) before 205 can cite a
green full run. `defaultWaitDeadline`'s 45s is otherwise proven safe/correct at
build/vet/gofmt/unit level.

## Next step

Next iteration (or after 206 lands): re-run `ci/run.sh go test -count=1
./features/` once. If green, commit the final piece of evidence, cite this
log's sha, and flip 205 to `completed`. If it lands on the same
`attach_scroll` scenario again, that's further corroborating evidence for 206
(cite this log too) — do 206 first, then retry.
