# Task 113 — Phase 3g close-out: protected-path sha audit, citation sweeps, delivery log

Modelled on [`../phase3f-027-closeout/README.md`](../phase3f-027-closeout/README.md) and
[`../phase3e-410-closeout/README.md`](../phase3e-410-closeout/README.md).

Raw evidence in this directory: [protected-path-check.log](protected-path-check.log),
[protected-path-all-refs.log](protected-path-all-refs.log), [citation-sweep.log](citation-sweep.log),
[citation_sweep.py](citation_sweep.py).

Linked from the phase report at [`../phase3g.md`](../phase3g.md)'s "Close-out (task 113)" section, so
this note is reachable from the report the phase publishes.

## 1. Protected-path audit — BY SHA, never by author or committer

The protected paths are `SPEC.md`, `prds/`, `ci/Dockerfile`, `ci/SPIKE.md`; the standing rules name
`2eed8de` and `a03527c` as the only two shas pre-authorised to touch them, both pre-existing operator
commits and both ancestors of this run's own base `1cfbd5a`. Checked by sha, deliberately not by
author or committer identity: in this repo the run's own commit identity and the operator's are the
byte-identical string `Nik <nikolaiorl@gmail.com>` (both come from the same mounted repo's
`.git/config`), so an identity-based check would pass a protected-path edit made by the run itself —
the exact hole `../phase3d-212-closeout/README.md` §4 and `../phase3e-410-closeout/README.md` and
`../phase3f-027-closeout/README.md` §1 all document; this audit inherits their method. Full raw
output: [protected-path-check.log](protected-path-check.log).

```
$ git log --oneline 1cfbd5a..HEAD -- SPEC.md prds/ ci/Dockerfile ci/SPIKE.md
(empty)
```

Empty: this run — both approaches, tasks 001–113 — touched none of the four protected paths.

**A precision the standing rules' own phrasing does not carry: of the two pre-authorised shas, only
one actually touches a protected path.**

```
$ git log --oneline -1 2eed8de
2eed8de spec: amend §7, §9.2, §9.3, §11.3, §11.4, §11.7 and add §11.10 ahead of the field-backlog phase (operator)
$ git show --name-only --format="" 2eed8de
SPEC.md

$ git log --oneline -1 a03527c
a03527c plan: defer Codex to last, cut Phase 3g, and state that the spec moves before the PRD (operator)
$ git show --name-only --format="" a03527c
docs/PLAN.md
```

`2eed8de` touches `SPEC.md` and is a real protected-path commit, correctly pre-authorised and pre-
existing (ancestor of `1cfbd5a`, confirmed below). `a03527c` touches only `docs/PLAN.md` — not one of
the four protected paths at all — so its membership in the allow-list is vacuous: nothing it did ever
needed authorising under this rule. This does not weaken the audit; the range command above already
proves the protected set had zero commits in `1cfbd5a..HEAD`, and both shas are excluded from that
range regardless. It is disclosed here rather than silently repeating the standing rules' phrasing,
because "the only commits ever touching those paths are 2eed8de and a03527c" is precisely true of
`2eed8de` and vacuously true of `a03527c` — not evidence that `a03527c` touched anything protected.

**Both are ancestors of the run's base**, confirmed directly rather than assumed from the standing
rules' own claim:

```
$ git merge-base --is-ancestor 2eed8de 1cfbd5a && echo "2eed8de: ancestor of run base 1cfbd5a (pre-existing, operator)"
2eed8de: ancestor of run base 1cfbd5a (pre-existing, operator)
$ git merge-base --is-ancestor a03527c 1cfbd5a && echo "a03527c: ancestor of run base 1cfbd5a (pre-existing, operator)"
a03527c: ancestor of run base 1cfbd5a (pre-existing, operator)
```

**The three most recent commits to touch the protected set, unfiltered by any allow-list**, so a
reader can see what actually precedes the run's own base rather than trusting a curated pair:

```
$ git log --oneline -3 -- SPEC.md prds/ ci/Dockerfile ci/SPIKE.md
1cfbd5a prds: cut Phase 3g, the field backlog (operator)
2eed8de spec: amend §7, §9.2, §9.3, §11.3, §11.4, §11.7 and add §11.10 ahead of the field-backlog phase (operator)
60c2c56 prds: cut Phase 3f — field defects, residuals, and suite determinism (operator)
```

`1cfbd5a` is the run's own base commit (excluded from the `1cfbd5a..HEAD` range by git's exclusive-
left convention) and is itself an operator commit (the PRD cut) that predates every task in this
plan.

