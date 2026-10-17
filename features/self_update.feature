Feature: deck update installs a release in place (R296)
  `deck update` is a detached verb answered before the configuration and tmux
  checks. Against a loopback fake release server, it installs the latest
  release over the running binary, and `deck update --check` reports a newer
  release while changing nothing. A mirror has no GitHub release API, so its
  release is never verified immutable (#104): the install says so, and
  DECK_REQUIRE_IMMUTABLE=1 refuses it.

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

  Scenario: deck update from a mirror says the release is not verified immutable
    Given a deck binary at version "v1.0.0" installed in a scratch directory
    And a fake release server whose latest release is "v1.2.0"
    When the operator runs "deck update"
    Then the command exits 0
    And the output mentions "not verified immutable" and "DECK_REQUIRE_IMMUTABLE=1"
    And the installed binary is the release's binary

  Scenario: DECK_REQUIRE_IMMUTABLE=1 refuses a release that is not verified immutable
    Given a deck binary at version "v1.0.0" installed in a scratch directory
    And a fake release server whose latest release is "v1.2.0"
    When the operator runs "deck update" with "DECK_REQUIRE_IMMUTABLE=1"
    Then the command exits 1
    And the output mentions "could not be verified immutable" and "nothing was installed"
    And the installed binary is unchanged
