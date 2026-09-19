# Retake: internal/tui/interactive_scroll_persist_test.go at cure-01-01-2's leaves

Task **retake-01-01-01-2**, depending on **cure-01-01-2** (R133's
"heal the stored offset from the used one; render clamped position and
preserve the next input position" fix, commit `8d934d1`). Tree sha this
retake was taken at: `f13c848ee21da85d00dfa2e26026ab640f9624ef` (HEAD at
the time of this task, clean and equal to `origin/main` — no code has
landed since cure-01-01-2's own fix commit `8d934d1`; `f13c848` and
`8f8e9e8` after it only touched `internal/tui/settings_groups*.go` and its
own test file for cure-01-02-2, not this file).

## Why this file needed a retake

`interactive_scroll_persist_test.go` originates from cure-01-01 (`60a551d`)
to cover the persistent-model healing property the review found missing
from `interactive_scroll_heal_test.go`'s direct-helper test. cure-01-01-2's
fix commit `8d934d1` moved the scroll position into the shared
`interactiveScrollState` cell (read via `m.interactiveScrollOffset()`,
written via `m.setInteractiveScrollOffset(n)`) and, in the same commit,
edited this file's own two tests (`git show 8d934d1 --stat` lists it,
+12/-... lines) to match the new accessor shape. This retake re-runs both
tests in the file at the tree cure-01-01-2 leaves, to confirm they still
pass at the sha the cure actually landed at (not merely at the moment the
cure's own commit was authored).

## Evidence (`ci/run.sh`, docker sibling, at `f13c848`)

Targeted (both tests in this file):

```
ci/run.sh go test -count=1 -v -run \
  'TestStoredScrollOffsetHealedAcrossViewAndNextScroll|TestInteractiveDispatcherNilDoesNotSnapStoredOffset' \
  ./internal/tui/
```

Result: both `PASS`, `ok` overall — [`targeted.log`](targeted.log).

Full package (regression check, same sha):

```
ci/run.sh go test -count=1 ./internal/tui/
```

Result: `ok` — [`package.log`](package.log).

## Verdict

`internal/tui/interactive_scroll_persist_test.go`'s two tests
(`TestStoredScrollOffsetHealedAcrossViewAndNextScroll`,
`TestInteractiveDispatcherNilDoesNotSnapStoredOffset`) are re-taken
(re-recorded) at the tree cure-01-01-2 leaves and both pass, with the
surrounding `internal/tui` package also green at the same sha.
