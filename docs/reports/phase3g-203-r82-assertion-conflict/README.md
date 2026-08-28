# Task 203 — F27: R82's unchanged-PTY-assertion condition, resolved as the `SPEC.md` contradiction it is

**Requirement**: independent review (finding 4, `/run/ralphd/review-findings.md`)
found that `docs/reports/phase3g-findings.md` §2.3 and `docs/reports/phase3g.md`'s
R82 section had dismissed task 016's create-modal unsatisfiable as a
standing-rule collision, when it is in fact a genuine `SPEC.md`-vs-PRD
contradiction with no operator ruling waiving it. This task files that
contradiction as finding F27 (side by side quote of `SPEC.md:1357` and the
PRD's "unchanged" clause, citing `adb7db4`), resolves it per the PRD's own
`SPEC.md`-wins precedence rule (`prds/phase3g-field-backlog.md:21-22`), and
proves the resolution — narrowing `clientCWDFieldShowsNoGhostText`'s scan to
the cwd field's own rows — is load-bearing rather than a rubber stamp, with
two experiments.

See `docs/reports/phase3g-findings.md`'s F27 row and `docs/reports/phase3g.md`'s
R82 section for the full write-up. This directory carries only the two
experiment logs.

## Experiment 1 — the ORIGINAL whole-grid assertion, restored and run: red

[`original-assertion-red.log`](original-assertion-red.log). The pre-`adb7db4`
body of `clientCWDFieldShowsNoGhostText` (a whole-grid "no dimmed cell
anywhere" scan) was restored verbatim into the current, themed tree, working
tree only, and run against `create_cwd_ghost.feature`. It fails exactly where
task 016's own reproduction predicted: the Name field's help line, dimmed per
`SPEC.md:1357`, at row 3 column 2 — not the cwd ghost. Reverted immediately
after capture; `git diff features/create_cwd_ghost_test.go` returned to the
narrowed, current version (confirmed identical to the pre-experiment file via
`diff`).

## Experiment 2 — the narrowed scan's positive control: red then green

[`positive-control-red.log`](positive-control-red.log). A new step,
`clientCWDFieldShowsGhostText`, built on the exact same bounded-scan helpers
(`cwdFieldRowBounds`, `cwdFieldDimmedCell`) the narrowed
`clientCWDFieldShowsNoGhostText` now uses, is wired into
`create_cwd_ghost.feature`'s "a unique directory match..." scenario. With
`internal/tui/tui.go`'s `createCWDGhostSuffix` mutated (working tree only) to
unconditionally `return ""` — disabling ghost rendering for every field, not
just the cwd field — the new step goes red on its own:
`client "A" has no dimmed-token cell inside the cwd field's own rows 4-5,
want a ghost there`. Both the mutation and the one feature-file line moved
to isolate the failure were reverted immediately after capture; the log's
own "revert confirmation" run shows all 6 `create_cwd_ghost.feature`
scenarios green again, unmodified.

## Final state

`internal/tui/tui.go` has no diff against `HEAD` — the positive-control
mutation never landed. `features/create_cwd_ghost_test.go` and
`features/create_cwd_ghost.feature` carry the new shared helpers, the new
`clientCWDFieldShowsGhostText` step, and its one line wired into the
scenario — the actual, committed change. Both `create_cwd_ghost.feature` and
`create_session.feature` pass targeted at this commit (22 scenarios, 224
steps, all green).
