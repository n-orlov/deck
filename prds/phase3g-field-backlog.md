# Phase 3g — the field backlog

## Goal

Close the operator's GitHub backlog and Phase 3f's open findings. **Seventeen requirements,
R76–R92**, continuing 3f's numbering, every one of them either something the operator hit by
using the Phase 3f build or a defect Phase 3f found, root-caused and deliberately left standing.

There is no new subsystem here. There is one requirement that matters more than the rest:

**R76 makes deck repair a session it has wedged.** A row in a terminal status with a live pane
underneath it is unusable by every action — resume declines because nothing needs launching,
kill declines because the row already says stopped — and the only escape is a keystroke pair
the user has to know is an escape. GitHub #6 and #11 were both this state arrived at by
different routes. R74 and R75 closed the route; nothing repairs a row already in it, and one
such row is sitting in the operator's live `state.db` as this PRD is written. A phase that
ships the other fourteen requirements and not R76 has missed the point of the phase.

## Read this before anything else: the `SPEC.md` amendment has LANDED

**`SPEC.md` is the authoritative product spec and must not be modified by this job.** Where
this PRD and `SPEC.md` disagree, `SPEC.md` wins and the disagreement is a finding for
`docs/reports/phase3g-findings.md`.

Eight of the requirements below **contradicted `SPEC.md` as it stood**, and the correct
response to that would have been to refuse them. So the operator amended `SPEC.md` first, in
commit **`2eed8de`** (`spec: amend §7, §9.2, §9.3, §11.3, §11.4, §11.7 and add §11.10 ahead of
the field-backlog phase (operator)`), with the plan change in **`a03527c`**. **Read
`git show 2eed8de` before planning.** That diff is the requirement; this PRD is the delivery
plan for it.

What it changed, so you can find each requirement's authority:

| § | what it now says |
|---|---|
| §7 | a terminal row with a live pane is an **invariant violation the reconcile repairs**; the reconcile table's "session present, pane alive → keep the current status" now excepts it |
| §9.2 | a deleted session's name is **available again** and taking it forfeits that session's undo; an **archived row keeps its name** and `dd` frees it; a tombstone outliving its process is reaped at the next store open |
| §9.3 | lease owner is `pid@boot_id#<generation>`; the generation is **random**; a superseded hook is recorded without writing status; a launch **releases** its lease on conclusion, clearing the deadline only |
| §11.3 | the footer omits a key that would **refuse the current selection**, with one shared eligibility definition per action; its fixed set is curated |
| §11.4 | **`↑`/`↓` move between fields**; `tab` is reserved for completion; a transient list owns `↑`/`↓` while open; an overlay with no fields spends `↑`/`↓` on scrolling; a dialog is **themed**; a dialog **opens on the choice the user last made** |
| §11.7 | the recent-directory cycle is **`Ctrl+P`/`Ctrl+N`** |
| §11.10 | **new** — the `/` filter, including `esc` clearing a query held in force with the text field closed |

Those two commits are also the **only** commits this job should ever find touching a protected
path. At close-out, verify by **sha**, never by author or committer: this job's commits carry
the operator's own git identity, so an authorship-based "did the run touch this?" check is
vacuous. The protected set is `SPEC.md`, `prds/`, `ci/Dockerfile` and `ci/SPIKE.md`.

## Standing rules, restated because review never reads `docs/PLAN.md`

Review is re-prompted from this PRD on each pass. A rule that lives only in `docs/PLAN.md` is
a rule review has never seen — and for the same reason **a steer cannot reach review**.
Steering reaches the worker; the worker can only relay an operator ruling as a claim, and a
reviewer that accepted "the operator says this is fine" from the thing under review would be
worthless. The only way to change acceptance criteria mid-flight is to change this PRD.

1. **Commit and push your own work, one commit per completed task.** Messages say *why*, not
   what the diff already shows. Never force-push, amend or rewrite published history — fix
   forward. **If review requires corrections after a commit, add a follow-up commit — never
   amend.** Git credentials are mounted; no token is ever handled in a prompt, a file or a
   commit message.
2. **Review blocks on real defects and tolerates documentation nits.** Dead code satisfying a
   requirement on paper, a vacuous test, a flaky scenario, a false claim about behaviour, or
   anything touching a session's `cwd` must block. Wording, formatting and stale sentences in
   derived summaries are recorded as notes and do not fail a phase. The test is whether a
   reader would be *misled about behaviour*.
