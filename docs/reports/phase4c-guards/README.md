# Phase 4c — final-code guards (retake-01-01-06, cure-01-09 re-take)

Re-take of the task-021-equivalent guard record (steering 003/004): build,
vet, gofmt and the four reserved paths re-verified at the final code sha,
plus the `schemaV8` search — re-run at the tree `cure-01-01-2` leaves,
i.e. current `HEAD`, superseding the prior record made at `1ad530b`.

## Final code sha

The last commit touching a path outside `docs/` is:

    10c5021f1f70549d54f6717a697c69ed6fe2b797
    tui: name the header cursor's fold keys in the list footer too (task 015, #32)
    2026-09-24 13:08:13 +0000

Confirmed by walking commits newest-first and checking
`git diff-tree --no-commit-id --name-only -r <sha>` against `^docs/` — this
is the first (most recent) commit whose changed-file list contains a
non-`docs/` path.

Every commit after it, up to `HEAD`, is docs-only (confirmed via
`git show --stat --format='' <sha>` on each):

    806414a11e7516ddf2d7ff5b6e5a161720b72a93  docs: re-record the 19-package whole-suite gate at 10c5021, post-015 (task 022, #32)
      docs/reports/phase4c-fullsuite/README.md, docs/reports/phase4c-fullsuite/fullsuite.log
    94879552cd246b973b15fe2675054bc4d6ab5a17  docs: retake new_session_select_test.go at cure-01-01-2 leaves (retake-01-01-03-2)
      docs/reports/phase4c-retake-01-01-03-2/README.md, docs/reports/phase4c-retake-01-01-03-2/package.log, docs/reports/phase4c-retake-01-01-03-2/targeted.log
    9c1c84b2a05786c5c14bcf3d9b3f129feb77d2e9  docs: retake ten-run stability sweep at cure-01-01-2's leaves (retake-01-01-05)
      docs/reports/phase4c-stability10/README.md, docs/reports/phase4c-stability10/run-*.log

HEAD at guard time: `9c1c84b2a05786c5c14bcf3d9b3f129feb77d2e9` (== `origin/main`
== `main`, tree clean).

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

All five guards are green at the final code sha `10c5021`. No behavioural
code has changed since; this record commit and its three predecessors on
top of that sha (`806414a`, `9487955`, `9c1c84b`) touch only `docs/` paths.
This record supersedes the prior guards record taken at `1ad530b` (before
015's footer cure landed).
