# Task 815 — approach 08 close-out evidence

This directory backs the `## Close-out (approach 08)` section of
[`../phase3g.md`](../phase3g.md). Because a commit cannot quote its own sha, this
task lands in two commits: the primary commit (which adds this file, the guard
log below, and the close-out section) and a small follow-up addendum commit
that names the primary commit's own sha and re-runs the same two guard checks
at it. Both are named inside the close-out section itself; this file only
carries the raw logs.

## `pre-commit-guard.log`

Captured **before** the primary commit exists, at the then-current HEAD
`9e6d525` (task 814's commit, `== origin/main` at that point): `git status
--porcelain` (shows only this new untracked directory — the same honest
"not yet clean" shape task 813's round 1 disclosed rather than hid),
`git rev-parse HEAD`, and `git log --oneline -1 origin/main`. This is the
starting point for the close-out's own final-sha claim, not the claim itself
— the claim is re-shown, genuinely clean, in the addendum's own log (added by
the follow-up commit, once the primary commit has landed and this directory
is no longer untracked).

## `addendum-guard.log` (added by the follow-up commit)

Re-runs the identical two checks at the primary commit's own sha, after it has
been pushed and before the addendum's own file changes exist in the worktree:
`git status --porcelain` empty, and `git rev-parse HEAD` equal to
`git log --oneline -1 origin/main`. See the close-out section's addendum
paragraph for the exact sha this resolves to.

## `addendum2-guard.log` and `addendum3-guard.log` (added by the later correction commits)

The close-out section needed two prose corrections after its first addendum: one
rewording bare path-like tokens the section's own first draft introduced (which the
citation sweep flagged), and one removing tokens the correction itself
reintroduced. Each landed as a normal follow-up commit — never an amend or a
force-push — so each advanced the phase's final docs sha by one. These two logs run
the identical `git status --porcelain` / `git rev-parse HEAD` /
`git log --oneline -1 origin/main` triple at those later shas, captured before the
commit that adds each log had touched the worktree, so the clean-tree and
`HEAD == origin/main` pair is shown at each of them and not only at the first:
`addendum2-guard.log` at the first correction, `addendum3-guard.log` at the phase's
true final docs sha. The close-out section's second and third addendum paragraphs
name the exact shas and say which claim each one corrects.
