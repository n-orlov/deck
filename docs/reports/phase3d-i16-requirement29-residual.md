# I-16: requirement 29's residual, resolved by I-1's outcome (task 020)

`I-1` (tasks 003/004/005/005a) identified and fixed the single-keystroke drop: **layer
4**, `internal/tui`'s own attention-sort tie-break, a real product defect (see
`docs/reports/phase3d-i1-rootcause.md`). The fix landed in `8d274fc` and `d74077a`. Per
`I-16`, this task re-opens task 113's substance and re-measures.

## 1. All six paths reach and execute the fingerprint assertion, observably

`features/fingerprint_test.go`'s `assertNamedDirectoryMatchesFingerprint` now logs every
execution to `DECK_FINGERPRINT_ASSERT_LOG` (a committed file, no-op unless the env var is
set) with the dirLabel, fpLabel, path and pass/fail outcome. This makes "the assertion ran
and passed" distinguishable from "the scenario reported green without the step ever being
reached" (e.g. an earlier checkpoint timing out first).

Each of the six tagged scenarios below was run **10 consecutive isolated runs**
(`go test -count=1 -run TestFeatures ./features/` per tag, one process per run, matching
task 004/134's methodology), with `DECK_FINGERPRINT_ASSERT_LOG` set. Full logs:
`docs/reports/phase3d-i16-reach-and-execute.log`.

| scenario | passed | load average range across the 10 runs | fingerprint labels reached x10, 0 fails |
|---|---|---|---|
| `@requirement-29-purge` | 10/10 | 1.96 – 2.04 | `fp-purge`/`before-purge` |
| `@requirement-29-archive` | 10/10 | 4.01 – 4.36 | `fp-archive`/`before-archive` |
| `@requirement-29-kill-and-archive` | 10/10 | 4.01 – 4.72 | `fp-kill-and-archive`/`before-kill-and-archive` |
| `@requirement-29-bulk-kill` | 10/10 | 4.72 – 6.07 | `fp-bk-1/cwd`+`fp-bk-2/cwd` |
| `@requirement-29-bulk-delete` | 10/10 | 6.07 – 6.67 | `fp-bd-1/cwd`+`fp-bd-2/cwd` |
| `@requirement-29-batch-undo` | 10/10 | 6.67 – 7.24 | `fp-bu-1/cwd`+`fp-bu-2/cwd` |

For every one of the 60 runs above, the assertion log records exactly one execution per
label per run, all `outcome=pass`, zero `outcome=FAIL` entries (grep in the log). Task 134's
original failure measurement was at load 6.18–7.28; this run's `bulk-kill`, `bulk-delete` and
`batch-undo` figures span or exceed that range (4.72–7.24), so this is not merely "passed at
a quieter host" — the fix holds under comparable load.

## 2. The assertion is not a no-op: two of the six re-run under all four of task 003's
   mutation modes

`features/fingerprint_test.go` gained a permanent, opt-in step (registered in
`registerFingerprintSteps`, a no-op unless a `.feature` file names it):
`the directory "<label>" is corrupted with mode "<mode>" for an I-16 red demonstration`,
implementing exactly task 003's four mutation modes (content — mtime preserved so it isolates
from mode 2; mtime; new-file; removed-file).

Two of the six scenarios were temporarily edited to call this step immediately before their
own fingerprint assertion, run to observe red, then reverted (`git diff -- features/kill_delete_undo.feature`
is empty in this commit — only the reusable step definition in `fingerprint_test.go` is kept).
Full logs: `docs/reports/phase3d-i16-mutation-redemo.log`.

| scenario | mode | result |
|---|---|---|
| `@requirement-29-archive` | content | RED: `directory "fp-archive" no longer matches fingerprint "before-archive": path "state.db" size changed: 19 -> 26` |
| `@requirement-29-archive` | mtime | RED: `directory "fp-archive" no longer matches fingerprint "before-archive": path "state.db" mtime changed` |
| `@requirement-29-bulk-kill` | new-file (on `fp-bk-1/cwd`) | RED: `directory "fp-bk-1/cwd" no longer matches fingerprint "before-bk-one": path "i16-red-demo-new-file.txt" was added` |
| `@requirement-29-bulk-kill` | removed-file (on `fp-bk-2/cwd`) | RED: `directory "fp-bk-2/cwd" no longer matches fingerprint "before-bk-two": path ".hidden" was removed` |

All four of task 003's mutation modes are demonstrated caught, across two of the six real
scenarios (not merely the generic unit test), through the exact same
`assertNamedDirectoryMatchesFingerprint` step every scenario in `features/kill_delete_undo.feature`
uses in production. After capturing the four red runs, the feature file was reverted and the
full six-tag green re-run (§1 above, and re-confirmed once more post-revert:
`go test -run TestFeatures` with all six tags together, `ok`, 6.0s) shows the assertion is
green again with no corruption present.

## Verdict

`I-1` fixed the drop (layer 4, product code). `docs/reports/phase3.md`'s requirement 29 row
is restored to `DONE`, citing this measurement plus task 005/005a's fix.
