# Phase 3j — stability10 gate (task 032)

## Supersedes

This refresh supersedes the earlier gate published at code sha
`a44ee320b93186496d56364836b0aed00a6f1e0b`, per task 060 (approach 03): the final
code sha advanced past `a44ee32` with the findings-1-3 fix commits landed after the
earlier gate was published, so this directory is re-run and refreshed **in place**
at the new final code sha — no new numbered report directory. It also replaces the
first approach-03 refresh of this same directory, whose gate procedure was rejected
on a launch-procedure detail (the completion of the background run was inspected
once before the first `sleep 120` poll); the run published below was launched anew
and polled with `sleep 120` intervals only.

## Code sha and HEAD

Final code sha (last commit touching `*.go` or `*.feature`) at the time this gate
was launched:

```
$ git log -1 --format=%H -- '*.go' '*.feature'
b29afb8c4fd8a1cf193c7efef5c5f7e1456481f7
```

`HEAD`/`origin/main` at launch time: `6bb64b29a6043d006dbed266eb4467766d9bbed8`, a
docs-only descendant of `b29afb8` (report refreshes under `docs/reports/` only, no
`*.go`/`*.feature` change) — per the plan's standing rules a docs-only tail commit
does not invalidate a gate. `git status --porcelain` was empty and
`git rev-parse HEAD origin/main` agreed on
`6bb64b29a6043d006dbed266eb4467766d9bbed8` both before this gate launched and after
collection.

This sha supersedes, and is a descendant of, `a44ee320b93186496d56364836b0aed00a6f1e0b`
(the previously published gate sha).

## Command

Launched exactly as this task's own success criteria specify, no other test or
gate run in the same iteration:

```
nohup sh -c 'timeout 7200 ci/stability.sh 10 > /tmp/stability-060.log 2>&1; echo $? > /tmp/stability-060.log.exitstatus' >/dev/null 2>&1 &
```

Launched 2026-09-03T21:53:27Z, polled with `sleep 120` only (never a longer or
shorter interval, and no completion check before the first such poll), no narrowing
of the command. The exit-status file was first observed present at
2026-09-03T23:01:40Z (~68 minutes for 10 runs — each run invokes the whole-suite
sweep `ci/run.sh go test -p=1 -count=1 ./...`, ~6-7 min/run, matching the earlier
gate's cadence).

## Script's own captured exit status

```
$ cat /tmp/stability-060.log.exitstatus
0
```

## Result

The committed `summary.log` in this directory is the script's own summary,
copied byte-for-byte from `ci/stability.sh`'s own tmp dir
`/tmp/deck-stability.Sak9xP/summary.log` (not retyped or reformatted; verified with
`cmp`). Its final line, quoted verbatim, never rounded up:

```
10/10 passed
```

Every one of the 10 runs is labelled `PASS` in that same file (`=== RUN i: PASS
(exit 0) ===` for `i` = 1..10) — no run is labelled `FAIL`, so there is no failing
run to name and no per-run log needs to be published under this task's own
criteria (that clause only applies when N is below 10).

The published number (10/10) is exactly what the script reported; the gate was
not re-run to try to improve or otherwise alter it.
