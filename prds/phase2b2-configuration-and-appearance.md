# Phase 2b-2 — configuration and appearance

## Goal

Make deck **configurable and legible**: the §11.4 dialog contract retrofitted onto the
dialogs that already exist, the §11.5 settings takeover generated from the same schema that
parses `config.toml`, and the §11.6 theme system with its picker and its quantised 16-colour
floor.

Where 2b-1 was geometry — rectangles, floors, row order, hit-testing — this phase is
**schema**: a key set, a dialog contract, a token set. It is judged by one question: **can the
operator change anything deck can be configured to do, and read the result, without opening a
text editor or squinting at undifferentiated grey text?**

This phase also fixes **the defects the operator found using 2b-1 by hand** and closes one
deferral 2b-1 recorded honestly (requirements 37–47). Those are not a side quest: they are the
part of the phase with a user waiting on it. Two of them — a frame that overflows its height,
and a live session stuck at `error` — make deck actively misleading, and one of those two
(requirements 43–47) is the most consequential correctness work in the phase.

`SPEC.md` §11.4, §11.5, §11.6 and §6.5 specify this down to the token. **Read them first.**

## The requirement everything else serves

**The schema is the single source of truth for parsing the file and for generating the
settings view.**

§6.5 states it and §11.5 restates it, because it is the one property a reasonable
implementation gets wrong: a settings view hand-written as a list of fields drifts from the
parser the first time a key is added, and the drift is silent in both directions — a key you
can set in the file and cannot see, or a field that shows in settings and is dropped on load.
`allow_yolo` reachable only by hand-editing a file is exactly the R7 violation this closes.

So `internal/config/toml.go` is **replaced** by a schema-driven parser, not extended. One
declaration per key carries its name, kind, bounds, default, description and scope; the parser
walks it and the settings view renders it. **Requirement 22 is the assertion**: a test that
enumerates the schema and fails if any flat key in it is unreachable in settings, or any field
in settings is absent from it. If that cannot be made to hold structurally, stop and record a
finding — do not maintain two lists and a comment asking the next person to keep them in sync.

## Context

### Where work happens

This job container has the docker CLI and the host docker socket; containers you start are
siblings on the host daemon, so every bind source must be a **host** path.
`$RALPHD_HOST_WORKSPACE` is the host path of the directory mounted here at `/workspace`.
`ci/run.sh` encapsulates this correctly — use it for all Go and tmux work
(`ci/run.sh go test ./...`). If the image is missing:
`docker build -t deck-ci:local -f ci/Dockerfile ci`. It carries Go 1.25.13 and tmux 3.5a.
Install no toolchain into this container.

### What already exists — do not rebuild it

- **The §11 two-panel shell** (Phase 2b-1, operator-verified): layout modes, panel chrome, the
  seam, the preview, the attention sort, workspace grouping, the collapsed strip, mouse
  navigation, and the one-line footer. This phase **colours and configures** that shell; it
  does not re-shape it. `internal/tui/panel.go` owns the box drawing, the escape-aware
  `stringWidth`/`truncateToWidth`, and `framedDialog`.
- **Five real dialogs**, all already routed through `framedDialog`: the create modal
  (`createView`), the `i` detail dialog (`detailView`), the `P` profile picker
  (`profileSwitchView`), the `p` pin dialog (`pinView`) and the `?` help overlay
  (`helpView`). These are the retrofit's whole surface. **There is no kill-confirm dialog and
  you must not add one** — `x` acts directly today, and kill/undo is Phase 3.
- **`internal/config`** parses the `DECK_*` controls and today's flat keys into
  `config.Settings`. The env-over-file precedence rule (§6.5) is already implemented and
  tested; preserve it exactly — §13.1 depends on it.
- **The godog harness** drives the released binary through a pty with a real tmux on a private
  socket, and can now resize the pty mid-scenario and synthesise SGR mouse reports (2b-1
  requirements 1–2). `ci/stability.sh 10` exists.
- **`cmd/fake-claude` / `cmd/fake-pi`** fire §8.1 events and render named fixtures verbatim,
  including the deterministic fake pane 2b-1 added for the preview.
- **The `internal/agent` probe corpus** (`internal/agent/probe.go`) with its fixtures under
  `internal/agent/testdata/probes/`. Requirement 38 changes pi's, and only pi's.

### Working practice: commit as you go

**This phase commits and pushes its own work** (see `docs/PLAN.md`). **One commit per completed
task**; messages say *why* and reference requirement numbers; the commit log is the durable
memory a later phase reads. `~/.gitconfig` and `~/.git-credentials` are already mounted, so
`git push origin main` works with no token handling from you — never put a token in a URL, a
file, or a message.

**Commit hygiene, not a commit count.** If review requires a correction to something already
committed, **add a follow-up commit**. Never force-push, amend, rebase or reset published
history: fix forward, always. Run build, vet and the suite before each commit.

Never commit build output, caches, secrets, or `SPEC.md` / `prds/` / `ci/Dockerfile` /
`ci/SPIKE.md`.

### Assertions this phase must deliberately change

Colour is bytes in the stream, and this phase puts bytes into every frame the suite reads.

