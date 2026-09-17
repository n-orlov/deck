# Phase 4 approach 3 — ten-run stability sweep

## Sha

Tail code sha: **`3568bd7971a782fadbf589d79ce5777c0f1b5315`**
("tui: paint the interactive preview branch's own notice and pad rows (task 002)")
— the same sha task 003 recorded (the last commit touching a `*.go` or
`*.feature` file; task 002 is the last code-touching task in this approach).
Confirmed unchanged since task 003's recording:

```
$ git log --format=%H -1 -- '*.go' '*.feature'
3568bd7971a782fadbf589d79ce5777c0f1b5315
$ git rev-parse HEAD
ed5751f80fe850266581e1a3e1193a3babca2890   # task 003's own docs-only commit
$ git diff --stat 3568bd7971a782fadbf589d79ce5777c0f1b5315 HEAD -- '*.go' '*.feature'
   (empty — no drift in any *.go or *.feature file since the tail sha)
```

Tree was clean (`git status --porcelain` empty) before and after this recording.

## What produced this

`ci/stability.sh 10` — ten repetitions of the same unnarrowed whole-tree suite
(`ci/run.sh go test -p=1 -count=1 ./...`, no `-run` filter, no package list,
every package in the module), each from a clean state (`-count=1` disables
Go's test cache; each run's sibling container is `--rm`, so no state leaks
between runs). No narrowing was needed — the full `ci/stability.sh 10` ran to
completion inside the wall-clock budget, so **no features-only fallback was
taken**.

Backgrounded with `nohup timeout 5400 ... &` and polled per the standing
rules (never blocked on inline). Total wall-clock: **run-1.log created
2026-09-17T05:55:31Z, run-10.log / summary completed
2026-09-17T06:57:29Z, script launched ~2026-09-17T05:48:16Z — approximately
1h09m end to end** (~6% under the ~1h12m estimate from the plan-time
measurement).

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

**Headline: 10/10 passed** (agrees with `ci/stability.sh`'s own tally in
`stability-summary.log`: `10/10 passed`, and with `exit 0` from the script
itself). No run's `go test` exit status was non-zero; no `FAIL` line appears
in any of the ten per-run logs (`grep -l FAIL run-*.log` — empty).

## Known-open advisory flakes

The two known-open advisory flakes named in the standing rules —
`TestSigwinchCountDistinguishesTwoFromThree` and `internal/tmux`'s
`TestSendKeysInvalidHexByteIsSilentlyDiscarded` empty-capture case — were
checked for in every one of the ten logs (`grep -n` for both test names
across `run-1.log` … `run-10.log`). **Neither appears in any of the ten
runs** — they did not manifest this sweep, so no row in the table above
carries the "advisory" label.

"Advisory" here describes how this record classifies the two cases, **not**
how they behave in the suite. Both are ordinary Go tests that report through
`t.Fatalf` (`features/sigwinch_count_test.go:78-150`,
`internal/tmux/literal_send_test.go`'s
`TestSendKeysInvalidHexByteIsSilentlyDiscarded` empty-capture assertion), so
either one firing makes `go test` exit non-zero, and `ci/stability.sh` labels
a repetition from that exact exit status (`status=$?` captured immediately
after the un-piped `go test`, never from log text). Had either fired, that
repetition's verdict would therefore read **FAIL**, with the failing test
named in its row and the row additionally labelled advisory — a FAIL labelled
advisory, never a PASS. Neither did fire here, so every row is a genuine
exit-0 PASS.
