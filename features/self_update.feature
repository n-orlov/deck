Feature: deck update installs a release in place (R296)
  `deck update` is a detached verb answered before the configuration and tmux
  checks. Against a loopback fake release server, it installs the latest
  release over the running binary, and `deck update --check` reports a newer
  release while changing nothing. A GitHub immutable release is required by
  default (#104); a mirror has no GitHub release API, so its release is never
  confirmed immutable and is refused unless DECK_REQUIRE_IMMUTABLE=0 opts out,
  in which case the install says the release is not verified immutable. A
  mirror serves both the archive and checksums.txt, so it also requires the
  build attestation (#102): with no authenticated gh to check it, the install
  is refused as if DECK_REQUIRE_ATTESTATION=1 were set.

  Scenario: deck update installs the latest release
    Given a deck binary at version "v1.0.0" installed in a scratch directory
    And a fake release server whose latest release is "v1.2.0"
    And an authenticated gh that verifies the attestation
    When the operator runs "deck update" with "DECK_REQUIRE_IMMUTABLE=0"
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

  Scenario: deck update refuses a mirror's release by default
    Given a deck binary at version "v1.0.0" installed in a scratch directory
    And a fake release server whose latest release is "v1.2.0"
    When the operator runs "deck update"
    Then the command exits 1
    And the output mentions "could not be confirmed immutable", "mirror" and "DECK_REQUIRE_IMMUTABLE=0"
    And the output mentions "nothing was installed" and "v1.2.0"
    And the installed binary is unchanged

  Scenario: DECK_REQUIRE_IMMUTABLE=0 installs a mirror's release with a note
    Given a deck binary at version "v1.0.0" installed in a scratch directory
    And a fake release server whose latest release is "v1.2.0"
    And an authenticated gh that verifies the attestation
    When the operator runs "deck update" with "DECK_REQUIRE_IMMUTABLE=0"
    Then the command exits 0
    And the output mentions "not verified immutable" and "DECK_REQUIRE_IMMUTABLE=0 is set"
    And the installed binary is the release's binary

  Scenario: DECK_REQUIRE_IMMUTABLE=1 still refuses a release that is not verified immutable
    Given a deck binary at version "v1.0.0" installed in a scratch directory
    And a fake release server whose latest release is "v1.2.0"
    When the operator runs "deck update" with "DECK_REQUIRE_IMMUTABLE=1"
    Then the command exits 1
    And the output mentions "could not be confirmed immutable" and "nothing was installed"
    And the installed binary is unchanged

  Scenario: a mirror's release is refused when the attestation cannot be checked
    Given a deck binary at version "v1.0.0" installed in a scratch directory
    And a fake release server whose latest release is "v1.2.0"
    When the operator runs "deck update" with "DECK_REQUIRE_IMMUTABLE=0"
    Then the command exits 1
    And the output mentions "no authenticated gh", "mirror (DECK_UPDATE_URL)" and "nothing was installed"
    And the installed binary is unchanged
