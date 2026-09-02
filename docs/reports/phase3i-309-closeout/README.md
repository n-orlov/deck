# Phase 3i — approach 3 closing report (task 309)

Approach 3 of run `deck-phase3i-2` lands **zero** `*.go`/`*.feature` changes. Every task in
this approach (301-309) is a gate artifact or a `docs/` correction over the final code sha
**`a559e7c61a00ab5fa31c2d98eaf5ce787744e4dc`** (`git log -1 --format=%H -- '*.go' '*.feature'`,
frozen since task 203 of approach 2 and unchanged by every commit in approach 3).

## Tasks 301-308, statuses read from `/run/ralphd/tasks.json` at write time

All eight are `validated`. Read fresh with:

```
python3 -c "import json;d=json.load(open('/run/ralphd/tasks.json'));[print(t['id'],t['status'],t['title']) for t in d['tasks'] if t['id'] in [str(i) for i in range(301,309)]]"
```

| Task | Status | Title |
|---|---|---|
| 301 | validated | Stop the engine's `artifacts/` scratch dir from dirtying the worktree |
| 302 | validated | Re-run and republish the whole-suite sweep with strict 60-second polling |
| 303 | validated | Publish the verbose companion sweep's Gherkin tally at the final code sha |
| 304 | validated | Retract findings section 5's invented R100-versus-SPEC disagreement |
| 305 | validated | Record approach 3's gate disposition at final code sha `a559e7c` in the findings report |
| 306 | validated | Bring `docs/reports/phase3i.md`'s R100 and R103 rows in line with the final tree |
| 307 | validated | Correct `docs/DELIVERY-LOG.md`'s Phase 3i paragraph against the final tree |
| 308 | validated | Publish a re-runnable guard script that re-verifies every citation of the approach-3 documents |

**Task 309 is this report's own task** — the commit this README lands in. At the moment this
file is written, `tasks.json` still shows 309 `in-progress` (the harness only applies the
`complete` verdict once this iteration's process exits); it is not listed above as `validated`
because it is not, yet — it is the commit being made right now, not a prior deliverable being
summarised.

## Terminal non-completed tasks of the earlier approaches — no requirement rests on any of them

Read fresh from the two frozen approach backups (this run's own `tasks.json` holds only
301-309; the statuses below come from `/run/ralphd/approaches/01/tasks.json` and
`/run/ralphd/approaches/02/tasks.json`):

| Task | Approach | Status | What it was, and what stands in its place |
|---|---|---|---|
| 119 | 1 | `failed` (validation-exhausted) | "Prove the two displacement flavours tear down differently" — failed on test strength (the stolen-claim test didn't count ownership-option *reads*, only unsets), not on product behaviour. Residual carved into task 136 (`validated`, `fcdb994`), which counts the `show-options` reads in the stolen-claim teardown test and demonstrates the added assertion is load-bearing. No requirement's discharge cites 119 itself. |
| 134 | 1 | `skipped` | An earlier citation-guard task, superseded by the same guard's later, corrected form (approach 3's task 308). No requirement cites 134. |
| 204 | 2 | `failed` (validation-exhausted) | "Run and publish the whole-suite sweep at the new final code sha" — failed on sweep-polling discipline (a `sleep 180` poll misrepresented as `sleep 60`). Superseded by this approach's task 302 (`validated`, strict 60-second polling, `7db4756`). No requirement cites 204; the current gate evidence is 302. |
| 205 | 2 | `pending` | "Publish the verbose companion sweep's Gherkin tally" — `dependsOn: ["204"]`, so once 204 failed this task could never be scheduled and the run moved to approach 3 before it ran; it never reached a terminal status of its own. Superseded by this approach's task 303 (`validated`, `319 scenarios / 3682 steps`, `7206877`). No requirement cites 205; the current gate evidence is 303. |
| 211 | 2 | `skipped` | "Set `phase3i.md`'s R103 row to met" — superseded by this approach's task 306 (`validated`, `90fd553`). No requirement cites 211. |
| 212 | 2 | `skipped` | "Correct the Phase 3i paragraph in `docs/DELIVERY-LOG.md`" — superseded by this approach's task 307 (`validated`, `3761469`). No requirement cites 212. |

None of R98-R103 rests on 119, 134, 204, 205, 211 or 212 in either `docs/reports/phase3i.md`'s
requirement table or `docs/reports/phase3i-findings.md`; each requirement's current evidence
column names the approach-3 task that superseded or discharged the corresponding residual
instead (136 for 119's test-strength gap; 302/303/206 for 204/205's gate artifacts; 306/307 for
211/212's document corrections).

## Final code sha and current gate evidence

- **Final code sha**: `a559e7c61a00ab5fa31c2d98eaf5ce787744e4dc` — `git log --oneline
  a559e7c..HEAD -- '*.go' '*.feature'` is empty; approach 3 lands zero `*.go`/`*.feature`
  changes.
- **Gate directories**, all measured at that sha and all currently the live evidence (not
  superseded):
  - [`docs/reports/phase3i-302-fullsuite/`](../phase3i-302-fullsuite/) — whole-suite sweep,
    exit `0`, 17 packages, 0 FAIL, 60-second polling throughout (task 302).
  - [`docs/reports/phase3i-303-fullsuite-verbose/`](../phase3i-303-fullsuite-verbose/) —
    verbose companion, exit `0`, 319 scenarios / 3682 steps (task 303).
  - [`docs/reports/phase3i-206-stability10/`](../phase3i-206-stability10/) — `ci/stability.sh
    10`, `10/10 passed`, not re-run in approach 3 since no code commit landed after it
    (task 206, approach 2).
- **Guard directory**:
  [`docs/reports/phase3i-308-guards/`](../phase3i-308-guards/) — `verify.sh` re-checks every
  sha and path cited across `docs/reports/phase3i.md`, `docs/reports/phase3i-findings.md` and
  `docs/DELIVERY-LOG.md`'s Phase 3i paragraph, plus worktree/push/protected-path state, five
  labelled `PASS`/`FAIL` checks, exit `0` (task 308).

All four directory paths above pass `git ls-files --error-unmatch` (checked before this commit
and re-checked by task 308's own guard, whose check 2 scans every path cited in the three
documents it covers).

## What this commit also does

The same commit that adds this file also adds `docs/reports/phase3i-308-guards/` and
`docs/reports/phase3i-309-closeout/` to `docs/reports/phase3i.md`'s R103 evidence-list column
(the last column of the R103 row in the requirement table), alongside the paths already
listed there.

## Re-verifying this report

```
sh docs/reports/phase3i-308-guards/verify.sh
```

exits `0` after this commit is pushed, re-checking every sha and path cited by the three
documents task 308's guard covers (this file is not itself in that guard's scope — it covers
`phase3i.md`, `phase3i-findings.md` and `docs/DELIVERY-LOG.md`'s Phase 3i paragraph only). Its
output for this run is saved under `/run/ralphd/artifacts/` (not committed to the repository,
per the standing rule that a document cannot quote its own commit's sha, and per the run's
`artifacts/`-not-tracked convention).
