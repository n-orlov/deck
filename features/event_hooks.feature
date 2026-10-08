@event-hooks
Feature: The event hook (SPEC section 10), end to end against the released binary
  deck spawns one user-supplied executable when a session records an event.
  Every scenario points event_hook at a capture script that records its argv,
  a selected set of DECK_* variables and its stdin payload, and prints one
  output line. That script is the only program the hook ever runs: nothing
  here reaches a notification service. Events come from the released
  `deck _hook` (a Claude hook payload, run synchronously, so a scenario that
  expects no invocation reads the log after `_hook` has exited) or from the
  running TUI itself (the user's own kill, which is asynchronous).

  Background:
    Given a long-running fake "claude" binary is on PATH for future deck clients

  Scenario: A session can switch the hook on and off, and deck hands the script the whole contract
    Given the scenario's event hook is the capture script with settings:
      """
      event_hook_default = false
      """
    And deck client "A" is started
    When deck client "A" creates claude session "gate" with permission profile "safe"
    And the released deck _hook receives "Notification" for session "gate" using injected identity:
      | notification_type | permission_prompt |
    Then the capture script has recorded exactly 0 invocations
    When the state database session "gate" has its own event hook on
    And the released deck _hook receives "Notification" for session "gate" using injected identity:
      | notification_type | idle_prompt |
    Then the capture script has recorded exactly 1 invocation
    And capture invocation 1 has the event kind "waiting" as its only argument
    And capture invocation 1 has environment "DECK_EVENT_KIND" equal to "waiting"
    And capture invocation 1 has environment "DECK_EVENT_REASON" equal to "idle_prompt"
    And capture invocation 1 has environment "DECK_SESSION_NAME" equal to "gate"
    And capture invocation 1 has environment "DECK_SESSION_AGENT" equal to "claude"
    And capture invocation 1 has a non-empty environment "DECK_SESSION_ID"
    And capture invocation 1 has a non-empty environment "DECK_EVENT_AT"
    And capture invocation 1 has stdin JSON field "version" equal to "1"
    And capture invocation 1 has stdin JSON field "session.name" equal to "gate"
    And capture invocation 1 has stdin JSON field "session.status" equal to "waiting"
    And capture invocation 1 has stdin JSON field "event.kind" equal to "waiting"
    And capture invocation 1 has stdin JSON field "event.reason" equal to "idle_prompt"
    And capture invocation 1 has stdin JSON field "event.at" that is non-empty
    And capture invocation 1 has stdin JSON field "deck.version" that is non-empty
    And session "gate"'s latest "notification" event records hook kind "waiting", exit status 0 and output containing "captured waiting"
    When the state database session "gate" has its own event hook off
    And the released deck _hook receives "Notification" for session "gate" using injected identity:
      | notification_type | elicitation_dialog |
    Then the capture script has recorded exactly 1 invocation
    When the scenario's event hook is the capture script with settings:
      """
      event_hook_default = true
      """
    And the released deck _hook receives "Notification" for session "gate" using injected identity:
      | notification_type | another_prompt |
    Then the capture script has recorded exactly 1 invocation
    When the state database session "gate" has its own event hook inherit
    And the released deck _hook receives "Notification" for session "gate" using injected identity:
      | notification_type | one_more_prompt |
    Then the capture script has recorded exactly 2 invocations
    And capture invocation 2 has environment "DECK_EVENT_REASON" equal to "one_more_prompt"
    When deck client "A" exits cleanly

  Scenario: deck filters by the allow-list before it spawns anything
    Given the scenario's event hook is the capture script with settings:
      """
      event_hook_default = true
      """
    And deck client "A" is started
    When deck client "A" creates claude session "filtered" with permission profile "safe"
    And the released deck _hook receives "Stop" for session "filtered" using injected identity:
      | last_assistant_message | finished |
    Then the capture script has recorded exactly 0 invocations
    And session "filtered" has one "stop" event with payload field "last_assistant_message" equal to "finished"
    When the released deck _hook receives "Notification" for session "filtered" using injected identity:
      | notification_type | permission_prompt |
    Then the capture script has recorded exactly 1 invocation
    And capture invocation 1 has the event kind "waiting" as its only argument
    When the released deck _hook receives "StopFailure" for session "filtered" using injected identity:
      | error_type | tool_failure |
    Then the capture script has recorded exactly 2 invocations
    And capture invocation 2 has the event kind "error" as its only argument
    And capture invocation 2 has environment "DECK_EVENT_REASON" equal to "tool_failure"
    When deck client "A" exits cleanly

  Scenario: A session's own list replaces the global list and is never merged with it
    Given the scenario's event hook is the capture script with settings:
      """
      event_hook_default = true
      event_hook_events = ["waiting", "error"]
      """
    And deck client "A" is started
    When deck client "A" creates claude session "own list" with permission profile "safe"
    And the state database session "own list" has its own event hook events "idle"
    And the released deck _hook receives "Notification" for session "own list" using injected identity:
      | notification_type | permission_prompt |
    Then the capture script has recorded exactly 0 invocations
    When the released deck _hook receives "Stop" for session "own list" using injected identity:
      | last_assistant_message | done |
    Then the capture script has recorded exactly 1 invocation
    And capture invocation 1 has the event kind "idle" as its only argument
    When the state database session "own list" inherits the event hook events
    And the released deck _hook receives "Notification" for session "own list" using injected identity:
      | notification_type | another_prompt |
    Then the capture script has recorded exactly 2 invocations
    And capture invocation 2 has the event kind "waiting" as its only argument
    When deck client "A" exits cleanly

  Scenario: With no script configured the whole feature is inert whatever a session says
    Given the scenario's config.toml is written with:
      """
      event_hook_default = true
      """
    And the capture script is installed but not configured
    And deck client "A" is started
    When deck client "A" creates claude session "inert" with permission profile "safe"
    And the state database session "inert" has its own event hook on
    And the released deck _hook receives "Notification" for session "inert" using injected identity:
      | notification_type | permission_prompt |
    Then the capture script has recorded exactly 0 invocations
    And session "inert" has one "notification" event with payload field "notification_type" equal to "permission_prompt"
    And session "inert"'s latest "notification" event records no hook result
    # The control: the very same session and payload spawn once a script is named.
    When the scenario's event hook is the capture script with settings:
      """
      event_hook_default = true
      """
    And the released deck _hook receives "Notification" for session "inert" using injected identity:
      | notification_type | another_prompt |
    Then the capture script has recorded exactly 1 invocation
    When deck client "A" exits cleanly

  Scenario: A script that outlives event_hook_timeout is killed and the timeout is recorded on the event
    Given the scenario's event hook is the capture script with settings:
      """
      event_hook_default = true
      event_hook_timeout = 1
      """
    And the capture script holds each invocation open for several seconds
    And deck client "A" is started
    When deck client "A" creates claude session "slow" with permission profile "safe"
    And the released deck _hook receives "Notification" for session "slow" using injected identity and returns within 4 seconds:
      | notification_type | permission_prompt |
    Then the capture script has recorded exactly 1 invocation
    And session "slow"'s latest "notification" event records a timed out hook
    And the capture script process of invocation 1 is gone within 3 seconds
    And the state database session "slow" has hook status "waiting", reason "permission_prompt", message "", acknowledged 0, and notify_epoch 0
    When deck client "A" exits cleanly

  Scenario: A session-end payload never waits for the script
    Given the scenario's event hook is the capture script with settings:
      """
      event_hook_default = true
      """
    And the capture script holds each invocation open for several seconds
    And deck client "A" is started
    When deck client "A" creates claude session "closing" with permission profile "safe"
    And fake Claude session "closing" exits its pane cleanly
    And the released deck _hook receives "SessionEnd" for session "closing" using injected identity and returns within 3 seconds:
      | reason | logout |
    Then the capture script has recorded exactly 1 invocation
    And capture invocation 1 has the event kind "ended" as its only argument
    And capture invocation 1 has stdin JSON field "event.kind" equal to "ended"
    And session "closing" has one "session_end" event with payload field "reason" equal to "logout"
    And session "closing"'s latest "session_end" event records no hook result
    And the capture script process of invocation 1 is gone within 12 seconds
    When deck client "A" exits cleanly

  Scenario: The same prompt in one attention episode spawns once and a new episode spawns again
    Given the scenario's event hook is the capture script with settings:
      """
      event_hook_default = true
      """
    And deck client "A" is started
    When deck client "A" creates claude session "episode" with permission profile "safe"
    And the released deck _hook receives "Notification" for session "episode" using injected identity:
      | notification_type | permission_prompt |
    And the released deck _hook receives "Notification" for session "episode" using injected identity:
      | notification_type | permission_prompt |
    Then the capture script has recorded exactly 1 invocation
    And the state database session "episode" has hook status "waiting", reason "permission_prompt", message "", acknowledged 0, and notify_epoch 0
    When deck client "A" attaches to and detaches from its selected agent
    Then the state database session "episode" is "running" from "user" with acknowledged=1, notify_epoch=1, and 1 attached event
    When the released deck _hook receives "Notification" for session "episode" using injected identity:
      | notification_type | permission_prompt |
    Then the capture script has recorded exactly 2 invocations
    And capture invocation 2 has the event kind "waiting" as its only argument
    When deck client "A" exits cleanly

  Scenario: An event the running TUI records by itself reaches the hook too
    Given the scenario's event hook is the capture script with settings:
      """
      event_hook_default = true
      event_hook_events = ["killed"]
      """
    And deck client "A" is started
    When deck client "A" creates claude session "victim" with permission profile "safe"
    And deck client "A" kills session "victim"
    Then the capture script has recorded exactly 1 invocation
    And capture invocation 1 has the event kind "killed" as its only argument
    And capture invocation 1 has environment "DECK_SESSION_NAME" equal to "victim"
    And capture invocation 1 has stdin JSON field "event.kind" equal to "killed"
    And session "victim"'s latest "killed" event records hook kind "killed", exit status 0 and output containing "captured killed"
    When deck client "A" exits cleanly
