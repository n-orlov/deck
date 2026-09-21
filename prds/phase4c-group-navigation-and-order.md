# Phase 4c — group navigation you can trust

## What the operator gets

The groups they just got, made usable by keyboard:

1. **The selection is always on screen.** Today `PgDn`, `G`, a fold or a filter edit can leave the
   selected row outside the sidebar's window, so the list renders with nothing visibly selected and
   the next keystroke acts on a session the operator cannot see.
2. **Any group can be folded and unfolded, by keyboard, including an empty one.** Today `c` is
   effectively fold-only: it walks down the sidebar folding every group in turn, and a folded group
   cannot be named again because it has no visible row to select.
3. **The header click works while a preview is live**, not only in list mode.
4. **`default` can sit first**, for an operator whose ungrouped sessions are the ones they work in.

All four were found by the operator in field testing v0.2.2-phase4b and filed as **#31, #32, #33,
#34**. The release is **held** on this phase — that was the operator's call on 2026-09-21, made after
reading all four: manual groups are not finished while the group you most want to reopen is the one
group you cannot address.

## Why now

**Phase 4b shipped the group model and left its keyboard surface incomplete.** #25 and #30 are closed
and `schemaV7` is installed on the operator's real machine, so there is no "revert and rethink"
option: the model is live and the gaps are what the operator hits every day. Each of the four is a
few hours of work and one of them (#32) is a genuine design hole rather than an oversight —
`setGroupCollapsed`'s cursor eviction means the fold key cannot express "reopen *that* group" at all.

**`SPEC.md` has already been amended for this phase** (`02c5d11`, an operator commit): §11 now states
group headers are cursor stops, that folding never evicts the cursor, that every session-scoped key
is inert on a header, that `default_group_first` exists, and — new — that the viewport follows the
cursor on **every** selection move. §11.3's keymap gained `c` and `←`/`→`; it had never listed `c` at
all, which was itself a §11.3 defect by that section's own two-lists rule. §6.5's `[ui]` key list and
§13.5's feature inventory follow.

So, as in Phase 4b: **nothing here is blocked on the operator.** Where this PRD and SPEC disagree,
SPEC wins and the disagreement is a finding.

**#27** (multi-tenant profiles) is explicitly **not** in this phase.

## Ground rules

- **`SPEC.md` is the authority and is read-only to this job**, as are `prds/`, `ci/Dockerfile` and
  `ci/SPIKE.md`.

  ```sh
  BASE=$(git log --format=%H --diff-filter=A -1 -- prds/phase4c-group-navigation-and-order.md)
  git log --oneline "$BASE..HEAD" -- SPEC.md prds/ ci/Dockerfile ci/SPIKE.md   # must print nothing
  ```

  The SPEC amendment this phase depends on (`02c5d11`) lands *before* `$BASE` and is therefore
  outside the audit range by construction. It is not this run's commit and must not be re-made,
  extended or "cured".
- **No schema change, no migration.** `schemaV7` is already on the operator's live database. Nothing
  in this phase touches `internal/store`'s migration ladder, and a `schemaV8` appearing in the tree
  is blocking. `~/.local/share/deck/state.db` is the operator's real session list — the job never
  opens it, reads it or backs it up; every test uses its own temp database.
- **This is a defect phase, so a test that passes against today's code has proven nothing.** Every
  requirement below names the assertion that must **fail before the fix and pass after**. A
  requirement whose test is green on the unfixed tree has not been delivered, whatever the report
  says.
- **The docker socket is for `ci/run.sh` and nothing else.** Never remove or kill containers by
  label — a previous ralphd job on this host SIGKILLed itself sweeping `label=ralphd.run`, which is
  its own engine container. No `docker prune`, no wildcard `rm`/`rmi`, and never signal by pattern:
  resolve a pid, verify it, signal that pid. Other runs live on this host.
- **The CI container has no agent binaries** — every scenario runs against the `cmd/fake-*` stubs on
  a fixture `PATH`.

## Where the code is

- **Viewport follow (R136):** `internal/tui/tui.go` — `scrollSessionIntoView` (`:5547`, the helper
  that already exists and is deliberately unwired), `clampSidebarScroll` (`:5591`),
  `sidebarRowsPerPage` (`:4689`), and the seven binding sites that move `m.selected` without it:
  `up`/`k` (`:3501`), `down`/`j` (`:3505`), `pgup`/`pgdown` (`:3509-3517`), `space` (`:3900`), `c`
  (`:3927`), `g`/`G` (`:3935-3951`); plus `internal/tui/filter.go:134,154`. `m.sidebarScroll` is
  declared at `:983` and moved by exactly one gesture today — `scrollSidebar`
  (`internal/tui/mouse.go:193-202`).
- **Header cursor (R137):** `internal/tui/group.go` **in full** — `setGroupCollapsed` (`:214-224`,
  whose trailing `nearestVisibleSelection` call is the defect), `nearestVisibleSelection` (`:289`),
  `visualOrder` (`:258`), `visibleSessionIndices` (`:269`), `next`/`prevVisibleSelection`
  (`:321-364`), `pageSelection` (`:373`), `isSessionVisible`, `groupSessions` (`:149`),
  `sessionGroupID`. Then `internal/tui/tui.go`'s `c` handler (`:3909-3931`), `g`/`G`
  (`:3935-3951`), `sidebarEntries` (`:5448`, which already tags every header with `groupID`),
  the 41 `m.sessions[m.selected]` sites in `tui.go` and the rest in `rename.go` (9),
  `env_editor.go` (4), `launch_inputs.go` (3), `interactive.go`, `interactive_displacement.go`;
  the detail footer (`tui.go:6567`) and `?` help.
- **Interactive header click (R138):** `internal/tui/tui.go:4018-4022` (the press rescue that tests
  `hitTargetRow` and nothing else), `:4032-4036` (`beginInteractiveSelection`, which must stay
  downstream of it), `internal/tui/mouse.go`'s `handleMousePress` (`:239-244`, the list-mode path
  being matched), `hitTest` (`:57`) and `restoreFromCollapsedStrip`.
