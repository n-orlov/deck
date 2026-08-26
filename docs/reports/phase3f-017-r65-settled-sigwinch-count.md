# R65 — the SIGWINCH count assertion settles before it reads, and stays exact (task 017)

## What was unsound

`Then the fake "<agent>" agent received exactly N SIGWINCH signals`
(`features/fake_agent_size_test.go`) read the fixture's own counter through
`waitForSigwinchCount`, a poll that returned **the instant it first observed
`want` or anything above it**. That is not an exact assertion:

- for `exactly 1` it behaved as *at least 1* — it accepted the first moment the
  count reached 1 and never noticed a second SIGWINCH still in flight;
- for `exactly 0` it did no waiting at all — 0 is the initial value, so the poll
  returned immediately and the read was a bare sample of an asynchronous counter.

Both failure directions have been observed in the field:
`docs/reports/phase3e-408-stability-10-at-75861e0/README.md` item 2 (run 9,
highest start-loadavg of the ten) failed `preview.feature:147` with
`received 1 SIGWINCH signals, want exactly 0`, and
`docs/reports/phase3e-stability/README.md` item 1 records the same assertion
shape failing at the previous commit.

## What changed

`theFakeAgentReceivedExactlySigwinchSignals` now **settles first, then reads
once, then compares for equality**:

1. `h.settleClients(ctx, captureSettledQuietWindow)` — waits until every
   still-running pty client this scenario started has produced no new output for
   400 ms, using **`ScreenDriver.WaitForQuiescence`** (`features/pty_driver_test.go:530`,
   the same helper `features/mouse_synthesis_test.go:226` already uses). No second
   waiter was written, and the quiet window is the existing
   `captureSettledQuietWindow` constant, not a new number.
2. `readSigwinchCount(path)` — **one** read.
3. `got != want` — plain equality, unchanged.

The passive fit is what made sampling racy, and it is exactly what the settle
observes: `previewFit` (`internal/tui/tui.go`) issues
`tmux.Client.FitWindowToPane`'s `resize-window` convergence loop, whose landing
changes the preview panel's content (and its crop/geometry notice line), i.e. it
shows up as further PTY output on the deck client. Waiting for that output to
stop is waiting on the observable consequence — no `sleep` was added, no timeout
widened, no expected count touched.

A client that has already exited is skipped rather than reported as an error
("gone" is a stronger form of "quiet"). The two
`features/interactive_sigwinch_budget.feature` sites start **no** deck client at
all (they drive tmux directly and put the fixture in a bare tmux pane), so they
settle nothing and read once; their own load-bearing `200 milliseconds pass`
steps — which predate this task and are unchanged — are what pace the enter/exit
SIGWINCH pair there. That feature's header comment was updated because it
described the old step's behaviour; the comment keeps its original line count so
the assertion lines stay at `:33` and `:46`.

`waitForSigwinchCount` itself is retained, unused by any step, as the **arrival
wait** for `features/sigwinch_count_test.go`, where reaching a known number of
real `TIOCSWINSZ` resizes is the precondition and the assertions are the separate
explicit `readSigwinchCount` comparisons that follow it. Its doc comment now says
so.

## All seven sites use the settled step

There is one step definition, so all seven assertion sites moved together. No
expected count changed:

| site | assertion | unchanged? |
|---|---|---|
| `features/preview.feature:95` | `exactly 1` | yes |
| `features/preview.feature:107` | `exactly 1` | yes |
| `features/preview.feature:130` | `exactly 0` | yes |
| `features/preview.feature:147` | `exactly 0` | yes |
| `features/preview.feature:149` | `exactly 1` | yes |
| `features/interactive_sigwinch_budget.feature:33` | `exactly 2` | yes |
| `features/interactive_sigwinch_budget.feature:46` | `exactly 2` | yes |

`git diff` for this task touches only `features/fake_agent_size_test.go` and
`features/interactive_sigwinch_budget.feature` (its header comment). No product
code changed.

## Both red directions, with the settle in place

Both controls were produced by mutating **product** code in place, running the
scenarios, then restoring the file and confirming `git status --short` showed no
product change. Both mutations were run WITH the new settle already in the step,
which is the point: a settle that waits and then still compares exactly must
catch an extra signal, and must catch an unexpected one.

### Direction 1 — one extra SIGWINCH turns `exactly 1` red

