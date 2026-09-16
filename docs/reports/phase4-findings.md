# Phase 4 findings (R132)

This is the run's findings ledger: everything found during phase 4 (codex
adapter, chrome legibility, hook receiver, gutter) and deliberately not
fixed, plus the two items the PRD names by name as known-unverified. Nothing
else is claimed exhaustive.

## The two known-unverified codex items (PRD R132 / non-goals)

1. **Whether `acceptEdits`/`plan`/`dontAsk` are reachable on codex at all.**
   No flag combination on the real codex-cli 0.154.0 (spiked in
   `docs/reports/codex-cli-0.154.0-spike.md`) was found that puts codex into
   an `acceptEdits`, `plan`, or `dontAsk` permission mode — codex 0.154.0
   has no plan mode and the adapter's declared caps (`internal/agent/codex.go`)
   do not offer those profiles. This was not probed exhaustively against
   every possible codex CLI flag/config combination (out of scope per the
   PRD's non-goals: "Codex has no plan mode on 0.154.0 and no flag
   combination reached `acceptEdits`/`plan`/`dontAsk` (R132 records this as
   unverified)"), so absence-of-evidence is recorded here rather than
   claimed as proof of impossibility.
2. **Whether the hook trust hash is stable across codex versions.** All
   measurements this run took (the hook payload shape, the trust-hash gate
   `--dangerously-bypass-hook-trust`, and the receiver's mapping) were taken
   against exactly one codex-cli build, 0.154.0. Nothing in this run
   installed or spiked a second codex version to confirm the hash algorithm
   or its trust-file format is unchanged release to release; the PRD's own
   non-goals list "seeding codex's hook trust, harvesting its trust hash" as
   out of scope, and no later phase-4 task revisited it, so version
   stability stays unverified rather than assumed.

## Inventory

The command that defines this run's findings set,
`git log --grep='FINDING:' $BASE..HEAD`, with
`BASE=$(git log --format=%H --diff-filter=A -1 -- prds/phase4-codex-and-chrome.md)`
= `08a1ffe3eb229f8ebe5ba9791fbb3e3cec6e0c06` (the notes' audit rule). Pasted
verbatim, unedited:

```
commit a11cc860a61a72db6956fbd6580b6c5e9b4cbe55
Author: Nik <nikolaiorl@gmail.com>
Date:   Wed Sep 16 13:36:41 2026 +0000

    tui/features: drive codex's plan degradation through ResolveProfile (task 025)
    
    R127 rejection cure. features/permission_modes.feature's codex scenario
    used to create the session with "safe" and then hand-write the degradation
    sentence into permission_profile_reason via sessionMarkedDegraded
    (features/agent_steps_test.go:1589), so it asserted a fixture's own copy of
    the wording and a regression in the generic degrade path would have stayed
    green. It now drives the only surface that can hold an unsupported request
    at all -- the create modal, where a profile is REQUESTED -- with keystrokes
    only: choose "plan" on claude (which declares it), then cycle the Agent
    field onto codex (which does not), and the modal degrades the field and
    says why.
    
    internal/tui/tui.go:
    - createProfileRequested remembers the profile the user explicitly cycled
      to, so a later Agent change can still explain what was asked for.
    - degradedCreateProfile takes the fallback VALUE from
      agent.Caps.ResolveProfile (the same generic call service.CreateAgent
      makes) whenever the adapter is the reason it is unavailable, and
      options[0] only for a profile the adapter does declare but config
      withholds (yolo without allow_yolo), where ResolveProfile has nothing to
      say. Behaviour for that second case is unchanged.
    - createProfileDegradeNote returns ResolveProfile's own reason string,
      re-derived from the selected adapter's caps on every render rather than
      copied into internal/tui, and both render paths (createBody and
      styledCreateBody) print it on its own dedicated line under the
      Permission profile row, for the same wrap reason createNameReuseWarning
      has its own line. No codex-specific branch anywhere, no plan-to-safe
      alias in the adapter, no new config knob.
    
    features/permission_modes_codex_test.go adds the two keystroke steps the
    scenario needs (open the modal on an agent with a chosen profile without
    submitting; cycle the Agent field), registered in godog_test.go. The
    scenario also asserts the fallback took EFFECT: the created row is stored
    "safe" and its launch argv carries codex's own safe pair (-a on-request -s
    workspace-write) with no "plan" and no "--last" token.
    
    internal/tui/create_profile_degrade_test.go pins the same path at unit
    level against ResolveProfile's own return values, never literals, and
    proves the note is derived live (cycling back to claude clears it).
    
    Verified the scenario is not a fake green: with createProfileDegradeNote
    stubbed to return "", permission_modes.feature fails on the
    screen-contains step (frame dump shows the modal snapping plan->safe
    silently); restored before committing.
    
    Re-ran all four of this task's feature files in the CI container after the
    tui change and replaced their logs (the earlier one-line logs measured a
    tree without it), each
      ci/run.sh sh -c 'DECK_GODOG_PATHS=<file> go test -count=1 -v -run TestFeatures ./features/'
    all green, under docs/reports/phase4-scenario-logs/*-task025.log. Also
    green: internal/tui package tests, dialogs/create_session/create_cwd_ghost/
    create_cwd_tab features (the other create-modal consumers), go build, go
    vet ./..., gofmt (pre-existing 4-file list unchanged).
    
    FINDING: internal/service/agent.go:67's ResolveProfile degrade is
    unreachable for an ordinary create through the released TUI -- the create
    modal now degrades and explains first (internal/tui/tui.go's
    degradedCreateProfile), and internal/service/resume.go:223 never
    re-resolves a stored profile, so a row whose stored profile later drifts
    out of its adapter's declared set fails the relaunch with the adapter's
    own "unsupported permission profile" error instead of SPEC §5's
    degrade-to-safe. Left as is: making resume re-resolve is a behaviour
    change beyond R127.

commit 06b4521d7447314dad154127543d11007621db3c
Author: Nik <nikolaiorl@gmail.com>
Date:   Wed Sep 16 09:50:23 2026 +0000

    tui: the gutter's four colour states, R119 (task 009)
    
    sidebarGutterBar (tui.go) paints the two-line gutter bar SPEC §11.3
    requires: `accent` when the row is selected (line 1's `>` drawn with
    `background` as its own foreground), `badge` when marked but not
    selected (line 2's `✓`/`*` under DECK_ASCII, same treatment), no
    painted background at all when the row is neither, and `accent` again
    -- never `badge` -- when a row is both, selection winning the bar's
    one colour across both lines per SPEC's own wording. sidebarRowLines now
    looks up m.marked[session.ID] and calls the new helper instead of the
    plain two-space/arrow strings task 008 left in its place.
    
    New internal/tui/sidebar_gutter_color_test.go covers all four states at
    SidebarWidthFloor (24 columns, the tightest gutter/truncation budget),
    read per-cell off a real vt.Emulator grid (renderSettingsToEmulator/
    cellFgHex/cellBgHex/tokenHex/findRowContainingInSidebar/findCol, the
    existing internal/tui colour-test idiom). Red-first verified by hand:
    swapped accent<->badge in the switch (both selected- and marked-only
    tests failed on the wrong hex), then separately swapped the glyph's
    foreground token from `background` to `title` (all three
    foreground-asserting tests failed on the wrong hex) -- both reverted
    after, `git diff --stat` clean, tests re-passed green.
    
    Fixes fallout in sidebar_row_fill_test.go: task 321/R58b's own
    full-width-selection-background tests asserted every column from the
    border to the seam on a selected row equals `selection`/
    `selection_idle`, which the new `accent` gutter bar (columns 2-3 of
    every row) now deliberately breaks by design -- the two tests gain a
    gutterHex exception for exactly those two columns; the third (stripe,
    unselected/unmarked) test needed no change since its gutter carries no
    bar at all.
    
    FINDING: features/panel_background_rectangle.feature's two
    @requirement-58 scenarios assert background token "selection" across
    columns 1-34 (35-col case) and 1-23 (SidebarWidthFloor case) of a
    selected row, columns which now include the gutter's own `accent` bar
    (cols 2-3) -- stale the same way task 007's own commit predicted for
    task 004's canvas paint. Left for task 010 (R119's feature-level gutter
    scenario), which already owns writing new gutter-colour scenarios and is
    the natural place to also flip this file's now-wrong selection-column
    span, rather than fixed here (this task's own scope is internal/tui unit
    tests only).
    
    Verified: ci/run.sh go test -count=1 ./internal/tui/ ok; guards
    (build/vet/gofmt) show only the four pre-existing gofmt files; audit
    silent; HEAD will equal origin/main after push.

commit 903418a8c35ace557147da4fe50d8a9394339a58
Author: Nik <nikolaiorl@gmail.com>
Date:   Wed Sep 16 09:40:50 2026 +0000

    tui: gutter moves outside the row's text run, R119 (task 008)
    
    sidebarContentLine gains a gutter parameter (panel.go), composed OUTSIDE
    padTrunc's own text budget: joined as border + pad + gutter + padded-text +
    pad, with the row's own content width shrunk by the gutter's width before
    padTrunc ever sees it, so a long name's truncation can never synthesise a
    reset inside the gutter's own span (SPEC \u00a711.3). sidebarRowLines now
    returns a gutter string per physical line alongside text/bg -- line 1's
    selection arrow moves out of the text segment into gutter1; line 2's
    gutter2 is blank here (task 009 gives it the mark glyph). The `\u2713
    marked` text badge that used to be appended to line 1's own badge run
    (task 112) is gone -- SPEC \u00a711.3 moves the mark cue into the gutter's
    own second line instead, task 009's job, not this one's.
    
    features/panel_background_rectangle.feature's two existing scenarios
    update their seam assertion (column 35) from "no background set" to
    "background" -- stale since task 004's global canvas paint, task 007's
    own commit message already predicted this -- and gain a third scenario at
    SidebarWidthFloor (24 columns, the tightest gutter/truncation budget),
    proving the highlight rectangle stays unbroken with the gutter in its own
    columns. FINDING: at width 24 the sidebar's own "socket: <name>" header
    line no longer fits contentWidth (22) and wraps to two physical rows,
    shifting every session row down by one versus the 35-column case -- an
    existing wrapText interaction, not something this task's gutter change
    introduced; the new scenario's row numbers (5-6, not 4-5) account for it,
    documented in the scenario's own prose.
    
    Run log: docs/reports/phase4-scenario-logs/gutter-rectangle.log (3
    scenarios/41 steps green in the CI container).
    
    Verified: ci/run.sh go test -count=1 ./internal/tui/ ok; guards
    (build/vet/gofmt) show only the four pre-existing gofmt files; audit
    silent; HEAD will equal origin/main after push.

commit cd6d566b2c603fb3c5e9440cdcf685953f58f5f0
Author: Nik <nikolaiorl@gmail.com>
Date:   Wed Sep 16 09:02:25 2026 +0000

    features: assert every built-in theme paints the frame's whole canvas (task 007)
    
    R118: features/panel_background_themes.feature adds one scenario per
    internal/theme/registry.go builtinFiles entry (empire, daylight, matrix,
    cobalt, parchment), each asserting the theme's background token across
    every deck-owned cell of a rendered frame -- both border rows in full,
    and each content row's sidebar+seam+pad (cols 0-36) and preview
    border+pad (cols 98-99) -- including the pad columns past session
    "aaa"'s own short row text, reusing cellsRangeHaveBackgroundToken and
    resolveScenarioTokenHex verbatim from cell_attributes_token_test.go.
    Two more scenarios render the plain startup frame under NO_COLOR and
    under DECK_COLOR_DEPTH=16 and reuse theme_frame_geometry_test.go's
    screen-text-matches step to show borders and text land in the same
    cells regardless of colour. Deliberately excludes preview columns
    37-97 from every claim: previewContentLine composes that span without
    canvasBackground by design (task 006's captured-pane exception), so
    it is never deck-owned for this claim's purposes.
    
    Red-first verified by hand: temporarily removed panel.go's
    sidebarContentLine bg=="" fallback to theme.Background (task 004's own
    line) and reran -- got a real failure (cell "|" has no background
    colour set) at row 2 col 0 on the parchment scenario, then reverted
    and reran green; no product code changed by this task.
    
    Run log: docs/reports/phase4-scenario-logs/panel-background-themes.log
    (7 scenarios, 77 steps, all passed, in the CI container).
    
    FINDING: features/panel_background_rectangle.feature:1 -- its two
    seam "cell ... has no background set" assertions at column 35 now
    fail after task 004's global canvas paint (every border cell,
    including the seam, now correctly carries background); out of scope
    here since that file asserts the selection/stripe rectangle, not this
    task's plain-canvas claim.
```

## Entries

One entry per quoted `FINDING:` line above, each with a file:line and the
reason it was left unfixed.

1. **`internal/service/agent.go:67`** (commit `a11cc86`, task 025) —
   `ResolveProfile`'s degrade-to-safe fallback is unreachable through the
   released TUI for an ordinary create: the create modal
   (`internal/tui/tui.go`'s `degradedCreateProfile`) now degrades and
   explains the profile before `service.CreateAgent` is ever called, and
   `internal/service/resume.go:223` never re-resolves a stored profile
   against the adapter's current caps, so a session whose stored profile
   later drifts out of its adapter's declared set fails resume with the
   adapter's own "unsupported permission profile" error instead of SPEC
   §5's degrade-to-safe. Left as is: making resume re-resolve is a
   behaviour change beyond R127's own scope.

2. **`features/panel_background_rectangle.feature:96-100`** (commit
   `06b4521`, task 009) — the file's two `@requirement-58` scenarios
   asserted background token `"selection"` across the full content span
   (columns 1-34 in the 35-column case, 1-23 at `SidebarWidthFloor`) of a
   selected row; the new gutter bar (task 009) now paints columns 2-3 of
   that same span with its own `accent` token instead, making the blanket
   `selection` assertion stale. Left for task 010, which already owned
   writing the new gutter-colour scenarios and was the natural place to
   also flip this file's now-wrong selection-column span, rather than
   fixed inside a task scoped to `internal/tui` unit tests only. (Task 010
   did subsequently narrow the columns-2-to-3 exception into this same
   scenario — see the file's current lines 96-99 — but the finding stands
   as a historical entry in the ledger per the run's own rule that a later
   green does not erase an earlier red from the record.)

3. **`features/panel_background_rectangle.feature:125-160`** (commit
   `903418a`, task 008) — at `SidebarWidthFloor` (24 columns,
   `internal/tui/layout.go:25`) the sidebar's own "socket: `<name>`" header
   line no longer fits `contentWidth` (22) and wraps to two physical rows
   (an existing `wrapText` interaction), shifting every session row down by
   one line versus the 35-column case. Not something this task's gutter
   change introduced; the new `SidebarWidthFloor` scenario's row numbers
   (5-6, not 4-5) were written to account for it, documented in the
   scenario's own prose, rather than changing `wrapText` or the header
   line's own layout.

4. **`features/panel_background_rectangle.feature:1`** (commit `cd6d566`,
   task 007) — the file's two seam "cell ... has no background set"
   assertions at column 35 stopped holding once task 004's global canvas
   paint gave every border cell, seam included, deck's own `background`
   token. Out of scope for task 007, which asserts a different claim
   (every theme paints the frame's whole canvas), not the
   selection/stripe rectangle this file protects; task 008 (commit
   `903418a`, same day) is the one that actually flips this file's seam
   assertion from "no background set" to "background".
