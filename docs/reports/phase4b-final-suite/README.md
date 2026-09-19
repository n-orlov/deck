# Phase 4b — whole-suite gate sweep (task 021)

Freeze-line sweep: the whole Go test suite, no test-name filter, no narrowed
package list, run in the CI container at task 021's launch sha — and, under
the follow-up task the red lanes were carved into, again from scratch at the
cured sha.

This file records TWO whole-suite runs, at two different shas, because two
tasks gate here:

1. **Task 021's own gate, at task 021's launch sha `baf92ed`: RED (exit
   1), 489s** — the run this task is accountable for. Its log
   ([`fullsuite-baf92ed.log`](./fullsuite-baf92ed.log)) and exit-status
   file ([`exit-status-baf92ed.txt`](./exit-status-baf92ed.txt)) are
   committed beside this README and stay in the record as history; the RED
   result was never erased by the green one. Section
   [Task 021's gate at its launch sha](#task-021s-gate-at-its-launch-sha-baf92ed--red-exit-1)
   below carries its invocation, wall clock, real exit status and every
   package result line.
2. **Task `021-resweep-01`'s from-scratch re-run, at ITS launch sha
   `a224e43`: GREEN (exit 0), 463s** — the confirmation that both red lanes
   task 021 found are cured (`021-cure-01` at `db9732b`, `021-cure-02` at
   `a224e43`). All 15 tested packages `ok`, including `features`, with no
   test-name filter and no narrowed package list.

`a224e43` is NOT task 021's launch sha (an earlier revision of this README
said so, wrongly): task 021 launched from `baf92ed`, and `a224e43` is where
the second cure landed and where the re-sweep task launched from.

## The re-sweep run of record (`021-resweep-01`)

- **Code sha (task `021-resweep-01`'s launch sha):**
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

## Package result lines of the re-sweep (from `fullsuite-a224e43.log`)

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

## Task 021's gate at its launch sha (`baf92ed`) — RED (exit 1)

- **Code sha (task 021's launch sha):**
  `baf92eda072f6b59ba9b788340f71ed798077504` (`baf92ed`, "docs: re-verify
  task 019's group-delete branches at HEAD (R131 part 2, #30)").
- **Invocation:** `ci/run.sh go test -p=1 -count=1 ./...` — no `-run`
  filter, no package list, no `DECK_GODOG_TAGS` override.
- **Wall clock:** 489s (8m09s) — start `2026-09-19T20:12:49Z`, end
  `2026-09-19T20:20:58Z`.
- **Real exit status:** `1` (RED), taken from the run's own exit code, in
  [`exit-status-baf92ed.txt`](./exit-status-baf92ed.txt)
  (`SHA:baf92ed…` / `EXIT:1` / `SECONDS:489`).
- **Captured log:** [`fullsuite-baf92ed.log`](./fullsuite-baf92ed.log),
  committed beside this README (6602 lines — the failing scenario's frame
  dumps are why it is large).

Package result lines from that log — 18 in total: 14 `ok`, 1 `FAIL`, 3
`[no test files]`:

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

Reproducible from the committed log with the same tab-anchored patterns used
for the re-sweep above: `grep -cE '^ok' fullsuite-baf92ed.log` → 14,
`grep -cP '^FAIL\t' fullsuite-baf92ed.log` → 1,
`grep -c 'no test files' fullsuite-baf92ed.log` → 3 (14+1+3 = 18 = the
module's own package count).

### The two red lanes (both cured; the cured sha is confirmed green above)

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
after a step fails — not a skip anyone requested. The re-sweep's own log
([`fullsuite-a224e43.log`](./fullsuite-a224e43.log)) carries no godog
summary at all: the invocation passed no `-v`, so `go test` prints a
passing package's output nowhere — the log is the 18 package lines and
nothing else, and `features` is `ok` (367.756s) among them. No scenario
failed there, so no steps were skipped for that reason.

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
exactly why a separate task (`021-resweep-01`) re-ran the whole suite from
scratch rather than trusting the isolation re-runs or the cure commits' own
targeted tests.

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
| `021-resweep-01` | the whole-suite gate re-run from scratch at the cured sha | `7370e1d` (README update; suite run against `a224e43`'s tree) |

All three are real entries in this run's own plan (`tasks.json`), each
carrying `splitFrom: "021"`, and all three stand `validated` — that is the
checkable form of "carved into a new task rather than patched under this
one", and it is what makes the table above a record rather than an
intention. A first attempt to carve them was refused by the harness on
lint grounds and no task existed for a while; the accepted proposal is the
one whose ids are listed here. Verify with:

```
$ python3 -c "import json;d=json.load(open('tasks.json'));
print([(t['id'],t['status'],t.get('splitFrom')) for t in d['tasks']
       if t.get('splitFrom')=='021'])"
```

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
18 non-doc files in all. That is why the gate was re-run at task 021's own
launch sha `baf92ed`, and why the green 440s figure must not be quoted as
this phase's gate result.

## What these two runs do and do not cover

Both runs are pinned to their own shas: `baf92ed` (task 021, RED) and
`a224e43` (`021-resweep-01`, GREEN). Four code-touching commits landed after
`a224e43` — `4a1e352`, `8d934d1` (the R133 offset cure, `cure-01-01-2`) and
`8f8e9e8`, `f13c848` (the R131 settings-visibility cure, `cure-01-02-2`), all
under `internal/tui/` — so neither log describes the tree at any sha later
than `a224e43`. Those cures carry their own package-level evidence in
`docs/reports/phase4b-retake-01-01-2/` and
`docs/reports/phase4b-retake-01-04-01/`; a whole-suite gate at a sha later
than `a224e43` belongs to whichever task gates there, and nothing in this
file should be read as one.
