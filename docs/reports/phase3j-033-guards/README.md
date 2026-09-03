# Phase 3j task 033 — protected-path audit and branch guards re-verify

Re-run at the true final code sha, in a single iteration, per task 033's
`successCriteria`. All four commands below were run back-to-back in one
iteration on a clean tree; every output shown is the exact, unedited command
output — nothing summarised or rounded.

## 1. Recomputed final code sha

Command:

```
git log -1 --format=%H -- '*.go' '*.feature'
```

Output:

```
a44ee320b93186496d56364836b0aed00a6f1e0b
```

## 2. Protected-path audit (PRD / standing-rules command, run verbatim)

Command:

```
BASE=$(git log --format=%H --diff-filter=A -1 -- prds/phase3j-launch-and-teardown-hooks.md); git log --oneline "$BASE..HEAD" -- SPEC.md prds/ ci/Dockerfile ci/SPIKE.md
```

`$BASE` resolved to `06ea5b72d98ce0dda251d16af452b7f6d87e2c2f`.

Output:

```
(empty)
```

No lines were printed — `SPEC.md`, `prds/`, `ci/Dockerfile` and `ci/SPIKE.md`
have not been touched since the PRD's own base commit. Guard holds.

## 3. Working tree cleanliness

Command:

```
git status --porcelain
```

Output:

```
(empty)
```

## 4. Branch guard — HEAD vs. origin/main

Command:

```
git rev-parse HEAD origin/main
```

Output:

```
899af55dbee44be238920cc227d4873856c7d201
899af55dbee44be238920cc227d4873856c7d201
```

Both lines agree: `HEAD` and `origin/main` are the same commit
(`899af55dbee44be238920cc227d4873856c7d201`), i.e. the tree is fully pushed
and in sync with the remote's `main` branch.

## Summary

| Check | Result |
|---|---|
| Final code sha (`*.go`/`*.feature`) | `a44ee320b93186496d56364836b0aed00a6f1e0b` |
| Protected-path audit | empty (no violations) |
| `git status --porcelain` | empty (clean tree) |
| `HEAD` vs `origin/main` | agree at `899af55dbee44be238920cc227d4873856c7d201` |

All four guards pass at the true final code sha. This report supersedes
nothing (task 033 has no prior published directory); it is filed under
`docs/reports/phase3j-033-guards/` per the task's own successCriteria.
