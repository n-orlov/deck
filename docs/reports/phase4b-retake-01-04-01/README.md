# Re-take: internal/tui/settings_groups_test.go

Task: `retake-01-04-01`. Depends on `cure-01-04`, `cure-01-05`, `cure-01-02-2` (all
validated). This re-runs the file at the tree cure-01-02-2 leaves — HEAD `0d966e4`
(docs-only commit; the last code change ahead of the previous full-suite gate was
cure-01-02-2's own `f13c848`).

## Command and result

```
ci/run.sh go test -count=1 -run TestSettingsGroups -v ./internal/tui/...
```

All 9 top-level tests (and their subtests, listed below) PASS at `0d966e4`:

- `TestSettingsGroupsCategoryIsSynthetic`
- `TestSettingsGroupsCopyStatesEditsApplyImmediately`
- `TestSettingsGroupsEscOffersNoDiscardPrompt`
- `TestSettingsGroupsEscDuringCreateCancelsOnlyTheEditorNotTheTakeover`
- `TestSettingsGroupsNCreatesGroupImmediately`
- `TestSettingsGroupsCreateShowsValidateGroupNameErrorInline`
- `TestSettingsGroupsRenameLeavesMemberGroupIDsUntouched`
- `TestSettingsGroupsListLongerThanViewportKeepsSelectionAndEditorVisible`
  (subtests: 80x24/create, 80x24/create_rejected_name_keeps_the_validation_visible,
  80x24/rename_rejected_name_keeps_the_validation_visible, 80x24/rename, 80x24/delete,
  and the same five under 100x40)
- `TestSettingsGroupsWindowKeepsSelectedBlockWhollyInsideCapacity`

```
PASS
ok  	github.com/n-orlov/deck/internal/tui	0.159s
```

Package-wide confirmation (same sha):

```
ci/run.sh go test -count=1 ./internal/tui/...
ok  	github.com/n-orlov/deck/internal/tui	8.034s
```

## Notes

`TestSettingsGroupsListLongerThanViewportKeepsSelectionAndEditorVisible`'s
`.../create_rejected_name_keeps_the_validation_visible` and
`.../rename_rejected_name_keeps_the_validation_visible` subtests are exactly the
scenario cure-01-02-2 attempt 2 (`f13c848`) fixed (reserving the inline validation
note's row out of window capacity via a measured `selectedExtra` block, after attempt 1
`8f8e9e8` was rejected for appending the note after the row budget was already spent).
Both pass here.
