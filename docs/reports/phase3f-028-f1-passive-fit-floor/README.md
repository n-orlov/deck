# Task 028 — finding F1: the passive-fit SIGWINCH assertion that raced its own shrink

Fixes review finding F1 (`docs/reports/phase3f-findings.md:310`), the single
failure that cost approach 01 its 10/10 stability run:
`fake "claude" agent received 1 SIGWINCH signals, want exactly 0` at
`features/preview.feature:147` (scenario `@steer-018-preview-fit-on-navigation`,
`features/preview.feature:134` before this task's edit).

Contents:

1. [The mechanism, re-derived from the code](#1-the-mechanism-re-derived-from-the-code)
2. [What `ResizeAndAwaitRender` does and does not guarantee (F1's own sentence is wrong)](#2-what-resizeandawaitrender-does-and-does-not-guarantee-f1s-own-sentence-is-wrong)
3. [The repair, and why it is the sanctioned one](#3-the-repair-and-why-it-is-the-sanctioned-one)
4. [The deterministic pin: RED with the fix reverted, GREEN with it applied](#4-the-deterministic-pin-red-with-the-fix-reverted-green-with-it-applied)
5. [Twenty consecutive targeted runs, each with its own loadavg](#5-twenty-consecutive-targeted-runs-each-with-its-own-loadavg)
6. [Prohibitions this task had to respect, and how the diff respects them](#6-prohibitions-this-task-had-to-respect-and-how-the-diff-respects-them)
7. [What this task deliberately did NOT change](#7-what-this-task-deliberately-did-not-change)

Evidence in this directory:

| file | what it is |
| --- | --- |
| [`red-fix-reverted.log`](red-fix-reverted.log) | the pinning assertion RED with the scenario prefix reverted to its pre-fix shape |
| [`red-loadavg.txt`](red-loadavg.txt) | `/proc/loadavg` at that run |
| [`green-fix-applied.log`](green-fix-applied.log) | the same assertion GREEN with the fix applied (`go test -v`, all four `@steer-018-preview-fit-on-navigation` scenarios) |
| [`green-loadavg.txt`](green-loadavg.txt) | `/proc/loadavg` at that run |
| [`twenty-runs.log`](twenty-runs.log) | 20 consecutive `DECK_GODOG_PATHS=preview.feature` runs, each preceded by its own `/proc/loadavg` |
| [`fit-trace-before.log`](fit-trace-before.log) | the pre-fix instrumented trace of `previewFit`'s own decisions (the derivation's raw material) |
| [`fit-trace-after.log`](fit-trace-after.log) | the same trace with the fix applied |
| [`fit-trace-instrument.patch`](fit-trace-instrument.patch) | the scratch-only instrument both traces were produced with (never committed as code) |
| [`features-package-full.log`](features-package-full.log) | the whole `features` package at this task's code, `ok` in 303.780s — the driver and step-definition changes affect every scenario in it, so it was run once in full |

## 1. The mechanism, re-derived from the code

The assertion that flaked is the `exactly 0` one. Reading the code (all line
numbers at `9a20134`, the tip this task started from):

* `previewFit` (`internal/tui/tui.go:1376`) is called from the `previewTick`
  case (`internal/tui/tui.go:1954`), i.e. once every `DECK_PREVIEW_MS` — **50 ms**
  in every scenario (`features/lifecycle_test.go:229`).
* It issues a fit unless one of its guards refuses. The two guards that decide
  this scenario are, in the order the function checks them:
  * `session.ID == m.previewFitSessionID || m.previewFitInFlight != ""`
    (`internal/tui/tui.go:1387`) — the row has already had its one coalesced
    attempt, or one is outstanding;
  * `width <= 0 || height < interactiveMinInnerRows`
    (`internal/tui/tui.go:1391`) — §11.9's 7-inner-row floor, measured by
    `m.previewContentSize()`, which is derived from **`m.width`/`m.height`**.
* `m.width`/`m.height` change in exactly one place: the `tea.WindowSizeMsg`
  case (`internal/tui/tui.go:1434`).
* `previewFitSessionID` is written in exactly one place, `previewFitDone`
  (`internal/tui/tui.go:1959`), and cleared in exactly one other,
  `exitInteractive` (`internal/tui/interactive.go:202`) — nothing clears it on
  a resize, which is `preview.feature:56`'s own guarantee that an outer-terminal
  resize never re-fits.

So a fit for the counted `beacon` row is licensed on any tick where beacon is
selected, beacon has not yet had its coalesced attempt, and `m.height` is still
large enough. The pre-fix scenario prefix created beacon at the client's default
**100x30** (`features/pty_driver_test.go:82-83`), and requirement 52
auto-selects a freshly created row (`features/new_session_selection.feature:20`).
That is precisely the licensed state, and it persists from the moment beacon's
row appears until the shrink's `WindowSizeMsg` is processed — tens to hundreds of
milliseconds, i.e. **several 50 ms ticks**.

The instrumented trace of the pre-fix scenario shows both interleavings
directly ([`fit-trace-before.log`](fit-trace-before.log); the instrument is
[`fit-trace-instrument.patch`](fit-trace-instrument.patch), applied to a scratch
tree only and never committed). In the run captured there the shrink won by
**4 ms**:

```
1787788877402 pid=1406 windowSize 100x9 (was 100x30)
1787788877405 pid=1406 skip floor sel=beacon box=61x6 term=100x9
```

(`fit-trace-before.log:155-156`; `pid=1406` is this scenario's own client.)
`windowSize 100x9` is the `WindowSizeMsg` being processed; the next tick, **3 ms**
later, is the first one that sees beacon selected — and by then the box is
`61x6`, below the floor, so no fit is issued and the count stays 0. Had that
tick fired 3 ms earlier it would have read `box=61x27 term=100x30`, passed the
floor, and issued the fit whose `resize-window` is the SIGWINCH F1 reports. The
same file shows exactly what that costs, for a client of a neighbouring scenario
that legitimately fits at the larger size (`fit-trace-before.log:20-23`):

```
1787788870265 pid=269 schedule sel=beacon box=61x27 term=100x30
1787788870272 pid=269 previewPane slug=beacon ok=true err=<nil>
1787788870280 pid=269 fitWindowToPane slug=beacon want=61x27 calls=1 err=<nil>
1787788870280 pid=269 previewFitDone id=f1daf2b4-e14b-4217-b52a-d4bb9c4ef1a5
```

one real `resize-window` call, hence one SIGWINCH at the fake agent.

Two further facts from the same trace matter for the repair:

* `previewPane slug=beacon ok=true` on every attempt in the trace — beacon's pane is live
  well before the shrink, so **the no-live-pane early return is not what makes
  the observed failure happen**. It is a real second hazard in the same
  scenario, in the opposite direction (an attempt spent while the pane is not
  yet live sets `previewFitSessionID=beacon`, which would refuse the fit the
  `exactly 1` assertion needs), but the run that actually failed did not take it.
* `And within one configured reconcile interval deck client "solo" screen
  contains "running"` is satisfied by **alpha's** own `running` status, so it
  does not delay past beacon's launch and does not narrow the race window.

## 2. What `ResizeAndAwaitRender` does and does not guarantee (F1's own sentence is wrong)

`docs/reports/phase3f-findings.md:313` says of the resize step: "The resize step
returns without waiting for deck to process the `tea.WindowSizeMsg`". The first
half of that sentence is false and the conclusion is right for a different
reason, so it is restated here from the code:

* `deck client "X" terminal is resized to CxR` is `resizeNamedClient`
  (`features/resize_test.go:39`), which calls
  `ScreenDriver.ResizeAndAwaitRender` (`features/pty_driver_test.go:209`). It
  **does** wait: it records the raw-output length, performs the real
  `TIOCSWINSZ` resize (`features/pty_driver_test.go:151`, which is what makes the
  kernel deliver SIGWINCH to deck), and then blocks until
  `resizeRenderMarker` — the unparameterised `\x1b[H` every full-screen
  bubbletea flush starts with (`features/pty_driver_test.go:181`) — appears in
  the output produced *after* the resize call.
* What that wait does **not** guarantee is that `m.width`/`m.height` were
  updated before the next step's keypress is dispatched. The driver's own
  comment already says so (`features/pty_driver_test.go:197-208`, task 210 /
  steer 019 §2): on a screen with its own periodic ticks the marker only proves
  that *some* full-screen repaint happened after the resize in wall-clock time,
  not that the repaint which satisfied the wait is the one that processed the
  resulting `WindowSizeMsg`. deck's session screen ticks twice over
  (`previewTick` every 50 ms, `reconcileTick` every 250 ms), so an unrelated
  tick-driven repaint using the pre-resize geometry satisfies it just as well.

Both halves matter for this task. The second is why a scenario may not assume
the model is at the new size right after a resize step. The first is why F1's
proposed reading — "the step does not wait at all" — must not be repeated as
established: the step waits, the wait is simply not a proof about
`m.width`/`m.height`. Note also that in the failing shape the race was *not*
between the resize and the next keypress at all: the fit was licensed by a
periodic tick on a row that requirement 52 had already selected, so gating the
following keypress on the new geometry would not have fixed it.

## 3. The repair, and why it is the sanctioned one

F1 sanctions two repairs: "restructure the scenario's prefix so no fit is
licensed at the larger size, or … bound the no-live-pane return so it cannot
spend the coalesced fit" (`docs/reports/phase3f-findings.md:329-331`). Section 1
shows the observed failure is the *first* hazard — a fit licensed at the larger
size — and that the no-live-pane return was not on the failing path. So the
repair is the prefix restructure, and the product code is unchanged
(`git diff --stat` touches `features/` only).

The restructure had to satisfy one hard constraint: the client whose SIGWINCHes
are counted must never have a preview content box above the floor **before** the
`exactly 0` assertion, and requirement 52 auto-selects a created row, so that
client cannot be the one that creates the claude session. Two shapes were tried:

* *Create beacon after the shrink, in the same client.* Rejected because it does
  not work: the create modal cannot render in a 9-row terminal, and the step
  fails with `timed out waiting for frame "Create shell session"` — reproduced
  before choosing the final shape.
* *Split the roles* (kept). `maker` runs at the ordinary size with
  `DECK_PREVIEW_FIT=0` — `config.Load`'s own documented override for
  `[ui] preview_fit` (`internal/config/config.go:249-255`), a public config key,
  not a test hook — so it can never fit anything; it creates `alpha` and
  `beacon` through the real create modal and exits. `solo`, the only client with
  fit enabled, is then started **at 100x9** via the existing
  `deck client "X" is started with terminal size CxR` step
  (`features/resize_test.go:15`), so its content box is `61x6` on every frame it
  ever renders before the assertion. Nothing is licensed at a larger size by
  construction rather than by winning a race.

The scenario keeps its meaning and both of its counts: the floor still refuses
the fit (`exactly 0`), and growing the panel back above the floor still retries
it exactly once (`exactly 1`). The post-fix trace shows it end to end
([`fit-trace-after.log`](fit-trace-after.log)): `maker` (`pid=1459`,
`fit-trace-after.log:144-152`) skips every tick at the
`skip settings/interactive/socket` guard, so it never fits anything; `solo`
(`pid=1512`) is born at `100x9` (`:153`) and skips every tick at the `floor`
guard with `box=61x6 term=100x9` for both selections (`:154-169`); and after the
growth to 100x30 there is exactly one `schedule sel=beacon box=61x27` →
`fitWindowToPane … calls=1` → `previewFitDone` (`:171-174`), with every later
tick refused by the coalescing guard. The instrument is the same one used for the
before-trace and is reproducible from
[`fit-trace-instrument.patch`](fit-trace-instrument.patch) (applied to a scratch
tree, run, reverted — `git status --short` is clean of it in the committed tree).

## 4. The deterministic pin: RED with the fix reverted, GREEN with it applied

A count assertion alone cannot pin this repair: the pre-fix scenario passed
roughly nine runs in ten, so "it is green now" proves nothing about it. The pin
is therefore a new scenario assertion on the property the repair actually
establishes — *the observing client was never tall enough for a fit to be
licensed at all* — which is true or false on every single run:

```
Then deck client "solo" has never been taller than 9 rows
And the fake "claude" agent received exactly 0 SIGWINCH signals
```

It is backed by `clientHasNeverBeenTallerThan` (`features/preview_test.go:26`,
`:64`) over `ScreenDriver.TallestRows` (`features/pty_driver_test.go:240`), a
new record of the largest row count the client's pty has ever had — its start
size plus every `Resize` since. It reads the harness's own kernel/emulator
geometry, never deck's internals, and it needs the *history* because the current
size cannot distinguish a client born small from one shrunk a moment ago, which
is exactly the difference that decides whether deck ever had a frame above the
floor to act on.

**RED with the fix reverted.** With the scenario prefix put back to its pre-fix
shape (`solo` started at the default 100x30, creating beacon itself, then
resized to 100x9) and the new assertion left in place
([`red-fix-reverted.log`](red-fix-reverted.log), loadavg
[`red-loadavg.txt`](red-loadavg.txt) = `1.40 1.90 2.16`):

```
  Scenario: a fit is skipped below the 7-inner-row floor, and retried once the panel grows back above it # preview.feature:134
    Then deck client "solo" has never been taller than 9 rows # preview.feature:169
      Error: after scenario hook failed: deck client "solo" has been 30 rows tall at some point, want never more than 9: a taller frame licenses the very passive fit this scenario asserts never happened, so the assertion below would be racing that fit rather than observing its absence (task 028, finding F1)
--- FAIL: TestFeatures (9.69s)
    --- FAIL: TestFeatures/a_fit_is_skipped_below_the_7-inner-row_floor,_and_retried_once_the_panel_grows_back_above_it (2.10s)
FAIL	github.com/n-orlov/deck/features	9.700s
```

**GREEN with the fix applied** ([`green-fix-applied.log`](green-fix-applied.log),
loadavg [`green-loadavg.txt`](green-loadavg.txt) = `4.14 2.59 2.38`):

```
    Then deck client "solo" has never been taller than 9 rows                                            # preview_test.go:26 -> github.com/n-orlov/deck/features.clientHasNeverBeenTallerThan
4 scenarios (4 passed)
59 steps (59 passed)
--- PASS: TestFeatures/a_fit_is_skipped_below_the_7-inner-row_floor,_and_retried_once_the_panel_grows_back_above_it (2.34s)
ok  	github.com/n-orlov/deck/features	10.028s
```

Would it go red if the fix were reverted? It did, above, deterministically and
on its own step's error rather than by package timeout — and it fails *before*
the count assertion it protects, naming the reason.

## 5. Twenty consecutive targeted runs, each with its own loadavg

`ci/run.sh env DECK_GODOG_PATHS=preview.feature go test -count=1 -run TestFeatures ./features/`,
twenty times in a row, no re-runs discarded and none needed:
**20 of 20 `ok`, 20 of 20 `exit=0`** ([`twenty-runs.log`](twenty-runs.log), which
records `/proc/loadavg` immediately before each run, then that run's own verdict
line and exit status). Host one-minute load across the twenty ranged from
**3.50 to 5.50** — deliberately *not* a quiet host, since F1's own failure was
observed at the lowest start loadavg of its ten-run set
(`docs/reports/phase3f-findings.md:324-327`), so a quiet-host streak would prove
the least. Per-run wall clock was 16.65 s – 17.38 s.

First and last entries, verbatim:

```
=== run 1 loadavg=3.51 2.55 2.37 1/4184 70709
ok  	github.com/n-orlov/deck/features	17.145s
exit=0
…
=== run 20 loadavg=3.50 3.79 3.05 21/4141 72368
ok  	github.com/n-orlov/deck/features	17.076s
exit=0
```

This is not offered as a stability claim for the phase: task 033 re-measures
`ci/stability.sh 10` at the final code commit and publishes the real rate. What
these twenty show is that the specific assertion F1 named no longer has a losing
interleaving to lose, and section 4 is what pins that rather than the streak.

## 6. Prohibitions this task had to respect, and how the diff respects them

| prohibition | evidence |
| --- | --- |
| no expected SIGWINCH count changed anywhere | `git diff features/` contains exactly one line touching a count assertion, and it changes only the Gherkin keyword (`Then` → `And`) because a new `Then` now precedes it: `-    Then the fake "claude" agent received exactly 0 SIGWINCH signals` / `+    And the fake "claude" agent received exactly 0 SIGWINCH signals`. Both counts (`exactly 0`, `exactly 1`) are byte-identical to before, and no other `received exactly N SIGWINCH` line in `features/` is touched. |
| no `sleep`, no widened timeout, no proxy wait | the diff adds no `sleep`/`time.Sleep`/`time.After`, no timeout constant and no polling loop; `clientHasNeverBeenTallerThan` reads an already-recorded value and returns. |
| no tag exclusion, no scenario skipped, `defaultTags` untouched | `features/preview.feature`'s tag lines are unchanged (`@steer-018-preview-fit-on-navigation` still selects the scenario, and it still runs in the default set), and `godog_test.go` is not in the diff. |
| no weakened step: the scenario still goes through the real UI | `maker` creates both sessions through the real create modal (`n`, typed name and cwd, cycled Agent/Permission-profile fields, `Enter`), and `solo` still walks to the row with real `g`/arrow keypresses via `selectRowByName`. Nothing calls the service directly, and the new step definition only *starts* a client — with a public config key in its environment, `DECK_PREVIEW_FIT=0` (`internal/config/config.go:249-255`). |
| product code unchanged | the whole diff is under `features/`; `internal/` is untouched, so no other scenario's SIGWINCH budget can move. |
| no protected path touched | `SPEC.md`, `prds/`, `ci/Dockerfile`, `ci/SPIKE.md` are not in the diff. |

## 7. What this task deliberately did NOT change

* **The no-live-pane early return** (`internal/tui/tui.go:1406-1413`) still emits
  `previewFitDone` and therefore still spends the row's one coalesced attempt.
  Section 1 shows it is not the mechanism behind the observed failure, and F1
  sanctions either repair on its own. It remains a real hazard in the opposite
  direction — an attempt spent while a row's pane is not yet live refuses the
  next legitimate fit until the selection moves away and back — but it is a
  product behaviour change with its own blast radius (every scenario that counts
  SIGWINCHes for a session that starts while selected), and this phase forbids
  changing any expected count to accommodate one. It is left open for its own
  task rather than folded in here; the restructured scenario no longer depends on
  which way that race falls, because at 100x9 no attempt is scheduled at all.
* **`ResizeAndAwaitRender`** is unchanged. Section 2's finding is that its wait is
  not a proof about `m.width`/`m.height`; strengthening it (a content-specific
  gate, as `golden_frame_test.go` layers on) would not have fixed this scenario,
  whose fit was licensed by a periodic tick and not by the following keypress.
* **`features/preview.feature`'s other scenarios** are untouched, including
  `:56`'s "an outer-terminal resize never re-fits" guarantee that explains why
  nothing clears `previewFitSessionID` on a resize.
