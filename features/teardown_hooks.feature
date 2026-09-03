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
  brings the row back stopped without a second hook run -- the row's
  external resources stay released until the next r rebuilds them (help's
  own Hooks section states this in as many words).

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
    Given deck client "A" is started with terminal size 100x300
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
    Then the state database session "teardown-archive-undo" is not archived
    And deck client "A" screen contains "teardown-archive-undo stopped"
    And the teardown hook artefact file contains exactly one line "teardown-archive-undo"
    When deck client "A" opens help
    Then deck client "A" screen contains "the next r rebuilds"
    When deck client "A" exits cleanly
