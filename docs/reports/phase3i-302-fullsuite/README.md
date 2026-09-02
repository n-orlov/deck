# Task 302 — whole-suite sweep republished with strict 60-second polling

## Command run

```
timeout 2400 ci/run.sh go test -p=1 -count=1 ./...
```

Launched backgrounded exactly as the standing rules require:

```
nohup sh -c 'timeout 2400 ci/run.sh go test -p=1 -count=1 ./... > docs/reports/phase3i-302-fullsuite/sweep.log 2>&1; echo $? > docs/reports/phase3i-302-fullsuite/sweep.log.exitstatus' >/dev/null 2>&1 &
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

## Verbatim polling record

- Launched: 2026-09-02T17:38:59Z
- Poll 1 (`sleep 60` then check): sweep.log had 3 package result lines (cmd/deck,
  cmd/fake-claude, cmd/fake-pi); no exitstatus file yet.
- Poll 2 (`sleep 60`): unchanged at 3 lines (the `features` package, ~330s, was still running).
- Poll 3 (`sleep 60`): unchanged at 3 lines.
- Poll 4 (`sleep 60`): unchanged at 3 lines; no exitstatus file yet.
- Poll 5 (`sleep 60`): unchanged at 3 lines; no exitstatus file yet.
- Poll 6 (`sleep 60`): 10 lines now present (through `internal/theme`); `features` had
  completed; no exitstatus file yet.
- Poll 7 (`sleep 60`): 17 lines present (all packages reported), `sweep.log.exitstatus`
  contained `0`. Sweep observed complete.
- Wall time: ~7m (launch 17:38:59Z, observed complete at the poll ending ~17:46Z), in line
  with the ~6.6–7.3 min previously measured for this sweep.

Every completion check in this task was preceded by `sleep 60` and by no other sleep
duration — no `sleep 180` was used anywhere in this run.

## Exit status

Captured from `sweep.log.exitstatus`, never from `$?` after a `tee`:

```
0
```
(see `sweep.log.exitstatus` in this directory)

## Tag exclusion

`features/godog_test.go`'s `defaultTags`:

```
const defaultTags = "~@real-agents && ~@nightly"
```

This is the only tag exclusion in effect for this sweep, and it is unchanged from the frozen
final code sha. No other skipped marker, `-run` narrowing, `DECK_GODOG_PATHS` override or
module-level skip was used.

## Package count

`ci/run.sh go list ./...` at this sha lists 17 packages; `sweep.log` reports exactly 17
package result lines, one per package.

## All 17 package result lines (verbatim, from `sweep.log`)

```
ok  	github.com/n-orlov/deck/cmd/deck	7.713s
ok  	github.com/n-orlov/deck/cmd/fake-claude	0.794s
ok  	github.com/n-orlov/deck/cmd/fake-pi	0.772s
ok  	github.com/n-orlov/deck/features	331.179s
ok  	github.com/n-orlov/deck/internal/agent	0.005s
ok  	github.com/n-orlov/deck/internal/audit	0.018s
ok  	github.com/n-orlov/deck/internal/config	0.026s
ok  	github.com/n-orlov/deck/internal/hookrecv	4.161s
ok  	github.com/n-orlov/deck/internal/interactive	11.334s
?   	github.com/n-orlov/deck/internal/notify	[no test files]
?   	github.com/n-orlov/deck/internal/search	[no test files]
ok  	github.com/n-orlov/deck/internal/service	4.260s
ok  	github.com/n-orlov/deck/internal/store	2.296s
ok  	github.com/n-orlov/deck/internal/theme	0.004s
ok  	github.com/n-orlov/deck/internal/tmux	19.577s
ok  	github.com/n-orlov/deck/internal/tui	3.428s
?   	github.com/n-orlov/deck/internal/unit	[no test files]
```

All 17 lines report `ok` (14) or `[no test files]` (3, expected — `internal/notify`,
`internal/search`, `internal/unit` carry no `_test.go` files at this sha); none report `FAIL`.

## Files in this directory

- `sweep.log` — verbatim stdout+stderr of the sweep command.
- `sweep.log.exitstatus` — the captured exit status (`0`).
- this `README.md`.
