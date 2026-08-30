# Task 013 — verification for docs/reports/phase3h.md's cited shas and paths

Every sha and path cited in `docs/reports/phase3h.md` (its four requirement sections,
the R94-R97 per-requirement table, and its prose) is checked below with
`git cat-file -e <sha>^{commit}` and `git ls-files --error-unmatch <path>`, run at this
task's own tree.

## Shas (`git cat-file -e <sha>^{commit}`)

```
$ git cat-file -e a53146a^{commit}; echo "exit: $?"
exit: 0
$ git cat-file -e c176751^{commit}; echo "exit: $?"
exit: 0
$ git cat-file -e 0b7dce5^{commit}; echo "exit: $?"
exit: 0
$ git cat-file -e f700025^{commit}; echo "exit: $?"
exit: 0
$ git cat-file -e 2f952da^{commit}; echo "exit: $?"
exit: 0
$ git cat-file -e afa55b8^{commit}; echo "exit: $?"
exit: 0
$ git cat-file -e 9cd8f37^{commit}; echo "exit: $?"
exit: 0
$ git cat-file -e 357867e^{commit}; echo "exit: $?"
exit: 0
$ git cat-file -e 67cefcd^{commit}; echo "exit: $?"
exit: 0
$ git cat-file -e 7634895^{commit}; echo "exit: $?"
exit: 0
$ git cat-file -e d578c03^{commit}; echo "exit: $?"
exit: 0
$ git cat-file -e 2c2ec30^{commit}; echo "exit: $?"
exit: 0
$ git cat-file -e 8d6ed72^{commit}; echo "exit: $?"
exit: 0
$ git cat-file -e 2ccb1d3^{commit}; echo "exit: $?"
exit: 0
$ git cat-file -e e686a97^{commit}; echo "exit: $?"
exit: 0
$ git cat-file -e 1f919b7^{commit}; echo "exit: $?"
exit: 0
$ git cat-file -e 2badb74^{commit}; echo "exit: $?"
exit: 0
$ git cat-file -e 08171ce^{commit}; echo "exit: $?"
exit: 0
$ git cat-file -e 5708f52^{commit}; echo "exit: $?"
exit: 0
$ git cat-file -e 5042852^{commit}; echo "exit: $?"
exit: 0
$ git cat-file -e de90a5c^{commit}; echo "exit: $?"
exit: 0
$ git cat-file -e a24ff8d^{commit}; echo "exit: $?"
exit: 0
```

All 22 shas resolve: the R94/R95/R96 revert commits (`f700025`, `2f952da`, `afa55b8`),
their outcome-record/proof commits (`9cd8f37`, `357867e`, `67cefcd`, `7634895`), the
three reverted target commits (`a53146a`, `c176751`, `0b7dce5`), R95's two commits
(`d578c03`, `2c2ec30`), R96's two commits (`8d6ed72`, `2ccb1d3`), R97's six commits
(`e686a97`, `1f919b7`, `2badb74`, `08171ce`, `5708f52`, `5042852`), and the operator's
ruling/plan shas (`de90a5c`, `a24ff8d`).

## Paths (`git ls-files --error-unmatch <path>`)

