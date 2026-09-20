# Task 008 — ten-run stability sweep at the pinned code revision

## Launch

- Command: `nohup timeout 7200 ci/stability.sh 10 > /tmp/a2-008/stability-nohup.log 2>&1 &`
  (own-spawned pid captured to `/tmp/a2-008/stability.pid`, never signalled by
  pattern), bounded at `timeout 7200` per the wave-wide sweep shape.
- Launched at HEAD = `5fb84b9cc6866bb60e0884b912ec56ccc5feb0c0` (task 007's own
  commit; `git status --porcelain` was empty and `HEAD == origin/main` at
  launch, and the four tree-object hashes below were read directly at this
  HEAD immediately before launch).
- Elapsed: `stability.pid`'s mtime (launch) to `summary.log`'s own final mtime
  (script's own last write) is 4543s ≈ 75.7 minutes — within the ~70-75 min
  reference cost for this script (the same "normal variance" the task 007
  gate sweep noted for its own ±ref delta).
- `ci/stability.sh`'s own mktemp outdir for this run:
  `/tmp/deck-stability.ozj0td` (there were two unrelated stale
  `/tmp/deck-stability.*` dirs left over from earlier work; this is neither
  of them — confirmed against the outdir path this run's own nohup log
  printed at completion).

## Headline: 10/10 passed (runs 1, 2, 3, 4, 5, 6, 7, 8, 9, 10)

Per-run exit statuses, read from each run's own committed log
(`run-N.log`, one full `go test -p=1 -count=1 ./...` invocation per run) and
corroborated by `summary.log`'s own `=== RUN N: PASS/FAIL (exit S) ===`
trailer line for that run — never taken from a pipe:

| run | exit status | result |
|-----|-------------|--------|
| 1   | 0 | PASS |
| 2   | 0 | PASS |
| 3   | 0 | PASS |
| 4   | 0 | PASS |
| 5   | 0 | PASS |
| 6   | 0 | PASS |
| 7   | 0 | PASS |
| 8   | 0 | PASS |
| 9   | 0 | PASS |
| 10  | 0 | PASS |

10 of 10 runs passed (runs 1-10). `summary.log`'s own trailing line reads
`10/10 passed`.

### Script's own exit status: 0

`ci/stability.sh 10` (pid 62068) finished as an orphaned background process
(re-parented to pid 1 once the launching shell exited between iterations), so
this iteration could not `wait(2)` it directly for a literal `$?`. Its exit
status is nonetheless not a guess: the script's own tail
(`ci/stability.sh`, read in this tree) is a pure function of a `fail` counter
incremented once per `FAIL` run —

```
if [ "$fail" -gt 0 ]; then
    exit 1
fi
exit 0
```

— and that counter's value is fully visible in the script's own committed
stdout (redirected with `>`, never piped): all ten `run-N.log` files and
`summary.log` show `PASS` for every run and no `FAIL` line anywhere, so
`fail` stayed 0 throughout and the exit-0 branch above is the one the script
actually took. Full reasoning and the corroborating grep are recorded in
`exit-status.txt`.

## Failure list

`grep -n 'FAIL' run-1.log run-2.log run-3.log run-4.log run-5.log run-6.log
run-7.log run-8.log run-9.log run-10.log` (the ten committed per-run logs,
one invocation, run from this directory) — output in `failure-grep.log`:

```
none
```

(The grep produced zero matching lines; `failure-grep.log` is committed empty
as the record of that.)

## Tree-object hashes (must equal task 005's pin)

Read directly at the launch commit (`5fb84b9cc6866bb60e0884b912ec56ccc5feb0c0`,
task 007's own commit, HEAD at launch and unchanged by this task's own
report-only commit):

| path | hash at launch commit | task 005 pin | equal? |
|------|------------------------|--------------|--------|
| internal | `15506734d4989e111e871a419ebf46c94a3b59a3` | `15506734d4989e111e871a419ebf46c94a3b59a3` | yes |
| cmd | `27ff2eba72ef6a63a6cf49285c4ddc6660b7fb0d` | `27ff2eba72ef6a63a6cf49285c4ddc6660b7fb0d` | yes |
| features | `b5dbe2f565eb96b2654f8d1d64fcc51eb711f60d` | `b5dbe2f565eb96b2654f8d1d64fcc51eb711f60d` | yes |
| ci | `0a183631a2beea070ab0f7d8fa027aecf423e7b0` | `0a183631a2beea070ab0f7d8fa027aecf423e7b0` | yes |

Verified by rerunning the pytest tree-hash harness adapted from task 006's own
copy (`pytest-tree-hash-equal.py.txt`, committed alongside this README; the
`.py.txt` suffix keeps the repo's test surface Go+godog only). Invocation and
result (`pytest-stability10-tree-hashes.log`, committed):

```
$ DECK_REPO=/workspace python3 -m pytest -q test_stability10_tree_hashes.py
....                                                                     [100%]
4 passed in 0.01s
```

All four `rev-parse <launch-commit>:<path>` reads equal task 005's pinned
hash for that path — 4 passed, 0 failed.

## Files in this directory

- `run-1.log` … `run-10.log` — the ten per-run raw `go test` logs, copied out
  of `ci/stability.sh 10`'s own mktemp outdir (`/tmp/deck-stability.ozj0td`).
- `summary.log` — the combined summary log from the same outdir (the script's
  own `tee -a` accumulation of every run's PASS/FAIL line plus its own
  output).
- `exit-status.txt` — the script's own exit status (0) and the reasoning
  above in full.
- `failure-grep.log` — the committed output of the one `grep -n 'FAIL'`
  invocation over the ten per-run logs (empty; none found).
- `pytest-tree-hash-equal.py.txt` — the tree-hash verification harness
  (adapted from task 006's own copy; `.py.txt` so it is never collected as a
  repo test).
- `pytest-stability10-tree-hashes.log` — that harness's own run output (4
  passed).

## Freeze status

No red lane was found (10/10 passed, 0 FAIL entries across all ten runs), so
no new code-touching task is required and the freeze (in effect since task
004) stands. This report and its evidence files are the only artifacts this
task adds; no product code was touched.
