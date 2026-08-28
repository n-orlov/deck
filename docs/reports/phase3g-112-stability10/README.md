# Phase 3g — task 112: `ci/stability.sh 10` at the final code commit, real rate published

**Rate: 9/10. Script exit status: 1.** Ten runs commissioned, ten run, none
re-run and none relabelled. The one failing run (run 7) failed a scenario that
is neither F2 nor F22; its root cause — an assertion with no barrier between
the `u` keystroke and the database read it checks, racing an undo the product
writes asynchronously by construction — is established from the committed log
and the code it names in §4, and not folded into either existing flake.

## 1. What was run, where, and at which sha

| | |
|---|---|
| command | `ci/stability.sh 10` — nothing else: no tag selector, no path filter, no `DECK_*` env override, no `-run` |
| per-run command inside the script | `ci/run.sh go test -p=1 -count=1 ./...`, one throwaway `--rm` sibling container per run |
| checked-out commit | `a3f45a0` (`docs: run the whole suite green at 9f61e21 and publish the log (task 111)`) |
| code sha exercised | **`9f61e21`** (`docs: make the findings report's citation check exhaustive, not sampled (task 110)`) |
| worktree | clean before the run and clean after it (`git status --short` empty both times, aside from this report's own new, untracked directory) |
| toolchain | go1.25.13 linux/amd64, tmux 3.5a (inside the sibling image `deck-ci:local`) |
| start | `1787935762` unix = `2026-08-28T16:49:22Z` |
| end | `1787939487` unix = `2026-08-28T17:51:27Z` |

`a3f45a0` is a documentation-only commit on top of `9f61e21` (task 111's own
whole-suite report), so the tree these ten runs compiled and exercised is
`9f61e21`'s:

```
$ git rev-parse HEAD
a3f45a0db2bd44b6bbcc416e844107da90c1c10e
$ git diff --stat 9f61e21 HEAD -- '*.go' '*.feature' '*.toml' '*.sh' go.mod go.sum
                        # empty: no Go, feature, theme, script or module change
$ git diff --name-only 9f61e21 HEAD
docs/reports/phase3g-111-fullsuite/README.md
docs/reports/phase3g-111-fullsuite/features-scenario-total-tail.log
docs/reports/phase3g-111-fullsuite/full-suite.log
```

This is the same code sha task 111's own single whole-suite run measured
([../phase3g-111-fullsuite/README.md](../phase3g-111-fullsuite/README.md)),
so this ten-run measurement is at the same tree that run already showed green
once.

## 2. The script's own verdict line and exit status, verbatim

The last two lines the script printed on stdout, quoted exactly as they
appeared (from [stability-summary.log](stability-summary.log), and identical
in the raw capture used to derive it):

```
full per-run logs and combined summary log kept in: /tmp/deck-stability.nivvKL
9/10 passed
```

And the script's exit status, captured immediately after the script returned,
written to a file before anything else ran:

```
$ cat /tmp/stab112/exit.txt
EXIT=1
```

`ci/stability.sh` exits `1` whenever `fail > 0` — one run failed, so exit `1`
and `9/10 passed` are two independent expressions of the same result.

Total wall clock: `1787939487 - 1787935762` = **3725 s = 62m05s** for the ten
runs, consistent with the ~60 min/10-run budget in the standing rules.

## 3. Per-run table

| run | verdict (script's own marker line) | wall (approx, from log mtimes) |
|---|---|---|
| 1 | `=== RUN 1: PASS (exit 0) ===` | 376 s |
| 2 | `=== RUN 2: PASS (exit 0) ===` | 380 s |
| 3 | `=== RUN 3: PASS (exit 0) ===` | 376 s |
| 4 | `=== RUN 4: PASS (exit 0) ===` | 377 s |
| 5 | `=== RUN 5: PASS (exit 0) ===` | 376 s |
| 6 | `=== RUN 6: PASS (exit 0) ===` | 374 s |
| 7 | `=== RUN 7: FAIL (exit 1) ===` | 372 s |
| 8 | `=== RUN 8: PASS (exit 0) ===` | 366 s |
| 9 | `=== RUN 9: PASS (exit 0) ===` | 363 s |
| 10 | `=== RUN 10: PASS (exit 0) ===` | 365 s |

Per-run wall clock is derived from consecutive `run-N.log` mtimes (the moment
the script finished writing that run's captured output), not from a
timestamped marker log — this report does not reproduce phase3f-033's
per-second stdout timestamping, since it is not required to establish the
rate or the failing run's root cause. `/proc/loadavg` was sampled every 5 s
for the whole window into [loadavg-trace.log](loadavg-trace.log) (744
samples); over the run it ranged **1.97–7.92**, mean **4.06** — a moderately
loaded host, not idle, not saturated.

Nine of the ten runs produced the same 14 `ok` / 3 `?` (`[no test files]`)
package lines (`run-1.log` … `run-6.log`, `run-8.log` … `run-10.log`, 894
bytes each — a fully green `go test -p=1 -count=1 ./...` prints one line per
package and nothing else). Run 7 is verbose and large (1,028,062 bytes)
because `go test` dumps a failing package's full captured stdout, including a
goroutine stack dump from the SIGQUIT that teardown sends to any client still
running when a scenario ends (§4 — the client was idle, not hung); it still
shows 13 `ok` lines (12 fully-green packages plus `features` itself, which
`go test` reports as `FAIL` rather than `ok`, so the count is one short of
the other nine runs' 14 by design, not by omission).

## 4. Run 7's failure: root cause

**The failing scenario, quoted verbatim** (`run-7.log:5015-5021`, ANSI colour
codes stripped for readability; the committed log keeps them):

```
--- Failed steps:

  Scenario: dd found through / tombstones an archived row, u returns it to the archived pool intact, and the reap removes it # filter.feature:107
    And the state database session "filter-dd-archived" is not tombstoned # filter.feature:130
      Error: after scenario hook failed: session "filter-dd-archived" has deleted_at=1787938118179, want not tombstoned
surviving deck client: hung deck client killed after 1s
sent (input timeline):
  [17:28:37.945] +0 "n"
```

`311 scenarios (310 passed, 1 failed)` / `3520 steps (3513 passed, 1 failed, 6
skipped)` (`run-7.log:5068-5069`) — exactly one real scenario failure in the
whole `features` package this run.

**What the scenario does** (`features/filter.feature:107-136`,
`@requirement-33-dd-reaches-and-tombstones-an-archived-row`): it starts a
client with `DECK_DELETE_GRACE_MS=200` (a real-wall-clock 200 ms delete-grace
window — `features/kill_delete_undo_test.go:257-271`), archives a session,
filters to it, presses `dd` then confirms the open dialog (this is the first
tombstone), asserts `screen contains "press u to undo"` and the DB row is
tombstoned, then **presses `u`** to undo it and asserts the row is no longer
tombstoned.

**What the captured input timeline shows**: the dialog-confirming `\r` was
sent at `17:28:38.179` and the undo keystroke `u` was sent at `17:28:38.195`
— only **16 ms** later, well inside the 200 ms grace window, so this is not a
case of the undo arriving after the window legitimately expired. The
`deleted_at` value the failing assertion reports, `1787938118179`, is exactly
`17:28:38.179` in unix milliseconds — the timestamp the *original* `dd`
confirmation wrote, unchanged. So at the instant the assertion read the row,
`u` had not yet taken effect: `deleted_at` still held the value the first
tombstone wrote, 16 ms before `u` was sent.

**Root cause: the assertion has no barrier between the keystroke and the
database read, and the undo it waits for is written asynchronously.** Four
facts in the tree at `9f61e21` compose into the race, each one checkable
without re-running anything:

1. **The keystroke step returns as soon as the byte is in the PTY buffer.**
   `clientPressesUndo` (`features/kill_delete_undo_test.go:315-325`) is
   `return client.Send("u")` and nothing else, and `Send`
   (`features/pty_driver_test.go:383-389`) records the byte and returns
   `d.terminal.Write`'s error — it does not, and cannot, wait for the client to
   read, handle or act on it.
2. **The step in between is not a barrier.** The only step separating `presses
   u` from the database read is `Then deck client "A" screen contains
   "filter-dd-archived"` (`features/filter.feature:129`), which resolves to
   `clientScreenContains` → `clientScreenContainsBefore`
   (`features/assertions_test.go:105-107,156-171`) → `WaitForFrame`
   (`features/pty_driver_test.go:470-485`). `WaitForFrame` tests the *current*
   frame first and returns immediately if the substring is already there — and
   `filter-dd-archived` was already on screen before `u` was ever sent: the
   raw capture's last renders before the failure carry the filter's own status
   line, `Filter "filter-dd-archived" in force (1 matching) — / to change, Esc
   to clear` (`grep -c 'in force (1 matching)' run-7.log` → `6`; the quotes
   appear backslash-escaped in the log because the whole raw capture is one
   `%q`-quoted Go string). So this step passes on its first poll, waiting for
   no render and proving nothing about `u`.
3. **The product's undo is asynchronous by design.** `u`'s delete-undo branch
   (`internal/tui/tui.go:2663-2672`) clears the in-model trio
   (`deleteUndoSessionID`/`Name`, bumping `deleteUndoGeneration`) and returns a
   `tea.Cmd` closure; the `restoreSvc` call that clears `deleted_at` in SQLite
   runs on bubbletea's command goroutine and reports back later as
   `sessionRestored` (`internal/tui/tui.go:1923`). Even a perfectly responsive
   client therefore commits the undo some time *after* it accepts the byte.
4. **The database probe reads once and never retries.** `sessionDeletedAt`
   (`features/kill_delete_undo_test.go:420-435`) opens the observed database,
   runs one `SELECT deleted_at FROM sessions WHERE name = ?`, and returns; the
   `is not tombstoned` assertion built on it
   (`features/kill_delete_undo_test.go:448-457`) has no poll loop and no
   deadline, unlike the screen assertions, which all poll
   (`features/pty_driver_test.go:470-485`).

**The competing `u` affordance on screen is not the explanation.** The frame
at the time carried two undo toasts at once — `Deleted — press u to undo` and
`Killed and archived — press u to unarchive (agent stays stopped)` — so it is
fair to ask whether `u` went to the archive window instead. It cannot have:
the `u` handler checks the kill trio, the batch-kill window, then the
delete-undo trio, and reaches the archive window only when all of those are
empty (`internal/tui/tui.go:2620-2700`, whose own comment pins that order),
and the immediately preceding step `Then deck client "A" screen contains
"press u to undo"` (`features/filter.feature:126`) passed, which is exactly
the render that proves `deleteUndoSessionID` was still set
(`internal/tui/tui.go:3360-3369`). So `u` took the delete-undo branch, and the
branch it took is asynchronous.

So the scenario reads the row microseconds-to-milliseconds after writing `u`
into the PTY (the input timeline above runs at a ~16 ms cadence), and what it
asserts — "the undo has landed in SQLite" — is something it never waits for.
Nine runs won that race; run 7 lost it. The failure is a missing
synchronisation in the scenario's own step definitions, not a demonstration
that the undo path is broken: the assertion as written cannot tell "the undo
did not happen" from "the undo has not happened *yet*".

**The `hung deck client` line is teardown's signature for "the client was
still alive", not evidence of a hang.** `ScenarioHarness.Close`
(`features/lifecycle_test.go:399-403`) calls `client.Stop(time.Second)` on
every client it owns, and `ScreenDriver.Stop`
(`features/pty_driver_test.go:580-613`) waits for the process to *exit*,
SIGQUITs it when it has not, and returns exactly `hung deck client killed
after 1s`. Nothing in that path ever asks the client to quit, so any client
still running at teardown produces this line. This scenario quits its client
in its own last step, `And deck client "A" exits cleanly`
(`features/filter.feature:136`) — which never ran, because the failure at
`filter.feature:130` aborted the scenario six steps early (exactly the `6
skipped` the log's own step tally reports, `run-7.log:5069`). The line is a
consequence of the failure, not its cause.

**The goroutine dump agrees: the client was idle, not stuck.** In the dump the
raw capture preserved, goroutine 1 is parked in bubbletea's `eventLoop`
`select` (`bubbletea@v1.3.10/tea.go:384`) — no `Update` in flight — and the
input reader, goroutine 15, is parked in `EpollWait`
(`cancelreader@v0.2.2/cancelreader_linux.go:134`) with nothing left to read,
meaning the `u` byte had already been drained from the PTY. No frame in the
captured portion of the dump is in `github.com/n-orlov/deck/internal` code at
all, and `Cannot restore` — the note the model renders when `restoreSvc`
returns an error (`internal/tui/tui.go:1923-1926`) — appears nowhere in
`run-7.log` (`grep -c "Cannot restore" run-7.log` → `0`).

**What this one observation does not establish**, stated so nothing later
over-reads it: the dump is truncated mid-stack after goroutine 110 (the raw
capture is bounded), so it is not a complete goroutine census; and because
teardown removed the scenario's `DECK_HOME`, there is no post-mortem read
proving the restore did commit a moment later. What is established is the
race, from the four cited code paths, and that the failing assertion cannot
distinguish it from a real defect. Giving that step a real barrier (polling
the row the way every screen assertion polls the frame) is a follow-up task,
not this measurement's business; per the standing rules the run is published
as observed and is not re-run.

**This is not F2 and not F22.** F2 is `internal/theme`'s (actually
`internal/tui`'s) `TestGoldenMinimumFrame` settle flake; F22 is
`internal/interactive`'s `TestSessionRendersAreCoalescedAgainstAKnownByteArrivalPattern`
connect-budget flake. Neither test name appears anywhere in run 7's failure
(confirmed: `grep -n "TestGoldenMinimumFrame\|ByteArrivalPattern"
docs/reports/phase3g-112-stability10/run-7.log` matches nothing), and the
failing scenario is in `features/filter.feature`, a different package from
both. This is a previously uncatalogued failure mode, named here with its own
log path (`docs/reports/phase3g-112-stability10/run-7.log`) rather than
folded into either existing finding.

**A second "Failed steps:" block in the same log is not a second failure.**
`run-7.log:5141-5148` shows another failed-steps block, from a scenario named
`error binding` in a path under
`/tmp/TestGodogRejectsUndefinedAndFailedStepsfailed.../failure.feature`. This
is `features/godog_test.go`'s own `TestGodogRejectsUndefinedAndFailedSteps`
(`features/godog_test.go:145-166`): a self-test that deliberately builds a
throwaway godog suite with one step registered to always fail, runs it, and
asserts godog's own `suite.Run()` returns nonzero — i.e. it is *designed* to
print exactly this block, in every run, and its own outer Go test always
passes (the assertion is "godog correctly rejected the failure", not "no
failure occurred"). It writes directly to stdout via godog's own formatter,
bypassing `go test`'s per-test output capture, so it is only visible when
`go test` dumps a failing package's full raw stdout (as it does here, because
`TestFeatures` failed) — the nine clean runs' 894-byte logs never show it
because `go test` discards a fully-passing package's captured stdout
entirely. `311 scenarios (310 passed, 1 failed)` at `run-7.log:5068` already
accounts for the one real failure inside `TestFeatures`; this second block
belongs to a different top-level Go test in the same package and is expected
in every run once that package's output becomes visible.

## 5. Nothing was skipped, tagged out, edited, or re-run for a better streak

- **Ten runs were commissioned and ten were run.** `stability-summary.log`
  contains exactly ten `=== RUN n ===` / `=== RUN n: PASS|FAIL ===` pairs,
  numbered 1–10, no gap, no repeat.
- **This is the only ten-run measurement taken this iteration**, and it is
  the one published, unedited, including its one failure.
- **No scenario was deleted, skipped, `@flaky`-tagged, or tag-excluded** to
  reach a higher rate. `defaultTags` is unchanged
  (`~@real-agents && ~@nightly`); the script passes no tag override.
- **No retry loop, no `sleep`, no widened timeout** was added; the tree is
  byte-identical to task 111's (§1).
- **The pass/fail label is `go test`'s own exit status**, captured by the
  script immediately after the command that produced it and never through a
  pipe (`ci/stability.sh`'s anti-`tee` invariant) — run 7's `FAIL (exit 1)` is
  the real exit status, not a mislabel.
- **Per the standing rule, this rate is published as observed and is not
  re-run to try for 10/10.** 9/10 is what happened at `9f61e21`.

## 6. Logs kept with this report

| path | what it is |
|---|---|
| [run-1.log](run-1.log) … [run-10.log](run-10.log) | each run's own `go test -p=1 -count=1 ./...` output, copied verbatim from the script's `/tmp/deck-stability.nivvKL/run-N.log`. Nine are 894 bytes (fully green, compact); run-7.log is 1,028,062 bytes (full verbose dump of the one failing package, including a runtime goroutine dump from the SIGQUIT teardown sent to the client the aborted scenario left running — §4). |
| [stability-summary.log](stability-summary.log) | the script's own combined `summary.log`: the ten `=== RUN n ===` / `=== RUN n: PASS\|FAIL ===` marker pairs interleaved with each run's package lines, ending in `9/10 passed`. |
| [loadavg-trace.log](loadavg-trace.log) | `/proc/loadavg` every 5 s for the whole 3725 s (744 samples), each line `<unix> <1min> <5min> <15min> <procs> <lastpid>`. |

The script's own scratch directory was `/tmp/deck-stability.nivvKL` (its
`run-1.log` … `run-10.log` and `summary.log`); `/tmp` does not survive the
run, which is why it is committed here instead of merely cited.

## 7. Verdict against the requirement

**The rate is 9/10, and the evidence is the directory containing this file:**
`docs/reports/phase3g-112-stability10/` — ten committed per-run logs, the
script's own [stability-summary.log](stability-summary.log) ending `9/10
passed`, script exit status `1`, at code sha `9f61e21` from commit `a3f45a0`.

This does **not** meet the phase's "green means ten consecutive times" bar
(standing rules, "Green means"). The gap is real and is reported as found: one
run in ten lost a race the scenario itself never synchronises —
`features/filter.feature`'s
`@requirement-33-dd-reaches-and-tombstones-an-archived-row` reads `deleted_at`
straight out of SQLite a few milliseconds after writing `u` into the client's
PTY, while the undo that clears it is written asynchronously on bubbletea's
command goroutine (§4's four cited code paths). It is a failure mode not
previously catalogued as F2 or F22, root-caused in §4 to a missing barrier in
the scenario's own step definitions, and left as a new finding for a future
task rather than fixed here or hidden by a re-run.

Two limits on what this measures, stated so no later report over-reads it:

- Ten runs bound a per-run failure rate loosely, not tightly. This single
  9/10 is not evidence of a 1-in-10 rate specifically — it is the one
  measurement taken, published exactly as observed.
- It measures `9f61e21`. Any later commit that touches compiled code
  (`*.go`, `*.feature`, `*.toml`, `*.sh`, `go.mod`, `go.sum`) invalidates this
  run and obliges a fresh one.
