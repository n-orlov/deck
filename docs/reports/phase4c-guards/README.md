# Phase 4c — final-code guards (retake-01-01-06 re-take at cure-01-01-4's sha)

Re-take of the task-021-equivalent guard record (steering 003/004): build,
vet, gofmt and the four reserved paths re-verified at the final code sha,
plus the `schemaV8` search — re-run at the tree `cure-01-01-4` (and task
015) leave, superseding the prior record made at `610be00`
(commit `027415c`).

## Final code sha

The last commit touching a path outside `docs/` is:

    be7cdbc32e5fea8bc0be69dc0c5df66bcbd0eaee
    ci: forward DECK_* selectors into the sibling (task 015)

Confirmed with `git diff-tree --no-commit-id --name-only -r be7cdbc`, which
lists only `ci/run.sh` — outside `docs/`.

`be7cdbc` itself lands on top of `cure-01-01-4`'s own code commit:

    2d282e2  tui: follow the viewport and re-anchor by ID when Esc clears a held filter (cure-01-01-4, R136, SPEC §11, ruling002)
      internal/tui/cure_01_01_4_esc_filter_viewport_test.go, internal/tui/tui.go

Every commit after `be7cdbc`, up to `HEAD`, is docs-only (confirmed via
`git show --stat --format='' <sha>` on each):

    a547e38  docs: re-record the 19-package whole-suite gate at be7cdbc, post-cure-01-01-4/task-015 (task 022, #32)
      docs/reports/phase4c-fullsuite/README.md, docs/reports/phase4c-fullsuite/fullsuite.log
    e6306a7  docs: retake stability10 sweep at be7cdbc leaves (retake-01-01-05)
      docs/reports/phase4c-stability10/README.md, docs/reports/phase4c-stability10/run-*.log

HEAD at guard time: `e6306a787aa10b2c4b5b214319e5931571fc749b` (== `origin/main`
== `main`, tree clean), confirmed with `git merge-base --is-ancestor be7cdbc HEAD`.

## Guard results (all via `ci/run.sh`, image `deck-ci:local`)

### `go build ./...`

    $ ci/run.sh go build ./...
    exit=0

Full output: [`build.log`](build.log) — empty stdout/stderr, exit 0.

### `go vet ./...`

    $ ci/run.sh go vet ./...
    exit=0

Full output: [`vet.log`](vet.log) — empty stdout/stderr, exit 0.

### `gofmt -l .`

    $ ci/run.sh gofmt -l .
    .spike-preview/cmd/conformance/main.go
    .spike-preview/conformance/conformance.go
    .spike-preview/conformance/conformance_test.go
    exit=0

Full output: [`gofmt.log`](gofmt.log) — exactly the three pre-existing
`.spike-preview/` files (unrelated spike code, never in scope for this run),
no file this run touched.

### Protected-path audit

    $ git diff --stat 2752c9e..HEAD -- SPEC.md prds/ ci/Dockerfile ci/SPIKE.md
    (no output)
    exit=0

Full output: [`protected-path-audit.log`](protected-path-audit.log) — empty,
so none of the four reserved paths (`SPEC.md`, `prds/`, `ci/Dockerfile`,
`ci/SPIKE.md`) was modified between the launch sha `2752c9e` and this guard's
`HEAD`.

### `schemaV8` search

    $ grep -rn "schemaV8" internal/store
    (no output)
    exit=1

Full output: [`schemav8-search.log`](schemav8-search.log) — no match (grep's
exit 1 signals "no match", not a failure of the search itself).

## Conclusion

All five guards are green at the final code sha `be7cdbc`. No behavioural
code has changed since; this record commit and its two predecessors on top
of that sha (`a547e38`, `e6306a7`) touch only `docs/` paths. This record
supersedes the prior guards record taken at `610be00` (commit `027415c`),
which predates task 015 and `cure-01-01-4`'s Esc-viewport fix.
