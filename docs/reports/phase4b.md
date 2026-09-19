# Phase 4b — manual session groups + interactive scroll cue (task 024)

Per-requirement record for the whole phase (`prds/phase4b-manual-groups-and-scroll-cue.md`,
GH #25/#30), written at the point the freeze-line tail (tasks 021–027) is nearly closed out.

- **Code sha this file is written against:** `b96015e1aa78f5a1de4476e888a5ddd6c958b67a`
  (this file's own first landing, task 024 — the tip of `main` at the moment this
  revision was authored; the revision corrects two commit attributions, see
  "Attribution corrections" at the end of this file).
- **Task 021's gate sha (the whole-suite sweep's launch sha, and the sha every
  requirement below is scored against):** `0806ba64ede4356af50b404affd10f26f68d4d84`
  (task 020, "tui: prove shared-state.db group edits reach another client on reload
  (R131)" — the last code-touching commit on the GO branch). Every commit from
  `924a039` (task 021's launch) through this file's own commit is record-only
  (`docs/reports/`, `docs/DELIVERY-LOG.md`, `docs/roadmap.md`, `docs/prds/README.md`);
  `git diff --stat 0806ba6..HEAD -- '*.go' '*.feature'` prints nothing, so the code
  tree at every one of those commits — including this one — is byte-identical to
  `0806ba6`, and the requirement scoring below holds at all of them.
- **Docs-only tail:** this file, `docs/reports/phase4b-findings.md` (task 025) and the
  new `docs/DELIVERY-LOG.md` row (task 026) are the remaining docs-only tail after
  this commit; task 027 then re-verifies the guards at whichever sha lands last.

## Tier 1 → Tier 2 decision

Both tiers were started and shipped. The decision to proceed with Tier 2 —
**GO** — its budget snapshot and its rationale are recorded in full at
[`docs/reports/phase4b-tier2-decision.md`](./phase4b-tier2-decision.md) (task 006,
commit `16fc86a`). That record is the source of truth for the tier branch taken;
this file does not restate its budget arithmetic.

## Gate result and stability headline

- **Whole-suite gate (task 021):** green, exit `0`, 440s wall clock, at gate sha
  `0806ba64ede4356af50b404affd10f26f68d4d84` — 15 `ok` package lines + 3
  `[no test files]` lines (`internal/notify`, `internal/search`, `internal/unit`), no
  `FAIL`. Quoted from the source: "**Exit status:** `0` (green) … **Wall clock:**
  440s … in line with the launch-sha baseline of 438s". Source:
  [`docs/reports/phase4b-final-suite/README.md`](./phase4b-final-suite/README.md).
- **Ten-run stability (task 023):** 10/10 passed, exit `0` every run, 73.83 minutes
  wall clock (`2026-09-19T16:01:32Z`–`2026-09-19T17:15:22Z`), same gate sha. Quoted
  from the source: "## Result: 10/10 passed … Taken directly from
  `ci/stability.sh`'s own PASS/FAIL labels, which come from each run's actual (never
  piped/`tee`'d) `go test` exit status". Neither known-open flake class
  (transient-`starting` assertion, SIGWINCH exact-count) occurred in any of the ten
  runs. Source:
  [`docs/reports/phase4b-stability10/README.md`](./phase4b-stability10/README.md).

## Tier 1 — the interactive scroll cue (GH #30)

### R133 — the scrolled-back view says so, from the offset actually used

**Shipped**, in two commits:

- `8bd8a91` (task 001) — `interactiveBodyLines` now consumes `RenderRows`' second
  return (the clamped, actually-used offset) and heals
  `m.interactiveScrollOffset` to it, instead of discarding it as at launch sha
  `c3b530a`. Test: `internal/tui/interactive_scroll_heal_test.go` — a test that
  scrolls a model past the real scrollback length and asserts the stored offset
  ends equal to the clamped used offset, and a second test asserting the II-49
  not-repainted notice renders again once a stale-high stored offset is healed.
- `e44b17f` (task 002) — the interactive footer renders the "how far back / not
  live" cue drawn from the healed offset, glyph resolved through `m.glyph` (ASCII
  fallback under `[ui] ascii`). Test: `internal/tui/interactive_footer_scroll_cue_test.go`
  — a test scrolling past the real scrollback length asserting the clamped
  position is reported (recorded as observed failing against the raw
  `m.interactiveScrollOffset` field before the fix), a top-of-scrollback wording
  test distinct from ordinary scrolled-back wording, and an offset-0 test
  asserting no cue and an unchanged footer line.

Proof: `internal/tui` package tests green (`docs/reports/phase4b-tier1-suite/internal-tui.log`,
task 005) and again at the whole-suite gate (task 021).

### R134 — the interactive footer advertises the scroll keys

**Shipped** in `c1dd5f1` (task 003). `interactiveFooterLine` now names
Shift+PgUp/PgDn (already bound by interactive mode) and degrades three optional
segments — the R133 cue, the new scroll-key advertisement, and the pre-existing
forward-note reminder — from least to most essential, width-budgeted with
`stringWidth` the same way `footerLegendWithin` degrades the list-mode legend
(the commit's own comment names that function as the model it mirrors; it does
not call it), `Ctrl+Q` never dropping. The commit touches exactly two
files (`git diff-tree --no-commit-id --name-only -r c1dd5f1`):
`internal/tui/tui.go` and the new test file below. Help (`helpText`'s Keys/Mouse
sections) and the mouse table already documented Shift+PgUp/PgDn before this task
and needed **no** wording change, so the pinned substrings in
`cmd/deck/main_test.go`/`internal/tui/tui_test.go` were left untouched as well —
the commit message records exactly this, and both packages' full suites were run
green to confirm nothing had to move. Test:
`internal/tui/interactive_footer_scroll_advertisement_test.go` — an 80-column
footer test asserting the frame is not exceeded with `Ctrl+Q` surviving every
degradation step it drives (widths 200/100/80/60/20), and
`TestInteractiveFooterScrollKeyWordingAgreesWithHelpAndMouseTables`, which
asserts the footer agrees with the *existing* help/mouse wording rather than
changing either.
Golden 80×24 frame test (`features/golden_frame_test.go`) green — see item 6 of
`docs/reports/phase4b-tier1-suite/README.md`.

### R135 — a keystroke that forwards nothing must not snap the view to the live bottom

**Shipped** in `6fcb7bd` (task 004). `updateInteractive`'s
`m.interactiveScrollOffset = 0` reset is now conditional on a key actually
forwarding bytes (moved past the `interactiveNamedKey`/`interactiveLiteralPayload`
decision), `Ctrl+Q` unaffected (it returns earlier). Test:
`internal/tui/interactive_forward_gate_test.go` — a key matching neither helper
into a scrolled-back model asserting the offset is unchanged (fails against
pre-fix code), plus a key that does forward still snapping to 0.

**Tier 1 targeted evidence** (all six items, build/vet/`internal/tui`/`cmd/deck`/
three interactive `.feature` files/golden-frame, all exit `0`) is recorded at
[`docs/reports/phase4b-tier1-suite/README.md`](./phase4b-tier1-suite/README.md)
(task 005, commit `ae81fee`, sha `6fcb7bd`).

## Tier 2 — manual session groups (GH #25)

### R128 — the group model replaces the workspace label

**Shipped**, across four commits:

- `bf1c085` (task 008, part 1) — `schemaV7` (SPEC §4's `groups` table +
  `sessions.group_id`, no foreign key by design), the one-transaction migration
  (every existing session lands in `default`, group list starts empty), and the
  removal of `sessions.workspace`/`store.DefaultWorkspace`/`Session.Workspace`/
  `Session.WorkspaceColumn` — landed together in one commit per the standing
  rules' irreversible-migration discipline. This same commit also carries the
  environment rename `DECK_SESSION_WORKSPACE` → `DECK_SESSION_GROUP` (holding
  `GroupName` verbatim, the old name never aliased) in
  `internal/service/session_context.go`, with
  `TestSessionContextEnvExportsGroupNotWorkspace` in
  `internal/service/session_context_test.go`. Test:
  `internal/store/schema_v7_migration_test.go` — a fixture DB built by the
  previous schema with several distinct `workspace` values, asserting every row
  reads `default` afterwards and the file still opens, plus an atomicity test (a
  forced mid-way failure leaves the old schema intact and readable).
- `9eefaa2` (part of task 008) — the notification-payload half `bf1c085` left
  out: `internal/service/session_context.go` gains exported
  `NotificationSession` (json tags = SPEC §10.1's eight session field names,
  including `group` and never `workspace`) and `NotificationSessionPayload`,
  which projects one `store.Session` row onto them, `group` reading
  `store.Session.GroupName` verbatim with no cwd-derived fallback. The commit
  touches exactly `internal/service/session_context.go` and
  `internal/service/session_context_test.go`; it adds no schema and no
  migration, and it does not rename the environment variable (that landed in
  `bf1c085`, above). Test:
  `TestNotificationSessionPayloadCarriesGroupNotWorkspace` in
  `internal/service/session_context_test.go`, asserting the rendered JSON (not
  the Go field) carries `group`, mentions `workspace` nowhere, and holds exactly
  those eight fields.
- `60075ec` + `5adeb83` (task 009, part 2) — group CRUD with SPEC's name rules:
  trimmed, non-empty, no control characters, case-insensitive uniqueness
  (`COLLATE NOCASE`), `default` reserved in any case, 32-char cap. Test:
  `internal/store/group_crud_test.go`.
- `f171168` (task 014) — the guard closing R128/R129: no `workspace` identifier
  survives in `internal/store`, `internal/tui` or `internal/service`. Test:
  `internal/tui/registry_guard_test.go` (the black-box idiom named in the PRD).

### R129 — the sidebar renders manual groups

**Shipped**, across five commits:

- `35806e8` + `3e3e9f7` (task 011, part 1) — alphabetical, case-insensitive group
  order with `default` always last; header budgeted against the panel's real
  text width. Test: `internal/tui/group_order_test.go` — order tests including a
  group named `zzz` and one named `aaa` with `default` still last.
- `89125f2` (task 012, part 2) — `[ui] group_by_workspace` +
  `DECK_GROUP_BY_WORKSPACE` deleted (config/toml/schema/settings/`group.go`), flat
  (no-header) sidebar mode deleted, `reorderPreservingGrouping`'s attention-follow
  behaviour deleted. Test: `internal/config/toml_write_test.go` (new cases),
  `internal/tui/flat_sidebar_test.go` and the flat branch's own tests deleted with
  the behaviour (not inverted), per the standing rules.
- `8e9de6e` (task 013, part 3) — sidebar collapse and hit-test keyed by group id,
  `collapsed_groups` persisted in `ui_state` (`3044e9b`, task 010), surviving a
  Model rebuild. Test: `internal/tui/group_id_navigation_test.go`.
- `f171168` (task 014) — see R128 above; this is the same commit closing both
  requirements' "no workspace identifier" clause.
- `8773100` (task 015, part 4) — `/` matches name, group and `cwd`, per-group
  matching counts under an active filter. Test: `internal/tui/filter_test.go`
  (new cases) — a filter test matching a group name.

Header count-carrying (`▾ <name>  (<n>)`, `(0)` included, name elides but count/
chevron never do) and the dangling-`group_id`-degrades-to-`default` behaviour are
exercised inside `group_order_test.go`/`group_test.go`/`group_id_navigation_test.go`
across the commits above. `e67dd63` (features package repair after the workspace
drop and group-order change) keeps the `features` suite green through this
requirement's landing.

### R130 — membership is set where the session is

**Shipped**, across two commits (plus one cure):

- `ddeab8d` + `338a600` (task 016, part 1) — create modal `Group` field cycling
  available groups alphabetically with `default` last, defaulting to the last
  group created into (persisted in `ui_state` beside the last-used-agent key,
  same fallback-to-`default` precedent as `GetLastCreateAgent`). Test:
  `internal/tui/create_last_used_group_test.go`, `features/create_session_test.go`
  — a create-into-a-group scenario asserting the persisted `group_id`, and the
  remembered default surviving a restart / falling back when its group is gone.
- `c754c43` (task 017, part 2) + cure `bf964b8` — the `i` detail dialog shows the
  session's group and a `g` picker moves it, resolved off the session row itself
  (the cure fixed a stale-snapshot bug found after the first landing). Test:
  `internal/tui/group_move_test.go` — a move via `i` asserting exactly one row
  changed, and a footer/help parity test naming `g`.

### R131 — the group list is edited in settings

**Shipped**, across three commits:

- `8038b4d` (task 018, part 1) — settings Groups section: `n` creates (inline
  validation from R128's name rules), `r` renames, applied immediately (not
  staged), `esc` never offers a discard prompt in that section. Test:
  `internal/tui/settings_groups_test.go`.
- `74263ab` (task 019, part 2) + cure `61b6f7d` — `d` deletes: an empty group
  deletes with no prompt (default is never a row); a non-empty group opens the
  two-branch confirm (`m` → `SetSessionGroup(NULL)` for every member then drop
  the row; `d` → routes into the **existing** `dd` batch path via
  `m.marked`/`m.deleteConfirming` → `updateBulkDeleteConfirm`/`Service.Delete`, no
  second deletion implementation). The cure added the group-row deletion once
  the batch actually commits (a partial failure keeps the row). Test:
  `internal/tui/settings_groups_test.go` (both delete branches as scenarios, the
  destructive one asserting tombstones and that one `u` restores the whole
  batch; an empty-group delete with no prompt; a guard that settings' delete
  calls the same service `dd` does), plus
  `features/create_session_test.go`'s `kill_delete_undo.feature` destructive
  scenario (38 scenarios/515 steps passed at cure sha `61b6f7d`).
- `0806ba6` (task 020) — proves the shared-`state.db` case: two `store.OpenPath`
  handles on one file; create/rename seen via `ListGroups`, rename leaves member
  `group_id` untouched, delete observed through the second client's real TUI
  reload path degrading to `default`, never vanishing. Test:
  `internal/tui/group_shared_state_db_test.go`.

## Nothing missing

All seven requirements (R133, R134, R135, R128, R129, R130, R131) shipped; Tier 2
was started and completed, per the GO decision at
`docs/reports/phase4b-tier2-decision.md`. There is no unshipped requirement to
account for here — the residual gaps this run chose not to fix (if any) are
`docs/reports/phase4b-findings.md`'s job (task 025), not this file's.

## Attribution corrections (this revision)

The first landing of this file (`b96015e`) mis-attributed two changes; both are
corrected above against `git diff-tree`/`git show` on the commits themselves, and
are recorded here rather than silently rewritten:

1. **R134 / `c1dd5f1`** — it claimed the commit also updated the help and mouse
   wording and the pinned substrings in `cmd/deck/main_test.go` and
   `internal/tui/tui_test.go`. It did not: `git diff-tree --no-commit-id
   --name-only -r c1dd5f1` lists only `internal/tui/tui.go` and
   `internal/tui/interactive_footer_scroll_advertisement_test.go`, and the commit
   message states those existing strings already named the keys and needed no
   update.
2. **R128 / `9eefaa2`** — it attributed the `DECK_SESSION_WORKSPACE` →
   `DECK_SESSION_GROUP` environment rename to `9eefaa2`. `git show bf1c085`
   proves the rename landed in `bf1c085` (its message: "`DECK_SESSION_WORKSPACE`
   -> `DECK_SESSION_GROUP` holding GroupName verbatim, old name never aliased");
   `9eefaa2` added only the SPEC §10.1 notification-payload projection carrying
   the `group` field.
3. **R134 / `c1dd5f1`, minor** — it named `elideToWidth`/`footerLegendWithin` as
   the seams the degradation runs through. The added code budgets with
   `stringWidth` and only *mirrors* `footerLegendWithin` (named in its comment);
   it calls neither.

## Report paths cited above (resolve at this commit)

- [`docs/reports/phase4b-tier1-suite/README.md`](./phase4b-tier1-suite/README.md)
- [`docs/reports/phase4b-final-suite/README.md`](./phase4b-final-suite/README.md)
- [`docs/reports/phase4b-stability10/README.md`](./phase4b-stability10/README.md)
- [`docs/reports/phase4b-tier2-decision.md`](./phase4b-tier2-decision.md)
