# Phase 4c: unnarrowed whole-suite gate (task 022) — re-run at the final code sha

This directory's evidence was originally taken at `eaf2477` (13:30Z), BEFORE
the cure-01-01..05/-10 code commits landed on top of it. Per ruling 001 +
steering 003/004, task 022 is "the unnarrowed whole-suite gate re-run at the
final code sha" — the fc33851 record was stale the moment cure-01-01 landed.
This README and the two files beside it (`fullsuite.log`, `fullsuite.exit`)
are that re-run, replacing the stale evidence in place (same three files,
new content).

## Code sha

`1ad530b3c63335c6a06ca07950640dafe410d75b` — `git rev-parse HEAD` immediately
before dispatching the run below showed this same value, and `git status
--porcelain` was empty (checked again immediately after the run finished,
before this file was written).

This sha is itself two commits past the record's namesake `340b4d6`, both
landed inside this same task 022 sweep, per the standing rule that a lane
found red is cured inside the task holding it, not left for a future task:

- `340b4d6` "tui: follow selection onto a session that arrives under an
  empty header" — fixed a genuine regression cure-01-05 (`4e30475`)
  introduced: a concurrent client that starts before any session exists gets
  its cursor promoted onto the default group's HEADER by cure-01-05's own
  header-only-load fix, and that header selection never budged even once a
  session appeared in its own, still-uncollapsed bucket — every row-only
  action (kill included) went inert against a header with no exemption
  (cure-01-01, F1). First unnarrowed run at 2bb61a8 (before this fix) failed
  three tests this way: `cmd/deck`'s
  `TestDeckBinaryRefreshesAllConcurrentClients` ("timed out waiting for
  \"resumable\" within 1.25s"), `features`' `TestFeatures/create_and_kill_...`
  scenario (same "resumable" wait, via godog), and `features`'
  `TestGoldenMinimumFrame` (a golden fixture stale against cure-01-02's
  header-selection-cue gutter, regenerated with `UPDATE_GOLDEN=1`).
- `1ad530b` "tui: follow selection off an empty header through live filter
  edits too" — the SAME defect class reached through a second call site
  (`updateFilter`'s live-narrowing keystrokes) that `340b4d6` did not cover:
  re-running the whole suite again at `340b4d6` still failed
  `features/filter.feature`'s "dd found through / tombstones an archived
  row" scenario ("timed out waiting for frame \"again\"", ~46s), for the
  identical reason — archiving the sidebar's only session leaves the
  header selected, and typing a filter query that surfaces that one
  archived row never walked the cursor onto it.

Both commits' own fail-before evidence (a new, targeted `internal/tui` unit
test that failed against the pre-fix tree, then passed after) is in their
own commit messages and the two probe logs below:

- `/run/ralphd/artifacts/task022-cure0105-newtest-before.log`
- `/run/ralphd/artifacts/task022-filter-newtest-before.log`

## The whole-suite run

Command (unnarrowed, no `-run` filter, no tag override, no package list):

```
ci/run.sh go test -p=1 -count=1 ./...
```

Run in the CI sibling container (`deck-ci:local`, warm `deck-go-cache`
volume), in the background, with the test process's own exit status
captured to a file rather than inferred from a pipe or from log prose.

- Log: `fullsuite.log` (this directory)
- Exit status (the `go test` process's own, written by the shell that ran
  it, `echo $? > fullsuite.exit` immediately after the `go test` command):
  `fullsuite.exit` (this directory) — contents: `0`
- Wall clock: started `1790245949` (unix time), ended `1790246442`, elapsed
  **493 seconds (~8.2 minutes)** — in line with the ~7.5-minute baseline
  measured at `2752c9e` (18 packages, no grouping/header-cursor work yet);
  the 19th package (`internal/tui/sidebarcursor`, no test files) and the
  five behavioural cures' own added test files cost a little over half a
  minute more, not a regression in per-test cost.

### Package accounting (19 packages: 15 `ok` + 4 `[no test files]`)

Verified against `ci/run.sh go list ./...` run at this exact sha (output
above, 19 lines) — every package that command names is accounted for below,
and nothing else appears in either list.

`ok` (15):
`cmd/deck`, `cmd/fake-claude`, `cmd/fake-codex`, `cmd/fake-pi`, `features`,
`internal/agent`, `internal/audit`, `internal/config`, `internal/hookrecv`,
`internal/interactive`, `internal/service`, `internal/store`,
`internal/theme`, `internal/tmux`, `internal/tui`

`[no test files]` (4):
`internal/notify`, `internal/search`, `internal/unit`,
`internal/tui/sidebarcursor`

15 + 4 = 19, matching `go list ./...`'s own package count at this sha. No
package reported `FAIL`; `fullsuite.exit` holds `0`.
