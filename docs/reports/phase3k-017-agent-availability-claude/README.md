# Task 017: only-claude-installed scenario in agent_availability.feature

Added a second scenario to `features/agent_availability.feature`
(`@requirement-114-only-claude-installed`) whose only agent fixture is
`a fake "claude" binary is on PATH for future deck clients` (existing step,
`fakeClaudeOnPATHForFutureClients` in `features/agent_steps_test.go`).

The scenario opens the create modal and asserts:
- `Agent: shell (left/right cycles: claude, shell)` -- claude is offered,
  pi is not, shell is still the opening default (registry order is
  alphabetical: claude, pi, shell -- `Registry.Kinds()` sorts).
- `not on PATH: pi` in the Agent field's help text.
- cycling right (via the existing `cycles the open dialog's field right`
  step, reused verbatim -- no new step added) reaches `claude` and cycling
  right again returns to `shell`; pi never appears in the cycle text at
  any point.

No new godog step was added; every step reused already-registered
vocabulary (`a fake "claude" binary is on PATH for future deck clients`,
`deck client "A" is started`, `opens the create modal`, `screen contains`,
`presses down N times in the open dialog`, `cycles the open dialog's field
right`, `closes the create modal`, `exits cleanly`).

## Evidence

`ci/run.sh env DECK_GODOG_PATHS=agent_availability.feature go test -count=1 -v ./features/`
exit 0 (see `verbose.log`): both scenarios in the file pass --
`with nothing installed the Agent field offers only shell` and
`with only claude installed the Agent field offers shell and claude but not pi`.
