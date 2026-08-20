# Phase 2b-1 report

This file is completed by task 036 (per-requirement evidence for
requirements 1-46, requirement 45). It is created early, by task 023, only
to record requirement 29's tie-break key so that fact has a home in this
report from the moment it exists rather than waiting for the final task to
retroactively add it.

## Requirement 29: the attention sort's tie-break key

Sessions sort by the requirement-28 group order (`waiting` → `error` →
`running` → `starting` → `idle` → `stopped`); within a group, ties are
broken by **`(StatusAt ascending, ID ascending)`** — `StatusAt` is the
timestamp of the session's current status (so "oldest first" for `waiting`
falls straight out of it), and `ID` is unique and stable for the session's
lifetime, so it fully resolves any remaining tie (e.g. two sessions that
became `waiting` in the same millisecond under a frozen test clock). See
`internal/tui/attention.go` (`sortSessionsByAttention`, `lessByAttention`)
and `docs/reports/phase2b1-findings.md`'s "Task 023" section for the full
write-up and unit-test evidence.

The rest of this report — commands and captured output per requirement
1-46 — is filled in by task 036 once every requirement lands.