- **Group order (R139):** `internal/tui/group.go`'s `groupSortsBefore` (`:198-204`) and its single
  call site in `groupSessions` (`:174-176`); `internal/config/config.go:122-134,256,308-309`
  (`SortOrder`/`PreviewFit` are the two shapes to copy), `internal/config/toml.go:151,197`,
  `toml_write.go:243`, `schema.go:453` (`ui.preview_fit`'s row is the bool precedent);
  `internal/tui/settings.go`'s `settingsApplyLiveFields` (`:436-490`, whose `resortSessionsLive`
  call at `:488` is the live-apply seam) and `resortSessionsLive` (`tui.go:5521`, which preserves the
  selected session **by id, never by index**).

## Scope and materiality

**Tier 1 — R136, R138, R139.** Three independent fixes with disjoint file sets, none of which touches
the cursor model. Acceptance requires all three. Each is separately shippable and each is something
the operator uses the day it lands, so this tier is where the phase becomes deliverable.

**Tier 2 — R137.** The header cursor. One requirement, and the widest change in the phase: it
rewrites `group.go`'s navigation primitives and changes what `m.selected` is allowed to mean. It is
**all-or-nothing** — a tree where `↑`/`↓` sometimes land on a header and some session-scoped key
still acts on a stale row is worse than today, because today's behaviour is at least consistent. If
the budget is thin, **stop and write the record**: an unstarted R137 leaves #32 open and the release
held, which is a budget outcome the operator can act on. A half-built cursor is not.

**Blocking:** a requirement's behaviour absent, contradicted, or asserted only by a test that also
passes against the unfixed tree; a session-scoped key acting on a session while the cursor is on a
header (R137); the selection reachable off-screen by any binding in R136's table; a schema change or
any access to the operator's real `state.db`; a narrowed suite sweep; a test-only env knob in product
code (R8); a protected path modified; a secret in the tree.

**Curable in place, never a replan:** citations, shas, counts, report prose, scenario titles,
doc-comment wording, help/footer phrasing that names the right key in the wrong words. Fix forward
with a docs commit. *Phase 3j lost five of seven approaches to this class being treated as blocking;
it is not. Phase 4b's own review rejected approach 1 on three findings, all cured in one pass.*

