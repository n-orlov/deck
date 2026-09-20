# Phase 4b findings (task 025)

Companion to [`phase4b.md`](phase4b.md) (the per-requirement evidence report for
R133–R135, R128–R131): what the requirement table does not carry — everything
this run found while landing Tier 1 and Tier 2 and deliberately chose **not**
to fix, plus the non-findings the plan names by name (`docs/reports/phase4b.md`
defers "[r]esidual gaps this run chose not to fix" to this file, in its
"Not shipped / still open" section — cited by section rather than by line
number, since task 024 re-wrote that file at gate sha `baf92ed`).

Written at gate sha `0806ba64ede4356af50b404affd10f26f68d4d84` (task 020, the
last code-touching commit on the GO branch); every commit from task 021's
launch (`924a039`) through this file's own commit is record-only
(`git diff --stat 0806ba6..HEAD -- '*.go' '*.feature'` prints nothing), so
every file:line below resolves identically at `0806ba6` and at this file's own
commit. Every path cited is tracked under `git ls-files --error-unmatch`
(checked below) unless explicitly marked untracked/gitignored.

- [1. Findings — discovered and left alone](#1-findings--discovered-and-left-alone)
  - [1.1 The interactive footer's scroll-mode degradation reimplements, rather than calls, `footerLegendWithin`](#11-the-interactive-footers-scroll-mode-degradation-reimplements-rather-than-calls-footerlegendwithin)
  - [1.2 A pre-existing tmux-attach fifo-timing flake surfaced once during task 017's targeted run](#12-a-pre-existing-tmux-attach-fifo-timing-flake-surfaced-once-during-task-017s-targeted-run)
- [2. Non-findings (named by the plan, disclosed for completeness)](#2-non-findings-named-by-the-plan-disclosed-for-completeness)
  - [2.1 Pre-existing gofmt drift in the three `.spike-preview/` files](#21-pre-existing-gofmt-drift-in-the-three-spike-preview-files)
  - [2.2 The two known-open flake classes named by the PRD](#22-the-two-known-open-flake-classes-named-by-the-prd)
- [3. Tier 2 status](#3-tier-2-status)
- [4. How to re-check every citation in this report](#4-how-to-re-check-every-citation-in-this-report)
- [5. Approach 2 addendum (task 010)](#5-approach-2-addendum-task-010)
  - [5.1 VG-6 — `groups.id` is `INTEGER PRIMARY KEY AUTOINCREMENT`, diverging from `SPEC.md:332`](#51-vg-6--groupsid-is-integer-primary-key-autoincrement-diverging-from-specmd332)
  - [5.2 Task 008's failure list (approach 2 stability10 sweep): none](#52-task-008s-failure-list-approach-2-stability10-sweep-none)
  - [5.3 The `mkfifo`/`osc11` flake instance seen during this approach's planning](#53-the-mkfifoosc11-flake-instance-seen-during-this-approachs-planning)
  - [5.4 gofmt drift, re-checked for approach 2](#54-gofmt-drift-re-checked-for-approach-2)
  - [5.5 How to re-check §5](#55-how-to-re-check-5)

## 1. Findings — discovered and left alone

### 1.1 The interactive footer's scroll-mode degradation reimplements, rather than calls, `footerLegendWithin`

**`internal/tui/tui.go:4695`** (doc comment on `interactiveFooterLine`) states
the R134 scroll-mode footer "degrade[s] the way `footerLegendWithin` degrades
the list-mode legend: whole segments drop, one at a time from the least
essential end". **`internal/tui/tui.go:4706`**, the function body itself,
does not call `footerLegendWithin` (or its sibling `elideToWidth`) to do that
— it re-implements the same drop-from-the-least-essential-end loop inline
(`internal/tui/tui.go:4726-4755`, the `footerSegment`/`segments` literal and
its own `kept`/`used` loop) with
its own three-segment priority list (cue, scroll advertisement, forward note)
plus the mandatory, never-dropped `Ctrl+Q` segment appended after.

This was already caught and disclosed once, in the "Attribution corrections"
section of `docs/reports/phase4b.md` (item 3): the first landing of that
report over-attributed the seam call and a later revision corrected it to
"the added code budgets with `stringWidth` and only *mirrors*
`footerLegendWithin` ... it calls neither." That correction is about the
report's own wording; this entry is the corresponding product-code
disclosure the report pointed at this file to carry.

**Left alone because:** `footerLegendWithin`'s own segment shape (a single
degrading legend string) does not fit `interactiveFooterLine`'s three
independently-droppable, differently-styled segments plus a mandatory
trailing one without either widening `footerLegendWithin`'s own contract (used
by the list-mode footer, a different call site with different priorities) or
adding a second, parallel priority-list abstraction shared by both — a
refactor with no test-observable benefit today: `interactive_footer_scroll_advertisement_test.go`
and `interactive_footer_scroll_cue_test.go` both pass, and the frame budget
this loop protects (80 columns) is exercised by both. Correct behaviour,
duplicated logic; recorded here rather than refactored.

### 1.2 A pre-existing tmux-attach fifo-timing flake surfaced once during task 017's targeted run

**`internal/tui/force_enter_test.go:70`** (`func
TestForceEntersDespiteAnAttachedClient`) failed once during task 017's own
targeted `internal/tui` run (commit `c754c43`, "tui/store/service: the `i`
detail dialog's `g` picker moves a session's group (R130 part 2)" — see its
own commit message: "one internal/tui run hit the pre-existing
`TestForceEntersDespiteAnAttachedClient` fifo-timing flake, reproduced green
in isolation immediately after — not this task's, per notes.md's documented
flake class"). The test drives a real tmux client attach through a private
socket (`attachForceEnterPTY`, `internal/tui/force_enter_test.go:88`) and
polls for the attach to be observed before exercising force-enter; its
flakiness is a fifo/attach-timing race pre-existing this phase, not a
regression task 017 introduced — task 017's own change (`internal/service`,
`internal/tui/rename.go`, `internal/tui/group_move_test.go`) touches none of
the tmux-attach machinery this test exercises.

**Left alone because:** re-ran green in isolation immediately after the one
observed failure (task 017's own evidence trail), and per this run's own
standing gotcha (handoff notes' "Residuals" section): a timing flake that
reproduces green in isolation is named with a log path if hit again, never
chased. It did not recur in either the whole-suite gate (task 021) or the
ten-run stability sweep (task 023) — `grep -il "attach\|force" run-*.log` on
all ten stability logs finds no failure line (checked below, §4) — so there
is nothing currently open to chase beyond this one disclosed occurrence.

## 2. Non-findings (named by the plan, disclosed for completeness)

### 2.1 Pre-existing gofmt drift in the three `.spike-preview/` files

Per the standing rules ("Pre-existing `gofmt` drift in the three
`.spike-preview/` files is left alone and is not a finding") and the PRD
("It is *not* clean on the tree today ... Leave them; naming them in the
report is enough and their presence is not a finding"):

- `.spike-preview/cmd/conformance/main.go`
- `.spike-preview/conformance/conformance.go`
- `.spike-preview/conformance/conformance_test.go`

These three paths are **untracked and gitignored** (`.gitignore:4` —
`.spike-preview/`), so they carry no git blob/line history and no commit sha
pins their content; `git ls-files --error-unmatch` on any of them fails for
exactly that reason (checked in §4), which is why this entry cites paths, not
`file:line`s tied to a commit — the drift is whole-file (gofmt formats a file
as a unit) and the files are the same three today as at every prior guard
check this run took.

**Evidence path:** `docs/reports/phase4b-guards/README.md` (task 022) — its
own `ci/run.sh gofmt -l .` run at gate sha `0806ba6` lists exactly these three
paths and no other file, and separately confirms
`internal/theme/quantize_test.go` — which the PRD claims drifts — is in fact
clean today (`ci/run.sh gofmt -l internal/theme/quantize_test.go` → exit `0`,
empty output). Re-checked here: `ls .spike-preview/cmd/conformance/main.go
.spike-preview/conformance/conformance.go
.spike-preview/conformance/conformance_test.go` (§4) confirms all three still
exist on disk in this tree unchanged.

### 2.2 The two known-open flake classes named by the PRD

Per the PRD ("Two flake classes are known open — the transient-`starting`
assertion and a SIGWINCH exact-count assertion — and instances of those are
advisory, reported with evidence, not chased"):

1. **Transient-`starting`/"not settled" assertion** —
   `features/golden_frame_test.go:85` (`func TestGoldenMinimumFrame`), whose
   settle-retry loop's failure message is at
   `features/golden_frame_test.go:282` (`"frame kept changing after the
   fixture rendered; not settled after %d attempts"`).
2. **SIGWINCH exact-count assertion** —
   `features/sigwinch_count_test.go:24` (`func
   TestSigwinchCountDistinguishesTwoFromThree`), an exact-equality count
   assertion documented at `features/sigwinch_count_test.go:20-23` as
   deliberately exact so neither direction passes by treating the count as a
   floor.

**Evidence path:** `docs/reports/phase4b-stability10/README.md` (task 023,
"Known-open flake classes: not observed this sweep") — both classes were
checked for explicitly across all ten stability-sweep logs
(`grep -il "not settled" run-*.log` and `grep -il "sigwinch" run-*.log`, both
"no match in any of the ten logs") and neither occurred in the whole-suite
gate (task 021) either. Reported here as not-observed-this-run, per the
plan's own instruction, not chased.

## 3. Tier 2 status

Tier 2 was **started and completed** this run — the decision was **GO**,
recorded in full at
[`docs/reports/phase4b-tier2-decision.md`](phase4b-tier2-decision.md) (task
006, commit `16fc86a`). The task-025 criterion that would require recording
"the spec-versus-code grouping gap ... as a disclosed non-finding" applies
only "if Tier 2 was not started" — it was, so that clause does not apply here;
the Tier 1→Tier 2 decision record is cited above for completeness only.

## 4. How to re-check every citation in this report

```
$ git rev-parse HEAD
# (this file's own commit, once committed) — code tree byte-identical to 0806ba6

$ git diff --stat 0806ba6..HEAD -- '*.go' '*.feature'
# (empty)

$ git ls-files --error-unmatch internal/tui/tui.go internal/tui/force_enter_test.go \
    features/golden_frame_test.go features/sigwinch_count_test.go \
    docs/reports/phase4b.md docs/reports/phase4b-guards/README.md \
    docs/reports/phase4b-stability10/README.md docs/reports/phase4b-tier2-decision.md
# every path printed, none erred

$ git ls-files --error-unmatch .spike-preview/cmd/conformance/main.go
# error: pathspec did not match any file(s) known to git -- untracked, confirms §2.1

$ sed -n '4695p;4706p' internal/tui/tui.go
$ sed -n '70p' internal/tui/force_enter_test.go
$ sed -n '85p;282p' features/golden_frame_test.go
$ sed -n '24p' features/sigwinch_count_test.go
# each line matches the quoted text above

$ ls .spike-preview/cmd/conformance/main.go .spike-preview/conformance/conformance.go \
     .spike-preview/conformance/conformance_test.go
# all three exist

$ grep -n "^\.spike-preview/" .gitignore
# 4:.spike-preview/

$ grep -il "not settled\|sigwinch" docs/reports/phase4b-stability10/run-*.log; echo "exit:$?"
exit:1   # no match in any of the ten committed logs -- matches §2.2's claim

$ grep -il "TestForceEntersDespiteAnAttachedClient\|fifo" docs/reports/phase4b-stability10/run-*.log; echo "exit:$?"
exit:1   # no match either -- the §1.2 occurrence was task 017's own targeted
         # run, not the stability sweep; it did not recur there
```

## 5. Approach 2 addendum (task 010)

Everything above (§1-§4) is approach 1's own findings record, written at gate
sha `0806ba6` and left as history per this run's standing rules ("Approach
1's task ids ... are HISTORY, not work"); it is not re-scored here. This
section is approach 2's own addition -- new findings/non-findings this
approach's own work (tasks 001-009) surfaced, on top of, not instead of, §1-§4.

Approach 2's own measurement anchor is task 005's pinned revision,
`70c7430df3b23a46fb735e8573e26ec55908adeb` (REV), with the four tree-object
hashes `docs/reports/phase4b-a2-code-revision.md` pins for `internal`, `cmd`,
`features`, `ci`. Every commit from task 005 onward (including this one) is
record-only, so every `file:line` below resolves identically at REV and at
this file's own commit -- re-checked in §5.5, not assumed.

### 5.1 VG-6 — `groups.id` is `INTEGER PRIMARY KEY AUTOINCREMENT`, diverging from `SPEC.md:332`

**Finding, stated and explicitly NOT edited** (per this run's standing rules,
"[w]here PRD and SPEC disagree, SPEC wins and the disagreement is a finding,
not an edit" — and here the disagreement is between SPEC itself and the
shipped schema, which is the same instruction: disclose, do not edit either
side):

- **`SPEC.md:332`** (inside the `CREATE TABLE groups` block, `SPEC.md:331-333`)
  reads the plain form: `  id         INTEGER PRIMARY KEY,`.
- **`internal/store/store.go:2419`** (the `schemaV7` migration that actually
  creates the table) reads the AUTOINCREMENT form: `` `CREATE TABLE IF NOT
  EXISTS groups (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL
  UNIQUE COLLATE NOCASE)` ``.

The divergence is deliberate and documented in-tree, not an oversight: the
comment directly above the migration (`internal/store/store.go:2397-2419`)
explains that plain `INTEGER PRIMARY KEY` lets SQLite's ordinary rowid
assignment reuse a deleted row's id (`max(rowid)+1` among rows *currently* in
the table), which would silently rebind stale references held by
`sessions.group_id`, `ui_state`'s `last_create_group` and `collapsed_groups`
(all keyed on this id with no foreign key) onto whatever unrelated group next
claims the reused id, instead of degrading to default the way R128/R130
require — proven by the id-reuse regression in `group_id_reuse_test.go`.
AUTOINCREMENT retires a deleted id for good (via SQLite's hidden
`sqlite_sequence` table), matching `sessions.id`'s own durable-identity
approach (a TEXT UUID, immune to this reuse class by construction) for the
one column here that cannot use a UUID and still cycle through the create
modal's numeric picker.

**Left as a finding, not edited**, for the reason the standing rules already
give: SPEC wins the disagreement, but the AUTOINCREMENT-vs-plain question here
is not "which one is correct" — it is that the code's own choice is correct
for durable identity (proven by a regression test) while SPEC's prose has not
been updated to say so. Changing either side is out of scope for this task
(SPEC.md is read-only per the standing rules; the schema is frozen code from
task 004 onward). Disclosure only.

### 5.2 Task 008's failure list (approach 2 stability10 sweep): none

Task 008's own README (`docs/reports/phase4b-a2-stability10/README.md`,
"Failure list" section) publishes the list this clause quantifies over —
one `grep -n 'FAIL'` invocation across the ten committed per-run logs
(`run-1.log` … `run-10.log`) — and it is empty: the committed
`docs/reports/phase4b-a2-stability10/failure-grep.log` is a zero-byte file,
and the README's own body says so explicitly ("none", "grep produced zero
matching lines"). Re-confirmed here (not copied forward): the same ten
committed logs, re-grepped, still produce nothing — §5.5 and
`check-findings.py.txt`'s own `test_failure_list_is_still_empty_across_the_ten_committed_logs`
check this directly. No entries to list; stated explicitly per this
clause's own instruction for when the list is empty.

### 5.3 The `mkfifo`/`osc11` flake instance seen during this approach's planning

**`internal/interactive/replydrain_test.go:86`** (`func
TestTerminalQueryInPaneOutputNeverStallsSession`), specifically its `osc11`
subtest, failed during this approach's own planning (a disposable
`.plan-probe` scratch worktree, not a committed suite run) with an `ENOENT`
on the FIFO-creation exec at **`internal/interactive/replydrain_test.go:100`**
(the `t.Fatalf("Start: %v", err)` this subtest's own `Start` call reaches):
`mkfifo /tmp/deck-interactive-pipe-630249350/pane.fifo: exit status 1: mkfifo:
cannot create fifo '/tmp/deck-interactive-pipe-630249350/pane.fifo': No such
file or directory`. A second, differently-shaped instance of the same
subtest racing the same temp-dir/FIFO-arming path (a 5s connect timeout
rather than an ENOENT) was also captured during planning.

**Committed log path:**
[`docs/reports/phase4b-a2-findings/mkfifo-flake-planning.log`](phase4b-a2-findings/mkfifo-flake-planning.log)
(both captured instances, verbatim, with their own provenance and the
command line that produced each).

**Left alone because:** this is the same pre-existing tmux/FIFO temp-dir
contention class the standing notes' own gotcha already names ("internal/tui
can hit rare tmux pipe-pane timeouts under sibling load"), not a regression —
neither instance happened inside this task's or any other approach-2 task's
own suite run, and the underlying `ArmPipePane` code path
(`internal/tmux/pipe.go`) is unit-tested and untouched by any of tasks
001-004's cures. It did not recur in task 007's gate sweep
(`docs/reports/phase4b-a2-final-suite/gate-run.log`) or task 008's ten-run
stability sweep (`docs/reports/phase4b-a2-stability10/run-*.log`) — checked
directly in §5.5, not assumed — so there is nothing open to chase beyond
this one disclosed pair of planning-time occurrences.

### 5.4 gofmt drift, re-checked for approach 2

Re-confirmed fresh for this approach (not copied forward from §2.1's
approach-1 evidence, per the standing rules' "a FACT to re-derive at the
commit that writes it"):

- `ci/run.sh gofmt -l .` at this file's own tree lists exactly the same three
  untracked, gitignored paths §2.1 already named —
  `.spike-preview/cmd/conformance/main.go`,
  `.spike-preview/conformance/conformance.go`,
  `.spike-preview/conformance/conformance_test.go` — and no other file.
  Committed output:
  [`docs/reports/phase4b-a2-findings/gofmt.log`](phase4b-a2-findings/gofmt.log)
  (exit `0`).
- **Correction, re-confirmed:** `internal/theme/quantize_test.go` — which the
  PRD claims drifts — is in fact clean today, same as §2.1 already found:
  `ci/run.sh gofmt -l internal/theme/quantize_test.go` → empty output, exit
  `0`. Committed output:
  [`docs/reports/phase4b-a2-findings/gofmt-quantize.log`](phase4b-a2-findings/gofmt-quantize.log).

No new drift; the three-file, untracked, `.spike-preview`-only shape §2.1
recorded still holds, and the `quantize_test.go` correction still holds.

### 5.5 How to re-check §5

```
$ sed -n '332p' SPEC.md
  id         INTEGER PRIMARY KEY,

$ sed -n '2419p' internal/store/store.go
	`CREATE TABLE IF NOT EXISTS groups (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL UNIQUE COLLATE NOCASE)`,

$ sed -n '86p;100p' internal/interactive/replydrain_test.go
func TestTerminalQueryInPaneOutputNeverStallsSession(t *testing.T) {
				t.Fatalf("Start: %v", err)

$ grep -n 'FAIL' docs/reports/phase4b-a2-stability10/run-*.log; echo "exit:$?"
exit:1   # empty -- matches §5.2's "none" and task 008's own failure-grep.log

$ grep -il 'mkfifo\|osc11' docs/reports/phase4b-a2-final-suite/gate-run.log \
    docs/reports/phase4b-a2-stability10/run-*.log; echo "exit:$?"
exit:1   # no match in either sweep -- matches §5.3's "did not recur"

$ cat docs/reports/phase4b-a2-findings/gofmt.log
.spike-preview/cmd/conformance/main.go
.spike-preview/conformance/conformance.go
.spike-preview/conformance/conformance_test.go
EXIT:0

$ cat docs/reports/phase4b-a2-findings/gofmt-quantize.log
EXIT:0

$ DECK_REPO=/workspace python3 -m pytest -q docs/reports/phase4b-a2-findings/check-findings.py.txt
# committed at docs/reports/phase4b-a2-findings/check-findings.log -- 7 passed, exit 0
```

All six `file:line` citations §5 adds (`SPEC.md:332`;
`internal/store/store.go:2397`, `:2419`;
`internal/interactive/replydrain_test.go:86`, `:100`) resolve at this file's
own commit — the code tree they point into is untouched by this commit (a
`docs/reports/**`-only, record-only change per the freeze in effect since
task 004), so the reads above, taken against the working tree immediately
before this commit, hold identically once this commit lands.
