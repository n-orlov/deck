# Task 038 — Phase 3f approach 02 close-out: protected-path sha audit, git hygiene, citation sweeps, push

Modelled on [../phase3d-212-closeout/README.md](../phase3d-212-closeout/README.md) and
[../phase3f-027-closeout/README.md](../phase3f-027-closeout/README.md) (approach 01's
close-out, which this one supersedes for tasks 028-038 and does not re-derive).

Raw evidence in this directory:
[protected-path-check.log](protected-path-check.log),
[git-hygiene.log](git-hygiene.log),
[check-citations-all-reports.log](check-citations-all-reports.log),
[citation-sweep.log](citation-sweep.log),
[delivery-log-manual-check.log](delivery-log-manual-check.log),
[delivery_log_sweep.py](delivery_log_sweep.py).

## 0. Scope and dependency check

Approach 02 is the commit range **`60c2c56..HEAD`** — `60c2c56` is the operator's phase 3f PRD
commit, and approach 02's own work starts at `2b39124` (task 028) and runs to `84b26a9` (task 037).
The wider phase-3f range **`bce80ea..HEAD`** (`bce80ea` = the last phase-3e commit) is the range the
protected-path audit in §1 uses, because the two recognised protected-path shas are the phase-opening
pair `c80a14c` (`SPEC.md`) and `60c2c56` (the PRD) and both sit *before* `2b39124`. 44 commits in
`bce80ea..HEAD`: those two, 32 approach-01 commits (tasks 001-027, closed out by
[../phase3f-027-closeout/README.md](../phase3f-027-closeout/README.md)) and 10 approach-02 commits
(tasks 028-037).

Task states were tallied from `/run/ralphd/tasks.json` in this iteration — recompute the same way,
by tallying the `status` field of every entry; do not copy a count forward. At pickup: **11 entries,
10 `validated` (028-037), and 038 itself `in-progress`**, because a task cannot be terminal while
the iteration that closes it is still running. So every task 028-037 was terminal before this
close-out started, and every one of them has a commit on `main` (§3).

## 1. Protected-path audit — BY SHA, never by identity

The protected paths are `SPEC.md`, `prds/`, `ci/Dockerfile`, `ci/SPIKE.md`. The recognised sha set
for this phase is **exactly two shas, `c80a14c` and `60c2c56`**, both announced in advance as the
operator's own. The audit is a pass/fail on the **sha list** and deliberately does *not* look at
author or committer identity: in this repo the run's own commit identity and the operator's are the
byte-identical string `Nik <nikolaiorl@gmail.com>` (both come from the same mounted repo's
`.git/config`), so an identity-based check would happily pass a protected-path edit made by the run
itself. That is the exact hole [../phase3d-212-closeout/README.md](../phase3d-212-closeout/README.md)
§4 documented; this audit inherits its method unchanged.

Full raw output: [protected-path-check.log](protected-path-check.log).

```
$ git log --format=%H bce80ea..HEAD -- SPEC.md prds/ ci/Dockerfile ci/SPIKE.md
60c2c56af9b7456df81c7408e7ca812a9434701a
c80a14cd13110d884bc7a79cb1c8de0619f58610
```

Two shas, and they are set-equal to the two recognised ones — not merely a superset of them: no
third sha appeared, and had one appeared it would be **named here as a hard failure**, unclassified,
for the operator to confirm, rather than explained away.

Per path, independently, and what each recognised sha actually touched:

| path | commits in `bce80ea..HEAD` |
| --- | --- |
| `SPEC.md` | 1 — `c80a14c` (`SPEC.md` only, +42/-15) |
| `prds/` | 1 — `60c2c56` (`prds/phase3f-residuals-and-suite-determinism.md` only, +1110) |
| `ci/Dockerfile` | 0 |
| `ci/SPIKE.md` | 0 |

**Protected-path audit: PASS.** Approach 02's ten commits contribute nothing to this output — four
code commits under `internal/` and `features/` and six docs commits under `docs/` — and neither did
approach 01's. Task 038's own commits add files only under `docs/reports/`, so the audit is unchanged
after they land; it is re-run and re-quoted after the push in §5.

## 2. `git status --short` and unpushed commits

At pickup, at HEAD `84b26a9` (task 037), the tree's only entry was this close-out directory itself,
which did not exist yet, and nothing was unpushed:

```
$ git status --short
?? docs/reports/phase3f-038-closeout/
$ git log origin/main..HEAD --oneline
(empty)
$ git rev-parse HEAD origin/main
84b26a97a02eba77ca81cc987e013db8d43f9dcd
84b26a97a02eba77ca81cc987e013db8d43f9dcd
```

The state that matters for close-out is *after* this task's own commits are pushed — a clean
`git status --short` and an empty `git log origin/main..HEAD` — and that is §5.

## 3. One commit per task 028-037, and no rewritten history

Full raw output: [git-hygiene.log](git-hygiene.log).

