# Retake: internal/tui/interactive_scroll_heal_test.go at cure-01-01's leaves

Task **retake-01-01-01**, depending on **cure-01-01** (R133 part 1's
"heal the stored offset" fix, commit `60a551d`). Tree sha this retake was
taken at: `57e1a6a62c849c0f0b3962371bf117f2beb903cc` (HEAD at the time of
this task — the tree also carries cure-01-02..06, landed after cure-01-01;
cure-01-01 itself did not touch this test file, so its own leaf and this
later HEAD agree on the two tests below).

## Why this file needed a retake

`interactive_scroll_heal_test.go` is R133 part 1's own red-first proof: it
drives `interactiveBodyLines` directly and checks the render-local healed
offset. The independent review (`artifacts/review/tui-review-prd-test.go`)
found this direct-helper test passes but does not exercise the *persistent*
model bubbletea keeps between `Update` calls — the actual defect cure-01-01
fixed lives in `scrollInteractiveByLines` (`interactive_scroll.go`), not in
this file's own code path. cure-01-01 (`60a551d`) added a separate file,
`interactive_scroll_persist_test.go`, to cover the persistent-model property;
it left `interactive_scroll_heal_test.go` itself unmodified. This retake
re-runs `interactive_scroll_heal_test.go`'s own two tests at the tree
cure-01-01 (and the rest of the cure wave) leaves, to confirm they still
pass and were not invalidated by the fix landing elsewhere.

## Evidence (`ci/run.sh`, docker sibling, at `57e1a6a`)

Targeted (both tests in this file):

```
ci/run.sh go test -count=1 -v -run \
  'TestInteractiveBodyLinesHealsTheStoredOffsetToTheClampedUsedOffset|TestInteractiveBodyLinesHealingReenablesTheNotRepaintedNotice' \
  ./internal/tui/
```

Result: both `PASS`, `ok` overall — [`targeted.log`](targeted.log).

Full package (regression check, same sha):

```
ci/run.sh go test -count=1 ./internal/tui/
```

Result: `ok` — [`package.log`](package.log).

## Verdict

`internal/tui/interactive_scroll_heal_test.go`'s two tests
(`TestInteractiveBodyLinesHealsTheStoredOffsetToTheClampedUsedOffset`,
`TestInteractiveBodyLinesHealingReenablesTheNotRepaintedNotice`) are
re-taken (re-recorded) at the tree cure-01-01 leaves and both pass, with
the surrounding `internal/tui` package also green at the same sha.
