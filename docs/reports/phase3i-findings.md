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
- [5. R100's literal "including its set shape" wording disagrees with SPEC 11.9's plain unset -- SPEC wins, pinned by task 203](#5-r100s-literal-including-its-set-shape-wording-disagrees-with-spec-119s-plain-unset----spec-wins-pinned-by-task-203)
- [6. How to re-check every citation in this report](#6-how-to-re-check-every-citation-in-this-report)

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

## 5. R100's literal "including its set shape" wording disagrees with SPEC 11.9's plain unset -- SPEC wins, pinned by task 203

PRD `phase3i-force-attach.md`'s R100 ("the window's original geometry outlives an arbitrary
chain of steals") ends its own unit-evidence bullet with a byte-exact-restore promise:

```
$ sed -n '145,148p' prds/phase3i-force-attach.md
- Unit evidence in `internal/tmux` and `internal/tui`: two sequential steals of one window
  restore the pre-first-entry size byte-exactly (width, height, and the `window-size` value
  including its unset shape); a stolen-from holder's teardown issues zero `resize-window`; a
  reclaim over a stolen claim touches neither option.
```

Read literally and generally -- "restore ... byte-exactly ... the `window-size` value
[including its shape, whether that shape was set or unset]" -- that clause asks for a
value-preserving restore: if a window's `window-size` was window-locally SET to a specific
value before deck's first claim on it, R100 would have the exit recipe put that same set value
back, not merely leave the option unset. This is the reading `internal/tmux/restore_plain_unset_test.go`
names explicitly in its own doc comment (`c025c54`..`a559e7c`'s ancestor commit range is not
needed here; the comment is quoted verbatim): "It diverges from a LITERAL reading of PRD
phase3i-force-attach.md's R100, which asks two sequential steals to 'restore the pre-first-entry
size byte-exactly (width, height, and the window-size value including its set shape)' -- read as
a general byte-exact-restore promise, that clause would require this test's pre-entry SET value
to come back after RestoreWindowGeometry, not merely to end up unset."

**SPEC.md 11.9 and PRD phase3b II-9 both name a plain unset, unconditionally, as the exit
recipe's last step -- never a value-preserving restore.** The product follows SPEC. The
committed code, `internal/tmux/geometry.go`, confirms it: `RestoreWindowGeometry`'s only
window-size-restoring call is unconditional and last,

```go
// internal/tmux/geometry.go:243
	return c.unsetWindowSize(ctx, target)
```

and `unsetWindowSize` (`geometry.go:201`) issues `set-option -w -u window-size` and nothing
else -- it never reads `WindowGeometry.WindowSizeValue` at all, so there is nothing in the
committed code path that could write a captured set value back. Per the standing rule ("Where
the PRD and SPEC disagree, SPEC wins"), this is deliberate: the fix is not to make
`RestoreWindowGeometry` value-preserving, and this report does not ask for one.

**Evidence -- task 203, commit `a559e7c61a00ab5fa31c2d98eaf5ce787744e4dc`** (`tmux: pin
RestoreWindowGeometry's plain-unset shape against a set window-size (task 203)`), adds
`internal/tmux/restore_plain_unset_test.go`. Against a real tmux server on a private socket, it
sets `window-size` window-locally to `"manual"` BEFORE `CaptureWindowGeometry` runs (so the
captured geometry has `WindowSizeSet == true`, `WindowSizeValue == "manual"`), calls
`RestoreWindowGeometry` with zero attached clients, and asserts the window-local `window-size`
option reads back UNSET afterwards -- the pre-entry SET value is deliberately not restored.
The test's own failure message, quoted verbatim, names the authority and the diverging clause
together: "...per SPEC.md §11.9 / PRD phase3b II-9's plain-unset recipe, not R100's literal
\"including its set shape\" wording".

**`internal/tui/double_steal_restore_test.go` (task 114, commit
`cb27c5c0630a9c8f9a73ebc6c28abcea3246bf85`) proves byte-exact restoration only for the UNSET
shape, not the set one.** Its own pre-entry capture asserts `WindowSizeSet` is false --
"Capture the window's pre-first-entry geometry -- width, height and the window-size read,
INCLUDING its unset shape -- before any deck entry has touched the window at all" -- and then
fails the test outright if that assumption does not hold
(`if preEntry.WindowSizeSet { t.Fatalf("test assumption violated...") }`). Two sequential
steals and a final legitimate exit are then shown to restore that pre-entry state
byte-exactly. That test's own scope is the unset shape; it says nothing about a pre-entry SET
value, and is not evidence for a value-preserving restore -- task 203's test is the one that
covers the SET case, and it shows the opposite of a literal R100 reading.

**Disposition.** No task in this plan makes `RestoreWindowGeometry` value-preserving, and none
should: SPEC 11.9's plain unset is the authority, task 203's real-tmux test pins the product's
actual (plain-unset) behaviour against a pre-entry SET value, and this section is the disclosure
the standing rules call for when SPEC and the PRD disagree, filed rather than fixed by editing
either protected document.

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
    docs/reports/phase3i-129-stability10/README.md
```

All resolve. `prds/phase3i-force-attach.md`, `SPEC.md` and the other protected paths named in
[§2](#2-the-protected-path-audit-range-6197b53head-necessarily-contains-one-operator-commit)
are quoted there, never edited — nothing in this report writes to any of them.
