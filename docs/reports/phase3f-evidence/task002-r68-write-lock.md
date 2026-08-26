# Task 002 — R68 fix 2: stop holding `s.mu` across `grid.Write`

Issue **#5** (interactive preview deadlocks the whole TUI when the previewed pane queries the
terminal). Fix 1 (the reply drain) landed as `f3c25d5` in task 001; this is the second, independent
fix the issue asks for: *"Don't hold `s.mu` across `grid.Write`. The lock is what escalates one
stuck goroutine into a dead UI."*

## What changed (`internal/interactive/grid.go`, +205/−43)

The single lock was split in two, and a one-frame cache was added so a repaint never has to wait
for the transport at all:

| lock/field | guards | held across a `Write`? |
|---|---|---|
| `s.mu` (`RWMutex`, existing) | the grid **pointer** only | **no** (this is the fix) |
| `s.writes` (`RWMutex`, new) | grid **mutation** vs. multi-call readers | yes — but no repaint ever waits on it |
| `s.lastFrame` (`atomic.Pointer[renderedFrame]`, new) | last successfully composed `RenderRows` frame | n/a |

* **`writeGrid(data)`** is now the ONE place an *installed* grid is mutated (`drain`'s per-read
  write and `writeNotice` both go through it). It resolves the grid pointer under `s.mu`
  (`currentGrid`), **releases `s.mu`**, then takes `s.writes` around `g.Write(data)`, then
  `MarkDirty`. A reseed's writes go into a private grid nobody can render yet and need none of it.
* **Lock ordering** is pinned in the `Session` struct's doc comment: a goroutine may take `writes`
  alone, or `mu` then `writes`, but must **never** take `mu` while holding `writes`. That is why
  `writeGrid` resolves the pointer *before* taking `writes`, and why nothing holding `writes`
  installs a grid.
