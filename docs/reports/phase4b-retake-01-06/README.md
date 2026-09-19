# Retake: internal/store/group_crud_test.go at cure-01-06's leaves

Task: retake-01-06-01 (depends on cure-01-06).

## What this re-takes

`internal/store/group_crud_test.go` was last confirmed before cure-01-06 landed. cure-01-06
(57e1a6a, R128/R130: `groups.id` moved to `INTEGER PRIMARY KEY AUTOINCREMENT` so a deleted
group's id can never be reissued to an unrelated new one) touches the exact create/delete path
this file exercises, and its own commit records 3 regressions proven red pre-fix and green
post-fix in this file (among others). This retake re-runs the file's own tests at HEAD, which
carries cure-01-06 and the rest of the cure wave plus all prior retakes.

- HEAD at retake time: `683683d` (docs: re-take settings_groups_test.go at cure-01-05's leaves)
- `git merge-base --is-ancestor 57e1a6a HEAD` confirms cure-01-06 is an ancestor of HEAD.

## What was run

Targeted (all 7 test functions defined in the file):

```
ci/run.sh go test -v -run 'TestCreateGroupRejectsReservedDefaultName|TestCreateGroupRejectsCaseInsensitiveDuplicate|TestCreateGroupRejects33CharacterName|TestCreateGroupAccepts32CharacterName|TestRenameGroupUpdatesOneRowAndCarriesItsTwoMembers|TestGroupNameRejectsControlCharactersAnywhere|TestDeleteGroupThenCreateNewGroupNeverReusesTheDeletedID' -count=1 ./internal/store/
```
→ exit 0, all 7 `--- PASS` (see `targeted.log`).

Full package, to confirm no regression elsewhere:

```
ci/run.sh go test -count=1 ./internal/store/
```
→ exit 0, `ok  github.com/n-orlov/deck/internal/store  3.275s` (see `package.log`).

## Result

All tests in `internal/store/group_crud_test.go` PASS at cure-01-06's leaves, including
`TestDeleteGroupThenCreateNewGroupNeverReusesTheDeletedID` which directly exercises the
AUTOINCREMENT fix. No regression found; no code changes were needed for this retake.
