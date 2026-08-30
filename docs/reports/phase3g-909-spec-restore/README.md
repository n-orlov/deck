# Task 909 — restore SPEC.md, carry the operator's §11.8 wording forward

Review finding 3: `b69b5ba` (task 205) edited protected `SPEC.md` under a
steering note's own licence, which the review contract does not honour —
only a ruling present under read-only `/config/amendments/` does, and the
sole such ruling (`001-202.md`) grants no protected-path exception.

This task's own commit forward-reverts `b69b5ba`'s nine `SPEC.md` lines
(`git revert --no-commit b69b5ba`, applied cleanly, no conflicts) so that:

- `git diff 1cfbd5a..HEAD -- SPEC.md prds/ ci/Dockerfile ci/SPIKE.md` prints
  nothing.
- `git log --all --oneline 1cfbd5a.. -- SPEC.md prds/ ci/Dockerfile ci/SPIKE.md`
  lists exactly `b69b5ba` and this task's own revert commit.
- `b69b5ba` itself is untouched, unamended, unrebased — it stands in
  published history exactly as before.

The operator's steering-018 §11.8 wording — the exact text `b69b5ba` added —
is carried forward verbatim as new finding **F41** in
[`docs/reports/phase3g-findings.md`](../phase3g-findings.md), which states:

- the amendment is the operator's own act to land, outside this run
  (this job may not modify `SPEC.md`);
- `b69b5ba` is in published history and published history is never
  rewritten, so the remedy here is a forward revert, never an amend,
  rebase or force-push;
- R93's shipped drag-selection behaviour (`internal/tui/interactive_select.go`,
  `internal/interactive/grid.go`'s selection-aware render, all unmodified by
  this task) is therefore specified by direct operator ruling but omitted
  from `SPEC.md` itself, a disclosed gap rather than a claim of missing
  behaviour.

## Verification

`ci/run.sh go test -count=1 ./internal/tui/ ./internal/interactive/`:
[`test.log`](test.log) / [`test.exitstatus`](test.exitstatus) — exit `0`,
both packages `ok`. R93's product code and tests are untouched by this
commit; only `SPEC.md` and `docs/` change (`git show --stat`).
