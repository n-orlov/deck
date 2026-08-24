# Phase 3b (Part II — interactive preview) findings

Findings recorded as measurements/decisions against this repository, never as
edits to SPEC.md or prds/ (`git diff` proves neither was touched by this
file's own commits). Content below is mined directly from `git log` (commit
messages and diffs), not written from memory — each section names the
commit(s) it was mined from.

## Findings, not spec edits (the PRD's own “Findings, not spec edits”
## heading, `prds/phase3c-residual-and-interactive-preview.md`) (task 073)

The PRD asks for exactly three things under this heading, answered here in
order:

**1. Every place a measurement here disagreed with the PRD's asserted
numbers.** Four, all already measured and cited in `docs/reports/phase3b.md`
(its own opening paragraph lists them together) — repeated here because this
is the file the PRD names for this class of finding:
- II-8: 4 real resizes measured (deck's own entry fit plus SIGWINCH's own
  coalescing), not the PRD's cited 5-6.
- II-27: render-coalescing measured at a 1.99x reduction here, not the
  spikes' cited 5.11x.
- II-36: tmux's own internal command-string ceiling measured at
  16340-16360 bytes / 5445-5455 `-H` args, not the PRD's cited 16380/8192 —
  shown above (same underlying ~16 KiB ceiling, two argument-width
  assumptions, not two limits).
- II-51: one empty 120x40 grid measured at ~5.9 MiB resident over an
  empty-process baseline, not the PRD's cited ~53 MiB.

Every one of the four is attributed, per `tasks.json`'s own
`discovered.prdCorrections` record, to the 2026-08-22 spikes' raw evidence
being unreachable from this container (confirmed again for this task: `ls
~/deck-spikes` and a filesystem-wide `find` both come up empty here, exactly
as task 022 already recorded) — not to a different algorithm running here
versus there.

**2. Whether a REAL agent repaints its full transcript on widening — "the
single most valuable measurement this phase can add."** This is SPEC.md's
own open question 10 (`## 14. Open questions`, SPEC.md:1743-1746:
"Do real agents repaint their full transcript on widening? ... Unmeasured
against a real agent: every spike was fenced from launching one."), still
open at HEAD. Not measured here either; recorded explicitly rather than
guessed, per the PRD's own fallback wording ("whether ... it could not be
measured"). Checked directly before writing this down, not assumed: no real
`claude` binary is on `PATH` in this
container (`which claude` empty; `ci/Dockerfile` installs Go and tmux only,
no agent CLI) — the one place this repository already gates a real-agent
run, `features/real_agent_smoke.feature`/`fake_agent_drift.feature`
(`@real-agents`, skipped by `features/godog_test.go`'s own
`defaultTags = "~@real-agents && ~@nightly"` unless a caller opts in), only
ever covers Claude, never a real `pi` CLI at all — there is no
`installedPiIsAvailable`-shaped step anywhere in `features/`. A `pi` binary
is on `PATH` in this container (`/usr/bin/pi`, resolving to
`/usr/lib/node_modules/@earendil-works/pi-coding-agent/dist/cli.js`), but it
is this run's own coding-agent harness (the tool executing this very task),
not a fixture with any real-agent godog coverage in this repo, and standing
up a first-of-its-kind harness to launch it recursively as a deck-managed
target and drive a widen-then-inspect-transcript probe through it would be a
new, non-trivial harness addition — out of scope for measuring, not building,
and risking recursive interference with the very process running this task.
Recorded as unmeasured, with the reason, rather than fabricated or silently
dropped. Requirement 49's announcement (task 067/II-49, this file's own
II-41/II-42 section below and `docs/reports/phase3b.md`) therefore stays what
the PRD calls it absent this measurement: load-bearing *if* real agents do
not repaint on widening, defensive if they do — this phase cannot say which.

