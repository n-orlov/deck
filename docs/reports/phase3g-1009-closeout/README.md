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

`postpush-guard.log` — after commit 1's push: tree clean (this directory is
now tracked, not `??`), `HEAD == origin/main` at commit 1's own sha (named in
that log, not here, since this README is part of commit 1's own tree and
cannot state commit 1's hash without invalidating it — see
`docs/reports/phase3g.md`'s "## Close-out (approach 08)" section for the same
regress spelled out in full), `git ls-remote origin refs/heads/main` agrees,
and the code diff against `a5f8f6b` is still empty.

## Verification

```
$ git ls-files docs/reports/phase3g-1009-closeout/
docs/reports/phase3g-1009-closeout/README.md
docs/reports/phase3g-1009-closeout/postpush-guard.log
docs/reports/phase3g-1009-closeout/pre-commit-guard.log
```

`python3 docs/reports/phase3g-813-guards/citation_sweep.py` exits 0 after both
commits, with the run's two pre-existing, disclosed false positives
(`/run/ralphd/approaches/NN/tasks.json`, `9/10`) as the only unresolved
strings.
