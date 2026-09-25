# Phase 4c: unnarrowed whole-suite gate (task 022) — re-run at the final code sha

This directory's evidence has moved five times now, each time for the same
reason: task 022 gates "the final code sha", and the final code sha kept
moving as later cure work landed on top of it. Prior takes: `eaf2477`
(before cure-01-01..05/-10), `1ad530b` (after those cures, before task
015), `10c5021` (after task 015's list-footer cure), `610be00`
(`cure-01-01-3`'s header-cursor cure). Since `610be00` two more
non-`docs/` commits landed on `main` — `2d282e2` (`cure-01-01-4`: never
let the viewport lose an explicitly navigated header-only-sidebar cursor
across an Esc-cleared filter) and `be7cdbc` (task 015: `ci/run.sh` now
forwards caller-set `DECK_*` vars into the sibling) — so this is that
re-run, replacing the stale `610be00` evidence in place (same three
files, new content).

## Code sha

`be7cdbc996b356a2b17ad31a4b7e095e0a98c98a` — `git rev-parse HEAD` before
dispatching the run below showed this same value, and `git status
--porcelain` was empty both before dispatch and after the run finished,
before this file was written.

`cmd/deck/main_test.go`'s stale `c`-key help assertion (task 022's first
clause) was fixed in `eaf2477` ("c toggle the selected row's manual group"
→ the two-line substring matching task 014's header-cursor rework of
c/left/right, `internal/tui/tui.go:9283`); that commit is an ancestor of
`be7cdbc` (`git merge-base --is-ancestor eaf2477 be7cdbc` — clean) and no
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
- Wall clock: started unix `1790317696`, ended unix `1790318172`, elapsed
  **476 seconds (~7.9 minutes)** — in line with the ~7.5-minute baseline
  measured at `2752c9e`.

### Package accounting (19 packages: 15 `ok` + 4 `[no test files]`)

Verified against `ci/run.sh go list ./...` run at this exact sha (19
lines) — every package that command names is accounted for below, and
nothing else appears in either list.

`ok` (15):
`cmd/deck` (7.350s), `cmd/fake-claude` (0.786s), `cmd/fake-codex`
(0.121s), `cmd/fake-pi` (0.770s), `features` (399.576s), `internal/agent`
(0.007s), `internal/audit` (0.018s), `internal/config` (0.034s),
`internal/hookrecv` (4.397s), `internal/interactive` (15.332s),
`internal/service` (7.018s), `internal/store` (3.204s), `internal/theme`
(0.007s), `internal/tmux` (25.670s), `internal/tui` (7.109s)

`[no test files]` (4):
`internal/notify`, `internal/search`, `internal/unit`,
`internal/tui/sidebarcursor`

No `--- FAIL` or `FAIL` line appears anywhere in `fullsuite.log`; the
process's own exit status is `0`.
