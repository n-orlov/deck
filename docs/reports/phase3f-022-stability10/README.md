# `ci/stability.sh 10` at the phase's final code commit — the REAL rate is 9/10 (task 022)

**Rate: 9/10. Not 10/10.** One run failed; it is root-caused below to a named
mechanism, and that mechanism **is the one R65 was drawn from**, so it is written
up here as a **FAILED requirement**, not as a host-load note. Nothing was re-run
to hunt a streak: ten runs were commissioned, ten were run, the tenth was the
last.

## What was run, and at which tree

| | |
|---|---|
| command | `ci/stability.sh 10` (no arguments beyond the count; no tags, no path filter, no env override) |
| commit | `c12c30e` (`docs: publish the green whole-suite run at the phase's final code commit (task 021)`) |
| code tree | identical to `e47cb35`, the phase's final code commit — see below |
| per-run command inside the script | `ci/run.sh go test -p=1 -count=1 ./...` (one throwaway sibling container per run) |
| script exit status | **1** (the script exits non-zero whenever any run failed) |
| script's own verdict line | `9/10 passed` |
| summary log path | `/tmp/deck-stability.QRo6Ac/summary.log` (per-run logs `run-1.log` … `run-10.log` in the same directory) |
| start / end | `1787772301` → `1787775862` unix (`2026-08-26T19:25:01Z` → `2026-08-26T20:24:22Z`) |
| total wall clock | `3561s` = 59m21s |
| host | 28-core sibling host, go1.25.13, tmux 3.5a |

`c12c30e` and `300ee86` are documentation-only commits on top of `e47cb35`; the
code tree this run exercised is `e47cb35`'s, the same tree task 021's whole-suite
run used:

```
$ git rev-parse HEAD
c12c30e5104b59123b1b8ad6b9ba72b09f466dfb
$ git diff --stat e47cb35 HEAD -- '*.go' '*.feature' '*.toml' '*.sh' go.mod go.sum
                                    # empty: no code, feature, theme, script or module change
$ git status --short                # empty, before and after the ten runs
```

Load-bearing ordering relevant to this deliverable: **R63 (`f7b97fe`) landed
before this stability run**, as required, and before R65's own evidence
(`5071389`, `202e1ba`).

### Logs kept with this report

- `run-markers-timestamped.log` — every `=== RUN n ===` / `=== RUN n: PASS|FAIL ===`
  line the script printed, prefixed with the unix second it appeared.
- `loadavg-trace.log` — `/proc/loadavg` sampled every 5s for the whole hour (725
  samples), each line `<unix> <1min> <5min> <15min> <procs> <lastpid> | <last RUN marker seen>`.
- `run-pass-logs.log` — the nine PASS runs' own `go test` logs, concatenated with
  their source paths.
- `run-9-FAIL-trimmed.log` — run 9's full log with the multi-MB SIGQUIT goroutine
  dump's over-long lines dropped (`awk 'length($0)<400'`); nothing else removed.

Copies of all four are also in the run's artifacts directory as
`task022-stability-*`.

### How the times and loads were captured (and their resolution)

The script prints its `=== RUN n ===` markers on stdout; that stdout was
redirected to a file, and two **read-only** companions ran beside it: a 1s poller
that stamped each newly appearing marker line with `date +%s`, and a 5s sampler
that appended `/proc/loadavg` plus the last marker seen. Neither touches the
suite, the script, or its exit status — the script's own no-pipe exit capture is
untouched, and its `9/10 passed` line agrees with the table below.

One caveat, stated rather than papered over: the marker poller was started ~6
minutes into run 1 (its first attempt used a `tail -F | awk` pipeline, which
GNU `tail` block-buffers to a pipe, so it emitted nothing; it was replaced by
PID). Run 1's **start** is therefore taken from the loadavg trace, which brackets
it between the `pre-start` sample at `1787772301` and the first `=== RUN 1 ===`
sample at `1787772306`; `1787772301` is used, so run 1's wall clock is if
anything over-stated by ≤5s. Every other timestamp in the table is a direct
stamp of the marker line, ±1s.

## Per-run table

`load1 @start` is the 1-minute figure from the first `/proc/loadavg` sample at or
after that run's start; `load1 band` is the min–max 1-minute figure across every
5s sample taken inside the run.