* **`RenderRows`** keeps `s.mu.RLock()` for the whole composition, then takes `s.writes` with
  **`TryRLock`**: if a write is in flight it does not wait and does not touch the emulator at all
  (a parked writer also holds vt's `se.mu`, so even `g.Width()` would block behind it) — it returns
  `staleRows`, the previously composed frame, immediately. On success it stores a `slices.Clone`d
  frame in `lastFrame`.
* **`AbsoluteRow`/`SelectedText`** take a *blocking* `s.writes.RLock()`: they resolve ONE mouse
  gesture, where a wrong row is a wrong selection rather than a one-frame-stale picture, so
  exactness is worth waiting out a write — and that wait is bounded by fix 1's reply drain.
* `StartWithTransport` primes `lastFrame` with one `RenderRows(0, height)` after the seed write, so
  "the previous frame" is the seed rather than nothing even if the first repaint coincides with the
  first pipe read.

### What now guarantees `RenderRows` sees a consistent grid (criterion 1)

Three things, none of which is a lock held across a `Write`:

1. `s.mu` (read) held for the whole composition ⇒ every emulator call in it runs against **one**
   grid instance; a reseed's `installGrid` (write lock on `s.mu`) happens entirely before or
   entirely after.
2. `s.writes` (read) excludes grid **mutation** for that same span ⇒ the multi-call composition
   (`Scrollback()` + its `Len()`/`Line()`, plus `Render()`) is atomic against the
   `CellAt`/`Scrollback`-pointer-after-return hazard (task 085's gotcha), because every mutation of
   an installed grid goes through `writeGrid`.
3. When (2) cannot be had without waiting, `RenderRows` serves the last composed frame instead of
   blocking. That is bounded staleness, not permanent: `writeGrid` calls `MarkDirty` **after**
   releasing `writes`, so the coalesced render that write triggers finds the lock free unless yet
   another write is already in flight — and that one marks dirty in turn. The last write always
   ends in a fresh composition.

## Tests (`internal/interactive/writestall_test.go`, new)

Hermetic, no tmux. A **real** `grid.Write` is stalled by writing DA1 (`\x1b[c`) into a grid built
with `newGrid` (i.e. deliberately *without* fix 1's reply drain), so vt parks writing its answer
into the unbuffered `io.Pipe`. Every test is bounded by its own 3 s deadline, never the package
timeout.

1. `TestStalledGridWriteNeverBlocksRenderRows` — loops `RenderRows` for 250 ms while a write is
   parked, requires ≥20 completed calls, asserts the served frame still contains the pre-stall
   marker, and asserts the write is *still* parked at the end (otherwise the test would be
   inconclusive rather than green).
2. `TestStalledGridWriteNeverBlocksAReseed` — `Resize` must return while a write is parked, the
   grid pointer must change, and the parked write must return once `retireGrid` runs.
3. `TestConcurrentWritesAndReadsStayConsistent` — 300 rounds × 3 goroutines
   (`writeNotice` / `RenderRows` / `AbsoluteRow`+`SelectedText`) on a drained grid: catches a
   lock-order deadlock, and under `-race` catches the race if the `writes` gate is removed.

### Red controls

* **A — revert `grid.go` to `f3c25d5`, keep the new tests:** tests 1 and 2 FAIL on their own 3 s
  deadlines with the deadlock messages (test 3 passes — it is the race/deadlock guard, not the
  wedge guard). So the wedge tests cannot pass vacuously.
* **B — fix applied but `s.writes` stripped out, run under `-race`:** `WARNING: DATA RACE`
  (ultraviolet `Buffer.DeleteLineArea` write vs. `SelectedText` read) —
  `task002-red-nogate-race.log`. So the gate is load-bearing, not decoration.

Both controls were applied to copies and the tree was diffed back afterwards; no
`TEMPORARY RED CONTROL` marker remains.

## Verification

```
$ ci/run.sh gofmt -l internal/interactive/      # (no output)
$ ci/run.sh go vet ./internal/interactive/      # clean
$ ci/run.sh go test -count=1 ./internal/interactive/ ./internal/tui/
ok  github.com/n-orlov/deck/internal/interactive  11.429s
ok  github.com/n-orlov/deck/internal/tui          0.679s

$ ci/run.sh go test -race -count=1 ./internal/interactive/
ok  github.com/n-orlov/deck/internal/interactive  87.9s      # task002-race-interactive.log

$ ci/run.sh env DECK_GODOG_TAGS='@requirement-21,@requirement-22,@steer-018-preview-fit-on-navigation' \
    go test -count=1 ./features/
ok  github.com/n-orlov/deck/features  23.100s   (and 23.464s on a second run)

$ ci/run.sh env DECK_GODOG_TAGS='<the 9 tags in features/interactive_*.feature>' go test -count=1 -v ./features/
14 scenarios (14 passed)   # focus, both refusals, the floor-shrink leave, both repaint-notice
                           # scenarios, all three scrollback scenarios, the badge probe, both
                           # drag-to-copy scenarios
```

`git diff --stat` touches exactly one tracked file (`internal/interactive/grid.go`); no feature file
is modified, `features/preview.feature`, `features/interactive_sigwinch_budget.feature` and every
`features/interactive_*.feature` are byte-identical to `f3c25d5`.

### Honest note on the tag subset

Godog matches tags **exactly**: `@requirement-21` and `@requirement-22` match **nothing** in the
suite (the real tags are suffixed — `@requirement-21-preview-no-side-effects`,
`@requirement-22-undo-toast`). Only `@steer-018-preview-fit-on-navigation` selects anything: 4
scenarios in `features/preview.feature`.

Three of the four `interactive_*.feature` files carry **no tags at all**
(`interactive_geometry`, `interactive_option_tables`, `interactive_sigwinch_budget`), so no tag
expression can select them; they only run in the untagged full-suite run (task 021's job). The 9
tags above are every tag those files do carry.

## Finding: `@steer-018` "fit skipped below the 7-inner-row floor" is a pre-existing flake

`features/preview.feature:134` fails intermittently with
`fake "claude" agent received 1 SIGWINCH signals, want exactly 0` at the step on line 147 — under
host load, at **both** shas. A/B, same command, alternating, same host
(`task002-godog-ab-preview-fit-flake.log`):

| grid.go | load ≈ 1.5 | load 3.5–5.5 |
|---|---|---|
| with fix 2 | 2 pass / 0 fail (required subset) + 3 pass | 2 pass / 4 fail |
| at `f3c25d5` (fix 1 only) | — | 3 pass / 2 fail |

So it fails **without this change too**, at a comparable rate, and both required-command runs on a
quiet host are green. Mechanism: `When deck client "solo" terminal is resized to 100x9` returns
without waiting for deck to process the `tea.WindowSizeMsg`, so the next step's selection keypress
can be handled while `m.width/m.height` are still `100x30` — `previewFit`'s floor check then passes
and issues exactly one fit. That is R65's own class ("settle before reading the SIGWINCH counter",
task 017) with R63's overlapping-passive-fit (task 015) next door; higher load widens the window.
**No expected SIGWINCH count was changed to accommodate it.**

## Finding: can `cmd/fake-claude` emit DA1 at startup? (criterion 5)

**It can emit the bytes, but they can never reach deck's grid, so a startup-DA1 end-to-end scenario
is not possible and was not added.**

* `FAKE_CLAUDE_FIXTURE` + `FAKE_AGENT_FIXTURE_DIR` make `renderThenFallSilent` → `renderFixture`
  copy fixture bytes **verbatim** to stdout, so a fixture containing `\x1b[c` *is* emitted at
  startup.
* But that emission happens before deck arms `pipe-pane`, and the seed comes from `capture-pane`
  (escapes already stripped) — which is precisely the note issue #5 itself makes about why
  previewing a deck-launched agent normally looks fine. A startup DA1 therefore never traverses the
  R68 path.
* The route that **would** work end-to-end is `FAKE_CLAUDE_COMMANDS=1` plus a
  `{"command":"fixture","name":"..."}` line typed into the pane *while interactive mode is live*
  (the shape `replydrain_test.go`'s `sendLiteralLine` already uses at the unit seam). That needs a
  new feature file, new step registrations and a DA1 fixture dir; recorded as follow-up work, not
  done here. The wedge itself is covered at the transport seam by task 001's three query subtests
  and this task's three stall tests.

## Findings: issue #5's two "also noticed" leaks

1. **Orphaned `/tmp/deck-interactive-pipe-*` dirs — real, still open.** `ArmPipePane`
   (`internal/tmux/pipe.go:94`) creates the dir; only `closeLocal` (via `Close`/`CloseLocal`)
   removes it. `Session.Close` does call `pipe.Close()`, and `exitInteractive` calls
   `interactiveGrid.Close()`, so **Ctrl+Q always cleans up**. What has no cleanup at all is
   *abnormal* exit: nothing on the `tea.Quit` paths (`tui.go:1934`, `tui.go:5032`,
   `rename.go:53`) nor in `cmd/deck/main.go` after `Program.Run()` returns tears interactive mode
   down, and there is no signal handler for SIGTERM/SIGHUP. So a killed deck — exactly issue #5's
   own repro, where the wedged process had to be killed from another terminal — leaks the temp dir
   and FIFO, leaves `pipe-pane` armed, and leaves the window fitted and ownership held. A
   defensive sweep of stale `deck-interactive-pipe-*` dirs at startup (or moving the FIFO under
   `DECK_HOME`, where the existing leak scan can see it) would close it. Not fixed here: it is
   `internal/tmux` + `cmd/deck` lifecycle work, outside R68's lock fix.
2. **Preview-switch teardown — checked, no orphan.** `enterInteractive` returns immediately when
   `m.interactive` is already true (`internal/tui/interactive.go:38`), and the only retarget path,
   a mouse press on a different sidebar row, explicitly does `exitInteractive()` and *then*
   `enterInteractive()` on the new row (`internal/tui/mouse.go:286–289`). Keyboard navigation
   cannot switch the preview while interactive, since every key is forwarded to the pane. So no
   in-process path can hold two live `Session`s or drop one without `Close`. The issue's
   observation (two FIFOs open, one per pid) was **two separate deck processes**, which is expected
   — a preview switch inside one process does tear the old `Session` down. Only leak 1 above is a
   genuine defect.
