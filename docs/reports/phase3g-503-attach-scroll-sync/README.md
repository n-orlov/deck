# Task 503 — `attach_scroll.feature:11`'s post-gesture frame race

## Mechanism (cited by file:line)

The scenario at `features/attach_scroll.feature:11` captures a baseline frame
(`clientCapturesFrameAs`, `features/mouse_synthesis_test.go:173`), performs a
wheel-scroll burst and cancels copy-mode
(`clientExitsCopyModeOnAttachedPane`, `features/attach_scroll_test.go:276`
pre-fix), then asserts the frame is byte-identical to the baseline
(`clientFrameStillMatchesCaptured`, `features/mouse_synthesis_test.go:239`).
That comparison includes the tmux status line, and the status line's window
name field is where the race lives.

tmux's `window_name` — the value the status line actually renders — is a
**cached** field. It is populated once by the window's default naming and is
only *refreshed* by tmux's own `automatic-rename` option (task 115) on
internal events such as a job/process-table change; it is not recomputed
merely because a client typed a command or left copy-mode. `#{pane_current_
command}`, by contrast, is read live from `/proc` at query time.

This task's own diagnostic proved the gap directly (temporary
`fmt.Fprintf(os.Stderr, ...)` instrumentation in
`waitForAutomaticRenameToRender`, removed before this commit): immediately
after cancelling copy-mode via `client.Send("q")` and confirming
`WaitForFrameGone` for the scrollback marker
(`features/attach_scroll_test.go:291` in the pre-fix tree), a direct
query showed

```
pane_current_command = "sh"
window_name           = "[tmux]"   (stable, unchanged for a full 300ms poll)
```

`"[tmux]"` is not a transient value still "in flight" — it had already
stopped changing. Nothing in tmux was scheduled to re-run automatic-rename
just because copy-mode ended, so the stale cached name would sit there
indefinitely, until some *unrelated* later tmux command happened to trigger
a re-check as a side effect. Before this fix, that unrelated trigger was the
scenario's own gesture: the wheel-scroll burst (or, in the "before" baseline
half of the race, the client's own SGR mouse reports) was frequently the
first thing to force tmux to notice the pane's real foreground command and
flush a fresh status line to the client's pty — sometimes before the
baseline capture, sometimes after, depending on scheduling. That is exactly
why the failure was a status-line-only diff between two frames that were
otherwise byte-identical, and why it needed load to reproduce reliably (see
"Red before" below): under load, tmux's own scheduling of that unrelated
event slides relative to the scenario's fixed step sequence.

Two independent transitions were racing this way, both fixed by the same
primitive (`waitForAutomaticRenameToRender`, `features/attach_scroll_test.go
:181-198`):

1. **Before the baseline** (`clientFillsAttachedPaneWithScrollback`,
   `features/attach_scroll_test.go:123-151`): a just-attached window's name
   can still be a transient value from session creation (observed as literal
   `"tmux"` in the narrowed reproduction —
   `docs/reports/phase3g-503-attach-scroll-sync/red-before.log` — and as
   `"env"` in the earlier whole-feature capture,
   `docs/reports/phase3g-503-attach-scroll-sync/red-before-whole-feature-first-capture.log`)
   when the shell has already taken over the pane.
2. **After cancelling copy-mode, before the final compare**
   (`clientExitsCopyModeOnAttachedPane`, `features/attach_scroll_test.go
   :276-311`): the diagnostic case above — `window_name` stuck on the
   copy-mode-only placeholder `"[tmux]"` while `pane_current_command`
   already read `"sh"`.

## The fix

`waitForAutomaticRenameToRender` (`features/attach_scroll_test.go:181-198`)
sidesteps automatic-rename's own timing entirely instead of trying to wait
it out: it reads the pane's live, authoritative `#{pane_current_command}`
(`paneCurrentCommandAndWindow`, `features/attach_scroll_test.go:207-217`,
via `tmux list-panes -F "#{pane_current_command}|#{window_id}"`) and
explicitly renames the window to match it
(`tmux rename-window -t <window_id> <command>`). Explicitly changing a
window's name is not a lazily-deferred operation the way automatic-rename's
own internal recompute is — like any other command that mutates window
state, tmux pushes the resulting status-line redraw to the attached
client's pty as part of handling the command, synchronously. The function
then blocks on `client.WaitForFrameFunc` until that redraw is actually
observable in the client's own rendered frame before returning, so the
caller never proceeds on the strength of the *command having been issued*
alone — only on the strength of the *client having rendered its effect*.

It is called at both race points identified above: once at the end of
`clientFillsAttachedPaneWithScrollback` (before the baseline is captured)
and once at the end of `clientExitsCopyModeOnAttachedPane` (before the
final comparison). Both sides of the compared pair are therefore
deterministically pinned to the pane's real foreground command, never to
whichever side tmux's own automatic-rename hook happened to have serviced
first.

## Why this is NOT "wait longer"

`docs/reports/phase3e-406-mouse-off-frame-race/README.md` already tried
widening a sleep for a status-line-adjacent race in this same scenario file
and reports it made the suite 5/5 **red** — waiting longer inside the
client's own pty stream cannot force a pending, unrelated redraw to become
observable there, because nothing schedules one on a timer.

