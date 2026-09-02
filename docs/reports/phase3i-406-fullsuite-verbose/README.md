# Phase 3i / task 406 — verbose companion sweep + Gherkin tally at the new final code sha

## Command (exact, same wrapper/polling discipline as task 404)

```
nohup sh -c 'timeout 2400 ci/run.sh go test -p=1 -count=1 -v ./... > /tmp/sweep-406.log 2>&1; echo $? > /tmp/sweep-406.log.exitstatus' >/dev/null 2>&1 &
```

Polled with `sleep 60` and no other sleep duration until `/tmp/sweep-406.log.exitstatus`
appeared. **7** `sleep 60` polls were used (poll 7 found the exit-status file). Launched
2026-09-02T21:51:12Z, exit-status file observed 2026-09-02T21:57Z.

No `-run`, no `DECK_GODOG_PATHS`. `features/godog_test.go`'s `defaultTags` is unchanged
(`"~@real-agents && ~@nightly"`).

## Where this ran — docs-only descendant of the frozen final code sha

The repo was at `e4d30284873099767116a324fedff382d0000a0f` (HEAD == origin/main, task 405's
docs commit) when this run was launched, and stayed there throughout (this is a read-only
test run; no commit was made mid-run). That commit is a docs-only descendant of the frozen
final code sha `3b70bfbc7e3552ff375ae675af117805a1eee944` (task 402):

```
$ git log --oneline 3b70bfbc7e3552ff375ae675af117805a1eee944..HEAD -- '*.go' '*.feature'
```

produced no output (empty), confirmed immediately before publishing this report, at the sha
this run executed at (`e4d3028`).

## Result

- **Exit status: 0**
- **17 package result lines**, all `ok` or `?` (no test files) — no `FAIL`:

```
ok  	github.com/n-orlov/deck/cmd/deck	7.301s
ok  	github.com/n-orlov/deck/cmd/fake-claude	0.793s
ok  	github.com/n-orlov/deck/cmd/fake-pi	0.771s
ok  	github.com/n-orlov/deck/features	318.123s
ok  	github.com/n-orlov/deck/internal/agent	0.005s
ok  	github.com/n-orlov/deck/internal/audit	0.018s
ok  	github.com/n-orlov/deck/internal/config	0.023s
ok  	github.com/n-orlov/deck/internal/hookrecv	4.110s
ok  	github.com/n-orlov/deck/internal/interactive	10.951s
?   	github.com/n-orlov/deck/internal/notify	[no test files]
?   	github.com/n-orlov/deck/internal/search	[no test files]
ok  	github.com/n-orlov/deck/internal/service	4.315s
ok  	github.com/n-orlov/deck/internal/store	2.314s
ok  	github.com/n-orlov/deck/internal/theme	0.006s
ok  	github.com/n-orlov/deck/internal/tmux	19.647s
ok  	github.com/n-orlov/deck/internal/tui	3.511s
?   	github.com/n-orlov/deck/internal/unit	[no test files]
```

(line numbers of these 17 lines in the captured log, found with
`grep -nE '^(ok|FAIL|\?)\s+github.com' sweep-verbose.log`: 28, 70, 108, 6103, 6209, 6231,
6387, 6815, 6958, 6959, 6960, 7134, 7320, 7569, 7926, 9629, 9630.)

## Gherkin tally

The two tally lines emitted by the `features` package's godog run, spliced as raw bytes
straight out of the captured log by `splice_tally.py` (never retyped):

```
$ python3 splice_tally.py sweep-verbose.log
scenarios tally, line 5642: b'319 scenarios (\x1b[32m319 passed\x1b[0m)'
  byte-containment check (line in open(path,'rb').read()): True
steps tally, line 5643: b'3682 steps (\x1b[32m3682 passed\x1b[0m)'
  byte-containment check (line in open(path,'rb').read()): True
```

- **Scenarios**: 319, line 5642 — `319 scenarios (319 passed)` (ANSI colour codes around
  "319 passed" in the raw bytes, shown above).
- **Steps**: 3682, line 5643 — `3682 steps (3682 passed)` (same ANSI wrapping).

`splice_tally.py`'s byte-containment check is exactly `line in open(path,'rb').read()`: it
finds each tally line as a `bytes` object by scanning the log's own `\n`-split lines, then
confirms that literal `bytes` object is a substring of the whole file's raw content — i.e.
the printed line is not a retyped/reformatted approximation, it is the log's own bytes. Both
checks printed `True`.

## Log

`sweep-verbose.log` in this directory is the captured `/tmp/sweep-406.log` verbatim. Proof of
byte-identity:

```
$ cmp /tmp/sweep-406.log docs/reports/phase3i-406-fullsuite-verbose/sweep-verbose.log
```

produced no output (files identical) and exit status 0. (The log is ~1.24 MB, 9630 lines;
not reproduced in full here — see the file.)
