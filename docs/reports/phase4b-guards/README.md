# Phase 4b — build/vet/gofmt guards + read-only path check (task 022)

Freeze-line guard sweep: `go build`, `go vet` and a `gofmt` drift check over
the whole module, plus a check that the four read-only paths this job is
forbidden to touch are still byte-identical to their launch-sha content —
all run once, in the CI sibling container, at the same code sha task 021's
whole-suite gate sweep gated (`docs/reports/phase4b-final-suite/README.md`).

- **Code sha (same sha task 021 gated):**
  `0806ba64ede4356af50b404affd10f26f68d4d84` (task 020, "tui: prove
  shared-state.db group edits reach another client on reload (R131)" — the
  last code-touching commit on the GO branch). `HEAD` at the time this
  report's commands ran was `f97774fc33adeb9c45eed675b90dc7ddc99fc046`
  (task 021's two record-only commits, `924a039` and `f97774f`, stacked on
  top of `0806ba6`); neither touches a `.go` file or either read-only path,
  so the code tree at `HEAD` is byte-identical to the code tree at
  `0806ba6` and the guard results below hold at both shas.
- **Launch sha for this run:** `c3b530a` — every comparison below is taken
  against this sha, per the task's own wording ("taken against launch sha
  c3b530a").

## Build

- **Invocation:** `ci/run.sh go build ./...`
- **Exit status:** `0` (green) — captured immediately after the command
  returned (`echo "BUILD_EXIT:$?"` → `BUILD_EXIT:0`), no output on stdout or
  stderr.

## Vet

- **Invocation:** `ci/run.sh go vet ./...`
- **Exit status:** `0` (green) — captured the same way (`VET_EXIT:0`), no
  output on stdout or stderr.

## Gofmt drift check

- **Invocation:** `ci/run.sh gofmt -l .`
- **Exit status:** `0` — `gofmt -l` exits 0 whether or not it lists drifted
  files; drift is read from the file list it prints, not the exit code.
- **Full output (3 lines, exactly what the check printed):**

  ```
  .spike-preview/cmd/conformance/main.go
  .spike-preview/conformance/conformance.go
  .spike-preview/conformance/conformance_test.go
  ```

- **Finding:** the only pre-existing gofmt drift left alone is these three
  `.spike-preview/` files, per the standing rules — no other file in the
  module is listed.
- **`internal/theme/quantize_test.go` — clean today, contrary to the PRD's
  claim:** the file is absent from the `gofmt -l .` output above, and a
  direct, single-file check confirms it independently:
  `ci/run.sh gofmt -l internal/theme/quantize_test.go` → exit `0`, **empty**
  stdout/stderr (no output at all — `gofmt -l` prints nothing for a file
  that needs no reformatting). The PRD's claim that this file carries gofmt
  drift is stale as of this sha.

## Touched-file set (`*.go` files this run touched, `c3b530a..0806ba6`)

Taken from `git diff --name-only c3b530a..0806ba6 -- '*.go'` (62 files, one
per line, exactly what the check printed):

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
internal/tui/attention_next_test.go
internal/tui/badge_detail_test.go
internal/tui/create_last_used_group_test.go
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
internal/tui/interactive_scroll_heal_test.go
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

Count check: `git diff --name-only c3b530a..0806ba6 -- '*.go' | wc -l` = 62,
matching the 62 lines above.

## Read-only path check (`c3b530a` vs `0806ba6`)

For each of the four paths this job is forbidden from touching, per the
standing rules (`git diff --stat c3b530a..HEAD -- SPEC.md prds/
ci/Dockerfile ci/SPIKE.md`) — run here as `git diff c3b530a..0806ba6 --
<path>` (equivalently `--stat`; both printed nothing):

| Path | Command | Output | Differs from launch sha? |
| --- | --- | --- | --- |
| `SPEC.md` | `git diff c3b530a..0806ba6 -- SPEC.md` | *(empty — 0 lines)* | No |
| `prds/` | `git diff c3b530a..0806ba6 -- prds/` | *(empty — 0 lines)* | No |
| `ci/Dockerfile` | `git diff c3b530a..0806ba6 -- ci/Dockerfile` | *(empty — 0 lines)* | No |
| `ci/SPIKE.md` | `git diff c3b530a..0806ba6 -- ci/SPIKE.md` | *(empty — 0 lines)* | No |

Every one of the four commands above printed nothing (`wc -l` on each ==
`0`) and exited `0` with an empty `--stat` summary too — none of the four
read-only paths differ, in any way, from their content at the launch sha
`c3b530a`. This job has made no edit to any of them.

## Summary

| Guard | Invocation | Exit |
| --- | --- | --- |
| build | `ci/run.sh go build ./...` | 0 |
| vet | `ci/run.sh go vet ./...` | 0 |
| gofmt | `ci/run.sh gofmt -l .` | 0 (3 pre-existing `.spike-preview/` files listed, no other drift) |

No red guard. No new task carved. The four read-only paths are unchanged
from launch. `internal/theme/quantize_test.go` is clean today; the PRD's
claim that it drifts is stale.
