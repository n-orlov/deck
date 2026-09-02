# Phase 3i / task 404 — whole-suite sweep at the new final code sha

## Command (exact, unnarrowed)

```
nohup sh -c 'timeout 2400 ci/run.sh go test -p=1 -count=1 ./... > /tmp/sweep-404.log 2>&1; echo $? > /tmp/sweep-404.log.exitstatus' >/dev/null 2>&1 &
```

Polled with `sleep 60` and no other sleep duration until `/tmp/sweep-404.log.exitstatus`
appeared. **7** `sleep 60` polls were used (poll 7 found the exit-status file).

No `-run`, no `DECK_GODOG_PATHS`. `features/godog_test.go`'s `defaultTags` is unchanged
(`"~@real-agents && ~@nightly"`, line 16, confirmed with `grep -n defaultTags
features/godog_test.go` immediately before publishing this report).

## Result

- **Exit status: 0**
- **Final code sha this sweep ran at**: `3b70bfbc7e3552ff375ae675af117805a1eee944`
  (`git log -1 --format=%H -- '*.go' '*.feature'`, run immediately before publishing).
- **17 package result lines**, all `ok` or `?` (no test files) — no `FAIL`:

```
ok  	github.com/n-orlov/deck/cmd/deck	7.480s
ok  	github.com/n-orlov/deck/cmd/fake-claude	0.789s
ok  	github.com/n-orlov/deck/cmd/fake-pi	0.774s
ok  	github.com/n-orlov/deck/features	327.839s
ok  	github.com/n-orlov/deck/internal/agent	0.005s
ok  	github.com/n-orlov/deck/internal/audit	0.018s
ok  	github.com/n-orlov/deck/internal/config	0.033s
ok  	github.com/n-orlov/deck/internal/hookrecv	4.066s
ok  	github.com/n-orlov/deck/internal/interactive	11.106s
?   	github.com/n-orlov/deck/internal/notify	[no test files]
?   	github.com/n-orlov/deck/internal/search	[no test files]
ok  	github.com/n-orlov/deck/internal/service	4.390s
ok  	github.com/n-orlov/deck/internal/store	2.337s
ok  	github.com/n-orlov/deck/internal/theme	0.006s
ok  	github.com/n-orlov/deck/internal/tmux	19.642s
ok  	github.com/n-orlov/deck/internal/tui	3.392s
?   	github.com/n-orlov/deck/internal/unit	[no test files]
```

## Log

`sweep.log` in this directory is the captured `/tmp/sweep-404.log` verbatim. Proof of
byte-identity:

```
$ cmp /tmp/sweep-404.log docs/reports/phase3i-404-fullsuite/sweep.log
```

produced no output (files identical) and exit status 0.

## Note on the prior attempt

A first launch of this exact same command (same iteration-boundary evidence, not counted
against this iteration's one-whole-suite-run budget) failed at exit 1 with a single flake:
`TestFeatures/in_passive_preview,_a_live_pane_larger_than_the_panel_is_cropped_with_its_real_
geometry_stated` (`preview.feature:177`) timed out waiting for frame "starting" after 66.21s.
That attempt's log is preserved as prior evidence only, not as a discharge, at
`/run/ralphd/artifacts/notes-evidence/sweep-404-attempt1-FAIL.log` (and
`.exitstatus`). `git log --oneline 3b70bfb..HEAD -- features/preview.feature` is empty, so
that scenario's source did not change between the two attempts; this run's clean pass on the
same sha is consistent with the first failure having been a flake rather than a regression,
and this sweep's result — exit 0, that same scenario passing within the `features` package's
overall `ok 327.839s` — is what this report discharges task 404 on.
