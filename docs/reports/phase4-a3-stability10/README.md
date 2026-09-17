# Phase 4 approach 3 — ten-run stability sweep

## Sha

Tail code sha: **`bfdb69ecd6a00f4dc79475c0fb721453b0bd17b7`**
("tui: point R121's override controls at their own distinct roots (task
cure-03-02-2)") — the same sha `docs/reports/phase4-a3-final-suite/README.md`
records for the full-suite gate, and the last commit touching a `*.go` or
`*.feature` file. Confirmed at launch:

```
$ git log --format=%H -1 -- '*.go' '*.feature'
bfdb69ecd6a00f4dc79475c0fb721453b0bd17b7
$ git rev-parse HEAD
bfdb69ecd6a00f4dc79475c0fb721453b0bd17b7
```

This recording **supersedes** the two earlier ten-run records written here, at
sha `3568bd7` (commits `5f1fb5c`/`deca67c`) and at sha `7bb1f8a` (commit
`c069c34`). The cure pass landed R121's environment-layering fix — `594b0b4`
("resolve transcript server env from the actual tmux server, not the
observer's ambient env") and `bfdb69e` (its override controls pointed at their
own distinct roots), both `*.go` — on top of `7bb1f8a`, so those earlier
sweeps are measurements of superseded trees. Every log and every table row
below is a fresh measurement of the post-cure tree, not an amendment of an
older one.

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

Backgrounded (`nohup timeout 7200 <driver> &`) and polled per the standing
rules, never blocked on inline. **Repeat count: 10.** Total wall-clock:
**launched 2026-09-17T14:37:45Z, `run-1.log` completed 2026-09-17T14:44:47Z,
`run-10.log` and the summary completed 2026-09-17T15:50:34Z — 1h12m49s end to
end**, matching the plan-time estimate of ~1h12m.

Per-run logs (`run-1.log` … `run-10.log`) and the combined `ci/stability.sh`
summary (`stability-summary.log`, its own PASS/FAIL line per run plus the
script's own final `9/10 passed` tally) are committed alongside this README.

## Result table

| # | log path | completed (UTC) | verdict | failing test / scenario |
|---|----------|-----------------|---------|-------------------------|
| 1 | run-1.log | 14:44:47Z | PASS | — |
| 2 | run-2.log | 14:51:51Z | PASS | — |
| 3 | run-3.log | 14:58:56Z | PASS | — |
| 4 | run-4.log | 15:06:05Z | PASS | — |
| 5 | run-5.log | 15:13:34Z | **FAIL** | `TestFeatures` → scenario *a single click on a sidebar row selects it and enters interactive mode on the same press, and Ctrl+Q returns to the list* (`features/mouse.feature:10`), in its after-scenario frame-unchanged hook (`features/mouse_synthesis_test.go:254`) |
| 6 | run-6.log | 15:20:42Z | PASS | — |
| 7 | run-7.log | 15:28:24Z | PASS | — |
| 8 | run-8.log | 15:35:28Z | PASS | — |
| 9 | run-9.log | 15:42:45Z | PASS | — |
| 10 | run-10.log | 15:50:33Z | PASS | — |

**Headline: 9/10 passed — this sweep is NOT clean.** The tally agrees with the
ten rows above, with `ci/stability.sh`'s own count in
`stability-summary.log` (`9/10 passed`, `=== RUN 5: FAIL (exit 1) ===` at line
6651) and with the script's own non-zero exit status (1). Every verdict is read
from that repetition's `go test` exit status, captured immediately after the
un-piped command, never from log text.

## The one failing run, named in full

`run-5.log` (line 6350 onward): `--- FAIL: TestFeatures (367.86s)`, one
scenario of 355 (`355 scenarios (354 passed, 1 failed)`, `4199 steps (4196
passed, 1 failed, 2 skipped)`), all other 17 packages `ok` in that same run.

The failing step is the scenario's last assertion,
`deck client "A" frame still matches the captured "click-entered-bravo" frame`
(`features/mouse.feature:31`): the after-scenario hook re-enters interactive
mode and diffs the frame against the one captured at
`features/mouse.feature:26`. The only difference between `want` and `got` in
the log is the sidebar's own status word for the second session:

```
want:  | > click-enter-bravo starting     |
got:   | > click-enter-bravo running      |
```

That is deck's ordinary reconcile transition `starting` → `running` landing
between the capture and the re-check, not a paint, layout, colour, gutter,
crop or interactive-mode difference. The scenario's own wait step
(`features/mouse.feature:20`, "within one configured reconcile interval deck
client A screen contains 'running'") is satisfied by the FIRST session's
status word, so the second session can still read `starting` at capture time —
the race is in the fixture's wait, in test code, and is independent of this
approach's cures (its territory is `features/mouse.feature` and
`features/mouse_synthesis_test.go`; this approach's code commits touched
`internal/tui/*.go`, `internal/tmux/geometry.go` and
`features/real_agent_hooks_test.go`).

It is filed as a FINDING (not fixed by this approach, whose scope is review
pass 234's reds plus the R121 cure) — see `docs/reports/phase4-findings.md`
and this record commit's own message:

```
FINDING: features/mouse.feature:20 waits for "running" on any row, so the
click-enter-bravo capture at features/mouse.feature:26 can be taken while
that row still reads "starting", making the frame-unchanged re-check at
features/mouse.feature:31 (hook at features/mouse_synthesis_test.go:254)
fail ~1 run in 10.
```

The nine other repetitions of the same unnarrowed suite pass at this same sha,
so the product contract this sweep measures holds in 9 of 10 runs and the one
red is this fixture race, reported here rather than hidden or re-run away. No
run was re-run, discarded or replaced.

## Known-open advisory flakes

The two known-open advisory flakes named in the standing rules —
`TestSigwinchCountDistinguishesTwoFromThree`
(`features/sigwinch_count_test.go:24`) and `internal/tmux`'s
`TestSendKeysInvalidHexByteIsSilentlyDiscarded` empty-capture case
(`internal/tmux/literal_send_test.go:123`) — were searched for by name in every
one of the ten logs (`grep -rn` for both names across `run-*.log`: no match).
**Neither appears in any of the ten runs**, so neither is the cause of run 5's
red; the mouse-gesture race above is a *third*, distinct case, and it is named
here rather than absorbed into the advisory label.

"Advisory" describes how this record would *classify* a case, **not** how it
behaves in the suite: both advisory tests report through `t.Fatalf`, and
`ci/stability.sh` labels a repetition from `go test`'s actual exit status, so
either one firing would make that repetition read **FAIL** with the test
named — a FAIL labelled advisory, never a PASS. Run 5 is labelled exactly that
way here: FAIL, with its scenario named.
