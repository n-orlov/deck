# Task 002 — targeted proof

Forward-reverted `0b7dce5` (commit `afa55b8`) to restore the narrow
live-pane repair trigger in `internal/service/reconcile.go`, per de90a5c's
ruling (`spec: resolve §7's error-under-live-pane self-contradiction and
land §11.8's R93 wording (operator)`).

Command:

    ci/run.sh go test -count=1 ./internal/service/ ./internal/store/

Result: exit 0 (see `test.exitstatus`), full output in `test.log`.

    ok  	github.com/n-orlov/deck/internal/service	4.770s
    ok  	github.com/n-orlov/deck/internal/store	2.911s
