# Task 003 — R94's three reverts hold, three named scenarios pass unedited

HEAD at the time this report was written == `origin/main` == this commit's parent
(`7634895`, task 002's proof-log commit). All commands below were run at that sha.

## The three revert commit shas resolve

```
$ git cat-file -e f700025^{commit}; echo "exit: $?"
exit: 0
$ git cat-file -e c176751^{commit}; echo "exit: $?"
exit: 0
$ git cat-file -e 0b7dce5^{commit}; echo "exit: $?"
exit: 0
```

- `f700025` — "features: forward-revert a53146a's stale-sampling re-pointing (task 001)" — R94
  revert 1, landed by an earlier approach.
- `c176751` — this plan's own task 001 revert target (R94 revert 2, `StopFailure`
  re-pointing), forward-reverted at `2f952da`.
- `0b7dce5` — this plan's own task 002 revert target (R94 revert 3, the narrow live-pane
  repair), forward-reverted at `afa55b8`.

## The three revert proofs, at HEAD, with output

```
$ git diff --exit-code a53146a^ HEAD -- features/status_probe.feature features/status_probe_test.go
$ echo "exit: $?"
exit: 0
```

```
$ git diff --exit-code c176751^ HEAD -- features/status_claude_hooks.feature
$ echo "exit: $?"
exit: 0
```

```
$ git diff --exit-code 0b7dce5^ HEAD -- internal/service/reconcile.go
$ echo "exit: $?"
exit: 0
```

All three diffs are empty: `status_probe.feature`/`status_probe_test.go`,
`status_claude_hooks.feature` and `internal/service/reconcile.go` are byte-for-byte identical
at HEAD to their state immediately before `a53146a`, `c176751` and `0b7dce5` respectively —
i.e. all three R94 reverts hold at HEAD, undisturbed by anything landed since.

## `status_attach.feature` passes unedited since the operator's plan commit

```
$ git diff --exit-code a24ff8d..HEAD -- features/status_attach.feature
$ echo "exit: $?"
exit: 0
```

Empty diff: nobody has touched `status_attach.feature` since the operator's own plan commit
`a24ff8d`, so the scenario below is exercised exactly as the operator wrote it — no repair
edit was needed to make it pass.

## The three named scenarios pass, run individually

Command shape (per the standing rules' diagnostics-only entry):

```
ci/run.sh env DECK_GODOG_PATHS=<file>.feature go test ./features/ -count=1
```

`DECK_GODOG_PATHS` is relative to the `features/` package directory (see
`features/godog_test.go`'s `godogPaths()`), so the value is `status_attach.feature`, not
`features/status_attach.feature`.

| Feature | Attempt | Exit | Wall time | Log |
|---|---|---|---|---|
| `status_attach.feature` | 1 | 0 | 19.310s | `status_attach.attempt1.log` |
| `status_claude_hooks.feature` | 1 | 0 | 21.251s | `status_claude_hooks.attempt1.log` |
| `status_probe.feature` | 1 | 0 | 20.278s | `status_probe.attempt1.log` |
| `status_probe.feature` | 2 | 0 | 20.222s | `status_probe.attempt2.log` |
| `status_probe.feature` | 3 | 0 | 20.424s | `status_probe.attempt3.log` |

Each `.log` is the raw stdout/stderr of its run; each matching `.exitstatus` file holds the
process exit code and is `0` in every case above.

`status_probe.feature`'s restored assertion is timing-sensitive (see the notes file's R94
gotcha), so it was run three times rather than once, and all three attempts — not only the
first — are committed here per the task's own instruction to publish every attempt, not just
the green one. In this run all three happened to be green; had any come back non-zero its log
and exit-status file would be committed alongside the others, unedited.

`status_attach.feature` and `status_claude_hooks.feature` were each run once: neither is
flagged as timing-sensitive anywhere in the plan or the notes file, and the single attempt
came back green.

## Scope

This report only runs and records the three named feature files individually. It does not
re-run the whole suite (`ci/run.sh go test -p=1 -count=1 ./...`) — that is a separate,
explicitly rate-limited command under the standing rules, and no criterion here calls for it.
