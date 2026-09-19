# Phase 4b — manual session groups, and the interactive scroll cue

## What the operator gets

Two things they can see and use:

1. **Sessions organised into groups they chose** — `sprint work`, `tooling maintenance`,
   `ongoing queries` — created and renamed and deleted from `,`, picked in the create dialog,
   changed from `i`, collapsible with a count on every header, remembered across restarts. The
   cwd-derived "workspace" grouping goes away completely.
2. **A preview that tells them where they are when scrolled back**, and a footer that admits the
   scroll keys exist. Plus the fix for the key that silently throws the scroll position away.

The second is small and lands first, so the phase is deliverable from its third task onward. The
first is the body of the phase.

## Why now

**`SPEC.md` already describes manual groups and the code still does the old thing.** The operator
amended the spec completely before Phase 4 ran — §4's `groups` table and `sessions.group_id`
(`SPEC.md:295,331`), §11's grouping bullets (`:1275-1312`), §11.5's unstaged group section
(`:1821-1825`), `DECK_SESSION_GROUP` (`:462`), the hook payload `group` (`:1177`) — and it now
contains **no** mention of `sessions.workspace`, `store.DefaultWorkspace`, `[ui] group_by_workspace`
or `DECK_SESSION_WORKSPACE`. Phase 4 then declined the tier on wall clock
(`docs/reports/phase4-tier2-decision.md`). So `[ui] group_by_workspace` is currently an
**undocumented config key implementing behaviour the source of truth has deleted**, and
`internal/tui/group.go:36` still buckets on `store.DefaultWorkspace(session.CWD)` against a store at
`schemaV6` with no `groups` table.

#30 was split out of #29 (shipped in v0.2.2). #29 made the grid's scrollback start pre-populated
with up to 2000 rows of real tmux history, which turned "no position cue" from cosmetic into
practical: there is now a lot of room to be lost in and no feedback about being off the live bottom.

**#27** (multi-tenant profiles) is explicitly **not** in this phase.

## Ground rules

- **`SPEC.md` is the authority and is read-only to this job**, as are `prds/`, `ci/Dockerfile` and
  `ci/SPIKE.md`. Where this PRD and SPEC disagree, SPEC wins and the disagreement is a finding, not
  an edit. **There is no SPEC amendment left to make for this phase** — unusually for this repo,
  nothing here is blocked on the operator.

  ```sh
  BASE=$(git log --format=%H --diff-filter=A -1 -- prds/phase4b-manual-groups-and-scroll-cue.md)
  git log --oneline "$BASE..HEAD" -- SPEC.md prds/ ci/Dockerfile ci/SPIKE.md   # must print nothing
  ```

- **The migration is the one irreversible thing here.** `state.db` holds the operator's real
  session list. One transaction; never leave the tree at a commit where the schema bump has landed
  and the read path has not.
- **This is a removal phase as much as an addition phase.** A tree where the group model and the
  workspace model are both live has not met R129 — SPEC describes one grouping model, not two
  behind a switch.
- **The docker socket is for `ci/run.sh` and nothing else.** Never remove or kill containers by
  label — a previous ralphd job on this host SIGKILLed itself sweeping `label=ralphd.run`, which is
  its own engine container. No `docker prune`, no wildcard `rm`/`rmi`, and never signal by pattern:
  resolve a pid, verify it, signal that pid. Other runs live on this host.
- **The CI container has no agent binaries** — every scenario runs against the `cmd/fake-*` stubs on
  a fixture `PATH` (R114, phase 3k, established the vocabulary).

## Where the code is

- **Groups, store:** `internal/store/store.go` — `Session.Workspace`/`WorkspaceColumn` (`:200-215`),
  `scanSession` (`:539-562`), `DefaultWorkspace` (`:587-600`), the session `SELECT` list (`:606`),
  `schemaV1`'s `sessions` DDL (`:2219`), the `schemaV1..schemaV6` ladder (`:2155-2190`), and
  `getUIState`/`GetLastCreateAgent` (`:2320-2345`) — the precedent for both `collapsed_groups` and
  the remembered last group.
