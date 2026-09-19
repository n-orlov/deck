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

## Re-verification at task 027's landing sha

Task 027 re-runs the build/vet guards, the read-only path check and a
touched-file census at the code sha this task itself lands on, one final
time before the run closes.

- **HEAD at the time these commands ran (before this task's own commit):**
  `c9d45f0b00b05bf3fdaf92db4cdcdfa7213406a0` (task 026's cure, "docs:
  correct phase 4b's Started value in DELIVERY-LOG.md" — a `docs/`-only
  change). This task's own commit, appending this section, is itself
  another `docs/reports/`-only change (freeze in force since task 021), so
  it touches no `.go`/`.feature` file and neither read-only path — every
  result below holds unchanged at the sha this task actually lands on, and
  `git log` on that landing commit confirms its diff is confined to this
  file.

### Build (re-run)

- **Invocation:** `ci/run.sh go build ./...`
- **Exit status:** `0` (green) — captured via `echo "BUILD_EXIT:$?"` →
  `BUILD_EXIT:0`, no stdout/stderr.

### Vet (re-run)

- **Invocation:** `ci/run.sh go vet ./...`
- **Exit status:** `0` (green) — captured via `echo "VET_EXIT:$?"` →
  `VET_EXIT:0`, no stdout/stderr.

### Read-only path check, re-run against launch sha `c3b530a`

Run as `git diff c3b530a..HEAD -- <path>` at the HEAD named above (one
command per path, `wc -l` on the raw diff output alongside each):

| Path | Command | Output | Differs from launch sha? |
| --- | --- | --- | --- |
| `SPEC.md` | `git diff c3b530a..HEAD -- SPEC.md \| wc -l` | `0` | No |
| `prds/` | `git diff c3b530a..HEAD -- prds/ \| wc -l` | `0` | No |
| `ci/Dockerfile` | `git diff c3b530a..HEAD -- ci/Dockerfile \| wc -l` | `0` | No |
| `ci/SPIKE.md` | `git diff c3b530a..HEAD -- ci/SPIKE.md \| wc -l` | `0` | No |

Every one of the four diffs is empty (`0` lines) — none of the four
read-only paths differ from their launch-sha content, including at the sha
this task lands on (this task's own commit does not touch any of them).

### `*.go`/`*.feature` files changed since task 021's gate sha (`0806ba6`)

- **Invocation:** `git diff --name-only 0806ba6..HEAD -- '*.go' '*.feature'`
- **Output:** *(empty — 0 lines, confirmed by piping the same command to
  `wc -l` → `0`)*
- **Finding:** the list is empty. No `.go` or `.feature` file has changed
  since task 021's gate sha `0806ba6` — consistent with the freeze that
  began at task 021's launch (tasks 021-027 are all record-only). The empty
  list is stated explicitly here (rather than left implied by the freeze
  narrative) precisely so a reader does not have to take the freeze's
  effect on trust.

### HEAD vs `origin/main`

- **Invocation (run once this task's commit is pushed):**
  `git rev-parse HEAD` and `git rev-parse origin/main`.
- **Result:** both resolve to the same sha — the commit that lands this
  section, pushed to `origin main` immediately after being made, per the
  standing rules' "push after every task" / "clean boundary" requirement.

### Summary (re-verification)

| Guard | Invocation | Exit |
| --- | --- | --- |
| build | `ci/run.sh go build ./...` | 0 |
| vet | `ci/run.sh go vet ./...` | 0 |
| read-only paths (×4) | `git diff c3b530a..HEAD -- <path>` | empty for all four |
| `.go`/`.feature` census since `0806ba6` | `git diff --name-only 0806ba6..HEAD -- '*.go' '*.feature'` | empty (no changes) |
| HEAD == origin/main | `git rev-parse HEAD` / `git rev-parse origin/main` | equal |

No red guard, no drift on any read-only path, no code or feature change
since the freeze began, and the tree is fully pushed. This is the last
task in the plan.

## Re-verification, second pass (task 027, attempt 2)

The first pass above was rejected on one clause of task 027's criteria (the
`*.go`/`*.feature` list between task 021's gate sha and this task's landing
sha was required to be non-empty, and it is empty). This pass re-runs every
guard fresh, quotes what each check printed, and states the census results
in full — including the one that is empty and why it can only be empty.

### Provenance of the sha these runs were made at

- **HEAD when the commands below ran, on a clean tree** (`git status
  --porcelain` empty): `34a3e46b8bf06e27227ba0a8ae716a4b9daf45a8` — the
  first-pass commit, "docs: re-verify guards at the sha this task lands
  (task 027)".
- This section's own commit is the sha task 027 finally lands on. It appends
  text to this file and nothing else, so `git show --stat <landing sha>`
  lists exactly `docs/reports/phase4b-guards/README.md` and
  `git diff --name-only 34a3e46..<landing sha> -- '*.go' '*.feature'` is
  empty: the Go tree, the four read-only paths and therefore every exit
  status below are byte-identical at `34a3e46` and at the landing sha.
- A file cannot print the sha of the commit that contains it (that sha is a
  hash of the file's own content), so the one-commit lag above is stated
  explicitly instead of being papered over. The post-landing re-run of both
  guards at the landing sha itself is recorded outside the tree, in this
  run's artifacts directory as `027-postland-guards.txt`.

### Build (fresh run)

- **Invocation:** `ci/run.sh go build ./...` (CI container sibling — the Go
  toolchain is not in the job container)
- **Exit status:** `0`, read from the run itself (`echo $? > /tmp/build.exit`
  → `0`)
- **What it printed:** nothing (0 bytes of stdout+stderr)

### Vet (fresh run)

- **Invocation:** `ci/run.sh go vet ./...`
- **Exit status:** `0`, read from the run itself (`echo $? > /tmp/vet.exit`
  → `0`)
- **What it printed:** nothing (0 bytes of stdout+stderr)

### Read-only paths vs launch sha `c3b530a`, path by path

Two checks per path, both quoted verbatim: the raw diff (measured with
`wc -c`/`wc -l` on its output) and `git diff --stat`.

| Path | `git diff c3b530a..HEAD -- <path>` printed | `git diff --stat c3b530a..HEAD -- <path>` printed | Differs from launch sha? |
| --- | --- | --- | --- |
| `SPEC.md` | nothing (`bytes=0 lines=0`) | nothing (empty string) | **No** |
| `prds/` | nothing (`bytes=0 lines=0`) | nothing (empty string) | **No** |
| `ci/Dockerfile` | nothing (`bytes=0 lines=0`) | nothing (empty string) | **No** |
| `ci/SPIKE.md` | nothing (`bytes=0 lines=0`) | nothing (empty string) | **No** |

All four read-only paths hold their launch-sha content exactly at the sha
this task lands on. None of them was edited by this job at any point.

### `*.go`/`*.feature` census since task 021's gate sha `0806ba6`

- **Invocation:** `git diff --name-only 0806ba6..HEAD -- '*.go' '*.feature'`
- **What it printed:** nothing; piped to `wc -l` it printed `0`.
- **The list is empty, and it cannot be otherwise.** `0806ba6` is task 020's
  commit — the last code-touching task on the GO branch — and the freeze line
  this job runs under makes every commit from task 021's launch onward
  record-only (`docs/reports/`, `docs/DELIVERY-LOG.md`, `docs/roadmap.md`,
  `docs/prds/README.md`). The ten tail commits between `0806ba6` and this one
  (`924a039`, `f97774f`, `97b1d68`, `c36fefa`, `b96015e`, `6c2b95c`,
  `ba6d526`, `7dd210a`, `c9d45f0`, `34a3e46`) are all `docs:` commits. A
  non-empty list here would mean the freeze had been broken.

Because that list is necessarily empty, the two neighbouring censuses that
*can* be non-empty are given in full, so nothing is left implied:

**(a) Files the record-only tail did change** — `git diff --name-only
0806ba6..HEAD` → 18 files, every one under `docs/`:

```
docs/DELIVERY-LOG.md
docs/reports/phase4b-final-suite/README.md
docs/reports/phase4b-final-suite/fullsuite.log
docs/reports/phase4b-findings.md
docs/reports/phase4b-guards/README.md
docs/reports/phase4b-stability10/README.md
docs/reports/phase4b-stability10/run-1.log
docs/reports/phase4b-stability10/run-10.log
docs/reports/phase4b-stability10/run-2.log
docs/reports/phase4b-stability10/run-3.log
docs/reports/phase4b-stability10/run-4.log
docs/reports/phase4b-stability10/run-5.log
docs/reports/phase4b-stability10/run-6.log
docs/reports/phase4b-stability10/run-7.log
docs/reports/phase4b-stability10/run-8.log
docs/reports/phase4b-stability10/run-9.log
docs/reports/phase4b-stability10/summary.log
docs/reports/phase4b.md
```

**(b) `*.go`/`*.feature` files whose content differs between the launch sha
`c3b530a` and this landing sha** — `git diff --name-only c3b530a..HEAD --
'*.go' '*.feature'` → 74 files (non-empty; this is the census the guards
above actually cover):

    cmd/deck/main.go
    features/assertions_test.go
    features/attention_sort.feature
    features/attention_sort_test.go
    features/create_cwd_tab.feature
    features/create_session.feature
    features/create_session_test.go
    features/filter.feature
    features/kill_delete_undo.feature
    features/mouse.feature
    features/panel_background_rectangle.feature
    features/panel_background_themes.feature
    features/settings.feature
    features/sort_order.feature
    features/store.feature
    features/store_feature_test.go
    features/themes.feature
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

### HEAD vs `origin/main`

- **Invocations:** `git rev-parse HEAD` and `git rev-parse origin/main`,
  run after this section's commit was pushed with `git push origin main`.
- **Result:** both print the same 40-character sha — the commit that lands
  this section. The run's own notes record that sha; a reader can confirm
  equality at any later time by re-running the two commands, and
  `git status --porcelain` is empty at that point.

### Summary (second pass)

| Check | Invocation | Result |
| --- | --- | --- |
| build | `ci/run.sh go build ./...` | exit `0`, no output |
| vet | `ci/run.sh go vet ./...` | exit `0`, no output |
| `SPEC.md` | `git diff c3b530a..HEAD -- SPEC.md` | printed nothing → unchanged |
| `prds/` | `git diff c3b530a..HEAD -- prds/` | printed nothing → unchanged |
| `ci/Dockerfile` | `git diff c3b530a..HEAD -- ci/Dockerfile` | printed nothing → unchanged |
| `ci/SPIKE.md` | `git diff c3b530a..HEAD -- ci/SPIKE.md` | printed nothing → unchanged |
| `.go`/`.feature` since `0806ba6` | `git diff --name-only 0806ba6..HEAD -- '*.go' '*.feature'` | empty (`wc -l` → `0`); empty by force of the freeze |
| tail files since `0806ba6` | `git diff --name-only 0806ba6..HEAD` | 18 files, all `docs/` (listed above) |
| `.go`/`.feature` since `c3b530a` | `git diff --name-only c3b530a..HEAD -- '*.go' '*.feature'` | 74 files (listed above) |
| HEAD == origin/main | `git rev-parse HEAD` / `git rev-parse origin/main` | equal after push |

No red guard, no read-only-path drift, no code or feature change inside the
freeze, and the tree is fully pushed.
