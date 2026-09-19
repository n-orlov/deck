# Phase 4b — whole-suite gate sweep (task 021)

Freeze-line sweep: the whole Go test suite, no test-name filter, no
narrowed package list, run once in the CI container at this task's launch
sha.

- **Code sha (launch sha for task 021):** `0806ba64ede4356af50b404affd10f26f68d4d84`
  (task 020, "tui: prove shared-state.db group edits reach another client on
  reload (R131)" — the last code-touching commit on the GO branch; the
  freeze on record-only commits begins from this task's launch).
- **Invocation:** `ci/run.sh go test -p=1 -count=1 ./...` (the CI sibling
  container wrapper over the module's packages; `-p=1` serialises package
  execution the same way the planning-time baseline measured it).
- **Wall clock:** 440s (start `2026-09-19T15:31:13Z`, end
  `2026-09-19T15:38:33Z`) — in line with the launch-sha baseline of 438s
  recorded in the notes at planning time.
- **Exit status:** `0` (green), taken from the run's own exit code
  (`EXIT:0` captured immediately after the `go test` invocation returned).

## Package result lines (from the captured log)

```
ok  	github.com/n-orlov/deck/cmd/deck	7.495s
ok  	github.com/n-orlov/deck/cmd/fake-claude	0.791s
ok  	github.com/n-orlov/deck/cmd/fake-codex	0.118s
ok  	github.com/n-orlov/deck/cmd/fake-pi	0.775s
ok  	github.com/n-orlov/deck/features	365.998s
ok  	github.com/n-orlov/deck/internal/agent	0.009s
ok  	github.com/n-orlov/deck/internal/audit	0.018s
ok  	github.com/n-orlov/deck/internal/config	0.031s
ok  	github.com/n-orlov/deck/internal/hookrecv	4.532s
ok  	github.com/n-orlov/deck/internal/interactive	15.296s
?   	github.com/n-orlov/deck/internal/notify	[no test files]
?   	github.com/n-orlov/deck/internal/search	[no test files]
ok  	github.com/n-orlov/deck/internal/service	6.860s
ok  	github.com/n-orlov/deck/internal/store	3.185s
ok  	github.com/n-orlov/deck/internal/theme	0.006s
ok  	github.com/n-orlov/deck/internal/tmux	25.341s
ok  	github.com/n-orlov/deck/internal/tui	6.203s
?   	github.com/n-orlov/deck/internal/unit	[no test files]
```

Every package the module resolves is present in the list above: 18 package
lines in total — 15 `ok` lines and 3 `[no test files]` lines
(`internal/notify`, `internal/search`, `internal/unit` — the out-of-scope
Phase 5/6/7 packages that stay one-line `doc.go` per the standing rules),
no `FAIL` lines. The count is reproducible from the committed log and the
module's own package list: `grep -c '^ok' fullsuite.log` = 15,
`grep -c 'no test files' fullsuite.log` = 3, `grep -c FAIL fullsuite.log`
= 0, and `ci/run.sh go list ./... | wc -l` = 18, so no package the module
resolves is missing from the log.

(An earlier revision of this README miscounted the `ok` lines as 14; the
committed log always held 15. Corrected here without re-running the gate —
the run itself, its sha, wall time and exit status are unchanged.)

## Skips

The one skip in force is godog's own default tag filter,
`~@real-agents && ~@nightly` (`features/godog_test.go:16`, `defaultTags`),
which this invocation used unmodified — no `DECK_GODOG_TAGS` override was
set. No other marker, mode or package was skipped: the invocation carried
no `-run` filter and no narrowed package list, and every package the log
lists ran (or reported `[no test files]`) under that single default godog
tag filter.

## Result

Green. No lane is red; there is nothing to carve into a new task.

The full captured log is committed alongside this README as
[`fullsuite.log`](./fullsuite.log).