**Coverage — every task 028-037 maps to at least one commit**, derived mechanically from the
`(task NNN)` suffix the standing rules require in every subject line
(`git log --format=%h --grep="(task NNN)" 60c2c56..HEAD`): *"completed tasks with no commit:
(none)"*. Ten tasks, ten commits, **exactly one commit each** — no task in this approach carries a
second commit:

| task | commit | what it delivered |
| --- | --- | --- |
| 028 | `2b39124` | finding F1's fix: `preview.feature`'s passive-fit floor scenario no longer races its own shrink |
| 029 | `a0d4887` | R74 leg 1: a per-launch generation minted into the launch lease |
| 030 | `196e6f4` | R74 leg 2: a hook write from a superseded launch generation is dropped |
| 031 | `0a5034d` | R75: a concluded launch releases its lease (final **code** commit of the phase) |
| 032 | `9ce65be` | the green whole-suite run at `0a5034d` |
| 033 | `2ab9f4a` | the real 10/10 stability measurement at `0a5034d` |
| 034 | `ac273c9` | `docs/reports/phase3f.md`: R65 re-derived, R74/R75 recorded |
| 035 | `7bc8a39` | `docs/reports/phase3f-findings.md`: F1 closed, six new findings |
| 036 | `f8174f7` | `docs/DELIVERY-LOG.md` reconciled with the re-derived verdict |
| 037 | `84b26a9` | the R74/R75 fixing shas posted to issue #11, recorded |

**Deviations, disclosed rather than smoothed over.** Two, both structural, both in this task:

