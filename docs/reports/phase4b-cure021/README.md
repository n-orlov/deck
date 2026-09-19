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

# Cure 021-cure-02 — theme_geometry_test.go's 100ms settle comparison

**What was red**: the same gate log,
`docs/reports/phase4b-final-suite/fullsuite-baf92ed.log`, also failed at
`features/theme_geometry_test.go:185` (the `renderThemeFrame` call site;
`t.Helper()` attributes the fatal there):

```
theme_geometry_test.go:185: theme "parchment" (ascii=true) frame kept changing after settling
```

`TestThemeChangesAttributesButNotFrameGeometry` stayed green when run in
isolation at `baf92ed` — this only reproduced under the gate's own load
(`go test -p=1 -count=1 ./...`, every package's tests running concurrently).

**Root cause**: `renderThemeFrame`'s settle guard took exactly one 100ms
sample pair — capture, sleep 100ms, capture again, fail if they differ. That
is a fixed budget for "is the client still painting its start-up frame",
and under host load (other packages' tests competing for CPU/scheduler
time) 100ms is not always long enough for a legitimately-still-settling
frame to finish painting. The old code could not tell that case apart from
an actual "frame never stops changing" bug — it had exactly one sample pair
to decide with.

**Cure** (`features/theme_geometry_test.go` only, no other file touched):
replaced the single sample-and-compare with a retry loop that keeps
re-sampling every 100ms, accepting the frame the moment two *consecutive*
samples agree, and only failing (same message, same meaning: "frame kept
changing after settling") once `settleTimeout` (5s — far longer than any
observed real settle time, short enough to still catch a genuinely
never-stabilising frame) has passed without ever producing an agreeing
pair. Every existing assertion in the file — the exact-content geometry
check, the attribute-difference count, the ASCII colour check — is
unchanged; only the settle guard's sampling budget changed, from one fixed
sample pair to a bounded retry loop.

**Evidence**: `ci/run.sh go test -count=1 -run
TestThemeChangesAttributesButNotFrameGeometry ./features/` at the fix
commit, exit 0:

```
ok  	github.com/n-orlov/deck/features	1.828s
```
(full log: `theme-geometry-run1.log`). Re-run 5 times sequentially, all exit
0 (1.7-1.9s each, not separately logged beyond the first). Re-run 3 more
times *concurrently* (three `go test` invocations launched together, to put
the scheduler contention the gate itself produces back under the test) —
all exit 0, full log of one of the three: `theme-geometry-load-run1.log`.

`ci/run.sh gofmt -l features/theme_geometry_test.go` and `ci/run.sh go build
./...` are both clean (no output) at the fix commit.

**Scope**: only `features/theme_geometry_test.go` changed. 021-resweep-01
(whole-suite gate, from scratch) is still open after this lands, per the
same standing-rules sentence quoted above.