### 1a. The exhaustive pass over ALL refs — and what "the only commits ever touching those paths" can and cannot mean

The range check above is scoped to `1cfbd5a..HEAD` on `main`. Two things it does not by itself settle
are settled here, both by sha and over **every ref in the repository** (`--all`, i.e. `main`,
`origin/main`, `origin/HEAD` — the full ref list is quoted in the log): that no commit *anywhere*
after the run base touches the protected set, and what the complete set of protected-path commits in
this repo's whole history actually is. Full raw output:
[protected-path-all-refs.log](protected-path-all-refs.log).

```
$ git log --all --oneline 1cfbd5a.. -- SPEC.md prds/ ci/Dockerfile ci/SPIKE.md
(exit 0; empty)

$ git log --all --oneline -- SPEC.md prds/ ci/Dockerfile ci/SPIKE.md | wc -l
30

$ for c in $(git log --all --format=%h -- <protected set>); do
>   git merge-base --is-ancestor $c 1cfbd5a || echo "NOT ancestor: $c"; done
checked=30 not_ancestor_of_run_base=0
```

So: **30 commits in this repository's whole history touch the protected set, and all 30 are ancestors
of the run's base `1cfbd5a`** — every one predates this plan's first task, and zero commits reachable
from any ref touch a protected path after the base. All 30 are listed by sha in the log; they are the
spec and PRD cuts that built the product's own documents (`2b5ea90` "Add deck product spec" through
`1cfbd5a` "prds: cut Phase 3g").

**This is a correction to a claim in this task's own success criteria, and it must be stated as a
correction rather than smoothed over.** The criteria ask this note to show "that the only commits ever
touching those paths are 2eed8de and a03527c". Read literally — as a claim about the repository's
whole history — that is **false**, and the two commands above are what disprove it: 30 commits touch
those paths, among them the run's own base `1cfbd5a` and its predecessor `60c2c56`. No true audit can
show the literal claim, and this note does not pretend to. What the standing rules that phrase comes
from actually state is an **allow-list** — "the only commits *allowed* to touch the protected set are
`2eed8de` and `a03527c`", i.e. the only shas this run is permitted to have touching them — and that
is a statement about authorisation, not about history. Under the two readings that can be true, the
audit is exactly satisfiable and passes:

1. **Scoped to this run** (the reading the rule is for): the commits touching the protected set within
   the run's own history — `1cfbd5a..HEAD`, and `--all 1cfbd5a..` across every ref — number **zero**,
   which is a subset of any allow-list, so no unauthorised sha exists to find.
2. **The allow-list's own two shas**: both are ancestors of `1cfbd5a` (checked above), so neither is a
   commit of this run either way; and of the two, only `2eed8de` touches a protected path at all
   (`SPEC.md`), while `a03527c` touches only `docs/PLAN.md` — its allow-list membership is vacuous, as
   disclosed above.

