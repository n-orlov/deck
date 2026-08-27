# Task 015: footer↔bindings parity test

`internal/tui/footer_bindings_parity_test.go` re-parses tui.go's own
`footerLegend` slice literal (never a copy of it) and cross-checks it in
both directions:

- `TestFooterEntriesNameOnlyBoundKeysViaSourceParse` — every footer entry
  names a key the list-mode switch actually binds (direction a), reusing
  `help_keymap_parity_test.go`'s own `listModeBoundKeys`/`helpKeyTokenToBoundKeys`.
- `TestFooterEntryEligibilityMatchesRealPredicate` — every per-row entry
  carries a real eligibility predicate (never a bare `nil` unless it is
  one of the closed set of global keys), and for a representative row of
  every state the predicates distinguish (including a marked-batch row),
  the entry is shown in the rendered footer if and only if that predicate
  — looked up by the name the source names, and evaluated through the
  real `footerRowEligible` with the same batch flag — accepts the row
  (direction b).
- `TestFooterFixedSetMatchesSpecAndExcludesRareKeys` — cross-checks the
  parsed glyph set directly against SPEC.md §11.3's own prose (also
  re-read at test time, never copied into a fixed Go list): every glyph
  the fixed-set sentence requires is present, and neither `P` nor `p`
  (the sentence's own named exclusions) is.

## Demonstrated regressions (both fail the new tests; HEAD is unmodified)

- `red-comma-removed.log`: `,` deleted from `footerLegend` →
  `TestFooterFixedSetMatchesSpecAndExcludesRareKeys` fails
  (`SPEC.md §11.3's footer fixed set requires "," but tui.go's footerLegend
  does not have it`).
- `red-P-added-back.log`: `{"P", "P", "profile", nil}` appended to
  `footerLegend` (the simplest, and historically actual, shape a
  reintroduced-but-ungated `P` would take) → fails BOTH
  `TestFooterEntryEligibilityMatchesRealPredicate` (`carries no
  eligibility predicate and is not one of the fixed global keys allowed
  to have none`) and `TestFooterFixedSetMatchesSpecAndExcludesRareKeys`
  (`SPEC.md §11.3 keeps "P" out of the footer ... but tui.go's
  footerLegend has it`).

Both edits were made in a throwaway diff, captured, then reverted before
committing; `git status`/`git diff` show `tui.go` unmodified at commit
time.

`ci/run.sh go test -count=1 ./internal/tui/` is green at HEAD (see repo
history; not re-captured into this dir to avoid duplicating the whole
suite's log for a single-package run).
