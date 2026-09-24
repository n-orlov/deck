# Phase 4c: unnarrowed whole-suite gate (task 022) — re-run at the final code sha

This directory's evidence has moved four times now, each time for the same
reason: task 022 gates "the final code sha", and the final code sha kept
moving as later cure work landed on top of it. It was taken at `eaf2477`
(before cure-01-01..05/-10), then `1ad530b` (after those cures, before task
015), then `10c5021` (after task 015's list-footer cure). `cure-01-01-3`'s
`610be00` (tui: never let a background arrival steal an explicitly
navigated header-only-sidebar cursor, R136/R137, SPEC §11, #31/#32) is the
latest non-`docs/` commit on `main` since then, so this is that re-run,
replacing the stale `10c5021` evidence in place (same three files, new
content).

## Code sha

`610be0093db4c10c920b28e40b73efad4efee59f` — `git rev-parse HEAD` before
dispatching the run below showed this same value, and `git status
--porcelain` was empty both before dispatch and after the run finished,
before this file was written.

`cmd/deck/main_test.go`'s stale `c`-key help assertion (task 022's first
clause) was fixed in `eaf2477` ("c toggle the selected row's manual group"
→ the two-line substring matching task 014's header-cursor rework of
c/left/right, `internal/tui/tui.go:9283`); that commit is an ancestor of
`610be00` (`git merge-base --is-ancestor eaf2477 610be00` — clean) and no
further code change was needed: `cmd/deck` is `ok` in the run below,
confirming `TestDeckBinaryEmptyHelpAndQuitThroughPTY` still passes against
the current `helpText`.

## The whole-suite run

Command (unnarrowed, no `-run` filter, no tag override, no package list):

```
ci/run.sh go test -p=1 -count=1 ./...
```

Run in the CI sibling container (`deck-ci:local`, warm `deck-go-cache`
volume), in the background, with the test process's own exit status
captured to a file rather than inferred from a pipe or from log prose
(`echo $? > fullsuite.exit` immediately after the `go test` command, by
the same shell that ran it).

- Log: `fullsuite.log` (this directory)
- Exit status: `fullsuite.exit` (this directory) — contents: `0`
- Wall clock: started unix `1790266296`, ended unix `1790266772`, elapsed
  **476 seconds (~7.9 minutes)** — in line with the ~7.5-minute baseline
  measured at `2752c9e`.

### Package accounting (19 packages: 15 `ok` + 4 `[no test files]`)

Verified against `ci/run.sh go list ./...` run at this exact sha (19
lines) — every package that command names is accounted for below, and
nothing else appears in either list.

`ok` (15):
`cmd/deck` (7.274s), `cmd/fake-claude` (0.783s), `cmd/fake-codex`
(0.119s), `cmd/fake-pi` (0.771s), `features` (399.459s), `internal/agent`
(0.007s), `internal/audit` (0.018s), `internal/config` (0.035s),
`internal/hookrecv` (4.465s), `internal/interactive` (15.255s),
`internal/service` (7.114s), `internal/store` (3.175s), `internal/theme`
(0.005s), `internal/tmux` (25.431s), `internal/tui` (7.284s)

`[no test files]` (4):
`internal/notify`, `internal/search`, `internal/unit`,
`internal/tui/sidebarcursor`

No `--- FAIL` or `FAIL` line appears anywhere in `fullsuite.log`; the
process's own exit status is `0`.
