# Phase 3f task 019 (R67 part 1): the eleven task-numbered test files, renamed after what they test

Parent commit: `ce8ef91`. Toolchain: `ci/run.sh` (sibling container), `/proc/loadavg` at the
verification run: `3.13 3.74 3.38`.

## Why

Eleven test files were named after the *task number* that created them, not after the behaviour they
cover. Ten of them sit in `internal/tui`, so `settings_task017_test.go` and
`internal/config/config_task017_test.go` coexisted while covering unrelated things, and task ids get
reused across phases — the name told a reader nothing and mis-told them something.

## The mapping (all eleven, `git mv`, no body edited)

| old | new | what it actually covers |
| --- | --- | --- |
| `internal/config/config_task017_test.go` | `internal/config/env_overrides_test.go` | `LoadFrom` records which schema keys the environment overrode (`Settings.EnvOverrides`) |
| `internal/tui/settings_task002_test.go` | `internal/tui/settings_env_override_write_test.go` | saving does not rewrite an env-overridden file value unless the field was edited |
| `internal/tui/settings_task003_test.go` | `internal/tui/settings_env_entries_editor_test.go` | the `[env]` entries editor: add / edit / remove / esc-cancel |
| `internal/tui/settings_task006_test.go` | `internal/tui/settings_live_apply_test.go` | live-scope fields apply on save; restart-to-apply fields do not |
| `internal/tui/settings_task007_test.go` | `internal/tui/settings_group_by_staging_test.go` | `ui.group_by` stages until save rather than applying on keypress |
| `internal/tui/settings_task015_test.go` | `internal/tui/settings_field_display_and_edit_test.go` | per-`FieldKind` value rendering, tilde collapse, scope label, enter-toggle, `+`/`-` bounded int |
| `internal/tui/settings_task016_test.go` | `internal/tui/settings_save_and_discard_test.go` | `ctrl+s` atomic save, esc discard prompt, save-failure note |
| `internal/tui/settings_task017_test.go` | `internal/tui/settings_env_override_label_test.go` | labelling an env-overridden field without lying about the running value |
| `internal/tui/settings_task018_test.go` | `internal/tui/settings_schema_parity_test.go` | schema/settings structural parity both directions + per-field round trip |
| `internal/tui/settings_task019_test.go` | `internal/tui/settings_color_cues_test.go` | painted per-cell colours: label/value tokens, border focus cue, selection cue |
| `internal/tui/settings_task310_test.go` | `internal/tui/settings_edits_from_settings_test.go` | `settingsEditsFromSettings` covers every flat key (the `PreviewFit` drop guard) |

No two new names collide inside a package, and none collides with an existing file
(`settings_test.go`, `settings_scope_parity_test.go`, `matrix_status_tokens_test.go` are untouched
except for one stale pointer, below). No file was split: eleven files in, eleven files out.

Two *comments in other/renamed files* pointed at old filenames and were repointed in the same commit
so no dangling in-code reference remains: `internal/tui/matrix_status_tokens_test.go:18`
(`settings_task019_test.go` -> `settings_color_cues_test.go`) and
`internal/tui/settings_edits_from_settings_test.go:10` (`settings_task018_test.go` ->
`settings_schema_parity_test.go`). Comments only — no assertion, helper or test body changed.

## Proof the test set is identical

`ci/run.sh go test -list '.*' ./internal/tui/ ./internal/config/`, captured at the parent commit and
after the renames, name lines only (the trailing `ok <pkg> 0.00Ns` lines carry a per-run duration and
are kept in the `.raw.txt` captures instead of the diffed lists):

- `docs/reports/phase3f-019-r67-test-file-names/go-test-list-parent-ce8ef91.raw.txt` (443 lines, raw)
- `docs/reports/phase3f-019-r67-test-file-names/go-test-list-parent-ce8ef91.txt` (441 test names, sorted)
- `docs/reports/phase3f-019-r67-test-file-names/go-test-list-renamed.raw.txt` (443 lines, raw)
- `docs/reports/phase3f-019-r67-test-file-names/go-test-list-renamed.txt` (441 test names, sorted)

```
$ diff go-test-list-parent-ce8ef91.txt go-test-list-renamed.txt && echo "DIFF-EMPTY exit=$?"
DIFF-EMPTY exit=0
```

441 test functions before, 441 after, same names — a rename, not a rewrite.

## Verification

```
$ ls internal/*/*task[0-9]*_test.go
ls: cannot access 'internal/*/*task[0-9]*_test.go': No such file or directory

$ ci/run.sh go test -count=1 ./internal/tui/ ./internal/config/
ok  	github.com/n-orlov/deck/internal/tui	0.789s
ok  	github.com/n-orlov/deck/internal/config	0.033s
```

`git show --stat` for this commit shows the eleven as renames (`R`), and `git log --follow` on a new
path resolves through the old one — see `docs/reports/phase3f-019-r67-test-file-names/git-rename-evidence.txt`.

## Finding (protected path, for task 020 / the close-out report)

`prds/phase3f-residuals-and-suite-determinism.md:467-479` lists all eleven files under their OLD
names. `prds/` is protected, so it is deliberately NOT edited; the citation is stale by design and
recorded here instead. Historical reports (`docs/reports/phase2b2*.md`, `docs/reports/phase3.md`,
`docs/reports/phase3d-215-preview-fit-on-nav.md`) also name the old files: they are a record of what
the tree looked like then and are left as written. `docs/DELIVERY-LOG.md:397`'s carried-items
paragraph is task 020's to update.
