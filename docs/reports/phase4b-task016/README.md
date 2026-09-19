# Task 016 (R130 part 1): the create modal's Group field — re-verification at HEAD

## What this is

Task 016's criteria were rejected once (attempt 1, at `ddeab8d`): the remembered
group was updated only when the created session had a non-nil `GroupID`, so
creating into a *named* group and then into the structural *default* group left the
named group remembered — the next modal, and the next restart, opened on a group the
user had already moved off.

That defect was cured in `338a600` ("tui/store: remember a create into the default
group too (R130 part 1 cure)"), which removed the nil-gated branch in
`shellCreated`, made `store.SetLastCreateGroup` treat `id <= 0` as "default" (by
clearing the `ui_state` row, which `GetLastCreateGroup` already reads back as
default), and added two named regressions — one at the model seam, one at the store
seam — both recorded red pre-fix in that commit message.

HEAD has since moved a long way (the whole review-cure wave `cure-01-01..06`,
including `57e1a6a`'s `groups.id AUTOINCREMENT` change, plus the eight `retake-01-*`
record tasks). This report re-takes task 016's evidence at the current HEAD, in the
same CI container, so nothing about the criteria rests on a measurement taken before
those changes landed.

## Ancestry

HEAD this verification ran against: `3edc64d` ("docs: re-take
create_last_used_group_test.go at cure-01-06's leaves").

```
$ git merge-base --is-ancestor ddeab8d HEAD && echo "016 attempt 1 is an ancestor"
$ git merge-base --is-ancestor 338a600 HEAD && echo "016 cure is an ancestor"
$ git merge-base --is-ancestor 57e1a6a HEAD && echo "cure-01-06 is an ancestor"
```

All three are ancestors of `3edc64d`, so what was measured carries both halves of
task 016 *and* the cure wave that followed it.

## Criterion-by-criterion

1. **Group field cycling the groups the way Agent cycles kinds, alphabetical with
   default last** — `internal/tui/tui.go`: `createGroupCycleOptions` appends the
   structural default group (`{ID: 0, Name: "default"}`) after `m.createGroups`
   (which is `store.ListGroups`' own case-insensitive alphabetical order), and
   `cycleCreateField`'s `case 9` walks that slice with the same
   `(idx + delta + len) % len` wrap the other cycling fields use, clearing the
   `(last used)` label on the first deliberate cycle.
2. **Defaulting to the last group created into, as persisted in `ui_state`** —
   `New` reads `GetLastCreateGroup` into `m.lastCreateGroupID`; the `n` handler calls
   `pickCreateGroup`, which honours it only while it still names a group in the live
   cycle set; `shellCreated` records **every** successful create's group, default
   included, in memory and through `persistLastCreateGroup`.
3. **Named test: remembered default degrades to default rather than to an empty
   field once its group is deleted** —
   `internal/tui/create_last_used_group_test.go:TestCreateGroupDefaultDegradesWhenRememberedGroupDeleted`
   (asserts `createGroupID == 0` *and* `createGroupName(...) == "default"`, i.e. never
   blank).
4. **Named test: the remembered group survives a restart** —
   `internal/tui/create_last_used_group_test.go:TestRememberedCreateGroupSurvivesRestart`.
5. **Scenario creating a session into a named group and asserting the persisted
   `group_id` on that row** — `features/create_session.feature`,
   `@requirement-30-create-into-named-group`: "creating a session into a named group
   persists its group_id"; it drives the real modal's Group field and then asserts
   the row's stored group via "the state database session ... was created into group
   ...".
6. **The rejection's own case** —
   `TestCreateIntoDefaultGroupForgetsTheRememberedNamedGroup` (model seam) and
   `internal/store/ui_state_groups_test.go:TestSetLastCreateGroupDefaultForgetsTheRememberedNamedGroup`
   (store seam) pin that a create into default forgets a still-alive named group, in
   memory, in `ui_state`, in the next modal and after a restart.
7. **Green in the CI container** — see below.

## What ran (all via `ci/run.sh`, docker sibling, at `3edc64d`)

```
$ ci/run.sh go test -count=1 ./internal/tui/ ./internal/store/      # packages.log, exit 0
ok  github.com/n-orlov/deck/internal/tui    7.271s
ok  github.com/n-orlov/deck/internal/store  3.928s

$ ci/run.sh env DECK_GODOG_PATHS=create_session.feature go test -count=1 \
      -run TestFeatures -v ./features/                              # create_session-feature.log, exit 0
17 scenarios (17 passed) / 146 steps (146 passed)
  ... incl. "creating a session into a named group persists its group_id"

$ ci/run.sh go test -count=1 -v -run '<the four Group-field tests>' ./internal/tui/
                                                                    # targeted.log, exit 0
--- PASS: TestCreateGroupDefaultDegradesWhenRememberedGroupDeleted
--- PASS: TestRememberedCreateGroupSurvivesRestart
--- PASS: TestCreateIntoDefaultGroupForgetsTheRememberedNamedGroup
--- PASS: TestRememberedCreateGroupDoesNotRebindAcrossRestartAfterDeleteAndReplacement
```

Exit statuses were taken from the runs themselves (`echo exit=$?` into a separate
file), not read out of log prose. Logs in this directory: `packages.log`,
`create_session-feature.log`, `targeted.log`.
