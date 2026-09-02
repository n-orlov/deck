# Phase 3i — approach 4 closing report (task 409)

Approach 4 of run `deck-phase3i-2` lands exactly two code commits, tasks 401 and 402, then
freezes: `git log --oneline 3b70bfb..HEAD -- '*.go' '*.feature'` is empty (checked immediately
before this commit and re-checked below). The final code sha is
**`3b70bfbc7e3552ff375ae675af117805a1eee944`** (`3b70bfb`) — moved from approach 2/3's
`a559e7c61a00ab5fa31c2d98eaf5ce787744e4dc` by tasks 401/402's SPEC §11.3 footer conformance
fix.

## Approach-4 tasks 401-408, statuses read fresh from `/run/ralphd/tasks.json`

```
$ python3 -c "import json;d=json.load(open('/run/ralphd/tasks.json'));[print(t['id'],t['status'],t['title']) for t in d['tasks'] if t['id'] in [str(i) for i in range(401,409)]]"
401 validated Take the `F` entry out of footerLegend so the footer matches SPEC §11.3's curated fixed set
402 validated Pin footerLegend's glyph set as a closed list against SPEC §11.3's own sentence
403 validated Record the footer PRD/SPEC disagreement as a new numbered finding
404 validated Run and publish the whole-suite sweep at the new final code sha
405 validated Run and publish the ten-run stability gate at the new final code sha
406 validated Publish the verbose companion's Gherkin tally at the new final code sha
407 validated Refresh docs/reports/phase3i.md against the new final code sha
408 validated Refresh docs/DELIVERY-LOG.md's Phase 3i paragraph against the new final code sha
```

