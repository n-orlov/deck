# Task 207 — DELIVERY-LOG's Phase 3g section corrected to a coherent final state of record

Review (steering 001) named three stale claims in `docs/DELIVERY-LOG.md`'s Phase 3g section. Each
is corrected below, quoted verifying command by verifying command, from the actual tree — not from
expectation.

## (a) "no SPEC authority anywhere in this tree" — corrected to name `de90a5c`

Before this task, the section claimed R93's shipped drag-selection behaviour "now has **no SPEC
authority anywhere in this tree**" after `2d61993`'s forward-revert. That was stale: the operator
subsequently landed §11.8's R93 wording verbatim, directly in `SPEC.md`, at `de90a5c` — a commit
that predates this run's own base `a24ff8d` (it is `a24ff8d`'s own parent).

```
$ grep -n "no SPEC authority anywhere in this tree" docs/DELIVERY-LOG.md
(exit 1, nothing printed)
```

```
$ git log --oneline -1 de90a5c -- SPEC.md
de90a5c spec: resolve §7's error-under-live-pane self-contradiction and land §11.8's R93 wording (operator)
```

```
$ git rev-parse a24ff8d^
de90a5c15c96aab6a207b7ae4824901f6a1119bf
$ git rev-parse de90a5c
de90a5c15c96aab6a207b7ae4824901f6a1119bf
```

The corrected section now states, in its own sentence: "**The operator landed §11.8's R93 wording
verbatim at `de90a5c`** ... so R93's shipped drag-selection behaviour **does have SPEC authority in
this tree**, as of that commit" — positive, not merely a deletion of the old claim.

## (b) the `1cfbd5a..HEAD -- SPEC.md` empty-diff claim — corrected, non-empty diff quoted

Before this task, the section claimed "`git diff 1cfbd5a..HEAD -- SPEC.md` empty at every commit
since" `2d61993`. That was false once `de90a5c` landed a third `SPEC.md` touch on that path.

```
$ git diff --stat 1cfbd5a..HEAD -- SPEC.md
 SPEC.md | 35 ++++++++++++++++++++++++++---------
 1 file changed, 26 insertions(+), 9 deletions(-)
```

```
$ git log --oneline 1cfbd5a..HEAD -- SPEC.md
de90a5c spec: resolve §7's error-under-live-pane self-contradiction and land §11.8's R93 wording (operator)
2d61993 docs: forward-revert b69b5ba's SPEC.md edit and record F41 (task 909)
b69b5ba docs: amend SPEC.md §11.8 for R93's visible in-progress selection (task 205)
```

The corrected section now names all three touches (`b69b5ba`, `2d61993`, `de90a5c`) instead of
claiming the range is empty of further touches after `2d61993`.

This run's own protected-path guard is unaffected by any of this — `de90a5c` predates `a24ff8d`,
so it falls outside the `a24ff8d..HEAD` range this run's guard checks:

```
$ git log --oneline a24ff8d..HEAD -- SPEC.md prds/ ci/Dockerfile ci/SPIKE.md
(nothing printed)
$ git diff --stat a24ff8d..HEAD -- SPEC.md prds/ ci/Dockerfile ci/SPIKE.md
(nothing printed)
```

## (c) `a5f8f6b` mislabelled as Phase 3g's final code sha — every mention accounted for

