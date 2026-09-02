# Task 303 — verbose companion sweep at the final code sha

Verbose companion to task 302's `phase3i-302-fullsuite` sweep, run at the same frozen final
code sha, with `-v` added and nothing else changed, so the Gherkin scenario/step tally (phase 3g
finding F34) is visible in the log.

## Command run

```
timeout 2400 ci/run.sh go test -p=1 -count=1 -v ./...
```

Launched backgrounded exactly as the standing rules require:

```
nohup sh -c 'timeout 2400 ci/run.sh go test -p=1 -count=1 -v ./... > /tmp/sweep-303.log 2>&1; echo $? > /tmp/sweep-303.log.exitstatus' >/dev/null 2>&1 &
```

No `-run`, no `DECK_GODOG_PATHS`, and `features/godog_test.go`'s `defaultTags` was not touched
(verified below).

## Final code sha (unchanged)

```
git log -1 --format=%H -- '*.go' '*.feature'
```
→ `a559e7c61a00ab5fa31c2d98eaf5ce787744e4dc` — unchanged; approach 3 lands zero
`*.go`/`*.feature` changes, so this is the same sha R98-R102 were reviewed at and the same sha
task 302's non-verbose sweep ran at.
`git log --oneline a559e7c..HEAD -- '*.go' '*.feature'` printed nothing at launch time and
again after the sweep.

## Verbatim polling record

Every completion check was issued as exactly this one command, differing only in the poll it
belongs to — the `sleep 60` prefix is part of the check itself, so no check in this iteration
was preceded by any other sleep duration (no `sleep 180`, no `sleep 30`, no bare check):

```
sleep 60; date -u +%Y-%m-%dT%H:%M:%SZ; wc -l < /tmp/sweep-303.log; cat /tmp/sweep-303.log.exitstatus 2>/dev/null || echo "not done"
```

Launch timestamp, printed by `date -u +%Y-%m-%dT%H:%M:%SZ` in the launching command:

```
2026-09-02T18:06:37Z
```

Then, verbatim output of each poll (timestamp / whole-file `wc -l` of the log so far /
exit-status file):

```
Poll 1: 2026-09-02T18:07:39Z
        943
        not done
Poll 2: 2026-09-02T18:08:41Z
        2152
        not done
Poll 3: 2026-09-02T18:09:44Z
        3271
        not done
Poll 4: 2026-09-02T18:10:46Z
        4189
        not done
Poll 5: 2026-09-02T18:11:48Z
        5389
        not done
Poll 6: 2026-09-02T18:12:51Z
        7775
        not done
Poll 7: 2026-09-02T18:13:53Z
        9642
        0
```

Reading of that record: the numbers above are whole-file line counts from `wc -l`, not a `tail`
window, growing monotonically as the verbose `-v` output streamed in. Poll 7 shows the log at
its final 9642 lines and `sweep.log.exitstatus` containing `0`, i.e. the sweep observed
complete.

- Wall time: launch 18:06:37Z → observed complete at the poll ending 18:13:53Z, ≈7m16s of
  polling granularity, in line with the ~9-10 min previously estimated for the verbose
  companion (verbose output adds line-emission overhead but not meaningfully more wall time
  than the non-verbose sweep at this sha).
- Seven polls, all `sleep 60`.

## Byte-identity of the published log

The sweep wrote `/tmp/sweep-303.log`; it was copied here unmodified and compared:

```
cmp /tmp/sweep-303.log docs/reports/phase3i-303-fullsuite-verbose/verbose.log   # no output → identical
wc -c /tmp/sweep-303.log docs/reports/phase3i-303-fullsuite-verbose/verbose.log
 1275809 /tmp/sweep-303.log
 1275809 docs/reports/phase3i-303-fullsuite-verbose/verbose.log
```

`verbose.log.exitstatus` was copied from `/tmp/sweep-303.log.exitstatus` the same way.

## Exit status

Captured from the `.exitstatus` file written by the backgrounded shell, never from `$?` after a
`tee`:

```
0
```
(see `verbose.log.exitstatus` in this directory)

## Tag exclusion

`features/godog_test.go`'s `defaultTags` (`grep -n defaultTags features/godog_test.go`, line
16):

```
const defaultTags = "~@real-agents && ~@nightly"
```

This is the only tag exclusion in effect for this sweep, and it is unchanged from the frozen
final code sha. No other skipped marker, `-run` narrowing, `DECK_GODOG_PATHS` override or
module-level skip was used.

## Package count

`ci/run.sh go list ./...` at this sha exits 0 and lists **17** packages; `verbose.log` reports
exactly **17** package result lines (`grep -cE '^(ok|\?|FAIL)[[:space:]]' verbose.log` → 17),
one per package, and `grep -c '^FAIL' verbose.log` → 0.

