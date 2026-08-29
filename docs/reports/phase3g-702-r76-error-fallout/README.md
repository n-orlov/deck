# Phase 3g — task 702: R76's error-branch fallout, enumerated (no scenario edited)

## Sha this bundle is measured at

```
$ git rev-parse HEAD
89edd3c6fbcab8e39f9e9ebe24976747e635b920
$ git log -1 --oneline
89edd3c reconcile: give the terminal-row repair its own status test so a bare hook/probe error row under a live pane is repaired too (task 701)
```

`89edd3c` is task 701's own commit and, at the time this task ran, `HEAD == origin/main`
with a clean tree — the "post-701 sha" the task names. This task adds only the
`docs/reports/phase3g-702-r76-error-fallout/` directory; no `.go`, `.feature` or other
product file is touched (`git diff --stat 89edd3c..HEAD -- '*.go' '*.feature'` is empty,
confirmed again below).

## What was run, and how

Both runs used the mandated launcher `ci/run.sh`, backgrounded with `nohup timeout N …
&` and polled — never blocked on — exactly per the standing rules, with the exit status
written to a file by the same `sh -c` that ran the command (never inferred from the
absence of `FAIL`).

**Note on the features run:** the first launch of `ci/run.sh go test -count=1
./features/` was started as a plain `nohup timeout 1800 … &` without an exit-status
capture. Before it finished, that gap was noticed; the run was stopped cleanly
(`docker stop` on the exact container id it had created, which then removed itself —
`--rm`) and relaunched with the exit-status-capturing form below. The log kept in this
directory is the relaunch's log; the aborted first attempt was truncated (0 log lines
written to a temp file, not committed anywhere) and left no artifact worth keeping.

### `./features/`

```
$ git rev-parse HEAD
89edd3c6fbcab8e39f9e9ebe24976747e635b920
$ git status --short
(clean)
$ nohup sh -c 'timeout 1800 ci/run.sh go test -count=1 ./features/ > /run/ralphd/artifacts/702/features.log 2>&1; s=$?; printf "%s\n" "$s" > /run/ralphd/artifacts/702/features.exitstatus' &
```

Polled with `sleep N; tail -N …` only, never a blocking wait:

```
$ sleep 300; tail -10 /run/ralphd/artifacts/702/features.log     # not finished yet
$ sleep 60;  tail -10 /run/ralphd/artifacts/702/features.log     # log complete; exitstatus file present
```

**Captured exit status: `1`**, read from
[`features-suite.exitstatus`](features-suite.exitstatus), written by the same `sh -c`
that ran the command. Wall time (from the log's own summary line): **337.440s**
(`FAIL github.com/n-orlov/deck/features 337.440s`) — close to the ~5m14s (314s) measured
at planning; the difference is ordinary run-to-run variance on a shared CI sibling, not
a regression in itself.

Full unedited log, unmodified byte-for-byte from the run: [`features-suite.log`](features-suite.log)
(7,810 lines, 3.97 MB — the non-verbose launcher still prints godog's own "pretty"
formatter output for `./features/`, unlike the terse per-package summary `./...`
prints, so this log is large by the nature of the launcher, not by anything done here).

### `./internal/tui/ ./internal/hookrecv/ ./internal/service/`

```
$ nohup sh -c 'timeout 600 ci/run.sh go test -count=1 ./internal/tui/ ./internal/hookrecv/ ./internal/service/ > /run/ralphd/artifacts/702/internal.log 2>&1; s=$?; printf "%s\n" "$s" > /run/ralphd/artifacts/702/internal.exitstatus' &
$ sleep 25; tail -20 /run/ralphd/artifacts/702/internal.log; cat /run/ralphd/artifacts/702/internal.exitstatus
```

**Captured exit status: `0`**, read from
[`internal-packages.exitstatus`](internal-packages.exitstatus). Full unedited log
(3 lines, all `ok`, no `FAIL`): [`internal-packages.log`](internal-packages.log):

```
ok  	github.com/n-orlov/deck/internal/tui	1.598s
ok  	github.com/n-orlov/deck/internal/hookrecv	5.500s
ok  	github.com/n-orlov/deck/internal/service	5.103s
```

None of these three packages turned red. (`internal/hookrecv` and `internal/service`
are the packages closest to R76's own code path; `internal/tui` renders the rows the
scenarios below assert against. All three stay green — the fallout is entirely in the
black-box `features` suite, where a live tmux pane is actually driven.)

## The complete list of scenarios `./features/` turned red

`311 scenarios (302 passed, 9 failed)` / `3523 steps (3472 passed, 9 failed, 42
skipped)` (quoted from `features-suite.log`, `grep -n '^[0-9]* scenarios' features-suite.log`).
**All 9 failures are read as R76's error branch working as designed**: every one of
them writes (or has fired into it) a bare `error` status — via direct state-database
injection, a probe-sourced write, or a genuine Claude `StopFailure` hook — onto a
session whose tmux pane is still alive and un-exited. Before task 701, that bare
`error` row was never reached by `repairTerminalRowWithLivePane` (nested inside
`terminal`, which excludes `error`) and so sat untouched; after 701 it is now a
`stopped || error` status match on its own, independent of `terminal`, and the
reconcile repairs it (to `starting`, source `tmux`) on the very next tick — before
each scenario's own assertion about the word `"error"` (or a derived word) ever fires.
No scenario or assertion was edited to produce this list; it is read straight off the
one log above, backed by that log's own exit status (`1`, captured above in the same
shell call as the command).

