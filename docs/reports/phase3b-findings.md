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
