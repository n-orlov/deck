# Build/vet/gofmt guards — approach 2, re-run at the corrected tail sha (task 011b)

- **Tail code sha**: `e93a7902582672d799dadfa8bbaa4f1f25eb28dd` (`e93a790`,
  task 011) — the same tail code sha named in
  `docs/reports/phase4-cure-final-suite/README.md`. The tree these guards
  ran against is `HEAD` (`1f38195`, task 011b's golden-fixture
  regeneration) at commit time, which differs from `e93a790` in no
  `*.go`/`*.feature` file: `git diff --stat e93a790 HEAD -- '*.go'
  '*.feature'` prints nothing.

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
  rules) and this run's own commit (the golden-fixture regeneration,
  `features/testdata/golden/side_by_side_80x24.golden`, not a `.go` file)
  did not add to that list:
  - `internal/theme/quantize_test.go`
  - `.spike-preview/cmd/conformance/main.go`
  - `.spike-preview/conformance/conformance.go`
  - `.spike-preview/conformance/conformance_test.go`

  No other path appears in the `gofmt -l .` output. gofmt is clean on
  every file this run wrote.

## Disposition

This overwrites task 014's own report directory per the standing rules
("013/014/015 are re-run from scratch afterwards, never re-run under the
task that found the red lane" — task 013's original sweep, whose
red-lane finding this task's own fix answers, is what forces this
re-run). Task 014 had not yet produced its own first-attempt report when
this task ran (it was still `pending`, blocked behind 013's deferral), so
there is no prior content here to preserve as history — this is simply
014's clean, from-scratch result at the corrected tail sha, run under
task 011b per that task's own criteria.
