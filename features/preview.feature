Feature: The preview capture engine and its visible behaviour
  §11 requirement 21's capture-pane -e engine never attaches a client, never
  spawns a control-mode client, uses no pipe-pane, and never resizes a pane
  it reads; §23-27 govern what the panel shows once a capture lands.

  @requirement-21-preview-no-side-effects
  Scenario: capturing the preview never attaches a tmux client, resizes a pane, or triggers a SIGWINCH, across selection, mode, sidebar-width and outer-terminal changes
    Given a long-running fake "claude" binary is on PATH for future deck clients
    And deck client "solo" is started
    And deck client "solo" creates shell session "alpha"
    And deck client "solo" creates claude session "beacon" with permission profile "safe"
    And the private tmux window for session "beacon" is captured as "before"
    And the fake claude agent's size log is captured as "before"
    When deck client "solo" selects the next session
    And deck client "solo" selects the next session
    And deck client "solo" sends "|"
    And deck client "solo" sends "|"
    And deck client "solo" sends ">"
    And deck client "solo" sends "<"
    And deck client "solo" terminal is resized to 120x40
    And deck client "solo" terminal is resized to 100x30
    Then the private tmux server reports no attached clients
    And the private tmux window for session "beacon" still matches "before"
    And the fake claude agent's size log still matches "before"
    And deck client "solo" exits cleanly

  @requirement-23-preview-crop-geometry
  Scenario: a live pane larger than the panel is cropped with its real geometry stated
    Given deck client "solo" is started
    And deck client "solo" creates shell session "alpha"
    Then deck client "solo" screen matches the pattern "\d+x\d+ of \d+x\d+"
    And deck client "solo" exits cleanly

  @requirement-24-preview-wide-cell-boundary
  Scenario: wide glyphs in a cropped pane never shear the preview's border
    Given deck client "solo" is started
    And deck client "solo" creates shell session "widepane"
    When deck client "solo" fills the selected session's pane with wide characters
    Then deck client "solo" screen contains "界"
    And deck client "solo" every full-width row is bordered on both edges
    And deck client "solo" exits cleanly

  @requirement-22-24-preview-colour-border-integrity
  Scenario: a coloured pane's SGR escapes never shear the preview's border
    Given deck client "solo" is started
    And deck client "solo" creates shell session "colourpane"
    When the private tmux pane for session "colourpane" prints red-coloured text "REDLINE"
    Then deck client "solo" screen contains "REDLINE"
    And deck client "solo" every full-width row is bordered on both edges
    And deck client "solo" the row containing "REDLINE" is bordered on both edges at the full grid width
    And deck client "solo" exits cleanly

  @requirement-25-preview-gesture-no-ops
  Scenario: clicking or scrolling over the preview panel does nothing
    Given deck client "solo" is started
    And deck client "solo" creates shell session "alpha"
    And within one configured reconcile interval deck client "solo" screen contains "running"
    And deck client "solo" captures its frame as "before-preview-gesture"
    When deck client "solo" clicks at column 70 row 15
    And deck client "solo" double-clicks at column 70 row 15
    And deck client "solo" scrolls the wheel up at column 70 row 15
    And deck client "solo" scrolls the wheel down at column 70 row 15
    Then deck client "solo" frame still matches the captured "before-preview-gesture" frame
    And deck client "solo" exits cleanly

  @requirement-26-preview-crash-tail
  Scenario: an error row's preview shows the durable crash tail, headed by copy stating it is not live
    Given a crash-tail fixture and long-running fake Claude are configured
    And deck client "A" is started
    When deck client "A" creates claude session "doomed" with permission profile "safe"
    And fake Claude session "doomed" renders the colored crash-tail fixture
    And the agent process "fake-claude-real" in private tmux session "deck_doomed" is killed with SIGKILL
    Then within one configured reconcile interval deck client "A" screen contains "error"
    And deck client "A" screen contains "Last output before exit - not live:"
    And deck client "A" screen contains "crash final line"
    When deck client "A" exits cleanly

  @requirement-26-preview-placeholder
  Scenario: a stopped session's preview names its own state instead of showing stale bytes
    Given deck client "A" is started
    When deck client "A" creates shell session "retiring"
    And shell session "retiring" exits with status zero
    Then within one configured reconcile interval deck client "A" screen contains "resumable"
    And deck client "A" screen contains "Session is stopped. No live preview to show."
    When deck client "A" exits cleanly

  @requirement-27-preview-suppressed-below-floor
  Scenario: the preview is suppressed below its floor and the sidebar takes the space
    Given deck client "solo" is started
    And deck client "solo" creates shell session "alpha"
    And the private tmux pane for session "alpha" prints "PREVIEWMARKERXYZ"
    Then deck client "solo" screen contains "PREVIEWMARKERXYZ"
    When deck client "solo" terminal is resized to 60x12
    Then deck client "solo" screen stops containing "PREVIEWMARKERXYZ"
    When deck client "solo" terminal is resized to 100x30
    Then deck client "solo" screen contains "PREVIEWMARKERXYZ"
    And deck client "solo" exits cleanly
