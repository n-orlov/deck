# Task 1001 evidence: a live-pane hook-sourced error row is not wedged

Pins, by test, the factual half of the R76/F40 defence
(`docs/phase3g.md`'s R76 section): a session with `Status: "error"`,
`StatusSource: "hook"` and no `PaneExitStatus` is not a SPEC-sense wedge
("a row that every action refuses", SPEC.md:559-575), because `canKill`,
`canReachPane` and `canRestart` all still accept it -- only `canResume`
declines, correctly, since resume is defined for a stopped row only.

New file: `internal/tui/live_error_not_wedged_test.go`. Same package
(`tui`), so it reuses `footer_handler_agreement_test.go`'s existing
unexported helpers (`buildFooterAgreementModel`, `footerAgreementKeys`,
`parseFooterLegendSource`) rather than duplicating or editing them --
no pre-existing test file and no production `.go` file is touched by
this task's own commit.

Two tests:

- `TestLiveErrorRowIsNotWedgedByEligibilityPredicates` calls `canKill`,
  `canReachPane`, `canRestart` and `canResume` directly against the fixture
  row.
- `TestLiveErrorRowFooterAndHandlersAgree` runs the fixture row through
  every key `footer_handler_agreement_test.go` already drives (Y, x, r, R,
  A, U, dd, `\u21b5`, a, i), asserting the real rendered footer legend and
  the real single-row key handler agree key-by-key, the same check that
  file already runs for its six existing row classes.

## Red evidence

`red.log` / `red.log.exitstatus` come from a scratch worktree
(`git worktree add --detach .scratch-1001 HEAD`, removed again with
`git worktree remove --force` + `git worktree prune` -- never a mutation to
this repo's own working tree) at this task's parent commit, with the new
test file copied in and `canKill` mutated to also refuse an `error` row:

```diff
diff --git a/internal/tui/tui.go b/internal/tui/tui.go
index 73f03c4..0c1f329 100644
--- a/internal/tui/tui.go
+++ b/internal/tui/tui.go
@@ -1585,7 +1585,7 @@ func canAcknowledge(session store.Session) bool {
 // had its corpse collected or its row repaired by the time the UI sees it,
 // so canKill no longer needs a live service round trip to be trustworthy.
 func canKill(session store.Session) bool {
-	return session.Status != "stopped"
+	return session.Status != "stopped" && session.Status != "error"
 }
 
 // canUnarchive reports whether U may act on session: only a row that is
 // actually archived.
```

Run: `ci/run.sh sh -c 'cd .scratch-1001 && go test -count=1 ./internal/tui/'`.
The failure is a genuine assertion failure inside
`TestLiveErrorRowIsNotWedgedByEligibilityPredicates` (not a compile or vet
error): `canKill refused a hook-sourced error row with no PaneExitStatus --
that would make the row wedged, contradicting finding F40`. Exit status 1
(`red.log.exitstatus`).

## Green evidence

`green.log` / `green.log.exitstatus`: `ci/run.sh go test -count=1
./internal/tui/` at this task's own commit (unmutated `canKill`). Exit
status 0 (`green.log.exitstatus`).
