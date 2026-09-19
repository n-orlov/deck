# Phase 4b — whole-suite gate sweep (task 021)

Freeze-line sweep: the whole Go test suite, no test-name filter, no narrowed
package list, run in the CI container at this task's launch sha.

**Result at the re-sweep sha (`a224e43`, this task's launch sha): GREEN
(exit 0).** Both red lanes the launch-sha gate (`baf92ed`) hit are cured
(`021-cure-01` at `db9732b`, `021-cure-02` at `a224e43`) and this
from-scratch re-run of the whole suite confirms it: all 15 tested packages
`ok`, including `features`, with no test-name filter and no narrowed
package list. The superseded `baf92ed` RED result stays in the record as
history — see [Red lanes at the launch sha](#red-lanes-at-the-launch-sha-baf92ed-history--both-cured-confirmed-green-above)
below; its log and exit-status file
([`fullsuite-baf92ed.log`](./fullsuite-baf92ed.log),
[`exit-status-baf92ed.txt`](./exit-status-baf92ed.txt)) are committed
unchanged beside this README's new `a224e43` files.

## The gate run of record

- **Code sha (this task's launch sha):**
  `a224e4339173bad93342abdeb5e946da71753090`
  (`a224e43`, "features: retry the theme-geometry settle guard instead of
  one fixed 100ms sample (#30)", `021-cure-02`), tree clean at launch,
  `HEAD == origin/main`. This is the sha both cure tasks land at
  (`021-cure-01` at `db9732b`, `021-cure-02` at `a224e43` itself).
- **Invocation:** `ci/run.sh go test -p=1 -count=1 ./...` (the CI sibling
  container wrapper over the module's packages; `-p=1` serialises package
  execution the same way the planning-time baseline measured it). No `-run`
  filter, no package list, no `DECK_GODOG_TAGS` override.
- **Wall clock:** 463s (7m43s) — start `2026-09-19T22:19:27Z`, end
  `2026-09-19T22:27:10Z`. The launch-sha (`baf92ed`) RED run was 489s
  (its own 47s harness timeout inflated that number); the planning-time
  green baseline at `c3b530a` was 438s. 463s sits between the two, in line
  with a green run carrying the cure wave's new/changed tests but no
  timeout.
- **Exit status:** `0` (GREEN), taken from the run's own exit code,
  captured in [`exit-status-a224e43.txt`](./exit-status-a224e43.txt)
  (`EXIT:0`) together with the sha and both timestamps.
- **Captured log:** [`fullsuite-a224e43.log`](./fullsuite-a224e43.log)
  (committed beside this README, 18 lines — the package result lines only;
  this run produced no per-scenario frame dumps because nothing failed).

## Package result lines (from the captured log)

```
ok  	github.com/n-orlov/deck/cmd/deck	7.443s
ok  	github.com/n-orlov/deck/cmd/fake-claude	0.791s
ok  	github.com/n-orlov/deck/cmd/fake-codex	0.119s
ok  	github.com/n-orlov/deck/cmd/fake-pi	0.770s
ok  	github.com/n-orlov/deck/features	367.756s
ok  	github.com/n-orlov/deck/internal/agent	0.007s
ok  	github.com/n-orlov/deck/internal/audit	0.018s
ok  	github.com/n-orlov/deck/internal/config	0.029s
ok  	github.com/n-orlov/deck/internal/hookrecv	4.431s
ok  	github.com/n-orlov/deck/internal/interactive	15.327s
?   	github.com/n-orlov/deck/internal/notify	[no test files]
?   	github.com/n-orlov/deck/internal/search	[no test files]
ok  	github.com/n-orlov/deck/internal/service	6.917s
ok  	github.com/n-orlov/deck/internal/store	3.257s
ok  	github.com/n-orlov/deck/internal/theme	0.007s
ok  	github.com/n-orlov/deck/internal/tmux	25.763s
ok  	github.com/n-orlov/deck/internal/tui	6.499s
?   	github.com/n-orlov/deck/internal/unit	[no test files]
```

18 package lines in total: **15 `ok`, 0 `FAIL`, and 3 `[no test files]`**
(`internal/notify`, `internal/search`, `internal/unit` — the out-of-scope
Phase 5/6/7 packages that stay one-line `doc.go` per the standing rules).
`features` is now `ok` (367.756s) where the launch-sha gate had it `FAIL`
(413.717s). The tally is reproducible from the committed log and the
module's own package list:

```
$ grep -c  '^ok'           fullsuite-a224e43.log   # 15
$ grep -cP '^FAIL\t'        fullsuite-a224e43.log   # 0   (the package line)
$ grep -c  'no test files'  fullsuite-a224e43.log   # 3
$ ci/run.sh go list ./... | wc -l                   # 18
```

(`go test` separates a package line's verdict from its import path with a
tab, so the `FAIL` package line is matched on that tab — `grep -c '^FAIL'`
would count 3, picking up the two bare `FAIL` markers godog and `go test`
also print. An earlier revision of this block used an `[ ]`-class pattern
that never matched a tab and so printed 0; the counts in the paragraph above
were always the log's own.)

so no package the module resolves is missing from the log. Every package ran
(or reported `[no test files]`); none was excluded from the invocation.

## Red lanes at the launch sha (`baf92ed`, history — both cured, confirmed green above)

Both failures were inside the `features` package and both were timeouts in
the test harness's own waits, hit under whole-suite load:

1. `TestFeatures/settings' group-delete d branch routes through the same dd
   batch confirm and one u restores the whole batch (R131 part 2)` (47.16s) —
   `features/kill_delete_undo.feature:688`, failing at its step on line 701
   (`creates shell session "groups-delete-dd-one" into group "batch-group"`)
   with `after scenario hook failed: timed out waiting for frame "starting":
   context deadline exceeded`. The frame the log dumps shows the session
   already at `running`: the wait is for a transient `starting` label the
   client had already left behind. **Cured** at `db9732b` (`021-cure-01`):
   the group-create wait no longer races the shell reconcile fast-path.
2. `TestThemeChangesAttributesButNotFrameGeometry/ascii` (0.75s) —
   `features/theme_geometry_test.go:185`, `theme "parchment" (ascii=true)
   frame kept changing after settling`. The helper compared two snapshots
   100ms apart (`features/theme_geometry_test.go:151-157`) and treated any
   difference as a still-settling frame. **Cured** at `a224e43`
   (`021-cure-02`): the settle guard now retries every 100ms and accepts
   the first agreeing consecutive pair, failing only after a 5s
   `settleTimeout` never sees one.

godog's own summary for the `baf92ed` run: `362 scenarios (361 passed, 1
failed)` / `4343 steps (4314 passed, 1 failed, 28 skipped)`. The 28 skipped
steps were the remainder of that one failed scenario, which godog skips
after a step fails — not a skip anyone requested. This re-sweep's own godog
summary is inside [`fullsuite-a224e43.log`](./fullsuite-a224e43.log)'s
`features` run (367.756s, `ok`) — no scenario failed, so no steps were
skipped for that reason.

### Isolation re-runs (diagnostics at the time, not a substitute for the gate)

At the `baf92ed` sha, each red lane was re-run on its own and passed even
before either cure landed — the evidence that both were load-sensitive, not
deterministic:

| lane | invocation | result | log |
| --- | --- | --- | --- |
| theme geometry | `ci/run.sh go test -count=1 -run TestThemeChangesAttributesButNotFrameGeometry ./features/` | `ok  ... 1.872s`, exit 0 | [`isolation-theme-geometry.log`](./isolation-theme-geometry.log) |
| kill/delete/undo feature | `ci/run.sh env DECK_GODOG_PATHS=kill_delete_undo.feature go test -count=1 -run TestFeatures ./features/` | `ok  ... 34.251s`, exit 0 | [`isolation-kill-delete-undo.log`](./isolation-kill-delete-undo.log) |

That did not make the `baf92ed` gate green: **that gate's own exit status
was 1**, and it stands in the record as the result of that run. A lane that
is red on the gate IS red, whatever an isolated re-run shows — which is
exactly why this task re-ran the whole suite from scratch rather than
trusting the isolation re-runs or the cure commits' own targeted tests.

### How the fix was handled

Per this run's standing rule — "if a sweep finds a red lane and the fix is
code, that fix is a NEW task and the sweep is re-run from scratch afterwards
— never re-run under the task that found the red" — nothing was patched
under task 021 itself and that gate was not re-run to chase a green. Three
follow-up tasks carried the work instead, one per red lane plus this
re-sweep:

| task | carried | landed at |
| --- | --- | --- |
| `021-cure-01` | the `kill_delete_undo.feature` create-step `starting` wait (red lane 1) | `db9732b` |
| `021-cure-02` | the `theme_geometry_test.go` settle comparison (red lane 2) | `a224e43` |
| `021-resweep-01` | the whole-suite gate re-run from scratch at the cured sha (this task) | `a224e43`'s tree, this file's own commit |

Both cures re-ran their own lane 5x sequential + 3x concurrent green before
landing (write-up: `docs/reports/phase4b-cure021/README.md`, one section
each) — this re-sweep is the from-scratch whole-suite confirmation the
standing rule requires on top of that, not a substitute for it.

Note for the two sweeps that gate at "the sha task 021 gated"
(`docs/reports/phase4b-guards`, `docs/reports/phase4b-stability10`): those
records still measured `baf92ed` and have not been re-taken at `a224e43` —
this re-sweep does not by itself supersede their own numbers; whoever next
touches those reports must decide whether they need a re-take.

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
