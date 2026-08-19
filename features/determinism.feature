Feature: Deterministic runtime controls
  The released binary remains reproducible when its documented deterministic controls are set.

  @clock-step
  Scenario: deterministic frames, shared clock stepping, and generated identifiers
    Given deck frames are byte-stable with DECK_ASCII and NO_COLOR
    When a stepped frozen-clock shell session is created and killed
    Then both running clients and a later hook subprocess share the stepped wall clock
    And its audit wall clock steps on demand while monotonic durations advance
    And repeating DECK_ID_SEED reproduces generated ids
