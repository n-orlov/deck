# Phase 3b (Part II — interactive preview) findings

Findings recorded as measurements/decisions against this repository, never as
edits to SPEC.md or prds/ (`git diff` proves neither was touched by this
file's own commits).

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
switch §13.1 forbids. Both paths are required to satisfy the same
`features/interactive_preview.feature` scenarios (task 070 proves this once
both paths exist), with one stated, named exception: the pipe-only
scenarios II-33 names (peeling a trailing semicolon off a literal
`send-keys` payload before re-sending it as `-H 3b`) have no meaning under
`capture`, which never calls `send-keys` for output at all — output there
is read back via `capture-pane`, not delivered through a pipe deck itself
armed. Every other observable (seed byte sequence, resize behaviour, exit
restore, dispatch identity checks, refusal cases) is a property of §11.9's
contract, not of the transport, and so must hold under either value of the
knob.

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
