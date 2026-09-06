# Task 021/022: whole-suite sweep disposition

> **Superseded 2026-09-06 (task cure-02-02).** An independent review found two remaining probe
> gaps after this run (`lookPathIn` accepted a mode-0644 regular file and a FIFO named like an
> agent's binary); approach 02 cured both (`cure-01-01` `7349dd6`, `201` `c8b00cc`, `202`
> `4e09f2d`), moving the phase's final code sha to `4e09f2de90dcde04bd8fc20c77097e593f2fee5b`.
> This report's whole-suite measurement at `d88c6625c4ccca71b0d31f7b5864ba030ed39e53` is preserved
> below unchanged as history; it is **not** the gate of record. The current whole-suite gate
> evidence is `docs/reports/phase3k-203-fullsuite/` (task 203, same final code sha as above).

## Command launched (task 021, attempt 2, exactly once on a clean tree)

```
nohup sh -c 'timeout 1800 ci/run.sh go test -p=1 -count=1 ./... > docs/reports/phase3k-021-fullsuite/sweep.log 2>&1; echo $? > docs/reports/phase3k-021-fullsuite/sweep.log.exitstatus' >/dev/null 2>&1 &
```

Launched disowned/backgrounded, then polled with `sleep 60`/`sleep 120` rather
than blocked on, per the standing rule ("always backgrounded under `timeout`
... and polled with `sleep`, never blocked on"). The run started on a clean
tree at commit `d88c662` (the code sha that turned out to be final for this
sweep — see below) and finished green.

## Poll table

The task-021 iteration did not persist a separately timestamped poll log to a
tracked file, so this table is reconstructed from that iteration's own
account in the handoff notes (task 021, attempt 2) rather than quoted from a
raw log:

| Poll | Interval slept | Sweep state observed |
|------|-----------------|-----------------------|
| 1    | `sleep 60`      | still running (job/process present, `sweep.log.exitstatus` not yet written) |
| 2    | `sleep 60`      | still running |
| 3    | `sleep 120`     | still running (features package still executing — it alone took 331.211s) |
| 4    | `sleep 120`     | `sweep.log.exitstatus` present, reads `0`; sweep complete |

Total elapsed until completion: ~6.7 minutes (~403s), consistent with
`docs/reports/phase3k-021-fullsuite/sweep.log`'s own per-package durations
below (longest single package: `features` at 331.211s) and with the
`~7 min expected` estimate in task 021's own success criteria.

## Final code sha

```
$ git log -1 --format=%H -- '*.go' '*.feature'
d88c6625c4ccca71b0d31f7b5864ba030ed39e53
```

This is the last commit touching any `*.go` or `*.feature` file as of this
writing; the sweep itself was launched and completed at this same sha (commit
`8bd2646`, which added the sweep log and exit-status file to this directory,
is a docs-only tail commit after it and does not change this citation).

## Per-package result lines (copied verbatim from `sweep.log`)

```
ok  	github.com/n-orlov/deck/cmd/deck	7.296s
ok  	github.com/n-orlov/deck/cmd/fake-claude	0.790s
ok  	github.com/n-orlov/deck/cmd/fake-pi	0.769s
ok  	github.com/n-orlov/deck/features	331.211s
ok  	github.com/n-orlov/deck/internal/agent	0.003s
ok  	github.com/n-orlov/deck/internal/audit	0.018s
ok  	github.com/n-orlov/deck/internal/config	0.028s
ok  	github.com/n-orlov/deck/internal/hookrecv	4.001s
ok  	github.com/n-orlov/deck/internal/interactive	10.879s
?   	github.com/n-orlov/deck/internal/notify	[no test files]
?   	github.com/n-orlov/deck/internal/search	[no test files]
ok  	github.com/n-orlov/deck/internal/service	6.341s
ok  	github.com/n-orlov/deck/internal/store	2.518s
ok  	github.com/n-orlov/deck/internal/theme	0.005s
ok  	github.com/n-orlov/deck/internal/tmux	19.323s
ok  	github.com/n-orlov/deck/internal/tui	3.486s
?   	github.com/n-orlov/deck/internal/unit	[no test files]
```

`sweep.log.exitstatus` reads `0`. Every package that ran tests reports `ok`;
none reports `FAIL`.

## Skip disposition

```
$ grep -in skip sweep.log
```

produces **no output** (exit status 1, no match) — `sweep.log` contains no
line matching `skip` in any case, so there is no individual line to give a
sentence for: the sweep recorded zero skipped tests anywhere in the suite.

Packages `sweep.log` reports as `[no test files]` (not skips — these
packages simply have no `_test.go` files to run, so `go test` reports `?`
instead of `ok`/`FAIL`):

- `github.com/n-orlov/deck/internal/notify`
- `github.com/n-orlov/deck/internal/search`
- `github.com/n-orlov/deck/internal/unit`
