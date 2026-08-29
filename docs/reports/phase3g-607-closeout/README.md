# Phase 3g task 607 — the close-out section's own evidence bundle

Task 607 wrote [`docs/reports/phase3g.md`](../phase3g.md)'s `## Close-out (approach 06)`
section (anchor [`#close-out-approach-06`](../phase3g.md#close-out-approach-06), linked from
that file's own section list). This directory is that section's tracked evidence: the checker
that verifies its independently-checkable clauses, the checker's captured output, and the
restated empty code-pattern diff back to the phase's final code sha `b0a4e7d`.

It exists because the close-out's exceptional-status table (clause (f)) must name, for each
listed task, both the sha(s) **and** a tracked evidence directory that delivered its scope —
including the three rows (approach-03 task 213, approach-04 task 309, approach-05 task 511)
whose close-out scope task 607 itself delivered. Those rows point here.

## Commits

| commit | what it landed |
|---|---|
| `3faafa7` | the close-out section itself — clauses (a)–(g), and the section-list link |
| `8545366` | the (a) addendum naming `3faafa7` (a commit cannot quote its own sha) |
| `628309b` | clause (f)'s evidence-and-residual correction: every delivered row now names a tracked evidence directory as well as its sha(s), the three task-607 rows point at this directory, and the delivery-log half that is genuinely undelivered points at open residual finding F35 instead of at a pending task |

The correction was made because the first validation pass of task 607 found clause (f)
under-cited in exactly nine rows: 213, 309 and 511 named no sha and no evidence path at all
(they said "delivered by this task"), 309 and 511 deferred their delivery-log half to a
*pending task* rather than to a findings row, and 209, 211, 304, 307, 505 and 509 named shas
without an evidence directory. Finding
[F35](../phase3g-findings.md#3-defects-found-and-deliberately-not-fixed-and-why) was filed in
the same commit to carry the one genuinely undelivered piece — `docs/DELIVERY-LOG.md`'s Phase
3g paragraph, still written at approach 02's state — as an open residual rather than as a
forward promise.

## Checker

[`verify.py`](verify.py) runs from the repository root and needs nothing but `git` and
Python 3:

```
$ python3 docs/reports/phase3g-607-closeout/verify.py
```

It checks five things, and exits non-zero if any of them fails:

1. the section exists, its GitHub-style heading slug equals `close-out-approach-06`, and at
   least one line above the section links to `(#close-out-approach-06)` — the file's own
   section list;
2. every sha the section quotes in backticks resolves under `git cat-file -e <sha>^{commit}`;
3. every markdown link target in the section resolves on disk relative to `docs/reports/`
   (in-document `#anchor` links are checked against the file's own headings instead);
4. every row of the (f) exceptional-status table (matched by its `0NN`/`1NN`/`2NN`/`3NN`/`5NN`
   task id in the first cell) names at least one sha **and** at least one tracked path link or
   an `F<NN>` findings row — i.e. no row claims delivery bare;
5. `git diff --stat b0a4e7d..HEAD -- '*.go' '*.feature' '*.sh' '*.toml' go.mod go.sum` is
   empty, so the section is still describing the same code tree the gates were measured on.

## Captured output

[`verify.log`](verify.log) — the checker's own output, re-captured at the addendum commit that
names `628309b` in the section's (a) block, with
[`verify.exitstatus`](verify.exitstatus) (`0`) captured in the same shell call:

```
OK   1a: '## Close-out (approach 06)' at line 1241, 145 lines
OK   1b: heading slug == anchor #close-out-approach-06
OK   1c: linked from the file's own section list at line(s) 47
OK    2: 46 quoted shas checked, 0 unresolved
OK    3: 62 link targets checked, 0 unresolved
OK    4: 29 (f) rows checked, 0 weak
OK   5: git diff --stat b0a4e7d..HEAD over *.go *.feature *.sh *.toml go.mod go.sum is empty (exit 0)

ALL CHECKS PASSED
```

The 29 rows of check 4 are the complete set of approach-01–05 tasks that ended in a
non-`completed` status, read from the archived per-approach task state at
`/run/ralphd/approaches/NN/tasks.json` — outside this repository, not part of the tracked
record, which is why the close-out quotes those statuses into the report rather than linking
them. The archived list itself was confirmed exhaustive by task 607's first validation pass.

[`code-diff-empty.log`](code-diff-empty.log) — the same empty code-pattern diff, captured
with its own `exit=0` in the same shell call:

```
$ git diff --stat b0a4e7d..HEAD -- '*.go' '*.feature' '*.sh' '*.toml' go.mod go.sum
exit=0
```

## What this bundle does not claim

- It does not re-measure either gate. The 10/10 stability rate is task 507's round-3
  measurement, cited (never re-rolled) by [`phase3g-605-stability-gate/`](../phase3g-605-stability-gate/README.md);
  the whole-suite sweep is [`phase3g-604-fullsuite/`](../phase3g-604-fullsuite/README.md),
  captured exit status `0`.
- It does not close finding F35: `docs/DELIVERY-LOG.md`'s Phase 3g paragraph is still at
  approach 02's state as of this commit, and that is task 608's declared remit.
- The named flakes stay named, never claimed fixed: F2, F22, F31's lost update, and the
  `status_recovery` dup-pane/R76 interaction.
