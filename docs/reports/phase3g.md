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
[R86 section](#r86--updown-navigate-dialog-fields-tab-is-completion-only-tasks-025-026)
with `docs/reports/phase3g-038-r86-dialog-arrow-nav/README.md`, and the
[R91 section](#r91--previewfits-spent-fit-and-the-sigwinch-re-baseline-it-licenses-tasks-034-035)
with `docs/reports/phase3g-038-r91-previewfit-latch/README.md`.

- Sections: [R76](#r76--the-reconcile-repairs-a-terminal-row-with-a-live-pane-tasks-001-002) ·
  [R77](#r77--a-deleted-sessions-name-is-reusable-tasks-003-006) ·
  [R78](#r78--an-archived-row-keeps-its-name-and-dd-frees-it-tasks-007-009) ·
  [R79](#r79--a-tombstone-that-outlives-its-process-is-reaped-at-the-next-store-open-tasks-010-011) ·
  [R80](#r80--the-footer-lists-only-what-the-current-selection-will-accept-tasks-012-013) ·
  [R81](#r81--the-footers-fixed-set-is-curated-tasks-014-015) ·
  [R82](#r82--dialogs-are-themed-tasks-016-021) ·
  [R83](#r83--the-three-dialogs-that-draw-past-the-frame-at-80x24-tasks-016-018-022) ·
  [R84](#r84--the-contrast-floor-covers-the-pairs-a-dialog-actually-uses-task-023-not-yet-done) ·
  [R85](#r85--the-create-modal-opens-on-the-last-used-agent-task-024) ·
  [R86](#r86--updown-navigate-dialog-fields-tab-is-completion-only-tasks-025-026) ·
  [R87](#r87--esc-clears-a-filter-held-in-force-tasks-027-028) ·
  [R88](#r88--inject-refuses-a-retained-dead-pane-task-029) ·
  [R89](#r89--the-interactive-pipe-leaks-nothing-on-abnormal-exit-tasks-030-031) ·
  [R90](#r90--a-hook-dropped-as-superseded-is-labelled-dropped-tasks-032-033) ·
  [R91](#r91--previewfits-spent-fit-and-the-sigwinch-re-baseline-it-licenses-tasks-034-035) ·
  [R92](#r92--r75s-release-failure-fallback-is-exercised-tasks-036-037) ·
  [known open regression](#known-open-regression-discovered-by-task-002s-own-evidence-not-fixed-here) ·
  [table](#per-requirement-table)

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
(covered by task 040's whole-suite run; no dedicated per-task log directory exists for
R78, since it is not one of the eight naive-test-trap requirements).

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
| create modal | 016 | `cdb927b`, `adb7db4` | **partial — task skipped, see below** |
| env editor | 017 | `5cc1b45`, `8c3351a` | done |
| bulk delete confirm | 018 | `26a5b47`, `f33d67a`, `7f1a780` | done |
| archive / delete-purge confirms | 019 | `103d430` | done |
| profile picker, pin conversation, restart-or-inject | 020 | `62c3abe` | done |
| rename dialog, event log | 021 | `1c8cbad` | **partial — task failed, see below** |

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
`/run/ralphd/artifacts/task016-unsatisfiable/README.md` and its
`original-step-vs-themed-modal.log` (the pre-016 step, replayed against the themed
modal at `adb7db4`: `step error: client "A" has a dimmed-token cell " " at row 3
column 2, want no ghost text anywhere on screen` — row 3 is the Name field's help
line, not the ghost). The create modal **is** themed and bounded in the tree; only the
literal "byte-unchanged" half of the criterion is what could not also hold.

**Task 021 is `failed`, not `validated`, after 3 validation attempts.** `1c8cbad`
themes both the rename dialog and the event log, and `ci/run.sh go test -count=1
./internal/tui/` plus `event_log.feature` are green — but validation found a residual
gap the criterion also implied: the rename dialog's always-focused "New name" field
renders through `detailField`, which applies only `theme.Hint`/`theme.Text`; nothing in
`internal/tui/rename.go` composes a `theme.Selection` background, contrary to §11.4's
focused-row treatment (the same treatment `renderCreateRowSegments` already gives the
create modal's own focused field). The recorded follow-up (not yet its own task): apply
`theme.Selection` to the rename dialog's focused field, with a rendered-grid assertion
that a New-name cell carries that background, plain text still byte-identical.

Green (both dialogs' own theme tests plus the whole `internal/tui` package, at
`cc36cfa`): `ci/run.sh go test -count=1 ./internal/tui/`; event log:
`ci/run.sh env DECK_GODOG_PATHS=event_log.feature go test ./features/ -run TestFeatures -count=1`.

**`NO_COLOR`/`DECK_ASCII` degradation and width accounting under the new tokens are
netted by task 022, which is still `pending`** — see [R83](#r83--the-three-dialogs-that-draw-past-the-frame-at-80x24-tasks-016-018-022).

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

**Task 022 — "net the themed dialogs against `NO_COLOR`, `DECK_ASCII` and width
accounting" — is `pending`, not yet started.** The bounding itself is in the tree and
covered by the dialogs' own theme tests; what is not yet done is the cross-dialog
regression net the PRD's "Width and height accounting must not shift" and "`NO_COLOR`
and `DECK_ASCII` degradation is the one thing this can regress" bullets ask for. This
report does not claim that net exists.

## R84 — the contrast floor covers the pairs a dialog actually uses (task 023, not yet done)

`SPEC.md` §11.6. Not one of the eight. **Task 023 is `pending`.** `hint`/`surface`,
`key`/`surface`, `error`/`surface`, and every text token over `Selection`/
`SelectionIdle` for the focused field are **not yet added** to
`internal/theme/contrast_test.go`'s coverage. This report makes no claim that R84 is
met; it is recorded here only so the requirement has a section, per this report's own
structural criterion.

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

**Red** (`inject.go` stashed, test kept),
`docs/reports/phase3g-029-inject-retained-dead-pane/red-before-fix.log`: fails on the
message-content assertion — the refusal that surfaces is tmux's own generic wrapped
message, not the crafted one naming the restart route.

**Green**, `green-after-fix.log`: passes.

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
(`cc36cfa` also converges `positionCreateModalOnProfileField` onto task 024's
`ensureCreateModalAgent` helper — needed because `lease_race.feature` creates two
claude sessions in one scenario, the same convergence gap R90's evidence flagged; only
this one further call site, not the full ~19-site sweep.)

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
`docs/reports/phase3g-findings.md` (task 039) and a follow-up task to rewrite the
scenario's assertion to a store read or otherwise account for R76.

## Per-requirement table

| req | status | tasks | shas | red/green quoted |
|---|---|---|---|---|
| R76 | met | 001, 002 | `89fcffc`, `15e33c6`, `904419c` | yes |
| R77 | met | 003–006 | `b80a4bd`, `70162d4`, `be3df32`, `31510ab`, `c791a6a` | yes (task 003 leg) |
| R78 | met | 007–009 | `47166c8`, `e699c31`, `3247a7a` | not required |
| R79 | met | 010–011 | `c987953`, `e475660`, `8276450`, `e45bf2e` | yes |
| R80 | met | 012–013 | `f5977d4`, `745a25b`, `f9611b7`, `ebbc3fd` | not required |
| R81 | met | 014–015 | `7dbe5c5`, `3498b3e` | not required |
| R82 | **partial** | 016 (skipped), 017–020, 021 (failed) | `cdb927b`,`adb7db4`,`5cc1b45`,`8c3351a`,`26a5b47`,`f33d67a`,`7f1a780`,`103d430`,`62c3abe`,`1c8cbad` | not required |
| R83 | **partial** | 016, 018, 022 (pending) | see R82 row | not required |
| R84 | **not done** | 023 (pending) | none | not required |
| R85 | met | 024 | `9991689` | not required |
| R86 | met | 025–026 | `a337671`, `17b7bb9`, `8cff03b` | yes (retroactive, disclosed) |
| R87 | met | 027–028 | `f3f3d26`, `3d749bb` | yes |
| R88 | met | 029 | `f9c6fc4` | yes |
| R89 | met | 030–031 | `6718823`, `f70b773`, `8ddf869`, `ce22192` | yes |
| R90 | met | 032–033 | `99fc4a3`, `ab14d19`, `ca43907` | not required (read-gap quoted anyway) |
| R91 | met | 034–035 | `c93f811`, `9d6c22a` | yes (retroactive, disclosed; plus the count prediction) |
| R92 | met | 036–037 | `78bc156`, `cc36cfa` | not required (no defect to revert) |

"Partial"/"not done" rows are not claims of completion; they are carried forward to
`docs/reports/phase3g-findings.md` (task 039) and the close-out (task 042).
