# Task 020: remembered-pi-falls-back scenario

Added a 5th scenario to `features/agent_availability.feature`
(`@requirement-114-remembered-agent-falls-back`): client "A" creates a pi
session while the pi fake is on PATH (persisting `pi` as SPEC.md:1364-1367's
"last create agent" to the shared state.db), the fake is then removed from
PATH, and client "B" (a fresh process, so it re-probes PATH at start) opens
the create modal and must see the Agent field fall back to shell exactly as
if nothing had ever been remembered -- value line `Agent: shell (left/right
cycles: shell)`, `not on PATH: claude, pi`, and no `(last used)` prefix on
the Agent field's own help line specifically (disambiguated the same way
`features/settings.feature`'s clear-recent-cwds scenario disambiguates the
cwd field's own "(last used)", since the shared literal prefix also occurs
on the cwd row and would satisfy a bare `screen does not contain
"(last used)"` vacuously).

## New step
`the fake "pi" binary is removed from PATH` (`fakePiRemovedFromPATH`,
`features/agent_steps_test.go`, registered in the already-registered
`registerAgentSessionSteps`) -- mirrors the existing
`fakeClaudeRemovedFromPATH` exactly, deleting only the `pi` wrapper file
from the scenario's shared fake-agent PATH directory. No existing step
could remove the pi binary from disk (the existing removal step is
claude-specific), and this is not a keystroke-duplicate of anything already
registered.

## Evidence
`ci/run.sh env DECK_GODOG_PATHS=agent_availability.feature go test -count=1 -v ./features/`
exit 0 (`ok  	github.com/n-orlov/deck/features	19.217s`); `TestFeatures`
reports `5 scenarios (5 passed)` for this file, including the new
`an_agent_remembered_from_a_previous_create_falls_back_to_shell,_unlabelled,_once_it_is_no_longer_installed`
subtest. Full verbose log: `verbose.log` in this directory.
