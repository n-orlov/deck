# Phase 4c: unnarrowed whole-suite gate (task 022) — re-run at the final code sha

This directory's evidence has moved twice already, each time for the same
reason: task 022 gates "the final code sha", and the final code sha kept
moving as later cure work landed on top of it. It was first taken at
`eaf2477` (13:30Z, before cure-01-01..05/-10), then re-taken at `1ad530b`
(after those cures but before task 015's list-footer cure). Task 015's
`10c5021` (header-cursor footer legend, #32) is the latest non-`docs/`
commit on `main`, so this is that re-run, replacing the stale `1ad530b`
evidence in place (same three files, new content).

## Code sha

`10c5021f1f70549d54f6717a697c69ed6fe2b797` — `git rev-parse HEAD` before
dispatching the run below showed this same value, and `git status
--porcelain` was empty both before dispatch and after the run finished,
before this file was written. No code change was needed to reach this
gate: `eaf2477`'s cmd/deck stale-assertion fix (task 022's first clause)
already landed earlier and is an ancestor of `10c5021`
(`git merge-base --is-ancestor eaf2477 10c5021` — clean).

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
- Wall clock: started `1790256274` (unix time), ended `1790256755`, elapsed
  **481 seconds (~8.0 minutes)** — in line with the ~7.5-minute baseline
  measured at `2752c9e` (18 packages, no grouping/header-cursor work yet);
  the 19th package (`internal/tui/sidebarcursor`, no test files) and the
  behavioural cures' own added test files cost a little over half a
  minute more, not a regression in per-test cost.

### Package accounting (19 packages: 15 `ok` + 4 `[no test files]`)

Verified against `ci/run.sh go list ./...` run at this exact sha (19 lines)
— every package that command names is accounted for below, and nothing
else appears in either list.

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
