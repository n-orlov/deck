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