3. **A suite is "green" only when it is green ten consecutive times from a clean state**
   (`ci/stability.sh 10`). A flaky harness is worse than a missing one. Equally, a green suite
   is not evidence a requirement is met.
4. **`SPEC.md` is read-only.** Contradictions go in `docs/reports/phase3g-findings.md`.
5. **All build and test work happens in the sibling container** (`ci/run.sh`). The job
   container has neither Go nor tmux and cannot install them.
6. **Do not signal `COMPLETE` while any task is still `pending`.** A premature completion
   burns a review pass and an approach for nothing. Check the task table before signalling.

### The docker socket is mounted, and one command on it will kill this run

This job runs with `--allow-docker` because `ci/run.sh` needs sibling containers. That is
root-equivalent host access, and it is shared with other live runs.

- **Never** run `docker rm`, `docker kill` or `docker stop` filtered by a label — in
  particular `label=ralphd.run=`. **The job's own container carries that label**, so a
  label-filtered sweep SIGKILLs the run mid-iteration. This has happened before.
- **Never** target the container named `ralphd-deck-phase3g`. That is this job.
- **Never** `docker prune`, and never a wildcard `docker rm`/`rmi`. Remove containers and
  images by exact name and exact tag only. Other runs' content-hashed images are on this host.
- **Never signal by pattern** (`pkill -f`, `killall`). Resolve a PID, verify what it is, then
  signal that PID.
- Other checkouts on this host are the live workspaces of other runs. Do not edit, stash or
  clean a working tree other than `/workspace`.

`ci/run.sh`'s own throwaway siblings are fine — it removes what it creates, by id.

## Requirements

### R76 — the reconcile repairs a terminal row with a live pane

`SPEC.md` §7. The reconcile pass observes a live, non-dead pane under a row whose status is
terminal (`stopped`, `error`), **corrects the row from what it can observe** using §7's
existing liveness and probe rules, records the correction as an event, and **touches the pane
not at all** — no kill, no relaunch. Dead-pane collection is decided first, so a corpse is
still collected rather than mistaken for a live contradiction.

- `internal/service/reconcile.go`. The existing collect-on-sight path is the model for it:
  idempotent, unleased, first-writer-wins, so N clients seeing one violation need no lease.
- The scenario must arrive at the state the way the field did — a row written to `stopped` by a
  hook while its pane lives — not by hand-editing the store, or it proves nothing about the
  invariant.