**Advisory, never blocking:** an unstarted Tier 2 and the open #32 it leaves; a known flake class
recurring in the gate, reported with its log path — the transient-`starting` assertion and the
`SIGWINCH` exact-count assertion are both known open; a stability run below 10/10 published honestly
with every failure named; style and naming opinions; any operator notification missed or late.

**Notifications.** The operator is AFK on Telegram; use the `notify` hat tool, in the same iteration
as the work. Send: anything that blocks work outright, each requirement as it lands, the Tier 1 →
Tier 2 decision with the budget behind it, every rejection and cure pass, the gate and stability
results, and one terminal summary. A missed or late send is cured by mentioning it in the next one —
it is never a success criterion and never a finding.

## Tier 1

### R136 — the viewport follows the cursor (GH #31)

- Every key that moves the selection leaves the selected row **fully inside the sidebar's visible
  window**, keeping **one whole session row (two entry lines) of context** beyond it where the list
  allows, and sitting flush at the list's own ends where it does not. SPEC §11 now states this as a
  general rule; it previously required scroll-into-view only for a re-sort and a new session, which
  is exactly the pair the code implements.
- The bindings that must each be fixed and each be tested — this table is the checklist:

  | binding | site |
  | --- | --- |
  | `up`/`k`, `down`/`j` | `tui.go:3501`, `:3505` |
  | `pgup`/`pgdown` | `tui.go:3509-3517` |
  | `g`/`G` | `tui.go:3935-3951` |
  | `space` (attention walk) | `tui.go:3900` |
  | `c` (fold relocating the cursor) | `tui.go:3927` |
  | `/` query edits | `filter.go:134,154` |

- **Prefer one seam over seven call sites.** A single `setSelection`-style mutator that assigns and
  then follows is both smaller and the natural place R137 hooks into; seven independent calls is how
  the eighth binding gets added without one.
- **The wheel keeps its documented independence** (§11.8: scrolls without selecting). It may drift
  away from the cursor; the *next* selection move snaps back. Do not make the wheel move the
  selection, and do not preserve the wheel's offset against a selection move.
- **The margin is in entry lines, not rows.** `sidebarRowLines` emits two `sidebarLineRow` entries
  per session and `scrollSessionIntoView` already scans for the `start`/`end` pair (`:5565-5572`).
  At the supported 80×24 floor `contentHeight` may not have room for the margin at all — degrade to
  flush rather than fighting the clamp, and never produce a negative offset or blank space below the
  last line (`clampSidebarScroll` is the existing guard, `:5591`).
- **Success:** a test per binding in the table above, on a list taller than the sidebar, asserting
  the selected row's entry lines fall inside `[sidebarScroll, sidebarScroll+contentHeight)` — **each
  must fail against today's code**; a wheel-away-then-`↓` test asserting the cursor returns to view;
  an 80×24 test asserting flush degradation with no blank tail.

### R138 — the header click works while a preview is live (GH #33)

- A left press on a group header toggles that group's collapse **in interactive mode exactly as it
  does in list mode**, persisted to `ui_state.collapsed_groups` the same way. §11.8's mouse table
  binds the header click with no mode qualification, and its own prose already says a click over the
  *sidebar* acts while interactive mode is live — so this is a defect against existing SPEC, not new
  behaviour, and needed no amendment.
- Today `tui.go:4018-4022` rescues a press that hit-tests to `hitTargetRow` and nothing else, so a
  header press falls through to `beginInteractiveSelection`, reports false outside the preview's
  content box, and dies as a no-op. `hitTargetCollapsedStrip` is dead the same way and is fixed in
  the same change (→ `restoreFromCollapsedStrip`).
- **The pane keeps the keyboard.** Folding a group is a list-view operation: it must not leave
  interactive mode, re-target the preview, or resize any window. Folding the group that *contains*
  the interactive session is allowed and simply hides its row.
- **Route both modes through one resolver** rather than duplicating the target switch. Today's
  duplication is precisely how the header case went missing.
