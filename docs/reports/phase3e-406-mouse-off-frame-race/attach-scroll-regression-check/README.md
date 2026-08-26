Task 406's first fix attempt widened `clientCapturesFrameAs` itself (the
SHARED helper backing 13 "captures its frame as" call sites across the
suite) to wait for quiescence before snapshotting. That regressed
`attach_scroll.feature`'s `@requirement-48-wheel-scrolls-attached-pane-
without-typing` scenario, caught before commit by re-running it in
isolation:

- `baseline-clean-tree-run-1.log`: unmodified tree (before task 406 touched
  anything), 3/3 green in isolation.
- `widened-fix-regressed-run-1.log`: with `clientCapturesFrameAs` itself
  widened to quiesce, 5/5 RED in isolation at moderate load (loadavg
  ~3.7) -- not even the same load level the original DECK_MOUSE=0
  reproduction needed. Failure: baseline shows the attached tmux client's
  status line naming the window `tmux*` (tmux's default, pre-rename
  name), compare shows `sh*` -- a DIFFERENT async source (tmux's
  automatic-rename settling once the shell returns to its prompt) that
  does not push new bytes down the pty on its own; nothing is pending to
  wait for, so a longer wait does not fix it, and a scenario whose click
  gesture itself triggers the client's next status-line redraw is exactly
  where "wait longer before treating the frame as a baseline" fails.
- `scoped-fix-clean-run-1.log`: reverted the shared-helper change, added a
  separate `clientCapturesSettledFrameAs`/"captures its settled frame as"
  step used ONLY by mouse.feature's DECK_MOUSE=0 scenario --
  attach_scroll.feature's own step (registered to the ORIGINAL, unchanged
  `clientCapturesFrameAs`) is untouched by the fix and green again, 3/3.

This is why the shipped fix in `internal/tui` (none -- this is a
harness-only defect) and `features/mouse_synthesis_test.go` adds a new,
separately-named step instead of changing the existing one: the same
"quiesce before you call it a baseline" idea is unsound as a blanket
change to a helper 13 different scenarios share, each with a different
notion of what "the next relevant change" looks like.