This task's own first attempt repeated that same mistake in a different
guise and failed the same way: an earlier revision of this fix polled
`window_name` on the real tmux server (not the harness's pty emulator) and
waited for it to go quiet for 300ms before trusting it as "settled" — still
a wait, just relocated to ground truth instead of to the rendered frame.
That revision passed 3/3 unloaded but failed **11/20**, then (after
widening the quiet window further) **20/20**, under load — because
`window_name` reaching a stable value said nothing about whether it was the
*final* value; per the mechanism above, it can sit on a stale, incorrect
value indefinitely since nothing is scheduled to re-check it. No timeout or
quiet-window length fixes that, because there is nothing pending to time
out on.

The landed fix never waits for tmux to decide it is time to rename or
redraw. It reads the one live field that is never stale
(`pane_current_command`) and performs the rename itself, synchronously,
at the exact two points the comparison needs it pinned — synchronising the
assertion on observable state (the client's own rendered frame,
confirmed via `WaitForFrameFunc`), exactly the fix class the standing rules
require and task 406 shows a sleep cannot deliver.

## Red before

`docs/reports/phase3g-503-attach-scroll-sync/red-before.log` is the
authoritative red-before capture. It is **narrowed to this one scenario
before any load is applied**, and every run in it carries the exact command
it ran and its own exit status, recorded in the same shell invocation as the
run (`/tmp/red503.sh`'s loop appends `$ <command>` before each run and
`exit status: $?` immediately after it).

The route is unmodified: a detached `git worktree` at `3f23287` — the parent
of this task's fix commit `9f7c239` — never a hand-edited tree. The exact
command, identical for every run in the log:

```
$ ci/run.sh sh -c 'cd .scratch-503b && env DECK_GODOG_PATHS=attach_scroll.feature go test ./features/ -run "TestFeatures/a_wheel_notch_scrolls_an_attached_pane.s_scrollback_and_leaves_the_shell.s_input_line_untouched" -count=1 -v'
```

The `-run` selector pins the run to `attach_scroll.feature:11` alone: the
feature file's sibling requirement-49 scenario reports `undefined` and never
executes, so each run's verdict is this scenario's verdict and nothing else
(`2 scenarios (1 failed, 1 undefined)` in the failing run's godog summary).

Sequence in the log:

- **narrowed, no load** — `exit status: 0` (log line 38). Narrowing is
  established as green *before* load is introduced, so the failure that
  follows is attributable to the race under load, not to the narrowing.
- **load applied** — 24 background `yes > /dev/null` processes, PIDs captured
  at spawn and later killed by PID; `/proc/loadavg` recorded in the log at
  `10.85` when load started and `13.72` when it stopped (log lines 41, 746).
- **under load, run 1** — `exit status: 0` (line 76)
- **under load, run 2** — `exit status: 0` (line 111)
- **under load, run 3** — **`exit status: 1`** (line 743): `after scenario
  hook failed: client "A" frame changed after gesture, want unchanged from
  captured "before-wheel-scroll"`, failing at
  `Then deck client "A" frame still matches the captured "before-wheel-scroll" frame`
  (`mouse_synthesis_test.go:122 -> clientFrameStillMatchesCaptured`), with
  `--- FAIL: TestFeatures/a_wheel_notch_scrolls_an_attached_pane's_scrollback_and_leaves_the_shell's_input_line_untouched`.
  The whole-frame diff is isolated to one field of the status line:
  `want` renders `[deck_atta0:tmux*  ... ]`, `got` renders
  `[deck_atta0:sh*  ... ]` — the baseline was captured while `window_name`
  still held the stale session-creation value `"tmux"`, and a later,
  unrelated tmux event flushed the real value `"sh"` to the client between
  the two frames. That is transition 1 of the mechanism above, verbatim.

The loop stops at the first failure by design, so the log ends there; the
reproduction rate on this attempt was 1 in 3 loaded runs (a prior sweep of
the same narrowed command on the *fixed* tree under the same load was 30/30
green, recorded under "Green after" below).

An earlier, weaker capture is kept for the record at
`docs/reports/phase3g-503-attach-scroll-sync/red-before-whole-feature-first-capture.log`:
it runs the whole feature file rather than this scenario alone (its summary
reads `2 scenarios (1 passed, 1 failed)`) and its command and exit statuses
live in this README instead of in the log itself. It reproduces the same
failure — there with the stale name `"env"` instead of `"tmux"` — under 24×
`yes` load on the same unmodified `3f23287` route, via
`ci/run.sh env DECK_GODOG_PATHS=attach_scroll.feature go test ./features/ -run TestFeatures -count=1`
(run 1 exit 0, run 2 exit 1). It is superseded by, not part of, the
narrowed evidence above.

## Green after

Both scenarios in the feature file, unloaded, exit 0:

```
$ ci/run.sh env DECK_GODOG_PATHS=attach_scroll.feature go test ./features/ -run TestFeatures -count=1
ok  	github.com/n-orlov/deck/features	4.9s
```

Required consecutive-green evidence for this exact command is captured in
`docs/reports/phase3g-503-attach-scroll-sync/green-after.log` (5/5, each
run's exit status captured in the same shell call as the run).

Additionally verified (not part of the pass/fail criterion, background
robustness checks only):
- The narrowed single-scenario command, under the same ~load-27–29 24×`yes`
  load used to reproduce the red-before failure: 30/30 green.
- `go vet ./features/` clean.
- The sibling requirement-49 scenario in the same file (`scrolling an
  attached pane never flips its badge...`), which also calls
  `clientExitsCopyModeOnAttachedPane`, is unaffected (still passes).
- A pre-existing, unrelated flake in `mouse.feature`'s single-click scenario
  was observed once under this same heavy load; reproduced identically on
  the unmodified tree, confirming it predates and is unrelated to this
  change (out of scope for task 503).

## Scope

- No change to `features/godog_test.go`'s `defaultTags`.
- No `t.Skip` added anywhere in the diff.
- The scenario keeps its `@requirement-48-wheel-scrolls-attached-pane-
  without-typing` tag and its whole-frame comparison
  (`clientFrameStillMatchesCaptured`) unchanged and un-weakened; the fix is
  entirely in synchronising the two frames being compared, not in the
  comparison itself.
