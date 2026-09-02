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
therefore captured **before** this revision's own commit, at parent commit
`9f7df0a87482f0cba9b344679522e6dbb7d4c5f3` — the tip of `main` this revision started
from — with the revised `verify.sh` held outside the live worktree (`/tmp/verify-rev.sh`)
and executed against a throwaway `git worktree add --detach` checkout of that same
commit, so `git status --porcelain` reported the committed tree, not the authoring
scratch. That commit's own sha is exactly one line below (check 4's
`HEAD origin/main` pair): after this task's commit is made and pushed, `HEAD` (and
`origin/main`) will read a **different**, newer sha there — every other check's PASS
text is invariant under a docs-only commit that touches nothing checks 1/2/5 scan, and
check 3 (clean worktree) reads exactly the same "empty" immediately after the push, since
nothing is left uncommitted. Re-running the script at any later HEAD reproduces the same
five PASS lines with only that one sha value advancing.

```
$ sh docs/reports/phase3i-308-guards/verify.sh
PASS: check 1: all 55 distinct cited shas resolve (git cat-file -e)
PASS: check 2: all 98 distinct cited paths pass git ls-files --error-unmatch
PASS: check 3: git status --porcelain is empty
PASS: check 4: git rev-parse HEAD origin/main agree (9f7df0a87482f0cba9b344679522e6dbb7d4c5f3)
PASS: check 5: git log --oneline 3090b68..HEAD -- SPEC.md prds ci/Dockerfile ci/SPIKE.md is empty
ALL GUARDS OK at 9f7df0a87482f0cba9b344679522e6dbb7d4c5f3
```
(exit status `0`, captured at parent commit `9f7df0a87482f0cba9b344679522e6dbb7d4c5f3`.)

## What each check actually asserts

