# Build/vet/gofmt guards — approach 2, re-run at the corrected tail sha (task 011b)

- **Tail code sha**: `0ba550a5e50bdfc84586d5328a0690af9c9888c4` (`0ba550a`,
  `features: settle the golden frame on a quiet PTY, not a torn read (task
  011b)`) — the same tail code sha named in
  `docs/reports/phase4-cure-final-suite/README.md`. These guards ran
  against exactly that commit's tree: `git diff --stat 0ba550a HEAD --
  '*.go' '*.feature'` printed nothing at the time they ran, and every
  commit written afterwards in this task is docs-only.
- **When**: `2026-09-17T02:10Z`, immediately after the full-suite gate
  above returned exit `0` and before the ten-run stability sweep started.

- **Commands as run** (each via `ci/run.sh`, the sibling-container wrapper
  named in the standing rules; none narrowed — full `./...` / `.` scope,
  no filter):

  ```
  ci/run.sh sh -c 'go build ./...'
  ci/run.sh sh -c 'go vet ./...'
  ci/run.sh sh -c 'gofmt -l .'
  ```

- **`go build ./...` output** (exit `0`):

  ```
  (no output)
  ```

- **`go vet ./...` output** (exit `0`):

  ```
  (no output)
  ```

- **`gofmt -l .` output** (exit `0`):

  ```
  .spike-preview/cmd/conformance/main.go
  .spike-preview/conformance/conformance.go
  .spike-preview/conformance/conformance_test.go
  internal/theme/quantize_test.go
  ```

- **Result**: `go build ./...` and `go vet ./...` both report no problems
  (empty output, exit `0`). `gofmt -l .` lists exactly the same four
  pre-existing files measured at plan time (`08a1ffe`, per the standing
  rules), and neither of task 011b's two code commits added to that list —
  `1f38195` touched only the golden testdata fixture
  (`features/testdata/golden/side_by_side_80x24.golden`, not a `.go`
  file), and `0ba550a`'s `features/golden_frame_test.go` was gofmt-formatted
  before it was committed:
  - `internal/theme/quantize_test.go`
  - `.spike-preview/cmd/conformance/main.go`
  - `.spike-preview/conformance/conformance.go`
  - `.spike-preview/conformance/conformance_test.go`

  No other path appears in the `gofmt -l .` output. gofmt is clean on
  every file this run wrote.

## Disposition

This overwrites the earlier content of this directory per the standing
rules ("013/014/015 are re-run from scratch afterwards, never re-run under
the task that found the red lane"): task 013's red sweep at `e93a790`, and
then task 011b's own second code commit `0ba550a`, each forced a fresh run.
Task 014 has produced no report of its own (it was still pending, blocked
behind 013's deferral, when this ran), so there is no first-attempt content
here to preserve as history beyond this file's own git history — this is
014's clean, from-scratch guard result at the corrected tail sha, run under
task 011b per that task's own criteria.
