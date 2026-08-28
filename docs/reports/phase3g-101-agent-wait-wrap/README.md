# Task 101 — Agent-field wait tolerates a wrapped dialog line

## What was wrong

`features/agent_steps_test.go`'s `ensureCreateModalAgent` waited on the literal
`"Agent: " + want + " (left/right cycles"`. `internal/tui.createFieldRows`
renders the Agent row's value and its `(left/right cycles: ...)` hint as one
logical sentence, then word-wraps it to the dialog's content width. Once that
width is narrow enough — `kill_delete_undo.feature`'s "the dd confirm dialog
width is 80% of the viewport clamped to [26,80]" scenario forces exactly this
via a 30x60 terminal, clamping the dialog to 26 columns — the wrap point falls
*inside* the awaited literal: `Agent: claude` renders on one grid row and
`(left/right cycles: claude, pi, shell)` renders on the next. `ScreenDriver.Frame`
joins grid rows with a literal `"\n"` (`NormalizeFrame`), which sits right in
the middle of the literal, so the substring can never match at that width and
the wait always times out.

## Red, captured at `b6cbbc7` (pre-fix)

`ci/run.sh env DECK_GODOG_PATHS=kill_delete_undo.feature go test ./features/ -run TestFeatures -count=1`
→ exit 1, full log in `red-b6cbbc7.log`. The failing scenario and the frame at
the moment of timeout:

```
Scenario: the dd confirm dialog width is 80% of the viewport clamped to [26,80], at both clamp ends # kill_delete_undo.feature:99
    Given deck client "A" is started with terminal size 30x60
    And deck client "A" creates shell session "dd-width-lower"
    after scenario hook failed: cycle the create modal's Agent field to "shell": timed out waiting for frame "Agent: shell (left/right cycles": context deadline exceeded
frame:
+------------------------+
| Create session         |
...
| shown directory match  |
| > Agent: claude        |
| (left/right cycles:    |
| claude, pi, shell)     |
| which coding agent     |
...
```

`Agent: claude` and `(left/right cycles:` are visibly two separate rows in that
frame — exactly the wrap the awaited literal can't survive.

## Fix

`ensureCreateModalAgent` in `features/agent_steps_test.go` now matches against
`dewrapCreateModalAgentRow(frame)` instead of the raw joined frame.
`dewrapCreateModalAgentRow` finds the grid row containing `"Agent: "`, strips
that row's and the following row's dialog-box border/padding
(`stripDialogBoxBorder`), and joins what's left with a single space —
reconstructing the same logical line `createFieldRows` produced before the box
wrapped it. The awaited marker is unchanged: `"Agent: " + want + " (left/right
cycles"` still pins the Agent row's actual *value*, not merely the `"Agent: "`
prefix; it is the frame representation being matched against, not the
literal, that became wrap-tolerant. The check is used both for the
already-there fast path and for the polling wait (`WaitForFrameFunc` replacing
the old `WaitForFrame(ctx, false, marker)`).

## Green, three consecutive runs (post-fix)

Same command, three back-to-back runs from a clean tree, all exit 0:

- `green-1.log` — `ok github.com/n-orlov/deck/features 31.117s`
- `green-2.log` — `ok github.com/n-orlov/deck/features 31.154s`
- `green-3.log` — `ok github.com/n-orlov/deck/features 31.416s`

No path under `internal/` or `cmd/` changed — this is a test-harness-only fix;
`internal/tui.createFieldRows`'s wrapping behaviour is correct and untouched.
