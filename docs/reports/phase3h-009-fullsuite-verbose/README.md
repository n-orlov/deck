# Task 009 — verbose companion sweep (Gherkin scenario tally)

## Command

Standing-rules tally companion, run as a background+poll job exactly as specified (sweep command
with `-v` added and nothing else):

```
nohup sh -c 'timeout 2400 ci/run.sh go test -p=1 -count=1 -v ./... > /tmp/phase3h-verbose.log 2>&1; echo $? > /tmp/phase3h-verbose.exitstatus' >/dev/null 2>&1 &
```

Polled with `sleep 60` (and one `sleep 90`) until `/tmp/phase3h-verbose.exitstatus` appeared,
~7m from launch.

## Launch-time HEAD / final code sha

At launch, HEAD was `e686a97bb4ca2b259a2175c8c0a7e1b7020504a6` (task 008's docs commit), a
docs-only descendant of the final code sha:

```
$ git log -1 --format=%H -- '*.go' '*.feature'
2ccb1d3c698d3c256aa94aea7ef2e3a58dc0641a

$ git diff --name-only 2ccb1d3c698d3c256aa94aea7ef2e3a58dc0641a..e686a97bb4ca2b259a2175c8c0a7e1b7020504a6
docs/reports/phase3h-008-fullsuite/.exitstatus
docs/reports/phase3h-008-fullsuite/README.md
docs/reports/phase3h-008-fullsuite/sweep.log
```

All three paths are under `docs/`. Final code sha this report belongs to: **`2ccb1d3`**
(`2ccb1d3c698d3c256aa94aea7ef2e3a58dc0641a`).

## Result

`.exitstatus` = **0**.

The `features` package's `TestFeatures` godog run reports the tally, quoted verbatim (ANSI colour
codes stripped) from the log — see `scenario-summary.txt`:

```
311 scenarios (311 passed)
3532 steps (3532 passed)
```

`go test -v` also runs `TestGodogRejectsUndefinedAndFailedSteps`, a self-test of the godog runner
itself that deliberately feeds it one undefined step and one failing step to prove they're
rejected; each of its two subtests prints its own tiny godog tally (`1 scenarios (1 undefined)`,
`1 scenarios (1 failed)`), and the outer `TestGodogRejectsUndefinedAndFailedSteps` itself is
`--- PASS`. Those two tallies belong to that fixture, not to the suite's own feature run, and are
not the tally quoted above.

All 17 packages are `ok` or `[no test files]` (`internal/notify`, `internal/search`,
`internal/unit`), matching task 008's non-verbose sweep exactly — no `FAIL` anywhere in the log
(`grep -a -n '^--- FAIL\|^FAIL' /tmp/phase3h-verbose.log` empty). `TestI1KeystrokeDropReproduction`
is `--- SKIP` (opt-in reproduction driver, pre-existing, gated on `DECK_I1_REPRO=1`, not part of
this plan).

## Contents

- `.exitstatus` — the captured `$?` of the `timeout 2400 ci/run.sh go test -p=1 -count=1 -v ./...`
  run (`0`).
- `scenario-summary.txt` — the godog scenario/step tally excerpt (tail of the `features` package's
  `TestFeatures` output) plus the full 17-line package summary (`ok`/`[no test files]`) extracted
  from `/tmp/phase3h-verbose.log`. The full verbose log itself (~1.2MB) is not committed, per the
  reports-cite-only-tracked-paths rule and the existing gotcha about large feature-log excerpts.
