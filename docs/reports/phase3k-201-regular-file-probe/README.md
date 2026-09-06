# Task 201 — reject non-regular files in the shared availability probe

## Change

`internal/service/availability.go`'s `lookPathIn` now requires
`info.Mode().IsRegular()` on the candidate in **both** branches:

- the path-separator branch (`os.Stat(file)` on a direct path)
- the PATH-search loop (`os.Stat(candidate)` per PATH directory)

Before this change, a FIFO (or any other non-regular node: socket, device,
etc.) with the executable bits set (mode 0755) passed `isExecutable` and was
reported available, even though it is not something `exec.Command` can
actually run as a binary. `os.Stat` follows symlinks, so a symlink to a real
regular executable is unaffected — it still passes `IsRegular`.

This is a deliberate tightening beyond what `exec.LookPath` itself
guarantees on some Go releases (older stdlib implementations are similarly
permissive about non-regular-but-executable-mode files) — it is not a
restoration of stdlib parity. Said so in `lookPathIn`'s doc comment.

## Test

`internal/service/availability_test.go` adds
`TestLookPathInRejectsFIFOEvenWithExecuteBits`:

- creates a FIFO named `pi` with mode 0755 via `syscall.Mkfifo` in a
  `t.TempDir()`
- asserts `lookPathIn("pi", dir) != nil`
- registers `agent.NewPi()` in a fresh registry, points `PATH` at that same
  dir, and asserts `pi` is absent from `Service{Agents: registry}.AvailableKinds()`

## Evidence

- `green.log` / `green.exitstatus` — `ci/run.sh go test -count=1
  ./internal/service/ ./internal/agent/` with the fix in place: exit 0,
  `ok  	github.com/n-orlov/deck/internal/service` and
  `ok  	github.com/n-orlov/deck/internal/agent`.
- `red.log` / `red.exitstatus` — same new test run alone
  (`-run TestLookPathInRejectsFIFOEvenWithExecuteBits -v`) with ONLY the
  `IsRegular` check removed from both branches (the loop's guard reverted to
  `!info.IsDir()`, the direct-path branch's `IsRegular` check deleted): the
  new test FAILs, exit 1, `lookPathIn("pi", ...) = nil, want a non-nil error
  (pi on that PATH is a FIFO, not a regular file)`. The check was restored
  (`git diff` after is clean vs. the committed state) before committing.
