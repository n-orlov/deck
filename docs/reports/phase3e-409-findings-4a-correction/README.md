# Task 409 — correct `phase3e-findings.md` §4a to agree with `phase3e.md`

## 1. The contradiction

Both `docs/reports/phase3e-findings.md` §4a and `docs/reports/phase3e.md`'s R54 section described
task 314's `@requirement-54-sidebar-click-on-interactive-row-is-a-no-op` scenario's tag-alone
discrimination gap as an **open, unfixed** anomaly — written when the tree stood at approach 01's
tip. Tasks 401 (root-cause), 402 (fix), 403 (waits removed) and 411 (regression fix) landed a real
fix for exactly that gap since then, but neither report was updated to say so. Left as written,
the gap between what the tree now does and what both documents claimed would only have grown.

## 2. The two stale passages, quoted side by side

**`phase3e-findings.md` §4a (before this task's edit):**

> `features/mouse.feature`'s `@requirement-54-sidebar-click-on-interactive-row-is-a-no-op` scenario
> (commit `d55f174`) is structured as two sessions: it first retargets interactive mode from
> session A to session B (exercising task 313's retarget path as a precondition), then clicks B's
> own already-interactive row to exercise the true no-op branch. [...] Run alone, with only the
> no-op scenario's own tag, against reverted code: the scenario passes, because pre-313 code treats
> *every* sidebar click while interactive as a passive no-op regardless of whether the row is the
> current target, so a scenario that never gets past B's own already-passing precondition trivially
> satisfies the no-op assertion too. [...] task 314's validation attempts (2) are exhausted [...]
> left as history rather than reopened without new information.

**`phase3e.md`'s R54 section (before this task's edit):**

> **Known anomaly, left untouched, out of scope for 326**: this task carries a recorded validation
> finding (`tasks.json`'s `validationNotes` on task 314) that the no-op scenario, run in isolation
> with only itself (`@requirement-54-sidebar-click-on-interactive-row-is-a-no-op` alone, not paired
> with the retargeting scenario in the same suite run), does not go red against a reverted task 313
> — the two-session restructuring in `d55f174` makes the no-op scenario's own red proof depend on
> the retargeting scenario having already been shown capable of detecting 313's absence in the same
> run, per that scenario's `validationNotes`. Recorded here rather than re-litigated; task 314
> stands `completed` in `tasks.json` per prior worker decisions, unchanged by this report task.

The two passages agreed with each other (both described the gap as open) but **both** were wrong
about the current tree — they described approach 01's tip, not tasks 401-411's repair pass over it.

## 3. Which claim was wrong, and why

The specific claim that broke first: "*run alone... does not go red against a reverted task 313*".
That was true of the tree at task 314's own tip, but is no longer true. Task 402
(`docs/reports/phase3e-402-r54-noop-sidebar-scoped-click/`, commit `bc6bc84`) fixed the actual
defect — `features/mouse_bindings_test.go`'s `locateText` scanning the whole frame and having its
match stolen by the preview panel's own top border once the target session became interactive
(root-caused by task 401, `docs/reports/phase3e-401-r54-noop-discriminator/`, commit `b358361`, by
direct file-based instrumentation of which `hitPanel*` target the click actually resolved to — not
by reasoning about the scenario's assertions, which were already correct). The tag-alone run now
DOES go red against reverted code — proven by
[`phase3e-402-r54-noop-sidebar-scoped-click/revert-e6e1af3-tag-alone.log`](../phase3e-402-r54-noop-sidebar-scoped-click/revert-e6e1af3-tag-alone.log):

```
$ git revert --no-commit e6e1af3
$ ci/run.sh sh -c 'DECK_GODOG_TAGS=@requirement-54-sidebar-click-on-interactive-row-is-a-no-op go test -count=1 -v -run TestFeatures ./features/'
$ git revert --abort
```

Exit **1**. Failure is at the retarget *precondition* step, not the no-op assertion itself
(`mouse.feature:129`, matching the scenario's own documented behaviour):

```
And deck client "A" has session "retarget-noop-b" selected # mouse.feature:129
  Error: after scenario hook failed: deck client "A" does not have
  session "retarget-noop-b" selected: timed out waiting for frame
  "> retarget-noop-b": context deadline exceeded

1 scenarios (1 failed)
21 steps (10 passed, 1 failed, 10 skipped)
--- FAIL: TestFeatures (7.49s)
```

This is the exact evidence this task's own success criteria named ("the review proved the
reverted-code run fails at its retarget precondition, exit 1") — it was already sitting in task
402's own committed log; the gap was purely that neither `phase3e-findings.md` nor `phase3e.md`
had been told about it yet.

## 4. What was changed

- **`docs/reports/phase3e-findings.md` §4a** — rewritten. The stale claim is quoted verbatim first
  (so the correction is traceable), then the actual mechanism (task 401's finding), the fix (task
  402, with 403's wait-removal and 411's regression-fix noted), and the current tag-alone red proof
  are described, citing report paths and commit shas (`b358361`, `bc6bc84`, `748bc80`, `517bf57`,
  `75861e0`). Task 314 is explicitly left `completed`/unchanged in `tasks.json` — the correction
  does not reopen it, it documents the follow-up tasks that closed its own recorded gap.
- **New `docs/reports/phase3e-findings.md` §4e** — added. Documents task 406's residual,
  found-but-not-fixed `Model.previewFit` re-entrancy hazard (`internal/tui/tui.go:1804-1810`),
  which task 406's own report (§7) explicitly flagged as belonging here. Task 401's refuted
  `r54MutantHypothesis` is folded into the §4a rewrite itself (it's part of that investigation's
  narrative, not a separate defect).
- **`docs/reports/phase3e.md`'s R54 section** — the stale "Known anomaly, left untouched" paragraph
  is replaced with a paragraph describing the same fix (401/402/403/411), so the two documents agree
  with each other again rather than just both being wrong the same way. The R54 evidence-table row
  (per-requirement table) gained tasks 401/402/403/411 and their commits, and the stale "34 distinct
  shas" count in that table's preamble was corrected (see §5).
- **`docs/reports/phase3e-findings.md`'s Cross-references** — added a bullet listing the five
  repair-pass reports §4a/§4e now cite.

## 5. Citation sweep — every sha and path in both files

Per this task's success criteria: every commit sha cited anywhere in `docs/reports/phase3e.md` and
`docs/reports/phase3e-findings.md` resolves (`git cat-file -e`), and every cited in-repo path
exists. Swept mechanically (script below), not by eye:

```python
# extracts every `[0-9a-f]{7,40}` inside backticks, and every markdown [..](..) link target,
# from both files; resolves shas with `git cat-file -e`, resolves link targets relative to
# docs/reports/ and checks os.path.exists.
```

Result, captured in [`citation-sweep.log`](citation-sweep.log):

```
Total distinct shas cited: 43
Sha resolution failures: 0
Total distinct link targets cited (excluding external): 20
Link resolution failures: 0
```

Zero failures across both files. (`phase3e.md`'s own preamble note about its sha count was updated
from a stale "34" to state the count grows and to point at this sweep instead of a number that will
itself go stale the next time either document gains a citation.)

## 6. Repository state

`git status --short` after this task's edits: only the two edited docs plus this new report
directory (see the commit this report ships with). No product code, no test code, no feature file
touched. `ci/run.sh` was not invoked — this task is documentation-only and needed no build/test
run; the citation sweep above is the applicable verification for a docs-only task.
