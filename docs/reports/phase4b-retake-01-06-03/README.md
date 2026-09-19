# Retake retake-01-06-03: internal/tui/create_last_used_group_test.go at cure-01-06's leaves

## What this is

Task `retake-01-06-03` re-takes (re-records) the result of
`internal/tui/create_last_used_group_test.go` at a HEAD that carries cure-01-06's fix
(groups.id switched to `INTEGER PRIMARY KEY AUTOINCREMENT`, so a deleted group's id is
never reissued to an unrelated new group). cure-01-06's own commit message
(57e1a6a) names this file as one of the three regressions it proved red pre-fix and
green post-fix.

## Ancestry check

HEAD at the time of this retake: `4266a13` (retake-01-06-02, "docs: re-take
ui_state_groups_test.go at cure-01-06's leaves").

```
$ git merge-base --is-ancestor 57e1a6a HEAD && echo "ancestor OK"
ancestor OK
```

cure-01-06 (`57e1a6a`) is confirmed an ancestor of the commit this retake runs
against, so the leaves being re-taken genuinely carry the fix.

## What ran

All 4 test funcs in `internal/tui/create_last_used_group_test.go`:

- `TestCreateGroupDefaultDegradesWhenRememberedGroupDeleted`
- `TestRememberedCreateGroupSurvivesRestart`
- `TestCreateIntoDefaultGroupForgetsTheRememberedNamedGroup`
- `TestRememberedCreateGroupDoesNotRebindAcrossRestartAfterDeleteAndReplacement`

Invocation (via `ci/run.sh`, the Go toolchain sibling; see `targeted.log`):

```
ci/run.sh go test -count=1 -run 'TestCreateGroupDefaultDegradesWhenRememberedGroupDeleted|TestRememberedCreateGroupSurvivesRestart|TestCreateIntoDefaultGroupForgetsTheRememberedNamedGroup|TestRememberedCreateGroupDoesNotRebindAcrossRestartAfterDeleteAndReplacement' -v ./internal/tui/
```

Result: all 4 PASS (0.095s), see `targeted.log`.

Then the whole `internal/tui` package, to confirm no regression elsewhere:

```
ci/run.sh go test -count=1 ./internal/tui/ -v
```

Result: PASS, 6.409s, see `package.log`.

## Conclusion

`create_last_used_group_test.go`'s 4 tests are green at cure-01-06's leaves (and at
every retake commit merged in since). No code change was needed — this task is a
re-take (evidence re-recording), not a fix. No regression found.
