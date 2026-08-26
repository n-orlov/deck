# Task 411 — fix the `locateText` regression from task 402

## What broke

Task 402 (commit `bc6bc84`) scoped `features/mouse_bindings_test.go`'s
`locateText` helper to search only `sidebarRegion`'s bounds. That is the
correct fix for `locateText`'s own two callers
(`clientClicksOnRowContaining`/`clientDoubleClicksOnRowContaining`, both of
which locate a *sidebar* row) — see
`docs/reports/phase3e-401-r54-noop-discriminator/` and
`docs/reports/phase3e-402-r54-noop-sidebar-scoped-click/`.

But `locateText` was *also* called directly by two other files that need to
locate text **inside the preview/interactive pane**, not the sidebar:

- `features/interactive_scroll_test.go`'s
  `clientScrollsInteractiveWheelOverLineContaining`
- `features/interactive_selection_test.go`'s
  `clientDragsToSelectTextOverInteractivePane` and
  `clientClicksOnceOnInteractivePaneLineContaining`

Once `locateText` was narrowed to `sidebarRegion`, these three callers could
never find their target text (it lives in the preview pane, to the right of
the seam, not in the sidebar), and every scenario driving them failed with
`no sidebar row of the frame contains "<text>"`.

This was discovered while working task 407 (produce a green whole-suite
run): checking out the tree's last non-docs commit (`668a94c`) and running
`ci/run.sh go test -p=1 -count=1 ./...` **three times** at different load
failed the SAME three scenarios every time, not flakily. Confirmed
100%-reproducible tag-alone at low load — this corrects task 406's notes,
which had (wrongly) dismissed one instance of this as an "unrelated
one-off flake" during 406's own repeated-invocation stress testing.

**Pre-fix red logs** (captured while working task 407, before this task's
fix; committed here as the red proof):

- `red-current-broken/red-task-216-drag-to-copy.log` —
  `DECK_GODOG_TAGS=@task-216-drag-to-copy` alone, both
  `interactive_selection.feature` scenarios fail:
  `no sidebar row of the frame contains "DRAGCOPYTOKEN"` /
  `"CLICKNODRAGTOKEN"`.
- `red-current-broken/red-requirement-51-bounded-scrollback.log` —
  `DECK_GODOG_TAGS=@requirement-51-bounded-scrollback` alone, the wheel-scroll
  scenario fails: `no sidebar row of the frame contains "SNAP_SCROLL_LINE_1"`
  (or similar numbered-loop token).

## The fix

Added a mirror-image pair of helpers to `features/mouse_bindings_test.go`,
alongside the existing `sidebarRegion`/`locateText`:

- **`previewRegion(frame)`** — same row/column-bound shape as
  `sidebarRegion`, but returns the **preview** panel's own bounds: in
  side-by-side/collapsed mode, the same row range as the sidebar (they share
  a border per row) but the column range starting just past the shared seam
  (`seamColumn(frame)+1`) instead of ending at it; in stacked mode, the
  *second* bordered box (the one below the sidebar's own bottom border)
  instead of the first.
- **`locatePreviewText(client, text)`** — `locateText`'s mirror image,
  scoped to `previewRegion`'s bounds instead of `sidebarRegion`'s.

`sidebarRegion`/`locateText` themselves are **untouched** — task 401/402's
own fix (the R54 no-op scenario's sidebar-only callers) keeps exactly the
scoping it had.

The three affected callers were switched from `locateText` to
`locatePreviewText`:

- `features/interactive_scroll_test.go`:
  `clientScrollsInteractiveWheelOverLineContaining`
- `features/interactive_selection_test.go`:
  `clientDragsToSelectTextOverInteractivePane`,
  `clientClicksOnceOnInteractivePaneLineContaining`

No product code changed. No feature file changed. `go build ./...`,
`go vet ./...`, `gofmt -l` on the touched files all clean.

## Proof (green, post-fix)

All three tag-alone runs, `ci/run.sh sh -c 'DECK_GODOG_TAGS=... go test
-count=1 -run TestFeatures -v ./features/'`:

| Tag | Result | Log |
|---|---|---|
| `@task-216-drag-to-copy` | 2/2 scenarios PASS | `green-task-216-drag-to-copy.log` |
| `@requirement-51-bounded-scrollback` | 3/3 scenarios PASS | `green-requirement-51-bounded-scrollback.log` |
| `@requirement-54-sidebar-click-on-interactive-row-is-a-no-op` | 1/1 scenario PASS (no regression) | `green-requirement-54-sidebar-click-on-interactive-row-is-a-no-op.log` |

The third run proves task 401/402's own fix (the R54 no-op scenario, whose
callers stay on `sidebarRegion` via the untouched `locateText`) still holds.

## Follow-up

Task 407 (produce+cite a green whole-suite run) was blocked on this task
landing; it must now be redone at the new tip (this fix's commit), since the
code changed since task 406's commit `668a94c`.
