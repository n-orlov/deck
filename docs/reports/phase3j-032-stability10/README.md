# Phase 3j — stability10 gate (task 032)

## Supersedes

This refresh supersedes the earlier gate published at code sha
`b29afb8c4fd8a1cf193c7efef5c5f7e1456481f7` (task 060, approach 03). Task 080 of
this approach (05) edited `features/launch_hooks.feature` (comments only),
which under the plan's Termination rule advances the final code sha
(`git log -1 --format=%H -- '*.go' '*.feature'`) past `b29afb8`, forcing this
gate to be re-run and refreshed **in place** at the new final code sha — no
new numbered report directory.

It also supersedes, in this same directory, the first approach-05 collection of
this gate (commit `2de170049ba6fb820dacbfbb68c6b2dc4b375c15`, task 083's first
attempt at the same final code sha `4fbd452430501805a860dd229ddca1cd3f5c1cd6`).
That earlier collection reported `9/10 passed` — RUN 9 failed on an
intermittent tmux/pty timing flake in
`internal/tmux.TestSendKeysUnknownKeyNameIsDeliveredAsLiteralTextWithExitZero`
(pane capture `"F$ robnicate"` instead of the literal `"Frobnicate"`). Its
collection was rejected on procedure, not on the number: the mandated polling
discipline (`sleep 120` only, and no inspection before the first such poll) was
not followed while that run was in flight, so the gate was re-launched once, at
the same code sha, with the procedure followed exactly. Its 9/10 measurement is
kept on the record here as evidence that this test can flake intermittently at
this sha — the 10/10 below is not a claim that it never does.

## Code sha and HEAD

Final code sha (last commit touching `*.go` or `*.feature`) at the time this
gate was launched — task 080's own commit:

```
$ git log -1 --format=%H -- '*.go' '*.feature'
4fbd452430501805a860dd229ddca1cd3f5c1cd6
```

`HEAD`/`origin/main` at launch time: `2de170049ba6fb820dacbfbb68c6b2dc4b375c15`,
a docs-only descendant of `4fbd452` (task 081's `phase3j-030-fullsuite` refresh,
task 082's `phase3j-031-fullsuite-verbose` refresh and this directory's first
approach-05 collection, all under `docs/reports/` only, no `*.go`/`*.feature`
change) — per the plan's standing rules a docs-only tail commit does not
invalidate a gate. `git status --porcelain` was empty before the launch and
showed only this directory's own two written files
(`docs/reports/phase3j-032-stability10/summary.log` and
`docs/reports/phase3j-032-stability10/summary.log.exitstatus`) after collection;
`git rev-parse HEAD origin/main` agreed on
`2de170049ba6fb820dacbfbb68c6b2dc4b375c15` throughout.

This sha supersedes, and is a descendant of, `b29afb8c4fd8a1cf193c7efef5c5f7e1456481f7`
(the previously published gate sha).

## Command

Launched exactly once, exactly as this task's own success criteria specify, with
no other test or gate run in the same iteration:

```
nohup sh -c 'timeout 7200 ci/stability.sh 10 > docs/reports/phase3j-032-stability10/summary.log 2>&1; echo $? > docs/reports/phase3j-032-stability10/summary.log.exitstatus' >/dev/null 2>&1 &
```

Launched 2026-09-04T02:22:21Z. Polled exclusively with `sleep 120` — every
inspection of the directory was preceded, in the same shell invocation, by one
`sleep 120`, with no inspection at all before the first such sleep, no shorter
or longer interval, and no narrowing of the command. The
`summary.log.exitstatus` file was first observed present at
2026-09-04T03:29:00Z (~66.5 minutes for 10 runs — each run invokes the
whole-suite sweep `ci/run.sh go test -p=1 -count=1 ./...`, matching the earlier
gate's cadence).

## Script's own captured exit status

```
$ cat docs/reports/phase3j-032-stability10/summary.log.exitstatus
0
```

Exit status `0` is `ci/stability.sh`'s own success path: it exits 1 whenever any
run failed, so a `0` here corroborates the tally below.

## Result

The published `summary.log` in this directory is exactly what
`ci/stability.sh 10`'s own stdout wrote when redirected straight into it per
the command above (the script deliberately suppresses the full per-run test
output from its own stdout via a trailing `>/dev/null` on that `tee`, so only
the `=== RUN i ===` / `=== RUN i: PASS|FAIL (exit N) ===` header lines and the
two trailer lines land in that file — full per-run output goes only to the
script's internal per-run log files inside its own ephemeral `mktemp` output
directory, which is not part of this repository).

Its final line, the tally, quoted verbatim from the published `summary.log`:

```
10/10 passed
```

All ten runs are labelled `PASS` in that file:

| run | label in `summary.log` |
| --- | --- |
| 1 | `=== RUN 1: PASS (exit 0) ===` |
| 2 | `=== RUN 2: PASS (exit 0) ===` |
| 3 | `=== RUN 3: PASS (exit 0) ===` |
| 4 | `=== RUN 4: PASS (exit 0) ===` |
| 5 | `=== RUN 5: PASS (exit 0) ===` |
| 6 | `=== RUN 6: PASS (exit 0) ===` |
| 7 | `=== RUN 7: PASS (exit 0) ===` |
| 8 | `=== RUN 8: PASS (exit 0) ===` |
| 9 | `=== RUN 9: PASS (exit 0) ===` |
| 10 | `=== RUN 10: PASS (exit 0) ===` |

No run failed, so there is no failing run to name and no per-run log path to
cite. The tally is the script's own number, published as measured and neither
rounded nor re-run to improve it.

## Reading this together with the superseded 9/10

Both collections ran the same command at the same final code sha
`4fbd452430501805a860dd229ddca1cd3f5c1cd6`. The pair — 9/10 then 10/10 — is
itself the evidence that
`internal/tmux.TestSendKeysUnknownKeyNameIsDeliveredAsLiteralTextWithExitZero`
is intermittently sensitive to tmux/pty key-delivery timing under load on this
host, rather than deterministically broken by this approach's comment-only
change. This gate's published number is the 10/10 above; the intermittency is
carried forward as an advisory observation for `docs/reports/phase3j-findings.md`
when that document is next refreshed, not as a claim that the suite is
flake-free.
