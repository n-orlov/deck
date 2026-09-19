# Retake: internal/tui/settings_groups_test.go at cure-01-05's leaves

Task: retake-01-04-01 (depends on cure-01-04, cure-01-05).

## What this re-takes

`internal/tui/settings_groups_test.go` was last confirmed at cure-01-04's leaves. cure-01-05
(43b7202, "tui: offer the same non-default purge choice inside the reused group-delete batch")
landed afterwards and touches the same group-delete batch path this file exercises. This retake
re-runs the file's own tests at HEAD, which carries cure-01-04, cure-01-05 and the rest of the cure
wave (cure-01-06, 57e1a6a, groups.id AUTOINCREMENT) plus all prior retakes.

- HEAD at retake time: `ce80b11` (docs: re-take session_context_test.go at cure-01-03's leaves)
- `git merge-base --is-ancestor 43b72023 HEAD` confirms cure-01-05 is an ancestor of HEAD.

## What was run

Targeted (all 21 top-level test functions defined in the file, one with 2 subtests — 22 `--- PASS`
lines total, matched via two `-run` prefixes since one func, `TestOrdinaryBulkDeleteNeverDropsAGroupRow`,
does not share the `TestSettingsGroup` name prefix):

```
ci/run.sh go test -v -run 'TestSettingsGroup|TestOrdinaryBulkDeleteNeverDropsAGroupRow' -count=1 ./internal/tui/
```
→ exit 0, all PASS (see `targeted.log`).

Full package, to confirm no regression elsewhere:

```
ci/run.sh go test -count=1 ./internal/tui/
```
→ exit 0, `ok  github.com/n-orlov/deck/internal/tui  6.570s` (see `package.log`).

## Result

All tests in `internal/tui/settings_groups_test.go` PASS at cure-01-05's (and cure-01-06's) leaves.
No regression found; no code changes were needed for this retake.
