# Task 024: ten-run stability gate at the final code sha

> **Superseded 2026-09-06 (task cure-02-02).** An independent review found two remaining probe
> gaps after this run (`lookPathIn` accepted a mode-0644 regular file and a FIFO named like an
> agent's binary); approach 02 cured both (`cure-01-01` `7349dd6`, `201` `c8b00cc`, `202`
> `4e09f2d`), moving the phase's final code sha to `4e09f2de90dcde04bd8fc20c77097e593f2fee5b`.
> This report's ten-run measurement at `d88c6625c4ccca71b0d31f7b5864ba030ed39e53` is preserved
> below unchanged as history; it is **not** the gate of record. Task 205's own attempt to re-run
> this gate at the new final code sha ended terminal `failed` (launch-form grounds an operator
> ruling later held were never a rejection basis). The current stability-gate evidence is
> `docs/reports/phase3k-cure-02-01-stability10/` (commit `0bb7a03`, 10/10 passed, exit status 0,
> exact final code sha).

## Command launched (exactly once, from a clean tree)

```
nohup sh -c 'timeout 7200 ci/stability.sh 10 > docs/reports/phase3k-024-stability10/summary.log 2>&1; echo $? > docs/reports/phase3k-024-stability10/summary.log.exitstatus' >/dev/null 2>&1 &
```

Launched disowned/backgrounded at **2026-09-05 23:19:00 UTC**, never blocked
on. `git status --porcelain` was empty immediately before launch (verified in
the same shell call that launched the gate). The gate wrote its last line at
**2026-09-06 00:24:16 UTC** (`stat -c %y summary.log`), i.e. **65.3 minutes**
of wall clock, consistent with the ~66.5-minute figure measured in phase 3j.

## Poll discipline: `sleep 120` only

Every one of the 33 polls slept exactly `sleep 120` before reading the log.
No shorter (or longer) interval was used at any point, including the first
poll. The timestamped poll record is committed beside this README as
`poll.log`; its consecutive `poll at ...` timestamps are ~120 s apart
throughout, which is the check for this claim.

An earlier attempt at this task (commit `c748abb`) recorded a first poll of
`sleep 5` and was rejected for exactly that; this iteration **relaunched the
gate from scratch** on a clean tree and polled with `sleep 120` only. The
`summary.log` / `summary.log.exitstatus` committed here are the new run's,
not that attempt's (the new run's scratch dir is
`/tmp/deck-stability.gPcigR`; the rejected attempt's was
`/tmp/deck-stability.5WgkJO`).

| Poll | Interval slept | Wall clock (UTC) | Last line of `summary.log` at that moment |
|------|----------------|------------------|--------------------------------------------|
| 1 | `sleep 120` | Sat Sep  5 23:21:05 UTC 2026 | `=== RUN 1 ===` (also checked: exitstatus: 0) |
| 2 | `sleep 120` | Sat Sep  5 23:23:10 UTC 2026 | `=== RUN 1 ===` (also checked: exitstatus mtime: 2026-09-05 23:16:50.745117446 +0000) |
| 3 | `sleep 120` | Sat Sep  5 23:25:20 UTC 2026 | `=== RUN 1 ===` |
| 4 | `sleep 120` | Sat Sep  5 23:27:20 UTC 2026 | `=== RUN 2 ===` |
| 5 | `sleep 120` | Sat Sep  5 23:29:20 UTC 2026 | `=== RUN 2 ===` |
| 6 | `sleep 120` | Sat Sep  5 23:31:20 UTC 2026 | `=== RUN 2 ===` |
| 7 | `sleep 120` | Sat Sep  5 23:33:20 UTC 2026 | `=== RUN 3 ===` |
| 8 | `sleep 120` | Sat Sep  5 23:35:23 UTC 2026 | `=== RUN 3 ===` |
| 9 | `sleep 120` | Sat Sep  5 23:37:23 UTC 2026 | `=== RUN 3 ===` |
| 10 | `sleep 120` | Sat Sep  5 23:39:23 UTC 2026 | `=== RUN 4 ===` |
| 11 | `sleep 120` | Sat Sep  5 23:41:23 UTC 2026 | `=== RUN 4 ===` |
| 12 | `sleep 120` | Sat Sep  5 23:43:23 UTC 2026 | `=== RUN 4 ===` |
| 13 | `sleep 120` | Sat Sep  5 23:45:26 UTC 2026 | `=== RUN 5 ===` |
| 14 | `sleep 120` | Sat Sep  5 23:47:26 UTC 2026 | `=== RUN 5 ===` |
| 15 | `sleep 120` | Sat Sep  5 23:49:26 UTC 2026 | `=== RUN 5 ===` |
| 16 | `sleep 120` | Sat Sep  5 23:51:26 UTC 2026 | `=== RUN 5 ===` |
| 17 | `sleep 120` | Sat Sep  5 23:53:26 UTC 2026 | `=== RUN 6 ===` |
| 18 | `sleep 120` | Sat Sep  5 23:55:28 UTC 2026 | `=== RUN 6 ===` |
| 19 | `sleep 120` | Sat Sep  5 23:57:28 UTC 2026 | `=== RUN 6 ===` |
| 20 | `sleep 120` | Sat Sep  5 23:59:28 UTC 2026 | `=== RUN 7 ===` |
| 21 | `sleep 120` | Sun Sep  6 00:01:28 UTC 2026 | `=== RUN 7 ===` |
| 22 | `sleep 120` | Sun Sep  6 00:03:28 UTC 2026 | `=== RUN 7 ===` |
| 23 | `sleep 120` | Sun Sep  6 00:05:31 UTC 2026 | `=== RUN 8 ===` |
| 24 | `sleep 120` | Sun Sep  6 00:07:31 UTC 2026 | `=== RUN 8 ===` |
| 25 | `sleep 120` | Sun Sep  6 00:09:31 UTC 2026 | `=== RUN 8 ===` |
| 26 | `sleep 120` | Sun Sep  6 00:11:31 UTC 2026 | `=== RUN 9 ===` |
| 27 | `sleep 120` | Sun Sep  6 00:13:31 UTC 2026 | `=== RUN 9 ===` |
| 28 | `sleep 120` | Sun Sep  6 00:15:33 UTC 2026 | `=== RUN 9 ===` |
| 29 | `sleep 120` | Sun Sep  6 00:17:33 UTC 2026 | `=== RUN 9 ===` |
| 30 | `sleep 120` | Sun Sep  6 00:19:33 UTC 2026 | `=== RUN 10 ===` |
| 31 | `sleep 120` | Sun Sep  6 00:21:33 UTC 2026 | `=== RUN 10 ===` |
| 32 | `sleep 120` | Sun Sep  6 00:23:33 UTC 2026 | `=== RUN 10 ===` |
| 33 | `sleep 120` | Sun Sep  6 00:25:36 UTC 2026 | `10/10 passed` |

