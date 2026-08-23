# Phase 3 report

Skeleton created by task 101 (report skeleton + baseline capture + per-requirement
evidence table). Populated incrementally as later tasks land; a cell holding the
literal word `PENDING` means the requirement has not been delivered yet in this
tree, not that it was overlooked. Task 130 is responsible for turning every
remaining `PENDING` into real evidence or an explicit "not delivered, because"
statement before the phase closes.

## Baseline suite capture

Real, unedited output of one full run, captured **before** any Phase 3
destructive-lifecycle work (tasks 102+) landed, at HEAD `b0dfa38` (the last
commit of the prior approach's kept work):

```
$ ci/run.sh go test -p=1 -count=1 -v ./...
```

Full capture (still contains Godog's ANSI colour bytes, consistent with
`docs/reports/test-count-evidence.md`'s convention of anchoring on plain
`=== RUN` lines rather than stripping colour):
[`docs/reports/phase3-baseline-suite.log`](phase3-baseline-suite.log).

- **Wall-clock duration**: `3m17.169s` (`time` around the `ci/run.sh` invocation;
  see the `real` line at the end of the log).
- **Result**: `features` package: 210 scenarios (209 passed, 1 failed), 2170
  steps (2168 passed, 1 failed, 1 skipped) — the failure is the pre-existing,
  already-tracked flake `a fake agent renders a preview fixture once and then
  falls silent#01` (task 114's target; see
  `docs/reports/phase3-findings.md` once task 114 lands). All 9 other packages
  (`cmd/deck`, `cmd/fake-claude`, `cmd/fake-pi`, `internal/agent`,
  `internal/audit`, `internal/config`, `internal/hookrecv`, `internal/service`,
  `internal/store`, `internal/theme`, `internal/tmux`, `internal/tui`) reported
  `ok`. This run is captured **as-is**, flake included, because requirement 45
  calls for real unedited output — not a rerun until green. Task 131's
  post-final-commit ten-run stability evidence is the number that must actually
  be 10/10.
- **Tool versions**, resolved with `ci/run.sh sh -c 'go version; tmux -V'`:
  `go1.25.13 linux/amd64`, `tmux 3.5a`.

## Test-counting convention

Adopted unchanged from `docs/reports/test-count-evidence.md` (Phase 0): only
top-level `go test` invocations count, matched by `^=== RUN   Test[A-Za-z0-9_]+$`
(no `/`, so Godog's per-scenario/per-step `TestFeatures/<...>` subtests are
excluded — they are steps of the one `TestFeatures` umbrella test, not
independent Go tests). `TestFeatures` itself counts as exactly one top-level Go
test by this convention, counted separately from the 210 scenarios / 2170 steps
it drives internally.

```
$ grep -cE '^=== RUN   Test[A-Za-z0-9_]+$' docs/reports/phase3-baseline-suite.log
462
```

**462 top-level Go tests** at this baseline commit.

## Per-requirement evidence

Requirement numbers and wording are `/run/ralphd/composite-prd.md`'s
`## Requirements` section (1–51). Evidence cites real scenario names / Go test
names, each greppable in the tree; a repository-relative capture path is given
where a task recorded one. Rows for requirements delivered by the previous
approach's tasks 001–023/059/060 (all still `completed` in `tasks.json`, kept
per `discovered.approach`) are filled in now. Everything else is `PENDING`.

