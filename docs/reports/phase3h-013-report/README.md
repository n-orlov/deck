# Task 013 — exhaustive verification for docs/reports/phase3h.md's cited shas and paths

Every sha and every path cited anywhere in `docs/reports/phase3h.md` — its four requirement
sections, its per-requirement R94–R97 table, its evidence citations and its prose — is
checked below, one quoted command per item, run at this task's own tree. Each block is the
verbatim command line, its verbatim output and its exit status. Nothing in the report is
cited that is not listed here.

## Shas — `git cat-file -e <sha>^{commit}` (24 items, all exit 0)

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
$ git cat-file -e 2ccb1d3c698d3c256aa94aea7ef2e3a58dc0641a^{commit}; echo "exit: $?"
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
$ git cat-file -e 46dad5e^{commit}; echo "exit: $?"
exit: 0
$ git cat-file -e de90a5c^{commit}; echo "exit: $?"
exit: 0
$ git cat-file -e a24ff8d^{commit}; echo "exit: $?"
exit: 0
```

Covered: the three reverted target commits (`a53146a`, `c176751`, `0b7dce5`); R94's three
forward-revert commits (`f700025`, `2f952da`, `afa55b8`) and its record/proof commits
(`9cd8f37`, `357867e`, `67cefcd`, `7634895`); R95's two (`d578c03`, `2c2ec30`); R96's two
(`8d6ed72`, `2ccb1d3`, the latter also quoted in full as the final code sha
`2ccb1d3c698d3c256aa94aea7ef2e3a58dc0641a`); R97's six (`e686a97`, `1f919b7`, `2badb74`,
`08171ce`, `5708f52`, `5042852`); Phase 3g's `46dad5e`, named in the task 012 paragraph as
the commit that fixed F37's scenario half; and the operator's ruling and plan shas
(`de90a5c`, `a24ff8d`).

## Paths — `git ls-files --error-unmatch <path>` (29 items, all exit 0)

Directories are checked in the exact trailing-slash form the report cites them in; the
command lists each tracked file beneath them, so an untracked or generated directory could
not pass.

```
$ git ls-files --error-unmatch SPEC.md; echo "exit: $?"
SPEC.md
exit: 0
$ git ls-files --error-unmatch ci/stability.sh; echo "exit: $?"
ci/stability.sh
exit: 0
$ git ls-files --error-unmatch features/create_cwd_ghost.feature; echo "exit: $?"
features/create_cwd_ghost.feature
exit: 0
$ git ls-files --error-unmatch features/create_cwd_ghost_test.go; echo "exit: $?"
features/create_cwd_ghost_test.go
exit: 0
$ git ls-files --error-unmatch features/filter.feature; echo "exit: $?"
features/filter.feature
exit: 0
$ git ls-files --error-unmatch features/status_attach.feature; echo "exit: $?"
features/status_attach.feature
exit: 0
$ git ls-files --error-unmatch features/status_claude_hooks.feature; echo "exit: $?"
features/status_claude_hooks.feature
exit: 0
$ git ls-files --error-unmatch features/status_probe.feature; echo "exit: $?"
features/status_probe.feature
exit: 0
$ git ls-files --error-unmatch internal/service/reconcile.go; echo "exit: $?"
internal/service/reconcile.go
exit: 0
$ git ls-files --error-unmatch internal/service/reconcile_live_error_precedence_test.go; echo "exit: $?"
internal/service/reconcile_live_error_precedence_test.go
exit: 0
$ git ls-files --error-unmatch docs/DELIVERY-LOG.md; echo "exit: $?"
docs/DELIVERY-LOG.md
exit: 0
$ git ls-files --error-unmatch docs/reports/phase3g.md; echo "exit: $?"
docs/reports/phase3g.md
exit: 0
$ git ls-files --error-unmatch docs/reports/phase3g-findings.md; echo "exit: $?"
docs/reports/phase3g-findings.md
exit: 0
$ git ls-files --error-unmatch docs/reports/phase3h.md; echo "exit: $?"
docs/reports/phase3h.md
exit: 0
$ git ls-files --error-unmatch docs/reports/phase3h-013-report/README.md; echo "exit: $?"
docs/reports/phase3h-013-report/README.md
exit: 0
$ git ls-files --error-unmatch docs/reports/phase3h-008-fullsuite/README.md; echo "exit: $?"
docs/reports/phase3h-008-fullsuite/README.md
exit: 0
$ git ls-files --error-unmatch docs/reports/phase3h-001-status-probe-revert/; echo "exit: $?"
docs/reports/phase3h-001-status-probe-revert/README.md
docs/reports/phase3h-001-status-probe-revert/reverify/run-1.exitstatus
docs/reports/phase3h-001-status-probe-revert/reverify/run-1.log
docs/reports/phase3h-001-status-probe-revert/reverify/run-2.exitstatus
docs/reports/phase3h-001-status-probe-revert/reverify/run-2.log
docs/reports/phase3h-001-status-probe-revert/reverify/run-3.exitstatus
docs/reports/phase3h-001-status-probe-revert/reverify/run-3.log
docs/reports/phase3h-001-status-probe-revert/reverify/run-4-plain.exitstatus
docs/reports/phase3h-001-status-probe-revert/reverify/run-4-plain.log
exit: 0
$ git ls-files --error-unmatch docs/reports/phase3h-002-narrow-repair/; echo "exit: $?"
docs/reports/phase3h-002-narrow-repair/README.md
docs/reports/phase3h-002-narrow-repair/test.exitstatus
docs/reports/phase3h-002-narrow-repair/test.log
exit: 0
$ git ls-files --error-unmatch docs/reports/phase3h-003-r94-scenarios/; echo "exit: $?"
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
exit: 0
$ git ls-files --error-unmatch docs/reports/phase3h-005-hint-token/; echo "exit: $?"
docs/reports/phase3h-005-hint-token/README.md
docs/reports/phase3h-005-hint-token/create_cwd_ghost.attempt1-v.exitstatus
docs/reports/phase3h-005-hint-token/create_cwd_ghost.attempt1-v.log
docs/reports/phase3h-005-hint-token/create_cwd_ghost.attempt1.exitstatus
docs/reports/phase3h-005-hint-token/create_cwd_ghost.attempt1.log
exit: 0
$ git ls-files --error-unmatch docs/reports/phase3h-006-f31/; echo "exit: $?"
docs/reports/phase3h-006-f31/README.md
docs/reports/phase3h-006-f31/red-before.exitstatus
docs/reports/phase3h-006-f31/red-before.log
exit: 0
$ git ls-files --error-unmatch docs/reports/phase3h-007-f31-guard/; echo "exit: $?"
docs/reports/phase3h-007-f31-guard/README.md
docs/reports/phase3h-007-f31-guard/a-targeted.exitstatus
docs/reports/phase3h-007-f31-guard/a-targeted.log
docs/reports/phase3h-007-f31-guard/b-service-store.exitstatus
docs/reports/phase3h-007-f31-guard/b-service-store.log
docs/reports/phase3h-007-f31-guard/c-attention-sort.exitstatus
docs/reports/phase3h-007-f31-guard/c-attention-sort.log
exit: 0
$ git ls-files --error-unmatch docs/reports/phase3h-008-fullsuite/; echo "exit: $?"
docs/reports/phase3h-008-fullsuite/.exitstatus
docs/reports/phase3h-008-fullsuite/README.md
docs/reports/phase3h-008-fullsuite/sweep.log
exit: 0
$ git ls-files --error-unmatch docs/reports/phase3h-008-fullsuite/.exitstatus; echo "exit: $?"
docs/reports/phase3h-008-fullsuite/.exitstatus
exit: 0
$ git ls-files --error-unmatch docs/reports/phase3h-009-fullsuite-verbose/; echo "exit: $?"
docs/reports/phase3h-009-fullsuite-verbose/.exitstatus
docs/reports/phase3h-009-fullsuite-verbose/README.md
docs/reports/phase3h-009-fullsuite-verbose/scenario-summary.txt
exit: 0
$ git ls-files --error-unmatch docs/reports/phase3h-009-fullsuite-verbose/scenario-summary.txt; echo "exit: $?"
docs/reports/phase3h-009-fullsuite-verbose/scenario-summary.txt
exit: 0
$ git ls-files --error-unmatch docs/reports/phase3h-010-stability10/; echo "exit: $?"
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
exit: 0
$ git ls-files --error-unmatch docs/reports/phase3h-010-stability10/summary.log; echo "exit: $?"
docs/reports/phase3h-010-stability10/summary.log
exit: 0
$ git ls-files --error-unmatch docs/reports/phase3h-012-delivery-log/; echo "exit: $?"
docs/reports/phase3h-012-delivery-log/README.md
exit: 0
```

Covered: the product and test paths named in the R94/R95/R96 prose (`internal/service/reconcile.go`,
`internal/service/reconcile_live_error_precedence_test.go`, `features/create_cwd_ghost.feature`,
`features/create_cwd_ghost_test.go`, `features/status_attach.feature`,
`features/status_claude_hooks.feature`, `features/status_probe.feature`,
`features/filter.feature`, `ci/stability.sh`, `SPEC.md`); the documents the R97 paragraph
reports on (`docs/DELIVERY-LOG.md`, `docs/reports/phase3g.md`,
`docs/reports/phase3g-findings.md`); the report itself and this README
(`docs/reports/phase3h.md`, `docs/reports/phase3h-013-report/README.md`, both added by task
013's commits); the ten evidence directories cited by the table; and the three individual
evidence files the report quotes verbatim from
(`docs/reports/phase3h-008-fullsuite/.exitstatus`,
`docs/reports/phase3h-009-fullsuite-verbose/scenario-summary.txt`,
`docs/reports/phase3h-010-stability10/summary.log`) plus
`docs/reports/phase3h-008-fullsuite/README.md`.

## One path deliberately NOT cited

`afa55b8` (task 002's forward-revert of `0b7dce5`) deleted the replacement test file that
`0b7dce5` had added alongside its widened repair. That file does not exist in the tree at
HEAD, so `docs/reports/phase3h.md` describes the deletion through `afa55b8`'s own diff and
never names the removed path as a citation — precisely so that every path token in the
report is tracked. Both halves of that statement are checkable:

```
$ git ls-files --error-unmatch internal/service/reconcile_bare_error_repair_test.go; echo "exit: $?"
error: pathspec 'internal/service/reconcile_bare_error_repair_test.go' did not match any file(s) known to git
Did you forget to 'git add'?
exit: 1
$ git cat-file -e 0b7dce5:internal/service/reconcile_bare_error_repair_test.go; echo "exit: $?"
exit: 0
```

The first shows the path is untracked at HEAD (hence not citable); the second shows it did
exist in `0b7dce5`'s tree, which is what `afa55b8` reverted.

## Verbatim measurement quotes re-checked against their tracked sources

```
$ cat docs/reports/phase3h-008-fullsuite/.exitstatus
0
$ cat docs/reports/phase3h-009-fullsuite-verbose/scenario-summary.txt
godog scenario/step summary excerpt from ci/run.sh go test -p=1 -count=1 -v ./... (TestFeatures block, tail of features package output):

    When deck client "A" exits cleanly                                                                # assertions_test.go:66 -> github.com/n-orlov/deck/features.clientExitsCleanly

