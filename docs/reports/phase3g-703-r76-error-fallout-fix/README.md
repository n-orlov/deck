# Phase 3g — task 703: 6 of task 702's 9 red scenarios fixed by a genuine route (3 left, documented)

## Sha this bundle starts and ends at

```
$ git rev-parse HEAD          # before this task's commit
608e030...                    # task 702's commit
```

Task 702 (`608e030`) enumerated the 9 scenarios `./features/` turned red after task 701
(R76's bare-hook/probe-error repair fix), without editing any scenario. This task fixes
**6 of those 9** by routing each one through a genuine state transition instead of the
raw-DB/live-pane pattern task 701's repair now (correctly) undoes. The remaining 3 need
a different, larger fix (a real client-restart around the vulnerable window, not just a
step substitution) and are left red, fully diagnosed below, for a follow-up task. Nothing
was deleted, skipped, or weakened to get there: every scenario below still asserts the
exact same word/token/order/count it always did; only *how the state is produced* changed.

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

## Not fixed yet: 3 scenarios (#4, #5, #6), and why they need a different fix

All three remaining scenarios fire a genuine hook (or probe) `error`/`stopped` write onto
a pane that **must stay alive afterward** for a later step in the same scenario (a
follow-up hook fired by send-keys into the same pane, or a client attaching to it) — so
"drive the pane to a real exit first" (this task's fix for the other 6) does not apply;
it would kill the very pane later steps still need.

- **`interactive_scroll.feature:86`** — the probe writes `error`/`source=probe` on a
  genuinely live, rendering Claude pane; the scenario then needs the *next* probe
  sample (`sampled`) to read correctly, so the pane must stay alive and keep being
  probed. `stale_after` in this scenario's shared config (`configureAttachScrollProbeScenario`)
  is 1s against a 250ms reconcile tick, so the natural cycle is repair (~250ms) → next
  probe eligible (~1s) → `sampled` (~250ms) → repair again, oscillating with a period of
  about 1.25s; the assertion's own poll window (`scenarioReconcileInterval + 250ms` ≈
  500ms) is shorter than that period, so it can land on either phase depending on when
  the probe-status write happens to fall in the cycle. A genuine fix needs either a
  scenario-specific longer poll (several full cycles, so `sampled` is still a real,
  required observation, just given enough repetitions of the real cycle to reappear) or
  a probe/repair-immune way to pose the same fact; not attempted this task.
- **`status_attach.feature:18`** and **`status_claude_hooks.feature:6`** — a genuine
  Claude `StopFailure` hook (fired by send-keys into the still-running fake-claude pane,
  exactly as a real hook subprocess would from a real Claude turn failure) writes
  `error`/`source=hook`/`tool_failure`; the very next reconcile tick's self-heal repairs
  it (to `starting`) because the pane is still alive and not crashed — and in both
  scenarios a *later* step needs that same pane still alive (`status_claude_hooks.feature`
  fires `UserPromptSubmit` through the same pane afterward; `status_attach.feature`
  attaches to it). Slowing down reconcile (`DECK_RECONCILE_MS`, already used by
  `launch_lease.feature` for an analogous race) is the closest existing tool, but both
  scenarios also rely on the *default-speed* reconcile loop elsewhere (to observe other
  hook-driven `waiting`/`idle` screen text) via the same client, so a blanket slowdown
  would just move the race to a different assertion, not remove it. A genuine fix likely
  needs the vulnerable window to run with *no* fast-reconciling client attached at all
  (e.g. closing the observing client before the `StopFailure` hook and reopening — or
  opening a second, slow-reconcile client only for that one moment — before any
  fast-reconciling client is running again), which is a larger, more structural change
  than a one-line step substitution; not attempted this task so as not to rush a
  structural change to a scenario under the one-task-per-iteration rule.

These 3 are left exactly as task 702 found them (still red, still asserting the same
things); no assertion was touched, weakened, or removed in either of them by this task.

## No unrelated file touched

```
$ git diff --stat 608e030..HEAD -- '*.go' '*.feature'
 features/attention_sort.feature | 26 +++++++++++++++++++++++---
 features/crash_test.go          | 36 ++++++++++++++++++++++++++++++++++++
 features/status_theme.feature   | 11 +++++++++--
 features/themes.feature         | 26 +++++++++++++++++++++++++-
 4 files changed, 93 insertions(+), 6 deletions(-)
```

`features/godog_test.go` is untouched (not in the diff above). No scenario, `Then`/`And`
assertion, or Examples row was deleted from any of these files — one Examples row moved
into its own `Scenario` (mirroring the file's own pre-existing pattern for `stopped`),
and each remaining edit swaps one `Given`/`When` producer step for another that reaches
the exact same asserted word/state through a real pane exit instead of a raw DB write.
