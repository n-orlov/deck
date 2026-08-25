# Phase 3e report

Skeleton created by task 322 while closing out R58c (the padTrunc/truncateToWidth
call-site audit) — the audit's own success criteria requires its enumeration to
live here, not only in the commit message. Populated incrementally as later
tasks land; task 326 is responsible for filling in the remaining per-requirement
evidence table (R52-R58, R59-R62, the four sort sequences, the truncating
name/width fixture, matrix's palette evidence, and every revert-red proof)
before the phase closes.

## R58c: padTrunc/truncateToWidth call-site audit (task 322, commit `c86e422`)

Enumeration of every padTrunc/truncateToWidth call site, per call site, whether
it could leak an open background/SGR span past its own text into the
pad-fill/flanking columns a caller's own background wrapper spans:

1. `panel.go` `sidebarContentLine` (side-by-side sidebar rows) — task 321
   already fixed this: bg opens after the border, closes once at the end,
   AFTER padTrunc's pad-fill. No change needed here.

2. `panel.go` `fullBoxContentLine` (stacked-mode sidebar/preview panels AND
   every `framedDialog`/`framedDialogScrollable` dialog body) — **REAL GAP,
   FIXED**. Task 321 moved `sidebarRowLines` to open no background of its own
   (`settingsRenderRowOpen`), on the assumption `sidebarContentLine` (its one
   side-by-side caller) was the only site that needed the wrapper treatment.
   `renderStackedFrame`'s sidebar loop feeds the SAME `sidebarEntry.bg` into
   `fullBoxContentLine`, which never got the bg parameter at all — so stacked
   mode painted NO selection/stripe background whatsoever, not merely a
   too-narrow one. `fullBoxContentLine` now takes a `bg theme.Token` and
   opens/closes it exactly like `sidebarContentLine`; `renderStackedFrame`
   threads `sidebarEntry.bg` through. `framedDialog`/`framedDialogScrollable`
   pass `""` (dialogs have no per-row background, only foreground-coloured
   body text via `colorToken`, which self-resets per call — nothing to leak).

3. `panel.go` `collapsedStripContentLine` (3-column collapsed strip) — safe,
   no change. `collapsedStripLines()` emits only the plain »/digit glyphs with
   zero colour tokens (via `m.glyph`, no `colorToken`/`backgroundSGR` call), so
   there is no background, open or otherwise, to leak.

4. `panel.go` `previewContentLine` (preview panel body) — safe, no change.
   Preview content is foreign tmux pane output cropped by `cropRow`, whose two
   `truncateToWidth` calls are already escape-honest via task 320 (a truncated
   span closes itself). `previewContentLine` adds no background of its own
   around that text, so there is nothing this layer could leak that task 320
   doesn't already close.

5. `settings.go` `settingsLeftContentLine`/`settingsRightContentLine` (the
   settings takeover's category/field/env-entry/search-result lists) — **REAL
   GAP, FIXED**. `settingsRenderRow` used to open bg (selection/selection_idle)
   at the very start of a row's text and close it with a SINGLE trailing reset
   immediately after that row's own segments — before
   `settingsLeftContentLine`/`settingsRightContentLine`'s own padTrunc
   pad-fill and flanking padding columns ever ran. Exactly task 321's original
   sidebar bug, one level removed: the highlight stopped at `"> UI"` (4
   columns) instead of spanning the whole panel width. Fixed by introducing
   `settingsListLine{text, bg}` (mirrors `sidebarRowLines`/
   `sidebarContentLine`'s own split exactly): every category/field/env-entry/
   search-result row now composes its text via `settingsRenderRowOpen` (no bg,
   no closing reset) and carries its background token separately;
   `settingsLeftContentLine`/`settingsRightContentLine` open bg once after the
   border and close once at the very end, spanning the pad-fill. `fitLines` is
   now generic (`fitLines[T any]`) so it fits `[]settingsListLine` the same way
   it always fit `[]string` — the zero value (`text ""`, `bg ""`) is exactly
   "no background", matching a truly empty string row under the old shape.
   `settingsRenderRow` itself is now dead (nothing calls it) and removed;
   `settingsRenderRowOpen`'s doc comment updated to say so.

6. `settings.go` `settingsFooterLine` / `tui.go` `belowMinimumNotice` — safe,
   no change. Plain uncoloured text (`truncateToWidth` called directly, no
   `colorToken`/`backgroundSGR` anywhere in the string).

### Non-vacuous evidence

`internal/tui/panel_leak_audit_test.go`, three new tests, each red-first
verified by reverting commit `c86e422`'s `panel.go`/`settings.go`/`tui.go` diff
in place (kept the new test file) and rerunning:

```
TestStackedSidebarSelectionBackgroundFillsFullPanelWidth (gap 2 above)
  reverted: "stacked selected row: row 0 col 1 has no background at
  all, want #26324b" -- FAIL
  fixed:    PASS

TestSettingsCategoryRowBackgroundFillsFullPanelWidth (gap 5, left list)
  reverted: "settings category row: col 1 has no background at all,
  want #26324b" -- FAIL
  fixed:    PASS

TestSettingsFieldRowBackgroundFillsFullPanelWidth (gap 5, right list)
  reverted: "settings field row: col 31 has no background at all, want
  #26324b" -- FAIL
  fixed:    PASS
```

```
$ ci/run.sh go test -count=1 ./internal/tui/
ok   (0.5s)
$ ci/run.sh go build ./...
(clean)
$ ci/run.sh go vet ./internal/tui/...
(clean)
$ gofmt -l internal/tui/*.go
(clean)
```

### Discovered but out of scope for 322

Running the full `@settings` feature tag surfaces TWO pre-existing failures
present already at 321 (`f74ed7f`), unrelated to commit `c86e422` — confirmed
by reverting that commit's diff and rerunning both scenarios in isolation,
which fail identically on the unmodified 321 tree:

- "a settings takeover save of ui.theme changes the running client's own
  render live" (requirement 19) times out cycling the Theme field to
  "daylight": tasks 315/316 added three more built-in themes without updating
  this scenario's +/- cycle count.
- "settings offers clearing the recent-directory history" (requirement 17)
  times out waiting for "cleared recent directory history": task 303 inserted
  `ui.sort_order` between `group_by_workspace` and `clear_recent_cwds` without
  adding the extra `j` this scenario's own in-file comment ("task 215 inserted
  preview_fit... a sixth j is needed") already flags as necessary whenever a
  field is inserted — a seventh `j` is now needed and was never added.

Recorded as task 333 in `tasks.json` (fixed separately, see that task's
evidence once it lands, cross-referenced here by task 326).

## R58d: per-cell background rectangle/seam godog evidence (task 323)

`features/panel_background_rectangle.feature` adds two scenarios read per-cell
off a real running client (never text-scraping), over a fixture with
`[ui] sort_order = "name"` and `group_by_workspace = false` so the row order
is deterministic: `rec-aaa`, `rec-bbb-selected-session-with-a-name-far-too-
long-to-fit-in-the-sidebar-at-all` (79 chars, truncates), `rec-ccc`,
`rec-ddd-stripe`.

Frame-row math (proved with a literal frame dump during authoring, quoted
below): row 0 is the sidebar's own top border, row 1 is the socket-info
header line, so the FIRST session's two lines start at row 2, not row 1 as
an earlier draft of this file assumed. Position 0 (rec-aaa) -> rows 2-3,
position 1 (rec-bbb, selected) -> rows 4-5, position 2 (rec-ccc) -> rows 6-7,
position 3 (rec-ddd-stripe, odd stripe phase) -> rows 8-9. Sidebar content
columns run 1-34, seam is column 35.

Debug frame dump that found the off-by-one (temporary step, removed before
the final commit):

```
+ deck - sessions -----------------+---------------------------------------------------------------+
| socket: deck_test_143_2          | /tmp/deck-scenario-476548515/walking-skeleton-cwd             |
|   rec-aaa running                |                                                               |
|   created <relative-time>               | No live preview captured for this row yet.                    |
| > rec-bbb-selected-session-wi... |                                                               |
|   created <relative-time>               |                                                               |
|   rec-ccc running                |                                                               |
|   created <relative-time>               |                                                               |
|   rec-ddd-stripe running         |                                                               |
|   created <relative-time>               |                                                               |
```

Two new cell-attribute steps were registered (`features/cell_attributes_test.go`):
`cells at row R columns C1 to C2 have background token "T"` (a per-column
loop, so a highlight that stops one column short anywhere in the rectangle
fails, not just at a spot-checked column) and `cell at row R column C has no
background set` (reads `Style.Bg == nil` directly, so "no colour" is never
confused with "some non-matching colour").

Green (`ci/run.sh sh -c 'DECK_GODOG_TAGS="@requirement-58-selection-background-fills-rectangle-and-seam-stays-clear,@requirement-58-surface-stripe-fills-rectangle" go test -count=1 ./features/ -run TestFeatures'`):
2 scenarios, 2 passed, 26 steps passed, 2.69s.

Red proof (`git revert --no-commit c86e422 f74ed7f 408a1b7`, i.e. tasks
322/321/320 reverted, same tree, same tags): 2 scenarios, 2 failed --
`client "A" cell at row 4 column 1: cell " " has no background colour set
(terminal default)` (selection scenario) and `client "A" cell at row 8
column 1: cell " " has no background colour set (terminal default)`
(surface-stripe scenario). Workspace restored with `git reset --hard HEAD`
immediately after (`git status --short` empty, `git diff --stat` empty).

`TestGoldenMinimumFrame` remains green on the final tree; `git diff --stat`
on `features/testdata/golden/side_by_side_80x24.golden` is empty (file
untouched by this task).

## R59: stop writing `probe.miss` as an event (steer 3e-001, task 329)

SPEC §7 amendment quoted in steer 3e-001 §3: "A diagnostic sampling result is
a column, not an event." At the default reconcile cadence one session that
samples but never matches a §7 probe rule was writing a fresh `probe.miss`
row to `events` roughly twice a second, forever, with no reader for the kind
(operator-reported hang; part of the 3e-001 root cause alongside the missing
events indexes (R60) and the render-path store read (R61)).

`internal/store/store.go`'s `RecordProbeMiss` no longer calls
`mutateSessionWithEvent` (which always pairs its UPDATE with an events
INSERT in the same transaction). It now runs a plain
`UPDATE sessions SET last_probe_at = ? WHERE id = ?`, checks `RowsAffected`
itself, and appends nothing to `events`. The `i` detail dialog's "sampled,
no rule matched" line (`internal/tui/tui.go:4064-4067`) is untouched — it
reads only `session.LastProbeAt > session.StatusAt`, never the events table.

Test (`internal/service/reconcile_test.go`,
`TestProbeMissRecordsSampleAgeWithoutTouchingStatus`) drives a session whose
sampled pane provably matches no §7 probe rule (`echo 'nothing recognisable
here'`) through `ReconcileWithProbes`, then asserts BOTH halves together per
steer 3e-001 §6.2: `last_probe_at` advanced to the reconcile clock's `now`,
AND the session's total `events` count is unchanged across the miss (a
separate `SELECT count(*) FROM events WHERE session_id = ? AND kind =
'probe.miss'` is also asserted `= 0`). Asserting only the count-unchanged
half would also pass if `RecordProbeMiss` were deleted outright, which would
break the `i` dialog — hence both halves are required in the same test.

Green (`ci/run.sh go test -count=1 ./internal/store/ ./internal/service/ ./internal/tui/`):
all three `ok` (store 1.835s, service 3.176s, tui 0.698s).

Red proof: `git stash push -- internal/store/store.go` (reverting only the
fix, keeping the new test), then
`ci/run.sh go test -count=1 ./internal/service/ -run TestProbeMissRecordsSampleAgeWithoutTouchingStatus -v`:

```
=== RUN   TestProbeMissRecordsSampleAgeWithoutTouchingStatus
    reconcile_test.go:384: session events count = 1 after probe miss, want unchanged from 0 (a probe miss must never append to events, SPEC §7 amendment)
--- FAIL: TestProbeMissRecordsSampleAgeWithoutTouchingStatus (0.05s)
FAIL
FAIL	github.com/n-orlov/deck/internal/service	0.057s
```

Workspace restored with `git stash pop` immediately after; re-run of the
same test then passes (`ok  	github.com/n-orlov/deck/internal/service	0.059s`).

No other production code references the `probe.miss` kind string;
`internal/tui/badge_detail_test.go`'s probe-miss assertions read
`LastProbeAt`/`StatusAt` directly and needed no change.

## R60: index events (steer 3e-001, task 330)

SPEC.md's amended events DDL (commit `6584299`) declares two indexes that a
fresh v1-created `events` table never had:

```
CREATE INDEX events_at ON events(at DESC, seq DESC);         -- §12's newest-first reads are never a full scan
CREATE INDEX events_session_kind ON events(session_id, kind); -- §6.4's env-apply reads likewise
```

`internal/store/store.go` gains `schemaV5` (both `CREATE INDEX IF NOT EXISTS`
statements) and bumps `SchemaVersion` 4->5; `migrate`'s switch grows a
`case 4: ... fallthrough`-terminated arm so a fresh database (falls through
from case 0) and an existing v1-v4 database (opens directly into case
1/2/3/4) both land on v5 through the same statements. The migration is
index-only -- no `ALTER TABLE`, no row touched.

**Migration correctness** (`internal/store/store_test.go`,
`TestOpenMigratesV4FixtureAddsEventsIndexesWithoutTouchingExistingRows`):
builds a byte-for-byte v4 fixture (schemaV1-V4 statements, `schema_version =
4`) with one `sessions` row and three `events` rows spanning both a real
`session_id` and two orphaned (`NULL`) rows -- the two shapes
`events_session_kind` covers. Opens it via `OpenPath` (migrates to v5 in
place), then asserts: `schema_version = 5`; both `events_at` and
`events_session_kind` exist in `sqlite_master`; the `events` row count is
still exactly 3 and every column of every row (`session_id`, `at`, `kind`,
`reason`, `payload`) reads back byte-identical to what the fixture wrote,
in original `seq` order; the `sessions` row count is still 1 and its `name`
is unchanged. Passes for a fresh database too (every other store test opens
a fresh `OpenPath` and lands on `SchemaVersion = 5` with both indexes
present, since schemaV5 always runs as part of the v1-created chain).

**Query-plan correctness on a seeded LARGE table, never on elapsed time**
(steer 3e-001 §6.1) (`TestListEventsQueryPlanUsesEventsAtIndexOnSeededLargeTable`):
a plan assertion on a small table is vacuous -- SQLite's planner picks either
a scan or an index on a handful of rows in microseconds either way -- so the
test seeds 20,000 `events` rows via a single prepared-statement transaction,
then runs `EXPLAIN QUERY PLAN` on ListEvents' own exact statement
(`SELECT seq, session_id, at, kind, reason, payload FROM events ORDER BY at
DESC, seq DESC LIMIT 200`). Assertion is on the plan TEXT: no row is the
bare string `"SCAN events"` (a full-table scan; with the index the row
reads `"SCAN events USING INDEX events_at"`, so the check is an exact-match
on the bare string, not a substring check, since `"SCAN events"` is itself a
prefix of the indexed form), and no row contains `"TEMP B-TREE"` (an
unindexed `ORDER BY` sorts via a temp b-tree as its own separate plan row).
A final `ListEvents(limit=1)` call over the seeded table proves the index
changed the plan without changing the result (still returns the newest row,
`at = 19999`).

Green (`ci/run.sh go test -count=1 ./internal/store/`): `ok
github.com/n-orlov/deck/internal/store 1.589s` (includes an update to
`TestOpenRefusesNewerFixtureWithoutMutation`, which had hardcoded
`schema_version = 5` as its "one newer than supported" fixture -- now
`SchemaVersion + 1` computed at test time so this task's own 4->5 bump does
not silently turn it into a same-version fixture that skips the refusal
path; a comment records why).

Red proof: `git diff internal/store/store.go > /tmp/task330.patch; git
stash -- internal/store/store.go` (reverts only schemaV5/the migration
switch arm/SchemaVersion, keeping both new tests), then
`ci/run.sh go test -count=1 ./internal/store/ -run
'TestOpenMigratesV4FixtureAddsEventsIndexesWithoutTouchingExistingRows|TestListEventsQueryPlanUsesEventsAtIndexOnSeededLargeTable'
-v`:

```
=== RUN   TestOpenMigratesV4FixtureAddsEventsIndexesWithoutTouchingExistingRows
    store_test.go:1718: index events_at after v4->v5 migration = 0, <nil>; want 1
--- FAIL: TestOpenMigratesV4FixtureAddsEventsIndexesWithoutTouchingExistingRows (0.12s)
=== RUN   TestListEventsQueryPlanUsesEventsAtIndexOnSeededLargeTable
    store_test.go:1847: query plan = [SCAN events USE TEMP B-TREE FOR ORDER BY], want no bare full-table scan of events (events_at index missing or unused)
--- FAIL: TestListEventsQueryPlanUsesEventsAtIndexOnSeededLargeTable (0.09s)
FAIL
FAIL	github.com/n-orlov/deck/internal/store	0.213s
```

Workspace restored with `git stash pop` immediately after; re-run of
`ci/run.sh go test -count=1 ./internal/store/` then passes
(`ok  github.com/n-orlov/deck/internal/store 1.651s`).

## R61: get the event-log store read out of the render path (steer 3e-001, task 331)

SPEC §11.4 amendment (steer 3e-001 §3/§6.3, commit `6584299`): "a dialog
never reads the store from its render path." Before this task, `eventLogBody`
-- called from `eventLogView`, which `View()` calls every frame while `E`'s
dialog is open -- ran `m.store.ListEvents(...)` directly, so a live client
left with the event log open re-queried `events` on every single rendered
frame (previewTick alone fires every `DECK_PREVIEW_MS`; reconcileTick every
`DECK_RECONCILE_MS`), part of the 3e-001 hang alongside R59/R60.

`internal/tui/event_log.go`'s `eventLogBody`/`eventLogView` now read only
two new `Model` fields, `eventLogRows []store.Event` and `eventLogErr
error` -- never `m.store` directly. The one real read is `loadEventLog`, a
`tea.Cmd` (mirroring `loadArchivedSessions`' own shape) that `internal/tui/
tui.go`'s `"E"` key handler dispatches exactly once, the same keypress that
sets `eventLogOpen = true` and resets `eventLogScroll`/`eventLogRows`/
`eventLogErr`. Its reply is consumed by one new `Update` case,
`eventLogLoaded`. `updateEventLog`'s PgUp/PgDn continue to measure
`eventLogBody()`'s output for scroll bounds, but that now measures the
in-memory rows too, not a fresh store call. Grep proof `eventLogView`/
`eventLogBody` never call `m.store`:

```
$ grep -n 'm\.store\.' internal/tui/event_log.go
97:	events, err := m.store.ListEvents(context.Background(), maxEventLogRows)
```

(the one hit is inside `loadEventLog`, never inside `eventLogView`/
`eventLogBody`). `E` is not deleted (steer 3e-001 §7); `maxEventLogRows`
(200) and `maxEventLogPayloadRunes` (96) are unchanged.

**Test** (`internal/tui/event_log_render_path_test.go`,
`TestEventLogViewNeverReReadsTheStoreOnceOpen`, per steer 3e-001 §6.3): uses
an INSTRUMENTED real store -- `internal/store/store.go` gained an atomic
`eventsListCalls` counter incremented inside `Store.ListEvents` itself (not
a mock/interface substitute layered over the concrete `*Store` type every
other `tui` code path depends on directly) and a `ListEventsCallCount()`
reader. The test opens the event log (runs the real `"E"` Update, executes
the returned `loadEventLog` Cmd, feeds its reply back through `Update`),
asserts the call count is exactly 1, then runs FIVE more rounds of
`View()` + `reconcileTick` + `previewTick` + `View()` while the dialog
stays open, and asserts the count is STILL exactly 1 -- not merely small.
A single-frame test could not distinguish load-once from load-every-frame;
this one deliberately drives several ticks and renders first.

Green (`ci/run.sh go test -count=1 ./internal/tui/ ./internal/store/`):
both `ok` (tui 0.678s-0.822s across runs, store 1.780s-1.915s).

Red proof: temporarily edited `eventLogBody` in place back to its
pre-fix shape (direct `m.store.ListEvents` call inside the render path,
keeping the new test and the store instrumentation untouched -- a
signature-compatible change, so no patch/stash round-trip was needed), then
`ci/run.sh go test -count=1 ./internal/tui/ -run
TestEventLogViewNeverReReadsTheStoreOnceOpen -v`:

```
=== RUN   TestEventLogViewNeverReReadsTheStoreOnceOpen
    event_log_render_path_test.go:66: after several ticks and renders, ListEvents was called 11 times, want exactly 1 (the render path must never re-read the store)
--- FAIL: TestEventLogViewNeverReReadsTheStoreOnceOpen (0.03s)
FAIL
FAIL	github.com/n-orlov/deck/internal/tui	0.031s
```

(1 call from the initial open + 10 more, one per `View()` in the five
round-trips -- the two `reconcileTick`/`previewTick` `Update` calls
themselves never call `View()`, so the count is `1 + 5*2 = 11`, exactly
matching how many times `eventLogBody` actually ran.) `eventLogBody` was
then restored to its post-fix shape and the suite re-run green.

`features/event_log.feature` (unmodified) still passes end-to-end through
the real bubbletea runtime, which DOES execute `Update`'s returned Cmd
(unlike the unit test above, which does so explicitly): `ci/run.sh sh -c
'DECK_GODOG_TAGS=@requirement-32 go test -count=1 ./features/'` -- both
`@requirement-32` scenarios pass (2 scenarios, 29 steps, ~2.6s).

**Pre-existing, unrelated failure noticed during this task's full
`./features/` run** (not caused by this task's diff -- confirmed by
`git stash`-ing this task's changes and re-running the same test against
the unmodified task-330 tip, `faba630`, which fails identically):
`TestBlackBoxAssertionsObserveRealSession`
(`features/assertions_test.go:1068`) hardcodes `databaseSchemaVersion(...,
4)`, which task 330's `SchemaVersion` 4->5 bump left stale (`schema version
= 5, want 4`). Flagged for task 324 to root-cause/fix, not fixed here (out
of this task's scope, and fixing a hardcoded schema-version assertion is
unrelated to the render-path change this task makes).

## Per-requirement evidence table

_To be completed by task 326: R52-R58, R59-R62, plus the whole-suite and
stability evidence and every revert-red proof cited by sha and log path._