| site | current assertion | after this phase |
|---|---|---|
| `features/testdata/golden/side_by_side_80x24.golden` and `features/determinism*` | byte-exact frames of an **uncoloured** 2b-1 layout | re-record under a **pinned theme and a pinned colour depth**. A golden frame is regenerated, never hand-edited, and the regeneration must be reproducible twice to identical bytes. State in the commit message which theme and which `DECK_COLOR_DEPTH` the frame is pinned to — a golden frame that does not pin both is a frame that will move when the default theme does |
| every `screen contains "…"` step in `features/` | matches plain text on an uncoloured screen | the screen now carries SGR sequences between and inside words. The harness's text extraction must keep reading the **grid's characters**, not the byte stream, so these keep working untouched. If any of them break, the fix is in the extraction, **not** in the assertion — an assertion relaxed to tolerate escape bytes stops testing the copy |
| `internal/tui/tui_test.go:22-42` (help copy) and `cmd/deck/main_test.go:361` (the same overlay through a real pty under `DECK_ASCII=1`) | pin the help overlay incl. the runtime-controls list | the overlay gains `,` (settings), `t` (theme picker), `DECK_COLOR_DEPTH` and `NO_COLOR`. Update both pins together |
| `internal/tui/tui_test.go:49`'s unavailable-action list | forbids copy for verbs the binary lacks | **`delete`, `send message`, `env editor`, `event log`, `filter list`, `snooze`, `archive`, `undo` and `tab` all stay on it.** Nothing this phase builds is on that list, and nothing may be removed from it. This test is the mechanism enforcing §11.3's footer rule — strengthen it, never relax it |
| `internal/tui/panel_test.go`'s `TestSidebarContentHasOneColumnPaddingBeforeSeam` and every column-arithmetic test in that file | count columns in plain strings | these read *visible width*, so they must keep passing **through** the SGR sequences a theme adds. `panel.go`'s `ansiEscapeLen`-aware width helpers already exist for this reason; if a colour breaks one of these tests, the colour is being applied in the wrong place |
| `features/permission_modes.feature:49` | asserts the full `yolo is not offered because allow_yolo is not enabled in config.toml` string at 220×30 | this copy names `config.toml` while settings now edits `allow_yolo` in place. If the copy changes so it is not a lie, **it must still be a full-string assertion at a geometry wide enough not to clip** — never shortened to a prefix. If the copy does not change, say why in the report |

Three rules about this table. First, **it is not exhaustive by construction** — sweep for
anything asserting screen bytes, frame stability or column arithmetic, and treat what you find
and it does not list as a finding. Second, **no assertion may be weakened to green a run**;
re-aim it and say in the commit message what it was protecting and how the new form still
protects it. Third, if re-aiming one would require changing behaviour `SPEC.md` specifies, that
is a finding, not a licence.

**The trap here is the inverse of 2b-1's.** In 2b-1 copy moved and left negative assertions
passing vacuously. Here **colour is invisible to a text assertion**: a theme applied to the
wrong token, or not applied at all, changes nothing any existing `screen contains` step can
see. Every colour claim in this phase must be asserted with a **per-cell attribute** read
(requirement 1) or it is not asserted. "The frame still renders and the suite is green" is
compatible with the theme system doing nothing whatsoever.

## Requirements

Each requirement must be individually verifiable by a command or scenario whose real output is
recorded in the phase report (requirement 49).

**Numbering is not build order. Requirements 1–6 are harness prerequisites and land first**,
because the colour requirements are unassertable without them.

### Harness prerequisites (build these first)

1. **Per-cell SGR attribute assertions** (§13.2). The harness can read a named cell's or a
   matched substring's **foreground colour, background colour and bold/dim/reverse flags** from
   the emulator's grid, and expose them as steps — e.g. "the cell at row R column C has
   foreground `#22c55e`", "the text `waiting` on the selected row has foreground
   `<token waiting>`". Steps must name **tokens**, not hex literals, wherever the assertion is
   about a token being *applied* rather than about a specific palette's value, so a theme's
   values can change without rewriting scenarios.
2. **`DECK_COLOR_DEPTH=truecolor|16`** (§13.1, `SPEC.md:1357`): forces the truecolour or the
   quantised path deterministically, regardless of what the harness terminal advertises. Both
   paths are behaviour and both must be drivable from a pty test. Documented in the help
   overlay like every other `DECK_*` knob.
3. **`NO_COLOR` honoured and assertable**: monochrome output, with §7 status carried by the
   glyph column alone. §11.6 makes the glyphs load-bearing precisely for this, so this is the
   scenario that proves they are.
4. **A theme-pinning step**: run a scenario under a named built-in theme, and under a **user
   theme written by the scenario** into `$XDG_CONFIG_HOME/deck/themes/`. Both discovery paths
   need to be drivable.
5. **A `config.toml`-writing step and a config-file-content assertion**: write a named config
   file before start-up, and assert the file's **parsed content** afterwards. Settings writes
   this file, and "the write is atomic and never leaves an unparseable file" (requirement 20)
   is unassertable without reading it back.
6. **A frame-budget assertion step**: assert that the rendered frame occupies **no more than
   the terminal's rows and columns** — no line longer than the width, no more lines than the
   height. Requirement 37 is a defect of exactly this shape, found by hand and not by the
   suite, because nothing asserted it outside the below-minimum case.

**Each of requirements 1–6 gets its own coverage in `features/harness.feature`**, which is
where Phase 0 established that harness capabilities prove themselves rather than being
trusted. A colour-reading step that silently returns the default attribute turns every scenario
built on it green for the wrong reason — and that failure is invisible in exactly the scenarios
meant to catch it. **This is the hazard requirement 1 in particular carries: assert that a
colour step can fail**, by reading a cell you know is a different colour.

### The dialog contract (`SPEC.md` §11.4)

