@requirement-33
Feature: The / list filter: by name, workspace and cwd, and the route back to an archived row (I-10, task 123)

  SPEC.md:984/\u00a711.3, requirement 33: `/` filters the sidebar by name,
  workspace and cwd, incrementally as the query is typed, clearing on Esc.
  Archived rows (requirement 27) are hidden from store.ListSessions' own
  default view entirely, so the filter is the only way one is FOUND again
  (phase3-sessions-and-lifecycle.md item 33: "a stated filter term for
  archived, not a separate mode"). It is not the whole way back: SPEC.md
  :323-332 makes archived_at as reversible as deleted_at, so `U` on a row
  surfaced this way clears the flag (R71, issue #8) and returns it to the
  default list -- find it here, then unarchive it.

  @requirement-33-filter-by-name
  Scenario: / filters the list down to the one row whose name matches
    Given deck client "A" is started
    And deck client "A" creates shell session "filter-name-alpha"
    And deck client "A" creates shell session "filter-name-beta"
    When deck client "A" opens the list filter
    And deck client "A" types "filter-name-alpha" into the filter field
    Then deck client "A" screen contains "filter-name-alpha"
    And deck client "A" screen does not contain "filter-name-beta"
    When deck client "A" clears the list filter with escape
    Then deck client "A" screen contains "filter-name-beta"
    And deck client "A" exits cleanly

  @requirement-33-filter-by-cwd-and-workspace
  Scenario: / filters the list down to the one row whose cwd (and its default workspace) matches
    # Workspace defaults to the basename of cwd (store.DefaultWorkspace)
    # and there is no product path to set it to anything else, so a
    # black-box scenario cannot distinguish the two fields from each other
    # -- internal/tui/filter_test.go's own unit tests construct sessions
    # with an explicit Workspace independent of CWD to prove the filter
    # checks each field independently.
    Given deck client "A" is started
    And deck client "A" creates shell session "filter-cwd-one" with a fresh working directory labelled "filter-cwd-target"
    And deck client "A" creates shell session "filter-cwd-two"
    When deck client "A" opens the list filter
    And deck client "A" types "filter-cwd-target" into the filter field
    Then deck client "A" screen contains "filter-cwd-one"
    And deck client "A" screen does not contain "filter-cwd-two"
    When deck client "A" clears the list filter with escape
    Then deck client "A" exits cleanly

  @requirement-33-filter-reaches-archived-row
  Scenario: the filter is the only route back to a row hidden by A archive
    Given deck client "A" is started
    And deck client "A" creates shell session "filter-archived-row"
    And deck client "A" archives its selected session "filter-archived-row"
    Then deck client "A" screen does not contain "filter-archived-row"
    When deck client "A" opens the list filter
    And deck client "A" types "filter-archived-row" into the filter field
    Then deck client "A" screen contains "filter-archived-row"
    And deck client "A" screen contains "Filter"
    When deck client "A" clears the list filter with escape
    Then deck client "A" exits cleanly

  @requirement-33-archive-round-trip
  Scenario: the whole round trip: A plus its confirm hides the row, / finds it, U and r bring it back live to the default list
    # R71 (issue #8) + R72 (issue #10) end to end, in the order an operator
    # meets them: the confirm that stops an accidental `A`, the archive that
    # takes the row out of the default list, the `/` filter that is the only
    # way to FIND it again, `U` to clear archived_at, and `r` to put a live
    # pane back under it -- finishing with no filter in force at all, so the
    # row is proven back in the plain default list rather than merely visible
    # inside a query that is still narrowing the sidebar. Every leg goes
    # through real keystrokes on the real pty: `A` opens the real dialog and
    # Enter submits it, and nothing here calls the archive/unarchive/resume
    # service directly.
    #
    # The target is created last because it is the auto-selected row
    # (requirement 52) that `A` then acts on; the bystander exists to prove
    # the filter really narrowed (it must vanish) and that clearing the
    # filter really restored the whole list (it must come back).
    Given deck client "A" is started
    And deck client "A" creates shell session "trip-bystander"
    And deck client "A" creates shell session "trip-target"
    Then the private tmux session "deck_trip-target" exists
    And the state database contains session "trip-target" with status "running"
    When deck client "A" presses A on its selected session "trip-target"
    Then deck client "A" screen contains "kills the live agent"
    And the state database session "trip-target" is not archived
    When deck client "A" submits the open dialog
    Then the state database session "trip-target" is archived
    And the private tmux session "deck_trip-target" does not exist
    And deck client "A" screen does not contain "trip-target"
    And deck client "A" screen contains "trip-bystander"
    When deck client "A" opens the list filter
    And deck client "A" types "trip-target" into the filter field
    And deck client "A" keeps the filter in force with enter
    Then deck client "A" screen contains "trip-target"
    And deck client "A" screen does not contain "trip-bystander"
    When deck client "A" unarchives its selected session "trip-target"
    Then the state database session "trip-target" is not archived
    When deck client "A" presses r on session "trip-target"
    Then the state database contains session "trip-target" with status "running"
    And the private tmux session "deck_trip-target" exists
    When deck client "A" opens the list filter
    And deck client "A" clears the list filter with escape
    Then deck client "A" screen does not contain "Filter:"
    And deck client "A" screen does not contain "in force"
    And deck client "A" screen contains "trip-target running"
    And deck client "A" screen contains "trip-bystander"
    And deck client "A" exits cleanly

  @requirement-33-dd-reaches-and-tombstones-an-archived-row
  Scenario: dd found through / tombstones an archived row, u returns it to the archived pool intact, and the reap removes it
    # internal/tui/filter.go's own comment used to call deleted_at != 0 AND
    # archived_at != 0 impossible. It is reachable exactly this way: A
    # archives (hidden from the default list), / is still the only route
    # back to it, and dd found through that route tombstones it without
    # ever clearing archived_at -- proven here through the real keystrokes,
    # the internal/store/tombstone_test.go store test exercises this same
    # round trip directly against the store API.
    Given deck client "A" is started with a short delete grace window
    And deck client "A" creates shell session "filter-dd-archived"
    And deck client "A" archives its selected session "filter-dd-archived"
    Then the state database session "filter-dd-archived" is archived
    When deck client "A" opens the list filter
    And deck client "A" types "filter-dd-archived" into the filter field
    And deck client "A" keeps the filter in force with enter
    Then deck client "A" screen contains "filter-dd-archived"
    When deck client "A" presses dd
    Then deck client "A" screen contains "archived record itself"
    When deck client "A" submits the open dialog
    Then deck client "A" screen contains "press u to undo"
    And the state database session "filter-dd-archived" is tombstoned
    When deck client "A" presses u
    Then deck client "A" screen contains "filter-dd-archived"
    And the state database session "filter-dd-archived" is not tombstoned
    And the state database session "filter-dd-archived" is archived
    When deck client "A" presses dd
    And deck client "A" submits the open dialog
    And 400 milliseconds pass
    Then the state database session "filter-dd-archived" is reaped
    And deck client "A" exits cleanly

  @requirement-33-unarchive-from-filter-results
  Scenario: U inside the filter's results returns the archived row to the default list
    # R71 (issue #8): the way back out of `A` that resume's own refusal
    # names. The row is reachable ONLY through the filter, so this presses
    # a real U on the real narrowed list -- Enter first, since while the
    # text field has focus U is filter text, not a keymap key.
    Given deck client "A" is started
    And deck client "A" creates shell session "unarchive-bystander"
    And deck client "A" creates shell session "unarchive-target"
    And deck client "A" archives its selected session "unarchive-target"
    Then the state database session "unarchive-target" is archived
    And deck client "A" screen contains "unarchive-bystander"
    When deck client "A" opens the list filter
    And deck client "A" types "unarchive-target" into the filter field
    And deck client "A" keeps the filter in force with enter
    Then deck client "A" screen contains "unarchive-target"
    And deck client "A" screen does not contain "unarchive-bystander"
    When deck client "A" unarchives its selected session "unarchive-target"
    Then the state database session "unarchive-target" is not archived
    When deck client "A" opens the list filter
    And deck client "A" clears the list filter with escape
    Then deck client "A" screen contains "unarchive-target"
    And deck client "A" screen contains "unarchive-bystander"
    And deck client "A" exits cleanly
