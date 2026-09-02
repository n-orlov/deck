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
- [4. The shipped helpText's `F` entry wrongly told users a live claim holder still refuses `F` -- corrected by task 201, guarded by task 202](#4-the-shipped-helptexts-f-entry-wrongly-told-users-a-live-claim-holder-still-refuses-f----corrected-by-task-201-guarded-by-task-202)
- [5. Correction: R100's actual wording ('including its unset shape') agrees with SPEC 11.9 -- there is no R100/SPEC disagreement, retracting the claim task 208 published](#5-correction-r100s-actual-wording-including-its-unset-shape-agrees-with-spec-119----there-is-no-r100spec-disagreement-retracting-the-claim-task-208-published)
- [6. How to re-check every citation in this report](#6-how-to-re-check-every-citation-in-this-report)
- [7. Approach 3's gate disposition at the final code sha `a559e7c`: whole-suite sweep, verbose companion and stability-10, all clean, no out-of-scope recurrence](#7-approach-3s-gate-disposition-at-the-final-code-sha-a559e7c-whole-suite-sweep-verbose-companion-and-stability-10-all-clean-no-out-of-scope-recurrence)

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

    GH #19, against the §11.9 amendment landed in 6197b53. Six requirements:
    R98 binds F and makes the two contention refusals advertise it; R99 adds
    the single-shot force claim (write over anything, keep the confirm-read, so
    exactly one winner and every loser stands down with no error); R100 puts
    the window's ORIGINAL geometry in a second window option so it survives an
    arbitrary chain of steals and only the last legitimate holder restores it;
    R101 gives the displaced client the lost-attach dialog and stops its
    keyboard; R102 makes passive previewFit stand down under a foreign live
    claim (a pre-existing gap — a second deck merely SELECTING a row resizes a
    window the first is typing into); R103 is the record.

    The phase is deliberately small and says so. Most of the mechanism already
    exists — the claim protocol is already write-then-confirm-read, Release is
    already steal-safe, the pipe transport already reports displacement — and
    the PRD names each of those so the run extends rather than rebuilds.

    Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>

 prds/phase3i-force-attach.md | 302 +++++++++++++++++++++++++++++++++++++++++++
 1 file changed, 302 insertions(+)
```

The diffstat's last two lines confirm it touches exactly one file, adding it whole (`302
insertions(+)`, zero deletions, zero other files) — `prds/phase3i-force-attach.md`, the PRD file
this very phase's plan was cut from.

**The PRD's own zero-commit clause, quoted verbatim (`prds/phase3i-force-attach.md:33-34`):**

> The audit range for this run is `6197b53..HEAD` and it must show **zero** protected-path
> commits.

Read literally, that clause is **UNMET**: the range is non-empty, as shown above, and it can
never become empty without doing one of two things, both of which are themselves forbidden —
(a) an operator amendment narrowing the clause to name a *different* range (e.g. one starting
after the operator's own last protected-path write), which only the operator can make, this job
cannot license itself, and no task in this plan has done; or (b) a history rewrite that drops or
moves `3090b68` out of `6197b53..HEAD`, which the standing rules forbid outright ("Never
force-push, never rewrite history"). Neither has happened, so the clause stands UNMET exactly as
written, and this section declines to try either forbidden route to make it read otherwise.

This is the same shape phase 3h's findings report disclosed for its own audit range
(`de90a5c..HEAD` necessarily containing the operator's own plan commit `a24ff8d`): whichever
commit opens a protected-path audit range, if the operator's own next act is to add the PRD (or
amend the spec) that names that range, the range will contain that commit by construction. This
run's own worker task commits are the ones the guard actually polices; confirming zero of
*those* touched a protected path is the range that starts immediately after the operator's own
last protected-path write, `3090b68..HEAD`:

```
$ git log --oneline 3090b68..HEAD -- SPEC.md prds ci/Dockerfile ci/SPIKE.md
$ git diff --name-only 3090b68..HEAD -- SPEC.md prds ci/Dockerfile ci/SPIKE.md
```

Both print nothing: no worker commit in this run touched a protected path — the worker-write
range is empty even though the PRD's own literal range is not. Filed here as a finding, not
fixed by editing the PRD or the standing rules — the guard's literal text stands
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

## 4. The shipped helpText's `F` entry wrongly told users a live claim holder still refuses `F` -- corrected by task 201, guarded by task 202

This is approach 2's own finding, not approach 1's: the code review that opened approach 2
found the `F` entry in `internal/tui/tui.go`'s `helpText` naming a refusal that R98/SPEC 11.9
says `F` exists to skip.

**What was shipped, quoted verbatim from the pre-fix source (task 201's diff shows the exact
removed lines):**

```
F force-enter interactive mode on the selected session, stealing it from
  any client already attached to it -- the one refusal Enter itself still
  respects that F exists to skip; every other refusal Enter has (the 7-row
  floor, no-width squeeze, a stopped session, a live process already
  holding the window's own claim) still applies
```

That text told the user `F` skips only the attached-client refusal, and that "a live process
already holding the window's own claim" is one of the refusals `F` still respects -- the
opposite of what R98/SPEC 11.9 specify. **R98** (`prds/phase3i-force-attach.md`) states `F`
differs from `Enter` in exactly two ways, both named in SPEC 11.9: "it does not refuse for
`SessionAttachedCount > 0`, and it takes ownership over a live holder (R99) instead of standing
down", and that the attached-client refusal and the **live-ownership refusal** are R98's two
named contention refusals `F` must skip. **SPEC.md 11.9** is the requirement's own text: an
owner "has recorded a size it is going to put back", and R98's force variant is specified to
take that claim rather than stand down for it -- a live holder is exactly the case `F` is built
to override, not one it still refuses on.

**Correction -- task 201, commit `c025c54451e816eadc3f84063321814c118a8e76`** (`tui: correct
helpText's F entry to match SPEC 11.9's two skipped refusals (task 201)`), in
`internal/tui/tui.go`. The corrected entry:

```
F force-enter interactive mode on the selected session, stealing it from
  any client already attached to it and claiming ownership over a
  live holder of the window's claim instead of standing down for one --
  the two refusals Enter itself still respects that F exists to skip; every
  other refusal Enter has (the 7-row floor, no-width squeeze, a stopped
  session, no live pane) still applies
```

Now names both refusals `F` skips (the attached-client refusal, and a live holder of the
window's ownership claim) and still names the refusals that do apply to `F` (the 7-row inner
floor, the no-width squeeze, a stopped session, no live pane) -- agreeing with R98/SPEC 11.9.
Post-fix, `grep -n "live process already" internal/tui/tui.go` prints nothing.

**Guard -- task 202, commit `5b554f2632c6a085e0da2d40a5fcab75bf759f06`** (`tui: pin helpText's F
entry wording to the force path's real semantics (task 202)`), adds
`internal/tui/help_force_semantics_test.go`. The test reads `tui.go`'s `helpText` `F` entry as
source text and fails if it says a live-ownership/claim refusal applies to `F` while
`internal/tui/interactive.go`'s force path calls `ForceClaimWindowOwnership` -- tying the
wording assertion to the actual force-path call, not to a copy of the old prose. Task 202's own
record states the pre-201 wording was restored against the test and made it fail, then
reverted, before the guard was committed; the bad wording is not left in the tree.

**Disposition.** Both commits are `validated` (tasks 201 and 202 in `tasks.json`); this defect
is fixed and pinned, not merely reported. Filed here because a wrong help entry that
contradicts the requirement it documents is exactly the shape of defect this report exists to
disclose, per the standing rules' instruction that a correction be recorded with the commit
that fixed it and the test that keeps it fixed.

## 5. Correction: R100's actual wording ("including its unset shape") agrees with SPEC 11.9 -- there is no R100/SPEC disagreement, retracting the claim task 208 published

**This section corrects an earlier claim in this same report, published by task 208 (commit
`ae35983`).** That version of this section quoted R100's unit-evidence bullet as ending
"including its **set** shape" and built an entire finding on that misquote: a claimed
R100-versus-SPEC-11.9 disagreement over whether `RestoreWindowGeometry` must value-preserve a
pre-entry SET `window-size`. Re-reading the PRD's actual text shows that quote was wrong, and
the disagreement it described does not exist. This section retracts it.

The PRD's own words, quoted fresh rather than carried forward from task 208's version:

```
$ sed -n '145,148p' prds/phase3i-force-attach.md
- Unit evidence in `internal/tmux` and `internal/tui`: two sequential steals of one window
  restore the pre-first-entry size byte-exactly (width, height, and the `window-size` value
  including its unset shape); a stolen-from holder's teardown issues zero `resize-window`; a
  reclaim over a stolen claim touches neither option.
```

R100 reads "including its **unset** shape", not "set shape". Read as written, that clause asks
the two-sequential-steals unit evidence to restore the pre-first-entry `window-size` value
**in its unset shape** -- i.e. to end up unset, exactly what an unconditional unset delivers --
not to value-preserve a pre-entry SET value. There is nothing in R100's actual text for SPEC
11.9's plain unset to conflict with.

**SPEC.md 11.9 specifies an unconditional plain unset as the exit recipe's last step, and R100's
actual wording asks for exactly that.** The committed code, `internal/tmux/geometry.go`, agrees
with both: `RestoreWindowGeometry`'s only window-size-restoring call is unconditional and last,

```go
// internal/tmux/geometry.go:243
	return c.unsetWindowSize(ctx, target)
```

and `unsetWindowSize` (`geometry.go:201`) issues `set-option -w -u window-size` and nothing
else -- it never reads `WindowGeometry.WindowSizeValue` at all. R100's own wording, SPEC 11.9,
and the shipped, unconditional `unsetWindowSize` all point the same direction: an unset shape,
unconditionally, every time. Per the standing rules ("do not invent a disagreement either:
check the quoted PRD line with `sed -n` before asserting one"), inventing one here was the
error this section corrects, not a real conflict this report needs to resolve.

**Disclosed residual: `internal/tmux/restore_plain_unset_test.go`'s doc comment still repeats
the same "including its set shape" misquote task 208 made, and is deliberately left unedited.**
That file's comments (lines 26 and 34, and the test's own failure-message string) quote R100 as
ending "including its set shape", the same wrong reading this section retracts. Approach 3
lands zero `*.go`/`*.feature` changes (the standing rules freeze the final code sha at
`a559e7c61a00ab5fa31c2d98eaf5ce787744e4dc`, validated by a 10/10 stability gate at that exact
sha); a comment-only edit to that test file would still move the file's git blob and the tree's
sha, invalidating both the whole-suite sweep and the stability gate's citation of that sha. The
misquote therefore stays in the test file's comments as a known, disclosed inaccuracy -- it does
not affect the test's behaviour (the test asserts against the real unset outcome, not against
its own comment text) -- rather than triggering a code change this phase is scoped to avoid.

**Disposition.** There is no R100/SPEC 11.9 disagreement over `window-size`: R100's actual
wording, SPEC 11.9's plain-unset recipe, and the shipped unconditional `unsetWindowSize` all
agree. Task 208's claim of a disagreement is retracted by this section. The only residual is
the cosmetic misquote in `restore_plain_unset_test.go`'s comments, disclosed above and left
unedited to avoid moving the frozen final code sha.

## 6. How to re-check every citation in this report

Every backticked sha above resolves under `git cat-file -e`; every backticked repo-relative
path names a file tracked under `git ls-files --error-unmatch`:

```
$ for sha in 6197b53 3090b68 b9243a1 0c022e8 4a9d745 3508c5a fcdb994 c025c54 5b554f2 a559e7c cb27c5c; do \
    git cat-file -e "$sha^{commit}" && echo "$sha ok"; done
6197b53 ok
3090b68 ok
b9243a1 ok
0c022e8 ok
4a9d745 ok
3508c5a ok
fcdb994 ok
c025c54 ok
5b554f2 ok
a559e7c ok
cb27c5c ok
$ git ls-files --error-unmatch \
    internal/tmux/ownership.go \
    internal/tui/displacement_teardown_test.go \
    internal/tui/interactive.go \
    internal/tui/tui.go \
    internal/tui/help_force_semantics_test.go \
    internal/tmux/geometry.go \
    internal/tmux/restore_plain_unset_test.go \
    internal/tui/double_steal_restore_test.go \
    prds/phase3i-force-attach.md \
    SPEC.md \
    docs/reports/phase3i.md \
    docs/reports/phase3h-findings.md \
    docs/reports/phase3i-127-fullsuite/sweep.log \
    docs/reports/phase3i-127-fullsuite/sweep.log.exitstatus \
    docs/reports/phase3i-127-fullsuite/README.md \
    docs/reports/phase3i-128-fullsuite-verbose/verbose.log \
    docs/reports/phase3i-129-stability10/summary.log \
    docs/reports/phase3i-129-stability10/summary.log.exitstatus \
    docs/reports/phase3i-129-stability10/README.md \
    docs/reports/phase3i-302-fullsuite/sweep.log \
    docs/reports/phase3i-302-fullsuite/sweep.log.exitstatus \
    docs/reports/phase3i-302-fullsuite/README.md \
    docs/reports/phase3i-303-fullsuite-verbose/verbose.log \
    docs/reports/phase3i-303-fullsuite-verbose/verbose.log.exitstatus \
    docs/reports/phase3i-303-fullsuite-verbose/README.md \
    docs/reports/phase3i-206-stability10/summary.log \
    docs/reports/phase3i-206-stability10/summary.log.exitstatus \
    docs/reports/phase3i-206-stability10/README.md
```

All resolve. `prds/phase3i-force-attach.md`, `SPEC.md` and the other protected paths named in
[§2](#2-the-protected-path-audit-range-6197b53head-necessarily-contains-one-operator-commit)
are quoted there, never edited — nothing in this report writes to any of them.

## 7. Approach 3's gate disposition at the final code sha `a559e7c`: whole-suite sweep, verbose companion and stability-10, all clean, no out-of-scope recurrence

Approach 3 lands zero `*.go`/`*.feature` changes (standing rules); the final code sha stays
frozen at `a559e7c61a00ab5fa31c2d98eaf5ce787744e4dc`, confirmed fresh against this report's own
tree:

```
$ git log -1 --format=%H -- '*.go' '*.feature'
a559e7c61a00ab5fa31c2d98eaf5ce787744e4dc
```

The three gate artifacts this section reports are, exactly:
[`docs/reports/phase3i-302-fullsuite/`](phase3i-302-fullsuite/),
[`docs/reports/phase3i-303-fullsuite-verbose/`](phase3i-303-fullsuite-verbose/) and
[`docs/reports/phase3i-206-stability10/`](phase3i-206-stability10/) -- each path passes
`git ls-files --error-unmatch` (checked in [§6](#6-how-to-re-check-every-citation-in-this-report)'s
citation list, which this section's paths have been added to).

### Whole-suite sweep (task 302, republished `7db4756`)

`ci/run.sh go test -p=1 -count=1 ./...` at `a559e7c`. Exit status, quoted verbatim from
`sweep.log.exitstatus`:

```
$ cat docs/reports/phase3i-302-fullsuite/sweep.log.exitstatus
0
```

All 17 package result lines `ok` or `[no test files]`, zero `FAIL`:

```
$ grep -a -c FAIL docs/reports/phase3i-302-fullsuite/sweep.log
0
```

### Verbose companion (task 303, `d523374` + fix `7206877`)

`ci/run.sh go test -p=1 -count=1 -v ./...` at the same sha. Exit status, quoted verbatim from
`verbose.log.exitstatus`:

```
$ cat docs/reports/phase3i-303-fullsuite-verbose/verbose.log.exitstatus
0
```

The deliverable Gherkin tally (phase 3g finding F34), lines 5642-5643 of `verbose.log`, spliced
straight from the log's own bytes -- ANSI SGR escape sequences included, exactly as task 303's
README does it, so a byte-exact search for these two lines finds them in both documents:

```
5642:319 scenarios ([32m319 passed[0m)
5643:3682 steps ([32m3682 passed[0m)
```

The same two lines through `cat -v` (renders each `ESC` byte as `^[`), for readers whose viewer
swallows control bytes:

```
5642:319 scenarios (^[[32m319 passed^[[0m)
5643:3682 steps (^[[32m3682 passed^[[0m)
```

**319 scenarios (319 passed) / 3682 steps (3682 passed)** -- every scenario and every step in
the real suite passed; none pending, skipped, undefined or failed. (The two deliberately-failed
and deliberately-undefined fixture tallies at lines 5977-5978 and 5984-5985, from
`TestGodogRejectsUndefinedAndFailedSteps`, are not part of this real-suite figure -- see task
303's own README for that distinction; this section repeats only the deliverable tally the
standing rules ask for.)

### Stability gate (task 206, `3508c5a`) -- NOT re-measured in approach 3

**Approach 3 does not re-run `ci/stability.sh 10`.** The gate stays task 206's 10/10 at the
frozen final code sha, because approach 3 lands no code change to re-measure against: the
range from that final code sha to this report's own tree is empty over the paths the gate
exists to cover:

```
$ git log --oneline a559e7c..HEAD -- '*.go' '*.feature'
```

prints nothing. A gate that certifies a code state stays valid for that code state as long as
the code state itself has not moved; it has not.

Final tally line and exit status, quoted verbatim:

```
$ tail -1 docs/reports/phase3i-206-stability10/summary.log
10/10 passed
$ cat docs/reports/phase3i-206-stability10/summary.log.exitstatus
0
```

10/10 passed, no rounding -- all 10 runs `PASS (exit 0)`; zero `FAIL` lines across all ten
per-run logs (task 206's own README's recurrence-check grep, reproduced there).

### Task 204's `phase3i-204-fullsuite` is superseded by 302, not a live requirement

Approach 2's task 204 ("Run and publish the whole-suite sweep at the new final code sha") is
`failed` (`validation-exhausted`) in the approach 2 task record
(`/run/ralphd/approaches/02/tasks.json`) -- not tracked in this run's own `tasks.json`, which
carries only approach 3's tasks 301-309, but the source of record for what approach 3 inherits.
Its artifact directory, `docs/reports/phase3i-204-fullsuite/`, published a sweep with the same
`0` exit status and the same 17-clean-lines shape as task 302's, but polled the run with an
inconsistent `sleep 60`/`sleep 180` mix rather than `sleep 60` throughout -- the discipline gap
task 302 exists to fix (task 302's own README: "This supersedes task 204's
`phase3i-204-fullsuite` ... this run polled with **`sleep 60` and no other duration**,
throughout"). **`docs/reports/phase3i-302-fullsuite/` supersedes `docs/reports/phase3i-204-fullsuite/`
as the current whole-suite-sweep gate; no requirement in [`phase3i.md`](phase3i.md) or this
report rests on task 204 or its artifact.** Per the standing rules, a terminally `failed` task's
artifact may be named only as superseded or stale, never as a requirement's discharge -- this
paragraph is that disclosure, not a claim that 204's own criteria were met.

### No recurrence of any out-of-scope carried finding in the whole-suite sweep

The seven out-of-scope findings carried forward by the standing rules -- F2 (golden-frame
settle), F20 (`status_recovery` dup-pane), F22 (`ByteArrivalPattern`), F37 (`sort_order` latent
race), F7 (quantisation collisions), the `features/filter.feature` dd/undo race, and the OSC 52
clipboard question -- were checked by name against task 302's own sweep log:

```
$ grep -ilE 'FAIL|panic|race detected' docs/reports/phase3i-302-fullsuite/sweep.log
(no matches, exit 1)
$ grep -iE 'F2\b|F20\b|F22\b|F37\b|F7\b|golden.frame|status_recovery|ByteArrivalPattern|sort_order|filter\.feature|osc.?52|clipboard' docs/reports/phase3i-302-fullsuite/sweep.log
(no matches, exit 1)
```

Both searches over `sweep.log` return no matches. The two packages every one of the seven
findings lives inside -- `features` and `internal/interactive` -- both report `ok` in the same
sweep:

```
$ grep -a 'features\|interactive' docs/reports/phase3i-302-fullsuite/sweep.log
ok  	github.com/n-orlov/deck/features	329.502s
ok  	github.com/n-orlov/deck/internal/interactive	11.674s
```

`go test` reports a package `ok` only when every test and subtest inside it passes, so a
recurrence of any of the seven would have surfaced as `FAIL` on one of those two lines, not
`ok` -- it did not. **This is not proof any of the seven is fixed** -- several are documented
elsewhere in this project's history as flakes with a base rate below one in ten runs, or
reproducible only under a forced interleaving no gate here applies -- this section is only the
by-name recurrence check the standing rules call for against approach 3's own sweep, and it
found nothing to report.

### Disposition

At the frozen final code sha `a559e7c`, the whole-suite sweep (task 302) and its verbose
companion (task 303) are both fresh republications with clean `0` exits and a full 319/3682
Gherkin tally; the stability gate (task 206) is not re-measured because no code moved since it
was taken, and its 10/10 stays the current, valid gate. Task 204's own sweep artifact is
superseded and carries no requirement. None of the seven out-of-scope carried findings recurred
in task 302's sweep log by name.

## 8. SPEC §11.3's footer fixed-set sentence and PRD R98's footer wording disagree over `F` -- SPEC wins, cured by tasks 401 and 402

This is approach 4's own finding, raised by the instruction
`prds/phase3i-force-attach.md` gives its own readers: "Where this PRD and `SPEC.md` disagree,
`SPEC.md` wins and the disagreement is a finding for `docs/reports/phase3i-findings.md`, never
an edit." `internal/tui/tui.go`'s `footerLegend` carried an `F` row (glyph `F`, verb `force`)
that SPEC §11.3's curated fixed-set sentence does not list -- the disagreement this section
records.

**SPEC §11.3's footer fixed-set sentence, quoted verbatim (`SPEC.md`):**

> **The footer's fixed set is curated for the keys worth a whole line of the frame.** It
> carries navigation, `↵`, `a`, `Y`, `n`, `x`, `r`, `R`, `dd`, the eligible one of `A`/`U`,
> `,`, `i`, `?` and `q`.

That sentence names an exhaustive, curated list -- fourteen entries by name, no `F` among
them -- and the same bullet goes on to say rarely-used per-row actions "stay bound, stay in
the `?` overlay and in §11's keymap, and stay out of the footer: one line is a budget", i.e.
absence from the footer's fixed set is a deliberate curation choice, not an oversight to be
closed by adding more glyphs.

**PRD R98's footer sentence, quoted from `prds/phase3i-force-attach.md`'s own R98 bullet list,
verbatim but for one marked elision:**

> Help and footer: `F` appears wherever `↵`/`a` do, so [... three parity tests, named in R98
> by bare basename, elided here ...] all agree with the amended §11.3 keymap.

The elision stands in for the three test files R98 names by bare basename. This report cites
paths only in the form that resolves as written, so those three are given here in this
report's own voice, repo-relative: `internal/tui/help_keymap_parity_test.go`,
`internal/tui/footer_bindings_parity_test.go` and
`internal/tui/footer_handler_agreement_test.go`. Nothing in the quoted clause that carries
the disagreement is elided: the words `F` appears wherever `↵`/`a` do stand exactly as R98
writes them.

Read literally, R98 asks `F` to appear in the footer everywhere `↵`/`a` do, since both of
those are in SPEC §11.3's fixed set -- directly contradicting §11.3's own curated list, which
names fourteen keys and not `F`.

**SPEC wins, per the PRD's own authority rule quoted above** ("Where this PRD and `SPEC.md`
disagree, `SPEC.md` wins"). `F` leaves `footerLegend` -- the footer's curated fixed set --
while staying bound in the bare-key switch, staying in
`internal/tui/help_keymap_parity_test.go`'s keymap parity and staying in the `?` overlay's
`helpText`, exactly as §11.3 prescribes for a rarely-used per-row action kept out of the
footer.

**The cure -- tasks 401 and 402:**

- **Task 401, commit `96bff5682571511a3cc63b1065d593fe64440ae9`**
  (`footer: drop F from footerLegend per SPEC §11.3 curated fixed set (task 401)`) removed the
  `{"F", "F", "force", ...}` row from `footerLegend` in `internal/tui/tui.go` and its dead
  completeness pair (`glyph: "F"`) from `footerAgreementKeys` in
  `internal/tui/footer_handler_agreement_test.go`. `F` stays bound (`case "F":` in
  `internal/tui/tui.go` still present once) and stays in `helpText` (`F force-enter
  interactive mode` still present once) and in the keymap and the `?` overlay, untouched.
- **Task 402, commit `3b70bfbc7e3552ff375ae675af117805a1eee944`**
  (`tui: pin footerLegend's glyph set as a closed list against SPEC §11.3 (task 402)`) added
  `TestFooterLegendGlyphSetIsClosedAgainstSpec` to
  `internal/tui/footer_bindings_parity_test.go`, asserting `footerLegend`'s parsed glyph set
  equals `specFooterFixedSetGlyphs(t)`'s SPEC-parsed set, both directions -- a closed-list
  guard against SPEC §11.3's sentence, not merely a subset check. Demonstrated load-bearing
  per task 402's own record: temporarily restoring the `F` row in `footerLegend` and re-running
  the new test alone produced a failure naming the file (`internal/tui/tui.go`, which the
  message itself abbreviates to its basename) and then, verbatim, `footerLegend has glyph "F"
  that SPEC.md §11.3's footer fixed-set sentence does not name`; the restoration was reverted
  before the commit.

`F` therefore now agrees with SPEC §11.3 exactly: out of the footer's curated fixed set, still
bound, still in `internal/tui`'s keymap, and still in the `?` overlay's help text -- and task
402's guard keeps `footerLegend` and SPEC §11.3 from drifting apart again.
