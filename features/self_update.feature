Feature: deck update installs a release in place (R296)
  `deck update` is a detached verb answered before the configuration and tmux
  checks. Against a loopback fake release server, it installs the latest
  release over the running binary, and `deck update --check` reports a newer
  release while changing nothing.

  Scenario: deck update installs the latest release
    Given a deck binary at version "v1.0.0" installed in a scratch directory
    And a fake release server whose latest release is "v1.2.0"
    When the operator runs "deck update"
    Then the command exits 0
    And the output mentions "v1.0.0", "v1.2.0" and "restart"
    And the installed binary is the release's binary

  Scenario: deck update --check reports a newer release and changes nothing
    Given a deck binary at version "v1.0.0" installed in a scratch directory
    And a fake release server whose latest release is "v1.2.0"
    When the operator runs "deck update --check"
    Then the command exits 0
    And the output mentions "update available" and "v1.2.0"
    And the installed binary is unchanged
    And the scratch home directory is still empty
