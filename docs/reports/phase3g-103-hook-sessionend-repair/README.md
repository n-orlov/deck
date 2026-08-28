# Task 103 — reach the SessionEnd assertion in a state R76 does not repair

## The red at `b6cbbc7`

`status_claude_hooks.feature`'s "Every declared Claude hook maps to honest status through
both identity routes" scenario used to pose `SessionEnd` by sending the hook command straight
into the fake claude's own tmux pane, via `fake Claude session "hook truth" fires "SessionEnd"
for itself using conversation identity:`. That command runs a real `deck _hook SessionEnd`
inside the pane while the pane itself stays alive underneath it. The very next reconcile tick
observes a `stopped` row paired with a live, non-dead pane and treats that combination as
SPEC §7's one self-healing invariant violation
(`internal/service/reconcile.go`'s `repairTerminalRowWithLivePane`, R76): it corrects the row
back to `starting`/`tmux` before the scenario's assertion ever reads it. Verbatim, at `b6cbbc7`
(`red-b6cbbc7-step-error.txt` in this directory):

```
ci/run.sh env DECK_GODOG_PATHS=status_claude_hooks.feature go test ./features/ -run TestFeatures -count=1 -v
(run against HEAD b6cbbc7, before this task's fix)

3 scenarios (2 passed, 1 failed)
64 steps (61 passed, 1 failed, 2 skipped)
--- FAIL: TestFeatures/Every_declared_Claude_hook_maps_to_honest_status_through_both_identity_routes (6.38s)

step error: session "hook truth" = status "starting" source "tmux" reason "tmux pane is alive; terminal row corrected" message "permission granted; work is complete" acknowledged=0 epoch=3; want "stopped" hook "logout" "permission granted; work is complete" 0 3
```

## The fix: change the route to the state, not the repair or the assertion

`internal/service/reconcile.go` is untouched by this commit. Every `Then` assertion the
scenario had at `b6cbbc7` — status, reason, message, acknowledged, notify_epoch, for both the
`SessionEnd` step and the late `SessionStart` step that follows it — is still present, unedited
(`git diff b6cbbc7 -- features/status_claude_hooks.feature` shows only the `When` steps and an
explanatory comment changing; every `Then` line is byte-identical).

What changed is how the scenario reaches the terminal state: a real Claude `SessionEnd` hook
genuinely coincides with that Claude process ending, so the scenario now drives the fake
claude's own pane to a confirmed, clean exit first, then delivers the hook the way a real hook
subprocess always does — as a one-shot, independent invocation of the released `deck _hook`,
never a `send-keys` into a still-live interactive pane:

1. `fake Claude session "hook truth" exits its pane cleanly` (new step,
   `fakeClaudeExitsPaneCleanly` in `features/status_claude_hooks_test.go`) sends the fixture's
   new `{"command":"exit"}` pane command (new case in `cmd/fake-claude/main.go`'s `runCommands`,
   which simply `return nil`s the command loop so the wrapper's `exec` lets the process exit
   0), then polls until the whole `deck_hook-truth` tmux session is gone — under deck's
   server-wide `remain-on-exit=failed`, a zero-exit pane is destroyed rather than retained, and
   since this scenario's session occupies its tmux session's only window/pane, the session
   disappears with it. The step blocks on that disappearance rather than returning as soon as
   `send-keys` completes, so the next step is guaranteed a genuinely dead pane underneath it,
   not a race against the exit still landing.
2. `the released deck _hook receives "SessionEnd" for session "hook truth" using conversation
   identity:` (new step, `releasedHookFiresForSession`) execs `deck _hook` directly with the
   payload on stdin and `DECK_SESSION_ID`/`DECK_LAUNCH_GENERATION` in its environment — exactly
   the identity substrate `releasedHookForSession` in `assertionHarness`/`assertions_test.go`
   already uses for the single-purpose "released running/waiting hook" steps — instead of
   routing through any pane at all.
3. The following `SessionStart` step (`late-after-clean-stop`) is switched to the same released
   route for the same reason: the pane it used to fire into no longer exists.

With the pane confirmed dead before the hook lands, reconcile has no live pane to observe
against the `stopped` row, so R76's repair never fires, and the scenario's own `Then` reads the
hook's honest write. Three consecutive green runs of the targeted feature
(`ci/run.sh env DECK_GODOG_PATHS=status_claude_hooks.feature go test ./features/ -run
TestFeatures -count=1`), captured as `green-run1.log`, `green-run2.log`, `green-run3.log` in
this directory — all `ok github.com/n-orlov/deck/features <5s`, exit 0.

## For task 110's findings fold-in: R76 working as specified, not a spec contradiction

A hook-declared terminal status arriving while the pane it came through is still alive is, on
inspection, the *same* class of case F20 already documents for a raw state-database write
(`docs/reports/phase3g-findings.md` §F20): SPEC §7 names `repairTerminalRowWithLivePane` as
"the one self-healing rule" specifically because a terminal status paired with a live,
non-dead pane is treated as an invariant violation to correct, not evidence to act on,
regardless of *which* mechanism produced the terminal write (raced test-only write, real hook,
or anything else) — the repair function's own doc comment reasons from "the pane is the part
that is right," not from the writer's identity. §7's precedence table (`user-terminal > hook >
probe > tmux`) governs which *source* wins when two live, competing signals disagree about the
current status; it does not except a hook-set terminal status from R76's separate, later check
against the pane's own physical liveness, because in the real product a genuine `SessionEnd`
hook and a genuinely dead pane always arrive together (the same process ending fires both) —
the scenario's original construction (posing the hook while deliberately leaving the pane
alive) built a state that a real Claude session can never actually reach, the same way F20's
scenario built its contradiction. So: **this is R76 working as specified.** The fix belongs in
the scenario's route to the state (make the pane's death real before the hook lands), exactly
as this task did, never in loosening or removing R76's repair. Task 110 can fold this in as a
sibling of F20 (both a scenario/requirement interaction confirmed to be by-design, not a
defect) rather than as a new distinct finding.
