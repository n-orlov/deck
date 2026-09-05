# Task 028: unit-test the probe's two discriminating cases

Adds two tests to `internal/service/availability_test.go`, alongside the
existing task-003 tests:

- `TestAvailDirectoryNamedPiDoesNotCount` — a directory named `pi` sitting on
  the probed PATH must not satisfy `kindAvailable`/`lookPathIn`: only an
  executable *file* counts, matching `lookPathIn`'s own `info.IsDir()` check.
- `TestAvailConfigEnvPathWinsOverProcessPath` — `Service.ConfigEnv["PATH"]`
  (config `[env]`) participates in `AvailableKinds`'s probe and wins over the
  process's own `os.Getenv("PATH")`, per `resolveLaunchEnv`'s layering
  (SPEC §6.3): pi installed only on the config PATH is available; with a
  config PATH set, pi installed only on the process PATH does not leak
  availability through it (config layer overrides the process layer instead
  of merely being consulted as a fallback).

## Evidence

`ci/run.sh go test -count=1 -v -run 'Avail' ./internal/service/` → exit 0,
5 PASS lines (3 pre-existing + 2 new), `ok`. See `avail-run.log` /
`avail-run.exitstatus`.

Whole-package sanity: `ci/run.sh go test -count=1 ./internal/service/` → ok
(6.3s), not otherwise cited/kept as a separate log since it added nothing
beyond the targeted run.
