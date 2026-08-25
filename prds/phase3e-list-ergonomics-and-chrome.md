# Phase 3e — list ergonomics and chrome

## Goal

Seven operator-reported items from living with the tool after Phase 3d. None of them is a new
subsystem; all seven are the difference between a list you *use* and a list you *fight*. Three
are outright defects in what deck already draws, two are ergonomics reversals the operator
asked for after the shipped behaviour lost an argument with daily use, one is a new config key,
and one is new colour data.

This is an **insert phase**. It is small on purpose, it comes ahead of the `SPEC.md` §14.2 Codex
spike, and its whole value is that every item is something the operator hit in the first hours
of using the Phase 3d build. Treat "the operator noticed this within a day" as the priority
signal it is.

## Read this before anything else: `SPEC.md` was amended for you

**`SPEC.md` is the authoritative product spec and must not be modified by this job.** Where this
PRD and `SPEC.md` disagree, `SPEC.md` wins and the disagreement is a finding.

Four of the seven requirements below **contradicted `SPEC.md` as it stood**, and the correct
response to that would have been to refuse them. So the operator amended `SPEC.md` first, in
**commit `6584299`** (`spec: configurable sort order, single-click interactive entry, shared seam
focus, open built-in theme set (operator)`), which is the commit immediately before this PRD's
own. **Read that diff before planning.** It is the requirement; this PRD is the delivery plan
for it.

That commit is also the *only* commit this job should ever find touching a protected path. The
standing protected-path rule is unchanged and still applies to `SPEC.md`, `prds/`,
`ci/Dockerfile` and `ci/SPIKE.md`: at close-out, verify by **sha**, never by author or committer
identity (they are byte-identical strings — see `docs/reports/phase3d-212-closeout/README.md`,
which got this right and is the model to copy), and treat **any** sha you do not recognise in
that range as a hard failure to report rather than something to classify. The recognised set for
this phase is exactly `6584299` plus this PRD's own commit.

## Where work happens

Unchanged from Phase 3/3b/3c/3d: the job container has no Go and no tmux and cannot install
them. **All building and testing happens in the sibling toolchain container via `ci/run.sh`.**

## What already exists — do not rebuild it

Every one of the seven items below has been traced to specific code on the tip this PRD is
written against (`f715a56` code / `4b21dea` docs). The root causes are given per requirement.
**They were read off the live tree, not guessed.** Verify each before you fix it — and if a root
cause here is wrong, say so in a report and fix the real one; a PRD's diagnosis is evidence, not
scripture. But do not re-derive them from scratch, and do not "fix" a symptom at a different
layer than the cause named here without stating why.

Reusable machinery that already exists and must not be duplicated:

- **The R7 config schema** (`internal/config/schema.go`) generates every flat `config.toml` key
  into the settings view from the same declaration that parses the file. `sort_order` is one
  declaration plus one parse case; it is **not** a new settings widget. `preview_fit` (task 215)
  and `yolo_default` (task 214) are the two most recent worked examples, including the
  `settingsEditsFromSettings` mapping (`internal/tui/settings.go:725-742`) that task 215 found a
  key can be silently omitted from — check yours is there.
- **`sortSessionsByAttentionStable`** (`internal/tui/attention.go:130`) already solves the hard
  part of any re-sort: preserving a tie's previous relative order so an in-flight marked-set
  idiom cannot have two rows swap under it. Its doc comment contains a worked 3-cycle proving why
  the naive "compare positions when both have one, else fall back to ID" is **not a strict weak
  ordering**. Any new comparator you write inherits that trap. Read it first.
- **`hitTest`** (`internal/tui/mouse.go:47`) already resolves a cell to a panel/row by asking the
  layout what it drew, per `SPEC.md` §11.8's own hit-testing rule. Requirement 54 needs it called
  from one more place, not reimplemented.
- **The theme pipeline** — `internal/theme/{registry,parse,contrast,quantize}.go` plus
  `internal/theme/builtin/*.toml`. Adding a built-in is a file drop plus a registry entry.
- **Per-cell SGR assertions** — `features/cell_attributes_token_test.go`,
  `internal/tui/theme_color_test.go` and the golden-frame harness read real per-cell attributes.
  Requirements 57 and 58 are **background-attribute** assertions and must use this, not text
  scraping. A test that only reads text cannot see either bug.

