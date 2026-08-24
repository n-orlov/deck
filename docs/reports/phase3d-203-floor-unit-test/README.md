# Task 203 — deterministic, tmux-free unit test of the 7-row-floor refusal (F3 backstop)

Review finding F3's remaining gap after 201/202: the ONLY coverage of PRD II-47/48's
7-inner-row floor refusal was `features/interactive_refusals.feature`'s
`@requirement-48-refuse-preview-below-seven-rows` godog scenario, which spins up a
real tmux server over a PTY and can flake under load (201's own root-cause report,
`docs/reports/phase3d-201-req48-degrade-rootcause.md`). No deterministic, tmux-free
test proved the refusal directly.

## What changed

`internal/tui/interactive.go`'s `enterInteractive`: the floor check (`previewContentSize`
vs `interactiveMinInnerRows`) — previously "Refusal case 2", checked AFTER
`SessionAttachedCount` — is now checked FIRST, before the attached-client check and
before every other tmux call in the function. `previewContentSize` is pure arithmetic
over `m`'s own fields; it touches no tmux state, so nothing about moving it first
weakens the attached-client check's own guarantee (that check exists to stop deck from
touching a window a bystander is watching — a check that never calls tmux at all
touches it even less). The attached-client check, and everything below it that
actually touches the window, is untouched in relative order. See the comment above
each check in `interactive.go` for the justification in place.

## New test

`internal/tui/interactive_test.go`:
`TestEnterInteractiveRefusesBelowTheSevenRowFloorWithoutAnyTmuxCall`.

- Builds a `Model` with `WithTmuxClient(tmux.Client{Socket: "no-such-tmux-server-203"})`
  — a non-empty `Socket` (so the zero-client degrade path does not fire) that resolves
  to nothing real. No tmux binary is even installed in this run's toolchain image
  (`which tmux` → not found), so if any tmux call executed at all the test would fail
  on a plain `exec: tmux: not found` error message instead of the floor message.
- Sets `m.width, m.height = 80, 9` — requirement 48's own godog fixture size
  (`features/interactive_refusals.feature`'s `@requirement-48` scenario) — and asserts
  `previewContentSize()` == 41x6, independently confirming this is the same frame
  201's root-cause report captured deck wrongly ENTERING interactive mode at
  (`docs/reports/phase3d-201-req48-degrade-rootcause.md`).
- Calls `enterInteractive()` directly (no `Update`/tea program, no PTY, no tmux
  process) and asserts: `interactive` stays false, `cmd` is nil, and `attachError`
  names the 7-row floor, offers `press a to attach`, and states the measured `6 inner
  rows` — the exact wording PRD II-47 requires.

### Non-vacuousness (pasted, not committed as code)

Lowering `interactiveMinInnerRows` from 7 to 1 turns the test red
(`unit-test-red-floor-lowered-to-1.log`):

```
=== RUN   TestEnterInteractiveRefusesBelowTheSevenRowFloorWithoutAnyTmuxCall
    interactive_test.go:382: fixture height 6 is not below interactiveMinInnerRows (1); this test would be vacuous
--- FAIL: TestEnterInteractiveRefusesBelowTheSevenRowFloorWithoutAnyTmuxCall (0.00s)
FAIL
```

Reverting to 7 turns it green again (`unit-test-green.log`).

## Evidence in this directory

- `unit-test-green.log` — the new unit test passing at `interactiveMinInnerRows = 7`.
- `unit-test-red-floor-lowered-to-1.log` — the same test failing at
  `interactiveMinInnerRows = 1` (non-vacuousness proof).
- `godog-@requirement-48-refuse-preview-below-seven-rows.log` — isolated tag run,
  green, verbose (still passes after the reorder).
- `godog-@requirement-47-refuse-attached-client.log` — isolated tag run, green
  (attached-client refusal unaffected by the reorder).
- `godog-@requirement-47-refuse-live-ownership.log` — isolated tag run, green
  (live-ownership refusal unaffected by the reorder).

## Build/vet/fmt

`ci/run.sh sh -c 'go build ./... && go vet ./... && gofmt -l $(git ls-files "*.go")'`
clean after the change (gofmt only needed a pass over the new test file itself).
`ci/run.sh go test -count=1 ./internal/tui/` green (full package, not just the new test).