- **A test that the pane survives the repair** is as load-bearing as the repair: the failure
  mode of a careless fix is a detector that "cleans up" a live agent, which is the operator
  losing a working session (GitHub #10 territory).
- Also assert the repair fires with **no TUI keypress** — this is the reconcile's job, not a
  key's.

### R77 — a deleted session's name is reusable

`SPEC.md` §9.2. Creating a session — or renaming one — with a name (or §3.2 slug) whose only
holder is a **tombstoned** row reaps that row and proceeds. The 60 s undo dies with it.

Facts to build against, all verified at `a03527c`:

- `name TEXT NOT NULL UNIQUE, slug TEXT NOT NULL UNIQUE` (`store.go:1688`) are **table-wide**
  implicit unique indexes, so they bind tombstoned and archived rows. **Adding
  `AND deleted_at = 0` to the pre-check at `store.go:402` is not sufficient** — the `INSERT`
  two lines later still violates the constraint and `CreateSession` already has that error path
  (`store.go:418-423`). The row must leave the table.
- **One transaction.** `CreateSession` runs in its own tx (`store.go:386`) and `ReapSession`
  opens its own (`store.go:1573`); calling the latter from inside the former on the same
  `*sql.DB` blocks on the write lock. Use a tx-scoped internal helper shared by both. A
  service-layer "reap, then create" is wrong: if the reap commits and the create then fails
  validation, a session's events are destroyed and nothing was created in exchange.
- **Filesystem cleanup after the commit.** `service.Reap` also removes the captures directory
  and the §9.4 history file (`internal/service/delete.go:110-117`), which cannot join the SQL
  transaction. A leftover captures directory is harmless; a removed one with no new session is
  not.
- **Only when the sole holder is tombstoned.** A live holder still refuses. An **archived**
  holder still refuses — see R78.
- **`u` must fail honestly.** The undo toast can still be on screen. `RestoreSession`
  (`store.go:1367`) is a bare `UPDATE … SET deleted_at = 0`; after the reap there is no row,
  so `u` reports that the session was reaped when its name was reused — never a raw
  "not found" and never a SQLite constraint error.
- **The dialog says so before it happens** (§11.4's in-dialog, specific validation): the Name
  field states that this name belongs to a deleted session and that reusing it discards that
  session's undo. A create that silently hard-deletes another row's record is not acceptable
  even though the loss is authorised.
- `RenameSession` (`store.go:1121`, slug pre-check `:1151`) gets the same treatment, or
  renaming onto a deleted name keeps failing while creating with it succeeds.

### R78 — an archived row keeps its name, and `dd` frees it

`SPEC.md` §9.2. Two halves, one of which is already true and untested.

- **Refusal, with a route out.** A create or rename whose name is held by an archived row is
  refused, and the message names the archived holder and both ways out: `U` to bring it back,
  or `dd` to delete it. Today the user gets `session name "x" already exists` about a row no
  list shows.
- **`dd` applies to an archived row.** This already works and nothing asserts it: `case "d"`
  gates only on `len(m.sessions) > 0` (`internal/tui/tui.go:2309`) and `service.Delete` has no
  archived guard (`internal/service/delete.go:20`), so `/` → query → `↵` → `dd` tombstones an
  archived row. The resulting `deleted_at != 0 AND archived_at != 0` is a combination the tree
  describes as impossible — `filteredSessions`' own comment says the two are "mutually
  exclusive of ListSessions' own WHERE clause" (`internal/tui/filter.go:63-65`) — and it
  happens to behave correctly at every step: `ListArchivedSessions` filters `deleted_at = 0`
  so the row leaves the archived pool, `u` clears only `deleted_at` so it returns to archived,
  and the reap removes it. **Every one of those is currently an accident.** Assert all four,
  and fix the comment.
- The delete confirm names the fact that the row is archived. §11.4 requires the confirmation
  to name the target and what survives it; "this is the archived record you were keeping" is
  part of that.

### R79 — a tombstone that outlives its process is reaped at the next store open

`SPEC.md` §9.2. On store open, reap every row whose `deleted_at` is older than
`DeleteGrace` (`internal/config/config.go:27`, `DefaultDeleteGraceMS = 60000`).

- Today the reap is driven **only** by an in-process `tea.Tick` (`tui.go:1655`, and `:1756`
  for the bulk case). `store.ListDeletedSessions` — whose own doc comment (`store.go:1250-1254`)
  describes calling `ReapSession` on each — has **zero non-test callers**. The sweep it was
  written for does not exist.
- So `dd` then quit (or crash, or `SIGKILL`, or close the terminal) within 60 s strands the
  row, its events, its captures directory, its history file and its name, permanently.
- Model it on `EnforceEventRetention` (`store.go:1514`), which already has the
  "on store open and thereafter at most once an hour" shape and a `ui_state` last-run key.
  Bounded batches; a sweep must never block the first frame for longer than one batch.
- The frozen-clock rule applies: the age comparison uses the injected clock, and a store write
  with no `At` falls back to real `time.Now()`, which would mix real time into a frozen store
  and make the comparison nonsense while looking like a sweep bug.

### R80 — the footer lists only what the current selection will accept

`SPEC.md` §11.3. `footerLegend` is a package-level `var` of 13 fixed entries
(`internal/tui/tui.go:3219-3242`) and `footerKeyLegend` (`:3250`) renders all of them
unconditionally with **no session argument at all**, so the footer cannot be contextual as
§11.3 requires. It advertises `x kill` on a `stopped` row and `r resume` on a live one.

- **One eligibility definition per action, shared by the footer and the key handler.** There is
  no such definition today: each key decides inline in its own case (`"Y"` `:2190`, `"x"`
  `:2198`, `"A"` `:2259`, `"U"` `:2282`, `"r"` `:2416`, `"R"` `:2430`). Extracting them is most
  of this requirement. A footer with a second, parallel copy of those conditions will drift out
  of agreement with the behaviour it advertises, and a test that only checks the keys that
  *should* be there passes happily on a footer that still shows `x` on a stopped row — so
  **assert absence as well as presence**.
- **With a mark set in force the question is asked of the marked rows** (`markedSessions`,
  `tui.go:4707`), because that is what `x`/`dd` would act on.
- Scenarios for at least: live, stopped, archived, attention-pending, marked, and the empty
  list.
- **The status reason shares the line** (§11.3), so a variable-length legend must still behave
  at 80 columns beside a long reason like `pane failed after the stale frame`.

### R81 — the footer's fixed set is curated

`SPEC.md` §11.3. `dd`, the eligible one of `A`/`U`, and `,` are in; `P` (profile) and `p` (pin)
come out of the footer while staying bound, staying in the `?` overlay and staying in §11's
keymap.

- **`,` is bound (`tui.go:2138`), in §11's keymap (`SPEC.md`'s keymap block) and in the `?`
  overlay — and absent from the footer**, so the entire settings view has no visible entry
  point in the default frame. The reason nothing caught it: requirement 38's parity test
  (`internal/tui/help_keymap_parity_test.go`) checks help↔bindings in both directions and there
  is no equivalent for the footer.
- **Build that equivalent.** Every footer entry names a bound key, and every entry shown for a
  row is one that row will accept. The second half needs R80's extracted predicates, which is
  another reason they must be real functions rather than inline conditions.
- `features/testdata/golden/side_by_side_80x24.golden:24` contains the legend text. With a
  contextual footer the fixture row's status decides which keys that frame shows, so the golden
  and the fixture become coupled — choose the fixture's status deliberately and say so in the
  report.

### R82 — dialogs are themed

`SPEC.md` §11.4. Every dialog body renders in §11.6's tokens, applied **at render time**.

Current state, verified at `a03527c` — "field lines only" means the dialog calls
`m.detailField` (`tui.go:4530`, `hint` label + `text` value) so its `Current:` /
`Conversation:` rows are themed while everything around them is not:

| dialog | renderer | themed? |
|---|---|---|
| create session | `tui.go:5323`, `createFieldRows` `:5300` | **no** — only the cwd ghost (`:5269`) |
| delete / purge confirm | `tui.go:4375` | field lines only |
| bulk delete confirm | `tui.go:4404` | field lines only |
| archive confirm | `tui.go:4331` | field lines only |
| profile picker | `tui.go:4022` | field lines only |
| pin conversation | `tui.go:4081` | field lines only |
| restart-or-inject | `tui.go:4157` | field lines only |
| rename | `rename.go:155` | **no** — zero `colorToken` calls in the file |
| env editor | `env_editor.go:98` | **no** — zero |
| event log | `event_log.go:40`/`:50` | **no** — zero |

- Title → `title`; label → `hint`, value → `text`; per-field help → `dimmed`; footer keys →
  `key` with the rest in `hint`; validation → `error`; the archive confirm's "confirming kills
  the live agent first" warning → `badge_warn`; the focused row → the `selection` treatment the
  sidebar and settings rows already use. Precedents to copy rather than reinvent:
  `help_style.go:139/151/159` and the main-view footer at `tui.go:3255-3257`.
- The create modal is the worst case and needs the most: its eight rows are one
  `fmt.Fprintf("%s%s: %s\n    %s\n", …)` (`tui.go:5337`), so label, value and help are
  indistinguishable and the `> ` marker (`:5324`) is the only focus cue.
- **`createFieldRows` keeps returning plain strings.** Colour is applied in the view, exactly as
  `styledHelpText` left `helpText` untouched (GitHub #3, `415723d`). The keyboard-only PTY
  assertions (`create_field_help_test.go`, `archive_confirm_test.go`, `delete_purge_test.go`,
  `dialog_width_test.go`, …) match plain substrings and must stay green **unchanged**.
- **Width and height accounting must not shift.** `wrapDialogLines`/`dialogContentBudget`
  (`internal/tui/panel.go:788`, `:809`) measure display width; no SGR sequence may leak into
  those measurements. `colorToken` self-resets per segment, so plain concatenation stays safe
  as long as no dialog composes tokens under a shared selection background the way the sidebar
  does — and a truncated coloured run re-emits its own reset (§11.3).
- **`NO_COLOR` and `DECK_ASCII` degradation is the one thing this can regress.** Every dialog
  must stay fully legible with every token stripped, which is what the current plain text has
  for free.

### R83 — the three dialogs that draw past the frame at 80×24

Finding **F6** in `docs/reports/phase3f-findings.md`. The env editor, the create modal and the
bulk delete confirm overflow `framedDialog` at 80×24 — deck's own supported minimum and the
size of its golden frame.

- `framedDialogScrollable` (`panel.go:966`) already exists for exactly this, and its own doc
  comment names these dialogs as the ones still on the unbounded path.
- Bound them rather than truncating: §11.4's width rule says a truncated-but-honest frame beats
  an unpredictable one, and a dialog that loses its submit line off the bottom is neither.
- Do this **together with R82**, not after it. Both requirements touch the same three dialogs,
  and doing them apart means rendering each one twice.

### R84 — the contrast floor covers the pairs a dialog actually uses

`SPEC.md` §11.6. `internal/theme/contrast_test.go` enforces ≥3.0 for every token over
`Background` (`:29-38`) and a subset over `Surface` (`sessionRowSurfaceChecks`, `:115-129`:
title, dimmed, text, badge, badge_warn). R82 draws tokens the floor does not check.

- Add `hint`/`surface`, `key`/`surface`, `error`/`surface`, and **every text token over
  `Selection` and `SelectionIdle`** for the focused field.
- Both the truecolour and the 16-colour quantised values, as the existing test already does.
- **This requirement pins what is already true; it does not license a palette change.** Measured
  at `76b7347`, every newly-covered pair clears 3.0 in `matrix` — thinnest `error` on
  `selection` at 3.16 and `dimmed` on `selection` at 3.78. If a *different* built-in fails the
  new checks, that is a finding to report, not a licence to recolour: the operator's ruling is
  legibility over distinctness, with `matrix` as the reference theme. Bring a failing pair to
  the operator via a finding rather than editing a theme file to make a test pass.

### R85 — the create modal opens on the last used agent

`SPEC.md` §11.4. Today `tui.go:2168` sets `m.createAgent = defaultCreateAgent(m.registry().Kinds())`
and `defaultCreateAgent` (`:738`) returns `shell` whenever the registry has it — which it always
does — so every `claude` session costs two extra keystrokes, forever.

- Persist in `ui_state` (schema v2, `store.go:1714`) via `getUIState`/`setUIState`
  (`:1749`/`:1761`) — a new key beside `sidebar_width`, **no new table and no migration**.
  §11.2's "ui_state is not load-bearing" applies: a missing or unparseable row degrades to
  today's `defaultCreateAgent`, which is also the right first-run behaviour.
- Written when a create **succeeds**, never when the field is cycled, so an abandoned dialog
  changes no default — the same rule `recent_cwds` follows.
- Validated against `m.registry().Kinds()` on read; an unregistered agent falls back.
- **Re-derive the profile.** `createProfile` is a function of `createAgent`: `:2169` pairs them
  at open and the cycle case re-derives or repairs at `:5166-5172`. Pre-selecting the agent must
  go through that derivation, or the dialog opens on `claude` carrying a profile only ever valid
  for `shell`.
- **Labelled**, per §11.4 and §11.7's `(last used)` precedent (`createCWDHelp`, `tui.go:5289`).
  Without the label this is a silent assumption about which agent is about to launch.
- No store read in a render path (§11.4): the value lives in model state, loaded when the dialog
  opens or held from startup, as `createCWDRecents` already is.

### R86 — `↑`/`↓` navigate dialog fields; `tab` is completion only

`SPEC.md` §11.4 and §11.7. The largest keymap change in this phase, and the one with the most
ways to half-land.

- `applyDialogContract` (`internal/tui/dialog_contract.go:65`) is the single implementation of
  the contract, so `tab`/`shift+tab` become `↑`/`↓` there once and the create modal, profile
  picker, pin and restart-or-inject all follow.
- **`tab` never navigates.** Delete the fall-through: `updateCreate` (`tui.go:4908`) currently
  intercepts `tab` on field 1 and only reaches the contract when `tabCompleteCreateCWD` reports
  false, so today the same keystroke either completes a path or jumps to the Agent field
  depending on what happens to exist on disk. On a path field `tab` completes; everywhere else
  it is **unbound**.
- **Recents move to `Ctrl+P`/`Ctrl+N`** (`cycleCreateCWDRecent`, `tui.go:4982-4996`), and
  `createCWDHelp` (`:5273`) stops advertising `↑`/`↓` for them.
- **The candidate list keeps `↑`/`↓` while open** (`tui.go:4953-4966`) and its caption
  (`:5345`) stays true.
- **The overlays must not regress.** `?`, `i` and `E` have no fields, so `↑`/`↓` remain line
  scroll (GitHub #7 / R73, now specified in §11.4). Assert it explicitly rather than assuming
  `Count 0` protects it — a contract that now binds `↑`/`↓` is exactly what could take those
  keys back.
- **On-screen text and the parity net:** every dialog footer (e.g. `tui.go:5354`), the candidate
  caption, the cwd help line, and the `?` overlay's keymap. `help_keymap_parity_test.go`
  re-parses the live source in both directions and will catch a half-done change — treat a red
  there as the requirement talking, not as a test to adjust.
- **Not in scope:** §11.5's settings takeover. It is not a §11.4 dialog and its `tab` switches
  between two whole regions, which §11.4 now says explicitly. Leave `settings.go` alone.

### R87 — `esc` clears a filter held in force

`SPEC.md` §11.10, finding **F11**. `filterStatusLine` promises
`Filter "x" in force (n matching) — / to change, Esc to clear` (`internal/tui/filter.go:147`),
and with the text field closed no `esc` clears anything: the only assignment that clears
`filterQuery` is `updateFilter`'s `case "esc"` (`filter.go:101-106`), dispatched only while
`m.filtering == true`. The top-level `esc` (`tui.go:2110`) clears `m.help`, `m.detail` and the
mark set and nothing else.

- Bind it: `esc` at the list clears a held query when nothing nearer is open — no overlay, no
  dialog, no mark set — and one press never does two of those at once.
- **The existing scenarios are no regression net.** Task 011's round trip and
  `@requirement-33-unarchive-from-filter-results` both reopen `/` before pressing `esc`, so they
  pass under both the true and the untrue copy. The new scenario must **not** reopen `/`.
- Three sites cite `SPEC.md:318` for an "esc clearing" rule and that line is inside the
  `recent_cwds` schema block — `filter.go:102`, `internal/tui/filter_test.go:141`,
  `features/filter_test.go:66`. The rule now genuinely exists (§11.10); re-point them at it.

### R88 — `inject` refuses a retained dead pane

Finding **F3**. `internal/service/inject.go:51` decides "can this shell pane receive
`send-keys`?" with `Exists`, which a **retained dead pane passes** — so an env injection is
sent into a corpse and reported as applied.

- This is a different defect from GitHub #6 / R69, which was about *reconciliation* of a dead
  pane. This is injection into one, and it needs its own test.
- The liveness question already has a right answer in this tree: R69's "resumable means has a
  live pane". Reuse that notion rather than inventing a second one.
- On refusal, say what happened — a `stopped`/`error` row cannot take an injection, and §6.4's
  restart-to-apply path is the route.

### R89 — the interactive pipe leaks nothing on abnormal exit

Finding **F4**. An abnormal exit leaves `/tmp/deck-interactive-pipe-*`, the FIFO, an **armed
`pipe-pane`** and window ownership behind (`internal/tmux/pipe.go:94`/`:280`,
`cmd/deck/main.go`).

- The armed `pipe-pane` is the one that matters: it keeps the agent's pane writing into a FIFO
  nothing reads, which is a wedge waiting to happen and a direct cousin of GitHub #5.
- Cover the exits a user actually produces: `SIGTERM`, `SIGKILL` (which cannot be handled — so
  the guarantee has to be "the next start reclaims it", not "the exit cleans it"), and a panic.
- §11.9 requires geometry to be claimed, bounded and **restored byte-exactly**; window
  ownership leaking past a crash is that guarantee failing outside the happy path.

### R90 — a hook dropped as superseded is labelled dropped

Finding **F15**. R74 stores a superseded hook verbatim and leaves the row untouched, so
"event present, status unchanged" is the only signal a forensic reader gets
(`internal/hookrecv/receiver.go:114-141`).

- Give it a distinct event kind or reason so the event log and §12 can answer "did a hook get
  declined here, and why" directly.
- `SPEC.md` §6's rule applies: **a kind is only written if something reads that kind.** So this
  requirement includes the reader — the `E` event log and the `i` detail must show it — or the
  new vocabulary is dead data.
- `internal/hookrecv/receiver.go:222` is the only non-test writer of `Source: "hook"` in the
  tree; that remains the single chokepoint and this requirement must not add a second.

### R91 — `previewFit`'s spent fit, and the SIGWINCH re-baseline it licenses

Finding **F12**. `previewFit`'s no-live-pane early return emits `previewFitDone` anyway
(`internal/tui/tui.go:1406-1413`), spending the row's one coalesced fit attempt on an attempt
that fitted nothing; the next legitimate fit is then refused until the selection moves away and
back.

- **This PRD licenses what Phase 3f forbade.** 3f could not fix it because bounding the early
  return changes passive-fit behaviour for every session that starts while selected, which
  moves other scenarios' expected SIGWINCH counts, and re-baselining a count was forbidden
  there. It is permitted here, under two conditions: every changed count is justified in the
  report by what the new behaviour *should* produce, derived before the run rather than read
  off a failure; and no scenario is deleted, skipped or tagged out to make a count agree.
- Do not re-break F1. `features/preview.feature`'s fit-floor scenario (`:171`, `:173`) was
  closed in `2b39124` by a maker/observer client split; its `has never been taller than`
  assertion must stay green and must keep failing *before* the count assertion it protects.

### R92 — R75's release-failure fallback is exercised

Finding **F17**. R75's best-effort release fallback — the `UPDATE` failing, an audit
`launch_lease.release_failed`, then §9.3's TTL as the backstop
(`internal/service/resume.go:184-193`) — is exercised by no test, because driving that `UPDATE`
to fail needs a store fault-injection seam the tree does not have.

- Build the seam, narrowly, and **as a product-honest one**: not a test-only branch in product
  code (R8 forbids those), but an interface the service already depends on which a test can
  substitute. `Resume` takes its store through a dependency today; that is the seam to widen.
- Assert both halves: the audit event is written, and the lease still expires by TTL so the row
  is not wedged. No verdict depends on the fallback — it *is* the pre-R75 behaviour — so this
  requirement is about proving the degradation is graceful, not about changing it.

## Ordering

1. **R76 first.** It is the phase's reason to exist and it is independent of everything else.
2. **R77, R78, R79 as one leg**, in that order: R77 establishes the tx-scoped reap helper R78's
   messages and R79's sweep both build on.
3. **R80 before R81** — the curated set is meaningless until eligibility exists.
4. **R82 and R83 together**, one dialog at a time: theme it and bound it in the same pass.
5. **R84 after R82**, since it is the floor for what R82 draws.
6. **R86 late.** It moves on-screen text in every dialog, so landing it after R82/R83 means
   touching those strings once. It is also the most likely to need a second pass.
7. **R85, R87, R88, R90, R91, R92** are independent; take them in whatever order the tier
   ladder favours.

## Green when

- `create_session`, `kill_delete_undo`, `environment`, `filter`, `preview`, `crash` and the new
  footer-eligibility feature all pass.
- `features/testdata/golden/side_by_side_80x24.golden` regenerated deliberately, with the
  fixture's status stated in the report.
- `help_keymap_parity_test.go` green, plus its new footer counterpart.
- `internal/theme` contrast tests green with R84's added pairs, on every built-in.
- `ci/run.sh go test -p=1 -count=1 ./...` green, and **`ci/stability.sh 10` 10/10** at the
  phase's final code commit — cite the sha and the log paths.

## Non-goals

- Anything in Phase 5 (notifications), 6 (shell state) or 7 (search, health view).
- The Codex adapter and §8.2 id discovery, now the last phase in `docs/PLAN.md`.
- **Theme palette changes.** F7's status-token quantisation collisions in `cobalt`, `daylight`,
  `empire` and `parchment` stay open: §11.6 asks those themes to be legible and they are.
- A generalised `default_permission_profile`. GitHub #1 is closed — `c550045` shipped
  `yolo_default` and removed the per-create confirm — and nothing has asked for a non-`yolo`
  default since.
- F2 (`TestGoldenMinimumFrame`'s settle flake) is **not** in scope and must not be claimed
  fixed. If it appears in a stability run, report it as a recurrence with the log path.
- F8, F10 and F14 are documentation nits in `docs/reports/`. Fix them if convenient; they
  block nothing.

## Reports

- `docs/reports/phase3g.md` — one section per requirement, each naming its fixing sha, its
  scenario or test, and the evidence path. Revert-and-reproduce for every product defect
  (R76, R77, R79, R86, R87, R88, R89, R91): quote the red before the fix and the green after.
- `docs/reports/phase3g-findings.md` — spec contradictions, defects found and deliberately not
  fixed (with the reason), and anything this PRD got wrong. Phase 3f's §6 is the model: a
  finding table that survives being cited from elsewhere.
- Both reports get the citation sweep Phase 3f used, and its known false-positive class: a
  sweep that greps a report's own prose for citations will flag the report describing itself.
