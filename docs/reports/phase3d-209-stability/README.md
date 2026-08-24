# I-20 stability measurement (task 209)

`ci/stability.sh 10` run from a clean checkout at `c4baf77` (docs-only commit on top of `9575534`,
task 208's declared final code commit). Command:

```
nohup ci/stability.sh 10 > /tmp/phase3d-209/stability.log 2>&1 &
```

A companion loop logged `date -u` + `uptime` every 60s to `loadtrace.log` for the run's duration.

## Result: 2/10 passed

```
=== RUN 1: PASS (exit 0) ===
=== RUN 2: PASS (exit 0) ===
=== RUN 3: FAIL (exit 1) ===
=== RUN 4: FAIL (exit 1) ===
=== RUN 5: FAIL (exit 1) ===
=== RUN 6: FAIL (exit 1) ===
=== RUN 7: FAIL (exit 1) ===
=== RUN 8: FAIL (exit 1) ===
=== RUN 9: FAIL (exit 1) ===
=== RUN 10: FAIL (exit 1) ===
2/10 passed
```

Wall time: started 19:26Z, finished 20:29Z (2026-08-24), ~63 minutes total. 1-minute loadavg
climbed steadily through the run, from ~5.9 (run 1) to a peak of ~11.6 (during run 6/7) settling
around 8.4-10.5 for the last several runs (see `loadtrace.log`; 28-core host). This is markedly
higher load than earlier isolated-tag measurements in this approach (loadavg 3.5-7.7), and higher
than the 4/10 baseline measured before task 208's repair pass.

Reported as measured, per this task's own criteria: no re-run, no exclusion, no tag change.
`features/godog_test.go`'s `defaultTags = "~@real-agents && ~@nightly"` is unchanged (verify with
`grep defaultTags features/godog_test.go`).

## Failure taxonomy observed across the 8 failing runs (raw grep, not yet root-caused — that is
## task 210's job, not this one)

- `TestFeatures/R_restarts_a_claude_session,_recording_environment_key_names_in_the_launch_audit_but_never_a_value`
  failed in 6/10 runs (3, 6, 7, 8, 9, 10). This is the SAME scenario flagged as a single,
  not-yet-investigated observation during task 206's validation (206's notes: "an UNRELATED
  failure ... audit log has 1 launch record, want 2 ... not investigated this iteration"). It did
  not recur in 208's full-suite run, but recurs heavily here.
- `TestFeatures/a_unique_directory_match_ghosts_in_the_dimmed_token_and_right_accepts_it_with_a_trailing_slash`
  failed in 2/10 runs (5, 7).
- `TestGoldenMinimumFrame` (subtests `run-1` and/or `run-2`, "frame kept changing after the
  fixture rendered; not settled") failed in 3/10 runs (4, 9, 10).
- `TestPanePipeReceivesGenuineEOFOnDisplacementWithPanePipeStillOne` (`internal/tmux`) failed in
  1/10 runs (5) — a test/package not touched by tasks 201-207.
- Run 3's `TestFeatures` failure is the audit-count scenario only (single subtest).

None of these match the scenarios task 201-206 targeted (`@requirement-48-refuse-preview-...`,
`attach_scroll`'s copy-mode hang) or task 207's pipe-race test by exact assertion text, though the
`internal/tmux` failure in run 5 is in the same test area as 207 and warrants comparison against
207's fix during root-causing.

Per-run logs: `run-1.log` .. `run-10.log`. Combined: `summary.log`. Load trace: `loadtrace.log`.
Raw log directory on the worker host (not committed, superseded by the copies here):
`/tmp/deck-stability.mSbdkK`.

This measurement is task 209's complete, honest deliverable. Root-causing and fixing (or
documenting non-reproducibility) belongs to task 210 next.
