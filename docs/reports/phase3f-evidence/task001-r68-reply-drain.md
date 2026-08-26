# Task 001 — R68 fix 1: drain the vt emulator's reply stream

**Requirement:** R68 (issue [#5](https://github.com/n-orlov/deck/issues/5)) fix 1 of 2.
This task is *only* the reply drain. Fix 2 ("do not hold `s.mu` across `grid.Write`") is a
separate task and is **not** delivered here — the lock is still held across the write, so the
class of hazard R68 names remains open until that task lands.

## What changed

`internal/interactive/grid.go`, `internal/interactive/resize.go`:

- `Session.startReplyDrain(g *Grid)` — one goroutine per `*Grid` instance, looping on
  `Grid.Read()` and **discarding** what it reads. Joined through the new
  `Session.replyDrains sync.WaitGroup`.
- `Session.newDrainedGrid(w, h)` — `newGrid` plus its drain, started **before** the caller writes
  a byte into the grid. That ordering matters: a seed/reseed capture is written by the calling
  goroutine, so a query inside it would park `Start`/`Resize` themselves, not the drain goroutine.
- `retireGrid(g *Grid)` — closes the emulator's reply-pipe **write end**
  (`g.InputPipe().(*io.PipeWriter).CloseWithError(io.EOF)`), which hands the parked reader `io.EOF`
  so its goroutine returns.
- `Session.installGrid(fresh)` — swaps the grid under the write lock (same ordering guarantee
  `Resize`'s doc already promised) and retires the grid it displaces. Used by `captureLoop`,
  `fallbackLoop` and `Resize`, all of which previously did the swap inline.
- `Session.Close` retires the last grid and `Wait`s for every drain, so no goroutine this Session
  started is still reading an emulator after `Close` returns. `Start`'s failure path does the same.

`vt.NewSafeEmulator` still has exactly one call site in the package
(`TestExactlyOneGridConstructorCallSite` passes unchanged).

### Discard, not forward — deliberately

The reply is dropped. deck is a viewer: the pane's program already has a real terminal on the far
side of tmux, and a reply *vt* synthesised inside deck's copy of the screen is not what that
terminal would have answered. The pipe is armed `-IO`, so forwarding is possible, but it would feed
a live agent a fabricated DA1 it never asked deck for. Discarding restores exactly the
no-preview-open situation: deck's emulator answers nothing.

### Why close the pipe writer instead of `Emulator.Close()`

Two reasons, both load-bearing:

1. **A drain must be retirable.** `captureLoop` reseeds every `capturePollInterval` (200 ms), so a
   grid whose drain cannot be stopped is a goroutine leak at five per second. Closing the write end
   ends the read with `io.EOF`.
2. **`Emulator.Close()` would be a data race.** `vt`'s `Emulator.closed` is written by `Close` and
   read, unsynchronised, by every `Read`/`Write` (`SafeEmulator.Read` deliberately does not take
   `se.mu`). Closing while a reader is live is therefore racy *inside the library* — recorded here
   as a finding, not worked around by giving up the reader.

A retired grid still renders exactly as before (only its reply pipe is gone), which keeps a pointer
previously handed out by `Grid()` usable; a further reply write now fails immediately with
`io.ErrClosedPipe` instead of blocking — the same non-stalling outcome.
`TestEmulatorInputPipeIsAPipeWriter` pins the one library detail this depends on, so a `go.mod`
bump that changes it fails loudly instead of silently leaking drains.

## The new test

`internal/interactive/replydrain_test.go`:

- `TestTerminalQueryInPaneOutputNeverStallsSession` — table-driven over **DA1** (`\033[c`),
  **DSR** (`\033[6n`) and the **OSC 11** colour query (`\033]11;?\007`), i.e. two CSI triggers and
  one OSC one, not just the observed sequence. Each case drives a **real tmux pane** (the actual
  defect path: pane output → `pipe-pane` → `Session.drain` → `grid.Write`), then asserts both
  things the deadlock took away: the drain kept going (a marker printed immediately *after* the
  query reaches the grid) and `RenderRows` still returns.
  - The marker is passed as two `printf` **arguments**, not spelled out in the format string,
    because the shell echoes the typed command line into the pane first — a marker visible in that
    echo would make the assertion pass against unfixed code.
  - The assertion work runs in a goroutine bounded by the test's own `time.After`
    (`replyQueryDeadline = 5s`) and fails with a message naming the deadlock. `Session.Close` is
    called through `closeSessionWithin` (3 s bound), because `Close` waits for the very drain
    goroutine that is parked when the regression is present.
- `TestRetireGridUnblocksItsReplyDrain` — hermetic (no tmux): a query write into a drained grid
  returns, and `retireGrid` makes the drain exit. Both halves bounded.
- `TestEmulatorInputPipeIsAPipeWriter` — the library-detail pin described above.

## Evidence (verbatim)

Host `/proc/loadavg` at the green run: `4.06 2.59 2.27 5/3969 775`; at the red run:
`1.34 1.95 2.06 1/3873 717`.

### Green at HEAD

```
$ ci/run.sh go test -count=1 -run 'TestTerminalQueryInPaneOutputNeverStallsSession|TestRetireGridUnblocksItsReplyDrain|TestEmulatorInputPipeIsAPipeWriter' -v ./internal/interactive/
=== RUN   TestTerminalQueryInPaneOutputNeverStallsSession
=== RUN   TestTerminalQueryInPaneOutputNeverStallsSession/da1
=== RUN   TestTerminalQueryInPaneOutputNeverStallsSession/dsr
=== RUN   TestTerminalQueryInPaneOutputNeverStallsSession/osc11
--- PASS: TestTerminalQueryInPaneOutputNeverStallsSession (0.11s)
    --- PASS: TestTerminalQueryInPaneOutputNeverStallsSession/da1 (0.04s)
    --- PASS: TestTerminalQueryInPaneOutputNeverStallsSession/dsr (0.04s)
    --- PASS: TestTerminalQueryInPaneOutputNeverStallsSession/osc11 (0.04s)
=== RUN   TestRetireGridUnblocksItsReplyDrain
--- PASS: TestRetireGridUnblocksItsReplyDrain (0.00s)
=== RUN   TestEmulatorInputPipeIsAPipeWriter
--- PASS: TestEmulatorInputPipeIsAPipeWriter (0.00s)
PASS
ok  	github.com/n-orlov/deck/internal/interactive	0.119s

real	0m1.415s
```

### Red with the drain reverted

Revert applied to `startReplyDrain`'s goroutine only — the reader stops reading, i.e. exactly
pre-fix behaviour (nothing calls `Grid.Read()`):

```go
 		defer s.replyDrains.Done()
+		// TEMPORARY RED CONTROL (R68): the reply drain reverted to
+		// pre-fix behaviour -- nothing reads Grid.Read().
+		if true {
+			return
+		}
 		buf := make([]byte, replyDrainBufSize)
```

Recorded at the earlier (8 s / 5 s) bounds, so the wall times below are the pre-tightening ones;
every failure is by the **test's own deadline**, none by package timeout:

```
=== RUN   TestTerminalQueryInPaneOutputNeverStallsSession/da1
    replydrain_test.go:149: DEADLOCK: after the pane emitted primary device attributes (CSI c), which vt answers at handlers.go:695 -- the query issue #5 was root-caused on, the assertion goroutine never returned within 8s -- Session.drain is parked inside grid.Write (vt writes the reply into an unbuffered io.Pipe nobody reads) while holding s.mu, so RenderRows can never take the read lock. This is issue #5: in the real client that parked View() and with it bubbletea's event loop, killing the whole TUI
    panic.go:615: Session.Close did not return within 5s -- its drain goroutine is still parked; leaking this Session rather than hanging the package
=== RUN   TestTerminalQueryInPaneOutputNeverStallsSession/dsr
    replydrain_test.go:149: DEADLOCK: after the pane emitted device status report / cursor position (CSI 6n), which vt answers at handlers.go:806, the assertion goroutine never returned within 8s -- Session.drain is parked inside grid.Write (vt writes the reply into an unbuffered io.Pipe nobody reads) while holding s.mu, so RenderRows can never take the read lock. This is issue #5: in the real client that parked View() and with it bubbletea's event loop, killing the whole TUI
    panic.go:615: Session.Close did not return within 5s -- its drain goroutine is still parked; leaking this Session rather than hanging the package
=== RUN   TestTerminalQueryInPaneOutputNeverStallsSession/osc11
    replydrain_test.go:149: DEADLOCK: after the pane emitted the OSC 11 background-colour query, which vt answers at osc.go:92, the assertion goroutine never returned within 8s -- Session.drain is parked inside grid.Write (vt writes the reply into an unbuffered io.Pipe nobody reads) while holding s.mu, so RenderRows can never take the read lock. This is issue #5: in the real client that parked View() and with it bubbletea's event loop, killing the whole TUI
    panic.go:615: Session.Close did not return within 5s -- its drain goroutine is still parked; leaking this Session rather than hanging the package
--- FAIL: TestTerminalQueryInPaneOutputNeverStallsSession (39.09s)
    --- FAIL: TestTerminalQueryInPaneOutputNeverStallsSession/da1 (13.03s)
    --- FAIL: TestTerminalQueryInPaneOutputNeverStallsSession/dsr (13.03s)
    --- FAIL: TestTerminalQueryInPaneOutputNeverStallsSession/osc11 (13.04s)
=== RUN   TestRetireGridUnblocksItsReplyDrain
    replydrain_test.go:185: DEADLOCK: grid.Write("\x1b[c") never returned within 5s -- the grid's reply drain is not reading, so vt's DA1 reply parked its writer permanently (issue #5)
--- FAIL: TestRetireGridUnblocksItsReplyDrain (5.00s)
=== RUN   TestEmulatorInputPipeIsAPipeWriter
--- PASS: TestEmulatorInputPipeIsAPipeWriter (0.00s)
FAIL
FAIL	github.com/n-orlov/deck/internal/interactive	44.101s

real	0m45.083s
```

The bounds were then tightened to 3 s poll / 5 s deadline / 3 s close so the whole red run reports
in roughly half a minute rather than three quarters of it; green wall time is unchanged (~1.4 s).
The revert was applied to a copy-and-restore of `grid.go` and the file was diffed back to the fixed
version afterwards (`diff` clean, no `REVERTED`/`if true` left in the tree).

### Regression cover around the change

```
$ ci/run.sh go test -count=1 ./internal/interactive/ ./internal/tui/ ./internal/tmux/
ok  	github.com/n-orlov/deck/internal/interactive	11.354s
ok  	github.com/n-orlov/deck/internal/tui	0.538s
ok  	github.com/n-orlov/deck/internal/tmux	19.375s

$ ci/run.sh go test -race -count=1 ./internal/interactive/
ok  	github.com/n-orlov/deck/internal/interactive	105.050s

$ ci/run.sh go build ./...        # BUILD_OK
$ ci/run.sh gofmt -l internal/interactive/   # (no output)
```

`features/` was not run in this iteration — R68's criteria require `features/preview.feature`,
`features/interactive_sigwinch_budget.feature` and the other `features/interactive_*.feature` to
pass unmodified, and that check belongs with fix 2 / the requirement's own verification task, which
touches the same lock those scenarios exercise.

## Findings (not fixed here)

1. **`vt`'s `Emulator.closed` is unsynchronised.** `Close` writes it; every `Read`/`Write` reads it,
   and `SafeEmulator` deliberately does not lock around `Read`. Any caller that closes an emulator
   while another goroutine reads or writes it races. deck now avoids `Emulator.Close()` entirely
   (see above) rather than depending on that being benign.
2. **Fix 2 is still open.** `drain`/`writeNotice` still hold `s.mu` across `grid.Write`, so any
   *future* undrained writer under `Write` would still escalate a stalled preview into a wedged UI.
   That is exactly the class R68's second fix removes.
3. Not investigated in this task: whether the fake agent can emit DA1 at startup (R68's end-to-end
   scenario question), and #5's two minor leaks (orphaned `/tmp/deck-interactive-pipe-*`,
   preview-switch teardown).