- The rescue stays **upstream** of `beginInteractiveSelection` (`:4032-4036`), and a press over the
  preview's content box still begins drag-to-copy, unchanged. The wheel branch above
  (`:3998-4008`) is out of scope: whether the wheel should scroll the sidebar while interactive is a
  separate question and must not be folded in here.
- **Success:** a test asserting a header press while `m.interactive` toggles collapse, returns the
  persist command, and leaves `m.interactive` true with no geometry call — it must fail today; a
  collapsed-strip press while interactive restoring the layout mode; a parity test asserting the
  header press has the *same* effect in both modes, so the divergence cannot silently return; the
  preview-drag path still green.

### R139 — `default` can sort first (GH #34)

- `[ui] default_group_first` (bool, **default `false`** — today's order is preserved for anyone who
  does not set it) renders the default group first instead of last. The remaining groups stay
  alphabetical, case-insensitive, either way, and rows within a group keep `[ui] sort_order`.
- Plumbed as a `[ui]` bool exactly as `preview_fit` is: `config.go`'s field and default,
  `toml.go`'s read and write cases, `toml_write.go`'s key case, `schema.go`'s row, and the settings
  takeover field that row generates. **Honoured live** through `settingsApplyLiveFields` →
  `resortSessionsLive`, which already preserves the selected session by id — the right behaviour
  here, since the pivot moves every group's position at once.
- `groupSortsBefore` (`group.go:198-204`) takes the flag as a parameter, or `groupSessions` seeds a
  comparator from settings. **Keep `groupSortsBefore` ignorant of `m.sessions`, attention and
  `sort_order`** — it compares two group names and that is all it should ever see. `sort.SliceStable`
  stays stable: bucket order within a group is `m.sessions`' own relative order.
- **Do not compare display names.** The default group's key is `""` and its *label* is `"default"`
  (`sessionGroupLabel`, `group.go:97-103`); a user-created group literally named `default` is a
  different thing with a real id, and a comparator switched to labels collides the two.
- Fold state is keyed by `groupID` (`group.go:206`) and must survive the toggle untouched.
- **Success:** order tests with the flag both ways over groups named `aaa`, `zzz` and `default`,
  plus a user-created group named `Default` proving the two do not collide; a live-apply test
  asserting the sidebar reorders and the same session stays selected; a round-trip through
  `config.toml` (write, re-read, unknown keys preserved); fold state intact across the toggle.

## Tier 2

### R137 — a group header is a cursor stop (GH #25 follow-on, #32)

Started only once Tier 1 is green. All-or-nothing.

- **The defect, precisely.** `setGroupCollapsed` (`group.go:214-224`) ends with
  `m.selected = m.nearestVisibleSelection(m.selected)`, and `nearestVisibleSelection` (`:289`)
  searches **forward first**. So folding a group ejects the cursor into the *next* group down, the
  same key folds that one, and `c` becomes a walk to the bottom of the list. When the last group
  folds, both searches fail and the function returns `0` — which is why exactly one arbitrary group
  (whoever owns `m.sessions[0]`) could be reopened at all. A folded group has no selectable row
  (`sidebarEntries` skips its sessions at `tui.go:5508-5510`; `visibleSessionIndices` filters them
  out), and a defined-but-empty group has no session to resolve through **ever**, so it is
  unreachable by keyboard whether folded or not.
- **The cursor addresses either a session row or a group header.** `↑`/`↓`/`k`/`j`/`PgUp`/`PgDn`/
  `g`/`G` land on headers as well as rows, one keypress moving exactly one visual stop. `c` folds or
  unfolds **the header under the cursor**; `←` folds and `→` unfolds explicitly (both are unbound in
  list mode today — verified: the only `left`/`right` cases live in `dialog_contract.go`,
  `settings.go` and `theme_picker.go`). SPEC §11.3's keymap already names all three.
- **Folding never evicts the cursor from the group it folded.** From a header, the cursor stays put.
  From a session row, the cursor moves to **that group's own header** — not to the next group — so
  the same key immediately undoes it. Drop the trailing `nearestVisibleSelection` call for the header
  case; keep "selection never lands on a hidden row" intact for every other path.
