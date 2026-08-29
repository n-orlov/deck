# Phase 3g task 512 — `ci/stability.sh 10`, literal launcher, correct metadata

## Why this directory exists

Task 505 measured `ci/stability.sh 10` at the post-501/502/503 tree honestly
(9/10, sole failure `TestSigwinchCountDistinguishesTwoFromThree`), but its
round-2 report was rejected on an **execution-shape** residual, not on the
measured substance:

- `/run/ralphd/artifacts/stability-505.nohup` is a 0-byte file left behind by
  the actual launch, proving an extra redirection was added to the pipeline
  at launch time — while `docs/reports/phase3g-505-stability10/README.md`
  claimed the launch command was used unmodified.
- That README also stated `summary.log` is 891 628 bytes; `stat`/`wc -c`
  report 891 714 bytes.

This directory redoes the capture with the launcher issued **verbatim**, and
every size/line-count claim below is checked against the committed file in
the same command that states it. It supersedes `docs/reports/
phase3g-505-stability10/` for the same reason round 2 there superseded its
own round 1: execution-shape integrity, not because either directory's
numbers were found untrue. `phase3g-505-stability10/` is left as-is.

## Launch

- **Launch commit** (`git rev-parse HEAD`, captured in the same shell call as
  the launch below): `bfa3aad39966349d53989500a58837147060d600`.
- **Launch command**, issued verbatim, no added redirection anywhere in the
  pipeline:

  ```
  nohup sh -c 'timeout 7200 ci/stability.sh 10 > /run/ralphd/artifacts/stability-512.log 2>&1; s=$?; printf "%s\n" "$s" > /run/ralphd/artifacts/stability-512.exitstatus' &
  ```

- **No stray artifact was created**: `ls /run/ralphd/artifacts/ | grep 512`
  after completion lists exactly `stability-512.log` and
  `stability-512.exitstatus` — no `stability-512.nohup` or any other
  unexpected file, unlike the round that produced 505's rejected report.
- Between the launch and the driver exiting, the only commands issued were
  the prescribed polls: `sleep 300; grep '^=== RUN'
  /run/ralphd/artifacts/stability-512.log`, repeated. Nothing else touched
  the machine while the suite ran.
- **Script exit status** (captured with `s=$?` in the same shell call that
  ran it, never through a pipe): `0` — see `script.exitstatus`
  (`cat script.exitstatus` reads `0`).
- Wall clock: launched 2026-08-29 09:24:52 UTC (the shell call that captured
  `git rev-parse HEAD` and launched the driver); `stability-512.exitstatus`'s
  mtime is 2026-08-29 10:24:53 UTC, so the ten runs took **60m01s** end to
  end. Those two timestamps are the only wall-clock facts claimed here;
  no committed file records per-run start/end times.

## Observed rate, quoted verbatim from `summary.log`

```
full per-run logs and combined summary log kept in: /tmp/deck-stability.VRvJVc
10/10 passed
```

**10/10.** Every one of the ten runs' `go test -p=1 -count=1 ./...` exited 0
with no `--- FAIL` line in any per-run log (`grep -l FAIL run-*.log` in this
directory matches nothing).

## Per-run PASS/FAIL table

PASS/FAIL comes from `driver.log`'s `=== RUN n: PASS/FAIL (exit N) ===`
markers, cross-checked against each `run-N.log`'s own `ok` package lines (no
`FAIL` line appears in any of the ten). `driver.log` carries no timestamps,
so no per-run wall clock can be read from it; the time column is instead each
run's own summed package test time, derived from that run's tracked
`run-N.log` (`go test -p=1` runs packages serially, so the sum is the run's
test time excluding that run's container start and Go build).

| Run | Result | Test time, summed from `run-N.log`'s package durations | Per-run log  |
|-----|--------|----------------------------------------------------------|--------------|
| 1   | PASS   | 357.7s (5m57s)                                            | `run-1.log`  |
| 2   | PASS   | 356.1s (5m56s)                                            | `run-2.log`  |
| 3   | PASS   | 358.5s (5m58s)                                            | `run-3.log`  |
| 4   | PASS   | 354.8s (5m54s)                                            | `run-4.log`  |
| 5   | PASS   | 359.1s (5m59s)                                            | `run-5.log`  |
| 6   | PASS   | 356.8s (5m56s)                                            | `run-6.log`  |
| 7   | PASS   | 358.9s (5m58s)                                            | `run-7.log`  |
| 8   | PASS   | 356.4s (5m56s)                                            | `run-8.log`  |
| 9   | PASS   | 356.8s (5m56s)                                            | `run-9.log`  |
| 10  | PASS   | 356.8s (5m56s)                                            | `run-10.log` |

Those ten sums total 3571.9s (59m32s), 29s short of the 60m01s end-to-end
elapsed above; the residue is the ten `--rm` sibling containers' start-up and
Go build time, which no tracked log records per run.

Integrity check on the committed evidence: prefixing each `run-N.log` with its
`=== RUN n ===` marker, appending its `=== RUN n: PASS (exit 0) ===` marker
and then the two tally lines (`full per-run logs and combined summary log
kept in: ...` and `10/10 passed`) reproduces `summary.log` **byte for byte**
(`wc -c summary.log` reads **9464 bytes**, verified by reconstructing it into
a scratch file and running `cmp` against the committed `summary.log` — exit
status 0), so the ten per-run logs here are exactly the ten runs the driver
labelled.

## No failures this round

No run failed. `TestSigwinchCountDistinguishesTwoFromThree`
(`features/sigwinch_count_test.go:89`), the flake 505 named at roughly 1-in-10
across both its rounds, did not recur in these ten runs — consistent with a
low-probability race, not evidence it is fixed. This is still a **finding for
tasks 506/509/511**: this measurement does not license a claim that it is
resolved, only that it did not reproduce this time. All ten runs (1–10) show
every package `ok` with no `--- FAIL` line anywhere in their log.

## Frozen-tree proof

No code commit landed between the launch and this report:

```
$ git rev-parse HEAD
bfa3aad39966349d53989500a58837147060d600
$ git diff bfa3aad39966349d53989500a58837147060d600..HEAD -- '*.go' '*.feature' '*.sh' '*.toml' go.mod go.sum
<empty>
```

## Named skips (unchanged from 505)

- `features/godog_test.go`'s `defaultTags`: `~@real-agents && ~@nightly`.
- Three packages have no test files and print `?   ... [no test files]`:
  `internal/notify`, `internal/search`, `internal/unit`.
- The harness self-test `TestGodogRejectsUndefinedAndFailedSteps` deliberately
  injects a step failure inside its own subtest in every run (present in every
  `run-N.log`'s `features` package output); this is a self-test proving the
  harness rejects undefined/failed steps, not a product failure, and every
  run's package-level `ok github.com/n-orlov/deck/features` line confirms the
  package as a whole still passed.

## Relationship to 505

505's round 2 (`docs/reports/phase3g-505-stability10/`, launch sha `d863538`)
measured 9/10 at an earlier point of the same frozen tree lineage, with the
same sole failure at the same file:line recurring across both of its rounds.
This directory's 10/10 does not overwrite or contradict that: the sigwinch
count race is a known low-probability flake (505's evidence: 2 hits in 20
total runs, round 1 + round 2 combined), and ten more clean runs are within
the variance a ~1-in-10 flake produces, not proof it stopped occurring.
Task 506/509/511 are where its disposition (fixed / documented as an open
finding) belongs, not here.
