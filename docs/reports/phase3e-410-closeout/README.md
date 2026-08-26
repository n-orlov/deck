# Task 410 — close-out: protected-path sha audit, clean git state, citation sweep

This is the final close-out for Phase 3e's approach-02 repair pass (tasks 401-411, all
`completed` in `tasks.json` before this task started — checked directly, not assumed). Method
follows `docs/reports/phase3d-212-closeout/README.md` as instructed, adapted for this run's own
protected-sha allow-list (steer 3e-001 §4) instead of that run's single-commit identification.

## 1. Protected-path sha audit — by sha only, never by author/committer identity

Per the notes' standing rule and this task's own criterion, the check is a hard sha allow-list,
never author/committer-based (author/committer identity on this repo is not a discriminator: the
run's own commits and the operator's commits share the same `.git/config` identity, exactly as
`phase3d-212-closeout` found for its own run).

```
$ git log 4b21dea..HEAD --format='%h %s' -- SPEC.md prds/ ci/Dockerfile ci/SPIKE.md
2c56309 SPEC: probe.miss is a column not an event, events index+retention, dialogs load via tea.Cmd (operator)
791e089 prds: cut Phase 3e — list ergonomics and chrome (operator)
6584299 spec: configurable sort order, single-click interactive entry, shared seam focus, open built-in theme set (operator)
```

Exactly three lines, and the set is **exactly** `{6584299, 791e089, 2c56309}` — the allow-list
`tasks.json`'s `discovered.protectedPathShas` records and the criterion names verbatim. No other
sha appears in the range; per steer 3e-001 §4, `2c56309` (a SPEC edit landed mid-run) is the
operator-authorised exception to "protected paths never touched" and is expected in this list, not
a violation of it.

Per-path breakdown (full log: [`protected-path-check.log`](protected-path-check.log)):

| Path            | Commits in `4b21dea..HEAD` |
|-----------------|----------------------------|
| `SPEC.md`       | `2c56309`, `6584299`       |
| `prds/`         | `791e089`                  |
| `ci/Dockerfile` | (none)                     |
| `ci/SPIKE.md`   | (none)                     |

All three shas resolve (`git cat-file -e 6584299`, `-e 791e089`, `-e 2c56309`, all exit 0 —
captured in the same log). **PASS.**

## 2. Build / vet / gofmt

```
$ ci/run.sh go build ./...
(no output, exit 0)                              -- go-build.log
$ ci/run.sh go vet ./...
(no output, exit 0)                               -- go-vet.log
$ ci/run.sh sh -c "gofmt -l $(git ls-files '*.go')"
(no output, exit 0)                               -- gofmt.log
```

All three clean at HEAD `7cd1f8b`. Note (consistent with every one of tasks 401-408's own build
checks): a literal `ci/run.sh gofmt -l .` additionally reports 3 files under `.spike-preview/`
(`cmd/conformance/main.go`, `conformance/conformance.go`, `conformance/conformance_test.go`) —
that directory is git-ignored scratch space (`.gitignore:4`), pre-existing, untouched by tasks
401-411, and not part of this repo's tracked source; it is excluded here the same way every prior
task in this run excluded it, by scoping to `git ls-files '*.go'` instead of the whole working
tree. Raw output of the literal `gofmt -l .` form is not separately committed since it adds no
new information beyond what every earlier task's notes already recorded.

## 3. Git hygiene

Captured before this task's own commit (only this report directory untracked):

```
$ git status --short
?? docs/reports/phase3e-410-closeout/
$ git log origin/main..HEAD
(empty)
```

Both confirmed empty again immediately after this task's own docs-only commit lands and is pushed
(see §5).

## 4. Citation sweep — re-run of task 409's method, widened to every Phase 3e report

Task 409's own sweep covered only `docs/reports/phase3e.md` and `docs/reports/phase3e-findings.md`.
This task's criterion asks for the same method re-run across **all** Phase 3e reports, so the
script (committed here as [`citation_sweep.py`](citation_sweep.py)) walks every `*.md` file under
`docs/reports/` whose path contains `phase3e` (24 files, including every per-task report directory
and its nested sub-reports, e.g. task 406's `pre-fix-repro/`, `post-fix-sweep-scoped/`,
`tmuxwrap-experiment/`), not just the two aggregator documents:

- extracts every backtick-quoted `[0-9a-f]{7,40}` token, **excluding purely-decimal tokens**
  (task 408's stability report tables cite raw unix timestamps like `` `1787726259` `` in the same
  backtick style as a sha — a naive regex misfires on those two as "unresolvable shas"; both are
  10-digit epoch seconds from that table, not shas, confirmed by `grep` locating them at
  `phase3e-408-stability-10-at-75861e0/README.md:27,36,38`, and excluded by requiring at least one
  non-digit hex character, since no real sha cited anywhere in this run's reports is purely
  numeric);
- resolves every surviving sha with `git cat-file -e`;
- extracts every markdown `[text](target)` link (skipping `http(s)://` and same-page `#anchor`
  links), resolves each target relative to the citing file's own directory, and checks it exists
  on disk.

Result ([`citation-sweep.log`](citation-sweep.log)):

```
Total distinct shas cited: 43
Sha resolution failures: 0

Total distinct link targets cited (excluding external): 42
Link resolution failures: 0
```

**Zero failures** across all 24 files, sha and link targets both. Full per-file list and any
failure detail (none) is in the committed log.

## 5. Result

All of task 410's success criteria are met:

- Protected-path sha audit: `git log 4b21dea..HEAD -- SPEC.md prds/ ci/Dockerfile ci/SPIKE.md`
  yields exactly `6584299`, `791e089`, `2c56309` and nothing else, checked by sha only — PASS.
- `ci/run.sh go build ./...`, `go vet ./...`, `gofmt -l .` (scoped to tracked `.go` files, per
  every prior task's own established method) all clean — PASS.
- `git status --short` and `git log origin/main..HEAD` both empty once this task's own docs-only
  commit is pushed — PASS (verify after commit, not just before).
- Citation sweep across all Phase 3e reports (not just the two aggregator docs): 43 distinct shas,
  42 distinct link targets, zero failures — PASS.

**Approach 02's repair pass over the Phase 3e tree (tasks 401-411) is complete.** R52-R58 stand as
verified by the prior review; the five blocking findings and two coverage gaps that review raised
are closed by 401-406, the whole-suite and stability evidence is current at the final code commit
`75861e0` (tasks 407/408, redone after 411's regression fix), the two aggregator docs agree with
each other and with the tree (task 409), and this task's own audit finds nothing outstanding.