- **Every session-scoped key is inert on a header, by one rule.** `↵`, `a`, `x`, `dd`, `r`, `R`, `i`,
  `e`, `P`, `p`, `s`, `Y`, `m`, `z`, `A`, `U`, `g` (the detail move) name no session and do nothing.
  Implement this as a single guard the bindings share, not a decision each of the 41+
  `m.sessions[m.selected]` sites makes for itself — per-site decisions are how partial behaviour
  creeps in, and one site that forgets is a blocking finding.
- **Keep `m.selected` as the derived session index** for the existing consumers rather than
  rewriting them, but make the header case *explicit* so a consumer cannot silently act on a stale
  session. The compiler should be the one that finds the sites: change the type so a bare index
  cannot be read without resolving the cursor kind.
- **Keyed by durable `groupID`, never by name** (`group.go:206-232`), so a rename preserves both the
  fold state and the cursor. `0` is the default group's sentinel — a value no `groups.id` row can
  hold. Under an active filter, `groupSessions` deliberately seeds no empty groups
  (`group.go:159-171`), so the cursor must not assume a header exists for every persisted group.
- **Do not persist the cursor.** Fold state persists (`ui_state.collapsed_groups`); where the cursor
  sat does not.
- Footer and `?` help must say what the cursor can do in each position. The help currently describes
  `c` as collapsing "the selected row's own group", which stops being true.
- **Success:** header stops reachable by `↑`/`↓` and by `g`/`G`; `c`, `←` and `→` folding and
  unfolding the header under the cursor in **both** directions, for a populated group, `default`, and
  a defined-but-empty group; a test asserting repeated `c` no longer folds more than one group — it
  must fail today; folding from a row leaving the cursor on that group's header; every group
  individually unfoldable with **all** groups folded; a table-driven test asserting inertness for
  every session-scoped key listed above; fold state surviving a restart and a rename; and the
  existing "selection never lands on a hidden row" guarantee still asserted.

## Ordering

1. **R139** — smallest, fully independent, and it exercises the `[ui]` bool path end to end, which is
   the least surprising thing in the phase.
2. **R138** — one press-rescue branch and its parity test.
3. **R136** — the viewport follow, in its session-cursor form. Build it behind one `setSelection`
   seam.
4. **The Tier 1 → Tier 2 decision**, with the remaining budget behind it, recorded either way.
5. **R137** — the header cursor, which extends R136's follow to header lines. **This re-aims
   `scrollSessionIntoView` at a cursor rather than a session index: one signature and one lookup, not
   a redesign.** Do not defer R136 to avoid that rework — R136 is what the operator notices first and
   it ships on its own.

## Green when

- `go build ./...` and `go vet ./...` clean, and `gofmt -l` clean on every file this run touches.
  (It is *not* clean on the tree today — `internal/theme/quantize_test.go` and three files under
  `.spike-preview/` have pre-existing drift. Leave them; naming them in the report is enough and
  their presence is not a finding.)
- **`ci/run.sh go test -p=1 -count=1 ./...` green** at the final code sha — whole suite, in the
  container, no narrowed package list and no `-run` filter. It takes about seven minutes and that is
  the price of the gate meaning something.
- **`features/session_groups.feature` exists.** SPEC §13.5 has named it since Phase 4b and it was
  never written: group behaviour is spread across ten existing feature files (`attention_sort`,
  `filter`, `create_session`, `settings`, `kill_delete_undo`, `mouse`, `themes`, `harness`,
  `panel_background_*`) and nothing owns §11's grouping rules. This phase's own scenarios land there
  — keyboard fold/unfold, the header cursor, the order toggle, the header click in both modes —
  which satisfies §13.5's inventory without re-litigating where the existing ten put theirs. Moving
  scenarios out of those ten is **not** in scope.
- The features package run for stability the way Phase 4b did, with **every** failure named and its
  log path published even when the headline is 10/10. The two known flake classes are advisory,
  reported with evidence, not chased.
- The protected-path audit prints nothing.
- **Every requirement has at least one test that fails against the unfixed tree**, named in the
  report with the failure it produces there. This is the phase's central obligation: it is a defect
  phase, and Phase 4b's suite was green while all four of these defects were live.
