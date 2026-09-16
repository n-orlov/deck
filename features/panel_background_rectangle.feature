Feature: A selected/striped sidebar row's background fills a real rectangle, and the seam stays clear (requirement 58, task 323)

  Tasks 320-322 (R58a-c) fixed truncateToWidth/padTrunc so a highlighted
  row's background span closes itself instead of leaking past a
  truncation point or a padded line's own end, and made every row's
  background span the WHOLE inner width rather than stopping at the
  text. This file is R58d's godog evidence for the same claim, read
  per-cell off a real running client (never text-scraping): every column
  from the first after the sidebar's left border to the last before the
  seam carries the row's background on BOTH physical lines of a
  two-line row, for both the selection highlight and the alternating
  surface stripe, even on a row whose name is long enough to actually
  truncate (the fixture trap R58's own tasks documented: a fixture where
  nothing truncates cannot discriminate a leak fix from no fix at all,
  since there is nothing to leak past). The seam column itself, one
  column further, belongs to the PREVIEW's own left border -- task
  004/R118's global canvas paint gives every border cell, seam included,
  deck's own `background` token (task 007's own commit message already
  named this as the correct, not the stale, expectation), so the seam
  assertion below checks `background` there rather than "no background
  set": the claim this file exists to protect is that the row's
  highlight rectangle stops exactly at the seam, never that the seam
  itself is unpainted.

  `[ui] sort_order = "name"` pins a deterministic row order (case-
  insensitive ascending) so this file does not depend on attention-sort
  tie-break behaviour: rec-aaa, rec-bbb-selected-..., rec-ccc, rec-ddd-
  stripe land in that literal order, at sidebar positions 0-3.
  `group_by_workspace = false` is set alongside it so no synthetic group
  header row shifts that position math by a line.

  Two harness steps that key off frame TEXT cannot be used on rec-bbb's
  own row once it truncates: "selects session" looks for the marker
  "> " + the FULL name (never rendered once truncated), and the plain
  "screen shows sessions in this order" step needs a substring that
  actually appears on the frame -- so its rec-bbb row uses the literal
  truncated prefix ("rec-bbb-selected-session-wi", the text this fixture
  is shown to render before the ellipsis), not the full 79-character
  name, while still proving the ordering. Reaching rec-bbb's row for the
  background assertions instead relies on task 301's newest-session
  auto-select landing on rec-ddd-stripe (position 3, alphabetically
  last) right after creation, then two plain "k" (up) presses to
  position 1 -- arithmetic, not text search.

  At the harness's default terminal (100 columns) and the default
  sidebar_width (35, per features/seam_focus.feature's own seam-column
  proof), the sidebar's content columns run 1-34 inclusive (column 0 is
  the left border, column 34 is the trailing pad column task 017 added,
  column 35 is the shared seam). Frame row 0 is the sidebar's top
  border and row 1 is the socket-info header line -- both proved by a
  literal frame dump in the task 323 commit message -- so the FIRST
  session's two lines start at row 2, not row 1: position 0 (rec-aaa)
  occupies rows 2-3, position 1 (rec-bbb-selected-...) rows 4-5,
  position 2 (rec-ccc) rows 6-7, position 3 (rec-ddd-stripe) rows 8-9.
  Position 1 is selected; position 3 sits at an odd stripe phase (task
  084's pos%2==1 rule).

  rec-bbb's name is
  "rec-bbb-selected-session-with-a-name-far-too-long-to-fit-in-the-sidebar-at-all"
  (79 characters) against a sidebar content width of 32 columns
  (width-3, per panel.go's sidebarContentLine) -- provably long enough
  that its first line truncates, per padTrunc's own ellipsis rule (the
  rendered first line is observed as "rec-bbb-selected-session-wi...",
  i.e. 28 characters of the name plus a 3-character ellipsis, at the
  35-column sidebar_width/100-column terminal this file uses), giving
  the selection-background rectangle assertion something to leak past
  if the R58 fixes were ever reverted.

  Both scenarios go red with tasks 320-322 reverted (a selected/striped
  row's background then stops at the text instead of filling the
  rectangle, or leaks a reset that opens a gap) -- see the task 323
  commit message for the quoted red output.

  @requirement-58-selection-background-fills-rectangle-and-seam-stays-clear
  Scenario: a selected row's background fills every content column on both lines, even with a truncating name, and the seam stays clear
    Given the scenario's config.toml is written with:
      """
      [ui]
      sort_order = "name"
      group_by_workspace = false
      """
    And deck client "A" is started with colour enabled
    When deck client "A" creates shell session "rec-aaa"
    And deck client "A" creates shell session "rec-bbb-selected-session-with-a-name-far-too-long-to-fit-in-the-sidebar-at-all"
    And deck client "A" creates shell session "rec-ccc"
    And deck client "A" creates shell session "rec-ddd-stripe"
    Then within one configured reconcile interval deck client "A" screen contains "running"
    And deck client "A" screen shows sessions in this order:
      | rec-aaa                       |
      | rec-bbb-selected-session-wi   |
      | rec-ccc                       |
      | rec-ddd-stripe                |
    When deck client "A" sends "k"
    And deck client "A" sends "k"
    And deck client "A" cell at row 4 column 1 has background token "selection"
    And deck client "A" cell at row 5 column 1 has background token "selection"
    And deck client "A" cells at row 4 columns 2 to 3 have background token "accent"
    And deck client "A" cells at row 5 columns 2 to 3 have background token "accent"
    And deck client "A" cells at row 4 columns 4 to 34 have background token "selection"
    And deck client "A" cells at row 5 columns 4 to 34 have background token "selection"
    And deck client "A" cell at row 4 column 35 has background token "background"
    And deck client "A" cell at row 5 column 35 has background token "background"
    When deck client "A" exits cleanly

  @requirement-58-surface-stripe-fills-rectangle
  Scenario: the alternating surface stripe fills every content column on both lines of a non-selected row
    Given the scenario's config.toml is written with:
      """
      [ui]
      sort_order = "name"
      group_by_workspace = false
      """
    And deck client "A" is started with colour enabled
    When deck client "A" creates shell session "rec-aaa"
    And deck client "A" creates shell session "rec-bbb-selected-session-with-a-name-far-too-long-to-fit-in-the-sidebar-at-all"
    And deck client "A" creates shell session "rec-ccc"
    And deck client "A" creates shell session "rec-ddd-stripe"
    Then within one configured reconcile interval deck client "A" screen contains "running"
    And deck client "A" selects session "rec-aaa"
    And deck client "A" cells at row 8 columns 1 to 34 have background token "surface"
    And deck client "A" cells at row 9 columns 1 to 34 have background token "surface"
    When deck client "A" exits cleanly

  @requirement-119-gutter-outside-text-run
  Scenario: a truncating row's background rectangle stays unbroken with the R119 gutter in its own columns, at the sidebar's minimum width
    Task 008/R119 moved the selection arrow out of the text handed to
    padTrunc into its own reserved gutter columns, composed before the
    text rather than inside it, and shrank the row's own content budget
    by the gutter's width. This is exactly the scenario the SPEC §11.3
    passage that motivates the move names as the risk: "if the marker is
    composed inside the text ... truncateToWidth can cut inside that
    background span on a long name and synthesise a reset there" --
    proved here at `SidebarWidthFloor` (24 columns, internal/tui/layout.go),
    the tightest budget the gutter and a truncating name ever share, by
    pressing "<" 11 times from the default 35-column sidebar_width. At
    width 24 the sidebar's content columns run 1-23 (column 0 is the left
    border, column 24 is the shared seam). Unlike the two scenarios above,
    the row math here is NOT rows 4-5: at this width the sidebar's own
    "socket: <name>" header line (sidebarEntries' first entry) no longer
    fits `contentWidth` (22) -- "socket: deck_test_<pid>_<seq>" runs past
    22 columns for any realistic pid/sequence -- so wrapText splits it
    across TWO physical rows instead of one, pushing every session row
    down by one line versus the 35-column case: rec-bbb (selected at
    visual position 1, after two "k" presses from rec-ddd-stripe's
    auto-selected position 3) lands on rows 5-6, not 4-5. Task 010 split
    the columns 1-23 background check below: since rec-bbb is selected,
    columns 2-3 (the R119 gutter's own two reserved columns) now carry the
    gutter bar's `accent` token rather than the row's own `selection`
    background -- column 1 (the R17 pad column) and columns 4-23 (the
    text run and its pad-fill) still carry `selection` either side of it.
    Given the scenario's config.toml is written with:
      """
      [ui]
      sort_order = "name"
      group_by_workspace = false
      """
    And deck client "A" is started with colour enabled
    When deck client "A" creates shell session "rec-aaa"
    And deck client "A" creates shell session "rec-bbb-selected-session-with-a-name-far-too-long-to-fit-in-the-sidebar-at-all"
    And deck client "A" creates shell session "rec-ccc"
    And deck client "A" creates shell session "rec-ddd-stripe"
    Then within one configured reconcile interval deck client "A" screen contains "running"
    When deck client "A" presses "<" 11 times
    And deck client "A" sends "k"
    And deck client "A" sends "k"
    And deck client "A" cell at row 5 column 1 has background token "selection"
    And deck client "A" cell at row 6 column 1 has background token "selection"
    And deck client "A" cells at row 5 columns 2 to 3 have background token "accent"
    And deck client "A" cells at row 6 columns 2 to 3 have background token "accent"
    And deck client "A" cells at row 5 columns 4 to 23 have background token "selection"
    And deck client "A" cells at row 6 columns 4 to 23 have background token "selection"
    And deck client "A" cell at row 5 column 24 has background token "background"
    And deck client "A" cell at row 6 column 24 has background token "background"
    When deck client "A" exits cleanly

  # Task 009 (R119 colour states) proved the gutter's four background
  # states directly against a Model, per-cell, off internal/tui's own
  # vt.Emulator idiom. This is the same claim's godog evidence, read off a
  # REAL running deck client instead: the gutter bar (columns 2-3 --
  # column 0 is the sidebar's left border, column 1 the R17 pad column,
  # column 2-3 the gutter's own two reserved columns, per
  # sidebarContentLine's own composition order) carries `accent` for a
  # selected row, `badge` for a marked-but-unselected row, and `accent`
  # again (selection wins) for a row that is both -- across BOTH physical
  # lines of each row's own two-line block, never just the line that
  # happens to carry the glyph.
  #
  # At the harness's default terminal (100 columns) and the default
  # sidebar_width (35), three sessions sorted by name (rec-aaa, rec-bbb,
  # rec-ccc, `sort_order = "name"`/`group_by_workspace = false` again
  # pinning that order deterministically) land at sidebar positions 0-2,
  # rows 2-3/4-5/6-7 respectively (row 0 the sidebar's top border, row 1
  # the socket-info header line, exactly the row math
  # panel_background_rectangle's own R58 scenarios above already proved at
  # this width). Task 301's newest-session auto-select lands on rec-ccc
  # (position 2, both the newest AND alphabetically last) the moment all
  # three exist, with nothing marked yet.
  #
  # From there: two "k" presses move the selection to rec-aaa (position 0)
  # without touching any mark; "m" marks rec-aaa while it is still
  # selected; "j" moves the selection on to rec-bbb (position 1), which
  # LEAVES rec-aaa marked but no longer selected (the mark survives the
  # selection moving away -- it is its own independent per-session flag,
  # never cleared by losing focus) while rec-bbb is now selected but was
  # never marked -- so this one frame already carries both the marked-
  # only state (rec-aaa) and the selected-only state (rec-bbb) at once. A
  # further "j" moves the selection on to rec-ccc (position 2, dropping
  # rec-bbb's selection, which it never had a mark to retain), and "m"
  # marks rec-ccc while it is selected, giving the both-cues state on its
  # own row.
  @requirement-119-gutter-background-tokens
  Scenario: the gutter bar's background token is accent when selected, badge when marked, and accent again when both
    Given the scenario's config.toml is written with:
      """
      [ui]
      sort_order = "name"
      group_by_workspace = false
      """
    And deck client "A" is started with colour enabled
    When deck client "A" creates shell session "rec-aaa"
    And deck client "A" creates shell session "rec-bbb"
    And deck client "A" creates shell session "rec-ccc"
    Then within one configured reconcile interval deck client "A" screen contains "running"
    And deck client "A" screen shows sessions in this order:
      | rec-aaa |
      | rec-bbb |
      | rec-ccc |
    When deck client "A" sends "k"
    And deck client "A" sends "k"
    And deck client "A" sends "m"
    And deck client "A" sends "j"
    Then deck client "A" cells at row 2 columns 2 to 3 have background token "badge"
    And deck client "A" cells at row 3 columns 2 to 3 have background token "badge"
    And deck client "A" cells at row 4 columns 2 to 3 have background token "accent"
    And deck client "A" cells at row 5 columns 2 to 3 have background token "accent"
    When deck client "A" sends "j"
    And deck client "A" sends "m"
    Then deck client "A" cells at row 6 columns 2 to 3 have background token "accent"
    And deck client "A" cells at row 7 columns 2 to 3 have background token "accent"
    When deck client "A" exits cleanly

  # Same three states, same session names, same keystrokes -- but the
  # client here never lifts the harness's own NO_COLOR=1 default, so the
  # claim this scenario protects is SPEC §11.3's own: the `>` selection
  # arrow and the mark glyph (`*` -- the harness's Environment always sets
  # DECK_ASCII=1, so every scenario in this suite renders the ASCII
  # fallback, never `\u2713`) are plain TEXT in the gutter's own fixed
  # columns (column 2, the first of the two reserved gutter columns, on
  # each row's own line 1/line 2 respectively), independent of whether any
  # colour is available to carry the cue at all -- and that with no colour
  # available, the gutter cells actually carry no background (proving the
  # cue really does survive on TEXT alone, not on a colour that happens to
  # still be there).
  @requirement-119-gutter-text-survives-no-color
  Scenario: the gutter's selection arrow and mark glyph survive as plain text in their own fixed columns under NO_COLOR
    Given the scenario's config.toml is written with:
      """
      [ui]
      sort_order = "name"
      group_by_workspace = false
      """
    And deck client "B" is started
    When deck client "B" creates shell session "rec-aaa"
    And deck client "B" creates shell session "rec-bbb"
    And deck client "B" creates shell session "rec-ccc"
    Then within one configured reconcile interval deck client "B" screen contains "running"
    And deck client "B" screen shows sessions in this order:
      | rec-aaa |
      | rec-bbb |
      | rec-ccc |
    When deck client "B" sends "k"
    And deck client "B" sends "k"
    And deck client "B" sends "m"
    And deck client "B" sends "j"
    Then deck client "B" cell at row 3 column 2 has content "*"
    And deck client "B" cell at row 3 column 2 has no background set
    And deck client "B" cell at row 4 column 2 has content ">"
    And deck client "B" cell at row 4 column 2 has no background set
    When deck client "B" sends "j"
    And deck client "B" sends "m"
    Then deck client "B" cell at row 6 column 2 has content ">"
    And deck client "B" cell at row 6 column 2 has no background set
    And deck client "B" cell at row 7 column 2 has content "*"
    And deck client "B" cell at row 7 column 2 has no background set
    When deck client "B" exits cleanly
