@real-agents
Feature: Real Copilot CLI session conformance
  These scenarios prove, against an actually installed GitHub Copilot CLI (not
  the fake-copilot fixture used everywhere else), the behaviour deck's copilot
  adapter is built on (R223): `--session-id <uuid>` creates a session and the
  very same argv resumes it, a session SIGKILLed before its first message is
  relaunched by Resume's argv, deck's plugin hooks fire for userPromptSubmitted
  and agentStop, and the six recorded probe fixtures still describe a live
  pane. Every launch runs with a temporary COPILOT_HOME, so no ~/.copilot state
  is read or written.

  They run only when opted in, with
  `DECK_GODOG_TAGS=@real-agents DECK_GODOG_PATHS=real_agent_copilot.feature go test -run TestFeatures -v ./features/`
  (CI's default tag filter, `~@real-agents`, excludes them). The first step
  skips each scenario with a stated reason when `copilot` is not on PATH or
  cannot answer a prompt (not logged in); it never prints a token. Upstream
  output is asserted without aliases or coercion: an incompatible Copilot
  upgrade is a visible conformance failure.

  Scenario: a real copilot session is created, instrumented, shown in its probed screens and resumed on the same argv
    Given the installed Copilot CLI resolves on PATH and answers a prompt, or this scenario is skipped with a stated reason
    And the real Copilot CLI is isolated in a temporary COPILOT_HOME
    And deck client "A" is started
    When deck client "A" creates copilot session "real copilot one" with permission profile "safe"
    Then the audit log's most recent launch argv for session "real copilot one" contains "--session-id"
    And the audit log's most recent launch argv for session "real copilot one" contains session "real copilot one"'s conversation id
    And the real Copilot home holds a session directory named by session "real copilot one"'s conversation id
    And session "real copilot one"'s real Copilot pane shows the "trust" probe fixture's key substrings
    When session "real copilot one" answers the real Copilot folder-trust prompt by remembering the folder
    Then session "real copilot one" reaches the real Copilot idle prompt, answering the folder-trust prompt if it appears
    And session "real copilot one"'s real Copilot pane shows the "idle" probe fixture's key substrings

    # A failed slash command draws the "✗ " line the error rule keys on.
    When session "real copilot one" sends the line "/cd /deck-real-agents-no-such-directory" to its real Copilot pane
    Then session "real copilot one"'s real Copilot pane shows the "error" probe fixture's key substrings

    # The permission and question dialogs need the model to call a tool; the
    # prompts name the tool so any model reaches them.
    When session "real copilot one" sends the line "sleepy: run the shell command `sleep 90` with your shell tool, and do nothing else." to its real Copilot pane
    Then session "real copilot one"'s real Copilot pane shows the "permission" probe fixture's key substrings
    When session "real copilot one" cancels the real Copilot dialog with Escape
    Then session "real copilot one" reaches the real Copilot idle prompt, answering the folder-trust prompt if it appears
    When session "real copilot one" sends the line "ask me which colour I prefer, using your ask_user tool." to its real Copilot pane
    Then session "real copilot one"'s real Copilot pane shows the "question" probe fixture's key substrings
    When session "real copilot one" cancels the real Copilot dialog with Escape
    Then session "real copilot one" reaches the real Copilot idle prompt, answering the folder-trust prompt if it appears

    # One ordinary turn: the busy footer, then deck's plugin hooks.
    When session "real copilot one" sends the line "Write two sentences about slow rivers." to its real Copilot pane
    Then session "real copilot one"'s real Copilot pane shows the "working" probe fixture's key substrings
    And session "real copilot one" receives the real Copilot "userPromptSubmitted" hook
    And session "real copilot one" receives the real Copilot "agentStop" hook
    And session "real copilot one" reaches the real Copilot idle prompt, answering the folder-trust prompt if it appears

    # The same argv resumes the session: no second session directory appears.
    When deck client "A" kills its selected session
    Then the state database contains session "real copilot one" with status "stopped"
    When deck client "A" presses r on session "real copilot one"
    Then the audit log's first and most recent launch argv for session "real copilot one" are identical
    And the audit log's most recent launch argv for session "real copilot one" does not contain "--resume"
    And session "real copilot one" reaches the real Copilot idle prompt, answering the folder-trust prompt if it appears
    And the real Copilot home holds exactly 1 session directory
    When deck client "A" exits cleanly

  Scenario: a real copilot session SIGKILLed before its first message is relaunched by Resume's argv
    Given the installed Copilot CLI resolves on PATH and answers a prompt, or this scenario is skipped with a stated reason
    And the real Copilot CLI is isolated in a temporary COPILOT_HOME
    And deck client "A" is started
    When deck client "A" creates copilot session "real copilot early" with permission profile "safe"
    Then the real Copilot home holds a session directory named by session "real copilot early"'s conversation id
    And session "real copilot early" reaches the real Copilot idle prompt, answering the folder-trust prompt if it appears
    When the real Copilot process of session "real copilot early" is killed with SIGKILL
    Then within one configured reconcile interval deck client "A" screen contains "error"
    # The crashed row is not stopped, so the relaunch key is R (restart).
    When deck client "A" presses R on session "real copilot early"
    Then the audit log's first and most recent launch argv for session "real copilot early" are identical
    And the audit log's most recent launch argv for session "real copilot early" contains "--session-id"
    And the audit log's most recent launch argv for session "real copilot early" does not contain "--resume"
    And the audit log's most recent launch argv for session "real copilot early" contains session "real copilot early"'s conversation id
    And session "real copilot early" reaches the real Copilot idle prompt, answering the folder-trust prompt if it appears
    And the real Copilot home holds exactly 1 session directory
    When deck client "A" exits cleanly
