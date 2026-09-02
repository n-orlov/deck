# Task 127 — whole-suite sweep at the final code sha

## Final code sha

```
$ git log -1 --format=%H -- '*.go' '*.feature'
b9243a1f415ba9ca77cc2ffa2ec557ca8b1be4cb
```

`HEAD` at sweep time was also `b9243a1f415ba9ca77cc2ffa2ec557ca8b1be4cb`, so:

```
$ git diff --stat b9243a1f415ba9ca77cc2ffa2ec557ca8b1be4cb..HEAD -- '*.go' '*.feature'
(empty)
```

`git status --porcelain` was empty before the sweep started, and
`git rev-parse HEAD origin/main` agreed (both
`b9243a1f415ba9ca77cc2ffa2ec557ca8b1be4cb`).

## Execution shape

Run from the repo root (`/workspace`), exactly the standing-rules shape:

```
nohup sh -c 'timeout 1800 ci/run.sh go test -p=1 -count=1 ./... > sweep.log 2>&1; echo $? > sweep.log.exitstatus' >/dev/null 2>&1 &
```

(output redirected into this directory's `sweep.log` / `sweep.log.exitstatus`).
Started 2026-09-02T12:56:17Z, polled with `sleep 60` / `sleep 120` / `sleep 180`
loops, observed complete by 2026-09-02T13:03:36Z — elapsed ≈**7m19s**, in the
same ballpark as the ~6m15s standing-rules baseline (this run's `features`
package alone took 325.9s / ~5m26s).

## Exit status

`sweep.log.exitstatus` contains:

```
0
```

## Skips

- `--- SKIP` lines in `sweep.log`: **0**
- `?` (no test files) packages, all three listed by `go test`:
  - `github.com/n-orlov/deck/internal/notify`
  - `github.com/n-orlov/deck/internal/search`
  - `github.com/n-orlov/deck/internal/unit`

## Full per-package result

```
ok  	github.com/n-orlov/deck/cmd/deck	7.651s
ok  	github.com/n-orlov/deck/cmd/fake-claude	0.786s
ok  	github.com/n-orlov/deck/cmd/fake-pi	0.771s
ok  	github.com/n-orlov/deck/features	325.934s
ok  	github.com/n-orlov/deck/internal/agent	0.005s
ok  	github.com/n-orlov/deck/internal/audit	0.018s
ok  	github.com/n-orlov/deck/internal/config	0.024s
ok  	github.com/n-orlov/deck/internal/hookrecv	3.969s
ok  	github.com/n-orlov/deck/internal/interactive	11.089s
?   	github.com/n-orlov/deck/internal/notify	[no test files]
?   	github.com/n-orlov/deck/internal/search	[no test files]
ok  	github.com/n-orlov/deck/internal/service	4.230s
ok  	github.com/n-orlov/deck/internal/store	2.301s
ok  	github.com/n-orlov/deck/internal/theme	0.005s
ok  	github.com/n-orlov/deck/internal/tmux	19.543s
ok  	github.com/n-orlov/deck/internal/tui	3.489s
?   	github.com/n-orlov/deck/internal/unit	[no test files]
```

No `-run`, no `DECK_GODOG_PATHS`, no tag change — this is the unnarrowed
whole-suite sweep (`./...`).
