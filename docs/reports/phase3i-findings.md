# Phase 3i findings

Companion to [`phase3i.md`](phase3i.md) (the per-requirement evidence report for R98–R103).
Modelled on [`phase3h-findings.md`](phase3h-findings.md): what the requirement table does not
carry — a task that ended `failed` and what discharges its residual, a protected-path audit
range that necessarily contains an operator commit, both gates' disposition quoted verbatim,
and this phase's check for a recurrence of every finding the PRD names out of scope.

Written by task 131 against the tree at the commit this report itself lands in (a docs-only
descendant of the final code sha `b9243a1f415ba9ca77cc2ffa2ec557ca8b1be4cb`; see
[`phase3i.md`](phase3i.md)'s own "Final code sha" section for the ancestry argument, which this
report does not repeat). Every sha cited resolves under `git cat-file -e` and every path cited
is tracked under `git ls-files --error-unmatch`, both checked in
[§4](#4-how-to-re-check-every-citation-in-this-report).

- [1. Task 119 ended `failed (validation-exhausted)` on test strength, not on product behaviour — discharged by task 136](#1-task-119-ended-failed-validation-exhausted-on-test-strength-not-on-product-behaviour--discharged-by-task-136)
- [2. The protected-path audit range `6197b53..HEAD` necessarily contains one operator commit](#2-the-protected-path-audit-range-6197b53head-necessarily-contains-one-operator-commit)
- [3. Gate disposition: whole-suite sweep and ten-run stability, both clean, no out-of-scope recurrence](#3-gate-disposition-whole-suite-sweep-and-ten-run-stability-both-clean-no-out-of-scope-recurrence)
- [4. How to re-check every citation in this report](#4-how-to-re-check-every-citation-in-this-report)

## 1. Task 119 ended `failed (validation-exhausted)` on test strength, not on product behaviour — discharged by task 136

Task 119 ("Prove the two displacement flavours tear down differently") is `failed` in
`tasks.json`, `failureKind: "validation-exhausted"`, after 3 validation attempts. `failed` has
no legal successor in the vigilant status table (steer 001 confirmed this explicitly rather
than moving it), so it stands as history; this section is the honest disclosure of what that
status does and does not mean, per the standing rules ("A `failed` task is reported as failed
... its residual is recorded in `docs/reports/phase3i-findings.md` with the task that
discharges it").

**What failed, quoted verbatim from task 119's own `validationNotes`:**

> Residual gap: the stolen-claim test still cannot detect an erroneous call to
> `WindowOwnership.Release`, because it asserts zero `@deck_isize_owner` unsets but `Release`
> reads the winner's claim and returns without unsetting it; `ownership.go` confirms that
> behavior, and the test does not count show-options reads.

Confirmed against the committed code today, `internal/tmux/ownership.go:342-350`:

```go
func (o *WindowOwnership) Release(ctx context.Context) error {
	got, err := o.client.readWindowOwnership(ctx, o.target)
	if err != nil {
		return err
	}
	if !got.Set || got.Value != o.claim {
		return nil
	}
	return o.client.unsetWindowOwnership(ctx, o.target)
}
```

A spurious call to `Release` on a stolen claim costs exactly one extra `show-options` **read**
and zero unsets — invisible to a test that only counts unsets, which is what task 119's
committed test does.

**This is a gap in verification strength, not in product behaviour.** Task 119's own
`successCriteria` asked, for the stolen-claim flavour, that the test show it "releases
nothing" — and the committed test does prove exactly that: zero resize-window commands, zero
option unsets, byte-exact preservation of both winner-set options (`validationNotes`, same
record: "the stolen test wire-asserts zero resize-window and zero option unsets and
byte-exact preservation of both winner-set options"). Verify's bar — distinguish `Release`'s
own internal ownership read from the test's one required `claimStillMine` probe, i.e. prove
`Release` is never *invoked* at all, not merely that it has no effect — is strictly stronger
than what 119's criteria asked for. The gap between "releases nothing" (proved) and "never
calls Release" (not provable by 119's test) is what task 119 failed to close within its
attempt budget, and what task 136 was carved out to close on its own.

**Task 136** ("Prove the stolen displacement flavour never invokes Release", `dependsOn:
["118"]` since 119 is terminal and cannot be depended on) discharges that residual. Its commit
is `fcdb994` ("tui: count ownership-option reads in stolen-claim teardown test (task 136)"),
landing in `internal/tui/displacement_teardown_test.go`. It counts `show-options` **reads** of
`@deck_isize_owner` on the tmux wire, not only unsets: the stolen-claim test now asserts
exactly one such read (the test's own required `claimStillMine` probe) and zero unsets,
which is load-bearing exactly where the unset-only assertion was not — a second read, which is
what a spurious `Release` call would cost, would fail that count. Task 136's own record
confirms the mechanism was verified working, not merely asserted: its notes report that
`teardownInteractiveClaim` in `internal/tui/interactive.go` gates on `if !stillMine { return }`
strictly above the `ownership.Release(ctx)` call, so on a stolen claim `Release` is structurally
unreachable — and 136 is `validated` (`ci/run.sh go test -count=1 ./internal/tui/` exits 0).

**No requirement row in [`phase3i.md`](phase3i.md) leans on task 119.** R101's row cites tasks
116, 117, 118, 120, 136 and 121–123, and its own prose states explicitly: "Task 119 is
deliberately absent from that list and from the row's shas; its status and disposition are
recorded in the phase 3i findings report (task 131), not here" — this section is that record.

## 2. The protected-path audit range `6197b53..HEAD` necessarily contains one operator commit

The standing rules' audit command is `git log --oneline 6197b53..HEAD -- SPEC.md prds
ci/Dockerfile ci/SPIKE.md`, required to be empty. Run fresh against this report's own tree, it
is **not** empty:

```
$ git log --oneline 6197b53..HEAD -- SPEC.md prds ci/Dockerfile ci/SPIKE.md
3090b68 prds: cut Phase 3i — force-attach, stealing the interactive preview (operator)
```

`3090b68` is the operator's own commit that adds `prds/phase3i-force-attach.md` — the PRD file
this very phase's plan was cut from — landing three minutes after `6197b53` (the operator's
`SPEC.md` amendment that opens the audit range) and, structurally, before any worker task in
this run could exist to violate the guard:

```
$ git show --stat 3090b68
commit 3090b68e990bfb063150cbb46f4c3a93bf574883
Author: Nik <nikolaiorl@gmail.com>
Date:   Wed Sep 2 09:00:21 2026 +0100

    prds: cut Phase 3i — force-attach, stealing the interactive preview (operator)
```

This is the same shape phase 3h's findings report disclosed for its own audit range
(`de90a5c..HEAD` necessarily containing the operator's own plan commit `a24ff8d`): whichever
commit opens a protected-path audit range, if the operator's own next act is to add the PRD (or
amend the spec) that names that range, the range will contain that commit by construction. This
run's own worker task commits are the ones the guard actually polices; confirming zero of
*those* touched a protected path is the range that starts immediately after the operator's own
last protected-path write, `3090b68..HEAD`:

```
$ git log --oneline 3090b68..HEAD -- SPEC.md prds ci/Dockerfile ci/SPIKE.md
$ git diff --stat 3090b68..HEAD -- SPEC.md prds ci/Dockerfile ci/SPIKE.md
```

Both print nothing: no worker commit in this run touched a protected path. Filed here as a
finding, not fixed by editing the PRD or the standing rules — the guard's literal text stands
exactly as written, and this section is the disclosure it calls for.

## 3. Gate disposition: whole-suite sweep and ten-run stability, both clean, no out-of-scope recurrence

**Whole-suite sweep (task 127, `0c022e8`)** — `ci/run.sh go test -p=1 -count=1 ./...` at final
code sha `b9243a1`. Published log path:
[`docs/reports/phase3i-127-fullsuite/sweep.log`](phase3i-127-fullsuite/sweep.log) (exit status
in [`sweep.log.exitstatus`](phase3i-127-fullsuite/sweep.log.exitstatus), README at
[`phase3i-127-fullsuite/README.md`](phase3i-127-fullsuite/README.md)). Exit status quoted
verbatim:

```
$ cat docs/reports/phase3i-127-fullsuite/sweep.log.exitstatus
0
```

All 17 package result lines `ok` or `[no test files]`; zero `FAIL` lines:

```
$ grep -a -c FAIL docs/reports/phase3i-127-fullsuite/sweep.log
0
```

**Stability gate (task 129, `3508c5a`)** — `ci/stability.sh 10` at the same sha. Published log
path: [`docs/reports/phase3i-129-stability10/summary.log`](phase3i-129-stability10/summary.log)
(exit status in
[`summary.log.exitstatus`](phase3i-129-stability10/summary.log.exitstatus), README at
[`phase3i-129-stability10/README.md`](phase3i-129-stability10/README.md)). Final tally line and
exit status quoted verbatim:

```
$ tail -1 docs/reports/phase3i-129-stability10/summary.log
10/10 passed
$ cat docs/reports/phase3i-129-stability10/summary.log.exitstatus
0
```

10/10 passed, no rounding — all 10 runs `PASS (exit 0)`; zero `FAIL` lines:

```
$ grep -a -c FAIL docs/reports/phase3i-129-stability10/summary.log
0
```

**No recurrence of any out-of-scope carried finding in either gate.** The standing rules carry
forward seven such findings: F2 (golden-frame settle, `features/golden_frame_test.go`), F20
(`status_recovery` dup-pane), F22 (`ByteArrivalPattern`, `internal/interactive`), F37
(`sort_order` latent race, `features/sort_order.feature`), F7 (quantisation collisions,
`features/color_depth_test.go`), the `features/filter.feature` dd/undo race, and the OSC 52
clipboard question. Every one of those lives inside the `features` or `internal/interactive`
package, and both gates report both packages clean throughout:

```
$ grep -a '^ok.*github.com/n-orlov/deck/features$\|^ok.*github.com/n-orlov/deck/internal/interactive$' docs/reports/phase3i-127-fullsuite/sweep.log
ok  	github.com/n-orlov/deck/features	325.934s
ok  	github.com/n-orlov/deck/internal/interactive	11.089s
```

`go test` reports a package `ok` only when every test and subtest inside it passes, so a
recurrence of any of the seven would have surfaced as one of these two lines reading `FAIL`,
not `ok`, in the sweep — and, for the stability gate, in at least one of its 10 runs. Both
report `ok`/clean; `phase3i-128-fullsuite-verbose`'s tally (319 scenarios/319 passed, 3682
steps/3682 passed) corroborates the same for the verbose companion. **This is not proof any of
the seven is fixed** — several are described elsewhere in this project's history as flakes with
a base rate below one in ten runs, or reproducible only under a forced interleaving this gate
never applies — this section is only the recurrence check the standing rules call for, and it
found nothing to report for this phase's two gates.

## 4. How to re-check every citation in this report

Every backticked sha above resolves under `git cat-file -e`; every backticked repo-relative
path names a file tracked under `git ls-files --error-unmatch`:

```
$ for sha in 6197b53 3090b68 b9243a1 0c022e8 4a9d745 3508c5a fcdb994; do \
    git cat-file -e "$sha^{commit}" && echo "$sha ok"; done
6197b53 ok
3090b68 ok
b9243a1 ok
0c022e8 ok
4a9d745 ok
3508c5a ok
fcdb994 ok
$ git ls-files --error-unmatch \
    internal/tmux/ownership.go \
    internal/tui/displacement_teardown_test.go \
    internal/tui/interactive.go \
    docs/reports/phase3i.md \
    docs/reports/phase3h-findings.md \
    docs/reports/phase3i-127-fullsuite/sweep.log \
    docs/reports/phase3i-127-fullsuite/sweep.log.exitstatus \
    docs/reports/phase3i-127-fullsuite/README.md \
    docs/reports/phase3i-128-fullsuite-verbose/verbose.log \
    docs/reports/phase3i-129-stability10/summary.log \
    docs/reports/phase3i-129-stability10/summary.log.exitstatus \
    docs/reports/phase3i-129-stability10/README.md
```

All resolve. `prds/phase3i-force-attach.md`, `SPEC.md` and the other protected paths named in
[§2](#2-the-protected-path-audit-range-6197b53head-necessarily-contains-one-operator-commit)
are quoted there, never edited — nothing in this report writes to any of them.
