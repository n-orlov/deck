# Tier 1 → Tier 2 decision (task 011)

## Tier 1 landing commits

| Requirement | GH issue | Landing commit | Title |
|---|---|---|---|
| R136 — the viewport follows the cursor | #31 | `cfeb2f3073492c14d046a0dd6e9ebbcb28ad0983` | `tui: test the viewport follow for every R136 binding (task 008, #31)` |
| R138 — header click works while a preview is live | #33 | `1ba7007ea7f2ed2a7bab51e383ca0d1b70ac0aa7` | `tui: route interactive-mode header/strip presses through the shared resolver (task 005, #33)` |
| R139 — `default` can sort first | #34 | `b7d7b7d6d193bdd4df3df50052658c32309d2b57` | `tui+config: apply ui.default_group_first live on save, no restart (task 003, #34)` |

Each landing commit is the tip of that requirement's own product+test commits
(R136: tasks 007→008; R138: task 005; R139: tasks 001→003); the corresponding
probe-audit commits (009 `f870398`, 006 `cdfce29`, 004 `1f6c02e`) are the
docs-only reports that verify each landing commit against the unfixed tree,
per `docs/reports/phase4c-probes/r136.md`, `r138.md` and `r139.md`.

All three Tier 1 requirements' scenarios also landed together in
`features/session_groups.feature` at task 010 (commit `5af5864`), which is
`HEAD == origin/main` at the moment this decision is written.

## Budget check against the Tier 2 gate

- Deadline (`status.json` `deadlineAt`): `2026-09-24T04:57:37Z`.
- Wall clock read at the moment of this decision (`date -u`):
  `2026-09-23T06:39:22Z`.
- Wall-clock budget remaining: **≈ 1338 minutes**.
- Tier 2 gate (standing rule, task 011): start R137 (tasks 012-016) only with
  **≥ ~800 minutes** of wall clock left.
- 1338 min ≥ 800 min gate — the gate is cleared with roughly 538 minutes of
  margin above the threshold, well clear of the ~213-minute review reserve
  that sits behind it.

## Decision

**R137 is started.** The remaining budget (≈1338 min) clears the ~800-minute
Tier 2 gate with substantial margin, so Tier 2 (tasks 012-016: the header
cursor, GH #32) proceeds from this point in the plan. This decision does not
itself implement any part of R137 — task 011 is record-only (see the freeze
rule below) — it only unblocks task 012, which is `pending` with `011` as
its sole dependency at the moment this file is written.

Because R137 is started, the fallback clause does not apply: #32 does not
stay open on account of this decision, and the release is not held on this
decision alone. (Whether the release remains held overall depends on
whether Tier 2 itself lands successfully in tasks 012-016, which this
document does not anticipate.)