7. **The five existing dialogs obey one contract.** `esc` cancels and changes nothing, `↵`
   submits, `tab`/`shift+tab` move between fields, `←`/`→`/`space` change a selection. Retrofit
   it onto `createView`, `detailView`, `profileSwitchView`, `pinView` and `helpView` — one
   implementation they share, not five that agree today.
8. **`esc` changes nothing, proven by state.** For each dialog: open it, alter every field it
   has, `esc`, and assert the session row, the store and `config.toml` are all unchanged.
   "Nothing happened" is the assertion, and it is one a wrong implementation passes by accident
   only if it is asserted on the state rather than on the screen.
9. **Additional load-bearing keys only where §11.4 states them inline.** §5's `y` yolo confirm
   is the canonical case and already exists in `profileSwitchView` and the create modal; it
   stays, and it is documented in-dialog. No dialog invents a key the contract does not give it
   and the dialog does not name on screen.
10. **In-dialog validation retains what was typed.** A rejected value is re-presented with the
    reason, never cleared and never silently corrected. The create modal's cwd validation is the
    existing case (see requirement 39).
11. **The mouse can neither cancel nor confirm a dialog** (§11.4, §11.8). `internal/tui/tui.go`
    already implements this; this phase keeps it true across the retrofit and asserts it for
    each of the five dialogs, with a click at the dialog's own border, at its buttons if it has
    any, and outside it.
12. **Width is 80% of the viewport clamped to `[26, 80]`.** Assert at a narrow, a middle and a
    wide terminal, including the geometry where the clamp binds at each end. `framedDialog`
    grows to fit its content today (`features/permission_modes.feature:49` documents a
    201-column box) — reconcile that with the clamp, and if content genuinely cannot fit the
    clamped width, wrap it. A dialog wider than its clamp is a defect; a truncated message that
    loses the reason is a worse one.
