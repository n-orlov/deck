# Task 303 — synchronising `TestSigwinchCountDistinguishesTwoFromThree`'s inter-resize pacing

## Which of task 302's failures this covers

Task 302's README (`docs/reports/phase3g-302-stability10/README.md`) named
`TestSigwinchCountDistinguishesTwoFromThree` (`features/sigwinch_count_test.go:80`) as run 2's
failure at code head `d0e36e4`: `sigwinch count after 2 resizes = 1, want exactly 2`. This is
the second of the five failures task 302 named (`TestDeckBinaryEmptyHelpAndQuitThroughPTY`, run
10's failure, was fixed and committed first — `6524ece`, `docs/reports/phase3g-303-help-pty-tail-sync/`).

## Root cause

The test drives two real kernel resizes (`TIOCSWINSZ` via `driver.Resize`) 50ms apart, then
waits up to 2s for the fixture's SIGWINCH counter to read 2. The fixture
(`cmd/fake-claude/main.go`'s `startSizeRecorder`) receives SIGWINCH on a channel of capacity 1
(`signals := make(chan os.Signal, 1)`), and a standard Unix signal is not queued by the kernel
either — if a second SIGWINCH is raised before the *runtime* has forwarded the first one into
that channel (or before the fixture's goroutine has drained it), the second occurrence is
coalesced away and the process only ever observes one.

The test's fixed 50ms sleep between the two resizes assumed that window would always have
closed by then. Under enough scheduling contention — the same class `ci/stability.sh`'s later
iterations experience — it sometimes has not: the *second* resize's SIGWINCH is the one that
vanishes, and the count sticks at 1. Once the underlying signal is genuinely dropped, no amount
of waiting *afterward* (the existing bounded `waitForSigwinchCount(path, 2)`, 2s deadline)
recovers it, because the event that would produce a 2 never happened at the fixture at all —
this is not a slow-arrival race that a longer wait window closes, it is a lost-event race that
only *not letting it happen* closes.

## Fix

Replace the blind `resize(81,24); sleep(50ms); resize(82,25); sleep(50ms)` pacing with a
synchronising wait on the *observable state* between the two resizes: wait for the count to
actually reach 1 (`waitForSigwinchCount(countPath, 1)`, the same helper the test already uses
elsewhere, bounded at 2s) before raising the second resize. The second resize now never fires
until the first SIGWINCH is proven to have already been drained and recorded by the fixture,
closing the coalescing window instead of merely hoping 50ms was enough. The final (third) resize
and its own wait are unchanged — nothing raises a signal after it while its wait is pending, so
that pairing was never the race task 302 observed.

No assertion was weakened, removed, skipped or tagged out: `waitForSigwinchCount(path, 2)` after
the second resize is exactly as strict as before (`got != 2` still fails the test), and the new
intermediate check (`got != 1`) is an *added* assertion, not a substitute for a removed one.
`features/godog_test.go` is untouched by this task:

```
$ git diff 6524ece..HEAD -- features/godog_test.go | wc -l
0
```

Diff: `features/sigwinch_count_test.go` only — the two-resize pacing and its explanatory
comment. No product code changed.

## Reproduction (red before)

`red-before-run-158.log` in this directory: a `git worktree add --detach .scratch-303-sigwinch
6524ece` checkout of the **unmodified pre-fix** `sigwinch_count_test.go` (never edited for this
reproduction — same route as task 302, just re-run). Unlike the first task-303 fix, contention
generated *outside* the sibling container (background `yes` loops in this job container, scaled
from 80 up to ~730 processes, over ~130 standalone attempts) did **not** reproduce the failure —
sibling containers apparently do not compete for the host's CPU with equal footing against
noise raised outside their own cgroup. Contention raised **inside the same sibling container**
as the test (`ci/run.sh sh -c 'cd .scratch-303-sigwinch && for i in $(seq 1 80); do ( yes >
/dev/null & ); done; sleep 1; for i in $(seq 1 200); do go test ...; done'`) reproduced it at
attempt 158, with the exact failure signature from task 302's README:

```
sigwinch_count_test.go:80: sigwinch count after 2 resizes = 1, want exactly 2
```

The `.scratch-303-sigwinch` worktree was removed (`git worktree remove --force`) before this
commit; it never became part of the tracked tree.

## Green after (fix applied)

Three consecutive **standalone** runs of the fixed test, each its own `ci/run.sh` invocation
from a clean `go test` cache-free state (`-count=1`), each log naming its exact command and exit
status:

- `green-after-run-1.log` — `ci/run.sh go test ./features/ -run
  TestSigwinchCountDistinguishesTwoFromThree -count=1 -v` → exit status 0
- `green-after-run-2.log` — same command → exit status 0
- `green-after-run-3.log` — same command → exit status 0

Additionally (not one of the three standalone runs above, extra confidence only): the **fixed**
test was re-run 60 times back-to-back under the identical in-container 80-`yes`-loop contention
that reproduced the red run above — all 60 passed, 0 failed (`pass=60 fail=0`, `sweep-*.log`
kept only inside the ephemeral sibling container, not copied out; the driver's own summary line
is the record).

## Scope

Only `features/sigwinch_count_test.go` changed. No product code, no
`features/godog_test.go`, no tag, `t.Skip`, or weakened/removed assertion. This report does not
close task 303 — three named failures remain: `attention_sort.feature:92`, `crash.feature:46`,
`attach_scroll.feature:11` (see `docs/reports/phase3g-302-stability10/README.md`'s "Relation to
the superseded first measurement" section).
