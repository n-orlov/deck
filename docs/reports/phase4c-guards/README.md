# Phase 4c — final-code guards (retake-01-01-06 re-take at cure-01-01-3's sha)

Re-take of the task-021-equivalent guard record (steering 003/004): build,
vet, gofmt and the four reserved paths re-verified at the final code sha,
plus the `schemaV8` search — re-run at the tree `cure-01-01-3` leaves,
i.e. current `HEAD`, superseding the prior record made at `10c5021`
(commit `db924a3`).

## Final code sha

The last commit touching a path outside `docs/` is:

    610be00c76b1c847a666d6dc4d18a12ba14fe37f
    tui: never let a background arrival steal an explicitly navigated header-only-sidebar cursor (cure-01-01-3, R136/R137, SPEC §11, #31, #32)

Confirmed with `git diff-tree --no-commit-id --name-only -r 610be00`, which
lists only `internal/tui/cure_01_01_3_header_cursor_test.go` and
`internal/tui/tui.go` — both outside `docs/`.

Every commit after it, up to `HEAD`, is docs-only (confirmed via
`git show --stat --format='' <sha>` on each):

    90f698f  docs: re-record the 19-package whole-suite gate at 610be00, post-cure-01-01-3 (task 022, #32)
      docs/reports/phase4c-fullsuite/README.md, docs/reports/phase4c-fullsuite/fullsuite.log
    68a38ae  docs: re-take phase4c stability10 sweep at 610be00 (retake-01-01-05)
      docs/reports/phase4c-stability10/README.md, docs/reports/phase4c-stability10/run-*.log

HEAD at guard time: `68a38ae748f318d7d457607cbf33b73212c1d093` (== `origin/main`
== `main`, tree clean), confirmed with `git merge-base --is-ancestor 610be00 HEAD`.

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

All five guards are green at the final code sha `610be00`. No behavioural
code has changed since; this record commit and its two predecessors on top
of that sha (`90f698f`, `68a38ae`) touch only `docs/` paths. This record
supersedes the prior guards record taken at `10c5021` (commit `db924a3`),
which predates `cure-01-01-3`'s header-cursor fix.
