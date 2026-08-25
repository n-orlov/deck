# Phase 3e — task 324: green whole-suite run at the final code commit

**Commit under test**: `7ebafce` (`features: fix schema-version literal and
requirement-52 auto-select fixture assumptions (324)`), working tree clean
(`git status --short` empty) at the time of the run below.

**Command**: `ci/run.sh go test -p=1 -count=1 ./...`

**Log**: [`go-test-p1-count1-all.log`](./go-test-p1-count1-all.log) (raw
stdout, `EXIT=0` appended by the wrapper).

**Result**: every package `ok` or `[no test files]`; `EXIT=0`.

**1-min loadavg** (`cat /proc/loadavg`, first field):
- start of run: `1.73`
- end of run: `2.72`

## What had to be fixed first (root-caused, not re-run for a streak)

An initial unfiltered `ci/run.sh go test -count=1 ./features/...` run at
332's tip (`8d63481`, before this task's own commit) surfaced 3 failures.
Each was individually reproduced in isolation (own `DECK_GODOG_TAGS` run,
serial, no competing sibling containers) to tell a real bug from host-load
flakiness, per the non-negotiable:

1. **`durable_identity.feature` "alpha, beta and gamma..."** (tag `@reboot`)
   and **`same_directory.feature` "two claude sessions..."** — both
   deterministic, reproduced every time in isolation. Root cause: a helper
   introduced to cope with task 301's auto-select intent
   (`selectSessionByNameThenSend`, `features/agent_steps_test.go`) sent a
   burst of navigation keys and read the terminal frame for a match
   immediately afterward, with no settle time — `ScreenDriver.Send` only
   writes to the pty and does not wait for the client to process/repaint.
   A "match" seen right after sending the burst could reflect fewer
   keystrokes than were actually sent; the still-in-flight remainder
   (typically the final down-arrow) landed *after* the resume/restart
   keypress was fired at what looked like the right row, moving the
   selection off the target a moment later. Symptom: `--resume` missing
   from the resumed session's most recent launch argv, because the keypress
   actually landed on the session created immediately after it. Fixed by
   moving the settle sleep to *before* every frame check, not only after a
   failed one. See commit `7ebafce` for the full diagnosis and the red/green
   evidence (three isolated repeats each, before and after the fix).

   Also root-caused in the same pass: `features/assertions_test.go`'s
   `TestBlackBoxAssertionsObserveRealSession` hardcoded schema version `4`,
   stale since task 330 bumped `internal/store.SchemaVersion` to 5 — updated
   to 5, along with `features/store.feature`'s three schema-version
   assertions and `features/store_feature_test.go`'s "one past what the
   binary understands" fixture literal (5 → 6). `features/kill_delete_undo.feature`'s
   "esc clears the mark" scenario and `features/themes.feature`'s
   requirement-49 chrome scenario both assumed the first-created of a pair
   of sessions stays selected; task 301's auto-select intent selects the
   *second* (most recently created) one instead — both scenarios updated to
   assert that.

2. **`mouse.feature` "DECK_MOUSE=0 disables every mouse gesture..."**
   (tag `@mouse-bindings`) — failed once, during a run where three separate
   godog suites (`@reboot`, `@mouse-bindings`, a throwaway diagnostic tag)
   were started in parallel sibling containers, all competing for CPU on
   this host. Passed 3/3 immediately afterward run in isolation with no
   competing load. The scenario's own comment already documents its
   post-gesture settle window as deliberately tight (100ms, "a mouse report
   deck does not react to produces no output at all"); under load that
   window can slip. Root cause: host load, not a product bug. No code
   change made for this one — see commit `7ebafce`'s message for the same
   note.

All fixes for (1) landed in commit `7ebafce`, which is the commit this
report's whole-suite run was taken against.

## Reproduction

```
cd /workspace
git checkout 7ebafce   # or verify HEAD is 7ebafce / a descendant with no further diff
ci/run.sh go test -p=1 -count=1 ./...
```