**3. Whether ~53 MiB per gridded pane is acceptable, noting deck has no
memory budget to judge it against.** This is SPEC.md's own open question 9
in the same section (SPEC.md:1740-1742: "Is ~53 MiB of resident memory per
gridded pane acceptable? ... deck has no memory budget to judge it against,
and it is the one axis on which §11.9's grid is materially worse than
polling"), also still open at HEAD — SPEC.md poses the question in its own
words, it does not answer it. Judgement, not a fix: `grep -rn
"memory.*budget\|MiB.*limit\|MaxMemory" internal/` (excluding SPEC.md/prds/,
which only restate the open question itself) finds nothing — this
repository defines no per-pane or aggregate memory ceiling anywhere in its
own code, config, or docs/reports/, so "acceptable" cannot be checked
against a stated number the way II-36's byte ceiling was. What can be
stated: (a) the PRD's own non-goal list ("Grids
for more than the selected session") already bounds the worst case to ONE
grid at a time, not N sessions' worth, so the real question is whether one
pane's cost is acceptable, not whether it multiplies; (b) the measured cost
here (~5.9 MiB baseline, II-51) is an order of magnitude below even the
PRD's own higher cited figure, so if the PRD's authors judged ~53 MiB
tolerable enough to cite as a working number rather than a stop condition,
the actual, smaller, measured cost clears that same bar without needing a
formal budget to compare against; (c) 28 MiB more for a full 2000-line
scrollback (task 068/II-51, `docs/reports/phase3b.md`) is the one number
that could plausibly matter at scale (many sessions, all left with a full
scrollback), and that number — not the empty-grid baseline — is the one a
future operator setting an actual budget should size against. Recorded as a
judgement call per this task's own successCriteria, not as a defect to
correct.

## II-5: DECK_INTERACTIVE_TRANSPORT is a transport selector, not a §13.1
## behaviour switch (task 031)

`interactive_transport` (`DECK_INTERACTIVE_TRANSPORT=pipe|capture`,
`internal/config/schema.go`) chooses between the two mechanisms Part II's
spikes measured for streaming a pane's live output into deck's grid:

- `pipe` (the default): arm tmux's `pipe-pane -IO` into a long-lived
  vt/x grid (II-16 onward).
- `capture`: poll and re-run `capture-pane` on a timer instead.

This is a selector between **two implementations of the same contract** —
§11.9's interactive-preview panel — not the kind of user-visible behaviour
switch §13.1 forbids: §11.9 never specifies HOW a transport keeps history,
so a selector between two implementations of that one contract is in scope
for this knob (this is the stated §13.1 position for `interactive_transport`,
not a fallback argument underneath a stronger claim). There is no literal
`features/interactive_preview.feature` file — the contract's surface spans
seven files (`interactive_focus.feature`, `interactive_geometry.feature`,
`interactive_option_tables.feature`, `interactive_refusals.feature`,
`interactive_repaint_notice.feature`, `interactive_scroll.feature`,
`interactive_sigwinch_budget.feature`, 16 `Scenario:` lines/187 steps total—
named precisely in `docs/reports/phase3b.md`). The parity result, as
measured (tasks 088/089, below): 12 of these 16 scenarios pass unchanged
under `DECK_INTERACTIVE_TRANSPORT=capture`; the other 4 — all of
`interactive_scroll.feature` — fail for a real, structural reason, not
merely because they fail. `§11.9 does NOT extend the contract to scrollback
depth or history accumulation` (below) is the scope this knob actually has;
it is narrower than "every other observable ... must hold under either
value of the knob" (this section's own earlier, now-stale framing) — that
framing named only the pipe-only II-33 exception and never anticipated this
second, structural one, which the parity run itself discovered.

Declaring the key (this task) does not implement either path; `pipe` is
implemented starting at task 040, `capture` and the parity run are task
070's job.

### Task 070 split: `capture` did not exist in code at all until now (task 088)

When task 070 was picked up, grepping for `InteractiveTransport` outside
`_test.go` files showed the knob fully wired end to end -- settings UI
(`internal/tui/settings.go`), the config file reader/writer
(`internal/config/toml.go`/`toml_write.go`), env override validation
(`internal/config/config.go`'s `interactiveTransportEnv`) -- but never
consulted anywhere a Session is actually constructed
(`internal/tui/interactive.go`'s one `interactive.Start` call site). `pipe`
was the only implementation that existed; `capture` was a validated,
settable, displayed value with no code path behind it at all. Proving
"the same scenarios pass with `DECK_INTERACTIVE_TRANSPORT=capture`" was
therefore not a test-writing task, it was an implementation task the PRD's
own wording ("capture ... are task 070's job", above) already flagged --
too large for one iteration alongside everything else task 070 asks for, so
it was split into three tasks in `tasks.json`: 088 (implement the transport
itself, in `internal/interactive`, with its own unit tests), 089 (wire it
into `internal/tui/interactive.go` and run/log/document the parity sweep),
and 070 itself, now the closing sign-off once 089's evidence exists.

**Task 088's implementation** (`internal/interactive/grid.go`): a new
`Transport` type (`TransportPipe`/`TransportCapture`) and
`StartWithTransport(ctx, client, target, width, height, seed, transport)`,
with `Start` kept as a thin `StartWithTransport(..., TransportPipe)`
wrapper so every pre-088 call site (one production, ~14 in tests) keeps
working unchanged. Under `TransportCapture`, `ArmPipePane` is never called
at all (`s.pipe` stays `nil` for the Session's whole life -- `markDead` and
`Close` gained nil guards, confirmed load-bearing by a red demonstration:
forcing the pipe to arm anyway, and separately removing `Close`'s nil
guard, both tried and reverted, `git diff` empty before commit) and a new
`captureLoop` goroutine replaces `drain`/`fallbackLoop`: it polls
`CaptureSeed` every `capturePollInterval` (a var, 200ms default, mirroring
`paneDeadPollInterval`'s own test-overridable shape) and replaces the grid
wholesale each tick, reusing the exact fresh-parser-per-reseed discipline
`fallbackLoop`/`Resize` already established (task 044/II-21-22) rather than
inventing a second one. `pollPaneDead` (II-23) is fully transport-agnostic
and runs unmodified under either transport. `Status` never leaves
`StatusLive` under `TransportCapture`, since `handlePipeGone` (the only
thing that ever moves it) is unreachable with no pipe armed to displace.

**Gotcha discovered building the tests**: `CaptureSeed`
(`internal/tmux/paneseed_atomic.go`'s `CapturePaneSeedAtomic`) rejects any
target that is not the exact `^%[0-9]+$` pane-id shape `Dispatcher` already
enforces (PRD II-30) -- unlike `ArmPipePane`/`capture-pane -p`, which
accept an ordinary session/window target string (e.g. `s0`) fine. A first
draft of `capture_transport_test.go` passed `s0` to `StartWithTransport`
directly and captureLoop silently failed every single poll (`CaptureSeed`'s
error swallowed by design, per its own doc, so the panel never regresses on
a transient tmux hiccup) -- confirmed by resolving `#{pane_id}` first and
re-running, which fixed it. Production is already correct here
(`interactive.Start(ctx, client, pane.ID, ...)`, always a real pane id);
this was a test-only bug, not a production one.

Verified for task 088: `go build`/`go vet`/`gofmt` clean; `go test -race
-count=1 ./internal/interactive/...` green (84.6s); `go test -count=1
./internal/... ./cmd/...` green; the five new tests in
`capture_transport_test.go` demonstrated non-vacuous by two separate red
demonstrations described above, both reverted before commit (`git diff`
empty). Task 088 does **not** wire this into `internal/tui` or run any
godog scenario under `capture` -- that is task 089's job, followed by 070's
own sign-off against this exact PRD wording.

### Task 089: wired into `internal/tui`, parity run, and a real gap the transport selector's own doc did not anticipate

`internal/tui/interactive.go`'s `enterInteractive` now reads
`m.settings.InteractiveTransport` (already fully wired end to end per this
section's own earlier discovery -- config file, env override, settings UI --
just never consulted here) and calls `interactive.StartWithTransport(...,
interactive.TransportCapture)` when it is `"capture"`, `TransportPipe`
otherwise. `internal/config`'s `interactiveTransportEnv` already rejects
any value other than `"pipe"`/`"capture"` at load time, so no third case
can reach this call site.

The parity run (`docs/reports/phase3b-interactive-transport-pipe.log` /
`-capture.log`, `DECK_INTERACTIVE_TRANSPORT=pipe|capture go test -v
-count=1 -run <subset> ./features/`, restricted to the seven files II-5's
own wording names -- there is no literal `interactive_preview.feature`)
found genuine parity on 12 of 16 scenarios, and a genuine, structural gap
on the other 4 -- all of `interactive_scroll.feature` (II-51/task 068).

**The gap, precisely**: `TransportCapture`'s `captureLoop` (task 088) calls
`CaptureSeed` -> `CapturePaneSeedAtomic` -> `capture-pane -p` (no `-S`) and
writes the result into a **brand-new** `Grid` every `capturePollInterval`
tick (`fresh := newGrid(...)`, `grid.go`), replacing whatever the previous
tick held outright. `vt`'s own scrollback (II-51's `ScrollbackMaxLines`)
only grows from a `Write` call being handed bytes that push existing rows
up and off the visible grid -- under `TransportPipe` this happens
continuously, every byte the pane emits, so anything that scrolls off the
visible area on its way past still lands in scrollback first. Under
`TransportCapture` there is no such stream: each tick sees only the pane's
current visible rows, and anything that scrolled off-screen **between**
two polls is gone -- never written anywhere, let alone into a scrollback
buffer that no longer even exists once the tick's fresh grid replaces the
old one. A capture Session therefore has no meaningful scrollback at all
(what little it has is whatever the current tick's single `capture-pane`
snapshot happened to include, discarded on the very next tick regardless
of whether it was ever scrolled through), and every one of
`interactive_scroll.feature`'s four scenarios -- Shift+PgUp/PgDn,
mouse-wheel, typing-snaps-back, and the badge/probe-during-scroll check --
depends on scrollback actually accumulating.

This is neither II-33 (peeling a trailing `;` off a literal `send-keys`
payload -- meaningless under `capture` because it never calls `send-keys`
for output) nor II-24 (the pipe displacement-vs-death distinction --
meaningless with no pipe armed to displace); it is a third class this
section's own earlier text did not anticipate when it said "every other
observable ... is a property of §11.9's contract, not of the transport" --
scrollback depth turns out NOT to be one of those, because §11.9 never
specifies HOW a transport keeps history, and `capture-pane`'s own default
behaviour (visible-only, no `-S`) makes accumulating any is structurally
impossible for a poll-and-replace design without a scrollback mechanism of
its own. Recorded here as a genuine, permanent contract gap between the
two transports, not something a faster poll or a bigger `-S` window closes
fully (a `-S -2000` capture would recover SOME history per tick, but still
not the byte-exact accumulation `TransportPipe` gets from streaming, and
changing `CaptureSeed`'s own tmux invocation is out of this task's scope --
it is shared with the seed-capture path task 041/II-19 already measured).

Verified for task 089: `go build`/`go vet`/`gofmt` clean; `go test -count=1
./internal/... ./cmd/...` green; the wiring change is the minimal one
(transport selection only, no change to `interactive.StartWithTransport`
itself, which task 088 already covered under `-race`).

### Decision (steer 013 item 2): capture's inert scrollback keys are an
### accepted, deliberate limitation, not a gap to fill

Under `DECK_INTERACTIVE_TRANSPORT=capture`, the scrollback keys task
068/II-51 added (mouse wheel, Shift+PgUp/PgDn) are silently inert: they
still run their key/event handling in `internal/tui`, but with no
scrollback ever accumulating (the mechanism section above), there is
nothing for them to scroll into, and no UI notice tells the operator why
nothing happened. This is deliberate, not an oversight, and no new
UI-notice task is created for it: `DECK_INTERACTIVE_TRANSPORT` defaults to
`pipe` (`internal/config/schema.go`, task 031), where the keys work exactly
as task 068 built them; `capture` is an opt-in diagnostic/fallback value a
user reaches only by deliberately setting the knob, at which point the
scrollback gap above is already the documented, structural cost of that
choice. Recorded here as fact so a future reader does not mistake silence
for a bug.

### Task 070 sign-off against its own literal wording

Task 070's own `successCriteria` text (written before 088/089 existed)
names two acceptable reasons for excluding a scenario under `capture`:
"II-33/II-24". The actual reason `interactive_scroll.feature`'s four
scenarios fail is neither of those two — it is the third, structural
scrollback-accumulation gap this section documents above. Read hyper-
literally ("a reason II-33/II-24 names"), the exclusion does not match
either named example. Read against the clause that actually carries the
intent — "no scenario is excluded merely because it fails" — it does: the
reason is real, mechanistic, checked against both named candidates and
explicitly ruled out before being written up as a new class, and recorded
honestly in both this file and `docs/reports/phase3b.md`'s II-5 section
rather than dropped from the tag set quietly. Task 089's own
`successCriteria` (written at the same split, with foreknowledge that a
third class was possible) says exactly this in so many words: "If some
other class of scenario turns out not to hold under capture for a reason
neither II-33 nor II-24 covers, that is a finding to record honestly ...
not a scenario to quietly skip." Task 070 is signed off on that basis: the
letter of its two named examples is stale (it predates the discovery), but
the governing clause it exists to enforce holds.

### Fact, no action required: the capture-transport parity sweep is a
### manual one-off, not part of the delivery suite

The parity run cited above (tasks 088/089) is `DECK_INTERACTIVE_TRANSPORT=
capture go test -run <subset> ./features/`, run by hand once and logged
(`docs/reports/phase3b-interactive-transport-{pipe,capture}.log`) — it was
never wired into `ci/run.sh`, `ci/stability.sh`, or either of tasks 075/077's
final CI runs, all of which run with `DECK_INTERACTIVE_TRANSPORT` unset
(defaulting to `pipe`). Task 088's five unit tests
(`internal/interactive/capture_transport_test.go`) are the only standing
regression cover `capture` has going forward. Neither this file nor
`docs/reports/phase3b.md` (which states the same fact directly above its
own II-5 section, task 089) may be read as implying the delivery suite
exercises both transports on every run — it exercises `pipe`, plus this one
manually-run, logged snapshot of `capture`.

## II-23/II-24: five gotchas that live only in commit messages, mined from
## `git log` for this task (073)

The PRD asks for these named explicitly if not already recorded elsewhere;
they were not, at HEAD, before this task. All five are on the pipe
transport's own drain path (`internal/tmux/pipe.go`,
`internal/interactive/grid.go`), mined from commits `6490a6d` (II-24) and
`8d7c2c7` (II-23) rather than restated from memory:

1. **The EOF that could not happen.** `PanePipe`'s original reader opened
   its FIFO end `O_RDWR` (a common non-blocking-open trick), which makes
   deck's own descriptor count as a writer on that FIFO — so `read(2)`
   never returns `0`, even once tmux's own `pipe-pane` job on the other end
   is long gone. Demonstrated directly, not assumed: a throwaway `O_RDWR`
   reader pointed at a displaced pipe just times out instead of ever
   seeing EOF. Fixed via the standard one-open convention, `O_RDONLY|
   O_NONBLOCK`, which lets a real EOF surface once no other writer remains.
2. **`#{pane_pipe}` flips true before the forked job has actually opened
   the FIFO.** tmux reports the option as set the instant it *accepts* the
   arm command, not once the job it just forked has reached its own
   `open()` of the FIFO — so a read immediately after arming can see a
   transient zero-writer state that looks exactly like a genuine EOF, at
   startup only. `waitForFifoWriter` polls (EAGAIN vs. immediate-EOF) to
   positively confirm a real writer exists before `ArmPipePane` returns,
   and buffers (`PanePipe.leftover`) any bytes it happens to consume while
   probing so the drain still delivers them, in order, exactly once.
3. **`os.NewFile` only poller-registers a descriptor that is *already*
   non-blocking at the moment it is wrapped.** Clearing `O_NONBLOCK`
   before handing the fd to `os.NewFile` (a plausible-looking
   simplification) produces a `Read` the Go runtime's poller cannot
   interrupt — silently breaking `Close`'s own unblock-a-blocked-`Read`
   contract (task 045). The fd is kept `O_NONBLOCK` for the `PanePipe`'s
   entire life instead, specifically because of this.
4. **`Close`'s own disarm produces the identical `io.EOF` an external
   displacement or disablement does** — tmux's `pipe-pane` job process
   exits the same way either way, so the error's *type* alone cannot tell
   an intentional shutdown apart from one that needs investigating.
   `PanePipe.WasClosed()` is the actual discriminator the drain path checks
   *first*, before ever inspecting the error itself.
5. **`remain-on-exit=failed` (deck's own server-wide `Bootstrap` default)
   keeps a dead pane's `pipe-pane` job — a bare `cat` still writing into
   the FIFO — running forever.** `drain`'s `read(2)` therefore never sees
   EOF on a dead pane at all, and would otherwise block indefinitely with
   the grid stuck rendering a stale frame and no signal that anything is
   wrong. Liveness has to become a poll (`pollPaneDead`, ticking
   `#{pane_dead}` every `paneDeadPollInterval`) rather than something
   inferred from the stream itself.

## II-14: ownership has no heartbeat and no TTL, because liveness is a
## syscall (task 033)

`internal/tmux/ownership.go`'s `ClaimWindowOwnership`/`Release` claim the
window-scoped `@deck_isize_owner` option as `<tag>:<pid>` before interactive
mode acts on a tmux window, exactly as the PRD specifies: read first (a
live owner, validated with a `kill(pid, 0)`-equivalent probe —
`os.FindProcess` + `Signal(syscall.Signal(0))`, the same probe
`internal/store/lease.go`'s `leaseOwnerAlive` already uses for the launch
lease — is respected untouched, never written over); write this process's
fresh claim only when the option is unset or its owner is confirmed dead;
re-read (the confirm-read) to catch a competing writer that landed between
the write and the read, looping back to a fresh read (which validates that
writer's liveness in turn) rather than trusting the write blindly.

There is deliberately no heartbeat and no TTL anywhere in this file —
`grep -n "time.Sleep\|time.Tick\|time.NewTicker\|time.NewTimer\|time.After("
internal/tmux/ownership.go` returns nothing. A lease-style TTL exists to
cover the case where the holder and the checker cannot directly ask each
other "are you still there" — e.g. across a network, or across a reboot
where a pid could be reused by an unrelated process. Neither applies here:
every writer of a tmux socket is a process running on that socket's own
host, in the same PID namespace as deck itself, so "is the owner still
alive" is exactly the same `kill(pid, 0)` syscall the launch lease already
uses, answerable synchronously and for free, with no window during which a
heartbeat could go stale and no clock to skew. The launch lease's own extra
boot-id check exists for a lease that can span a reboot; an interactive-mode
ownership claim's lifetime is at most one deck process's one call to
`ClaimWindowOwnership`, never that long-lived, so that check is not carried
over here.

Tests (`internal/tmux/ownership_test.go`, run against real tmux via
`ci/run.sh go test ./internal/tmux/...`) cover all three protocol cases the
task names, plus the race itself: a live preset owner is respected with the
option left untouched; a preset owner tagged with an unused pid (999999999)
is stolen from; and two goroutines calling `ClaimWindowOwnership`
concurrently against the same unclaimed window (run with `-race`) produce
exactly one winner and one stand-down, with the option left holding exactly
the winner's claim afterwards — proving the confirm-read is what resolves
the race rather than either side's own write being trusted blindly.

## II-16: `gridContains`'s test helper raced `CellAt`'s own contract (task 085,
## operator steer 009)

Task 045 (II-16's arm-before-vs-after-seed-capture proof) noted a pre-existing
data race in `internal/interactive/grid_test.go`'s `gridContains` helper,
newly exposed (not caused) by its own second drain goroutine, and deliberately
left it unfixed at the time — out of that task's scope. Steer 009 asked for
it to be resolved deliberately rather than left to evaporate with the run
directory. It is fixed here, in the test helper only; no production code
changed.

**Mechanism.** `charmbracelet/x/vt`'s `SafeEmulator.CellAt`
(`safe_emulator.go:59`) takes its `RLock`, calls the embedded `Emulator`'s
`CellAt`, and returns — before the caller ever touches the result:

```go
func (se *SafeEmulator) CellAt(x, y int) *uv.Cell {
	se.mu.RLock()
	defer se.mu.RUnlock()
	return se.Emulator.CellAt(x, y)
}
```

What it returns is a **pointer into the emulator's live cell array**. Every
`cell.Content` read the old `gridContains` performed happened *after* that
`RLock` was already released on return, racing any concurrent `Write` to the
same grid (the drain goroutine, in the failing test). `ScrollbackCellAt`
(`safe_emulator.go:199`) has the identical shape and is not currently used by
any test helper, but would have the same hazard if it were. This is an
upstream API contract issue, not something in this repository's control to
fix directly — it is worked around here by never calling `CellAt` from a
test that races a live `Write`.

**Fix.** `gridContains` now goes through `SafeEmulator.Render()`
(`safe_emulator.go:45`), which holds the `RLock` across the *entire* encode
and returns a plain `string` — race-free by construction, and the only vt
accessor with that property. (`SafeEmulator.String()` looked like a second
candidate but is not: `SafeEmulator` embeds `*Emulator` and never overrides
`String`, so calling it calls straight through to the unsynchronized
encoder — it is not actually safe despite living on a type named
`Safe`Emulator.)

`Render()`'s rows are separated by a bare `\n`
(`charmbracelet/ultraviolet`'s `Lines.Render`), so `gridContains` splits on
that and searches row by row exactly as before — never a single `Contains`
over the whole encoded screen, which could let a needle match across an
artificial line-wrap join that was never contiguous on screen. Each row is
run through a small ANSI-escape-stripping regexp before the search, because
`Render()` only emits an SGR/OSC sequence when a cell's style or link
actually changes (`ultraviolet/buffer.go`'s `renderLine`), so an escape can
land in the middle of what was contiguous plain cell content — exactly the
"positive assertion silently stops matching" trap steer 009 named as the
risk of this exact class of rewrite.

**Non-vacuousness, checked directly, not assumed:**
- `TestGridContainsFindsPlainAndStyledTextButNotAcrossRows` (new) proves the
  rewritten helper still finds plain text, still finds text whose row
  `Render()` breaks with a real SGR escape (a styled word bracketed by plain
  text — the escape-stripping regexp is proven load-bearing by temporarily
  reverting it and watching this exact case fail with the predicted message,
  then reverting), and still returns `false` for text that is genuinely
  absent, including a needle built by gluing one row's tail to the next
  row's head (which must NOT match).
- All twelve pre-existing `gridContains` call sites across four files
  (`grid_test.go`, `resize_test.go`, `displacement_test.go`,
  `composed_only_test.go`) — both positive and negative assertions — still
  pass: `go test -count=1 ./internal/interactive/...` green.
- The originally-failing test,
  `TestArmingPipeBeforeSeedCaptureDeliversInterstitialBytes`, plus its
  sibling and the new helper test, ran clean 10/10 under
  `go test -race -count=1` (previously nondeterministic — the bug needed
  several `-race` reruns to surface even before the fix, so a single green
  run proves nothing; ten were run instead).

**Separately discovered, not fixed here (out of this task's scope, recorded
so it does not evaporate either):** `TestSessionResizeDuringLiveDrainIsRaceFree`
(`resize_test.go:199`) failed once in six whole-package `-race` runs with a
`t.Fatalf` from a background goroutine (`sendLiteralLine`, a real `send-keys`
subprocess call), not a `WARNING: DATA RACE` report — a synchronization gap,
not a data race: the test's background goroutine is signalled to stop via
`close(stop)` but is never joined (no `<-done`), so it can still be mid-flight
against a socket the test's own `defer cleanup()` is concurrently tearing
down. Reproduces only under load/`-race`'s slowdown; ten isolated reruns of
just that test were all green. A future task fixing it should join the
goroutine (e.g. a `done` channel closed by the goroutine itself, waited on
before `cleanup()` runs) rather than touching `gridContains` or anything
this task changed.

## II-34: tmux's Home/End escape translation is faithful to a real attach
## (task 056)

`send-keys Home`/`send-keys End` and a real attached client typing the
same physical key produce the identical vt220 escape --
`ESC[1~`/`ESC[4~` -- confirmed directly against a real tmux 3.5a server
(`internal/tmux/key_test.go`'s `TestHomeAndEndEscapesMatchARealAttachedClientTypingTheSameKeys`):
tmux performs no retranslation of raw client input that does not match
one of its own key bindings, it forwards what the client sent straight
to the active pane, so `send-keys`'s translation is the same standard a
real attach goes through, not a separate, merely-self-consistent
convention deck invented. This is what makes it safe for
`Dispatcher.SendNamedKey` to rely on `send-keys <Name>` for every named
key interactive mode needs, rather than hand-encoding the escape itself.

`capture-pane -p` renders a stored raw ESC control byte as the
two-character caret notation `^[`, not the literal `0x1b` byte --
confirmed by hexdumping its output directly while building this task
(`5e 5b 5b 31 7e` = `^`, `[`, `[`, `1`, `~`). Any future test in this
package comparing against `capture-pane` output for a control character
should compare against that rendered form, not assume the raw byte
survives capture-pane's own text rendering.

## II-36: the PRD's `-H` ceiling ("8192 args") does not reproduce with valid
## hex bytes; the `-l --` ceiling ("16380 bytes") does (task 057)

PRD II-36 states two numbers for tmux's own internal command-string length
ceiling: `send-keys -l --` fails around 16380 bytes, `send-keys -H` fails
around 8192 arguments. `internal/tmux/chunk_test.go` bisects both directly
against the tmux 3.5a binary `ci/Dockerfile` installs, using ONLY valid
two-hex-digit `-H` arguments (an invalid one, e.g. `zz`, is silently
discarded instead of erroring — proven separately by
`literal_send_test.go`'s pre-existing
`TestSendKeysInvalidHexByteIsSilentlyDiscarded` — and would prove nothing
about a length ceiling).

**Measured here:**
- `-l --`: 16340 bytes succeeds, 16360 bytes fails with `command too long`.
  This confirms the PRD's "16380" figure closely — the real crossover sits
  right around it.
- `-H`: 5445 valid two-hex-digit arguments succeed, 5455 fail with
  `command too long`. This does **not** confirm the PRD's "8192" figure —
  the real crossover measured here is roughly two thirds of that.

**Why the two are still the same underlying limit.** 5450 two-character hex
tokens, each with one separating space, is itself ~16.3 KiB of internal
command-string content — matching the `-l --` crossover almost exactly. A
bisection run with single-CHARACTER (not two-character) `-H` tokens during
this same investigation crossed over right around 8190-8192 arguments,
which is consistent with the PRD's stated figure IF the argument width
assumed during whatever measurement produced it was one byte per argument
plus a separator, not tmux's actual minimum of two hex digits per `-H`
argument. Both figures are almost certainly the SAME ~16 KiB ceiling,
observed through two different argument-width assumptions — not two
independent limits. This repository's own tests and chunk size
(`hexChunkArgs = 4096` in `internal/tmux/send.go`) use the measurement made
here (real two-digit hex arguments), not the PRD's approximate figure,
since 4096 is comfortably under BOTH numbers regardless of which is used.

**Not ARG_MAX, and not a single-argument OS limit either.** A 100000-byte
single literal argument (six times the size that already fails against
tmux) still fails with the identical `command too long`, confirming the
ceiling belongs to tmux, not the OS's `ARG_MAX`. A MUCH larger single
argument (1,636,000 bytes) was tried while building this task and instead
failed before tmux ever ran at all, with Go's own `fork/exec ...: argument
list too long` — Linux's `MAX_ARG_STRLEN` (512 KiB per single `execve`
argument), a real but DIFFERENT ceiling that this task's tests deliberately
stay well clear of (100000 bytes), since hitting it would prove nothing
about tmux's own internal limit.

**Also recorded here per steer 009 item 2 (task 086), since this task's own
git history is itself evidence for future mining:** this finding is the
kind task 072/073 (docs/reports/phase3b.md, phase3b-findings.md's own
final drafts) are expected to mine directly from `git log`, not restate
from memory — see this commit's own message and `internal/tmux/chunk_test.go`'s
header comment for the reproduction steps in full.

## II-41/II-42: Ctrl+Enter cannot be bound in Bubble Tea v1.3.10; Ctrl+Q
## survives interactive mode only because raw mode clears IXON (task 061)

Task 061 rebinds `\u21b5` to enter §11.9 interactive mode, adds `a` as the
full-attach key `\u21b5` used to be, and binds Ctrl+Q as the only way back out.
Ctrl+Enter was investigated as a *fourth* binding (a natural "attach and
skip interactive" chord) and is deliberately NOT bound anywhere in this
repository. It is not a gap left for a later task -- there is no way to
bind it at all, for three independent reasons stacked on top of each
other, any ONE of which alone would already rule it out:

1. **Bubble Tea v1.3.10 cannot decode it.** `internal/tui` receives every
   keypress as a `tea.KeyMsg`; a modifier combination the decoder does not
   recognise instead arrives as `tea.KeyMsg{Type: tea.KeyRunes, ...}` for
   something it partially matched, or as the package's own
   `unknownCSISequenceMsg([]byte)` type
   (`charmbracelet/bubbletea@v1.3.10/key.go:544-548`) for a CSI sequence it
   does not recognise at all -- and that type is **unexported**
   (lowercase `unknownCSISequenceMsg`, confirmed by reading key.go directly
   out of the module cache): `internal/tui` cannot even name the type in a
   `case` to write a handler for it, regardless of what bytes a terminal
   that DOES emit a distinct Ctrl+Enter sequence (e.g. a Kitty-protocol- or
   CSI-u-aware terminal) would send.

2. **Even if it decoded, it would be indistinguishable from plain Enter.**
   Bubble Tea's own `KeyType` constants are the raw C0 control byte values
   (`key.go:134-152`), and `KeyCtrlM` -- literally named as Ctrl+M, the
   traditional name for the Enter/Return control code -- is defined as
   `KeyType = keyCR` (`key.go:181`), the exact same value as `KeyEnter`
   (`key.go:163`, `KeyEnter KeyType = keyCR`). A terminal that sends plain
   `\r` for Ctrl+Enter (which is what "Ctrl+Enter" has always meant on a
   real keyboard/terminal without an extended keyboard-protocol
   negotiation) is indistinguishable from plain Enter at the `KeyType`
   level by construction, not by a decoding gap.

3. **tmux itself flattens it before it would ever reach send-keys.**
   Confirmed directly against a real tmux 3.5a server in this task (not
   copied from a manual page, matching II-34/II-35's own standard):
   ```
   tmux -L t new-session -d -s t "python3 -c 'import sys,tty; tty.setraw(0); open(\"/tmp/out\",\"wb\").write(sys.stdin.buffer.read(2))'"
   tmux -L t send-keys -t t C-Enter
   tmux -L t send-keys -t t Enter
   ```
   both land as the identical two bytes `0d 0d` on the pane's own raw
   (ICRNL-disabled) stdin -- tmux's own key-name table has no distinct
   encoding for "C-Enter" from plain "Enter" by default, so `send-keys`
   cannot emit anything an attached program could tell apart, UNLESS the
   user's own tmux config sets `extended-keys always` (off by default,
   confirmed via `show-options -g extended-keys` above), which changes
   tmux's OWN CSI-u reporting for modified keys server-wide -- a setting
   deck does not control and must not silently assume, since flipping it
   changes every OTHER key's encoding for every attached client on that
   server, not just this one binding.

Any one of these three closes the door; together they mean Ctrl+Enter is
not a binding this repository chose to skip, it is one Bubble Tea
v1.3.10 plus a default tmux server cannot express end to end. Recorded
here rather than retried in a later task.

**Ctrl+Q's own survival is a different, narrower mechanism, and worth
recording precisely because it looks like it should fail the same way.**
Ctrl+Q is DC1/XON (byte `0x11`), the same byte a cooked (non-raw) tty's
line discipline uses for software flow control: with the termios `IXON`
flag set (the default for a freshly-opened tty), the kernel tty driver
itself intercepts `^S`/`^Q` to pause/resume output and never delivers
either byte to the reading process's `read()` call at all -- a program
running in a plain cooked terminal that binds Ctrl+Q simply never sees
it, no decoding layer involved. Bubble Tea's `tea.Program.Run` puts the
terminal into raw mode before reading any input (`golang.org/x/term`'s
`MakeRaw`, which clears `ICANON`, `ECHO`, `ISIG`, `ICRNL` and `IXON`
together as one termios state, not individually), so for the entire time
deck's own program is running, `IXON` is already off and Ctrl+Q reaches
`Update` as a completely ordinary `tea.KeyMsg` like any other control
byte -- interactive mode's own Ctrl+Q binding does not need to do
anything special to "unlock" flow control, it is simply never engaged
while Bubble Tea owns the terminal. The exception this is worth stating
plainly: outside deck (a genuinely cooked tty, or a terminal that has
somehow not gone through Bubble Tea's raw-mode setup), the same byte is
ordinary XON and would be swallowed before ever reaching an
application-level handler.

## Steer 011: the footer legend at the minimum supported size (80x24) is
## budget-cut, measured, not reasoned about (task 073)

`internal/tui/tui.go`'s `footerLegend` currently holds **13** entries (not
the fourteen an earlier note about this named — recounted directly by
`grep`, not carried over uncorrected):
`↑/↓`, `↵ interactive`, `a attach`, `Y acknowledge`, `n new`, `x kill`,
`r resume`, `R relaunch`, `P profile`, `p pin`, `i detail`, `? help`,
`q quit`. The checked-in golden frame for the minimum supported size
(`features/testdata/golden/side_by_side_80x24.golden`, line 24, generated
by a real deck binary through a real PTY at exactly 80x24) shows the
footer line cut after:

```
starting - awaiting signal    up/down - Enter interactive - a attach - Y acknow
```

Measured against the 13-entry list above: the first three entries render in
full; the fourth (`Y acknowledge`) is cut mid-word, showing only `Y acknow`;
the remaining **nine** entries — `n new`, `x kill`, `r resume`,
`R relaunch`, `P profile`, `p pin`, `i detail`, `? help` and `q quit` — are
entirely absent from the line, including the two an earlier note called out
by name (`q quit`, `? help`). Counting the truncated entry alongside the
nine fully-missing ones, **ten of the thirteen** entries are not fully
present on screen at 80x24 — the fraction an earlier note stated is
confirmed, its stated total (fourteen) is not; thirteen is what `grep -c`
against the current `footerLegend` literal returns.

**Before/after task 061's rebind, measured from `git show 0153a28`, not
reasoned about:** the pre-061 golden line (`git show 0153a28 --
features/testdata/golden/side_by_side_80x24.golden`) read
`up/down - Enter attach - Y acknowledge - n new -`, cut immediately before
`x kill` — i.e. the line was *already* cut at that same class of boundary
before task 061 touched anything, with `Y acknowledge` shown whole and
`n new` fully visible. Task 061 inserted the new `a attach` entry ahead of
`Y acknowledge` (shifting everything after it by exactly that entry's
width) and reworded `Enter attach` to `Enter interactive`; the net effect,
measured by diffing the two golden lines directly, is that `n new` — fully
visible before — is now entirely off the end, and `Y acknowledge` — also
fully visible before — is now cut mid-word. Task 061 marginally worsened a
pre-existing condition (the line was already budget-cut before it landed);
it did not create the cutting itself.

**Whether the tail is truncated or would instead wrap in a wrapping
emulator — measured from the dependency's own source, not inferred from
behaviour:** `github.com/charmbracelet/bubbletea@v1.3.10`'s
`standard_renderer.go` (read directly from the module cache via
`ci/run.sh`) truncates every line before it is ever written to the
terminal:

```go
// Truncate lines wider than the width of the window to avoid
// wrapping, which will mess up rendering. If we don't have the
// width of the window this will be ignored.
if r.width > 0 {
    line = ansi.Truncate(line, r.width, "")
}
```

— unconditional whenever the renderer knows the terminal width (which it
always does once the first `tea.WindowSizeMsg` arrives, `r.width = msg.Width`
at the same file's line 632), specifically so the *real* terminal's own
autowrap never fires for a line bubbletea produces. This is confirmed, not
merely plausible from the comment alone: `charmbracelet/x/vt`'s `Emulator`
(the library every golden/PTY test in this repo captures through) defaults
`ansi.ModeAutoWrap` to **on** (`mode.go`, `ansi.ModeAutoWrap: ansi.ModeSet`)
— if bubbletea's own line were ever written un-truncated past column 80 on
the footer's row (the emulator's last row, with no row 25 to wrap onto),
a real autowrap-driven terminal would have to scroll the whole 24-row
screen up by one line to make room for the wrapped continuation, visibly
destroying the golden's own top border. The checked-in golden's top border
is untouched, exactly as the truncation-before-write behaviour above
predicts and the scroll-up alternative would not. **Answer: truncated,
never wraps** — deck's own footer relies on bubbletea's own width-aware
truncation, not on the outer terminal's wrap setting one way or the other.

**Stated plainly, per this task's own successCriteria:** the golden's
current footer line at 80x24 is an **accepted budget outcome** of that
truncation applied to a legend that does not fit in 80 columns — it is not
an assertion, anywhere in this repository's tests, that the legend is
*complete* at the minimum supported size. No code change follows from this
finding; it is recorded so a future reader measures rather than assumes
which of the missing entries' keys are still live (all of them are — this
is strictly a display-budget gap, not a binding gap; task 021's own
`TestFooterKeyLegendNamesOnlyBoundKeys` already proves every entry that
*is* declared in `footerLegend` names a real bound key, independent of
which of them fit on screen).
