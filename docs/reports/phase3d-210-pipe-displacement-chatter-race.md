# Task 210 (part b): root cause and fix — `TestPanePipeReceivesGenuineEOFOnDisplacementWithPanePipeStillOne`

## Failure

`internal/tmux/pipe_displacement_test.go`'s
`TestPanePipeReceivesGenuineEOFOnDisplacementWithPanePipeStillOne` failed intermittently (1/10 in
task 209's `ci/stability.sh 10` measurement) with:

```
pipe_displacement_test.go:79: displaced reader: got n=2 err=<nil>, want n=0 err=io.EOF
```

Reproduced directly at low host load (1-min loadavg ~3): green 20/20 at `-count=20`, then
`-count=100` produced 4 failures, all with the identical shape (`n=2 err=<nil>`) — a low but
real, load-independent rate, exactly the kind of race steer 019 §2 asked this task to root-cause
rather than dismiss.

## Root cause: the same chatter race the file's own third test already documents and handles

The test arms a `PanePipe` against a **bare shell** target (`newBareGeometrySession`), then races
a single `Read` (via the old `readWithTimeout` helper, one Read call, no retry) against a second
`pipe-pane -IO` displacing deck's own pipe on the same target. The shell is a live process, not
something that stays silent until the displacement lands — it can (and, at the measured low
rate, does) emit its own bytes first (a prompt, a motd line, a shell-init echo) as an ordinary,
unrelated read that happens to win the race against the displacement's EOF.

This is **exactly** the race
`TestPanePipeWasClosedIsTrueBeforeAnyBlockedReadCanObserveOurOwnCloseAsEOF` (same file, added by
task 207) already discovered and solved for the self-close case: its own doc comment describes
"the pane is a bare shell, not a program that stays silent... it can emit its own bytes... at
any point" and its reader goroutine drains and discards any number of `n>0, err==nil` reads,
only evaluating the read that actually returns an error. The displacement test (and its sibling,
the disable test) never got the same treatment — they used a single bare `Read` via
`readWithTimeout`, which asserts on whichever read comes back first, chatter or not.

## Fix

Added `readUntilErrorWithTimeout`, a drain-loop sibling of the old `readWithTimeout`: it loops
`Read` on a background goroutine, discarding any `n>0, err==nil` result as unrelated pane
chatter, and only returns the read that actually carries an error (or times out if none ever
does). `TestPanePipeReceivesGenuineEOFOnDisplacementWithPanePipeStillOne` and
`TestPanePipeReceivesGenuineEOFOnDisableWithPanePipeZero` (its sibling, same construction, same
latent vulnerability though not observed failing in 209's sample) both switched to it. The old
single-shot `readWithTimeout` became unused once both call sites moved, and was removed rather
than left as dead code.

`TestPanePipeWasClosedIsTrueBeforeAnyBlockedReadCanObserveOurOwnCloseAsEOF` (the self-close test)
was not touched — it already had its own equivalent drain loop inline; `readUntilErrorWithTimeout`
generalizes that same pattern for the other two tests rather than duplicating it a third time.

No production code changed — this is a test-harness fix for a race in the *test's own*
single-Read assumption, not a race in `PanePipe`, `ArmPipePane`, or `drain` (the discriminator
those already use, `WasClosed()`-first, is unaffected and was not touched).

## Non-vacuousness

- **Before this fix**: isolated `ci/run.sh go test -count=100 -run
  TestPanePipeReceivesGenuineEOFOnDisplacementWithPanePipeStillOne ./internal/tmux/` at 1-min
  loadavg ~3 reproduced 4/100 failures, all `n=2 err=<nil>`.
- **After this fix**: `ci/run.sh go test -count=100 -run
  'TestPanePipeReceivesGenuineEOFOnDisplacementWithPanePipeStillOne|TestPanePipeReceivesGenuineEOFOnDisableWithPanePipeZero|TestPanePipeWasClosedIsTrueBeforeAnyBlockedReadCanObserveOurOwnCloseAsEOF'
  ./internal/tmux/` (300 sub-runs total across the three related tests) at 1-min loadavg ~3.5:
  100% green, `19.195s`/`26.303s` wall depending on the exact invocation.
- **Revert-and-reproduce**: `git stash` (removing this fix), then the same `-count=100`
  displacement-only rerun at 1-min loadavg ~2.7 reproduced the identical failure (4/100,
  `n=2 err=<nil>`); `git stash pop` restored the fix — `git diff --stat` afterward showed exactly
  the one intended file (`internal/tmux/pipe_displacement_test.go`).
- Full `internal/tmux` package: `go test -count=1 ./internal/tmux/...` green (`19.195s`).
- `go build ./...`, `go vet ./...`, `gofmt -l` on the touched file all clean.

Scratch repro output is this iteration's own terminal transcript (not written to a separate log
file), per this run's existing convention for short ad hoc reruns.

## Scope note

This resolves the **third** of task 210's four named failure classes (1/10 in 209's taxonomy,
root-caused together with task 207's fix per steer 019 §2, same file/mechanism). Still open: the
"a unique directory match ghosts in the dimmed token..." failure (2/10, not yet investigated).
Once that lands, task 217's final-code-commit sha needs re-declaring (this is a code commit) and
`ci/stability.sh 10` needs a fresh run before task 210 itself can be marked complete.
