# Phase 3j — stability10 gate (task 032)

Run at code sha (final sha touching `*.go`/`*.feature`): `a44ee320b93186496d56364836b0aed00a6f1e0b`
(HEAD/origin main at the time of this run: `b4807ce17a25010b3e4e301d2213e8dfad309b8c`,
a docs-only descendant of the code sha above — confirmed clean tree, `git status --porcelain`
empty, `git rev-parse HEAD origin/main` agreeing on `b4807ce17a25010b3e4e301d2213e8dfad309b8c`
both before launch and after collection).

Command launched exactly as specified by the task:

```
nohup sh -c 'timeout 7200 ci/stability.sh 10 > /tmp/stability-032.log 2>&1; echo $? > /tmp/stability-032.log.exitstatus' >/dev/null 2>&1 &
```

Launched 2026-09-03T15:24:18Z, polled with `sleep 120` only (never a longer interval), no
narrowing of the command. Exit-status file appeared at 2026-09-03T16:31Z (~67 minutes for
10 runs, matching the ~7 min/run estimate).

## Script's own captured exit status

```
$ cat /tmp/stability-032.log.exitstatus
0
```

## Result

`10/10 passed` — every run PASS, no failing run to name.

`summary.log` in this directory is the verbatim combined summary the script itself wrote
(copied byte-for-byte from `ci/stability.sh`'s own `$outdir/summary.log`, tmp dir
`/tmp/deck-stability.ZC7l24`, not retyped or reformatted). Per-run logs `run-1.log`..`run-10.log`
lived alongside `summary.log` in that same tmp directory; since the result is 10/10 (no
failures), no individual failing-run log needs to be published per this task's own criteria.

The published number (10/10) is exactly what the script reported and is not rounded up; the
gate was not re-run to try to improve it.
