# Task 110 — bring `phase3g-findings.md` up to date

`docs/reports/phase3g-findings.md`'s F18-F21 rows described defects as still
open even though each had since been fixed by a task in this approach (or, for
F20, by approach 01's own task 040, which landed just after the findings
report itself was published and was never folded back in). This task:

- Marks F18 resolved by task 105 (`ea6ce4b`, `internal/tui/rename.go`'s
  focused field now gets a `theme.Selection` background).
- Marks F19 resolved by task 104 (`045a6e6`, the remaining hard-coded
  create-modal title wait converged onto the title-independent path).
- Marks F20 resolved by approach 01's task 040 (`fe79040`, the scenario's
  frame-read wait repointed at the post-repair store state).
- Adds an inline sha citation to F21's existing "resolved by task 106" note
  (`57a6882`, refined in `0219e42`) — it already said "resolved" but never
  named the sha.
- Adds three new rows: F24 (task 101's wrap-sensitive Agent-field wait,
  `2549406`), F25 (task 102's R85 `(last used)` label collision,
  `89682e5`), and F26 (task 103's R76-vs-hook-declared-terminal-status
  interaction, `51b7f17` — filed as a sibling of F20, "R76 working as
  specified," per task 103's own recorded verdict, not as a defect).
- Updates the two prose cross-references to F18/F19 in §1 and §2.2 so they
  no longer say "not yet landed" / "expected to fail" next to a table that
  now says otherwise.
- Appends an addendum to §6's citation checklist covering the eight new/
  edited shas and eight new/edited paths; verbatim command output in
  [`citation-check.log`](citation-check.log) in this directory (all eighteen
  checks `ok`).

`SPEC.md` is not touched by this task — `git show --stat` on this task's own
commit shows only `docs/reports/phase3g-findings.md` and this evidence
directory.
