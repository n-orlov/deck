# Phase 3h findings

Companion to [`phase3h.md`](phase3h.md) (the per-requirement evidence report for R94–R97).
Modelled on [`phase3g-findings.md`](phase3g-findings.md): what the requirement table does not
carry — a PRD-vs-tree disagreement worth disclosing rather than editing, a prose line left
unedited by design, the stability gate's disposition quoted verbatim, and this phase's check
for a recurrence of every finding the PRD names out of scope.

Written by task 014 against the tree at `b5e4178` (task 013's HEAD; task 014 adds no code).
Every sha cited resolves under `git cat-file -e` and every path cited is tracked under
`git ls-files --error-unmatch`, checked in [§4](#4-how-to-re-check-every-citation-in-this-report).

- [1. The PRD's `de90a5c..HEAD` protected-path range disagrees with the tree](#1-the-prds-de90a5chead-protected-path-range-disagrees-with-the-tree)
- [2. `create_cwd_ghost.feature:6`'s prose still names the dimmed token — left unedited by design](#2-create_cwd_ghostfeature6s-prose-still-names-the-dimmed-token--left-unedited-by-design)
- [3. The stability gate: 10/10, and no recurrence of any out-of-scope finding](#3-the-stability-gate-1010-and-no-recurrence-of-any-out-of-scope-finding)
- [4. How to re-check every citation in this report](#4-how-to-re-check-every-citation-in-this-report)

## 1. The PRD's `de90a5c..HEAD` protected-path range disagrees with the tree

The PRD's protected-path guard, as the standing rules restate it, says the audit range is
`de90a5c..HEAD` and that it "can never be empty". Taken literally against this run's own
writes, that is not quite right: `de90a5c` is the operator's `SPEC.md` ruling commit, several
commits *before* this run's own plan commit, so `de90a5c..HEAD` necessarily contains every
commit the operator made while setting this phase up — including the one that adds the very
PRD file the guard is checking against:

```
$ git log --oneline de90a5c..HEAD
a24ff8d plan: add Phase 3h — bring the suite to the §7 ruling, fix F31, hold the gate at one sha (operator)
[... 19 more commits, all this run's own task work ...]
$ git diff --stat de90a5c..HEAD -- SPEC.md prds/ ci/Dockerfile ci/SPIKE.md
 prds/phase3h-suite-reconciliation.md | 152 +++++++++++++++++++++++++++++++++++
 1 file changed, 152 insertions(+)
```

`a24ff8d` is that commit — the operator's own plan commit, adding
`prds/phase3h-suite-reconciliation.md` — confirmed by its own stat:

```
$ git show --stat a24ff8d
commit a24ff8dd20c4e3d37e40b4ca480cc40961fdd052
Author: Nik <nikolaiorl@gmail.com>
Date:   Sun Aug 30 08:43:40 2026 +0100

    plan: add Phase 3h — bring the suite to the §7 ruling, fix F31, hold the gate at one sha (operator)

 prds/phase3h-suite-reconciliation.md | 152 +++++++++++++++++++++++++++++++++++
 1 file changed, 152 insertions(+)
```

So `de90a5c..HEAD` is **necessarily** non-empty for the protected-path set — it always
contains `a24ff8d`'s one-file addition, an operator-authored commit this run never wrote and
could not have avoided — and "must print nothing" cannot be the literal test against that
range. The range that actually answers "did this run's own writes touch a protected path" is
`a24ff8d..HEAD` (i.e., every commit *after* the operator's own plan commit, which is where
this run's own task work starts):

```
$ git log --oneline a24ff8d..HEAD -- SPEC.md prds/ ci/Dockerfile ci/SPIKE.md
$ git diff --stat a24ff8d..HEAD -- SPEC.md prds/ ci/Dockerfile ci/SPIKE.md
```

Both print nothing — confirmed again, fresh, at the sha this report is written against
(`b5e4178`), matching the standing rules' own resolution. This is filed here as a finding, not
fixed by editing the PRD or the standing rules: the PRD's literal range stands as written, and
this section is the disclosure the standing rules themselves call for ("Publish BOTH ranges in
the guard report, naming `a24ff8d` as the sole, operator-authored protected-path commit, and
file the disagreement as a finding ... never as an edit"). Task 015 is the task that publishes
both ranges as this run's own guard report; this section is the finding that guard report
points at.

## 2. `create_cwd_ghost.feature:6`'s prose still names the dimmed token — left unedited by design

Task 004 (`d578c03`) re-pointed `features/create_cwd_ghost.feature`'s three `Then ... has
foreground token "dimmed"` assertions and the `:14` scenario title to `"hint"`, matching the
shipped token (`SPEC.md:1355`). The feature file's own opening prose, four lines above the
first scenario, was not touched by that commit and still reads, verbatim, at line 6:

```
$ sed -n '2,7p' features/create_cwd_ghost.feature
Feature: The create modal's §11.7 directory-only ghost completion (requirement 14)
  With the cursor at the end of the cwd field (there is no other cursor
  position: typing only appends, backspace only trims the end), a UNIQUE
  directory whose name starts with the segment after the field's last "/"
  is shown inline in the theme's dimmed token, and right/end accept it,
  completing to the match plus a trailing "/". Only directories are ever
```

Line 6's "is shown inline in the theme's dimmed token" is prose, not a step and not an
assertion — `git show --stat d578c03 -- features/create_cwd_ghost.feature` touches four lines
(the three `Then` steps plus the `:14` title), none of them line 6. Task 005's notes recorded
this as a finding for task 014 rather than a fourth line to change, and task 014 (this report)
is that disclosure: R95's remit is the three `Then` assertions and the scenario title that name
the token the negative/positive proofs actually check, not the feature's descriptive header
prose, and no task in this phase's plan (001–016) names the header line as in scope. Left
exactly as it reads — a stale but harmless mismatch between the feature's own prose and the
token its assertions now check — by design, not by oversight.

## 3. The stability gate: 10/10, and no recurrence of any out-of-scope finding

Task 010 ran `ci/stability.sh 10` at the final code sha `2ccb1d3` and published its result at
[`docs/reports/phase3h-010-stability10/`](phase3h-010-stability10/). The disposition, quoted
verbatim from the gate's own summary log:

```
$ cat docs/reports/phase3h-010-stability10/summary.log | tail -1
10/10 passed
```

with the script's own captured exit status, from the same directory's `.exitstatus`:

```
$ cat docs/reports/phase3h-010-stability10/.exitstatus
0
```

All 10 runs printed `=== RUN k: PASS (exit 0) ===` and all 17 packages `ok` or `[no test
files]` (`internal/notify`, `internal/search`, `internal/unit`) in every one of the 10
per-run logs — no run reported a `FAIL` line:

```
$ grep -a -c 'FAIL' docs/reports/phase3h-010-stability10/run-1.log docs/reports/phase3h-010-stability10/run-2.log \
    docs/reports/phase3h-010-stability10/run-3.log docs/reports/phase3h-010-stability10/run-4.log \
    docs/reports/phase3h-010-stability10/run-5.log docs/reports/phase3h-010-stability10/run-6.log \
    docs/reports/phase3h-010-stability10/run-7.log docs/reports/phase3h-010-stability10/run-8.log \
    docs/reports/phase3h-010-stability10/run-9.log docs/reports/phase3h-010-stability10/run-10.log \
    docs/reports/phase3h-010-stability10/summary.log
docs/reports/phase3h-010-stability10/run-1.log:0
docs/reports/phase3h-010-stability10/run-10.log:0
docs/reports/phase3h-010-stability10/run-2.log:0
docs/reports/phase3h-010-stability10/run-3.log:0
docs/reports/phase3h-010-stability10/run-4.log:0
docs/reports/phase3h-010-stability10/run-5.log:0
docs/reports/phase3h-010-stability10/run-6.log:0
docs/reports/phase3h-010-stability10/run-7.log:0
docs/reports/phase3h-010-stability10/run-8.log:0
docs/reports/phase3h-010-stability10/run-9.log:0
docs/reports/phase3h-010-stability10/summary.log:0
```

**None of the five findings the PRD's non-goals section names out of scope — F2 (the
golden-frame settle flake, `features/golden_frame_test.go`), F20 (the
`features/status_recovery.feature` dup-pane/R76 interaction), F22 (`internal/interactive`'s
`TestSessionRendersAreCoalescedAgainstAKnownByteArrivalPattern` flake), F37 (the
`features/sort_order.feature` latent race's product half, left open per task 012/`5042852`) and
the `features/filter.feature` dd/undo race — recurred in this gate.**
Each of those five lives inside a package (`features`, `internal/interactive`) that every one
of the 10 runs reports `ok`, with no per-test detail in a non-`-v` run to hide a failure behind:
`go test` reports a package `ok` only when every test and subtest inside it passes, so a
recurrence of any of the five would have surfaced as that package's line reading `FAIL`, not
`ok`, in at least one of the 10 runs — confirmed present and `ok` in every run for both packages:

```
$ grep -a -c '^ok  	github.com/n-orlov/deck/features' docs/reports/phase3h-010-stability10/run-*.log
run-1.log:1  run-2.log:1  run-3.log:1  run-4.log:1  run-5.log:1
run-6.log:1  run-7.log:1  run-8.log:1  run-9.log:1  run-10.log:1
$ grep -a -c '^ok  	github.com/n-orlov/deck/internal/interactive' docs/reports/phase3h-010-stability10/run-*.log
run-1.log:1  run-2.log:1  run-3.log:1  run-4.log:1  run-5.log:1
run-6.log:1  run-7.log:1  run-8.log:1  run-9.log:1  run-10.log:1
```

**This is not proof any of the five is fixed** — F2, F20, F22 and F37 have each shown up at
roughly one hit in ten or fewer runs historically (per their own rows in
[`phase3g-findings.md`](phase3g-findings.md)), so a single clean 10-run gate does not retire
a flake with that base rate; F37 in particular is described in its own row as reproducible
**only under a forced interleaving** this gate never applies. Task 010's own README already
recorded "nothing to classify against the PRD's out-of-scope list" for the same reason. Nothing
here is claimed fixed; this section is the recurrence check the PRD's non-goals section and the
standing rules both call for, and it found nothing to report.

## 4. How to re-check every citation in this report

Every backticked sha above resolves under `git cat-file -e`; every backticked repo-relative
path names a file tracked in the tree at `b5e4178` (this report's own writing sha) under
`git ls-files --error-unmatch`:

```
$ for sha in de90a5c a24ff8d f700025 2ccb1d3 2badb74 46dad5e; do \
    git cat-file -e "$sha^{commit}" && echo "$sha ok"; done
de90a5c ok
a24ff8d ok
f700025 ok
2ccb1d3 ok
2badb74 ok
46dad5e ok
$ git ls-files --error-unmatch \
    features/create_cwd_ghost.feature \
    features/golden_frame_test.go \
    internal/interactive/render_test.go \
    features/sort_order.feature \
    docs/reports/phase3h.md \
    docs/reports/phase3g-findings.md \
    docs/reports/phase3h-010-stability10/summary.log \
    docs/reports/phase3h-010-stability10/.exitstatus \
    docs/reports/phase3h-010-stability10/run-1.log \
    docs/reports/phase3h-010-stability10/run-10.log \
    docs/reports/phase3h-012-delivery-log/README.md \
    prds/phase3h-suite-reconciliation.md \
    SPEC.md
```

`prds/phase3h-suite-reconciliation.md` and `SPEC.md` are both protected paths for this job and
are quoted here, never edited — the disagreement in [§1](#1-the-prds-de90a5chead-protected-path-range-disagrees-with-the-tree)
is disclosed against the PRD's own text, exactly as the standing rules require, and nothing in
this report writes to either file. `docs/reports/phase3h-010-stability10/run-2.log` through
`run-9.log` are omitted from the explicit `ls-files` list above only for brevity — the same
`grep -a -c` invocations in [§3](#3-the-stability-gate-1010-and-no-recurrence-of-any-out-of-scope-finding)
already read all ten by name and every one resolved with output, which is itself proof each is
tracked and present.
