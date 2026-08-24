# Task 207: fix `TestPanePipeWasClosedIsTrueBeforeAnyBlockedReadCanObserveOurOwnCloseAsEOF`'s race with real pane output

## Symptom

A prior isolated run ("run-8", referenced from the task) failed with:

```
got n=2 err=<nil>, want n=0 err=io.EOF
```

The test's reader goroutine did a single `pipe.Read(buf)` and asserted the result was `io.EOF`. But
the pane under test is a bare shell (`newBareGeometrySession`), which can legitimately emit its own
bytes (a prompt, a motd line) at any point after the pipe is armed, including during the 200ms the
test sleeps to let the read block before calling `Close()`. When that happens the FIRST `Read` simply
returns real pane data (`n>0, err=nil`), not our `Close`'s EOF — the test's assumption that the first
`Read` is always the one `Close` unblocks was wrong, not the product.

## Fix

`features/attach_scroll_test.go`-style principle applied here too: don't assume, prove. The reader
goroutine in `internal/tmux/pipe_displacement_test.go` now loops, draining and discarding any number
of non-error reads (each one is unrelated pane chatter), and only evaluates `WasClosed()` against the
read that actually returns an error — whichever real-numbered read that turns out to be. This still
proves exactly the discriminator the test exists for: by the time a blocked `Read` observes our own
`Close` as `io.EOF`, `WasClosed()` already reports `true`.

No assertion was removed or widened; the scenario/test's purpose is unchanged. `PanePipe.Close`,
`WasClosed`, and their locking are untouched — this is a test-only fix for a harness assumption.

## Non-vacuousness: red run with the ordering broken

To confirm the fixed test still catches a real regression in `WasClosed`'s set-before-disarm
ordering, `internal/tmux/pipe.go`'s `Close` was temporarily rewritten (scratch, not committed) to
release `closeMu` *before* setting `disarmed = true` and running the disarm command + `closeLocal`,
then re-acquire it only to set `disarmed = true` afterward — i.e. exactly the ordering bug the test's
comment describes ("WasClosed() reported false ... drain would misread this as an external
displacement/disablement"). Result: 30/30 isolated runs failed, captured in
`red-run-ordering-broken.log`. The scratch change was fully reverted (`git diff` clean on
`internal/tmux/pipe.go`) before committing this task.

## Evidence (first pass, commit 74538ac)

- `ci/run.sh sh -c 'go test -count=30 -run TestPanePipeWasClosed ./internal/tmux/'` — green, logged in
  `green-run-fixed.log`.
- Same command against the deliberately-broken `Close` ordering — red 30/30, logged in
  `red-run-ordering-broken.log`.
- `go build ./...`, `go vet ./...`, `gofmt -l internal/tmux/` all clean.
- Full `internal/tmux` package suite (`go test -count=1 ./internal/tmux/...`) green, 19.5s.

## Second race found on validation: `os.ErrClosed` ( "file already closed" )

Validation of the first pass re-ran the exact required command
(`ci/run.sh sh -c 'go test -count=30 -run TestPanePipeWasClosed ./internal/tmux/'`) 10 independent
times in a fresh sibling container and hit a genuinely different failure 3/10 times:

```
Close's own disarm command did not produce io.EOF here (got n=0 err=read /tmp/deck-interactive-pipe-*/pane.fifo: file already closed)
```

`os.ErrClosed`'s message IS `"file already closed"` (`internal/oserror`, aliased by both `io/fs` and
`os`). Root cause: `PanePipe.Close` (`internal/tmux/pipe.go`) issues tmux's disarm command and then,
in the SAME goroutine right after, calls `p.fifo.Close()` on our own fd. That races the still-blocked
`Read` against the real kernel-level EOF the disarmed job's eventual process exit produces. When our
own `fifo.Close()` wins that race, the Go runtime's poller aborts the pending `Read` itself and
returns `os.ErrClosed` — not `io.EOF` — because the fd was closed out from under it locally, before
the kernel ever surfaced the writer-side close as a genuine EOF. This is a real race in `Close`'s own
sequencing (disarm-then-closeLocal), not a test artifact — but it is also not a defect production
code needs fixing for: `internal/interactive/grid.go`'s `drain` already checks `WasClosed()` FIRST
and only escalates an `io.EOF` observed while `WasClosed()` is still false, so an `os.ErrClosed`
observed once `WasClosed()` is true is silently accepted as an ordinary shutdown there too, by
construction, with no special-casing needed. Only the test was too strict: it required `io.EOF`
specifically instead of "any error, once WasClosed() is true", which is the actual property that
matters and the one production code relies on.

**Fix (this pass):** the test's terminal assertion now accepts EITHER `io.EOF` OR `os.ErrClosed` as
a valid way to observe our own `Close`, while still requiring `WasClosed()` to be `true` at that
point — i.e. it now asserts exactly the property production code depends on, no more and no less.
No assertion was removed; the discriminator (`WasClosed()` first) is unchanged and still mandatory.

## Evidence (second pass, this commit)

- 10 independent runs of the exact required command in a fresh sibling container, post-fix: 10/10
  green (`green-run-fixed-retry.log` is one of them; all ten were `ok` with no `FAIL`).
- Non-vacuousness re-proved against the SAME fix: `internal/tmux/pipe.go`'s `Close` was again
  scratch-rewritten (this time releasing `closeMu` before setting `disarmed = true`, deferring that
  assignment until after the disarm command and `closeLocal` run) — 30/30 red
  (`red-run-ordering-broken-retry.log`), confirming the fixed test still catches the ordering bug the
  first pass's revert-test caught. Scratch change fully reverted (`git diff` clean on
  `internal/tmux/pipe.go`) before this commit.
- `go build ./...`, `go vet ./...`, `gofmt -l $(git ls-files '*.go')` all clean.
- Full `internal/tmux` package suite (`go test -count=1 ./internal/tmux/...`) green, 19.4s.
