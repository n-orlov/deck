# Task 903 — restore StopFailure's assertion to the hook's own verdict

## What changed and why

`bedb65a` (task 803) re-pointed `features/status_claude_hooks.feature`'s
`StopFailure` assertion off the hook's own `error`/`tool_failure` verdict and
onto the post-repair state (`starting`/`tmux`), because at that time SPEC
§7's live-pane self-heal (`internal/service.reconcile`'s
`repairTerminalRowWithLivePane`, R76) was unconditional (`89edd3c`, task 701)
and reached every non-`stopped` row, including a bare hook-sourced `error`.

Task 901's finding (F40, `docs/reports/phase3g-findings.md`) concluded that
reading contradicts SPEC's own precedence rule ("**Precedence:**
`user-terminal` > `hook` > `probe` > `tmux`", `SPEC.md:509,512`) and its
self-heal wedge definition (`SPEC.md:568-570`): the repair exists to unwedge
a `stopped` row under a live pane (the row every action refuses), not to
overwrite a higher-precedence `error` verdict that carries no pane-exit
status. Task 902 implemented the narrowed rule: repair `stopped`
unconditionally; repair `error` only when it carries a pane-exit verdict or a
`tmux`-/`user`-sourced verdict; never a bare `hook`- or `probe`-sourced
`error`.

The `StopFailure` scenario's `error`/`tool_failure` verdict here is
hook-sourced and carries no pane-exit status, so after task 902's change the
repair no longer reaches it. This commit reverts `bedb65a`'s re-pointing in a
new commit — never by amend, rebase or force-push — restoring the original
assertion:

> the state database session "hook truth" has hook status "error", reason
> "tool_failure", message "permission granted; work is complete",
> acknowledged 0, and notify_epoch 2

and replaces the comment block above it with one that names task 901's
finding (F40) and the SPEC precedence rule, instead of describing the
now-superseded repair behaviour.

The scenario's step count and every other assertion (including the
following `UserPromptSubmit`'s `notify_epoch 3`, which was already correct
in the pre-`bedb65a` version — the epoch bump there does not come from the
repair) are unchanged. No tag or `@requirement-` marker changes, no
`t.Skip` is added, and `features/godog_test.go` is not touched.

## Diff of the file this commit changes

`docs/reports/phase3g-903-hook-error-verdict/status_claude_hooks.feature.diff`
is `git diff` of this commit's change to `features/status_claude_hooks.feature`
alone: only the `StopFailure` assertion and its preceding comment block
change; every other line is untouched.

## Evidence

Command, run three times from a clean tree at this commit:

```
ci/run.sh env DECK_GODOG_PATHS=status_claude_hooks.feature go test ./features/ -run TestFeatures -count=1
```

| run | log | exit |
| --- | --- | --- |
| 1 | `run1.log` | `run1.log.exitstatus` = 0 |
| 2 | `run2.log` | `run2.log.exitstatus` = 0 |
| 3 | `run3.log` | `run3.log.exitstatus` = 0 |

All three runs exit 0 (`ok github.com/n-orlov/deck/features`).