```
28:ok  	github.com/n-orlov/deck/cmd/deck	7.717s
70:ok  	github.com/n-orlov/deck/cmd/fake-claude	0.785s
108:ok  	github.com/n-orlov/deck/cmd/fake-pi	0.771s
6103:ok  	github.com/n-orlov/deck/features	329.696s
6209:ok  	github.com/n-orlov/deck/internal/agent	0.005s
6231:ok  	github.com/n-orlov/deck/internal/audit	0.018s
6387:ok  	github.com/n-orlov/deck/internal/config	0.027s
6815:ok  	github.com/n-orlov/deck/internal/hookrecv	4.285s
6958:ok  	github.com/n-orlov/deck/internal/interactive	11.304s
6959:?   	github.com/n-orlov/deck/internal/notify	[no test files]
6960:?   	github.com/n-orlov/deck/internal/search	[no test files]
7134:ok  	github.com/n-orlov/deck/internal/service	4.323s
7320:ok  	github.com/n-orlov/deck/internal/store	2.582s
7569:ok  	github.com/n-orlov/deck/internal/theme	0.006s
7926:ok  	github.com/n-orlov/deck/internal/tmux	19.647s
9641:ok  	github.com/n-orlov/deck/internal/tui	3.503s
9642:?   	github.com/n-orlov/deck/internal/unit	[no test files]
```

All 17 lines report `ok` (14) or `[no test files]` (3, expected — `internal/notify`,
`internal/search`, `internal/unit` carry no `_test.go` files at this sha); none report `FAIL`.

## The Gherkin scenario/step tally (phase 3g finding F34)

`grep -n "scenarios (" verbose.log` and `grep -n "steps (" verbose.log` each return three
matches: one pair from the real `features` package run (the deliverable tally) and two pairs
from the `TestGodogRejectsUndefinedAndFailedSteps` fixture inside `internal/tui`, which
deliberately runs a failing step and an undefined step to prove godog's own step-rejection
behaviour — not part of the real suite's Gherkin coverage.

godog colours its tally lines, so each of those log lines carries raw ANSI SGR escape bytes
(`ESC` = `0x1b`) around the count. **The first fenced block in each subsection below is the
`grep -n` output byte-for-byte, escape bytes included** — copied straight out of `verbose.log`,
not retyped — so a byte-exact search for any quoted `NNNN:...` line finds it in both files. The
second fenced block repeats the same two lines through `cat -v`, which renders each `ESC` byte as
the two visible characters `^[`, for readers whose viewer swallows control bytes. To reproduce
either form from this directory:

```
grep -n 'scenarios (\|steps (' verbose.log            # the raw bytes (first block)
grep -n 'scenarios (\|steps (' verbose.log | cat -v   # the ^[ rendition (second block)
```

### The deliverable tally (from the `features` package's real run)

```
5642:319 scenarios ([32m319 passed[0m)
5643:3682 steps ([32m3682 passed[0m)
```

The same two lines with the ESC bytes made visible (`grep -n ... | cat -v`):

```
5642:319 scenarios (^[[32m319 passed^[[0m)
5643:3682 steps (^[[32m3682 passed^[[0m)
```

**319 scenarios (319 passed)** / **3682 steps (3682 passed)** — every scenario and every step in
the real suite passed; none pending, skipped, undefined or failed.

### Deliberately-excluded fixture pairs (from `TestGodogRejectsUndefinedAndFailedSteps`)

This fixture (`internal/tui`, see `=== RUN   TestGodogRejectsUndefinedAndFailedSteps` at line
5965) runs two throwaway single-scenario fixtures purely to assert that godog rejects a failing
step and an undefined step — both sub-fixtures pass as *Go tests* (the assertion is that godog
reports the failure/undefined status correctly), but their own internal godog tallies are not
part of the real suite's Gherkin coverage and are excluded from the 319/3682 figures above:

```
5977:1 scenarios ([31m1 failed[0m)
5978:1 steps ([31m1 failed[0m)
```

The same two lines with the ESC bytes made visible (`grep -n ... | cat -v`):

```
5977:1 scenarios (^[[31m1 failed^[[0m)
5978:1 steps (^[[31m1 failed^[[0m)
```
— from the `/failed` sub-fixture (line 5966 `=== RUN   TestGodogRejectsUndefinedAndFailedSteps/failed`):
1 scenario, 1 step, both deliberately failed to prove godog surfaces a failing step.

```
5984:1 scenarios ([33m1 undefined[0m)
5985:1 steps ([33m1 undefined[0m)
```

The same two lines with the ESC bytes made visible (`grep -n ... | cat -v`):

```
5984:1 scenarios (^[[33m1 undefined^[[0m)
5985:1 steps (^[[33m1 undefined^[[0m)
```
— from the `/undefined` sub-fixture (line 5980 `=== RUN   TestGodogRejectsUndefinedAndFailedSteps/undefined`):
1 scenario, 1 step, deliberately undefined to prove godog surfaces an undefined step.

`--- PASS: TestGodogRejectsUndefinedAndFailedSteps` (line 5998) and its two subtests (lines
5999-6000) confirm the fixture itself passed as a Go test — i.e. godog correctly rejected both
the failing and the undefined step, which is what the fixture exists to prove.

## Files in this directory

- `verbose.log` — verbatim stdout+stderr of the sweep command (1,275,809 bytes).
- `verbose.log.exitstatus` — the captured exit status (`0`).
- this `README.md`.
