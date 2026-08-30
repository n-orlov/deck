# Task 005 — the two cwd-ghost step helpers resolve the shipped `hint` token

`features/create_cwd_ghost_test.go`'s `clientCWDFieldShowsNoGhostText` and
`clientCWDFieldShowsGhostText` (R95's other half; the feature-file half landed at `d578c03`,
task 004) called `resolveScenarioTokenHex(ctx, "dimmed")`. Both now call
`resolveScenarioTokenHex(ctx, "hint")`, matching the three feature-file assertions task 004
already re-pointed:

```
$ grep -c 'resolveScenarioTokenHex(ctx, "dimmed")' features/create_cwd_ghost_test.go
0
$ grep -c 'resolveScenarioTokenHex(ctx, "hint")' features/create_cwd_ghost_test.go
2
```

## The token swap alone turned scenario 3 red, not just scenario 1

Re-pointing the two helpers at `hint` fixed the intended target (the unique-match positive
control, `clientCWDFieldShowsGhostText`, and scenario `:14`'s ghost check) but immediately
broke the *negative* proof, `clientCWDFieldShowsNoGhostText`, on the ambiguous-match scenario
("several matches with no further common prefix ghost nothing and show a match count"):

```
step error: client "A" has a dimmed-token cell ">" at row 4 column 2 (inside the cwd field's
own rows 4-4), want no ghost text there
```

The offending cell is the field's own focus marker/label ("`> Working directory: `"), not a
ghost. SPEC.md:1355 puts a field's label in the `hint` token unconditionally (see
`internal/tui/tui.go`'s `styledCreateBody` doc comment: "a field's label in `hint` and its
value in `text`... the focused field in `selection`"), and R95 moved the ghost suffix onto
that same `hint` token. Once both the label and the ghost share one token, a whole-row scan
for "any `hint` cell" can no longer tell them apart — the field's own label always contains a
`hint` cell, ghost or not, so `clientCWDFieldShowsNoGhostText` could never pass again as long
as the scan covers the label's own columns.

**Fix** (in the shared helper, not the step regex/scenario/assertion): a new
`cwdFieldLabelEndCol` locates the literal `"Working directory: "` text on the field's label
row and returns the column right after it; `cwdFieldDimmedCell` now starts its scan on that
row from that column instead of column 0, on every other (wrapped, label-free) row within
bounds it still scans from column 0 as before. This does not touch any step regex, scenario
or assertion message — both step functions' error strings are byte-identical to before, and
`create_cwd_ghost.feature` is untouched by this task. No file under `internal/` is touched and
no theme palette file changes:

```
$ git show --stat HEAD | grep -c '^ internal/'
0
```

## Green proof

Command (per the standing rules' diagnostics-only entry):

```
ci/run.sh env DECK_GODOG_PATHS=create_cwd_ghost.feature go test ./features/ -count=1
```

| Log | Exit | Contents |
|---|---|---|
| `create_cwd_ghost.attempt1.log` | 0 | `ok  github.com/n-orlov/deck/features  20.674s` — per standing-rules finding F34, the non-verbose form never prints the Gherkin tally on a pass, only "ok" |
| `create_cwd_ghost.attempt1-v.log` | 0 | same command with `-v` appended, run so the tally itself could be quoted below (F34's whole point: without `-v` there is nothing to quote) |

`create_cwd_ghost.attempt1.exitstatus` and `create_cwd_ghost.attempt1-v.exitstatus` both hold
`0`.

The `-v` log's tally, quoted verbatim:

```
6 scenarios (6 passed)
84 steps (84 passed)
```

(`create_cwd_ghost.attempt1-v.log` lines 151-152.)

## Scope

This task only runs and records `create_cwd_ghost.feature` individually; it does not re-run
the whole suite (`ci/run.sh go test -p=1 -count=1 ./...`) — that is a separate, not-yet-spent
measurement per the standing rules ("at most ONE whole-suite run per iteration").

HEAD at the time of this report equals `origin/main`.
