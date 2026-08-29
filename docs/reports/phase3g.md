# Phase 3g report

Phase 3g closes the operator's field backlog against
`prds/phase3g-residuals-and-suite-determinism.md` — **seventeen requirements,
R76–R92**, every one of them either a defect the operator hit using the Phase 3f
build or a defect Phase 3f found and deliberately left standing. `SPEC.md` was
amended first (`2eed8de`) so every requirement below has a spec authority; `SPEC.md`
itself is protected and untouched by this phase.

This report gives, per requirement, the implementing sha(s), the test/scenario that
carries the assertion, the evidence path, and — for the eight requirements the PRD
names as naive-test traps (**R76, R77, R79, R86, R87, R88, R89, R91**) — a quote of
the red before the fix and the green after, from the evidence directory captured for
it. Written incrementally by task 038 from the tree at task 037's HEAD (`cc36cfa`);
sections are not re-derived once written.

**Two requirements' evidence gaps are disclosed rather than hidden.** R86's fixing
tasks (025, 026) did not produce a dedicated evidence directory at their own
implementation time, and task 035 committed no test that fails without R91's fix, so
both of those red/green quotes were produced retroactively by task 038, by the
identical revert-and-reproduce method, and are labelled as such — see the
[R86 section](#r86---navigate-dialog-fields-tab-is-completion-only-tasks-025-026)
with `docs/reports/phase3g-038-r86-dialog-arrow-nav/README.md`, and the
[R91 section](#r91--previewfits-spent-fit-and-the-sigwinch-re-baseline-it-licenses-tasks-034-035)
with `docs/reports/phase3g-038-r91-previewfit-latch/README.md`.

- Sections: [R76](#r76--the-reconcile-repairs-a-terminal-row-with-a-live-pane-tasks-001-002) ·
  [R77](#r77--a-deleted-sessions-name-is-reusable-tasks-003006) ·
  [R78](#r78--an-archived-row-keeps-its-name-and-dd-frees-it-tasks-007009) ·
  [R79](#r79--a-tombstone-that-outlives-its-process-is-reaped-at-the-next-store-open-tasks-010011) ·
  [R80](#r80--the-footer-lists-only-what-the-current-selection-will-accept-tasks-012013) ·
  [R81](#r81--the-footers-fixed-set-is-curated-tasks-014015) ·
  [R82](#r82--dialogs-are-themed-tasks-016021) ·
  [R83](#r83--the-three-dialogs-that-draw-past-the-frame-at-8024-tasks-016-018-022) ·
  [R84](#r84--the-contrast-floor-covers-the-pairs-a-dialog-actually-uses-task-106) ·
  [R85](#r85--the-create-modal-opens-on-the-last-used-agent-task-024) ·
  [R86](#r86---navigate-dialog-fields-tab-is-completion-only-tasks-025-026) ·
  [R87](#r87--esc-clears-a-filter-held-in-force-tasks-027-028) ·
  [R88](#r88--inject-refuses-a-retained-dead-pane-task-029) ·
  [R89](#r89--the-interactive-pipe-leaks-nothing-on-abnormal-exit-tasks-030-031) ·
  [R90](#r90--a-hook-dropped-as-superseded-is-labelled-dropped-tasks-032-033) ·
  [R91](#r91--previewfits-spent-fit-and-the-sigwinch-re-baseline-it-licenses-tasks-034-035) ·
  [R92](#r92--r75s-release-failure-fallback-is-exercised-tasks-036-037) ·
  [known open regression](#known-open-regression-discovered-by-task-002s-own-evidence-not-fixed-here) ·
  [review finding 1](#review-finding-1--r76s-error-branch-and-its-scenario-fallout-tasks-701-702-703-802805) ·
  [review finding 2](#review-finding-2--r80s-one-definition-per-action-tasks-806808) ·
  [table](#per-requirement-table) ·
  [close-out](#close-out-task-113) ·
  [close-out (approach 06)](#close-out-approach-06)

## Tool versions

Every command below ran in the sibling toolchain container (`ci/run.sh`, image
`deck-ci:local`, `--user 1000:1000`, cache on the named volume `deck-go-cache`). No Go
and no tmux exist outside it; see `docs/reports/toolchain-versions.md` for the pinned
`go version`/`tmux -V` output, unchanged since Phase 3f.

## R76 — the reconcile repairs a terminal row with a live pane (tasks 001, 002)

`SPEC.md` §7. **Implementing shas: `89fcffc`, `15e33c6`** (task 001, the repair
itself and the terminal-verdict spend it must also clear) and **`904419c`** (task
002, the field-route proof). This is the phase's reason to exist and lands first, as
the PRD requires.

Defect: a row written to `stopped`/`error` by a hook while its pane is still alive was
never corrected — resume declines because nothing needs launching, kill declines
because the row already reads terminal — and the only escape was `dd` + recreate.

Fix: `internal/service/reconcile.go` observes a live, non-dead pane under a terminal
row, corrects the row from what §7's liveness rules can observe, records the
correction as an event, and **touches the pane not at all**. Dead-pane collection is
decided first, so a genuine corpse is still collected. `89fcffc` alone left a
user-killed row unrepaired (`killed_by_user` precedence) and a repaired crashed row
re-frozen by its own spent `pane_exit_status`; `15e33c6` clears both spent verdicts as
part of the same repair.

Tests: `internal/service/reconcile_terminal_repair_fake_tmux_test.go` (pane-survival
proof via a fake-tmux double asserting no `kill-session`/`kill-pane`/`respawn-*`/
`send-keys` argv appears, and idempotence over a second `Reconcile`);
`features/terminal_repair_field_route.feature` (the field route: a hook writes
`stopped` while the pane survives, no keypress).

**Red (before `89fcffc`/`15e33c6`), from `docs/reports/phase3g-001-r76-terminal-row-live-pane/red-1-no-repair.log`:**
```
red-1-no-repair.log: 4 tests RED — the live-pane branch `continue`s, so the row
stays `stopped`/`error` forever
```
and, after the first attempt at the fix (`89fcffc` alone, before `15e33c6`), from
`red-2-spent-verdicts-refreeze.log`:
```
red-2-spent-verdicts-refreeze.log: 2 tests RED — a user-killed row is not repaired
at all (killed_by_user precedence sets apply=false), and a repaired crashed row
keeps a spent crash verdict, which re-freezes it (terminal = status=="stopped" ||
PaneExitStatus != nil)
```

**Green**, `docs/reports/phase3g-001-r76-terminal-row-live-pane/green-after-fix.log`:
all 4 repair tests plus `./internal/service/` and `./internal/store/` whole packages.

**Field-route red/green**, `docs/reports/phase3g-002-r76-field-route/red-raw-full.log`
and the same directory's README — reverting `internal/service/reconcile.go` and
`internal/store/store.go` to `1cfbd5a` (the PRD's own starting commit) and re-running
`ci/run.sh env DECK_GODOG_PATHS=terminal_repair_field_route.feature go test ./features/ -run TestFeatures -count=1`:
```
step error: session "wedged" terminal fields never reached status "starting",
source "tmux", killed_by_user=0 within one reconcile interval; last observed
status "stopped", source "hook", killed_by_user=0
FAIL	github.com/n-orlov/deck/features	5.397s
```
Green with the fix restored: `ok github.com/n-orlov/deck/features 1.465s`, re-run 3
additional times with no flakiness observed.

Evidence: [`phase3g-001-r76-terminal-row-live-pane/`](phase3g-001-r76-terminal-row-live-pane/),
[`phase3g-002-r76-field-route/`](phase3g-002-r76-field-route/).

A regression this evidence itself discovered — not fixed by task 038 — is recorded
[below](#known-open-regression-discovered-by-task-002s-own-evidence-not-fixed-here).

**Task 103, no fix to R76 itself, a verdict on its interaction with a scenario.**
`status_claude_hooks.feature`'s "Every declared Claude hook maps to honest status
through both identity routes" scenario posed a hook-written `SessionEnd` `stopped` row
while the fake claude's own pane stayed alive underneath it — precisely the state R76's
`repairTerminalRowWithLivePane` exists to correct, so the repair fired before the
scenario's own assertion read the row. **Implementing sha: `51b7f17`** re-routes the
scenario to reach the same assertion through a state R76 does not repair (the fake
claude's pane is driven to a confirmed, clean exit first, then `deck _hook SessionEnd`
is delivered as a one-shot released invocation, never a `send-keys` into a still-live
pane) — `internal/service/reconcile.go` is untouched by the commit, and every
status/reason/message/acknowledged/notify_epoch assertion the scenario had at `b6cbbc7`
is unedited. Red at `b6cbbc7`, `docs/reports/phase3g-103-hook-sessionend-repair/red-b6cbbc7-step-error.txt`:
```
step error: session "hook truth" = status "starting" source "tmux" reason "tmux pane
is alive; terminal row corrected" message "permission granted; work is complete"
acknowledged=0 epoch=3; want "stopped" hook "logout" "permission granted; work is
complete" 0 3
```
Green, three consecutive runs of `status_claude_hooks.feature`
(`green-run1.log`/`green-run2.log`/`green-run3.log`, all `ok
github.com/n-orlov/deck/features <5s`). **Verdict recorded for task 110's fold-in**: a
hook-declared terminal status arriving while its own pane is still alive is R76 working
as specified, the same class of case F20 already documents for a raw state-database
write — not a spec contradiction, and not grounds to loosen or bypass the repair.
Evidence: [`phase3g-103-hook-sessionend-repair/`](phase3g-103-hook-sessionend-repair/).

## R77 — a deleted session's name is reusable (tasks 003–006)

`SPEC.md` §9.2. **Implementing shas: `b80a4bd` + `70162d4`** (task 003, the tx-scoped
reap plus the filesystem cleanup after commit), **`be3df32`** (task 004, `RenameSession`
gets the same treatment), **`31510ab`** (task 005, `u`'s honest report), **`c791a6a`**
(task 006, the create-dialog warning).

Defect: `name`/`slug` are table-wide unique indexes, so a tombstoned row still blocks
a create or rename onto its name; a naive fix (pre-check plus plain `INSERT`) still
violates the constraint, and a service-layer "reap then create" destroys a session's
events if the subsequent create fails validation.

Fix: a tx-scoped `reapSessionTx`/`reapTombstonedHolderTx` shared by `CreateSession`
and `RenameSession`, so the reap and the create/rename commit atomically or not at
all; `Service.removeReapedSessionFiles` (captures dir, §9.4 history file) runs only
**after** the transaction commits, shared by `CreateShell`/`CreateAgent` and the
explicit `Service.Reap`; `RestoreSession` (`u`) reports "reaped when its name was
reused" instead of a bare not-found or a raw SQLite constraint error; the create
dialog's Name field states the reuse and what it discards before it happens.

Tests: `internal/store/tombstone_test.go` (reap-on-create/rename, `RestoreSession`'s
honest report); `internal/service/create_name_reuse_test.go`
(`TestCreateShellReusingATombstonedNameCleansUpThatSessionsFiles`,
`TestCreateAgentReusingATombstonedNameCleansUpThatSessionsFiles`,
`TestRefusedCreateKeepsTheStillRestorableSessionsFiles`);
`features/create_reuse_warning_test.go` (task 006's dialog warning).

**Red**, task 003's residual (the filesystem half), from
`docs/reports/phase3g-003-r77-name-reuse-reap/red-service-cleanup-reverted.log`:
```
--- FAIL: TestCreateShellReusingATombstonedNameCleansUpThatSessionsFiles (0.05s)
    create_name_reuse_test.go:66: captures dir of the reaped holder after reuse:
    stat = <nil>, want IsNotExist
--- FAIL: TestCreateAgentReusingATombstonedNameCleansUpThatSessionsFiles (0.06s)
    create_name_reuse_test.go:104: captures dir after reuse: stat = <nil>,
    want IsNotExist
FAIL	github.com/n-orlov/deck/internal/service	0.158s
```

**Green**, `green-service-cleanup-present.log`: `ok github.com/n-orlov/deck/internal/service 0.159s`;
whole-package run in `green-store-service-packages.log`:
```
ok  	github.com/n-orlov/deck/internal/store	2.462s
ok  	github.com/n-orlov/deck/internal/service	4.815s
```
`TestRefusedCreateKeepsTheStillRestorableSessionsFiles` passes in **both** trees
deliberately — it guards the commit ordering, not the cleanup itself.

Evidence: [`phase3g-003-r77-name-reuse-reap/`](phase3g-003-r77-name-reuse-reap/). Tasks
004–006 landed on top of that mechanism without their own dedicated red/green
directory; their tests (`internal/store/tombstone_test.go`'s rename cases,
`features/create_reuse_warning_test.go`) are named above and are green at `cc36cfa`
(task 040's whole-suite run covers them).

## R78 — an archived row keeps its name, and `dd` frees it (tasks 007–009)

`SPEC.md` §9.2. Not one of the eight red/green-quoted requirements. **Implementing
shas: `47166c8`** (task 007, refusal names the archived holder and both routes out —
`U` and `dd`), **`e699c31`** (task 008, asserts `dd` reaches an archived row end to
end and fixes `filter.go`'s "mutually exclusive" comment, which the reachable
`deleted_at != 0 AND archived_at != 0` state contradicted), **`3247a7a`** (task 009,
the delete confirm names the archived fact).

Tests: `internal/store/tombstone_test.go` (refusal wording, and
`TestDDOnAnArchivedRowIsReachableAndReapsCleanly` walking tombstone →
`ListArchivedSessions` exclusion → restore-with-`archived_at`-intact → reap, all four
consequences task 008 names); `features/filter.feature` (the same round trip through
real keystrokes: `/` finds the archived row, `dd` tombstones it, `u` returns it to the
archived pool, a second `dd` plus grace-window expiry reaps it).

Green at `cc36cfa`: `ci/run.sh go test -count=1 ./internal/store/` and
`ci/run.sh env DECK_GODOG_PATHS=filter.feature go test ./features/ -run TestFeatures -count=1`
(covered by task 040's whole-suite run).

Evidence: [`phase3g-038-r78-archived-name-dd/`](phase3g-038-r78-archived-name-dd/) —
tasks 007–009 left no per-task directory, so its two green logs
(`green-store-archived.log`, `green-filter-feature.log`) were captured by task 038 at
`e02ef08` against the unmodified tree; that directory's README says so up front and
claims no implementation-time red/green pair (none is owed: R78 is not one of the
eight).

## R79 — a tombstone that outlives its process is reaped at the next store open (tasks 010–011)

`SPEC.md` §9.2. **Implementing shas: `c987953`, `e475660`, `8276450`** (task 010: the
sweep primitive, wiring it to the real call site with an injected clock, and bounding
its per-call cost independent of the backlog) and **`e45bf2e`** (task 011: the
end-to-end abandoned-`dd` proof through the real `deck` binary across two process
runs).

Defect: `store.ListDeletedSessions` — whose own doc comment describes calling
`ReapSession` on each row — had zero non-test callers, so the "on store open, at most
once an hour" sweep the schema promised did not exist; `dd` then quit (or crash, or
`SIGKILL`) inside the grace window stranded the row, its events, its files and its
name permanently.

Fix, modelled on `EnforceEventRetention`'s existing shape: `SweepTombstones` runs at
store open (best-effort — a sweep failure must not block startup) and again on the
reconcile tick, one bounded batch per call (oldest first), and honours the injected
clock rather than falling back to `time.Now()`.

Tests: `internal/store/tombstone_sweep_test.go`
(`TestSweepTombstonesHonoursFrozenClock`, `TestSweepTombstonesReapsOneBoundedBatchPerCall`);
`cmd/deck/tombstone_sweep_test.go` (`TestAbandonedDDIsReapedAtNextStoreOpen`, driving
the real binary through three runs: create + `dd` inside the grace window, quit before
the in-process tick fires, reopen the same store more than an hour later, check the row
directly against the store, then spend the freed name through the ordinary create
route).

**Red** (mutation testing at implementation time, copied verbatim into
[`phase3g-010-011-tombstone-sweep-evidence/`](phase3g-010-011-tombstone-sweep-evidence/)
by task 038 — see that directory's own README for the disclosure), from
`task010-frozen-clock-mutation-red.log` (real `time.Now()` swapped back in):
```
--- FAIL: TestSweepTombstonesHonoursFrozenClock/clock_frozen_in_the_past_leaves_a_row_the_real_clock_would_reap
    row reaped though it is inside the grace window measured against the
    injected `now` -- a real time.Now() leaked into the age comparison:
    session "sweep-frozen-past" not found
--- FAIL: TestSweepTombstonesHonoursFrozenClock/clock_frozen_in_the_future_reaps_a_row_the_real_clock_would_keep
    row survived though it is outside the grace window measured against the
    injected `now` -- a real time.Now() leaked into the age comparison
```
and `task010-bounded-batch-mutation-red.log` (the bound removed, a synchronous
multi-batch loop restored):
```
--- FAIL: TestSweepTombstonesReapsOneBoundedBatchPerCall
    expired tombstones after one pass = 0, want 250 (one call must reap exactly
    one bounded batch of 200 and return, never the whole 450-row backlog)
```
and `task011-callsite-removed-mutation-red.log` (task 010's call site stripped from
`cmd/deck/main.go`):
```
=== RUN   TestAbandonedDDIsReapedAtNextStoreOpen
    tombstone_sweep_test.go:214: session "5661bcce-..." row still exists after
    run 2's store open, want the abandoned tombstone reaped by task 010's call site
--- FAIL: TestAbandonedDDIsReapedAtNextStoreOpen (0.83s)
```

**Green**, `task011-green-store-service-cmd-deck.log` and (for the batch bound)
the second half of `task010-bounded-batch-mutation-red.log`:
```
ok  	github.com/n-orlov/deck/internal/store	2.964s
ok  	github.com/n-orlov/deck/internal/service	4.739s
ok  	github.com/n-orlov/deck/cmd/deck	6.084s
```

**Review finding 3, closed by tasks 202 and 214.** Independent review found this
fix's own bound left a genuine residual: `Store.SweepTombstones` stamped its
hourly throttle after a single bounded batch even when the backlog was bigger
than one batch, so a real backlog left over from store-open took a further
real *hour* per leftover batch instead of finishing within the same
open/startup cycle. Task 202 (`dd90a28`, `a46514e`) gave `SweepTombstones` a
second return value reporting whether rows remain, added
`DrainExpiredTombstones` to loop on that value, and wired it into
`cmd/deck`'s reconcile tick (the store-open call site stays a single bounded
pass, unchanged in shape). Task 214 (`6a01fe7`) then pinned the real
`cmd/deck` startup/tick continuation end to end, closing the one gap task
202's own criteria could not reach directly — see the per-requirement table's
R79 row and
[the tasks 201–503 table](#tasks-201-202-203-204-214-301-302-303-501-502-and-503-approaches-03-05-testscenario-and-evidence-path-per-sha)
below for the sha/test/evidence detail, including the operator ruling task
202's status rests on.

## R80 — the footer lists only what the current selection will accept (tasks 012–013)

`SPEC.md` §11.3. Not one of the eight red/green-quoted requirements. **Implementing
shas: `f5977d4`** (task 012, one eligibility predicate per action, shared by the
footer and the key handlers) and **`745a25b` + `f9611b7` + `ebbc3fd`** (task 013,
render the legend from those predicates, anchor the long-reason assertion in the
product's own `footerLine` join, and share the reason/legend line while dropping the
keys an empty list refuses).

Tests: `internal/tui/footer_legend_test.go` — presence and **absence**, per action,
across live/stopped/archived/attention-pending/marked/empty-list rows, plus the
80-column reason+legend width behaviour.

Green at `cc36cfa`: `ci/run.sh go test -count=1 ./internal/tui/` (covered by task 040's
whole-suite run).

Evidence: [`phase3g-038-r80-r81-footer-eligibility/`](phase3g-038-r80-r81-footer-eligibility/)
— `green-footer-eligibility.log`, the presence/absence and width cases run by name;
captured by task 038 at `e02ef08` against the unmodified tree (tasks 012–013 left no
directory of their own), disclosed as green-only confirmation in that README. No red is
owed: R80 is not one of the eight.

## R81 — the footer's fixed set is curated (tasks 014–015)

`SPEC.md` §11.3. Not one of the eight. **Implementing shas: `7dbe5c5`** (task 014,
`P`/`p` removed from the footer, `dd` and the mutually-exclusive `A`/`U` slot and `,`
added, golden frame regenerated) and **`3498b3e`** (task 015, the footer↔bindings
parity test that re-parses `tui.go`'s own `footerLegend` and cross-checks it against
`SPEC.md` §11.3's prose directly, never a copied literal).

Tests: `internal/tui/footer_legend_test.go` (curated-set presence/absence);
`internal/tui/footer_bindings_parity_test.go`
(`TestFooterEntriesNameOnlyBoundKeysViaSourceParse`,
`TestFooterEntryEligibilityMatchesRealPredicate`,
`TestFooterFixedSetMatchesSpecAndExcludesRareKeys`).

Demonstrated regressions (both fail the new parity test; HEAD unmodified — reverted
after capture), from `docs/reports/phase3g-015-footer-bindings-parity/`:
- `red-comma-removed.log` — `,` deleted from `footerLegend`: "SPEC.md §11.3's footer
  fixed set requires ',' but tui.go's footerLegend does not have it".
- `red-P-added-back.log` — `P` appended back to `footerLegend`: fails both the
  eligibility-predicate check ("carries no eligibility predicate and is not one of the
  fixed global keys allowed to have none") and the SPEC-exclusion check.

Green at `cc36cfa`: `ci/run.sh go test -count=1 ./internal/tui/`. The golden frame at
`features/testdata/golden/side_by_side_80x24.golden` was regenerated
(`docs/reports/phase3g-014-footer-fixed-set/golden-regen-and-pass.log`) and its bytes
are **byte-identical to the pre-task-014 file** — explained in that directory's own
README: the fixture's one row's footer already elides before reaching where `dd`/`A`-
or-`U`/`,` sit in the legend order, confirmed directly by the same eligibility code
path's own unit tests at both 80 and 200 columns.

Evidence: [`phase3g-014-footer-fixed-set/`](phase3g-014-footer-fixed-set/),
[`phase3g-015-footer-bindings-parity/`](phase3g-015-footer-bindings-parity/).

## R82 — dialogs are themed (tasks 016–021)

`SPEC.md` §11.4/§11.6. Not one of the eight red/green-quoted requirements (theming is
a rendering-only change; there is no field behaviour to revert-and-reproduce against —
the render tests below are the assertions themselves, of the "assert render-time
`colorToken` calls, plain text unchanged" shape).

**Implementing shas**, one dialog (or pair) per task, all following the same pattern
(`colorWhole`/`styled<X>Body` over `wrapDialogLines`, title/hint/text/dimmed/key/error
per `SPEC.md:1355`, `createFieldRows`-style plain strings kept, colour applied only at
render time):

| dialog | task | shas | status |
|---|---|---|---|
| create modal | 016 | `cdb927b`, `adb7db4`; task 203 (F27) | **resolved (task 203; F27), see below** |
| env editor | 017 | `5cc1b45`, `8c3351a` | done |
| bulk delete confirm | 018 | `26a5b47`, `f33d67a`, `7f1a780` | done |
| archive / delete-purge confirms | 019 | `103d430` | done |
| profile picker, pin conversation, restart-or-inject | 020 | `62c3abe` | done |
| rename dialog, event log | 021 | `1c8cbad`; task 105 (`ea6ce4b`) | **resolved (task 105, F18), see below** |

Tests, one file per dialog, each pinning "plain body free of SGR bytes, styled body
line-for-line identical once ANSI-stripped, per-cell token checks off a real emulator
grid": `internal/tui/create_view_theme_test.go`, `internal/tui/env_editor_theme_test.go`,
`internal/tui/bulk_delete_theme_test.go`, `internal/tui/archive_delete_confirm_theme_test.go`,
`internal/tui/profile_pin_restart_theme_test.go`, `internal/tui/rename_theme_test.go`,
`internal/tui/event_log_theme_test.go`.

**Task 016 is `skipped`, not `validated`.** Its own criterion — full theming *and* the
pre-existing keyboard-only PTY assertions staying green byte-unchanged — is
unsatisfiable as written: SPEC.md:1355 requires every field's help line to render
`dimmed`, and the pre-016 `features/create_cwd_ghost_test.go` step
(`clientCWDFieldShowsNoGhostText`) scans **every cell of the whole grid** for a dimmed
token on the premise — true only while the modal was unthemed — that the ghost is the
sole dimmed thing on screen. `cdb927b` (theming, bounding) and `adb7db4` (keeping the
ghost out of the measured strings) both landed; what did **not** land is a way to keep
that one PTY step's assertion scope unchanged, since narrowing it to the cwd field's
own rows is itself an edit to a pre-existing assertion. Full reproduction:
[`phase3g-203-r82-assertion-conflict/original-assertion-red.log`](phase3g-203-r82-assertion-conflict/original-assertion-red.log) —
the same original whole-grid assertion, restored verbatim and re-run red at task 203's
tree (`step error: client "A" has a dimmed-token cell " " at row 3 column 2, want no
ghost text anywhere on screen` — row 3 is the Name field's help line, not the ghost).
(The original task-016 reproduction also lives at
`/run/ralphd/artifacts/task016-unsatisfiable/README.md`, outside this repository, not
part of the record.) The create modal **is** themed and bounded in the tree; only the
literal "byte-unchanged" half of the criterion is what could not also hold.

**Resolved by task 203, per finding [F27](phase3g-findings.md#3-defects-found-and-deliberately-not-fixed-and-why)
(`docs/reports/phase3g-findings.md`).** Independent review (finding 4) correctly held
that this was a `SPEC.md`-vs-PRD contradiction, not merely a standing-rule collision as
first filed — `SPEC.md:1357` (every field's help renders `dimmed`) and
`prds/phase3g-field-backlog.md:261` ("the keyboard-only PTY assertions... must stay
green **unchanged**") cannot both hold once the create modal is themed, and no operator
ruling had waived the PRD's "unchanged" word. The PRD's own precedence rule
(`prds/phase3g-field-backlog.md:21-22`) resolves it: `SPEC.md` wins, so
`clientCWDFieldShowsNoGhostText`'s narrowed, cwd-rows-only scope (`adb7db4`) stands as
the correct fix, not a defect. Two experiments in
[`phase3g-203-r82-assertion-conflict/`](phase3g-203-r82-assertion-conflict/) prove this
is load-bearing rather than a rubber stamp: (1)
[`original-assertion-red.log`](phase3g-203-r82-assertion-conflict/original-assertion-red.log) —
the pre-`adb7db4` whole-grid assertion, restored verbatim into the current, themed tree
and run, reproduces the same red (`step error: client "A" has a dimmed-token cell " "
at row 3 column 2, ...`) fresh at this commit, then is reverted; (2)
[`positive-control-red.log`](phase3g-203-r82-assertion-conflict/positive-control-red.log) —
a new step, `clientCWDFieldShowsGhostText`, built on the same bounded-scan helpers the
narrowed assertion uses and wired into `create_cwd_ghost.feature`, still goes red when
`internal/tui/tui.go`'s `createCWDGhostSuffix` is mutated to always return `""` (no
ghost rendered at all), then goes green again once both the mutation and the feature
edit are reverted — so the narrowed scan still catches a real ghost regression, it is
not vacuously green. `create_cwd_ghost.feature` and `create_session.feature` both pass
targeted after the revert.

**Task 021 ran `failed`, not `validated`, after 3 validation attempts at the time** —
this is the historical approach-01 state described below, now resolved (see next
paragraph). `1c8cbad`
themes both the rename dialog and the event log, and `ci/run.sh go test -count=1
./internal/tui/` plus `event_log.feature` are green — but validation found a residual
gap the criterion also implied: the rename dialog's always-focused "New name" field
renders through `detailField`, which applies only `theme.Hint`/`theme.Text`; nothing in
`internal/tui/rename.go` composes a `theme.Selection` background, contrary to §11.4's
focused-row treatment (the same treatment `renderCreateRowSegments` already gives the
create modal's own focused field). The recorded follow-up (not yet its own task): apply
`theme.Selection` to the rename dialog's focused field, with a rendered-grid assertion
that a New-name cell carries that background, plain text still byte-identical.

**Resolved by task 105 (finding [F18](phase3g-findings.md#3-defects-found-and-deliberately-not-fixed-and-why)), implementing sha `ea6ce4b`.** A new `renderRenameFieldRow`
composes the "New name" label (`theme.Hint`) and value (`theme.Text`) via
`settingsRenderRowOpen` (which opens each segment's foreground but never closes it) and
wraps the whole result in one `bgColorToken(theme.Selection, ...)` — the same shape
`renderCreateRowSegments` already uses for the create modal's own focused row;
`styledRenameBody` calls it instead of `m.detailField("New name:  ", m.renameValue)`,
and `detailField` itself (still used by `detailView`/`pinView`/`profileSwitchView`) is
untouched. New assertion: `TestRenameViewFocusedFieldGetsSelectionBackground`
(`internal/tui/rename_theme_test.go`), reading the rendered dialog off a real
`vt.Emulator` grid. Red (with the fix reverted to the old `detailField` call),
`docs/reports/phase3g-105-rename-selection/red-before-fix.log`; green,
`green-internal-tui.log` and `green-dialogs-feature.log`
(`ci/run.sh env DECK_GODOG_PATHS=dialogs.feature go test ./features/ -run TestFeatures -count=1`).
The dialog's visible text is byte-identical after ANSI stripping (asserted in the same
test), and the pre-existing rename assertions are only added to, never loosened. Evidence:
[`phase3g-105-rename-selection/`](phase3g-105-rename-selection/).

Green (both dialogs' own theme tests plus the whole `internal/tui` package, at
`cc36cfa`): `ci/run.sh go test -count=1 ./internal/tui/`; event log:
`ci/run.sh env DECK_GODOG_PATHS=event_log.feature go test ./features/ -run TestFeatures -count=1`.

**`NO_COLOR`/`DECK_ASCII` degradation and width accounting under the new tokens, which
were netted by a still-`pending` task 022 as of `cc36cfa`, are now netted by task 107,
implementing sha `02e64a5`.** A new `internal/tui/dialog_degradation_net_test.go`,
`TestThemedDialogsDegradeCleanlyUnderNoColorAndASCII`, iterates all ten dialogs themed
this phase (create modal, env editor, bulk delete confirm, delete/purge confirm,
archive confirm, profile picker, pin conversation, restart-or-inject, rename, event
log), reusing each dialog's own existing model-building test helper so this file can
never silently diverge from what those dialogs' own theme tests already assert is a
themed render. For each dialog (several with a couple of mutated states) it builds the
themed (`Color: true`) and the degraded (`Color: false, ASCII: true` — NO_COLOR and
DECK_ASCII set **together**, the one combination `config.Load` can actually produce,
since the two env vars gate entirely independent `Settings` fields) renders and asserts:
the degraded body carries no SGR byte at all; the degraded body is byte-identical to the
themed body once ANSI is stripped (no label or value lost to either degradation); and
`wrapDialogLines` produces the same physical line count, line for line, for the themed
body and its own ANSI-stripped twin (the width/height-accounting bullet). A further
assertion, `TestRenameFieldRowTruncationReemitsItsOwnReset`, pins that `truncateToWidth`
re-emits its own reset when it truncates a coloured run, satisfying §11.3's truncated-run
bullet. Green: `internal-tui-go-test.log` (`ci/run.sh go test -count=1 ./internal/tui/`)
and `features-go-test.log`
(`ci/run.sh env DECK_GODOG_PATHS=dialogs.feature,environment.feature,create_session.feature go test ./features/ -run TestFeatures -count=1`),
both exit 0. Evidence:
[`phase3g-107-dialog-degradation-net/`](phase3g-107-dialog-degradation-net/). This also
satisfies [R83](#r83--the-three-dialogs-that-draw-past-the-frame-at-8024-tasks-016-018-022)'s
same net, which the R83 section below records.

## R83 — the three dialogs that draw past the frame at 80×24 (tasks 016, 018, 022)

Finding F6, `docs/reports/phase3f-findings.md`. Not one of the eight. The PRD asks
this be done **together with** R82, dialog by dialog, and it was: the env editor
(`5cc1b45`, task 017 — see the R82 table; env editor's own bounding sha is listed
there, this row corrects task numbering to the PRD's own R83 list of three), the
create modal (`cdb927b`, task 016) and the bulk delete confirm (`26a5b47`+`f33d67a`+
`7f1a780`, task 018) all moved onto `framedDialogScrollable` in the same commits that
themed them, each with its own submit-line-reachability fix where the first pass left
it unreachable (`8c3351a` for the env editor mid-edit, `7f1a780` for the bulk confirm
at any mark count).

Tests: the same per-dialog `*_theme_test.go` files listed under R82 assert the ≤24-line
bound at 80×24 and PgUp/PgDn reachability directly.

Evidence: [`phase3g-038-r83-dialog-frame-bound/`](phase3g-038-r83-dialog-frame-bound/) —
`green-80x24-bounds.log`, the frame-budget and submit-line-reachability tests of all
three dialogs run by name; captured by task 038 at `e02ef08` against the unmodified
tree, green-only, as its README states. No red is owed (R83 is not one of the eight).

**Task 022's net is superseded by task 107 (see the [R82](#r82--dialogs-are-themed-tasks-016021)
section above for the full description).** `02e64a5`'s
`TestThemedDialogsDegradeCleanlyUnderNoColorAndASCII` and
`TestRenameFieldRowTruncationReemitsItsOwnReset`
(`internal/tui/dialog_degradation_net_test.go`) are exactly the cross-dialog regression
net the PRD's "Width and height accounting must not shift" and "`NO_COLOR` and
`DECK_ASCII` degradation is the one thing this can regress" bullets ask for, covering
all ten dialogs themed this phase, not only the three named here. Evidence:
[`phase3g-107-dialog-degradation-net/`](phase3g-107-dialog-degradation-net/).

**Task 101, a test-wait fix, not a product change, on this same clamp.**
`kill_delete_undo.feature`'s "the dd confirm dialog width is 80% of the viewport
clamped to [26,80], at both clamp ends" scenario exercises exactly this requirement's
clamp at a 30x60 terminal, but `features/agent_steps_test.go`'s `ensureCreateModalAgent`
waited on a literal (`"Agent: " + want + " (left/right cycles"`) that the theming's
word-wrap can split across two grid rows at that width, so the wait timed out forever
once the clamp was narrow enough. **Implementing sha: `2549406`** drops the
`"(left/right cycles"` clause from the awaited literal while still pinning the Agent
row's own value. Red at `b6cbbc7`,
`docs/reports/phase3g-101-agent-wait-wrap/red-b6cbbc7.log`:
```
after scenario hook failed: cycle the create modal's Agent field to "shell": timed out
waiting for frame "Agent: shell (left/right cycles": context deadline exceeded
```
Green, three consecutive runs of `kill_delete_undo.feature`
(`green-1.log`/`green-2.log`/`green-3.log`). `git show --stat` for `2549406` touches no
path under `internal/` or `cmd/`. Evidence:
[`phase3g-101-agent-wait-wrap/`](phase3g-101-agent-wait-wrap/).

## R84 — the contrast floor covers the pairs a dialog actually uses (task 106)

`SPEC.md` §11.6. **Task 106, met. Implementing shas: `57a6882`, `0219e42`.**
`internal/theme/contrast_test.go` gains a third
table-driven test, `TestThemedDialogTokensClearContrastFloor`, adding exactly the pairs
R84 names: `hint/surface`, `key/surface`, `error/surface`, and every one of a dialog
focused field's text tokens (`text`, `dimmed`, `hint`, `key`, `error`) over both
`theme.Selection` and `theme.SelectionIdle` — both the theme's authored hex palette and
its 16-colour quantisation, for all five built-ins, exactly as the two pre-existing
contrast tests already do for their own pair sets.

`internal/theme/builtin/*.toml` is unmodified (`git show --stat` on this task's commit
touches only `internal/theme/contrast_test.go`) — per the PRD's own instruction, this
requirement pins what is already true and is not a licence to recolour a theme.
`matrix`, the PRD's reference theme, is measured to clear every new pair (thinnest
`error/selection` at 3.16:1 hex, matching the PRD's own citation). The floor is
hard-enforced for **every** built-in; the ten cells that were already sub-floor when the
coverage landed (`cobalt` ×1, `empire` ×7, `parchment` ×2, always `dimmed`/`hint`/`key`/
`error` against `Selection`/`SelectionIdle`) are exempted individually, at their measured
ratios, by `dialogPairAllowlist`, which itself fails on drift over 0.01 either way, on a
listed cell that has reached the floor, and on a key matching no cell. So today's palette
keeps `ci/run.sh go test -count=1 ./internal/theme/` at exit 0 while any *new* sub-floor
pair, in any theme, breaks the build. See
[§3's F23](phase3g-findings.md#3-defects-found-and-deliberately-not-fixed-and-why)
for the measured ratios and disposition.

Evidence: [`phase3g-106-contrast-floor/`](phase3g-106-contrast-floor/README.md) —
`dialog-contrast-v.log` (every pair's ratio, all five built-ins, the `FINDING`/`SUMMARY`
lines), `theme-suite-green.log` (`ci/run.sh go test -count=1 ./internal/theme/`, exit 0)
and the two reverted-mutation logs proving the enforcement bites
(`negative-unlisted-pair-fails.log`, `negative-recorded-ratio-drift-fails.log`).

## R85 — the create modal opens on the last used agent (task 024)

`SPEC.md` §11.4. Not one of the eight. **Implementing sha: `9991689`.**

`defaultCreateAgent` always returned `shell` whenever the registry had it (which it
always does), costing every `claude` session two extra keystrokes. Fix: the
last-successfully-created agent is persisted in `ui_state` (schema v2, no migration),
written only on a create that **succeeds** (never on a cycle, so an abandoned dialog
changes no default), validated against `m.registry().Kinds()` on read with a fallback
to `defaultCreateAgent`, its profile re-derived through the same function the cycle
case already uses, and labelled `(last used)` per the `createCWDHelp` precedent.

Tests: `internal/store/store_test.go` (persistence, degrade-on-missing/unparseable);
`internal/tui/create_last_used_agent_test.go` (pre-selection, label, profile
re-derivation, no store read in the render path).

Green at `cc36cfa`: `ci/run.sh go test -count=1 ./internal/store/ ./internal/tui/`
(covered by task 040's whole-suite run).

Evidence: [`phase3g-038-r85-last-used-agent/`](phase3g-038-r85-last-used-agent/) —
`green-tui-last-used-agent.log` (all six create-modal cases) and
`green-store-last-create-agent.log` (`ui_state` persistence and defaults); captured by
task 038 at `e02ef08` against the unmodified tree, green-only, as its README states.
Task 024 left no directory of its own and no red is owed: R85 is not one of the eight.

**Task 102, a test disambiguation, not a product fix.** `features/settings.feature`'s
`@requirement-17-clear-recent-cwds-history` scenario asserted the bare literal
`"(last used)"` was gone after clearing recent cwds — but R85's own `createAgentHelp`
(`internal/tui/tui.go:6486-6516`) prefixes the Agent row's help with the identical
`"(last used) "`, untouched by clearing recent cwds, so the assertion could never
legitimately pass once R85 landed. **Implementing sha: `89682e5`** narrows both of the
scenario's `(last used)` assertions to the working-directory row's own help text,
`"(last used) the session's cwd"`, leaving the Agent row's identical prefix alone. Red
at `b6cbbc7`, `docs/reports/phase3g-102-clear-recents-label/red-b6cbbc7-agent-label-collision.log`:
```
Then deck client "A" screen does not contain "(last used)" # settings.feature:402
  Error: after scenario hook failed: deck client "A" screen unexpectedly contains
  "(last used)":
...
|     (last used) which coding agent adapter launches this session             |
```
— the matched text is the Agent row's own help, not the cleared cwd prefill. Green,
three consecutive runs (`green-run-1.log`/`-2`/`-3`, `settings.feature`). The same
directory also shows the narrowed assertion is still load-bearing: with the recents
clear locally disabled (an uncommitted one-line patch, quoted in
`red-disabled-clear-loadbearing.log`), the scenario is still red — no assertion was
deleted, only the matched string was narrowed to the row it is actually about. Evidence:
[`phase3g-102-clear-recents-label/`](phase3g-102-clear-recents-label/).

## R86 — `↑`/`↓` navigate dialog fields; `tab` is completion only (tasks 025, 026)

`SPEC.md` §11.4/§11.7. **Implementing shas: `a337671`** (task 025, `applyDialogContract`'s
`tab`/`shift+tab` become `↑`/`↓`, recents move to `Ctrl+P`/`Ctrl+N`), **`17b7bb9`**
(task 025's own validation follow-up, the `Ctrl+N` direction directly exercised) and
**`8cff03b`** (task 026, every dialog's on-screen text and ~30 feature-level call sites
converged onto the new bindings).

**Evidence gap disclosed:** neither `a337671`/`17b7bb9` nor `8cff03b` produced a
dedicated `docs/reports/phase3g-02[56]-*/` evidence directory at implementation time.
The red/green quote below was produced retroactively by task 038 — the fixing hunk in
`internal/tui/dialog_contract.go` was reverted in the live tree, run, restored (`git
diff` confirmed empty), and re-run — captured into
[`phase3g-038-r86-dialog-arrow-nav/`](phase3g-038-r86-dialog-arrow-nav/), whose own
README states this deviation from "captured at implementation time" up front.

Defect (pre-`a337671`): `applyDialogContract`'s `case "tab"`/`case "shift+tab"` moved
the focused field, and `updateCreate` intercepted `tab` on field 1 to either complete a
path or fall through to the contract depending on what happened to exist on disk — so
the same keystroke did different things depending on the filesystem.

**Red** (the two `case` labels reverted from `"down"`/`"up"` back to `"tab"`/
`"shift+tab"` — the entire product diff `a337671` made to `applyDialogContract`),
`phase3g-038-r86-dialog-arrow-nav/red-before-fix.log`:
```
--- FAIL: TestApplyDialogContractCoreKeys/up_and_down_move_the_focused_field,_wrapping
    dialog_contract_test.go:55: down from last field: handled=false index=2, want wrap to 0
--- FAIL: TestApplyDialogContractCoreKeys/tab_and_shift+tab_are_not_contract_keys_(§11.4:_reserved_for_completion)
    dialog_contract_test.go:66: tab: handled=true index=1, want unhandled and unchanged
--- FAIL: TestCreateModalUpDownMoveFieldsTabDoesNot
    dialog_contract_test.go:137: "down" moved createField to 2, want 3
FAIL	github.com/n-orlov/deck/internal/tui	0.011s
```

**Green** (revert undone), `phase3g-038-r86-dialog-arrow-nav/green-after-fix.log`: all
of `TestApplyDialogContractCoreKeys`, `TestCreateModalUpDownMoveFieldsTabDoesNot`,
`TestCreateModalTabOnPathFieldWithNoMatchDoesNotMoveFocus`,
`TestCreateModalRecentCWDCyclesOnCtrlPCtrlN`,
`TestCreateModalCandidateListOwnsUpDownWhileOpen` PASS,
`ok github.com/n-orlov/deck/internal/tui 0.037s`.

The overlay non-regression (`?`/`i`/`E` still line-scroll on `↑`/`↓`) and the
on-screen-text parity net were **not** reverted — they are the assertions, not
something to break and re-fix — and are green in the same evidence file
(`overlays-and-parity-green.log`): `TestHelpOverlayKeymapMatchesBoundKeys`,
`TestFooterKeyLegendNamesOnlyBoundKeys`,
`TestScrollableOverlaysScrollOneLineWithArrows`,
`TestScrollableOverlaysScrollOneLineWithJK`.

**Task 104, the title-independent wait convergence (finding F19), implementing sha
`045a6e6`.** `createBody` titles the create modal "Create shell session" only while
`shell` is the pre-selected agent (task 024's "(last used)" pre-selection) and a plain
"Create session" otherwise, so every `WaitForFrame(ctx, _, "Create shell session")`
whose real purpose was just "the create modal is open" hung for the whole scenario
timeout once a non-shell agent had been created earlier in the same scenario — the same
class of wrap/label coupling task 101 fixed for `ensureCreateModalAgent`'s own Agent
wait, triggered a different way. Every remaining "the create modal is open" wait in
`features/*_test.go` converged onto `ensureCreateModalAgent(ctx, client, kind)` (when
the caller needs a specific agent selected) or a direct `WaitForFrame(ctx, _, "Agent: ")`
wait (when it does not), each converted site carrying an inline `// Title-independent
(F19): ...` comment; `clientOpensCreateModalForAgent`
(`features/agent_steps_test.go:521`) is among the converted sites. After conversion,
`grep -n "Create shell session" features/*_test.go`
(`docs/reports/phase3g-104-title-independent-waits/grep-output.txt`) contains only
comments and exactly one real wait, `features/create_blank_name_test.go:85`, which is a
deliberate title assertion (the scenario never creates a non-shell session first).
Green: `ci/run.sh env DECK_GODOG_PATHS=event_log.feature,durable_identity.feature,create_session.feature,dialogs.feature go test ./features/ -run TestFeatures -count=1`
(`godog-run.log`), exit 0. Evidence:
[`phase3g-104-title-independent-waits/`](phase3g-104-title-independent-waits/).

**Task 108 re-proves R86's on-screen text and the overlays' surviving line scroll,
reproducibly. Implementing shas: `ab34cb4`, `860c412`.** Validation rejected the first
cut on its own criterion (a) alone — a hand-assembled footer table, missing the detail-
view footer (`tui.go:5660`) and the help-overlay closing footer (`tui.go:7152`);
`860c412` rebuilds that section of
`docs/reports/phase3g-108-r86-proof/README.md` on ten quoted, reproducible commands
(A1–A10, whole output plus exit status, mirrored in `criterion-a-greps.log`): A1 greps
every footer's closing words for `↑`/`↓` field navigation and `Ctrl+P`/`Ctrl+N` recents
with `tab` advertised only as path completion, A1b `colorFooterLine(` (nine styled
twins), A1c the bulk-delete constants including the scrollable variant no literal grep
sees; every A1 hit is classified in a 13-row table and tied to `dialogFields.Count`/
`applyDialogContract` (`dialog_contract.go:82-92`) by A8/A9. Criterion (b),
`internal/tui/help_keymap_parity_test.go` and `internal/tui/footer_bindings_parity_test.go`,
pass (`go-test-parity-verbose.log`). Criterion (c), that `?`/`i`/`E` still scroll exactly
one line on `↑`/`↓` under task 025's dialog contract, is covered by the pre-existing
`internal/tui/overlay_line_scroll_test.go` — cited by name, no test added, none loosened
(`go-test-overlay-line-scroll-verbose.log`). Criterion (d),
`git log --oneline 1cfbd5a..HEAD -- internal/tui/settings.go`, is empty
(`settings-go-untouched.log`). Criterion (e), `ci/run.sh go test -count=1
./internal/tui/` (`go-test-tui.log`) and
`ci/run.sh env DECK_GODOG_PATHS=dialogs.feature,create_session.feature,event_log.feature go test ./features/ -run TestFeatures -count=1`
(`go-test-features.log`), both exit 0. Evidence:
[`phase3g-108-r86-proof/`](phase3g-108-r86-proof/).

## R87 — `esc` clears a filter held in force (tasks 027, 028)

`SPEC.md` §11.10, finding F11. **Implementing sha: `f3f3d26`** (task 027, the binding
itself); **`3d749bb`** (task 028, re-pointing three stale `SPEC.md:318` citations at
the new §11.10).

Defect: with the filter text field closed, no `esc` cleared a held query — the only
clearing assignment lived in `updateFilter`'s own `esc` case, dispatched only while
`m.filtering == true`.

Fix: the top-level `esc` (`internal/tui/tui.go`) now also clears a held filter query
when nothing nearer is open (no overlay, no dialog, no mark set) and one press never
does two of those at once.

Tests: `internal/tui/filter_test.go`
(`TestEscClearsAFilterHeldInForceWithoutReopeningTheField`, plus the pre-existing
mark-set/help-overlay/rename-dialog exclusivity tests, unaffected);
`features/filter.feature`'s new scenario (deliberately never reopening `/`, unlike the
pre-existing round trips, so it is not a vacuous regression net).

**Red** (`internal/tui/tui.go`'s change stashed, tests kept),
`docs/reports/phase3g-027-esc-clears-held-filter/red-unit-tests.log`:
```
TestEscClearsAFilterHeldInForceWithoutReopeningTheField FAILs: top-level esc left
a held query in force: "alpha-agent"
```
and `red-feature-summary.log`/`red-feature-tail.log`: the new scenario "esc clears a
filter held in force at the plain list, with the text field never reopened" fails —
the second session never reappears after the bare escape, hangs on the 5s step
timeout.

**Green** (fix restored): `ci/run.sh go test -count=1 ./internal/tui/` ok;
`ci/run.sh env DECK_GODOG_PATHS=filter.feature go test ./features/ -run TestFeatures -count=1`
ok, 7 scenarios all passed.

Evidence: [`phase3g-027-esc-clears-held-filter/`](phase3g-027-esc-clears-held-filter/).

## R88 — `inject` refuses a retained dead pane (task 029)

Finding F3. **Implementing sha: `f9c6fc4`.**

Defect: `internal/service/inject.go`'s liveness guard used `TMux.Exists`
(has-session), which a retained dead pane (tmux's `remain-on-exit failed`, the same
corpse shape as GitHub #6) passes forever — so `send-keys` was sent into a corpse and
reported as applied, only ever failing deep inside `tmux.Client.SendKeys` with a
generic message.

Fix: the guard now calls `TMux.HasLivePane` — R69's "has a live pane" notion, reused
rather than a second liveness definition invented for injection — and the refusal
names what happened and the restart-to-apply route out.

Tests: `internal/service/inject_retained_corpse_test.go`
(`TestInjectEnvRefusesARetainedDeadShellPane`); `features/environment.feature`.

**Red** (`inject.go` stashed, test kept), captured at implementation time in
`docs/reports/phase3g-029-inject-retained-dead-pane/red-before-fix.log` — it fails on the
message-content assertion, because the refusal that surfaces is tmux's own generic
wrapped message, not the crafted one naming the restart route:
```
=== RUN   TestInjectEnvRefusesARetainedDeadShellPane
    inject_retained_corpse_test.go:117: refusal = "inject environment key \"INJECT_TARGET\" into session \"dead-shell-pane\": send keys to session \"dead-shell-pane\": no live pane", want it to say a stopped/error row cannot take an injection
--- FAIL: TestInjectEnvRefusesARetainedDeadShellPane (0.06s)
FAIL
FAIL	github.com/n-orlov/deck/internal/service	0.067s
FAIL
```

**Green** (same directory, `green-after-fix.log`, the fix restored), verbatim:
```
=== RUN   TestInjectEnvRefusesARetainedDeadShellPane
--- PASS: TestInjectEnvRefusesARetainedDeadShellPane (0.06s)
PASS
ok  	github.com/n-orlov/deck/internal/service	0.067s
```

Full criterion, both green at `cc36cfa`: `ci/run.sh go test -count=1 ./internal/service/`;
`ci/run.sh env DECK_GODOG_PATHS=environment.feature go test ./features/ -run TestFeatures -count=1`.

Evidence: [`phase3g-029-inject-retained-dead-pane/`](phase3g-029-inject-retained-dead-pane/).

## R89 — the interactive pipe leaks nothing on abnormal exit (tasks 030, 031)

Finding F4. **Implementing shas: `6718823` + `f70b773`** (task 030, the reclaim-at-
next-start mechanism plus its own evidence capture) and **`8ddf869`** (task 030's own
unblocking of two stale PTY drivers) for the SIGKILL half; **`ce22192`** (task 031)
for the SIGTERM-and-panic half.

Defect: an abnormal exit left `/tmp/deck-interactive-pipe-*`, the FIFO, an **armed**
`pipe-pane` and window ownership behind. SIGKILL cannot be handled at all, so its
guarantee is "the next start reclaims it"; SIGTERM and a panic both still run Go code
first, so their guarantee is stronger — "the exit itself cleans it up".

Fix (SIGKILL, task 030): `internal/tmux/reclaim.go`'s `reclaimOne`, called from
`cmd/deck/main.go` at every start, disarms a stale dead-owner's `pipe-pane`, restores
geometry and removes the FIFO/temp dir. Fix (SIGTERM/panic, task 031):
`Model.ShutdownInteractive` (factored out of `exitInteractive`'s own teardown) is
called from `cmd/deck/interactive_shutdown.go`'s `interactiveShutdownGuard`, wrapping
the **outermost** layer of every test-wrapper `main.go` composes, so its `recover`
sees a panic from any of them; `main.go` also calls the same shutdown helper on the
model `tea.Program.Run()` returns, which is the only remaining SIGTERM route since
`QuitMsg` never lets a panic-style recover fire.

Tests: `internal/tmux/reclaim_test.go`
(`TestReclaimLeakedInteractivePipesDisarmsRestoresAndRemovesAStaleDeadOwnerClaim`);
`features/interactive_pipe_leak.feature`; `cmd/deck`'s
`TestDeckBinarySIGTERMMidInteractiveDisarmsPipeAndRestoresWindow` and
`TestDeckBinaryPanicMidInteractiveDisarmsPipeAndRestoresWindow`;
`features/no_leak_scan.feature` (regression net).

**Red (SIGKILL half)**,
`docs/reports/phase3g-030-reclaim-leaked-interactive-pipe/red-before-fix-unit.log`:
```
reclaimed = [], want exactly [...]
```
and `red-before-fix-feature.log`: `Then tmux pane pipe is not armed for session
"leaky"` fails, `#{pane_pipe} still reads 1 for "deck_leaky", want 0 (disarmed)`.

**Green**: `green-after-fix-unit.log`, `green-after-fix-feature.log` — pipe-pane
disarmed, temp dir/FIFO gone, `@deck_isize_owner` unset, geometry unchanged, observed
from a second client that never itself attached to the reclaimed window.

**Red (SIGTERM/panic half)**,
`docs/reports/phase3g-031-sigterm-panic-interactive-cleanup/red.log` (production files
reverted to task 030's own HEAD, test files kept): both
`TestDeckBinarySIGTERMMidInteractiveDisarmsPipeAndRestoresWindow` and
`TestDeckBinaryPanicMidInteractiveDisarmsPipeAndRestoresWindow` fail with
`pipe-pane is still armed ... after deck exited, want disarmed`.

**Green**, `green.log`: both pass; `full-target-suite.log`,
`ci/run.sh go test -count=1 ./internal/tmux/ ./cmd/deck/`, green;
`no_leak_scan.log`, `no_leak_scan.feature`, 2/2 scenarios green.

Evidence: [`phase3g-030-reclaim-leaked-interactive-pipe/`](phase3g-030-reclaim-leaked-interactive-pipe/),
[`phase3g-031-sigterm-panic-interactive-cleanup/`](phase3g-031-sigterm-panic-interactive-cleanup/).

## R90 — a hook dropped as superseded is labelled dropped (tasks 032, 033)

Finding F15. Not one of the eight (there is no naive-test trap named for it in the
PRD; it is a read/write pair rather than a behavioural regression). **Implementing
shas: `99fc4a3`** (task 032, the write side — `internal/hookrecv/receiver.go` marks a
superseded hook's stored event with a `supersededEventKind` and the reason) and
**`ab14d19` + `ca43907`** (task 033, the read side — `store.LastDroppedHook` and the
`E` event log / `i` detail's "Hook declined: `<kind>` — `<reason>` (`<age>`)" row).

`SPEC.md` §6's "a kind is only written if something reads that kind" is honoured by
landing both halves together: `receiver.go:222` stays the single chokepoint writing
`Source: "hook"`, and both readers (`E`, `i`) go through it.

Tests: `internal/hookrecv/launch_generation_test.go`;
`internal/tui/dropped_hook_test.go`
(`TestEventLogDisplaysTheDroppedHookKindAndReasonAfterPressingE`, driving the real `E`
key, asserting a dispatched command rather than an inline `View()` read, fed through
`Update` before the text is matched); `features/event_log.feature`.

**Read-side gap disclosed** (validation of `ab14d19` found two gaps, both about how the
claim was proved, not about behaviour): the original test never pressed `E` at all, and
`event_log.feature` was red for an unrelated reason — task 024's last-used-agent
pre-selection meant the modal's title is `Create shell session` only when that agent
*is* shell, and the scenario's third create waited on that literal. Fix,
`ca43907`: the test now drives `Update(key("E"))` for real, and
`features/agent_steps_test.go` gains `ensureCreateModalAgent`, which waits on the
title-independent `Agent: ` row and cycles to the wanted value by reading the frame.
Before/after quotes,
`docs/reports/phase3g-033-dropped-hook-label/event_log-red-before.txt` /
`event_log-green-after.txt`:
```
(red)  Error: after scenario hook failed: timed out waiting for frame "Create shell session"
       ... | Create session | | Agent: claude (left/right cycles: claude, pi, shell) | (last used) ...
(green) ok  github.com/n-orlov/deck/features 2.034s
```

Green at `cc36cfa`: `ci/run.sh go test -count=1 ./internal/tui/`;
`ci/run.sh env DECK_GODOG_PATHS=event_log.feature go test ./features/ -run TestFeatures -count=1`;
regression sweep over `walking_skeleton.feature,create_session.feature,dialogs.feature,filter.feature,event_log.feature`.

Evidence: [`phase3g-033-dropped-hook-label/`](phase3g-033-dropped-hook-label/). Its
own "still open" note records that `ensureCreateModalAgent` converged only this one
call site — the wider convergence across ~19 other hard-coded-title sites is a
separate, already-flagged item (see `notes.md`), not part of R90.

## R91 — `previewFit`'s spent fit, and the SIGWINCH re-baseline it licenses (tasks 034, 035)

Finding F12. **Implementing sha: `9d6c22a`** (task 035); **`c93f811`** (task 034, the
pre-derivation this PRD requires before any code change — no product code, no test
code, documentation only).

Defect: `previewFit`'s no-live-pane early return reported the identical
`previewFitDone{sessionID}` as a real fit, so `previewFitSessionID` latched on a
session that was never actually resized — spending its one coalesced fit attempt and
refusing every future legitimate fit for that session ID for the rest of the model's
lifetime, even after the pane later became live again.

**Per this PRD's own licence** (Phase 3f could not fix this because bounding it moves
other scenarios' SIGWINCH counts, forbidden there; this PRD permits it under two
conditions: every changed count justified *before* the run, never read off a failure,
and no scenario deleted/skipped/tagged to force agreement) — task 034 read
`internal/tui/tui.go`, `internal/tmux/tmux.go` and every exact-count assertion in
`features/*.feature` **before any code changed**, at HEAD `ca43907`, and predicted
**zero deltas** across the suite's 7 exact-SIGWINCH/resize assertions, because none of
them exercises the no-live-pane branch (two never construct a `Model` at all, three are
blocked by an earlier guard, two involve a pane that stays live the whole scenario).

Fix, `9d6c22a`: `previewFitDone` gained a `noLivePane bool` field (zero value `false`,
so every pre-existing literal keeps its old behaviour); the no-live-pane return sets it
true; the handler only latches `previewFitSessionID` when it is false.

Tests: `internal/tui/preview_fit_overlap_test.go` (pre-existing R63 tests, unaffected —
none of their literals set `noLivePane`); the same seven exact-count sites in
`features/preview.feature` and `features/interactive_sigwinch_budget.feature`.

**Red before the fix, green after** — retroactively captured by task 038, a disclosed
deviation from the "captured at implementation time" rule: task 035 committed no test
that fails without its fix, so no implementation-time red existed for the latch itself.
Task 038 reverted `internal/tui/tui.go` to `9d6c22a^`'s content and ran a throwaway
evidence harness (removed again in the same iteration, source preserved verbatim as
`docs/reports/phase3g-038-r91-previewfit-latch/zz_r91_latch_evidence_test.go.txt`) that
names no field the fix added, so it compiles either side of it; it runs the fit closure
for real against a non-existent tmux socket — the genuine no-live-pane return — and
hands the resulting `previewFitDone` to `Model.Update`.

**Red**, `docs/reports/phase3g-038-r91-previewfit-latch/red-before-fix.log`:
```
    zz_r91_latch_evidence_test.go:51: previewFitSessionID = "s1" after a no-live-pane
    return, want empty: nothing was resized, so this session must stay eligible for a
    real fit once its pane becomes live again (R91)
--- FAIL: TestR91NoLivePaneReturnDoesNotLatchTheFittedSession (0.00s)
```

**Green** (committed `tui.go`), same directory's `green-after-fix.log`:
```
--- PASS: TestR91NoLivePaneReturnDoesNotLatchTheFittedSession (0.01s)
ok  	github.com/n-orlov/deck/internal/tui	0.008s
```

That harness is **not** in the suite: R91's real residual gap is that nothing committed
fails if the `noLivePane` guard is removed again (promoting the harness into
`internal/tui/preview_fit_overlap_test.go` is a follow-up worth a task).

The count side of R91 — the part the PRD's SIGWINCH licence turns on — held too:
**every one of the 7 predicted counts matched exactly**, and the *prediction itself*,
written before the fix, is what would have gone wrong if the fix had moved a count.
`docs/reports/phase3g-035-previewfit-no-live-pane-latch/features-preview-sigwinch-green.log`:
```
15 scenarios (15 passed)
152 steps (152 passed)
```
with the per-line table (`interactive_sigwinch_budget.feature:33`→2,`:46`→2,
`preview.feature:95`→1,`:107`→1,`:130`→0,`:171`→0,`:173`→1) unchanged from task 034's
prediction, all confirmed observed-post-fix in the same directory's README.

**The fit-floor assertion (finding F1, `2b39124`) still fails before the count it
protects**, demonstrated with a throwaway, never-committed edit (`git diff
features/preview.feature` confirmed empty afterward) inserting a genuine height
violation:
```
Then deck client "solo" has never been taller than 9 rows
after scenario hook failed: deck client "solo" has been 20 rows tall at some
point, want never more than 9
```
— `preview-floor-assertion-forced-red.log`, proving the ordering task 028 designed is
still load-bearing, not merely unexercised, after this fix.

Green: `ci/run.sh go test -count=1 ./internal/tui/` (`internal-tui-tests.log`).

Evidence: [`phase3g-034-previewfit-derivation/`](phase3g-034-previewfit-derivation/),
[`phase3g-035-previewfit-no-live-pane-latch/`](phase3g-035-previewfit-no-live-pane-latch/),
[`phase3g-038-r91-previewfit-latch/`](phase3g-038-r91-previewfit-latch/) (the retroactive
red/green pair and its disclosure).

## R92 — R75's release-failure fallback is exercised (tasks 036, 037)

Finding F17. **Implementing shas: `78bc156`** (task 036, the seam) and **`cc36cfa`**
(task 037, the test).

R75's best-effort release fallback (`internal/service/resume.go`'s deferred release
failing → audit `launch_lease.release_failed` → §9.3's TTL as backstop) had no test
because driving the release `UPDATE` to fail needed a fault-injection seam the tree did
not have, and R8 forbids a test-only branch or env knob in product code.

Fix: `78bc156` widens `Resume`'s existing store dependency into an exported interface
seam, `internal/service.LaunchLeaseReleaser` (one method,
`ReleaseLaunchLease`) plus `Service.LeaseReleaser` — a plain optional-override DI
field, nil-default, not a branch conditioned on a test-only signal. `cc36cfa` then
substitutes a `failingLeaseReleaser` through that seam and asserts: the audit event
fires, the lease columns stay set (not eagerly cleared, so the row is bound by TTL
rather than wedged), the verdict the caller sees is unchanged (`ResumeStarted`), and a
later resume once a stepped test clock passes the TTL succeeds (proving the row is not
permanently wedged).

Tests: `internal/service/lease_release_failure_test.go`.

**No red/green revert-and-reproduce is claimed here** — R92 is explicitly about
*proving a pre-existing degradation is graceful*, not about changing behaviour ("no
verdict depends on the fallback — it *is* the pre-R75 behaviour", PRD's own words), so
there is no defect to show red before a fix. The seam itself is additive (a new nil-
default field nothing else in the tree sets), so reverting it would only delete the new
test's own ability to compile, not demonstrate a behavioural regression.

Green at `cc36cfa`: `ci/run.sh go test -count=1 ./internal/service/`;
`ci/run.sh env DECK_GODOG_PATHS=lease_race.feature go test ./features/ -run TestFeatures -count=1`.

Evidence: [`phase3g-038-r92-lease-release-failure/`](phase3g-038-r92-lease-release-failure/)
— `green-lease-release-failure.log`, the seam-driven test run by name; captured by task
038 at `e02ef08` against the unmodified tree, green-only (no red is owed, per the
paragraph above), as its README states.

(`cc36cfa` also converges `positionCreateModalOnProfileField` onto task 024's
`ensureCreateModalAgent` helper — needed because `lease_race.feature` creates two
claude sessions in one scenario, the same convergence gap R90's evidence flagged; only
this one further call site, not the full ~19-site sweep.)

## R93 — an in-progress drag-to-copy selection is visible on screen (tasks 205–207)

New requirement this approach, from a direct operator ruling (steering 018, 2026-08-28,
filed as github.com/n-orlov/deck issue #18): a drag-to-copy selection in the interactive
preview must be visible from press to release, the way tmux's own `MouseDragEnd1Pane`
gesture is. The gesture already worked end to end since task 216 (Phase 3d) — only the
visual feedback was missing — and `SPEC.md` had no requirement covering it at all, so
this is a genuine spec omission rather than a regression in 216's work.

**SPEC amendment, its own code-free commit, under steering 018's explicit licence to
touch the otherwise-protected `SPEC.md`: `b69b5ba`** (task 205) — amends §11.8's
"Selection and copy" paragraph: the in-progress selection is marked with the
`selection` theme token from press to release, the marking is linear (matching
`SelectedText`'s own run, never rectangular), and it clears when the release commits
the copy.

**Implementing shas:**

- **`b249992`** (task 206) — `internal/interactive/grid.go`'s
  `Session.SelectionHighlightRange` (resolves a view row's membership through
  `AbsoluteRow` itself, never a second, independently derived row window) and
  `internal/tui/interactive_select.go`'s `highlightInProgressSelection`/
  `highlightRangeSGR`, wired into the interactive preview's single grid-draw call site
  (`internal/tui/interactive.go`'s `interactiveBodyLines`, right after `RenderRows`).
  New tests in `internal/interactive/selection_test.go`:
  `TestSelectionHighlightRangeAgreesWithSelectedTextAcrossWrappedRows` (the
  anchor/current highlight and `SelectedText`'s own run agree byte-for-byte across a
  wrapped multi-row drag) and `TestSelectionHighlightRangeReportsNoHighlightOutsideSelectedRows`.
- **`c3a5a06`** (task 206) — proves that same agreement through the real render path
  (`Model.View()` through a real `vt.Emulator`, a real `tea.MouseMsg` press/motion/release
  sequence and a real tmux pane), not just the grid helper in isolation, so a no-op or
  broken renderer cannot pass. New test:
  `internal/tui/interactive_select_highlight_test.go`'s
  `TestInProgressSelectionHighlightsExactlyTheCellsTheCopyReturns`. Mutation evidence
  committed alongside: neutering the render call to a no-op
  (`mutation-renderer-noop.log`) and mutating the highlight range rectangular
  (`mutation-rectangular.log`) both go red.
- **`7e7b0be`** (task 207, the adjacent confirmation steering 018 licensed if it fit in
  one task) — `selectionCopyNote`, the success counterpart to the pre-existing
  `attachError` failure message; with the in-progress highlight cleared the instant a
  release commits the copy, a copy that worked and a copy that never happened were
  otherwise indistinguishable on screen. New test:
  `internal/tui/interactive_select_test.go`'s
  `TestFailedInteractiveSelectionCopySetsNoConfirmation`; the pre-existing
  `TestDragOverInteractivePreviewCopiesSelectedTextToTheNamedTmuxBuffer` is extended to
  assert the confirmation text.
- **`a0bf89e`** (task 207) — round 1 of validation was right to reject the test, not the
  implementation: both cases above read `Model.selectionCopyNote` directly and rendered
  nothing, so a confirmation the model holds but `mainView` never appends would still
  have passed. Both cases now render `View()` into a real terminal emulator and assert
  on rendered cell contents instead (`renderedScreenRows`/`screenRowContaining`
  helpers) — the success case needs one screen row naming both the copied size and
  deck's own tmux buffer, the failure case needs the unchanged "Cannot copy selection:
  ..." message on screen with no confirmation row anywhere in the frame.
  `mutation-unrendered-note.log` is the committed red once `mainView`'s append of the
  note is removed; `targeted-suite-tui.log` is the green on the real tree.

Linear-only, never rectangular (the standing rule this requirement singles out): pinned
by `c3a5a06`'s `mutation-rectangular.log` going red on a rectangular mutation. Passive
preview and the OSC 52 clipboard defect stay out of scope and untouched across all four
shas — `internal/tui/interactive_select.go`'s `oscClipboardWriter`/
`writeOSCClipboardBestEffort`/`SetSelectionBuffer` call sites carry no diff, and no
`internal/theme/builtin/*.toml` file or `internal/tui/panel.go` changed (the highlight is
a background-only span using the existing `text/selection` contrast-floor entry, never a
new token pairing).

Tests: `internal/interactive/selection_test.go`, `internal/tui/interactive_select_highlight_test.go`,
`internal/tui/interactive_select_test.go`.

Evidence: [`phase3g-206-r93-visible-selection/`](phase3g-206-r93-visible-selection/)
(tasks 205–206) and [`phase3g-207-copy-confirmation/`](phase3g-207-copy-confirmation/)
(task 207).

## Known open regression, discovered by task 002's own evidence, not fixed here

`features/status_recovery.feature`'s "`r` on a session whose tmux session already
exists reports already-running, never an error" scenario is **currently red** at
`cc36cfa`. It was flagged as a discovered side effect in task 002's own evidence
(`docs/reports/phase3g-002-r76-field-route/README.md`, "Note: unrelated discovered
regression") and reconfirmed by task 038 while assembling this report:

```
$ ci/run.sh env DECK_GODOG_PATHS=status_recovery.feature go test ./features/ -run TestFeatures -count=1
4 scenarios (3 passed, 1 failed)
step error: client "A" did not show "stopped - resumable" within 500ms:
timed out waiting for frame "stopped - resumable": context deadline exceeded
```

Root cause: the scenario's own store-read assertion at line 30
(`the state database session "dup pane" is "stopped" from "hook"`) is sound, but the
**frame-read** assertion at line 35 races R76's own repair (`89fcffc`/`15e33c6`) — the
scenario's fake `SessionEnd` fires while the real tmux pane is still alive (that is the
scenario's whole point: prove `r` reports "already-running" rather than an error), and
R76's reconcile now corrects that same contradiction before the scenario's frame-read
assertion can observe the pre-repair `stopped` text. This is exactly R76 doing its job
against a scenario written before R76 existed to contradict it — a genuine interaction
between two requirements, not a flaw in either one alone. Not fixed by task 038 (out of
scope for a report-writing task); recorded here and due for
`docs/reports/phase3g-findings.md` (task 039's deliverable, not yet written at
`e02ef08`) and a follow-up task to rewrite the
scenario's assertion to a store read or otherwise account for R76.

## Review finding 1 — R76's error branch and its scenario fallout (tasks 701, 702, 703, 802–805)

Review finding 1 (against `d266346`) is the ruling this approach's R76 work is built on
(restated in full as finding F36 in `docs/reports/phase3g-findings.md`, task 801,
`794313f`): `SPEC.md` names `error` explicitly as an invariant-violation status exactly
like `stopped`, but `internal/service/reconcile.go`'s pre-existing repair
(`89fcffc`/`15e33c6`, task 001) sat inside the `terminal` expression, which a bare
hook/probe `error` row (no `PaneExitStatus`) never satisfies — that row was never
repaired. This section records finding 1's closure: the repair fix itself, the fallout
it produces across `./features/`, which scenarios were re-pointed onto the post-repair
state, and which one scenario remains open.

### The fix (task 701): sha `89edd3c`

`internal/service/reconcile.go`'s repair decision becomes its own status test
(`session.Status == "stopped" || session.Status == "error"`), evaluated independently
of `terminal`. Test: `internal/service/reconcile_bare_error_repair_test.go`'s
`TestReconcileRepairsBareErrorRowWithLivePane`. **Red/green pair: yes**, same
directory — `red-pre-fix.log`/`.exitstatus` (RED, the repair call nested inside
`terminal`) against `green-post-fix.log`/`.exitstatus` (GREEN), both produced by a
`git stash` revert-and-reproduce around the unmodified new test. Evidence directory:
[`phase3g-701-r76-bare-error/`](phase3g-701-r76-bare-error/) (7 tracked files).

### The fallout enumerated, no scenario touched (task 702): sha `608e030`

Read-only: a whole-`./features/` run at `89edd3c` turned up exactly 9 red scenarios,
all the same shape (a bare `error` write — direct-DB, probe-sourced, or a genuine
`StopFailure` hook — onto a still-live pane, now correctly repaired before the
scenario's own assertion observes it); full file:line and Go-subtest-name table in
[`phase3g-702-r76-error-fallout/README.md`](phase3g-702-r76-error-fallout/README.md).
**No red/green pair belongs to this task** — it made no code or scenario change
(`git diff --stat 89edd3c..HEAD -- '*.go' '*.feature'` empty, confirmed in the report
itself); its own `features-suite.log`/`.exitstatus` (exit `1`) is this finding's "red",
reused by every scenario fix below. Evidence directory:
[`phase3g-702-r76-error-fallout/`](phase3g-702-r76-error-fallout/) (5 tracked files).

### 7 of 9 fixed by a genuine route (task 703): shas `5ea9475`, `2094b83`

Six scenarios (`attention_sort.feature` ×3, `status_theme.feature` ×2,
`themes.feature` ×1) re-pointed from a raw `has status "error"` DB write onto a
genuine nonzero pane exit (`shell session "X" exits with status N`,
`features/crash_test.go`'s `shellSessionExitsWithNonzeroStatus`), which durably
removes the row from the live-pane repair's reach via crash-collection instead. One
more (`interactive_scroll.feature`) re-pointed the same assertion onto a longer,
derived poll window (`clientRowContainsAcrossSeveralProbeCycles`, four full
probe/repair cycles instead of one reconcile interval), since that scenario's probed
`error` really does recur, just not within a single tick. **Red/green pair: yes** —
red is task 702's own `features-suite.log` (exit 1, naming all 9 by Go subtest name);
green is this task's 3 consecutive runs each of `attention_sort.feature`,
`status_theme.feature` and `themes.feature` together (`run1.log`–`run3.log`) and of
`interactive_scroll.feature` (`interactive-scroll-run1.log`–`run3.log`), plus
two whole-suite regression checks (`full-features-suite.log`,
`full-features-suite-after-interactive-scroll-fix.log`, both exit 1, naming exactly
the scenarios still left red). Evidence directory:
[`phase3g-703-r76-error-fallout-fix/`](phase3g-703-r76-error-fallout-fix/) (17
tracked files).

### The `status_claude_hooks.feature` leg fixed (task 803): sha `bedb65a`

The remaining `StopFailure` block's `Then` assertion re-points from the unreachable
raw `error` verdict onto the deterministic post-repair tuple (`starting`/`tmux`,
reason "tmux pane is alive; terminal row corrected", `acknowledged=0`,
`notify_epoch=3`), via a new step (`databaseSessionIsRepairedTo`) that checks the
caller-supplied `status_source` instead of hard-coding `"hook"`. Scenario:
`features/status_claude_hooks.feature`'s "Every declared Claude hook maps to honest
status through both identity routes". **Red/green pair: yes**, same directory —
`pre-change-red.log`/`.exitstatus` (RED, quoting exactly the post-repair tuple this
task then asserts) against `post-change-run1.log`–`run3.log` (GREEN, 3 consecutive
runs). Evidence directory:
[`phase3g-803-hook-truth-stopfailure/`](phase3g-803-hook-truth-stopfailure/) (9
tracked files).

### `features/sort_order.feature`'s six latent races fixed (task 804): sha `46dad5e`

All six raw `has status "error"` writes (`ord-bravo` ×5, `gso-ab` ×1) replaced with
`shell session "X" exits with status 1`, plus an explicit reconcile-interval wait
before each order/group assertion. **Red/green pair: yes** —
`pre-change-red.log`/`.exitstatus` (RED, reproduced in a throwaway `git worktree` at
the pre-change commit `bedb65a`, with one extra wait step forcing the repair to land
first, never touching the live tree) against `run1.log`–`run5.log` (GREEN, 5
consecutive runs). Evidence directory:
[`phase3g-804-sort-order-error-route/`](phase3g-804-sort-order-error-route/) (13
tracked files).

### `features/status_recovery.feature`'s stale-tmux-verdict scenario fixed (task 805): sha `4651653`

The "row at error from a stale tmux launch-failure verdict recovers on the next hook"
scenario's assertion re-points from the raw forced `error`/`tmux` value (read with no
wait at all) onto a wait for the mandated repair (`starting`/`tmux`) before the later
hook fires. **Red/green pair: yes** — `red-worktree-trial.log` (RED, reproduced in a
throwaway `git worktree` at the pre-change commit `46dad5e`, forcing the reconcile
tick to land first) against `green-run-1.log`–`green-run-5.log` (GREEN, 5 consecutive
runs). Evidence directory:
[`phase3g-805-stale-tmux-verdict/`](phase3g-805-stale-tmux-verdict/) (12 tracked
files). This is a *different* `status_recovery.feature` scenario from F20's "dup
pane" one (the [known open
regression](#known-open-regression-discovered-by-task-002s-own-evidence-not-fixed-here)
above) — F20 stays open and unfixed, exactly as the standing rules require.

### Left open, verbatim: `status_attach.feature:18` (task 802)

Task 802 set out to re-point this same scenario's ("attach acknowledges a live error
without replacing its verdict") status-field assertions the same way 803/804/805 did,
while keeping its own `!`-marker assertions unweakened. **That combination is
unsatisfiable as written**: the repair runs synchronously inside the same `deck _hook`
subprocess that writes the hook-sourced error, before the subprocess returns, so
there is no window in which the raw error is ever durably observable for a later step
to race — re-pointing the status field and keeping the marker assertion are mutually
exclusive (full derivation in task 802's own `unsatisfiableReason` in `tasks.json`).
Task 802 is **`skipped`, not completed**, and carries **no fixing sha** —
`git log 1cfbd5a..HEAD -- features/status_attach.feature` is empty, confirming the
file is untouched by this run. The scenario remains red at HEAD, exactly as task 702
first found it. **No tracked red/green pair exists for this one**: the trial-edit
diagnostic that proved the mechanism was saved outside the repository
(`/run/ralphd/artifacts/phase3g-802-unsatisfiable/`, disclosed there as untracked, not
repository evidence) and is not cited here as such — `docs/reports/phase3g-802-live-error-attach/`
exists on disk but is empty and untracked (`git ls-files` returns nothing for it). The
last repository-tracked confirmation that this scenario is still red is task 703's own
`full-features-suite-after-interactive-scroll-fix.log` (exit 1), which names it
explicitly as one of the two scenarios that task left unfixed.

## Review finding 2 — R80's one-definition-per-action (tasks 806–808)

Review finding 2: `A` and `x` each had (or risked) two separate eligibility
definitions — one the footer consults to decide whether to *show* the key, one the
key handler consults to decide whether to *act* on it — free to drift apart. This
section records what closed each half, and names precisely what did not land.

### `A`: shared `canArchive` predicate (task 806, status **failed** — landed in code, not accepted as written): shas `41ae9cd`, `b7a3a81`

`canArchive` (`session.ArchivedAt == 0`) became the one predicate both
`footerLegend`'s `A` entry and `case "A"` call by name; the old always-true
handler-only predicate and the old footer-only-named predicate are both gone
(`grep -rn footerArchiveEligible internal/` has no match). Tests, all in the new
`internal/tui/archive_eligibility_test.go`: `TestArchiveKeyOnAnArchivedRowRefusesAndNamesU`
(behavioural), `TestArchiveKeyHandlerCallsTheFooterPredicateByName` (structural
source-parse, closes the "inlined-but-identical" hole the first pass, `41ae9cd`, left
open), `TestArchiveKeyEligibilityMatchesFooterPredicate`. **Red/green pairs: yes**,
several, same directory — `red-mutation-inlined-copy.log` and
`red-mutation-no-check.log` (RED, two separate mutations: the handler stops calling
`canArchive` by name but stays behaviourally identical; the handler drops the check
entirely) against `green-mutations-reverted.log` (GREEN, both reverted); plus the
first pass's own `red-mutation.log`/`green-mutation-reverted.log`. Evidence directory:
[`phase3g-806-archive-eligibility/`](phase3g-806-archive-eligibility/) (15 tracked
files).

**What did not land**: task 806 exhausted its tier ladder at 3 validation attempts
and is recorded `failed`, not `completed` — its own success criteria required the two
pre-existing footer tests (`internal/tui/footer_legend_test.go`,
`internal/tui/footer_bindings_parity_test.go`) to show **no change beyond an added
case**, and the landed commits instead rename the shared predicate identifier inside
both files (a `footerPredicateByName` map key and one comment word), which the task's
own `validationNotes` (in `tasks.json`) quote and attribute to `git diff
4651653..HEAD`. No follow-up task exists yet to close that specific gap (checked
against the current 801–815 task list: none). The functional fix — one shared,
correctly-named predicate, refusing `A` on an archived row with both new tests green —
is in the tree and pushed to the `origin` remote's `main`; the literal "existing
tests unedited" criterion is the piece that did not land. Do not read the `failed`
status as evidence the predicate itself is missing or broken:
`TestFooterHandlerAgreementAcrossEveryRowClass` (task 808, below) independently
exercises the same `A` key across all six row classes and passes.

### `x`: single-row handler now consults `canKill` (task 807, validated): sha `b434079`

`case "x"`'s single-row (no-marks) branch now calls `canKill(session)` before
dispatching, returning the same `"already stopped"` wording `service.Kill` used to
produce so `features/kill_delete_undo.feature:17`'s existing assertion is unedited.
Test: `internal/tui/kill_key_eligibility_test.go`'s
`TestKillKeyHandlerConsultsCanKillForTheSingleSelectedRow`. **Red/green pair: yes**,
same directory — `red-mutation-no-cankill-check.log`/`red-mutation-full-package.log`
(RED, the `canKill` guard removed) against `green-mutation-reverted.log` (GREEN).
Evidence directory: [`phase3g-807-kill-eligibility/`](phase3g-807-kill-eligibility/)
(6 tracked files).

### The cross-key agreement matrix (task 808, validated): shas `33e7935`, `fdf4507`

`internal/tui/footer_handler_agreement_test.go`'s
`TestFooterHandlerAgreementAcrossEveryRowClass` drives the real key handler for all
ten eligibility-gated footer keys (`Y`, `x`, `r`, `R`, `A`, `U`, `dd`, and — added by
the second commit after validation found the first round covered only seven of ten
gated entries — `↵`, `a`, `i`) across six row classes (60 subtests), asserting the
handler acted if and only if the real rendered footer legend advertised the key.
`TestFooterHandlerAgreementCoversEveryEligibilityGatedFooterKey` parses
`footerLegend`'s own source and fails if any gated entry lacks a press/observe pair,
so the matrix cannot go stale by omission. **Red/green pairs: yes**, two, same
directory — `red-mutation-no-canRestart-check.log` (RED, `case "R"`'s `!canRestart`
guard removed) against `green-mutation-reverted-full-package.log` (GREEN); and
`red-mutation-no-canReachPane-check.log` (RED, `attachSelected`'s `!canReachPane`
guard removed, added after the first validation round) against
`green-mutation-reverted-canReachPane-check.log` (GREEN). Evidence directory:
[`phase3g-808-footer-handler-agreement/`](phase3g-808-footer-handler-agreement/) (8
tracked files).

### Citation check for both finding sections (task 809)

The two finding sections above were written by `4c7bf2a`; `18063cf` added this
subsection, and a third docs-only commit — the one that adds the paragraph you are
reading — replaced its sha enumeration with the exhaustive one below. All three
commits touch only paths under `docs/`, confirmed by `git show --stat`. All three
checks pass, and each names the command that produces its result:

- **Every sha cited in the new text resolves — the enumeration is the extraction
  command's output, not a hand-kept list.** The audited region is every line this
  task's commits add to this file, and the tokens are read off it mechanically:

  ```
  git diff 4c7bf2a^..HEAD -- docs/reports/phase3g.md | grep '^+' \
    | grep -oE '\b[0-9a-f]{7,40}\b' | sort -u
  ```

  with `HEAD` at this task's third and final commit. That yields **25** distinct
  tokens, every one of which passes `git cat-file -e <sha>^{commit}` (25 OK, 0
  unresolvable): `15e33c6`, `1cfbd5a`, `2094b83`, `33e7935`, `41ae9cd`, `4651653`,
  `46dad5e`, `4c7bf2a`, `51b7f17`, `5ea9475`, `608e030`, `745a25b`, `794313f`,
  `89edd3c`, `89fcffc`, `904419c`, `b434079`, `b7a3a81`, `bedb65a`, `d266346`,
  `ebbc3fd`, `f5977d4`, `f9611b7`, `fdf4507`, `18063cf`. Twenty-two of those are the
  fixing/context shas the finding sections and the rewritten R76/R80 table rows cite;
  the remaining three name commits rather than fixes — `1cfbd5a` (this run's base sha,
  cited in task 802's subsection as the range endpoint proving
  `features/status_attach.feature` was never edited) and `4c7bf2a` plus `18063cf`
  (this task's own first two docs commits). `18063cf`'s published enumeration listed
  only the 22 fixing shas while calling itself the set of shas "appearing in the added
  text", so it omitted `1cfbd5a` and `4c7bf2a`: that omission — not an unresolvable
  sha — is the gap this paragraph closes, and the command above, re-run at the final
  commit, is the authority over any list typed by hand.
  Task 802 deliberately carries **no** sha, for the reason its own subsection gives.
- **Every linked path is tracked.** Each of the ten markdown links in the added text
  (`phase3g-701-r76-bare-error/`, `phase3g-702-r76-error-fallout/` and its
  `README.md`, `phase3g-703-r76-error-fallout-fix/`,
  `phase3g-803-hook-truth-stopfailure/`, `phase3g-804-sort-order-error-route/`,
  `phase3g-805-stale-tmux-verdict/`, `phase3g-806-archive-eligibility/`,
  `phase3g-807-kill-eligibility/`, `phase3g-808-footer-handler-agreement/`) is listed
  non-empty by `git ls-files`, with the tracked-file counts quoted in each subsection
  (7, 5, 17, 9, 13, 12, 15, 6, 8) matching `git ls-files | wc -l` for that directory
  exactly. Every product/test path cited in prose (`internal/service/reconcile.go`,
  `internal/service/reconcile_bare_error_repair_test.go`,
  `internal/tui/archive_eligibility_test.go`,
  `internal/tui/kill_key_eligibility_test.go`,
  `internal/tui/footer_handler_agreement_test.go`,
  `internal/tui/footer_legend_test.go`,
  `internal/tui/footer_bindings_parity_test.go`, `features/crash_test.go`,
  `features/sort_order.feature`, `features/status_recovery.feature`,
  `features/status_claude_hooks.feature`, `features/kill_delete_undo.feature`,
  `docs/reports/phase3g-findings.md`) is likewise tracked, and every bare log
  filename cited (`red-pre-fix.log`, `green-post-fix.log`, `features-suite.log`,
  `run1.log`–`run5.log`, `interactive-scroll-run1.log`, `full-features-suite.log`,
  `full-features-suite-after-interactive-scroll-fix.log`, `pre-change-red.log`,
  `post-change-run1.log`, `red-worktree-trial.log`, `green-run-1.log`,
  `green-run-5.log`, `red-mutation.log`, `red-mutation-inlined-copy.log`,
  `red-mutation-no-check.log`, `red-mutation-no-cankill-check.log`,
  `red-mutation-full-package.log`, `red-mutation-no-canRestart-check.log`,
  `red-mutation-no-canReachPane-check.log`, `green-mutation-reverted.log`,
  `green-mutations-reverted.log`, `green-mutation-reverted-full-package.log`,
  `green-mutation-reverted-canReachPane-check.log`) resolves to a tracked file inside
  the evidence directory its own subsection names. The one non-repository path in the
  added text (`/run/ralphd/artifacts/phase3g-802-unsatisfiable/`) is labelled
  untracked and outside the repository at the sentence that cites it, and
  `docs/reports/phase3g-802-live-error-attach/` is disclosed as empty and untracked
  (`git ls-files` returns nothing for either). `citation_sweep.py` reports no new
  unresolved citation beyond the two pre-existing ones it already documents
  (`/run/ralphd/approaches/NN/tasks.json`, `9/10`).
- **Both new sections appear in the document's own section list.**
  `grep -n '^## ' docs/reports/phase3g.md` lists
  `## Review finding 1 — R76's error branch and its scenario fallout (tasks 701, 702, 703, 802–805)`
  and
  `## Review finding 2 — R80's one-definition-per-action (tasks 806–808)`
  between `## Known open regression, …` and `## Per-requirement table`, and the three
  in-document anchors used above
  (`#review-finding-1--r76s-error-branch-and-its-scenario-fallout-tasks-701-702-703-802805`,
  `#review-finding-2--r80s-one-definition-per-action-tasks-806808`,
  `#known-open-regression-discovered-by-task-002s-own-evidence-not-fixed-here`) are the
  GitHub slugs of those headings.

Neither finding section claims a closure task 802–808 did not deliver: task 802 is
recorded `skipped`/unsatisfiable with no sha and its scenario still red, and task 806
is recorded `failed` with the residual criterion it missed named explicitly, both in
the subsections above and in the R76/R80 rows below.

## Per-requirement table

| req | status | tasks | shas | red/green quoted |
|---|---|---|---|---|
| R76 | met (review finding 1 closed by 701; one scenario left open, task 802 unsatisfiable — see [section](#review-finding-1--r76s-error-branch-and-its-scenario-fallout-tasks-701-702-703-802805)) | 001, 002, 103, 701, 702, 703, 802 (skipped), 803, 804, 805 | `89fcffc`, `15e33c6`, `904419c`, `51b7f17`, `89edd3c`, `608e030`, `5ea9475`, `2094b83`, `bedb65a`, `46dad5e`, `4651653` | yes |
| R77 | met | 003–006 | `b80a4bd`, `70162d4`, `be3df32`, `31510ab`, `c791a6a` | yes (task 003 leg) |
| R78 | met | 007–009 | `47166c8`, `e699c31`, `3247a7a` | not required |
| R79 | met (review finding 3 closed by 202, 214) | 010–011, 202, 214 | `c987953`, `e475660`, `8276450`, `e45bf2e`, `dd90a28`, `a46514e`, `6a01fe7` | yes |
| R80 | met (review finding 2: `x`/matrix closed by 807/808; `A` functionally landed by 41ae9cd/b7a3a81 but task 806 itself `failed` on a residual criterion — see [section](#review-finding-2--r80s-one-definition-per-action-tasks-806808)) | 012–013, 806 (failed), 807, 808 | `f5977d4`, `745a25b`, `f9611b7`, `ebbc3fd`, `41ae9cd`, `b7a3a81`, `b434079`, `33e7935`, `fdf4507` | not required |
| R81 | met | 014–015 | `7dbe5c5`, `3498b3e` | not required |
| R82 | **met (resolved by task 203; F27; 021's own gap closed by 105)** | 016 (skipped), 017–020, 021 (failed), 105, 107, 203 | `cdb927b`,`adb7db4`,`5cc1b45`,`8c3351a`,`26a5b47`,`f33d67a`,`7f1a780`,`103d430`,`62c3abe`,`1c8cbad`,`ea6ce4b`,`02e64a5` | yes (105, 203; retroactive not needed) |
| R83 | met (net closed by 107) | 016, 018, 101, 107 | see R82 row, plus `2549406` | not required |
| R84 | met | 106 | `57a6882`, `0219e42` | not required |
| R85 | met | 024, 102 | `9991689`, `89682e5` | not required |
| R86 | met | 025–026, 104, 108 | `a337671`, `17b7bb9`, `8cff03b`, `045a6e6`, `ab34cb4`, `860c412` | yes (retroactive, disclosed) |
| R87 | met | 027–028 | `f3f3d26`, `3d749bb` | yes |
| R88 | met | 029 | `f9c6fc4` | yes |
| R89 | met | 030–031 | `6718823`, `f70b773`, `8ddf869`, `ce22192` | yes |
| R90 | met | 032–033 | `99fc4a3`, `ab14d19`, `ca43907` | not required (read-gap quoted anyway) |
| R91 | met | 034–035 | `c93f811`, `9d6c22a` | yes (retroactive, disclosed; plus the count prediction) |
| R92 | met | 036–037 | `78bc156`, `cc36cfa` | not required (no defect to revert) |
| R93 | met | 205–207 | `b69b5ba`, `b249992`, `c3a5a06`, `7e7b0be`, `a0bf89e` | yes (retroactive on 207's round 1; disclosed) |

R82's task-016 residual is resolved (task 203; F27, per the PRD's own
`SPEC.md`-wins precedence rule) and carried in
`docs/reports/phase3g-findings.md` and the [close-out (task 113)](#close-out-task-113).

### Tasks 101–108 (approach 02), test/scenario and evidence path per sha

| task | req | sha(s) | test/scenario (file) | evidence path |
|---|---|---|---|---|
| 101 | R83 | `2549406` | `features/agent_steps_test.go` `ensureCreateModalAgent`; `kill_delete_undo.feature`'s dd-confirm 80%-clamp scenario | `docs/reports/phase3g-101-agent-wait-wrap/` |
| 102 | R85 | `89682e5` | `features/settings.feature` `@requirement-17-clear-recent-cwds-history` | `docs/reports/phase3g-102-clear-recents-label/` |
| 103 | R76 (verdict only) | `51b7f17` | `features/status_claude_hooks.feature`'s SessionEnd scenario; `features/status_claude_hooks_test.go` `releasedHookFiresForSession` | `docs/reports/phase3g-103-hook-sessionend-repair/` |
| 104 | R86 | `045a6e6` | `features/agent_steps_test.go:521` `clientOpensCreateModalForAgent`; `event_log.feature`, `durable_identity.feature`, `create_session.feature`, `dialogs.feature` | `docs/reports/phase3g-104-title-independent-waits/` |
| 105 | R82 (F18) | `ea6ce4b` | `internal/tui/rename_theme_test.go` `TestRenameViewFocusedFieldGetsSelectionBackground` | `docs/reports/phase3g-105-rename-selection/` |
| 106 | R84 | `57a6882`, `0219e42` | `internal/theme/contrast_test.go` `TestThemedDialogTokensClearContrastFloor` | `docs/reports/phase3g-106-contrast-floor/` |
| 107 | R82/R83 | `02e64a5` | `internal/tui/dialog_degradation_net_test.go` `TestThemedDialogsDegradeCleanlyUnderNoColorAndASCII`, `TestRenameFieldRowTruncationReemitsItsOwnReset` | `docs/reports/phase3g-107-dialog-degradation-net/` |
| 108 | R86 | `ab34cb4`, `860c412` | `internal/tui/help_keymap_parity_test.go`, `internal/tui/footer_bindings_parity_test.go`, `internal/tui/overlay_line_scroll_test.go` | `docs/reports/phase3g-108-r86-proof/` |
| 203 | R82 (F27) | (working-tree only; no product sha — assertion-scope finding) | `features/create_cwd_ghost_test.go` `clientCWDFieldShowsGhostText`; `create_cwd_ghost.feature`, `create_session.feature` | `docs/reports/phase3g-203-r82-assertion-conflict/` |

### Tasks 201, 202, 203, 204, 214, 301, 302, 303, 501, 502 and 503 (approaches 03-05), test/scenario and evidence path per sha

One row per task, in task-id order. 203's row duplicates the one above (it belongs to
both this table's task list and R82's F27 closure); every other row is new to this
report.

| task | sha(s) | test/scenario (file) | evidence path |
|---|---|---|---|
| 201 | `7e3261c`, `248d257` | `internal/service/rename_reuse_test.go` `TestRenameOntoATombstonedNameCleansUpThatSessionsFiles`, `TestRefusedRenameKeepsLiveAndArchivedHoldersFiles` | `docs/reports/phase3g-201-rename-reuse-cleanup/` |
| 202 | `dd90a28`, `a46514e` | `internal/store/tombstone_sweep_test.go` `TestSweepTombstonesReapsOneBoundedBatchPerCall` (rewritten in place, per operator ruling `001-202.md`) | `docs/reports/phase3g-202-tombstone-drain/` |
| 203 | `3e883d7` | `features/create_cwd_ghost_test.go` `clientCWDFieldShowsGhostText`; `create_cwd_ghost.feature`, `create_session.feature` | `docs/reports/phase3g-203-r82-assertion-conflict/` |
| 204 | `54fd6e0`, `e5ad762` | `features/kill_delete_undo_test.go` `waitForSessionColumnState`; `filter.feature`'s `@requirement-33-dd-reaches-and-tombstones-an-archived-row` scenario | `docs/reports/phase3g-204-async-db-assert-sync/` |
| 214 | `6a01fe7` | `cmd/deck/tombstone_startup_continuation_test.go` | `docs/reports/phase3g-214-tombstone-continuation/` |
| 301 | `bdc1879` | re-derivable citation-only closure of review findings 2, 3, 4 (no new test; re-runs `internal/service/rename_reuse_test.go`, `cmd/deck/tombstone_startup_continuation_test.go`/`internal/store`, and the `create_cwd_ghost.feature`/`create_session.feature` pair) | `docs/reports/phase3g-301-review-findings-closure/` |
| 302 | `d0e36e4`, `b222875`, `52c5107` | `ci/stability.sh 10` measured at the frozen code tree (the observed rate itself is out of scope for this section — see tasks 507/508/511) | `docs/reports/phase3g-302-stability10/` |
| 303 | `6524ece`, `157bb52` | `cmd/deck/main_test.go` `TestDeckBinaryEmptyHelpAndQuitThroughPTY`; `features/sigwinch_count_test.go` `TestSigwinchCountDistinguishesTwoFromThree` | `docs/reports/phase3g-303-help-pty-tail-sync/`, `docs/reports/phase3g-303-sigwinch-count-pace/` |
| 501 | `97f8832`, `19fff8d` | `internal/tmux/create_env_race_test.go` `TestCreateSurvivesInstantExitEnvironmentMirroring`, `TestCreateStillFailsOnGenuineEnvironmentError` | `docs/reports/phase3g-501-create-env-race/` |
| 502 | `306d57a`, `3f23287` | `features/attention_sort.feature`'s "the collapsed strip's attention count matches the sort's own notion of attention" scenario | `docs/reports/phase3g-502-attention-count-sync/` |
| 503 | `9f7c239`, `739fb7b` | `features/attach_scroll.feature`'s "a wheel notch scrolls an attached pane's scrollback and leaves the shell's input line untouched" scenario | `docs/reports/phase3g-503-attach-scroll-sync/` |

Task 202's row shows the sha(s) actually landed; its status, however, rests on
operator ruling `001-202.md` (steering 019), which replaced 202's own
`successCriteria` with the bar the run's own verifier had already confirmed
point by point, after 202's original criteria demanded a `cmd/deck`-level
integration test that does not exist as a seam. The one property that
replacement bar could not itself confirm — a committed test driving `cmd/deck`'s
real startup/tick continuation end to end, not `Store.DrainExpiredTombstones`
directly — was named as a residual and carried into task 214, which closed it
(`6a01fe7`, `cmd/deck/tombstone_startup_continuation_test.go`).

Every sha and every local evidence link cited anywhere in this file is swept
mechanically — 64 distinct shas through `git cat-file -e`, 29 markdown links and 33
backtick-quoted evidence paths through `os.path.exists`, both commands and their whole
output checked in at
[`phase3g-109-report-update/`](phase3g-109-report-update/). The one deliberately
non-resolving string is R86's glob `phase3g-02[56]-*/`, which names a directory that was
never produced; that absence is the disclosure, not a broken link.

## Close-out (task 113)

The phase's close-out note is
[`phase3g-113-closeout/README.md`](phase3g-113-closeout/README.md), and it is part of this
report by reference: everything below is stated and evidenced there, not here.

- **Protected-path audit, by sha and never by author or committer** (§1, §1a;
  [`protected-path-check.log`](phase3g-113-closeout/protected-path-check.log),
  [`protected-path-all-refs.log`](phase3g-113-closeout/protected-path-all-refs.log)):
  `git log --oneline 1cfbd5a..HEAD -- SPEC.md prds/ ci/Dockerfile ci/SPIKE.md` is empty, and so
  is the same query over every ref (`git log --all --oneline 1cfbd5a.. -- <protected set>`) —
  this run touched none of the four protected paths. All **30** commits in the repo's whole
  history that do touch them are ancestors of the run's base `1cfbd5a`; of the two
  pre-authorised shas only `2eed8de` touches a protected path (`SPEC.md`) at all, `a03527c`
  touching only `docs/PLAN.md`. Both of those precisions are corrections the close-out
  discloses rather than restating the standing rules' looser phrasing.
- **Citation sweep over both phase 3g reports** (§2;
  [`citation_sweep.py`](phase3g-113-closeout/citation_sweep.py),
  [`citation-sweep.log`](phase3g-113-closeout/citation-sweep.log)): shas, relative links and
  same-document anchors across `phase3g.md` and
  [`phase3g-findings.md`](phase3g-findings.md) all resolve; the sweep's first run found eight
  broken anchors in *this* file's own section list (en dash between digits, and `×`/`↑`/`↓`,
  slug to no separator) and only the anchor strings were fixed. The known self-citation
  false-positive class is disclosed there, including where it legitimately fires
  (`phase3g-findings.md` §4).
- **Delivery log** (§3): Phase 3g is recorded in [`../DELIVERY-LOG.md`](../DELIVERY-LOG.md) with
  task 111's whole-suite result (green at `9f61e21`, 311/311) and task 112's stability rate
  (9/10, root-caused), each with its log path, plus — as approach 01's close-out then stood —
  R82's *partial* status and the known open regression above. That R82 line is historical only:
  R82 was resolved afterwards by task 105 (`ea6ce4b`, finding
  [F18](phase3g-findings.md#3-defects-found-and-deliberately-not-fixed-and-why)), as the dialog
  table above records; `../DELIVERY-LOG.md`'s own text still carries the older wording.

## Close-out (approach 06)

This section closes the reporting tail (approach 06, tasks 601–608) against the phase's
final state. It supersedes nothing above — task 113's close-out (§ above) and the
R76–R93 sections stand as the record of what each approach actually delivered — this
section adds the final-sha attestation, the two suite gates, the guard re-verification,
an account of every approach this run spent, and (per (f) below) a table of every task
from approaches 01–05 that ended in a non-`completed` status, read from the archived
per-approach state at `/run/ralphd/approaches/NN/tasks.json` (outside this repository,
not part of the tracked record — quoted here rather than linked, as that task requires).

### (a) Final shas and the empty code-diff

The phase's **final code sha is `b0a4e7d`**: every commit after it is docs-only. This
section's own primary commit cannot quote its own hash inside itself (a commit's sha is
a function of its content, so it cannot contain itself) — the same regress phase 3f's
task 038 close-out named and stopped one level down, by committing the report first and
recording that commit's sha in an immediately-following addendum commit. This section
follows the identical shape: task 607's primary commit lands everything else in this
section, and the addendum immediately below — landed by a second, tiny commit — names
that primary commit's own sha as the final commit of this close-out's authorship.

> **Addendum (commit B).** Task 607's primary commit ("commit A" above), which lands
> everything else in this section, is **`3faafa7`** — "docs: write the phase close-out
> section against the final sha b0a4e7d (task 607)". This addendum is commit B, landed
> immediately after; commit B's own sha cannot be named here for the same reason commit
> A's couldn't be named inside itself, and is not needed to satisfy this criterion, which
> asks only for "this commit's own sha as the final commit" — commit A's, quoted above.
>
> **Addendum, second part (commit C).** Clause (f) below was re-cited after this section's
> first validation pass, by a third commit — **`628309b`**, "docs: cite a sha and tracked
> evidence in every close-out exceptional-status row, and file F35 for the undelivered
> delivery-log half (task 607)" — which also filed finding
> [F35](phase3g-findings.md#3-defects-found-and-deliberately-not-fixed-and-why) and added the
> tracked evidence bundle [`phase3g-607-closeout/`](phase3g-607-closeout/). `628309b` is the
> final substantive commit of this close-out's authorship; this line naming it is landed by a
> fourth, tiny commit, for the same reason commit A could not name itself.

```
$ git diff --stat b0a4e7d..HEAD -- '*.go' '*.feature' '*.sh' '*.toml' go.mod go.sum; echo "exit=$?"
exit=0
```

Empty, as it has been at every re-check since `b0a4e7d`: no product code, feature file,
CI script, TOML or Go module file has changed since the phase's final code commit.

### (b) Review finding 1's stability gate

[Task 605's stability-gate note](phase3g-605-stability-gate/README.md) closes review
finding 1 **by citation** of task 507's round-3 measurement rather than a re-run, per
this run's standing rule against re-rolling an already-held gate. Quoted verbatim from
[`phase3g-507-stability10/round3/summary.log`](phase3g-507-stability10/round3/summary.log):

```
10/10 passed
```

launched at commit `16186e3` (script exit status `0`, 10 `PASS (exit 0)` lines, 0 `FAIL`
lines), with the code-diff proof that `16186e3` and `b0a4e7d` are code-identical:
`git diff --stat 16186e3..HEAD` and `git diff --stat b0a4e7d..HEAD`, both over the same
code-pattern set, both empty. **10/10 passed, never rounded.**

### (c) The whole-suite sweep

[Task 604's whole-suite report](phase3g-604-fullsuite/README.md) — fresh sweep at launch
sha `b4c90ca`, captured exit status **`0`** (`docs/reports/phase3g-604-fullsuite/full-suite.exitstatus`),
17/17 Go packages accounted for (14 `ok`, 3 `[no test files]`: `internal/notify`,
`internal/search`, `internal/unit`), no `FAIL` anywhere, both `defaultTags` exclusions
(`~@real-agents`, `~@nightly`) named, and the Gherkin tally quoted from task 508's
committed `-v` companion log: **311 scenarios (311 passed), 3523 steps (3523 passed)**.

### (d) Guard re-verification

[Task 606's guard bundle](phase3g-606-guards/README.md) re-verifies, at starting sha
`2bad935`, all seven guards this run tracks: the run-range protected-path audit (exactly
`b69b5ba`, and nothing else, over `1cfbd5a..HEAD`), a clean tree, `HEAD == origin/main`,
`godog_test.go`'s `defaultTags` byte-unchanged, the scenario-count delta fully accounted
for, every run-range `t.Skip` addition enumerated, and a re-implemented, fenced-code-aware
citation sweep over both phase3g reports with zero unresolved citations after disposition.

### (e) Every approach this run spent

| approach | task-id range | commit-subject convention | example commit |
|---|---|---|---|
| 01 | `0NN` (001–042) | `<area>: <why> (task 0NN)` | `89fcffc` — "service,store: repair a terminal row with a live pane in Reconcile (task 001)" |
| 02 | `1NN` (101–113) | `<area>: <why> (task 1NN)` | `2549406` — "features: dewrap the create-modal Agent-field wait's box-wrapped row (task 101)" |
| 03 | `2NN` (201–214) | `<area>: <why> (task 2NN)` | `7e3261c` — "service: clean up a reaped tombstoned holder's files on rename too (task 201)" |
| 04 | `3NN` (301–309) | `<area>: <why> (task 3NN)` | `bdc1879` — "docs: re-derivable evidence bundle closing review findings 2, 3, 4 at HEAD (task 301)" |
| 05 | `5NN` (501–512) | `<area>: <why> (task 5NN)` | `97f8832` — "tmux: mirror Create's session environment via new-session -e, not a racing set-environment follow-up (task 501)" |
| 06 | `6NN` (601–608) | `<area>: <why> (task 6NN)` | `34ca8ce` — "docs: add F29–F32 findings rows for approach 04/05's 501/502/503/507 discoveries (task 601)" |

Every approach used the same suffix convention, `(task NNN)`, distinguished only by the
numeric range — never a different prefix word — which is why every standing-rules and
report citation in this run names the task id alongside the sha.

### (f) Every approach 01–05 task that ended in a non-`completed` status

Read from the archived per-approach state (`/run/ralphd/approaches/NN/tasks.json`,
outside this repository, quoted rather than linked). Every row below names either the
sha(s) and tracked evidence directory that delivered the task's scope, or the findings
row (in [`phase3g-findings.md`](phase3g-findings.md)) that carries an undelivered piece
as an open residual.

| task | approach | archived status | title | delivered by |
|---|---|---|---|---|
| 016 | 01 | skipped | Theme and bound the create modal | `cdb927b` + `adb7db4`; the one unmet clause (assertion left unchanged) is the `SPEC.md`-vs-PRD contradiction resolved as finding F27 by task 203 (`3e883d7`) — [`phase3g-203-r82-assertion-conflict/`](phase3g-203-r82-assertion-conflict/) |
| 021 | 01 | failed | Theme the rename dialog and the event log | `1c8cbad`; the residual clause (focused-field selection background) closed by task 105 (`ea6ce4b`), finding F18 — [`phase3g-105-rename-selection/`](phase3g-105-rename-selection/) |
| 022 | 01 | pending | Net themed dialogs against `NO_COLOR`/`DECK_ASCII`/width | task 107, `02e64a5` — [`phase3g-107-dialog-degradation-net/`](phase3g-107-dialog-degradation-net/) |
| 023 | 01 | pending | Extend the contrast floor to the pairs a dialog uses | task 106, `57a6882`, `0219e42` — [`phase3g-106-contrast-floor/`](phase3g-106-contrast-floor/) |
| 026 | 01 | skipped | Update dialog text for new keys, prove overlays keep line scroll | `8cff03b` + task 108, `ab34cb4`, `860c412` — [`phase3g-108-r86-proof/`](phase3g-108-r86-proof/) |
| 038 | 01 | skipped | Write `docs/reports/phase3g.md` | delivered incrementally: task 038's own `9ee8672`/`e02ef08`/`050ca9f`, folded forward by task 109 (`28b0ada`) — [`phase3g-109-report-update/`](phase3g-109-report-update/) — and every later reporting task through this section |
| 040 | 01 | in-progress | Run the whole suite green at the final code commit | superseded through 111/210/305/508; delivered at record by task 604, `b4c90ca` (exit `0`) — [`phase3g-604-fullsuite/`](phase3g-604-fullsuite/) |
| 041 | 01 | pending | Run `ci/stability.sh 10` and publish the real rate | superseded through 112/211/304/302/505; the 10/10 gate is established at `b0a4e7d` by task 507's round 3, closed by citation in task 605, `2bad935` — [`phase3g-507-stability10/round3/`](phase3g-507-stability10/round3/) |
| 042 | 01 | pending | Close out: protected-path audit, citation sweeps, delivery log | delivered by task 113, `df7a35e`/`70fdea0` — [`phase3g-113-closeout/`](phase3g-113-closeout/); re-verified run-range-scoped by task 606, `e36112a` — [`phase3g-606-guards/`](phase3g-606-guards/); the delivery-log half is **undelivered**, carried as open residual finding [F35](phase3g-findings.md#3-defects-found-and-deliberately-not-fixed-and-why) (task 608's own remit) |
| 113 | 02 | skipped | Close the phase out: protected-path audit, citation sweeps, delivery log | delivered in substance at `df7a35e`/`70fdea0` — [`phase3g-113-closeout/`](phase3g-113-closeout/); the one unsatisfiable clause (claiming only two shas ever touch protected paths) is superseded by task 606's run-range-scoped, exact-count restatement (30 pre-base commits) — [`phase3g-606-guards/guard-a-protected-paths.log`](phase3g-606-guards/guard-a-protected-paths.log) |
| 202 | 03 | awaiting-validation | Drain the whole expired-tombstone backlog (review finding 3, R79) | `dd90a28`, `a46514e`; the one property the replacement bar could not confirm was carried into task 214, `6a01fe7` — [`phase3g-202-tombstone-drain/`](phase3g-202-tombstone-drain/), [`phase3g-214-tombstone-continuation/`](phase3g-214-tombstone-continuation/); process recorded as finding F33 in `phase3g-findings.md` |
| 208 | 03 | pending | Record approach 03's work in `phase3g.md` | task 504, `af288eb` — [`phase3g-504-report-update/`](phase3g-504-report-update/) |
| 209 | 03 | pending | Bring `phase3g-findings.md` up to date at approach-03 state | task 110, `961e9cc` (F18–F26) — [`phase3g-110-findings-update/`](phase3g-110-findings-update/) — and the task-202 planning lesson finally as finding F33, task 602, `2058c08`, whose delivered artefact is the tracked findings report itself, [`phase3g-findings.md`](phase3g-findings.md#3-defects-found-and-deliberately-not-fixed-and-why) |
| 210 | 03 | pending | Sweep: one whole-suite run at the post-work head | superseded through 305/508; delivered by task 604, `b4c90ca` — [`phase3g-604-fullsuite/`](phase3g-604-fullsuite/) |
| 211 | 03 | pending | Sweep: `ci/stability.sh 10` at 10/10 (finding 1's gate) | superseded through 304/505; delivered at `b0a4e7d` by task 507's round 3 — [`phase3g-507-stability10/round3/`](phase3g-507-stability10/round3/) — closed by citation task 605, `2bad935` — [`phase3g-605-stability-gate/`](phase3g-605-stability-gate/) |
| 212 | 03 | pending | Re-verify the run's guards at the final code sha | superseded through 308/510; delivered by task 606, `e36112a` — [`phase3g-606-guards/`](phase3g-606-guards/) |
| 213 | 03 | pending | Write the close-out section against the true final sha | superseded through 309/511; delivered by this task, 607: `3faafa7` (this section) and `8545366` (its (a) addendum), corrected by `628309b` — evidence [`phase3g-607-closeout/`](phase3g-607-closeout/) |
| 303 | 04 | in-progress | Synchronise every scenario task 302 recorded as failing | `6524ece` (help-overlay PTY tail race) and `157bb52` (SIGWINCH inter-resize pacing) — [`phase3g-303-help-pty-tail-sync/`](phase3g-303-help-pty-tail-sync/), [`phase3g-303-sigwinch-count-pace/`](phase3g-303-sigwinch-count-pace/) |
| 304 | 04 | pending | Establish review finding 1's gate: 10/10 on the final tree | superseded by 505; delivered at `b0a4e7d` by task 507's round 3 — [`phase3g-507-stability10/round3/`](phase3g-507-stability10/round3/) — closed by citation task 605, `2bad935` — [`phase3g-605-stability-gate/`](phase3g-605-stability-gate/) |
| 305 | 04 | pending | Sweep: one whole-suite run at the final code sha | superseded by 508; delivered by task 604, `b4c90ca` — [`phase3g-604-fullsuite/`](phase3g-604-fullsuite/) |
| 306 | 04 | pending | Record approach 03/04's work, including a new R93 section | task 504, `af288eb` — [`phase3g-504-report-update/`](phase3g-504-report-update/) |
| 307 | 04 | pending | Bring `phase3g-findings.md` up to date at approach-04 state | task 506, `7fa6f89` (F28 disposition) — [`phase3g-506-sigwinch-disposition/`](phase3g-506-sigwinch-disposition/) — plus task 601, `34ca8ce` (F29–F32) and task 602, `2058c08` (F33/F34), whose delivered artefact is the tracked findings report itself, [`phase3g-findings.md`](phase3g-findings.md#3-defects-found-and-deliberately-not-fixed-and-why) |
| 308 | 04 | pending | Re-verify the run's guards, scoped to the run range | superseded by 510; delivered by task 606, `e36112a` — [`phase3g-606-guards/`](phase3g-606-guards/) |
| 309 | 04 | pending | Write the close-out against the true final sha, update delivery log | superseded by 511; close-out half delivered by this task, 607: `3faafa7`, `8545366` and `628309b` — evidence [`phase3g-607-closeout/`](phase3g-607-closeout/); the delivery-log half is **undelivered**, carried as open residual finding [F35](phase3g-findings.md#3-defects-found-and-deliberately-not-fixed-and-why) |
| 505 | 05 | failed | Measure `ci/stability.sh 10` at the post-fix tree | delivered a real, if launcher-imprecise, 9/10 measurement (`bfa3aad`) naming the one sigwinch failure — [`phase3g-505-stability10/`](phase3g-505-stability10/) (both rounds, kept unedited) — wholly superseded by task 507's clean 10/10 round 3 at `b0a4e7d` — [`phase3g-507-stability10/round3/`](phase3g-507-stability10/round3/) — so no residual is live |
| 508 | 05 | skipped | Sweep the whole suite once, every exclusion named | delivered in substance at `550a265`/`898a54e`/`b5a228d` (exact-launcher log at `8f8e214`) — [`phase3g-508-fullsuite/`](phase3g-508-fullsuite/); the one unmet clause (one log with both the exact launcher and the verbose tally) is a `go test` stdout-buffering constraint, recorded as finding F34 by task 602, `2058c08`, and superseded cleanly by task 604's single-launcher, `tail -5`-only sweep, `b4c90ca` |
| 509 | 05 | pending | Bring `phase3g-findings.md` up to date at approach-05 state | task 601, `34ca8ce` (F29–F32) + task 602, `2058c08` (F33/F34); the delivered artefact is the tracked findings report itself, [`phase3g-findings.md`](phase3g-findings.md#3-defects-found-and-deliberately-not-fixed-and-why), and those rows rest on the tracked evidence directories [`phase3g-501-create-env-race/`](phase3g-501-create-env-race/), [`phase3g-502-attention-count-sync/`](phase3g-502-attention-count-sync/), [`phase3g-503-attach-scroll-sync/`](phase3g-503-attach-scroll-sync/), [`phase3g-507-sigwinch-startup-race/`](phase3g-507-sigwinch-startup-race/) and [`phase3g-508-fullsuite/`](phase3g-508-fullsuite/) |
| 510 | 05 | pending | Re-verify the run's guards at the current sha | delivered by task 606, `e36112a` — [`phase3g-606-guards/`](phase3g-606-guards/) |
| 511 | 05 | pending | Write the close-out against the true final sha, update delivery log | close-out half delivered by this task, 607: `3faafa7`, `8545366` and `628309b` — evidence [`phase3g-607-closeout/`](phase3g-607-closeout/); the delivery-log half is **undelivered**, carried as open residual finding [F35](phase3g-findings.md#3-defects-found-and-deliberately-not-fixed-and-why) |

### (g) The four review findings of the last review pass

| finding | what it required | closing sha(s) | tracked evidence |
|---|---|---|---|
| 1 | `ci/stability.sh 10` at 10/10 on the final code tree | measured at `b0a4e7d` by task 507's round 3 (launch `16186e3`), closed by citation in task 605, `2bad935` | [`phase3g-507-stability10/round3/`](phase3g-507-stability10/round3/), [`phase3g-605-stability-gate/`](phase3g-605-stability-gate/) |
| 2 | R77's rename-path post-commit filesystem cleanup | `7e3261c` (task 201), re-verified at HEAD by task 301, `bdc1879` | [`phase3g-201-rename-reuse-cleanup/`](phase3g-201-rename-reuse-cleanup/), [`phase3g-301-review-findings-closure/`](phase3g-301-review-findings-closure/) |
| 3 | R79's whole expired-tombstone backlog drained within the open/startup cycle | `dd90a28`, `a46514e` (task 202) plus `6a01fe7` (task 214), re-verified by task 301, `bdc1879` | [`phase3g-202-tombstone-drain/`](phase3g-202-tombstone-drain/), [`phase3g-214-tombstone-continuation/`](phase3g-214-tombstone-continuation/), [`phase3g-301-review-findings-closure/`](phase3g-301-review-findings-closure/) |
| 4 | R82's unchanged-PTY-assertion condition vs. `SPEC.md`'s dimmed-help requirement | `3e883d7` (task 203), filed as finding F27, re-verified by task 301, `bdc1879` | [`phase3g-203-r82-assertion-conflict/`](phase3g-203-r82-assertion-conflict/), [`phase3g-301-review-findings-closure/`](phase3g-301-review-findings-closure/) |

All four review findings are closed by sha and tracked evidence, none by assertion alone.
