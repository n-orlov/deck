# Task 003 (R77) — reusing a deleted session's name reaps it, files included

Requirement: SPEC.md §9.2 / PRD R77 — a name (or §3.2 slug) held only by a tombstoned
row is available again, and taking it *reaps* that session: its row, its events, **and
deck's own per-session files** (captures dir, §9.4 history file).

The SQL half landed in commit `b80a4bd` (tx-scoped `reapSessionTx` /
`reapTombstonedHolderTx`, shared by `ReapSession` and `CreateSession`). Validation of
task 003 found the residual gap this evidence covers: the *filesystem* half was missing
from the create routes — `Service.CreateShell` / `Service.CreateAgent` only called
`Store.CreateSession`, so a reaped holder's captures dir and history file survived the
reuse. Cleanup existed only in the explicit `Service.Reap`.

## The fix

- `store.TombstonedNameHolders(ctx, name)` — read-only: which tombstoned rows hold this
  name or its slug, i.e. exactly the rows the create is about to reap in its own tx.
- `store.SessionRowExists(ctx, id)` — read-only post-commit confirmation that a holder
  really was reaped (a row restored in between keeps its scrollback).
- `Service.removeReapedSessionFiles` — the file half of a reap, now shared by
  `Service.Reap` and the create routes.
- `CreateShell`/`CreateAgent` note the tombstoned holders **before** the create and
  remove their files **after** `CreateSession`'s transaction has committed — never
  before, so a refused or failed create destroys nothing.

## Revert-and-reproduce

Red: the same tree with only the service-layer post-commit cleanup removed (the two
`reapedHolderFiles` call sites stripped in a throwaway `git worktree` at `.revert003`,
removed afterwards):

    --- FAIL: TestCreateShellReusingATombstonedNameCleansUpThatSessionsFiles (0.05s)
        create_name_reuse_test.go:66: captures dir of the reaped holder after reuse: stat = <nil>, want IsNotExist
    --- FAIL: TestCreateAgentReusingATombstonedNameCleansUpThatSessionsFiles (0.06s)
        create_name_reuse_test.go:104: captures dir after reuse: stat = <nil>, want IsNotExist
    FAIL	github.com/n-orlov/deck/internal/service	0.158s

Full log: `red-service-cleanup-reverted.log`.

Green with the fix (`green-service-cleanup-present.log`):

    ok  	github.com/n-orlov/deck/internal/service	0.159s

Both packages, whole-package runs (`green-store-service-packages.log`,
`ci/run.sh go test -count=1 ./internal/store/ ./internal/service/`):

    ok  	github.com/n-orlov/deck/internal/store	2.462s
    ok  	github.com/n-orlov/deck/internal/service	4.815s

Note that `TestRefusedCreateKeepsTheStillRestorableSessionsFiles` passes in *both*
trees, deliberately: it guards the ordering (nothing is deleted before the commit), so
it must never go red when the cleanup is absent — only the two reuse tests do.

## What is still not claimed here

- `RenameSession` still needs the same tombstone-reap treatment (task 004).
- No writer for the captures dir / history file exists in this tree yet (Phase 6); the
  tests seed those paths themselves, exactly as `TestReapRemovesCapturesDirAndHistoryFile`
  already does, to prove the removal side independently.
