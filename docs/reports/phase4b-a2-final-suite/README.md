# Phase 4b, approach 2 — task 007: unnarrowed whole-suite gate sweep

## What ran

`ci/run.sh go test -p=1 -count=1 ./...` (no `-run` filter, no package list), inside the
CI sibling container, bounded with `timeout 3000`, launched with `nohup ... &` and polled
rather than blocked on. Exit status taken from the run's own recorded outcome, never from a
pipe — see `gate-exit-status.txt`.

- Raw log: `gate-run.log`
- Exit status file: `gate-exit-status.txt` → `exit=0 elapsed=445s`

## Tree identity

Taken at commit `d0b3100ac38c64720ca5da87b353d3d2fc9d1d29` (HEAD == origin/main at the time
of the run; `git status --porcelain` showed only this report directory as untracked, no
tracked file was dirty). Four tree-object hashes, matched against the ones task 005 pinned
for this same code revision:

| path       | this run (`d0b3100:<path>`)                 | task 005 pin                                |
|------------|----------------------------------------------|------------------------------------------------|
| `internal` | `15506734d4989e111e871a419ebf46c94a3b59a3`   | `15506734d4989e111e871a419ebf46c94a3b59a3`   |
| `cmd`      | `27ff2eba72ef6a63a6cf49285c4ddc6660b7fb0d`   | `27ff2eba72ef6a63a6cf49285c4ddc6660b7fb0d`   |
| `features` | `b5dbe2f565eb96b2654f8d1d64fcc51eb711f60d`   | `b5dbe2f565eb96b2654f8d1d64fcc51eb711f60d`   |
| `ci`       | `0a183631a2beea070ab0f7d8fa027aecf423e7b0`   | `0a183631a2beea070ab0f7d8fa027aecf423e7b0`   |

All four equal. No code-touching commit has landed since task 005 pinned the revision (the
freeze from task 004 onward holds), so this sweep is exercising the same tree.

## Timing

This run: **445s** wall-clock (`exit=0 elapsed=445s`, measured start-to-finish around the
`timeout 3000 ci/run.sh ...` invocation). Reference measured at `5128783` during planning:
**438s** (`/run/ralphd/artifacts/planning-baseline-fullsuite.log`, also exit 0). The two are
within normal sibling-container run-to-run variance (~1.6% higher here); no regression.

## Package accounting

`go list ./...` at this tree prints 18 packages. The gate log accounts for all 18 result
lines: 15 `ok` and 3 `[no test files]` (`internal/notify`, `internal/search`,
`internal/unit` — all Phase 5/6/7 placeholders, out of scope per the standing rules). No
`FAIL` line, no skipped package.

```
$ grep -cE '^(ok|\?)' gate-run.log
18
$ grep -c '^ok' gate-run.log
15
$ grep -c 'no test files' gate-run.log
3
$ grep -c 'FAIL' gate-run.log
0
```

## Verdict

Unnarrowed whole-suite gate is green at the pinned code revision. No red lane found; no new
code-touching task is warranted (freeze stands).
