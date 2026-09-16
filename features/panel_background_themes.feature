Feature: every built-in theme paints its own `background` token across the whole canvas, not just the row highlight (requirement 118, task 007)

  Tasks 004-006 (R118) made deck paint its own canvas -- the `background`
  token composed across every cell deck itself draws (borders, padding,
  row bodies, the empty state, dialog interiors) instead of leaving those
  cells to whatever the terminal's own background happens to be (SPEC
  §11.3, "deck paints its own canvas") -- and proved it with internal/tui
  unit tests over Model.View() in isolation. This file is that same claim's
  godog evidence, read per cell off a REAL running client (never
  text-scraping), across EVERY built-in theme internal/theme/registry.go's
  builtinFiles lists today (empire, daylight, matrix, cobalt, parchment),
  in the features/panel_background_rectangle.feature + cell_attributes_
  token_test.go idiom: the existing per-cell/per-range background-token
  steps (cellsRangeHaveBackgroundToken, task 323/R58d), reused rather than
  re-implemented, resolved against each scenario's own pinned theme
  (resolveScenarioTokenHex) so this keeps passing if a built-in's palette
  is retuned later.

  `[ui] sort_order = "name"` and `group_by_workspace = false` pin a
  deterministic, ungrouped order (the same pin panel_background_rectangle.
  feature already uses): session "aaa" sorts first (position 0) and stays
  UNSELECTED (task 301's newest-session auto-select lands on "zzz-session",
  created second), and position 0 is never stripe-tinted (task 084's
  pos%2==1 rule) -- so "aaa"'s own two rendered lines carry nothing but the
  plain `background` token, with no selection/stripe override to carve an
  exception out of the "every cell" claim below. "aaa" is short enough
  (3 characters against a 32-column sidebar content width) that most of
  its own row is PAD, not text -- exactly the pad-columns-outside-a-short-
  row's-text case R118's own fix targeted, per canvasBackground's doc
  comment in panel.go.

  At the harness's default terminal (100 columns, 30 rows) with one line
  reserved for the footer, the box occupies rows 0-28: row 0 is the top
  border, row 28 is the bottom border, and every row in between is a
  content row. Column 0 is the sidebar's own left border, columns 1-34 are
  its content (SPEC requirement 17's padding included), column 35 is the
  shared seam (the preview's own left border -- SPEC §11.3 "the preview's
  left border *is* the divider"), column 36 is the preview's own leading
  pad column, and columns 98-99 are the preview's own trailing pad and
  right border. Columns 37-97 are the preview's own INNER text/capture
  area, and this file DOES assert a background over them too, deck's own
  placeholder copy included -- panel.go's previewBodyLines (task 002/B1)
  marks that placeholder (the no-session sentence, its wrapped text, and
  the blank fitLines pad around it) deck-owned, and previewContentLine
  composes a deck-owned row as one canvasBackground span, border to
  border, columns 37-97 included. The one true exception is SPEC §11.3's
  own -- "captured pane output is never repainted" (task 006): once a
  session is selected and its pane actually has a live tmux capture on
  file, previewBodyLines hands that capture's own foreign bytes to
  cropPreviewBottomLeft instead, and THOSE columns 37-97 are left exactly
  as the pane left them, unpainted by deck. Each scenario below therefore
  asserts columns 37-97 right after its client starts and before either
  session exists -- previewBodyLines' empty-session branch, always
  deck-owned, per task 002/B1's own internal/tui proof
  (TestNoLiveCapturePreviewInteriorCarriesDeckBackground*) -- then, once
  "aaa" and "zzz-session" exist and the auto-selected "zzz-session" has a
  real live capture sitting in that same span, goes back to asserting only
  0-36 and 98-99, the columns that stay deck-owned regardless of which
  branch previewBodyLines took. Every column this file asserts over on a
  content row -- 0-36 and 98-99 unconditionally, 37-97 while no live
  capture is present -- and 0-99 on both border rows, is composed through
  panel.go's canvasBackground helper.

  GOTCHA (discovered writing this file, out of scope here): panel_
  background_rectangle.feature's two "cell ... has no background set"
  assertions at column 35 (the seam) currently fail after tasks 004-006's
  global canvas paint -- column 35 is a border cell like any other and
  now correctly carries `background` (SPEC §11.3 lists "borders" among
  what the canvas paints), which task 004's own commit message already
  flagged as a predictable side effect ("any OLD test asserting 'no
  background' ... is now WRONG by design"). That file's own two scenarios
  are unrelated to this task's success criteria (they assert the
  selection/stripe RECTANGLE, not the plain-canvas claim), so fixing their
  now-stale "no background set" assertions is left for whichever task
  next runs the whole suite (the Tier 1 gate) to pick up.

  @requirement-118-canvas-background
  Scenario: the "empire" built-in theme's background token fills every deck-owned cell of the frame, pad columns past a short row's own text included
    Given the scenario's config.toml is written with:
      """
      [ui]
      theme = "empire"
      sort_order = "name"
      group_by_workspace = false
      """
    And deck client "bgempire" is started with colour enabled
    And deck client "bgempire" cells at row 2 columns 37 to 97 have background token "background"
    When deck client "bgempire" creates shell session "aaa"
    And deck client "bgempire" creates shell session "zzz-session"
    Then within one configured reconcile interval deck client "bgempire" screen contains "running"
    And deck client "bgempire" cells at row 0 columns 0 to 99 have background token "background"
    And deck client "bgempire" cells at row 28 columns 0 to 99 have background token "background"
    And deck client "bgempire" cells at row 2 columns 0 to 36 have background token "background"
    And deck client "bgempire" cells at row 3 columns 0 to 36 have background token "background"
    And deck client "bgempire" cells at row 2 columns 98 to 99 have background token "background"
    And deck client "bgempire" cells at row 3 columns 98 to 99 have background token "background"
    And deck client "bgempire" cells at row 6 columns 0 to 36 have background token "background"
    When deck client "bgempire" exits cleanly

  @requirement-118-canvas-background
  Scenario: the "daylight" built-in theme's background token fills every deck-owned cell of the frame, pad columns past a short row's own text included
    Given the scenario's config.toml is written with:
      """
      [ui]
      theme = "daylight"
      sort_order = "name"
      group_by_workspace = false
      """
    And deck client "bgdaylight" is started with colour enabled
    And deck client "bgdaylight" cells at row 2 columns 37 to 97 have background token "background"
    When deck client "bgdaylight" creates shell session "aaa"
    And deck client "bgdaylight" creates shell session "zzz-session"
    Then within one configured reconcile interval deck client "bgdaylight" screen contains "running"
    And deck client "bgdaylight" cells at row 0 columns 0 to 99 have background token "background"
    And deck client "bgdaylight" cells at row 28 columns 0 to 99 have background token "background"
    And deck client "bgdaylight" cells at row 2 columns 0 to 36 have background token "background"
    And deck client "bgdaylight" cells at row 3 columns 0 to 36 have background token "background"
    And deck client "bgdaylight" cells at row 2 columns 98 to 99 have background token "background"
    And deck client "bgdaylight" cells at row 3 columns 98 to 99 have background token "background"
    And deck client "bgdaylight" cells at row 6 columns 0 to 36 have background token "background"
    When deck client "bgdaylight" exits cleanly

  @requirement-118-canvas-background
  Scenario: the "matrix" built-in theme's background token fills every deck-owned cell of the frame, pad columns past a short row's own text included
    Given the scenario's config.toml is written with:
      """
      [ui]
      theme = "matrix"
      sort_order = "name"
      group_by_workspace = false
      """
    And deck client "bgmatrix" is started with colour enabled
    And deck client "bgmatrix" cells at row 2 columns 37 to 97 have background token "background"
    When deck client "bgmatrix" creates shell session "aaa"
    And deck client "bgmatrix" creates shell session "zzz-session"
    Then within one configured reconcile interval deck client "bgmatrix" screen contains "running"
    And deck client "bgmatrix" cells at row 0 columns 0 to 99 have background token "background"
    And deck client "bgmatrix" cells at row 28 columns 0 to 99 have background token "background"
    And deck client "bgmatrix" cells at row 2 columns 0 to 36 have background token "background"
    And deck client "bgmatrix" cells at row 3 columns 0 to 36 have background token "background"
    And deck client "bgmatrix" cells at row 2 columns 98 to 99 have background token "background"
    And deck client "bgmatrix" cells at row 3 columns 98 to 99 have background token "background"
    And deck client "bgmatrix" cells at row 6 columns 0 to 36 have background token "background"
    When deck client "bgmatrix" exits cleanly

  @requirement-118-canvas-background
  Scenario: the "cobalt" built-in theme's background token fills every deck-owned cell of the frame, pad columns past a short row's own text included
    Given the scenario's config.toml is written with:
      """
      [ui]
      theme = "cobalt"
      sort_order = "name"
      group_by_workspace = false
      """
    And deck client "bgcobalt" is started with colour enabled
    And deck client "bgcobalt" cells at row 2 columns 37 to 97 have background token "background"
    When deck client "bgcobalt" creates shell session "aaa"
    And deck client "bgcobalt" creates shell session "zzz-session"
    Then within one configured reconcile interval deck client "bgcobalt" screen contains "running"
    And deck client "bgcobalt" cells at row 0 columns 0 to 99 have background token "background"
    And deck client "bgcobalt" cells at row 28 columns 0 to 99 have background token "background"
    And deck client "bgcobalt" cells at row 2 columns 0 to 36 have background token "background"
    And deck client "bgcobalt" cells at row 3 columns 0 to 36 have background token "background"
    And deck client "bgcobalt" cells at row 2 columns 98 to 99 have background token "background"
    And deck client "bgcobalt" cells at row 3 columns 98 to 99 have background token "background"
    And deck client "bgcobalt" cells at row 6 columns 0 to 36 have background token "background"
    When deck client "bgcobalt" exits cleanly

  @requirement-118-canvas-background
  Scenario: the "parchment" built-in theme's background token fills every deck-owned cell of the frame, pad columns past a short row's own text included
    Given the scenario's config.toml is written with:
      """
      [ui]
      theme = "parchment"
      sort_order = "name"
      group_by_workspace = false
      """
    And deck client "bgparchment" is started with colour enabled
    And deck client "bgparchment" cells at row 2 columns 37 to 97 have background token "background"
    When deck client "bgparchment" creates shell session "aaa"
    And deck client "bgparchment" creates shell session "zzz-session"
    Then within one configured reconcile interval deck client "bgparchment" screen contains "running"
    And deck client "bgparchment" cells at row 0 columns 0 to 99 have background token "background"
    And deck client "bgparchment" cells at row 28 columns 0 to 99 have background token "background"
    And deck client "bgparchment" cells at row 2 columns 0 to 36 have background token "background"
    And deck client "bgparchment" cells at row 3 columns 0 to 36 have background token "background"
    And deck client "bgparchment" cells at row 2 columns 98 to 99 have background token "background"
    And deck client "bgparchment" cells at row 3 columns 98 to 99 have background token "background"
    And deck client "bgparchment" cells at row 6 columns 0 to 36 have background token "background"
    When deck client "bgparchment" exits cleanly

  # The two scenarios below are this task's other half: rendering the SAME
  # frame shape under NO_COLOR and under DECK_COLOR_DEPTH=16 changes colour
  # only, never geometry -- the borders and text stay in exactly the same
  # cells. They deliberately render the plain, session-less startup frame
  # (rather than repeating the "aaa"/"zzz-session" setup above) because
  # comparing two independently-started clients' screen text only holds if
  # both reach an identical selection state, and a freshly started client
  # loading pre-existing sessions from a shared store selects index 0 (the
  # first sorted row) rather than replaying task 301's newest-session
  # auto-select the CREATING client went through -- a second client that
  # joined after "aaa"/"zzz-session" already existed would disagree with
  # the first client about which row is marked selected, for a reason with
  # nothing to do with colour depth. The plain startup frame carries no
  # selection at all, so this comparison is exactly a geometry/content
  # proof, exactly as features/theme_frame_geometry_test.go's own doc
  # comment states of the identical step it reuses here.
  @requirement-118-canvas-background
  Scenario: NO_COLOR changes only colour, never which cells carry deck's borders and text, on the same canvas-painted frame
    Given the scenario's config.toml is written with:
      """
      [ui]
      theme = "empire"
      """
    And deck client "bgnc-colour" is started with colour enabled
    And deck client "bgnc-mono" is started
    Then deck client "bgnc-colour" screen text matches deck client "bgnc-mono" screen text
    When deck client "bgnc-colour" exits cleanly
    And deck client "bgnc-mono" exits cleanly

  @requirement-118-canvas-background
  Scenario: DECK_COLOR_DEPTH=16 changes only colour, never which cells carry deck's borders and text, on the same canvas-painted frame
    Given the scenario's config.toml is written with:
      """
      [ui]
      theme = "empire"
      """
    And deck client "bgdepth-colour" is started with colour enabled
    And deck client "bgdepth-16" is started with colour enabled and colour depth "16"
    Then deck client "bgdepth-colour" screen text matches deck client "bgdepth-16" screen text
    When deck client "bgdepth-colour" exits cleanly
    And deck client "bgdepth-16" exits cleanly
