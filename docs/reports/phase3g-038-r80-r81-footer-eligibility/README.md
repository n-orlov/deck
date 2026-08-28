# R80/R81 evidence — the footer's eligibility-driven legend and curated fixed set

Requirement R80 (tasks 012–013, shas `f5977d4`, `745a25b`, `f9611b7`, `ebbc3fd`) and the
predicate/parity half of R81 (tasks 014–015, shas `7dbe5c5`, `3498b3e`).

Neither is one of the PRD's eight product-defect requirements. R81's own
revert-and-reproduce demonstrations live in `../phase3g-015-footer-bindings-parity/`
(two forced reds, captured at implementation time). This directory adds the green run
that R80's section had no path for; it was **captured by task 038** at HEAD `e02ef08`
(unmodified tree) and is green-only confirmation, not an implementation-time pair.

- `green-footer-eligibility.log` —
  `ci/run.sh go test -count=1 -v -run 'TestFooterKeyLegendReflectsEligibility|TestFooterLineSharesLineWithLongStatusReason|TestFooterEntriesNameOnlyBoundKeysViaSourceParse|TestFooterEntryEligibilityMatchesRealPredicate|TestFooterFixedSetMatchesSpecAndExcludesRareKeys' ./internal/tui/`
  — presence **and absence** per action across live/stopped/archived/attention-pending/
  marked/empty-list rows, the 80-column reason+legend behaviour, and the three
  footer↔bindings↔SPEC parity checks.
