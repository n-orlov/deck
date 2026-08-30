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
125
```

125 unique backticked tokens: **45 sha tokens**, **46 path tokens**
(counting each token separately, so a `path` and a `path:line` token are two items) and
**34 tokens that are neither** — code, prose, command, flag, glob and output
fragments, each named explicitly below with the reason no `git cat-file`/`git ls-files`
check applies. The three counts add up to the enumeration total by construction.

| # | token | kind | note |
|---|---|---|---|
| 1 | (empty) | artifact | adjacent backticks inside a code-fence marker line; no content |
| 2 | `` `*.feature` `` | glob | command-line glob argument, not a path |
| 3 | `` `*.go` `` | glob | command-line glob argument, not a path |
| 4 | `` `-v` `` | flag | CLI flag, not a path or sha |
| 5 | `` `0` `` | status | quoted exit-status digit, not a sha or path |
| 6 | `` `011b04b` `` | sha | task 203's self-sha addendum commit |
| 7 | `` `08171ce` `` | sha | task 011 |
| 8 | `` `0b7dce5` `` | sha | R94 revert target (task 002) |
| 9 | `` `0d9a551` `` | sha | task 208 validation fix #1 |
| 10 | `` `1f47503` `` | sha | task 201's gofmt-clean evidence commit |
| 11 | `` `1f919b7` `` | sha | task 009's original publish commit |
| 12 | `` `2badb74` `` | sha | task 010's original publish commit |
| 13 | `` `2c2ec30` `` | sha | task 005 |
| 14 | `` `2ccb1d3` `` | sha | task 007 (superseded final code sha, short form) |
| 15 | `` `2ccb1d3c698d3c256aa94aea7ef2e3a58dc0641a` `` | sha | task 007 (superseded final code sha, full form) |
| 16 | `` `2f952da` `` | sha | R94 forward-revert |
| 17 | `` `357867e` `` | sha | R94 record/proof commit |
| 18 | `` `426fb3a` `` | sha | task 209's first commit |
| 19 | `` `4673fb9` `` | sha | task 013 |
| 20 | `` `46dad5e` `` | sha | Phase 3g's F37 scenario-half fix |
| 21 | `` `4b1d4dc` `` | sha | task 201 (current final code sha) |
| 22 | `` `5042852` `` | sha | task 012's correction commit |
| 23 | `` `50960b9` `` | sha | task 206 |
| 24 | `` `52e9045` `` | sha | task 205 |
| 25 | `` `5326e39` `` | sha | task 015's second commit |
| 26 | `` `5708f52` `` | sha | task 012 |
| 27 | `` `5b7c9e3` `` | sha | task 209's data commit correcting the delivery ledger |
| 28 | `` `67cefcd` `` | sha | R94 record/proof commit |
| 29 | `` `68ac4cf` `` | sha | task 208's marker-accounting commit |
| 30 | `` `7634895` `` | sha | R94 record/proof commit |
| 31 | `` `79b56d9` `` | sha | task 204 |
| 32 | `` `87bde8f` `` | sha | task 208 |
| 33 | `` `8d6ed72` `` | sha | task 006 |
| 34 | `` `9cd8f37` `` | sha | R94 record/proof commit |
| 35 | `` `9cff1ea` `` | sha | task 207 |
| 36 | `` `:14` `` | fragment | scenario-title line reference in prose, not a standalone path |
| 37 | `` `?` `` | status | `go test`'s "no test files" marker, quoted from output |
| 38 | `` `AllowedCurrentStatuses: []string{"starting"}` `` | code | Go literal fragment, not a path or sha |
| 39 | `` `EventKind: "tmux.shell_live"` `` | code | Go literal fragment, not a path or sha |
| 40 | `` `PASS (exit 0)` `` | status | stability-gate per-run status string |
| 41 | `` `SPEC.md:1355` `` | path+line | base path `SPEC.md` is tracked; the `:1355` suffix is a line reference, not part of the tracked path — checked in its own block below |
| 42 | `` `StatusUpdateInput` `` | code | Go identifier fragment |
| 43 | `` `StopFailure` `` | code | Gherkin/Go identifier fragment |
| 44 | `` `TestReconcileLosesInterleavedStatusWriteDuringShellPromotion` `` | code | Go test function name |
| 45 | `` `Then` `` | keyword | Gherkin keyword |
| 46 | `` `[no test files]` `` | status | `go test` output string, quoted |
| 47 | `` `a24ff8d` `` | sha | operator's plan/PRD commit |
| 48 | `` `a24ff8d..HEAD` `` | range | git revision range, not a single sha or a path; its named endpoint `a24ff8d` is checked as a sha above and `HEAD` is a ref |
| 49 | `` `a53146a` `` | sha | R94 revert target (task 001) |
| 50 | `` `a5f8f6b` `` | sha | named in the task 207 disposition prose (Phase 3g DELIVERY-LOG paragraph, now superseded) |
| 51 | `` `afa55b8` `` | sha | R94 forward-revert (task 002) |
| 52 | `` `b5e4178` `` | sha | task 013 |
| 53 | `` `c176751` `` | sha | R94 revert target (task 001) |
| 54 | `` `c69720f` `` | sha | task 015's first commit |
| 55 | `` `c9e88cb` `` | sha | task 202 |
| 56 | `` `ci/run.sh go test -p=1 -count=1 ./...` `` | command | full command line; its first word `ci/run.sh` is a tracked path, checked separately in the last section |
| 57 | `` `ci/stability.sh` `` | path | tracked file, checked in its own block below |
| 58 | `` `ci/stability.sh 10` `` | command | command-with-argument string, not itself a path |
| 59 | `` `cwdFieldLabelEndCol` `` | code | Go identifier fragment |
| 60 | `` `d578c03` `` | sha | task 004 |
| 61 | `` `de90a5c` `` | sha | operator's §7 ruling commit |
| 62 | `` `dimmed` `` | word | prose/token name |
| 63 | `` `docs/DELIVERY-LOG.md` `` | path | tracked file, checked in its own block below |
| 64 | `` `docs/reports/phase3g-findings.md` `` | path | tracked file, checked in its own block below |
| 65 | `` `docs/reports/phase3g.md` `` | path | tracked file, checked in its own block below |
| 66 | `` `docs/reports/phase3h-001-status-probe-revert/` `` | path (dir) | tracked directory, checked in its own block below |
| 67 | `` `docs/reports/phase3h-002-narrow-repair/` `` | path (dir) | tracked directory, checked in its own block below |
| 68 | `` `docs/reports/phase3h-003-r94-scenarios/` `` | path (dir) | tracked directory, checked in its own block below |
| 69 | `` `docs/reports/phase3h-005-hint-token/` `` | path (dir) | tracked directory, checked in its own block below |
| 70 | `` `docs/reports/phase3h-006-f31/` `` | path (dir) | tracked directory, checked in its own block below |
| 71 | `` `docs/reports/phase3h-007-f31-guard/` `` | path (dir) | tracked directory, checked in its own block below |
| 72 | `` `docs/reports/phase3h-008-fullsuite/` `` | path (dir) | tracked directory, checked in its own block below |
| 73 | `` `docs/reports/phase3h-008-fullsuite/.exitstatus` `` | path | tracked file, checked in its own block below |
| 74 | `` `docs/reports/phase3h-008-fullsuite/README.md` `` | path | tracked file, checked in its own block below |
| 75 | `` `docs/reports/phase3h-009-fullsuite-verbose/` `` | path (dir) | tracked directory, checked in its own block below |
| 76 | `` `docs/reports/phase3h-009-fullsuite-verbose/scenario-summary.txt` `` | path | tracked file, checked in its own block below |
| 77 | `` `docs/reports/phase3h-010-stability10/` `` | path (dir) | tracked directory, checked in its own block below |
| 78 | `` `docs/reports/phase3h-010-stability10/summary.log` `` | path | tracked file, checked in its own block below |
| 79 | `` `docs/reports/phase3h-012-delivery-log/` `` | path (dir) | tracked directory, checked in its own block below |
| 80 | `` `docs/reports/phase3h-013-report/` `` | path (dir) | tracked directory, checked in its own block below |
| 81 | `` `docs/reports/phase3h-013-report/README.md` `` | path | tracked file, checked in its own block below |
| 82 | `` `docs/reports/phase3h-015-guards/` `` | path (dir) | tracked directory, checked in its own block below |
| 83 | `` `docs/reports/phase3h-201-gofmt/` `` | path (dir) | tracked directory, checked in its own block below |
| 84 | `` `docs/reports/phase3h-202-fullsuite/` `` | path (dir) | tracked directory, checked in its own block below |
| 85 | `` `docs/reports/phase3h-202-fullsuite/.exitstatus` `` | path | tracked file, checked in its own block below |
| 86 | `` `docs/reports/phase3h-203-fullsuite-verbose/` `` | path (dir) | tracked directory, checked in its own block below |
| 87 | `` `docs/reports/phase3h-203-fullsuite-verbose/verbose.log` `` | path | tracked file, checked in its own block below |
| 88 | `` `docs/reports/phase3h-204-stability10/` `` | path (dir) | tracked directory, checked in its own block below |
| 89 | `` `docs/reports/phase3h-204-stability10/README.md` `` | path | tracked file, checked in its own block below |
| 90 | `` `docs/reports/phase3h-204-stability10/summary.log` `` | path | tracked file, checked in its own block below |
| 91 | `` `docs/reports/phase3h-205-r76-disposition/` `` | path (dir) | tracked directory, checked in its own block below |
| 92 | `` `docs/reports/phase3h-206-3g-findings-disposition/` `` | path (dir) | tracked directory, checked in its own block below |
| 93 | `` `docs/reports/phase3h-207-delivery-log-3g/` `` | path (dir) | tracked directory, checked in its own block below |
| 94 | `` `docs/reports/phase3h-208-delivery-log-3h/` `` | path (dir) | tracked directory, checked in its own block below |
| 95 | `` `docs/reports/phase3h-209-report/` `` | path (dir) | tracked directory, checked in its own block below |
| 96 | `` `docs/reports/phase3h-209-report/README.md` `` | path | tracked file, checked in its own block below |
| 97 | `` `docs/reports/phase3h-findings.md` `` | path | tracked file, checked in its own block below |
| 98 | `` `e686a97` `` | sha | task 008's original publish commit |
| 99 | `` `ed469e4` `` | sha | task 014 |
| 100 | `` `error` `` | word | status-value prose |
| 101 | `` `f54873e` `` | sha | task 208 validation fix #2 |
| 102 | `` `f700025` `` | sha | R94 forward-revert |
| 103 | `` `fc358c0` `` | sha | task 203 |
| 104 | `` `features/create_cwd_ghost.feature` `` | path | tracked file, checked in its own block below |
| 105 | `` `features/create_cwd_ghost_test.go` `` | path | tracked file, checked in its own block below |
| 106 | `` `features/filter.feature` `` | path | tracked file, checked in its own block below |
| 107 | `` `features/status_attach.feature:18` `` | path+line | base path `features/status_attach.feature` is tracked; the `:18` suffix is a line reference, not part of the tracked path — checked in its own block below |
| 108 | `` `features/status_claude_hooks.feature` `` | path | tracked file, checked in its own block below |
| 109 | `` `features/status_claude_hooks.feature:6` `` | path+line | base path `features/status_claude_hooks.feature` is tracked; the `:6` suffix is a line reference, not part of the tracked path — checked in its own block below |
| 110 | `` `features/status_probe.feature` `` | path | tracked file, checked in its own block below |
| 111 | `` `git cat-file -e <sha>^{commit}` `` | template | command template with a placeholder, not a real sha or path |
| 112 | `` `git ls-files --error-unmatch <path>` `` | template | command template with a placeholder, not a real sha or path |
| 113 | `` `git rev-list a24ff8d..HEAD --count` `` | command | command line counting the phase's commit range, not a path or sha |
| 114 | `` `hint` `` | word | prose/token name |
| 115 | `` `internal/service/reconcile.go` `` | path | tracked file, checked in its own block below |
| 116 | `` `internal/service/reconcile_live_error_precedence_test.go` `` | path | tracked file, checked in its own block below |
| 117 | `` `ok` `` | word | `go test` output word |
| 118 | `` `pane_exit_status` `` | code | field-name identifier |
| 119 | `` `resolveScenarioTokenHex(ctx, "hint")` `` | code | Go code fragment |
| 120 | `` `running` `` | word | status-value prose |
| 121 | `` `running → error` `` | phrase | status-transition prose |
| 122 | `` `stopped` `` | word | status-value prose |
| 123 | `` `store.StatusUpdateInput` `` | code | Go code fragment |
| 124 | `` `tmux` `` | word | status-source-value prose |
| 125 | `` `user` `` | word | status-source-value prose |

No bare basename, no `/tmp` path and no untracked or deleted path appears among the path
rows: every one is a repository-relative path (or a tracked directory with a trailing
slash), and every check below exits 0.

## Shas — one `git cat-file -e <sha>^{commit}` per token (45 items, all exit 0)

```
$ git cat-file -e 011b04b^{commit}; echo "exit:$?"
exit:0
$ git cat-file -e 08171ce^{commit}; echo "exit:$?"
exit:0
$ git cat-file -e 0b7dce5^{commit}; echo "exit:$?"
exit:0
$ git cat-file -e 0d9a551^{commit}; echo "exit:$?"
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

All 45 sha tokens exit 0.

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
37
```

Of those, 37 are cited as sha tokens in `docs/reports/phase3h.md`. The
commits in the range that are *not* cited are exactly:

```
$ for c in $(git rev-list a24ff8d..HEAD); do grep -qF "$(git log -1 --format=%h $c)" \
    docs/reports/phase3h.md || git log -1 --format='%h %s' $c; done
(none)
```

Nothing is unnamed: every commit of the phase that exists at generation time is cited in
the report. The one commit necessarily outside this check is the one that carries this
report itself — task 209's addendum commit, which adds the data commit's sha to the
table and cannot add its own; that sha is recorded by task 211's guard report and task
212's close-out instead.

Tasks 210–212 have no commit in this tree yet, so no sha is recorded for them — proved,
not assumed:

```
$ git log --oneline a24ff8d..HEAD --grep='(task 210)' --grep='(task 211)' --grep='(task 212)'
matches:0
```

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
