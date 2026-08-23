# Part II pre-check: SPEC.md §11.9 staged amendment (composition rule 4)

Task 025 (I/II boundary). Verifies, before any Part II code is written, that the staged
`SPEC.md` amendment described by `docs/spikes/interactive-preview-spec-changes.md` is
already applied at HEAD (`36e5a01`), per composition rule 4's instruction to check this
before starting Part II. `SPEC.md` is **not modified** by this task — see the empty diff at
the end of this report.

## Line-by-line confirmation

| Claim | Where | Confirmed text |
|---|---|---|
| §11.9 exists | `SPEC.md:1387` | `### 11.9 Interactive preview` |
| §3.3 makes `a` the attach key and `Enter` the interactive-mode key | `SPEC.md:203-204` | `**\`a\` attaches the client directly**: \`tmux -L deck attach -t deck_<slug>\`. On detach the client returns to the TUI, which resumes its render loop. \`Enter\` enters §11.9's interactive preview instead — the cheaper, reversible half of the same intent — and \`a\` is the escalation.` |
| §11.3 states two focus stops entered by `Enter` rather than a tab cycle | `SPEC.md:1091-1093` | `Focus is visible, and there are exactly two places it can be. The sidebar is focused by default; §11.9's interactive preview is the second and only other stop, and it is entered by \`Enter\` rather than by a \`tab\` cycle, because a cycle would imply stops that do nothing.` |
| §3.2 sets history-limit | `SPEC.md:177` | `` `set -g history-limit <N>`. tmux's default is 2000, and deck has never set it. `` (same bulleted item as the other bootstrap options, `SPEC.md:164-182`) |
| §11.8's double-click enters interactive mode | `SPEC.md:1320` | `` | **double**-click a sidebar row | enter §11.9's interactive preview | `↵` | `` |
| §13.1 declares `DECK_INTERACTIVE_MS`/`DECK_INTERACTIVE_TRANSPORT` | `SPEC.md:1478-1479` | `` | **Interactive render rate** | `DECK_INTERACTIVE_MS` — §11.9's grid render-coalescing interval. ... | `` and `` | **Interactive transport** | `DECK_INTERACTIVE_TRANSPORT=pipe\|capture` pins §11.9's render path. ... | `` |

All six line citations were read directly out of `SPEC.md` at HEAD in this task (not copied
from a prior report) and match verbatim.

## Finding, not an edit: the passive-preview bullet at `SPEC.md:932`

§11's passive-preview bullet still reads (verbatim, `SPEC.md:932`):

> `never interactive. ↵ is how you get a real terminal.`

This sentence is now stale relative to §11.9 (`Enter` no longer means "attach a real
terminal" — that meaning moved to `a`, per §3.3 above — `Enter` now enters the in-process
interactive preview). It is **deliberately left in place**, not a defect to fix here:

- The staged amendment's own item 3 (`docs/spikes/interactive-preview-spec-changes.md`)
  says explicitly to "keep every existing bullet" in §11's passive-preview list.
- The new two-modes bullet immediately below it (`SPEC.md:934` area, "The preview attaches
  no tmux client and never resizes a pane...") scopes the old bullet: it is now read as
  describing the **passive** preview specifically (no embedded PTY emulator, capture-only,
  non-interactive), which is still true, while §11.9 is a separate, later capability
  layered on top.
- This is recorded as a **finding for the operator**, per this task's own success
  criteria, and is explicitly **NOT the stop condition** for composition rule 4: the staged
  amendment is applied, and Part II code may proceed.

This finding is also cross-referenced in `docs/reports/phase3-findings.md` and in
`tasks.json`'s `discovered.prdCorrections` (added by task 022 / carried in the notes file),
so it is not lost if this report is not the one a later task reads.

## Composition rule 4 verdict

**Not triggered.** Every element of the staged §11.9 amendment named by this task's success
criteria is present and correct at HEAD. Part II tasks (026 onward) may proceed without a
SPEC.md edit or an operator escalation.

## Proof SPEC.md is unmodified by this task

```
$ git status --short -- SPEC.md
$ git diff --stat -- SPEC.md
```

Both commands produced empty output when run from a clean tree at HEAD `36e5a01` while this
report was written — confirmed again immediately before commit.
