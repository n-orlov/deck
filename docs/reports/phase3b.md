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

## II-30: never dispatch by session name, and the impostor lands (task 052)

`internal/tmux/dispatch_impostor_test.go` proves PRD II-30 both ways: the
raw hazard, and that deck's own `Dispatcher` closes it.

### The raw hazard: renamed session, reused name, silent impostor delivery

`TestNameDispatchedPayloadLandsInTheImpostorPane` renames a session
(`orig` -> `orig-renamed`), creates a NEW session reusing the name
`orig` (the impostor), and issues a bare `send-keys -t orig -l -- <marker>`
-- never through `Dispatcher`. tmux resolves `-t orig` dynamically at
call time, so it silently lands in the impostor's pane: measured `exit=0`,
empty stderr, and the marker is confirmed present in the impostor's
capture and absent from the renamed original's. Nothing about the
command's own result distinguishes this from a correct dispatch.

### The fix: Dispatcher requires a pane id and builds its own -t

Two changes to `internal/tmux/dispatch.go` close this structurally, not
just documentarily:

1. `NewDispatcher` now validates `target` against `panePattern`
   (`^%[0-9]+$`) and refuses anything else -- a session name can never
   even be used to construct a `Dispatcher` in the first place
   (`TestNewDispatcherRejectsASessionNameTarget`).
2. `Dispatcher.Send`'s contract changed: callers pass the command name and
   arguments **without** `-t` at all (`Send(ctx, "send-keys", "-l", "--",
   payload)`, not `Send(ctx, "send-keys", "-t", target, "-l", "--",
   payload)`). Send refuses outright if `args` contains `"-t"`, and always
   builds the dispatch command's own target from `d.target` -- the exact
   pane id captured and re-verified at entry. This closes a real gap in
   task 050's original design: verify and the actual command target were
   two independently-supplied values with nothing forcing them to agree;
   after this task there is only one value, ever.

### The green control: Dispatcher refuses in the identical setup

`TestDispatcherRefusesAfterSessionRenameEvenThoughPaneIDIsUnchanged` runs
the identical rename+reuse setup, but the `Dispatcher` was constructed on
the ORIGINAL pane's id before the rename. `Send` refuses, wrapping
`ErrIdentityDrifted` -- the captured `SessionName` (`orig`) no longer
matches the pane's fresh `SessionName` (`orig-renamed`), which is exactly
the drift check task 050 built for II-29's respawn case, now shown to
also catch II-30's rename case for free. The marker is confirmed absent
from BOTH the original (renamed) pane and the impostor's -- the refusal
actually stopped the send, it did not merely report an error while
delivering anyway.

### The grep proof

`TestNoSendPathUsesSessionNameAsTarget` scans every non-test `.go` file in
`internal/tmux` for a `"-t"` tmux flag built from a `SessionName` field,
and fails naming file+line if one ever appears. With `Send`'s new
contract this is unreachable by construction (no caller can supply `-t`
at all), but the test is what keeps that property honest against a future
change, in the same idiom as `dispatch_test.go`'s own
`TestNoSendPathBypassesTheDispatcherVerify`. Confirmed non-vacuous
(`TestNoSendPathUsesSessionNameAsTargetIsNonVacuous`): the guard's pattern
does match a realistic violation string, never wired into a real file.

`dispatch_test.go`'s five pre-existing `NewDispatcher(ctx, client, "s0")`
call sites were updated to `"%0"` (a fresh bare session's first pane,
already pinned by `TestCaptureIdentityReadsAllFiveFieldsFromARealPane`)
and their `Send` calls' `"-t", "s0"` pairs removed, to match the new
pane-id-only/no-caller-`-t` contract -- no test assertion was loosened,
only the argv shape callers must use.

### Validation-failure round two: the grep proof was vacuous against a real production path

The first landing of this task (later marked `validation-failed`) missed a
real, live violation: `internal/tmux/tmux.go`'s pre-existing `Client.SendKeys`
(task 023's inject-instead primitive, called from
`internal/service/inject.go:70`) built
`c.run(ctx, "send-keys", "-t", name, "-l", "--", literal)` where `name` is
`sessionName(slug)` -- a tmux **session name**, not a pane id -- entirely
outside `Dispatcher`. `TestNoSendPathUsesSessionNameAsTarget`'s regex
(`"-t"[^\n]*SessionName|SessionName[^\n]*"-t"`) only matches the literal Go
identifier `SessionName` (the `Identity` struct field) adjacent to `"-t"` --
`name` is a local variable, never spelled `SessionName`, so the guard never
saw it. The mechanism itself (`Dispatcher`/`NewDispatcher`) was correct and
its own tests were genuine; the gap was that `SendKeys` was never migrated to
use it, and the grep was scoped too narrowly to catch that class of bypass.

**Fix**: `SendKeys` now resolves the session's live pane id via
`PreviewPane` (the same read-only lookup the preview panel already uses),
constructs a `Dispatcher` on that pane id, and sends both the literal
payload and the following `Enter` through `Dispatcher.Send` -- so the real
tmux command's `-t` is always the verified pane id, and a rename between
resolution and send is caught as `ErrIdentityDrifted` rather than silently
reaching an impostor. The real end-to-end scenario
(`@requirement-023-inject-instead-exports-into-live-shell-without-restarting`)
stays green against the fix, unmodified.

