# Task 201 — rename-route name-reuse cleanup (review finding 2, R77)

`internal/service/rename.go`'s `Rename` called `store.RenameSession` without ever
using the existing `Service.tombstonedNameHolders` / `Service.reapedHolderFiles`
pair that `CreateShell` and `CreateAgent` already use. The store half of R77
(reaping a tombstoned name/slug holder's *row*, inside `RenameSession`'s own
transaction) was already correct — this only affects the tombstoned holder's
**files** (captures dir + §9.4 history file), which survived a rename that
freed their name.

## Fix

`Rename` now:
1. calls `tombstonedNameHolders(ctx, trimmed)` **before** `store.RenameSession`
   to note which tombstoned rows currently hold the target name/slug;
2. calls `store.RenameSession`;
3. on success, calls `reapedHolderFiles(ctx, reapedHolders)` to remove the
   files of whichever of those holders `RenameSession` actually reaped.

No new filesystem helper was added — this reuses the exact same two
`Service` methods `CreateShell`/`CreateAgent` already call, in the same order.

## Tests added

`internal/service/rename_reuse_test.go`:

- `TestRenameOntoATombstonedNameCleansUpThatSessionsFiles` — creates a session
  with a real captures directory and a real §9.4 history file, tombstones it,
  renames a second session onto the freed name, and asserts both of the
  reaped session's paths are gone while the renamed session's own
  captures/history survive untouched.
- `TestRefusedRenameKeepsLiveAndArchivedHoldersFiles` — a rename refused by a
  live holder's name, and one refused by an archived holder's name, both
  leave that holder's files (captures dir + history file) intact, and leave
  the subject session's own name unchanged.

## Revert-and-reproduce evidence

Command used for both runs:

```
ci/run.sh go test -count=1 -run 'TestRenameOntoATombstonedNameCleansUpThatSessionsFiles|TestRefusedRenameKeepsLiveAndArchivedHoldersFiles' -v ./internal/service/
```

- **Red** (pre-fix `rename.go`, i.e. `git stash` of just that file with the new
  test committed): `red.log` — `TestRenameOntoATombstonedNameCleansUpThatSessionsFiles`
  fails (`captures dir of the reaped holder after rename: stat = <nil>, want
  IsNotExist`); the refusal test already passed before the fix, since it only
  proves nothing regresses on the refusal path.
- **Green** (post-fix `rename.go`): `green.log` — both tests pass.

## Required package run

```
ci/run.sh go test -count=1 ./internal/service/ ./internal/store/
```

Result: `ok`, ~5.8s total (`full.log`).