## Final code sha

```
$ git log -1 --format=%H -- '*.go' '*.feature'
d88c6625c4ccca71b0d31f7b5864ba030ed39e53
```

The gate ran against a tree whose HEAD at launch time
(`c748abbd7d1371c1f5df3542a534119edf719f95`) is a docs-only descendant of that
final code sha — no `*.go` or `*.feature` file has changed since `d88c662`.

## Result — quoted verbatim from `summary.log`

```
=== RUN 1 ===
=== RUN 1: PASS (exit 0) ===
=== RUN 2 ===
=== RUN 2: PASS (exit 0) ===
=== RUN 3 ===
=== RUN 3: PASS (exit 0) ===
=== RUN 4 ===
=== RUN 4: PASS (exit 0) ===
=== RUN 5 ===
=== RUN 5: PASS (exit 0) ===
=== RUN 6 ===
=== RUN 6: PASS (exit 0) ===
=== RUN 7 ===
=== RUN 7: PASS (exit 0) ===
=== RUN 8 ===
=== RUN 8: PASS (exit 0) ===
=== RUN 9 ===
=== RUN 9: PASS (exit 0) ===
=== RUN 10 ===
=== RUN 10: PASS (exit 0) ===
full per-run logs and combined summary log kept in: /tmp/deck-stability.gPcigR
10/10 passed
```

`summary.log.exitstatus` reads `0`.

## Failures

None. All ten runs passed. Quoted verbatim from the tracked
`docs/reports/phase3k-024-stability10/summary.log` (the line the gate itself
prints as its final disposition):

```
10/10 passed
```

Because that line reports ten of ten runs passing, this section names no
failing run and copies out no per-run log: there is no per-run FAIL line and no
non-zero per-run exit code anywhere in
`docs/reports/phase3k-024-stability10/summary.log` (every
`=== RUN N: PASS (exit 0) ===` line above comes from that same file, quoted in
full in the "Result" section).

**Pass count, exactly as that file states it: `10/10 passed`.** Ten of ten
runs, no FAIL line, no non-zero per-run exit code — nothing rounded up.

Every path this section cites is tracked in git:

- `docs/reports/phase3k-024-stability10/summary.log` — the combined gate log,
  the authoritative record of every run's PASS/FAIL decision.
- `docs/reports/phase3k-024-stability10/summary.log.exitstatus` — the gate
  command's exit status, `0`.
- `docs/reports/phase3k-024-stability10/poll.log` — the timestamped poll log for
  the gate run.

The scratch directory that `ci/stability.sh` creates with `mktemp -d`, echoed in
the script's own final line quoted in the "Result" section above, is ephemeral
container state and is deliberately not cited as evidence here; no failing run
existed, so nothing from it needed copying into this report directory.
