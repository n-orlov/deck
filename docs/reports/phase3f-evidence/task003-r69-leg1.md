# Task 003 — R69 leg 1 evidence

## loadavg / toolchain
7.01 3.07 3.17 1/3957 3787
go version go1.25.13 linux/amd64

## Fixed: targeted test
=== RUN   TestReconcileCollectsRetainedDeadPaneWhenRowAlreadyReadsStopped
--- PASS: TestReconcileCollectsRetainedDeadPaneWhenRowAlreadyReadsStopped (0.08s)
PASS
ok  	github.com/n-orlov/deck/internal/service	0.083s

## Fixed: required packages
ok  	github.com/n-orlov/deck/internal/service	3.360s
ok  	github.com/n-orlov/deck/internal/store	2.250s

## Reverted guard (git stash push -- internal/service/reconcile.go), same test

=== RUN   TestReconcileCollectsRetainedDeadPaneWhenRowAlreadyReadsStopped
    reconcile_retained_corpse_test.go:92: collected row reads "stopped" (source "hook"), want error: the crash was not collected
--- FAIL: TestReconcileCollectsRetainedDeadPaneWhenRowAlreadyReadsStopped (0.07s)
FAIL
FAIL	github.com/n-orlov/deck/internal/service	0.070s
FAIL

Working tree restored byte-identical after `git stash pop` (diff against a pre-stash copy empty).
