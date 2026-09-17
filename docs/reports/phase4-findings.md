# Phase 4 findings (R132)

This is the run's findings ledger: everything found during phase 4 (codex
adapter, chrome legibility, hook receiver, gutter) and deliberately not
fixed, plus the two items the PRD names by name as known-unverified, plus
the one place this approach found the PRD and SPEC disagreeing with each
other. Nothing else is claimed exhaustive.

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

## The PRD-R120-versus-SPEC-§11 permission-badge disagreement

**SPEC.md:1339** states line 2 of a sidebar row carries "`env↻` and
`launch↻` when either is dirty, the row's age, and the permission badge for
non-`safe` **last**" — i.e. the badge is shown only when the row's profile
is not `safe`; a `safe` row gets no badge at all on line 2.

**The PRD's R120 bullet** (`prds/phase4-codex-and-chrome.md:284`, under
"R120 — the row says less and means more") instead says the permission
badge "(`[safe]`, `[yolo]`, …) moves to the **end** of that line, after the
age" — its own example lists `safe` first among the badges that move there,
implying every applicable profile, including `safe`, renders one.

These two texts disagree: SPEC says `safe` gets no line-2 badge, the PRD's
R120 parenthetical implies `safe` gets one like every other profile. Per
this run's own standing precedence rule ("SPEC.md is the authority; where
the PRD and SPEC disagree, SPEC wins"), **SPEC.md:1339 is authoritative**,
and the PRD's R120 parenthetical is the one that is wrong.

