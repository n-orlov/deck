# Input-count instrument (I-1 / task 003)

## What this is

A test-only instrument that answers, from outside the process, "how many
keys has the running `deck` program actually received", independently of
whatever the screen ends up showing. It exists because I-1's keystroke-drop
investigation needs to distinguish two failure modes that render an
identical, unchanged frame:

1. the pty/tmux/bubbletea layer never delivered the byte at all, or
2. deck's `Update` loop received the key but something in the application
   (a dialog, a key binding, a checkpoint) did not act on it as expected.

Screen-content assertions (`WaitForFrame`, `FindText`, ...) cannot tell these
apart. This instrument answers only the first question, cleanly.

## How it works

- `cmd/deck/inputcount_hook.go` (build tag `deckinputcount`) wraps the outer
  `tea.Model` in `main.go` with `inputCountingModel`, which increments a
  counter on every `tea.KeyMsg` the model's `Update` sees — at the earliest
  programmatic point reachable without patching bubbletea itself, before
  deck's own coalesced-`KeyMsg` splitting (`internal/tui/tui.go`, task 118)
  or any application logic runs.
- `cmd/deck/inputcount_default.go` (`!deckinputcount`, i.e. every real build:
  `go build ./...`, `go vet ./...`, `go test ./...`, every release) makes
  `wrapForInputCounting` a no-op. The instrument adds zero surface to the
  shipped product.
- Even in a `-tags deckinputcount` build, the wrapper stays inert unless the
  environment variable `DECK_INPUT_COUNT_FILE` is set to a path. No
  documented deck control ever sets this on a user's behalf.
- When set, every counted key overwrites that file (via write-to-temp-then-
  rename, so a concurrent reader never sees a torn value) with the running
  total as a bare decimal integer — nothing else, no newline required.

## Weighting: "keys", not "Update calls"

A `tea.KeyMsg` counts for the number of *keystrokes* it represents, not for
one Update call:

- a named key (`Up`, `Enter`, ...) or a single-rune key weighs 1;
- a coalesced multi-rune `KeyMsg{Type: KeyRunes, Runes: "jx"}` — bubbletea's
  own PTY reader reporting that two keys landed in the same `read(2)` —
  weighs `len(Runes)`, mirroring exactly the test `internal/tui/tui.go`'s
  own coalescing-detection case already uses;
- a bracketed paste (`msg.Paste`) weighs 1 regardless of length, the same
  exemption deck's own splitting gives it — a paste is one input event, not
  N keystrokes.
- every non-key message (resize, mouse, render ticks, ...) is uncounted.

## How to build and run it

```sh
go build -tags deckinputcount -o /tmp/deck ./cmd/deck
DECK_INPUT_COUNT_FILE=/tmp/deck-input-count \
DECK_HOME=/tmp/deck-home DECK_TMUX_SOCKET=deck_probe \
/tmp/deck
```

Reading `/tmp/deck-input-count` from another shell at any point shows the
running total of keys deck has received so far.

## How the test harness reads it

`features/input_count_test.go` adds:

- `readInputCount(path) (int, error)` — reads the current total, treating a
  not-yet-created file as 0.
- `waitForInputCount(path, want) (int, error)` — polls (in the same spirit
  as `features/fake_agent_size_test.go`'s `waitForFixtureFullyRendered`
  byte-count poll, applied to input instead of output) until the total
  reaches `want` or a 2s deadline, so a scenario can send N keys and then
  assert the program's own `Update` loop saw all N.

A scenario or test that wants this instrument builds the deck binary with
`buildDeckBinaryWithTags(t, "deckinputcount")` (already used by
`features/mouse_exit_paths_test.go` for the panic-hook build) and sets
`DECK_INPUT_COUNT_FILE` in the environment passed to `StartScreenDriver`.

## Proof the counter is real

- `cmd/deck/inputcount_hook_test.go` (build tag `deckinputcount`) drives the
  wrapper directly (no pty) and checks the file after every single message:
  a named key, a single-rune key, a non-key message (must NOT move the
  counter), a 3-rune coalesced key (must weigh 3, not 1), a paste (must
  weigh 1 regardless of length), and a "withheld key" case that asserts the
  counter does *not* advance on its own between two real increments.
- `features/input_count_test.go`'s
  `TestInputCountInstrumentCountsRealKeystrokesThroughARealPTY` is the
  harness-level proof: it drives an actual deck process through a real PTY,
  sends 3 individually-paced keys and then 2 unpaced (coalescing-shaped)
  keys, and asserts the file reflects exactly 5 — proving the file is fed by
  deck's real `Update` loop over a real pty, not fabricated by the test.
- Both tests were verified red under two deliberate mutations during
  development (reverted before commit, tree confirmed clean):
  1. hard-coding the per-key weight to 1 (dropping the `len(Runes)` case)
     failed `TestInputCountingModelCountsKeysNotUpdateCalls` at the 3-rune
     coalesced-key assertion (`count = 3, want 5`);
  2. asserting a higher target than what was actually sent through the real
     pty (6 instead of 5) failed
     `TestInputCountInstrumentCountsRealKeystrokesThroughARealPTY` with a
     timeout (`last seen 5`), proving the harness-level poll does not
     manufacture a pass.

## What this task does not do

This task builds the instrument and proves it is real. It does not yet use
it to investigate or fix I-1's keystroke drop — that is task 004's job, now
unblocked: task 004 can drive a real scenario, send the keys the marked-set
idiom (`k`,`m`,checkpoint,`j`,checkpoint,`m`) sends, and ask this instrument
whether deck's `Update` loop ever saw the `j` that the screen-content
checkpoint says never landed, distinguishing a pty-level drop from an
application-level one.
