# Task 905 — comment accuracy for the narrowed live-pane repair

Task 905 is a comment-only sweep: after task 902 (`a1ca33e`) narrowed
`repairTerminalRowWithLivePane`'s trigger, no comment in the tree may still say
the repair overwrites a hook- or probe-sourced `error` row that carries no
pane-exit verdict.

## Round 1 — `0ddc4bc` (comment fixes) + `6193cae` (log)

Corrected `internal/service/reconcile.go` (function doc comment),
`internal/tui/tui.go` (two sites), all six occurrences in
`features/sort_order.feature`, `features/sort_order_test.go`,
`features/crash_test.go`, `features/status_theme.feature`,
`features/themes.feature`, `features/attention_sort.feature`,
`features/interactive_scroll_test.go` and
`features/status_claude_hooks_test.go`.

- `log.log` / `log.log.exitstatus` — `ci/run.sh go test -count=1
  ./internal/service/ ./internal/tui/`, exit **0**.

## Round 2 — `4b148cb` (comment fixes) + this commit (log)

Validation of round 1 found two sites still describing the trigger as
unconditional over `error` rows. Both are corrected in `4b148cb`, comment lines
only:

- `internal/store/store.go`, the `tmuxTerminalRepair` gate's comment block —
  was "a terminal row (stopped/error) paired with a live pane is an invariant
  violation the reconciler repairs from tmux liveness alone"; now states that
  eligibility is decided by `reconcile.go`'s terminal-row branch (stopped
  always; `error` only with a pane-exit or `tmux`/`user`-sourced verdict), that
  a bare hook/probe `error` is never repaired (finding F40, task 901), and that
  no such write reaches this predicate — the predicate only admits the repair
  write that does arrive.
- `internal/service/reconcile_terminal_repair_test.go`, the shell repair test's
  doc comment — was "a terminal (stopped/error) status paired with a live,
  non-dead pane"; now names the stopped case it actually exercises, the
  pane-exit / tmux-user-sourced `error` case, and the excluded bare hook/probe
  `error`.

Guards, both run at `4b148cb`:

    git show 4b148cb -U0 | grep -E '^[+-]' | grep -vE '^(\+\+\+|---)' \
      | grep -vE '^[+-][[:space:]]*(//|#)'      # prints nothing
    ci/run.sh go build ./...                     # exit 0
    ci/run.sh go vet ./internal/store/ ./internal/service/   # exit 0
    ci/run.sh gofmt -l <the two touched files>   # prints nothing

- `round2-service-tui.log` / `round2-service-tui.log.exitstatus` —
  `ci/run.sh go test -count=1 ./internal/service/ ./internal/tui/` at
  `4b148cb`, exit **0** (`ok internal/service 4.474s`, `ok internal/tui
  1.340s`).