Task 011 (`e93a790`, "tui: sidebar row's permission badge follows SPEC
§11's non-safe rule") is the code change that resolved this in deck's
favour of SPEC: `sidebarRowLines`' own call to `profileBadgeSegment`
(`internal/tui/tui.go`) now skips the badge entirely for a `safe` row,
citing SPEC.md:1339 in both the call site and `profileBadgeSegment`'s own
doc comment, while `plan`/`edits`/`yolo` rows still render their badge as
line 2's last segment. Every other caller of `profileBadgeSegment`
(`profileBadge`, used by the `i` detail dialog and its kin) is unchanged —
only the sidebar row's own line-2 composition follows the non-`safe` rule.
`internal/tui/profile_badge_safe_hidden_test.go`'s
`TestSidebarRowHidesSafeBadgeButKeepsNonSafeBadgeAtMinimumWidth` pins both
halves (no badge for `safe`, badge kept for the other three) at
`SidebarWidthFloor`. This same fix forced a golden-fixture regression
(the pre-fix `[safe]` badge baked into
`features/testdata/golden/side_by_side_80x24.golden`), corrected by task
011b (`1f38195` regenerates the golden; see the Inventory below for the
discovery and fix commits).

## Inventory

The command that defines this run's findings set is
`git log --grep='FINDING:' "$BASE..HEAD"`, with
`BASE=$(git log --format=%H --diff-filter=A -1 -- prds/phase4-codex-and-chrome.md)`
= `08a1ffe3eb229f8ebe5ba9791fbb3e3cec6e0c06` (the standing rules' own audit
command). Pasted verbatim, unedited, HEAD at `1ee3cbd`:

```
commit 0ca8667465fe8915169c1cc956c8e75cdf99014b
Author: Nik <nikolaiorl@gmail.com>
Date:   Thu Sep 17 03:26:10 2026 +0000

    docs: re-run gate/guards/stability at the settled tail sha (task 011b)
    
    All three sweeps re-run FROM SCRATCH at the new tail code sha 0ba550a
    (features/golden_frame_test.go's quiescence-based settle gate), overwriting
    the three report directories with clean results:
    
    - docs/reports/phase4-cure-final-suite/: unnarrowed `go test -p=1 -count=1
      -timeout=40m ./...`, exit 0, all 18 packages ok, 7m21s (02:02:30Z ->
      02:09:51Z). suite.log committed verbatim.
    - docs/reports/phase4-cure-guards/: `go build ./...` and `go vet ./...`
      silent at exit 0; `gofmt -l .` lists exactly the four pre-existing files
      measured at 08a1ffe and nothing this task wrote.
    - docs/reports/phase4-cure-stability10/: `ci/stability.sh 10` = 10/10
      passed, script exit 0, 1h12m24s (02:11:37Z -> 03:24:01Z). No FAIL line
      in any of the ten committed logs.
    
    The earlier 7/10 sweep's two golden-frame failures are fixed at the root by
    0ba550a, not waited out. Its third failure -- the one-off empty
    `capture-pane` in internal/tmux's
    TestSendKeysInvalidHexByteIsSilentlyDiscarded -- did not recur in these ten
    runs; it stays disclosed as an advisory finding via the FINDING: line in
    b98ce9c's body, which `git log --grep='FINDING:'` still picks up for task
    017's ledger. Ten clean runs do not erase that observation.
    
    Record-only commit: no *.go or *.feature file is touched, so the tail code
    sha stays 0ba550a and tasks 014-018 cite it from
    docs/reports/phase4-cure-final-suite/README.md.

commit b98ce9cad29f17462f181dcda72e40303ecdc140
Author: Nik <nikolaiorl@gmail.com>
Date:   Thu Sep 17 01:49:21 2026 +0000

    docs: re-run gate/guards/stability sweeps at the corrected tail sha (task 011b)
    
    Task 013's full-suite gate, task 014's guards and task 015's stability
    sweep, each re-run FROM SCRATCH at the corrected tail code sha e93a790
    (unchanged: git diff --stat e93a790 HEAD -- '*.go' '*.feature' prints
    nothing) now that the golden-fixture regression (task 011, fixed by this
    task's own prior commit 1f38195) is corrected. Per the standing rules,
    these are re-run under this task, never under 013/014/015 themselves --
    task 013's own report stands as history at 059704a.
    
    - docs/reports/phase4-cure-final-suite/: full gate PASS, exit 0, all 18
      packages ok, 7m32s wall-clock (00:17:13Z-00:24:45Z).
    - docs/reports/phase4-cure-guards/: go build/go vet clean, gofmt -l . lists
      only the four pre-existing files measured at plan time (08a1ffe).
    - docs/reports/phase4-cure-stability10/: 7/10 passed, 1h19m53s
      (00:27:12Z-01:47:05Z). Runs 2 and 3 recur the known-open golden-frame
      settle-race flake (advisory, unchanged from every prior sweep). Run 10
      hits a new, previously undisclosed flake:
    
    FINDING: internal/tmux/literal_send_test.go:123
    TestSendKeysInvalidHexByteIsSilentlyDiscarded read an empty capture-pane
    once in run 10/10 of this stability sweep (not reproduced in three
    immediate isolated re-runs) -- a new, previously undisclosed real-tmux
    timing flake, load-sensitive, disclosed as advisory; not a product
    regression and not caused by task 011b's golden-fixture change (unrelated
    package, no test in this file reads the golden fixture).
    
    The golden-fixture fix itself held in every one of the ten runs' byte
    comparisons -- both golden-frame failures above are the settle-check, not
    the fixture compare.

commit 059704a4649d41682dce05a3ef7b133c40016200
Author: Nik <nikolaiorl@gmail.com>
Date:   Thu Sep 17 00:01:13 2026 +0000

    docs: full-suite gate sweep at e93a790 finds a real golden-fixture regression (task 013)
    
    FINDING: features/golden_frame_test.go:91 TestGoldenMinimumFrame fails
    deterministically (3/3 attempts, 6/6 subtests) against
    features/testdata/golden/side_by_side_80x24.golden line 5, which still
    expects the pre-task-011 [safe] badge that task 011 (e93a790) correctly
    removed per SPEC.md:1339. Not the known-open transient-starting settle
    flake (which surfaced once, separately, in attempt 2's run-1 only).
    godog's own TestFeatures run is fully green (352 scenarios, 4168 steps).
    All other 17 test-bearing/no-test packages ok. Fix (regenerate the golden
    via UPDATE_GOLDEN=1 and re-run 013/014/015 fresh) proposed as a new task
    per standing rules; not done here since task 013 is record-only past the
    freeze line.

commit 0e72ec19d67f789466584e1666e7ba223c939609
Author: Nik <nikolaiorl@gmail.com>
Date:   Wed Sep 16 22:45:17 2026 +0000

    tui: paint the fill and crop marker beside a captured pane's own bytes (task 003)
    
    cropRow's padTrunc-equivalent fill (the columns past a captured row's own
    visible bytes when the row is narrower than the panel) and its cropMarker
    substitution (when the row overflows) were composed with plain, unpainted
    strings.Repeat(" ", ...) / cropMarker() text, inheriting whatever SGR
    state the pane's own bytes left open right up to the panel's own right
    border/pad -- previewContentLine/fullBoxPreviewContentLine's own outer
    reset only protects the border, not these columns cropRow itself draws
    inside the row.
    
    New paintForeignFill(s) resets (canvasResetIfPainting, so a colour-
    disabled build still emits no stray escape) then paints s with deck's own
    theme.Background; cropRow now routes its fill and its cropMarker (plus the
    degenerate all-marker row when the marker itself doesn't fit) through it,
    while the pane's own bytes are still only ever truncated, never
    re-composed or scanned for the pane's own resets.
    
    New internal/tui/preview_pane_fill_marker_test.go's
    TestCapturedPaneFillPastCaptureCarriesDeckBackground and
    TestCapturedPaneCropMarkerCarriesDeckBackground cover both layouts with a
    synthetic capture (paneRowOpenTail/paneRowOpenTailWide) that sets colour,
    resets mid-line, then leaves a THIRD colour open with no reset at all --
    proving the fill/marker columns pick up deck's background via the new
    explicit reset while the capture's own cells (including the one still
    carrying that open attribute) are never touched. Both failed against the
    pre-task implementation (deck-painted cells carrying the pane's own
    #141e2c foreground) and pass after it; both existing
    TestCapturedPane{SideBySide,Stacked}KeepsOwnColourFrameCarriesDeckBackground
    remain green.
    
    FINDING: cropPreviewBottomLeft's own geom line (panel.go, "WxH of
    realWxrealH") and its blank-fill rows for a pane smaller than the panel
    (panel.go's blank := strings.Repeat(...) loop) are still composed as plain
    text at a row still marked previewLineForeign -- outside this task's own
    scope (padTrunc fill/cropMarker beside a REAL capture's own bytes) and
    task 002's (the no-live-capture placeholder path only); left unpainted for
    the findings ledger.

commit 96b0ba9bfce151bea7e9d257d9ab2acdc482d6e9
Author: Nik <nikolaiorl@gmail.com>
Date:   Wed Sep 16 22:36:03 2026 +0000

    tui: paint deck-generated preview content (task 002)
    
    previewBodyLines now returns a second slice, []previewLineOwner, giving
    every returned row a provenance: previewLineDeckOwned for the no-session
    sentence, previewPlaceholderLines' own copy and the blank fitLines pad
    around either, previewLineForeign for a live capture (cropPreviewBottomLeft)
    or the live interactive grid (interactiveBodyLines). previewContentLine and
    fullBoxPreviewContentLine each take a new `owned bool` and, when owned,
    compose the WHOLE row -- both borders, both pad columns and text -- as one
    canvasBackground span exactly like every other chrome line in panel.go,
    instead of always taking the "foreign, never repaint" path that left the
    interior span outside canvasBackground even when nothing foreign was ever
    there. renderSideBySideFrame/renderStackedFrame thread the new per-row
    owners through to each call. Foreign (captured-pane) rows are untouched:
    same two-span-plus-explicit-reset composition as before.
    
    New tests, internal/tui/preview_no_capture_background_test.go, assert for
    every internal/theme.Builtins() theme that a frame with NO live capture
    (no session selected at all) carries theme.Background on every preview
    interior cell -- including the columns past the placeholder sentence's own
    text -- in both the side-by-side and stacked frames. Run against the
    pre-task implementation (previewContentLine/fullBoxPreviewContentLine with
    their old signatures, no owner routing) these fail for all five builtins in
    both layouts, e.g.:
    
        --- FAIL: TestNoLiveCapturePreviewInteriorCarriesDeckBackgroundSideBySide/empire
            preview_no_capture_background_test.go:78: theme "empire": preview
            interior cell (37,1) has no background at all, want deck's
            background #0f172a
        --- FAIL: TestNoLiveCapturePreviewInteriorCarriesDeckBackgroundStacked/empire
            preview_no_capture_background_test.go:123: theme "empire": preview
            interior cell (2,10) has no background at all, want deck's
            background #0f172a
    
    (and the matching failure for cobalt, daylight, matrix, parchment in both
    tests). After this commit's fix all ten subtests pass.
    
    internal/tui/preview_placeholder_test.go's own previewBodyLines call site
    is updated for the new two-return signature (the returned owners are
    unused there, `_`).
    
    `ci/run.sh sh -c 'go build ./... && go vet ./... && gofmt -l .'` is clean
    apart from the four pre-existing files named in the standing rules.
    `ci/run.sh go test -count=1 ./internal/tui/...` passes (3.9s).
    
    FINDING: task 003's own scope (per plan v7) still applies unchanged --
    cropPreviewBottomLeft's blank-fill rows past a short real capture, its
    "WxH of realWxrealH" geometry line, and the padTrunc fill columns/crop
    marker beside an actual capture's own bytes are all still marked
    previewLineForeign by this commit (no regression, but also not yet
    painted); that finer within-row split is task 003's own deliverable, not
    this one's.

commit fac1db08bd2ec7b8ab214d91f9ae2aa3a2332ac0
Author: Nik <nikolaiorl@gmail.com>
Date:   Wed Sep 16 21:40:04 2026 +0000

    docs: R132 findings ledger with the two unverified codex items (task 043)
    
    docs/reports/phase4-findings.md names the PRD's two known-unverified codex
    items (R132 non-goals) by name and reason: whether acceptEdits/plan/dontAsk
    are reachable on codex at all (no flag combination on real codex-cli 0.154.0
    reached them, per the spike report; not exhaustively probed, out of scope
    per the PRD's own non-goals), and whether the hook trust hash is stable
    across codex versions (every measurement this run took was against exactly
    0.154.0, seeding/harvesting codex's hook trust is out of scope per the PRD's
    non-goals, no later task revisited it).
    
    The file's Inventory section pastes verbatim the output of
    `git log --grep='FINDING:' $BASE..HEAD`, BASE = 08a1ffe3eb229f8ebe5ba9791fbb3e3cec6e0c06
    (`git log --format=%H --diff-filter=A -1 -- prds/phase4-codex-and-chrome.md`,
    the notes' audit rule) -- confirmed byte-identical to the file's own block via
    diff before committing. Below it, one entry per quoted FINDING line, each with
    a file:line and the reason it was left unfixed: internal/service/agent.go:67
    (resume never re-resolves a stored profile against current caps, task 025);
    features/panel_background_rectangle.feature:96-100 (blanket "selection"
    assertion stale once the R119 gutter bar claims columns 2-3, task 009, left
    for task 010's own gutter-colour scope); features/panel_background_rectangle.feature:125-160
    (SidebarWidthFloor header wrap shifts every row down one line, an existing
    wrapText interaction, task 008, documented in the scenario's own prose rather
    than changed); features/panel_background_rectangle.feature:1 (seam assertion
    stale after task 004's global canvas paint, out of scope for task 007's own
    theme-canvas claim, actually fixed same-day by task 008/903418a).
    
    Record-only commit, docs/reports/ only, verified with git show --stat before
    push.
    
    Pasted git log --grep='FINDING:' $BASE..HEAD output follows verbatim:
    
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
reason it was left unfixed. Some `FINDING:` lines appear twice in the paste
above — once inside `fac1db0`'s own body, which quoted four earlier commits
in full when it first built this ledger, and once again because those four
earlier commits are themselves still inside the `$BASE..HEAD` range and so
matched a second time in their own right. That duplication is a byproduct
of history, reproduced here exactly because the paste is verbatim; each
distinct finding gets one entry below regardless of how many times its text
appears above.

1. **`internal/tmux/literal_send_test.go:123`** (commit `b98ce9c`, task
   011b) — `TestSendKeysInvalidHexByteIsSilentlyDiscarded` read an empty
   `capture-pane` once, in run 10 of 10 of the 011b stability sweep taken at
   the (then still golden-fixture-broken) tail `e93a790`; not reproduced in
   three immediate isolated re-runs. Disclosed as an advisory, load-sensitive
   real-tmux timing flake, unrelated to that sweep's golden-fixture package.
   The later, clean `0ca8667` stability sweep (10/10, at the corrected tail
   `0ba550a`) did not see it recur, but per this run's own rule a later green
   does not erase an earlier red — it stays in the known-open flake list
   (`notes.md`'s gotchas).
2. **`features/golden_frame_test.go:91`** (commit `059704a`, task 013) —
   `TestGoldenMinimumFrame` failed deterministically (3/3 attempts, 6/6
   subtests) against `features/testdata/golden/side_by_side_80x24.golden`,
   which still expected the pre-task-011 `[safe]` badge that task 011
   (`e93a790`) correctly removed per SPEC.md:1339 (see "The PRD-R120-versus-
   SPEC-§11 permission-badge disagreement" above). A real regression, not
   left unfixed: task 011b regenerated the golden (`1f38195`,
   `UPDATE_GOLDEN=1`) and, once a second, independent settle-race issue in
   the same test surfaced, fixed it at the root (`0ba550a`). Kept as its own
   ledger entry — a since-fixed red stays in the record per this run's own
   rule — rather than folded into the badge-disagreement narrative above.
3. **`internal/tui/panel.go:809-810,816-818`** (`cropPreviewBottomLeft`'s
   `"WxH of realWxrealH"` geometry line and its `blank := strings.Repeat`
   fill loop; commits `96b0ba9` task 002 and `0e72ec1` task 003) — task 002
   painted the deck-owned placeholder path (no live capture at all) and
   flagged this residual forward to task 003; task 003 painted the crop
   fill/marker beside a REAL capture's own bytes but explicitly left
   `cropPreviewBottomLeft`'s own geometry line and its blank-fill rows (for a
   real capture smaller than the panel) still composed as plain,
   unpainted text at a row still marked `previewLineForeign` — outside both
   tasks' own stated scope. No later task in this plan claims that scope;
   left unpainted for this ledger as a genuine, disclosed residual (deck's
   canvas paint is not total for a live, undersized real capture's own
   geometry/blank-fill decoration).
4. **`internal/service/agent.go:67`** (commit `a11cc86`, task 025) —
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
5. **`features/panel_background_rectangle.feature:96-100`** (commit
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
6. **`features/panel_background_rectangle.feature:125-160`** (commit
   `903418a`, task 008) — at `SidebarWidthFloor` (24 columns,
   `internal/tui/layout.go:25`) the sidebar's own "socket: `<name>`" header
   line no longer fits `contentWidth` (22) and wraps to two physical rows
   (an existing `wrapText` interaction), shifting every session row down by
   one line versus the 35-column case. Not something this task's gutter
   change introduced; the new `SidebarWidthFloor` scenario's row numbers
   (5-6, not 4-5) were written to account for it, documented in the
   scenario's own prose, rather than changing `wrapText` or the header
   line's own layout.
7. **`features/panel_background_rectangle.feature:1`** (commit `cd6d566`,
   task 007) — the file's two seam "cell ... has no background set"
   assertions at column 35 stopped holding once task 004's global canvas
   paint gave every border cell, seam included, deck's own `background`
   token. Out of scope for task 007, which asserts a different claim
   (every theme paints the frame's whole canvas), not the
   selection/stripe rectangle this file protects; task 008 (commit
   `903418a`, same day) is the one that actually flips this file's seam
   assertion from "no background set" to "background".
