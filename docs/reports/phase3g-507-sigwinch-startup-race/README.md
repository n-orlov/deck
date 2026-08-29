# Task 507 — the sigwinch-count startup race (`sigwinch_count_test.go:89`)

## Background

Every prior stability measurement on this tree (505, 512 rounds 1 and 2, and
507's own first round) hit the same single flake, always with the identical
message, always at the same line:

```
sigwinch_count_test.go:89: sigwinch count after 1st resize = 0, want exactly 1 before sending the 2nd
```

Cumulative before this fix: 5 hits across 40 stability-run repetitions (505
2/20, 512 2/10, 507 round 1 1/10) — a low but real rate, never confirmed
worse or better, and the reason 507 could not close finding 1 by re-rolling
the dice alone. This report roots the cause out and fixes it.

## Root cause

`TestSigwinchCountDistinguishesTwoFromThree` (`features/sigwinch_count_test.go`)
starts the `fake-claude` fixture and, before sending its first real
`TIOCSWINSZ` resize, "confirms" the SIGWINCH counter starts at 0:

```go
if got, err := waitForSigwinchCount(countPath, 0); err != nil { ... }
```

`readSigwinchCount` (`features/fake_agent_size_test.go`) treats a
**not-yet-created** counter file as count 0, by design (it mirrors
`input_count_test.go`'s `readInputCount`, which must tolerate a counter that
never fires at all). That design is correct for the counter file itself, but
it means this particular "before any resize" check passed **trivially, the
instant it was polled** — including the very first poll, taken essentially
the moment the fixture's process was `exec`'d, long before the fixture's own
`main()` had run far enough to call `startSizeRecorder`
(`cmd/fake-claude/main.go`), which is what installs the `SIGWINCH` handler
via `signal.Notify`. The check gave the test **no synchronisation
guarantee at all** that the handler was installed before the test raised the
first SIGWINCH.

A standard Unix signal has no queue: if `TIOCSWINSZ` fires while the
process's disposition for `SIGWINCH` is still the default (ignore, because
`signal.Notify` has not run yet), the kernel delivers it, the default
handler discards it, and it is gone forever — no later `SIGWINCH` recovers
it, because each subsequent resize is a distinct signal instance. Under
enough scheduling contention (the fixture's `exec`, pty allocation, and
Go runtime start-up racing the harness's own resize call) that startup
window is occasionally wide enough for exactly this to happen to the
*first* resize, leaving the counter stuck at 0 and failing this test's
"count after 1st resize" assertion — never any later resize, because by
then the handler is long since installed.

## Fix

`cmd/fake-claude/main.go`'s `startSizeRecorder` already writes the
fixture's *initial* terminal size to `fake-claude-sizes.log`
(`recordSize(path)`) as its very first action, on the line immediately
before it calls `signal.Notify(signals, syscall.SIGWINCH)` — no intervening
syscall separates the two. That write is therefore a tight, purely
observable proxy for "the handler is now installed": product code is
untouched.

`TestSigwinchCountDistinguishesTwoFromThree` (test-only change) now waits for
that initial `80x24` line to appear in `fake-claude-sizes.log`
(`waitForRecordedSizes`, already used by `features/fake_agent_size_test.go`
and `harness.feature`'s `@requirement-4-fake-agent-sizes` scenario for the
same fixture) **before** sending the first resize, closing the startup
window the same way `157bb52`/task 303 already closed the inter-resize
window later in the same test (waiting on the fixture's own observed count
rather than a fixed sleep). No assertion is weakened: the test still expects
exactly 0/1/2/3 sigwinch counts at each step, still exact-equality, still the
same three resizes and both off-by-one directions.

## Evidence

All three runs below used the identical CPU-contention harness: 40
`docker run --rm --label ralphd.run=$RALPHD_RUN_ID --label
ralphd.role=sibling deck-ci:local sh -c 'timeout 90 sh -c "yes > /dev/null"'`
containers launched in the background (labelled siblings, self-terminating
after 90s, never targeted by any run/kill-by-pattern command), then, after a
2s settle:

```
timeout 100 ci/run.sh go test ./features/ -run TestSigwinchCountDistinguishesTwoFromThree -count=60 -v
```
exit status captured with `echo "exit=$?"` in the same shell call as the
test command, no pipe.

### Red-before (unmodified pre-fix tree, HEAD `ad251d9`)

`red-before.log` — **exit=1**, `--- FAIL` x4 of 60 (`grep -c '^--- FAIL'
red-before.log` = 4), every failure the identical message at the same line:

```
$ grep -n 'FAIL:\|sigwinch_count_test.go' red-before.log
6:    sigwinch_count_test.go:89: sigwinch count after 1st resize = 0, want exactly 1 before sending the 2nd
7:--- FAIL: TestSigwinchCountDistinguishesTwoFromThree (2.84s)
15:    sigwinch_count_test.go:89: sigwinch count after 1st resize = 0, want exactly 1 before sending the 2nd
16:--- FAIL: TestSigwinchCountDistinguishesTwoFromThree (2.81s)
24:    sigwinch_count_test.go:89: sigwinch count after 1st resize = 0, want exactly 1 before sending the 2nd
25:--- FAIL: TestSigwinchCountDistinguishesTwoFromThree (3.13s)
97:    sigwinch_count_test.go:89: sigwinch count after 1st resize = 0, want exactly 1 before sending the 2nd
98:--- FAIL: TestSigwinchCountDistinguishesTwoFromThree (3.20s)
```

4/60 (≈6.7%) under this artificial contention is consistent with the ~5/40
(≈12.5%) real stability-run rate landing on the same single test each time
— the same race, reproduced from the unmodified route, no hand-edited state.

### Green-after (fixed tree, same contention harness, two independent rounds)

`green-after-round1.log` — **exit=0**, `=== RUN` x60, `--- PASS` x60,
`--- FAIL` x0 (`grep -c` of each confirms the counts).

`green-after-round2.log` — **exit=0**, `=== RUN` x60, `--- PASS` x60,
`--- FAIL` x0.

60/60 twice in a row under the identical load that produced 4/60 failures
before the fix (120/120 total, 0 failures).

### No regression in the shared fixture path

`ci/run.sh env DECK_GODOG_PATHS=harness.feature go test ./features/ -run
TestFeatures -count=1 -v`, exit=0, 24.4s — includes
`@requirement-4-fake-agent-sizes` (the scenario that already exercises
`waitForRecordedSizes` against this same fixture) and
`@requirement-5-preview-fixtures`, both passing.

## Scope check

- Product code (`cmd/fake-claude/main.go`) is unchanged — `git diff` for
  this commit touches only `features/sigwinch_count_test.go`.
- `features/godog_test.go`'s `defaultTags` is unchanged; no `t.Skip` added;
  no scenario deleted, retagged, or weakened.
- `launch.CWD`/session cwd handling is untouched — this fix has nothing to
  do with sessions or launch at all.
- This is exactly the "synchronise on observable state, never weaken"
  pattern already licensed and used by tasks 501/502/503/303, applied to a
  fourth, previously-unclosed race in the same class.

## Next step

With this race closed, task 507 re-launches `ci/stability.sh 10` (the
mandated launch-and-poll shape) on top of this fix, aiming for the first
genuine 10/10 on this lineage. See the sibling directory
`docs/reports/phase3g-507-stability10/` for that round, and this directory's
own history for the fix that made it possible.
