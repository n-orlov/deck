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

## II-10: `set -g window-size latest` is not a restore (task 039)

PRD II-10 names a specific, plausible "simplification" of `RestoreWindowGeometry`
distinct from II-9's reversed-order mistake: since `Client.Bootstrap` already
writes `window-size latest` at the server-**global** scope (task 032), an
implementer could believe re-asserting that same global value on exit is
enough to restore the window, instead of unsetting the WINDOW-LOCAL override
`resize-window` always leaves behind. It is not — the window-local value
shadows the global one regardless of what the global value is re-asserted
to.

`internal/tmux/restore_test.go`'s
`TestGlobalWindowSizeWriteDoesNotRestoreWindowLocalPin` proves this directly
against real tmux: after entering (which leaves `window-size manual`
window-locally, per task 034/035), issuing `set-option -g window-size
latest` directly — never through `RestoreWindowGeometry`, which never
issues this call — leaves the window-local value unchanged at `manual`,
and a **fresh** client attaching afterwards at a third size (`100x40`) is
still ignored, staying pinned at the interactive preview's `45x15`. The
same test then contrasts this with the correct scope: with that identical
client still attached, unsetting the WINDOW-LOCAL `window-size` (not the
global one) immediately hands the window to that client's own size,
proving the failure above is about scope, not about the client or the
value `"latest"` itself.

### The shipping prior art documents this wrongly

`docs/spikes/interactive-preview.md`'s finding 4 names this exact trap in
`agent-of-empires`'s shipped code: "`set -g window-size latest` restores
nothing — `resize-window` writes `window-size manual` window-locally,
shadowing the global. AoE's code is accidentally right; its comment is
wrong, and an implementer following the comment ships the broken
version." `RestoreWindowGeometry` (task 035/II-9) already implements the
correct recipe (unset the window-local option, never re-assert the
global one) and never had this bug; this task's purpose is to pin that
correctness with a scenario that would go red if a later
"simplification" — following AoE's comment instead of its code — were
ever applied here, rather than leaving the trap to be rediscovered
against a live agent.

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

## II-19: seed state from tmux formats, including the three the prior art omits (task 041)

`internal/tmux/paneseed.go`'s `PaneSeedState` reads a pane's full mode state
in one `display-message -p` invocation, joining sixteen `#{...}` formats
with `|`: `alternate_on`, `cursor_x`/`cursor_y`, `cursor_flag`,
`insert_flag`, `keypad_cursor_flag`, `keypad_flag`, the five mouse-tracking
flags (`mouse_any_flag`, `mouse_button_flag`, `mouse_sgr_flag`,
`mouse_standard_flag`, `mouse_utf8_flag`), and the three PRD II-19 names as
absent from the shipping prior art: `wrap_flag`, `origin_flag`,
`scroll_region_upper`/`scroll_region_lower`.

### Non-vacuous per field, measured against a real pane

