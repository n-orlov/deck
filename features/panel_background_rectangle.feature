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
  surface stripe -- and the seam column itself, one column further,
  never carries any background at all, even on a row whose name is long
  enough to actually truncate (the fixture trap R58's own tasks
  documented: a fixture where nothing truncates cannot discriminate a
  leak fix from no fix at all, since there is nothing to leak past).

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
    And deck client "A" cells at row 4 columns 1 to 34 have background token "selection"
    And deck client "A" cells at row 5 columns 1 to 34 have background token "selection"
    And deck client "A" cell at row 4 column 35 has no background set
    And deck client "A" cell at row 5 column 35 has no background set
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