- **The record:** `docs/reports/phase4c.md` says per requirement what shipped, the commits and the
  tests that prove it — and for anything that did not, what is missing and why;
  `docs/reports/phase4c-findings.md` carries what this run found and chose not to fix, each with a
  `file:line`; `docs/DELIVERY-LOG.md` gains this phase's row. Written **once**, at the final code sha
  (the last commit touching `*.go` or `*.feature`); a docs-only tail commit does not invalidate the
  gates and is not re-verified. Back-filling the log's missing rows is not this run's job.

## Non-goals

- **#27, multi-tenant profiles.** Not partially, not "the `DECK_HOME` groundwork".
- **Phase 5 notifications, Phase 6 shell state, Phase 7 search and health.** `internal/notify`,
  `internal/search` and `internal/unit` are one-line `doc.go` placeholders and stay that way.
- **Manual per-group ordering** — a `groups.sort_order` column with reorder keys. Considered for #34
  and declined in favour of the boolean; it is a schema change and this phase makes none. Do not
  build a "groundwork" column for it either.
- **Any schema change at all**, including a `schemaV8` that only adds an index.
- **Making the cursor persist**, or making the wheel move the selection.
- **Extending the marked set** to group operations. `x` and `dd` remain the only batch verbs.
- **Per-row collapse.** Only groups collapse.
- **Reinstating attention-ranked group order** in any form. `default_group_first` moves one stable
  pivot the user chose; it is not a re-rank and must not become one.
- **Moving existing group scenarios** out of the ten feature files that hold them today.
- **Touching `~/.local/share/deck/state.db`**, the operator's live database, for any reason.

## For the planner

- **R137 is the whole risk; the other three are close to mechanical.** Plan Tier 1 as three small
  tasks that can each land and be reported independently, then take the tier decision honestly. R137
  wants to be a few substantial tasks (cursor type + primitives; the uniform inertness guard;
  render/help/footer + scenarios), not many small ones, because a half-migrated cursor does not
  compile into anything shippable.
- **Drive R137's fan-out with the compiler, not with grep.** Change the type so `m.selected` cannot
  be read as a bare session index, then let `go build` enumerate the 41 sites in `tui.go` and the
  rest across `rename.go`, `env_editor.go`, `launch_inputs.go`, `interactive.go`,
  `interactive_displacement.go`. A grep-driven sweep is how one navigation primitive keeps the old
  behaviour.
- **These tests assert today's behaviour and need re-aiming.** A starting point, not a closed set —
  the compiler and the suite are the authority:
  - `internal/tui/`: `group_test.go`, `group_order_test.go`, `group_id_navigation_test.go`,
    `group_visual_order_test.go`, `navigation_parity_test.go`, `attention_next_test.go`,
    `empty_groups_render_test.go`, `group_move_test.go`, `sidebar_hierarchy_test.go`,
    `sidebar_stripe_test.go`, `mouse_test.go`, `mouse_interactive_retarget_test.go`,
    `filter_test.go`, `mark_test.go`, `i1_marked_nav_test.go`, `sort_order_live_apply_test.go`,
    `sort_order_render_test.go`
  - `internal/config/config_test.go` for R139's key
  - `features/`: `attention_sort`, `filter`, `mouse`, `settings`, `create_session`
  - `navigation_parity_test.go` is the one worth care: its guarantee that a hidden row never becomes
    selectable is exactly what R137 could break. Extend it to the header cursor rather than
    loosening it.
- **R136's margin has an interaction with R137 worth planning for**: once a header is a cursor stop,
  the follow must target header *lines*, and a header is one entry line while a row is two. Write
  R136's helper against "the cursor's entry-line span", even in Tier 1 where that span is always a
  row's two lines.
- **The known-flake classes are not this phase's to fix** and a recurrence is advisory. Do not spend
  iterations on the transient-`starting` or `SIGWINCH` assertions; report and move on.
- **Phase 4b's own rejection is the model to expect.** Review found three blocking findings against a
  green suite, all marked curable, and one cure pass cleared them. That is the harness working.
  Budget for one cure pass rather than treating a rejection as a failure.
