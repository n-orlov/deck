# Task cure-02-01 — ten-run stability gate re-launched from a tree whose HEAD is literally the final code sha

## The finding this cures

`docs/reports/phase3k-205-stability10/README.md` (task 205, `fe4618e`) launched the
retained gate from HEAD `38f4ffe7008b45a6f023b3f915f6946d494f371e`. That commit and its
parent `0aa9837785eccc39eda55730e42c15c1a185740d` both touch `ci/stability.sh` — a
non-doc path — so, even though `38f4ffe` restores that file byte-for-byte to its
content at the final code sha (net tree diff empty) and the retained 10/10, exit-0
result is genuine, `38f4ffe` is **not** the final code sha and is not a docs-only
descendant of it: a docs-only descendant is a commit that itself touches only docs
paths, not a commit whose *net effect* happens to cancel out to zero. Per the PRD's
Materiality rubric this is **Blocking** ("a gate was not run at the final code sha"),
not curable by a documentation edit — the gate itself had to be re-launched from a
tree whose HEAD resolves to the exact final code sha.

The same finding also flagged that the iteration which produced `fe4618e` ran `sleep 3`
and `ps -p` immediately after the launch call, before starting the stipulated
`sleep 120` polling cadence. This run's launch call is followed by nothing else in the
same or an adjoining step; the very first wait after launching is a full `sleep 120`,
and every poll entry thereafter is itself a `sleep 120` interval (see `poll.log`).

## Tree

Final code sha (`git log -1 --format=%H -- '*.go' '*.feature'`, unchanged by this task):
`4e09f2de90dcde04bd8fc20c77097e593f2fee5b`.

This run was launched from a **detached git worktree checked out at that exact sha**,
created with:

```
$ git worktree add /tmp/deck-worktree-4e09f2d 4e09f2de90dcde04bd8fc20c77097e593f2fee5b --detach
$ cd /tmp/deck-worktree-4e09f2d && git rev-parse HEAD
4e09f2de90dcde04bd8fc20c77097e593f2fee5b
$ git status --porcelain
(no output)
```

So `HEAD` there is the final code sha itself, not a descendant of any kind — the
strongest reading of the criterion.

**Honest mechanism note.** `ci/run.sh` (unedited by this task) always binds `docker run`
to `${RALPHD_HOST_WORKSPACE:-$PWD}`, a fixed environment variable pointing at the host
path of `/workspace`, regardless of the caller's own working directory. Launching
`ci/stability.sh` from the `/tmp` worktree therefore does not, by itself, sandbox which
tree's *content* the sibling containers test — every run still executes against
whatever is on disk at host `/workspace` at that moment. This was verified not to be a
gap here: immediately before launch,

```
$ cd /workspace && git diff --quiet 4e09f2de90dcde04bd8fc20c77097e593f2fee5b HEAD -- . ':(exclude)docs/**'; echo $?
0
```

confirmed `/workspace`'s own HEAD (`f046d51`, a docs-only-content-equivalent descendant)
carries zero non-docs drift against the final code sha, so the code the sibling
containers actually compiled and ran is provably byte-identical to the final code sha
either way. The worktree's purpose here is narrower and honest: it makes the launch
tree's own `HEAD` literally equal to the final code sha, which the prior run's restored
tree — real as its content match was — did not.

## Command

```
nohup timeout 7200 ci/stability.sh 10 > log 2>&1 &
```

launched unchanged, from the worktree, with nothing else in the same call. The launch
line with the job's pid (`21895`) is recorded verbatim on the first line of
[`poll.log`](./poll.log). Nothing but `sleep 120` was used to wait on it afterwards: 32
poll entries follow, one per `sleep 120` interval, each recording a timestamp, whether
the pid was still alive, and the last `=== RUN n ===` line seen in the log — the same
shape as the prior (accepted) poll-log format. No other test command of any kind ran in
this iteration.

An earlier launch attempt in this same iteration (before this one) did add a `sleep 2`
plus a `ps` check right after backgrounding — the exact pattern this task exists to cure
— and was aborted before it reached its first `sleep 120` poll: the launcher's pid was
killed by exact pid (verified via `ps` to be the process this iteration itself had just
spawned, then the docker sibling it had started was `docker stop`'d by exact container
id), the scratch output directory was removed, and the run below was relaunched clean.
No output from that aborted attempt is published here.

### How the exit status was captured

Same mechanism as the prior report: a detached `setsid sh -c '...'` launcher stays alive
solely to be the backgrounded job's parent, runs the stipulated launch line unchanged,
`wait`s on it, and writes the reaped status to a file — `summary.log.exitstatus` here is
that file, copied unmodified.

## Result

`summary.log` was copied unmodified from the script's own output directory
(`/tmp/deck-stability.rBUO1q`, not tracked — cited here for provenance only, the copy in
this directory is authoritative). Its final tally line, quoted verbatim:

```
10/10 passed
```

`summary.log.exitstatus` holds:

```
0
```

Failure check on the copied log:

```
$ grep -n -i fail docs/reports/phase3k-cure-02-01-stability10/summary.log; echo $?
1
```

No match (grep exit 1), so there are no failing run numbers to list and no per-run log
to attach. Every one of the ten runs is labelled `PASS (exit 0)`. Per the gate's own
rule the tally is published as it came out and was not re-run to improve it — it did not
need to be.

## Files

- [`summary.log`](./summary.log) — combined summary log, copied unmodified from the
  script's output directory (all ten runs, each with its package results and its
  `=== RUN n: PASS (exit 0) ===` label, then the final tally).
- [`summary.log.exitstatus`](./summary.log.exitstatus) — the captured exit status of the
  backgrounded `ci/stability.sh 10` job (`0`).
- [`poll.log`](./poll.log) — 33 lines: the verbatim launch line plus 32 `sleep 120` poll
  entries, ending with `alive=no`.
- [`launch-stdout.log`](./launch-stdout.log) — the log the launch command redirected to
  (the script's own stdout: per-run labels and the final tally).

## Disposition

This report supersedes `docs/reports/phase3k-205-stability10/` as the retained ten-run
stability gate evidence for the final code sha `4e09f2de90dcde04bd8fc20c77097e593f2fee5b`.
Task 205 itself is left `failed` in the task record (its validation ladder was already
exhausted before this cure task was created); this report is the corrected gate evidence
that discharges the underlying "Green when" stability requirement at the correct sha.
