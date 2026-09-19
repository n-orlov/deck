# Task 017 — `i` detail dialog: `g` shows and moves the session's group (R130 part 2, #30)

Third pass (attempt 3) is appended at the bottom under "Attempt 3"; read it
before the per-criterion section below, whose sha references are attempt 2's.

Re-verification of every success criterion at HEAD `f80f771` (verify with
`git rev-parse HEAD`; the numbers below were derived in this run, not copied
forward). The rejected defect from attempt 1 was **already cured in the tree**
by `bf964b8` ("tui: resolve the detail dialog's group row off the session row
itself"), which landed after the rejection was written; nothing further was
needed in product code this pass. Commits carrying the task:
`c754c43` (feature) and `bf964b8` (cure).

## Criterion-by-criterion

1. **Detail shows the group; `g` opens a move picker, added to `updateDetailView`
   without disturbing `q`, `ctrl+c`, `i`, `r`, `l`.**
   - `internal/tui/tui.go:7193` — `detailBody` prints
     `Group: <sessionGroupLabel(session)>`. `sessionGroupLabel`
     (`internal/tui/group.go`) reads `store.Session.GroupName` (the LEFT JOIN's
     already-resolved §11 label) and falls back to the literal `default`, so the
     row no longer depends on `m.moveGroupOptions` — the exact defect attempt 1
     was rejected for.
   - `internal/tui/rename.go:90-96` — the `case "g"` arm inside
     `updateDetailView`, alongside the untouched `q`/`ctrl+c`, `i`, `r`, `l`
     arms.
   - Pinned by `TestDetailDialogNamesTheSessionsGroupBeforeThePickerOpens`
     (named-group **and** structural-default subtests; asserts
     `moveGroupOptions` is still empty at the moment it reads the row — the
     rejection's own `GroupID=7`/`GroupName="alpha"` repro) and by
     `TestGDoesNotDisturbQCtrlCIOrRInsideDetail`.

2. **A named Go test moves a session through the picker and asserts one
   `sessions` row changed `group_id` while the other fixture rows did not.**
   - `TestMoveSessionGroupThroughPickerChangesOnlyThatOneRow`
     (`internal/tui/group_move_test.go:96`) — three fixture rows
     (session-alpha/bravo/default), moves alpha into bravo's group through the
     picker, then re-reads all three straight from the store: alpha's
     `group_id` is bravo's, bravo's is unchanged, default's is still NULL.
   - `TestMoveSessionGroupToDefaultClearsGroupID` pins the other direction
     (`group_id` → NULL, not a sentinel).

3. **Detail footer and `?` both name `g`, pinned by a footer/help parity test.**
   - Footer: `internal/tui/tui.go:7270` —
     `r renames · l edits launch inputs · g moves group · i or Esc closes detail`
     (plus the ASCII variant).
   - Help: `internal/tui/tui.go:8874` — the `i` bullet's
     `g inside it opens a picker that moves the session ... group`.
   - `TestDetailFooterAndHelpBothNameGForGroupMove` asserts both, and that the
     footer's other three keys were not displaced.
   - (The criterion's `tui.go:6567` / `tui.go:8068` line numbers are pre-move
     coordinates; the surfaces themselves are the two above.)

4. **`internal/tui/mark_test.go`'s batch-verb set unchanged — `x` and `dd`
   remain the only batch verbs.** `git diff c754c43^..bf964b8 --
   internal/tui/mark_test.go` is empty: this task's two commits do not touch the
   file at all. (It does differ from the launch sha `c3b530a`, but only through
   `8e9de6e` and the `43b7202` purge cure, neither of which belongs to this
   task and neither of which adds a batch verb.)

5. **`internal/tui` and `cmd/deck` green in the CI container.**
   `ci/run.sh go test -count=1 ./internal/tui/ ./cmd/deck/...` → both `ok`
   (`packages.log`). `ci/run.sh gofmt -l internal/tui/` printed nothing.

## Logs

- `packages.log` — `ci/run.sh go test -count=1 ./internal/tui/ ./cmd/deck/...`
- `targeted.log` — `ci/run.sh go test -count=1 -v -run '<the five named tests>'
  ./internal/tui/`, 5 tests + 2 subtests, all PASS.
- `packages-attempt3.log`, `targeted-attempt3.log`, `fifo-wait-cure.log` — the
  attempt-3 re-runs (see below); each begins with the exact invocation used.

## Attempt 3 — the red lane this task inherited, and its cure

Attempt 2 was rejected for one reason only: the required `internal/tui`
package run was **not** green in the CI container. Every task-specific
criterion (the detail group row, `g`'s picker, the untouched
`q`/`ctrl+c`/`i`/`r`/`l` arms, footer/help parity, `mark_test.go` byte-identical
across the task commits) had already passed in that same validation run. The
failure was `TestRaiseLostAttachOnStolenClaimTouchesNothing`:

    wait for pipe-pane's job to connect to /tmp/deck-interactive-pipe-*/pane.fifo:
    timed out after 5s waiting for pipe-pane's job to open the fifo

That is the load-sensitive pipe-pane FIFO failure class already recorded twice
in `docs/reports/phase3j-findings.md` (two other stolen-pane tests) and handled
there by rerunning. It is cured here rather than rerun, per this run's
red-lane rule: `internal/tmux/pipe.go`'s `waitForFifoWriter` conflated "this
fork+exec is merely slow" with "this pipe is broken" in one 5s wall-clock
bound, so a healthy, still-armed pipe-pane job that a busy host had not yet
scheduled through its own `open()` was reported as a timeout. Commit
`d179b2c` splits the bound in two — `fifoWriterProbeGrace` (5s, latency
window) and `fifoWriterConnectTimeout` (60s, pathology bound) — with one
`#{pane_pipe}` probe at the grace mark deciding between them: not armed fails
at once and says so, still armed buys patience. `ctx` is honoured throughout.
Nothing was weakened: the writer is still positively confirmed via the EAGAIN
discriminator before `ArmPipePane` returns, and the extra probe only ever
reaches the wire on the slow path, so the common path's tmux invocation
sequence is unchanged (which is what keeps the wire-counting tests in
`internal/tui` valid). Four new tests in
`internal/tmux/pipe_writer_wait_test.go` pin it, and the first was proved
load-bearing by re-running the same late-writer case with `grace == timeout`
(the old shape): it fails with the exact old message.

### Attempt-3 re-verification (derived at the shas printed below, not copied)

- Criteria 1-3: the five named tests re-run at the pre-cure HEAD `0dac638`
  and again in the package run after the cure — 5 tests + 2 subtests, all
  PASS (`targeted-attempt3.log`).
- Criterion 4: `git diff --stat c754c43^..bf964b8 -- internal/tui/mark_test.go`
  is still empty, and the cure commit touches only `internal/tmux/`, so `x`
  and `dd` remain the only batch verbs.
- Criterion 5: `ci/run.sh go test -count=1 ./internal/tmux/
  ./internal/interactive/ ./internal/tui/ ./cmd/deck/...` → all four `ok`
  (25.839s / 15.575s / 6.664s / 7.723s), `packages-attempt3.log`. The two
  packages the criterion names are in that set; `internal/tmux` and
  `internal/interactive` are included because the cure lives in the former and
  the latter is its other real caller. `ci/run.sh gofmt -l internal/tmux/` and
  `ci/run.sh go vet ./internal/tmux/` both printed nothing.
- Read-only guard: `git diff --stat c3b530a..HEAD -- SPEC.md prds/
  ci/Dockerfile ci/SPIKE.md` is empty.
