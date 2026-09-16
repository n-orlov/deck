# Build/vet/gofmt guards

- **Code sha**: `db669658ce20de10ef6aaad311c94f6830538436` (`db66965`), the
  tail code sha named in `docs/reports/phase4-final-suite/README.md` (task
  039) — the most recent commit touching a `*.go` or `*.feature` file. The
  tree these guards ran against is `HEAD` at commit time, which differs
  from `db66965` in no `*.go`/`*.feature` file: `git diff --stat db66965
  HEAD -- '*.go' '*.feature'` prints nothing.

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
  (empty output, exit `0`). `gofmt -l .` lists exactly the four files
  above — all four are **pre-existing drift** measured at `08a1ffe` (per
  the standing rules) and this run did not reformat any of them:
  - `internal/theme/quantize_test.go`
  - `.spike-preview/cmd/conformance/main.go`
  - `.spike-preview/conformance/conformance.go`
  - `.spike-preview/conformance/conformance_test.go`

  No other path appears in the `gofmt -l .` output. gofmt is clean on
  every file this run wrote.
