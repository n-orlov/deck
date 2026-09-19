# Cure 021-cure-01 — kill_delete_undo.feature's create-step "starting" wait

**What was red**: `docs/reports/phase4b-final-suite/fullsuite-baf92ed.log` (task
021's whole-suite gate, `ci/run.sh go test -p=1 -count=1 ./...` at code sha
`baf92ed`) failed the scenario at `features/kill_delete_undo.feature:701`
("settings' group-delete d branch routes through the same dd batch confirm
and one u restores the whole batch (R131 part 2)") with:

```
step error: timed out waiting for frame "starting": context deadline exceeded
```

The frame dumped alongside the timeout already showed
`groups-delete-dd-one running` in the sidebar — "starting" was nowhere on
screen. The create had already succeeded; the wait was still looking for a
transient label the client had already left.

**Root cause**: `clientCreatesShellSessionIntoGroupWithFreshCWDLabelled`
(features/create_session_test.go) ended with a bare
`client.WaitForFrame(ctx, false, "starting")`. `internal/service/
reconcile.go`'s shell fast-path (`session.Agent == "shell" && session.Status
== "starting"` → `"running"`) can promote a freshly created shell session to
"running" fast enough, especially once the group-create path's extra field
cycling (`cycleCreateModalGroupFieldTo`) delays the submit, that no single
rendered frame ever carries the bare word "starting" — the create modal's
close and the reconcile promotion can land between two polls of the
emulator. `WaitForFrame` has no way to know the session got there some other
way; it just times out.

**Cure** (`features/create_session_test.go`, no product code touched): added
`waitForCreatedSessionPastStarting`, which accepts either the transient
"starting" frame or the row reporting `<sessionName> running` directly, via
`WaitForFrameFunc` rather than the single-string `WaitForFrame`.
`clientCreatesShellSessionIntoGroupWithFreshCWDLabelled` now calls it instead
of waiting on the bare "starting" substring. No scenario, step or assertion
was dropped — the wait still positively confirms the create landed, it is
just no longer pinned to one specific transient render of it.

**Evidence**: `ci/run.sh env DECK_GODOG_PATHS=kill_delete_undo.feature go
test -count=1 -run TestFeatures -v ./features/` at the fix commit — full log
in `kill_delete_undo-run.log`, tail:

```
--- PASS: TestFeatures/settings'_group-delete_d_branch_routes_through_the_same_dd_batch_confirm_and_one_u_restores_the_whole_batch_(R131_part_2) (1.64s)
PASS
ok  	github.com/n-orlov/deck/features	34.308s
exit=0
```

Run 3 more times without `-v` to check for flakiness under repetition, all
green: `ok github.com/n-orlov/deck/features 34.156s` / `34.294s` /
`34.302s` (exit 0 each time; not separately logged — the `-v` run above is
the one committed).

`ci/run.sh go build ./...`, `ci/run.sh go vet ./...` and `ci/run.sh gofmt -l
features/create_session_test.go` are all clean (no output) at the fix
commit.

**Scope**: only `features/create_session_test.go` changed. This does not
re-run the whole-suite gate — that is 021-resweep-01's job, after this and
021-cure-02 both land, per the standing rules ("a red lane a sweep finds
becomes a new task and the gate is re-run from scratch afterwards").
