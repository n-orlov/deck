@requirement-9.2-teardown-hooks
Feature: post_destroy teardown hooks fire on A/dd, never on x, exactly once
  post_destroy (SPEC §9.2) is a session's own teardown hook, set here
  through the launch-inputs editor's Post-destroy command field (task 023),
  ahead of task 026's create-modal field. It runs once a session's pane is
  gone, after A (archive) or dd (delete) -- and never after x, which leaves
  the row stopped and resumable with its external resources expected to
  still be there. This file proves both halves against a real file the
  hook's own subprocess writes under DECK_HOME, never a mock: x leaves it
  absent, dd writes it exactly once, and A followed by its own undo (u)
  brings the row back stopped -- durably, in sessions.status -- without a
  second hook run, and says so: the undo's own toast states that the hook
  already ran and the next r rebuilds what it released (SPEC.md:508).

  Scenario: x never runs the teardown hook, but dd runs it exactly once
    Given deck client "A" is started
    And deck client "A" creates shell session "teardown-kill-then-delete"
    When deck client "A" opens the launch inputs editor for session "teardown-kill-then-delete"
    And deck client "A" types "echo $DECK_SESSION_NAME >> $DECK_HOME/pd.txt" into the post-destroy field
    And deck client "A" submits the launch inputs editor
    And deck client "A" closes detail
    When deck client "A" kills its selected session
    Then the state database session "teardown-kill-then-delete" is "stopped" from "user" with killed_by_user=1
    And the teardown hook artefact file is absent
    When deck client "A" presses dd
    And deck client "A" submits the open dialog
    Then the state database session "teardown-kill-then-delete" is tombstoned
    And the teardown hook artefact file contains exactly one line "teardown-kill-then-delete"
    When deck client "A" exits cleanly

  Scenario: A runs the teardown hook once, and its own undo leaves the row stopped with no second run
    # 100 columns wide for launch_hooks.feature's own reason: both the
    # editor's one-line hook field and the undo toast must render unwrapped
    # for a literal-substring frame wait to see them.
    Given deck client "A" is started with terminal size 100x40
    And deck client "A" creates shell session "teardown-archive-undo"
    When deck client "A" opens the launch inputs editor for session "teardown-archive-undo"
    And deck client "A" types "echo $DECK_SESSION_NAME >> $DECK_HOME/pd.txt" into the post-destroy field
    And deck client "A" submits the launch inputs editor
    And deck client "A" closes detail
    When deck client "A" presses A on its selected session "teardown-archive-undo"
    And deck client "A" submits the open dialog
    Then the state database session "teardown-archive-undo" is archived
    And deck client "A" screen contains "Killed and archived"
    And the teardown hook artefact file contains exactly one line "teardown-archive-undo"
    When deck client "A" undoes the archive with u for "teardown-archive-undo"
    Then deck client "A" screen contains "the next r rebuilds"
    And deck client "A" screen contains "teardown-archive-undo stopped"
    And the state database session "teardown-archive-undo" is not archived
    And the state database session "teardown-archive-undo" is "stopped" from "user" with killed_by_user=1
    And the teardown hook artefact file contains exactly one line "teardown-archive-undo"
    When deck client "A" exits cleanly

  Scenario: a failing teardown hook never blocks dd, and its failure lands durably in the event log
    Given deck client "A" is started
    And deck client "A" creates shell session "teardown-failing-hook"
    When deck client "A" opens the launch inputs editor for session "teardown-failing-hook"
    And deck client "A" types "echo teardown-hook-failure-marker >&2; exit 7" into the post-destroy field
    And deck client "A" submits the launch inputs editor
    And deck client "A" closes detail
    When deck client "A" presses dd
    And deck client "A" submits the open dialog
    Then the state database session "teardown-failing-hook" is tombstoned
    And deck client "A" screen contains "session post_destroy failed"
    And the state database session "teardown-failing-hook" has an event of kind "note" with reason containing "session post_destroy failed"
    When deck client "A" exits cleanly

  Scenario: a bulk dd over two marked rows runs each row's own teardown hook exactly once, under its own DECK_SESSION_ID
    Given deck client "A" is started
    And 200 milliseconds pass
    And deck client "A" creates shell session "td-bulk-one"
    And deck client "A" creates shell session "td-bulk-two"
    When deck client "A" opens the launch inputs editor for session "td-bulk-one"
    And deck client "A" types "echo $DECK_SESSION_ID >> $DECK_HOME/pd_ids.txt" into the post-destroy field
    And deck client "A" submits the launch inputs editor
    And deck client "A" closes detail
    When deck client "A" opens the launch inputs editor for session "td-bulk-two"
    And deck client "A" types "echo $DECK_SESSION_ID >> $DECK_HOME/pd_ids.txt" into the post-destroy field
    And deck client "A" submits the launch inputs editor
    And deck client "A" closes detail
    When deck client "A" sends "k"
    And 100 milliseconds pass
    And deck client "A" sends "m"
    Then deck client "A" screen contains "td-bulk-one running [marked]"
    When deck client "A" sends "j"
    Then deck client "A" screen contains "> td-bulk-two running"
    When deck client "A" sends "m"
    Then deck client "A" screen contains "td-bulk-one running [marked]"
    And deck client "A" screen contains "td-bulk-two running [marked]"
    When deck client "A" presses dd
    Then deck client "A" screen contains "Delete 2 marked sessions"
    When deck client "A" submits the open dialog
    Then the state database session "td-bulk-one" is tombstoned
    And the state database session "td-bulk-two" is tombstoned
    And the teardown hook artefact file records each of "td-bulk-one" and "td-bulk-two"'s own DECK_SESSION_ID exactly once
    When deck client "A" exits cleanly