- **Groups, TUI:** `internal/tui/group.go` **in full** — `sessionWorkspace` (`:36`), `groupSessions`
  (`:100-114`), `groupingEnabled` (`:118-128`), `reorderPreservingGrouping`, and every navigation
  primitive (`visualOrder`, `visibleSessionIndices`, `nearestVisibleSelection`,
  `next`/`prevVisibleSelection`, `pageSelection`). Also `internal/tui/tui.go`'s `c` collapse handler
  (`:3470-3474`), `g`/`G` (`:3475-3491`), the detail dialog's key switch
  (`internal/tui/rename.go`'s `updateDetailView`, `:45-80`) and its footer (`tui.go:6567`).
- **Groups, config:** `internal/config/config.go:122-126,261-268`, `toml.go:33,156,202`,
  `toml_write.go:245`, `schema.go`'s `group_by_workspace` row,
  `internal/tui/settings.go:878,923-946`.
- **Groups, service:** `internal/service/session_context.go` — `DECK_SESSION_WORKSPACE` and the hook
  payload's `workspace` field.
- **Scroll cue:** `internal/tui/interactive.go` — `interactiveBodyLines` (`:531`, whose
  `lines, _ := m.interactiveGrid.RenderRows(...)` at `:532` discards the clamped offset) and
  `updateInteractive` (`:471-493`, the `m.interactiveScrollOffset = 0` at `:481` that precedes every
  forwarding decision); `internal/tui/interactive_scroll.go`'s `scrollInteractiveByLines` (`:39-52`)
  and `scrollInteractiveByPage`; `internal/tui/tui.go`'s `interactiveFooterLine` (`:4314-4320`) and
  the help/mouse tables (`:8002-8005`, `:8231`).

## Scope and materiality

**Tier 1 — the scroll cue (R133, R134, R135).** Acceptance requires all three. Small, first, and
every line it touches is under `internal/tui/interactive*.go` plus one footer builder — a file set
disjoint from everything Tier 2 rewrites.

**Tier 2 — manual groups (R128–R131).** Acceptance requires all four, and it is
**all-or-nothing**: the whole model lands (schema and migration, render, create/move, settings and
delete, features) or **it is not started**. There is no accepted middle. A half-migrated `state.db`
is blocking; an unstarted Tier 2 is a budget outcome — record it with the remaining budget that
decided it, as Phase 4 did, and say plainly that SPEC describes manual groups while the code still
groups by `workspace`. "Curing" that gap by editing `SPEC.md` is a protected-path violation and
**is** blocking.

R128–R131 keep the numbers Phase 4 gave them: they were written there, never implemented, and are
cited by number in that phase's records.

**Blocking:** a requirement's behaviour absent, contradicted, or asserted by a test that passes
against a product that does not have it; a migration left partially applied, or a schema bump
committed without its read path; the workspace model still reachable by any key, env var or code
path after R129 reports green; a narrowed suite sweep; a second implementation of session deletion
(R131); a test-only env knob in product code (R8); a protected path modified; a secret in the tree.

**Curable in place, never a replan:** citations, shas, counts, report prose, scenario titles,
doc-comment wording. Fix forward with a docs commit. *Phase 3j lost five of seven approaches to
this class being treated as blocking; it is not.*

**Advisory, never blocking:** an unstarted Tier 2 and the spec gap it leaves; a known flake class
recurring in the gate, reported with its log path; a stability run below 10/10 published honestly
with every failure named; style and naming opinions; any operator notification missed or late.

**Notifications.** The operator is AFK on Telegram; use the `notify` hat tool, in the same iteration
as the work. Send: anything that blocks work outright, each Tier 1 requirement as it lands, the
Tier 1 → Tier 2 decision with the budget behind it, **the migration landing** (the one irreversible
step), each Tier 2 requirement, every rejection and cure pass, the gate and stability results, and
one terminal summary. A missed or late send is cured by mentioning it in the next one — it is never
a success criterion and never a finding.

## Tier 1 — the interactive scroll cue (GH #30)

### R133 — the scrolled-back view says so, from the offset actually used

- While interactive and scrolled back, deck renders a cue stating **how far back the view is and
  that it is not live** — e.g. `⇡ 240 lines back` — with an ASCII fallback under `[ui] ascii` like
  every other glyph (`m.glyph`). Its home is the preview title or the interactive footer; the
  planner chooses, and `?` and the footer agree with what ships.
- **Render it from the CLAMPED offset, never from `m.interactiveScrollOffset`.**
  `interactiveBodyLines` (`interactive.go:532`) discards `RenderRows`' second return — the offset
  actually used after the grid clamps against its *real* scrollback length — while
  `scrollInteractiveByLines` clamps only to `[0, ScrollbackMaxLines]` (`interactive_scroll.go:44-50`).
  So the stored value can sit far past the real length while the view is pinned at the top, and a
  cue drawn from it reports a position the user is not at.
- **Heal the stored offset from the used one.** That fixes a second thing for free: the II-49 "has
  not repainted" notice is gated on `offset == 0` (`interactive.go:547`) and is suppressed today
  whenever the stored offset is stale-high.
- At the **top of the scrollback** — the clamp bit — the cue says so; 2000 lines is a boundary users
  now reach. At offset 0 there is no cue at all: live is the unmarked default.
- **The transport asymmetry is correct behaviour, not a gap to work around.** #29 put the history
  seed on the *entry* seed only, so under `interactive_transport = capture`, and after a pipe
  displacement, the scrollback is empty and the cue correctly never appears. No test may assume
  otherwise (`interactive.EntrySeedHistoryLines` owns that decision).
- **Success:** a test that scrolls past the real scrollback length and asserts the cue reports the
  clamped position — it must fail if the cue is wired to `m.interactiveScrollOffset`; top-of-
  scrollback distinguishable from ordinary scrolled-back; offset 0 renders no cue; the II-49 notice
  reappears once the offset is healed.

### R134 — the interactive footer advertises the scroll keys

- `interactiveFooterLine` (`tui.go:4314-4320`) gains them. It renders exactly one line today —
  `keystrokes forward to the live pane · Ctrl+Q leave interactive mode` — and its own doc comment
  states the rule it follows: never name a key interactive mode does not itself bind.
  **Shift+PgUp/PgDn IS bound by interactive mode**, so it already qualifies; the omission is an
  oversight, not a policy.
- Room is tight at 80 columns. Degrade the way every other footer here does
  (`elideToWidth`/`footerLegendWithin` are the precedents) rather than overflowing the frame, and
  **`Ctrl+Q` is never the part that drops** — it is the only way out. Showing the scroll hint only
  while scrolled back is an acceptable answer; a line that exceeds the frame at the golden 80×24 is
  not.
- Help (`:8002-8005`) and the mouse table (`:8231`) already document the keys; they must not
  disagree with the footer afterwards. `cmd/deck/main_test.go` and `internal/tui/tui_test.go` pin
  help/footer substrings — update them in the same commit.
- **Success:** a footer test at 80 columns asserting the frame is not exceeded and `Ctrl+Q` survives
  every degradation step; a test naming the scroll keys; the golden 80×24 frame still green.

### R135 — a keystroke that forwards nothing must not snap the view to the live bottom

- `updateInteractive` zeroes `m.interactiveScrollOffset` at `interactive.go:481`, **before** the
  `m.interactiveDispatcher == nil` guard and before `interactiveNamedKey` /
  `interactiveLiteralPayload` decide whether the key produces any bytes. So a key that forwards
  nothing still destroys the scroll position. The snap-to-bottom rule itself is right and stays
  (PRD II-51, and it is the ordinary terminal convention) — it must simply become **conditional on
  something actually being forwarded**.
- The harm this removes: a terminal delivering Shift+PgUp as a plain `tea.KeyPgUp` both forwards
  `PageUp` to the agent *and* wipes the scroll position, so the one gesture the user meant as
  "scroll back" is the gesture that cancels it. Pre-exists #29; #29 did not fix it.
- `Ctrl+Q` is unaffected — it returns before this line.
- **Success:** a test driving a key matching neither helper into a scrolled-back model, asserting the
  offset is unchanged (it must fail against today's code), plus one asserting a key that *does*
  forward still snaps to 0.

## Tier 2 — manual session groups (GH #25)

Started only once Tier 1 is green, and all-or-nothing.

### R128 — the group model replaces the workspace label

- `state.db` gains SPEC §4's `groups` table and `sessions.group_id` as `schemaV7` on the existing
  ladder. `sessions.workspace`, `store.DefaultWorkspace`, `Session.Workspace` and
  `Session.WorkspaceColumn` are **removed**, not deprecated in place. `group_id` carries **no**
  foreign key deliberately: R131's delete flow chooses between two branches for a group's members,
  and `ON DELETE SET NULL` would pre-empt one of them silently.
- **`default` is `group_id IS NULL`.** There is no row for it, which is what makes "always exists /
  always last / never renamed / never deleted" structural instead of four validation rules — and a
  `group_id` that no longer resolves (another client deleted the group between load and render)
  reads as `default` rather than vanishing or crashing.
- **Membership is by id, not name**, so a rename is one row update and carries every member for
  free.
- **Migration: every existing session lands in `default`** and the group list starts empty. A
  deliberate clean break — the operator's words were "let that one go". Seeding a group per distinct
  old `workspace` value would recreate exactly the cwd-derived grouping this replaces. One
  transaction.
- Name rules: trimmed, non-empty, no control characters, unique case-insensitively
  (`COLLATE NOCASE`), `default` in any case reserved, length cap **32**. The cap is a sanity bound
  and cannot be a fitting bound: §11.2 clamps `sidebar_width` to `[24, width − 40]`, so the content
  floor is 20 cells, and `▾ ` + name + two-space gap + `(999)` leaves 11 cells for a name there.
  **Elision does the real work** (R129); 32 is long enough for every name in the operator's own
  examples ("tooling maintenance" is 19) and short enough to stay legible in the `,` editor and the
  create modal's cycling field. Put the derivation in a comment beside the cap.
- `DECK_SESSION_WORKSPACE` becomes `DECK_SESSION_GROUP` (the group's name, empty for `default`) and
  §10's payload field `workspace` becomes `group`, both in `internal/service/session_context.go`.
  **No alias for either** — a hook reading the old name sees nothing set, the operator's explicit
  decision.
- Default session names keep their `<basename-of-cwd>-<MMDD-HHMM>` shape (SPEC §200). That
  computation stays; it stops being called "workspace" and stops implying grouping.
- **Success:** a migration test over a fixture DB built by the previous schema with several distinct
  `workspace` values, asserting every row reads `default` afterwards and the file still opens; an
  atomicity test (a forced mid-way failure leaves the old schema intact and readable);
  case-insensitive uniqueness, reserved-name and name-cap tests; a pane-env test for the renamed
  variable and a payload test for the renamed field; and a guard asserting **no `workspace`
  identifier survives** in `internal/store`, `internal/tui` or `internal/service`
  (`internal/tui/registry_guard_test.go` is the black-box idiom).

### R129 — the sidebar renders manual groups

- Group order is **alphabetical, case-insensitive, `default` always last** regardless of where its
  name would sort. Rows *within* a group keep `[ui] sort_order`
  (`attention`/`created`/`activity`/`name`), unchanged.
- **Everything in this table is removed, not switched off:**

  | Removed | Sites |
  | --- | --- |
  | `[ui] group_by_workspace` + `DECK_GROUP_BY_WORKSPACE` | `config.go:122-126,261-268`, `toml.go:33,156,202`, `toml_write.go:245`, `schema.go`'s row, `settings.go:878,923-946`, `group.go:118-128` |
  | flat (no-header) sidebar mode, R35 | `group.go`'s `groupingEnabled`/`visualOrder` flat branch, and its own page-size and elision maths |
  | group order follows attention, R53 | `reorderPreservingGrouping` (`group.go`) |
  | the header's representative-`cwd` suffix | `groupHeaderText` — a manual group's members span any number of directories, so one member's cwd is noise |

  **Grouping is unconditional after this.** Groups are the user's own, so there is nothing to switch
  off; a session with no group sits under `default`. Removing the key does not break an existing
  `config.toml`: the reader skips unrecognised keys and `toml_write.go` leaves their lines untouched,
  so a stale key is inert and preserved.
- **Every header carries its member count, `(0)` included**, and a defined-but-empty group still
  renders — it is how the user sees the group exists and where to collapse it. Header text is
  `▾ <name>  (<n>)` with an ASCII fallback. **The name elides; the count and the chevron never do.**
- Under an active filter (§11.10) only groups with a match render, and the count shown is the
  matching count. `/` matches **name, group and `cwd`** — it matches "name, workspace or cwd" today.
- **Collapse persists** in `ui_state` under `collapsed_groups` (SPEC §4, `:337`) keyed by **group
  id**; today it is an in-memory `map[string]bool` keyed by a workspace string. `c` toggles the
  selected row's group and a header click still does the same (§11.8) — the hit-test key becomes the
  id. **Only groups collapse**; a session row stays the two-line block it is.
- Every navigation primitive keeps its contract — one keypress moves exactly one visual row,
  selection never lands on a hidden row — keyed on group id. That includes `g`/`G`
  (`tui.go:3475-3491`), which resolve through `visibleSessionIndices`.
- **Accepted consequence, stated deliberately:** alphabetical order means a `waiting` session can sit
  below the fold in a way R53's ordering prevented. `space` (the attention walk) and the collapsed
  strip's attention count remain the guarantee it is reachable — the same argument SPEC §11 already
  makes for offering a non-`attention` `sort_order`. Do not add a compensating re-order.
- **Success:** order tests including a group named `zzz` and one named `aaa` with `default` still
  last; counts for populated and `(0)` groups; a header-elision test at the 24-column sidebar floor
  asserting the count survives; collapse surviving a Model rebuild from `ui_state`; the
  navigation-parity tests rewritten to prove grouped behaviour alone; a dangling `group_id`
  rendering under `default`; a filter test matching a group name.

### R130 — membership is set where the session is

- The create modal gains a `Group` field cycling the available groups exactly as `Agent` cycles
  kinds — alphabetical, `default` last — defaulting to the **last group created into**, persisted in
  `ui_state` beside the existing last-used-agent key. `GetLastCreateAgent` /
  `lastCreateAgentUIStateKey` (`store.go:2340`) is the precedent, **including its fallback when the
  remembered value is gone**: a deleted group degrades to `default`, not to an empty field.
- The `i` detail dialog shows the session's group and opens a picker to move it. **The key is `g`.**
  Collision-checked: `updateDetailView` (`rename.go:45-80`) handles only `q`, `ctrl+c`, `i`, `r`, `l`
  plus the shared dialog contract, and list-level `g`/`G` (top/bottom, `tui.go:3475-3491`) are both
  guarded by `!m.detail`, so there is no dispatch conflict. The detail footer (`tui.go:6567`, today
  `r renames · l edits launch inputs · i or Esc closes detail`) and `?` (`:8068`) both name it.
- **The marked set is not extended to moves.** `x` and `dd` remain the only batch verbs (SPEC §11,
  R28) — a scope line the operator drew deliberately, not an omission to fill in helpfully.
- **Success:** a create-into-a-group scenario asserting the persisted `group_id`; the remembered
  default surviving a restart and falling back when its group is gone; a move via `i` asserting
  exactly one row changed; a footer/help parity test naming `g`.

### R131 — the group list is edited in settings

- Settings gains a groups section: `n` creates (inline validation errors from R128's rules), `r`
  renames, `d` deletes.
- **It is not staged.** Today `,` edits scalar keys against a staged `config.toml` with an explicit
  save; groups live in `state.db`, so edits apply immediately, the section must read as applying
  immediately, and **`esc` must not offer to discard what it cannot discard** (SPEC §11.5,
  `:1821-1825`). `settings_group_by_staging_test.go` is today's proof for the very key R129 deletes —
  it goes with the key.
- Deleting a non-empty group prompts for one of two branches:

  ```
  Delete group "tooling maintenance" (3 sessions)?

    (m) move all 3 to default
    (d) delete all 3 sessions
  ```

  `m` → `UPDATE sessions SET group_id = NULL`, group row deleted, nothing destructive. `d` → the
  **existing** §9.2 `dd` batch path, reused and not reimplemented: its confirm dialog naming what
  survives (cwd and conversation never touched), its non-default transcript-purge offer, its
  tombstones, its `DECK_DELETE_GRACE_MS` window and **one** `u` restoring the whole batch. The group
  row goes once the batch commits. An empty group prompts nothing. `default` offers no delete.
- **No second implementation of deletion exists after this.** If the `dd` batch path needs a seam to
  be callable from settings, add the seam; do not copy the deletion. SPEC §11.5's lifecycle
  carve-out is written narrowly on purpose and this requirement is the whole of it.
- Multiple deck clients share one `state.db`, so a group edit in one is picked up by the others on
  their next reload; R128's dangling-`group_id` rule is what makes that safe.
- **Success:** both delete branches as feature scenarios, the destructive one asserting tombstones
  and that one `u` restores the whole batch; an empty-group delete with no prompt; a rename carrying
  every member; `esc` offering no discard prompt in that section; a guard that settings' delete calls
  the same service `dd` does.

## Ordering

1. **R133 → R134 → R135.** R133 establishes the clamped-offset plumbing R134's conditional hint
   reads; R135 is independent but cheapest last (a two-line move and one test).
2. **The Tier 1 → Tier 2 decision**, with the budget behind it.
3. **R128** — schema, migration, and the removals in store/service/config. The removals belong with
   the model: leaving `Session.Workspace` standing while the TUI is rewritten produces the
   two-models-live tree R129 forbids.
4. **R129** — the sidebar render and every navigation primitive. The widest change in the phase.
5. **R130** — create field, then the `i` move picker.
6. **R131** — the settings section and the delete branches, last because the destructive branch
   reuses machinery that must already be intact.

## Green when

- `go build ./...` and `go vet ./...` clean, and `gofmt -l` clean on every file this run touches.
  (It is *not* clean on the tree today — `internal/theme/quantize_test.go` and three files under
  `.spike-preview/` have pre-existing drift. Leave them; naming them in the report is enough and
  their presence is not a finding.)
- **`ci/run.sh go test -p=1 -count=1 ./...` green** at the final code sha — whole suite, in the
  container, no narrowed package list and no `-run` filter. It takes about seven minutes and that is
  the price of the gate meaning something.
- The features package run for stability the way phase 3k did, with **every** failure named and its
  log path published even when the headline is 10/10. Two flake classes are known open — the
  transient-`starting` assertion and a `SIGWINCH` exact-count assertion — and instances of those are
  advisory, reported with evidence, not chased.
- The protected-path audit prints nothing.
- Every requirement has at least one test that fails if the behaviour is removed.
- **The record:** `docs/reports/phase4b.md` says per requirement what shipped, the commits and the
  tests that prove it — and for anything that did not, what is missing and why;
  `docs/reports/phase4b-findings.md` carries what this run found and chose not to fix, each with a
  `file:line`; `docs/DELIVERY-LOG.md` gains this phase's row. Written **once**, at the final code
  sha (the last commit touching `*.go` or `*.feature`); a docs-only tail commit does not invalidate
  the gates and is not re-verified. Back-filling the log's four missing rows (3c–3k, 4) is not this
  run's job.

## Non-goals

- **#27, multi-tenant profiles.** Not partially, not "the `DECK_HOME` groundwork".
- **Phase 5 notifications, Phase 6 shell state, Phase 7 search and health.** `internal/notify`,
  `internal/search` and `internal/unit` are one-line `doc.go` placeholders and stay that way.
- **A config knob to make any of this optional.** SPEC *removed* `group_by_workspace` rather than
  adding to it; a `[ui] groups` switch would reintroduce the two-models tree R129 forbids.
- **Seeding groups from the old `workspace` values.** The operator asked for a clean break.
- **Extending the marked set** beyond `x` and `dd`. **Per-row collapse** — only groups collapse.
- **A second deletion implementation** — R131 reuses the `dd` batch path or adds a seam to it.
- **Reinstating attention-ranked group order** in any form, including "only when a group holds a
  `waiting` session".
- **Changing #29's transport asymmetry.** The history seed stays on the entry seed; the 200 ms
  reseed loops stay visible-only. That was measured.

## For the planner

- **Tier 2's risk is wall clock, not difficulty.** Phase 4 measured ~3.1 iterations and ~24 minutes
  per task on bounded work and estimated this tier at 8–12 hours on top of a multi-hour tail
  (`docs/reports/phase4-tier2-decision.md`); nothing has changed that. Plan the four requirements as
  a few substantial tasks rather than many small ones, and take the tier decision honestly and
  early. **If the budget is thin, stop and write the record.** An unstarted Tier 2 costs nothing; a
  half-migrated `state.db` costs the operator their session list.
- **R129 is the widest change.** `group.go` is rewritten and the identifiers R128 removes are
  referenced across roughly 28 Go files. Drive the removal with the compiler — delete the store
  field and let `go build` enumerate the call sites — rather than site-by-site from a grep, which is
  how a flat-mode branch survives in one forgotten navigation primitive.
- **These files assert today's behaviour and need re-aiming.** A starting point, not a closed set —
  the compiler and the suite are the authority:
  - `features/`: `attention_sort`, `sort_order`, `filter`, `create_session`, `settings`, `mouse`,
    `themes`, `harness`, `panel_background_rectangle`
  - `internal/tui/`: `group_test.go`, `group_visual_order_test.go`, `flat_sidebar_test.go`,
    `navigation_parity_test.go`, `attention_next_test.go`, `sidebar_stripe_test.go`,
    `settings_group_by_staging_test.go`, `sort_order_render_test.go`, `mouse_test.go`,
    `panel_test.go`, `filter_test.go`, `mark_test.go`, `main_view_theme_test.go`,
    `unarchive_test.go`, `i1_marked_nav_test.go`
  - `internal/config/config_test.go`, `internal/store/store_test.go`,
    `internal/service/session_context_test.go`; Tier 1 additionally pins footer/help substrings in
    `cmd/deck/main_test.go` and `internal/tui/tui_test.go`.

  A test that asserted flat mode or attention-ranked group order is **deleted with the behaviour**,
  not rewritten to assert the opposite. `navigation_parity_test.go` is the exception worth care: its
  *grouped* half is what will catch a hidden row becoming selectable. Keep that half.
- **Do not fix the cue by changing the clamp.** Both clamps are deliberate and documented
  (`interactive_scroll.go:18-38` explains why nothing on that side inspects the length). R133 reads
  the used offset and heals the stored one; it does not move the clamping.
- **Tier 1 is genuinely independent.** Three requirements, one file set, no store and no config. If
  Tier 2 is declined, Tier 1 still shipped something the operator uses daily — which is why it is
  first.
