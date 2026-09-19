# Phase 4b — whole-suite gate sweep (task 021)

Freeze-line sweep: the whole Go test suite, no test-name filter, no narrowed
package list, run in the CI container at this task's launch sha.

**Result at the launch sha: RED (exit 1).** One package, `features`, failed
with two load-sensitive timeouts; every other package is green. The two red
lanes are carved into new tasks and are deliberately NOT patched under this
task — see [Red lanes](#red-lanes) below.

## The gate run of record

- **Code sha (this task's launch sha):**
  `baf92eda072f6b59ba9b788340f71ed798077504`
  (`baf92ed`, "docs: re-verify task 019's group-delete branches at HEAD
  (R131 part 2, #30)"), tree clean at launch, `HEAD == origin/main`.
  The last code-touching commit below it is `d179b2c`
  ("tmux: wait out a slow pipe-pane job instead of timing out on it"), so
  `baf92ed` and `d179b2c` carry identical Go sources.
- **Invocation:** `ci/run.sh go test -p=1 -count=1 ./...` (the CI sibling
  container wrapper over the module's packages; `-p=1` serialises package
  execution the same way the planning-time baseline measured it). No `-run`
  filter, no package list, no `DECK_GODOG_TAGS` override.
- **Wall clock:** 489s (8m09s) — start `2026-09-19T20:12:49Z`, end
  `2026-09-19T20:20:58Z`. The planning-time baseline at launch sha `c3b530a`
  was 438s green; the extra ~50s is the failing scenario's own 47s timeout
  plus the cure wave's new tests.
- **Exit status:** `1` (RED), taken from the run's own exit code, captured in
  [`exit-status-baf92ed.txt`](./exit-status-baf92ed.txt) (`EXIT:1`) together
  with the sha and both timestamps.
- **Captured log:** [`fullsuite-baf92ed.log`](./fullsuite-baf92ed.log)
  (committed beside this README, 6602 lines, unfiltered including the failing
  scenario's frame dumps).

## Package result lines (from the captured log)

```
ok  	github.com/n-orlov/deck/cmd/deck	7.445s
ok  	github.com/n-orlov/deck/cmd/fake-claude	0.789s
ok  	github.com/n-orlov/deck/cmd/fake-codex	0.118s
ok  	github.com/n-orlov/deck/cmd/fake-pi	0.769s
FAIL	github.com/n-orlov/deck/features	413.717s
ok  	github.com/n-orlov/deck/internal/agent	0.007s
ok  	github.com/n-orlov/deck/internal/audit	0.018s
ok  	github.com/n-orlov/deck/internal/config	0.024s
ok  	github.com/n-orlov/deck/internal/hookrecv	4.427s
ok  	github.com/n-orlov/deck/internal/interactive	15.412s
?   	github.com/n-orlov/deck/internal/notify	[no test files]
?   	github.com/n-orlov/deck/internal/search	[no test files]
ok  	github.com/n-orlov/deck/internal/service	7.128s
ok  	github.com/n-orlov/deck/internal/store	3.620s
ok  	github.com/n-orlov/deck/internal/theme	0.007s
ok  	github.com/n-orlov/deck/internal/tmux	25.706s
ok  	github.com/n-orlov/deck/internal/tui	6.408s
?   	github.com/n-orlov/deck/internal/unit	[no test files]
```

18 package lines in total: **14 `ok`, 1 `FAIL` (`features`), and 3
`[no test files]`** (`internal/notify`, `internal/search`, `internal/unit` —
the out-of-scope Phase 5/6/7 packages that stay one-line `doc.go` per the
standing rules). The tally is reproducible from the committed log and the
module's own package list:

```
$ grep -cE '^ok[ \t]'   fullsuite-baf92ed.log   # 14
$ grep -cE '^FAIL[ \t]' fullsuite-baf92ed.log   # 1
$ grep -c 'no test files' fullsuite-baf92ed.log # 3
$ ci/run.sh go list ./... | wc -l               # 18
```

so no package the module resolves is missing from the log. Every package ran
(or reported `[no test files]`); none was excluded from the invocation.

## Red lanes

Both failures are inside the `features` package and both are timeouts in the
test harness's own waits, hit under whole-suite load:

1. `TestFeatures/settings' group-delete d branch routes through the same dd
   batch confirm and one u restores the whole batch (R131 part 2)` (47.16s) —
   `features/kill_delete_undo.feature:688`, failing at its step on line 701
   (`creates shell session "groups-delete-dd-one" into group "batch-group"`)
   with `after scenario hook failed: timed out waiting for frame "starting":
   context deadline exceeded`. The frame the log dumps shows the session
   already at `running`: the wait is for a transient `starting` label the
   client had already left behind.
2. `TestThemeChangesAttributesButNotFrameGeometry/ascii` (0.75s) —
   `features/theme_geometry_test.go:185`, `theme "parchment" (ascii=true)
   frame kept changing after settling`. The helper compares two snapshots
   100ms apart (`features/theme_geometry_test.go:151-157`) and treats any
   difference as a still-settling frame.

godog's own summary for the run: `362 scenarios (361 passed, 1 failed)` /
`4343 steps (4314 passed, 1 failed, 28 skipped)`. The 28 skipped steps are
the remainder of that one failed scenario, which godog skips after a step
fails — not a skip anyone requested.

### Isolation re-runs (diagnostics, not a substitute for the gate)

At the same sha, each red lane was re-run on its own and passed:

| lane | invocation | result | log |
| --- | --- | --- | --- |
| theme geometry | `ci/run.sh go test -count=1 -run TestThemeChangesAttributesButNotFrameGeometry ./features/` | `ok  ... 1.872s`, exit 0 | [`isolation-theme-geometry.log`](./isolation-theme-geometry.log) |
| kill/delete/undo feature | `ci/run.sh env DECK_GODOG_PATHS=kill_delete_undo.feature go test -count=1 -run TestFeatures ./features/` | `ok  ... 34.251s`, exit 0 | [`isolation-kill-delete-undo.log`](./isolation-kill-delete-undo.log) |

So both are load-sensitive, not deterministic failures. That does not make
the gate green: **the gate's own exit status at `baf92ed` is 1**, and it
stands as the result of record. A lane that is red on the gate IS red.

### How the fix is being handled

Per this run's standing rule — "if a sweep finds a red lane and the fix is
code, that fix is a NEW task and the sweep is re-run from scratch afterwards
— never re-run under the task that found the red" — nothing was patched
under task 021 and the gate was not re-run to chase a green. The cure of the
two harness waits, and the fresh from-scratch gate at the cured sha, are
carved into their own follow-up tasks (`021-cure-01`, `021-resweep-01`).
This file is updated by that re-sweep task with the new sha and its exit
status; until then the red result above is this phase's gate result.

Note for the two sweeps that gate at "the sha task 021 gated"
(`docs/reports/phase4b-guards`, `docs/reports/phase4b-stability10`): that sha
is `baf92ed` unless the cure above has landed, in which case they must name
the sha they actually measured.

## Skips

The one skip in force is godog's own default tag filter,
`~@real-agents && ~@nightly` (`features/godog_test.go:16`, `defaultTags`),
which this invocation used unmodified — no `DECK_GODOG_TAGS` override was
set. No other marker, mode or package was skipped: the invocation carried no
`-run` filter and no narrowed package list, and every package the log lists
ran (or reported `[no test files]`) under that single default godog tag
filter. The 28 skipped godog steps noted above are the consequence of the
failed step in one scenario, not a skip directive.

## Earlier run at `0806ba6` (history, superseded)

The first recording of this task gated at
`0806ba64ede4356af50b404affd10f26f68d4d84` (task 020, the last code-touching
commit of the numbered plan): `ci/run.sh go test -p=1 -count=1 ./...`, 440s,
exit `0` (green), 15 `ok` + 3 `[no test files]`, log kept as
[`fullsuite.log`](./fullsuite.log). An early revision of this README
miscounted those `ok` lines as 14; the committed log always held 15, and the
count was corrected in `f97774f` without re-running anything.

That run no longer describes the tree: eight code-touching commits landed
after it (`60a551d`, `f77368f`, `db0f94b`, `386649d`, `42c1ffc`, `43b7202`,
`57e1a6a`, `d179b2c` — the review-cure wave plus the pipe-pane wait cure),
18 non-doc files in all. That is why the gate was re-run at this task's
current launch sha, and why the green 440s figure must not be quoted as this
phase's gate result.
