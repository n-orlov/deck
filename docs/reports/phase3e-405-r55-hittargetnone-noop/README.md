# Task 405 — a sidebar press resolving to `hitTargetNone` enters nothing (R55 coverage gap)

## Claim

SPEC §11.8/R55's own scope is "a click on a sidebar *row* selects it and
enters interactive mode on the same press" (`internal/tui/mouse.go`'s
`clickSidebarRow`, task 311). Nothing in the pre-existing suite exercised
the sibling case: a left press that hit-tests *inside the sidebar panel*
but *not* onto any row, header or the collapsed strip — the blank padding
below the last visible entry, or (equally) any padding cell once the list
is shorter than the panel's content height. `hitTest` names this case
`hitTargetNone` (`internal/tui/mouse.go:31`); `handleMousePress`'s switch on
`hit.target` (`internal/tui/mouse.go:226-243`) has no `case` for it at all,
so it falls straight through the switch and the enclosing `case
hitPanelSidebar:` block to the function's own trailing `return m, nil`
(`mouse.go:244`). That is exactly the desired no-op, but until this task
nothing forced it to *stay* that way — a future edit adding a
`default:` case (or reordering the switch) could route it into
`clickSidebarRow` and nothing would fail.

## What was added

1. **Unit test** `internal/tui/mouse_test.go`:
   `TestClickSidebarPaddingBelowLastRowIsANoOp`. Builds a 2-session model
   at 100×30 (the package's own `mouseTestModel` fixture), sets
   `m.selected = 1` (deliberately *not* 0 — see "why selected=1" below),
   computes the padding row's coordinate from `m.sidebarEntries` (the real,
   un-padded entry count) and `contentRowY`, asserts via `m.hitTest` that
   the chosen cell really does resolve to `hitPanelSidebar`/`hitTargetNone`
   (a setup guard, not the test's own claim — this is what stops the test
   silently asserting nothing if a future layout change shrinks the
   padding area to zero), then presses there through `m.Update` and checks
   `selected` is still 1, `interactive` is still false, and the returned
   `tea.Cmd` is nil.

   *Why `selected = 1`, not 0:* `hitResult`'s zero value has
   `sessionIndex == 0`. If a mutant let `hitTargetNone` fall through to
   `clickSidebarRow(hit.sessionIndex, e)`, it would call
   `clickSidebarRow(0, e)`. Starting the test at `selected == 0` would make
   that mutation's `m.selected = index` assignment invisible (0 → 0, no
   observable change) — exactly the kind of test that "reaches the code
   path" but can't tell a mutant from the real fix. Starting at `selected
   == 1` makes the mutant's `1 → 0` move observable no matter what the
   real fix and the mutant's sessionIndex zero-value happen to agree on.

   *Why the count comes from `sidebarEntries`, not `sidebarVisibleEntries`:*
   `sidebarVisibleEntries` (`internal/tui/tui.go:3342-3354`) always returns
   a slice of exactly `contentHeight` length, zero-padding *past* the real
   entries itself (its own doc comment: "returns ... a
   contentHeight-length slice"). A first draft of this test read
   `len(visible)` as "how many real rows there are" and got `contentHeight`
   back regardless of session count: with 2 sessions and a 27-row content
   box that draft's own setup guard failed with `visible=27,
   contentHeight=27` (observed while drafting, not saved as a log since it
   never reached a real assertion) before the fix below landed. The real
   entry count is `len(m.sidebarEntries(width))`.

2. **godog scenario** `features/mouse.feature`
   `@requirement-33-sidebar-padding-click-is-a-no-op` ("a click on the
   sidebar's empty padding below the last row selects nothing and enters
   nothing"). Tagged `@requirement-33-*` to match this repo's existing
   convention for R55 (the per-requirement evidence table in
   `docs/reports/phase3e.md` already maps R55 to the `@requirement-33-*`
   tag family, not a `@requirement-55-*` one — task 311/312's own
   scenarios use that tag, and this task's own title cites `mouse.go:250`
   which is inside `clickSidebarRow`, the same R55 function).

   Creates two sessions (default 100×30 terminal,
   `features/pty_driver_test.go`'s `terminalColumns`/`terminalRows`),
   selects the first, captures the frame, clicks at a fixed column/row
   (col 10, row 15 — well inside the sidebar's ~27-row content box and far
   below either session's two-line row, which occupy frame rows 3-6),
   then asserts the frame is byte-identical to the capture (`frame still
   matches the captured ... frame`, the same idiom
   `@requirement-37-deck-mouse-disables-gestures` and
   `@requirement-33-preview-gesture-no-ops` already use for "this gesture
   changed nothing") and that the selection is still the first session.
   An earlier draft also asserted `screen does not contain "interactive"`,
   which is wrong regardless of the fix: the list view's own footer always
   reads `"...Enter interactive..."`, so that assertion fails on *correct*
   code too — removed before landing (see `feature-clean-green.log`, no
   such step present).

## Red proof — unit test

Mutation (`internal/tui/mouse.go`, `handleMousePress`'s
`hitPanelSidebar`/`hit.target` switch): added

```go
case hitTargetRow:
	return m.clickSidebarRow(hit.sessionIndex, e)
