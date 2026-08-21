@mouse-bindings
Feature: §11.8 mouse bindings and the [ui] mouse / DECK_MOUSE opt-out (requirements 33-37, 41)
  Every gesture below duplicates an existing key -- none is the only way to
  perform its action -- and hit-testing resolves each click through the
  same layout the renderer itself drew from (task 028). With mouse
  reporting off, every one of these gestures is a no-op and only the
  shortcut is lost; the keyboard path underneath keeps working.

  @requirement-33-click-selects-not-attaches
  Scenario: a single click on a sidebar row selects it without attaching
    Given deck client "A" is started
    When deck client "A" creates shell session "click-selects-alpha"
    And deck client "A" creates shell session "click-selects-bravo"
    And deck client "A" clicks on the row containing "click-selects-bravo"
    Then deck client "A" has session "click-selects-bravo" selected
    And deck client "A" screen contains "deck - sessions"
    When deck client "A" exits cleanly

  @requirement-33-double-click-attaches
  Scenario: a double click on a sidebar row attaches
    Given deck client "A" is started
    When deck client "A" creates shell session "double-click-attaches"
    And deck client "A" double-clicks on the row containing "double-click-attaches"
    Then deck client "A" screen stops containing "deck - sessions"
    When deck client "A" detaches
    Then deck client "A" screen contains "deck - sessions"
    When deck client "A" exits cleanly

  @requirement-34-header-click-collapses
  Scenario: clicking a workspace group's header collapses only that group
    Given deck client "A" is started
    When deck client "A" creates shell session "header-click-default"
    And deck client "A" creates shell session "header-click-other"
    And the state database session "header-click-other" has workspace "header-click-workspace"
    Then deck client "A" screen contains "header-click-workspace"
    And deck client "A" screen contains "header-click-default"
    And deck client "A" screen contains "header-click-other"
    When deck client "A" clicks on the row containing "header-click-workspace"
    Then deck client "A" screen stops containing "header-click-other"
    And deck client "A" screen contains "header-click-default"
    And deck client "A" screen contains "header-click-workspace"
    When deck client "A" clicks on the row containing "header-click-workspace"
    Then deck client "A" screen contains "header-click-other"
    When deck client "A" exits cleanly

  @requirement-34-wheel-scrolls-without-selecting
  Scenario: the wheel scrolls the sidebar's view without changing selection
    Given deck client "A" is started
    When deck client "A" creates shell session "wheel-scroll-1"
    And deck client "A" creates shell session "wheel-scroll-2"
    And deck client "A" creates shell session "wheel-scroll-3"
    And deck client "A" creates shell session "wheel-scroll-4"
    And deck client "A" creates shell session "wheel-scroll-5"
    And the state database session "wheel-scroll-1" has status "idle" 50 seconds ago
    And the state database session "wheel-scroll-2" has status "idle" 40 seconds ago
    And the state database session "wheel-scroll-3" has status "idle" 30 seconds ago
    And the state database session "wheel-scroll-4" has status "idle" 20 seconds ago
    And the state database session "wheel-scroll-5" has status "idle" 10 seconds ago
    Then within one configured reconcile interval deck client "A" screen contains "idle"
    And deck client "A" sends "|"
    And deck client "A" sends "|"
    Then deck client "A" screen stops containing "wheel-scroll-5"
    When deck client "A" selects session "wheel-scroll-1"
    And deck client "A" scrolls the wheel down at column 5 row 5
    And deck client "A" scrolls the wheel down at column 5 row 5
    And deck client "A" scrolls the wheel down at column 5 row 5
    And deck client "A" scrolls the wheel down at column 5 row 5
    Then deck client "A" screen contains "wheel-scroll-5"
    When deck client "A" scrolls the wheel up at column 5 row 5
    And deck client "A" scrolls the wheel up at column 5 row 5
    And deck client "A" scrolls the wheel up at column 5 row 5
    And deck client "A" scrolls the wheel up at column 5 row 5
    Then deck client "A" screen contains "wheel-scroll-1"
    And deck client "A" has session "wheel-scroll-1" selected
    When deck client "A" exits cleanly

  @requirement-35-seam-drag-resizes
  Scenario: dragging the seam adjusts sidebar_width live
    Given deck client "A" is started
    When deck client "A" creates a long-named shell session "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxWIDENOWyyyyyyyyyy"
    Then deck client "A" screen does not contain "WIDENOW"
    When deck client "A" drags from column 36 row 6 to column 61 row 6
    Then deck client "A" screen contains "WIDENOW"
    When deck client "A" exits cleanly

  @requirement-33-preview-gesture-no-ops
  Scenario: clicking, double-clicking or scrolling over the preview panel does nothing
    Given deck client "A" is started
    When deck client "A" creates shell session "preview-gesture-noop"
    And within one configured reconcile interval deck client "A" screen contains "running"
    And deck client "A" captures its frame as "before-preview-mouse-bindings"
    And deck client "A" clicks at column 70 row 15
    And deck client "A" double-clicks at column 70 row 15
    And deck client "A" scrolls the wheel up at column 70 row 15
    And deck client "A" scrolls the wheel down at column 70 row 15
    Then deck client "A" frame still matches the captured "before-preview-mouse-bindings" frame
    When deck client "A" exits cleanly

  @requirement-37-deck-mouse-disables-gestures @requirement-41-keyboard-still-works
  Scenario: DECK_MOUSE=0 disables every mouse gesture, and only the shortcut is lost
    Given the deck config disables mouse reporting
    And deck client "A" is started
    When deck client "A" creates shell session "mouse-off-alpha"
    And deck client "A" creates shell session "mouse-off-bravo"
    And the state database session "mouse-off-alpha" has status "idle" 20 seconds ago
    And the state database session "mouse-off-bravo" has status "idle" 10 seconds ago
    Then within one configured reconcile interval deck client "A" screen contains "idle"
    And deck client "A" captures its frame as "before-mouse-off-click"
    And deck client "A" clicks on the row containing "mouse-off-bravo"
    Then deck client "A" frame still matches the captured "before-mouse-off-click" frame
    When deck client "A" selects session "mouse-off-bravo"
    Then deck client "A" has session "mouse-off-bravo" selected
    When deck client "A" exits cleanly
