Feature: The shared sidebar/preview seam reads focused whenever either panel is (requirement 57)

  Task 318 (SPEC amendment 6584299) resolves the single shared column
  between the sidebar and the preview panel -- and the ┬/┴ T-junctions
  where it meets each panel's own top/bottom border -- from a rule that
  belongs to neither panel alone: `border_focus` whenever EITHER the
  sidebar OR the preview (via interactive mode) currently holds the main
  view's one unit of keyboard focus, and plain `border` only in the one
  case neither does -- some overlay atop mainView (here, the `/` list
  filter, task 123) intercepts every keystroke before either panel's own
  bindings see it, per filter.go's own comment that mainView keeps
  rendering underneath it. Before task 318 the seam followed
  previewBorderToken alone, which reads `border` whenever the sidebar (not
  the preview) holds focus -- reverting 318 turns this scenario's very
  first seam assertion red.

  At the harness's default 100×24 terminal, the default sidebar_width (35)
  puts the seam's top T-junction at row 0, column 35 -- proved directly
  against a live client's rendered grid, per cell, never by text-scraping.

  @requirement-57-seam-follows-either-panel-focused
  Scenario: the seam reads border_focus with the sidebar focused, border_focus while interactive, and plain border with neither focused
    Given deck client "seam" is started with colour enabled
    And deck client "seam" creates shell session "seam-target"
    Then within one configured reconcile interval deck client "seam" screen contains "running"
    And deck client "seam" selects session "seam-target"
    And deck client "seam" cell at row 0 column 35 has foreground token "border_focus"
    When deck client "seam" enters interactive mode
    Then deck client "seam" cell at row 0 column 35 has foreground token "border_focus"
    When deck client "seam" leaves interactive mode
    And deck client "seam" opens the list filter
    Then deck client "seam" cell at row 0 column 35 has foreground token "border"
    When deck client "seam" clears the list filter with escape
    Then deck client "seam" cell at row 0 column 35 has foreground token "border_focus"
    And deck client "seam" exits cleanly
