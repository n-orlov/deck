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
