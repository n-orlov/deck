@settings
Feature: The `,` settings takeover (requirement 48)
  SPEC §11.5's settings takeover is generated from
  internal/config.Schema, not a second, hand-written field list: every flat
  config.toml key is reachable and, where it has an in-place editor
  (toggle, bounded integer, cycled enum), editable; `/` searches every
  field's label AND description; ctrl+s writes through the atomic writer
  (task 012) and the file it leaves behind parses back to what the
  takeover showed; esc with unsaved changes prompts before discarding and
  never touches config.toml when it does; a key the environment already
  overrides is labelled honestly instead of pretending a save changed the
  running value; [env]'s restart-to-apply scope is stated on screen; and
  [notify] is a single honestly-unavailable entry, per §6.5/§11.5.
  None of this may ever create, delete or otherwise touch a session row
  (requirement 24) -- the takeover edits config.toml, nothing else.

  Scenario: every category and its schema-declared fields are reachable, including the honestly-unavailable [notify] entry
    Given deck client "A" is started
    When deck client "A" sends ","
    Then deck client "A" screen contains "Categories"
    And deck client "A" screen contains "Fields"
    And deck client "A" screen contains "General"
    And deck client "A" screen contains "Allow Yolo"
    And deck client "A" screen contains "Stale After"
    And deck client "A" screen contains "Capture Min Interval"
    When deck client "A" sends "j"
    Then deck client "A" screen contains "UI"
    And deck client "A" screen contains "Theme"
    And deck client "A" screen contains "Ascii"
    And deck client "A" screen contains "Mouse"
    And deck client "A" screen contains "Recent Cwd Limit"
    When deck client "A" sends "j"
    Then deck client "A" screen contains "Environment Variables"
    When deck client "A" sends "j"
    Then deck client "A" screen contains "Notify"
    And deck client "A" screen contains "Notification Rules: unavailable this phase"
    When deck client "A" sends "	"
    And deck client "A" sends ""
    Then deck client "A" screen contains "Notification Rules: unavailable this phase"
    When deck client "A" sends ""
    Then deck client "A" screen contains "deck - sessions"
    When deck client "A" exits cleanly

  Scenario: a toggle and a bounded integer are edited in place and ctrl+s writes config.toml to match what the takeover showed
    Given deck client "A" is started
    When deck client "A" sends ","
    And deck client "A" sends "	"
    Then deck client "A" screen contains "Allow Yolo: Off"
    When deck client "A" sends ""
    Then deck client "A" screen contains "Allow Yolo: On"
    When deck client "A" sends "j"
    Then deck client "A" screen contains "Stale After: 45 seconds (min 1)"
    When deck client "A" sends "+"
    Then deck client "A" screen contains "Stale After: 46 seconds (min 1)"
    When deck client "A" sends ""
    Then deck client "A" screen contains "saved "
    When deck client "A" sends ""
    Then deck client "A" screen contains "deck - sessions"
    And the scenario's config.toml parses with allow_yolo true
    And the scenario's config.toml parses with stale_after "46s"
    When deck client "A" exits cleanly

  Scenario: `/` finds a field only its description mentions, and enter jumps both lists onto it
    Given deck client "A" is started
    When deck client "A" sends ","
    And deck client "A" sends "/"
    Then deck client "A" screen contains "Search: "
    When deck client "A" sends "scrollback"
    Then deck client "A" screen contains "Capture Min Interval"
    And deck client "A" screen does not contain "Allow Yolo"
    When deck client "A" sends ""
    Then deck client "A" screen contains "Capture Min Interval: 5 seconds (min 1)"
    When deck client "A" sends ""
    Then deck client "A" screen contains "deck - sessions"
    When deck client "A" exits cleanly

  Scenario: esc with an unsaved change prompts to discard, and discarding leaves config.toml exactly as it was
    Given deck client "A" is started
    And the scenario's config.toml is captured as "before"
    When deck client "A" sends ","
    And deck client "A" sends "	"
    And deck client "A" sends ""
    Then deck client "A" screen contains "Allow Yolo: On"
    When deck client "A" sends ""
    Then deck client "A" screen contains "discard unsaved changes"
    When deck client "A" sends "y"
    Then deck client "A" screen contains "deck - sessions"
    And the scenario's config.toml still matches the captured "before"
    When deck client "A" exits cleanly

  Scenario: an environment-overridden field is labelled honestly and a save never pretends the running value moved
    Given deck client "A" is started
    When deck client "A" sends ","
    And deck client "A" sends "j"
    And deck client "A" sends "	"
    And deck client "A" sends "j"
    Then deck client "A" screen contains "Ascii: On (overridden by environment: DECK_ASCII)"
    And deck client "A" screen contains "Overridden by environment: DECK_ASCII is set, so the running"
    When deck client "A" sends ""
    Then deck client "A" screen contains "Ascii: Off (overridden by environment: DECK_ASCII)"
    When deck client "A" sends ""
    Then deck client "A" screen contains "saved "
    And deck client "A" screen contains "Ascii: Off (overridden by environment: DECK_ASCII)"
    When deck client "A" sends ""
    Then deck client "A" screen contains "deck - sessions"
    When deck client "A" exits cleanly

  Scenario: the [env] table states its restart-to-apply scope on screen
    Given deck client "A" is started
    When deck client "A" sends ","
    And deck client "A" sends "j"
    And deck client "A" sends "j"
    Then deck client "A" screen contains "Environment Variables: 0 entries"
    And deck client "A" screen contains "Kind: list-of-strings · Scope: restart-to-apply"
    When deck client "A" sends ""
    Then deck client "A" screen contains "deck - sessions"
    When deck client "A" exits cleanly

  Scenario: driving every key the takeover binds leaves the session set untouched
    Given deck client "A" is started
    When deck client "A" creates shell session "keep-a"
    And deck client "A" creates shell session "keep-b"
    Then the state database contains session "keep-a"
    And the state database contains session "keep-b"
    And the state database has exactly 2 sessions
    When deck client "A" sends ","
    And deck client "A" sends "	"
    And deck client "A" sends ""
    And deck client "A" sends "j"
    And deck client "A" sends "+"
    And deck client "A" sends "j"
    And deck client "A" sends "-"
    And deck client "A" sends "	"
    And deck client "A" sends "j"
    And deck client "A" sends "	"
    And deck client "A" sends "+"
    And deck client "A" sends "/"
    And deck client "A" sends "mouse"
    And deck client "A" sends ""
    And deck client "A" sends ""
    And deck client "A" sends ""
    Then deck client "A" screen contains "deck - sessions"
    And the state database contains session "keep-a"
    And the state database contains session "keep-b"
    And the state database has exactly 2 sessions
    When deck client "A" exits cleanly
