# Task 303 — synchronising `TestDeckBinaryEmptyHelpAndQuitThroughPTY`

## Which of task 302's failures this covers

Task 302's README (`docs/reports/phase3g-302-stability10/README.md`) named two Go-test
failures at code head `d0e36e4` (plus three superseded ones from an earlier, invalidated
measurement at the same tree). This report covers **run 10's failure**:
`TestDeckBinaryEmptyHelpAndQuitThroughPTY` (`cmd/deck/main_test.go:500`), "released help
missing ... through the real PTY", with the captured `help` string truncated mid-word.

## Root cause

`helpView()`'s alt-screen frame at this test's PTY size (260×100) is ~46.9KB of ANSI
output — far bigger than a single PTY read (individual reads observed at 1–12KB
throughout a run). `io.Copy` into the test's `ptyOutput` buffer therefore lands the frame
across many separate reads spread over real wall-clock time, not atomically.

The test opened help with `?`, then called `waitForScreen(t, output, done,
"DECK_TMUX_SOCKET")` before capturing `help := output.String()`. `"DECK_TMUX_SOCKET"`
appears in the *Runtime controls* section, roughly 5KB before the end of the ~46.9KB
frame — well before the *Mouse* section and the overlay's closing line. Once that
substring landed, the very next statement captured the buffer immediately, with no wait
for the remaining tail. Under enough scheduling contention, the reader goroutine's next
read (carrying that tail) lags behind the 5ms poll tick that notices `"DECK_TMUX_SOCKET"`,
and the capture races the still-in-flight write — truncating exactly the assertions this
test makes about the *Mouse* section (`"copies it into deck's own tmux buffer"`, `"system
clipboard via OSC 52"`, `"override modifier (usually shift)"`).

## Fix

Wait for the help overlay's own last rendered line instead of a substring from its
middle: `helpView()` (`internal/tui/tui.go`) ends with the literal line `"? closes help;
Esc closes help; q quits deck."`. Waiting for `"q quits deck."` before capturing
`output.String()` synchronises the capture on the *whole* frame having arrived, not
merely started — a bounded wait on the observable state (the frame's own terminator),
not a fixed sleep or a raced assertion. `TestDeckBinaryEmptyHelpAndQuitThroughPTY`'s
`?`-then-`?`-then-`Esc` sequence right after this wait is otherwise unchanged, and no
assertion, tag or skip was added, removed or weakened.

Diff: `cmd/deck/main_test.go`, one changed `waitForScreen` call plus an explanatory
comment. `features/godog_test.go` is untouched by this task
(`git diff <302-head>..HEAD -- features/godog_test.go` is empty).

## Reproduction (red before)

`red-before-run-33.log` in this directory: a `git worktree add --detach .scratch-303
<302-head-sha>` checkout of the **unmodified pre-fix** `main_test.go` (never edited for
this reproduction — same route as task 302, just re-run), driven under 80 background
CPU-contention loops (`yes > /dev/null &`) to widen the same scheduling window a
`ci/stability.sh` run's later iterations experience (task 302 run 10 hit this after nine
prior full-suite runs). `TestDeckBinaryEmptyHelpAndQuitThroughPTY` was re-run in a loop
under that load; **attempt 33 failed** with the exact truncation signature from task
302's README (`help` cut off mid-word inside the *Mouse* section, at `"...selects text;
releas"`, missing `"override modifier (usually shift)"` among others). The log records
the exact command, the load-generation wrapper, and the failing attempt's full `go test
-v` output with its non-zero exit status.

## Green after (fix applied)

Three consecutive **standalone** runs of the fixed test, each its own `ci/run.sh`
invocation from a clean `go test` cache-free state (`-count=1`), each log naming its exact
command and exit status:

- `green-after-run-1.log` — `ci/run.sh go test ./cmd/deck/ -run
  TestDeckBinaryEmptyHelpAndQuitThroughPTY -count=1 -v` → exit status 0
- `green-after-run-2.log` — same command → exit status 0
- `green-after-run-3.log` — same command → exit status 0

Additionally (not one of the three standalone runs above, extra confidence only): the
**fixed** code was re-run 60 times back-to-back under the same 80-background-`yes`-loop
contention that reproduced the red run — all 60 passed (not individually logged; this
paragraph is the record of that sweep).

## Scope

Only `cmd/deck/main_test.go` changed. No product code, no `features/godog_test.go`, no
tag, `t.Skip`, or weakened/removed assertion. The `.scratch-303` worktree used for the red
reproduction was removed (`git worktree remove --force`) before this commit; it never
became part of the tracked tree.
