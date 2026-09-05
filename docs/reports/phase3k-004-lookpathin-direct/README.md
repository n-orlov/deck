# Task 004: unit-test lookPathIn directly

Added three tests in `internal/service/availability_test.go`, exercising the
package-private `lookPathIn` function directly (not through `kindAvailable`
or `AvailableKinds`):

- `TestLookPathInFindsExecutableOnPath`: an executable named `pi` present in
  one of `pathEnv`'s directories → nil error.
- `TestLookPathInErrorsWhenNameAbsentFromPath`: name absent from `pathEnv` →
  non-nil error.
- `TestLookPathInErrorsWhenNameIsADirectory`: `pathEnv` contains a directory
  (not a file) whose name matches the requested name → non-nil error.

## Verification

```
$ ci/run.sh go test -count=1 -v -run 'LookPath' ./internal/service/
=== RUN   TestLookPathInFindsExecutableOnPath
--- PASS: TestLookPathInFindsExecutableOnPath (0.00s)
=== RUN   TestLookPathInErrorsWhenNameAbsentFromPath
--- PASS: TestLookPathInErrorsWhenNameAbsentFromPath (0.00s)
=== RUN   TestLookPathInErrorsWhenNameIsADirectory
--- PASS: TestLookPathInErrorsWhenNameIsADirectory (0.00s)
PASS
ok  	github.com/n-orlov/deck/internal/service	0.003s
```

Whole-package run: `ci/run.sh go test -count=1 ./internal/service/` → `ok` 6.256s.
