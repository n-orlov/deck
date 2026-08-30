# Phase 3h — task 201: restore gofmt-clean formatting in `internal/service/reconcile.go`

## Final code sha

`git log -1 --format=%H -- '*.go' '*.feature'` → `4b1d4dcbd4480013470a0555795e6c64db3bf96d`
(this is also the sha of this task's own commit — the sole commit in this task's work that
carries `(task 201)`, the one touching `internal/service/reconcile.go`; this report is published
by a separate, immediately-following docs-only commit that touches no `*.go`/`*.feature` path and
so does not shift the final code sha).

## Commit under measurement

`4b1d4dcbd4480013470a0555795e6c64db3bf96d` — "service: restore gofmt-clean formatting of the
AllowedCurrentStatuses guard (task 201)".

## gofmt is clean on the file we touched

```
$ ci/run.sh gofmt -l internal/service/reconcile.go
$ echo $?
0
```

Output is empty and the exit status is `0`: `internal/service/reconcile.go` is gofmt-clean at
this commit.

## The change is whitespace-only

```
$ git diff -w --exit-code 2ccb1d3 HEAD -- internal/service/reconcile.go
$ echo $?
0
```

`git diff -w` (ignore-whitespace) between `2ccb1d3` (R96's guard commit, gofmt-dirty) and this
task's commit is empty and exits `0`: no non-whitespace change was made.

`git diff 2ccb1d3 HEAD -- internal/service/reconcile.go` (no `-w`) shows only realignment of the
`store.StatusUpdateInput` struct literal's field colons/values — the six touched lines
(`SessionID`, `Status`, `Reason`, `Source`, `At`, `EventKind`) are re-column-aligned to match the
already-correctly-aligned `AllowedCurrentStatuses` field that `2ccb1d3` (R96, task 007) added;
nothing else in the file changed.

R96's guard survives, unedited in substance:

```
$ grep -n 'AllowedCurrentStatuses: \[\]string{"starting"}' internal/service/reconcile.go
125:						AllowedCurrentStatuses: []string{"starting"},
```

## The two pre-existing gofmt-dirty files were not touched

```
$ ci/run.sh gofmt -l internal/theme/quantize_test.go internal/tui/footer_bindings_parity_test.go
internal/theme/quantize_test.go
internal/tui/footer_bindings_parity_test.go
```

Both still list as gofmt-dirty at this commit — unchanged from before this task, per the notes
file's "State of the tree" record (they were already gofmt-dirty at `a24ff8d`, the operator's own
plan commit, and are out of this task's scope).

```
$ git diff --exit-code a24ff8d HEAD -- internal/theme/quantize_test.go internal/tui/footer_bindings_parity_test.go
$ echo $?
0
```

Empty diff, exit `0`: neither file was touched by this task's commit or any commit since
`a24ff8d`.

## Targeted test run

`ci/run.sh go test -count=1 ./internal/service/ ./internal/store/` at this commit — unedited log
and exit status committed alongside this README:

- log: `docs/reports/phase3h-201-gofmt/go-test-service-store.log`
- exit status: `docs/reports/phase3h-201-gofmt/go-test-service-store.exitstatus` → `0`

```
$ cat docs/reports/phase3h-201-gofmt/go-test-service-store.log
ok  	github.com/n-orlov/deck/internal/service	4.848s
ok  	github.com/n-orlov/deck/internal/store	2.919s
$ cat docs/reports/phase3h-201-gofmt/go-test-service-store.exitstatus
0
```

## Scope note

This task touches a `*.go` file, so both deliverable gates (`ci/run.sh go test -p=1 -count=1
./...` and `ci/stability.sh 10`) must be re-measured at this new final code sha
(`4b1d4dcbd4480013470a0555795e6c64db3bf96d`) before either gate can be republished as current;
`phase3h-008`/`phase3h-009`/`phase3h-010` (measured at `2ccb1d3`) become superseded history, not
deleted. Re-measuring those gates is out of scope for task 201 itself.