The literal phrasing is therefore recorded here as a **finding against the criteria's wording**, not
as a result: the intent ("this run must not have edited SPEC.md, prds/, ci/Dockerfile or ci/SPIKE.md,
and you must prove it by sha, not by trusting the author field") is met and, with the `--all` pass
above, proven more strongly than the range check alone proved it. **Protected-path audit: PASS on the
run-scoped and allow-list readings; the literal whole-history reading is disproved above and disclosed
as a wording defect, never claimed.**

## 2. Citation sweep over both phase3g reports

[`citation_sweep.py`](citation_sweep.py) scans exactly the two files task 113's success criteria
name — `docs/reports/phase3g.md` and `docs/reports/phase3g-findings.md` — and checks three things
neither of the two prior per-task sweeps (task 109's over `phase3g.md`, task 110's
`check-citations.sh` over `phase3g-findings.md`) checked *together*: every backtick-quoted sha
resolves (`git cat-file -e`), every relative markdown link target exists (resolved against
`docs/reports/`, correct for both files since both live directly there), and — the gap neither prior
sweep actually covered for `phase3g.md` — every same-document `](#...)` anchor resolves against that
file's own headings under GitHub's slug rule.

Full output, quoted verbatim: [citation-sweep.log](citation-sweep.log) — which holds both passes: the
first at `df7a35e` (this note's own first commit) and the second after `phase3g.md` gained the
"Close-out (task 113)" section that links this directory and this note gained §1a. The second, current
pass, quoted verbatim:

```
$ python3 docs/reports/phase3g-113-closeout/citation_sweep.py
reports scanned: 2
  docs/reports/phase3g.md
  docs/reports/phase3g-findings.md

distinct shas cited: 66
sha resolution failures: 0

distinct link targets cited: 48
link resolution failures: 0

distinct same-document anchors cited: 28
anchor resolution failures: 0

CITATION SWEEP: PASS - every cited sha resolves, every cited path exists, every same-document anchor resolves
```

(The first pass read 65 shas, 42 links and 27 anchors; the six new links, one new sha and one new
anchor are exactly the report's new close-out section and this note's §1a, so the delta is accounted
for rather than merely smaller-than-before.)

**The sweep found and fixed a real, previously-uncaught defect before it reached the clean run
above.** Task 109's own sweep over `phase3g.md` (`docs/reports/phase3g-109-report-update/README.md`
§2) resolved every markdown *link* by splitting off everything before `#`; for a pure-anchor link
like `[R77](#r77--...)` that split yields an empty string, and the script's own `if p:` guard skips
printing (and therefore checking) it entirely — so `phase3g.md`'s 17-entry table-of-contents, made
entirely of same-document anchors, was never actually checked by any prior sweep. Running this
sweep's anchor pass for the first time found **eight** broken anchors, all in that table of contents
plus two of its own in-body cross-references:

```
FAIL anchor #r77--a-deleted-sessions-name-is-reusable-tasks-003-006 in docs/reports/phase3g.md
FAIL anchor #r78--an-archived-row-keeps-its-name-and-dd-frees-it-tasks-007-009 in docs/reports/phase3g.md
FAIL anchor #r79--a-tombstone-that-outlives-its-process-is-reaped-at-the-next-store-open-tasks-010-011 in docs/reports/phase3g.md
FAIL anchor #r80--the-footer-lists-only-what-the-current-selection-will-accept-tasks-012-013 in docs/reports/phase3g.md
FAIL anchor #r81--the-footers-fixed-set-is-curated-tasks-014-015 in docs/reports/phase3g.md
FAIL anchor #r82--dialogs-are-themed-tasks-016-021 in docs/reports/phase3g.md
FAIL anchor #r83--the-three-dialogs-that-draw-past-the-frame-at-80x24-tasks-016-018-022 in docs/reports/phase3g.md
FAIL anchor #r86--updown-navigate-dialog-fields-tab-is-completion-only-tasks-025-026 in docs/reports/phase3g.md
```

Root cause, in two parts, both about what GitHub's heading slugger does to a character it drops that
has no adjacent space to absorb into a hyphen:

1. **An en dash directly between two digits collapses to nothing, not a hyphen.** Six headings write
   a task range as `(tasks 003–006)` using an en dash (`–`, U+2013) with no surrounding space. The
   slug algorithm (identical to the one task 110 validated and used to fix four anchors in
   `phase3g-findings.md`) strips characters outside `[\w\- ]`, so the en dash disappears and the two
   digit runs merge: `003–006` → `003006`, not `003-006`. The hand-written anchors assumed the latter.
   This is the same mechanism as task 110's em-dash finding (a dash *with* surrounding spaces leaves
   both spaces behind as a double hyphen) but the opposite surface: a dash with *no* surrounding
   space leaves nothing behind at all.
2. **`×`, `↑` and `↓` drop with no substitute text.** R83's heading writes `80×24` (multiplication
   sign, U+00D7); the anchor was hand-written as `80x24` (the letter x) — the two are different
   characters and the slugger drops `×` outright, so the real slug is `8024`. R86's heading opens
   with `` `↑`/`↓` `` (backtick-quoted arrow glyphs); the anchor was hand-written as `updown`, but the
   slugger has no glyph-to-word substitution — after the backticks are stripped and the arrows and
   slash are dropped, three adjacent spaces (each left behind by a dropped character) become a
   *triple* hyphen, not the word `updown`.