| run | start (unix) | end (unix) | wall | result | load1 @start | loadavg @start (1/5/15) | load1 band during run | log |
|---|---|---|---|---|---|---|---|---|
| 1 | 1787772301 | 1787772671 | 370s | PASS (exit 0) | 3.88 | 3.88 4.09 3.64 | 3.69 – 14.98 | `run-1.log` |
| 2 | 1787772671 | 1787773049 | 378s | PASS (exit 0) | 7.44 | 7.44 6.49 5.03 | 4.24 – 13.97 | `run-2.log` |
| 3 | 1787773049 | 1787773422 | 373s | PASS (exit 0) | 5.06 | 5.06 7.06 5.83 | 2.91 – 11.59 | `run-3.log` |
| 4 | 1787773422 | 1787773770 | 348s | PASS (exit 0) | 5.36 | 5.36 6.28 5.77 | 1.51 – 6.93 | `run-4.log` |
| 5 | 1787773770 | 1787774116 | 346s | PASS (exit 0) | 1.61 | 1.61 3.29 4.60 | 1.47 – 3.23 | `run-5.log` |
| 6 | 1787774116 | 1787774464 | 348s | PASS (exit 0) | 3.15 | 3.15 2.81 3.95 | 1.29 – 3.66 | `run-6.log` |
| 7 | 1787774464 | 1787774811 | 347s | PASS (exit 0) | 2.11 | 2.11 2.36 3.35 | 1.66 – 3.34 | `run-7.log` |
| 8 | 1787774811 | 1787775160 | 349s | PASS (exit 0) | 2.21 | 2.21 2.29 3.00 | 1.45 – 4.61 | `run-8.log` |
| **9** | 1787775160 | 1787775513 | 353s | **FAIL (exit 1)** | **1.49** | 1.49 2.25 2.81 | **1.21 – 4.33** | `run-9.log` |
| 10 | 1787775513 | 1787775862 | 349s | PASS (exit 0) | 3.59 | 3.59 2.75 2.82 | 1.41 – 3.59 | `run-10.log` |

**REAL rate: 9/10 PASS, 1/10 FAIL.** Log paths are relative to
`/tmp/deck-stability.QRo6Ac/`; the same content is committed here as
`run-pass-logs.log` and `run-9-FAIL-trimmed.log`.

**The failing run was the quietest run of the ten.** Run 9 started at the lowest
1-minute loadavg of all ten starts (`1.49`) and its whole in-run load band
(`1.21`–`4.33`) sits below runs 1–3, which peaked at `11.6`–`15.0` and all passed.
So "the host was busy" is not available as an explanation here — it is ruled out
by the trace, not merely doubted. (This inverts Phase 3e's observation, where the
same assertion failed at the *highest* start-loadavg of ten:
`docs/reports/phase3e-408-stability-10-at-75861e0/README.md` item 2.)

Every PASS run's log is the full package set — 14 `ok` and 3 `[no test files]`,
`features` at 297.3–328.7s (run 9's failing `features` took 303.1s, squarely inside
that band) — matching task 021's green run
(`docs/reports/phase3f-021-fullsuite/README.md`).

## Nothing was skipped, tagged out, or re-run

- `ci/stability.sh` runs `go test -p=1 -count=1 ./...` with **no** `DECK_GODOG_TAGS`
  and **no** `DECK_GODOG_PATHS` set, so `defaultTags` stayed
  `"~@real-agents && ~@nightly"` and every non-nightly, non-real-agent scenario
  ran in every run. Run 9's log shows the full population:
  `306 scenarios (305 passed, 1 failed)`, `3440 steps (3436 passed, 1 failed, 3 skipped)`.
  The 3 skipped steps are the remaining steps of the one failed scenario, which
  godog skips after a failure — not a tag exclusion.
- No `@flaky`/`@wip` tag exists, no retry loop was added, no scenario was deleted
  or excluded, and no expected SIGWINCH count was touched for this run.
- **No eleventh run.** The rate above is the first and only ten-run measurement
  taken at this tree; it is published as measured.

## Root cause of the single failure (run 9)

Failure, quoted from `run-9-FAIL-trimmed.log`:

```
--- FAIL: TestFeatures (284.45s)
    --- FAIL: TestFeatures/a_fit_is_skipped_below_the_7-inner-row_floor,_and_retried_once_the_panel_grows_back_above_it (2.56s)
        suite.go:640: after scenario hook failed: fake "claude" agent received 1 SIGWINCH signals, want exactly 0
  Scenario: a fit is skipped below the 7-inner-row floor, and retried once the panel grows back above it  # preview.feature:134
    Then the fake "claude" agent received exactly 0 SIGWINCH signals                                      # preview.feature:147
FAIL	github.com/n-orlov/deck/features	303.113s
```

It was the only failing scenario in the only failing package of the only failing
run; every other package in run 9 is `ok` (`run-9-FAIL-trimmed.log:4965-4977`).