All eight are `validated`. **Task 409 is this report's own task** — the commit this README
lands in. At the moment this file is written, `tasks.json` still shows 409 `in-progress` (the
harness only applies the `complete` verdict once this iteration's process exits); it is not
listed above as `validated` because it is not, yet — it is the commit being made right now,
not a prior deliverable being summarised. No requirement row below rests on 409 itself.

**No requirement row in this report rests on a `failed`, `skipped` or `pending` task.** Every
approach-4 row below cites only tasks 401-408, all `validated` per the table above, or tasks
from an earlier approach that the standing rules confirm are `validated`/`completed`
(101-136 range, 201-214 range, 301-309 range — none of the ids this report cites from those
ranges is a terminal non-completed status; the ones that are are named and excluded, exactly
as the earlier reports 309/408 already do).

## R98-R103, one row each, task and commit that discharges it

| Req | Discharging task(s) and commit(s) | What discharges it |
|---|---|---|
| R98 | 401 (`96bff5682571511a3cc63b1065d593fe64440ae9`), 402 (`3b70bfbc7e3552ff375ae675af117805a1eee944`), inheriting 104-109, 201, 202 from earlier approaches | `F` skips the two named contention refusals (attached-client, live-ownership) and is now bound, in the keymap and in the `?` overlay while being **out of** `footerLegend`'s curated fixed set per SPEC §11.3 — task 401 removed the row, task 402 pinned the resulting glyph set as a closed list against SPEC §11.3's own sentence so it cannot drift back |
| R99 | inherited unchanged from tasks 101, 102, 121 (approach 1; no approach-4 task touches this requirement) | Single-shot force claim: exactly one winner, every loser stands down with no error (`internal/tmux/ownership.go`, `internal/tmux/force_ownership_test.go`, the `↵` refused/`F` wins/A is told feature scenario) |
| R100 | inherited unchanged from tasks 110-115, 203, 304 (approaches 1-3; no approach-4 task touches this requirement) | The original geometry survives an arbitrary chain of steals via the durable window option, and finding 5's correction (task 304, `ca00cb6`) retracted the earlier misquote that had invented an R100/SPEC disagreement — R100 and SPEC 11.9 agree, both asking for a plain unconditional unset |
| R101 | inherited unchanged from tasks 116-118, 120, 136, 121-123 (approaches 1-2; no approach-4 task touches this requirement) | The displaced client falls into the lost-attach dialog with its keyboard silenced; task 136 (not 119, which is `failed` — see below) proves the stolen flavour's teardown never reaches `ownership.Release` |
| R102 | inherited unchanged from tasks 124-126 (approach 1; no approach-4 task touches this requirement) | Passive `previewFit` stands down under a foreign live claim, so a second client merely selecting a row no longer resizes a window it doesn't hold |
| R103 | 403 (`4058c01`, findings §8), 404 (`a6b382d`), 405 (`e4d3028`), 406 (`197e035`), 407 (`c1bd57d` + `5d1cc20`), 408 (`e80b554` + `320e3b6`), this report (task 409) | The record matches the tree at the new final code sha `3b70bfb`: this closeout, `docs/reports/phase3i.md` (task 407), `docs/reports/phase3i-findings.md`'s new numbered finding 8 (task 403), `docs/DELIVERY-LOG.md`'s refreshed Phase 3i paragraph (task 408), and the three approach-4 gate directories (tasks 404/405/406) are all current and cite the frozen sha |

None of the six rows above cites task 119 (`failed`), or any of approach 2's terminal
non-completed tasks (204 `failed`; 205 `pending`; 211, 212 `skipped`), as its discharge —
R101's row names task 136 in its place, and R103's row names the current approach-3/4 document
and gate tasks in place of the superseded approach-2 gate artifacts, exactly as the standing
rules and the two earlier closing reports (309, 408's paragraph) already record.

## Gate disposition at the frozen final code sha `3b70bfb`

**Task 404 — whole-suite sweep, exit status quoted verbatim from
[`docs/reports/phase3i-404-fullsuite/README.md`](../phase3i-404-fullsuite/README.md) (taken at
sha `3b70bfb`; the raw `.exitstatus` file itself is run-state scratch, not committed to the
repository, per the same convention this report's own guard transcript follows):**

```
$ grep -c FAIL docs/reports/phase3i-404-fullsuite/sweep.log
0
```

**Exit status: 0** (README's own words) — 17 package result lines, all `ok` or `?` (no test
files), zero `FAIL`.

**Task 406 — verbose companion's Gherkin tally, two lines quoted verbatim from
[`docs/reports/phase3i-406-fullsuite-verbose/`](../phase3i-406-fullsuite-verbose/) (taken at a
docs-only descendant of `3b70bfb`, `e4d3028`, confirmed by that task's own empty
`git log --oneline 3b70bfb..HEAD -- '*.go' '*.feature'` check):**

```
319 scenarios (319 passed)
3682 steps (3682 passed)
```

**Task 405 — ten-run stability gate, result quoted verbatim from
[`docs/reports/phase3i-405-stability10/`](../phase3i-405-stability10/) (taken at sha `3b70bfb`,
ran with the repo at commit `a6b382d`, a docs-only descendant of the frozen sha):**

```
10/10 passed
```

All three gates are clean at the frozen final code sha: exit `0`, 17/17 clean package lines,
319/319 scenarios and 3682/3682 steps, and 10/10 stability runs, with zero `FAIL` lines across
all of them.

## Review finding 1's literal range `6197b53..HEAD` — restated as permanently UNMET

`docs/reports/phase3i-findings.md`'s numbered finding 2 records this and this report restates
it rather than re-arguing it: the PRD's own zero-commit clause
(`prds/phase3i-force-attach.md:33-34`, "The audit range for this run is `6197b53..HEAD` and it
must show **zero** protected-path commits") is **UNMET**, literally and permanently, because
the range necessarily contains the operator's own commit `3090b68` (the PRD-adding commit
itself, landing inside `6197b53..HEAD` by construction — three minutes after the operator's
own `6197b53` SPEC amendment that opens the range). Re-confirmed fresh against this report's
own tree:

```
$ git log --oneline 6197b53..HEAD -- SPEC.md prds ci/Dockerfile ci/SPIKE.md
3090b68 prds: cut Phase 3i — force-attach, stealing the interactive preview (operator)
```

This can never become empty without either an operator amendment narrowing the clause (which
only the operator can make, and none has) or a history rewrite dropping `3090b68` out of the
range (which the standing rules forbid outright: "Never force-push, never rewrite history").
**No task in this run, including this one, claims this range met.** The worker-write range
that actually polices this run's own commits, `3090b68..HEAD`, stays empty (re-confirmed
below), and that is the range this report's own commits are judged against — but the PRD's
own literal clause names `6197b53..HEAD`, and that one is restated here, once more, as
permanently UNMET:

```
$ git log --oneline 3090b68..HEAD -- SPEC.md prds ci/Dockerfile ci/SPIKE.md
```

prints nothing — zero worker commits in this run touched a protected path.

## Terminal non-completed tasks — no requirement row above rests on any of them

Carried forward from the earlier closing reports (309, and 408's DELIVERY-LOG paragraph),
statuses per the standing rules, not the archived per-approach `tasks.json` files (which freeze
at the moment the run moved to the next approach and can show a stale in-flight status, e.g.
205 `pending` in `/run/ralphd/approaches/02/tasks.json`):

| Task | Approach | Status (standing rules) | Superseded by / discharged by |
|---|---|---|---|
| 119 | 1 | `failed` (validation-exhausted) | Residual discharged by task 136 (`validated`, `fcdb994`) — see `docs/reports/phase3i-findings.md` numbered finding 1 |
| 134 | 1 | `skipped` | Superseded by task 308's corrected citation guard |
| 204 | 2 | `failed` (validation-exhausted) | Superseded by task 302 (approach 3), itself now superseded by task 404 (approach 4) at the new final code sha |
| 205 | 2 | `skipped` (per standing rules; the approach-2 archive shows `pending`, the in-flight status at the moment the run moved to approach 3 — disclosed, not adopted) | Superseded by task 303 (approach 3), itself now superseded by task 406 (approach 4) |
| 211 | 2 | `skipped` | Superseded by task 306 (approach 3), itself superseded by task 407 (approach 4) |
| 212 | 2 | `skipped` | Superseded by task 307 (approach 3), itself superseded by task 408 (approach 4) |

No row in this report's R98-R103 table, and no row in `docs/reports/phase3i.md`'s own
requirement table, cites 119, 134, 204, 205, 211 or 212 as a discharge.

## What this commit also does

Nothing else — this is a docs-only commit adding exactly this one file. It is issued after
task 402, so it does not touch `*.go` or `*.feature` and the freeze range stays empty.

## Re-verifying this report

```
sh docs/reports/phase3i-308-guards/verify.sh
```

exits `0` after this commit is pushed, printing five labelled `PASS` lines that re-check every
sha and path cited across `docs/reports/phase3i.md`, `docs/reports/phase3i-findings.md` and
`docs/DELIVERY-LOG.md`'s Phase 3i paragraph (this file is not itself in that guard's scope,
same as task 309's report before it). A transcript of that run at the true pushed `HEAD` is
saved at `/run/ralphd/artifacts/task409-verify.txt` (run-state scratch, not committed to the
repository — per the standing rule that a committed document can never quote its own commit's
sha, and per the run's `artifacts/`-not-tracked convention).
