# Task 010 — `ci/stability.sh 10` at the final code sha

## Command, exactly as run

```
nohup sh -c 'timeout 6000 ci/stability.sh 10 > /tmp/phase3h-stability.log 2>&1; echo $? > /tmp/phase3h-stability.exitstatus' >/dev/null 2>&1 &
```

Polled with `sleep 180` until `/tmp/phase3h-stability.exitstatus` appeared (~66 minutes wall
clock, inside the 60-70 min estimate and well under the `timeout 6000` = 100 min ceiling).

## Preconditions (per this task's `successCriteria`)

- `git status --porcelain` was empty immediately before launch.
- `HEAD` at launch was `1f919b7` (task 009's docs-only commit), a docs-only descendant of the
  **final code sha** `2ccb1d3` (`git log -1 --format=%H -- '*.go' '*.feature'`):
  `git diff --name-only 2ccb1d3..1f919b7` touches only paths under
  `docs/reports/phase3h-008-fullsuite/` and `docs/reports/phase3h-009-fullsuite-verbose/`.
  `HEAD` == `origin/main` == `1f919b7` at launch.

## Result — 10/10 passed

The script's own captured exit status (`docs/reports/phase3h-010-stability10/.exitstatus`):

```
0
```

The `N/10 passed` line, quoted verbatim from `docs/reports/phase3h-010-stability10/summary.log`:

```
10/10 passed
```

Every one of the 10 runs printed `=== RUN k: PASS (exit 0) ===` for k=1..10 in
`summary.log`, and each `run-k.log` (k=1..10, all committed alongside `summary.log`) shows
all 17 packages `ok` or `[no test files]` (`internal/notify`, `internal/search`,
`internal/unit`), matching the shape of tasks 008/009's whole-suite sweep at the same final
code sha. No failing run, so there is nothing to classify against the PRD's out-of-scope
list (F2, F20, F22, F37, the `filter.feature` dd/undo race) and nothing new to name.

Representative per-run package timing (`run-1.log`, all ten runs consistent):

```
ok  	github.com/n-orlov/deck/cmd/deck	7.639s
ok  	github.com/n-orlov/deck/cmd/fake-claude	0.793s
ok  	github.com/n-orlov/deck/cmd/fake-pi	0.777s
ok  	github.com/n-orlov/deck/features	319.083s
ok  	github.com/n-orlov/deck/internal/agent	0.005s
ok  	github.com/n-orlov/deck/internal/audit	0.018s
ok  	github.com/n-orlov/deck/internal/config	0.028s
ok  	github.com/n-orlov/deck/internal/hookrecv	4.200s
ok  	github.com/n-orlov/deck/internal/interactive	11.237s
?   	github.com/n-orlov/deck/internal/notify	[no test files]
?   	github.com/n-orlov/deck/internal/search	[no test files]
ok  	github.com/n-orlov/deck/internal/service	4.388s
ok  	github.com/n-orlov/deck/internal/store	2.433s
ok  	github.com/n-orlov/deck/internal/theme	0.004s
ok  	github.com/n-orlov/deck/internal/tmux	19.444s
ok  	github.com/n-orlov/deck/internal/tui	1.427s
?   	github.com/n-orlov/deck/internal/unit	[no test files]
```

## Files in this directory

- `summary.log` — the script's own combined summary log (copied verbatim from its
  `mktemp -d` output directory, `/tmp/deck-stability.WPir0x/summary.log` at run time).
- `run-1.log` .. `run-10.log` — the ten per-run logs, copied verbatim from the same
  `mktemp -d` directory.
- `.exitstatus` — the captured exit status of `ci/stability.sh 10` itself (`0`), read from
  the file per the standing rule against reading a background command's status through a
  pipe.
- This `README.md`.

## Final code sha, unchanged by this docs-only task

`2ccb1d3c698d3c256aa94aea7ef2e3a58dc0641a` (`git log -1 --format=%H -- '*.go' '*.feature'`),
same as tasks 006-009. This task adds no `*.go`/`*.feature` change, so the final code sha is
unchanged by this commit.
