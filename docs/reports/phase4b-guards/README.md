# Phase 4b — guards (build, vet, gofmt, read-only path check)

## Sha this report is taken at

Task 021's own whole-suite gate ran at **baf92ed** (its RED gate-of-record; the later green
resweep at a224e43 belongs to the split follow-up tasks 021-cure-01/02/021-resweep-01, not to
task 021 itself). Per this task's own criteria ("at the same code sha task 021 gated"), every
guard below is measured against the tracked tree at **baf92ed**, not against this report's own
HEAD.

That distinction now matters: HEAD has moved on from baf92ed since this report was first taken.
Four code-touching cure commits (`4a1e352`, `8d934d1`, `8f8e9e8`, `f13c848`) plus two resweep
commits (`db9732b`, `a224e43`) landed between baf92ed and this report's own HEAD
(`93f5790`), so the code at HEAD is **no longer** byte-identical to baf92ed:

```
$ git diff --stat baf92ed..HEAD -- . | grep -v '^ docs/'
 features/create_session_test.go                                    |   22 +-
 features/theme_geometry_test.go                                    |   25 +-
 internal/tui/interactive.go                                        |   34 +-
 internal/tui/interactive_footer_scroll_advertisement_test.go        |    2 +-
 internal/tui/interactive_footer_scroll_cue_test.go                  |   10 +-
 internal/tui/interactive_forward_gate_test.go                       |   10 +-
 internal/tui/interactive_page_scroll_test.go                        |   28 +-
 internal/tui/interactive_scroll.go                                  |  163 +-
 internal/tui/interactive_scroll_heal_test.go                        |   12 +-
 internal/tui/interactive_scroll_persist_test.go                     |   26 +-
 internal/tui/interactive_scroll_render_heal_test.go                 |  182 +
 internal/tui/interactive_select.go                                  |    8 +-
 internal/tui/interactive_select_colored_highlight_test.go           |    2 +-
 internal/tui/interactive_select_highlight_test.go                   |    2 +-
 internal/tui/mouse_interactive_retarget_test.go                     |   12 +-
 internal/tui/settings.go                                            |  200 +-
 internal/tui/settings_groups_test.go                                |  272 +
 internal/tui/tui.go                                                 |   82 +-
 (plus report/log files under docs/reports/, elided above by the grep)
```

To measure the guards honestly against the sha this task must gate on, they were run in a
separate `git worktree` checked out at `baf92ed` (removed again before this task's own commit —
`git worktree list` shows only `/workspace` afterward), never against the live HEAD tree.

## Touched-file set (against launch sha c3b530a)

`git diff --name-only c3b530a..baf92ed -- '*.go' | sort` — this is the touched-file set anchored
at the same sha as the guards above, and it differs from the same command run against HEAD (the
later cure/resweep commits touch `features/theme_geometry_test.go`,
`internal/tui/interactive_page_scroll_test.go`, `internal/tui/interactive_scroll_render_heal_test.go`,
`internal/tui/interactive_select.go`, `internal/tui/interactive_select_colored_highlight_test.go`,
`internal/tui/interactive_select_highlight_test.go`, and `internal/tui/mouse_interactive_retarget_test.go`
— none of which existed yet, or were touched yet, at baf92ed):

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

67 files (this report previously mislabeled the count "63 files" while listing all 67 — corrected
here; the list itself was already right).

## Guard 1 — build

Invocation (run inside the `baf92ed` worktree, mounted through `ci/run.sh`):
`ci/run.sh sh -c 'cd .wt-baf92ed && go build ./...'`

Result: **exit 0**, no output.

## Guard 2 — vet

Invocation: `ci/run.sh sh -c 'cd .wt-baf92ed && go vet ./...'`

Result: **exit 0**, no output.

## Guard 3 — gofmt

Invocation: `ci/run.sh sh -c 'cd .wt-baf92ed && gofmt -l .'` (lists any `.go` file under the tree
whose formatting differs from `gofmt`'s own; exit status was 0 — `gofmt -l` only exits non-zero on
a parse error, never merely for listing drifted files).

`.spike-preview/` is untracked (gitignored, `.gitignore:4`) so a fresh worktree checkout of
baf92ed does not contain it by itself; those three files are the same physical files present in
the live `/workspace` tree regardless of which commit is checked out (checking out a different
sha never touches an untracked path), so they were copied into the worktree before running this
guard, to make the check see exactly what the live tree sees.

Output (verbatim):

```
.spike-preview/cmd/conformance/main.go
.spike-preview/conformance/conformance.go
.spike-preview/conformance/conformance_test.go
```

These three files are the only pre-existing gofmt drift, and per the standing rules it is left
alone — not a finding of this task. None of them is in the touched-file set above (`.spike-preview/`
is a separate, untracked spike tree with its own `go.mod`, outside `internal/`, `cmd/deck/`,
`features/`, `internal/config`, `internal/service`, `internal/store`, `internal/tmux`,
`internal/tui` proper). No file in the touched-file set appears in the `gofmt -l` output, so every
`.go` file this run touched is gofmt-clean at baf92ed.

`internal/theme/quantize_test.go` — checked individually, contrary to the PRD's claim that it
drifts:

```
$ ci/run.sh sh -c 'cd .wt-baf92ed && gofmt -l internal/theme/quantize_test.go'
(no output, exit 0)
```

It is clean today (and was already clean at baf92ed). The PRD's claim is stale (this matches the
standing rules' own note to the same effect).

## Guard 4 — read-only path check

Per-path `git diff --stat` from launch sha c3b530a to the guarded sha baf92ed (re-checked against
current HEAD `93f5790` too — identical result either way, since none of these four paths has ever
been touched since launch):

| path | differs from c3b530a (at baf92ed)? | differs from c3b530a (at HEAD)? | what the check printed |
|---|---|---|---|
| `SPEC.md` | no | no | *(no output — `git diff --stat c3b530a..baf92ed -- SPEC.md` and `c3b530a..HEAD -- SPEC.md` both print nothing)* |
| `prds/` | no | no | *(no output for either range against `prds/`)* |
| `ci/Dockerfile` | no | no | *(no output for either range against `ci/Dockerfile`)* |
| `ci/SPIKE.md` | no | no | *(no output for either range against `ci/SPIKE.md`)* |

All four read-only paths are unchanged since launch, at baf92ed and still at the current HEAD.

## Summary

| guard | invocation | exit |
|---|---|---|
| build | `ci/run.sh sh -c 'cd <baf92ed worktree> && go build ./...'` | 0 |
| vet | `ci/run.sh sh -c 'cd <baf92ed worktree> && go vet ./...'` | 0 |
| gofmt | `ci/run.sh sh -c 'cd <baf92ed worktree> && gofmt -l .'` | 0 (3 pre-existing `.spike-preview/` files listed, none touched) |
| read-only paths | `git diff --stat c3b530a..<sha> -- <path>`, four paths, checked at both baf92ed and HEAD | all 4 unchanged either way |

Green across the board at the sha task 021 gated (baf92ed) — measured in a scratch worktree
pinned to that sha rather than assumed identical to a moving HEAD, since HEAD has since taken on
four cure commits and two resweep commits that this report's earlier revision did not account
for. The worktree used to take these measurements was removed again before this task's own commit;
it is not part of the tree.
