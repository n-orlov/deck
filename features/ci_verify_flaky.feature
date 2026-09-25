Feature: CI verify flaky scenario (task 019)
  Throwaway probe for C.7 (R145/R146): a single scenario that fails on its
  first run and passes on ci/suite.sh's own per-scenario solo rerun
  (DECK_GODOG_PATHS=<file>:<line>), proving the retry path reruns only the
  failed scenario. Never merged to main.

  Scenario: fail once then pass
    Given the Godog harness is available
    Then the ci-verify flaky marker is toggled
