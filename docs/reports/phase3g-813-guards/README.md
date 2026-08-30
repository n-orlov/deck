# Task 813 — guard re-verification at the run's true final sha

Starting sha for this task: **`b1dbfc4`** (HEAD after task 812,
`docs: publish ci/stability.sh 10's observed 0/10 rate at the final code
commit, root-caused (task 812)`), clean, `== origin/main`. Every claim below
is scoped to the run range `1cfbd5a..HEAD` (`1cfbd5a` is the run's own base
sha) — **never to full repository history**, which is what sank task 113
(see the standing rules' own citation of that lesson).

One log per guard below, each carrying its exact command and an exit status
captured in the same shell call. Because a commit cannot quote its own sha,
a follow-up addendum commit names this bundle's own final sha and re-shows
the clean-tree / HEAD-equals-origin/main pair at it (per this task's own
criteria).

## Round 2 correction — what the first round got wrong

The first round of this bundle (`8eaf474` + `72c936d`) carried three defects.
All three are addressed here, in a third, follow-up commit; nothing above or
below is quietly rewritten to hide them.

1. **Guard (b) claimed empty output that its own log contradicts.**
   `guard-b-clean-tree.log` records `?? docs/reports/phase3g-813-guards/`,
   while the prose said "Empty". Corrected in place below: the first run's
   unedited output is published *with* the `??` line, and a round-2 re-run at
   `72c936d` — captured before any file of this round existed in the worktree —
   supplies the genuinely empty output the guard asks for.
2. **Guard (g)'s quoted output was truncated with an editorial `[...]`.**
   Corrected below: the full, unedited sweep output is now inline, byte-for-byte
   as the script printed it, with no elision.
3. **Guard (e) cannot hold as this task's criteria state it.** The criteria
   require `git diff 1cfbd5a..HEAD -- features/godog_test.go` to be *empty*;
   the real output is not empty and cannot be made empty by any work this run
   is permitted to do. See guard (e) below for the full reasoning, the two
   commits responsible and the shas. This task is therefore filed
   `unsatisfiable` on that one criterion; every other guard in this bundle
   stands, re-verified at `72c936d`.

Every guard below was re-run unchanged at `72c936d` (the last preceding
commit at the time of this correction) into a `round2-*.log` companion; the
original round-1 logs are left untouched as the record of what round 1
actually observed.

## (a) Protected paths — `guard-a-protected-paths.log`, `round2-guard-a-protected-paths.log`

```
$ git log --all --oneline 1cfbd5a.. -- SPEC.md prds/ ci/Dockerfile ci/SPIKE.md
b69b5ba docs: amend SPEC.md §11.8 for R93's visible in-progress selection (task 205)
exit=0
```

Exactly one commit, `b69b5ba` — steering 018's licensed SPEC §11.8 amendment
for R93 — and nothing else, over the run range `1cfbd5a..HEAD`. This is a
claim about that range only: 30 pre-base commits touch these protected paths
outside it, and no claim is made about those. Round 2's re-run at `72c936d`
(`round2-guard-a-protected-paths.log`) prints exactly the same one line.

## (b) Clean tree — `guard-b-clean-tree.log`, `round2-guard-bc-clean-tree-head.log`

Round 1, at starting sha `b1dbfc4` — unedited, including the line round 1's
prose wrongly omitted:

```
$ git status --porcelain
?? docs/reports/phase3g-813-guards/
exit=0
```

That one `??` line is this bundle's own directory, still untracked at the
moment the guard ran: no tracked file was modified, added or deleted, but the
guard as this task words it ("`git status --porcelain` empty") is only met once
the bundle itself is committed. Round 2 supplies exactly that, at `72c936d`
(the round-1 addendum commit, i.e. after the last preceding commit), with the
output captured into `/tmp` *before* any file of this round was created in the
worktree, so this round's own work cannot appear in it either:

```
$ git status --porcelain
exit=0
```

Empty.

## (c) HEAD == origin/main — `guard-c-head-vs-origin.log`, `round2-guard-bc-clean-tree-head.log`

Round 2, at `72c936d`, from the same capture as (b) above:

```
$ git rev-parse HEAD
72c936df0cad67d5fe11338a1c8e5d57e2dbda51
exit=0

$ git log --oneline -1 origin/main
72c936d docs: name this guard bundle's own final sha and re-show its clean-tree/HEAD==origin/main pair (task 813)
exit=0
```

Both name `72c936d`: local `HEAD` and `origin/main` agree, nothing unpushed.
Round 1's own observation at its starting sha follows.


```
$ git rev-parse HEAD
b1dbfc494d6c13ed0c5a3413cac793b621104234
exit=0

$ git log --oneline -1 origin/main
b1dbfc4 docs: publish ci/stability.sh 10's observed 0/10 rate at the final code commit, root-caused (task 812)
exit=0
```

Both name `b1dbfc4` at the starting point. (This task's own commit and its
push advance both together, as a consequence of completing, not a property
of the starting sha this guard is about — restated at the true final sha in
the addendum commit below.)

## (d) `git diff --stat` since task 812's stability sha — `guard-d-diffstat-since-812-sha.log`, `round2-guard-d-diffstat.log`

Task 812's own report (`docs/reports/phase3g-812-stability10/README.md`)
names **`17b1649`** as "HEAD at launch and at commit time" — the code state
`ci/stability.sh 10` actually ran against, before task 812's own docs-only
commit (`b1dbfc4`) landed. That is "task 812's stability sha" this task's
own criterion refers to.

```
$ git diff --stat 17b1649..HEAD -- '*.go' '*.feature' '*.sh' '*.toml' go.mod go.sum
exit=0
```

Empty: task 812's own commit touched only files under
`docs/reports/phase3g-812-stability10/`, none matching these globs, so the
code+CI tree `ci/stability.sh 10` exercised is byte-identical to the current
tree. (This is a different, narrower question than task 812's own report
answered — that report checked the same globs against `fdf4507`, the last
commit to touch `.go`/`.feature` code, and found one non-empty line, a
docs-evidence `.sh` script from task 810 round 2 that happens to match the
unrestricted `'*.sh'` glob though it is not product or CI code. This guard
uses `17b1649` — the sha task 812's own report names as its literal "code
sha" — which case task 812 also confirmed is trivially empty since it's a
commit diffed against itself once HEAD was still `17b1649`; here HEAD has
since advanced to `b1dbfc4`, docs-only, so it remains empty.) Round 2's
re-run at `72c936d` (`round2-guard-d-diffstat.log`) is empty too: the two
commits HEAD gained since — `8eaf474` and `72c936d`, this bundle's own — add
only files under `docs/reports/phase3g-813-guards/`, and `citation_sweep.py`
is the one file among them matching none of these globs by design (it is
`.py`, not `.sh`).

## (e) `features/godog_test.go` — `guard-e-godog-defaulttags.log`, `round2-guard-e-godog.log`

**This guard's stated form is not satisfiable, and this report does not claim
it is.** The criterion requires `git diff 1cfbd5a..HEAD -- features/godog_test.go`
to be *empty*. It is not, at `72c936d` or at any other sha in this run, and it
cannot be made empty:

```
$ git log --oneline 1cfbd5a..HEAD -- features/godog_test.go
6718823 tmux: reclaim a leaked interactive pipe at the next start (task 030)
904419c features: prove R76's repair through the field's own hook route, not a keypress (task 002)
exit=0
```

Both commits are approach 01's, pushed long before this approach existed, and
each adds exactly one `register…Steps(sc)` line for a feature file the same
commit adds (`features/terminal_repair_field_route.feature`,
`features/interactive_pipe_leak.feature`). Deleting either line would leave
that feature file's steps undefined, which the suite's own
`TestGodogRejectsUndefinedAndFailedSteps` exists to fail on; deleting the
commits would rewrite published history, which the standing rules forbid
outright. So no permitted action makes this diff empty — the criterion asserts
a false fact about the repository rather than setting a bar to clear.

What the standing rules actually guard here — `defaultTags`, the one authority
for suite coverage — *is* byte-unchanged over the whole run range, and that is
shown below. The unedited diff, published in full:

```
$ git diff 1cfbd5a..HEAD -- features/godog_test.go
diff --git a/features/godog_test.go b/features/godog_test.go
index 24aa98d..2b380eb 100644
--- a/features/godog_test.go
+++ b/features/godog_test.go
@@ -119,6 +119,7 @@ func initializeScenario(sc *godog.ScenarioContext) {
 	registerSortOrderSteps(sc)
 	registerNewSessionSelectionSteps(sc)
 	registerStatusRecoverySteps(sc)
+	registerTerminalRepairFieldRouteSteps(sc)
 	registerSettingsSteps(sc)
 	registerDialogsSteps(sc)
 	registerEnvEditorSteps(sc)
@@ -138,6 +139,7 @@ func initializeScenario(sc *godog.ScenarioContext) {
 	registerInteractiveScrollSteps(sc)
 	registerInteractiveSelectionSteps(sc)
 	registerInteractiveRetargetSteps(sc)
+	registerInteractivePipeLeakSteps(sc)
 }

 exit=0

$ grep -n "const defaultTags" features/godog_test.go
16:const defaultTags = "~@real-agents && ~@nightly"
exit=0
```

Byte-identical to task 606's own re-verification of this same guard at its
own, much earlier, HEAD (`2bad935`): the only two hunks in the entire run
range still add step registrations for two feature files added early in the
run (`terminal_repair_field_route.feature`, `interactive_pipe_leak.feature`);
none of tasks 701–813 touched this file again. `defaultTags` itself is
byte-unchanged: `~@real-agents && ~@nightly` — which is the property the
standing rules require, and the one this run can honestly claim.

## (f) Every `t.Skip(` added in `1cfbd5a..HEAD` — `guard-f-tskip-audit.log`, `round2-guard-f-tskip.log`

```
$ git diff 1cfbd5a..HEAD -- '*.go' '*.feature' '*.sh' '*.toml' | grep -n '^+.*t\.Skip('
9123:+		t.Skip("this theme's key and hint tokens share a colour; the prose-vs-key distinctness assertion below would be vacuous")
9193:+		t.Skip("this theme's key and hint tokens share a colour; the prose-vs-key distinctness assertion below would be vacuous")
9736:+		t.Skip("this theme's key and hint tokens share a colour; the prose-vs-key distinctness assertion below would be vacuous")
11803:+		t.Skip("this theme's key and hint tokens happen to share a colour; the distinctness assertion below would be vacuous")
14471:+		t.Skip("this theme's key and hint tokens share a colour; the prose-vs-key distinctness assertion below would be vacuous")
exit=0
```

Widened to every `.go`/`.feature`/`.sh`/`.toml` file (not just `*_test.go`,
per this task's own criterion) — the result is unchanged: all five hits are
in Go test files, none in a `.feature`/`.sh`/`.toml` file. Mapped to file and
test by line-number lookup against `git diff`'s own `+++ b/...` headers, then
confirmed by `grep -n 't.Skip(\|^func Test'` against each named file at
HEAD:

| # | file | test | guard text | line (HEAD) |
|---|---|---|---|---|
| 1 | `internal/tui/archive_delete_confirm_theme_test.go` | `TestArchiveConfirmTokensMatchSpec` | "share a colour" | 159 |
| 2 | `internal/tui/archive_delete_confirm_theme_test.go` | `TestDeleteConfirmTokensMatchSpec` | "share a colour" | 229 |
| 3 | `internal/tui/bulk_delete_theme_test.go` | `TestBulkDeleteConfirmTokensMatchSpec` | "share a colour" | 305 |
| 4 | `internal/tui/env_editor_theme_test.go` | `TestEnvViewClosingHintKeysAreKeyToken` | "happen to share a colour" | 279 |
| 5 | `internal/tui/profile_pin_restart_theme_test.go` | `TestProfileSwitchTokensMatchSpec` | "share a colour" | 202 |

All five are the same class of guard: `if keyHex == hintHex { t.Skip(...) }`.
Each test asserts that a footer legend's bound-key word renders in
`theme.Key` while the surrounding prose renders in `theme.Hint` — i.e. that
the two tokens are visually distinct. If the active built-in theme's `Key`
and `Hint` colours happen to be identical (or quantise to the same hex),
that one comparison has nothing to distinguish and would pass or fail on a
tautology rather than any real rendering behaviour, so it is skipped; every
other assertion in each test (title/dimmed/key-vs-prose, etc.) still runs
unconditionally regardless of this guard. `git show 1cfbd5a:<path>` exits
128 for all four files (confirmed above) — every one is a brand-new file
added within the run range, so all five additions are genuinely new, not
edits to a pre-existing skip.

**Difference from task 606's own audit of this same guard.** Task 606's
report (`docs/reports/phase3g-606-guards/README.md`, guard (f)) named the
same five raw grep lines but enumerated only four files/tests (missing
`internal/tui/profile_pin_restart_theme_test.go`) — because
`profile_pin_restart_theme_test.go` did not exist yet at task 606's own HEAD
(`2bad935`); it was added by a later task in this run, still within
`1cfbd5a..HEAD`, before task 813. This report's enumeration is the current,
complete one; task 606's is left as historical record for its own HEAD, not
corrected retroactively.

None of the five disables a test outright: each still runs its non-vacuous
assertions unconditionally, and only skips the single comparison that would
be tautological under the specific colour coincidence the guard checks for.
Round 2's re-run at `72c936d` (`round2-guard-f-tskip.log`) prints the same
five lines: this run added no sixth `t.Skip(` anywhere.

## (g) Citation sweep — `guard-g-citation-sweep.log`, `round2-guard-g-citation-sweep.log`, `citation_sweep.py`

Re-run at `72c936d` (round 2; round 1 ran it at this task's starting sha
`b1dbfc4`, with the same result) over `docs/reports/
phase3g.md` and `docs/reports/phase3g-findings.md` — the exact same script
task 606 wrote (`citation_sweep.py`, copied verbatim into this directory,
byte-identical to `docs/reports/phase3g-606-guards/citation_sweep.py`,
confirmed by `diff`), unmodified: it is repo-root-relative and re-reads the
two files fresh, so it needed no code change to check the current, larger
document state (both files grew substantially across tasks 809/810/812).

```
$ python3 docs/reports/phase3g-813-guards/citation_sweep.py
=== 1. sha citations (single-backtick spans only; fenced blocks excluded) ===
134 unique candidate shas across both files
non-resolving shas: (none)

=== 2. markdown link targets ===
207 link targets checked
unresolved: (none)

=== 3. backtick-quoted repository paths (single-backtick spans only) ===
464 candidate backtick paths checked
unresolved by the fixed base-directory list: ['/run/ralphd/approaches/NN/tasks.json', '001-202.md', '9/10', 'README.md', 'citation-sweep.log', 'citation_sweep.py', 'criterion-a-greps.log', 'dialog-contrast-v.log', 'f37-repro.log', 'features-go-test.log', 'features-suite.log', 'full-features-suite-after-interactive-scroll-fix.log', 'full-features-suite.log', 'full-suite-verbose.log', 'full-suite.log', 'full-target-suite.log', 'go-test-features.log', 'go-test-overlay-line-scroll-verbose.log', 'go-test-parity-verbose.log', 'go-test-tui.log', 'godog-run.log', 'gotest-output-buffering-probe.log', 'green-1.log', 'green-2.log', 'green-3.log', 'green-80x24-bounds.log', 'green-after-fix-feature.log', 'green-after-fix-unit.log', 'green-after-fix.log', 'green-dialogs-feature.log', 'green-filter-feature.log', 'green-footer-eligibility.log', 'green-internal-tui.log', 'green-lease-release-failure.log', 'green-mutation-reverted-canReachPane-check.log', 'green-mutation-reverted-full-package.log', 'green-mutation-reverted.log', 'green-mutations-reverted.log', 'green-post-fix.log', 'green-run-1.log', 'green-run-5.log', 'green-run1.log', 'green-run2.log', 'green-run3.log', 'green-service-cleanup-present.log', 'green-store-archived.log', 'green-store-last-create-agent.log', 'green-store-service-packages.log', 'green-tui-last-used-agent.log', 'green.log', 'inject.go', 'interactive-scroll-run1.log', 'internal-tui-go-test.log', 'internal-tui-tests.log', 'main.go', 'mutation-rectangular.log', 'mutation-renderer-noop.log', 'mutation-unrendered-note.log', 'negative-recorded-ratio-drift-fails.log', 'negative-unlisted-pair-fails.log', 'no_leak_scan.log', 'notes.md', 'original-assertion-red.log', 'overlays-and-parity-green.log', 'positive-control-red.log', 'post-change-run1.log', 'prds/phase3g-residuals-and-suite-determinism.md', 'pre-change-red.log', 'preview-floor-assertion-forced-red.log', 'protected-path-all-refs.log', 'protected-path-check.log', 'receiver.go:222', 'red-2-spent-verdicts-refreeze.log', 'red-P-added-back.log', 'red-before-fix-feature.log', 'red-comma-removed.log', 'red-disabled-clear-loadbearing.log', 'red-feature-summary.log', 'red-feature-tail.log', 'red-mutation-full-package.log', 'red-mutation-inlined-copy.log', 'red-mutation-no-canReachPane-check.log', 'red-mutation-no-canRestart-check.log', 'red-mutation-no-cankill-check.log', 'red-mutation-no-check.log', 'red-mutation.log', 'red-pre-fix.log', 'red-worktree-trial.log', 'run1.log', 'run3.log', 'run5.log', 'settings-go-untouched.log', 'targeted-suite-tui.log', 'task010-bounded-batch-mutation-red.log', 'task010-frozen-clock-mutation-red.log', 'task011-callsite-removed-mutation-red.log', 'task011-green-store-service-cmd-deck.log', 'tasks.json', 'theme-suite-green.log']

=== 3b. manual disposition of every string unresolved above ===
(class E: recursively search docs/reports/ for a same-named file, since
 the surrounding prose names a specific report subdirectory a few words
 earlier that the fixed base-directory list above does not try; classes
 B/C/D/F: known, hardcoded, deliberate non-resolutions)

  DELIBERATE  '001-202.md': class F (new) -- not a repository file at all: the identifier of an operator ruling delivered to this run outside the repository. phase3g-findings.md's own F33 names it plainly ('operator ruling identified as `001-202`... delivered to this run outside the repository, not part of the tracked record, so not linked here'); phase3g.md's task-202 table row and its prose two lines later abbreviate the same identifier with a `.md` suffix and no repeated disclosure at that exact sentence. The disclosure exists elsewhere in the same two-file record (findings.md F33), so the citation is deliberate and accounted for, not silently missing.
  RESOLVED    'README.md': found at ['docs/reports/phase3d-202-resize-render-wait/README.md', 'docs/reports/phase3d-203-floor-unit-test/README.md', 'docs/reports/phase3d-204-shrink-below-floor/README.md', 'docs/reports/phase3d-207-pipe-race/README.md', 'docs/reports/phase3d-208-final-sweep/README.md', 'docs/reports/phase3d-209-stability/README.md', 'docs/reports/phase3d-217-final-sweep/README.md', 'docs/reports/phase3d-210-stability-10of10/README.md', 'docs/reports/phase3d-212-closeout/README.md', 'docs/reports/phase3e-302-r52-godog/README.md', 'docs/reports/phase3e-307-r53-orders/README.md', 'docs/reports/phase3e-fullsuite/README.md', 'docs/reports/phase3e-stability/README.md', 'docs/reports/phase3e-328-closeout/README.md', 'docs/reports/phase3e-334-navigation-settle/README.md', 'docs/reports/phase3e-401-r54-noop-discriminator/README.md', 'docs/reports/phase3e-402-r54-noop-sidebar-scoped-click/README.md', 'docs/reports/phase3e-403-r54-noop-observable-settles/README.md', 'docs/reports/phase3e-404-r57-seam-body-cell/README.md', 'docs/reports/phase3e-405-r55-hittargetnone-noop/README.md', 'docs/reports/phase3e-406-mouse-off-frame-race/README.md', 'docs/reports/phase3e-406-mouse-off-frame-race/pre-fix-repro/README.md', 'docs/reports/phase3e-406-mouse-off-frame-race/pre-fix-repro/loadavg-redo/README.md', 'docs/reports/phase3e-406-mouse-off-frame-race/post-fix-sweep-scoped/README.md', 'docs/reports/phase3e-406-mouse-off-frame-race/post-fix-sweep-scoped/loadavg-redo/README.md', 'docs/reports/phase3e-406-mouse-off-frame-race/attach-scroll-regression-check/README.md', 'docs/reports/phase3e-406-mouse-off-frame-race/tmuxwrap-experiment/README.md', 'docs/reports/phase3e-411-locatetext-preview-region/README.md', 'docs/reports/phase3e-407-whole-suite-at-75861e0/README.md', 'docs/reports/phase3e-408-stability-10-at-75861e0/README.md', 'docs/reports/phase3e-409-findings-4a-correction/README.md', 'docs/reports/phase3e-410-closeout/README.md', 'docs/reports/phase3f-021-fullsuite/README.md', 'docs/reports/phase3f-022-stability10/README.md', 'docs/reports/phase3f-evidence/README.md', 'docs/reports/phase3f-027-closeout/README.md', 'docs/reports/phase3f-028-f1-passive-fit-floor/README.md', 'docs/reports/phase3f-029-r74-launch-generation/README.md', 'docs/reports/phase3f-030-r74-superseded-hooks/README.md', 'docs/reports/phase3f-031-r75-launch-lease-release/README.md', 'docs/reports/phase3f-032-fullsuite/README.md', 'docs/reports/phase3f-033-stability10/README.md', 'docs/reports/phase3f-038-closeout/README.md', 'docs/reports/phase3g-001-r76-terminal-row-live-pane/README.md', 'docs/reports/phase3g-002-r76-field-route/README.md', 'docs/reports/phase3g-003-r77-name-reuse-reap/README.md', 'docs/reports/phase3g-014-footer-fixed-set/README.md', 'docs/reports/phase3g-015-footer-bindings-parity/README.md', 'docs/reports/phase3g-027-esc-clears-held-filter/README.md', 'docs/reports/phase3g-029-inject-retained-dead-pane/README.md', 'docs/reports/phase3g-030-reclaim-leaked-interactive-pipe/README.md', 'docs/reports/phase3g-031-sigterm-panic-interactive-cleanup/README.md', 'docs/reports/phase3g-034-previewfit-derivation/README.md', 'docs/reports/phase3g-035-previewfit-no-live-pane-latch/README.md', 'docs/reports/phase3g-038-r86-dialog-arrow-nav/README.md', 'docs/reports/phase3g-010-011-tombstone-sweep-evidence/README.md', 'docs/reports/phase3g-038-r91-previewfit-latch/README.md', 'docs/reports/phase3g-038-r78-archived-name-dd/README.md', 'docs/reports/phase3g-038-r80-r81-footer-eligibility/README.md', 'docs/reports/phase3g-038-r83-dialog-frame-bound/README.md', 'docs/reports/phase3g-038-r84-contrast-floor-absent/README.md', 'docs/reports/phase3g-038-r85-last-used-agent/README.md', 'docs/reports/phase3g-038-r92-lease-release-failure/README.md', 'docs/reports/phase3g-101-agent-wait-wrap/README.md', 'docs/reports/phase3g-102-clear-recents-label/README.md', 'docs/reports/phase3g-103-hook-sessionend-repair/README.md', 'docs/reports/phase3g-104-title-independent-waits/README.md', 'docs/reports/phase3g-105-rename-selection/README.md', 'docs/reports/phase3g-106-contrast-floor/README.md', 'docs/reports/phase3g-107-dialog-degradation-net/README.md', 'docs/reports/phase3g-108-r86-proof/README.md', 'docs/reports/phase3g-109-report-update/README.md', 'docs/reports/phase3g-110-findings-update/README.md', 'docs/reports/phase3g-111-fullsuite/README.md', 'docs/reports/phase3g-112-stability10/README.md', 'docs/reports/phase3g-113-closeout/README.md', 'docs/reports/phase3g-201-rename-reuse-cleanup/README.md', 'docs/reports/phase3g-202-tombstone-drain/README.md', 'docs/reports/phase3g-203-r82-assertion-conflict/README.md', 'docs/reports/phase3g-204-async-db-assert-sync/README.md', 'docs/reports/phase3g-206-r93-visible-selection/README.md', 'docs/reports/phase3g-301-review-findings-closure/README.md', 'docs/reports/phase3g-302-stability10/README.md', 'docs/reports/phase3g-303-help-pty-tail-sync/README.md', 'docs/reports/phase3g-303-sigwinch-count-pace/README.md', 'docs/reports/phase3g-501-create-env-race/README.md', 'docs/reports/phase3g-502-attention-count-sync/README.md', 'docs/reports/phase3g-503-attach-scroll-sync/README.md', 'docs/reports/phase3g-504-report-update/README.md', 'docs/reports/phase3g-505-stability10/README.md', 'docs/reports/phase3g-512-stability10/README.md', 'docs/reports/phase3g-506-sigwinch-disposition/README.md', 'docs/reports/phase3g-507-stability10/README.md', 'docs/reports/phase3g-507-sigwinch-startup-race/README.md', 'docs/reports/phase3g-508-fullsuite/README.md', 'docs/reports/phase3g-604-fullsuite/README.md', 'docs/reports/phase3g-605-stability-gate/README.md', 'docs/reports/phase3g-606-guards/README.md', 'docs/reports/phase3g-607-closeout/README.md', 'docs/reports/phase3g-701-r76-bare-error/README.md', 'docs/reports/phase3g-702-r76-error-fallout/README.md', 'docs/reports/phase3g-703-r76-error-fallout-fix/README.md', 'docs/reports/phase3g-803-hook-truth-stopfailure/README.md', 'docs/reports/phase3g-804-sort-order-error-route/README.md', 'docs/reports/phase3g-805-stale-tmux-verdict/README.md', 'docs/reports/phase3g-806-archive-eligibility/README.md', 'docs/reports/phase3g-807-kill-eligibility/README.md', 'docs/reports/phase3g-808-footer-handler-agreement/README.md', 'docs/reports/phase3g-810-findings/README.md', 'docs/reports/phase3g-812-stability10/README.md', 'docs/reports/phase3g-813-guards/README.md'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'citation-sweep.log': found at ['docs/reports/phase3e-409-findings-4a-correction/citation-sweep.log', 'docs/reports/phase3e-410-closeout/citation-sweep.log', 'docs/reports/phase3f-027-closeout/citation-sweep.log', 'docs/reports/phase3f-038-closeout/citation-sweep.log', 'docs/reports/phase3g-113-closeout/citation-sweep.log', 'docs/reports/phase3g-504-report-update/citation-sweep.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'citation_sweep.py': found at ['docs/reports/phase3e-410-closeout/citation_sweep.py', 'docs/reports/phase3f-027-closeout/citation_sweep.py', 'docs/reports/phase3g-113-closeout/citation_sweep.py', 'docs/reports/phase3g-606-guards/citation_sweep.py', 'docs/reports/phase3g-813-guards/citation_sweep.py'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'criterion-a-greps.log': found at ['docs/reports/phase3g-108-r86-proof/criterion-a-greps.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'dialog-contrast-v.log': found at ['docs/reports/phase3g-106-contrast-floor/dialog-contrast-v.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'f37-repro.log': found at ['docs/reports/phase3g-810-findings/f37-repro.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'features-go-test.log': found at ['docs/reports/phase3g-107-dialog-degradation-net/features-go-test.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'features-suite.log': found at ['docs/reports/phase3g-702-r76-error-fallout/features-suite.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'full-features-suite-after-interactive-scroll-fix.log': found at ['docs/reports/phase3g-703-r76-error-fallout-fix/full-features-suite-after-interactive-scroll-fix.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'full-features-suite.log': found at ['docs/reports/phase3g-703-r76-error-fallout-fix/full-features-suite.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'full-suite-verbose.log': found at ['docs/reports/phase3g-508-fullsuite/full-suite-verbose.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'full-suite.log': found at ['docs/reports/phase3g-111-fullsuite/full-suite.log', 'docs/reports/phase3g-508-fullsuite/full-suite.log', 'docs/reports/phase3g-604-fullsuite/full-suite.log', 'docs/reports/phase3g-604-fullsuite/superseded-2f0ef61/full-suite.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'full-target-suite.log': found at ['docs/reports/phase3g-031-sigterm-panic-interactive-cleanup/full-target-suite.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'go-test-features.log': found at ['docs/reports/phase3g-108-r86-proof/go-test-features.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'go-test-overlay-line-scroll-verbose.log': found at ['docs/reports/phase3g-108-r86-proof/go-test-overlay-line-scroll-verbose.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'go-test-parity-verbose.log': found at ['docs/reports/phase3g-108-r86-proof/go-test-parity-verbose.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'go-test-tui.log': found at ['docs/reports/phase3g-108-r86-proof/go-test-tui.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'godog-run.log': found at ['docs/reports/phase3g-104-title-independent-waits/godog-run.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'gotest-output-buffering-probe.log': found at ['docs/reports/phase3g-508-fullsuite/gotest-output-buffering-probe.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'green-1.log': found at ['docs/reports/phase3e-stability/redproof-interactive-goroutine-leak/green-1.log', 'docs/reports/phase3g-101-agent-wait-wrap/green-1.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'green-2.log': found at ['docs/reports/phase3g-101-agent-wait-wrap/green-2.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'green-3.log': found at ['docs/reports/phase3g-101-agent-wait-wrap/green-3.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'green-80x24-bounds.log': found at ['docs/reports/phase3g-038-r83-dialog-frame-bound/green-80x24-bounds.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'green-after-fix-feature.log': found at ['docs/reports/phase3g-030-reclaim-leaked-interactive-pipe/green-after-fix-feature.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'green-after-fix-unit.log': found at ['docs/reports/phase3g-030-reclaim-leaked-interactive-pipe/green-after-fix-unit.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'green-after-fix.log': found at ['docs/reports/phase3g-001-r76-terminal-row-live-pane/green-after-fix.log', 'docs/reports/phase3g-029-inject-retained-dead-pane/green-after-fix.log', 'docs/reports/phase3g-038-r86-dialog-arrow-nav/green-after-fix.log', 'docs/reports/phase3g-038-r91-previewfit-latch/green-after-fix.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'green-dialogs-feature.log': found at ['docs/reports/phase3g-105-rename-selection/green-dialogs-feature.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'green-filter-feature.log': found at ['docs/reports/phase3g-038-r78-archived-name-dd/green-filter-feature.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'green-footer-eligibility.log': found at ['docs/reports/phase3g-038-r80-r81-footer-eligibility/green-footer-eligibility.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'green-internal-tui.log': found at ['docs/reports/phase3g-105-rename-selection/green-internal-tui.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'green-lease-release-failure.log': found at ['docs/reports/phase3g-038-r92-lease-release-failure/green-lease-release-failure.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'green-mutation-reverted-canReachPane-check.log': found at ['docs/reports/phase3g-808-footer-handler-agreement/green-mutation-reverted-canReachPane-check.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'green-mutation-reverted-full-package.log': found at ['docs/reports/phase3g-808-footer-handler-agreement/green-mutation-reverted-full-package.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'green-mutation-reverted.log': found at ['docs/reports/phase3g-806-archive-eligibility/green-mutation-reverted.log', 'docs/reports/phase3g-807-kill-eligibility/green-mutation-reverted.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'green-mutations-reverted.log': found at ['docs/reports/phase3g-806-archive-eligibility/green-mutations-reverted.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'green-post-fix.log': found at ['docs/reports/phase3g-701-r76-bare-error/green-post-fix.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'green-run-1.log': found at ['docs/reports/phase3g-102-clear-recents-label/green-run-1.log', 'docs/reports/phase3g-805-stale-tmux-verdict/green-run-1.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'green-run-5.log': found at ['docs/reports/phase3g-805-stale-tmux-verdict/green-run-5.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'green-run1.log': found at ['docs/reports/phase3g-103-hook-sessionend-repair/green-run1.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'green-run2.log': found at ['docs/reports/phase3g-103-hook-sessionend-repair/green-run2.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'green-run3.log': found at ['docs/reports/phase3g-103-hook-sessionend-repair/green-run3.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'green-service-cleanup-present.log': found at ['docs/reports/phase3g-003-r77-name-reuse-reap/green-service-cleanup-present.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'green-store-archived.log': found at ['docs/reports/phase3g-038-r78-archived-name-dd/green-store-archived.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'green-store-last-create-agent.log': found at ['docs/reports/phase3g-038-r85-last-used-agent/green-store-last-create-agent.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'green-store-service-packages.log': found at ['docs/reports/phase3g-003-r77-name-reuse-reap/green-store-service-packages.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'green-tui-last-used-agent.log': found at ['docs/reports/phase3g-038-r85-last-used-agent/green-tui-last-used-agent.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'green.log': found at ['docs/reports/phase3e-302-r52-godog/green.log', 'docs/reports/phase3e-307-r53-orders/green.log', 'docs/reports/phase3g-031-sigterm-panic-interactive-cleanup/green.log', 'docs/reports/phase3g-201-rename-reuse-cleanup/green.log', 'docs/reports/phase3g-202-tombstone-drain/green.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'inject.go': found at ['internal/service/inject.go'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'interactive-scroll-run1.log': found at ['docs/reports/phase3g-703-r76-error-fallout-fix/interactive-scroll-run1.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'internal-tui-go-test.log': found at ['docs/reports/phase3g-107-dialog-degradation-net/internal-tui-go-test.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'internal-tui-tests.log': found at ['docs/reports/phase3g-035-previewfit-no-live-pane-latch/internal-tui-tests.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'main.go': found at ['cmd/deck/main.go'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'mutation-rectangular.log': found at ['docs/reports/phase3g-206-r93-visible-selection/mutation-rectangular.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'mutation-renderer-noop.log': found at ['docs/reports/phase3g-206-r93-visible-selection/mutation-renderer-noop.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'mutation-unrendered-note.log': found at ['docs/reports/phase3g-207-copy-confirmation/mutation-unrendered-note.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'negative-recorded-ratio-drift-fails.log': found at ['docs/reports/phase3g-106-contrast-floor/negative-recorded-ratio-drift-fails.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'negative-unlisted-pair-fails.log': found at ['docs/reports/phase3g-106-contrast-floor/negative-unlisted-pair-fails.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'no_leak_scan.log': found at ['docs/reports/phase3g-031-sigterm-panic-interactive-cleanup/no_leak_scan.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  DELIBERATE  'notes.md': class C -- phase3g.md:893 ('see `notes.md`') refers to this run's own /run/ralphd/notes.md, outside this repository, tracking the wider ~19-site title convergence as a still-open item. Same wording gap as tasks.json above, same disposition.
  RESOLVED    'original-assertion-red.log': found at ['docs/reports/phase3g-203-r82-assertion-conflict/original-assertion-red.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'overlays-and-parity-green.log': found at ['docs/reports/phase3g-038-r86-dialog-arrow-nav/overlays-and-parity-green.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'positive-control-red.log': found at ['docs/reports/phase3g-203-r82-assertion-conflict/positive-control-red.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'post-change-run1.log': found at ['docs/reports/phase3g-803-hook-truth-stopfailure/post-change-run1.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  DELIBERATE  'prds/phase3g-residuals-and-suite-determinism.md': class B -- phase3g-findings.md quotes this exact wrong filename to name an earlier draft's mistake; the correct file is prds/phase3f-residuals-and-suite-determinism.md, which exists. The string is deliberately non-resolving and the surrounding sentence says so.
  RESOLVED    'pre-change-red.log': found at ['docs/reports/phase3g-803-hook-truth-stopfailure/pre-change-red.log', 'docs/reports/phase3g-804-sort-order-error-route/pre-change-red.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'preview-floor-assertion-forced-red.log': found at ['docs/reports/phase3g-035-previewfit-no-live-pane-latch/preview-floor-assertion-forced-red.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'protected-path-all-refs.log': found at ['docs/reports/phase3g-113-closeout/protected-path-all-refs.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'protected-path-check.log': found at ['docs/reports/phase3d-212-closeout/protected-path-check.log', 'docs/reports/phase3e-328-closeout/protected-path-check.log', 'docs/reports/phase3e-410-closeout/protected-path-check.log', 'docs/reports/phase3f-027-closeout/protected-path-check.log', 'docs/reports/phase3f-038-closeout/protected-path-check.log', 'docs/reports/phase3g-113-closeout/protected-path-check.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'receiver.go:222': found at ['internal/hookrecv/receiver.go'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'red-2-spent-verdicts-refreeze.log': found at ['docs/reports/phase3g-001-r76-terminal-row-live-pane/red-2-spent-verdicts-refreeze.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'red-P-added-back.log': found at ['docs/reports/phase3g-015-footer-bindings-parity/red-P-added-back.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'red-before-fix-feature.log': found at ['docs/reports/phase3g-030-reclaim-leaked-interactive-pipe/red-before-fix-feature.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'red-comma-removed.log': found at ['docs/reports/phase3g-015-footer-bindings-parity/red-comma-removed.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'red-disabled-clear-loadbearing.log': found at ['docs/reports/phase3g-102-clear-recents-label/red-disabled-clear-loadbearing.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'red-feature-summary.log': found at ['docs/reports/phase3g-027-esc-clears-held-filter/red-feature-summary.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'red-feature-tail.log': found at ['docs/reports/phase3g-027-esc-clears-held-filter/red-feature-tail.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'red-mutation-full-package.log': found at ['docs/reports/phase3g-807-kill-eligibility/red-mutation-full-package.log', 'docs/reports/phase3g-808-footer-handler-agreement/red-mutation-full-package.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'red-mutation-inlined-copy.log': found at ['docs/reports/phase3g-806-archive-eligibility/red-mutation-inlined-copy.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'red-mutation-no-canReachPane-check.log': found at ['docs/reports/phase3g-808-footer-handler-agreement/red-mutation-no-canReachPane-check.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'red-mutation-no-canRestart-check.log': found at ['docs/reports/phase3g-808-footer-handler-agreement/red-mutation-no-canRestart-check.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'red-mutation-no-cankill-check.log': found at ['docs/reports/phase3g-807-kill-eligibility/red-mutation-no-cankill-check.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'red-mutation-no-check.log': found at ['docs/reports/phase3g-806-archive-eligibility/red-mutation-no-check.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'red-mutation.log': found at ['docs/reports/phase3g-806-archive-eligibility/red-mutation.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'red-pre-fix.log': found at ['docs/reports/phase3g-701-r76-bare-error/red-pre-fix.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'red-worktree-trial.log': found at ['docs/reports/phase3g-805-stale-tmux-verdict/red-worktree-trial.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'run1.log': found at ['docs/reports/phase3g-703-r76-error-fallout-fix/run1.log', 'docs/reports/phase3g-804-sort-order-error-route/run1.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'run3.log': found at ['docs/reports/phase3g-703-r76-error-fallout-fix/run3.log', 'docs/reports/phase3g-804-sort-order-error-route/run3.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'run5.log': found at ['docs/reports/phase3g-804-sort-order-error-route/run5.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'settings-go-untouched.log': found at ['docs/reports/phase3g-108-r86-proof/settings-go-untouched.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'targeted-suite-tui.log': found at ['docs/reports/phase3g-207-copy-confirmation/targeted-suite-tui.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'task010-bounded-batch-mutation-red.log': found at ['docs/reports/phase3g-010-011-tombstone-sweep-evidence/task010-bounded-batch-mutation-red.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'task010-frozen-clock-mutation-red.log': found at ['docs/reports/phase3g-010-011-tombstone-sweep-evidence/task010-frozen-clock-mutation-red.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'task011-callsite-removed-mutation-red.log': found at ['docs/reports/phase3g-010-011-tombstone-sweep-evidence/task011-callsite-removed-mutation-red.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  RESOLVED    'task011-green-store-service-cmd-deck.log': found at ['docs/reports/phase3g-010-011-tombstone-sweep-evidence/task011-green-store-service-cmd-deck.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)
  DELIBERATE  'tasks.json': class C -- every occurrence (phase3g-findings.md, task 026's own unsatisfiableReason) refers to this run's /run/ralphd/tasks.json, outside this repository. Bare, not prefixed with /run/ralphd/ and not carrying the explicit 'outside this repository' label at the citing sentence itself -- a pre-existing wording gap in text task 606 does not own or edit, disclosed here rather than silently passed.
  RESOLVED    'theme-suite-green.log': found at ['docs/reports/phase3g-106-contrast-floor/theme-suite-green.log'] (class D/E: bare abbreviation of a file the surrounding prose names in full a few words earlier)

STILL UNRESOLVED AFTER MANUAL DISPOSITION (real defects): ['/run/ralphd/approaches/NN/tasks.json', '9/10']
exit=0
```

That block is the round-2 run's complete output, verbatim and unelided —
byte-identical to `round2-guard-g-citation-sweep.log` in this directory
(`diff <(python3 docs/reports/phase3g-813-guards/citation_sweep.py) \
docs/reports/phase3g-813-guards/round2-guard-g-citation-sweep.log` reproduces
it, modulo that log's own leading command line and trailing `exit=0`). Round
1's own run, at `b1dbfc4`, is in `guard-g-citation-sweep.log`; the two agree
line for line, since the sweep reads only `docs/reports/phase3g.md` and
`docs/reports/phase3g-findings.md`, neither of which this task touches. Round
1's README elided the middle of this output with an editorial `[...]`; that
elision is what round 2 removes.

**The two items left "unresolved after manual disposition" are not
defects**, and match exactly what task 606's own report already recorded as
this script's permanent, known-benign residue (see the standing rules'
citation of this fact and task 810's notes): `/run/ralphd/approaches/NN/
tasks.json` is a literal placeholder path quoted in prose (the `NN` is not a
real path segment — no directory named `NN` exists), and `9/10` is a bare
ratio the sweep's path-shaped-string heuristic (`looks_like_path`, matching
on `/`) mistakes for a path; neither is a citation of a real path that
should resolve. Both predate this task and are unchanged by it.

**Known self-citation false-positive class, disclosed** (class A, verbatim
from `citation_sweep.py`'s own docstring — the authoritative, re-runnable
source): *"the two reports citing each other and citing their own filenames
in prose describing the sweep itself (`phase3g.md`/`phase3g-findings.md`
referencing each other, or a report naming its own citation-checker
command) — a sweep that greps a report's own prose flags the report
describing itself."* The other disclosed classes (B deliberate-wrong-path
callout, C `/run/ralphd/...` bare references, D bare abbreviation/glob/
extension-as-label, E a report's own log filename abbreviated once its
subdirectory is named nearby, F the `001-202` operator-ruling identifier)
are unchanged from task 606's own disclosure and are not restated in full
here; the full docstring is in `citation_sweep.py` itself, in this
directory.

Every R76–R93 section's cited test/scenario files remain backtick-quoted
repository paths already covered by class 3 of this same sweep; none of
them appear in the unresolved list at any stage.

## Reproducing this bundle

```
cd <repo root>
python3 docs/reports/phase3g-813-guards/citation_sweep.py           # (g)
git log --all --oneline 1cfbd5a.. -- SPEC.md prds/ ci/Dockerfile ci/SPIKE.md   # (a)
git status --porcelain                                              # (b)
git rev-parse HEAD; git log --oneline -1 origin/main                # (c)
git diff --stat 17b1649..HEAD -- '*.go' '*.feature' '*.sh' '*.toml' go.mod go.sum   # (d)
git diff 1cfbd5a..HEAD -- features/godog_test.go                    # (e)
git diff 1cfbd5a..HEAD -- '*.go' '*.feature' '*.sh' '*.toml' | grep -n '^+.*t\.Skip('   # (f)
```

## Addendum (this bundle's own final sha)

This bundle's own commit (the one adding every file above) is **`8eaf474`**
(`8eaf47436f079b1c9080fee95d406808788ff4d7`). Re-run in this follow-up
addendum commit, at that exact sha, before this addendum's own change is
staged (full transcript in `addendum-final-sha-check.log`):

```
$ git status --porcelain
exit=0

$ git rev-parse HEAD
8eaf47436f079b1c9080fee95d406808788ff4d7
exit=0

$ git log --oneline -1 origin/main
8eaf474 docs: re-verify this run's guards at HEAD and publish the bundle (task 813)
exit=0
```

Clean tree and `HEAD == origin/main`, both confirmed at `8eaf474` — this
bundle's own final sha — closing the one thing a commit cannot say about
itself directly.

## Addendum, round 2 (the correction commit's own final sha)

Round 1's addendum above is left exactly as it stood; it is true of the sha it
names. The round-2 correction that fixes the three defects listed at the top of
this report is its own commit, **`ddc0936`**
(`ddc09362c6116d2d3112da300b89663c52b447d0`), and a commit still cannot quote
its own sha — so the pair is re-shown here, at `ddc0936`, captured after that
commit was pushed and before this addendum's own file changes existed in the
worktree (full transcript in `addendum-round2-final-sha-check.log`):

```
$ git status --porcelain
exit=0

$ git rev-parse HEAD
ddc09362c6116d2d3112da300b89663c52b447d0
exit=0

$ git log --oneline -1 origin/main
ddc0936 docs: publish the guard bundle's outputs unedited and record why the godog diff guard cannot be empty (task 813)
exit=0
```

Clean tree, and local `HEAD` equal to `origin/main`, both at `ddc0936`. The
bundle's final sha is this addendum's own commit, one further on; the same
`git status --porcelain` / `git rev-parse HEAD` / `git log --oneline -1
origin/main` triple re-run at any later point reproduces the same agreement,
which is why this report states the property and its command rather than
chasing a sha it cannot contain.
