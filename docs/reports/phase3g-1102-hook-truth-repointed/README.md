# Task 1102 — re-point status_claude_hooks.feature's StopFailure assertion onto the repaired state

Task 1101 reverted `internal/service/reconcile.go`'s live-pane self-heal (R76) back to
unconditional over every terminal row (`stopped` or `error`, whatever the status source),
per `SPEC.md:560`'s terminal-row precedence sentence: **"A terminal row with a live pane is
an invariant violation, and the same pass repairs it."** With the repair unconditional again,
task 903's re-pointing of `features/status_claude_hooks.feature`'s `StopFailure` assertion
onto the hook's own bare `error`/`tool_failure` verdict is exactly the scenario-side casualty
the findings report's F40 row records: `docs/reports/phase3g-findings.md`'s F40 documents
the "SPEC-internal contradiction" between the transition table (`SPEC.md:485`), the
precedence rule (`SPEC.md:509,512`), and the self-heal paragraph's own wedge definition
(`SPEC.md:568-570`) — and records that task 902/903's narrowing is superseded once the
repair widens back to unconditional, restoring "both scenarios keep, or (for
`status_claude_hooks.feature`) regain, their original hook-verdict assertions rather than
being re-pointed onto a repair that no longer reaches them" is itself reversed the other
way: the repair now DOES reach the bare hook `error` row again, so the scenario must go back
to asserting the post-repair state.

## What changed

`git apply -R docs/reports/phase3g-903-hook-error-verdict/status_claude_hooks.feature.diff`
— the exact inverse of the tracked diff that task 903 applied. No step, scenario or
assertion was deleted, skipped or tagged out; the comment block and the `Then` assertion for
the `StopFailure` block are restored verbatim to their pre-903 text (repaired to `starting`
from `tmux`, reason `tmux pane is alive; terminal row corrected`, notify_epoch 3), and the
following `UserPromptSubmit` block's notify_epoch expectation (3) is unchanged and stays
consistent with the restored repair's epoch bump.

## Evidence — three consecutive green runs

Command (identical each run):
`ci/run.sh env DECK_GODOG_PATHS=status_claude_hooks.feature go test ./features/ -run TestFeatures -count=1`

| run | log | exit status file | result |
|-----|-----|-------------------|--------|
| 1 | `run1.log` | `run1.log.exitstatus` | `0` |
| 2 | `run2.log` | `run2.log.exitstatus` | `0` |
| 3 | `run3.log` | `run3.log.exitstatus` | `0` |

All three: `ok  	github.com/n-orlov/deck/features	<Ns>`.
