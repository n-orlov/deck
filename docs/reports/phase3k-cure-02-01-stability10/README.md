# Task cure-02-01 — ten-run stability gate re-launched from a tree whose HEAD is literally the final code sha

Re-launched 2026-09-06 (this report replaces this task's own first attempt, whose only
defect was that its prose called an abbreviated redirect target "verbatim"; see
"Launch command" below).

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
`sleep 120` polling cadence. In the run recorded here the launch call is the last thing
in its own step; the first wait after it is a full `sleep 120`, every later poll entry is
another `sleep 120` interval, and no `ps`, `pgrep` or other liveness probe was used at
any point — each poll reads only the launcher's `status` file and the log's last
`=== RUN n ===` line (see `poll.log`).

## Tree

Final code sha (`git log -1 --format=%H -- '*.go' '*.feature'`, unchanged by this task):
`4e09f2de90dcde04bd8fc20c77097e593f2fee5b`.

This run was launched from a **detached git worktree checked out at that exact sha**,
created with:

```
$ git worktree add /tmp/deck-wt-cure0201b 4e09f2de90dcde04bd8fc20c77097e593f2fee5b --detach
Preparing worktree (detached HEAD 4e09f2d)
HEAD is now at 4e09f2de service: pin create preflight against FIFO named like agent binary (task 202)
$ cd /tmp/deck-wt-cure0201b && git rev-parse HEAD
4e09f2de90dcde04bd8fc20c77097e593f2fee5b
$ git status --porcelain
(no output)
```

So `HEAD` there is the final code sha itself, not a descendant of any kind — the
strongest reading of the criterion. The worktree lives outside the repository tree and
was removed afterwards, so nothing about it is (or should be) tracked here.

**Honest mechanism note.** `ci/run.sh` (unedited by this task) always binds `docker run`
to `${RALPHD_HOST_WORKSPACE:-$PWD}`, a fixed environment variable pointing at the host
path of `/workspace`, regardless of the caller's own working directory. Launching
`ci/stability.sh` from the worktree therefore does not, by itself, sandbox which tree's
*content* the sibling containers test — every run still executes against whatever is on
disk at host `/workspace` at that moment. This was verified not to be a gap here: in the
same call as the launch, immediately before it,

```
$ cd /workspace; git diff --quiet 4e09f2de90dcde04bd8fc20c77097e593f2fee5b HEAD -- . ':(exclude)docs/**'; echo $?
0
```

confirmed `/workspace`'s own HEAD (`3eaeecc`, a docs-only descendant of the final code
sha) carries zero non-docs drift against the final code sha, so the code the sibling
containers actually compiled and ran is provably byte-identical to the final code sha
either way. That check's result is recorded on the first line of `poll.log`, beside the
launch line. The worktree's purpose here is narrower and honest: it makes the launch
tree's own `HEAD` literally equal to the final code sha, which the prior run's restored
tree — real as its content match was — did not.

## Launch command

The inner launch line, quoted exactly as it was executed, character for character:

```
nohup timeout 7200 ci/stability.sh 10 > log 2>&1 &
```

The redirect target is the bare relative path `log`, in the worktree's own working
directory (`ci/stability.sh` is likewise the bare relative path, resolved from that same
directory) — no directory prefix, nothing abbreviated in this quotation and nothing
rewritten in the execution. That file is published unmodified here as
[`launch-stdout.log`](./launch-stdout.log). The line appears verbatim on the first line
of [`poll.log`](./poll.log) as well.

**Why the first attempt of this task was rejected, and what changed.** The first attempt
(commit `3eaeecc`) executed
`nohup timeout 7200 ci/stability.sh 10 > /tmp/deck-stability-cure0201/log 2>&1 &` — the
same command with an absolute redirect target — while its report quoted the short form
and called it "launched unchanged". The result it measured was sound, but the quotation
was not accurate, so that gate is not the evidence of record; the run documented here
replaces it and uses the literal short form, with the working directory (not a path
prefix) placing `log` in the worktree.

### How the exit status was captured

Same mechanism as the prior reports: a detached `setsid sh -c '...'` launcher stays alive
solely to be the backgrounded job's parent, runs the stipulated launch line unchanged,
`wait`s on it, and writes the reaped status to a file. The launcher, in full:

```
setsid sh -c 'trap "" HUP; cd /tmp/deck-wt-cure0201b; nohup timeout 7200 ci/stability.sh 10 > log 2>&1 & pid=$!; wait "$pid"; echo $? > status' </dev/null >/dev/null 2>&1 &
```

`summary.log.exitstatus` in this directory is that `status` file, copied unmodified.

### One earlier call in the same iteration, for the record

Before the launch above, one shell call in this iteration attempted to create the
worktree and launch in a single `&&` chain; the trailing `&` backgrounded the *whole*
chain, which the harness then terminated when the call returned, part-way through
`git worktree add`. No gate process was started by it (no new `/tmp/deck-stability.*`
directory appeared, no sibling container was created, nothing was signalled or killed),
`git worktree prune` restored a clean state, and the worktree was then created in a call
of its own. Nothing from that call is published here because it produced no measurement.

## Result

`summary.log` was copied unmodified from the script's own output directory
(`/tmp/deck-stability.bTLi2p`, ephemeral scratch, not tracked — cited for provenance
only; the copy in this directory is authoritative). Its final tally line, quoted
verbatim:

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

No match (grep exit 1), so there are no failing run numbers to list and no per-run log to
attach. All ten runs are labelled `PASS (exit 0)` (`grep -c 'PASS (exit 0)'` → 10). Per
the gate's own rule the tally is published as it came out and was not re-run to improve
it — it did not need to be.

## Files

- [`summary.log`](./summary.log) — combined summary log, copied unmodified from the
  script's output directory (all ten runs, each with its package results and its
  `=== RUN n: PASS (exit 0) ===` label, then the final tally).
- [`summary.log.exitstatus`](./summary.log.exitstatus) — the captured exit status of the
  backgrounded `ci/stability.sh 10` job (`0`).
- [`poll.log`](./poll.log) — 35 lines: the verbatim launch line plus 34 poll entries, one
  per `sleep 120` interval, the last one recording `status_file=0` and
  `=== RUN 10: PASS (exit 0) ===`.
- [`launch-stdout.log`](./launch-stdout.log) — the `log` file the launch command
  redirected to (the script's own stdout: per-run labels and the final tally).

## Timeline

Launched 2026-09-06T07:08:33Z, finished by 2026-09-06T08:16:48Z (~68 min for ten runs,
in line with the measured ~66.5 min), all within one iteration with no other test command
of any kind run in it.

## Disposition

This report supersedes `docs/reports/phase3k-205-stability10/` **and this task's own
first attempt (`3eaeecc`)** as the retained ten-run stability gate evidence for the final
code sha `4e09f2de90dcde04bd8fc20c77097e593f2fee5b`. Task 205 itself is left `failed` in
the task record (its validation ladder was already exhausted before this cure task was
created); this report is the corrected gate evidence that discharges the underlying
"Green when" stability requirement at the correct sha.
