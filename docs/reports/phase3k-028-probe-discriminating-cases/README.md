# Task 028: unit-test the probe's two discriminating cases

Adds two tests to `internal/service/availability_test.go`, alongside the
existing task-003 tests:

- `TestAvailDirectoryNamedPiDoesNotCount` — a directory named `pi` sitting on
  the probed PATH must not satisfy `kindAvailable`/`lookPathIn`: only an
  executable *file* counts, matching `lookPathIn`'s own `info.IsDir()` check.
- `TestAvailConfigEnvPathWinsOverProcessPath` — `Service.ConfigEnv["PATH"]`
  (config `[env]`) participates in `AvailableKinds`'s probe and wins over the
  process's own `os.Getenv("PATH")`, per `resolveLaunchEnv`'s layering
  (SPEC §6.3). Three assertions, each with exactly one directory holding a
  `pi` executable for the case it decides:
  - (a) **config-PATH-only positive** — `pi` exists only in `configDirWithPi`;
    process `PATH` is `configDirWithoutPi` (empty). `AvailableKinds() == [pi]`.
  - (b) **sanity** — same process `PATH` = `processDirWithPi` (the only dir
    with `pi` for the negative case), *no* `ConfigEnv`. `[pi]`. This exists so
    (c) cannot pass because the fake `pi` was unfindable to begin with.
  - (c) **process-PATH-only negative** — process `PATH` still
    `processDirWithPi` (holds the only `pi`), config `[env]` `PATH` =
    `configDirWithoutPi` (holds none). `AvailableKinds() == []`: config's PATH
    *replaces* captured_path's, so a process-PATH-only `pi` must not leak
    availability. This is the case a previous version of the test failed to
    cover — it had put a `pi` in *both* directories and still expected `[pi]`,
    which cannot discriminate.

## Evidence

- Targeted run: `ci/run.sh go test -count=1 -v -run 'Avail'
  ./internal/service/` → exit 0, 5 PASS lines (3 pre-existing + both new
  tests named). `avail-run.log` / `avail-run.exitstatus`.
- Whole-package: `ci/run.sh go test -count=1 ./internal/service/` → exit 0,
  `ok ... 6.238s`. `service-package.log` / `service-package.exitstatus`.

### Mutation checks (product code mutated in the worktree, run, then reverted)

Both mutations were applied to `internal/service/availability.go`, run with
the same targeted command, and reverted (`git status --porcelain` showed only
`internal/service/availability_test.go` modified afterwards).

1. `pathEnv := os.Getenv("PATH")` — config `[env]` PATH ignored entirely.
   Targeted run exits 1, `TestAvailConfigEnvPathWinsOverProcessPath` FAILs
   (at assertion (a)). `mutation-ignore-config-path.log` /
   `.exitstatus`.
2. `pathEnv := resolveLaunchEnv(...)["PATH"] + ":" + os.Getenv("PATH")` — the
   plausible *wrong* implementation where the process PATH is appended as a
   fallback rather than being overridden. Assertions (a) and (b) still pass;
   the run exits 1 on assertion (c) alone:
   `... = [pi], want [] (config PATH wins; a process-only pi must not leak
   availability)`. `mutation-append-process-path.log` / `.exitstatus`.

Mutation 2 is the one that matters for the criterion: it shows (c) genuinely
discriminates "config PATH wins" from "config PATH is merely consulted".
