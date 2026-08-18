Feature: Deterministic runtime controls
  The released binary remains reproducible when its documented deterministic controls are set.

  Scenario: deterministic frames, audit timing, and generated identifiers
    Given deck frames are byte-stable with DECK_ASCII and NO_COLOR
    When a stepped frozen-clock shell session is created and killed
    Then its wall clock steps while monotonic durations advance
    And repeating DECK_ID_SEED reproduces generated ids
