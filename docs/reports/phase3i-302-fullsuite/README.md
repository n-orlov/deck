# Task 302 — whole-suite sweep republished with strict 60-second polling

This directory was **re-run and republished** in a later iteration (2026-09-02T17:55Z) after a
review pass found the previous README's polling record inaccurate (it reported a `tail -10`
window as if it were the log's line count at that poll). Rather than patch a record taken from
an earlier iteration, the sweep was re-run from scratch here and every poll below is the
verbatim output of the command that was actually issued in that same iteration. `sweep.log` and
`sweep.log.exitstatus` in this directory are that re-run's own captured files.

## Command run

```
timeout 2400 ci/run.sh go test -p=1 -count=1 ./...
```

Launched backgrounded exactly as the standing rules require:

```
nohup sh -c 'timeout 2400 ci/run.sh go test -p=1 -count=1 ./... > /tmp/sweep-302.log 2>&1; echo $? > /tmp/sweep-302.log.exitstatus' >/dev/null 2>&1 &
```

No `-run`, no `DECK_GODOG_PATHS`, and `features/godog_test.go`'s `defaultTags` was not touched
(verified below). This supersedes task 204's `phase3i-204-fullsuite` (terminally `failed`,
polled with a `sleep 180` at one point) — this run polled with **`sleep 60` and no other
duration**, throughout, as recorded verbatim below.

## Final code sha (unchanged)

```
git log -1 --format=%H -- '*.go' '*.feature'
```
→ `a559e7c61a00ab5fa31c2d98eaf5ce787744e4dc` — unchanged; approach 3 lands zero
`*.go`/`*.feature` changes, so this is the same sha R98-R102 were reviewed at.
`git log --oneline a559e7c..HEAD -- '*.go' '*.feature'` printed nothing at launch time and
again after the sweep.

## Verbatim polling record

Every completion check was issued as exactly this one command, differing only in the poll it
belongs to — the `sleep 60` prefix is part of the check itself, so no check in this iteration
was preceded by any other sleep duration (no `sleep 180`, no `sleep 30`, no bare check):

```
sleep 60; date -u +%Y-%m-%dT%H:%M:%SZ; wc -l < /tmp/sweep-302.log; cat /tmp/sweep-302.log.exitstatus 2>/dev/null || echo "no exitstatus yet"
```

Launch timestamp, printed by `date -u +%Y-%m-%dT%H:%M:%SZ` in the launching command:

```
2026-09-02T17:55:40Z
```

Then, verbatim output of each poll (timestamp / `wc -l` of the log so far / exit-status file):

```
Poll 1: 2026-09-02T17:56:43Z
        3
        no exitstatus yet
Poll 2: 2026-09-02T17:57:45Z
        3
        no exitstatus yet
Poll 3: 2026-09-02T17:58:47Z
        3
        no exitstatus yet
Poll 4: 2026-09-02T17:59:49Z
        3
        no exitstatus yet
Poll 5: 2026-09-02T18:00:52Z
        3
        no exitstatus yet
Poll 6: 2026-09-02T18:01:54Z
        14
        no exitstatus yet
Poll 7: 2026-09-02T18:02:56Z
        17
        0
```

Reading of that record: the numbers above are whole-file line counts from `wc -l`, not a
`tail` window. Polls 1-5 show the three `cmd/...` package results with the `features` package
(329.502s) still running; poll 6 shows 14 lines — `features` had finished and the packages
through `internal/theme` had reported; poll 7 shows all 17 package result lines and
`sweep.log.exitstatus` containing `0`, i.e. the sweep observed complete.

- Wall time: launch 17:55:40Z → observed complete at the poll ending 18:02:56Z, ≈7m16s of
  polling granularity (the sweep itself finished between 18:01:54Z and 18:02:56Z), in line with
  the ~6.6–7.3 min previously measured for this sweep.
- Seven polls, all `sleep 60`.

## Byte-identity of the published log

The sweep wrote `/tmp/sweep-302.log`; it was copied here unmodified and compared:

```
cmp /tmp/sweep-302.log docs/reports/phase3i-302-fullsuite/sweep.log   # no output → identical
wc -c /tmp/sweep-302.log docs/reports/phase3i-302-fullsuite/sweep.log
 894 /tmp/sweep-302.log
 894 docs/reports/phase3i-302-fullsuite/sweep.log
```

`sweep.log.exitstatus` was copied from `/tmp/sweep-302.log.exitstatus` the same way.

## Exit status

Captured from the `.exitstatus` file written by the backgrounded shell, never from `$?` after a
`tee`:

```
0
```
(see `sweep.log.exitstatus` in this directory)

## Tag exclusion

`features/godog_test.go`'s `defaultTags` (`grep -n defaultTags features/godog_test.go`, line 16):

```
const defaultTags = "~@real-agents && ~@nightly"
```

This is the only tag exclusion in effect for this sweep, and it is unchanged from the frozen
final code sha. No other skipped marker, `-run` narrowing, `DECK_GODOG_PATHS` override or
module-level skip was used.

## Package count

`ci/run.sh go list ./...` at this sha exits 0 and lists **17** packages; `sweep.log` reports
exactly **17** package result lines (`grep -cE '^(ok|\?|FAIL|---)' sweep.log` → 17), one per
package, and `grep -c FAIL sweep.log` → 0.

## All 17 package result lines (verbatim, from `sweep.log`)

```
ok  	github.com/n-orlov/deck/cmd/deck	7.657s
ok  	github.com/n-orlov/deck/cmd/fake-claude	0.788s
ok  	github.com/n-orlov/deck/cmd/fake-pi	0.778s
ok  	github.com/n-orlov/deck/features	329.502s
ok  	github.com/n-orlov/deck/internal/agent	0.006s
ok  	github.com/n-orlov/deck/internal/audit	0.018s
ok  	github.com/n-orlov/deck/internal/config	0.035s
ok  	github.com/n-orlov/deck/internal/hookrecv	4.315s
ok  	github.com/n-orlov/deck/internal/interactive	11.674s
?   	github.com/n-orlov/deck/internal/notify	[no test files]
?   	github.com/n-orlov/deck/internal/search	[no test files]
ok  	github.com/n-orlov/deck/internal/service	4.463s
ok  	github.com/n-orlov/deck/internal/store	2.427s
ok  	github.com/n-orlov/deck/internal/theme	0.006s
ok  	github.com/n-orlov/deck/internal/tmux	19.800s
ok  	github.com/n-orlov/deck/internal/tui	3.548s
?   	github.com/n-orlov/deck/internal/unit	[no test files]
```

All 17 lines report `ok` (14) or `[no test files]` (3, expected — `internal/notify`,
`internal/search`, `internal/unit` carry no `_test.go` files at this sha); none report `FAIL`.

## Files in this directory

- `sweep.log` — verbatim stdout+stderr of the sweep command (894 bytes).
- `sweep.log.exitstatus` — the captured exit status (`0`).
- this `README.md`.