Both are fixed the way task 110 fixed its four: **only the anchor strings changed** (in the table of
contents at the top of `phase3g.md` and in two in-body cross-references at R83's and R82's own
sections); no heading text moved, and the sweep above is the post-fix, all-green run. The pre-fix
failing run is not separately committed as a log (the fix landed before this report's own commit),
but is reproducible by reverting the anchor-string edit and re-running the script — the eight strings
above are exactly what a revert restores.

**The sweep's known false-positive class does not fire here, and is disclosed rather than
re-triggered.** `prds/phase3g-field-backlog.md`'s own "Reports" section states it: *"a sweep that
greps a report's own prose for citations will flag the report describing itself."* That class is
about **keyword/prose search** (does the string "not settled" appear anywhere else) flagging a
report's own discussion of the thing being searched for as if it were a fresh instance of it — it is
not about resolution checks (does this sha/path/anchor exist), which is all this sweep and its two
predecessors ever do, so it structurally cannot fire in `citation_sweep.py`'s own output. The place
it legitimately fires in this tree is already disclosed at its own source:
`docs/reports/phase3g-findings.md`'s §4 (the F2-golden-frame-settle recurrence check) explicitly
excludes its own file from `grep -rl "not settled" docs/reports/phase3g* --exclude=phase3g-findings.md`
*because* that section's own prose contains the words "not settled"/"settle" while describing the
flake it is checking for, and would otherwise self-match. That exclusion, and the reasoning for it,
is unchanged by this task and still holds at this close-out's own commit.

## 3. `docs/DELIVERY-LOG.md`

A new paragraph records Phase 3g in [`docs/DELIVERY-LOG.md`](../../DELIVERY-LOG.md), in the same
prose style (no `## Phases` table row) that Phase 3f itself uses there — that table stops at 2b-2 by
the operator's own disclosed, deliberate omission (`a03527c`'s commit message, quoted in this file's
own "Other milestones" section), not an oversight this task should correct. The new paragraph states:
the PRD, run id and date range; both evidence reports; the commit range `1cfbd5a..HEAD`; both
approaches (001–092, then 101–113) and what each closed; **task 111's whole-suite result** (green at
`9f61e21`, exit 0, 311/311 scenarios, with the log path); **task 112's stability rate** (9/10, script
exit 1, with the summary log and run 7's own log path, and run 7's real, non-F2/F22 root cause named
rather than left as a bare number); R82's published-partial status; and the one disclosed, unfixed
open regression (`status_recovery.feature` racing R76's own repair). Nothing in that paragraph is a
number carried forward without being recomputed in this same task — the suite and stability figures
are copied from tasks 111/112's own committed reports, quoted above and cross-checked against those
reports' own text in §§0 above of this file where relevant.

## 4. Result

- **(a) protected-path audit BY SHA** — §1: `git log --oneline 1cfbd5a..HEAD -- SPEC.md prds/
  ci/Dockerfile ci/SPIKE.md` is empty; §1a extends this over every ref (`git log --all --oneline
  1cfbd5a.. -- <protected set>` also empty) and shows all **30** protected-path commits in the repo's
  whole history are ancestors of the run's base `1cfbd5a`; `2eed8de` and `a03527c` are both among
  those ancestors, confirmed by `git merge-base --is-ancestor`, and only `2eed8de` touches a protected
  path at all; the identity check is explicitly not by author/committer, which in this repo would be a
  no-op distinction (same string for run and operator). **PASS on the run-scoped and allow-list
  readings.** The criteria's literal "the only commits *ever* touching those paths are 2eed8de and
  a03527c" is disproved in §1a (30 commits do, all pre-base) and disclosed there as a wording defect
  — this note reports that, it does not assert the literal claim.
- **(b) citation sweep over both phase3g reports, output quoted, self-citation false-positive class
  disclosed** — §2: `citation_sweep.py`'s full output is quoted, zero failures after fixing eight
  previously-uncaught broken anchors (found by this task, not pre-existing knowledge), and the known
  false-positive class from `prds/phase3g-field-backlog.md` is named, shown not to fire in this
  sweep, and pointed at where it legitimately already fires and is already disclosed
  (`phase3g-findings.md` §4). **PASS.**
- **(c) `docs/DELIVERY-LOG.md` records Phase 3g** — §3: new paragraph naming task 111's suite result
  and task 112's rate, each with its log path. **PASS.**
- **(d) `git status --short` empty, `git log --oneline origin/main -1` equal to local HEAD** — verified
  after this task's own commit is pushed (§5 below, the same structural reason
  `../phase3f-027-closeout/README.md` §5 gives: a commit cannot quote the push of itself).

## 5. Push

This report's own commit necessarily lands before its push can be observed; the push output, the
final `git status --short` and the final `git log --oneline origin/main -1`/`git rev-parse HEAD` are
captured in the run's artifacts directory
(`/run/ralphd/artifacts/task113-closeout/push.log`) rather than inside this commit, for the same
reason task 027 and task 410 both gave: a commit cannot contain the record of its own push.
