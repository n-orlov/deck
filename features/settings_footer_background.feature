Feature: The settings takeover's own footer carries deck's background token in every mode (requirement 118, task cure-03-01)

  Review pass 234's B1 finding: settingsFooterLine (internal/tui/settings.go)
  used to return its composed text directly, never routed through
  canvasBackground the way mainView's own footerLine (task 004/R118) already
  was, so the settings takeover's own footer row leaked the terminal's own
  background in every mode instead of carrying deck's `background` token --
  SPEC.md:1576's own named example of a line "deck paints its own canvas".
  Cured by wrapping the footer's existing text composition
  (settingsFooterLineContent) in a canvasBackground(theme.Background, ...)
  call, mirroring footerLine/footerLineContent's own split, so the fix is
  in the one place every settings view already calls through
  (settingsFooterLine), not duplicated per view.

  This file is that cure's real-binary evidence, read per cell off a real
  running client (never text-scraping), across the footer row's own text
  span (every column its literal string occupies, not merely column 0 --
  internal/tui's own TestSettingsFooterCarriesDeckBackground already proves
  the same claim through Model.View for all six modes and every built-in
  theme; this file adds a real-client proof for the three modes reachable
  without any extra fixture: the plain takeover, the discard-confirm
  prompt and `/` search), reused across those three. Each scenario closes
  the takeover with esc (twice for search, once each to leave search then
  the takeover) before exiting cleanly -- "q" is not bound while
  m.settingsOpen (Update dispatches to updateSettings, never reaching the
  top-level quit case), so a scenario that sent "exits cleanly" while the
  takeover was still open would hang until the harness's own timeout.

  `[ui] theme = "daylight"` pins the scenarios' expected hex the same way
  the retained review probe (review_settings_footer.feature) did. Each
  scenario's own column range is the footer text's own literal length at
  the harness's default 100-column terminal (90 for the plain footer, 100
  for the discard prompt, 57 for search) -- truncateToWidth never pads a
  short line out to the terminal's full width, so a range past that length
  is legitimately unpainted and asserting over it would be testing the
  wrong thing, not a stronger form of this claim.

  @requirement-118-canvas-background
  Scenario: the settings takeover's plain footer paints deck background across its whole row
    Given the scenario's config.toml is written with:
      """
      [ui]
      theme = "daylight"
      """
    And deck client "A" is started with colour enabled
    When deck client "A" sends ","
    Then deck client "A" screen contains "Categories"
    And deck client "A" screen contains "ctrl+s save"
    And deck client "A" cells at row 29 columns 0 to 89 have background token "background"
    When deck client "A" sends ""
    Then deck client "A" screen contains "deck - sessions"
    When deck client "A" exits cleanly

  @requirement-118-canvas-background
  Scenario: the settings takeover's discard-confirm footer paints deck background across its whole row
    Given the scenario's config.toml is written with:
      """
      [ui]
      theme = "daylight"
      """
    And deck client "A" is started with colour enabled
    When deck client "A" sends ","
    And deck client "A" sends "	"
    And deck client "A" sends ""
    And deck client "A" sends ""
    Then deck client "A" screen contains "discard unsaved changes"
    And deck client "A" cells at row 29 columns 0 to 99 have background token "background"
    When deck client "A" sends "y"
    Then deck client "A" screen contains "deck - sessions"
    When deck client "A" exits cleanly

  @requirement-118-canvas-background
  Scenario: the settings takeover's search footer paints deck background across its whole row
    Given the scenario's config.toml is written with:
      """
      [ui]
      theme = "daylight"
      """
    And deck client "A" is started with colour enabled
    When deck client "A" sends ","
    And deck client "A" sends "/"
    Then deck client "A" screen contains "Search:"
    And deck client "A" cells at row 29 columns 0 to 56 have background token "background"
    When deck client "A" sends ""
    Then deck client "A" screen contains "Categories"
    When deck client "A" sends ""
    Then deck client "A" screen contains "deck - sessions"
    When deck client "A" exits cleanly
