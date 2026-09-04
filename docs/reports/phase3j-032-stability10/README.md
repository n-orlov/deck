# Phase 3j — stability10 gate (task 032)

## Supersedes

This refresh supersedes the earlier gate published at code sha
`b29afb8c4fd8a1cf193c7efef5c5f7e1456481f7` (task 060, approach 03). Task 080 of
this approach (05) edited `features/launch_hooks.feature` (comments only),
which under the plan's Termination rule advances the final code sha
(`git log -1 --format=%H -- '*.go' '*.feature'`) past `b29afb8`, forcing this
gate to be re-run and refreshed **in place** at the new final code sha — no
new numbered report directory.

## Code sha and HEAD

Final code sha (last commit touching `*.go` or `*.feature`) at the time this
gate was launched — task 080's own commit:

```
$ git log -1 --format=%H -- '*.go' '*.feature'
4fbd452430501805a860dd229ddca1cd3f5c1cd6
```

`HEAD`/`origin/main` at launch time: `e00d40f2e578b512c99700ff411185284ef3a87b`,
a docs-only descendant of `4fbd452` (task 081's `phase3j-030-fullsuite` refresh
and task 082's `phase3j-031-fullsuite-verbose` refresh, both under `docs/reports/`
only, no `*.go`/`*.feature` change) — per the plan's standing rules a docs-only
tail commit does not invalidate a gate. `git status --porcelain` was empty and
`git rev-parse HEAD origin/main` agreed on `e00d40f2e578b512c99700ff411185284ef3a87b`
both before this gate launched and after collection.

This sha supersedes, and is a descendant of, `b29afb8c4fd8a1cf193c7efef5c5f7e1456481f7`
(the previously published gate sha).

## Command

Launched exactly as this task's own success criteria specify, no other test or
gate run in the same iteration:

```
nohup sh -c 'timeout 7200 ci/stability.sh 10 > docs/reports/phase3j-032-stability10/summary.log 2>&1; echo $? > docs/reports/phase3j-032-stability10/summary.log.exitstatus' >/dev/null 2>&1 &
```

Launched 2026-09-04T01:10:39Z, polled with `sleep 120` only (never a longer or
shorter interval, and no completion check before the first such poll), no
narrowing of the command. `docs/reports/phase3j-032-stability10/summary.log.exitstatus`
was first observed present at 2026-09-04T02:18:05Z (~67 minutes for 10 runs —
each run invokes the whole-suite sweep `ci/run.sh go test -p=1 -count=1 ./...`,
matching the earlier gate's cadence).

## Script's own captured exit status

```
$ cat docs/reports/phase3j-032-stability10/summary.log.exitstatus
1
```

## Result

The published `summary.log` in this directory is exactly what
`ci/stability.sh 10`'s own stdout wrote when redirected straight into it per
the command above (the script deliberately suppresses the full per-run test
output from its own stdout via a trailing `>/dev/null` on that `tee`, so only
the `=== RUN i ===` / `=== RUN i: PASS|FAIL (exit N) ===` header lines and the
two trailer lines land in that file — full per-run output goes only to the
script's internal `$outdir/run-i.log` files). Its final line, quoted
verbatim, never rounded up:

```
9/10 passed
```

That tally is not `10/10 passed`. One run failed:

- **RUN 9**: `=== RUN 9: FAIL (exit 1) ===`. Per-run log path (the script's own
  ephemeral sibling-container `/tmp` output directory, named in the summary
  trailer line `full per-run logs and combined summary log kept in:
  /tmp/deck-stability.Rz6fRM`, not a repo path and not expected to survive
  past this run): `/tmp/deck-stability.Rz6fRM/run-9.log`. Its content is
  reproduced verbatim below for durability, since that `/tmp` path is not
  part of this repository and may be cleaned up:

```
ok  	github.com/n-orlov/deck/cmd/deck	7.402s
ok  	github.com/n-orlov/deck/cmd/fake-claude	0.794s
ok  	github.com/n-orlov/deck/cmd/fake-pi	0.773s
ok  	github.com/n-orlov/deck/features	341.652s
ok  	github.com/n-orlov/deck/internal/agent	0.004s
ok  	github.com/n-orlov/deck/internal/audit	0.018s
ok  	github.com/n-orlov/deck/internal/config	0.024s
ok  	github.com/n-orlov/deck/internal/hookrecv	4.121s
ok  	github.com/n-orlov/deck/internal/interactive	11.114s
?   	github.com/n-orlov/deck/internal/notify	[no test files]
?   	github.com/n-orlov/deck/internal/search	[no test files]
ok  	github.com/n-orlov/deck/internal/service	6.378s
ok  	github.com/n-orlov/deck/internal/store	2.583s
ok  	github.com/n-orlov/deck/internal/theme	0.004s
--- FAIL: TestSendKeysUnknownKeyNameIsDeliveredAsLiteralTextWithExitZero (0.01s)
    literal_send_test.go:158: pane capture = "F$ robnicate", want it to contain the literal text "Frobnicate" (ten bytes typed as-is, PRD item 35/II-38)
FAIL
FAIL	github.com/n-orlov/deck/internal/tmux	19.619s
ok  	github.com/n-orlov/deck/internal/tui	3.577s
?   	github.com/n-orlov/deck/internal/unit	[no test files]
FAIL
```

The other 9 runs (1-8, 10) are each labelled `PASS` in `summary.log`
(`=== RUN i: PASS (exit 0) ===`).

This single failure is an intermittent flake in
`internal/tmux.TestSendKeysUnknownKeyNameIsDeliveredAsLiteralTextWithExitZero`
(pane capture off by one character, `"F$ robnicate"` instead of
`"Frobnicate"` — a tmux/pty timing race in the test's own key-delivery
polling, not a deterministic failure of the product code touched by this
approach's task 080 comment-only change). The published number (9/10) is
exactly what the script reported; the gate was not re-run to try to improve
or otherwise alter it. This gate's own result stands as the record of the
flake; a later task in this approach may cite it in
`docs/reports/phase3j-findings.md` when that document is next refreshed.