Mutation (`internal/tmux/geometry.go`, `FitWindowToPane`): before each real
`resizeWindow`, issue one extra `resizeWindow` at `newWidth+1`, with an 80 ms
pause between the two (the fixture's signal channel has capacity 1, so an
unpaced pair would be coalesced into a single observed signal and the control
would prove nothing).

```
$ ci/run.sh env DECK_GODOG_TAGS='@steer-018-preview-fit-on-navigation' \
    go test -count=1 -run TestFeatures ./features/
      6 step error: fake "claude" agent received 2 SIGWINCH signals, want exactly 1
--- FAIL: TestFeatures (11.50s)
FAIL	github.com/n-orlov/deck/features	11.518s
```

Log: `artifacts/task017-r65-red-extra-sigwinch.log` (loadavg at start
`2.50 1.92 2.11`).

### Direction 2 — an injected SIGWINCH turns `exactly 0` red

Mutation (`internal/tui/tui.go`, `previewFit`): drop the `m.settings.PreviewFit`
config gate and clamp instead of refusing below `interactiveMinInnerRows`, so a
fit fires in precisely the two scenarios that assert no SIGWINCH at all
(`preview.feature:130`, the `preview_fit = false` scenario, and `:147`, below the
7-inner-row floor).

```
$ ci/run.sh env DECK_GODOG_TAGS='@steer-018-preview-fit-on-navigation' \
    go test -count=1 -run TestFeatures ./features/
      6 step error: fake "claude" agent received 1 SIGWINCH signals, want exactly 0
--- FAIL: TestFeatures (11.63s)
FAIL	github.com/n-orlov/deck/features	11.652s
```

Log: `artifacts/task017-r65-red-zero-sigwinch.log` (loadavg at start
`1.67 1.83 2.07`).

`received 1 ... want exactly 0` is the *identical* message the stability run
reported in the field, now produced deliberately: the settled step still fails on
a real extra signal rather than being made permissive by the wait.

## Green runs — and the one red they exposed

`features/interactive_sigwinch_budget.feature`'s scenarios carry **no tags**, so
they cannot be tag-selected; the only way to run them targeted is the whole godog
suite under `-run TestFeatures` (which also runs all of `preview.feature`). Three
consecutive such runs, `/proc/loadavg` recorded per run
(`artifacts/task017-r65-full-suite-summary.txt`):

| run | loadavg before | result | log |
|---|---|---|---|
| 1 | `1.71 1.84 2.07` | `ok  github.com/n-orlov/deck/features  282.527s` | `artifacts/task017-r65-full-suite-run1.log` |
| 2 | `3.10 2.15 2.10` | `ok  github.com/n-orlov/deck/features  282.982s` | `artifacts/task017-r65-full-suite-run2.log` |
| 3 | `2.56 2.83 2.48` | **FAIL** `282.169s` — `preview.feature:147`, `received 1 SIGWINCH signals, want exactly 0` | `artifacts/task017-r65-full-suite-run3-RED-trimmed.log` |

