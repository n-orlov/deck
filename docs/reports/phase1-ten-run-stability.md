# Phase 1: ten-run clean-state stability record

## Command used

```
ci/stability.sh 10
```

Run from the repository root on the Phase 1 tree (HEAD at the time of this
run: `a99fcb0` plus the `ci/stability.sh` wording fix for task 034). Each of
the 10 runs invokes `ci/run.sh go test -count=1 ./...` in a fresh `--rm`
sibling toolchain container (so any tmux sockets or other run-scoped state
die with the container between runs), redirecting to its own per-run log
file, and captures the real exit status of that `go test` invocation
directly (never through a `tee` pipeline — see the "ten-run-loop
mislabelled-PASS defect" entry in `docs/reports/phase1-findings.md` for why
that distinction matters).

## Result

All 10 runs passed. The regenerated log
[`docs/reports/phase1-ten-run-stability.log`](phase1-ten-run-stability.log)
contains all 10 per-run PASS labels, contains **no** line matching `^FAIL`
or `--- FAIL` anywhere (verified with
`grep -c '^FAIL\|--- FAIL' docs/reports/phase1-ten-run-stability.log` ->
`0`), and its final line reads `10/10 passed`.

## Timing

- Wall-clock duration of one full `go test -count=1 ./...` run inside the
  sibling container (measured standalone immediately before the loop, same
  tree): **35.7s** (`ok`/`?` lines for every package, including the
  `features` package's godog suite at ~34.5s of that total).
- Wall-clock duration of the entire 10-run loop (measured with `time`
  around the whole `ci/stability.sh 10` invocation, including the
  per-run `--rm` sibling container startup/teardown overhead each
  iteration): **5m51.79s** (351.79s), i.e. an average of ~35.2s/run,
  consistent with the standalone single-run timing above.

## Fix applied for this task

`ci/stability.sh`'s final two lines previously printed, in order:

```
=== FINAL COUNT: N / M runs passed ===
full per-run logs and combined summary log kept in: <tmpdir>
```

so the log's actual final line was the "kept in" path line, not a
"N/M passed" statement, which the task's success criteria require
literally. The two lines were reordered and the count line's wording
simplified to `N/M passed` so it is now unambiguously the final line of
the log. No test/build logic changed; this is a script-output-ordering fix
only, verified by inspecting the regenerated log above.