1. **Task 038 carries two commits, by design** — commit **A** (this report, §§0-4 and §6, plus the
   evidence files) and commit **B** (§5's quoted push output and `push.log`). A commit cannot
   contain the output of its own push, so quoting the push of the commit that carries this report
   requires a second commit. Both are ordinary forward commits with a `(task 038)` subject; neither
   amends, rebases or replaces the other. This is the same structure and the same reason as
   [../phase3f-027-closeout/README.md](../phase3f-027-closeout/README.md) §5.
2. **No local amend in approach 02.** The newest `commit (amend)` entry anywhere in the `HEAD` reflog
   is `202e1ba` at `2026-08-26 18:20:27 +0000` (approach 01's task 017, disclosed in that
   close-out's §3), which predates approach 02's first commit `2b39124` (pushed
   `2026-08-27 00:03:37 +0000`) by more than five hours. No `rebase`, `reset` or `filter-branch`
   entry falls inside approach 02's window either. Approach 01's amend stands as published history
   and is **not** re-litigated here.

**No forced update, ever.** All **448** `refs/remotes/origin/main` reflog entries are plain
`update by push`; `grep -c forced-update` over them is **0** — that is the repository's entire push
history, not just this phase's window. Approach 02's ten pushes are one entry per commit, in commit
order, `2b39124` → `84b26a9`. Both operator commits are published, checked by ancestry rather than
by reading a log: `git merge-base --is-ancestor c80a14c origin/main` → `c80a14c: PUBLISHED`, and the
same for `60c2c56`.

## 4. Citation sweeps — every cited sha and log path resolves

Both committed checkers are run **from the repo root** and both are green. They are complementary:
sweep A resolves backticked *source* paths but reads only the two aggregator documents; sweep B walks
every phase-3f report and resolves links relative to the **citing** directory. A third, hand-written
sweep covers `docs/DELIVERY-LOG.md`, which neither committed checker looks at.

**Sweep A — `docs/reports/phase3f-evidence/check-citations.sh`.** Full output:
[check-citations-all-reports.log](check-citations-all-reports.log).

```
$ sh docs/reports/phase3f-evidence/check-citations.sh
checked 59 shas, 141 cited paths and 81 backticked source paths across: docs/reports/phase3f.md docs/reports/phase3f-findings.md
ALL CITED SHAS RESOLVE, ALL CITED PATHS EXIST AND EVERY BACKTICKED SOURCE PATH EXISTS
(exit 0)
```

**Sweep B — `docs/reports/phase3f-027-closeout/citation_sweep.py`.** Full output:
[citation-sweep.log](citation-sweep.log).

```
$ python3 docs/reports/phase3f-027-closeout/citation_sweep.py
reports scanned: 34
distinct shas cited: 44
sha resolution failures: 0
distinct (citing dir, link target) pairs: 167
link resolution failures: 0
CITATION SWEEP: PASS - every cited sha resolves and every cited path exists
(exit 0)
```

The counts grew with approach 02's reports, which is the point of re-running rather than citing
approach 01's numbers: sweep A 49→**59** shas, 96→**141** cited paths, 51→**81** backticked source
paths; sweep B 27→**34** reports, 35→**44** shas, 90→**167** (citing dir, link target) pairs. Both
re-run at `84b26a9` before this report existed and again with this report in the tree (§5) — the
sweep that counts is the one run against the tree that is committed.

**Sweep C — `docs/DELIVERY-LOG.md`, by hand.** Neither committed checker reads it: sweep A reads only
the two aggregators, and sweep B only walks `docs/reports/**/*.md` whose path contains `phase3f`. So
[delivery_log_sweep.py](delivery_log_sweep.py) is committed alongside this report and sweeps it,
classifying every backticked hex token instead of demanding it be a commit — the delivery log's PRD
column deliberately cites the **blob** sha of the PRD file version a run was given, and four rows cite
shas that live in another repository (`n-orlov/ralphd`) or name an "in-run snapshot" PRD version that
was never committed here. Full output:
[delivery-log-manual-check.log](delivery-log-manual-check.log).

```
$ python3 docs/reports/phase3f-038-closeout/delivery_log_sweep.py
distinct hex tokens cited: 42
  commit      31  ...
  blob         7  ...
  foreign      4  ...
  UNEXPLAINED  0
distinct relative link targets: 6 - failures: 0
distinct backticked repo paths: 29 - failures: 0
VERDICT: PASS - no unexplained sha, every link and path resolves
(exit 0)
```

The 31 commit-resolving tokens include all ten of approach 02's shas. One further token in that
document is purely decimal and is skipped by the same convention sweep B uses (the stability tables
contain unix timestamps that look like shas); it happens to resolve as a PRD blob anyway, so nothing
is hidden by the skip. **Citation sweeps: PASS, all three, zero failures.**

## 5. Push

Filled in by commit **B** (see §3 deviation 1): this section quotes commit A's push output, the
post-push `git status --short` and `git log origin/main..HEAD`, the re-run protected-path audit and
the re-run sweeps, none of which exist until commit A has been pushed.

## 6. Result against task 038's criteria, and the truths carried forward

Each criterion against the section that shows it:

- **(a) protected-path audit BY SHA** — §1:
  `git log --format=%H bce80ea..HEAD -- SPEC.md prds/ ci/Dockerfile ci/SPIKE.md` yields exactly
  `c80a14c` and `60c2c56`, set-equal to the recognised pair, as a hard pass/fail on the sha list and
  explicitly **not** on author or committer identity (which is the byte-identical
  `Nik <nikolaiorl@gmail.com>` for the operator and the run alike, so an identity check would pass a
  run-made edit). A third sha would have been named here as a hard failure; none appeared. **PASS.**
- **(b) clean tree, nothing unpushed, push output captured** — §2 (at pickup) and §5 (after this
  task's own two commits). **PASS.**
- **(c) no forced update in `origin/main`'s reflog** — §3: all 448 entries are `update by push`,
  `forced-update` count **0**, across the repository's whole push history. **PASS.**
- **(d) both citation checkers green from the repo root, with counts** — §4: `check-citations.sh`
  (59 shas, 141 cited paths, 81 backticked source paths, exit 0) and `citation_sweep.py`
  (34 reports, 44 shas, 167 link pairs, exit 0), plus a third hand-written sweep of
  `docs/DELIVERY-LOG.md`, which neither committed checker covers. **PASS.**
- **task-to-commit map for 028-037, deviations disclosed** — §3: ten tasks, ten commits, one each;
  no task carries a second commit and there was no amend in approach 02's window. The two disclosed
  deviations are task 038's own two commits (a commit cannot quote its own push) and the record of
  approach 01's single local pre-publish amend, which stands as published history. **PASS.**

**The truths this close-out carries forward, unrounded.** They are
[`docs/reports/phase3f.md`](../phase3f.md)'s published verdicts; any later summary must carry these,
not a tidier version:

1. **Thirteen requirements met — R63-R73 from the PRD plus R74 and R75 from operator steer 002
   (issue #11) — and none FAILED.** R65 is the one verdict that moved: approach 01 published it
   **FAILED** at `e47cb35`, and task 034 re-derives it as **met** from finding F1's fix (`2b39124`)
   and a 10/10 measurement at the final code commit `0a5034d`. Approach 01's FAILED verdict stays
   published as history rather than being edited away.
2. **The real stability rate at `0a5034d` is 10/10** — ten runs commissioned, ten run, none repeated
   to manufacture a streak ([logs](../phase3f-033-stability10/)). Approach 01's **9/10** at
   `c12c30e` stays published as the record of the previous code tree
   ([logs](../phase3f-022-stability10/)); the improvement is attributed to `2b39124` and nothing else.
   No expected SIGWINCH count was re-baselined, and no scenario was deleted, skipped or tagged out.
3. **Three things remain open and are not claimed fixed**: `previewFit`'s no-live-pane early return
   still spends the row's one coalesced fit (`internal/tui/tui.go:1406-1413`, finding F12);
   `TestGoldenMinimumFrame`'s "frame kept changing" recurrence did not fire in any of task 033's ten
   runs but ten samples do not retire it; and `reconcile.go`'s "terminal row + live pane" invariant
   detector was explicitly out of R75's scope (finding F16). The full list, including the six
   findings approach 02 added, is [`docs/reports/phase3f-findings.md`](../phase3f-findings.md).
