@create-session
Feature: The create modal's §11.7 cwd prefill (requirement 12)
  The create modal's working-directory field opens pre-filled rather than
  blank: with any §11.7 recent_cwds history it shows the most recently
  promoted entry, labelled on screen as the last used so the user can tell
  it is a default and not something they typed; with no history at all it
  falls back to the directory deck itself was started in. Either way, the
  first keystroke in the field replaces the whole prefill rather than
  appending to it -- the user never has to clear a default they did not
  ask for before typing their own path.

  @requirement-12-no-history-startup-cwd
  Scenario: with no recent_cwds history the cwd field pre-fills with the directory deck started in
    Given deck client "A" is started in a fresh directory labelled "start"
    When deck client "A" opens the create modal
    Then deck client "A" screen contains the directory labelled "start"
    And deck client "A" screen does not contain "(last used)"
    When deck client "A" closes the create modal
    And deck client "A" exits cleanly

  @requirement-12-last-used-prefill
  Scenario: the cwd field pre-fills with the most recent recent_cwds entry, labelled "last used"
    Given deck client "A" is started
    When deck client "A" creates shell session "cs-seed-recent" with a fresh working directory labelled "recent"
    And deck client "A" opens the create modal
    Then deck client "A" screen contains the directory labelled "recent"
    And deck client "A" screen contains "(last used)"
    When deck client "A" closes the create modal
    And deck client "A" exits cleanly

  @requirement-12-wholesale-replace
  Scenario: typing in the cwd field replaces the prefill wholesale rather than appending to it
    Given deck client "A" is started
    When deck client "A" creates shell session "cs-seed-recent2" with a fresh working directory labelled "recent2"
    And deck client "A" creates shell session "cs-typed-over" typing over the prefilled working directory with the directory labelled "typed"
    Then the state database session "cs-typed-over" has cwd exactly the directory labelled "typed"
    When deck client "A" exits cleanly

  @requirement-13-cycle-recent
  Scenario: up/down cycle the cwd field through recent_cwds history, shell-history style, showing "recent N/M"
    Given deck client "A" is started
    When deck client "A" creates shell session "cs-cycle-1" with a fresh working directory labelled "cycle-1"
    And deck client "A" creates shell session "cs-cycle-2" with a fresh working directory labelled "cycle-2"
    And deck client "A" creates shell session "cs-cycle-3" with a fresh working directory labelled "cycle-3"
    And deck client "A" creates shell session "cs-cycle-4" with a fresh working directory labelled "cycle-4"
    And deck client "A" creates shell session "cs-cycle-5" with a fresh working directory labelled "cycle-5"
    And deck client "A" opens the create modal
    Then deck client "A" screen contains the directory labelled "cycle-5"
    And deck client "A" screen does not contain "recent 1/5"
    When deck client "A" tabs to the cwd field
    And deck client "A" presses "up" in the cwd field 2 times
    Then deck client "A" screen contains the directory labelled "cycle-4"
    And deck client "A" screen contains "recent 2/5"
    When deck client "A" presses "down" in the cwd field 1 times
    Then deck client "A" screen contains the directory labelled "cycle-5"
    And deck client "A" screen contains "recent 1/5"
    When deck client "A" presses "down" in the cwd field 1 times
    Then deck client "A" screen contains the directory labelled "cycle-5"
    And deck client "A" screen contains "(last used)"
    And deck client "A" screen does not contain "recent 1/5"
    When deck client "A" closes the create modal
    And deck client "A" exits cleanly

  @requirement-6-blank-name-default
  Scenario: an empty name defaults to <workspace>-<MMDD-HHMM> from the frozen clock
    Given deck client "A" is started in a fresh directory labelled "blank-name" with the clock frozen at "2025-08-20T14:43:00Z"
    When deck client "A" opens the create modal
    And deck client "A" submits the create modal with a blank name
    Then the state database has exactly one session in the directory labelled "blank-name", named "create-session-blank-name-0820-1443"
    When deck client "A" exits cleanly

  @requirement-6-blank-name-collision-suffix
  Scenario: a second blank-name create in the same directory and minute gets a -2 collision suffix
    Given deck client "A" is started in a fresh directory labelled "blank-collide" with the clock frozen at "2025-08-20T14:43:00Z"
    When deck client "A" opens the create modal
    And deck client "A" submits the create modal with a blank name
    And deck client "A" opens the create modal
    And deck client "A" submits the create modal with a blank name
    Then the state database has sessions named "create-session-blank-collide-0820-1443" and "create-session-blank-collide-0820-1443-2", both with cwd exactly the directory labelled "blank-collide"
    When deck client "A" exits cleanly

  # Requirement 15: each rejection the create modal can produce -- a duplicate
  # name, a slug collision with an existing session, a working directory that
  # does not exist, a working directory that exists but is not a directory, a
  # malformed env entry, and malformed launch_args -- names the specific
  # problem in-modal and retains exactly what was typed, rather than closing
  # the modal or clearing a field. Abandoning the modal with esc, meanwhile,
  # creates nothing at all.

  @requirement-15-duplicate-name
  Scenario: submitting a name that already exists names the collision and keeps the modal open
    Given deck client "A" is started in a fresh directory labelled "dup-start"
    When deck client "A" creates shell session "cv-dup-original" with a fresh working directory labelled "dup-original"
    And deck client "A" attempts to create shell session "cv-dup-original" with a fresh working directory labelled "dup-second", expecting rejection
    Then deck client "A" screen contains "already exists"
    And deck client "A" screen contains "cv-dup-original"
    When deck client "A" closes the create modal
    And deck client "A" exits cleanly

  @requirement-15-slug-collision
  Scenario: submitting a name that collides with an existing slug names the collision and keeps the modal open
    Given deck client "A" is started in a fresh directory labelled "slug-start"
    When deck client "A" creates shell session "cv-slug original" with a fresh working directory labelled "slug-original"
    And deck client "A" attempts to create shell session "cv-slug  original" with a fresh working directory labelled "slug-second", expecting rejection
    Then deck client "A" screen contains "collides with existing slug"
    And deck client "A" screen contains "cv-slug  original"
    When deck client "A" closes the create modal
    And deck client "A" exits cleanly

  @requirement-15-nonexistent-cwd
  Scenario: submitting a working directory that does not exist names it and keeps the modal open
    Given deck client "A" is started
    When deck client "A" opens the create modal
    And deck client "A" types "cv-nonexistent-cwd" into the create modal name field
    And deck client "A" types a nonexistent path labelled "cv-missing" into the create modal cwd field
    And deck client "A" submits the create modal expecting rejection
    Then deck client "A" screen contains, allowing word-wrap, "does not exist"
    And deck client "A" screen contains the directory labelled "cv-missing"
    When deck client "A" closes the create modal
    And deck client "A" exits cleanly

  @requirement-15-cwd-not-a-directory
  Scenario: submitting a working directory that exists but is a file names it and keeps the modal open
    Given deck client "A" is started
    When deck client "A" opens the create modal
    And deck client "A" types "cv-cwd-not-dir" into the create modal name field
    And deck client "A" types a file path labelled "cv-notadir" into the create modal cwd field
    And deck client "A" submits the create modal expecting rejection
    Then deck client "A" screen contains, allowing word-wrap, "is not a directory"
    And deck client "A" screen contains the directory labelled "cv-notadir"
    When deck client "A" closes the create modal
    And deck client "A" exits cleanly

  @requirement-15-malformed-env-key
  Scenario: a malformed env entry names the offending entry and keeps the modal open
    Given deck client "A" is started
    When deck client "A" opens the create modal
    And deck client "A" types "cv-malformed-env" into the create modal name field
    And deck client "A" types "novalue,GOOD=1" into the create modal env field
    And deck client "A" submits the create modal expecting rejection
    Then deck client "A" screen contains "key=value"
    And deck client "A" screen contains "novalue,GOOD=1"
    When deck client "A" closes the create modal
    And deck client "A" exits cleanly

  @requirement-15-malformed-launch-args
  Scenario: malformed launch_args JSON names the problem and keeps the modal open
    Given deck client "A" is started
    When deck client "A" opens the create modal
    And deck client "A" types "cv-malformed-launch-args" into the create modal name field
    And deck client "A" types "{not json" into the create modal launch args field
    And deck client "A" submits the create modal expecting rejection
    Then deck client "A" screen contains "launch_args must be a JSON array"
    And deck client "A" screen contains "{not json"
    When deck client "A" closes the create modal
    And deck client "A" exits cleanly

  @requirement-15-esc-abandons
  Scenario: esc abandons the create modal, creating nothing
    Given deck client "A" is started in a fresh directory labelled "esc-abandon"
    When deck client "A" opens the create modal
    And deck client "A" types "cv-esc-abandon" into the create modal name field
    And deck client "A" closes the create modal
    Then the state database has zero sessions
    When deck client "A" exits cleanly
