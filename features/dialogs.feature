@dialogs
Feature: The §11.4 dialog contract, asserted per dialog (requirements 7, 8, 9, 10, 11, 50)
  createView, detailView, profileSwitchView, pinView and helpView all defer
  to the ONE contract implementation in internal/tui/dialog_contract.go
  (task 029): esc cancels and changes nothing, enter submits, tab/shift+tab
  move between fields, left/right/space change a selection. This feature
  proves that contract holds for each of the five dialogs on STATE, not on
  screen text: opening a dialog, altering every field it has, and pressing
  esc must leave the session row, the store and config.toml byte-identical
  to what they were before -- "nothing happened" is the assertion, and a
  wrong implementation that merely repaints the same screen cannot pass it
  by accident. It also covers enter's submit, tab's field navigation, the
  [26,80] width clamp (task 030) at both ends, the mouse's inability to
  cancel or confirm a dialog (border, body and outside all no-ops), and
  in-dialog validation retaining a rejected value.

  Scenario: create dialog -- esc after altering every field creates no session and touches no config
    Given deck client "A" is started
    And the scenario's config.toml is captured as "before-create-esc"
    When deck client "A" opens the create modal and alters every field
    And deck client "A" closes the dialog with escape
    Then the state database does not contain session "dc-name"
    And the state database has exactly 0 sessions
    And the scenario's config.toml still matches the captured "before-create-esc"
    When deck client "A" exits cleanly

  @requirement-7-every-field-reachable-editable-described
  Scenario: create dialog -- every field is reachable by keyboard alone, editable, and each edit is visible
    Given deck client "A" is started
    When deck client "A" walks and edits every create modal field by keyboard, asserting each change is visible
    When deck client "A" closes the create modal
    And deck client "A" exits cleanly

  Scenario: create dialog -- enter submits a valid form
    Given deck client "A" is started
    When deck client "A" creates shell session "dc-submit"
    Then the state database contains session "dc-submit"
    When deck client "A" exits cleanly

  Scenario: create dialog -- tab moves focus between fields
    Given deck client "A" is started
    When deck client "A" sends "n"
    Then deck client "A" screen contains "Create shell session"
    And deck client "A" screen contains "> Name:"
    When deck client "A" tabs 2 times in the open dialog
    Then deck client "A" screen contains "> Agent:"
    And deck client "A" screen does not contain "> Name:"
    When deck client "A" closes the create modal
    And deck client "A" exits cleanly

  Scenario: create dialog -- in-dialog validation retains the typed value and states the reason
    Given deck client "A" is started
    When deck client "A" attempts to create a shell session named "dc-validate" with working directory "/dc-does-not-exist"
    Then deck client "A" screen contains "dc-validate"
    And deck client "A" screen contains "/dc-does-not-exist"
    And deck client "A" screen contains "does not exist"
    And the state database does not contain session "dc-validate"
    When deck client "A" closes the create modal
    And deck client "A" exits cleanly

  Scenario: create dialog -- the mouse can neither cancel nor confirm it, at its border, its body or outside it
    Given deck client "A" is started
    When deck client "A" sends "n"
    Then deck client "A" screen contains "Create shell session"
    When deck client "A" captures its frame as "before-create-mouse"
    # column 1 row 1: the dialog's own top-left border corner.
    And deck client "A" clicks at column 1 row 1
    # column 40 row 5: inside the box, over its body text -- none of the
    # five §11.4 dialogs render a clickable button, so this stands in for
    # "its buttons if any" per requirement 11.
    And deck client "A" clicks at column 40 row 5
    # column 95 row 25: past the box's own [26,80]-clamped width in the
    # harness's 100x30 default terminal, and past its content rows too.
    And deck client "A" clicks at column 95 row 25
    Then deck client "A" frame still matches the captured "before-create-mouse" frame
    When deck client "A" closes the create modal
    And deck client "A" exits cleanly

  Scenario: create dialog -- width is 80% of the viewport clamped to [26,80], at both clamp ends
    Given deck client "A" is started with terminal size 30x60
    When deck client "A" sends "n"
    Then deck client "A" screen contains "Create shell session"
    And deck client "A" dialog box width is 26
    When deck client "A" closes the create modal
    And deck client "A" exits cleanly

  Scenario: create dialog -- width saturates at 80 well past the upper clamp
    Given deck client "A" is started with terminal size 220x30
    When deck client "A" sends "n"
    Then deck client "A" screen contains "Create shell session"
    And deck client "A" dialog box width is 80
    When deck client "A" closes the create modal
    And deck client "A" exits cleanly

  Scenario: detail dialog -- esc changes nothing (it has no fields to alter)
    Given deck client "A" is started
    When deck client "A" creates shell session "dc-detail"
    And the scenario's config.toml is captured as "before-detail-esc"
    And deck client "A" opens detail for session "dc-detail"
    And deck client "A" closes the dialog with escape
    Then the state database has exactly 1 sessions
    And the scenario's config.toml still matches the captured "before-detail-esc"
    When deck client "A" exits cleanly

  Scenario: detail dialog -- the mouse can neither cancel nor confirm it, at its border, its body or outside it
    Given deck client "A" is started
    When deck client "A" creates shell session "dc-detail-mouse"
    Then within one configured reconcile interval deck client "A" screen contains "running"
    When the scenario's config.toml is captured as "before-detail-mouse-cfg"
    And deck client "A" opens detail for session "dc-detail-mouse"
    Then deck client "A" screen contains "detail"
    When deck client "A" captures its frame as "before-detail-mouse"
    # column 1 row 1: the dialog's own top-left border corner.
    And deck client "A" clicks at column 1 row 1
    # column 5 row 4: the exact cell the hidden sidebar's "dc-detail-mouse"
    # row occupies underneath the dialog (same technique task 001/002's
    # settings-takeover test and features/settings.feature:157 use), so
    # a bypassed guard's double click at this point would attach it.
    And deck client "A" clicks at column 5 row 4
    # column 95 row 25: past the box's own [26,80]-clamped width in the
    # harness's 100x30 default terminal, and past its content rows too.
    And deck client "A" clicks at column 95 row 25
    # a double click at the border and the body is the attach gesture
    # (SPEC requirement 11.8) applied to the hidden sidebar row underneath;
    # a bypassed guard would attach "dc-detail-mouse" even with selection
    # unchanged, so the attached-event count is the assertion, not the
    # frame alone.
    And deck client "A" double-clicks at column 1 row 1
    And deck client "A" double-clicks at column 5 row 4
    Then deck client "A" frame still matches the captured "before-detail-mouse" frame
    And the state database has exactly 1 sessions
    And the state database session "dc-detail-mouse" has 0 attached events
    And the scenario's config.toml still matches the captured "before-detail-mouse-cfg"
    When deck client "A" closes the dialog with escape
    And deck client "A" exits cleanly

  Scenario: profile switch dialog -- esc after altering its field leaves the persisted profile untouched
    Given a fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates claude session "dc-profile" with permission profile "safe"
    And the scenario's config.toml is captured as "before-profile-esc"
    And deck client "A" opens the permission profile dialog for session "dc-profile"
    And deck client "A" cycles the open dialog's field right
    Then deck client "A" screen contains "plan (left/right cycles"
    When deck client "A" closes the dialog with escape
    Then the state database session "dc-profile" has permission profile "safe"
    And the scenario's config.toml still matches the captured "before-profile-esc"
    When deck client "A" exits cleanly

  Scenario: profile switch dialog -- enter submits the cycled value
    Given a fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates claude session "dc-profile2" with permission profile "safe"
    And deck client "A" opens the permission profile dialog for session "dc-profile2"
    And deck client "A" cycles the open dialog's field right
    And deck client "A" submits the open dialog
    Then the state database session "dc-profile2" has permission profile "plan"
    When deck client "A" exits cleanly

  Scenario: profile switch dialog -- the mouse can neither cancel nor confirm it, at its border, its body or outside it
    Given a fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates claude session "dc-profile-mouse" with permission profile "safe"
    And the scenario's config.toml is captured as "before-profile-mouse-cfg"
    And deck client "A" opens the permission profile dialog for session "dc-profile-mouse"
    And deck client "A" cycles the open dialog's field right
    Then deck client "A" screen contains "plan (left/right cycles"
    When deck client "A" captures its frame as "before-profile-mouse"
    # column 1 row 1: the dialog's own top-left border corner.
    And deck client "A" clicks at column 1 row 1
    # column 5 row 4: the exact cell the hidden sidebar's "dc-profile-mouse"
    # row occupies underneath the dialog (same technique task 001/002's
    # settings-takeover test and features/settings.feature:157 use), so
    # a bypassed guard's double click at this point would attach it.
    And deck client "A" clicks at column 5 row 4
    # column 95 row 25: past the box's own [26,80]-clamped width in the
    # harness's 100x30 default terminal, and past its content rows too.
    And deck client "A" clicks at column 95 row 25
    # a double click at the border and the body is the attach gesture
    # (SPEC requirement 11.8) applied to the hidden sidebar row underneath;
    # a bypassed guard would attach "dc-profile-mouse" even with selection
    # unchanged, so the attached-event count is the assertion, not the
    # frame alone.
    And deck client "A" double-clicks at column 1 row 1
    And deck client "A" double-clicks at column 5 row 4
    Then deck client "A" frame still matches the captured "before-profile-mouse" frame
    And the state database session "dc-profile-mouse" has permission profile "safe"
    And the state database session "dc-profile-mouse" has 0 attached events
    And the scenario's config.toml still matches the captured "before-profile-mouse-cfg"
    When deck client "A" closes the dialog with escape
    And deck client "A" exits cleanly

  Scenario: pin dialog -- esc after altering its field leaves the persisted resume mode untouched
    Given a fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates claude session "dc-pin" with permission profile "safe"
    Then the state database session "dc-pin" has resume mode "auto"
    When the scenario's config.toml is captured as "before-pin-esc"
    And deck client "A" opens the pin dialog for session "dc-pin"
    And deck client "A" cycles the open dialog's field right
    Then deck client "A" screen contains "pinned (left/right cycles"
    When deck client "A" closes the dialog with escape
    Then the state database session "dc-pin" has resume mode "auto"
    And the scenario's config.toml still matches the captured "before-pin-esc"
    When deck client "A" exits cleanly

  Scenario: pin dialog -- enter submits the cycled value
    Given a fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates claude session "dc-pin2" with permission profile "safe"
    And deck client "A" opens the pin dialog for session "dc-pin2"
    And deck client "A" cycles the open dialog's field right
    And deck client "A" submits the open dialog
    Then the state database session "dc-pin2" has resume mode "pinned"
    When deck client "A" exits cleanly

  Scenario: pin dialog -- the mouse can neither cancel nor confirm it, at its border, its body or outside it
    Given a fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates claude session "dc-pin-mouse" with permission profile "safe"
    And the scenario's config.toml is captured as "before-pin-mouse-cfg"
    And deck client "A" opens the pin dialog for session "dc-pin-mouse"
    And deck client "A" cycles the open dialog's field right
    Then deck client "A" screen contains "pinned (left/right cycles"
    When deck client "A" captures its frame as "before-pin-mouse"
    # column 1 row 1: the dialog's own top-left border corner.
    And deck client "A" clicks at column 1 row 1
    # column 5 row 4: the exact cell the hidden sidebar's "dc-pin-mouse"
    # row occupies underneath the dialog (same technique task 001/002's
    # settings-takeover test and features/settings.feature:157 use), so
    # a bypassed guard's double click at this point would attach it.
    And deck client "A" clicks at column 5 row 4
    # column 95 row 25: past the box's own [26,80]-clamped width in the
    # harness's 100x30 default terminal, and past its content rows too.
    And deck client "A" clicks at column 95 row 25
    # a double click at the border and the body is the attach gesture
    # (SPEC requirement 11.8) applied to the hidden sidebar row underneath;
    # a bypassed guard would attach "dc-pin-mouse" even with selection
    # unchanged, so the attached-event count is the assertion, not the
    # frame alone.
    And deck client "A" double-clicks at column 1 row 1
    And deck client "A" double-clicks at column 5 row 4
    Then deck client "A" frame still matches the captured "before-pin-mouse" frame
    And the state database session "dc-pin-mouse" has resume mode "auto"
    And the state database session "dc-pin-mouse" has 0 attached events
    And the scenario's config.toml still matches the captured "before-pin-mouse-cfg"
    When deck client "A" closes the dialog with escape
    And deck client "A" exits cleanly

  Scenario: help dialog -- esc changes nothing (it has no fields to alter)
    Given deck client "A" is started
    And the scenario's config.toml is captured as "before-help-esc"
    When deck client "A" opens help
    And deck client "A" closes the dialog with escape
    Then the state database has exactly 0 sessions
    And the scenario's config.toml still matches the captured "before-help-esc"
    When deck client "A" exits cleanly

  Scenario: help dialog -- the mouse can neither cancel nor confirm it, at its border, its body or outside it
    Given deck client "A" is started
    When deck client "A" creates shell session "dc-help-mouse"
    And the scenario's config.toml is captured as "before-help-mouse-cfg"
    And deck client "A" opens help
    And deck client "A" captures its frame as "before-help-mouse"
    # column 1 row 1: the dialog's own top-left border corner.
    And deck client "A" clicks at column 1 row 1
    # column 5 row 4: the exact cell the hidden sidebar's "dc-help-mouse"
    # row occupies underneath the dialog (same technique task 001/002's
    # settings-takeover test and features/settings.feature:157 use), so
    # a bypassed guard's double click at this point would attach it.
    And deck client "A" clicks at column 5 row 4
    # column 95 row 25: past the box's own [26,80]-clamped width in the
    # harness's 100x30 default terminal, and past its content rows too.
    And deck client "A" clicks at column 95 row 25
    # a double click at the border and the body is the attach gesture
    # (SPEC requirement 11.8) applied to the hidden sidebar row underneath;
    # a bypassed guard would attach "dc-help-mouse" even with selection
    # unchanged, so the attached-event count is the assertion, not the
    # frame alone.
    And deck client "A" double-clicks at column 1 row 1
    And deck client "A" double-clicks at column 5 row 4
    Then deck client "A" frame still matches the captured "before-help-mouse" frame
    And the state database has exactly 1 sessions
    And the state database session "dc-help-mouse" has 0 attached events
    And the scenario's config.toml still matches the captured "before-help-mouse-cfg"
    When deck client "A" closes the dialog with escape
    And deck client "A" exits cleanly
