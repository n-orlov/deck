# R79 evidence — tombstone sweep at store open (tasks 010, 011)

These four logs were captured at implementation time by tasks 010 and 011, in the
run's own artifacts directory rather than under `docs/reports/`. Copied here verbatim
(byte-identical, not regenerated) by task 038 so the phase report's evidence paths
resolve inside the repository a clone of it actually has.

- [`task010-frozen-clock-mutation-red.log`](task010-frozen-clock-mutation-red.log) —
  `TestSweepTombstonesHonoursFrozenClock`, red with `now` swapped back for
  `time.Now()` (task 010's own commit message: "Confirmed by mutation: with `now`
  swapped for time.Now() both subtests fail").
- [`task010-bounded-batch-mutation-red.log`](task010-bounded-batch-mutation-red.log) —
  `TestSweepTombstonesReapsOneBoundedBatchPerCall`, red with the synchronous
  multi-batch loop restored, green after with the one-bounded-batch-per-call fix; both
  halves are in the one file.
- [`task011-callsite-removed-mutation-red.log`](task011-callsite-removed-mutation-red.log) —
  `TestAbandonedDDIsReapedAtNextStoreOpen`, red with task 010's `SweepTombstones` call
  site removed from `cmd/deck/main.go`: "row still exists after run 2's store open,
  want the abandoned tombstone reaped by task 010's call site".
- [`task011-green-store-service-cmd-deck.log`](task011-green-store-service-cmd-deck.log) —
  the same three packages green with every call site restored.
