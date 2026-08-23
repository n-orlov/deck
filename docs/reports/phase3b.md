# Phase 3b (Part II — interactive preview) report

This report is assembled incrementally as Part II's tasks land; task 072 is
responsible for its final completeness pass (SIGWINCH counts, the restore
recipe, the seed byte sequence, resident-memory cost, the
DECK_INTERACTIVE_TRANSPORT contract statement, and every spike departure).
Each section below is added by the task that measured it, in the same
commit.

## II-7/II-8: entry geometry capture and the chrome-compensated window fit
## (task 034)

`internal/tmux/geometry.go`'s `CaptureWindowGeometry` reads
`#{window_width}x#{window_height}` and the **window-local** `window-size`
value via `show-options -wv` before interactive mode's entry touches
anything (PRD II-7). `window-size` is a builtin option: task 028 already
established that an unset builtin window option prints an empty line and
exits 0 (unlike a "@"-prefixed user option's "invalid option" error, which
`internal/tmux/ownership.go`'s `OwnershipOption` hits instead) — both are
treated as "none", per the PRD's own instruction.

`FitWindowToPane` resizes the **window**, never the pane — `resize-pane` is
never called on this path; `internal/tmux/geometry_test.go`'s
`TestFitWindowToPaneNeverCallsResizePane` greps `geometry.go`'s own source
for the string and fails the build if it ever appears there, so this is not
a one-off manual check that bit-rots.

### Why a loop, not one shot

On a single-pane window `#{pane_height} == #{window_height}` (chrome is
zero) and the first `resize-window` call lands the target exactly —
measured: `TestFitWindowToPaneOnASinglePaneWindowHasZeroChrome`, 1 resize.

On a **split** window, tmux's layout engine redistributes space between
sibling panes **proportionally** on every resize: a sibling pane created
with a fixed `-l 10` (10 rows) does **not** stay at 10 rows as the window
is resized — measured directly against real tmux (not a fixed-chrome
assumption):

| resize-window call | window height | main pane height | sibling pane height |
|---|---|---|---|
| (initial) | 42 | 31 | 10 |
| 1 | 33 | 26 | 6 |
| 2 | 29 | 24 | 4 |
| 3 | 27 | 23 | 3 |
| 4 | 26 | 22 | 3 |

So a single `chrome := window - pane` measurement taken once and added to
the wanted pane height gives the window size that would have produced the
wanted pane height **at the old proportions**, not the new ones once the
window has actually changed size. `FitWindowToPane` re-measures chrome
fresh every iteration instead, which is what converges: it lands a 22-row
main pane inside what started as a 42-row window (a 10-row sibling split)
in **4 `resize-window` calls**, exactly matching the table above.

### The PRD's own numbers vs. what this repository could reproduce

The PRD (`prds/phase3b-interactive-preview.md`, requirement 8) cites, from
the unavailable raw spike evidence (`tasks.json`'s
`discovered.prdCorrections`: `~/deck-spikes/` does not exist in this
container), **12 iterations stuck at `pane_height` 11** for a naive
pane-targeting loop, and **5-6 resizes** for the chrome-compensated one
landing a 22-row pane in a 42-row window. Measured here, on the closest
reproducible layout this repository can build (a 42-row window, one
`split-window -v -l 10` sibling, same 22-row target):

- chrome-compensated (`FitWindowToPane`): **4 resizes**, landing exactly
  80x22 (`internal/tmux/geometry_test.go`'s
  `TestFitWindowToPaneConvergesOnASplitWindow`,
  `features/interactive_geometry.feature`'s first scenario).
- naive (requests the wanted PANE height as the window height directly,
  never compensating for chrome): stuck at `pane_height` **20**, never
  22, for the full 15-attempt bound
  (`TestNaivePaneTargetingLoopNeverConverges`,
  `features/interactive_geometry.feature`'s second scenario).

Both numbers are close to, but not identical to, the PRD's cited figures.
This is attributed to a different tmux version and/or split layout than
the unreachable spike used, not to a different algorithm — the *shape* the
PRD names (chrome-compensated converges in a small bounded number of
resizes; naive pane-targeting plateaus below target and never converges)
reproduces exactly. Both are measured directly against real tmux via
`ci/run.sh go test ./internal/tmux/...` and
`ci/run.sh go test ./features/... -run TestFeatures` (scenario tags
`interactive_geometry.feature`), not asserted from the PRD's text alone.

### The choice to target the window, not the pane

Targeting the window is deliberate, not incidental: a pane's size is a
function of its window's size and its siblings' layout, not an
independently settable property from outside tmux's own layout engine (the
only pane-level resize command tmux exposes, `resize-pane`, works by
asking the layout engine to shrink or grow ONE pane and redistribute the
rest — which is exactly the "naive" alternative this task's negative
control demonstrates converges to the wrong value or not at all on a split
window). `resize-window`, by contrast, is the one operation whose target is
the quantity `window-size manual` actually pins (task 032/II-9's exit
restore depends on this same fact), and it composes correctly with the
chrome-compensation loop because window height IS the quantity the loop
solves for.

## II-9: exit restores in the load-bearing order (task 035)

`internal/tmux/geometry.go`'s `RestoreWindowGeometry` implements the PRD's
exit recipe exactly: `resize-window` back to the saved dimensions **only
when** `#{session_attached} == 0` (new `Client.SessionAttachedCount`),
**then** `set-option -w -u window-size`, unconditionally, last.

### The reversed order, demonstrated red

`internal/tmux/restore_test.go`'s `TestReversedRestoreOrderLeavesWindowPinned`
issues the two steps in the wrong order directly (never through
`RestoreWindowGeometry`, which never produces this order): unset first,
`resize-window` second. Measured against real tmux:

- after the reversed sequence, `window-size` reads `manual` again, not
  unset — `resize-window` always writes that as a side effect regardless of
  what the preceding unset just did.
- a client then attaching at a **third** size is not followed at all: the
  window stays exactly at whatever the reversed sequence's `resize-window`
  last set, proving "pinned" is a real, observed behaviour, not merely
  option state nobody reads.
- run again with the **correct** order (same test, second half), the same
  client-attach step instead lands the window at the attached client's own
  size, on the unset alone — the contrast is captured in one test so the
  before/after is unambiguous.

### With a client attached: the unset alone, no third SIGWINCH

`TestRestoreWindowGeometryAttachedUnsetFollowsClientWithNoThirdSigwinch`
runs a pane trapping real `SIGWINCH` into an append-only counter file (a
kernel signal count, not a size-log inference) through one full
enter/exit-with-a-client-attached cycle:

| step | SIGWINCH count |
|---|---|
| after entering (one `resize-window` call, single-pane window, zero chrome) | 1 |
| after a client attaches at a third size (`window-size` still `manual`, so the attach itself must not move the window) | 1 (unchanged) |
| after `RestoreWindowGeometry` with that client attached (resize-window skipped, unset only) | 2 |

Measured here: exactly **2**, never 3 — the unset's own automatic
"follow the latest attached client" behaviour (`window-size latest` at the
server-global scope, set once by `Client.Bootstrap`/task 032) does the
resize implicitly; `RestoreWindowGeometry` never issues an explicit
`resize-window` while a client is attached, so there is no third,
wasted `SIGWINCH`.

One incidental finding recorded here rather than left as a silent -1 in
the test: the pane the attached client sees is one row shorter than that
client's own raw terminal (e.g. a 90x30 client yields a 90x29 pane) —
tmux's default status line (one row, on by default) is subtracted from
the client's own screen before `window-size latest` sizes the window to
fit. This is the client's own status-line chrome, distinct from task
034/II-8's sibling-pane chrome inside the window.

## II-11: exactly two SIGWINCH per full enter/exit cycle, attached and detached (task 036)

`features/interactive_sigwinch_budget.feature` (two scenarios) proves the
same claim task 035's own `TestRestoreWindowGeometryAttachedUnsetFollowsClientWithNoThirdSigwinch`
already measured for the attached exit half, but for the **whole** cycle
(`CaptureWindowGeometry` + `FitWindowToPane` to enter, `RestoreWindowGeometry`
to exit) and read through the same fake-fixture SIGWINCH counter task 027
built (`fake-claude`'s own `$DECK_HOME/log/fake-claude-sigwinch-count`,
asserted via the existing `the fake "claude" agent received exactly N
SIGWINCH signals` step), against a bare tmux session created directly on
the scenario's own private socket (no deck-level Enter consumer exists yet;
same precedent as task 034/035's own `.feature` scenarios).

Measured, both against real tmux:

| scenario | SIGWINCH count |
|---|---|
| detached (nobody attached for the whole cycle) | **2** |
| a real client attached throughout the whole cycle | **2** |

Not 3, in either case. The detached count is one `resize-window` to enter
(single-pane window, zero chrome) plus one explicit `resize-window` back to
the original dimensions on exit (`RestoreWindowGeometry`'s `attached == 0`
branch); the attached count is the same entry resize plus, on exit, the
unset-triggered automatic "follow the attached client" resize task 035
already found — never a third, explicit `resize-window` while a client is
attached.

The attached scenario's client is deliberately attached at a size
(`80x25` raw) that nets to the **same** effective pane size as the window
already had (`80x24`, after subtracting the client's own one-row status
line) before either the entry or exit resize runs, so the attach step
itself costs zero SIGWINCH of its own — the two-signal budget measured
above belongs entirely to the enter/exit cycle, not to attaching.

### Gotcha discovered building this scenario: the fixture's own counter can undercount two back-to-back SIGWINCH

`cmd/fake-claude/main.go`'s SIGWINCH counter uses `signal.Notify` into a
channel with a buffer of exactly 1 (`signals := make(chan os.Signal, 1)`,
main.go:645). Go's own `os/signal` contract for that shape: a signal that
arrives while the channel already holds one undelivered notification is
silently dropped, never queued. Measured directly here: running this
scenario's enter and exit steps back-to-back with **no** pause between
them let a real, independently-confirmed second kernel `SIGWINCH` land
before the fixture's own counting goroutine had drained the first
notification off the channel — the counter's own file then read `1`
even though tmux really delivered two (confirmed by re-reading the same
counter file via a bare, manually-paced `tmux resize-window` sequence
against the same fixture, which reliably reports 2). This is a limitation
of the fixture's own counting mechanism under back-to-back signals, not a
deck defect; `features/interactive_sigwinch_budget.feature`'s two
scenarios each insert an explicit 200ms pause between the enter and exit
steps (and a second 200ms pause before the final assertion, guarding
against task 027's own count step returning on a transient reading before
a hypothetical extra signal would have landed) for exactly the same
reason `features/sigwinch_count_test.go` (task 027) already paces its own
resizes 50ms apart. Demonstrated directly: with the inter-step pause
removed, this scenario's own "exactly 1" red control passed when it
should have failed, because the second signal was coalesced away before
ever being counted — restoring the pause reproduces the correct "exactly
2, reject 1" result. Recorded here since a future consumer pacing real
resizes against this same fixture (or reusing task 027's counter idiom
against a different fixture with an identically small channel buffer)
would hit the same silent undercount.