| # | Scenario | Feature file:line | Go subtest name (from `--- FAIL`) | Why R76 turns it red |
|---|----------|--------------------|-------------------------------------|-----------------------|
| 1 | sessions render in the full waiting/error/running/starting/idle/stopped order, ties broken oldest-first | `attention_sort.feature:14` | `TestFeatures/sessions_render_in_the_full_waiting/error/running/starting/idle/stopped_order,_ties_broken_oldest-first` | `s-error`'s state-db row is written `error` 40s ago on a live shell pane (`internal/service/reconcile.go`'s own `And the state database session "s-error" has status "error" 40 seconds ago`); R76 now repairs it to `starting` before the ordering assertion, so `s-error` never sorts into the `error` bucket and the expected row order (`s-waiting-old, s-waiting-new, s-error, s-running, …`) breaks — the log's own diagnostic is `deck client renders session "s-running" at line 7, want strictly after session "s-error" at line 9`. |
| 2 | the collapsed strip's attention count matches the sort's own notion of attention | `attention_sort.feature:92` | `TestFeatures/the_collapsed_strip's_attention_count_matches_the_sort's_own_notion_of_attention` | Same pattern: `cnt-err` is written `error` 5s ago on a live pane; R76 repairs it before `Then within one configured reconcile interval deck client "A" row "cnt-err" contains "error"` fires, so that step itself times out (`client "A" row "cnt-err" did not contain "error" within reconcile interval`), before the collapsed-strip attention count is even checked. |
| 3 | `space` walks only what needs attention, wraps, and changes no session's status | `attention_sort.feature:110` | ``TestFeatures/`space`_walks_only_what_needs_attention,_wraps,_and_changes_no_session's_status`` | `sp-2` is written `error` 3s ago on a live pane, expected to need attention and be the first `space` target; R76 repairs it to `starting` (which is not an attention state) before `space` is ever pressed, so the walk lands elsewhere and the client never shows the expected `"> sp-2"` frame — `deck client "A" does not have session "sp-2" selected: timed out waiting for frame "> sp-2"`. |
| 4 | scrolling the interactive grid's own scrollback never flips the session's badge, and the probe reads the pane's live bottom, not the scrolled-back view | `interactive_scroll.feature:86` | `TestFeatures/scrolling_the_interactive_grid's_own_scrollback_never_flips_the_session's_badge,_and_the_probe_reads_the_pane's_live_bottom,_not_the_scrolled-back_view` | `the state database session "ig-claude" has probe status "error" with reason "api error"` on a live claude pane; the scenario expects the row to still read a probe-derived `"sampled"` badge next. R76 repairs the bare `error` row to `starting`/`tmux` first, so the probe-sourced `"sampled"` text this step waits for never appears — `client "B" row "ig-claude" did not contain "sampled" within reconcile interval`. This is the probe-sourced case explicitly flagged as expected fallout in the handoff notes. |
| 5 | attach acknowledges a live error without replacing its verdict | `status_attach.feature:18` | `TestFeatures/attach_acknowledges_a_live_error_without_replacing_its_verdict` | A genuine Claude `StopFailure` hook fires `error`/`tool_failure` on a live pane (`StatusSource: "hook"`, not a test fixture writing the DB directly) — R76's own status test (`Status == "stopped" \|\| Status == "error"`) has no `StatusSource` exception, so this real hook-sourced error is repaired too, before the scenario attaches and checks the verdict survives. Log: `session "failed prompt" = status "starting" source "tmux" reason "tmux pane is alive; terminal row corrected" … want "error" hook "tool_failure" …`. Not previously called out in the fallout notes — a genuine Claude hook error, not just a probe/test-fixture one, is now also repaired. |
| 6 | Every declared Claude hook maps to honest status through both identity routes | `status_claude_hooks.feature:6` | `TestFeatures/Every_declared_Claude_hook_maps_to_honest_status_through_both_identity_routes` | Same mechanism as #5: a `StopFailure`/`tool_failure` hook fires on a live pane partway through this scenario's hook-by-hook walk; R76 repairs it before the scenario's next assertion reads the hook-derived `"error"` state, so the walk's remaining steps see `starting`/`tmux` instead. Log: `session "hook truth" = status "starting" source "tmux" … want "error" hook "tool_failure" …`. |
| 7 | the error status token colours the error status word | `status_theme.feature:75` | `TestFeatures/the_error_status_token_colours_the_error_status_word` | `tok-target`'s state-db row is written `error` 5s ago on a live pane; R76 repairs it before `Then … screen contains "error"` fires, so the word never renders and the step times out — `client "A" did not show "error" within 500ms`. This is exactly the fallout the handoff notes predicted for `status_theme.feature`. |
| 8 | statuses never borrow each other's colour | `status_theme.feature:89` | `TestFeatures/statuses_never_borrow_each_other's_colour` | Identical setup to #7 (`tok-target` written `error` 5s ago on a live pane); same repair-before-assertion race, same timeout waiting for the literal word `"error"`. |
| 9 | a built-in theme colours each of the seven §7 status tokens, read per cell from a real client (Examples row `tok49-err`, `subject=target`, `status=error`) | `themes.feature:21` (Scenario Outline; the failing example is the row naming client `tok49-err`) | `` TestFeatures/a_built-in_theme_colours_each_of_the_seven_§7_status_tokens,_read_per_cell_from_a_real_client#04 `` | The Outline's 5th example row writes `target`'s state-db status to `error` 5 seconds ago on a live pane and then asserts the screen shows `"error"` with the theme's `error` foreground token; R76 repairs the bare row first, so the word (and its coloured token) never appear — `client "tok49-err" did not show "error" within 500ms`. This is the `themes.feature` error-token row the handoff notes named explicitly. |

