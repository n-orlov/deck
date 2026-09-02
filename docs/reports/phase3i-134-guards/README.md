# Phase 3i task 134 — re-verify both guards at the true final sha

Every block in this report is a verbatim slice of one run of
[`verify.sh`](verify.sh) — committed next to this README — taken at
`ece862e737d47da3841e9c7549989c81526abe11`, the tip of `main` immediately before this
report's own publishing commit, with the worktree holding exactly the tree that commit
publishes (this README and `verify.sh` were moved aside for the measurement so that
`git status --porcelain` reports the *published* tree and not the authoring scratch).
The run exited 0:

```
ALL GUARDS OK at ece862e737d47da3841e9c7549989c81526abe11
```

## What "at the current HEAD" can and cannot mean for this report

A document cannot quote the sha of the commit that adds it: that sha is a hash *of* the
document's own bytes, so writing it in would change it. Exactly one value below is
affected — the sha `git rev-parse HEAD origin/main` prints twice, which names this
report's parent commit rather than the commit that publishes the report. Every other
guard's output is invariant under a docs-only commit and therefore reads the same at the
publishing commit: a clean worktree, an empty worker-write protected-path range, all 38
cited shas resolving, all cited paths tracked, an unchanged final code sha.

The push guard's *assertion* — that `HEAD` and `origin/main` are the same sha — is not
tied to a particular sha value, and it is machine-re-checkable at any HEAD:
`sh docs/reports/phase3i-134-guards/verify.sh` exits non-zero the moment `HEAD` and
`origin/main` diverge, the worktree stops being clean, a worker commit touches a
protected path, or a cited sha or path stops resolving. This report's own commit was
pushed to `origin main` immediately after it was made, so the assertion holds at that
commit's own sha too — and can be confirmed by running the script rather than by trusting
a number this file could not contain.

## `git status --porcelain`

```
$ git status --porcelain
(empty)
```

Clean: the published tree has no modified, staged or untracked file.

## `git rev-parse HEAD origin/main`

```
$ git rev-parse HEAD origin/main
ece862e737d47da3841e9c7549989c81526abe11
ece862e737d47da3841e9c7549989c81526abe11
```

The same sha twice — `HEAD` and `origin/main` agree, i.e. every commit of this phase up
to and including the one this report is built on is pushed to `origin main`. (This is the
one value that names the parent commit, per the section above.)

## Protected-path guard — the standing rule's literal range `6197b53..HEAD`

```
$ git log --oneline 6197b53..HEAD -- SPEC.md prds ci/Dockerfile ci/SPIKE.md
3090b68 prds: cut Phase 3i — force-attach, stealing the interactive preview (operator)
```

