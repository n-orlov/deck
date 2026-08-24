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