13. **Dialogs for unbuilt behaviour are absent, not stubbed** (§11.3's footer rule). No kill
    confirm, no env editor, no rules dialog, no path picker beyond requirement 39's minimum.
14. **Destructive actions confirm and name what survives.** Nothing in this phase's surface is
    destructive except discarding unsaved settings (requirement 20), which is where this rule
    is asserted. If you find another, it is a finding — not a licence to build Phase 3's kill
    confirm.

### The settings takeover (`SPEC.md` §11.5, §6.5)

15. **`,` opens a full-screen takeover**, not a modal: a category list on the left, the
    selected category's fields on the right. It is not a §11.4 dialog and does not pretend to
    be one.
16. **Navigation, exactly as §11.5 spells it out**: `tab`/`←`/`→` switch between the category
    list and the field list, `↑`/`↓` move within the focused list, `/` fuzzy-searches every
    field by label **and** description, `ctrl+s` saves, `esc` prompts to discard if anything
    changed and otherwise closes.
17. **Every flat key in `config.toml` is editable here**: `allow_yolo`, `stale_after`,
    `capture_min_interval`, `[ui] theme`, `[ui] ascii`, `[ui] mouse`, `[ui] recent_cwd_limit`,
    and the `[env]` table. **`[notify]` is the stated exception** — it appears as a single
    navigable entry whose action is unavailable this phase (§11.3's footer rule: absent, not
    stubbed, and the entry says so honestly rather than opening an empty dialog). Phase 5 owns
    it.
18. **Field kinds are explicit**: toggle, integer with bounds, string, path, enum (cycled),
    list-of-strings, and *link*. Each field states what it does and what changes when it
    changes.
19. **Scope is labelled per field**: global (`config.toml`), or per-session override where one
    exists (§6.1). A field that only takes effect on the next launch says **restart-to-apply**,
    consistent with §6.2 and `P`. A setting that claims to have taken effect on a live pane
    when it has not is the same class of lie as a fabricated status — so for each field, the
    report states which it is and the scenario proves the claim.
20. **Save is explicit and atomic.** `ctrl+s` (or the Save action) writes; `esc` with unsaved
    changes prompts to discard; the write is atomic and **can never leave an unparseable
    `config.toml`**. Assert the file's parsed content after a save, after a discard (unchanged),
    and after a save that races nothing — plus a test that an interrupted write leaves the
    previous file intact.
21. **Environment still outranks the file** (§6.5). A key set in the environment shows in
    settings as overridden-by-environment, and editing it does not pretend to change the running
    value. §13.1 depends on env-over-file, so this is the requirement that keeps the harness
    honest once settings can write the file.
22. **Schema/settings parity is structural, not documented.** A test enumerates the schema and
    fails if any flat key is unreachable in settings or any settings field is absent from the
    schema. Adding a key to the schema and forgetting settings must turn the suite red.
23. **An unknown key is ignored; an unparseable value for a known key is a stated error naming
    the file and line** (§6.5). Never a silent fallback to the default. Settings must not delete
    unknown keys it did not understand when it saves — a config written by a newer deck survives
    a save by an older one.
24. **Settings edits configuration and nothing else.** It cannot create, kill, resume, attach or
    delete a session; nothing in §9's lifecycle is reachable from it. Assert this by driving
    every key the takeover binds and confirming the session set is untouched.

### Themes (`SPEC.md` §11.6)

25. **A theme is one TOML file** with the semantic token set §11.6 declares. Built-ins are
    embedded in the binary; user themes live in `$XDG_CONFIG_HOME/deck/themes/*.toml` and are
    discovered at start-up. **Adding a built-in is a one-file drop plus one registry entry** —
    no per-theme code and no per-theme test. Ship at least two built-ins (a dark and a light) so
    that claim is demonstrated rather than asserted.
26. **The token set is exactly §11.6's**, and **the seven status tokens are exactly §7's seven
    statuses**. A status rendered in a colour borrowed from another status is a defect: assert
    each of the seven independently, per-cell.
27. **`[ui] theme` selects**, editable in settings and from the **`t` picker**, which previews
    the theme live on the real list while you move through the options and **reverts on `esc`**.
    The revert is the assertion: preview, `esc`, and the frame's attributes are byte-for-byte
    what they were.
28. **An unknown or unparseable theme name falls back to the default and says so** — on first
    paint, and in the health view if one exists yet (it does not; Phase 7). It never silently
    renders the default as though the chosen theme had applied. This is the theme system's
    version of a fabricated status.
29. **Truecolour when advertised; the 16-colour floor otherwise**, quantised at load time to
    the nearest of the 16 ANSI colours by Euclidean RGB distance **against §11.6's declared
    reference palette** (the xterm defaults, listed there). The quantised palette is what
    renders. Drive both paths from a pty with `DECK_COLOR_DEPTH`.
30. **Legibility after quantisation is a tested property with §11.6's stated method**: for every
    built-in theme, `text`, `hint`, `title` and each of the seven status tokens hold a WCAG
    contrast ratio ≥ 3:1 against `background`, and `text` against `selection`, computed over
    **both** the hex palette and its quantisation to the reference palette. A loader-level golden
    test over data, like §7's probe fixtures.
31. **`NO_COLOR` drops to monochrome** and status is then carried by the glyph column alone.
32. **A theme cannot change layout, spacing, glyphs or keybindings.** It is colour only. The
    proof is that 2b-1's column-arithmetic tests and the golden frame's **geometry** are
    unchanged under every built-in theme: assert the frame's cell *positions* are identical
    across themes while its attributes differ.
33. **Every colour in the render code comes from a token.** No hex literal and no
    `lipgloss.Color("…")` outside the theme package and its data. This is what makes requirement
    32 true by construction rather than by discipline, so assert it mechanically — a test or a
    grep-based check that fails on a colour literal in `internal/tui`.
34. **Labels are distinguishable from values, everywhere a view shows a pair.** The `i` detail
    dialog, the settings field list, the pin and profile dialogs and the footer all render
    `label: value` pairs that are currently one undifferentiated colour, which is the operator's
    stated complaint. Labels render in `hint`, values in `text`, and the distinction is asserted
    per-cell in at least the detail dialog and the settings field list. §11.6's token list has no
    dedicated `label` token — **using `hint` for this is a decision this PRD makes, not a licence
    to add a token**; record it in findings as a §11.6 clarification request for the operator.
35. **The list itself is legible, not just colourful.** `title` for the panel titles, `group` for
    workspace headers, `border`/`border_focus` for chrome, `selection`/`selection_idle` for the
    selected row in the focused and unfocused panel, `key` for footer keycaps, `hint` for footer
    descriptions, `badge`/`badge_warn` for the live/sampled, `env↻` and non-safe permission
    badges, `dimmed` for `starting` rows and elided detail, `search_match` where search exists
    (it does not yet; Phase 7 — the token is unused, and an unused token is fine, an unused
    token silently used for something else is not).
36. **Themes and `DECK_ASCII` are independent.** Every colour assertion holds under
    `DECK_ASCII=1`, and the ASCII frame's geometry is unchanged by any theme. 2b-1's pty help
    test already runs in ASCII mode; keep it that way.

### Defects found by using 2b-1 (fix these; they have a user waiting)

37. **A message rendered outside the panels must be inside the frame budget.** `mainView`
    appends `m.attachError` and `m.resumeNote` after the panels and before the footer, but
    `computeLayout` reserves rows only for the footer and the startup banner. So any attach
    error or resume note pushes the frame one line past the terminal's height, the terminal
    scrolls, and **the sidebar's top border scrolls off the top without any resize** — the
    operator's report, reproduced exactly:

    ```
    at 100x30: clean frame = 30 lines, with attachError = 31 lines, terminal height = 30
    at 100x30: clean frame = 30 lines, with resumeNote  = 31 lines, terminal height = 30
    ```

    The comment at `internal/tui/tui.go:882-891` already explains this exact hazard **for the
    startup banner** and reserves rows for it; the same reasoning was never applied to these two
    messages. Fix it generally: **every row the frame emits is budgeted**, including a message
    that wraps to more than one line. Then prove it with requirement 6's step over a matrix of
    sizes and message lengths — including a message long enough to wrap at 80 columns — and
    extend `TestBelowMinimumFrameStaysWithinBudget` from the below-minimum case to the general
    one. A frame that exceeds its height by one line is invisible to every assertion the suite
    has today, which is why a person found it and the suite did not.

38. **A `pi` session never leaves `starting`.** `internal/agent/probe.go`'s pi rules are fitted
    to **invented fixtures**: `internal/agent/testdata/probes/pi/*.txt` are hand-written panes
    containing `pi coding agent`, `> `, `Working · ctrl-c to stop`, `Allow tool execution?` and
    `Starting pi…`. A real `pi` (0.84.2) prints **none** of them, so no rule ever matches, the
    probe returns no verdict, and the row honestly — and permanently — stays `starting`. This is
    spec-conformant behaviour on top of a corpus that describes an agent that does not exist:
    §7 forbids inferring `running` from a live pane, so the row is right and the rules are
    wrong. Claude's rules match real Claude strings, which is why claude works.

    A real pi 0.84.2 idle pane ends with a two-line status footer of this shape (captured
    100×30, cwd `/tmp`, box-drawn separators above it):

    ```
    /tmp
    0.0%/1.0M (auto)                                  (amazon-bedrock) eu.anthropic.claude-opus-5 • high
    ```

    Three things are required, and the third matters most:

    - **Refit pi's rules against recorded captures of the real binary**, and commit those
      captures as the fixtures, replacing the invented ones. Record the pi version each fixture
      was captured from **in the fixture or beside it** — a probe corpus with no provenance is
      how this defect happened. The default suite must not invoke a real pi (see Constraints):
      the fixtures are committed data, exactly as §7's probe fixtures already are.
    - **Make `cmd/fake-pi` render panes derived from those captures**, so the fake stops being
      self-consistently wrong. A fake that agrees with a fixture neither of which resembles the
      real thing is a closed loop that tests nothing.
    - **Make a total probe miss visible instead of silent.** A row that has been sampled
      repeatedly with **no rule matching at all** is diagnosably different from a row that has
      simply had no signal yet, and today they are indistinguishable — which is why this went
      unnoticed from 14 to 21 August. Surface it where a user can see it: the `i` detail dialog
      states that the pane was sampled and matched no rule, with the sample's age. §7 supplies
      no copy for this, so **propose the copy in findings** for the operator to fold into the
      spec; do not change any status value, any `status_source`, or §7's transition table.

    Only the states you can capture without driving a live agent are in scope for a refit. For
    any pi state you cannot capture, **say so in the report and leave the rule out** rather than
    inventing a marker — an invented marker is precisely the defect being fixed. Note also that
    pi's `Error:` rule is broad enough to match any pane in which any command printed `Error:`,
    which would flip a working session to `error`; claude's equivalent is the narrower
    `API Error:`. Narrow it or record why it is safe.

39. **A `~`-prefixed working directory is rejected as non-existent.** The create modal stats
    `m.createCWD` verbatim (`internal/tui/tui.go:1769`), and nothing in the codebase expands a
    leading `~`, so `~/Projects/invp-ops-dev-agents` fails with `working directory … does not
    exist` for a directory that exists. §11.7 states that a leading `~` expands. **Scope is the
    minimum**: expand a leading `~` (and `~/`) to the user's home directory when validating and
    when submitting, in the create modal and in any other path field this phase's settings
    introduces, and store the **resolved absolute path**. `~otheruser` is out of scope — reject
    it with a reason rather than half-expanding it. **Everything else in §11.7 stays in Phase
    3**: no `recent_cwds` table, no prefill, no `↑`/`↓` cycling, no ghost completion, no `tab`.
    Assert the expansion in the create modal through a pty, and assert the store holds the
    resolved path, not the tilde.

40. **`features/crash.feature:57` flakes about one run in thirty.** The step
    `the private tmux session "deck_unattended-victim" does not exist` resolves to
    `privateSessionDoesNotExist` (`features/assertions_test.go:267`), a **one-shot**
    `tmux has-session` with no polling. The scenario SIGKILLs the fake agent inside the pane and
    tmux tears the session down asynchronously, so the check can sample before teardown lands.
    Observed: the 2b-1 job's two `ci/stability.sh 10` runs and the review's were 10/10, and an
    independent 10/10 attempt got **9/10**, failing at this line. The error reads
    `still exists: ` with an empty tail, which is not a second bug — `has-session` prints
    nothing on success. **Poll with a deadline**, the way the screen-based `stops containing`
    steps already do, and apply the same treatment to the other caller of the same helper
    (`features/walking_skeleton.feature`, after an explicit TUI kill — likelier safe, same
    shape). This predates 2b-1 and is recorded in no findings file, which is itself worth
    fixing: it means "requirement 43 verified 10/10" was recorded as durable when it was not.

41. **Requirement 43 is re-established after requirement 40, not before.** A stability run that
    predates the flake fix is not evidence about the flake. See requirement 48.

42. **The focused panel's border colour cue (2b-1's requirement 19) lands here.** 2b-1 recorded
    it as *unobservable and explicitly not verified*, deferred to this phase because it needs
    theme tokens and per-cell attribute reads, both of which now exist. §11.3's focused panel
    draws its border in `border_focus` and the unfocused one in `border`; the selected row uses
    `selection` in the focused panel and `selection_idle` in the unfocused one. Note the main
    view has **one** focusable region (§11.3, which is why `tab` is unbound), so the second
    surface this needs is the settings takeover's two lists — assert the cue there, and assert
    that the main view's single region does not advertise a focus move. `docs/reports/
    phase2b1-findings.md:931-949` records the deferral; close it explicitly in this phase's
    findings.

### Status recovery after an in-session resume (found live, 21 Aug)

The operator resumed an existing Claude conversation **from inside** a running deck session
(claude's own `/resume`), and the row went to `error` while the session itself carried on
working fine. The full causal chain is in deck's own event log, and it is one root cause with
three defects hanging off it. Session `magpie`, `claude`, cwd `~/Projects/invp-ops-dev-agents`:

```
seq 69  11:26:36  launch.ready
seq 70  11:26:38  session_start   reason=startup   conversation 859ac95b…
seq 71  11:27:01  session_end     reason=resume    conversation 859ac95b…   <- claude's own /resume
seq 72  11:27:01  session_start   reason=resume    conversation 1f6876ea…   <- same pane, new conversation
seq 73  11:27:04  user_prompt_submitted            conversation 1f6876ea…
seq 74  11:27:49  launch_lease_acquired  reason=user                        <- operator pressed `r`
seq 75  11:27:49  launch.failed   "…tmux -L deck new-session -d -s deck_magpie …: exit status 1: duplicate session: deck_magpie"
seq 76  11:28:02  attached
seq 77  11:30:11  stop                             conversation 1f6876ea…   <- alive and working
seq 78  11:31:11  notification    reason=idle_prompt                        <- still alive
```

and the row, read out of the store afterwards:

```
status = "error"   status_source = "tmux"   status_at = 11:27:49
status_reason = "resume session \"magpie\": create session \"deck_magpie\": … duplicate session: deck_magpie"
conversation_id = "859ac95b-a11f-4969-a4c9-951ffb95937e"      <- the DEAD conversation
```

`internal/hookrecv/receiver.go:39-46` is where this is decided. `SessionEnd` maps to `stopped`
from any state, so seq 71 stopped the row. `SessionStart` carries
`AllowedFrom: ["starting"]`, so seq 72 — arriving in the same second, for a live pane — was
**refused** and the row stayed `stopped`. `UserPromptSubmit` allows only `["idle","error"]`, so
seq 73 was refused too. The operator then did the reasonable thing with a live session showing
`stopped` and pressed `r`; the tmux session already existed, so the launch failed and wrote
`error`. From there `Stop` and `Notification` both allow only `["running"]`, so seq 77 and 78
were refused as well — `status_at` stayed at 11:27:49 while the last proof of life was 11:31:11.
The row was eventually cleared, but **only by the human typing another prompt**:
`UserPromptSubmit` is the one mapping whose `AllowedFrom` includes `"error"`, so a prompt at
11:34:27 took it to `running` and the `Stop` at 11:40:59 to `idle`. So the defect is not that the
row is permanently stuck — it is that **no agent-side hook can clear it**, and the row lies about
a live, working session for as long as the user does not happen to submit a prompt. That was
seven minutes here and is unbounded in general.

43. **An in-session resume or clear is not a session end.** Claude's `/resume` and `/clear`
    end its conversation and start a new one **in the same pane**: the tmux session, the pane
    and the process all survive. `SessionEnd` with `reason` = `resume` (and `clear`, and any
    other reason that is not the process going away) must therefore be recorded as an event and
    **must not stop the row**. Deriving "stopped" from the pane's own liveness is what deck
    already does correctly elsewhere; a hook reason is not evidence a pane died. §8.1's table
    says "session end → `stopped`" without enumerating reasons, and the reason set is an
    upstream contract — so **record the reason taxonomy you implement, and any reason you chose
    to treat as terminal, in findings** for the operator to fold into the spec.
44. **The stored `conversation_id` must follow the live conversation.** After the in-session
    resume, deck held `859ac95b…` while claude was on `1f6876ea…`. Durable identity is Phase 1's
    entire reason to exist, and a later `r` would have resumed the dead conversation — silently,
    and with the transcript the operator wanted left behind. A `SessionStart` for a session deck
    already owns updates `conversation_id` to the id in the payload, and the change is recorded
    as an event so the history says which conversation the row was on when.
45. **A live hook always outranks a stale non-hook verdict.** §7's precedence is
    `user-terminal > hook > probe > tmux`, but the per-event `AllowedFrom` allow-lists defeat it:
    a row sitting at `error` from a `tmux`-sourced launch failure refuses `Stop`, `Notification`
    and `SessionStart`, which is exactly the state observed. A hook arriving **now** is fresher
    than any probe or tmux verdict by construction, so it must be applied. Re-derive the allowed
    transitions from §7's transition table and its precedence rule, rather than from a
    hand-maintained per-event list — and keep the property the allow-lists were protecting: a
    hook must not resurrect a session the **user** deliberately killed (`killed_by_user`,
    `user-terminal` precedence), and an orphan is still an orphan. State in the report which
    allow-list entries you removed and which §7 rule now does that entry's job.
46. **Starting or resuming a session whose tmux session already exists is not an error.** `r`
    on a live session must be a no-op that says so — deck already owns that session; adopt it,
    do not `new-session` over it. `duplicate session: deck_<name>` reaching the user as a status
    is a deck bug reported as an agent failure. §9.3's launch lease is about *concurrent*
    launchers; this is the *already-launched* case and needs its own check before the lease is
    even taken.
47. **A launch failure must be clearable and must not masquerade as an agent error.** Whatever
    status a failed launch writes, subsequent higher-precedence truth (requirement 45) must be
    able to replace it, and the row must not sit at `error` for a session that is demonstrably
    alive. If §7 has no distinct state for "deck could not launch this", record that in findings
    rather than inventing one — the requirement here is recoverability, not a new status.

### Scenarios that define this phase

48. **`features/settings.feature`** — `,` opens the takeover; every flat key is reachable and
    editable; `/` finds a field by description as well as by label; a save writes
    `config.toml` and the parsed file matches; `esc` with changes prompts and discards; an
    env-overridden key is labelled and does not lie; a restart-to-apply field says so; the
    `[notify]` entry is honestly unavailable; and nothing in the session set changes throughout.
49. **`features/themes.feature`** — a built-in applies, asserted per-cell across the seven
    status tokens plus `border_focus`, `selection`, `title`, `group`, `key` and `hint`; a user
    theme in `$XDG_CONFIG_HOME` is discovered; an unknown name falls back **and says so**; `t`
    previews live and reverts on `esc`; `DECK_COLOR_DEPTH=16` renders the quantised palette;
    `NO_COLOR` renders monochrome with glyphs carrying status; and the frame's geometry is
    identical under every built-in.
50. **`features/dialogs.feature`** — the §11.4 contract asserted **per dialog**: `esc` changes
    nothing (proven against the store), `↵` submits, `tab` moves between fields, the width clamp
    at both ends, the mouse neither cancelling nor confirming, and validation retaining what was
    typed.
51. **`features/status_recovery.feature`** — requirements 43–47 as one scenario per link in the
    observed chain, driven through the fake agent's hook path: an in-session resume
    (`SessionEnd` reason=`resume` immediately followed by `SessionStart` reason=`resume`, same
    pane) leaves the row **running, not stopped**, and moves `conversation_id` to the new
    conversation; `r` on a session whose tmux session already exists reports "already running"
    and writes no `error`; a row sitting at `error` from a failed launch **recovers** on the next
    `Stop` or `Notification`; and a session the **user** killed is not resurrected by a late
    hook. `cmd/fake-claude` already fires the §8.1 event set, so this needs no real agent —
    extend it to emit the end/start resume pair.
52. **The golden minimum frame** stays green and is re-recorded under a pinned theme and a
    pinned `DECK_COLOR_DEPTH`, at exactly **80×24**, byte-exact, regenerated (never hand-edited)
    and reproducible to identical bytes twice.
53. **Existing scenarios keep passing**, including `layout_modes`, `preview`, `attention_sort`,
    `mouse`, `harness`, `status_probe` and every Phase 0–2 file. This phase adds colour to frames
    those scenarios read; requirement 32 is what keeps them green for the right reason. It also
    changes transition policy (requirement 45), so **any Phase 2 assertion that depends on a hook
    being refused must be re-aimed with its reasoning stated, not deleted** — if one of them was
    pinning the behaviour that made `magpie` unrecoverable, say so explicitly.

### Evidence and stability

54. **The suite is green ten consecutive times from a clean state** — `ci/stability.sh 10`,
    10/10, with the real output in the report, **run after requirement 40's fix**. If any run
    fails, the failure and its diagnosis go in the report; a 9/10 recorded honestly is worth
    more than a 10/10 that hides a known race, and this project has already learned that the
    expensive way.
55. `docs/reports/phase2b2.md` records, per requirement number, the command or scenario that
    verifies it and its **real output** — not a claim that it passes. Include: the schema/
    settings parity mechanism (requirement 22), the quantisation table with its computed
    contrast ratios (requirement 30), the pi fixtures' provenance and which pi states could not
    be captured (requirement 38), the theme + colour depth the golden frame is pinned to
    (requirement 52), and — for requirement 45 — the before/after transition policy with the §7
    rule that justifies each change.
56. The help overlay is updated (R7): `,`, `t`, `DECK_COLOR_DEPTH`, `NO_COLOR`, and every key
    the settings takeover and the theme picker bind. The overlay is deck's only documentation,
    so it is part of the deliverable, not a comment.
57. No root-owned files left in the workspace; no leftover tmux sockets; `git status` clean at
    the end. **Container hygiene needs no action from you and you must not attempt it**: every
    sibling `ci/run.sh` starts is `--rm`, so there is nothing to clean up. Check it read-only
    and scoped to the image:
    `docker ps -a --filter ancestor=deck-ci:local --format '{{.ID}} {{.Status}}'`.
    **A sweep filtered on `label=ralphd.run=…` deletes your own container** — the job container
    carries that label — which SIGKILLs this job mid-iteration, loses the verdict, and has now
    done so twice on this project. Verifying this requirement means running the read-only
    command above and reading its output. Nothing else.

## Review guidance

Block on **real defects**; do not block on documentation nits. Wording, formatting and stale
sentences in derived summaries are recorded as notes and do not fail a phase. The test is
whether a reader would be **misled about behaviour**. Phase 0b was rejected three times over
three sentences with a finished product in the tree; that is a bug in the process, not
diligence.

In this phase specifically, these are blocking:

- **A colour claim asserted by reading deck's own code, its logs, or a plain-text screen match
  rather than the emulator's per-cell attributes.** The theme system can do nothing at all and
  leave every text assertion in the suite green; per-cell reads are the only thing that
  distinguishes "themed" from "compiles".
- A colour step that cannot fail — one that returns a default attribute when it cannot read the
  grid, so its assertions pass whatever the screen holds.
- **Two lists of config keys** — a parser's and a settings view's — kept in sync by discipline
  rather than by construction (requirement 22).
- A settings save that can leave an unparseable `config.toml`, or that drops unknown keys.
- Settings pretending an edit took effect on a live pane when it did not, or omitting a
  restart-to-apply label where one is due.
- A theme that changes geometry, spacing, glyphs or a keybinding, or a colour literal reaching
  `internal/tui` outside the theme package (requirement 33).
- An unknown theme name silently rendering the default without saying so.
- A quantisation implemented against anything other than §11.6's declared reference palette, or
  a contrast claim computed over only one of the two palettes.
- **A golden frame regenerated by hand-editing bytes**, or made stable by excluding the coloured
  region, or recorded without pinning both the theme and the colour depth.
- A weakened or deleted Phase 0–2b-1 assertion instead of a re-aimed one; a full-string
  assertion shortened to a prefix; or a screen assertion relaxed to tolerate escape bytes
  instead of the extraction being fixed to read the grid.
- Anything removed from `internal/tui/tui_test.go:49`'s unavailable-action list.
- **Requirement 37 "fixed" by shrinking the panels unconditionally** rather than by budgeting
  the rows a message actually occupies, or fixed without a test that a *wrapped* multi-line
  message also fits.
- **Requirement 38's pi rules refitted against invented markers again** — any new marker not
  traceable to a recorded capture of a real pi, or a fixture committed without its provenance.
  Equally blocking: making the row leave `starting` by inferring `running` from a live pane,
  which §7 forbids and which would be a fabricated status.
- Requirement 39 expanded into Phase 3's §11.7 work (recent directories, ghost completion,
  `tab`), or a tilde "expanded" by string-prefixing `$HOME` without resolving the result.
- Requirement 40 fixed by raising a timeout rather than by polling to a deadline, or the
  stability evidence for requirement 54 collected before that fix.
- **Requirement 45 "fixed" by widening every `AllowedFrom` list to accept everything.** The
  lists exist for a reason and one of those reasons is load-bearing: a late hook must never
  resurrect a session the user deliberately killed (`user-terminal` outranks `hook`). A
  transition policy re-derived from §7 is the fix; `AllowedFrom: []string{…all…}` is the same
  defect in the other direction, and it is the tempting one.
- Requirement 43 implemented by treating **every** `SessionEnd` as non-terminal. A real session
  end must still stop the row; the distinction is the reason field plus the pane's own liveness,
  and the taxonomy chosen must be recorded in findings.
- Requirement 44 left as "the id is updated in memory" — a `conversation_id` that is right in
  the running process and wrong in `state.db` fails the moment deck restarts, which is the
  scenario Phase 1 exists for.
- Requirement 46 implemented by deleting the `duplicate session` error path rather than by
  checking for the existing session **before** taking the launch lease, or by swallowing a
  genuine tmux failure that is not a duplicate.
- A new harness capability (requirements 1–6) used by scenarios but not itself covered in
  `features/harness.feature`.
- Building any of Phase 3 (kill confirm, `dd`, undo, env editor, rename, event log, `recent_cwds`),
  Phase 5's notification rules dialog, or Phase 7's search and health view.

## Findings, not spec edits

**`SPEC.md` is read-only to this job.** Record in `docs/reports/phase2b2-findings.md` anything
the spec left undefined that you had to decide, anything contradictory or impossible, and every
deferral with its target phase. Likely candidates, so they are not surprises: the absence of a
dedicated `label` token in §11.6 (requirement 34); the copy for a pane that matched no probe
rule (requirement 38); how §11.4's `[26, 80]` width clamp reconciles with content that does not
fit it (requirement 12); where the settings takeover's discard prompt sits given it is not a
§11.4 dialog; what the theme picker previews when the list is empty; the exact copy for a
theme that failed to load; **which `SessionEnd` reasons are terminal and which are an
in-session restart** (requirement 43), since §8.1's table does not enumerate them and the set is
an upstream contract; and the copy for `r` on a session that is already running (requirement
46). Close 2b-1's requirement-19 deferral explicitly (requirement 42).

If a new `DECK_*` control is needed to make something testable, introduce it, document it in
the help overlay, and record it as a finding for the operator to fold into `SPEC.md` — do not
silently invent configuration, and do not edit the spec to match your code.

**A hazard warning belongs inside the requirement whose verification triggers it**, not only in
a general note — 2b-1 learned this the hard way when a container-hygiene sweep filtered by the
job's own label killed the job twice despite a warning in the Constraints section.

## Non-goals for this phase

Do not implement, even partially: Phase 3's create-modal completion, §11.7 path entry beyond
requirement 39's tilde minimum, the env editor, kill/undo, `dd` tombstones, reap, purge,
archive, bulk marks, rename or the event log; the Codex adapter (Phase 4); notification
channels, rules, the outbox or the `[notify]` rules dialog (Phase 5); scrollback replay, history
files or `last_cwd` (Phase 6); send-without-attach (§11.1), cross-session search or the health
view (Phase 7); systemd units.

Do not re-shape 2b-1's layout, do not make the preview interactive, do not add a keystroke path
into a previewed pane, and **do not add a user-facing command line** (R7). `↵` is how a user
gets a real terminal, and that is the whole design.

## Constraints

- Commit and push as described. **Fix forward; never rewrite published history.**
- Do not modify `SPEC.md`, anything under `prds/`, `ci/Dockerfile`, or `ci/SPIKE.md`.
- Do all building and testing in siblings via `ci/run.sh`. Install nothing into this container.
- **The default suite must not depend on a real agent binary, network access, or model output.**
  Only `@real-agents` scenarios may touch an installed CLI, and they are excluded by default.
  Requirement 38's pi work is done against **committed captures**, not a live pi — there is no
  pi in the CI image and you must not add one.
- deck never writes to or deletes anything inside a session's `cwd` (R1).
- Every tmux server you start is on a **private socket** and is killed afterwards. Never touch
  the default socket.
- Every sibling container is `--rm`, so no cleanup sweep is needed or wanted. **Never run
  `docker rm` / `docker kill` filtered by `label=ralphd.run=…`, and never target the container
  named `ralphd-deck-phase2b2`: your own job container carries that label, so such a sweep
  SIGKILLs the job mid-iteration and loses the verdict.** To check for leftovers, do it
  read-only and scoped to the image:
  `docker ps -a --filter ancestor=deck-ci:local --format '{{.ID}} {{.Status}}'`.
- Prefer a small, readable implementation over a complete one. This phase is judged by whether
  the operator can configure deck from inside deck and read the result at a glance — not by
  feature count.
