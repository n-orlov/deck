# Phase 4 approach 3 — ten-run stability sweep

## Sha

Tail code sha: **`a260abfaa36fe96068fb19f4735d66b3b040459b`**
("tui: close the foreign-content-to-deck boundary reset independently of
deck's own colour (task cure-03-01-2)") — the same sha
`docs/reports/phase4-a3-final-suite/README.md` records for the full-suite
gate, and the last commit touching a `*.go` or `*.feature` file. Confirmed at
launch:

```
$ git log --format=%H -1 -- '*.go' '*.feature'
a260abfaa36fe96068fb19f4735d66b3b040459b
$ git rev-parse HEAD
a260abfaa36fe96068fb19f4735d66b3b040459b
```

This recording **supersedes** the three earlier ten-run records written
here, at sha `3568bd7` (commits `5f1fb5c`/`deca67c`), at sha `7bb1f8a`
(commit `c069c34`) and at sha `bfdb69e` (the R121 cure recording, 9/10). The
cure pass landed R118's foreign-content-to-deck boundary reset fix —
`a260abf` — on top of `bfdb69e`, so those earlier sweeps are measurements of
superseded trees. Every log and every table row below is a fresh measurement
of the post-cure tree, not an amendment of an older one.

Tree was clean (`git status --porcelain` empty) at launch.

## What produced this

`ci/stability.sh 10` — ten repetitions of the same unnarrowed whole-tree suite
(`ci/run.sh go test -p=1 -count=1 ./...`, no `-run` filter, no package list,
every package in the module), each from a clean state (`-count=1` disables
Go's test cache; each run's sibling container is `--rm`, so no state leaks
between runs). No narrowing was needed — the full `ci/stability.sh 10` ran to
completion inside the wall-clock budget, so **no features-only fallback was
taken**.

Every one of the ten per-run logs lists all **18** packages `go list ./...`
returns for this module (15 with tests plus `internal/notify`,
`internal/search`, `internal/unit`, each reported `[no test files]`) — the
sweep was unnarrowed in every repetition.

Backgrounded (`nohup /tmp/r118/sweep.sh &`, the driver capturing each
command's own exit status into its own status file immediately after the
un-piped command) and polled per the standing rules, never blocked on
inline. **Repeat count: 10.** Total wall-clock: **launched
2026-09-17T16:17:54Z, `run-1.log` completed 2026-09-17T16:24:54Z, `run-10.log`
and the summary completed 2026-09-17T17:28:08Z — 1h10m14s end to end**,
consistent with the plan-time estimate of ~1h12m and the prior recordings'
~1h12m-1h13m.

Per-run logs (`run-1.log` … `run-10.log`) and the combined `ci/stability.sh`
summary (`stability-summary.log`, its own PASS/FAIL line per run plus the
script's own final `10/10 passed` tally) are committed alongside this
README.

## Result table

| # | log path | completed (UTC) | verdict | failing test / scenario |
|---|----------|-----------------|---------|-------------------------|
| 1 | run-1.log | 16:24:54Z | PASS | — |
| 2 | run-2.log | 16:32:04Z | PASS | — |
| 3 | run-3.log | 16:39:02Z | PASS | — |
| 4 | run-4.log | 16:46:00Z | PASS | — |
| 5 | run-5.log | 16:53:06Z | PASS | — |
| 6 | run-6.log | 17:00:02Z | PASS | — |
| 7 | run-7.log | 17:07:09Z | PASS | — |
| 8 | run-8.log | 17:14:07Z | PASS | — |
| 9 | run-9.log | 17:21:09Z | PASS | — |
| 10 | run-10.log | 17:28:08Z | PASS | — |

**Headline: 10/10 passed — this sweep IS clean.** The tally agrees with the
ten rows above, with `ci/stability.sh`'s own count in `stability-summary.log`
(`10/10 passed`) and with the script's own exit status (0). Every verdict is
read from that repetition's `go test` exit status, captured immediately after
the un-piped command, never from log text.

## The prior sweep's one failing run, no longer applicable here

The `bfdb69e` recording of this same report (superseded above) found run 5
red on `features/mouse.feature:10`'s scenario, via the frame-unchanged
re-check hook at `features/mouse_synthesis_test.go:254` racing the sidebar's
own `starting` → `running` reconcile transition — filed as a FINDING there,
not fixed by this approach (its territory is `features/mouse.feature` and
`features/mouse_synthesis_test.go`, never touched by any commit in this
approach). That race is a probabilistic fixture-timing gap, not a
deterministic regression: it did not reproduce in any of these ten runs. It
remains an OPEN, disclosed finding (`docs/reports/phase4-findings.md`) —
this clean 10/10 does not retract or supersede that finding, it only reports
that this particular sweep did not observe it.

## Known-open advisory flakes

The two known-open advisory flakes named in the standing rules —
`TestSigwinchCountDistinguishesTwoFromThree`
(`features/sigwinch_count_test.go:24`) and `internal/tmux`'s
`TestSendKeysInvalidHexByteIsSilentlyDiscarded` empty-capture case
(`internal/tmux/literal_send_test.go:123`) — were searched for by name in
every one of the ten logs (`grep -rn` for both names across `run-*.log`: no
match). **Neither appears in any of the ten runs.**

"Advisory" describes how this record would *classify* a case, **not** how it
behaves in the suite: both advisory tests report through `t.Fatalf`, and
`ci/stability.sh` labels a repetition from `go test`'s actual exit status, so
either one firing would make that repetition read **FAIL** with the test
named — a FAIL labelled advisory, never a PASS. This sweep had no FAIL at
all.