The two budget-feature sites (`:33`, `:46`) passed in all three runs; run 3's only
failing scenario was `preview.feature:134` ("a fit is skipped below the
7-inner-row floor"). `features/sigwinch_count_test.go`'s fixture-contract test
(`TestSigwinchCountDistinguishesTwoFromThree`, the remaining user of
`waitForSigwinchCount`) passed after those runs.

The `@steer-018-preview-fit-on-navigation` scenarios — which are the five
`preview.feature` count sites — were then run **8 times in isolation**, all green,
loadavg `1.22`–`3.23` (`artifacts/task017-r65-steer018-isolated-8x.txt`), plus the
two earlier tag-scoped green runs. So the observed rate of this red is 1 of 4
whole-suite runs and 0 of 10 isolated runs. **No streak was hunted after run 3.**

## FINDING — `preview.feature:134` has a pre-resize fit race that R65 exposes rather than causes

R65's own rule applies verbatim: *"If making a site settle changes what it
observes, that is a finding about the product, not a number to update: report it
and stop."* No count was changed. What the settle exposed:

1. The scenario creates `beacon` at the client's default 100x30, where the row is
   selected on creation, and waits for `running`. A passive fit for `beacon` is
   fully licensed in that window — it is the very behaviour `preview.feature:88`
   asserts.
2. Whether that pre-resize fit actually issues a `resize-window` depends on a race
   the scenario never bounds: `previewFit`'s command reads `client.PreviewPane`
   first, and on "no live pane yet" it returns `previewFitDone{sessionID}` anyway
   (`internal/tui/tui.go`, the no-live-pane early return). That message sets
   `previewFitSessionID`, so the one coalesced fit `beacon` gets while it stays
   selected is **spent on a failed attempt** — which is why the scenario usually
   observes 0. When `beacon`'s pane happens to exist by the time that tick fires,
   the fit succeeds instead and a real kernel SIGWINCH reaches the fixture.
3. Run 3's frames show exactly that: the pane ends at `61x27` — the 100x30 content
   box, not the 100x9 one — and the crop line reads `61x6 of 61x27` after the
   resize.

Consequences worth stating:

- The failure is **pre-existing**, not introduced here: the same scenario failed
  the same way before this change (`artifacts/task015-r63-features-exacttags-flake.log`,
  recorded in the phase notes at task 015), and R63 can only suppress fits, never
  add one.
- The settle raises its **detection** rate, which is the point: before R65 the
  late-landing fit was read as `0` at `:147` and could then be counted as if it
  were the post-growth fit `:149` expects — a pass in both places for the wrong
  reason. That is precisely the vacuity R65 exists to remove.
- Making `:134` deterministic means restructuring its prefix (so no fit is ever
  licensed at the larger size) or re-baselining its two counts. R65 must not touch
  the counts, and the restructure belongs with the unsound-waypoint work, not with
  the assertion — so it is reported here, not fixed here. It also threatens the
  whole-suite and `ci/stability.sh 10` deliverables, so it wants a task of its own.

## Targeted runs of the two feature files — three consecutive greens

The three whole-suite runs above are the *broadest* evidence, not the targeted
one R65's own criteria ask for: they also drag in every other feature file, which
is where `preview.feature:134`'s pre-existing race (previous section) shows up.
Running the two files that actually carry the seven assertion sites was
previously impossible — `interactive_sigwinch_budget.feature`'s scenarios carry
**no tags**, so `DECK_GODOG_TAGS` cannot name them and the entire ~5 minute suite
was the only route to them. So this task added the missing selector, in the same
shape as the existing tags override (`features/godog_test.go`, `godogPaths()`):

    DECK_GODOG_PATHS='preview.feature,interactive_sigwinch_budget.feature'

Unset — every ordinary `go test ./features/`, every CI job, every deliverable
suite run — it is `[]string{"."}`, byte-for-byte the previous behaviour, so it
cannot shrink what the suite covers. `defaultTags` is untouched and no scenario
is excluded from anything; the knob narrows one diagnostic invocation, it does
not narrow the suite.

Three consecutive runs on the tree of this task's second commit ("features: let a
targeted run select feature files by path"), `/proc/loadavg` per run
(`artifacts/task017-r65-targeted-two-files-summary.txt`):

| run | loadavg before | scenarios | steps | result | log |
|---|---|---|---|---|---|
| 1 | `3.62 2.48 2.32` | 15 passed | 150 passed | `ok ... 18.847s` | `artifacts/task017-r65-targeted-two-files-run1.log` |
| 2 | `6.50 3.26 2.59` | 15 passed | 150 passed | `ok ... 18.628s` | `artifacts/task017-r65-targeted-two-files-run2.log` |
| 3 | `7.13 3.67 2.74` | 15 passed | 150 passed | `ok ... 18.660s` | `artifacts/task017-r65-targeted-two-files-run3.log` |

15 scenarios is the whole of both files (13 + 2), and each run's log contains all
seven settled assertions passing, with their expected values unchanged and in
order — `1 1 0 0 1` for `preview.feature:95,:107,:130,:147,:149` then `2 2` for
`interactive_sigwinch_budget.feature:33,:46`:

```
$ for i in 1 2 3; do grep -ao 'received exactly [0-9]' run$i.log | grep -o '[0-9]$' | tr '\n' ' '; done
1 1 0 0 1 2 2
1 1 0 0 1 2 2
1 1 0 0 1 2 2
```

Loadavg *rose* across the three runs (3.62 → 7.13, higher than any of the
whole-suite runs' start load), so these greens are not a quiet-host artefact.
They are also not a streak hunt: three runs were planned, three were run, and no
further run was made. What they do **not** do is retire the `preview.feature:134`
finding — that scenario is green here and red about one whole-suite run in four,
and the deliverable whole-suite (task 021) and `ci/stability.sh 10` (task 022)
runs must still face it.