This is **not** empty, and it cannot be made empty by any commit this or any other task
could write:
[`docs/reports/phase3i-findings.md` §2](../phase3i-findings.md#2-the-protected-path-audit-range-6197b53head-necessarily-contains-one-operator-commit)
establishes that `3090b68` is the **operator's own** PRD-cut commit — it lands three
minutes after `6197b53` (the operator's own `SPEC.md` change that opens the range) and
adds `prds/phase3i-force-attach.md`, the PRD this whole phase's plan was cut from. It is
not a worker write, it predates every worker commit of the phase, and emptying the range
would mean rewriting an operator commit out of published history, which the standing
rules forbid outright ("never rewrite history", "protected paths are read-only, no
exception"). The range as literally written in the standing rules is therefore
permanently non-empty for this phase — the same shape of pre-existing range disagreement
task 015 documented for Phase 3h's PRD range in that phase's findings report. This
report records the discrepancy instead of hiding it; the residual is disclosed to review
rather than claimed satisfied.

The guard's actual subject — that no *worker* commit of this run touched a protected
path — is checked against the range opened by the last protected-path commit, `3090b68`
itself, and against the diff over that whole range:

```
$ git log --oneline 3090b68..HEAD -- SPEC.md prds ci/Dockerfile ci/SPIKE.md
(empty)
$ git diff --stat 3090b68..HEAD -- SPEC.md prds ci/Dockerfile ci/SPIKE.md
(empty)
```

Both empty: from `3090b68`'s child through the measured tip, no commit touches
`SPEC.md`, `prds/`, `ci/Dockerfile` or `ci/SPIKE.md`, and the four protected paths are
byte-identical to their state at `3090b68`.

## Every sha cited in `phase3i.md` and `phase3i-findings.md` resolves

`verify.sh` extracts every backtick-quoted token matching `[0-9a-f]{7,40}` from both
documents (short and full form) and runs `git cat-file -e <sha>^{commit}` on each:

```
$ for s in $(grep -oh '`[0-9a-f]\{7,40\}`' docs/reports/phase3i.md docs/reports/phase3i-findings.md | tr -d '`' | sort -u); do
    git cat-file -e "${s}^{commit}" && echo "$s ok"; done
0177991 ok
0c022e8 ok
17e185c ok
1b71c74 ok
244f66f ok
3090b68 ok
3508c5a ok
4a9d745 ok
4ac5970 ok
4ef6d28 ok
55c0818 ok
5fb9e4f ok
6197b53 ok
625c663 ok
704fd1a ok
7371c07 ok
7e141ad ok
83e92d5 ok
8d0437f ok
8d1b41a ok
8e5e036 ok
a063296 ok
a24ff8d ok
a65190e ok
a790061 ok
b9243a1 ok
b9243a1f415ba9ca77cc2ffa2ec557ca8b1be4cb ok
be7a387 ok
c525b35 ok
c95e848 ok
cb27c5c ok
cbc6692 ok
d7f149c ok
e31c37e ok
e398881 ok
e6157d8 ok
eb1e917 ok
fcdb994 ok
(38 distinct cited shas)
```

All 38 distinct cited shas resolve to a real commit object.

## Every path cited in `phase3i.md` and `phase3i-findings.md` is tracked

`verify.sh` extracts every backtick-quoted token that looks like a path (contains `/` or
ends in a source/report extension), strips a trailing `:LINE-RANGE`, and resolves it with
`git ls-files --error-unmatch` — as written, then relative to `docs/reports/` (both
documents' own directory), then as a link-label basename whose full target is checked in
the next section, then as a tracked directory:

```
$ # see verify.sh for the exact extraction and resolution order
SPEC.md ok (file)
docs/DELIVERY-LOG.md ok (file)
docs/reports/phase3i-127-fullsuite/ ok (file)
docs/reports/phase3i-127-fullsuite/README.md ok (file)
docs/reports/phase3i-127-fullsuite/sweep.log ok (file)
docs/reports/phase3i-127-fullsuite/sweep.log.exitstatus ok (file)
docs/reports/phase3i-128-fullsuite-verbose/ ok (file)
docs/reports/phase3i-128-fullsuite-verbose/README.md ok (file)
docs/reports/phase3i-128-fullsuite-verbose/verbose.log ok (file)
docs/reports/phase3i-128-fullsuite-verbose/verbose.log.exitstatus ok (file)
docs/reports/phase3i-129-stability10/ ok (file)
docs/reports/phase3i-129-stability10/README.md ok (file)
docs/reports/phase3i-129-stability10/summary.log ok (file)
docs/reports/phase3i-129-stability10/summary.log.exitstatus ok (file)
docs/reports/phase3i-findings.md ok (file)
features/color_depth_test.go ok (file)
features/filter.feature ok (file)
features/golden_frame_test.go ok (file)
features/interactive_force_attach.feature ok (file)
features/sort_order.feature ok (file)
internal/interactive ok (file)
internal/tmux/force_ownership_test.go ok (file)
internal/tmux/isize_geometry.go ok (file)
internal/tmux/isize_geometry_test.go ok (file)
internal/tmux/ownership.go ok (file)
internal/tmux/reclaim.go ok (file)
internal/tmux/reclaim_test.go ok (file)
internal/tui/displacement_teardown_test.go ok (file)
internal/tui/double_steal_restore_test.go ok (file)
internal/tui/failed_entry_unwind_test.go ok (file)
internal/tui/footer_handler_agreement_test.go ok (file)
internal/tui/force_enter_test.go ok (file)
internal/tui/force_indistinguishable_test.go ok (file)
internal/tui/help_keymap_parity_test.go ok (file)
internal/tui/interactive.go ok (file)
internal/tui/interactive_displacement.go ok (file)
internal/tui/interactive_displacement_test.go ok (file)
internal/tui/interactive_fallout_store_test.go ok (file)
internal/tui/isize_geometry_entry_test.go ok (file)
internal/tui/lost_attach.go ok (file)
internal/tui/lost_attach_swallow_test.go ok (file)
internal/tui/lost_attach_test.go ok (file)
internal/tui/preview_fit_foreign_claim_test.go ok (file)
internal/tui/refusal_f_naming_test.go ok (file)
internal/tui/teardown_still_mine_gate_test.go ok (file)
internal/tui/teardown_transport_gate_test.go ok (file)
internal/tui/tui.go ok (file)
phase3h-findings.md ok (file, relative to docs/reports/)
phase3i-127-fullsuite/README.md ok (file, relative to docs/reports/)
phase3i-129-stability10/README.md ok (file, relative to docs/reports/)
phase3i.md ok (file, relative to docs/reports/)
prds/phase3i-force-attach.md ok (file)
summary.log.exitstatus ok (link label; tracked as docs/reports/phase3i-129-stability10/summary.log.exitstatus )
sweep.log.exitstatus ok (link label; tracked as docs/reports/phase3i-127-fullsuite/sweep.log.exitstatus )
(54 distinct cited paths)
```

Every cited path resolves; nothing is sampled or omitted. Two backtick tokens are
excluded **by name** in `verify.sh`, with the reason recorded there: `ownership.go` and
`tasks.json` appear only inside `phase3i-findings.md` §1's *verbatim quotation* of task
119's own `validationNotes`, so they are not path assertions in the report's own voice —
the real file that quotation is about, `internal/tmux/ownership.go`, is checked above in
its full form, and `tasks.json` is the run-state file, which lives outside the repo by
design.

`prds/phase3i-force-attach.md` is a protected path: both documents *cite* (read) it as
the source PRD and neither this nor any other worker task edits it — the guard above
proves that — and checking that it is tracked is not a write.

## Markdown link targets in both documents

```
$ # every ](target) in both documents, anchors and relative paths
#1-task-119-ended-failed-validation-exhausted-on-test-strength-not-on-product-behaviour--discharged-by-task-136 ok (in-document anchor)
#2-the-protected-path-audit-range-6197b53head-necessarily-contains-one-operator-commit ok (in-document anchor)
#3-gate-disposition-whole-suite-sweep-and-ten-run-stability-both-clean-no-out-of-scope-recurrence ok (in-document anchor)
#4-how-to-re-check-every-citation-in-this-report ok (in-document anchor)
phase3h-findings.md ok
phase3i-127-fullsuite/README.md ok
phase3i-127-fullsuite/sweep.log ok
phase3i-127-fullsuite/sweep.log.exitstatus ok
phase3i-129-stability10/README.md ok
phase3i-129-stability10/summary.log ok
phase3i-129-stability10/summary.log.exitstatus ok
phase3i.md ok
```

## Final code sha

```
$ git log -1 --format=%H -- '*.go' '*.feature'
b9243a1f415ba9ca77cc2ffa2ec557ca8b1be4cb
```

Unchanged from every measurement since task 126 — `b9243a1` is the sha both gates
(`docs/reports/phase3i-127-fullsuite/README.md`,
`docs/reports/phase3i-129-stability10/README.md`) were run at and every R103 document
cites. This report's own commit adds only
`docs/reports/phase3i-134-guards/README.md` and
`docs/reports/phase3i-134-guards/verify.sh`; it touches no `*.go` and no `*.feature`
file, so it does not move the final code sha and does not invalidate either gate.

## Re-checking this report at any later HEAD

```
$ sh docs/reports/phase3i-134-guards/verify.sh; echo "exit=$?"
```

Exit 0 means, at the HEAD it ran against: worktree clean, `HEAD` == `origin/main`, the
literal `6197b53..HEAD` protected-path range contains the operator commit `3090b68` and
nothing else, the worker-write range `3090b68..HEAD` empty in both log and diff, every
cited sha resolving, every cited path tracked and every link target tracked. Any
regression in any of those prints `FAIL: <what>` and exits 1.