**Broadened guard**: `TestNoProductionCodeBuildsADangerousCommandWithItsOwnExplicitTarget`
replaces the narrow SessionName-identifier match with a general one -- any
of the three input-dispatch commands (`send-keys`/`paste-buffer`/`load-buffer`)
together with an explicit `"-t"` flag on the same line, in any production
file other than `dispatch.go` itself (the one place allowed to build `"-t",
d.target` for a caller-supplied command name). Confirmed non-vacuous against
the ACTUAL shipped violation, not a synthetic one
(`TestNoProductionCodeBuildsADangerousCommandWithItsOwnExplicitTargetIsNonVacuous`
feeds it tmux.go's real pre-fix `SendKeys` line verbatim and confirms a
match). The original `TestNoSendPathUsesSessionNameAsTarget` is kept
alongside it (still true, just narrower) rather than removed.

## II-31: carry the socket and a server-lifetime discriminator (task 053)

`internal/tmux/dispatch_socket_test.go` proves PRD II-31's two named cases:
pane ids collide across independent sockets, and a restarted server on the
same socket path reissues the same id -- and that deck's `Identity`
(already carrying `SocketPath`/`#{socket_path}` and `ServerPID`/`#{pid}`
since task 050) is what tells these apart.

### The raw hazard: cross-socket collision, measured directly

