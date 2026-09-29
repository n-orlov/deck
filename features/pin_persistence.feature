@sidebar-pin
Feature: `p` pins a row to the top of its group with a marker, and the pin survives a deck restart
  SPEC §11's pin rule (R160): a pinned row (`pinned_at != 0`) sorts above
  every unpinned row of the same group and shows the `✦` marker before its
  name. `p` toggles the pin (R159) on the selected row; the pin is a column
  on the row itself (schemaV8's `pinned_at`, R158), so it survives every
  mutation including a whole deck restart -- a fresh process reading the
  same durable DECK_HOME, exactly like features/durable_identity.feature's
  own restart. `p` again drops the row back to its ordinary sorted position
  and removes the marker.

  Scenario: p pins a middle row to the top of its group, the pin survives a deck restart, and p again restores its place
    Given the scenario's config.toml is written with:
      """
      [ui]
      sort_order = "name"
      """
    And deck client "A" is started
    When deck client "A" creates shell session "pin-alpha"
    And deck client "A" creates shell session "pin-bravo"
    And deck client "A" creates shell session "pin-charlie"
    Then deck client "A" screen shows sessions in this order:
      | pin-alpha   |
      | pin-bravo   |
      | pin-charlie |
    And deck client "A" row "pin-bravo" does not show the pin marker
    When deck client "A" presses p on session "pin-bravo"
    Then deck client "A" screen shows sessions in this order:
      | pin-bravo   |
      | pin-alpha   |
      | pin-charlie |
    And deck client "A" row "pin-bravo" shows the pin marker
    When deck client "A" exits cleanly
    # CI stand-in for a deck restart (features/durable_identity.feature's
    # own pattern): a fresh client process reading the same durable
    # DECK_HOME. The pin is a column on the row itself, not client state.
    And deck client "B" is started
    Then deck client "B" screen shows sessions in this order:
      | pin-bravo   |
      | pin-alpha   |
      | pin-charlie |
    And deck client "B" row "pin-bravo" shows the pin marker
    When deck client "B" presses p on session "pin-bravo"
    Then deck client "B" screen shows sessions in this order:
      | pin-alpha   |
      | pin-bravo   |
      | pin-charlie |
    And deck client "B" row "pin-bravo" does not show the pin marker
    When deck client "B" exits cleanly