default:
	return m.clickSidebarRow(hit.sessionIndex, e)
```

`ci/run.sh go test -count=1 -run TestClickSidebarPaddingBelowLastRowIsANoOp -v ./internal/tui/`
→ [`unit-mutant-red.log`](unit-mutant-red.log):

```
--- FAIL: TestClickSidebarPaddingBelowLastRowIsANoOp (0.00s)
    mouse_test.go:232: selected changed after a click on sidebar padding: 0, want unchanged 1
```

Restored (`git diff internal/tui/mouse.go` empty again) →
[`unit-restored-green.log`](unit-restored-green.log): `PASS`.

## Red proof — godog scenario

Same mutation reapplied, same tag run alone:
`ci/run.sh sh -c 'DECK_GODOG_TAGS=@requirement-33-sidebar-padding-click-is-a-no-op go test -count=1 -run TestFeatures -v ./features/'`
→ [`feature-mutant-red.log`](feature-mutant-red.log):

```
--- FAIL: TestFeatures (2.37s)
    --- FAIL: TestFeatures/a_click_on_the_sidebar's_empty_padding_below_the_last_row_selects_nothing_and_enters_nothing (2.35s)
```

(the footer now reads "...Ctrl+Q leave interactive mode" instead of the
list's own footer, because the mutant's fallthrough entered interactive
mode on session index 0 — `padding-noop-alpha`, which happens to already
be selected in this scenario, so the frame content differs only in the
footer line, exactly where `frame still matches the captured frame` looks.
The harness's own goroutine dump the mutant's still-running interactive
subprocess produced is also visible in the raw output; it is the
harness's client-hang-after-red-scenario cleanup, unrelated to this task's
claim, and not investigated further here — task 406 is the place that
root-causes that class of dump for the `DECK_MOUSE=0` scenario
specifically).

Restored (`git diff internal/tui/mouse.go` empty again), same tag run
alone → [`feature-restored-green.log`](feature-restored-green.log):

```
1 scenarios (1 passed)
11 steps (11 passed)
```

Also committed for the record, the first clean run before any mutation
was ever applied → [`feature-clean-green.log`](feature-clean-green.log)
(identical pass/fail shape to the restored run).

## Full-package proof

`ci/run.sh go test -count=1 ./internal/tui/` →
[`internal-tui-package.log`](internal-tui-package.log): `ok`.

`ci/run.sh sh -c 'go build ./... && go vet ./... && gofmt -l .'`: clean
except the three pre-existing `.spike-preview/...` gofmt hits this repo's
notes already document as untouched/known.

`git status --short` at commit time: only
`features/mouse.feature`, `internal/tui/mouse_test.go`, and this report
directory — `internal/tui/mouse.go` (the file both mutations touched)
is byte-identical to its pre-task state.

## Success-criteria checklist

- [x] Unit test in `internal/tui` asserting a left press on sidebar padding
  below the last row leaves `m.selected` unchanged and `m.interactive`
  false: `TestClickSidebarPaddingBelowLastRowIsANoOp`.
- [x] A test reaches `hitTargetNone`: the same test's setup assertion
  pins `m.hitTest(x, y).target == hitTargetNone` before checking anything
  else.
- [x] godog scenario in `features/mouse.feature` clicking that padding row
  and asserting the frame's selection marker and mode are unchanged:
  `@requirement-33-sidebar-padding-click-is-a-no-op`.
- [x] Red proof committed: mutating `handleMousePress` so a
  `hitTargetNone` sidebar press falls through to `clickSidebarRow`, both
  new tests shown red, restored, shown green — all four logs above.
- [x] `ci/run.sh go test -count=1 ./internal/tui/` green.
- [x] The mouse.feature tag run green (tag-alone, twice: pre-mutation and
  post-restore).
