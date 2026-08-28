# R83 evidence — the three dialogs that drew past the frame at 80×24

Requirement R83 (finding F6), tasks 016 and 018 for the bounding that landed; task 022
(the `NO_COLOR`/`DECK_ASCII`/width-accounting net) is **still pending** and this
directory makes no claim about it. Fixing shas: `cdb927b` (create modal), `5cc1b45` +
`8c3351a` (env editor), `26a5b47` + `f33d67a` + `7f1a780` (bulk delete confirm).

Not one of the PRD's eight product-defect requirements. **Captured by task 038** at HEAD
`e02ef08` (unmodified tree): green-only confirmation that the ≤24-line bound and the
submit-line reachability assertions the report names do run and pass, not an
implementation-time red/green pair.

- `green-80x24-bounds.log` —
  `ci/run.sh go test -count=1 -v -run 'StaysWithinFrameBudgetAt80x24|SubmitLineReachableViaPgDown|ClosingHintReachableViaPgDown|SubmitLineVisibleForAnOrdinaryMarkSet|SubmitLinePinnedWhileTheMarkListScrolls|EditPromptAndSubmitLineVisibleWhenListOverflows' ./internal/tui/`
  — the 80×24 frame-budget and reachability cases of
  `create_view_theme_test.go`, `env_editor_theme_test.go`, `bulk_delete_theme_test.go`
  (the run also picks up the detail/main-view budget tests, which share the name).
