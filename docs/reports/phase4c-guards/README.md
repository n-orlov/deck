# Phase 4c — final-code guards (cure-01-09, operator ruling 1 / steering 003)

Task-021 equivalent per steering 003: build, vet, gofmt and the four reserved
paths re-verified at the final code sha, plus the `schemaV8` search.

## Final code sha

The last commit touching a path outside `docs/` is:

    1ad530b3c63335c6a06ca07950640dafe410d75b
    tui: follow selection off an empty header through live filter edits too (task 022, #32)
    2026-09-24 10:32:17 +0000

Every commit after it, up to `HEAD`, is docs-only (confirmed via
`git show --stat --format='' <sha>` on each):

    da92f63663a711b94ee8b82641e45ff1270df54a  docs: re-record the 19-package whole-suite gate at 1ad530b, post-sweep-cure (task 022, #32)
      docs/reports/phase4c-fullsuite/README.md, docs/reports/phase4c-fullsuite/fullsuite.log
    6d861a0689473b73acd7c36de692539c52ac8ae4  docs: record phase 4c ten-run stability sweep (cure-01-06)
      docs/reports/phase4c-stability10/README.md, docs/reports/phase4c-stability10/run-*.log

HEAD at guard time: `6d861a0689473b73acd7c36de692539c52ac8ae4` (== `origin/main`
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

All five guards are green at the final code sha `1ad530b`. No behavioural code
has changed since; this record commit and its two predecessors
(`da92f63`, `6d861a0`) touch only `docs/` paths.
