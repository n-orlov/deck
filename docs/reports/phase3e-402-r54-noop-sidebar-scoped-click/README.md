# Task 402 — scoping the click-locating helper to the sidebar panel

## 1. What changed and why

Task 401 (`docs/reports/phase3e-401-r54-noop-discriminator/README.md`)
root-caused why the R54 no-op scenario
(`@requirement-54-sidebar-click-on-interactive-row-is-a-no-op`,
`features/mouse.feature`) passed even with its own no-op guard
(`index == m.selected`, `internal/tui/mouse.go:285`) deleted: the scenario's
own second click never reaches `retargetInteractiveSidebarClick` a second
time at all, mutant or not. `clientClicksOnRowContaining`
(`features/mouse_bindings_test.go`) calls `locateText`, which searched the
WHOLE frame in row order for the target session's name and clicked the
FIRST match. Once a session is the interactive target, its name also
appears in the preview's own top border (`clientPreviewTopBorderContains`'s
own doc comment names this as SPEC's safeguard) — and that border is frame
row 0, which sorts before the sidebar's own row. So the "second click on
`retarget-noop-b`'s row" landed on the preview's border instead, which
falls through to the (unrelated, pre-existing) drag-to-copy switch and is a
no-op for a reason that has nothing to do with task 313's guard.

401's own §4 enumerated the candidate fixes and chose: **scope the
click-locating helper to the sidebar panel's own rows/columns**, reusing
the same shape-based detection (`detectLayoutMode`/`seamColumn`) that
`previewTopBorderText` already uses for the mirror-image problem on the
preview side. The existing `@deck_isize_owner` ownership-claim assertion
(`features/interactive_retarget_test.go`) is kept as-is — task 401 found it
sound in principle, just never exercised.

## 2. The fix

`features/mouse_bindings_test.go`:

- New `sidebarRegion(frame string) (rowStart, rowEnd, colEnd int, err error)`:
  returns the row bounds (0-based, inclusive) and column bound (0-based,
  exclusive; `-1` = whole line) of the sidebar panel, by the same
  first-bordered-line detection `detectLayoutMode` uses for `topIdx`, then:
  - **stacked** mode: rows `topIdx` down to the first bordered line found
    below it (the sidebar's own bottom border — the next such line after
    that is the *preview's* top border, which stacked mode draws as an
    independent box).
  - **side-by-side/collapsed** mode: the whole shared box's rows (down to
    the shared bottom border), columns `0` to `seamColumn(frame)`
    (exclusive) — the seam is constant down every row (confirmed against
    `features/testdata/golden/side_by_side_80x24.golden`), so this is a
    single column bound for the whole box, not just the top row.
- `locateText` now searches only inside those bounds instead of the whole
  frame in row order. Column indices returned are unchanged (relative to
  each line's own start, not shifted), so `clientClicksOnRowContaining`'s
  1-based coordinate math is untouched.

No product code (`internal/`) changed. No scenario in `features/mouse.feature`
changed, deleted, or retagged (`git diff` of that file for this task is
empty). No `time.Sleep`/`milliseconds pass` step added.

## 3. Proof: clean code, tag alone, green

```
ci/run.sh sh -c 'DECK_GODOG_TAGS=@requirement-54-sidebar-click-on-interactive-row-is-a-no-op go test -count=1 -v -run TestFeatures ./features/'
```

Exit 0. `clean-tag-alone.log`:

```
1 scenarios (1 passed)
21 steps (21 passed)
--- PASS: TestFeatures (1.69s)
    --- PASS: TestFeatures/a_sidebar_click_on_the_already-interactive_row_triggers_no_resize (1.69s)
```

## 4. Proof: the 401 mutation applied, tag alone, RED — failure names the no-op assertion

Applied `docs/reports/phase3e-401-r54-noop-discriminator/mutation.diff`
(deletes `index == m.selected` from `retargetInteractiveSidebarClick`,
`internal/tui/mouse.go:285`) on top of this task's fix, ran the same tag
alone:

Exit 1. `mutant-tag-alone.log`:

```
Scenario: a sidebar click on the already-interactive row triggers no resize # mouse.feature:93
  And the private tmux window ownership claim for session "retarget-noop-b" still matches "retarget-noop-b-after-retarget" # mouse.feature:136
    Error: after scenario hook failed: private tmux window ownership claim
    for session "retarget-noop-b" changed (a leave+re-enter re-claimed it):
    captured "retarget-noop-b-after-retarget" as
    "154d2304c5d30e44:207", now "db178951e61c9871:207"

1 scenarios (1 failed)
21 steps (17 passed, 1 failed, 3 skipped)
--- FAIL: TestFeatures (2.71s)
    --- FAIL: TestFeatures/a_sidebar_click_on_the_already-interactive_row_triggers_no_resize (2.71s)
```

This is exactly the no-op assertion at `mouse.feature:136` — not a
precondition step — and it now fails under the mutant precisely because
the second click reliably reaches `retargetInteractiveSidebarClick` a
second time (the fix's whole point). The mutation was fully reverted
(`git checkout -- internal/tui/mouse.go`) after this run.

## 5. Proof: `git revert --no-commit e6e1af3` (task 313's whole feature), tag alone, RED

```
git revert --no-commit e6e1af3   # "tui: hit-test a sidebar click first while interactive, re-targeting it (313)"
ci/run.sh sh -c 'DECK_GODOG_TAGS=@requirement-54-sidebar-click-on-interactive-row-is-a-no-op go test -count=1 -v -run TestFeatures ./features/'
git revert --abort
```

Exit 1. `revert-e6e1af3-tag-alone.log`:

```
And deck client "A" has session "retarget-noop-b" selected # mouse.feature:129
  Error: after scenario hook failed: deck client "A" does not have
  session "retarget-noop-b" selected: timed out waiting for frame
  "> retarget-noop-b": context deadline exceeded

1 scenarios (1 failed)
21 steps (10 passed, 1 failed, 10 skipped)
--- FAIL: TestFeatures (7.49s)
```

As the scenario's own comment predicts (`mouse.feature:106-118`): with the
whole retarget feature reverted, the *retarget* step itself (moving
selection from `retarget-noop-a` to `retarget-noop-b`) fails first, before
the no-op check is ever reached — still red, as required, just at a
different, expected step than §4's. `git revert --abort` restored the tree
(`git status --short` empty afterward).

## 6. `internal/tui` unchanged

```
ci/run.sh go test -count=1 ./internal/tui/
```

`internal-tui-package.log`: `ok  	github.com/n-orlov/deck/internal/tui	0.564s`
— `internal/tui/mouse_interactive_retarget_test.go` still passes; this
task touched no product code.

## 7. What was NOT done here (deliberately out of scope)

- The scenario's three `And 200 milliseconds pass` steps are untouched —
  task 403's job, not this one.
- `features/mouse.feature` itself is byte-identical to before this task
  (`git diff features/mouse.feature` is empty) — the existing
  `@deck_isize_owner` assertions at lines 131/136 already say exactly what
  402's success criteria ask for; what was broken was the click reaching
  the code path those assertions discriminate, not the assertions'
  wording.
