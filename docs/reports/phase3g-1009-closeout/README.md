# Task 1009 — approach 10 close-out

This directory carries the guard evidence for `docs/reports/phase3g.md`'s
"## Close-out (approach 10)" section. It follows the two-commit shape task
1007 already used and named as the lesson for this task: a check run *after*
a commit is pushed cannot live inside a file in that same commit's tree, so
the post-push triple (clean tree, `HEAD == origin/main`, empty code diff
since the final code sha) is published as raw output by a follow-up commit
that quotes the first commit's sha — never by an addendum chasing its own.

## Two commits, not a self-naming spiral

- **Commit 1** (this task's primary commit) lands the close-out section in
  `docs/reports/phase3g.md` plus this README and
  [`pre-commit-guard.log`](pre-commit-guard.log), captured **before** commit 1
  existed, at HEAD `a7c8d76` (== `origin/main` at the time): clean tree,
  `HEAD == origin/main`, and `git diff --stat a5f8f6b..HEAD -- '*.go'
  '*.feature' '*.sh' '*.toml' go.mod go.sum` empty.
- **Commit 2** (the follow-up, fix-forward per the standing rules) adds
  [`postpush-guard.log`](postpush-guard.log): the same three checks, run from
  `/workspace` **after commit 1 was pushed**, quoting commit 1's own sha —
  never commit 2's. That is the "taken after this task's own push" evidence
  the close-out section itself points at, so no third, self-naming addendum
  commit is needed to make the section's guard claim true.

## What the two logs show

`pre-commit-guard.log` — before commit 1: tree already clean (no untracked
report files existed yet at that point — this directory was created but held
no tracked file), `HEAD == origin/main == a7c8d76` (task 1008's own final
sha), code diff against `a5f8f6b` empty.

`postpush-guard.log` — after commit 1's push: **round 1's own defect**, found
by validation: `git status --porcelain` was run from inside `/workspace`
*after* `postpush-guard.log` itself had already been written to that
working tree but *before* it was committed, so its own output reads `??
docs/reports/phase3g-1009-closeout/postpush-guard.log` — the capture named
itself as the untracked file it was about to become, the same self-naming
hazard this README's commit-1 section warns about for a sha, just one step
earlier in the pipeline (the file, not the sha). `HEAD == origin/main` and
the empty code diff in that same log are still accurate; only the
`git status --porcelain` line is wrong. The log is kept, unedited, as the
record of that defect — not deleted, not silently fixed in place.

`postpush-guard-2.log` — round 2 (task 1010), the corrected capture: the same
three checks were run and redirected to a file **outside the workspace**
(`/tmp`, never committed) before `postpush-guard-2.log` was created inside
`docs/reports/phase3g-1009-closeout/` at all, so nothing the capture measures
can be the file that reports it. That capture showed `git status --porcelain`
genuinely blank, `HEAD == origin/main` at commit 1's own sha, and the code
diff against `a5f8f6b` still empty; its unedited content was then copied
verbatim into this tracked file. This is the capture the close-out section's
guard claim now rests on.

## Verification

```
$ git ls-files docs/reports/phase3g-1009-closeout/
docs/reports/phase3g-1009-closeout/README.md
docs/reports/phase3g-1009-closeout/postpush-guard-2.log
docs/reports/phase3g-1009-closeout/postpush-guard.log
docs/reports/phase3g-1009-closeout/pre-commit-guard.log
```

`python3 docs/reports/phase3g-813-guards/citation_sweep.py` exits 0 after every
commit in this directory's history, with the run's two pre-existing,
disclosed false positives (`/run/ralphd/approaches/NN/tasks.json`, `9/10`) as
the only unresolved strings.
