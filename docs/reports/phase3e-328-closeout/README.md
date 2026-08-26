# Task 328 — final close-out: protected-path verification by sha, clean build/vet/gofmt, clean git state

This is the close-out task for Phase 3e (R52-R58, plus steer 3e-001's R59-R62). Every other task in
`tasks.json` is `completed` except `334` (steer 3e-002's settle-race fix, pending, unrelated to this
task's checks — it touches only `features/*_test.go` and does not modify SPEC.md/prds/ci/Dockerfile
/ci/SPIKE.md, so it cannot affect the protected-path result below however it lands).

HEAD at the time this task ran: `c4ad2bb` (`docs: write phase3e-findings.md (327)`).

## 1. Git hygiene

```
$ git status --short
(empty)
$ git log origin/main..HEAD
(empty)
```

Both empty at `c4ad2bb` — everything is committed and pushed to `origin/main`.

## 2. Build / vet / gofmt

```
$ ci/run.sh go build ./...
(no output, exit 0)
$ ci/run.sh go vet ./...
(no output, exit 0)
$ ci/run.sh sh -c "gofmt -l $(git ls-files '*.go')"
(no output, exit 0)
```

315 tracked `.go` files checked by the `gofmt -l` invocation (`git ls-files '*.go' | wc -l` = 315);
zero of them are listed as needing formatting. Raw logs (all three empty, confirming clean):
`build.log`, `vet.log`, `gofmt.log` (this directory).

## 3. Protected-path check — by sha only, never by author/committer identity

Command (task 328's own literal formulation, using the PRD commit `791e089` as the range anchor so
the check covers the whole run's history, same shape as `docs/reports/phase3d-212-closeout/README.md`):

```
$ git log 791e089~2..HEAD --format='%h %s' -- SPEC.md prds/ ci/Dockerfile ci/SPIKE.md
2c56309 SPEC: probe.miss is a column not an event, events index+retention, dialogs load via tea.Cmd (operator)
791e089 prds: cut Phase 3e — list ergonomics and chrome (operator)
6584299 spec: configurable sort order, single-click interactive entry, shared seam focus, open built-in theme set (operator)
```

Full raw output plus a per-path breakdown: `protected-path-check.log` (this directory).

**Three shas appear, not two.** Task 328's success criteria text was written expecting exactly
`6584299` and `791e089` — that was accurate when the task was authored, but steer 3e-001 (mid-run)
authorized a further SPEC edit, `2c56309` ("SPEC: probe.miss is a column not an event, events
index+retention, dialogs load via tea.Cmd (operator)", subject ends `(operator)` per the steer's own
convention), landed and pushed by this run on the operator's explicit instruction (steer 3e-001 §4).
The standing-rules block in `notes.md` was updated at that time to record the **recognised set** as
all three: `6584299` (SPEC amendment for R52-R58), `791e089` (the PRD cut), and `2c56309` (steer
3e-001's authorised SPEC edit for R59-R62). This mirrors the exact precedent in
`docs/reports/phase3d-212-closeout/README.md` §4, where a task's literal sha-count formulation was
written before a legitimate mid-run operator SPEC push and had to be read against the *recognised
set*, not the stale literal count, once that push landed.

Per path, independently:
- `SPEC.md`: two commits, `2c56309` and `6584299` — both in the recognised set.
- `prds/`: one commit, `791e089` — in the recognised set.
- `ci/Dockerfile`: zero commits.
- `ci/SPIKE.md`: zero commits.

Each of the three shas resolves in this repository:

```
$ git cat-file -e 6584299 && echo OK
OK
$ git cat-file -e 791e089 && echo OK
OK
$ git cat-file -e 2c56309 && echo OK
OK
```

**No unrecognised sha appears in the range.** Per the standing instruction ("Any unrecognised sha in
range is reported as a hard failure, not classified"), this was checked as a hard pass/fail: every
sha in the output above is a member of the recognised set (`6584299`, `791e089`, `2c56309`); nothing
else is present. Had a fourth, unrecognised sha shown up, this task would report it verbatim, flag it
as a hard failure, and stop — it would not be classified, explained away, or silently absorbed.

**Protected-path check: PASS** (three legitimate operator commits, sha-verified, nothing else).

## 4. Result

All of task 328's success criteria are met, read against the recognised sha set that supersedes the
task's originally-authored two-sha count (steer 3e-001 §4's `2c56309` push, recorded in `notes.md`'s
standing rules, happened after this task was written into the plan):

- Protected-path check done by sha only — three recognised operator commits, zero unrecognised.
- `go build ./...`, `go vet ./...`, `gofmt -l` on all 315 tracked `.go` files — all clean.
- `git status --short` and `git log origin/main..HEAD` — both empty.

Note for the record (not a gap in this task's own criteria, which say nothing about a full-suite
run): the code tip has moved since task 324/325's full-suite/stability citations — commit `fb9bd71`
(task 325's own fix for the `internal/interactive` goroutine-outlives-test panic) changed
`internal/interactive/resize_test.go` after task 324's cited green run at `7ebafce`. Everything
between `fb9bd71` and this task's HEAD (`c4ad2bb`) is docs-only (`git diff --name-only
fb9bd71..HEAD` touches only `docs/reports/phase3e-findings.md` and `docs/reports/phase3e.md`), so
`fb9bd71` remains the true current code tip; task 325's own report already discloses that its
published 7/10 rate was measured before that fix and was deliberately not re-run after it landed
(report-completeness reasoning, not a rate manufacture — see
`docs/reports/phase3e-stability/README.md`). This task does not require or perform a fresh
whole-suite run; it is flagged here only so a future reader is not surprised that the cited
full-suite/stability shas are one code commit behind the actual tip.
