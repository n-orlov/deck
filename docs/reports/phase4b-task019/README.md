# Task 019 re-verification — settings `d` deletes a group (R131 part 2)

Verified at `b8ccad4` (`git rev-parse HEAD` == `origin/main`, `git status --porcelain`
empty when the runs started). No code changed in this pass: the defect the first
attempt was rejected for — the destructive `d` branch never dropping the group row
after the batch committed — was already cured in-tree by `61b6f7d`
("settings: drop the group row once the routed dd batch commits (R131 part 2 cure)").
This pass re-checked every criterion against that HEAD and recorded the evidence.

## The rejected defect, re-checked point by point

| Rejection claim | State at `b8ccad4` |
| --- | --- |
| `sessionsBulkDeleted` handler contains no `DeleteGroup` call | It does: `internal/tui/tui.go:2850-2862` reads `m.bulkDeleteGroupID`, clears it unconditionally, and calls `m.store.DeleteGroup` when `firstErr == nil`. |
| `settingsRouteGroupDeleteToBulkConfirm` only sets `m.marked`/`m.deleteConfirming` | It also parks the routed id/name: `internal/tui/settings.go:876-877` (`m.bulkDeleteGroupID`, `m.bulkDeleteGroupName`), documented as "the row is dropped at the batch's commit point, not here". |
| `DeleteGroup` appears only in the empty and `m` branches | Three product call sites now: empty (`settings.go:760`), `m` (`settings.go:828`), destructive batch commit (`tui.go:2855`). |
| the destructive feature asserts `batch-group` is visible after undo | It asserts the opposite: `features/kill_delete_undo.feature:722` (`the state database has no group named "batch-group"` after the batch), `:730` (still gone after `u`), `:736` (`screen does not contain "batch-group"`), with `default  (2)` proving both restored rows land under default per SPEC §11's dangling-`group_id` rule. |

## Criteria → evidence

1. **Empty group prompts nothing; `default` offers no delete at all.**
   `settingsStartGroupDelete` (`internal/tui/settings.go:726`) resolves the complete
   persisted member set via `groupMemberSessions` and routes an empty one straight to
   `settingsDeleteEmptyGroupNow` (`settings.go:755`, no prompt). `default` is never a
   row in `m.settingsGroups` at all (`computeAvailableGroups`, `tui.go:1239`, returns
   only real `groups` rows), so `d` can never select it.
   Tests: `TestSettingsGroupDeleteEmptyGroupHasNoPrompt`,
   `TestSettingsGroupDeleteWithNoGroupsIsANoOp` — both PASS in `targeted-tui-group-delete.log`.
2. **Non-empty group prompts two branches; `m` moves members to default and drops the
   row destroying nothing.** `updateSettingsGroupDeleteConfirm` (`settings.go:778`)
   binds `m`/`d`/`esc`; `settingsMoveGroupMembersToDefaultAndDeleteGroup`
   (`settings.go:809`) calls `SetSessionGroup(..., 0, ...)` per member (SPEC's
   "<=0 means default", i.e. the member's `group_id` no longer names the group) before
   `DeleteGroup`. Tests: `TestSettingsGroupDeleteNonEmptyOpensTwoBranchPrompt`,
   `TestSettingsGroupDeleteMBranchMovesMembersToDefaultAndDropsGroup`,
   `TestSettingsGroupDeleteMBranchClearsArchivedMembersToo` — PASS.
3. **`d` routes into the existing §9.2 bulk `dd` path through a seam, not a second
   deletion implementation.** `settingsRouteGroupDeleteToBulkConfirm` (`settings.go:870`)
   sets exactly the fields the `dd` chord sets (`deleteConfirming`, `bulkDeleteSessions`,
   `bulkDeletePurgeValue`, `marked`) so `updateBulkDeleteConfirm` /
   `bulkDeleteConfirmBody` and `internal/service`'s delete call run unchanged.
4. **Guard test: the settings branch reaches the same service call `dd` does.**
   `TestSettingsGroupDeleteDBranchReachesTheSameServiceCallDDDoes`
   (`internal/tui/settings_groups_test.go:502`) — PASS. Containment guard
   `TestOrdinaryBulkDeleteNeverDropsAGroupRow` (`:738`) — PASS.
5. **Feature coverage.** `features/settings.feature:443` (empty-group delete, no
   prompt), `:464` (two-branch prompt + `m`), `features/kill_delete_undo.feature:688`
   (destructive `d`: both members tombstoned, group row gone, one top-level `u`
   restores the whole batch).

## Runs (CI container, `ci/run.sh`, all at `b8ccad4`)

Exit statuses as recorded by the driver: `exit-status.txt` (every line `exit=0`).

| Invocation | Result | Log |
| --- | --- | --- |
| `ci/run.sh gofmt -l features/ internal/` | no output | — |
| `ci/run.sh go build ./...` / `go vet ./...` | clean | — |
| `ci/run.sh go test -count=1 ./internal/tui/ ./internal/store/ ./internal/service/` | ok 7.276s / 4.839s / 7.932s | `packages.log` |
| `ci/run.sh go test -count=1 -run 'TestSettingsGroupDelete\|TestOrdinaryBulkDeleteNeverDropsAGroupRow' -v ./internal/tui/` | 17 tests, all PASS | `targeted-tui-group-delete.log` |
| `ci/run.sh env DECK_GODOG_PATHS=settings.feature go test -count=1 -run TestFeatures -v ./features/` | **19 scenarios (19 passed), 355 steps**, ok 12.661s | `settings-feature.log` |
| `ci/run.sh env DECK_GODOG_PATHS=kill_delete_undo.feature go test -count=1 -run TestFeatures -v ./features/` | **38 scenarios (38 passed), 515 steps**, ok 34.202s | `kill-delete-undo-feature.log` |
