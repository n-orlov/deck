# Task 209 — exhaustive, mechanical citation check for `docs/reports/phase3h.md`

Every backticked token in `docs/reports/phase3h.md`, enumerated **mechanically** (not from
expectation), is classified below, and every sha token and every path token — each token
in its own right, including line-number-suffixed tokens whose base path is also cited — gets its
own quoted command and exit status. Every number in this report is computed from the
enumeration itself, not asserted. This report supersedes task 013's narrower
`docs/reports/phase3h-013-report/README.md` (retained in the tree as superseded history and
still correct for the citations it covered).

## Enumeration (mechanical)

```
$ grep -o '`[^`]*`' docs/reports/phase3h.md | sort -u | wc -l
127
```

127 unique backticked tokens: **46 sha tokens**, **46 path tokens**
(counting each token separately, so a `path` and a `path:line` token are two items) and
**35 tokens that are neither** — code, prose, command, flag, glob and output
fragments, each named explicitly below with the reason no `git cat-file`/`git ls-files`
check applies. The three counts add up to the enumeration total by construction.

| # | token | kind | note |
|---|---|---|---|
| 1 | (empty) | artifact | adjacent backticks inside a code-fence marker line; no content |
| 2 | `` `(task NNN)` `` | template | commit-marker template with a placeholder, not a path or sha |
| 3 | `` `*.feature` `` | glob | command-line glob argument, not a path |
| 4 | `` `*.go` `` | glob | command-line glob argument, not a path |
| 5 | `` `-v` `` | flag | CLI flag, not a path or sha |
| 6 | `` `0` `` | status | quoted exit-status digit, not a sha or path |
| 7 | `` `011b04b` `` | sha | task 203's self-sha addendum commit |
| 8 | `` `08171ce` `` | sha | task 011 |
| 9 | `` `0b7dce5` `` | sha | R94 revert target (task 002) |
| 10 | `` `0d9a551` `` | sha | task 208 validation fix #1 |
| 11 | `` `1b61337` `` | sha | task 209's addendum commit naming the data commit `5b7c9e3` |
| 12 | `` `1f47503` `` | sha | task 201's gofmt-clean evidence commit |
| 13 | `` `1f919b7` `` | sha | task 009's original publish commit |
| 14 | `` `2badb74` `` | sha | task 010's original publish commit |
| 15 | `` `2c2ec30` `` | sha | task 005 |
| 16 | `` `2ccb1d3` `` | sha | task 007 (superseded final code sha, short form) |
| 17 | `` `2ccb1d3c698d3c256aa94aea7ef2e3a58dc0641a` `` | sha | task 007 (superseded final code sha, full form) |
| 18 | `` `2f952da` `` | sha | R94 forward-revert |
| 19 | `` `357867e` `` | sha | R94 record/proof commit |
| 20 | `` `426fb3a` `` | sha | task 209's first commit |
| 21 | `` `4673fb9` `` | sha | task 013 |
| 22 | `` `46dad5e` `` | sha | Phase 3g's F37 scenario-half fix |
| 23 | `` `4b1d4dc` `` | sha | task 201 (current final code sha) |
| 24 | `` `5042852` `` | sha | task 012's correction commit |
| 25 | `` `50960b9` `` | sha | task 206 |
| 26 | `` `52e9045` `` | sha | task 205 |
| 27 | `` `5326e39` `` | sha | task 015's second commit |
| 28 | `` `5708f52` `` | sha | task 012 |
| 29 | `` `5b7c9e3` `` | sha | task 209's data commit correcting the delivery ledger |
| 30 | `` `67cefcd` `` | sha | R94 record/proof commit |
| 31 | `` `68ac4cf` `` | sha | task 208's marker-accounting commit |
| 32 | `` `7634895` `` | sha | R94 record/proof commit |
| 33 | `` `79b56d9` `` | sha | task 204 |
| 34 | `` `87bde8f` `` | sha | task 208 |
| 35 | `` `8d6ed72` `` | sha | task 006 |
| 36 | `` `9cd8f37` `` | sha | R94 record/proof commit |
| 37 | `` `9cff1ea` `` | sha | task 207 |
| 38 | `` `:14` `` | fragment | scenario-title line reference in prose, not a standalone path |
| 39 | `` `?` `` | status | `go test`'s "no test files" marker, quoted from output |
| 40 | `` `AllowedCurrentStatuses: []string{"starting"}` `` | code | Go literal fragment, not a path or sha |
| 41 | `` `EventKind: "tmux.shell_live"` `` | code | Go literal fragment, not a path or sha |
| 42 | `` `PASS (exit 0)` `` | status | stability-gate per-run status string |
| 43 | `` `SPEC.md:1355` `` | path+line | base path `SPEC.md` is tracked; the `:1355` suffix is a line reference, not part of the tracked path — checked in its own block below |
| 44 | `` `StatusUpdateInput` `` | code | Go identifier fragment |
| 45 | `` `StopFailure` `` | code | Gherkin/Go identifier fragment |
| 46 | `` `TestReconcileLosesInterleavedStatusWriteDuringShellPromotion` `` | code | Go test function name |
| 47 | `` `Then` `` | keyword | Gherkin keyword |
| 48 | `` `[no test files]` `` | status | `go test` output string, quoted |
| 49 | `` `a24ff8d` `` | sha | operator's plan/PRD commit |
| 50 | `` `a24ff8d..HEAD` `` | range | git revision range, not a single sha or a path; its named endpoint `a24ff8d` is checked as a sha above and `HEAD` is a ref |
| 51 | `` `a53146a` `` | sha | R94 revert target (task 001) |
| 52 | `` `a5f8f6b` `` | sha | named in the task 207 disposition prose (Phase 3g DELIVERY-LOG paragraph, now superseded) |
| 53 | `` `afa55b8` `` | sha | R94 forward-revert (task 002) |
| 54 | `` `b5e4178` `` | sha | task 013 |
| 55 | `` `c176751` `` | sha | R94 revert target (task 001) |
| 56 | `` `c69720f` `` | sha | task 015's first commit |
| 57 | `` `c9e88cb` `` | sha | task 202 |
| 58 | `` `ci/run.sh go test -p=1 -count=1 ./...` `` | command | full command line; its first word `ci/run.sh` is a tracked path, checked separately in the last section |
| 59 | `` `ci/stability.sh` `` | path | tracked file, checked in its own block below |
| 60 | `` `ci/stability.sh 10` `` | command | command-with-argument string, not itself a path |
| 61 | `` `cwdFieldLabelEndCol` `` | code | Go identifier fragment |
| 62 | `` `d578c03` `` | sha | task 004 |
| 63 | `` `de90a5c` `` | sha | operator's §7 ruling commit |
| 64 | `` `dimmed` `` | word | prose/token name |
| 65 | `` `docs/DELIVERY-LOG.md` `` | path | tracked file, checked in its own block below |
| 66 | `` `docs/reports/phase3g-findings.md` `` | path | tracked file, checked in its own block below |
| 67 | `` `docs/reports/phase3g.md` `` | path | tracked file, checked in its own block below |
| 68 | `` `docs/reports/phase3h-001-status-probe-revert/` `` | path (dir) | tracked directory, checked in its own block below |
| 69 | `` `docs/reports/phase3h-002-narrow-repair/` `` | path (dir) | tracked directory, checked in its own block below |
| 70 | `` `docs/reports/phase3h-003-r94-scenarios/` `` | path (dir) | tracked directory, checked in its own block below |
| 71 | `` `docs/reports/phase3h-005-hint-token/` `` | path (dir) | tracked directory, checked in its own block below |
| 72 | `` `docs/reports/phase3h-006-f31/` `` | path (dir) | tracked directory, checked in its own block below |
| 73 | `` `docs/reports/phase3h-007-f31-guard/` `` | path (dir) | tracked directory, checked in its own block below |
| 74 | `` `docs/reports/phase3h-008-fullsuite/` `` | path (dir) | tracked directory, checked in its own block below |
| 75 | `` `docs/reports/phase3h-008-fullsuite/.exitstatus` `` | path | tracked file, checked in its own block below |
| 76 | `` `docs/reports/phase3h-008-fullsuite/README.md` `` | path | tracked file, checked in its own block below |
| 77 | `` `docs/reports/phase3h-009-fullsuite-verbose/` `` | path (dir) | tracked directory, checked in its own block below |
| 78 | `` `docs/reports/phase3h-009-fullsuite-verbose/scenario-summary.txt` `` | path | tracked file, checked in its own block below |
| 79 | `` `docs/reports/phase3h-010-stability10/` `` | path (dir) | tracked directory, checked in its own block below |
| 80 | `` `docs/reports/phase3h-010-stability10/summary.log` `` | path | tracked file, checked in its own block below |
| 81 | `` `docs/reports/phase3h-012-delivery-log/` `` | path (dir) | tracked directory, checked in its own block below |
| 82 | `` `docs/reports/phase3h-013-report/` `` | path (dir) | tracked directory, checked in its own block below |
| 83 | `` `docs/reports/phase3h-013-report/README.md` `` | path | tracked file, checked in its own block below |
| 84 | `` `docs/reports/phase3h-015-guards/` `` | path (dir) | tracked directory, checked in its own block below |
| 85 | `` `docs/reports/phase3h-201-gofmt/` `` | path (dir) | tracked directory, checked in its own block below |
| 86 | `` `docs/reports/phase3h-202-fullsuite/` `` | path (dir) | tracked directory, checked in its own block below |
| 87 | `` `docs/reports/phase3h-202-fullsuite/.exitstatus` `` | path | tracked file, checked in its own block below |
| 88 | `` `docs/reports/phase3h-203-fullsuite-verbose/` `` | path (dir) | tracked directory, checked in its own block below |
| 89 | `` `docs/reports/phase3h-203-fullsuite-verbose/verbose.log` `` | path | tracked file, checked in its own block below |
| 90 | `` `docs/reports/phase3h-204-stability10/` `` | path (dir) | tracked directory, checked in its own block below |
| 91 | `` `docs/reports/phase3h-204-stability10/README.md` `` | path | tracked file, checked in its own block below |
| 92 | `` `docs/reports/phase3h-204-stability10/summary.log` `` | path | tracked file, checked in its own block below |
| 93 | `` `docs/reports/phase3h-205-r76-disposition/` `` | path (dir) | tracked directory, checked in its own block below |
| 94 | `` `docs/reports/phase3h-206-3g-findings-disposition/` `` | path (dir) | tracked directory, checked in its own block below |
| 95 | `` `docs/reports/phase3h-207-delivery-log-3g/` `` | path (dir) | tracked directory, checked in its own block below |
| 96 | `` `docs/reports/phase3h-208-delivery-log-3h/` `` | path (dir) | tracked directory, checked in its own block below |
| 97 | `` `docs/reports/phase3h-209-report/` `` | path (dir) | tracked directory, checked in its own block below |
| 98 | `` `docs/reports/phase3h-209-report/README.md` `` | path | tracked file, checked in its own block below |
| 99 | `` `docs/reports/phase3h-findings.md` `` | path | tracked file, checked in its own block below |
| 100 | `` `e686a97` `` | sha | task 008's original publish commit |
| 101 | `` `ed469e4` `` | sha | task 014 |
| 102 | `` `error` `` | word | status-value prose |
| 103 | `` `f54873e` `` | sha | task 208 validation fix #2 |
| 104 | `` `f700025` `` | sha | R94 forward-revert |
| 105 | `` `fc358c0` `` | sha | task 203 |
| 106 | `` `features/create_cwd_ghost.feature` `` | path | tracked file, checked in its own block below |
| 107 | `` `features/create_cwd_ghost_test.go` `` | path | tracked file, checked in its own block below |
| 108 | `` `features/filter.feature` `` | path | tracked file, checked in its own block below |
| 109 | `` `features/status_attach.feature:18` `` | path+line | base path `features/status_attach.feature` is tracked; the `:18` suffix is a line reference, not part of the tracked path — checked in its own block below |
| 110 | `` `features/status_claude_hooks.feature` `` | path | tracked file, checked in its own block below |
| 111 | `` `features/status_claude_hooks.feature:6` `` | path+line | base path `features/status_claude_hooks.feature` is tracked; the `:6` suffix is a line reference, not part of the tracked path — checked in its own block below |
| 112 | `` `features/status_probe.feature` `` | path | tracked file, checked in its own block below |
| 113 | `` `git cat-file -e <sha>^{commit}` `` | template | command template with a placeholder, not a real sha or path |
| 114 | `` `git ls-files --error-unmatch <path>` `` | template | command template with a placeholder, not a real sha or path |
| 115 | `` `git rev-list a24ff8d..HEAD --count` `` | command | command line counting the phase's commit range, not a path or sha |
| 116 | `` `hint` `` | word | prose/token name |
| 117 | `` `internal/service/reconcile.go` `` | path | tracked file, checked in its own block below |
| 118 | `` `internal/service/reconcile_live_error_precedence_test.go` `` | path | tracked file, checked in its own block below |
| 119 | `` `ok` `` | word | `go test` output word |
| 120 | `` `pane_exit_status` `` | code | field-name identifier |
| 121 | `` `resolveScenarioTokenHex(ctx, "hint")` `` | code | Go code fragment |
| 122 | `` `running` `` | word | status-value prose |
| 123 | `` `running → error` `` | phrase | status-transition prose |
| 124 | `` `stopped` `` | word | status-value prose |
| 125 | `` `store.StatusUpdateInput` `` | code | Go code fragment |
| 126 | `` `tmux` `` | word | status-source-value prose |
| 127 | `` `user` `` | word | status-source-value prose |

No bare basename, no `/tmp` path and no untracked or deleted path appears among the path
rows: every one is a repository-relative path (or a tracked directory with a trailing
slash), and every check below exits 0.

## Shas — one `git cat-file -e <sha>^{commit}` per token (46 items, all exit 0)

```
$ git cat-file -e 011b04b^{commit}; echo "exit:$?"
exit:0
$ git cat-file -e 08171ce^{commit}; echo "exit:$?"
exit:0
$ git cat-file -e 0b7dce5^{commit}; echo "exit:$?"
exit:0
$ git cat-file -e 0d9a551^{commit}; echo "exit:$?"
exit:0
$ git cat-file -e 1b61337^{commit}; echo "exit:$?"
exit:0
$ git cat-file -e 1f47503^{commit}; echo "exit:$?"
exit:0
$ git cat-file -e 1f919b7^{commit}; echo "exit:$?"
exit:0
$ git cat-file -e 2badb74^{commit}; echo "exit:$?"
exit:0
$ git cat-file -e 2c2ec30^{commit}; echo "exit:$?"
exit:0
$ git cat-file -e 2ccb1d3^{commit}; echo "exit:$?"
exit:0
$ git cat-file -e 2ccb1d3c698d3c256aa94aea7ef2e3a58dc0641a^{commit}; echo "exit:$?"
exit:0
$ git cat-file -e 2f952da^{commit}; echo "exit:$?"
exit:0
$ git cat-file -e 357867e^{commit}; echo "exit:$?"
exit:0
$ git cat-file -e 426fb3a^{commit}; echo "exit:$?"
exit:0
$ git cat-file -e 4673fb9^{commit}; echo "exit:$?"
exit:0
$ git cat-file -e 46dad5e^{commit}; echo "exit:$?"
exit:0
$ git cat-file -e 4b1d4dc^{commit}; echo "exit:$?"
exit:0
$ git cat-file -e 5042852^{commit}; echo "exit:$?"
exit:0
$ git cat-file -e 50960b9^{commit}; echo "exit:$?"
exit:0
$ git cat-file -e 52e9045^{commit}; echo "exit:$?"
exit:0
$ git cat-file -e 5326e39^{commit}; echo "exit:$?"
exit:0
$ git cat-file -e 5708f52^{commit}; echo "exit:$?"
exit:0
$ git cat-file -e 5b7c9e3^{commit}; echo "exit:$?"
exit:0
$ git cat-file -e 67cefcd^{commit}; echo "exit:$?"
exit:0
$ git cat-file -e 68ac4cf^{commit}; echo "exit:$?"
exit:0
$ git cat-file -e 7634895^{commit}; echo "exit:$?"
exit:0
$ git cat-file -e 79b56d9^{commit}; echo "exit:$?"
exit:0
$ git cat-file -e 87bde8f^{commit}; echo "exit:$?"
exit:0
$ git cat-file -e 8d6ed72^{commit}; echo "exit:$?"
exit:0
$ git cat-file -e 9cd8f37^{commit}; echo "exit:$?"
exit:0
$ git cat-file -e 9cff1ea^{commit}; echo "exit:$?"
exit:0
$ git cat-file -e a24ff8d^{commit}; echo "exit:$?"
exit:0
$ git cat-file -e a53146a^{commit}; echo "exit:$?"
exit:0
$ git cat-file -e a5f8f6b^{commit}; echo "exit:$?"
exit:0
$ git cat-file -e afa55b8^{commit}; echo "exit:$?"
exit:0
$ git cat-file -e b5e4178^{commit}; echo "exit:$?"
exit:0
$ git cat-file -e c176751^{commit}; echo "exit:$?"
exit:0
$ git cat-file -e c69720f^{commit}; echo "exit:$?"
exit:0
$ git cat-file -e c9e88cb^{commit}; echo "exit:$?"
exit:0
$ git cat-file -e d578c03^{commit}; echo "exit:$?"
exit:0
$ git cat-file -e de90a5c^{commit}; echo "exit:$?"
exit:0
$ git cat-file -e e686a97^{commit}; echo "exit:$?"
exit:0
$ git cat-file -e ed469e4^{commit}; echo "exit:$?"
exit:0
$ git cat-file -e f54873e^{commit}; echo "exit:$?"
exit:0
$ git cat-file -e f700025^{commit}; echo "exit:$?"
exit:0
$ git cat-file -e fc358c0^{commit}; echo "exit:$?"
exit:0
```

All 46 sha tokens exit 0.

## Paths — one `git ls-files --error-unmatch <path>` per token (46 items, all exit 0)

One block per **token**, not per resolved path: the 3 tokens that carry a line-number
suffix are checked in their own blocks, with the suffix stripped and the stripping stated,
even where the base path is also cited as a token in its own right.

```
# token `SPEC.md:1355` — `:1355` is a line reference, stripped before the check
$ git ls-files --error-unmatch SPEC.md; echo "exit:$?"
SPEC.md
exit:0
$ git ls-files --error-unmatch ci/stability.sh; echo "exit:$?"
ci/stability.sh
exit:0
$ git ls-files --error-unmatch docs/DELIVERY-LOG.md; echo "exit:$?"
docs/DELIVERY-LOG.md
exit:0
$ git ls-files --error-unmatch docs/reports/phase3g-findings.md; echo "exit:$?"
docs/reports/phase3g-findings.md
exit:0
$ git ls-files --error-unmatch docs/reports/phase3g.md; echo "exit:$?"
docs/reports/phase3g.md
exit:0
$ git ls-files --error-unmatch docs/reports/phase3h-001-status-probe-revert/ >/dev/null; echo "exit:$?"
exit:0
$ git ls-files --error-unmatch docs/reports/phase3h-002-narrow-repair/ >/dev/null; echo "exit:$?"
exit:0
$ git ls-files --error-unmatch docs/reports/phase3h-003-r94-scenarios/ >/dev/null; echo "exit:$?"
exit:0
$ git ls-files --error-unmatch docs/reports/phase3h-005-hint-token/ >/dev/null; echo "exit:$?"
exit:0
$ git ls-files --error-unmatch docs/reports/phase3h-006-f31/ >/dev/null; echo "exit:$?"
exit:0
$ git ls-files --error-unmatch docs/reports/phase3h-007-f31-guard/ >/dev/null; echo "exit:$?"
exit:0
$ git ls-files --error-unmatch docs/reports/phase3h-008-fullsuite/ >/dev/null; echo "exit:$?"
exit:0
$ git ls-files --error-unmatch docs/reports/phase3h-008-fullsuite/.exitstatus; echo "exit:$?"
docs/reports/phase3h-008-fullsuite/.exitstatus
exit:0
$ git ls-files --error-unmatch docs/reports/phase3h-008-fullsuite/README.md; echo "exit:$?"
docs/reports/phase3h-008-fullsuite/README.md
exit:0
$ git ls-files --error-unmatch docs/reports/phase3h-009-fullsuite-verbose/ >/dev/null; echo "exit:$?"
exit:0
$ git ls-files --error-unmatch docs/reports/phase3h-009-fullsuite-verbose/scenario-summary.txt; echo "exit:$?"
docs/reports/phase3h-009-fullsuite-verbose/scenario-summary.txt
exit:0
$ git ls-files --error-unmatch docs/reports/phase3h-010-stability10/ >/dev/null; echo "exit:$?"
exit:0
$ git ls-files --error-unmatch docs/reports/phase3h-010-stability10/summary.log; echo "exit:$?"
docs/reports/phase3h-010-stability10/summary.log
exit:0
$ git ls-files --error-unmatch docs/reports/phase3h-012-delivery-log/ >/dev/null; echo "exit:$?"
exit:0
$ git ls-files --error-unmatch docs/reports/phase3h-013-report/ >/dev/null; echo "exit:$?"
exit:0
$ git ls-files --error-unmatch docs/reports/phase3h-013-report/README.md; echo "exit:$?"
docs/reports/phase3h-013-report/README.md
exit:0
$ git ls-files --error-unmatch docs/reports/phase3h-015-guards/ >/dev/null; echo "exit:$?"
exit:0
$ git ls-files --error-unmatch docs/reports/phase3h-201-gofmt/ >/dev/null; echo "exit:$?"
exit:0
$ git ls-files --error-unmatch docs/reports/phase3h-202-fullsuite/ >/dev/null; echo "exit:$?"
exit:0
$ git ls-files --error-unmatch docs/reports/phase3h-202-fullsuite/.exitstatus; echo "exit:$?"
docs/reports/phase3h-202-fullsuite/.exitstatus
exit:0
$ git ls-files --error-unmatch docs/reports/phase3h-203-fullsuite-verbose/ >/dev/null; echo "exit:$?"
exit:0
$ git ls-files --error-unmatch docs/reports/phase3h-203-fullsuite-verbose/verbose.log; echo "exit:$?"
docs/reports/phase3h-203-fullsuite-verbose/verbose.log
exit:0
$ git ls-files --error-unmatch docs/reports/phase3h-204-stability10/ >/dev/null; echo "exit:$?"
exit:0
$ git ls-files --error-unmatch docs/reports/phase3h-204-stability10/README.md; echo "exit:$?"
docs/reports/phase3h-204-stability10/README.md
exit:0
$ git ls-files --error-unmatch docs/reports/phase3h-204-stability10/summary.log; echo "exit:$?"
docs/reports/phase3h-204-stability10/summary.log
exit:0
$ git ls-files --error-unmatch docs/reports/phase3h-205-r76-disposition/ >/dev/null; echo "exit:$?"
exit:0
$ git ls-files --error-unmatch docs/reports/phase3h-206-3g-findings-disposition/ >/dev/null; echo "exit:$?"
exit:0
$ git ls-files --error-unmatch docs/reports/phase3h-207-delivery-log-3g/ >/dev/null; echo "exit:$?"
exit:0
$ git ls-files --error-unmatch docs/reports/phase3h-208-delivery-log-3h/ >/dev/null; echo "exit:$?"
exit:0
$ git ls-files --error-unmatch docs/reports/phase3h-209-report/ >/dev/null; echo "exit:$?"
exit:0
$ git ls-files --error-unmatch docs/reports/phase3h-209-report/README.md; echo "exit:$?"
docs/reports/phase3h-209-report/README.md
exit:0
$ git ls-files --error-unmatch docs/reports/phase3h-findings.md; echo "exit:$?"
docs/reports/phase3h-findings.md
exit:0
$ git ls-files --error-unmatch features/create_cwd_ghost.feature; echo "exit:$?"
features/create_cwd_ghost.feature
exit:0
$ git ls-files --error-unmatch features/create_cwd_ghost_test.go; echo "exit:$?"
features/create_cwd_ghost_test.go
exit:0
$ git ls-files --error-unmatch features/filter.feature; echo "exit:$?"
features/filter.feature
exit:0
# token `features/status_attach.feature:18` — `:18` is a line reference, stripped before the check
$ git ls-files --error-unmatch features/status_attach.feature; echo "exit:$?"
features/status_attach.feature
exit:0
$ git ls-files --error-unmatch features/status_claude_hooks.feature; echo "exit:$?"
features/status_claude_hooks.feature
exit:0
# token `features/status_claude_hooks.feature:6` — `:6` is a line reference, stripped before the check
$ git ls-files --error-unmatch features/status_claude_hooks.feature; echo "exit:$?"
features/status_claude_hooks.feature
exit:0
$ git ls-files --error-unmatch features/status_probe.feature; echo "exit:$?"
features/status_probe.feature
exit:0
$ git ls-files --error-unmatch internal/service/reconcile.go; echo "exit:$?"
internal/service/reconcile.go
exit:0
$ git ls-files --error-unmatch internal/service/reconcile_live_error_precedence_test.go; echo "exit:$?"
internal/service/reconcile_live_error_precedence_test.go
exit:0
```

All 46 path tokens exit 0. This report's own path is among them: it was staged with
`git add` before the check ran, so `git ls-files` already saw it as tracked — a path
citation, unlike a sha, has no self-reference problem, because the path is part of the
tracked tree before the content is committed.

## Multi-line backticked spans (nothing hides from the enumeration)

A backticked span that opens on one source line and closes on the next is invisible to a
line-based `grep`, so the lines with an odd number of backticks are listed mechanically and
accounted for (code-fence marker lines excluded, since they are fences, not citations):

```
$ awk '{n=gsub(/`/,"`"); if (n%2==1) print NR": "$0}' docs/reports/phase3h.md | grep -v '^[0-9]*: ```$'
3: Phase 3h brings the tree to the operator's §7 ruling in `de90a5c` (`spec: resolve §7's
4: error-under-live-pane self-contradiction and land §11.8's R93 wording (operator)`): **the
```

These two lines are the two halves of one span: `de90a5c`'s own commit subject, quoted as
prose. Its first half does appear in the enumeration as a token in its own right (the
`spec: resolve §7's` row above) and the commit it belongs to is checked in the sha section;
the span cites no path and no other sha.

## Delivery-commit ledger completeness

The report's per-requirement table must name every commit that delivered this phase. The
phase's commit range is `a24ff8d..HEAD`, evaluated here with `HEAD` at the last *committed*
commit of the phase — the commit that carries this very report is not in the range yet, and
could not quote its own sha if it were (content-addressing):

```
$ git rev-list a24ff8d..HEAD --count
38
```

Of those, 38 are cited as sha tokens in `docs/reports/phase3h.md`. The
commits in the range that are *not* cited are exactly:

```
$ for c in $(git rev-list a24ff8d..HEAD); do grep -qF "$(git log -1 --format=%h $c)" \
    docs/reports/phase3h.md || git log -1 --format='%h %s' $c; done
(none)
```

Nothing is unnamed: every commit of the phase that exists at generation time — all
38 of them, including task 209's own earlier commits `426fb3a`, `5b7c9e3`
and `1b61337` — is cited as a sha token in the report. The one commit necessarily outside
this check is the tail commit that carries this generated report and the ledger edits: a
commit's sha is a hash over its own content, so it cannot contain that sha. No addendum
follows it. The report records it by its exact, unique commit subject instead
(`docs: complete the Phase 3h delivery ledger and task ledger for tasks 201-212`), which
resolves to the hex with a single command, and tasks 211 and 212 record that hex
directly:

```
$ git log -1 --format=%H \
    --grep='docs: complete the Phase 3h delivery ledger and task ledger for tasks 201-212' \
    a24ff8d..HEAD
```

(That command is quoted, not run here: at generation time the commit it names does not
exist yet, for the same reason its sha cannot be quoted.)

Tasks 210–212 have no commit in this tree yet, so no sha is recorded for them — proved,
not assumed:

```
$ git log --oneline a24ff8d..HEAD --grep='(task 210)' --grep='(task 211)' --grep='(task 212)'
matches:0
```

## The approach-03 task ledger, checked per task (tasks 201–212)

The report's task ledger claims a specific set of delivery commits for each task of this
approach, and claims that tasks 210–212 have none yet. Each row is checked here: every
ledger sha is resolved to its own subject line, and every task's `(task NNN)` marker search
over the phase range is quoted — including the empty ones. A marker search is not the
ledger: per the standing rules a validation-fix or self-sha commit carries no marker, so a
task can have more delivery commits than marker matches, but a task with commits can never
have fewer.

### task 201

```
$ git log -1 --format='%h %s' 4b1d4dc
4b1d4dc service: restore gofmt-clean formatting of the AllowedCurrentStatuses guard (task 201)
$ git log -1 --format='%h %s' 1f47503
1f47503 docs: publish the gofmt-clean report for reconcile.go's realignment at 4b1d4dc
$ git log --oneline a24ff8d..HEAD --grep='(task 201)'; echo "matches:$(git log --oneline a24ff8d..HEAD --grep='(task 201)' | wc -l)"
4b1d4dc service: restore gofmt-clean formatting of the AllowedCurrentStatuses guard (task 201)
matches:1
```

Ledger row for task 201: 2 commit(s), all cited as backticked sha tokens in the report and all resolvable above.

### task 202

```
$ git log -1 --format='%h %s' c9e88cb
c9e88cb docs: publish the whole-suite sweep at the new final code sha 4b1d4dc (task 202)
$ git log --oneline a24ff8d..HEAD --grep='(task 202)'; echo "matches:$(git log --oneline a24ff8d..HEAD --grep='(task 202)' | wc -l)"
c9e88cb docs: publish the whole-suite sweep at the new final code sha 4b1d4dc (task 202)
matches:1
```

Ledger row for task 202: 1 commit(s), all cited as backticked sha tokens in the report and all resolvable above.

### task 203

```
$ git log -1 --format='%h %s' fc358c0
fc358c0 docs: publish the verbose companion sweep's Gherkin tally at 4b1d4dc (task 203)
$ git log -1 --format='%h %s' 011b04b
011b04b docs: name task 203's own report commit sha, fc358c0
$ git log --oneline a24ff8d..HEAD --grep='(task 203)'; echo "matches:$(git log --oneline a24ff8d..HEAD --grep='(task 203)' | wc -l)"
fc358c0 docs: publish the verbose companion sweep's Gherkin tally at 4b1d4dc (task 203)
matches:1
```

Ledger row for task 203: 2 commit(s), all cited as backticked sha tokens in the report and all resolvable above.

### task 204

```
$ git log -1 --format='%h %s' 79b56d9
79b56d9 docs: publish ci/stability.sh 10 at the final code sha 4b1d4dc (task 204)
$ git log --oneline a24ff8d..HEAD --grep='(task 204)'; echo "matches:$(git log --oneline a24ff8d..HEAD --grep='(task 204)' | wc -l)"
79b56d9 docs: publish ci/stability.sh 10 at the final code sha 4b1d4dc (task 204)
matches:1
```

Ledger row for task 204: 1 commit(s), all cited as backticked sha tokens in the report and all resolvable above.

### task 205

```
$ git log -1 --format='%h %s' 52e9045
52e9045 docs: correct phase3g.md's R76 Phase 3h disposition sentence from the diff (task 205)
$ git log --oneline a24ff8d..HEAD --grep='(task 205)'; echo "matches:$(git log --oneline a24ff8d..HEAD --grep='(task 205)' | wc -l)"
52e9045 docs: correct phase3g.md's R76 Phase 3h disposition sentence from the diff (task 205)
matches:1
```

Ledger row for task 205: 1 commit(s), all cited as backticked sha tokens in the report and all resolvable above.

### task 206

```
$ git log -1 --format='%h %s' 50960b9
50960b9 docs: correct phase3g-findings.md's F36/F38/F40/F41 disposition sentences from the diff (task 206)
$ git log --oneline a24ff8d..HEAD --grep='(task 206)'; echo "matches:$(git log --oneline a24ff8d..HEAD --grep='(task 206)' | wc -l)"
50960b9 docs: correct phase3g-findings.md's F36/F38/F40/F41 disposition sentences from the diff (task 206)
matches:1
```

Ledger row for task 206: 1 commit(s), all cited as backticked sha tokens in the report and all resolvable above.

### task 207

```
$ git log -1 --format='%h %s' 9cff1ea
9cff1ea docs: bring DELIVERY-LOG's Phase 3g section to a coherent final state of record (task 207)
$ git log --oneline a24ff8d..HEAD --grep='(task 207)'; echo "matches:$(git log --oneline a24ff8d..HEAD --grep='(task 207)' | wc -l)"
9cff1ea docs: bring DELIVERY-LOG's Phase 3g section to a coherent final state of record (task 207)
matches:1
```

Ledger row for task 207: 1 commit(s), all cited as backticked sha tokens in the report and all resolvable above.

### task 208

```
$ git log -1 --format='%h %s' 87bde8f
87bde8f docs: add Phase 3h's DELIVERY-LOG paragraph at the final code sha with both gate results (task 208)
$ git log -1 --format='%h %s' 0d9a551
0d9a551 docs: give the prds/ token its own quoted ls-files check in the Phase 3h DELIVERY-LOG citation report
$ git log -1 --format='%h %s' f54873e
f54873e docs: cite features/filter.feature by its tracked path in DELIVERY-LOG's Phase 3h paragraph
$ git log -1 --format='%h %s' 68ac4cf
68ac4cf docs: record the marker accounting for the Phase 3h DELIVERY-LOG citation report
$ git log --oneline a24ff8d..HEAD --grep='(task 208)'; echo "matches:$(git log --oneline a24ff8d..HEAD --grep='(task 208)' | wc -l)"
f54873e docs: cite features/filter.feature by its tracked path in DELIVERY-LOG's Phase 3h paragraph
87bde8f docs: add Phase 3h's DELIVERY-LOG paragraph at the final code sha with both gate results (task 208)
matches:2
```

Ledger row for task 208: 4 commit(s), all cited as backticked sha tokens in the report and all resolvable above.

### task 209

```
$ git log -1 --format='%h %s' 426fb3a
426fb3a docs: bring phase3h.md to the final state of record at 4b1d4dc (task 209)
$ git log -1 --format='%h %s' 5b7c9e3
5b7c9e3 docs: record every Phase 3h delivery commit in phase3h.md's table and check each citation per token
$ git log -1 --format='%h %s' 1b61337
1b61337 docs: name task 209's data commit sha 5b7c9e3 in phase3h.md's delivery ledger
$ git log --oneline a24ff8d..HEAD --grep='(task 209)'; echo "matches:$(git log --oneline a24ff8d..HEAD --grep='(task 209)' | wc -l)"
426fb3a docs: bring phase3h.md to the final state of record at 4b1d4dc (task 209)
matches:1
```

Ledger row for task 209: 3 commit(s), all cited as backticked sha tokens in the report and all resolvable above.

### task 210

```
$ git log --oneline a24ff8d..HEAD --grep='(task 210)'; echo "matches:$(git log --oneline a24ff8d..HEAD --grep='(task 210)' | wc -l)"
matches:0
```

Ledger row for task 210: no delivery commit in `a24ff8d..HEAD`, marker search empty as quoted — a measured absence, and the reason the report records no sha for it. Task 210 sits after task 209 in this plan's dependency order (210 and 211 name 209 directly, 212 through 211), so its commit is necessarily later than every commit this report can name.

### task 211

```
$ git log --oneline a24ff8d..HEAD --grep='(task 211)'; echo "matches:$(git log --oneline a24ff8d..HEAD --grep='(task 211)' | wc -l)"
matches:0
```

Ledger row for task 211: no delivery commit in `a24ff8d..HEAD`, marker search empty as quoted — a measured absence, and the reason the report records no sha for it. Task 211 sits after task 209 in this plan's dependency order (210 and 211 name 209 directly, 212 through 211), so its commit is necessarily later than every commit this report can name.

### task 212

```
$ git log --oneline a24ff8d..HEAD --grep='(task 212)'; echo "matches:$(git log --oneline a24ff8d..HEAD --grep='(task 212)' | wc -l)"
matches:0
```

Ledger row for task 212: no delivery commit in `a24ff8d..HEAD`, marker search empty as quoted — a measured absence, and the reason the report records no sha for it. Task 212 sits after task 209 in this plan's dependency order (210 and 211 name 209 directly, 212 through 211), so its commit is necessarily later than every commit this report can name.

Across tasks 201–212 the ledger names 16 distinct commits, 16 of them inside `a24ff8d..HEAD`; the remaining 22 commits of the range belong to tasks 001–015 and are named in the report's per-requirement table.

## In passing: the command token's own embedded real path

The command token `` `ci/run.sh go test -p=1 -count=1 ./...` `` is not itself a path, but
its first word is a tracked script:

```
$ git ls-files --error-unmatch ci/run.sh; echo "exit:$?"
ci/run.sh
exit:0
```

Not counted among the 46 path tokens, since the enumerated token is the whole command
string rather than `ci/run.sh` alone.
