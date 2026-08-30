# Task 012 — bring `docs/DELIVERY-LOG.md`'s Phase 3g paragraph to the final state of record

## What was already true before this task

Finding F35 (`docs/reports/phase3g-findings.md`) named five statements as stale in the Phase 3g
paragraph. Re-checking the paragraph as it stood at this task's start (`08171ce`), four of the
five were already corrected by earlier phase3g work (tasks 608/814/1008/1009, `bb72d96`/`9e6d525`/
`1a0d877`/`a7c8d76`):

- approach count and task-id ranges: already stated as ten approaches, 01 (`0NN`, 001–042)
  through 10 (`10NN`, 1001–1009).
- `ci/stability.sh 10` figure: already `10/10 (Go suite exit 0) green at approach 06's
  then-final sha b0a4e7d`; `grep -n '9/10' docs/DELIVERY-LOG.md` matches nothing inside the
  Phase 3g paragraph (lines 509–674) both before and after this commit.
- R82: already stated `resolved, not partial`.
- requirement set: already stated as `R76–R93`.

## What this task actually changed

The one statement genuinely stale at this task's start: F31 (the reconcile lost update) was still
recorded **open** in the delivery-log paragraph, even though Phase 3h's own task 007 (`2ccb1d3`)
had since fixed it for real by adding `AllowedCurrentStatuses: []string{"starting"}` to the
`tmux.shell_live` promotion — exactly the candidate fix F31's own row in
`phase3g-findings.md` had named and declined to apply.

This commit inserts one paragraph, immediately after the existing F31 discussion, stating:

- F31 is fixed for real, naming task 007's commit `2ccb1d3` as the fix.
- The regression evidence: task 006's red-before (`8d6ed72`,
  [`docs/reports/phase3h-006-f31/`](../phase3h-006-f31/README.md)) and task 007's green-after
  ([`docs/reports/phase3h-007-f31-guard/`](../phase3h-007-f31-guard/README.md)).
- F2, F20, F22 and F37 are explicitly left recorded open — unaffected by F31's fix, still the
  out-of-scope, never-claimed-fixed items this run's own standing rules name.

## Follow-up: the F37 wording (same task, second commit)

Validation of the first commit (`5708f52`) found the F37 statement contradictory: the disposition
paragraph left F37 "recorded open" while the task-810 sentence later in the same paragraph said F37
was **fixed** by task 804 (`46dad5e`). Both halves were true of different things, so neither was
withdrawn — the wording was made precise instead, in two places and nothing else:

- the disposition paragraph now says which half Phase 3h leaves open for F37: the *product*-side
  mechanism (the unconditional live-pane repair that can win the reconcile tick and reorder a row),
  untouched by task 804 and untouched by task 007's promotion guard, reproducible only under the
  tracked forced interleaving `sh docs/reports/phase3g-810-findings/reproduce-f37.sh`. F2, F20 and
  F22 stay open alongside it; Phase 3h claims no fix for any of the four.
- the task-810 sentence now attributes task 804's fix to the *scenario*-side red it actually fixed
  (`features/sort_order.feature`'s six raw `error` writes, rerouted through a genuine nonzero pane
  exit) and points at the disposition paragraph for the open product-side half.

This matches `docs/reports/phase3g-findings.md`'s own F37 row, which records exactly that fix and
exactly that reroute. No claim of a Phase 3h fix for F2, F20, F22 or F37 appears anywhere in the
paragraph.

No other sentence in the paragraph was rewritten. `git show --stat HEAD` (this task's commit)
touches only `docs/DELIVERY-LOG.md` and this report directory.

## Verification

- `grep -n '9/10' docs/DELIVERY-LOG.md` — no line inside the Phase 3g paragraph (lines 509–674).
- Every sha cited in the paragraph resolves: `git cat-file -e <sha>^{commit}` exits 0 for all of
  `17b1649 1cfbd5a 2094b83 2ccb1d3 2d61993 2eed8de 46dad5e 5ea9475 608e030 89edd3c 8d6ed72
  a03527c a5f8f6b b0a4e7d b434079 b69b5ba d266346 ea6ce4b fdf4507`.
- `git diff --stat` before commit: only `docs/DELIVERY-LOG.md` (13 insertions, 2 deletions).
