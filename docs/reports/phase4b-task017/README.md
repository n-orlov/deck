# Task 017 — `i` detail dialog: `g` shows and moves the session's group (R130 part 2, #30)

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
