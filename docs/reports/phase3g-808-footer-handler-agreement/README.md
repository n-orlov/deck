# Task 808: the footer/handler agreement test

`internal/tui/footer_handler_agreement_test.go` adds
`TestFooterHandlerAgreementAcrossEveryRowClass`: a generic matrix test that
drives the real key handler (through `Model.Update`) for every one of the
seven footer action keys the task names -- `Y`, `x`, `r`, `R`, `A`, `U` and
the `dd` chord -- against a fresh `Model` built for each of the six row
classes the task names (empty list, live/running, stopped, archived,
attention-pending, and a non-empty mark set whose selected row and marked
row disagree), and asserts, for every one of the 7*6 = 42 pairs, that the
handler acted if and only if the *real* rendered footer legend
(`Model.footerKeyLegend()`) advertised that key -- both directions, so a
handler that silently acts on a row the footer refuses to name is caught
just as surely as the reverse.

Command: `ci/run.sh go test -count=1 ./internal/tui/`

## Why this test is new, not a duplicate of 806/807

- 806 (`archive_eligibility_test.go`) and 807 (`kill_key_eligibility_test.go`)
  each cover exactly one key (`A`, `x`) with a *structural* source-parse
  check (catches an inlined-but-equivalent predicate copy) plus a narrow
  behavioural check for that one key.
- Nothing before this task drove `R`, `U` or `Y`'s real handler at all and
  cross-checked the result against the footer's own rendering. This test
  is generic across all seven keys and is deliberately behavioural: the
  footer side of the comparison is `footerKeyLegend()`'s actual text, and
  the handler side is what pressing the real key really does (a recorded
  service call, or a dialog flag the handler itself sets) -- never a
  second call to `footerRowEligible`, which would just repeat the function
  under test.

## Red: mutation demo (`case "R"`'s eligibility check removed)

`internal/tui/tui.go`'s `case "R":` arm had its
`if !canRestart(session) { ... return ... }` guard removed for one
iteration (no other line touched), so pressing `R` on a stopped row would
call `m.restart` unconditionally even though the footer -- which still
calls `canRestart` -- correctly stops advertising `R` for that row. Nothing
covered this divergence before this task: no existing test presses `key("R")`
on a stopped row at all.

- `red-mutation-no-canRestart-check.log` -- `ci/run.sh go test -run
  TestFooterHandlerAgreementAcrossEveryRowClass -v -count=1
  ./internal/tui/`, **exit 1**. Exactly the two subtests where the row's
  `Status == "stopped"` (`stopped/R` and `archived/R`, since an archived
  row is also stopped) fail; every other of the 42 subtests still passes.
- `red-mutation-full-package.log` -- `ci/run.sh go test -count=1
  ./internal/tui/`, **exit 1**. Confirms the mutation regresses *only*
  `TestFooterHandlerAgreementAcrossEveryRowClass` in the whole package --
  no other test in `internal/tui` reacted to this specific regression,
  which is exactly the gap this task closes.

## Green: mutation reverted

The guard was restored verbatim (confirmed by `git diff` showing no
tracked-file changes before the commit below, since the mutation is only
ever demonstrated in an untracked working-tree edit that is reverted before
committing).

- `green-mutation-reverted-full-package.log` -- `ci/run.sh go test -count=1
  ./internal/tui/`, **exit 0**.
