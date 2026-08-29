# Phase 3g — task 703: 7 of task 702's 9 red scenarios fixed by a genuine route (2 left, documented)

## Sha this bundle starts and ends at

```
$ git rev-parse HEAD          # before this task's commit
608e030...                    # task 702's commit
```

Task 702 (`608e030`) enumerated the 9 scenarios `./features/` turned red after task 701
(R76's bare-hook/probe-error repair fix), without editing any scenario. Across two
iterations this task fixes **7 of those 9** by routing each one through a genuine state
transition (6) or a deliberately-derived, longer-but-still-bounded observation window (1)
instead of the raw-DB/live-pane pattern, or too-short single-tick assumption, that task
701's repair now exposes. The remaining 2 (`status_attach.feature:18`,
`status_claude_hooks.feature:6`) need a different, larger fix and are left red, fully
re-diagnosed below with the earlier (incorrect) hypothesis corrected, for a follow-up
task. Nothing was deleted, skipped, or weakened to get there: every scenario below still
asserts the exact same word/token/order/count/status field it always did; only *how the
state is produced*, or *how long the assertion is willing to wait for it*, changed.

## The invariant this task works around, restated

SPEC §7's self-heal (`internal/service.reconcile`'s `repairTerminalRowWithLivePane`,
task 040/701) is unconditional: **any** row whose `status` is `stopped` or `error`,
paired with a tmux pane that is present and not itself dead, is repaired on the very
next reconcile tick — regardless of `status_source` (raw DB write, probe, or a genuine
hook), and regardless of how fresh the write is. Task 701 made this correctly cover the
hook/probe case that task 040 originally missed (confirmed intentional, reviewer-
approved: `/run/ralphd/review-findings.md` finding 1, task 702's own report). A scenario
that wants a *stable*, durably-observable `error` (or `stopped`) row on what was a shell
or agent session must therefore make the pane **actually, verifiably die** first — the
`crashedPane`/`terminal` path (`session.PaneExitStatus != nil`) is the only route that
removes the row from repair's reach for good, because once collected the tmux session is
killed and `present` becomes false on every later pass.

## Fixed: attention_sort.feature (3 scenarios), status_theme.feature (2), themes.feature (1)

### The mechanism (one new step, reused six times)

`features/crash_test.go` already had `shellSessionExitsZero` (send-keys `exit 0` into a
shell pane, used by every "stopped" scenario for the same reason). This task adds its
one general-purpose counterpart:

```go
sc.Step(`^shell session "([^"]+)" exits with status ([1-9][0-9]*)$`, shellSessionExitsWithNonzeroStatus)
```

`shellSessionExitsWithNonzeroStatus` sends `exit <code>` (code > 0) into the named
shell's real pane the same way `shellSessionExitsZero` sends `exit 0`. deck's
server-wide `remain-on-exit=failed` retains the dead pane; the next reconcile tick's
`crashedPane` check is then true, so the row goes through crash collection (captures
the tail, writes a real `tmux`-sourced `error` with `pane_exit_status` set, kills the
tmux session) instead of the self-heal branch, which explicitly requires `!crashed`.
The result is a genuinely terminal, permanently-stable `error` row — not a race against
the next tick, unlike a raw `status="error"` DB write on a still-live pane.

| # (702's numbering) | Scenario | File:line | Fix |
|---|---|---|---|
| 1 | sessions render in the full waiting/error/running/starting/idle/stopped order, ties broken oldest-first | `attention_sort.feature:14` | `s-error`'s raw `has status "error" 40 seconds ago` replaced with `shell session "s-error" exits with status 1`, plus an explicit `within one configured reconcile interval … row "s-error" contains "error"` wait before the order assertion (so the order check never races the crash-collection tick either). |
| 2 | the collapsed strip's attention count matches the sort's own notion of attention | `attention_sort.feature:92` | `cnt-err`'s raw write replaced with `shell session "cnt-err" exits with status 1`; the scenario's own existing `within one configured reconcile interval … row "cnt-err" contains "error"` step (already present) now observes a state that cannot be repaired out from under it. |
| 3 | `space` walks only what needs attention, wraps, and changes no session's status | `attention_sort.feature:110` | `sp-2`'s raw write replaced with `shell session "sp-2" exits with status 1`, plus a new explicit `row "sp-2" contains "error"` wait before the first `space` press (the scenario's existing wait names a *different* session, `sp-1`/"waiting", so it gave no guarantee `sp-2` had already landed on `error`). |
| 7 | the error status token colours the error status word | `status_theme.feature:75` | `tok-target`'s raw write replaced with `shell session "tok-target" exits with status 1`. |
| 8 | statuses never borrow each other's colour | `status_theme.feature:89` | Same substitution (`tok-target`, same reasoning as #7 — this scenario's Background creates a fresh `tok-target` per scenario). |
| 9 | a built-in theme colours each of the seven §7 status tokens … (Outline row `tok49-err`/`target`/`error`) | `themes.feature:21` | The `error` row can no longer share the shared Outline (every other row in it — `waiting`, `running`, `idle`, `starting`, `archived` — is still a plain DB write, which is fine for those: none of them is `stopped`/`error`, so none is subject to repair). Removed the `tok49-err` row from the Outline's `Examples:` table and added a **new, separate** `Scenario: a built-in theme colours the error status token, read per cell from a real client`, structured identically to the file's own pre-existing "stopped" scenario (same file, a few lines below, already split out of this same Outline for the identical reason, with its own explanatory comment) — same client name (`tok49-err`), same setup, `shell session "target" exits with status 1` instead of the DB write, same three assertions (`screen contains "error"`, `text "error" has foreground token "error"`, `exits cleanly`). |

### Evidence: 3 consecutive green runs, all three touched feature files together

```
$ nohup sh -c 'timeout 400 ci/run.sh env DECK_GODOG_PATHS=attention_sort.feature,status_theme.feature,themes.feature go test ./features/ -run TestFeatures -count=1 -v > run{N}.log 2>&1; echo $? > run{N}.exitstatus' &
```
(backgrounded with `nohup … &`, polled with `sleep N; tail …`, never blocked on — same
launch/poll discipline as tasks 701/702.)

| Run | Exit status (file) | Scenarios | Steps | Wall time |
|---|---|---|---|---|
| 1 | [`run1.exitstatus`](run1.exitstatus) = `0` | `28 passed` | `338 passed` | 31.55s |
| 2 | [`run2.exitstatus`](run2.exitstatus) = `0` | `28 passed` | `338 passed` | 31.62s |
| 3 | [`run3.exitstatus`](run3.exitstatus) = `0` | `28 passed` | `338 passed` | 31.53s |

Full unedited logs: [`run1.log`](run1.log), [`run2.log`](run2.log), [`run3.log`](run3.log).
All 28 scenarios across the three files pass in every run, including the 6 previously-red
ones by name (`--- PASS: TestFeatures/sessions_render_in_the_full_waiting/error/…`,
`--- PASS: TestFeatures/the_collapsed_strip's_attention_count_…`, `` --- PASS:
TestFeatures/`space`_walks_only_what_needs_attention… ``, `--- PASS:
TestFeatures/the_error_status_token_colours_the_error_status_word`, `--- PASS:
TestFeatures/statuses_never_borrow_each_other's_colour`, `--- PASS:
TestFeatures/a_built-in_theme_colours_the_error_status_token,_read_per_cell_from_a_real_client`).