**Named mechanism: a pre-resize passive preview fit for `beacon`, licensed at the
client's default 100x30 before the scenario shrinks the panel to 100x9, whose
success is decided by an unbounded race in `previewFit`'s "no live pane yet"
early return.** This is exactly the mechanism written up as the FINDING in
`docs/reports/phase3f-017-r65-settled-sigwinch-count.md` ("`preview.feature:134`
has a pre-resize fit race that R65 exposes rather than causes"):

1. The scenario creates `beacon` while its row is selected at 100x30 and waits
   for `running`. A passive fit in that window is licensed behaviour — it is what
   `preview.feature:88` asserts.
2. `previewFit` returns `previewFitDone{sessionID}` even when there is no live
   pane yet, which spends the single coalesced fit `beacon` gets while selected.
   Usually that attempt is the wasted one and the scenario observes `0`; when the
   pane happens to exist first, the fit succeeds and a real kernel SIGWINCH
   reaches the fixture before the assertion's window.
3. Run 9's own frames carry that signature: the crop line reads `61x6 of 61x27`
   (six occurrences in `run-9.log`) — a 61x27 content box, i.e. the **100x30**
   geometry, not the 100x9 one the scenario resizes to. Identical to the
   signature in `artifacts/task017-r65-full-suite-run3-RED-trimmed.log`.

Run 9's frame dump itself is partly illegible because Go's SIGQUIT goroutine dump
interleaves with it (the harness's hung-client dump: `surviving deck client: hung
deck client killed after 1s`); the crop-line evidence above is read from the
surviving short lines, and matches task 017's legible reproduction of the same
failure.

## Is this a mechanism R64 or R65 claimed to fix? — R65: **YES, and it is therefore a FAILED requirement**

Stated plainly, because the phase's deliverable rule turns on it:

- **R64 — no.** R64's mechanism is a frame-reading `screen contains "starting"`
  waypoint on a **shell** session that the spec's fast-forward may skip
  (`features/create_cwd_ghost.feature:26`, `:91`). Neither scenario appears
  anywhere in run 9's failures, and no `starting` assertion failed in any of the
  ten runs. R64's mechanism did **not** recur.
- **R65 — yes, at the site and with the message R65 was drawn from.** R65's own
  source is `docs/reports/phase3e-408-stability-10-at-75861e0/README.md` item 2:
  run 9 of ten, `preview.feature:147`, `received 1 SIGWINCH signals, want exactly 0`.
  This run's failure is the **same feature file, the same line, the same
  assertion, the same message**. The field symptom R65 was commissioned against
  is still present at the phase's final tree, so **the requirement is recorded
  here as FAILED**, not as a host-load note — and the loadavg trace above removes
  the host-load defence outright.

The precise, non-excusing distinction — because a failed requirement still
deserves an accurate reason:

- R65's *scope* was the assertion's unsoundness (a `waitForSigwinchCount` poll
  that made `exactly 1` mean "at least 1" and `exactly 0` mean nothing). That
  part was delivered and is sound: the step now settles on
  `WaitForQuiescence` and then compares for equality, with both red directions
  demonstrated (`artifacts/task017-r65-red-extra-sigwinch.log`,
  `artifacts/task017-r65-red-zero-sigwinch.log`).
- What remains is a **second, product-side mechanism at the same site** that R65
  was explicitly forbidden to paper over ("If making a site settle changes what
  it observes, that is a finding about the product, not a number to update:
  report it and stop") and that it therefore left standing, with the finding
  written up and flagged as threatening this very deliverable.
- Consequence for the record: R65's *assertion* criteria are met, R65's
  *field-symptom* claim is not. `preview.feature:134` is still non-deterministic
  in a whole-suite run at this tree — one failure in ten runs here, on top of the
  one-in-four observed in task 017's three whole-suite runs — and no re-baselining
  of its counts is permitted, so the fix is a restructure of the scenario's
  prefix (never license a fit at the larger size, or bound `previewFit`'s
  no-live-pane return so it cannot spend the coalesced fit). **That restructure
  is not in this phase's task list and wants its own task.**

The second known flake of this phase — `TestGoldenMinimumFrame`'s "frame kept
changing" recurrence (`artifacts/task016-goldenframe-count10*.log`) — did **not**
fire in any of the ten runs; `internal/tui` is `ok` in all ten logs. It remains
open and unproven-fixed rather than retired by this run.

## Summary

| claim | value |
|---|---|
| runs commissioned / run / re-run | 10 / 10 / 0 |
| REAL rate | **9/10** |
| script exit status | 1 |
| failures | 1 — `preview.feature:134`, assertion at `:147`, `received 1 SIGWINCH signals, want exactly 0` |
| mechanism | pre-resize passive fit for `beacon` at 100x30 + `previewFit`'s no-live-pane early return spending the coalesced fit |
| R64 mechanism recurred? | no |
| R65 mechanism recurred? | **yes — same file, line, assertion and message as R65's cited source → FAILED requirement** |
| host load as explanation | ruled out: the failing run had the lowest start loadavg (1.49) and the second-quietest in-run band of the ten |