`TestPaneSeedStateReadsEachNonDefaultFieldFromARealPane` drives a bare
40x10 tmux pane into a non-default value for each field individually (a
real `printf` of the corresponding escape sequence, run as the pane's own
program output, never as tmux-buffer-pasted bytes or literal keyboard
input) and asserts `PaneSeedState` reads exactly that value back. All
sixteen fields pass; the boolean-field cases use single DEC private mode
sequences (e.g. `ESC[?1049h` for `alternate_on`, `ESC[?7l` for
`wrap_flag`), the cursor-position case uses `ESC[5;10H` and asserts
`CursorX==9 && CursorY==4` (tmux's formats are 0-based), and the scroll
region case uses `ESC[3;7r` and asserts `ScrollRegionUpper==2 &&
ScrollRegionLower==6`.

**Gotcha: a shell prompt redraw after the command finishes perturbs
cursor position and can silently overwrite the very state a test just
set.** Sending `printf '<escapes>'` via `send-keys -l -- ... ; Enter` and
reading state after a settle sleep is enough for the *boolean* mode
flags (a prompt reprinting text does not clear a DEC private mode or
DECSTBM), but it is NOT enough for `cursor_x`/`cursor_y`: bash prints its
next prompt immediately after the command returns, starting from wherever
the command left the cursor, which silently relocates it again before the
test ever reads state. The fix used throughout this test
(`runInPaneBlocking`) appends `; cat > /dev/null` to every probe command,
so the shell blocks reading stdin and never reaches its next prompt; the
pane is left with that blocking `cat` running and torn down by
`kill-server` in cleanup.

### The negative half: a capture carries none of this

`TestCapturePaneCarriesNoModeState` is the reason `PaneSeedState` has to
exist as a separate read at all. It drives a pane into a pile of
non-default mode state at once (alternate screen, a 3;7 scroll region,
origin mode, hidden cursor, insert mode, mouse tracking, *and* SGR-coloured
text) and then runs a plain `capture-pane -p -e`. The captured bytes are
scanned for three dangerous escape classes — DEC private mode
(`CSI ? ... h/l`), DECSTBM (`CSI ... r`) and absolute cursor positioning
(`CSI ... H`/`CSI ... f`) — and contain **zero** occurrences of any of
them, while the SGR-coloured literal text (`ESC[31mRED ESC[0m`) survives
intact. A capture is cell content and SGR only; not one DEC private mode,
no scroll region, and no cursor position survives it. This is exactly why
`PaneSeedState`'s separate `display-message` read is needed, and why
task 042 (II-17/II-18) seeds mode state from it rather than trying to
infer any of it from the capture body.

### Mechanical guard demonstrated red then reverted

Swapping `WrapFlag`'s and `OriginFlag`'s field-index assignments in
`parsePaneSeedState` (a plausible off-by-one when the format string and
the struct literal drift) makes
`TestPaneSeedStateReadsDefaultState` fail correctly, naming both fields;
reverted before committing. The per-field non-default test would have
caught the same class of bug for either field individually.

## II-27: coalesce renders at DECK_INTERACTIVE_MS (task 049)

`internal/interactive/render.go`'s `RenderCoalescer` is a plain
trailing-edge debounce on a fixed ticker: a background goroutine ticks
every `interval`, and each tick delivers exactly one notification on
`Renders()` if and only if at least one `MarkDirty` call happened since
the previous tick, however many happened. `Session` (`grid.go`) wires one
in (driven by the package var `renderCoalesceInterval`, default 60ms to
match `internal/config/schema.go`'s `interactive_ms` default) and calls
`MarkDirty` from every place that changes grid content: `drain`'s
per-read `Write` (one call per pipe **read**, not per byte — this is the
exact baseline II-27 measures against), the seed write in `Start`, the
displacement fallback's periodic re-capture, `writeNotice`, and
`Resize`'s reseed. `Renders()` itself carries no frame data, only an "a
repaint is due" signal — a caller renders by calling `Grid().Render()`
each time it receives one, so the render always reflects whatever the
grid holds at that moment, never a stale snapshot from whenever the
notification was produced.

### Render frequency, not parsing, dominates the cost — measured, not assumed

`internal/interactive/render_test.go`'s
`TestRenderCostPerCallDominatesParsingCostPerCall` times 2000 `Write()`
calls of a representative chunk (plain text, one SGR colour transition, a
reset, a line ending) against 2000 `Render()` calls on the same 120x40
grid (the geometry task 068's own ~53 MiB resident-cost baseline uses).
Measured in this container:

| operation | total (2000 calls) | per call |
|---|---|---|
| `Write()` | 38.4ms | 19.2µs |
| `Render()` | 333.7ms | 166.8µs |

`Render()` costs **8.69x** what `Write()` costs per call here — composing
the whole grid into a string is the expensive step, not parsing the
incoming bytes, exactly as II-27 claims.

### Renders are coalesced against a real, known byte-arrival pattern — not one per read

`TestSessionRendersAreCoalescedAgainstAKnownByteArrivalPattern` drives a
real tmux pane through a known, reproducible pattern (40 lines, ~12ms
apart via a throttled shell loop — the same throttled-loop idiom task
043's own gotcha already established as reproducible here, in contrast to
an unthrottled flood) and counts `Session.Renders()` notifications, each
followed by an actual `Grid().Render()` call whose cost is summed, at
three settings of `renderCoalesceInterval`:

| interval | renders (900ms window) | total `Render()` cost |
|---|---|---|
| 1ms (effectively per-read) | 42 | 1.00ms |
| 60ms (SPEC default) | 9 | 0.196ms |
| 2s (longer than the whole counting window — non-vacuous control) | 0 | 0 |

Measured ratio here: per-read `Render()` cost is **5.11x** 60ms-coalesced
`Render()` cost, for this pattern and this container. PRD II-27 cites
1.99x, from the 2026-08-22 spikes' `b/REPORT.md`; that evidence is
unreachable from this container (`tasks.json`'s
`discovered.prdCorrections` — `~/deck-spikes/` does not exist here), so
this number is not a reproduction of that figure, deliberately: it is
this repository's own measurement of the identical property (coalescing
to 60ms costs meaningfully less than rendering per read) on hardware and a
tmux version this repository can actually exercise, following the same
departure-and-say-so precedent task 048 already set for an equally
unreachable spike number. The 0-renders control at a 2s interval confirms
the ticker is genuinely gating on the interval — the pattern alone, run
against a coalescer that never ticks inside the counting window, produces
nothing.

`internal/interactive/render_test.go`'s two `RenderCoalescer`-level tests
(`TestRenderCoalescerFiresAtMostOncePerIntervalRegardlessOfDirtyCallCount`,
`TestRenderCoalescerAtTwoIntervalSettingsGivesDifferentRenderCounts`) pin
the same coalescing math deterministically, independent of tmux/pipe
timing: 50 `MarkDirty` calls 1ms apart at a 20ms interval collapse to 2
renders; ~200ms of continuous dirty signals at a 10ms interval produce 20
renders (matching 200ms/10ms exactly) against 2 renders at a 100ms
interval on the identical pattern (matching 200ms/100ms) — the two-setting
comparison II-27's successCriteria names directly.

### Not yet wired into internal/tui

As of this commit, nothing in `internal/tui` constructs an
`interactive.Session` (unchanged from task 044's own note to the same
effect) — `RenderCoalescer`'s interval is the package var
`renderCoalesceInterval`, not yet fed from `settings.InteractiveMS`.
`internal/config/schema.go`'s comment on the `interactive_ms` key is
updated in this commit to say so precisely: the mechanism now exists,
but nothing hands it this setting yet. Whichever later task wires
interactive mode into `internal/tui` is responsible for setting
`renderCoalesceInterval` from `settings.InteractiveMS` at that point.

## II-28: dispatch on a verified five-field identity (task 050)

`internal/tmux/dispatch.go`'s `Client.CaptureIdentity`/`Dispatcher` is the
mechanism PRD II-28 asks for: socket path (`#{socket_path}`), server pid
(`#{pid}`), pane id (`#{pane_id}`), pane pid (`#{pane_pid}`) and session
name (`#{session_name}`) are read together with `#{pane_dead}` in one
`display-message` invocation (`dispatchIdentityFormat`) — the same
atomic-single-round-trip discipline task 043/II-20 established for the
capture+state pairing, applied here to identity+liveness instead.
`NewDispatcher` captures that tuple once, at construction ("at entry");
every `Dispatcher.Send` call re-resolves the identical five fields
immediately before running its dispatch command, and refuses instead of
sending — returning `ErrIdentityDrifted`, `ErrPaneDead`, or the dispatch
command's own wrapped tmux error — on any of the three conditions II-28
names: drift from the entry identity, `pane_dead != 0`, or a nonzero tmux
exit from the command itself.

### Counting the re-resolutions against the sends

`dispatch_test.go`'s `TestDispatcherReResolvesIdentityBeforeEverySend` is
the direct proof: `Dispatcher.Verifications()` reads exactly the number of
times `Send`/`Verify` have re-resolved identity. Five `Send` calls produce
`Verifications() == 5`; a following bare `Verify` (no dispatch command
run) brings it to 6 — one re-resolution per call, never one-at-entry-only.
`TestDispatcherRejectsOnNonZeroTmuxExit` additionally confirms the
re-resolution still happens even when the dispatch command itself then
fails (`Verifications() == 1` after exactly one failed `Send`) — the
count is "how many times identity was checked", not "how many sends
succeeded".

### The three refusal conditions, each proven against real tmux

- **Drift** (`TestDispatcherRejectsOnIdentityDriftAfterRespawn`):
  `respawn-pane -k` on the target leaves `PaneID` and `SessionName`
  unchanged (confirmed directly) while `PanePID` moves; `Send` afterward
  refuses with a wrapped `ErrIdentityDrifted`. This is the tmux-primitive
  half of PRD II-29's full scenario; task 051 builds the complete
  four-vs-five-field scenario this mechanism is meant to support.
- **Dead pane** (`TestDispatcherRejectsOnPaneDead`): killing the pane's
  process under `remain-on-exit failed` and waiting for `#{pane_dead}` to
  flip true makes `Send` refuse with a wrapped `ErrPaneDead`, without ever
  running the dispatch command.
- **Nonzero tmux exit** (`TestDispatcherRejectsOnNonZeroTmuxExit`): an
  unrecognized `send-keys` flag (a genuine tmux usage error, exit 1) is
  surfaced as a plain wrapped error, distinct from the two named sentinel
  errors. `send-keys -H zz` was deliberately NOT used for this case — PRD
  II-38 documents it as one of three commands that **silently return exit
  0**, so it would not have exercised this condition at all; that hazard
  is task 054's to close, not this one's to fake past.

### Non-vacuous positive control

`TestDispatcherSendDeliversToARealPaneAndCountsOneVerification` sends a
real marker string into a live pane through `Dispatcher.Send` and polls
`capture-pane` until it appears — proving the verify-then-send path
actually delivers, not merely that it can refuse.

### Grep proof: no send path bypasses the verify

`TestNoSendPathBypassesTheDispatcherVerify` scans every non-test `.go`
file in `internal/tmux` for a literal `"send-keys"`, `"paste-buffer"` or
`"load-buffer"` tmux command name outside comments, and fails naming
file+line if one appears anywhere except `tmux.go`'s pre-existing,
narrowly-scoped `Client.SendKeys` (task 023's env-injection primitive,
documented there as never driving a coding agent's own input). As of this
task there is no other production dispatch call site at all — later tasks
(054-060) that add real send primitives (`send-keys -l --`,
`load-buffer`+`paste-buffer`, etc.) must route their argv through
`Dispatcher.Send`, or deliberately widen this test's allowlist in the same
commit, never silently. Demonstrated non-vacuous directly: a throwaway
file with a bypassing `c.run(ctx, "send-keys", ...)` call was added
temporarily, the guard failed naming that exact file and line, and the
file was removed before committing (`git status --short` confirmed clean
afterward).

`Dispatcher` does not yet drive any real key/keystroke encoding (that is
tasks 054+); this task's scope is the verify-and-refuse bottleneck itself,
generic over whatever tmux argv a caller supplies.

## II-29: why pane_pid is in the identity tuple (task 051)

`internal/tmux/pane_pid_identity_test.go` builds the complete scenario
PRD II-29 asks for, on top of II-28's `respawn-pane` mechanism.

### The measurement: only pane_pid moves

`TestRespawnPaneChangesOnlyPanePID` reads `pane_id`, `session_name`,
`pane_dead`, `pane_start_time` and `pane_current_command` (plus
`pane_pid`) in one `display-message` call, respawns the pane
(`respawn-pane -k -t s0 bash`, so the replacement runs the identical
command name as the original -- a like-for-like comparison rather than an
artifact of whatever shell the tmux server defaults to), and reads the
same six fields again. All five PRD-named fields are confirmed unchanged;
only `pane_pid` differs. **Finding, confirmed on this tmux version:
`#{pane_start_time}` reads empty both before and after -- it is useless as
a discriminator**, exactly as PRD II-29 states; the test logs this as an
explicit FINDING line rather than leaving it implicit in a passing
unchanged-value assertion (an always-empty field would trivially look
"unchanged" even if the measurement were never actually taken).

### The insufficiency chain: pane_id alone, pane_id+session_name, and all four non-pid fields together

A deliberately minimal stand-in for `Dispatcher.Send`
(`verifyAndSendOnFieldSubset`) is parameterized by exactly which fields it
checks, so the same mechanism can be asked "does THIS subset notice a
respawn":

- `TestPaneIDAloneStillDeliversIntoTheReplacementProgramAfterRespawn`:
  checking `pane_id` alone does not notice a respawn-pane at all -- the
  marker is delivered straight into the **replacement** program.
- `TestPaneIDPlusSessionNameStillDeliversIntoTheReplacementProgramAfterRespawn`:
  adding `session_name` does not help either, because it too survives a
  respawn unchanged.
- `TestFourFieldVerifyStillDeliversIntoTheReplacementProgramAfterRespawn`:
  the punchline -- even checking **all four** of PRD II-29's other named
  fields together (`pane_id`, `session_name`, `pane_dead`,
  `pane_current_command`; `pane_start_time` is left out of this set
  deliberately, since the measurement above already shows it is always
  empty and would add nothing but the appearance of a stronger check)
  still fails to notice the respawn and still delivers into the
  replacement program.

### The non-vacuous control

`TestFourFieldVerifyIsNonVacuousAndDeliversAgainstAnUnmutatedPane` runs
the identical four-field verify against a pane that was never respawned,
and confirms it still delivers. This rules out the alternative reading of
the three insufficiency tests above -- that `verifyAndSendOnFieldSubset`
is simply a stub that always allows the send regardless of any field --
by proving the same mechanism, under ordinary unmutated conditions,
succeeds for the ordinary reason (nothing drifted), not vacuously.

### Adding pane_pid rejects

`TestAddingPanePIDRejectsAfterRespawn` takes the exact four-field set
above, adds `pane_pid`, and re-runs it against the same respawned pane:
the verify now refuses, naming `pane_pid` as the drifted field, and
`waitForMarkerAbsentFromPane` confirms the marker genuinely never reached
the pane -- the refusal actually stopped the send, not merely returned an
error while the command ran anyway. This is `pane_pid`'s entire reason for
being in `Dispatcher`'s identity tuple: it is the *only* one of the six
fields measured here that a respawn-pane actually changes.
