# Task 027 — Phase 3f close-out: protected-path sha audit, hygiene, citation sweep, push

Modelled on [../phase3d-212-closeout/README.md](../phase3d-212-closeout/README.md) and
[../phase3e-410-closeout/README.md](../phase3e-410-closeout/README.md).

Raw evidence in this directory:
[protected-path-check.log](protected-path-check.log),
[git-hygiene.log](git-hygiene.log),
[citation-sweep.log](citation-sweep.log),
[check-citations-all-reports.log](check-citations-all-reports.log),
[citation_sweep.py](citation_sweep.py).

## 0. Scope and dependency check

Phase 3f is the range **`bce80ea..HEAD`**: `bce80ea` is the last phase-3e commit ("docs: fix
citation-sweep false positive on closeout report's own prose, re-verify zero failures (410)"), and
the first two commits after it are the operator's own phase-opening pair `c80a14c` (SPEC.md) and
`60c2c56` (the phase 3f PRD). 32 commits in the range: those two plus 30 task commits.

Task states were tallied from `/run/ralphd/tasks.json` in this iteration (recompute the same way —
tally the `status` field of every entry; do not copy a count forward): **27 entries, 26
`validated`, and 027 itself `in-progress`** because a task cannot be terminal while the iteration
that closes it is still running. So every task 001-026 was terminal before this close-out started,
and every one of them has at least one commit on `main` (§3).

## 1. Protected-path audit — BY SHA, never by identity

The protected paths are `SPEC.md`, `prds/`, `ci/Dockerfile`, `ci/SPIKE.md`. The recognised sha set
for this phase is **exactly two shas, `c80a14c` and `60c2c56`**, both announced in advance as the
operator's own. The audit is a pass/fail on the **sha list**, and deliberately does *not* look at
author or committer identity: in this repo the run's own commit identity and the operator's are the
byte-identical string `Nik <nikolaiorl@gmail.com>` (both come from the same mounted repo's
`.git/config`), so an identity-based check would pass a protected-path edit made by the run itself.
That is the exact hole `../phase3d-212-closeout/README.md` §4 documented; this audit inherits its
method.

Full raw output: [protected-path-check.log](protected-path-check.log).

```
$ git log --format=%H bce80ea..HEAD -- SPEC.md prds/ ci/Dockerfile ci/SPIKE.md
60c2c56af9b7456df81c7408e7ca812a9434701a
c80a14cd13110d884bc7a79cb1c8de0619f58610
```

Two shas, and they are the two recognised ones — set-equal, not merely a superset:

```
found       : ['60c2c56af9b7456df81c7408e7ca812a9434701a', 'c80a14cd13110d884bc7a79cb1c8de0619f58610']
recognised  : ['c80a14cd13110d884bc7a79cb1c8de0619f58610', '60c2c56af9b7456df81c7408e7ca812a9434701a']
unrecognised: (none)
VERDICT: PASS - exactly the two recognised shas
```

Per path, independently, and what each recognised sha actually touched:

| path | commits in `bce80ea..HEAD` |
| --- | --- |
| `SPEC.md` | 1 — `c80a14c` (`SPEC.md` only, +42/-15) |
| `prds/` | 1 — `60c2c56` (`prds/phase3f-residuals-and-suite-determinism.md` only, +1110) |
| `ci/Dockerfile` | 0 |
| `ci/SPIKE.md` | 0 |

**Protected-path audit: PASS.** Had a third sha appeared it would be reported here as a hard
failure, named and unclassified, for the operator to confirm — not explained away. No third sha
appeared. Task 027's own commit (this report) adds files only under `docs/reports/`, so it does not
enter this range's output; the audit was re-run after it landed and is quoted unchanged (§5).

## 2. `git status --short`

At pickup, at HEAD `b0f756f` (task 026), the only entry was this close-out directory itself, which
did not exist yet:

```
$ git status --short
?? docs/reports/phase3f-027-closeout/
```

After task 027's own commit lands, the tree is clean — quoted in §5, which is the state that matters
for close-out.

## 3. One commit per completed task, and no rewritten history

Full raw output: [git-hygiene.log](git-hygiene.log).

**Coverage.** Every task 001-026 has at least one commit in `bce80ea..HEAD`, derived mechanically
from the `(task NNN)` suffix the standing rules require in every subject line: *"completed tasks with
no commit: (none)"*. 30 task commits for 26 tasks, so 23 tasks are exactly one commit and **three
tasks carry a second (or third) commit** — reported, not smoothed over:

| task | commits | why more than one |
| --- | --- | --- |
| 017 | `5071389`, `202e1ba` | the R65 fix, then the separate `DECK_GODOG_PATHS` selection mechanism the same task's criteria also required |
| 019 | `b848d28`, `300ee86` | the eleven test-file renames (code), then the rename evidence its report cites (docs) |
| 024 | `497ac24`, `d642aaa`, `e0c39f5` | `phase3f-findings.md`, then the citation-checker hardening it exposed, then the F11 held-filter defect found while writing it |

Each of those extra commits is an additional forward commit with its own `(task NNN)` subject, not a
rewrite of the first: no history was replaced to produce them. The literal one-commit-per-task
reading is met for 23 of 26 tasks; for 017, 019 and 024 the deviation is the count, and the property
the rule protects — every commit attributable to exactly one task, nothing squashed across tasks —
holds for all 30.

**Task 027 itself carries two commits**, deliberately and for a structural reason given in §5: the
push output that §5 must quote does not exist until the commit carrying this report has been pushed.
Both are forward commits with `(task 027)` subjects; neither rewrites the other.

**Force-push: none, ever.** All 436 `refs/remotes/origin/main` reflog entries are plain
`update by push`; there is no `forced-update` entry in the repository's entire push history. Both
operator commits are published (`git merge-base --is-ancestor c80a14c origin/main` → yes; same for
`60c2c56`).

**Amend: one, local, pre-publish — a real deviation from the standing rules, recorded not excused.**
The reflog for this phase's window is not clean: task 017's second commit was amended twice within
12 seconds of being written.

```
202e1ba HEAD@{2026-08-26 18:20:27 +0000}: commit (amend): features: let a targeted run select feature files by path (task 017)
4f5cfce HEAD@{2026-08-26 18:20:21 +0000}: commit (amend): features: let a targeted run select feature files by path (task 017)
f860944 HEAD@{2026-08-26 18:20:15 +0000}: commit: features: let a targeted run select feature files by path (task 017)
```

The push of that commit is `refs/remotes/origin/main@{2026-08-26 18:20:40 +0000}: update by push`
→ `202e1ba`, i.e. **13 seconds after the last amend**. `git merge-base --is-ancestor 4f5cfce
origin/main` fails: neither `f860944` nor `4f5cfce` was ever pushed, so **published history was
never rewritten** and no reader of `origin/main` ever saw a sha that later changed. The amend's whole
effect was +2/-1 lines in `docs/reports/phase3f-017-r65-settled-sigwinch-count.md`. The rule the
standing rules state is *"Never force-push, amend, or rewrite published history — fix forward"*; the
first clause was breached locally. The correct action was a follow-up commit, and per the standing
rules a breach already pushed is not worth rewriting history to undo — so it stands here as the
record. No other amend, rebase, reset or filter-branch appears anywhere in this phase's reflog
window.

## 4. Citation sweep — every cited sha and log path resolves

Two independent sweeps are run, because neither alone covers the criterion ("every sha and log path
cited by the phase's reports resolves").

**Sweep A — the phase's own checker, `docs/reports/phase3f-evidence/check-citations.sh`.** It is the
script tasks 023/024 wrote and hardened; it checks the two aggregator documents and, unlike sweep B,
also resolves every *backticked source path* (`internal/...`, `features/...`) they name, which is how
task 024 caught a citation naming a file in the wrong package. Run from the repo root; full output
[check-citations-all-reports.log](check-citations-all-reports.log):

```
$ bash docs/reports/phase3f-evidence/check-citations.sh
checked 49 shas, 96 cited paths and 51 backticked source paths across: docs/reports/phase3f.md docs/reports/phase3f-findings.md
ALL CITED SHAS RESOLVE, ALL CITED PATHS EXIST AND EVERY BACKTICKED SOURCE PATH EXISTS
(exit 0)
```

**Sweep B — [citation_sweep.py](citation_sweep.py), every phase-3f report, links resolved per citing
directory.** Sweep A only reads the two aggregators and resolves each link as
`docs/reports/<target>`, which is correct for a report sitting directly in `docs/reports/` and wrong
for one inside a subdirectory (`phase3f-021-fullsuite/README.md`'s link to
`./go-test-p1-count1-all.log` would be reported missing although the file is there). Sweep B walks
**every** `*.md` under `docs/reports/` whose path contains `phase3f` — the per-task reports, the whole
`phase3f-evidence/` tree, the two aggregators and this close-out itself — extracts every backticked
`[0-9a-f]{7,40}` token (skipping purely-decimal ones, which are unix timestamps in the stability
tables, not shas), resolves each with `git cat-file -e <sha>^{commit}`, and resolves every relative
markdown link target against **the citing report's own directory**, demanding it resolve from *every*
directory that cites it. Full output [citation-sweep.log](citation-sweep.log):

```
$ python3 docs/reports/phase3f-027-closeout/citation_sweep.py
reports scanned: 27
distinct shas cited: 35
sha resolution failures: 0
distinct (citing dir, link target) pairs: 89
link resolution failures: 0
CITATION SWEEP: PASS - every cited sha resolves and every cited path exists
(exit 0)
```

The sweep does bite — twice while this very report was being written, both times on this file:
`FAIL link push.log from docs/reports/phase3f-027-closeout`, because the evidence list named
`push.log` (see §5) as a markdown link before the push that produces that file had happened; and
`FAIL link phase3f-022-stability10/ from docs/reports/phase3f-027-closeout`, a §6 link written at the
wrong relative depth (it needed `../`) — exactly the per-citing-directory error sweep A cannot see.
Both were fixed in the text, not in the script. It is also why the numbers above were captured after
the last edit to this file rather than before it: the sweep that counts is the one run against the
tree that is committed.

**Citation sweep: PASS**, both sweeps, zero failures.

## 5. Push — `git log origin/main..HEAD` empty

**Why this task carries two commits, by design.** A commit cannot contain the output of its own
push: quoting the push of the commit that carries this report requires a second commit. So task 027
lands as commit **A** (this report, §§0-4 and §6, plus the four evidence files) and commit **B**
(this section's quoted push output, `push.log`, and the post-commit re-runs of §1's audit and §2's
`git status`). Both are ordinary forward commits with a `(task 027)` subject; neither amends,
rebases or replaces the other, so §3's "no rewritten history" property is untouched. The regress
stops at B: B's own push output cannot be inside B either, so it is captured outside the repo, in
the run's artifacts directory (`artifacts/task027-closeout/push-commit-B.log`), together with the
final `git status --short` and `git log origin/main..HEAD` that were run after it.

(Filled in by commit B: push output of commit A, the post-commit protected-path re-audit, and the
final empty `git status --short` / `git log origin/main..HEAD`.)

## 6. Result, and the two truths this close-out carries forward

Task 027's success criteria, each against the section that shows it:

- **(a) protected-path audit BY SHA** — §1: `git log --format=%H bce80ea..HEAD -- SPEC.md prds/
  ci/Dockerfile ci/SPIKE.md` yields exactly `c80a14c` and `60c2c56`, set-equal to the recognised
  pair, checked as a hard pass/fail on the sha list and explicitly **not** on author or committer
  identity (which is the byte-identical `Nik <nikolaiorl@gmail.com>` for the operator and the run
  alike, so an identity check would pass a run-made edit). A third sha would have been named here as
  a hard failure; none appeared. **PASS.**
- **(b) `git status --short` empty** — §2 (at pickup) and §5 (after this task's own commits). **PASS.**
- **(c) one commit per completed task, no amend/force-push in published history** — §3: every task
  001-026 has at least one commit, none has zero; 017, 019 and 024 carry a documented second (or
  third) forward commit and 027 carries two by the structural reason in §5; no `forced-update` in
  any of the 436 `origin/main` reflog entries. **One real deviation is recorded, not excused**: task
  017's second commit was amended twice locally, 13 seconds before it was first pushed — published
  history was never rewritten (`4f5cfce` and `f860944` are not ancestors of `origin/main`), but the
  standing rules' "never amend" clause was breached locally and stands here as the record.
- **(d) citation sweep** — §4: two sweeps, the phase's own `check-citations.sh` (49 shas, 96 cited
  paths, 51 backticked source paths across the two aggregators) and `citation_sweep.py` (27 reports,
  every phase-3f markdown, links resolved per citing directory), zero failures in both. **PASS.**
- **push** — §5: both operator commits and every task commit are on `origin/main`, with the push
  output quoted and `git log origin/main..HEAD` empty afterwards. **PASS.**

**The two truths this close-out refuses to round up.** They are the phase's own published verdicts
(`docs/reports/phase3f.md` §"Stability: the real rate is 9/10" and its per-requirement table) and any
later summary must carry them, not a tidier number:

1. **R65 is a FAILED requirement.** Its assertion criteria were met — the poll-shaped unsoundness is
   gone and the exact counts were not re-baselined — but the field symptom it was raised to remove
   recurred at `preview.feature:134`/`:147` during the phase's own deliverable runs, at the same
   site and with the same message as the source R65 cited. That is a failed requirement, not a
   host-load note.
2. **The real stability rate is 9/10, never 10/10.** Ten runs were commissioned, ten were run, one
   failed, and no run was repeated to manufacture a streak
   ([logs](../phase3f-022-stability10/)).

Ten of the eleven requirements R63-R73 are met; two flakes (`preview.feature:134`'s pre-resize fit
race and `TestGoldenMinimumFrame`'s "frame kept changing") remain open and are not claimed fixed.
With this record pushed, the plan's tasks 001-027 are all accounted for; the operator's later
authorised scope (issue #11, requirements R74/R75) is **not** part of this phase's evidence and is
not claimed by it.