**Scenarios/tests that did *not* turn red because of R76, checked explicitly:** the
`stopped`-status theme scenario in `themes.feature` (a comment on that very scenario
already explains stopped rows on live panes get repaired to `running`, unrelated to
this change) stayed green, as did every other status-token row in the same Outline
(`waiting`, `running`, `idle`, `starting`, `archived`). `status_probe.feature` and
`launch_lease.feature` — both named as *possible* hits in the pre-run notes — did **not**
turn red in this run; neither did `sort_order.feature`. `internal/tui`,
`internal/hookrecv` and `internal/service` (checked above, exit 0) turned up nothing.
The self-checks `TestGodogRejectsUndefinedAndFailedSteps/undefined` and `/failed` in
`features/godog_test.go` are unrelated meta-tests that deliberately feed godog a broken
scenario to prove godog's own `Strict` rejection works; their printed `FAIL`/`undefined`
text in the log (lines 7781-7810) is that meta-test's *expected* internal output, not a
tenth failure — the Go test itself only fails if godog *accepts* the broken input, which
it did not.

## No product code, `.feature` or `features/*_test.go` file was touched

```
$ git diff --stat 89edd3c..HEAD -- '*.go' '*.feature'
(empty)
$ git status --short
?? docs/reports/phase3g-702-r76-error-fallout/
```

This task is a read-only enumeration. Fixing any of the 9 scenarios above (deciding,
per SPEC §7, whether each should now read the repaired `starting` state, or whether the
step itself needs a different real transition to still reach `error`) is separate,
follow-up work — out of scope for this task by its own title ("without editing any
scenario").
