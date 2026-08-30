# Task 813 — guard re-verification at the run's true final sha

Starting sha for this task: **`b1dbfc4`** (HEAD after task 812,
`docs: publish ci/stability.sh 10's observed 0/10 rate at the final code
commit, root-caused (task 812)`), clean, `== origin/main`. Every claim below
is scoped to the run range `1cfbd5a..HEAD` (`1cfbd5a` is the run's own base
sha) — **never to full repository history**, which is what sank task 113
(see the standing rules' own citation of that lesson).

One log per guard below, each carrying its exact command and an exit status
captured in the same shell call. Because a commit cannot quote its own sha,
a follow-up addendum commit names this bundle's own final sha and re-shows
the clean-tree / HEAD-equals-origin/main pair at it (per this task's own
criteria).

## (a) Protected paths — `guard-a-protected-paths.log`

```
$ git log --all --oneline 1cfbd5a.. -- SPEC.md prds/ ci/Dockerfile ci/SPIKE.md
b69b5ba docs: amend SPEC.md §11.8 for R93's visible in-progress selection (task 205)
exit=0
```

Exactly one commit, `b69b5ba` — steering 018's licensed SPEC §11.8 amendment
for R93 — and nothing else, over the run range `1cfbd5a..HEAD`. This is a
claim about that range only: 30 pre-base commits touch these protected paths
outside it, and no claim is made about those.

## (b) Clean tree — `guard-b-clean-tree.log`

```
$ git status --porcelain
exit=0
```

Empty, at starting sha `b1dbfc4`, run before this task's own directory was
created (so it is not an artefact of this task's own untracked work).

## (c) HEAD == origin/main — `guard-c-head-vs-origin.log`

```
$ git rev-parse HEAD
b1dbfc494d6c13ed0c5a3413cac793b621104234
exit=0

$ git log --oneline -1 origin/main
b1dbfc4 docs: publish ci/stability.sh 10's observed 0/10 rate at the final code commit, root-caused (task 812)
exit=0
```

Both name `b1dbfc4` at the starting point. (This task's own commit and its
push advance both together, as a consequence of completing, not a property
of the starting sha this guard is about — restated at the true final sha in
the addendum commit below.)

## (d) `git diff --stat` since task 812's stability sha — `guard-d-diffstat-since-812-sha.log`

Task 812's own report (`docs/reports/phase3g-812-stability10/README.md`)
names **`17b1649`** as "HEAD at launch and at commit time" — the code state
`ci/stability.sh 10` actually ran against, before task 812's own docs-only
commit (`b1dbfc4`) landed. That is "task 812's stability sha" this task's
own criterion refers to.

```
$ git diff --stat 17b1649..HEAD -- '*.go' '*.feature' '*.sh' '*.toml' go.mod go.sum
exit=0
```

Empty: task 812's own commit touched only files under
`docs/reports/phase3g-812-stability10/`, none matching these globs, so the
code+CI tree `ci/stability.sh 10` exercised is byte-identical to the current
tree. (This is a different, narrower question than task 812's own report
answered — that report checked the same globs against `fdf4507`, the last
commit to touch `.go`/`.feature` code, and found one non-empty line, a
docs-evidence `.sh` script from task 810 round 2 that happens to match the
unrestricted `'*.sh'` glob though it is not product or CI code. This guard
uses `17b1649` — the sha task 812's own report names as its literal "code
sha" — which case task 812 also confirmed is trivially empty since it's a
commit diffed against itself once HEAD was still `17b1649`; here HEAD has
since advanced to `b1dbfc4`, docs-only, so it remains empty.)

## (e) `features/godog_test.go`'s `defaultTags` — `guard-e-godog-defaulttags.log`

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

Byte-identical to task 606's own re-verification of this same guard at its
own, much earlier, HEAD (`2bad935`): the only two hunks in the entire run
range still add step registrations for two feature files added early in the
run (`terminal_repair_field_route.feature`, `interactive_pipe_leak.feature`);
none of tasks 701–812 touched this file again. `defaultTags` itself is
byte-unchanged: `~@real-agents && ~@nightly`.

## (f) Every `t.Skip(` added in `1cfbd5a..HEAD` — `guard-f-tskip-audit.log`

```
$ git diff 1cfbd5a..HEAD -- '*.go' '*.feature' '*.sh' '*.toml' | grep -n '^+.*t\.Skip('
9123:+		t.Skip("this theme's key and hint tokens share a colour; the prose-vs-key distinctness assertion below would be vacuous")
9193:+		t.Skip("this theme's key and hint tokens share a colour; the prose-vs-key distinctness assertion below would be vacuous")
9736:+		t.Skip("this theme's key and hint tokens share a colour; the prose-vs-key distinctness assertion below would be vacuous")
11803:+		t.Skip("this theme's key and hint tokens happen to share a colour; the distinctness assertion below would be vacuous")
14471:+		t.Skip("this theme's key and hint tokens share a colour; the prose-vs-key distinctness assertion below would be vacuous")
exit=0
```

Widened to every `.go`/`.feature`/`.sh`/`.toml` file (not just `*_test.go`,
per this task's own criterion) — the result is unchanged: all five hits are
in Go test files, none in a `.feature`/`.sh`/`.toml` file. Mapped to file and
test by line-number lookup against `git diff`'s own `+++ b/...` headers, then
confirmed by `grep -n 't.Skip(\|^func Test'` against each named file at
HEAD:

| # | file | test | guard text | line (HEAD) |
|---|---|---|---|---|
| 1 | `internal/tui/archive_delete_confirm_theme_test.go` | `TestArchiveConfirmTokensMatchSpec` | "share a colour" | 159 |
| 2 | `internal/tui/archive_delete_confirm_theme_test.go` | `TestDeleteConfirmTokensMatchSpec` | "share a colour" | 229 |
| 3 | `internal/tui/bulk_delete_theme_test.go` | `TestBulkDeleteConfirmTokensMatchSpec` | "share a colour" | 305 |
| 4 | `internal/tui/env_editor_theme_test.go` | `TestEnvViewClosingHintKeysAreKeyToken` | "happen to share a colour" | 279 |
| 5 | `internal/tui/profile_pin_restart_theme_test.go` | `TestProfileSwitchTokensMatchSpec` | "share a colour" | 202 |

All five are the same class of guard: `if keyHex == hintHex { t.Skip(...) }`.
Each test asserts that a footer legend's bound-key word renders in
`theme.Key` while the surrounding prose renders in `theme.Hint` — i.e. that
the two tokens are visually distinct. If the active built-in theme's `Key`
and `Hint` colours happen to be identical (or quantise to the same hex),
that one comparison has nothing to distinguish and would pass or fail on a
tautology rather than any real rendering behaviour, so it is skipped; every
other assertion in each test (title/dimmed/key-vs-prose, etc.) still runs
unconditionally regardless of this guard. `git show 1cfbd5a:<path>` exits
128 for all four files (confirmed above) — every one is a brand-new file
added within the run range, so all five additions are genuinely new, not
edits to a pre-existing skip.

**Difference from task 606's own audit of this same guard.** Task 606's
report (`docs/reports/phase3g-606-guards/README.md`, guard (f)) named the
same five raw grep lines but enumerated only four files/tests (missing
`internal/tui/profile_pin_restart_theme_test.go`) — because
`profile_pin_restart_theme_test.go` did not exist yet at task 606's own HEAD
(`2bad935`); it was added by a later task in this run, still within
`1cfbd5a..HEAD`, before task 813. This report's enumeration is the current,
complete one; task 606's is left as historical record for its own HEAD, not
corrected retroactively.

None of the five disables a test outright: each still runs its non-vacuous
assertions unconditionally, and only skips the single comparison that would
be tautological under the specific colour coincidence the guard checks for.

## (g) Citation sweep — `guard-g-citation-sweep.log`, `citation_sweep.py`

Re-run at this task's starting sha (`b1dbfc4`) over `docs/reports/
phase3g.md` and `docs/reports/phase3g-findings.md` — the exact same script
task 606 wrote (`citation_sweep.py`, copied verbatim into this directory,
byte-identical to `docs/reports/phase3g-606-guards/citation_sweep.py`,
confirmed by `diff`), unmodified: it is repo-root-relative and re-reads the
two files fresh, so it needed no code change to check the current, larger
document state (both files grew substantially across tasks 809/810/812).

```
$ python3 docs/reports/phase3g-813-guards/citation_sweep.py
=== 1. sha citations (single-backtick spans only; fenced blocks excluded) ===
134 unique candidate shas across both files
non-resolving shas: (none)

=== 2. markdown link targets ===
207 link targets checked
unresolved: (none)

=== 3. backtick-quoted repository paths (single-backtick spans only) ===
464 candidate backtick paths checked
[... every candidate unresolved by the fixed base-directory list is then
     manually disposed below ...]
STILL UNRESOLVED AFTER MANUAL DISPOSITION (real defects): ['/run/ralphd/approaches/NN/tasks.json', '9/10']
exit=0
```

Full output, including every disposition, is in `guard-g-citation-sweep.log`.

**The two items left "unresolved after manual disposition" are not
defects**, and match exactly what task 606's own report already recorded as
this script's permanent, known-benign residue (see the standing rules'
citation of this fact and task 810's notes): `/run/ralphd/approaches/NN/
tasks.json` is a literal placeholder path quoted in prose (the `NN` is not a
real path segment — no directory named `NN` exists), and `9/10` is a bare
ratio the sweep's path-shaped-string heuristic (`looks_like_path`, matching
on `/`) mistakes for a path; neither is a citation of a real path that
should resolve. Both predate this task and are unchanged by it.

**Known self-citation false-positive class, disclosed** (class A, verbatim
from `citation_sweep.py`'s own docstring — the authoritative, re-runnable
source): *"the two reports citing each other and citing their own filenames
in prose describing the sweep itself (`phase3g.md`/`phase3g-findings.md`
referencing each other, or a report naming its own citation-checker
command) — a sweep that greps a report's own prose flags the report
describing itself."* The other disclosed classes (B deliberate-wrong-path
callout, C `/run/ralphd/...` bare references, D bare abbreviation/glob/
extension-as-label, E a report's own log filename abbreviated once its
subdirectory is named nearby, F the `001-202` operator-ruling identifier)
are unchanged from task 606's own disclosure and are not restated in full
here; the full docstring is in `citation_sweep.py` itself, in this
directory.

Every R76–R93 section's cited test/scenario files remain backtick-quoted
repository paths already covered by class 3 of this same sweep; none of
them appear in the unresolved list at any stage.

## Reproducing this bundle

```
cd <repo root>
python3 docs/reports/phase3g-813-guards/citation_sweep.py           # (g)
git log --all --oneline 1cfbd5a.. -- SPEC.md prds/ ci/Dockerfile ci/SPIKE.md   # (a)
git status --porcelain                                              # (b)
git rev-parse HEAD; git log --oneline -1 origin/main                # (c)
git diff --stat 17b1649..HEAD -- '*.go' '*.feature' '*.sh' '*.toml' go.mod go.sum   # (d)
git diff 1cfbd5a..HEAD -- features/godog_test.go                    # (e)
git diff 1cfbd5a..HEAD -- '*.go' '*.feature' '*.sh' '*.toml' | grep -n '^+.*t\.Skip('   # (f)
```

## Addendum (this bundle's own final sha)

Pending — added by a follow-up commit once this bundle's own commit sha is
known, per this task's own criterion ("because a commit cannot quote its
own sha, a follow-up addendum commit names the bundle's own final sha and
re-shows the clean-tree and HEAD-equals-origin/main pair at it").