Two independent `-L` sockets, each a freshly started tmux server, each
started via `newBareGeometrySession` with no relationship to each other:
their first pane is `%0` on BOTH -- confirmed directly
(`TestCrossSocketPaneIDCollisionSilentlyHitsTheLocalPane` asserts this
before doing anything else, failing the test outright if the collision
this whole task rests on didn't actually happen). A bare
`send-keys -t %0 -l -- <marker>` aimed at socket B -- naming only the pane
id, exactly what a caller who tracked the id but not which server it came
from would do -- succeeds with `exit=0` and empty stderr, and the marker
lands in socket B's own unrelated `%0`. Socket A's `%0`, the pane the
caller may have actually meant, never sees it. tmux gives no signal
whatsoever that `%0` means something completely different depending on
which socket you ask.

### The discriminator: Identity's extra fields, not PaneID, break the tie

`TestIdentityDistinguishesSameLooksLikePaneIDAcrossSockets` captures
`Identity` via `Client.CaptureIdentity` on both sockets' `%0` and shows
`PaneID` is identical (`"%0" == "%0"`) while `SocketPath` and `ServerPID`
both differ, so the full `Identity` values are unequal despite the
matching pane id -- exactly the property `Dispatcher.Send`'s `fresh !=
d.identity` comparison (task 050) depends on to tell these apart.

### The Dispatcher-level refusal: modelling the misdirection directly

`TestDispatcherRefusesWhenTargetIdentityBelongsToTheWrongSocket`
constructs a `Dispatcher` normally against socket A (identity captured
there, exactly as `NewDispatcher` always does), then reassigns the
Dispatcher's own `client` field to socket B's `Client` -- modelling, from
inside the package, the misdirection a caller who tracked only a bare
pane-id string could fall into. `Send` refuses, wrapping
`ErrIdentityDrifted`; the marker is confirmed absent from both sockets'
panes afterward, not merely reported as failed while still delivered.

**Non-vacuous, demonstrated and reverted this task** (not left as an
assumption): temporarily narrowing `verify`'s comparison from the full
`fresh != d.identity` to `fresh.PaneID != d.identity.PaneID` made both
this test and the restart test below fail exactly as expected (`got nil
error, want a refusal for identity drift`), confirming the discriminator
fields, not some other mechanism, are what the tests are actually
exercising. Reverted before commit; `git diff` on `dispatch.go` is empty.

### The second named case: a restarted server reissuing `%0`

`TestRestartedServerReissuesPaneIDAndDispatcherRefuses` captures a
`Dispatcher` on a fresh session's `%0`, kills that tmux server
(`kill-server` on the same `-L` socket), starts a brand-new server on the
exact SAME socket path with a session of the exact same name (`s0`,
deliberately unchanged from before the restart, to rule it out as an
accidental discriminator), and confirms directly: `PaneID` unchanged
(`%0` both times -- a fresh server's first pane always is), `SessionName`
unchanged (kept identical on purpose), `SocketPath` unchanged (it names a
filesystem path, not a server instance -- a restart on the same `-L` name
reuses the same path, measured directly:
`/tmp/tmux-1000/deck-dispatch-socket-restart-<pid>-<ns>` before and after),
and `ServerPID` **different** -- the only one of the four that moves.
`Dispatcher.Send`, captured before the restart, refuses with
`ErrIdentityDrifted` against the new server, and the marker never reaches
the post-restart pane. This is the direct proof that `ServerPID`, not
`SocketPath`, is the actual "server-lifetime discriminator" PRD II-31
names -- a socket-path-only carry-along would have let this one straight
through.

### What this task did not need to change

`Identity`'s `SocketPath`/`ServerPID` fields, `Dispatcher`'s capture-at-
construction/re-resolve-before-every-send design, and the `==`-comparison
drift check were all already in place from task 050 -- built to satisfy
II-28's five-field identity, which happens to already be the II-31
discriminator too. This task's contribution is the missing proof: that
these fields actually distinguish the two hazards PRD II-31 names, with a
raw/no-deck-code hazard demonstration for each, not new production code.
`go build`/`go vet`/`gofmt` clean; `go test -race -count=1
./internal/tmux/...` green; `go test -count=1 ./internal/... ./cmd/...`
green (whole `./...` suite not re-run this task per the budget rule --
change confined to one new test file plus this doc).

## II-32/II-38: `send-keys -l --` for every literal payload, every exit
## code checked (task 054)

`internal/tmux/send.go`'s `Dispatcher.SendLiteral` is the one reviewed
code path this package permits for delivering a literal payload: it
always builds `send-keys -l --` through `Dispatcher.Send` (never a bare
`Client.run`), so the pane id target is always the identity-verified one
(PRD II-30/II-28) and the underlying tmux command's own exit is always
checked (PRD II-38). `Client.SendKeys` (task 023's pre-existing
env-injection primitive) now calls `SendLiteral` instead of inlining its
own `-l --` call, so there is exactly one call site in the whole package
that ever assembles a literal `send-keys` argv.

### The two named `--`/`-l` hazards, proven raw against real tmux

PRD item 32 names two failure shapes for a payload sent without `--`, and
they are genuinely different, not the same mistake twice:

- A payload that happens to spell one of `send-keys`' own flag letters
  (e.g. literally `-l`) is silently absorbed as ANOTHER occurrence of that
  flag: `send-keys -l -t s0 -l` exits 0 and delivers nothing --
  `TestSendKeysDashPayloadWithoutSeparatorIsSilentlyDiscarded`. This is
  the sharpest form of the hazard, because a payload beginning with `-`
  that does NOT name a real flag (e.g. `-foo`) instead fails outright
  with "unknown flag" (exit 1) -- checking the exit code cannot tell
  these two cases apart from "delivered correctly" without already
  knowing which shape the payload takes.
- A payload of exactly `--help` sent without `--` is parsed by tmux as
  its own (invalid) long-flag syntax and errors outright --
  `TestSendKeysDoubleDashHelpWithoutSeparatorErrors`. Both hazards close
  the same way (always send `--`), but they are demonstrated separately
  because their failure shapes differ.

Without `-l`, tmux treats the payload's words as key NAMES, not text:
`send-keys -t s0 -- Enter` on an otherwise-empty prompt line advances the
cursor to a new, still-empty prompt line (a real carriage return) with no
`"Enter"` text anywhere in the pane --
`TestSendKeysWithoutLiteralFlagEnterBecomesACarriageReturn`.

Two green controls confirm `Dispatcher.SendLiteral` actually closes the
first two hazards, not merely avoids re-deriving them: sending literal
`-l` and literal `--help` through `SendLiteral` both land in the pane as
exactly the bytes given (`TestDispatcherSendLiteralDeliversADashPrefixedPayloadCorrectly`,
`TestDispatcherSendLiteralDeliversDoubleDashHelpCorrectly`).

### PRD II-38's three named "exit 0" hazards

`-l -l` (an accidentally doubled literal flag immediately followed by a
dash-shaped payload with no `--`) hits the exact same swallowed-as-a-flag
mechanism as the first hazard above --
`TestSendKeysDoubledLiteralFlagWithoutSeparatorIsSilentlyDiscarded`.
`-H zz` (`zz` is not a valid hex byte) is accepted with exit 0 and
delivers nothing -- `TestSendKeysInvalidHexByteIsSilentlyDiscarded`. An
unrecognized key name (e.g. `Frobnicate`, no `-l`) is the odd one out:
exit 0, but it IS delivered, character by character, as literal text --
`TestSendKeysUnknownKeyNameIsDeliveredAsLiteralTextWithExitZero` (also
PRD item 35's own hazard, closed properly by task 056's allowlist).
Grouping all three under "returns 0" is the PRD's actual point: the exit
code alone cannot distinguish "delivered correctly" from "silently
discarded" from "delivered WRONG" for any of them -- the only real fix is
never constructing the wrong argv shape in the first place, which is
exactly what routing every literal send through the one `SendLiteral`
call site accomplishes.

### The two grep guards

`TestEveryLiteralSendUsesDashLDashDash` fails, naming file and line, if
`send-keys` and `-l` ever appear on the same line of production code
without `--` also present there; confirmed non-vacuous by temporarily
removing `send.go`'s own `--` and watching it fail with the exact
predicted message, then reverting (`git diff` on `send.go` confirmed
empty before commit). `TestNoIgnoredExitStatusForInputDispatch` fails if
any of `send-keys`/`paste-buffer`/`load-buffer`'s return value is ever
discarded via Go's blank identifier (both the two-value `Client.run` shape
and the one-value `Dispatcher.Send`/`SendLiteral` shape); its
non-vacuousness is demonstrated against realistic violation strings
directly (the same string-literal idiom `dispatch_test.go`'s and
`dispatch_impostor_test.go`'s own grep guards already use, since wiring a
real single-value discard into `send.go` breaks the build on an unused
import rather than exercising the grep path). `dispatch_test.go`'s
pre-existing `TestNoSendPathBypassesTheDispatcherVerify` allowlist is
widened to include `send.go` (task 052's own guard explicitly anticipated
this: "forcing whoever adds a new send primitive ... to deliberately
widen this allowlist in the same commit, never silently").

`go build`/`go vet`/`gofmt` clean; `go test -race -count=1
./internal/tmux/...` green; `go test -count=1 ./internal/... ./cmd/...`
green (whole `./...` suite not re-run this task per the budget rule --
change confined to `internal/tmux` plus this doc).

## II-33: peel exactly one trailing `;`, re-send it as `-H 3b` (task 055)

Measured directly against real tmux (no deck code), zero/one/two/three
trailing-semicolon cases: `send-keys -l -- "ab"` delivers `ab` intact;
`"ab;"` delivers `ab` (the lone trailing `;` vanishes, even with `--`
present -- `--` only protects a *leading* `-`, a different parser
quirk); `"ab;;"` delivers `ab;` (exactly ONE of the two vanishes, never
both, never neither); `"ab;;;"` delivers `ab;;`. The loss is always
exactly one `;`, never `N-1`. An interior `;` (`"a;b"`) is completely
unaffected either way.

Because the loss is always exactly one regardless of how many trail, a
single-peel-then-resend fix is not enough once two or more trail: the
resent remainder would itself still end in `;` and lose one again.
`Dispatcher.SendLiteral` (`internal/tmux/send.go`) now calls
`peelTrailingSemicolons`, a fixed-point loop that strips ALL trailing
`;` off the payload (interior ones are never touched -- the loop only
ever inspects the current last byte), sends the now-semicolon-free body
through the ordinary `-l --` path (nothing left there for tmux's parser
to eat), and re-delivers every peeled semicolon through one batched
`send-keys -H 3b` call (one hex byte per peeled `;`) -- `-H` bypasses
the `-l` literal-text parser entirely, so a hex-encoded semicolon can
never be reinterpreted as tmux's own trailing-`;` separator. The PRD's
own `\;` escape is deliberately not used (PRD II-33: "not composable");
`semicolon_test.go`'s `TestNoBackslashSemicolonEscapeIsUsedFor...` greps
this package to prove it never appears.

`semicolon_test.go` proves both the raw hazard (zero/one/two trailing
semicolon cases named by the PRD, plus a three-semicolon case exercising
the fixed-point loop beyond a single peel, plus the interior-untouched
case) and the fix (`Dispatcher.SendLiteral` green controls for the same
cases, plus the degenerate all-semicolon payload where the literal body
is empty and only the `-H` batch runs). Non-vacuousness confirmed by
temporarily reverting `SendLiteral` to the pre-fix single-call form and
watching exactly the semicolon-related green controls turn red with the
predicted messages (`git diff` on `send.go` empty before commit).

Gotcha discovered building this: `newBareGeometrySession` returns as
soon as `tmux new-session -d` completes, before the freshly-forked
shell has started reading the pty -- `send-keys` issued immediately
after can land bytes that the pty's local echo shows right away but the
shell's own prompt then prints itself onto the SAME line, AFTER the
already-echoed text (observed directly: `"a;b"` sent with zero delay
after `new-session` came back as `a;b$`, the prompt landing after, not
before). `semicolon_test.go`'s `waitForBarePrompt` polls for the pane's
first line to read a bare `$` before sending anything, closing the race
at its source rather than loosening the assertions.

`go build`/`go vet`/`gofmt` clean; `go test -count=1 ./internal/tmux/...`
green (repeated 8x targeted, 3x whole-package, to confirm no flake from
the prompt-race fix); whole `./...` suite not re-run this task per the
budget rule (change confined to `internal/tmux` plus this doc).

## II-34/II-35: named keys go by tmux name, validated against an allowlist before spawning (task 056)

`internal/tmux/key.go` adds `Dispatcher.SendNamedKey(ctx, name)`: it sends
`send-keys <name>` (never `-l`), so tmux's own key-string translation
produces whatever bytes that name means, and checks `name` against a
fixed `namedKeyAllowlist` *before anything else happens* -- an unlisted
name returns an error with no tmux invocation of any kind, not even the
identity re-resolution `Dispatcher.Send` would otherwise perform first.
`key_test.go`'s `TestSendNamedKeyRejectsAnUnknownNameWithoutSpawningTmux`
proves the "before spawning" half directly: `Verifications()` (task
050's re-resolution counter) does not move at all for a rejected name.

### The allowlist itself, confirmed against real tmux, not copied from a manual

Every entry in `namedKeyAllowlist` was checked directly against a real
tmux 3.5a server while building this task: each candidate name was sent
(raw, no deck code) to a pane running `cat` -- which echoes whatever
bytes it receives right back, unedited -- and the resulting
`capture-pane` content inspected byte for byte. Accepted names
(`Up`/`Down`/`Left`/`Right`, `Home`/`End`, `IC`/`DC`/`Insert`/`Delete`,
`PPage`/`NPage`/`PageUp`/`PageDown`/`PgUp`/`PgDn`, `BSpace`/`BTab`/`Tab`,
`Enter`/`Escape`/`Space`, `F1`-`F12`) all came back as tmux's own
translated escape or control byte. Plausible-looking names that turned
out NOT to be key-string translation targets at all were excluded and
are asserted rejected by `TestSendNamedKeyRejectsEveryNameNotOnTheAllowlist`:
`KPDivide`/`KPMultiply`/`KPMinus`/`KPPlus`/`KPPeriod` (unlike `KP0`-`KP9`
and `KPEnter`, which ARE recognized but are not needed by this task and
so were left out) and `WheelUpPane` (a mouse bind-key target, not a
key-string name -- typed back literally, exactly like an unrecognized
name). Ctrl/Shift/Alt-modified names (e.g. `C-a`, `S-Up`) are
deliberately out of scope: task 060/II-40 forwards Ctrl combinations as
literal control bytes through `SendLiteral` instead, precisely so `C-b`
reaches the agent without ever matching tmux's own prefix table.

### The named hazard, and deck's refusal of it

PRD item 35's own hazard (`send-keys Frobnicate` types ten literal
bytes into the pane, exit 0) is demonstrated raw, with no deck code
involved, by `literal_send_test.go`'s pre-existing
`TestSendKeysUnknownKeyNameIsDeliveredAsLiteralTextWithExitZero`. This
task's `TestSendNamedKeyRejectsAnUnknownNameWithoutSpawningTmux` shows
`SendNamedKey` refuses that exact input instead: an error, zero tmux
invocations, and nothing delivered into the pane.

### The finding: tmux's Home/End translation is faithful to a real attach, not just self-consistent

`TestHomeAndEndEscapesMatchARealAttachedClientTypingTheSameKeys` proves
the finding PRD item 34 asks for directly, not by assertion: a real
tmux 3.5a server emits the identical vt220 escape for Home (`ESC[1~`)
and End (`ESC[4~`) whether the bytes arrive via `send-keys <Name>` (this
task's `SendNamedKey`) or via a real attached client's own pty receiving
the same raw bytes a physical terminal emulator would send for that
key. tmux performs no retranslation of raw client input that does not
match one of its own key bindings -- it forwards what the client sent
straight to the active pane -- so writing those bytes directly into an
attached client's pty (bypassing only the physical keyboard +
terminal-emulator step) is a faithful stand-in for "a user pressed
Home/End while attached". `capture-pane -p` itself renders a stored raw
ESC byte as the two-character caret notation `^[`, not the literal
`0x1b` byte -- confirmed by hexdumping its output directly
(`5e 5b 5b 31 7e` = `^`, `[`, `[`, `1`, `~`) -- both tests compare
against that rendered form rather than a `0x1b`-literal guess.

### The grep proof: no hand-built escape sequence

`key_grep_test.go`'s `TestNamedKeySendNeverHandBuildsAnEscapeSequence`
fails, naming the line, if `key.go`'s own source ever contains a Go
escape-literal spelling of an ESC byte (`\x1b`/`\033`/`\u001b`) --
`SendNamedKey` only ever hands tmux the NAME, never the escape sequence
itself. `key_test.go`'s own use of literal escape bytes (to describe
tmux's expected translated output, and to stand in for a real
terminal's translation of a physical keypress) is not covered by this
guard, which only inspects `key.go`.

`dispatch_test.go`'s `TestNoSendPathBypassesTheDispatcherVerify`
allowlist is widened to include `key.go` in this same commit (task 052's
own guard explicitly anticipated this).

`go build`/`go vet`/`gofmt` clean; `go test -count=1 ./internal/tmux/...`
green (17.5s); whole `./...` suite not re-run this task per the budget
rule (change confined to `internal/tmux` plus this doc).

## II-36: chunk literals at 8192 bytes, `-H` at 4096 args, stream anything larger through `load-buffer` (task 057)

### The real ceiling, measured directly against tmux 3.5a in this repo's CI image

`internal/tmux/chunk_test.go` bisects both of PRD II-36's named hazards
raw, with no deck code involved, against the exact tmux binary
`ci/Dockerfile` installs:

- `send-keys -l --` with a 16340-byte literal payload succeeds (exit 0);
  with a 16360-byte payload it fails with tmux's own `command too long`
  (`TestSendKeysLiteralFailsAtTheRealCommandLengthCeiling`). A 100000-byte
  payload -- six times larger, still comfortably under Linux's own
  512 KiB per-argument `MAX_ARG_STRLEN` -- fails with the identical
  `command too long`, not any OS/exec-level error
  (`TestSendKeysLiteralFailureIsNotARGMAX`), confirming the ceiling is
  tmux's own, not the OS's `ARG_MAX`.
- `send-keys -H` with 5445 valid two-hex-digit arguments (e.g. `61`)
  succeeds; 5455 fails with `command too long`
  (`TestSendKeysHexFailsAtTheRealArgCountCeiling`).

**Correction to the PRD's own numbers** (recorded in full in
`docs/reports/phase3b-findings.md`'s II-36 entry): PRD II-36 states the
`-l --` ceiling as "16380 bytes", which this measurement confirms almost
exactly (the real crossover sits between 16340 and 16360). It also
states the `-H` ceiling as "8192 args", which this measurement does
**not** confirm for valid two-hex-digit arguments -- the real crossover
measured here is between 5445 and 5455 args, roughly two thirds of 8192.
5450 two-character hex tokens plus one separating space apiece is itself
~16.3 KiB, matching the `-l --` crossover almost exactly, which strongly
suggests the PRD's "8192" figure was derived assuming a narrower
per-argument width (closer to one byte per argument) rather than
measured with real two-digit hex bytes -- both figures point at the SAME
underlying ~16 KiB internal command-string limit, not two independent
ceilings.

### The fix: chunk small, stream large

`internal/tmux/send.go`'s `Dispatcher.SendLiteral` now splits its body
at `literalChunkBytes` (8192 bytes, comfortably under either measured
ceiling): a body at or under that size still goes through the single
`send-keys -l --` call this package always used; a larger body is
handed to a NEW `streamLiteralViaLoadBuffer`, which `load-buffer`s the
body over stdin (no argv-length ceiling at all, regardless of payload
size) into a uniquely-named server-side buffer, then delivers it with
`paste-buffer -d -b <name>` through `Dispatcher.Send` (so the actual
delivery step is still identity-re-verified like every other send in
this package) -- `load-buffer` itself is not, since it never touches the
target pane at all, only stages bytes into a scratch buffer. A failed
`paste-buffer` calls `delete-buffer` explicitly, since `-d` only deletes
the buffer on success (the same finding task 058/II-37 restates for its
own `paste-buffer -d -p` multi-line path).

The peeled-trailing-semicolon `-H` re-delivery (task 055/II-33) is
chunked the same way at `hexChunkArgs` (4096 args): `sendHexByteRun`
issues as many `send-keys -H` calls as needed, each capped at 4096 hex
arguments. There is no `load-buffer` substitute for this path --
`load-buffer`/`paste-buffer` inserts raw literal bytes, which is exactly
the `-l` parser hazard `-H` exists to route around for a trailing `;` --
so an oversized run is chunked into multiple calls instead of streamed.

### Non-vacuous proof

`TestDispatcherSendLiteralStreamsOversizedPayloadViaLoadBufferAndArrivesIntact`
sends a 20000-byte, order-sensitive (non-repeating) payload through
`SendLiteral` and confirms it arrives at the target pane byte-for-byte,
verified via `capture-pane -p -J -S -` (join soft-wrapped lines, include
full scrollback) so a single very long typed line that wrapped across
far more physical rows than the pane's own height can be reassembled and
compared as one continuous string. This is non-vacuous by construction,
demonstrated directly while building this task (temporarily reverted
before commit, `git diff` empty): with the `literalChunkBytes` check
removed (every payload going straight to one `send-keys -l --` call,
this package's pre-057 behaviour), this EXACT test fails with the same
`command too long` demonstrated above -- success is only possible
because the oversized body actually streamed through `load-buffer`.
`TestDispatcherSendLiteralChunksAnOversizedHexRunAndArrivesIntact` proves
the analogous claim for `-H`: 6000 trailing semicolons
(`TestSendKeysHexFailsAtTheRealArgCountCeiling` already showed a single
6000-argument `-H` call fails outright) still arrive intact, because
`sendHexByteRun` splits the run into two calls (4096 + 1904).
`TestDispatcherSendLiteralAtExactlyTheChunkBoundaryStillUsesASingleCall`
is the regression control at the boundary itself: a body of exactly
8192 bytes (nowhere near either measured ceiling) still round-trips
correctly.

`dispatch_test.go`'s `TestNoSendPathBypassesTheDispatcherVerify` widens
`send.go`'s own allowlist entry (per-file, not per-command, as of this
task) to include `load-buffer`/`paste-buffer` alongside `send-keys`, in
this same commit, per that test's own documented escape hatch.
`internal/tmux/tmux.go` gains a generic `Client.runWithStdin`, used only
by `send.go`'s `streamLiteralViaLoadBuffer` -- it never itself names
`load-buffer`, so it needed no allowlist change of its own.

`go build`/`go vet`/`gofmt` clean; `go test -count=1 ./internal/tmux/...`
green (18.2s); `go test -count=1 ./internal/... ./cmd/...` green (broader
unit sweep, not the whole `./...` suite, per the budget rule).

## II-47/II-48: refuse to enter in three named cases, and the 7-row floor behind them (task 066)

Requirement 47 names three, and only three, cases in which entering
interactive mode is REFUSED rather than degraded: another client already
attached to the session, the preview box below the measured 7-inner-row
floor, or a live process already holding the window's ownership option.
`internal/tui/interactive.go`'s `enterInteractive` checks all three before
doing anything that would otherwise commit to entering, each setting
`m.attachError` to a message that names the reason and states `press a to
attach instead` -- `a` (`attachSelected`) is never disabled by a refusal,
so the offer is not merely stated but true.

1. **Another client attached** (`Client.SessionAttachedCount`, already
   shipped for II-9's own exit gate): `#{session_attached} > 0` on the
   session's own window is checked before any tmux call that would resize
   it, because the squeeze is unavoidable -- one tmux window has one size,
   and a bystander watching that session from a second, real tmux client
   would see their own terminal collapse into the preview panel's box for
   the duration.
2. **Preview box below the 7-row floor**: `previewContentSize` is computed
   (a pure function of `m`'s own layout, no tmux call) before anything
   else, and refused if the row count is below `interactiveMinInnerRows`
   (7) -- see the measurement below for why 7, not merely "greater than
   zero".
3. **A live process holds ownership**: `Client.ClaimWindowOwnership`
   returning `acquired == false` (its own kill(pid, 0) liveness check
   already excludes a stale claim from a dead process) is exactly this
   case; the refusal message was renamed from the pre-066 "another deck is
   resizing this window" to name the PRD's own wording and add the `press
   a` offer.

### The measurement behind "7"

`docs/reports/phase3d-ii47-48-floor-measurement.log` is `capture-pane -p`
output from a real tmux pane, on this repo's own CI image's tmux 3.5a, at
each of the PRD's four named sizes plus a fifth case for the soft-wrap
claim. The fixture (a disposable shell script, not committed -- see the
layout below) draws a Claude-Code-shaped screen: a one-row header, a
variable-height transcript region, a bordered input box (`╭─...─╮` /
`│ > ... │` / `╰─...─╯`), and a one-row hint line, i.e. exactly the same
row budget PRD 48 describes: `fixed = header(1) + blank(1) + box(3) +
hint(1) = 6` fixed rows, `transcript = rows - fixed`.

| size  | label               | transcript rows measured |
|-------|---------------------|---------------------------|
| 41x22 | comfortable-default | 16                        |
| 36x22 | comfortable-narrow  | 16                        |
| 41x7  | smallest-usable     | 1                         |
| 41x6  | pure-chrome-floor   | 0                         |

41x6 renders the header, the full 3-row box and the hint line and NOTHING
else -- zero transcript rows, exactly PRD 48's "pure chrome and zero
transcript" claim, measured rather than assumed. 41x7 renders exactly one
transcript line above the same chrome -- the smallest box in which any
transcript content is visible at all, which is why requirement 47's
refusal threshold is `< 7`, not `<= 0`.

The fifth case demonstrates the PRD's closing claim that 7 is a floor, not
a guarantee: at the SAME 41x7 size, growing the input box's own content
from 1 to 2 rows (a real agent's box soft-wrapping a longer typed line)
grows the box from 3 to 4 fixed rows, pushing `fixed` from 6 to 7 --
`transcript = 7 - 7 = 0`. A wrapped input at the 7-row floor is exactly as
transcript-less as an unwrapped one at 6 rows; the floor is where SOME
agent becomes usable, not every agent at every input length.

### Non-vacuous demonstration (task 066's own refusal code)

Reverting `internal/tui/interactive.go` to its pre-066 refusal
messages/thresholds (`git stash` during this task, not part of the
committed diff) and re-running
`DECK_GODOG_TAGS="@requirement-47-refuse-attached-client,@requirement-48-refuse-preview-below-seven-rows,@requirement-47-refuse-live-ownership"
go test -run TestFeatures ./features/` fails exactly where expected: the
live-ownership scenario's `screen contains "holds ownership"` step times
out against the old `"Cannot enter interactive mode: another deck is
resizing this window"` wording (the attached-client and below-floor cases
did not exist in the pre-066 code at all -- entering degraded through to
a real `ClaimWindowOwnership` call in the attached-client case, and
through to a `width<=0||height<=0` check that never fires at 6 content
rows in the below-floor case). Restoring the committed code makes all
three scenarios pass again, run individually and together
(`go test -count=1 ./features/`, 2.5s for the three scenarios).

`go build`/`go vet`/`gofmt` clean; `go test -count=1 ./internal/tui/...`
green; the three new scenarios in `features/interactive_refusals.feature`
pass individually and together via `DECK_GODOG_TAGS`.

### Discharging task 047's forward reference

Task 047's own commit left an explicit promise to "re-verify at the
Session/TUI boundary once 066 actually lands a refusal decision point":
now that it has, the chain of custody is vacuously closed on the refusal
paths themselves and non-vacuously closed on the paths after them. All
three refusals above return before `ClaimWindowOwnership` is ever called,
so there is no pipe armed yet to release on any refusal path; every path
AFTER ownership is claimed that can still fail --
`client.FitWindowToPane` erroring, `tmux.NewDispatcher` erroring, or
`interactive.Start` erroring -- already calls `ownership.Release` and (for
the latter two) `client.RestoreWindowGeometry` before returning, and the
one case among those that has actually armed the pipe
(`interactive.Start` failing after `tmux.NewDispatcher` succeeded) is
exactly what task 047's own
`TestSessionStartFailureAfterArmingReleasesThePipeOnErrorExit` already
proves releases it on error exit.

### Steer 012: the attached-client refusal now asserts the bystander's window, not just deck's own screen

The `@requirement-47-refuse-attached-client` scenario asserted only
deck's own rendered screen text -- true even if the refusal ran AFTER
`FitWindowToPane` and then "changed its mind", since none of those three
assertions look at the bystander's real tmux window at all. The scenario
now additionally captures the private tmux window's geometry
(`features/preview_test.go:31`'s existing step, task 022) right after the
real second client attaches, and asserts it is byte-identical after the
refusal (`features/preview_test.go:32`), plus that
`@deck_isize_owner` is unset in the window scope
(`features/tmux_option_scope_test.go:126`'s existing step) proving this
refusal claimed no ownership either. Demonstrated non-vacuously: moving
the attached-client check (temporarily, `git diff` confirmed empty after
reverting) to run after `FitWindowToPane` makes the new geometry
assertion fail (`captured "before-refusal" as "80x23", now "61x27"`)
while the original three screen-text assertions still pass -- exactly the
contrast the steering note asked for: the old assertions cannot tell
"never touched the window" from "touched it and changed its mind", the
new one can.

## II-51: the grid keeps its own bounded scrollback, scrolled by the wheel and Shift+PgUp/PgDn (task 068)

`internal/interactive.Grid` (vendored `charmbracelet/x/vt`) already keeps a
scrollback -- `vt.DefaultScrollbackSize` is 10000 lines, unbounded by any
deck-owned limit -- and the wheel/Shift+PgUp/PgDn did not scroll it at all:
`interactiveBodyLines` always rendered the grid's own *live* bottom
(`Grid().Render()`), so the only way to see output that had scrolled off
the fitted view was the pane's own tmux scrollback (`a`, full attach), not
interactive mode's grid.

### The bound: `ScrollbackMaxLines = 2000`, not the PRD's own unreachable spike figure

PRD 51 states "the grid already costs ~53 MiB resident for one 120x40
emulator before any scrollback" -- one of the three 2026-08-22 spikes
whose raw evidence is not reachable from this container (see
`discovered.prdCorrections` in `tasks.json`). Measured directly in this
container instead (`TestScrollbackMemoryDoesNotGrowWithoutBound`,
`internal/interactive/scrollback_test.go`), a bare empty 120x40 grid with
no scrollback costs ~5.9 MiB `HeapAlloc` over an empty-process baseline --
an order of magnitude below the PRD's own cited figure, and not something
this task can reconcile without the missing spike evidence. What this
task measured instead, and is responsible for, is the *scrollback's own*
marginal cost, since that is what requirement 51 asks to be bounded:

  * feeding exactly 2000 lines of full-width (120-column), realistic,
    non-repeating content (`"line %06d"` padded to full width, not a
    single repeated byte -- Go's static single-byte-string optimisation
    made an earlier scratch measurement of ASCII filler misleadingly
    cheap, ~250-300 bytes/line, before this was caught) grows `HeapAlloc`
    by ~27.6 MiB over the empty baseline (5,930,088 -> 33,562,680 bytes);
  * flooding 40x that many lines (80,000 lines fed, 40x the bound) grows
    it by only ~0.55 MiB more (33,562,680 -> 34,138,824 bytes) --
    `vt.Scrollback`'s own ring-buffer eviction (`SetMaxLines`, called once
    from `newGrid` via `SetScrollbackSize(ScrollbackMaxLines)`) holds the
    line count at exactly 2000 throughout, so the ~27.6 MiB is
    (approximately) the ceiling, not a floor a real session could exceed
    by typing enough.

2000 lines (not vt's own `DefaultScrollbackSize` of 10000) is deck's own
override, chosen as a comfortable multiple of a typical terminal's height
(dozens of screens back) while keeping the measured ceiling in the tens,
not hundreds, of MiB per interactive pane -- one deck client can only ever
have one pane interactive at a time (§11.9's own single-target
constraint), so this is a per-client ceiling, not a per-session one that
would multiply across a sidebar full of sessions.

### Non-vacuous proof the bound holds under real volume, not merely under the exact bound

`TestScrollbackStaysBoundedRegardlessOfInputVolume` feeds exactly
`ScrollbackMaxLines` lines (asserts `ScrollbackLen() == ScrollbackMaxLines`
already), then 20x more again on top (asserts `ScrollbackLen() <=
ScrollbackMaxLines` still, and `== ScrollbackMaxLines` once the flood
settles) -- proof the *count* stays bounded, independent of the *memory*
measurement above, which uses a completely separate grid so one test's
result cannot leak into or mask the other's.

### The wire-up: `RenderRows`, an offset the caller owns, and no scrolling of anything else

`Session.RenderRows(offset, height)` composes `height` rows starting
`offset` lines back from the live bottom (scrollback lines first, then
the live grid's own rows), clamping `offset` to `[0, ScrollbackLen()]` and
returning the clamped value so the caller's own stored offset
(`Model.interactiveScrollOffset`) never drifts out of range from a single
call. At `offset == 0` its output is byte-for-byte identical to the
pre-068 `Grid().Render()` + `strings.Split` path
(`TestRenderRowsMatchesLiveRenderAtZeroOffset`); scrolling into and back
out of the scrollback finds and loses a known needle line exactly where
expected (`TestRenderRowsScrollsIntoScrollbackAndBack`); an offset request
past the actual scrollback length clamps down to it rather than reading
garbage or padding with blank rows mid-history
(`TestRenderRowsClampsOffsetToActualScrollbackLength`).

The wheel scrolls this only when the wheel event hit-tests onto
`hitPanelPreview` while `m.interactive` is true (so a wheel notch over the
sidebar while interactive keeps scrolling the *list*, per requirement 52's
own "scrolling never changes what deck reports" neighbour and the
existing sidebar-wheel binding, not this grid) -- otherwise interactive
mode's mouse behaviour is unchanged (click/drag/double-click over the
preview still do nothing, per the Mouse section's own contract).

Shift+PgUp/PgDn required detecting a message type Bubble Tea's pinned
v1.3.10 has no named key for at all (`KeyShiftPgUp`/`KeyShiftPgDown` do
not exist in this dependency's `key.go`; confirmed by grepping the
vendored source) -- the raw bytes (`\x1b[5;2~` / `\x1b[6;2~`) fall through
to Bubble Tea's own unexported `unknownCSISequenceMsg`, recognised here
only via its `fmt.Stringer` interface, matched against two strings built
with the exact same format Bubble Tea's own `String()` method uses
(`fmt.Sprintf("?CSI%+v?", raw-bytes-after-\x1b[)`) so a future dependency
bump breaks this visibly (a format mismatch) rather than silently.

Any other keystroke while scrolled back -- and entering or leaving
interactive mode -- resets the offset to 0, the ordinary
terminal-emulator convention that scrolled-back history is read-only and
typing snaps back to the live view; the "target has not repainted"
notice (task 067, II-49) is suppressed while scrolled back for the
inverse reason -- scrolled-back content is always genuine past output,
never evidence that nothing has happened yet.

### Coverage

`internal/interactive/scrollback_test.go` (unit, both memory-bound proofs
above plus the `RenderRows` correctness/clamping proofs) and
`features/interactive_scroll.feature` (three godog scenarios against a
real deck client and a real tmux pane: Shift+PgUp/PgDn round-tripping into
and out of 60 lines of real scrollback; the mouse wheel doing the same via
`locateText`-found coordinates over the preview panel; typing while
scrolled back snapping the view back to the live bottom) all pass,
individually and as a suite, including under `-race`
(`go test -race ./internal/interactive/... ./internal/tui/...`, both
packages race-clean).

`go build`/`go vet ./...` clean; `go test ./internal/interactive/...
./internal/tui/... ./cmd/deck/...` green; the three new scenarios in
`features/interactive_scroll.feature` pass individually and together with
the rest of `./features/...` (the suite's own pre-existing PTY-under-load
flakiness in unrelated scenarios -- reproduced identically on a clean,
unmodified checkout of this same commit -- is unrelated to this task and
not introduced by it).