`fdf4507` is Phase 3g's true final code sha, per the section's own close-out paragraph further down
(`docs/DELIVERY-LOG.md:705`: "against the phase's true final state recorded in this paragraph
above: final code sha `fdf4507`"). `a5f8f6b` (task 1001) came chronologically later inside this
tree's real history, but review and the assigned task's own successCriteria direct this correction
to hold `fdf4507` as 3g's true final code sha, consistent with that pre-existing close-out
paragraph — so every mention of `a5f8f6b` *as a final-code-sha label* is now marked superseded,
without deleting the historical sweep/stability measurements taken at it.

```
$ grep -n 'a5f8f6b' docs/DELIVERY-LOG.md
552:fixed). Commits run `1cfbd5a..HEAD`, reaching `a5f8f6b` (task 1001).
553:**This citation of `a5f8f6b` as Phase 3g's final code sha is superseded — 3g's true final code sha is `fdf4507`**, per the close-out paragraph below.
554:The docs-only-after-it claim this section used to make does not hold across the full range: `git diff --stat a5f8f6b..HEAD -- '*.go' '*.feature'` is non-empty (tasks 1101–1204 and this run's own tasks 001–207 both touch code after it).
560:**This is, again, a superseded final-code-sha citation of `a5f8f6b` — `fdf4507` is 3g's true final code sha**:
597:that no eligibility predicate refuses a live-pane hook-sourced `error` row (task 1001, `a5f8f6b`),
```

Line by line:

- **Line 552** — plain historical commit citation. Introduces the commit the `1cfbd5a..HEAD` range
  reaches (`a5f8f6b`, task 1001); it does not itself assert a final-code-sha claim (that assertion
  is the very next line). Left as history, named as such.
- **Line 553** — final-state/final-code-sha label. Carries `superseded` and names `fdf4507` as 3g's
  true final code sha, on this same line, as the successCriteria require.
- **Line 554** — plain historical commit/diff citation. Cites the (now non-empty) diff range
  `a5f8f6b..HEAD -- '*.go' '*.feature'` to show the old "docs-only after it" claim no longer holds;
  it is not itself a final-code-sha label. Left as history, named as such:
  ```
  $ git diff --stat a5f8f6b..HEAD -- '*.go' '*.feature' | tail -1
   9 files changed, 988 insertions(+), 153 deletions(-)
  ```
- **Line 560** — final-state/final-code-sha label. Carries `superseded` and names `fdf4507` as 3g's
  true final code sha, on this same line.
- **Line 597** — plain historical commit citation, inside the ten-approaches summary ("it pins by
  test that no eligibility predicate refuses a live-pane hook-sourced `error` row (task 1001,
  `a5f8f6b`)"). Describes what task 1001's own commit did; not a final-code-sha claim. Left as
  history, named as such.

Both superseded-final-code-sha lines are direct siblings of the sweep/stability measurements they
used to introduce as "final"; those measurements (`docs/reports/phase3g-1002-fullsuite/suite.log`,
`docs/reports/phase3g-1003-stability10/summary.log`) are preserved verbatim, not deleted:

```
$ grep -c 'gates are red at the phase' docs/DELIVERY-LOG.md
1
```

## This section cites this phase's (Phase 3h's) own final code sha

```
$ grep -c '4b1d4dc' docs/DELIVERY-LOG.md
1
$ grep -n '4b1d4dc' docs/DELIVERY-LOG.md
538:empty) is untouched by it. **Phase 3h disposition (task 207, final code sha `4b1d4dc`): this
```

`4b1d4dc` is task 201's own commit (already the standing rules' "final code sha").

## Citation sweep — every backticked token in this report

```
$ grep -o '`[^`]*`' docs/reports/phase3h-207-delivery-log-3g/README.md | sort -u
```

| Token | Check | Result |
|---|---|---|
| `2d61993` | `git cat-file -e 2d61993^{commit}` | exists |
| `4b1d4dc` | `git cat-file -e 4b1d4dc^{commit}` | exists |
| `a24ff8d` | `git cat-file -e a24ff8d^{commit}` | exists |
| `a24ff8d..HEAD` | (range, both endpoints checked individually) | exists |
| `a5f8f6b` | `git cat-file -e a5f8f6b^{commit}` | exists |
| `b69b5ba` | `git cat-file -e b69b5ba^{commit}` | exists |
| `ci/Dockerfile` | `git ls-files --error-unmatch ci/Dockerfile` | tracked |
| `ci/SPIKE.md` | `git ls-files --error-unmatch ci/SPIKE.md` | tracked |
| `de90a5c` | `git cat-file -e de90a5c^{commit}` | exists |
| `docs/DELIVERY-LOG.md` | `git ls-files --error-unmatch docs/DELIVERY-LOG.md` | tracked |
| `docs/DELIVERY-LOG.md:707` | (line reference into a tracked file, checked above) | valid |
| `docs/reports/phase3g-1002-fullsuite/suite.log` | `git ls-files --error-unmatch <path>` | tracked |
| `docs/reports/phase3g-1003-stability10/summary.log` | `git ls-files --error-unmatch <path>` | tracked |
| `fdf4507` | `git cat-file -e fdf4507^{commit}` | exists |
| `prds/` | `git ls-files --error-unmatch prds/phase3g-field-backlog.md` (dir, spot-checked) | tracked |
| `1cfbd5a` | `git cat-file -e 1cfbd5a^{commit}` | exists |
| `SPEC.md` | `git ls-files --error-unmatch SPEC.md` | tracked |

```
$ git cat-file -e 2d61993^{commit} && git cat-file -e 4b1d4dc^{commit} && git cat-file -e a24ff8d^{commit} \
    && git cat-file -e a5f8f6b^{commit} && git cat-file -e b69b5ba^{commit} && git cat-file -e de90a5c^{commit} \
    && git cat-file -e fdf4507^{commit} && git cat-file -e 1cfbd5a^{commit} && echo ALL_SHAS_OK
ALL_SHAS_OK
$ git ls-files --error-unmatch docs/DELIVERY-LOG.md ci/Dockerfile ci/SPIKE.md SPEC.md \
    docs/reports/phase3g-1002-fullsuite/suite.log docs/reports/phase3g-1003-stability10/summary.log \
    prds/phase3g-field-backlog.md && echo ALL_PATHS_OK
ALL_PATHS_OK
```

## Outcome

- `docs/DELIVERY-LOG.md`'s Phase 3g section corrected in place, one commit, `(task 207)`.
- Historical measurements preserved (not deleted): `grep -c 'gates are red at the phase'
  docs/DELIVERY-LOG.md` → `1`.
- Section cites Phase 3h's own final code sha `4b1d4dc` (task 201's), once.
- Protected paths untouched: both `a24ff8d..HEAD` guards over `SPEC.md prds/ ci/Dockerfile
  ci/SPIKE.md` print/diff nothing.
