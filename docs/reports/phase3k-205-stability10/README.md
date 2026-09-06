# Task 205 — ten-run stability gate at the new final code sha

> **Superseded.** The retained gate for the final code sha
> `4e09f2de90dcde04bd8fc20c77097e593f2fee5b` is now
> [`docs/reports/phase3k-cure-02-01-stability10/`](../phase3k-cure-02-01-stability10/README.md).
> This run's launch `HEAD` (`38f4ffe7008b45a6f023b3f915f6946d494f371e`) restores identical
> tree content but is not the final code sha or a docs-only descendant of it (task
> cure-02-01's finding) — kept here as history, not as current gate evidence.

Final code sha (`git log -1 --format=%H -- '*.go' '*.feature'`):
`4e09f2de90dcde04bd8fc20c77097e593f2fee5b`

HEAD at the time this gate ran: `38f4ffe7008b45a6f023b3f915f6946d494f371e`.
That commit restores `ci/stability.sh` byte-for-byte to its content at the final
code sha, so the tree the gate ran from carries **no non-docs drift at all**
against `4e09f2de90dcde04bd8fc20c77097e593f2fee5b`:

```
$ git diff --stat 4e09f2de90dcde04bd8fc20c77097e593f2fee5b..38f4ffe7008b45a6f023b3f915f6946d494f371e -- . ':(exclude)docs/**'
(no output)
$ git diff --quiet 4e09f2de90dcde04bd8fc20c77097e593f2fee5b 38f4ffe7008b45a6f023b3f915f6946d494f371e -- . ':(exclude)docs/**'; echo $?
0
```

`git status --porcelain` was empty and `git rev-parse HEAD origin/main` printed
the same sha immediately before the launch. The gate never writes into the
workspace tree — it only invokes `go test` inside a throwaway sibling container
— so the tree stayed clean for the whole run.

An earlier attempt at this gate ran from a tree in which `ci/stability.sh`
carried an added `EXIT` trap (commit `0aa9837785eccc39eda55730e42c15c1a185740d`).
That is non-docs drift against the final code sha, so this run was made again
from the restored tree, and the exit status is captured outside the script
instead (see below).

## Command

Launched from the clean tree in `/workspace`, exactly as the gate specifies —
`nohup` directly on the `timeout` invocation, output redirected to a log,
backgrounded:

```
nohup timeout 7200 ci/stability.sh 10 > log 2>&1 &
```

The launch line, with the absolute scratch redirect target actually used and the
job's pid (19699), is recorded verbatim on the first line of
[`poll.log`](./poll.log). The redirect target was an absolute scratch path so
nothing landed in the workspace tree; that captured stdout is preserved here as
[`launch-stdout.log`](./launch-stdout.log).

Nothing but `sleep 120` was used to wait on it: 32 poll entries follow the launch
line in [`poll.log`](./poll.log), one per `sleep 120` interval, each recording a
timestamp, whether the pid was still alive, and the last `=== RUN n ===` line
seen in the log. **No other test command of any kind ran in this iteration** — no
targeted package run, no self-test, no second suite run.

### How the exit status was captured

The launch command above is left exactly as specified, so it is not wrapped in
`bash -c '...; echo $? > file'`. A `wait` issued from any later shell cannot
report the truth either: each tool call is a fresh shell and is never the job's
parent, so it returns 127 once the job has exited. Instead the backgrounded job
was started by a detached launcher shell (`setsid sh -c '...'`) that stays alive
solely to be the job's parent: it runs the stipulated `nohup timeout 7200
ci/stability.sh 10 > log 2>&1 &` line unchanged, then `wait`s on that pid and
writes the reaped status to a file. `summary.log.exitstatus` in this directory is
that file, copied unmodified. It is the status of the `timeout 7200
ci/stability.sh 10` job itself, which is `ci/stability.sh`'s own exit status
whenever `timeout` did not fire (a timeout would have shown as `124`).

## Result

`summary.log` was copied unmodified from the script's own output directory, and
its final tally line, quoted verbatim, is:

```
10/10 passed
```

`summary.log.exitstatus` — the script's captured exit status — holds:

```
0
```

Failure check on the copied log:

```
$ grep -n -i fail docs/reports/phase3k-205-stability10/summary.log
$ echo $?
1
```

No match (grep exit 1), so there are no failing run numbers to list and no
per-run log to attach. Every one of the ten runs is labelled `PASS (exit 0)` in
`summary.log`. Per the gate's own rule, the tally above is published as it came
out and was not re-run to improve it — it did not need to be.

## Files

- [`summary.log`](./summary.log) — combined summary log, copied unmodified from
  the script's output directory (all ten runs, each with its package results and
  its `=== RUN n: PASS (exit 0) ===` label, then the final tally).
- [`summary.log.exitstatus`](./summary.log.exitstatus) — the captured exit status
  of the backgrounded `ci/stability.sh 10` job (`0`).
- [`poll.log`](./poll.log) — 33 lines: the verbatim launch line plus 32 `sleep
  120` poll entries, ending with `alive=no; log_tail=10/10 passed`.
- [`launch-stdout.log`](./launch-stdout.log) — the log the launch command
  redirected to (the script's own stdout: per-run labels and the final tally).
