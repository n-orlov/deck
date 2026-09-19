# Retake: internal/tui/group_order_test.go at cure-01-02's leaves

Task **retake-01-02-01**, depending on **cure-01-02** (R129 empty/default
groups + R131 another-client-reload fix, commit `db0f94b`). Tree sha this
retake was taken at: `d99dc5c2cf1a213e71f80fc6b03556fd509b0185` (current
HEAD at the time of this task — this HEAD carries the whole cure-01-01..06
wave, all landed after cure-01-02, plus retake-01-01-01's own docs commit;
cure-01-02 itself did not touch this test file).

## Why this file needed a retake

`group_order_test.go` exercises group ordering (alphabetical, default
always-last regardless of empty-string sort position) and the header-text
helper (`groupHeaderText`) for populated/empty groups, elision at the
sidebar floor, and per-mode content-width budgets. cure-01-02 changed how
the unfiltered sidebar is assembled (`sidebarEntries`/`groupSessions` in
`tui.go`) so that persisted groups — including empty ones and the
structural default — render correctly and a reload picks up another
client's edits. None of that touched `group_order_test.go`'s own file, but
the review that drove cure-01-02 flagged that some tests in this area
called `groupHeaderText` on a fabricated/synthetic `sidebarGroup` rather
than through real discovery/rendering
(`TestGroupHeaderTextCountsPopulatedAndEmptyGroups` does exactly that). This
retake re-runs the whole file's tests at cure-01-02's leaves to confirm
they still pass and were not invalidated by the sidebar-assembly fix
landing elsewhere.

## Evidence (`ci/run.sh`, docker sibling, at `d99dc5c`)

Targeted (all six tests/subtests in this file):

```
ci/run.sh go test -count=1 -v -run \
  'TestGroupOrderAaaLeadsAlphabetically|TestGroupOrderZzzSortsBeforeDefaultDespiteName|TestGroupOrderDefaultAlwaysLastRegardlessOfEmptyStringSortingFirst|TestGroupHeaderTextCountsPopulatedAndEmptyGroups|TestGroupHeaderTextElidesNameAtSidebarFloorKeepingChevronAndCount|TestSidebarEntryContentWidthMatchesEachModesRealTextBudget' \
  ./internal/tui/
```

Result: all `PASS`, `ok` overall — [`targeted.log`](targeted.log).

Full package (regression check, same sha):

```
ci/run.sh go test -count=1 ./internal/tui/
```

Result: `ok` — [`package.log`](package.log).

## Verdict

`internal/tui/group_order_test.go`'s tests
(`TestGroupOrderAaaLeadsAlphabetically`,
`TestGroupOrderZzzSortsBeforeDefaultDespiteName`,
`TestGroupOrderDefaultAlwaysLastRegardlessOfEmptyStringSortingFirst`,
`TestGroupHeaderTextCountsPopulatedAndEmptyGroups`,
`TestGroupHeaderTextElidesNameAtSidebarFloorKeepingChevronAndCount`,
`TestSidebarEntryContentWidthMatchesEachModesRealTextBudget`) are re-taken
(re-recorded) at the tree cure-01-02 leaves and all pass, with the
surrounding `internal/tui` package also green at the same sha. No code
changes were needed — no regression found.
