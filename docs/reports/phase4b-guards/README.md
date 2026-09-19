# Phase 4b — guards (build, vet, gofmt, read-only path check)

## Sha this report is taken at

Task 021's whole-suite gate ran at **baf92ed**. Every commit landed between baf92ed and this
report's own HEAD (2508333) is docs/reports/DELIVERY-LOG/roadmap only (the freeze that began at
task 021's launch is in force) — verified directly:

```
$ git diff --stat baf92ed..HEAD | grep -v '^ docs/'
(no output — nothing outside docs/ changed)
```

So the code at HEAD is byte-identical to the code at baf92ed, and every guard below was run
against that same code (checked out as HEAD; no separate checkout of baf92ed was needed).

## Touched-file set (against launch sha c3b530a)

`git diff --name-only c3b530a..HEAD -- '*.go' | sort` (identical to the same command run against
baf92ed instead of HEAD — the two commands produce the same list, confirmed with `diff`):

```
cmd/deck/main.go
features/assertions_test.go
features/attention_sort_test.go
features/create_session_test.go
features/store_feature_test.go
internal/config/config.go
internal/config/config_test.go
internal/config/schema.go
internal/config/schema_test.go
internal/config/toml.go
internal/config/toml_write.go
internal/config/toml_write_test.go
internal/service/agent.go
internal/service/agent_test.go
internal/service/group_move.go
internal/service/session_context.go
internal/service/session_context_test.go
internal/service/shell.go
internal/service/shell_test.go
internal/store/group.go
internal/store/group_crud_test.go
internal/store/schema_v7_migration_test.go
internal/store/store.go
internal/store/store_test.go
internal/store/ui_state_groups_test.go
internal/tmux/pipe.go
internal/tmux/pipe_writer_wait_test.go
internal/tui/attention_next_test.go
internal/tui/badge_detail_test.go
internal/tui/create_last_used_group_test.go
internal/tui/empty_groups_render_test.go
internal/tui/filter.go
internal/tui/filter_test.go
internal/tui/flat_sidebar_test.go
internal/tui/group.go
internal/tui/group_id_navigation_test.go
internal/tui/group_move_test.go
internal/tui/group_order_test.go
internal/tui/group_shared_state_db_test.go
internal/tui/group_test.go
internal/tui/group_visual_order_test.go
internal/tui/interactive.go
internal/tui/interactive_footer_scroll_advertisement_test.go
internal/tui/interactive_footer_scroll_cue_test.go
internal/tui/interactive_forward_gate_test.go
internal/tui/interactive_scroll.go
internal/tui/interactive_scroll_heal_test.go
internal/tui/interactive_scroll_persist_test.go
internal/tui/main_view_theme_test.go
internal/tui/mark_test.go
internal/tui/mouse.go
internal/tui/mouse_test.go
internal/tui/navigation_parity_test.go
internal/tui/panel.go
internal/tui/panel_leak_audit_test.go
internal/tui/panel_test.go
internal/tui/preview_placeholder_test.go
internal/tui/registry_guard_test.go
internal/tui/rename.go
internal/tui/settings.go
internal/tui/settings_group_by_staging_test.go
internal/tui/settings_groups_test.go
internal/tui/sidebar_row_fill_test.go
internal/tui/sidebar_stripe_test.go
internal/tui/sort_order_render_test.go
internal/tui/tui.go
internal/tui/unarchive_test.go
```

63 files.

## Guard 1 — build

Invocation: `ci/run.sh go build ./...`

Result: **exit 0**, no output.

## Guard 2 — vet

Invocation: `ci/run.sh go vet ./...`

Result: **exit 0**, no output.

## Guard 3 — gofmt

Invocation: `ci/run.sh gofmt -l .` (lists any `.go` file under the tree whose formatting differs
from `gofmt`'s own; exit status was 0 — `gofmt -l` only exits non-zero on a parse error, never
merely for listing drifted files).

Output (verbatim):

```
.spike-preview/cmd/conformance/main.go
.spike-preview/conformance/conformance.go
.spike-preview/conformance/conformance_test.go
```

These three files are the only pre-existing gofmt drift, and per the standing rules it is left
alone — not a finding of this task. None of them is in the touched-file set above (they are
outside `internal/`, `cmd/deck/`, `features/`, `internal/config`, `internal/service`,
`internal/store`, `internal/tmux`, `internal/tui` proper — `.spike-preview/` is a separate spike
tree). No file in the touched-file set appears in the `gofmt -l` output, so every `.go` file this
run touched is gofmt-clean.

`internal/theme/quantize_test.go` — checked individually, contrary to the PRD's claim that it
drifts:

```
$ ci/run.sh gofmt -l internal/theme/quantize_test.go
(no output, exit 0)
```

It is clean today. The PRD's claim is stale (this matches the standing rules' own note to the
same effect).

## Guard 4 — read-only path check

Per-path `git diff --stat` from launch sha c3b530a to HEAD (2508333):

| path | differs from c3b530a? | what the check printed |
|---|---|---|
| `SPEC.md` | no | *(no output — `git diff --stat c3b530a..HEAD -- SPEC.md` prints nothing)* |
| `prds/` | no | *(no output — `git diff --stat c3b530a..HEAD -- prds/` prints nothing)* |
| `ci/Dockerfile` | no | *(no output — `git diff --stat c3b530a..HEAD -- ci/Dockerfile` prints nothing)* |
| `ci/SPIKE.md` | no | *(no output — `git diff --stat c3b530a..HEAD -- ci/SPIKE.md` prints nothing)* |

All four read-only paths are unchanged since launch.

## Summary

| guard | invocation | exit |
|---|---|---|
| build | `ci/run.sh go build ./...` | 0 |
| vet | `ci/run.sh go vet ./...` | 0 |
| gofmt | `ci/run.sh gofmt -l .` | 0 (3 pre-existing `.spike-preview/` files listed, none touched) |
| read-only paths | `git diff --stat c3b530a..HEAD -- <path>`, four paths | all 4 unchanged |

Green across the board at the sha task 021 gated (baf92ed, code-identical to HEAD 2508333).