### Regression check: the whole `./features/` suite, once

```
$ nohup sh -c 'timeout 1800 ci/run.sh go test -count=1 ./features/ > full-features-suite.log 2>&1; echo $? > full-features-suite.exitstatus' &
```

**Captured exit status: `1`** (expected — the 3 scenarios this task does not fix yet are
still red), read from [`full-features-suite.exitstatus`](full-features-suite.exitstatus).
Full unedited log: [`full-features-suite.log`](full-features-suite.log). Its own summary
line: `311 scenarios (308 passed, 3 failed)` / `3523 steps (…)`, i.e. exactly `9 - 6 = 3`
failures, and by name (`grep -n '^[0-9]* scenarios\|--- FAIL' full-features-suite.log`)
they are precisely task 702's remaining #4, #5, #6 — nothing else regressed:

```
--- FAIL: TestFeatures/scrolling_the_interactive_grid's_own_scrollback_never_flips_the_session's_badge,_and_the_probe_reads_the_pane's_live_bottom,_not_the_scrolled-back_view
--- FAIL: TestFeatures/attach_acknowledges_a_live_error_without_replacing_its_verdict
--- FAIL: TestFeatures/Every_declared_Claude_hook_maps_to_honest_status_through_both_identity_routes
```

(The log's later `1 scenarios (1 failed)` / `1 scenarios (1 undefined)` blocks are
`TestGodogRejectsUndefinedAndFailedSteps`'s two deliberately-broken meta-fixtures —
proving godog's own `Strict` rejection still works — exactly as task 702's report
already explained for the same lines; that Go test itself is not in the package's
failing set, only `TestFeatures` is.)

