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

## Follow-up commit: the citation check now covers *every* citation in the rows

The first pass's [`citation-check.log`](citation-check.log) sampled eight shas
(the fixing shas) and eight paths, but F18-F21's edited rows and the new
F24-F26 rows cite seventeen distinct shas in total — the sampled eight plus
`b6cbbc7`, `9991689`, `050ca9f`, `15e33c6`, `1c8cbad`, `1cfbd5a`, `7033e12`,
`89fcffc` and `8cff03b`, which appear as states of record and as the causal
history each row narrates. Sampling is not what "every sha cited in the new or
edited rows resolves" asks for, so the check is now a committed, re-runnable
script rather than a transcript:

```
bash docs/reports/phase3g-110-findings-update/check-citations.sh   # exit 0 == all resolve
```

[`check-citations.sh`](check-citations.sh) checks, and
[`check-citations.log`](check-citations.log) is its whole output plus exit
status:

- all seventeen shas cited in the new/edited rows, plus `961e9cc` (this task's
  own first commit), each via `git cat-file -e <sha>^{commit}` with the
  commit subject printed so a reader can see *what* resolved. `1cfbd5a` is the
  run's base and appears in F21's row only inside the range expression
  `1cfbd5a..050ca9f`; the script resolves it by sha like the rest.
- all nine markdown file links in those rows, resolved relative to
  `docs/reports/`, plus the fourteen in-tree paths the rows cite in backticks.
- every same-document anchor link in the whole report, against slugs derived
  from the report's own headings by GitHub's rule (lowercase, drop everything
  but alphanumerics/`-`/`_`/space, spaces to hyphens). This found three broken
  anchors that predate this task and one in F19's own edited row: a heading
  whose text contains an em dash or an ellipsis slugs with a *double* hyphen
  there (`#4-f2--the-golden-frame-settle-flake-…`), because the punctuation is
  dropped and both surrounding spaces still become hyphens. All four links are
  corrected in this commit; no heading text changed.
- that no commit in `961e9cc~1..HEAD` touches `SPEC.md`.

The log is captured with this commit's content in the working tree, so its
`HEAD` line names the parent commit `961e9cc`; re-running the script at any
later commit reproduces the same result, since every citation is checked by sha
or by path, not by position in history.

`SPEC.md` is not touched by this task — `git show --stat` on this task's own
commit shows only `docs/reports/phase3g-findings.md` and this evidence
directory.