1. **Every sha cited across the three documents resolves as a commit**
   (`git cat-file -e "<sha>^{commit}"`). "Cited" is read as widely as the documents
   actually cite: the scan is a single `tr -c '0-9a-f' '\n'` over all three documents, so
   **every** maximal run of 7-40 lowercase hex characters is checked, whatever punctuation
   surrounds it — a sha quoted alone (`` `a559e7c` ``), **either endpoint of a quoted
   commit range** (`` `de90a5c..HEAD` ``, `` `55e04e6..17cc346` ``), and an
   **unbackticked** sha inside quoted git output or prose (`commit
   3090b68e990bfb063150cbb46f4c3a93bf574883`, "landed in 6197b53"). 55 distinct shas at
   the time of the capture above — the two that a backtick-only scan misses are
   `de90a5c` (range endpoint, `docs/reports/phase3i-findings.md`'s finding §2) and the
   unbackticked 40-hex `3090b68e990bfb063150cbb46f4c3a93bf574883` in the `git show --stat`
   block quoted in the same finding. The cost of scanning that widely is that a 7+ digit
   decimal number or a 7+ letter all-of-`a-f` word in these documents would also be
   treated as a cited sha and reported `FAIL`; that is the deliberate trade (a false FAIL
   is loud and fixable, a missed broken sha is silent) and no such token exists in the
   three documents at the capture above.
2. **Every path cited is tracked** (`git ls-files --error-unmatch`). "Cited" is again read
   as widely as the documents actually cite: **every backtick span is split on whitespace
   and every resulting token is considered**, so a path quoted inside a multi-token command
   span is checked exactly like a path quoted on its own — `` `ci/run.sh go test -count=1
   ./internal/tui/` `` contributes both `ci/run.sh` and `./internal/tui/`. Markdown link
   targets (every non-anchor `](target)`, not only `reports/`- and `phase3i`-prefixed ones)
   are checked too. A token counts as path-shaped when, after wrapping quotes, trailing
   prose punctuation and any `:NNN`/`:NNN-NNN` line-range suffix are stripped, it contains a
   `/`, or ends in one of this repo's extensions, or is a `phase3i*`/`phase3h*` report-dir
   shorthand; resolution falls back to a same-directory-relative form for the two source
   documents that live in `docs/reports/` and for `docs/DELIVERY-LOG.md`'s own
   `docs/`-relative links, then to a basename search for bare filenames used as markdown
   link *labels* whose real path is established elsewhere in the same document (e.g.
   `` `ownership.go` `` for `internal/tmux/ownership.go`). Five documented exclusions, each
   path-shaped but not a path in this repo: `tasks.json` and any `/run/ralphd/...` token
   (the run's own external state, quoted in prose about the run itself), `origin/main` (a
   git ref, quoted inside check 4's own command), `./...` (the Go package pattern), any
   token holding a shell/placeholder metacharacter such as `2>&1` or `~/.git-credentials`
   (a real path, but in `$HOME`), and a digits-and-slashes-only token such as a `10/10`
   ratio. **98** distinct paths at the time of the capture above — six more than the
   earlier revision's 92, which extracted only backtick spans containing no spaces and so
   never checked `ci/run.sh` or `ci/stability.sh` at all despite their nine citations
   across `docs/reports/phase3i.md` and `docs/reports/phase3i-findings.md` (see the check-2
   negative control below).
3. **`git status --porcelain` is empty** — nothing modified, staged or untracked.
4. **`git rev-parse HEAD origin/main` agree** — the branch is pushed.
5. **`git log --oneline 3090b68..HEAD -- SPEC.md prds ci/Dockerfile ci/SPIKE.md` is
   empty** — the worker-write protected-path range (standing rules) stays clear.

## Negative control for check 1 (why the widened scan is load-bearing)

Run at `fb78352ff2fc935e2c790b832913ebc7fb1a9869`, against a scratch file in `/tmp` (never
against the repository tree), with one good and one deliberately corrupted range endpoint:

```
$ cat /tmp/negctl.md
phase 3h disclosed the same shape for `de90a5c..HEAD` and for a typo `de90a5d..HEAD`.
$ grep -oh '`[0-9a-f]\{7,40\}`' /tmp/negctl.md | tr -d '`' | sort -u     # the old, backtick-only scan
$ # (no output at all: 0 tokens — both range citations were invisible to it)
$ cat /tmp/negctl.md | tr -c '0-9a-f' '\n' | grep -xE '[0-9a-f]{7,40}' | sort -u   # the scan check 1 now uses
de90a5c
de90a5d
$ git cat-file -e de90a5c^{commit} && echo resolves
resolves
$ git cat-file -e de90a5d^{commit} || echo 'DOES NOT resolve'
DOES NOT resolve
```

So a mistyped sha inside a range citation — the exact shape `docs/reports/phase3i-findings.md`
uses for phase 3h's `de90a5c..HEAD` — now produces a `FAIL: cited sha de90a5d does not
resolve` line and exit status 1, where the earlier backtick-only extraction reported `PASS`.

## Negative control for check 2 (why splitting command spans is load-bearing)

Same method, one revision later and for the path scan: run at
`9f7df0a87482f0cba9b344679522e6dbb7d4c5f3`, inside a throwaway
`git worktree add --detach` checkout of that commit under `/tmp/wt308b` (never the live
tree; removed afterwards), with the first `ci/run.sh` citation in
`docs/reports/phase3i-findings.md` corrupted to a path that does not exist. The two
scripts compared are `/tmp/verify-old.sh` (`git show
9f7df0a:docs/reports/phase3i-308-guards/verify.sh`, the space-free-span extraction) and
`/tmp/verify-rev.sh` (this revision):

```
$ sed -i '0,/ci\/run\.sh go test/s//ci\/run-typo.sh go test/' docs/reports/phase3i-findings.md
$ sh /tmp/verify-old.sh | grep 'check 2'
PASS: check 2: all 92 distinct cited paths pass git ls-files --error-unmatch
$ sh /tmp/verify-rev.sh | grep -e 'check 2' -e run-typo
FAIL: cited path ci/run-typo.sh is not tracked (git ls-files --error-unmatch)
$ git checkout -- docs/reports/phase3i-findings.md && git status --porcelain && echo CLEAN
CLEAN
```

And the extraction difference that causes it, over the same three documents at that same
commit (`$D` = the two reports plus the extracted Phase 3i paragraph):

```
$ grep -oh '`[^` ]*`' $D | tr -d '`' | sort -u | grep '^ci/'          # old: space-free spans only
ci/Dockerfile
ci/SPIKE.md
$ grep -oh '`[^`]*`' $D | tr -d '`' | tr -s ' \t' '\n' \
    | sed "s/^['\"]*//; s/['\",;)]*\$//" | sort -u | grep '^ci/'      # new: spans split on whitespace
ci/Dockerfile
ci/SPIKE.md
ci/run.sh
ci/stability.sh
```

So the nine `ci/run.sh` / `ci/stability.sh` citations — which appear only ever inside
multi-token command spans like `` `ci/run.sh go test -p=1 -count=1 ./...` `` and
`` `ci/stability.sh 10` `` — went from unchecked (a stale one would have passed silently)
to checked, and a broken one now fails the guard.

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
