# Phase 3i task 129 — `ci/stability.sh 10` at the final code sha

## Command shape (backgrounded, per standing rules)

```
nohup sh -c 'cd /workspace && timeout 6000 ci/stability.sh 10 > out.log 2>&1; echo $? > out.log.exitstatus' >/dev/null 2>&1 &
```

Polled with `sleep 180` loops (occasionally `sleep 120` near the tail) against
`out.log` / `out.log.exitstatus`, never piped through `tee` for the
exit-status-bearing command — `ci/stability.sh` itself follows the same
no-`tee`-on-the-status-line discipline internally (see its own header
comment), and this wrapper preserves that: the `echo $? >` runs immediately
after the `timeout ci/stability.sh 10` command line, before anything else
touches the file.

Actual wall-clock: launched 13:17:approx, completed 14:23 — **~66 minutes**
for 10 runs (close to the ~61 min estimate in the standing rules; each
individual run averaged ~6.5 min, slightly above the ~6m15s single-sweep
estimate, consistent with docker sibling container startup/teardown overhead
per run).

## Result

**`summary.log`** in this directory is the verbatim `summary.log` produced by
`ci/stability.sh` (copied byte-for-byte from the script's own `$outdir`,
`/tmp/deck-stability.SH2jgK/summary.log` — not reconstructed from the polling
transcript above).

Final tally line, quoted verbatim from `summary.log`:

```
10/10 passed
```

No rounding: this is the actual measured count. All 10 runs report
`PASS (exit 0)`; none failed, so there is no failure to name or per-run log
to copy in (the "for any run that failed" clause of the success criteria
does not apply here).

**`summary.log.exitstatus`** in this directory holds the script's own
captured exit status (from `echo $? > out.log.exitstatus` immediately after
the `ci/stability.sh 10` invocation, per the no-tee-on-the-status-line
discipline): `0`.

## Final code sha before and after the run

```
$ git rev-parse HEAD origin/main            # before launching the run
4a9d745075a4222de23ed2fd6f06d6e46a936d16
4a9d745075a4222de23ed2fd6f06d6e46a936d16

$ git log -1 --format=%H -- '*.go' '*.feature'   # before launching the run
b9243a1f415ba9ca77cc2ffa2ec557ca8b1be4cb

$ git rev-parse HEAD origin/main            # after the run completed
4a9d745075a4222de23ed2fd6f06d6e46a936d16
4a9d745075a4222de23ed2fd6f06d6e46a936d16

$ git log -1 --format=%H -- '*.go' '*.feature'   # after the run completed
b9243a1f415ba9ca77cc2ffa2ec557ca8b1be4cb
```

HEAD and the final code sha (`git log -1 --format=%H -- '*.go' '*.feature'`)
are unchanged across the run: `b9243a1f415ba9ca77cc2ffa2ec557ca8b1be4cb` is
the same final code sha task 127's whole-suite sweep and task 128's verbose
companion ran at. This gate therefore validates the same code state those
two deliverables already cover.

## Per-run breakdown (from `summary.log`)

| Run | Result |
|-----|--------|
| 1   | PASS (exit 0) |
| 2   | PASS (exit 0) |
| 3   | PASS (exit 0) |
| 4   | PASS (exit 0) |
| 5   | PASS (exit 0) |
| 6   | PASS (exit 0) |
| 7   | PASS (exit 0) |
| 8   | PASS (exit 0) |
| 9   | PASS (exit 0) |
| 10  | PASS (exit 0) |

This task ran no other work and owned its whole iteration, per the standing
rules and its own success criteria.
