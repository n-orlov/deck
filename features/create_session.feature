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
  Scenario: Ctrl+P/Ctrl+N cycle the cwd field through recent_cwds history, shell-history style, showing "recent N/M"
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
    And deck client "A" presses "ctrl+p" in the cwd field 2 times
    Then deck client "A" screen contains the directory labelled "cycle-4"
    And deck client "A" screen contains "recent 2/5"
    When deck client "A" presses "ctrl+n" in the cwd field 1 times
    Then deck client "A" screen contains the directory labelled "cycle-5"
    And deck client "A" screen contains "recent 1/5"
    When deck client "A" presses "ctrl+n" in the cwd field 1 times
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

  # F19: every "the create modal is open" wait in features/*_test.go used to
  # be a literal wait for the "Create shell session" title
  # (internal/tui.createBody), which reads that only while shell is the
  # pre-selected agent and a plain "Create session" otherwise (task 024's
  # "(last used)" pre-selection) -- so any of those steps would hang for the
  # whole scenario timeout once a non-shell agent was created earlier in
  # the same scenario. Every such wait converged onto the Agent row
  # (rendered unconditionally by createFieldRows) instead, either directly
  # or via ensureCreateModalAgent (features/agent_steps_test.go), which also
  # forces the field back to a specific kind. This scenario is the proof:
  # it creates a claude session first, so the modal's title would read
  # plain "Create session" at the very next open, then drives a shell
  # create through createShellSessionInLabelledCWD -- one of the converted
  # sites -- and shows it still reaches "starting" rather than hanging.
  @requirement-12-title-independent-after-non-shell
  Scenario: the create modal converges on the shell agent even after a non-shell session was created earlier
    Given a fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates claude session "cs-nonshell-seed" with permission profile "safe"
    And deck client "A" creates shell session "cs-after-nonshell" with a fresh working directory labelled "after-nonshell"
    Then the state database session "cs-after-nonshell" has cwd exactly the directory labelled "after-nonshell"
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
    # The create modal's field set plus a rejection's own two lines (task
    # 016 added a tenth field, Group) just clears the default 100x30
    # harness geometry's budget; two extra rows give framedDialogScrollable
    # (internal/tui/panel.go) enough room to show the rejection without
    # scrolling the typed Name row out of view, rather than trimming any
    # field's own content to fit.
    And deck client "A" terminal is resized to 100x32
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

  # R130: the create modal's Group field cycles the available groups
  # exactly as Agent cycles kinds -- this proves the persisted group_id on
  # a row actually created through the dialog, not merely set directly on
  # the row (attention_sort.feature's own "is in group" step does that,
  # for a different concern).
  @requirement-30-create-into-named-group
  Scenario: creating a session into a named group persists its group_id
    Given deck client "A" is started
    And the state database has a group named "tooling"
    When deck client "A" creates shell session "cs-into-group" into group "tooling" with a fresh working directory labelled "into-group"
    Then the state database session "cs-into-group" was created into group "tooling"
    When deck client "A" exits cleanly

  # Requirement 41 also names pre_launch as part of this file's own
  # coverage. features/crash.feature's "a failing pre_launch leaves visible
  # evidence without attaching" proves the FAILING half already -- a
  # pre_launch that exits non-zero short-circuits before the agent argv
  # ever execs (buildPaneCommand's `&&`), so no fixture is needed there.
  # What is still unproven anywhere is that a SUCCEEDING pre_launch, typed
  # into this same create dialog, actually runs at all rather than being
  # silently skipped on the path where the agent goes on to start --
  # agent_session.feature's plain claude-creation scenario proves the agent
  # starts, but says nothing about pre_launch, since it never sets one.
  @requirement-41-pre-launch-succeeds-before-agent
  Scenario: a succeeding pre-launch command, typed into the create dialog, runs before the agent starts
    Given a fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates claude session "cs-pre-launch-ok" with permission profile "safe" and pre-launch command "echo PRE_OK"
    Then deck client "A" screen contains "starting"
    And the private tmux session for "cs-pre-launch-ok" shows "PRE_OK" before "Fake Claude Code"
    And the state database session "cs-pre-launch-ok" has a non-empty conversation id
    When deck client "A" exits cleanly

  # §11.7's remaining recent-cwd interaction this file did not yet cover:
  # [ui] recent_cwd_limit (SPEC §6.5) actually bounds what the cwd field
  # offers, and re-using an already-recent directory does not duplicate its
  # entry (store.PromoteRecentCwd's ON CONFLICT dedup, proven at the Go
  # level by internal/store/store_test.go's
  # TestPromoteRecentCwdRepromotingExistingPathMovesToFrontWithoutDuplicating
  # -- this scenario reaches the same behaviour through the actual create
  # dialog rather than a direct store call). Ghost completion and tab
  # completion remain, as already noted above, in the sibling files
  # create_cwd_ghost.feature and create_cwd_tab.feature; this scenario does
  # not re-litigate that split.
  @requirement-41-recent-cwd-limit-evicts-and-dedupes
  Scenario: recent_cwd_limit bounds the cwd field's history and re-using a directory does not duplicate its entry
    Given the scenario's config.toml is written with:
      """
      [ui]
      recent_cwd_limit = 2
      """
    And deck client "A" is started
    When deck client "A" creates shell session "cs-limit-1" with a fresh working directory labelled "limit-1"
    And deck client "A" creates shell session "cs-limit-2" with a fresh working directory labelled "limit-2"
    And deck client "A" creates shell session "cs-limit-1-reuse" with a fresh working directory labelled "limit-1"
    And deck client "A" creates shell session "cs-limit-3" with a fresh working directory labelled "limit-3"
    And deck client "A" opens the create modal
    Then deck client "A" screen contains the directory labelled "limit-3"
    And deck client "A" screen contains "(last used)"
    When deck client "A" tabs to the cwd field
    And deck client "A" presses "ctrl+p" in the cwd field 2 times
    Then deck client "A" screen contains the directory labelled "limit-1"
    And deck client "A" screen contains "recent 2/2"
    And deck client "A" screen does not contain the directory labelled "limit-2"
    When deck client "A" presses "ctrl+p" in the cwd field 1 times
    Then deck client "A" screen contains the directory labelled "limit-1"
    And deck client "A" screen contains "recent 2/2"
    When deck client "A" closes the create modal
    And deck client "A" exits cleanly
