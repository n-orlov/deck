@create-session
Feature: The create modal's stated validation (requirement 15)
  Each rejection the create modal can produce -- a duplicate name, a slug
  collision with an existing session, a working directory that does not
  exist, a working directory that exists but is not a directory, a
  malformed env entry, and malformed launch_args -- names the specific
  problem in-modal and retains exactly what was typed, rather than closing
  the modal or clearing a field. Abandoning the modal with esc, meanwhile,
  creates nothing at all.

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