| # | Requirement (short) | Status | Evidence |
|---|---|---|---|
| 1 | `DECK_UNDO_MS`/`DECK_DELETE_GRACE_MS` knobs, documented, findings | DONE | Parsed and defaulted: `internal/config/config.go:140,144`, asserted by `internal/config/config_test.go` (`DECK_UNDO_MS`/`DECK_DELETE_GRACE_MS` cases incl. malformed-value rejection at line 377). Both wired to real behaviour and both documented in the help overlay (`internal/tui/tui.go`'s `DECK_UNDO_MS`/`DECK_DELETE_GRACE_MS` lines): `DECK_UNDO_MS` gates `x`'s undo-toast window (task 102), `DECK_DELETE_GRACE_MS` gates `dd`'s undo/reap window (task 106) — see `docs/reports/phase3-findings.md`'s "DECK_UNDO_MS/DECK_DELETE_GRACE_MS as new determinism knobs" and "DECK_DELETE_GRACE_MS gains its consumer" sections for the full paper trail, including the grep proving neither knob gates anything but a `tea.Tick` duration. |
| 2 | Both windows tick under frozen `DECK_CLOCK` | DONE | `features/kill_delete_undo.feature` `@requirement-2-monotonic-windows` "both the undo window and the delete grace window keep advancing while DECK_CLOCK is frozen": one client, `DECK_CLOCK` frozen for the whole run, `DECK_UNDO_MS=200`/`DECK_DELETE_GRACE_MS=200`; `x` kills (undo toast appears), `dd` deletes (tombstoned), then 400ms of real wall-clock wait proves the toast is gone AND the row is reaped despite `m.settings.Clock.Now()` never advancing -- both windows are scheduled via a real `tea.Tick`, never `m.settings.Clock` (`internal/tui/tui.go`'s `sessionKilled`/`undoExpired` and `sessionDeleted`/`deleteGraceExpired` handlers). Real capture: `docs/reports/phase3-task106-scenarios.log`. Task 106. |
| 3 | Working-directory fingerprint step, provably able to fail | DONE | `features/harness.feature:10` `@requirement-29-fingerprint-harness` scenario "a directory's fingerprint is byte-for-byte and stat-for-stat identical to itself" (tag misnumbered vs this PRD's own numbering — recorded as a cross-reference finding in `docs/reports/phase3-findings.md`, task 060). Four-mutation-mode negative proof: `features/fingerprint_mutation_test.go` `TestFingerprintDetectsMutations` (content/mtime/new-file/removed-file plus task 060's added mode-change and symlink-target-swap subtests). |
| 4 | Transcript fixture with recorded provenance | DONE | `docs/reports/phase3-fake-pi-transcript-provenance.md` (pi) + `docs/reports/phase3-findings.md`'s "Transcript path/naming convention, per adapter" section (claude, citing real authenticated-Claude-Code capture `docs/reports/phase2-real-claude-authenticated.log`). Fixtures: `cmd/fake-claude/main.go` `transcriptPath`, `cmd/fake-pi/main.go` `transcriptDir`/`encodeCwd`/`createTranscript`/`findExistingTranscript`. |
| 5 | Every create-modal field reachable/editable/described | DONE | `features/dialogs.feature:27` `@requirement-7-every-field-reachable-editable-described`, scenario "create dialog -- every field is reachable by keyboard alone, editable, and each edit is visible". `internal/tui/create_field_help_test.go` `TestCreateFieldRowsEveryFieldHasANonEmptyLabelAndHelp`. |
| 6 | Blank-name default `<workspace>-<MMDD-HHMM>` + collision suffix | DONE | `features/create_session.feature:65` "an empty name defaults to <workspace>-<MMDD-HHMM> from the frozen clock"; `:73` "a second blank-name create in the same directory and minute gets a -2 collision suffix". |
| 7 | Validation stated + input-preserving, esc creates nothing | DONE | `features/create_session.feature` lines 91–159, tags `@requirement-15-duplicate-name`, `-slug-collision`, `-nonexistent-cwd`, `-cwd-not-a-directory`, `-malformed-env-key`, `-malformed-launch-args`, `-esc-abandons` (7 scenarios). |
| 8 | `captured_path` advisory when `login_shell` set | DONE | `features/agent_session.feature:55` "login_shell marks captured_path advisory in the row and its detail". `internal/store/store_test.go` `TestCapturedPathAdvisoryReflectsLoginShellAndPersistsAcrossReopen`; `internal/service/agent_test.go` `TestCreateAgentLoginShellStoresCapturedPathButMarksItAdvisory`; `internal/tui/badge_detail_test.go` `TestDetailViewShowsCapturedPathAdvisoryWhenLoginShellSet`. |
| 9 | `pre_launch` runs before the agent; failure visible without attaching | DONE | `features/crash.feature:46` "a failing pre_launch leaves visible evidence without attaching". `internal/service/agent_test.go` `TestCreateAgentRunsSucceedingPreLaunchBeforeTheAgent`, `TestCreateAgentFailingPreLaunchNeverStartsTheAgent`. |
| 10 | Launch audit: exact argv + env-key names, never a value | PARTIAL | Create + resume: `features/agent_session.feature:71` "the launch audit records environment key names but never a value, across create and resume". Restart's own audit assertion is task 127 (R itself now exists as of task 022, unlike when task 019 split this off). |
| 11 | `recent_cwds` table: resolved path PK, monotonic `used_seq`, dedupe/evict | DONE | `internal/store/store_test.go` `TestPromoteRecentCwdOrdersMostRecentFirstByMonotonicSequence`, `TestPromoteRecentCwdRepromotingExistingPathMovesToFrontWithoutDuplicating`, `TestPromoteRecentCwdLimitZeroKeepsNothing`, `TestPromoteRecentCwdEvictsBeyondCallerSuppliedLimit`, `TestPromoteRecentCwdRejectsRelativePath`, `TestClearRecentCwdsRemovesEveryEntryButNothingElse`; `internal/service/agent_test.go` `TestCreateAgentPromotesCWDToRecentCwds`; `internal/service/shell_test.go` `TestCreateShellPromotesCWDToRecentCwds`. |
| 12 | `cwd` prefilled with most recent entry, "last used" label, wholesale replace | DONE | `features/create_session.feature:13,22,32` `@requirement-12-no-history-startup-cwd`, `-last-used-prefill`, `-wholesale-replace`. |
| 13 | `↑`/`↓` cycle recent list, "recent N/M" | DONE | `features/create_session.feature:40` `@requirement-13-cycle-recent`. |
| 14 | Directory-only ghost completion, dimmed token, `→`/`end` accepts | DONE | `features/create_cwd_ghost.feature` `@requirement-14-ghost-unique-match-right-accepts`, `-ghost-end-accepts`, `-ghost-files-never-candidates`, `-ghost-hidden-only-with-dot-segment`, `-ghost-tilde-expands` (5 scenarios). |
| 15 | Ambiguity ghosts nothing, shows match count (asserted negatively) | DONE | `features/create_cwd_ghost.feature:95` `@requirement-15-ghost-ambiguous-no-completion` "several matches with no further common prefix ghost nothing and show a match count". |
| 16 | `tab` is bash's contract (longest-common-prefix / list candidates) | DONE | `features/create_cwd_tab.feature` `@requirement-16-tab-advances-common-prefix`, `-tab-lists-candidates-and-selects`, `-tab-does-nothing-when-already-unique` (3 scenarios). |
| 17 | Settings offers clearing `recent_cwds`; table stays non-load-bearing | DONE | `features/settings.feature:333` `@requirement-17-clear-recent-cwds-history`. `internal/service/recent_cwds_clear_test.go` `TestClearRecentCwdsIsObservableAndNeverLeaksAPathIntoTheAuditLog`. |
| 18 | `e` env editor shows effective value + winning layer per key | DONE | `features/environment.feature:22` `@requirement-20-env-editor-winning-layer` "a key set in two layers names the session layer as the winner, and PATH names captured_path"; `:42` `@requirement-20-env-editor-esc-changes-nothing`. |
| 19 | Editing env sets `env_dirty`, mirrors via `tmux set-environment`, shows `env↻` | DONE | `features/environment.feature:54` `@requirement-021-env-editor-writes-env-dirty-and-tmux-mirror` "committing an edit writes session env, marks it env_dirty, mirrors into tmux, and leaves the live pane's own environment untouched". |
| 20 | `R` restarts with resume argv, clears `env_dirty`; shell offers inject-instead | DONE | `features/environment.feature:72` `@requirement-022-restart-applies-pending-env-edit-and-clears-badge`; `:96` `@requirement-023-inject-instead-exports-into-live-shell-without-restarting`; `:117` `@requirement-023-inject-instead-restart-path-still-available`. `features/agent_session.feature:33` "R restarts a running claude session with the resume argv, preserving its conversation id". |
| 21 | Secret-shaped values masked everywhere, reveal toggle, files never leak value | PENDING | Task 122. |
| 22 | `x` kills, `stopped`, 10s undo toast, `u` resumes | DONE | `features/kill_delete_undo.feature` `@requirement-22-undo-toast` (3 scenarios): "x kills without confirmation and refuses to kill an already-stopped row", "u undoes the most recent kill inside its DECK_UNDO_MS window", "u does nothing once the undo window has expired". Task 102. |
| 23 | `dd` two-key delete: pending-`d`, confirm, tombstone, 60s undo, reap | DONE | Store foundation (task 104): `internal/store.SoftDeleteSession`/`RestoreSession`/`ReapSession` (`internal/store/store.go`), `ListDeletedSessions` as the reaper's separate accessor, `ListSessions` excludes tombstoned rows by default; covered by `internal/store/tombstone_test.go`. `dd` key/UI (tasks 105/133): `internal/service.Service.Delete` kills the live pane then soft-deletes; `internal/tui.Model`'s `pendingDelete`/`deleteConfirming` render the pending indicator and confirm dialog, joining every open-dialog gating list and the §11.4 mouse/width-clamp contract; help overlay's `dd` line. Grace-window undo + reap (task 106): `internal/service.Service.Restore`/`Reap` (`internal/service/delete.go`, covered by `internal/service/delete_test.go`'s `TestRestoreClearsTombstoneAndReturnsToListSessions`/`TestReapRemovesTombstonedRowPermanently` plus both methods' empty-id guards); `internal/tui.Model`'s `deleteUndoSessionID`/`deleteUndoGeneration` trio and `deleteGraceExpired` tick (mirroring `undoSessionID`/`undoExpired` exactly, but as an independent tracker -- dd's own internal kill step never emits `sessionKilled`), wired through a new `u` branch that falls back to restoring a deleted row once the kill-undo trio is empty; the toast (`deleteUndoNoteLines`) deliberately never names the deleted session, unlike the kill toast, because the existing dd-submit scenario asserts the deleted session's name is gone from the whole screen immediately. `DECK_DELETE_GRACE_MS` help line added, plus its own present-word-list entries in `cmd/deck/main_test.go`/`internal/tui/tui_test.go` (task 001's blocking half is now unblocked). Covered by `features/kill_delete_undo.feature`'s 10 scenarios total (7 from tasks 105/133 plus 3 new: `@requirement-23-delete-undo`, `@requirement-23-delete-reap`, `@requirement-2-monotonic-windows`), run via `DECK_GODOG_TAGS="@requirement-23-delete-undo && ~@real-agents"` etc. Real capture: `docs/reports/phase3-task106-scenarios.log`. |
| 24 | Reaping leaves no trace (cascade rows + deck's own files, JSONL kept) | PENDING | Task 107. |
| 25 | Delete without purge leaves the agent's transcript intact | PENDING | Task 110. |
| 26 | Purge offered only in delete confirm, names exact path, declines honestly | PENDING | Tasks 109–110. |
| 27 | `A` archives (flag not status), requires `stopped`, "kill and archive" | PENDING | Task 111. |
| 28 | `m` marks; `x`/`dd` act on the set; one undo covers the batch | PENDING | Task 112. |
| 29 | R1: cwd fingerprint across every destructive path + every undo | PENDING | Tasks 108, 113. |
| 30 | Every transient message inside the frame budget | PARTIAL | `internal/tui` `TestUndoToastStaysWithinFrameBudgetAtEveryLayoutMode` (undo toast, all 4 layout modes, 80x24) and `TestUndoToastBudgetHoldsAcrossSizes` (5 sizes); `features/layout_modes.feature`'s 8 scenarios unchanged and still green. Task 103 proves the undo toast (the only transient message that exists yet) is counted in `computeLayout`'s reserved rows. The pending-`d` indicator (task 105) and the batch-undo message (task 112) do not exist yet; each of those tasks must add its own line to the same reserved-row expression and is expected to extend this row's evidence when it does. |
| 31 | Rename inside `i` detail dialog only, tmux name unchanged, stated on screen | PENDING | Task 125. |
| 32 | `E` event log: newest first, kind/reason/bounded payload, masked env | PENDING | Task 124. |
| 33 | `/` filters by name/workspace/cwd; also how archived rows are reached | PENDING | Task 123. |
| 34 | `[ui] group_by_workspace` declared once in schema | PENDING | Task 120. |
| 35 | Flat, header-free sidebar when grouping off; page size/elision in both modes | PENDING | Task 120. |
| 36 | Navigation identical in both modes (non-adjacent-workspace regression) | PENDING | Task 121. |
| 37 | `g`/`G` top/bottom; collapse rebound off a colliding key | PENDING | Task 119. |
| 38 | Cross-check test: help keymap vs bound keys, fails either direction | PENDING | Task 128. |
| 39 | Help overlay lists this phase's new keys within the frame budget | PENDING | Task 128. |
| 40 | Store durability contract decided and proven | PENDING | Task 126. |
| 41 | `features/create_session.feature` covers agent choice/cwd/args/env/pre_launch/name collisions/blank-name/§11.7 | PARTIAL | File exists and covers agent choice, cwd, args, name collisions, blank-name default (rows above). §11.7's ghost-completion and tab-completion scenarios live in the sibling files `features/create_cwd_ghost.feature` and `features/create_cwd_tab.feature` rather than inside `create_session.feature` itself — a file-organisation deviation from the PRD's literal wording, not a coverage gap; recorded here rather than silently. |
| 42 | `features/environment.feature` covers §6 layering, `env↻`, restart, inject, masking, no-leak | PARTIAL | File exists and covers layering/winner, `env↻`, restart-only-applies, inject (rows 18–20 above). Masking + read-the-files no-leak assertions are requirement 21, still `PENDING` (task 122). |
| 43 | `features/kill_delete_undo.feature` | PENDING | File does not exist yet. Tasks 102, 105, 106, 108, 111, 112, 113. |
| 44 | Existing scenarios (`layout_modes`, `preview`, `attention_sort`, `mouse`, `settings`, `themes`, `dialogs`, `status_recovery`, `durable_identity`, `launch_lease`, `lease_race`, `concurrency`, golden 80×24) keep passing | DONE (this run) | All present in `docs/reports/phase3-baseline-suite.log`'s `features` package output at this baseline commit; the run's one failure is the tracked flake (task 114), not one of these files. Re-verified at every later task's targeted run per the standing rules; final confirmation is task 132's full-suite run. |
| 45 | `docs/reports/phase3.md` with real output, per-requirement evidence, tool versions, wall-clock, gotchas | IN PROGRESS | This file (task 101). Gotchas accumulate here as later tasks add them; task 130 does the final completeness pass. |
| 46 | Ten consecutive clean-state suite passes, collected after the last commit | PENDING | Task 131. |
| 47 | No scenario deleted/skipped/tag-excluded to pass | DONE (policy, ongoing) | `defaultTags` unchanged (`~@real-agents && ~@nightly`, standing rule); no scenario removed by any task 101–132 commit — reverify at task 132. |
| 48 | Wheel notch scrolls the attached pane; does not type | PENDING | Tasks 115–116. |
| 49 | Scrolling never flips the badge; copy-mode probe experiment recorded | PENDING | Task 117. |
| 50 | `tmux_mouse` config key + `DECK_TMUX_MOUSE`, declared once, `false` = today | PENDING | Task 115. |
| 51 | Multi-rune `KeyMsg` dispatches every rune in navigation path | PENDING | Task 118. |

## Gotchas discovered so far

- **The baseline run's one failure is a pre-existing flake, not a regression.**
  `a fake agent renders a preview fixture once and then falls silent#01` in
  `features/harness.feature` failed in this task's own captured baseline run
  (`docs/reports/phase3-baseline-suite.log`). Forgetting this and treating any
  future red run of that scenario as "the previous task broke something" wastes
  a whole iteration chasing a regression that isn't there. It is task 114's to
  fix; until then, expect roughly 1 run in 3–4 to show it.
- **`ci/run.sh go test -p=1 -count=1 ./...` takes ~3m17s uncached-of-flake-retries** —
  budget accordingly; the standing rule capping this to once per iteration exists
  because of exactly this cost, not as a formality.
- **Requirement-tag numbers in existing feature files do not match this PRD's
  own numbering** (e.g. `@requirement-29-fingerprint-harness` in
  `harness.feature` is this PRD's requirement 3, not 29 — task 060 already
  recorded this cross-reference slip as a finding). Do not use `grep
  @requirement-N` as a shortcut for "which PRD requirement is this" without
  checking the scenario's actual content against the number.

## Next steps

Task 102 onward fill the `PENDING` rows in dependency order (store primitives
before the keys that use them; the tombstone before the undo window; the
fingerprint assertions once the destructive paths exist to fingerprint). Each
task should update its own row(s) in the table above in the same commit that
delivers the behaviour — do not defer evidence to the end the way the previous
approach did.
