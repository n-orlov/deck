# Phase 4c — ten-run stability sweep (retake, retake-01-01-05)

Sweep re-taken (re-recorded) with `ci/stability.sh 10` at final code sha
**`be7cdbc`** (`be7cdbc996b356a2b17ad31a4b7e095e0a98c98a`, "ci: forward
DECK_* selectors into the sibling (task 015)") — the last commit touching a
path outside `docs/`, i.e. the tree the code cures (`cure-01-01-2`,
`cure-01-01-3`, `cure-01-01-4`) and task 015 leave once landed. `main` ==
`origin/main` at the time of this retake, clean tree, verified by
`git merge-base --is-ancestor be7cdbc HEAD`. This retake supersedes the
prior recording here (sha `610be00`, commit `68a38ae`, predates
`cure-01-01-4` and task 015) at the current final code sha.

Commits between the prior retake's sha and this one, confirmed non-docs
with `git diff-tree --no-commit-id --name-only -r <sha>`:

    2d282e2  tui: follow the viewport and re-anchor by ID when Esc clears a held filter (cure-01-01-4, R136, SPEC §11, ruling002)
      internal/tui/cure_01_01_4_esc_filter_viewport_test.go, internal/tui/tui.go
    be7cdbc  ci: forward DECK_* selectors into the sibling (task 015)
      ci/run.sh

Each run invokes `ci/run.sh go test -p=1 -count=1 ./...` from a clean state
(fresh per-run container, `-count=1` disables the test cache) and is
labelled PASS/FAIL from `go test`'s own exit status, never a pipe's (see
`ci/stability.sh`'s header comment for why that distinction matters).

## Result: **10/10 passed**

| Run | Result | Log |
|-----|--------|-----|
| 1 | PASS (exit 0) | [`run-1.log`](./run-1.log) |
| 2 | PASS (exit 0) | [`run-2.log`](./run-2.log) |
| 3 | PASS (exit 0) | [`run-3.log`](./run-3.log) |
| 4 | PASS (exit 0) | [`run-4.log`](./run-4.log) |
| 5 | PASS (exit 0) | [`run-5.log`](./run-5.log) |
| 6 | PASS (exit 0) | [`run-6.log`](./run-6.log) |
| 7 | PASS (exit 0) | [`run-7.log`](./run-7.log) |
| 8 | PASS (exit 0) | [`run-8.log`](./run-8.log) |
| 9 | PASS (exit 0) | [`run-9.log`](./run-9.log) |
| 10 | PASS (exit 0) | [`run-10.log`](./run-10.log) |

No failure occurred in any of the 10 runs (`grep -n '^--- FAIL\|^FAIL'`
across all ten committed logs returns nothing), so neither of the two known
advisory flake classes (transient-`starting`, `SIGWINCH` exact count)
recurred in this retake — there is nothing to mark advisory this time; both
remain advisory in general per the PRD.

Each run reports the same 19 packages (15 `ok`, 4 `[no test files]`:
`internal/notify`, `internal/search`, `internal/unit`,
`internal/tui/sidebarcursor`), consistent with `ci/run.sh go list ./...` at
this sha.

Raw combined script output (all ten `=== RUN N ===` markers plus the final
`10/10 passed` summary line) was preserved in the sweep's own tmp working
directory (`/tmp/deck-stability.xHxl0L`) from the run that produced these
logs; the per-run logs above are the ones actually committed as evidence.
