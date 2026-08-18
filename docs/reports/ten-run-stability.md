# Ten-run suite stability evidence

This report captures ten consecutive, clean-state `ci/run.sh go test ./...
-count=1` invocations against the corrected tree (after the fake-agent pane
race fix described below), to demonstrate the suite is not merely
individually green but stable across repeated runs.

## Gotcha discovered while gathering this evidence

The first attempt at this ten-run sequence, run against the tree as it stood
after tasks 001-006, hit a genuine flake on run 1:
`TestFeatures/accepted_argv_and_controlled_exit_statuses_are_observable_from_panes`
failed because the success pane's `capture-pane` read ran before the
`fake-claude` fixture process had written its banner/argv output — the
scenario asserts pane content in the very next step after launching the
fixture, with no wait between "launch" and "assert output" steps. Reading
the full scrollback (the task 001 fix) made the *content* observation
race-free once output existed, but did not address the *timing* race of
reading before the fixture had produced any output at all.

Fix: `outputContains` in
[`features/fake_agent_feature_test.go`](../../features/fake_agent_feature_test.go)
now polls `capture-pane` (every 25ms, up to a 3s deadline) until the expected
banner, permission-mode line, and exact argv record are all present, instead
of asserting on a single immediate read. This mirrors the existing
`waitForPaneDeadStatus` poll used later in the same scenario for exit status,
and keeps the observation entirely through public tmux pane text with no
fixed-duration sleep.

After this fix, the sequence below was captured cleanly from the beginning
with no discarded/failed attempts folded in.

## Command

The exact loop used to produce the retained log (a POSIX `sh` script, run
from the repository root), including the pass counter and the loop-emitted
final-count line:

```sh
set -u
log=docs/reports/ten-run-stability.log
: > "$log"
pass=0
for i in $(seq 1 10); do
  echo "=== RUN $i ===" | tee -a "$log"
  if ci/run.sh go test ./... -count=1 2>&1 | tee -a "$log"; then
    echo "=== RUN $i: PASS (exit 0) ===" | tee -a "$log"
    pass=$((pass+1))
  else
    echo "=== RUN $i: FAIL (exit nonzero) ===" | tee -a "$log"
  fi
done
echo "=== FINAL COUNT: ${pass} / 10 runs passed ===" | tee -a "$log"
```

Each iteration is a fresh, clean-state invocation (`-count=1` disables Go's
test result cache; the sibling container started by `ci/run.sh` is `--rm`
and per-run). The log file is truncated (`: > "$log"`) before the loop
starts, so the retained log contains exactly and only this one sequence's
output, with no manual edits.

## Result

All 10 runs passed. Full unedited output:
[`ten-run-stability.log`](ten-run-stability.log).

Per-run pass markers (`grep -n 'RUN [0-9]*:' docs/reports/ten-run-stability.log`):

```
=== RUN 1: PASS (exit 0) ===
=== RUN 2: PASS (exit 0) ===
=== RUN 3: PASS (exit 0) ===
=== RUN 4: PASS (exit 0) ===
=== RUN 5: PASS (exit 0) ===
=== RUN 6: PASS (exit 0) ===
=== RUN 7: PASS (exit 0) ===
=== RUN 8: PASS (exit 0) ===
=== RUN 9: PASS (exit 0) ===
=== RUN 10: PASS (exit 0) ===
```

The loop itself emits the final count as its last line of output (not just
asserted in this prose); it is the last line of the retained log:

```
=== FINAL COUNT: 10 / 10 runs passed ===
```

Final successful-run count, per that loop-emitted line: **10 / 10**. No run
failed in this sequence, so the loop never printed a `FAIL` line and never
needed a restart mid-sequence.

### Total sequence attempts, truthfully

Counting every full ten-run (or attempted ten-run) sequence run while
gathering Phase 0 stability evidence, including the earlier failed one:

1. **Attempt 1 (failed):** run against the tree before the fake-agent
   timing fix described in the Gotcha section above. Run 1 of that sequence
   hit the `outputContains`-before-fixture-wrote-output flake and the
   sequence was abandoned/discarded after diagnosis; no log from that
   attempt is retained.
2. **Attempt 2 (this one, successful):** run fresh from scratch, from the
   beginning (run 1 through run 10), after the fix landed, using the exact
   loop shown above. All 10 runs passed on the first attempt with no
   restarts. This is the sequence retained in
   [`ten-run-stability.log`](ten-run-stability.log).

Total: **2 sequence attempts** (1 failed + 1 successful), and the log
linked here is exclusively the successful attempt 2, gathered cleanly from
its own run 1 with no discarded partial runs spliced in.
