# R85 evidence — the create modal opens on the last used agent

Requirement R85 (`SPEC.md` §11.4), task 024, fixing sha `9991689`.

Not one of the PRD's eight product-defect requirements. Task 024 left no evidence
directory, so both logs were **captured by task 038** at HEAD `e02ef08` (unmodified
tree): green-only confirmation of the assertions the report names, not an
implementation-time red/green pair.

- `green-tui-last-used-agent.log` — `ci/run.sh go test -count=1 -v -run '<the six
  tests>' ./internal/tui/` (`internal/tui/create_last_used_agent_test.go`:
  pre-selection, the `(last used)` label, label cleared on a cycle, no persistence on an
  abandoned dialog, persistence only on a create that succeeds, and the degrade path for
  a missing/unregistered stored value).
- `green-store-last-create-agent.log` — `ci/run.sh go test -count=1 -v -run
  'TestLastCreateAgentAccessorsDegradeToDocumentedDefaults' ./internal/store/`
  (`ui_state` persistence, single row per key, documented defaults).
