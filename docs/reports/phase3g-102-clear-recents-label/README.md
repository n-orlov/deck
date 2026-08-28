# Task 102 — disambiguate clear-recent-cwds from R85's Agent "(last used)" label

## Problem
`features/settings.feature`'s `@requirement-17-clear-recent-cwds-history` scenario asserted
the bare literal `"(last used)"` was gone after clearing recent cwds. R85 (task 024,
`internal/tui/tui.go:6486-6516`, `createAgentHelp`) later added the identical literal
`"(last used) "` as a prefix on the create modal's **Agent** row help
(`"(last used) which coding agent adapter launches this session"`), which is untouched by
clearing recent cwds. From that point on, the bare-literal assertion could never legitimately
pass — the Agent row still says `(last used)` whether or not the cwd prefill it was originally
about survives the clear.

## Red before the fix — `b6cbbc7` (`red-b6cbbc7-agent-label-collision.log`)
Command: `ci/run.sh env DECK_GODOG_PATHS=settings.feature go test ./features/ -run TestFeatures -count=1`
at `b6cbbc7` (before this task's edit), exit 1:

```
Scenario: settings offers clearing the recent-directory history, and clearing it costs only the prefill, never a session # settings.feature:372
  Then deck client "A" screen does not contain "(last used)" # settings.feature:402
    Error: after scenario hook failed: deck client "A" screen unexpectedly contains "(last used)":
+------------------------------------------------------------------------------+
| Create shell session                                                         |
...
|   Agent: shell (left/right cycles: claude, pi, shell)                        |
|     (last used) which coding agent adapter launches this session             |
...
```
The matched text is the Agent row's own `(last used) which coding agent adapter launches this
session`, not anything about the cleared cwd prefill — exactly the collision this task fixes.

## The fix
`features/settings.feature`'s two `(last used)` assertions (the positive check right after
seeding a cwd, and the negative check right after clearing recent cwds) are now pinned to the
cwd row's own help text specifically: `"(last used) the session's cwd"` instead of the bare
`"(last used)"`. That literal only ever appears as the prefix `createCWDHelp` puts on the
Working-directory row (`internal/tui/tui.go:6499-6516`); the Agent row's identical prefix is
followed by `"which coding agent..."`, never `"the session's cwd"`, so the two can no longer be
confused by either assertion. No assertion was deleted; only the matched string was narrowed to
the row it is actually about.

## Green after the fix — three consecutive runs (`green-run-1.log`, `green-run-2.log`, `green-run-3.log`)
Same command, same HEAD (with the fix applied), run three times back to back:
```
ok  	github.com/n-orlov/deck/features	10.284s
ok  	github.com/n-orlov/deck/features	10.397s
ok  	github.com/n-orlov/deck/features	10.356s
```

## Still load-bearing — `red-disabled-clear-loadbearing.log`
To prove the new, cwd-specific assertion is still doing real work (and is not satisfied
trivially, e.g. by the Agent row, nor vacuous), the clear itself was disabled locally with an
uncommitted one-line patch to `internal/tui/settings.go` (never committed, reverted immediately
after this run):

```diff
-	if err := m.store.ClearRecentCwds(context.Background()); err != nil {
+	_ = context.Background() // TEMP task-102 disable-and-check keeps import live
+	if err := (error)(nil); err != nil { // TEMP task-102 disable-and-check, revert before commit
```

With the store-side clear turned into a no-op (so the recent-cwd prefill and its "(last used)"
label survive "clearing"), the same command goes red again, on the *new* line and against the
*new*, disambiguated text:

```
Scenario: settings offers clearing the recent-directory history, and clearing it costs only the prefill, never a session # settings.feature:372
  Then deck client "A" screen does not contain "(last used) the session's cwd" # settings.feature:411
    Error: after scenario hook failed: deck client "A" screen unexpectedly contains "(last used) the session's cwd":
+------------------------------------------------------------------------------+
| Create shell session                                                         |
| > Name:                                                                      |
|     the display name; also the source of the session's tmux slug             |
| Working directory:                                                           |
| /tmp/deck-scenario-3939681147/create-session-clear-recent-seed               |
| (last used) the session's cwd; must exist and be a directory; Ctrl+P/Ctrl+N  |
| cycles recent history; right/end completes a shown directory match           |
...
```

The patch was reverted immediately after capturing this log (`git checkout --
internal/tui/settings.go`); `git status` was clean before moving on.

## A gotcha hit and fixed along the way
The first attempt at this edit (made with the generic `edit` tool) silently corrupted the whole
feature file: it turned every embedded raw `\r` byte (used throughout the file to mean "press
Enter", e.g. `sends "<CR>"`) into an actual `\n` line break, which broke godog's parser across
unrelated scenarios (parser errors at lines 39, 51, 101, ... 417) with no connection to the two
lines actually being edited. Fixed by reverting (`git checkout -- features/settings.feature`)
and redoing the two substitutions with a byte-level Python `bytes.replace`, which leaves every
other `\r` in the file untouched (`\r` count before and after: 15).