311 scenarios (311 passed)
3532 steps (3532 passed)
4m59.662776073s
--- PASS: TestFeatures (299.67s)

Full 17-package go test summary lines (ok / [no test files]):

ok  	github.com/n-orlov/deck/cmd/deck	7.650s
ok  	github.com/n-orlov/deck/cmd/fake-claude	0.797s
ok  	github.com/n-orlov/deck/cmd/fake-pi	0.780s
ok  	github.com/n-orlov/deck/features	316.233s
ok  	github.com/n-orlov/deck/internal/agent	0.005s
ok  	github.com/n-orlov/deck/internal/audit	0.018s
ok  	github.com/n-orlov/deck/internal/config	0.029s
ok  	github.com/n-orlov/deck/internal/hookrecv	4.188s
ok  	github.com/n-orlov/deck/internal/interactive	11.147s
?   	github.com/n-orlov/deck/internal/notify	[no test files]
?   	github.com/n-orlov/deck/internal/search	[no test files]
ok  	github.com/n-orlov/deck/internal/service	4.331s
ok  	github.com/n-orlov/deck/internal/store	2.418s
ok  	github.com/n-orlov/deck/internal/theme	0.006s
ok  	github.com/n-orlov/deck/internal/tmux	19.424s
ok  	github.com/n-orlov/deck/internal/tui	1.387s
?   	github.com/n-orlov/deck/internal/unit	[no test files]
$ tail -1 docs/reports/phase3h-010-stability10/summary.log
10/10 passed
$ git log -1 --format=%H -- '*.go' '*.feature'
2ccb1d3c698d3c256aa94aea7ef2e3a58dc0641a
```

These are the three measurements `docs/reports/phase3h.md` quotes (task 008's sweep exit
status, task 009's Gherkin tally, task 010's `10/10 passed` line) plus the final code sha it
names; each matches the report byte for byte.
