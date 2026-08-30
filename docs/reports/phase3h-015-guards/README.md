# Phase 3h task 015 — re-verify the protected-path and push guards at the true final sha

This report re-runs every guard the standing rules require, at the tree this task itself
produces (this commit does not touch any `*.go`/`*.feature` file, so the "final code sha"
below is unchanged from task 007/008/009/010's `2ccb1d3`). Because committing this very
README moves `HEAD` past the commit it was measured against, an immediately-following
one-line addendum commit (same `(task 015)` tag) re-publishes the identical HEAD/`origin/main`
triple at *that* commit's own sha — the addendum's own text names its sha explicitly.

## HEAD vs `origin/main`, and worktree cleanliness

Measured against the commit immediately preceding this report's own commit (task 014's
`ed469e4`, `HEAD` before this commit lands):

```
$ git rev-parse HEAD
ed469e4cd235051e6755cf821ac90077ab2efedb
$ git rev-parse origin/main
ed469e4cd235051e6755cf821ac90077ab2efedb
$ git status --porcelain
(empty)
```

`HEAD` and `origin/main` agree, and the worktree is clean before this report's own commit is
made — i.e. the push guard ("a task is not done until `git rev-parse HEAD` ==
`git rev-parse origin/main`") holds at the moment this measurement was taken.

## Protected-path guard, the worker-write range `a24ff8d..HEAD`

```
$ git log --oneline a24ff8d..HEAD -- SPEC.md prds/ ci/Dockerfile ci/SPIKE.md
(empty)
$ git diff --stat a24ff8d..HEAD -- SPEC.md prds/ ci/Dockerfile ci/SPIKE.md
(empty)
```

Both empty: no commit this run has written (`a24ff8d`'s child through `ed469e4`, the tip
measured above) touches any of the four protected paths.

## Protected-path guard, the PRD's literal range `de90a5c..HEAD`

```
$ git log --oneline de90a5c..HEAD -- SPEC.md prds/ ci/Dockerfile ci/SPIKE.md
a24ff8d plan: add Phase 3h — bring the suite to the §7 ruling, fix F31, hold the gate at one sha (operator)
```

Exactly one commit, `a24ff8d`. As task 014's finding
([`docs/reports/phase3h-findings.md`, §1](../phase3h-findings.md#1-the-prds-de90a5chead-protected-path-range-disagrees-with-the-tree))
already establishes, `a24ff8d` is the **operator's own plan commit**, adding
`prds/phase3h-suite-reconciliation.md` (152 insertions, one file — see that finding's own
`git show --stat a24ff8d` quote), written before this run's own task work starts and never a
worker write. The PRD's literal `de90a5c..HEAD` range can therefore never be empty, and this
one-commit listing is exactly the disagreement task 014 already filed as a finding — not a
new discovery, and not treated as a reason to edit `a24ff8d` or `de90a5c`.

## Scenario- and theme-file diff guards

```
$ git diff --exit-code a24ff8d..HEAD -- features/godog_test.go features/attention_sort.feature
(no output)
exit status: 0
$ git diff --exit-code a24ff8d..HEAD -- internal/theme/builtin/
(no output)
exit status: 0
```

Neither `features/godog_test.go`'s `defaultTags` nor `features/attention_sort.feature` nor
any file under `internal/theme/builtin/` has changed since the operator's own plan commit —
the "never narrow a deliverable sweep" and "no theme palette change" prohibitions both hold.

## Final code sha

```
$ git log -1 --format=%H -- '*.go' '*.feature'
2ccb1d3c698d3c256aa94aea7ef2e3a58dc0641a
```

Unchanged from tasks 007–010's measurements: this task's own commit touches only
`docs/reports/phase3h-015-guards/README.md`, no `*.go` or `*.feature` file, so the final code
sha this run's gates were measured against is still `2ccb1d3`.

## Re-checking the citations in this report

```
$ git cat-file -e de90a5c^{commit} && echo ok
ok
$ git cat-file -e a24ff8d^{commit} && echo ok
ok
$ git ls-files --error-unmatch docs/reports/phase3h-findings.md
docs/reports/phase3h-findings.md
```

Both cited shas resolve and the one cited path is tracked.

## Addendum — re-published at this guard commit's own sha

This README's own commit (`docs: re-verify protected-path and push guards at final sha
(task 015)`) is `c69720f63fef52384f32117494729c1fb5ff6c5d`. Re-running the HEAD/`origin/main`
triple immediately after that commit was pushed, at that commit's own sha:

```
$ git rev-parse HEAD
c69720f63fef52384f32117494729c1fb5ff6c5d
$ git rev-parse origin/main
c69720f63fef52384f32117494729c1fb5ff6c5d
$ git status --porcelain
(empty)
```

`HEAD` and `origin/main` agree at `c69720f`, and the worktree is clean. The push guard holds
at the guard commit's own sha, not only at the commit measured in the body above.
