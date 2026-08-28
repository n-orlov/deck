# R84 evidence — the contrast floor does NOT yet cover the dialog pairs

Requirement R84 (`SPEC.md` §11.6), task 023, **`pending` — not implemented**. There is
therefore **no fixing sha**, and this directory is deliberately evidence of *absence*.
The sha of record for the unmet state is `7033e12`, the last commit to touch
`internal/theme/contrast_test.go` (Phase 3, task 084) — that file's pair list is exactly
what R84 asks to grow.

`missing-pairs-absence-check.log` was produced by task 038 at HEAD `e02ef08` and greps
`internal/theme/contrast_test.go` for each pair R84 names: `hint/surface`, `key/surface`,
`error/surface` and every text token over `Selection`/`SelectionIdle` are **ABSENT**;
only `text/selection` (from requirement 30's original floor) is present. Reproduce with
the command in the log's own header comment.

Nothing in this directory is a claim that R84 is met.
