Feature: Interactive mode refuses rather than degrades in three named cases (Part II, requirement 47/48)

  Requirement 47 names exactly three cases in which entering interactive mode
  must be REFUSED, naming the reason on screen and offering `a` (the full
  attach) as the alternative, rather than degrading (entering anyway at a
  squeezed size, or silently doing nothing): another client already attached
  to the session, a preview box below the measured 7-inner-row floor
  (requirement 48), and a live process already holding the window's
  ownership option.

  @requirement-47-refuse-attached-client
  Scenario: entering interactive mode is refused while another client is attached to the session
    Given deck client "host" is started
    And deck client "host" creates shell session "watched"
    Then within one configured reconcile interval deck client "host" screen contains "running"
    And deck client "host" selects session "watched"
    And a real tmux client attaches to deck session "watched" at 80x24
    When deck client "host" enters interactive mode
    Then deck client "host" screen contains "attached to this session"
    And deck client "host" screen contains "press a to attach"
    And deck client "host" screen contains "deck - sessions"
    And the real tmux client attached to session "watched" detaches
    And deck client "host" exits cleanly

  @requirement-48-refuse-preview-below-seven-rows
  Scenario: entering interactive mode is refused while the preview box has fewer than 7 inner rows
    Given deck client "cramped" is started
    And deck client "cramped" creates shell session "squeezed"
    Then within one configured reconcile interval deck client "cramped" screen contains "running"
    And deck client "cramped" selects session "squeezed"
    And deck client "cramped" terminal is resized to 80x9
    When deck client "cramped" enters interactive mode
    Then deck client "cramped" screen contains "7-row floor"
    And deck client "cramped" screen contains "press a to attach"
    And deck client "cramped" screen contains "deck - sessions"
    And deck client "cramped" exits cleanly

  @requirement-47-refuse-live-ownership
  Scenario: entering interactive mode is refused while a live process holds the window's ownership
    Given deck client "solo" is started
    And deck client "solo" creates shell session "owned"
    Then within one configured reconcile interval deck client "solo" screen contains "running"
    And deck client "solo" selects session "owned"
    And another live process holds ownership of deck session "owned"'s window
    When deck client "solo" enters interactive mode
    Then deck client "solo" screen contains "holds ownership"
    And deck client "solo" screen contains "press a to attach"
    And deck client "solo" screen contains "deck - sessions"
    And deck client "solo" exits cleanly
