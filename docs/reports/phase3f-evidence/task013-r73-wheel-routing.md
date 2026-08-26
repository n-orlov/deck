# Task 013 — R73 leg 2: wheel routing for exactly the three scrollable overlays

Issue [#7](https://github.com/n-orlov/deck/issues/7). Leg 1 (task 012, `2714d1b`) added the
line-granular scroll and bound `up`/`down`/`j`/`k`. This leg lets the **wheel** reach the three
scrollable overlays without loosening the action-suppressing guard by one flag.

## What changed

- `internal/tui/panel.go`: `scrollableOverlayKind` + `wheelScrollableOverlay()` (which of the three
  scrollable overlays owns the keyboard right now, mirroring `Update`'s own `KeyMsg` dispatch order
  exactly) and `scrollWheelOverlay(dir)` (applies one notch via the *same* `dialogScrollByLines` step
  and the *same* body the key handlers pass; returns `false` when no scrollable overlay owns the
  keyboard).
- `internal/tui/tui.go`: one `if wheel-event { if scrolled, ok := m.scrollWheelOverlay(dir); ok { … } }`
  block immediately **before** the fifteen-flag early return. The guard's condition itself is
  byte-for-byte unchanged:

      $ git diff -U0 internal/tui/tui.go | grep -c '^[-+].*m.settingsDiscardConfirm'
      0        # the fifteen-flag line is not touched by this commit

  No hit test on `msg.X`/`msg.Y`: an overlay is modal, so there is no second thing a notch could have
  been meant for (interactive mode's own wheel *does* hit-test, because the sidebar beside the pane is
  live). `SPEC.md:1250` forbids *cancelling/confirming* and *reaching an action* by mouse; moving a
  read-only viewport does none of the three, the same test §11.8 applies to drag-to-select.

## Tests (`internal/tui/overlay_wheel_scroll_test.go`)

| test | asserts |
| --- | --- |
| `TestWheelScrollsScrollableOverlaysByOneLine` | one notch advances the first visible row by **exactly 1** wrapped line in help / detail / event log, a second notch by one more, `wheelUp` retreats by exactly one; offset is `1` after one notch; no `tea.Cmd` dispatched |
| `TestWheelInOverlayAgreesWithArrowKeys` | one notch and one `down` render byte-identically in all three (no second scroller) |
| `TestWheelInOverlayHonoursMouseOptOut` | with `Mouse=false` a wheel report still scrolls nothing (requirement 3/37) |
| `TestClickInsideOverlayStillDoesNothing` | press at the border / over body text / over the hidden sidebar row / outside the box, plus drag, release and a second press: no cmd, no interactive entry, no selection move, no sidebar scroll, no overlay scroll, no overlay opened or cancelled, frame byte-identical — for two **scrollable** overlays (detail, help) and two **unscrollable** ones (delete confirm opened through the real `dd`, rename) |
| `TestWheelOverUnscrollableOverlayDoesNothing` | a notch over delete-confirm / rename neither scrolls the dialog nor leaks to the sidebar underneath |
| `TestWheelStillScrollsTheSidebarWithNoOverlayOpen` | requirement 34's own wheel binding still fires when no overlay is open (`sidebarScroll` 0→1, selection unchanged) |

## Revert-and-reproduce (would this go red if the fix were reverted?)

Each mutation was applied in place to the product file, run, then reverted (`diff` against the
pre-mutation copy clean, `git status --short` showing only this task's own two modified files plus the
new test file).

| # | mutation (plausible naive implementation) | log | result |
| --- | --- | --- | --- |
| A | delete the new wheel block — i.e. the pre-R73 tree, where the blanket guard swallows every `MouseMsg` | `task013-mutationA-guard-swallows-wheel.log` | RED: `TestWheelScrollsScrollableOverlaysByOneLine` ("advanced by 0 lines, want exactly 1") and `TestWheelInOverlayAgreesWithArrowKeys` fail for all three overlays. Click tests stay green — correct, clicks were already suppressed |
| B | the tempting one-line "fix": drop `m.help`, `m.detail`, `m.eventLogOpen` from the fifteen-flag guard so mouse handling runs for scrollable overlays | `task013-mutationB-scrollable-overlays-let-clicks-through.log` | RED: `TestClickInsideOverlayStillDoesNothing/{detail_view,help_overlay}` — "press over the hidden sidebar row underneath moved the selection from 0 to 1". Wheel tests stay green, which is exactly why the click half is needed |
| C | wheel wired to `dialogScrollByPage` instead of `dialogScrollByLines` | `task013-mutationC-wheel-bound-to-page-step.log` | RED: "moved by more than one page, want exactly 1 line" (help, detail), "advanced by 14 lines, want exactly 1" (event log), plus the arrow-agreement test |

## Green runs at this commit

- `ci/run.sh go test -count=1 ./internal/tui/` → `ok github.com/n-orlov/deck/internal/tui 0.802s`;
  `gofmt -l internal/tui/` and `go vet ./internal/tui/` both silent.
- Mouse-related scenario tags: `ci/run.sh env DECK_GODOG_TAGS='@mouse-bindings,@dialogs,@requirement-2-mouse-synthesis,@requirement-3-deck-mouse,@attach-scroll' go test -count=1 ./features/ -run TestFeatures`
  → `ok … 37.046s` (`task013-mouse-feature-tags.log`). Non-vacuous: a bogus tag returns in `0.020s`
  against this run's 37s. `@dialogs` includes "detail dialog — the mouse can neither cancel nor
  confirm it" (clicks *and* double-clicks over the hidden row, asserting 0 attached events), so the
  end-to-end suppression is proven through the pty as well as in-package.
- `/proc/loadavg` at the scenario run: `9.03 4.92 3.65` before, `9.40 5.67 3.96` after (host busy;
  no timing claim rests on it).

## Still owed by R73 (task 014)

Help-overlay text for the new bindings (changes the help overlay's own scroll extent and
`cmd/deck/main_test.go`'s 260-row PTY window), and the report-only `framedDialog` overflow check
(findings task 024). The no-overlap page decision from leg 1 stands unchanged and must be recorded.
