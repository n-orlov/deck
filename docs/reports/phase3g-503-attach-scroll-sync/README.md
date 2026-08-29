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
   can still be a transient value from session creation (observed both as
   literal `"tmux"` and, in a later capture under heavier load, `"env"` —
   see `docs/reports/phase3g-503-attach-scroll-sync/red-before.log`) when
   the shell has already taken over the pane.
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

Reproduced against the unmodified pre-fix tree (`git stash` back to the
tree as of `3f23287`), with 24 background `yes >/dev/null` processes
raising load average to ~20–28, running the exact deliverable command:

```
$ ci/run.sh env DECK_GODOG_PATHS=attach_scroll.feature go test ./features/ -run TestFeatures -count=1
```

- run 1: exit 0
- run 2: **exit 1** — `client "A" frame changed after gesture, want unchanged from captured
  "before-wheel-scroll"`, diff isolated to the status line's window-name
  field: `want` shows `[deck_atta0:env*  ...]`, `got` shows
  `[deck_atta0:sh*  ...]` — i.e. the baseline was captured while `window_name`
  still reported a stale automatic-rename value (`"env"`, one more example of
  the same transient-value class as `"tmux"`), and something later forced
  tmux to flush the real value (`"sh"`).

Full captured output (trimmed of the harness's own unrelated SIGQUIT
goroutine dump, noted inline) is in
`docs/reports/phase3g-503-attach-scroll-sync/red-before.log`.

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
