# Task 904 — review finding 1's two named scenarios, unedited, now green

Finding 1 named two scenarios as casualties of the pre-narrowing repair
(`89edd3c`, task 701): `features/status_attach.feature:18` (finding 801's
row, `794313f`/F36) and `status_probe.feature`'s `Stale sampling is visible,
precedence-aware, and agent-only` (`docs/reports/phase3g-812-stability10/
README.md:120`). This task proves both now pass with their assertions
never edited, and that the fix is the repair-narrowing, not the scenarios.

## The fix

**Task 902's commit `a1ca33e`** ("service: stop the live-pane repair
overwriting a hook/probe error verdict") is the fix. It narrowed
`internal/service/reconcile.go`'s repair trigger so it no longer fires for
an `error` row whose `StatusSource` is `hook` or `probe` and whose
`PaneExitStatus` is `nil` — only for a `stopped` row (any source) or an
`error` row that carries a pane-exit verdict or a `tmux`/`user` source.
Before `a1ca33e` (parent `c19bdde`, task 901), the unconditional repair
reached a live-pane hook- or probe-sourced `error` row and overwrote it with
`starting`/`tmux`, clobbering a higher-precedence verdict — exactly the
failure both scenarios below were hitting.

## Diff-empty proof: the scenarios themselves were never touched

```
$ git diff 1cfbd5a..HEAD -- features/status_attach.feature features/status_probe.feature
(no output)
```

Confirmed at this task's own HEAD (see `git log -1` at the time of this
report). Neither feature file has moved one line since Phase 3g's base —
the fix is entirely in `internal/service/reconcile.go` and its tests
(task 902), not in the scenarios review named.

## Pre-fix red step text, quoted verbatim

### `status_attach.feature:18` — "attach acknowledges a live error without replacing its verdict"

Reproduced directly for this task by running the two-file feature command
below against commit `c19bdde` (task 901's HEAD, i.e. the last commit
before task 902's fix `a1ca33e`) in a throwaway `git worktree` (removed
after capture; the live workspace tree was never mutated):

```
--- FAIL: TestFeatures (10.72s)
    --- FAIL: TestFeatures/attach_acknowledges_a_live_error_without_replacing_its_verdict (5.04s)
        suite.go:640: after scenario hook failed: session "failed prompt" = status "starting" source "tmux" reason "tmux pane is alive; terminal row corrected" message "" acknowledged=0 epoch=1; want "error" hook "tool_failure" "" 0 0
```

This is the deterministic failure mode: the repair fires on essentially
every run at that commit because the scenario's hook-sourced error row sits
in the repair's window every time. This run's raw log is not committed
separately (it also captured an unrelated `hung deck client killed after
1s` SIGQUIT stack dump from the framework's timeout handling, not part of
the assertion failure); the quoted text above is copied verbatim from that
run's `godog` failure line.

### `status_probe.feature`'s "Stale sampling is visible, precedence-aware, and agent-only"

This scenario's failure is racy (the repair's window against the
probe-sourced error is narrower/looser than `status_attach`'s), so it does
not reproduce on every single run — it was captured previously in stability
evidence rather than re-derived here. Quoted verbatim from
`docs/reports/phase3g-812-stability10/README.md:120` (6/10 stability runs at
the pre-fix commit `89edd3c`'s era, e.g. `run-1.log`):

```
after scenario hook failed: session "sampled pi" verdict = "starting"/"tmux" reason "tmux pane is alive; terminal row corrected", want "error"/"probe" reason "agent error" (err=<nil>)
```

Both quotes show the same signature: a `tmux`-sourced repair overwriting a
higher-precedence (`hook` or `probe`) `error` verdict with `starting`/`tmux`
— the exact defect task 901 recorded (finding F40) and task 902 fixed.

## Post-fix: three consecutive green runs, assertions unedited

`ci/run.sh env DECK_GODOG_PATHS=status_attach.feature,status_probe.feature go test ./features/ -run TestFeatures -count=1`,
run three times at this task's HEAD:

| run | log | exit |
|---|---|---|
| 1 | `run-1.log` | `run-1.exitstatus` = 0 |
| 2 | `run-2.log` | `run-2.exitstatus` = 0 |
| 3 | `run-3.log` | `run-3.exitstatus` = 0 |

All three: `4 scenarios (4 passed)`.

## What is now passing unweakened

`status_attach.feature`'s second scenario carries the assertions review's
finding-801 row flagged as at risk of being weakened to accommodate the old
unconditional repair. None of them were touched (confirmed by the
diff-empty proof above), and all of them are among the assertions now
passing in the three green runs above:

- the **`!` unseen-marker assertions** — `within one configured reconcile
  interval deck client "A" row "failed prompt" contains "!"` (line 27) and
  `after one configured reconcile interval deck client "A" row "failed
  prompt" does not contain "!"` (line 30) — both still require the hook
  error's own unseen marker to appear, then clear only once the user
  attaches;
- the **`acknowledged=1`/one-attached-event assertion** —
  `Then the state database session "failed prompt" is "error" from "hook"
  with acknowledged=1, notify_epoch=0, and 1 attached event` (line 29) —
  still requires the row to keep its `hook`-sourced `error` verdict
  (`notify_epoch` unchanged at 0) while recording exactly one attach event
  and flipping `acknowledged` to 1.

None of these were re-pointed onto the repair's post-repair state
(`starting`/`tmux`) the way `bedb65a` (task 803, since reverted by task 903)
re-pointed `status_claude_hooks.feature`'s equivalent assertion — the
product changed (task 902) so the scenario's original, hook-truth-preserving
assertions hold as written.
