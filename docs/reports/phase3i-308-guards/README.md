# Phase 3i task 308 — a re-runnable guard for every citation in the approach-3 documents

[`verify.sh`](verify.sh), committed next to this README, re-checks every sha and path
cited by `docs/reports/phase3i.md`, `docs/reports/phase3i-findings.md` and
`docs/DELIVERY-LOG.md`'s Phase 3i paragraph (extracted by its own `**Phase 3i**` marker
up to the next `## ` heading — no other phase's citations in that 794-line file are ever
scanned), plus the run's own worktree/push/protected-path state. Run it yourself:

```
sh docs/reports/phase3i-308-guards/verify.sh
```

It exits 0 only when all five checks below pass, and prints exactly one `PASS: ...` line
per check (or one or more `FAIL: ...` lines naming what broke). No Go, no tmux, no
network — pure POSIX `sh`, `git` and `awk`.

## What "at the current HEAD" can and cannot mean for this report

A document cannot quote the sha of the commit that first adds it: that sha is a hash *of*
the document's own bytes, so writing it in would change it (the standing rule this run
runs under, restated from task 134's guard report in approach 1). The block below was
therefore captured **before** this task's own commit, at parent commit
`3761469a42824c3f0fff434f7ffa043cbf05e82b` — the tip of `main` this task started from —
with `verify.sh` and this README moved out of the worktree during the measurement so
`git status --porcelain` reported the tree this commit actually publishes, not the
authoring scratch. That commit's own sha is exactly one line below (check 4's
`HEAD origin/main` pair): after this task's commit is made and pushed, `HEAD` (and
`origin/main`) will read a **different**, newer sha there — every other check's PASS
text is invariant under a docs-only commit that touches nothing checks 1/2/5 scan, and
check 3 (clean worktree) reads exactly the same "empty" immediately after the push, since
nothing is left uncommitted. Re-running the script at any later HEAD reproduces the same
five PASS lines with only that one sha value advancing.

```
$ sh docs/reports/phase3i-308-guards/verify.sh
PASS: check 1: all 53 distinct cited shas resolve (git cat-file -e)
PASS: check 2: all 92 distinct cited paths pass git ls-files --error-unmatch
PASS: check 3: git status --porcelain is empty
PASS: check 4: git rev-parse HEAD origin/main agree (3761469a42824c3f0fff434f7ffa043cbf05e82b)
PASS: check 5: git log --oneline 3090b68..HEAD -- SPEC.md prds ci/Dockerfile ci/SPIKE.md is empty
ALL GUARDS OK at 3761469a42824c3f0fff434f7ffa043cbf05e82b
```
(exit status `0`, captured at parent commit `3761469a42824c3f0fff434f7ffa043cbf05e82b`.)

## What each check actually asserts

1. **Every sha cited (in backticks, 7-40 hex characters) across the three documents
   resolves as a commit** (`git cat-file -e "<sha>^{commit}"`). 53 distinct shas at the
   time of the capture above.
2. **Every path cited** — backtick-quoted bare paths (stripped of a trailing `:NNN` or
   `:NNN-NNN` line-range suffix) and markdown link targets — **is tracked**
   (`git ls-files --error-unmatch`, falling back to a same-directory-relative
   resolution for the two source documents that live in `docs/reports/` and for
   `docs/DELIVERY-LOG.md`'s own `docs/`-relative links, then to a basename search for
   bare filenames used as markdown link *labels* whose real path is established
   elsewhere in the same document, e.g. `` `ownership.go` `` for
   `internal/tmux/ownership.go`). `tasks.json` and any `/run/ralphd/...` path are
   excluded by name: both are the run's own external state, quoted verbatim in prose
   about the run itself, never asserted as a path inside this repo. 92 distinct paths
   at the time of the capture above.
3. **`git status --porcelain` is empty** — nothing modified, staged or untracked.
4. **`git rev-parse HEAD origin/main` agree** — the branch is pushed.
5. **`git log --oneline 3090b68..HEAD -- SPEC.md prds ci/Dockerfile ci/SPIKE.md` is
   empty** — the worker-write protected-path range (standing rules) stays clear.

## What this guard deliberately does NOT assert

**The PRD's own literal protected-path range, `6197b53..HEAD`, is permanently
non-empty, and that is by design, not a defect this script should ever flag.** That
range contains exactly one commit, the operator's own `3090b68` (the commit that added
this phase's PRD under `prds/`, landing after `6197b53` opens the range) — a fact
history cannot un-happen and no amendment narrows. Making `6197b53..HEAD` empty a pass
condition here would make this guard permanently and correctly-unfixably red, which is
worse than not asserting it: it would train a reader to ignore a red guard rather than
read what it means. The full disclosure — why the literal range is unmeetable, and why
the *worker-write* range `3090b68..HEAD` (check 5 above) is the meaningful, actually-met
substitute — is `docs/reports/phase3i-findings.md`'s numbered finding **§2, "The
protected-path audit range `6197b53..HEAD` necessarily contains one operator commit"**.
Check 5 above is that finding's own worker-write half, made mechanically re-runnable;
the PRD's literal half is reported UNMET everywhere else in this phase's documents and
stays that way here too, by omission, deliberately.
