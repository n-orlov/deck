# Stability sweep: ten runs at the corrected tail code sha (task 011b)

- **Tail code sha**: `0ba550a5e50bdfc84586d5328a0690af9c9888c4` (`0ba550a`,
  `features: settle the golden frame on a quiet PTY, not a torn read (task
  011b)`) — the same tail code sha named in
  `docs/reports/phase4-cure-final-suite/README.md` and
  `docs/reports/phase4-cure-guards/README.md`. The sweep ran against
  exactly that commit's tree: `git diff --stat 0ba550a HEAD -- '*.go'
  '*.feature'` printed nothing while it ran, and the only commit written
  afterwards in this task is this docs-only record.
- **Command**: `ci/stability.sh 10`, which runs `ci/run.sh go test -p=1
  -count=1 ./...` (every package, no `-run` filter, no package list,
  default godog tag filter `~@real-agents && ~@nightly`) ten times from a
  clean state (`-count=1` disables the test cache; each run gets its own
  `--rm` sibling container), reading each run's real `go test` exit status
  (not a piped `tee` status — see the script's own header comment on the
  mislabelled-PASS defect it exists to avoid).
- **Wall-clock**: 1h12m24s total (`2026-09-17T02:11:37Z` to
  `2026-09-17T03:24:01Z`), each run ~6m–7m — consistent with the standing
  rules' ~75 min estimate and approach 1's `phase4-stability10`
  measurement at `db66965`.
- **Script exit status**: `0` (it exits non-zero if any single run failed).

## Result: 10/10 passed

| Run | Result | Log | `features` package |
| --- | --- | --- | --- |
| 1 | PASS | `run-1.log` | ok (376.464s) |
| 2 | PASS | `run-2.log` | ok (379.114s) |
| 3 | PASS | `run-3.log` | ok (375.776s) |
| 4 | PASS | `run-4.log` | ok (375.368s) |
| 5 | PASS | `run-5.log` | ok (373.184s) |
| 6 | PASS | `run-6.log` | ok (378.546s) |
| 7 | PASS | `run-7.log` | ok (378.844s) |
| 8 | PASS | `run-8.log` | ok (376.042s) |
| 9 | PASS | `run-9.log` | ok (366.762s) |
| 10 | PASS | `run-10.log` | ok (352.266s) |

`summary.log` is the combined log the script itself accumulates (all ten
runs' output plus its own `=== RUN N ===` / `=== RUN N: PASS|FAIL (exit
S) ===` markers and the final `10/10 passed` line), kept alongside the
per-run logs for cross-reference.

## Every failure named: none

`grep -h 'FAIL' run-*.log` over the committed logs returns nothing, and
each of the ten logs contains the same 15 `ok` package lines plus the three
`[no test files]` packages (`internal/notify`, `internal/search`,
`internal/unit`) — 18 packages per run, no `FAIL`, no `SKIP` line emitted
by `go test` itself in any run.

Skips in force are the sweep's standing ones, unchanged from the gate
report: godog's default tag filter `~@real-agents && ~@nightly` (no
`DECK_GODOG_TAGS` opt-in), and the real-binary paths served by the
`cmd/fake-codex`/`cmd/fake-claude`/`cmd/fake-pi` fixtures because no real
agent binary exists in this container.

## What changed against the previous ten-run sweeps

This directory's superseded content (task 011b's first attempt, still in
this file's git history) recorded **7/10** at the earlier tail sha
`e93a790`: runs 2 and 3 red on `TestGoldenMinimumFrame`'s settle check
(`golden_frame_test.go:74`, `"frame kept changing after the fixture
rendered; not settled"`), and run 10 red on
`TestSendKeysInvalidHexByteIsSilentlyDiscarded`
(`internal/tmux/literal_send_test.go:123`) reading an empty `capture-pane`.

- The two golden-frame failures are **fixed at the root, not waited out**.
  Commit `0ba550a` replaced that test's bare-`Frame()` baseline with
  `ScreenDriver.WaitForQuiescence` (300ms quiet window > deck's 250ms
  `previewTick`) plus a bounded re-quiesce-and-compare retry, after the
  torn-baseline cause was reproduced directly at 3 of 12 sub-runs and
  shown to be a half-written frame (missing bottom border and footer),
  never a byte mismatch against the golden fixture. That flake class does
  not appear in any of these ten runs.
- The `internal/tmux` `capture-pane` flake did not recur in these ten runs
  either. It remains disclosed as a real, load-sensitive real-tmux timing
  observation from the earlier sweep — one occurrence, never reproduced in
  three immediate isolated re-runs — carried in the run's findings ledger
  via the `FINDING:` line in commit `b98ce9c`'s body (`git log
  --grep='FINDING:'` picks it up for task 017). Ten clean runs here neither
  erase that observation nor make it a blocker; the earlier red sweep
  stands as history in this file's git history and in that commit.
- The other previously named known-open flake class
  (`features/sigwinch_count_test.go`'s
  `TestSigwinchCountDistinguishesTwoFromThree`) did not occur in this
  sweep either.

## Disposition

Ten from-scratch, unnarrowed runs at the new tail code sha, all green,
script exit `0`. This overwrites the earlier content of this directory per
the standing rules ("013/014/015 are re-run from scratch afterwards, never
re-run under the task that found the red lane"); task 015 itself never ran
a sweep of its own (it was pending behind 013's deferral), so this is its
clean result, produced under task 011b per that task's own criteria. Tasks
016-018 read this file, and the sha named at the top of it, as current
evidence.
