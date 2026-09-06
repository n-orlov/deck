# Task 205 — ten-run stability gate at the new final code sha

Final code sha (`git log -1 --format=%H -- '*.go' '*.feature'`): `4e09f2de90dcde04bd8fc20c77097e593f2fee5b`

HEAD at the time this gate ran: `f062075e50671670d54c1137aa06f117d44045b8` (a
docs-only descendant of the final code sha — `git log --oneline
4e09f2de90dcde04bd8fc20c77097e593f2fee5b..f062075e50671670d54c1137aa06f117d44045b8
-- '*.go' '*.feature'` is empty). Tree was clean (`git status --porcelain`
empty) and `HEAD == origin/main` before the run started and remained so
throughout, since the gate only invokes `go test` inside a throwaway sibling
container and never touches the workspace tree.

## Command

Launched from a clean tree as a backgrounded, `timeout`-guarded run, polled
with `sleep 120` and nothing else (no other test run in this iteration). The
exit-status-capture gotcha recorded from task 203 applies here too — a plain
`nohup timeout 7200 ci/stability.sh 10 > log 2>&1 &` followed by a `wait` in a
later (fresh-shell) bash-tool call returns 127 instead of the real status,
because that later shell is never the launched job's parent — so the capture
is folded into the SAME backgrounded shell that runs the command:

```
nohup bash -c 'timeout 7200 ci/stability.sh 10 > /tmp/deck-205/log 2>&1; echo $? > /tmp/deck-205/log.exitstatus' &
```

i.e. `ci/stability.sh 10` was launched under `timeout 7200` exactly as
specified, redirected to a log, and backgrounded with `nohup`; the only
addition is the trailing `; echo $? > ...exitstatus` in the same shell
invocation, which is what makes `summary.log.exitstatus` below the script's
own real exit status rather than an artifact of a `wait`-after-the-fact race.

Poll log: [`poll.log`](./poll.log) — each entry is a `sleep 120` followed by a
liveness check and a one-line tail of the running log; no other test command
ran in any poll.

## Result

`ci/stability.sh`'s own exit status, captured atomically as described above
(`summary.log.exitstatus`'s content, verbatim):

```
0
```

Final tally, quoted verbatim from the last line of `summary.log` (copied
unmodified from the script's own `mktemp` output directory):

```
10/10 passed
```

10/10 is not below 10/10, so there is nothing to re-run and no failing run
number to report: `grep -n -i fail summary.log` exits 1 (no match) against
the copied `summary.log`, so there are no per-run failing logs to attach.

## Files in this directory

- `summary.log` — copied verbatim from `ci/stability.sh 10`'s own output
  directory (`$(mktemp -d "${TMPDIR:-/tmp}/deck-stability.XXXXXX")`); combined
  log across all 10 runs plus the PASS/FAIL line and final tally for each.
- `summary.log.exitstatus` — `ci/stability.sh`'s own captured exit status
  (`0`), captured atomically in the same backgrounded shell that ran it.
- `poll.log` — the `sleep 120` polling log for this run.
