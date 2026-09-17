# Phase 4 approach 3 — ten-run stability sweep

## Sha

Tail code sha: **`7bb1f8add502412618ebf4f195b18ffd5536b64a`**
("features: wait for codex's asynchronous first-hook identity adoption before
checking (task cure-03-02)") — the same sha task 003 recorded in
`docs/reports/phase4-a3-final-suite/README.md`, and the last commit touching a
`*.go` or `*.feature` file. Confirmed at launch:

```
$ git log --format=%H -1 -- '*.go' '*.feature'
7bb1f8add502412618ebf4f195b18ffd5536b64a
$ git rev-parse HEAD
abd963f7c58ea58f8c39119652cc8d2a8711f500   # task 003's own docs-only commit
$ git diff --stat 7bb1f8add502412618ebf4f195b18ffd5536b64a HEAD -- '*.go' '*.feature'
   (empty — no drift in any *.go or *.feature file since the tail sha)
```

This recording **supersedes** the ten-run record previously written here at sha
`3568bd7` (commits `5f1fb5c` / `deca67c`): the two cure tasks cure-03-01
(settings-footer paint, `2a04e5a`) and cure-03-02 (real-Codex first-hook wait,
`7bb1f8a`) landed `*.go` / `*.feature` changes on top of that sha, so a sweep of
that tree is a measurement of a superseded tree. Every log and every table row
below is a fresh measurement of the post-cure tree, not an amendment of the old
one.

Tree was clean (`git status --porcelain` empty) before this recording.

## What produced this

`ci/stability.sh 10` — ten repetitions of the same unnarrowed whole-tree suite
(`ci/run.sh go test -p=1 -count=1 ./...`, no `-run` filter, no package list,
every package in the module), each from a clean state (`-count=1` disables Go's
test cache; each run's sibling container is `--rm`, so no state leaks between
runs). No narrowing was needed — the full `ci/stability.sh 10` ran to completion
inside the wall-clock budget, so **no features-only fallback was taken**.

Every one of the ten per-run logs lists all **18** packages `go list ./...`
returns for this module (15 with tests plus `internal/notify`,
`internal/search`, `internal/unit`, each reported `[no test files]`) — the sweep
was unnarrowed in every repetition, and the only "skips" in it are those three
package directories that contain no test files at all.

Backgrounded with `nohup timeout 5400 ci/stability.sh 10 &` and polled per the
standing rules (never blocked on inline). **Repeat count: 10.** Total
wall-clock: **launched 2026-09-17T10:26:28Z, run-1.log completed
2026-09-17T10:33:40Z, run-10.log and the summary completed
2026-09-17T11:38:09Z — 1h11m41s (~1h12m) end to end**, matching the plan-time
estimate of ~1h12m.

Per-run logs (`run-1.log` … `run-10.log`) and the combined `ci/stability.sh`
summary (`stability-summary.log`, its own PASS/FAIL line per run plus the
script's own final `10/10 passed` tally) are committed alongside this README.

## Result table

| # | log path | verdict |
|---|----------|---------|
| 1 | run-1.log | PASS |
| 2 | run-2.log | PASS |
| 3 | run-3.log | PASS |
| 4 | run-4.log | PASS |
| 5 | run-5.log | PASS |
| 6 | run-6.log | PASS |
| 7 | run-7.log | PASS |
| 8 | run-8.log | PASS |
| 9 | run-9.log | PASS |
| 10 | run-10.log | PASS |

**Headline: 10/10 passed** (agrees with the ten rows above, with
`ci/stability.sh`'s own tally in `stability-summary.log`: `10/10 passed`, and
with the script's own `exit 0`). No run's `go test` exit status was non-zero; no
`FAIL` line appears in any of the ten per-run logs (`grep -rn FAIL run-*.log` —
empty). No row is a fail, so no row names a failing test.

## Known-open advisory flakes

The two known-open advisory flakes named in the standing rules —
`TestSigwinchCountDistinguishesTwoFromThree`
(`features/sigwinch_count_test.go:24`) and `internal/tmux`'s
`TestSendKeysInvalidHexByteIsSilentlyDiscarded` empty-capture case
(`internal/tmux/literal_send_test.go:123`) — were searched for by name in every
one of the ten logs. **Neither appears in any of the ten runs**: they did not
manifest in this sweep, so no row in the table above carries the "advisory"
label.

"Advisory" describes how this record would *classify* either case, **not** how
it behaves in the suite. Both are ordinary Go tests that report through
`t.Fatalf`, and `ci/stability.sh` labels a repetition from the actual exit
status of `go test` (`status=$?` captured immediately after the un-piped
command, never from log text), so either one firing makes that repetition exit
non-zero. Had either fired, that repetition's verdict would therefore read
**FAIL**, with the failing test named in its row and the row additionally
labelled advisory — a FAIL labelled advisory, never a PASS. Neither fired here,
so all ten rows are genuine exit-0 PASS verdicts.