```
$ git ls-files --error-unmatch docs/reports/phase3h-001-status-probe-revert
docs/reports/phase3h-001-status-probe-revert/README.md
docs/reports/phase3h-001-status-probe-revert/reverify/run-1.exitstatus
docs/reports/phase3h-001-status-probe-revert/reverify/run-1.log
docs/reports/phase3h-001-status-probe-revert/reverify/run-2.exitstatus
docs/reports/phase3h-001-status-probe-revert/reverify/run-2.log
docs/reports/phase3h-001-status-probe-revert/reverify/run-3.exitstatus
docs/reports/phase3h-001-status-probe-revert/reverify/run-3.log
docs/reports/phase3h-001-status-probe-revert/reverify/run-4-plain.exitstatus
docs/reports/phase3h-001-status-probe-revert/reverify/run-4-plain.log
$ git ls-files --error-unmatch docs/reports/phase3h-002-narrow-repair
docs/reports/phase3h-002-narrow-repair/README.md
docs/reports/phase3h-002-narrow-repair/test.exitstatus
docs/reports/phase3h-002-narrow-repair/test.log
$ git ls-files --error-unmatch docs/reports/phase3h-003-r94-scenarios
docs/reports/phase3h-003-r94-scenarios/README.md
docs/reports/phase3h-003-r94-scenarios/status_attach.attempt1.exitstatus
docs/reports/phase3h-003-r94-scenarios/status_attach.attempt1.log
docs/reports/phase3h-003-r94-scenarios/status_claude_hooks.attempt1.exitstatus
docs/reports/phase3h-003-r94-scenarios/status_claude_hooks.attempt1.log
docs/reports/phase3h-003-r94-scenarios/status_probe.attempt1.exitstatus
docs/reports/phase3h-003-r94-scenarios/status_probe.attempt1.log
docs/reports/phase3h-003-r94-scenarios/status_probe.attempt2.exitstatus
docs/reports/phase3h-003-r94-scenarios/status_probe.attempt2.log
docs/reports/phase3h-003-r94-scenarios/status_probe.attempt3.exitstatus
docs/reports/phase3h-003-r94-scenarios/status_probe.attempt3.log
$ git ls-files --error-unmatch docs/reports/phase3h-005-hint-token
docs/reports/phase3h-005-hint-token/README.md
docs/reports/phase3h-005-hint-token/create_cwd_ghost.attempt1-v.exitstatus
docs/reports/phase3h-005-hint-token/create_cwd_ghost.attempt1-v.log
docs/reports/phase3h-005-hint-token/create_cwd_ghost.attempt1.exitstatus
docs/reports/phase3h-005-hint-token/create_cwd_ghost.attempt1.log
$ git ls-files --error-unmatch docs/reports/phase3h-006-f31
docs/reports/phase3h-006-f31/README.md
docs/reports/phase3h-006-f31/red-before.exitstatus
docs/reports/phase3h-006-f31/red-before.log
$ git ls-files --error-unmatch docs/reports/phase3h-007-f31-guard
docs/reports/phase3h-007-f31-guard/README.md
docs/reports/phase3h-007-f31-guard/a-targeted.exitstatus
docs/reports/phase3h-007-f31-guard/a-targeted.log
docs/reports/phase3h-007-f31-guard/b-service-store.exitstatus
docs/reports/phase3h-007-f31-guard/b-service-store.log
docs/reports/phase3h-007-f31-guard/c-attention-sort.exitstatus
docs/reports/phase3h-007-f31-guard/c-attention-sort.log
$ git ls-files --error-unmatch docs/reports/phase3h-008-fullsuite
docs/reports/phase3h-008-fullsuite/.exitstatus
docs/reports/phase3h-008-fullsuite/README.md
docs/reports/phase3h-008-fullsuite/sweep.log
$ git ls-files --error-unmatch docs/reports/phase3h-009-fullsuite-verbose
docs/reports/phase3h-009-fullsuite-verbose/.exitstatus
docs/reports/phase3h-009-fullsuite-verbose/README.md
docs/reports/phase3h-009-fullsuite-verbose/scenario-summary.txt
$ git ls-files --error-unmatch docs/reports/phase3h-010-stability10
docs/reports/phase3h-010-stability10/.exitstatus
docs/reports/phase3h-010-stability10/README.md
docs/reports/phase3h-010-stability10/run-1.log
docs/reports/phase3h-010-stability10/run-10.log
docs/reports/phase3h-010-stability10/run-2.log
docs/reports/phase3h-010-stability10/run-3.log
docs/reports/phase3h-010-stability10/run-4.log
docs/reports/phase3h-010-stability10/run-5.log
docs/reports/phase3h-010-stability10/run-6.log
docs/reports/phase3h-010-stability10/run-7.log
docs/reports/phase3h-010-stability10/run-8.log
docs/reports/phase3h-010-stability10/run-9.log
docs/reports/phase3h-010-stability10/summary.log
$ git ls-files --error-unmatch docs/reports/phase3h-012-delivery-log
docs/reports/phase3h-012-delivery-log/README.md
```

Every one of the ten evidence directories cited by the per-requirement table is tracked
(each command above lists its committed files; none error). `docs/reports/phase3h.md`
itself is added by this same commit.
