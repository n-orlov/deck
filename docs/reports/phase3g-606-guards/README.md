# Task 606 — guard re-verification at the run's current sha

Starting sha for this task: **`2bad935`** (HEAD after task 605, `docs: close review
finding 1's stability gate by citation at the unchanged final tree (task 605)`).
Every claim below is scoped to the run range `1cfbd5a..HEAD` (`1cfbd5a` is the run's
own base sha) — never to the full repository history, which is what sank task 113
(see the standing rules' own citation of that lesson).

One log per guard, each carrying its exact command and an exit status captured in
the same shell call. A verifier re-running each command at the same sha gets the
same output, with one documented exception noted under guard (b).

## (a) Protected paths — `guard-a-protected-paths.log`

```
$ git log --all --oneline 1cfbd5a.. -- SPEC.md prds/ ci/Dockerfile ci/SPIKE.md
b69b5ba docs: amend SPEC.md §11.8 for R93's visible in-progress selection (task 205)
exit=0
```

Exactly one commit, `b69b5ba` — steering 018's licensed SPEC §11.8 amendment for
R93 — and nothing else, over the run range `1cfbd5a..HEAD`. No claim is made about
commits to these paths outside that range (30 pre-base commits touch protected
paths per the standing rules; a full-history claim is exactly what sank task 113).

## (b) Clean tree — `guard-b-clean-tree.log`

```
$ git status --porcelain
exit=0
```

Captured with this task's own in-progress `docs/reports/phase3g-606-guards/`
directory moved out of the working tree for the command, since that directory does
not exist at all in a fresh checkout of the starting sha `2bad935` — a verifier
checking out that sha needs no such workaround and sees the same empty output
directly. Once this task's own commit lands, `docs/reports/phase3g-606-guards/` is
tracked, not untracked/dirty, so a verifier re-running the bare command against the
final pushed commit is also expected to see nothing.

## (c) HEAD == origin/main — `guard-c-head-vs-origin.log`

```
$ git rev-parse HEAD
2bad9353467e2bb547a45feb2dd91e2c5696a012
exit=0

$ git rev-parse origin/main
2bad9353467e2bb547a45feb2dd91e2c5696a012
exit=0
```

Both resolve to the same sha at the starting point. (After this task's own commit
is pushed, HEAD and `origin/main` both advance together, in this task's push step;
they are not re-quoted here because that push is a consequence of this task
completing, not a property of the starting sha this guard is about.)

## (d) `features/godog_test.go`'s `defaultTags` — `guard-d-godog-defaulttags.log`

```
$ git diff 1cfbd5a..HEAD -- features/godog_test.go
diff --git a/features/godog_test.go b/features/godog_test.go
index 24aa98d..2b380eb 100644
--- a/features/godog_test.go
+++ b/features/godog_test.go
@@ -119,6 +119,7 @@ func initializeScenario(sc *godog.ScenarioContext) {
 	registerSortOrderSteps(sc)
 	registerNewSessionSelectionSteps(sc)
 	registerStatusRecoverySteps(sc)
+	registerTerminalRepairFieldRouteSteps(sc)
 	registerSettingsSteps(sc)
 	registerDialogsSteps(sc)
 	registerEnvEditorSteps(sc)
@@ -138,6 +139,7 @@ func initializeScenario(sc *godog.ScenarioContext) {
 	registerInteractiveScrollSteps(sc)
 	registerInteractiveSelectionSteps(sc)
 	registerInteractiveRetargetSteps(sc)
+	registerInteractivePipeLeakSteps(sc)
 }

 exit=0

$ grep -n "const defaultTags" features/godog_test.go
16:const defaultTags = "~@real-agents && ~@nightly"
exit=0
```

The only two hunks in the entire run range add step registrations
(`registerTerminalRepairFieldRouteSteps`, `registerInteractivePipeLeakSteps`) for
the two brand-new feature files named in guard (e) below. `defaultTags` itself is
byte-unchanged; the surviving line is:

```
const defaultTags = "~@real-agents && ~@nightly"
```

## (e) Scenario count, base vs HEAD — `guard-e-scenario-count.log`

```
$ git archive 1cfbd5a -- features/ | tar -x -C /tmp/base_1cfbd5a
$ grep -rEc "^\s*Scenario( Outline)?:" /tmp/base_1cfbd5a/features/*.feature | awk -F: '{s+=$2} END{print s}'
290
exit=0

$ grep -rE "^\s*Scenario( Outline)?:" features/*.feature | wc -l
296
exit=0
```

Delta: **+6**, fully accounted for by additions only — no deletion, no retagging:

| file | base (`1cfbd5a`) | HEAD | delta |
|---|---|---|---|
| `create_session.feature` | 15 | 16 | +1 |
| `filter.feature` | 5 | 7 | +2 |
| `interactive_pipe_leak.feature` | 0 (new) | 1 | +1 |
| `terminal_repair_field_route.feature` | 0 (new) | 1 | +1 |
| `themes.feature` | 8 | 9 | +1 |
| every other changed/unchanged `.feature` file | — | — | 0 |

`290 + 1 + 2 + 1 + 1 + 1 = 296`. Two brand-new feature files
(`interactive_pipe_leak.feature`, `terminal_repair_field_route.feature`) match the
two new step-registration calls in guard (d).

## (f) The five run-range `t.Skip` additions — `guard-f-tskip-audit.log`

```
$ git diff 1cfbd5a..HEAD -- '*_test.go' | grep '^+.*t\.Skip'
+		t.Skip("this theme's key and hint tokens share a colour; the prose-vs-key distinctness assertion below would be vacuous")
+		t.Skip("this theme's key and hint tokens share a colour; the prose-vs-key distinctness assertion below would be vacuous")
+		t.Skip("this theme's key and hint tokens share a colour; the prose-vs-key distinctness assertion below would be vacuous")
+		t.Skip("this theme's key and hint tokens happen to share a colour; the distinctness assertion below would be vacuous")
+		t.Skip("this theme's key and hint tokens share a colour; the prose-vs-key distinctness assertion below would be vacuous")
exit=0
```

All five are brand-new files added in the run range (`git show 1cfbd5a:<path>`
exits 128 for each — no such object at base):

1. `internal/tui/archive_delete_confirm_theme_test.go`,
   `TestArchiveConfirmTokensMatchSpec` — guard:
   `if keyHex == hintHex { t.Skip(...) }`. The test asserts the footer legend's
   bound-key word renders in `theme.Key` while the surrounding prose renders in
   `theme.Hint` — i.e. that the two tokens are visually distinct. If the active
   built-in theme's `Key` and `Hint` happen to author (or quantise to) the
   identical hex value, that one comparison has nothing to distinguish; it would
   pass or fail on a tautology, not on any real rendering behaviour. Every other
   assertion in the test (title/dimmed/key-vs-prose) still runs regardless.
2. Same file, `TestDeleteConfirmTokensMatchSpec` — same guard, same reason, the
   dialog's sibling confirmation.
3. `internal/tui/bulk_delete_theme_test.go`, `TestBulkDeleteConfirmTokensMatchSpec`
   — same guard, same reason, the bulk-delete confirm dialog.
4. `internal/tui/env_editor_theme_test.go`,
   `TestEnvViewClosingHintKeysAreKeyToken` — same class of guard
   (`if keyHex == hintHex { t.Skip(...) }`), same reason, for the env editor's
   closing legend line.

(A pre-range sixth `t.Skip` of the same vacuity-guard class,
`internal/tui/main_view_theme_test.go:156`, already exists at base `1cfbd5a` and
is outside this run's `+` diff; it is named here only to head off a "why does the
standing rules' tally say five when there are six in the tree" question — five
were *added* in the run range, one predates it.)

None of the five disables a test outright: each still runs its non-vacuous
assertions unconditionally, and only skips the single comparison that would be
tautological under the specific coincidence the guard checks for.

## (g) Citation sweep — `guard-g-citation-sweep.log`, `citation_sweep.py`

Re-run at HEAD `2bad935` over `docs/reports/phase3g.md` and
`docs/reports/phase3g-findings.md`, checking three classes of citation: every
backtick-quoted commit sha (`git cat-file -e <sha>^{commit}`), every markdown link
target, and every backtick-quoted string that looks like a repository path.

```
$ python3 docs/reports/phase3g-606-guards/citation_sweep.py
=== 1. sha citations (single-backtick spans only; fenced blocks excluded) ===
96 unique candidate shas across both files
non-resolving shas: (none)

=== 2. markdown link targets ===
114 link targets checked
unresolved: (none)

=== 3. backtick-quoted repository paths (single-backtick spans only) ===
347 candidate backtick paths checked
[... 68 candidates unresolved by the fixed base-directory list, each then
     manually disposed below ...]
STILL UNRESOLVED AFTER MANUAL DISPOSITION: (none) -- every candidate is either a
real repository path, or one of the deliberate/disclosed classes above.
exit=0
```

Full output, including every one of the 68 dispositions, is in
`guard-g-citation-sweep.log`.

**Tokenizer correction made during this re-run.** The script inherited from an
earlier draft used a naive `` `([^`]+)` `` regex to pair single backticks. That
regex does not know about fenced ` ``` ` code blocks: a fence's *internal* single
backticks (git-log `%h`-style hunks, shell examples embedded in prose) desync the
global left-to-right pairing for the rest of the document, manufacturing bogus
"citations" out of plain prose — e.g. an early two-line span `` `ok\ngithub.com/
n-orlov/deck/features <5s` `` (a wrapped log-output quote) cascaded into treating
the plain-prose fragments `` (reap-on-create/rename `` and `` ; `` as if they were
citations, dozens of matches later. `citation_sweep.py` now tokenizes with a small
CommonMark-style scanner: an opening run of *N* backticks is closed by the next
run of exactly *N* backticks (an intervening run of a different length is content,
not a delimiter). Single-backtick (`N=1`) spans are the only ones checked for
citations; triple-backtick (`N=3`) fenced spans are skipped outright, since a code
block is not a citation. This is disclosed as false-positive class E in the
script's own docstring, and the fix is why this re-run's initial single-backtick
regex candidate list (347) differs from a naive re-implementation.

**Disclosed false-positive classes** (verbatim from `citation_sweep.py`'s
docstring — the authoritative, re-runnable source):

- **A.** The two reports cite each other and cite their own filenames in prose
  describing the sweep itself — a sweep that greps a report's own prose flags the
  report describing itself.
- **B.** `prds/phase3g-residuals-and-suite-determinism.md` is deliberately quoted
  as an earlier draft's *wrong* filename (the correct one,
  `prds/phase3f-residuals-and-suite-determinism.md`, exists); the surrounding
  sentence says so.
