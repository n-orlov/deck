# Task 205 — ten-run stability gate at the new final code sha

Final code sha (`git log -1 --format=%H -- '*.go' '*.feature'`): `4e09f2de90dcde04bd8fc20c77097e593f2fee5b`

HEAD at the time this gate ran: `0aa9837785eccc39eda55730e42c15c1a185740d` — a
docs/ci-only descendant of the final code sha (`git log --oneline
4e09f2de90dcde04bd8fc20c77097e593f2fee5b..0aa9837785eccc39eda55730e42c15c1a185740d
-- '*.go' '*.feature'` is empty, so no Go or feature source moved). The tree was
clean (`git status --porcelain` empty) and `HEAD == origin/main` before the run
started and remained so throughout: the gate only invokes `go test` inside a
throwaway sibling container and never writes into the workspace tree.

## Command

Launched from the clean tree in `/workspace`, exactly as the gate specifies —
`nohup` directly on the `timeout` invocation, output redirected to a log,
backgrounded, and then polled with `sleep 120` and nothing else. No other test
command ran in this iteration.

```
nohup timeout 7200 ci/stability.sh 10 > log 2>&1 &
```

(the redirect target was an absolute scratch path so nothing lands in the
workspace tree; the launch line is recorded verbatim on the first line of
[`poll.log`](./poll.log), pid 18789.)

The script's exit status is **not** captured by wrapping the launch in
`bash -c '...; echo $? > file'` — that would move `nohup` off the `timeout`
invocation, and a `wait` from any later bash call is never the job's parent so
it reports 127 instead of the truth. Instead `ci/stability.sh` now records its
own exit status on every exit path, beside its `summary.log`, via an `EXIT`
trap added in commit `0aa9837785eccc39eda55730e42c15c1a185740d` (a ci-only
change; the FAIL path was proven to write `1` using the script's
`DECK_STABILITY_SELFTEST_FAIL=1` self-test hook on a copy outside the repo,
which never invokes the suite). `summary.log.exitstatus` in this directory is
that self-recorded file, copied unmodified from the script's own output
directory.

Poll log: [`poll.log`](./poll.log) — 34 entries, each a `sleep 120` followed by
a liveness check on the launched pid and a one-line tail of the running log.
Wall time ≈ 66 min (launched 03:13:19Z, finished by the 04:19:33Z poll).

## Result

`ci/stability.sh`'s own exit status (`summary.log.exitstatus`'s content,
verbatim):

```
0
```

Final tally, quoted verbatim from the last line of `summary.log` (copied
unmodified from the script's own `mktemp` output directory):

```
10/10 passed
```

`grep -c '=== RUN .*: PASS' summary.log` is `10`. The tally is not below 10/10,
so nothing was re-run and there is no failing run number to report:
`grep -n -i fail summary.log` prints nothing and exits 1 against the copied
`summary.log`, so no per-run log needed to be attached.

## Files in this directory

- `summary.log` — copied verbatim from `ci/stability.sh 10`'s own output
  directory (`$(mktemp -d "${TMPDIR:-/tmp}/deck-stability.XXXXXX")`); combined
  log across all 10 runs plus each run's PASS/FAIL line and the final tally.
- `summary.log.exitstatus` — the script's own captured exit status (`0`), written
  by `ci/stability.sh`'s `EXIT` trap beside its `summary.log` and copied here
  unmodified.
- `poll.log` — the launch line and the `sleep 120` polling log for this run.
