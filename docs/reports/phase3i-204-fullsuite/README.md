# Task 204 — whole-suite sweep at the final code sha

## Command run

```
timeout 2400 ci/run.sh go test -p=1 -count=1 ./...
```

Launched backgrounded (`nohup sh -c '... > sweep.log 2>&1; echo $? > sweep.log.exitstatus' >/dev/null 2>&1 &`)
and polled in `sleep 60`/`sleep 180` loops, exactly as the standing rules require. No `-run`,
no `DECK_GODOG_PATHS`, and `features/godog_test.go`'s `defaultTags` was not touched.

- Launched: 2026-09-02T15:26:22Z
- Observed complete (poll where the log stopped growing and the exit-status file appeared):
  2026-09-02T15:33:31Z
- Wall time: ~7m09s, in line with the ~7m19s previously measured for this sweep.

## Exit status

Captured from `sweep.log.exitstatus`, never from `$?` after a `tee`:

```
0
```
(see `sweep.log.exitstatus` in this directory)

## Final code sha

```
git log -1 --format=%H -- '*.go' '*.feature'
```
→ `a559e7c61a00ab5fa31c2d98eaf5ce787744e4dc` (task 203's commit; this is also current `HEAD`
and `origin/main` at the time this sweep ran — no code commit has landed since 203).

## Tag exclusion

`features/godog_test.go`'s `defaultTags`:

```
const defaultTags = "~@real-agents && ~@nightly"
```

This is the only tag exclusion in effect for this sweep. No other skipped marker, `-run`
narrowing, `DECK_GODOG_PATHS` override or module-level skip was used.

## All 17 package result lines (verbatim, from `sweep.log`)

```
ok  	github.com/n-orlov/deck/cmd/deck	7.717s
ok  	github.com/n-orlov/deck/cmd/fake-claude	0.790s
ok  	github.com/n-orlov/deck/cmd/fake-pi	0.766s
ok  	github.com/n-orlov/deck/features	330.974s
ok  	github.com/n-orlov/deck/internal/agent	0.005s
ok  	github.com/n-orlov/deck/internal/audit	0.018s
ok  	github.com/n-orlov/deck/internal/config	0.026s
ok  	github.com/n-orlov/deck/internal/hookrecv	4.256s
ok  	github.com/n-orlov/deck/internal/interactive	11.248s
?   	github.com/n-orlov/deck/internal/notify	[no test files]
?   	github.com/n-orlov/deck/internal/search	[no test files]
ok  	github.com/n-orlov/deck/internal/service	4.221s
ok  	github.com/n-orlov/deck/internal/store	2.451s
ok  	github.com/n-orlov/deck/internal/theme	0.004s
ok  	github.com/n-orlov/deck/internal/tmux	19.663s
ok  	github.com/n-orlov/deck/internal/tui	3.491s
?   	github.com/n-orlov/deck/internal/unit	[no test files]
```

All 17 lines report `ok` (14) or `[no test files]` (3, expected — `internal/notify`,
`internal/search`, `internal/unit` carry no `_test.go` files at this sha); none report `FAIL`.

## Files in this directory

- `sweep.log` — verbatim stdout+stderr of the sweep command.
- `sweep.log.exitstatus` — the captured exit status (`0`).
- this `README.md`.