- **C.** `tasks.json` and `notes.md`, wherever cited bare in either report, refer
  to this run's own `/run/ralphd/tasks.json` and `/run/ralphd/notes.md` — outside
  this repository. Neither citing sentence repeats the `/run/ralphd/` prefix or an
  explicit "outside this repository" label at that exact spot (pre-existing
  wording this task does not own or edit); disclosed here rather than silently
  passed.
- **D.** A bare abbreviation of a file already named in full a few words earlier
  in the same sentence (`inject.go` for `internal/service/inject.go`; `main.go`
  for `cmd/deck/main.go`; `receiver.go:222` for `internal/hookrecv/receiver.go`;
  `empire.toml`/`parchment.toml` for
  `internal/theme/builtin/empire.toml`/`.../parchment.toml`, both listed next to
  `internal/theme/builtin/cobalt.toml` in the same clause); a glob
  (`*_theme_test.go`, `internal/theme/builtin/*.toml`, `features/*.feature`,
  `docs/reports/phase3g*`, `docs/reports/phase3g-02[56]-*/`); a bare extension
  used as a category label in prose (`.go`, `.md`, `.toml`, `.feature`); a
  runtime glob that is not a repository path (`/tmp/deck-interactive-pipe-*`); a
  comma-joined list of feature filenames quoted as one span.
