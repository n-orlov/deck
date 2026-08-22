@create-session
Feature: The create modal's §11.7 directory-only ghost completion (requirement 14)
  With the cursor at the end of the cwd field (there is no other cursor
  position: typing only appends, backspace only trims the end), a UNIQUE
  directory whose name starts with the segment after the field's last "/"
  is shown inline in the theme's dimmed token, and right/end accept it,
  completing to the match plus a trailing "/". Only directories are ever
  candidates -- a same-named file never blocks or substitutes for one. A
  hidden directory is a candidate only when the segment itself starts with
  ".". A leading "~" expands for scanning without being rewritten in what
  is typed or shown.

  @requirement-14-ghost-unique-match-right-accepts
  Scenario: a unique directory match ghosts in the dimmed token and right accepts it with a trailing slash
    Given a scratch directory labelled "unique-right" exists
    And a directory named "uniqueprojright" exists in the scratch directory labelled "unique-right"
    And deck client "A" is started with colour enabled
    When deck client "A" opens the create modal
    And deck client "A" types "cwd-ghost-right-session" as the session name
    And deck client "A" tabs to the cwd field
    And deck client "A" types the scratch directory labelled "unique-right" followed by "uniquep" into the cwd field
    Then deck client "A" text "rojright/" has foreground token "dimmed"
    When deck client "A" presses "right" in the cwd field
    Then deck client "A" screen contains "uniqueprojright/"
    When deck client "A" submits the create modal
    Then deck client "A" screen contains "starting"
    And the state database session "cwd-ghost-right-session" has cwd exactly the scratch directory labelled "unique-right" plus "/uniqueprojright/"
    When deck client "A" exits cleanly

  @requirement-14-ghost-end-accepts
  Scenario: end also accepts the ghosted completion
    Given a scratch directory labelled "unique-end" exists
    And a directory named "uniqueprojend" exists in the scratch directory labelled "unique-end"
    And deck client "A" is started with colour enabled
    When deck client "A" opens the create modal
    And deck client "A" types "cwd-ghost-end-session" as the session name
    And deck client "A" tabs to the cwd field
    And deck client "A" types the scratch directory labelled "unique-end" followed by "uniquep" into the cwd field
    And deck client "A" presses "end" in the cwd field
    Then deck client "A" screen contains "uniqueprojend/"
    When deck client "A" submits the create modal
    Then deck client "A" screen contains "starting"
    And the state database session "cwd-ghost-end-session" has cwd exactly the scratch directory labelled "unique-end" plus "/uniqueprojend/"
    When deck client "A" exits cleanly

  @requirement-14-ghost-files-never-candidates
  Scenario: a same-named file never becomes a ghost candidate
    Given a scratch directory labelled "files-only" exists
    And a file named "onlyafileexists" exists in the scratch directory labelled "files-only"
    And deck client "A" is started with colour enabled
    When deck client "A" opens the create modal
    And deck client "A" tabs to the cwd field
    And deck client "A" types the scratch directory labelled "files-only" followed by "onlyafile" into the cwd field
    Then deck client "A" screen does not contain "onlyafileexists"
    When deck client "A" presses "right" in the cwd field
    Then deck client "A" screen does not contain "onlyafileexists"
    When deck client "A" closes the create modal
    And deck client "A" exits cleanly

  @requirement-14-ghost-hidden-only-with-dot-segment
  Scenario: a hidden directory is a candidate only when the segment starts with a dot
    Given a scratch directory labelled "hidden-dir" exists
    And a directory named ".hiddensecret" exists in the scratch directory labelled "hidden-dir"
    And deck client "A" is started with colour enabled
    When deck client "A" opens the create modal
    And deck client "A" types "cwd-ghost-hidden-session" as the session name
    And deck client "A" tabs to the cwd field
    And deck client "A" types the scratch directory labelled "hidden-dir" followed by "" into the cwd field
    Then deck client "A" screen does not contain "hiddensecret"
    When deck client "A" sends "."
    Then deck client "A" text "hiddensecret/" has foreground token "dimmed"
    When deck client "A" presses "right" in the cwd field
    And deck client "A" submits the create modal
    Then deck client "A" screen contains "starting"
    And the state database session "cwd-ghost-hidden-session" has cwd exactly the scratch directory labelled "hidden-dir" plus "/.hiddensecret/"
    When deck client "A" exits cleanly

  @requirement-14-ghost-tilde-expands
  Scenario: a leading tilde expands for scanning without being rewritten in the field
    Given a directory named "uniquetildehome" exists under the scenario home
    And deck client "A" is started with colour enabled and HOME set to the scenario home
    When deck client "A" opens the create modal
    And deck client "A" types "cwd-ghost-tilde-session" as the session name
    And deck client "A" tabs to the cwd field
    And deck client "A" sends "~/uniquet"
    Then deck client "A" screen contains "~/uniquet"
    And deck client "A" text "ildehome/" has foreground token "dimmed"
    When deck client "A" presses "right" in the cwd field
    Then deck client "A" screen contains "~/uniquetildehome/"
    When deck client "A" submits the create modal
    Then deck client "A" screen contains "starting"
    And the state database session "cwd-ghost-tilde-session" has cwd "uniquetildehome" resolved under the scenario home
    When deck client "A" exits cleanly

  @requirement-15-ghost-ambiguous-no-completion
  Scenario: several matches with no further common prefix ghost nothing and show a match count
    Given a scratch directory labelled "ambiguous" exists
    And a directory named "matchaaa" exists in the scratch directory labelled "ambiguous"
    And a directory named "matchbbb" exists in the scratch directory labelled "ambiguous"
    And a directory named "matchccc" exists in the scratch directory labelled "ambiguous"
    And deck client "A" is started with colour enabled
    When deck client "A" opens the create modal
    And deck client "A" tabs to the cwd field
    And deck client "A" types the scratch directory labelled "ambiguous" followed by "match" into the cwd field
    Then deck client "A" screen contains "3 matches — tab to list"
    And deck client "A" cwd field shows no ghost text
    When deck client "A" presses "right" in the cwd field
    Then deck client "A" screen does not contain "matchaaa"
    And deck client "A" screen does not contain "matchbbb"
    And deck client "A" screen does not contain "matchccc"
    When deck client "A" closes the create modal
    And deck client "A" exits cleanly