## Not fixed yet: 2 scenarios (#5, #6), and why the earlier hypothesis for them was wrong

Both remaining scenarios fire a genuine `StopFailure` Claude hook onto a pane that **must
stay alive afterward** for a later step in the same scenario (`status_claude_hooks.feature`
fires `UserPromptSubmit` through the same pane afterward; `status_attach.feature` attaches
to it) — so "drive the pane to a real exit first" (this task's fix for #1/2/3/7/8/9) does
not apply; it would kill the very pane later steps still need. "Widen the observation
window" (this task's fix for #4, above) does not apply either, and re-deriving why matters
more than just recording the negative result, because the first pass at this task's own
report guessed the wrong mechanism for these two:

- **The earlier hypothesis (recorded by the previous iteration, now corrected):** that the
  race was against the *observing client's own* periodic `ReconcileWithProbes` tick, the
  same mechanism #4 turned out to have, and that closing/reopening the observing client
  around the vulnerable window (or slowing its reconcile loop) would give the assertion a
  chance to win it.
- **What `internal/agent/claude.go:122` and `cmd/deck/main.go`'s `runHook` actually show:**
  deck injects `<deck executable> _hook` as the *Claude hook command itself* — the fake
  Claude fixture's `fireHook` (`cmd/fake-claude/main.go`) runs it exactly the way a real
  Claude hook subprocess would, as a synchronous child process. `runHook` (`cmd/deck/main.go`)
  writes the hook's own status via `hookrecv.Receive`, then — for every hook except
  `SessionEnd` — immediately calls `liveness.ReconcileWithin` *in the same subprocess
  invocation, before it returns*. That call re-reads the row it just wrote, finds the
  paired pane still live and not crashed, and self-heals it back to `starting` — all
  within the one `deck _hook` process the `StopFailure` event itself runs. There is no
  client involved, no ticker to race, and no window: by the time the hook subprocess (and
  therefore `fakeClaudeFires`'s send-keys step) returns control to the test, the repair has
  already happened. Confirmed empirically, not just by reading the code: both scenarios
  fail identically and deterministically on every one of 3 consecutive runs each (below),
  always with the exact same corrected-row message
  (`reason "tmux pane is alive; terminal row corrected"`), never intermittently — the
  signature of a same-process sequential effect, not a timing race with any observable
  spread.
- **Why this means the fix is not a step substitution or a longer poll:** unlike #4 (a
  client-driven probe tick, genuinely racing another client-driven repair tick, so a longer
  window gives the assertion more *independent* tries), here the write and its own undo are
  two statements in the same sequential Go call graph, with nothing external between them
  to observe. No amount of polling after the hook call returns can ever see the pre-repair
  value, because it no longer exists once the call returns. The only way to keep the error
  status observable while the pane stays alive is to prevent that same-process
  `ReconcileWithin` from running at all for this one hook delivery — and `runHook`
  performs it unconditionally for every non-`SessionEnd` event, with no seam a black-box
  scenario can reach without either (a) a test-only branch in product code (forbidden,
  R8) or (b) a structural rewrite of how the hook is delivered that does not go through
  the released `deck _hook` subcommand's ordinary post-hook liveness pass at all — which is
  a genuine, larger investigation (is there a legitimate way to fire a real `StopFailure`
  hook without the delivering process itself performing a liveness pass immediately after?
  is the reviewer-approved R76 self-heal, which is unconditional by design, actually meant
  to make a live-pane hook error permanently unobservable through this route, and if so is
  this scenario's own premise obsolete and due a SPEC-level finding rather than a step
  fix?) that the one-task-per-iteration rule says not to rush under this task.

### Evidence the corrected diagnosis is accurate: 3 consecutive identical failures, each

```
$ ci/run.sh env DECK_GODOG_PATHS=status_claude_hooks.feature go test ./features/ -run TestFeatures -count=1
$ ci/run.sh env DECK_GODOG_PATHS=status_attach.feature go test ./features/ -run TestFeatures -count=1
```

Both commands were run 3 times each (not committed as separate log files — the exact same
single-line failure message repeats verbatim on every run, so a single representative run
of each is quoted here instead of 6 near-duplicate logs):

```
session "hook truth" = status "starting" source "tmux" reason "tmux pane is alive; terminal row corrected" ...
session "failed prompt" = status "starting" source "tmux" reason "tmux pane is alive; terminal row corrected" ...
```

These 2 are left exactly as task 702 found them (still red, still asserting the same
things); no assertion was touched, weakened, or removed in either of them by this task.

## Fixed: interactive_scroll.feature (#4)

### The mechanism: the same assertion, a longer, deliberately-derived window

`interactive_scroll.feature:112`'s `within one configured reconcile interval deck client
"B" row "ig-claude" contains "sampled"` used the same generic, single-reconcile-interval
poll every other `within one configured reconcile interval` step uses
(`clientRowContainsWithinReconcile`, `features/status_probe_test.go`) — correct for a
plain client-driven state change, but this scenario's probed `error` status is written by
client A or B's own `ReconcileWithProbes` tick and *then repaired by the very next
reconcile tick of either attached client* (SPEC §7's self-heal, task 701), because the
underlying Claude pane is deliberately kept genuinely alive and non-crashed throughout.
With `stale_after` at 1s (`configureAttachScrollProbeScenario`,
`features/attach_scroll_test.go`) against a ~250ms reconcile ticker running independently
in *each* of the two attached clients, the row oscillates error → repaired-to-starting →
(1s later) re-probed-to-error → repaired again, with only the fraction of each ~1.25s
cycle between a probe write and whichever client's reconcile tick lands next (which can be
as little as a few milliseconds, since A and B's tickers are not phase-locked) actually
rendering `"sampled"`/`"error"` text. A single reconcile-interval-plus-250ms poll window
(~500ms) can land entirely inside a `"starting"` phase and see nothing, with nothing having
gone wrong — the assertion was simply not given enough *tries* at a real, recurring event.

The fix (`features/interactive_scroll_test.go`'s new `clientRowContainsAcrossSeveralProbeCycles`,
wired to a new step text `within several probe/repair cycles deck client "X" row "Y"
contains "Z"`, used only by this one scenario) keeps the exact same text/row check as the
original step, with a deadline of `4 * (attachScrollProbeStaleAfter + scenarioReconcileInterval)`
(a named constant shared with `configureAttachScrollProbeScenario`, not a re-typed literal)
— four full oscillation periods instead of one partial reconcile interval, so the row gets
several independent chances to be caught mid-error instead of needing the very first one.
No product code changed; no assertion text weakened; the same real probe write and the same
real self-heal repair produce the same real, transient `"sampled"` text this step always
required, just observed with a bound sized to the real period the product itself creates.

### Evidence: 3 consecutive green runs

```
$ ci/run.sh env DECK_GODOG_PATHS=interactive_scroll.feature go test ./features/ -run TestFeatures -count=1
```

| Run | Exit status (file) | Scenarios | Steps |
|---|---|---|---|
| 1 | [`interactive-scroll-run1.exitstatus`](interactive-scroll-run1.exitstatus) = `0` | `4 passed` | all passed |
| 2 | [`interactive-scroll-run2.exitstatus`](interactive-scroll-run2.exitstatus) = `0` | `4 passed` | all passed |
| 3 | [`interactive-scroll-run3.exitstatus`](interactive-scroll-run3.exitstatus) = `0` | `4 passed` | all passed |

Full unedited logs: [`interactive-scroll-run1.log`](interactive-scroll-run1.log),
[`interactive-scroll-run2.log`](interactive-scroll-run2.log),
[`interactive-scroll-run3.log`](interactive-scroll-run3.log) — each shows
`--- PASS: TestFeatures/scrolling_the_interactive_grid's_own_scrollback_never_flips_the_session's_badge,_and_the_probe_reads_the_pane's_live_bottom,_not_the_scrolled-back_view`.

### Regression check: the whole `./features/` suite, once, after this fix

```
$ nohup sh -c 'timeout 1800 ci/run.sh go test -count=1 ./features/ > full-features-suite-after-interactive-scroll-fix.log 2>&1; echo $? > full-features-suite-after-interactive-scroll-fix.exitstatus' &
```

**Captured exit status: `1`** (expected — the 2 scenarios this task still does not fix are
still red), read from
[`full-features-suite-after-interactive-scroll-fix.exitstatus`](full-features-suite-after-interactive-scroll-fix.exitstatus).
Full unedited log:
[`full-features-suite-after-interactive-scroll-fix.log`](full-features-suite-after-interactive-scroll-fix.log).
Its own summary line: `311 scenarios (309 passed, 2 failed)`, and by name
(`grep -n '^[0-9]* scenarios\|--- FAIL'`) they are precisely #5 and #6 — nothing else
regressed:

```
--- FAIL: TestFeatures/attach_acknowledges_a_live_error_without_replacing_its_verdict
--- FAIL: TestFeatures/Every_declared_Claude_hook_maps_to_honest_status_through_both_identity_routes
```

## No unrelated file touched

```
$ git diff --stat 608e030..HEAD -- '*.go' '*.feature'
 features/attach_scroll_test.go      |  9 ++++++-
 features/attention_sort.feature     | 26 +++++++++++++++++---
 features/crash_test.go              | 36 +++++++++++++++++++++++++++
 features/interactive_scroll.feature |  2 +-
 features/interactive_scroll_test.go | 49 +++++++++++++++++++++++++++++++++++++
 features/status_theme.feature       | 11 +++++++++--
 features/themes.feature             | 26 +++++++++++++++++++-
 7 files changed, 151 insertions(+), 8 deletions(-)
```

`features/godog_test.go` is untouched (not in the diff above). No scenario, `Then`/`And`
assertion, or Examples row was deleted from any of these files — one Examples row moved
into its own `Scenario` (mirroring the file's own pre-existing pattern for `stopped`),
each of 6 remaining edits swaps one `Given`/`When` producer step for another that reaches
the exact same asserted word/state through a real pane exit instead of a raw DB write, and
one edit (#4) swaps one poll step for another that checks the exact same text in the exact
same row over a longer, deliberately-derived window instead of a single reconcile tick.
