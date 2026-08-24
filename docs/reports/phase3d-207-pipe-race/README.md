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

## Evidence

- `ci/run.sh sh -c 'go test -count=30 -run TestPanePipeWasClosed ./internal/tmux/'` — green, logged in
  `green-run-fixed.log`.
- Same command against the deliberately-broken `Close` ordering — red 30/30, logged in
  `red-run-ordering-broken.log`.
- `go build ./...`, `go vet ./...`, `gofmt -l internal/tmux/` all clean.
- Full `internal/tmux` package suite (`go test -count=1 ./internal/tmux/...`) green, 19.5s.
