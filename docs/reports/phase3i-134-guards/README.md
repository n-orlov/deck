# Phase 3i task 134 — re-verify both guards at the true final sha

This report re-runs every guard the standing rules require, measured at the tree this
task's own commit is built on top of (`HEAD` immediately before this commit lands,
`2c5c098`, task 133's "GH issue #19 design section map" commit — docs-only, so it does
not move the "final code sha" below). Because committing this very README moves `HEAD`
past the commit it was measured against, an immediately-following one-line addendum
commit (same `(task 134)` tag) re-publishes the identical `HEAD`/`origin/main` triple at
*that* commit's own sha, the way task 015 did in Phase 3h.

## `git status --porcelain`

```
$ git status --porcelain
(empty)
```

The worktree is clean before this report's own commit is made.

## `git rev-parse HEAD origin/main`

```
$ git rev-parse HEAD origin/main
2c5c09835ffe7481915da21c556b037bdd3c1cd7
2c5c09835ffe7481915da21c556b037bdd3c1cd7
```

Both print the same sha: `HEAD` and `origin/main` agree — the push guard ("a task is
not done until `git rev-parse HEAD` == `git rev-parse origin/main`") holds at the
moment this measurement was taken.

## Protected-path guard — the standing rule's literal range `6197b53..HEAD`

```
$ git log --oneline 6197b53..HEAD -- SPEC.md prds ci/Dockerfile ci/SPIKE.md
3090b68 prds: cut Phase 3i — force-attach, stealing the interactive preview (operator)
```

This is **not** empty, and that is expected, not a regression this task introduces:
[`docs/reports/phase3i-findings.md`, §2](../phase3i-findings.md#2-the-protected-path-audit-range-6197b53head-necessarily-contains-one-operator-commit)
already establishes that `3090b68` is the **operator's own commit** — it lands three
minutes after `6197b53` (the operator's own SPEC change) and adds
`prds/phase3i-force-attach.md`, the PRD file this entire phase's plan was cut from — and
is therefore not a worker write. `6197b53` predates every worker task in this phase, but
it does not postdate the operator's *own* two protected-path commits, so the literal
range as written in the standing rules can never be empty for this phase; this is the
same shape of pre-existing disagreement task 015 documented for Phase 3h's PRD range in
that phase's own findings report.

The guard's actual intent — no *worker* write to a protected path — is checked directly
against the range opened by the last protected-path commit, `3090b68` itself:

```
$ git log --oneline 3090b68..HEAD -- SPEC.md prds ci/Dockerfile ci/SPIKE.md
(empty)
$ git diff --stat 3090b68..HEAD -- SPEC.md prds ci/Dockerfile ci/SPIKE.md
(empty)
```

Both empty: no commit any worker iteration of this run has written, from `3090b68`'s
child through the tip measured above, touches any of the four protected paths.

## Every sha cited in `docs/reports/phase3i.md` and `docs/reports/phase3i-findings.md` resolves

Every `` `<sha>` `` token quoted in either document (short or full form), checked with
`git cat-file -e <sha>^{commit}`:

```
$ for s in 0c022e8 3090b68 3508c5a 6197b53 a24ff8d b9243a1 \
    b9243a1f415ba9ca77cc2ffa2ec557ca8b1be4cb fcdb994 0177991 17e185c 1b71c74 \
    244f66f 4a9d745 4ac5970 4ef6d28 55c0818 5fb9e4f 625c663 704fd1a 7371c07 \
    7e141ad 83e92d5 8d0437f 8d1b41a 8e5e036 a063296 a65190e a790061 be7a387 \
    c525b35 c95e848 cb27c5c cbc6692 d7f149c e31c37e e398881 e6157d8 eb1e917; do
    git cat-file -e "${s}^{commit}" && echo "$s ok"
  done
0c022e8 ok
3090b68 ok
3508c5a ok
6197b53 ok
a24ff8d ok
b9243a1 ok
b9243a1f415ba9ca77cc2ffa2ec557ca8b1be4cb ok
fcdb994 ok
0177991 ok
17e185c ok
1b71c74 ok
244f66f ok
4a9d745 ok
4ac5970 ok
4ef6d28 ok
55c0818 ok
5fb9e4f ok
625c663 ok
704fd1a ok
7371c07 ok
7e141ad ok
83e92d5 ok
8d0437f ok
8d1b41a ok
8e5e036 ok
a063296 ok
a65190e ok
a790061 ok
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
```

All 38 distinct shas cited across both documents resolve to a real commit object.

Two backtick-quoted tokens in `phase3i-findings.md` were deliberately excluded from
this list because they are not sha citations by this report's own hand: `ownership.go`
(bare, no directory) and `tasks.json` both appear only inside a **verbatim quotation**
of task 119's own `validationNotes` text (§1, "What failed, quoted verbatim") — the
findings report is quoting what an earlier validation attempt wrote, not asserting
either token as a path in its own voice. The real path that quotation is about,
`internal/tmux/ownership.go`, is already covered in the path list below.

## Every path cited in `docs/reports/phase3i.md` and `docs/reports/phase3i-findings.md` is tracked

Every backtick-quoted path in either document, including markdown-link targets resolved
relative to `docs/reports/` (both documents' own directory), checked with
`git ls-files --error-unmatch <path>`:

```
$ for p in SPEC.md docs/DELIVERY-LOG.md \
    docs/reports/phase3i-127-fullsuite/README.md \
    docs/reports/phase3i-128-fullsuite-verbose/README.md \
    docs/reports/phase3i-129-stability10/README.md \
    docs/reports/phase3i-findings.md docs/reports/phase3i.md \
    docs/reports/phase3h-findings.md \
    features/color_depth_test.go features/filter.feature \
    features/golden_frame_test.go features/interactive_force_attach.feature \
    features/sort_order.feature internal/tmux/force_ownership_test.go \
    internal/tmux/isize_geometry.go internal/tmux/isize_geometry_test.go \
    internal/tmux/ownership.go internal/tmux/reclaim.go \
    internal/tmux/reclaim_test.go internal/tui/displacement_teardown_test.go \
    internal/tui/double_steal_restore_test.go \
    internal/tui/failed_entry_unwind_test.go \
    internal/tui/footer_handler_agreement_test.go \
    internal/tui/force_enter_test.go internal/tui/force_indistinguishable_test.go \
    internal/tui/help_keymap_parity_test.go internal/tui/interactive.go \
    internal/tui/interactive_displacement.go \
    internal/tui/interactive_displacement_test.go \
    internal/tui/interactive_fallout_store_test.go \
    internal/tui/isize_geometry_entry_test.go internal/tui/lost_attach.go \
    internal/tui/lost_attach_swallow_test.go internal/tui/lost_attach_test.go \
    internal/tui/preview_fit_foreign_claim_test.go \
    internal/tui/refusal_f_naming_test.go \
    internal/tui/teardown_still_mine_gate_test.go \
    internal/tui/teardown_transport_gate_test.go internal/tui/tui.go \
    prds/phase3i-force-attach.md; do
    git ls-files --error-unmatch "$p" >/dev/null && echo "$p ok"
  done
```

All 40 paths print `ok` (verified directly; the full per-path listing is omitted here
only for length — every one of the 40 commands above exits 0 and echoes `<path> ok`,
confirmed by counting `wc -l` against the 40-line input list before this section was
written).

`prds/phase3i-force-attach.md` is a protected path — it is cited (read) by both
documents as the source PRD, never edited by this or any other worker task, and
tracking it is not itself a protected-path write.

## Final code sha

```
$ git log -1 --format=%H -- '*.go' '*.feature'
b9243a1f415ba9ca77cc2ffa2ec557ca8b1be4cb
```

Unchanged from every measurement since task 126: this task's own commit touches only
`docs/reports/phase3i-134-guards/README.md`, no `*.go` or `*.feature` file.

## Addendum — re-published at this guard commit's own sha

This README's own commit is `docs: re-verify both guards at the true final sha
(task 134)`. Re-running the `HEAD`/`origin/main` triple immediately after that commit
was pushed, at that commit's own sha, is recorded in a one-line follow-up commit tagged
`(task 134)` appended below this line — the follow-up names its own sha explicitly so
the pair is citable without a commit having to quote itself.

<!-- addendum appended by a follow-up commit; do not remove this marker -->
