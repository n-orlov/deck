# Task 210 (F1) — stability measurement: 10/10 at final code commit `f715a56`

## Result

`ci/stability.sh 10` run at the current final code commit **`f715a56`** (`features: fix
delete-undo/restore DB-read race in kill_delete_undo.feature (210)`, the newest commit on the
branch at the time this measurement was taken — `git log origin/main..HEAD` was empty before and
remains empty after this docs-only commit is added on top).

**10/10 PASS.** Every one of the 14 testable packages reported `ok` in every one of the 10 runs
(`internal/notify`, `internal/search`, `internal/unit` have no test files and are excluded from
that count); zero `FAIL` lines anywhere across all 10 run logs. `features` alone ranged
5.0–5.3 minutes per run; every other package together is a few tens of seconds.

```
$ for f in run-*.log; do echo "$f: $(grep -c '^ok' $f) ok, $(grep -c FAIL $f) FAIL"; done
run-1.log: 14 ok, 0 FAIL     run-6.log:  14 ok, 0 FAIL
run-2.log: 14 ok, 0 FAIL     run-7.log:  14 ok, 0 FAIL
run-3.log: 14 ok, 0 FAIL     run-8.log:  14 ok, 0 FAIL
run-4.log: 14 ok, 0 FAIL     run-9.log:  14 ok, 0 FAIL
run-5.log: 14 ok, 0 FAIL     run-10.log: 14 ok, 0 FAIL
```

This satisfies task 210's (F1) first success-criteria branch directly: "a committed
`ci/stability.sh 10` measurement of 10/10 taken after the (possibly new) final code commit — with
the per-run logs, summary.log and the load trace committed". No root-cause writeup is needed for
any residual failure because there was none — this is the clean-streak branch of the criteria, not
the every-remaining-failure branch (that branch's five prior sub-findings, (1) R-restart audit race
`ebb4869`, (2a) golden-frame settle race `029893a`, (b) tmux pipe-displacement chatter race
`3a26ccb`, (c) ghost-completion starting race — host load, no fix `ea44a30`, (e) delete-undo restore
race `f715a56` — remain the record of *how* this streak was reached, not a substitute for it; see
each commit's own `docs/reports/phase3d-210-*.md` writeup).

## Load trace (host `uptime` sampled repeatedly across the run, this container's own polling)

| wall time | 1-min | 5-min | 15-min | run state at sample |
|---|---|---|---|---|
| 08:20:45 | 4.26 | 3.06 | 2.63 | before launch |
| 08:24:58 | 8.82 | 5.53 | 3.66 | run 1 in progress |
| 08:29:02 | 3.53 | 4.60 | 3.73 | run 1 PASS, run 2 started |
| 08:33:47 | 7.88 | 6.62 | 4.79 | run 2 PASS, run 3 started |
| 08:38:31 | 8.76 | 7.41 | 5.54 | run 3 in progress |
| 08:43:14 | 8.33 | 6.91 | 5.82 | run 3 PASS, run 4 started |
| 08:47:58 | 7.73 | 7.63 | 6.41 | run 4 PASS, run 5 started |
| 08:52:41 | 3.22 | 5.59 | 5.93 | run 5 PASS, run 6 started |
| 08:57:23 | 6.81 | 6.11 | 5.99 | run 6 PASS, run 7 started |
| 09:02:06 | **14.81** | 8.81 | 7.00 | run 7 in progress (this run's peak) |
| 09:06:49 | 7.59 | 7.04 | 6.57 | run 7 PASS, run 8 started |
| 09:11:32 | 2.56 | 4.34 | 5.54 | run 8 PASS, run 9 started |
| 09:16:15 | 1.42 | 2.53 | 4.44 | run 9 PASS, run 10 started |
| 09:20:57 | 2.33 | 2.50 | 3.91 | run 10 PASS, script exits |

1-min loadavg stayed under 15 for the entire ~60-minute window (peak 14.81 during run 7, far below
the 80–176 spikes seen during several of task 217's whole-suite attempts) — this measurement was
taken at genuinely low, stable host load, per steer 019's own standing preference for an honest
measurement over a manufactured one taken under favorable conditions it can't actually claim.
`docker ps` was checked at the start of the run and confirmed only pre-existing non-self containers
alongside this run's own sibling; none were touched.

## Files in this directory

- `summary.log` — the combined `ci/stability.sh` output (all 10 `=== RUN N ===`/PASS markers plus
  each run's full `go test` output inline, exactly as the script produced it).
- `run-1.log` … `run-10.log` — each run's own `go test -p=1 -count=1 ./...` output in isolation.
- This `README.md`.

## Provenance

Command: `ci/stability.sh 10` (unmodified, in-repo script, `ci/run.sh`-driven per-run sibling
containers, `--rm` per invocation per the script's own header comment). Run from `/workspace` at
commit `f715a56`. `git status --short` was empty before, during (no code touched while the
background run executed), and after (aside from adding this directory).
