@requirement-33
Feature: The / list filter: by name, workspace and cwd, and the only route back to an archived row (I-10, task 123)

  SPEC.md:984/\u00a711.3, requirement 33: `/` filters the sidebar by name,
  workspace and cwd, incrementally as the query is typed, clearing on Esc.
  Archived rows (requirement 27) are hidden from store.ListSessions' own
  default view entirely and have no restore, unlike a tombstoned row -- the
  filter is their only route back (phase3-sessions-and-lifecycle.md item 33:
  "a stated filter term for archived, not a separate mode").

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