---

# The requirements

Numbering continues `SPEC.md`'s own: 51 was the last. These are **52–58**.

## R52 — a newly created session is selected as soon as it appears

**Operator's words:** *"when creating new session, it should be automatically selected in the
list. currently it takes effort to spot where it appeared."*

**Spec:** `SPEC.md` §11, the new bullet added by `6584299` ("A newly created session is selected
as soon as it appears").

**Root cause, as read off the tree.** `internal/tui/tui.go`'s `shellCreated` case clears
`m.creating` and returns `m.loadSessions` — and nothing else. The subsequent `sessionsLoaded`
case preserves the *previously* selected id (`indexOfSessionID(m.sessions, selectedID)`) and
otherwise only clamps `m.selected` into range. The new session is never selected. It is worst in
the default `attention` order, where a brand-new row is `starting` — fifth of six ranks — so it
sorts near the bottom and can be off-screen entirely.

**What to build.** A one-shot "select this session id once it appears" intent, set when a create
succeeds and satisfied by the first `sessionsLoaded` that contains that id. Then clear it.

**Success criteria.**
- A godog scenario creates a session and asserts the new session's row is the selected row
  (`>` marker on its line) on the frame after it appears, **without any navigation keystroke
  between the create and the assertion.**
- The scenario must be **non-vacuous against the sort order**: the fixture must place the new
  session somewhere other than index 0 in the active order, so a "selection happens to be at the
  top" implementation fails it. State in the report how you guaranteed that, and prove it by
  reverting the fix and showing the scenario red.
- The intent is **one-shot**: a scenario moves the selection away with `j`/`k` after the create,
  lets at least one further reconcile tick land, and asserts the selection **stayed** where the
  user put it. A standing "always select the newest" rule fails this and is the wrong fix.
- If the created session never appears (create succeeds, row absent), the selection does not
  move and nothing panics.
- Interaction with the filter (`internal/tui/filter.go`, task 123/I-10): if a filter query is in
  force and the new session does not match it, the selection does not move. Do not clear the
  user's filter to reveal the new row.

## R53 — `[ui] sort_order`, four orders, `attention` still the default

**Operator's words:** *"sessions order in the list is unclear. let's make it configurable in
settings. options: by creation ts (new go on top), by last activity and alphabetically."*

**Spec:** `SPEC.md` §11's rewritten Sort bullet and §6.5's `[ui]` row, both in `6584299`.

**Decided by the operator, do not re-litigate:** `attention` **stays** and **stays the default**.
The three new orders are additions, not a replacement. A non-`attention` order does **not**
secretly re-rank by status — no hidden "waiting floats to the top" tier. That was offered and
declined.

**What to build.** `[ui] sort_order`, values `attention` (default) / `created` / `activity` /
`name`, declared once in the R7 schema and therefore appearing in settings for free.

- `created` — `CreatedAt` **descending** (newest first).
- `activity` — `StatusAt` **descending**. **"Activity" is a status change**, per the amended spec:
  `StatusAt` is §7's own timestamp. deck records no per-session last-output clock and this phase
  does **not** add one — a new column is a schema migration and is explicitly out of scope. If you
  believe `StatusAt` is the wrong field, file a finding; do not add a column.
- `name` — ascending, **case-insensitive**, so `Api` and `api` sort adjacently.
- Every order ends in `ID` ascending as a total-order tie-break.

**Success criteria.**
- An unknown/malformed `sort_order` value falls back to `attention` **and says so**, on the same
  footing as §11.6's unknown-theme rule — it never silently renders the default as though the
  configured value applied. Assert the message, not just the order.
- One scenario per order, each with a fixture where the four orders give **four different row
  sequences**. A fixture where two orders agree cannot tell them apart, and a scenario that
  cannot fail if the comparator is wrong is worth nothing. State the four expected sequences in
  the report.
- **`features/attention_sort.feature` passes unmodified.** It is the regression proof that the
  default did not move. If a scenario in it needs `sort_order = "attention"` written into its
  config to keep passing, that is a **defect in your default**, not a fixture to patch.
- Changing the order at runtime (via settings, with save) **does not move the selection**: the
  same session is selected before and after, and the viewport scrolls to keep it visible. Scenario
  required — this is the new spec bullet and it is easy to get wrong, because the naive
  implementation preserves the *index*.
- The new comparators must be strict weak orderings. Read
  `sortSessionsByAttentionStable`'s doc comment (`internal/tui/attention.go:130`) for the 3-cycle
  trap first, and add a unit test per comparator that a three-way tie cannot produce an
  intransitive result.
- Grouping (`group_by_workspace`) composes with all four orders: the order applies **within** a
  workspace group, and the groups themselves keep whatever order they have today. One scenario
  with grouping on and a non-`attention` order.
- Settings parity: the existing `TestSchemaPinsKeySet` /
  `TestSettingsCategoriesGroupEveryFlatKeyExactlyOnce` family must pass with the new key, and the
  key must be present in `settingsEditsFromSettings` — the exact omission task 215 found for
  `preview_fit`. Check it explicitly and say you did.

## R54 — a sidebar click works while interactive mode is active

**Operator's words:** *"when attached to preview, click on sesion list should still work -
navigation should happen."*

**Spec:** `SPEC.md` §11.8's new bullet "A sidebar click works while interactive mode is active,
and re-targets it", in `6584299`.

**Root cause, as read off the tree.** `Update`'s `tea.MouseMsg` case routes press/motion/release
**directly into the interactive drag-to-copy handler whenever `m.interactive` is true, never
reaching `handleMouse` at all** — stated in `internal/tui/mouse.go:216-221`'s own comment, added
by task 216. So while interactive, no click is hit-tested and the sidebar is dead surface.

**What to build.** While interactive, hit-test the press **first**. A press over the sidebar goes
to the sidebar bindings; a press over the preview keeps going to the drag-to-copy path exactly as
task 216 built it. The gesture is resolved by **where it started**, which is already `SPEC.md`
§11.8's rule for the seam-vs-preview drag — same rule, one more case.

**Success criteria.**
- A scenario enters interactive mode on session A, clicks session B's row in the sidebar, and
  asserts: selection moved to B, interactive mode is now on **B** (the preview's top border
  carries B's name — that label is legible text, not just a colour, per `SPEC.md` §11.3, so assert
  the text), and A's window size was **restored**.
- Clicking the row that is **already** the interactive target changes nothing and in particular
  does **not** leave-and-re-enter. Assert no resize of that session's window across the click —
  `#{window_width}x#{window_height}` unchanged, or the fake agent's own size log unchanged. A
  leave-and-re-enter that lands on the same size would pass a naive assertion, so assert the
  *absence of the transition*, not the final size.
- **Task 216's drag-to-copy must still work, unmodified.** `features/interactive_selection.feature`
  passes as-is, including the click-the-first-cell scenario, which is a press over the *preview*
  and must not be captured by this change. This is the requirement most at risk of a regression
  from R54, and R54 is the requirement most at risk of being "fixed" by weakening it.

## R55 — single-click enters interactive mode; the double-click binding goes away

**Operator's words:** *"I often catch myself starting typing when preview not attached after
clicking. attach should happen on SINGLE click, not double. If I need to access list, I will just
hit ctrl+q - it's needed rarer, really."*

**Spec:** `SPEC.md` §11.8's amended table row and the **reversed** bullet, both in `6584299`.
**Read that bullet.** It reverses a decision that the spec previously called load-bearing, it
says so, and it records the cost the reversal accepts. You are implementing a deliberate
trade-off, not correcting an oversight.

**Root cause / what exists.** `clickSidebarRow` (`internal/tui/mouse.go:250`) implements the
double-click pair with `m.lastClickIndex` / `m.lastClickAt` / `doubleClickWindow`.

**What to build.** One press on a sidebar row selects it **and** enters interactive mode on it.
The double-click state and window come out — dead state that no longer means anything is worse
than no state.

**Explicitly unchanged, and this is the line to hold:** **full attach (`a`) gets no mouse
affordance.** `Ctrl+Q` can undo interactive mode; nothing undoes handing over the whole terminal.
The amended spec keeps this and says why. A click must never full-attach.

**Success criteria.**
- `features/mouse.feature`'s click scenarios are **rewritten, not deleted**: one press on a row
  now asserts both the selection *and* interactive entry. A scenario that stops asserting the
  selection moved is a weakened scenario.
- A scenario asserts `Ctrl+Q` returns to the list from a click-entered interactive mode — the
  operator's stated escape hatch, and the mitigation the spec's cost paragraph names. If it does
  not work, the reversal is not shippable.
- Entering by click and entering by `↵` reach the **same state**. Assert it against the same
  rendered frame rather than by inspecting model fields — `SPEC.md` §11.8's "no capability is
  mouse-only" rule is checkable precisely because both paths land in one place.
- A click on a **group header** still toggles collapse and does **not** enter interactive mode;
  a click on the collapsed strip still restores the previous mode. Both already have scenarios;
  both must still pass. A blanket "any sidebar press enters interactive" breaks them, which is
  exactly the sloppy version of this change.
- A click below the last row, on padding, or on a row that is not a session enters nothing.
- `doubleClickWindow` and the click-tracking fields are **removed** if nothing else uses them.
  Grep and say what you found. Leaving them makes the next reader think a double-click still
  means something.
- The harness keeps its double-click synthesis capability (`SPEC.md` §13's harness-capability
  list still names it). Do not delete driver capability to match a binding change.

## R56 — more built-in themes, including a green "matrix" one

**Operator's words:** *"color schemes need more options. I want a greenish matrix style one!!"*

**Spec:** `SPEC.md` §11.6, amended in `6584299` to say the built-in set is **open-ended** and
required to hold at least one dark and one light rather than exactly one of each.

**What to build.** Ship **`matrix`** — a dark, green phosphor palette; the operator asked for it
by name and it is the point of this requirement — plus at least **one further dark** and **one
further light** so "more options" is true rather than technically satisfied. Current built-ins
are `empire` (dark) and `daylight` (light); the new ones are data files, not code.

**The trap, named in advance.** Two existing tests constrain the built-in set, and one of them
will fail the moment you add a third theme:

- **`internal/theme/theme_test.go:61` `TestBuiltinsHaveOneDarkOneLight`** asserts *exactly* one of
  each. It must be **relaxed to "at least one of each"** — which is what the amended spec now
  says — and the diff must be exactly that. **Deleting the test, or dropping the appearance
  assertion, is not the fix.**
- **`internal/theme/quantize_test.go:58` `TestBuiltinQuantizationPinned`** pins every built-in's
  exact quantisation. Each new theme needs its own pinned entry, computed from the palette you
  actually ship.

**And the trap that matters more.** `SPEC.md` §11.6's contrast floor applies to every built-in:
`text`, `hint`, `title` and each of the seven status tokens must clear it, and all seven statuses
must stay **mutually distinguishable**. A green-on-black palette makes this genuinely hard —
seven distinguishable hues inside one narrow phosphor range is the whole design problem of this
requirement. The amended spec is explicit: **the floor is never the thing that gets relaxed to
admit a palette.** If `matrix` cannot clear the floor as pure green, widen the palette (hue,
brightness, a non-green accent for `error`) — do not touch `contrast.go`, its threshold, or its
test. A commit that changes both a theme file and the contrast threshold will be read as
gaming the check.

**Success criteria.**
- `matrix` is selectable via `[ui] theme = "matrix"`, in the `t` picker, and in settings, with no
  per-theme code anywhere — prove the "one-file drop plus one registry entry" claim by showing the
  diff for one of the two non-`matrix` themes touches only a `.toml` and the registry.
- Every built-in, new ones included, passes the existing completeness, contrast and quantisation
  tests. No test in `internal/theme/` gets weaker except `TestBuiltinsHaveOneDarkOneLight`'s
  count, and that one gets **exactly** the relaxation named above.
- A scenario (or per-cell SGR unit test) asserts `matrix`'s seven status tokens are **seven
  distinct** colours as rendered. "It looks green" is not a criterion.
- The 16-colour quantised form of each new theme is still legible, per `SPEC.md`'s "every theme
  remains legible on a 16-colour terminal". Say what you checked.

## R57 — the seam takes the focus colour when the sidebar is focused

**Operator's words:** *"selected panels outline has issies. e.g when chat list is highlighted,
right border is still gray (as being part of inactive preview panel)."*

**Spec:** `SPEC.md` §11.3's new "The seam is shared" sentences, in `6584299`.

**This was not a bug — it was the spec, and the spec changed.** Worth understanding before you
touch it, because the shape of the fix depends on it. `previewContentLine`
(`internal/tui/panel.go:371`) draws the seam with `m.previewBorderToken()`, which is
`theme.Border` while the sidebar holds focus — and the old §11.3 said the preview draws all four
of its borders and the focused surface's border uses `border_focus`. The code was **correct
against the old spec**. The operator judged the *result* wrong: a focused sidebar with three
focus-coloured edges and one grey one reads as half-focused. The amendment separates ownership of
the **glyph** (the preview's, unchanged) from ownership of the **colour** (shared).

**What to build.** The seam column renders in `border_focus` when **either** adjacent panel is
focused. The `┬`/`┴` T-junctions where it meets the sidebar's top and bottom borders follow the
seam.

**Success criteria.**
- A per-cell SGR assertion — not a text assertion — that with the sidebar focused, the seam
  column's foreground is the theme's `border_focus`, and with interactive mode active it is still
  `border_focus` (the preview is focused then, so it is focused either way; the interesting
  negative is below).
- The negative case: a state where **neither** panel is focused, so the seam is `border`. A
  dialog open over the main view is that state (`SPEC.md` §11.3: "a dialog that opens takes focus
  and the sidebar's border reverts"). **Without this case the requirement is untestable and any
  implementation that hard-codes `border_focus` passes** — that is exactly the fixture-makes-both-
  behaviours-look-identical failure this project has been bitten by. Get this one right.
- `stacked` mode has no seam (each panel keeps four borders) and is unaffected; assert it still
  splits `border_focus`/`border` by which panel is focused.
- The collapsed strip's own border follows the same rule.
- `NO_COLOR`/golden-frame behaviour is unchanged: this is colour-only, so the golden frames must
  be **byte-identical**. If a golden frame changes, you changed geometry, and that is a bug in
  the fix.

## R58 — a row highlight is a rectangle, and never bleeds past the panel

**Operator's words:** *"alternation and selection background is not properly rectangular and
sometimes overflows onto panels border"* — with a screenshot. The screenshot shows two distinct
symptoms on adjacent rows: highlight blocks of different widths on the two lines of one session's
row, and a highlight extending across the vertical divider.

**Spec:** `SPEC.md` §11.3's new "A row highlight is a rectangle" bullet, in `6584299`.

**Root cause — two bugs, one family, both read off the tree.** This is a real defect, not a spec
change, and the mechanism is known precisely:

1. **Not rectangular.** `settingsRenderRow` (`internal/tui/settings.go:1256`) opens the
   background SGR, writes the segments, and emits `\x1b[0m` **immediately after the last
   segment**. So the highlight is exactly as wide as that row's *text*. The two lines of a
   sidebar row have different text (line 1: name + badges + status; line 2: profile badge +
   `created …`), so they get two different widths — the ragged blocks in the screenshot. The
   padding that `sidebarContentLine` (`internal/tui/panel.go:324`) adds afterwards lands
   **after** the reset and is therefore unhighlighted.

2. **Bleeds across the seam.** `truncateToWidth` (`internal/tui/panel.go:201`) `break`s out of
   its loop the moment the next rune would overflow the budget — **discarding every escape
   sequence after that point, including the closing `\x1b[0m`.** So a truncated row leaves its
   background *open*: the ellipsis, `sidebarContentLine`'s trailing pad, and then the seam glyph
   all inherit it. `borderColor` emits a **foreground** SGR only, so it does not close the
   background — the divider is painted in the selection colour. This is why the screenshot's
   bleeding rows are precisely the truncated ones (`pytest-bdd-migration sa…`,
   `ongoing queries sampled…`).

**What to build.** Both, and note they are separable — fixing one without the other leaves a
visible bug, so do not treat this as one edit. The rectangle needs the background to span the
panel's full inner width on every line (pad **inside** the coloured span, not after it). The
bleed needs truncation to be honest about escapes: a truncated coloured run re-emits its own
reset, so no background can ever escape the run it was opened in.

**Success criteria.**
- Per-cell **background** assertions, at the cell level, not text: for a selected two-line row,
  **every** cell from the first column after the left border to the last column before the seam
  carries the `selection` background, **on both lines**. Not "the name is highlighted".
- The same for the alternating `surface` stripe on a non-selected row.
- The seam column's background is **unset** on a row whose text is long enough to be truncated.
  The fixture must contain a session name that actually truncates at the test's sidebar width —
  **if nothing truncates, this scenario passes with the bug present**, which is the trap. State
  in the report what name you used, at what width, and confirm the pre-fix run is red.
- A `truncateToWidth` unit test: a string with an open SGR span, truncated mid-span, returns a
  string whose SGR state is closed. Test the function directly — it is used well beyond the
  sidebar, so this is the fix's real blast radius.
- **Check the other call sites.** `padTrunc`/`truncateToWidth` serve dialogs, the preview crop,
  the settings takeover and the footer. Enumerate them; say which were also leaking; fix them.
  Requirement 58 is not "the sidebar looks right", it is "truncation does not leak backgrounds".
- `selection_idle` (focus elsewhere) gets the same rectangle treatment as `selection`.
- Golden frames: this fix changes **colour attributes, not cell contents**, so the plain-text
  golden frames must stay byte-identical. If one moves, you changed geometry.

---

# Non-negotiables

These are the standing rules of this codebase. Every one of them has been broken at least once in
an earlier phase and cost a repair task.

1. **`SPEC.md`, `prds/`, `ci/Dockerfile`, `ci/SPIKE.md` are protected.** Do not modify them. At
   close-out verify by **sha**, never by identity; the recognised set is `6584299` plus this PRD's
   commit; any other sha in that range is a hard failure to report, not to classify. Copy
   `docs/reports/phase3d-212-closeout/README.md`'s method.
2. **No scenario is deleted, skipped, or tag-excluded to make a suite pass.** `defaultTags` stays
   `"~@real-agents && ~@nightly"`. No `@flaky`, no `@wip`, no retry loop.
3. **A fix is root-caused, not padded.** No `sleep` added to make a test pass. If you bound a
   wait, bound it on the *observable consequence* of the thing you are waiting for, not on a
   proxy for "a render happened" — task 210's `029893a` is the worked example, and its report
   explains why `ResizeAndAwaitRender` alone was insufficient there.
4. **Ask "would this go red if the fix were reverted?" of every test you write, and answer it in
   the report by actually reverting and reproducing.** This phase has four requirements whose
   naive fixture cannot distinguish correct from incorrect behaviour, and the PRD names each one
   (R52's index-0 fixture, R53's orders-that-agree fixture, R57's missing neither-focused case,
   R58's nothing-truncates fixture). Those are the four places to spend the effort.
5. **A `DECK_*` env knob may override a config value and nothing more.** No new knob changes
   behaviour beyond the key it shadows.
6. **Every capability is reachable by keyboard.** No mouse-only affordance — R54 and R55 are
   mouse changes and this rule is what keeps them testable.
7. **Host load is a known confound.** A 1-min loadavg of 143.50 was recorded during Phase 3d and
   caused failures that looked like product bugs. Before attributing a flake to a code change,
   run the discriminating experiment at low load and record the loadavg. `docs/reports/
   phase3d-220-*` and `phase3d-210-ghost-completion-starting-race.md` are the two models: one
   found a real regression, one correctly found nothing and left the test alone.
8. **Reports carry citations by sha and log path**, and every cited sha resolves and every cited
   log path exists.

# Deliverables

- All seven requirements implemented, each with scenarios/tests as specified above.
- `docs/reports/phase3e.md` — per-requirement evidence table, real command output, tool versions,
  wall-clock, gotchas. Same shape as `docs/reports/phase3.md`.
- `docs/reports/phase3e-findings.md` — anything discovered that the PRD got wrong, any spec
  ambiguity, any defect found and not fixed (with why).
- A green whole-suite run at the final code commit: `ci/run.sh go test -p=1 -count=1 ./...`,
  exit 0, **every** package `ok` or `[no test files]`, cited by sha and log path.
- `ci/stability.sh 10` at that same final code commit. Phase 3d reached **10/10** and that is now
  the bar; publish the real rate, and if it is below 10/10, root-cause every failure to a
  mechanism rather than re-running for a streak. An honest 8/10 with two named mechanisms is worth
  more than a 10/10 nobody can explain.
- `git status --short` and `git log origin/main..HEAD` both empty at close-out.
