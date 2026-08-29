# Phase 3g task 506 — disposition of every failure task 505 named

## Scope

Task 505's own report, `docs/reports/phase3g-505-stability10/README.md`, is
the naming authority this task closes against. This task's job is: for each
failing test or scenario that report names, either land a fix with its own
evidence directory, or record it as an out-of-scope open finding in
`docs/reports/phase3g-findings.md` — never both, never neither, never
described as fixed if it is the latter.

## What 505 named

`docs/reports/phase3g-505-stability10/README.md` (both of its published
rounds — round 2, the measurement of record at launch sha `d863538`, and
round 1, kept as history at launch sha `af288eb`) names **exactly one**
failing test, in both rounds, at the same file:line:

- **`TestSigwinchCountDistinguishesTwoFromThree`**, `features/sigwinch_count_test.go:89`
  — `sigwinch count after 1st resize = 0, want exactly 1 before sending the
  2nd`.
  - Round 2: run 6, `docs/reports/phase3g-505-stability10/run-6.log:5004`.
  - Round 1: run 5, `docs/reports/phase3g-505-stability10/round1/run-5.log:5004`.
  - Rate observed by 505 alone: 2 hits in 20 runs (one per round), both 9/10.

505 did not observe 10/10 in either round, so the "505 observed 10/10 and
named no failure" branch of this task's criteria does not apply — this is
not recorded as a fact of that shape.

Task 512 (`c170149`) superseded 505 for an execution-shape reason only (its
launcher and metadata, not its substance) and is the measurement of record
for this tree going forward. It reproduced the **same** test at the **same**
file:line twice in its own ten runs — run 1
(`docs/reports/phase3g-512-stability10/run-1.log:5004`) and run 4
(`docs/reports/phase3g-512-stability10/run-4.log:5004`), 8/10. Cited here for
completeness of the disposition, not as a second named failure: it is the
same defect 505 already named, recurring at a rate consistent with 505's own
observation (4 hits in 40 runs total on this tree: 505's 2/20 plus 512's
2/10).

## Disposition table

| Failure named by 505 | Outcome | Evidence |
|---|---|---|
| `TestSigwinchCountDistinguishesTwoFromThree` (`features/sigwinch_count_test.go:89`) | **(b) — recorded as an out-of-scope, open finding; not fixed under this task** | [`docs/reports/phase3g-findings.md`'s F28 row](../phase3g-findings.md#3-defects-found-and-deliberately-not-fixed-and-why) names the reason and every per-run log path (505's `run-6.log:5004` and `round1/run-5.log:5004`; 512's superseding `run-1.log:5004` and `run-4.log:5004`) |

No other failing test or scenario is named in 505's README, so this table has
one row.

## Why (b) and not (a)

- Task 303 (`157bb52`) already fixed one race in this exact test — the
  inter-resize pacing — and the fix landed and is unmodified since
  (`git log --oneline 1cfbd5a..HEAD -- features/sigwinch_count_test.go`
  below shows only that one commit). What 505/512 observe is a **residual**
  after that fix, at a materially lower rate (roughly 1-in-10) than whatever
  303 closed.
- A further fix requires its own root-cause investigation into sigwinch
  delivery timing under load (is the OS signal lost, is the test's own poll
  window too tight, is there a genuine product-level race in resize
  handling) — that investigation, and any fix it licenses, is explicitly the
  job of task 507 (finding 1's 10/10 gate, where a genuine product-race fix
  is in scope and required if root-caused) and task 509 (the findings sweep
  at the approach's final state). Doing that investigation here would also
  violate this task's own "one commit per failure" rule if it turned out to
  need more than a single, narrow, well-evidenced change.
- This is exactly the shape of F2 (`TestGoldenMinimumFrame`) and F22
  (`internal/interactive` ByteArrivalPattern) already in
  `phase3g-findings.md`: a known, named, low-rate flake, disclosed with its
  log path and reason, not claimed fixed.

```
$ git log --oneline 1cfbd5a..HEAD -- features/sigwinch_count_test.go
157bb52 features: synchronise sigwinch-count test's inter-resize pacing on the observed count (task 303)
```

## Guard checks

- `features/godog_test.go`'s `defaultTags` is unmodified by this task's
  commits.
- No `t.Skip` is added by this task's commits.
- No scenario is deleted or retagged by this task's commits.

```
$ git diff HEAD -- features/godog_test.go
$ git log -p --follow -- features/sigwinch_count_test.go | grep -c 't.Skip'
0
```

(Both checks are run again, against this task's actual commit range, at
commit time — see the commit message for the exact invocation and output.)