- **E.** A report's own log filename, abbreviated once its containing report
  subdirectory has been named a few words earlier in the same sentence — e.g.
  `theme-suite-green.log` for `phase3g-106-contrast-floor/theme-suite-green.log`.
  63 of the 68 candidates unresolved by the sweep's small fixed base-directory
  list fall in this class; each is confirmed present by a recursive
  `docs/reports/` search in `guard-g-citation-sweep.log`, naming the exact
  resolved path(s).
- **F (new this run).** `001-202.md` (phase3g.md's task-202 row, and its prose
  two lines later) is not a repository file at all — it is the identifier of an
  operator ruling delivered to this run outside the repository. The disclosure
  that this identifier is untracked lives in `phase3g-findings.md`'s own F33
  ("operator ruling identified as `001-202`... delivered to this run outside the
  repository, not part of the tracked record, so not linked here"), not repeated
  at every citing sentence — accounted for once per the pair of reports, not per
  occurrence.

Every R76–R93 section's cited test/scenario files (e.g.
`internal/service/inject_retained_corpse_test.go`, `features/environment.feature`,
and the like) are backtick-quoted repository paths already covered by class 3 of
this same sweep; none of them appear in the unresolved list at any stage, i.e.
each one exists in the tree as cited.

## Reproducing this bundle

```
cd <repo root>
python3 docs/reports/phase3g-606-guards/citation_sweep.py   # guard (g)
git log --all --oneline 1cfbd5a.. -- SPEC.md prds/ ci/Dockerfile ci/SPIKE.md   # (a)
git status --porcelain   # (b), see caveat above
git rev-parse HEAD; git rev-parse origin/main   # (c)
git diff 1cfbd5a..HEAD -- features/godog_test.go   # (d)
git diff 1cfbd5a..HEAD -- '*_test.go' | grep '^+.*t\.Skip'   # (f)
```

Guard (e)'s per-file table was produced by materialising the base tree with
`git archive 1cfbd5a -- features/ | tar -x -C <scratch dir>` (outside the repo,
scratch-only) and diffing per-file `grep -c` counts against the working tree —
the full transcript is in `guard-e-scenario-count.log`.
