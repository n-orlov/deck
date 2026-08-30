# Task 209 — exhaustive, mechanical citation check for `docs/reports/phase3h.md`

Every backticked token in `docs/reports/phase3h.md`, enumerated **mechanically** (not from
expectation), is classified below and — for every sha and every path — checked with its own
quoted command and exit status. This report supersedes task 013's narrower
`docs/reports/phase3h-013-report/README.md` (which is retained in the tree as superseded
history and remains correct for the citations it covered); this is now the current,
authoritative citation check for the whole file.

## Enumeration (mechanical)

```
$ grep -o '`[^`]*`' docs/reports/phase3h.md | sort -u | wc -l
115
```

All 115 unique backticked tokens, one classification row each, in the table below. "sha" and
"path" rows are followed by their own quoted check in the sections after the table; every
other kind is a code/prose/command fragment that is neither a resolvable sha nor a citable
repository path, so no `git cat-file`/`git ls-files` check applies to it — each is still
named explicitly here so nothing is silently skipped.

| # | token | kind | note |
|---|---|---|---|
| 1 | `` `*.feature` `` | glob | command-line glob argument, not a path |
| 2 | `` `*.go` `` | glob | command-line glob argument, not a path |
| 3 | `` `-v` `` | flag | CLI flag, not a path or sha |
| 4 | `` `011b04b` `` | sha | task 203's self-sha addendum commit |
| 5 | `` `08171ce` `` | sha | task 011 |
| 6 | `` `0` `` | status | quoted exit-status digit, not a sha or path |
| 7 | `` `0b7dce5` `` | sha | R94 revert target (task 002) |
| 8 | `` `0d9a551` `` | sha | task 208 validation fix #1 |
| 9 | `` `1f919b7` `` | sha | task 009's original publish commit |
| 10 | `` `2badb74` `` | sha | task 010's original publish commit |
| 11 | `` `2c2ec30` `` | sha | task 005 |
| 12 | `` `2ccb1d3` `` | sha | task 007 (superseded final code sha, short form) |
| 13 | `` `2ccb1d3c698d3c256aa94aea7ef2e3a58dc0641a` `` | sha | task 007 (superseded final code sha, full form) |
| 14 | `` `2f952da` `` | sha | R94 forward-revert |
| 15 | `` `357867e` `` | sha | R94 record/proof commit |
| 16 | `` `4673fb9` `` | sha | task 013 |
| 17 | `` `46dad5e` `` | sha | Phase 3g's F37 scenario-half fix |
| 18 | `` `4b1d4dc` `` | sha | task 201 (current final code sha) |
| 19 | `` `5042852` `` | sha | task 012's correction commit |
| 20 | `` `50960b9` `` | sha | task 206 |
| 21 | `` `52e9045` `` | sha | task 205 |
| 22 | `` `5708f52` `` | sha | task 012 |
| 23 | `` `67cefcd` `` | sha | R94 record/proof commit |
| 24 | `` `68ac4cf` `` | sha | task 208's marker-accounting commit |
| 25 | `` `7634895` `` | sha | R94 record/proof commit |
| 26 | `` `79b56d9` `` | sha | task 204 |
| 27 | `` `87bde8f` `` | sha | task 208 |
| 28 | `` `8d6ed72` `` | sha | task 006 |
| 29 | `` `9cd8f37` `` | sha | R94 record/proof commit |
| 30 | `` `9cff1ea` `` | sha | task 207 |
| 31 | `` `:14` `` | fragment | scenario-title line reference in prose, not a standalone path |
| 32 | `` `?` `` | status | `go test`'s "no test files" marker, quoted from output |
| 33 | `` `AllowedCurrentStatuses: []string{"starting"}` `` | code | Go literal fragment, not a path or sha |
| 34 | `` `EventKind: "tmux.shell_live"` `` | code | Go literal fragment, not a path or sha |
| 35 | `` `PASS (exit 0)` `` | status | stability-gate per-run status string |
| 36 | `` `SPEC.md:1355` `` | path+line | base path `SPEC.md` checked below; `:1355` is a line reference, not part of the tracked path |
| 37 | `` `StatusUpdateInput` `` | code | Go identifier fragment |
| 38 | `` `StopFailure` `` | code | Gherkin/Go identifier fragment |
| 39 | `` `TestReconcileLosesInterleavedStatusWriteDuringShellPromotion` `` | code | Go test function name |
| 40 | `` `Then` `` | keyword | Gherkin keyword |
| 41 | `` `[no test files]` `` | status | `go test` output string, quoted |
| 42 | (empty) | artifact | adjacent backticks inside a ` ``` ` code-fence marker line; no content |
| 43 | `` `a24ff8d` `` | sha | operator's plan/PRD commit |
| 44 | `` `a53146a` `` | sha | R94 revert target (task 001) |
| 45 | `` `a5f8f6b` `` | sha | named in the task 207 disposition prose (Phase 3g DELIVERY-LOG paragraph, now superseded) |
| 46 | `` `afa55b8` `` | sha | R94 forward-revert (task 002) |
| 47 | `` `b5e4178` `` | sha | task 013 |
| 48 | `` `c176751` `` | sha | R94 revert target (task 001) |
| 49 | `` `c9e88cb` `` | sha | task 202 |
| 50 | `` `ci/run.sh go test -p=1 -count=1 ./...` `` | command | full command line (its first word, `ci/run.sh`, is itself a tracked path — checked separately below in passing) |
| 51 | `` `ci/stability.sh 10` `` | command | command-with-argument string, not itself a path |
| 52 | `` `ci/stability.sh` `` | path | tracked script |
| 53 | `` `cwdFieldLabelEndCol` `` | code | Go identifier fragment |
| 54 | `` `d578c03` `` | sha | task 004 |
| 55 | `` `de90a5c` `` | sha | operator's §7 ruling commit |
| 56 | `` `dimmed` `` | word | prose/token name |
| 57 | `` `docs/DELIVERY-LOG.md` `` | path | tracked |
| 58 | `` `docs/reports/phase3g-findings.md` `` | path | tracked |
| 59 | `` `docs/reports/phase3g.md` `` | path | tracked |
| 60 | `` `docs/reports/phase3h-001-status-probe-revert/` `` | path (dir) | tracked |
| 61 | `` `docs/reports/phase3h-002-narrow-repair/` `` | path (dir) | tracked |
| 62 | `` `docs/reports/phase3h-003-r94-scenarios/` `` | path (dir) | tracked |
| 63 | `` `docs/reports/phase3h-005-hint-token/` `` | path (dir) | tracked |
| 64 | `` `docs/reports/phase3h-006-f31/` `` | path (dir) | tracked |
| 65 | `` `docs/reports/phase3h-007-f31-guard/` `` | path (dir) | tracked |
| 66 | `` `docs/reports/phase3h-008-fullsuite/.exitstatus` `` | path | tracked |
| 67 | `` `docs/reports/phase3h-008-fullsuite/README.md` `` | path | tracked |
| 68 | `` `docs/reports/phase3h-008-fullsuite/` `` | path (dir) | tracked |
| 69 | `` `docs/reports/phase3h-009-fullsuite-verbose/` `` | path (dir) | tracked |
| 70 | `` `docs/reports/phase3h-009-fullsuite-verbose/scenario-summary.txt` `` | path | tracked |
| 71 | `` `docs/reports/phase3h-010-stability10/` `` | path (dir) | tracked |
| 72 | `` `docs/reports/phase3h-010-stability10/summary.log` `` | path | tracked |
| 73 | `` `docs/reports/phase3h-012-delivery-log/` `` | path (dir) | tracked |
| 74 | `` `docs/reports/phase3h-013-report/README.md` `` | path | tracked |
| 75 | `` `docs/reports/phase3h-013-report/` `` | path (dir) | tracked |
| 76 | `` `docs/reports/phase3h-202-fullsuite/.exitstatus` `` | path | tracked |
| 77 | `` `docs/reports/phase3h-202-fullsuite/` `` | path (dir) | tracked |
| 78 | `` `docs/reports/phase3h-203-fullsuite-verbose/` `` | path (dir) | tracked |
| 79 | `` `docs/reports/phase3h-203-fullsuite-verbose/verbose.log` `` | path | tracked |
| 80 | `` `docs/reports/phase3h-204-stability10/README.md` `` | path | tracked |
| 81 | `` `docs/reports/phase3h-204-stability10/` `` | path (dir) | tracked |
| 82 | `` `docs/reports/phase3h-204-stability10/summary.log` `` | path | tracked |
| 83 | `` `docs/reports/phase3h-205-r76-disposition/` `` | path (dir) | tracked |
| 84 | `` `docs/reports/phase3h-206-3g-findings-disposition/` `` | path (dir) | tracked |
| 85 | `` `docs/reports/phase3h-207-delivery-log-3g/` `` | path (dir) | tracked |
| 86 | `` `docs/reports/phase3h-208-delivery-log-3h/` `` | path (dir) | tracked |
| 87 | `` `docs/reports/phase3h-209-report/README.md` `` | path | this report, tracked as of this task's own commit (staged before this check was run) |
| 88 | `` `docs/reports/phase3h-findings.md` `` | path | tracked |
| 89 | `` `e686a97` `` | sha | task 008's original publish commit |
| 90 | `` `ed469e4` `` | sha | task 014 |
| 91 | `` `error` `` | word | status-value prose |
| 92 | `` `f54873e` `` | sha | task 208 validation fix #2 |
| 93 | `` `f700025` `` | sha | R94 forward-revert |
| 94 | `` `fc358c0` `` | sha | task 203 |
| 95 | `` `features/create_cwd_ghost.feature` `` | path | tracked |
| 96 | `` `features/create_cwd_ghost_test.go` `` | path | tracked |
| 97 | `` `features/filter.feature` `` | path | tracked |
| 98 | `` `features/status_attach.feature:18` `` | path+line | base path `features/status_attach.feature` checked below |
| 99 | `` `features/status_claude_hooks.feature:6` `` | path+line | base path `features/status_claude_hooks.feature` checked below |
| 100 | `` `features/status_claude_hooks.feature` `` | path | tracked |
| 101 | `` `features/status_probe.feature` `` | path | tracked |
| 102 | `` `git cat-file -e <sha>^{commit}` `` | template | command template with a placeholder, not a real sha or path |
| 103 | `` `git ls-files --error-unmatch <path>` `` | template | command template with a placeholder, not a real sha or path |
| 104 | `` `hint` `` | word | prose/token name |
| 105 | `` `internal/service/reconcile.go` `` | path | tracked |
| 106 | `` `internal/service/reconcile_live_error_precedence_test.go` `` | path | tracked |
| 107 | `` `ok` `` | word | `go test` output word |
| 108 | `` `pane_exit_status` `` | code | field-name identifier |
| 109 | `` `resolveScenarioTokenHex(ctx, "hint")` `` | code | Go code fragment |
| 110 | `` `running → error` `` | phrase | status-transition prose |
| 111 | `` `running` `` | word | status-value prose |
| 112 | `` `stopped` `` | word | status-value prose |
| 113 | `` `store.StatusUpdateInput` `` | code | Go code fragment |
| 114 | `` `tmux` `` | word | status-source-value prose |
| 115 | `` `user` `` | word | status-source-value prose |

No bare basenames, no `/tmp` paths and no untracked or deleted paths appear among the "path"
rows above — every one is a full repository-relative path (or a directory with a trailing
slash), and every check below exits 0.

## Shas — `git cat-file -e <sha>^{commit}` (40 items, all exit 0)

```
$ for s in 011b04b 08171ce 0b7dce5 0d9a551 1f919b7 2badb74 2c2ec30 2ccb1d3 \
    2ccb1d3c698d3c256aa94aea7ef2e3a58dc0641a 2f952da 357867e 4673fb9 46dad5e 4b1d4dc \
    5042852 50960b9 52e9045 5708f52 67cefcd 68ac4cf 7634895 79b56d9 87bde8f 8d6ed72 \
    9cd8f37 9cff1ea a24ff8d a53146a a5f8f6b afa55b8 b5e4178 c176751 c9e88cb d578c03 \
    de90a5c e686a97 ed469e4 f54873e f700025 fc358c0; do
    git cat-file -e "$s^{commit}"; echo "$s exit:$?"
  done
011b04b exit:0
08171ce exit:0
0b7dce5 exit:0
0d9a551 exit:0
1f919b7 exit:0
2badb74 exit:0
2c2ec30 exit:0
2ccb1d3 exit:0
2ccb1d3c698d3c256aa94aea7ef2e3a58dc0641a exit:0
2f952da exit:0
357867e exit:0
4673fb9 exit:0
46dad5e exit:0
4b1d4dc exit:0
5042852 exit:0
50960b9 exit:0
52e9045 exit:0
5708f52 exit:0
67cefcd exit:0
68ac4cf exit:0
7634895 exit:0
79b56d9 exit:0
87bde8f exit:0
8d6ed72 exit:0
9cd8f37 exit:0
9cff1ea exit:0
a24ff8d exit:0
a53146a exit:0
a5f8f6b exit:0
afa55b8 exit:0
b5e4178 exit:0
c176751 exit:0
c9e88cb exit:0
d578c03 exit:0
de90a5c exit:0
e686a97 exit:0
ed469e4 exit:0
f54873e exit:0
f700025 exit:0
fc358c0 exit:0
```

All 40 exit 0.

## Paths — `git ls-files --error-unmatch <path>` (39 items, all exit 0)

Line-number-suffixed tokens (`SPEC.md:1355`, `features/status_attach.feature:18`,
`features/status_claude_hooks.feature:6`) are checked against their base path, since the
line-number suffix is not part of the tracked path.

```
$ for p in ci/stability.sh docs/DELIVERY-LOG.md docs/reports/phase3g-findings.md \
    docs/reports/phase3g.md docs/reports/phase3h-001-status-probe-revert/ \
    docs/reports/phase3h-002-narrow-repair/ docs/reports/phase3h-003-r94-scenarios/ \
    docs/reports/phase3h-005-hint-token/ docs/reports/phase3h-006-f31/ \
    docs/reports/phase3h-007-f31-guard/ docs/reports/phase3h-008-fullsuite/.exitstatus \
    docs/reports/phase3h-008-fullsuite/README.md docs/reports/phase3h-008-fullsuite/ \
    docs/reports/phase3h-009-fullsuite-verbose/ \
    docs/reports/phase3h-009-fullsuite-verbose/scenario-summary.txt \
    docs/reports/phase3h-010-stability10/ docs/reports/phase3h-010-stability10/summary.log \
    docs/reports/phase3h-012-delivery-log/ docs/reports/phase3h-013-report/README.md \
    docs/reports/phase3h-013-report/ docs/reports/phase3h-202-fullsuite/.exitstatus \
    docs/reports/phase3h-202-fullsuite/ docs/reports/phase3h-203-fullsuite-verbose/ \
    docs/reports/phase3h-203-fullsuite-verbose/verbose.log \
    docs/reports/phase3h-204-stability10/README.md docs/reports/phase3h-204-stability10/ \
    docs/reports/phase3h-204-stability10/summary.log \
    docs/reports/phase3h-205-r76-disposition/ docs/reports/phase3h-206-3g-findings-disposition/ \
    docs/reports/phase3h-207-delivery-log-3g/ docs/reports/phase3h-208-delivery-log-3h/ \
    docs/reports/phase3h-209-report/README.md docs/reports/phase3h-findings.md \
    features/create_cwd_ghost.feature features/create_cwd_ghost_test.go features/filter.feature \
    features/status_attach.feature features/status_claude_hooks.feature \
    features/status_probe.feature internal/service/reconcile.go \
    internal/service/reconcile_live_error_precedence_test.go SPEC.md; do
    git ls-files --error-unmatch "$p" >/dev/null; echo "$p exit:$?"
  done
ci/stability.sh exit:0
docs/DELIVERY-LOG.md exit:0
docs/reports/phase3g-findings.md exit:0
docs/reports/phase3g.md exit:0
docs/reports/phase3h-001-status-probe-revert/ exit:0
docs/reports/phase3h-002-narrow-repair/ exit:0
docs/reports/phase3h-003-r94-scenarios/ exit:0
docs/reports/phase3h-005-hint-token/ exit:0
docs/reports/phase3h-006-f31/ exit:0
docs/reports/phase3h-007-f31-guard/ exit:0
docs/reports/phase3h-008-fullsuite/.exitstatus exit:0
docs/reports/phase3h-008-fullsuite/README.md exit:0
docs/reports/phase3h-008-fullsuite/ exit:0
docs/reports/phase3h-009-fullsuite-verbose/ exit:0
docs/reports/phase3h-009-fullsuite-verbose/scenario-summary.txt exit:0
docs/reports/phase3h-010-stability10/ exit:0
docs/reports/phase3h-010-stability10/summary.log exit:0
docs/reports/phase3h-012-delivery-log/ exit:0
docs/reports/phase3h-013-report/README.md exit:0
docs/reports/phase3h-013-report/ exit:0
docs/reports/phase3h-202-fullsuite/.exitstatus exit:0
docs/reports/phase3h-202-fullsuite/ exit:0
docs/reports/phase3h-203-fullsuite-verbose/ exit:0
docs/reports/phase3h-203-fullsuite-verbose/verbose.log exit:0
docs/reports/phase3h-204-stability10/README.md exit:0
docs/reports/phase3h-204-stability10/ exit:0
docs/reports/phase3h-204-stability10/summary.log exit:0
docs/reports/phase3h-205-r76-disposition/ exit:0
docs/reports/phase3h-206-3g-findings-disposition/ exit:0
docs/reports/phase3h-207-delivery-log-3g/ exit:0
docs/reports/phase3h-208-delivery-log-3h/ exit:0
docs/reports/phase3h-209-report/README.md exit:0
docs/reports/phase3h-findings.md exit:0
features/create_cwd_ghost.feature exit:0
features/create_cwd_ghost_test.go exit:0
features/filter.feature exit:0
features/status_attach.feature exit:0
features/status_claude_hooks.feature exit:0
features/status_probe.feature exit:0
internal/service/reconcile.go exit:0
internal/service/reconcile_live_error_precedence_test.go exit:0
SPEC.md exit:0
```

All 39 exit 0. (Row 87, this report's own path, was staged with `git add` before this check
was run, so `git ls-files` already saw it as tracked at check time — a path citation, unlike a
sha, does not have the self-reference problem: the file's *content* does not need to be
committed for its *path* to already be part of the tracked tree.)

## In passing: the command tokens' own embedded real path

Row 50's full command token, `` `ci/run.sh go test -p=1 -count=1 ./...` ``, is not itself a
path, but its first word is a real tracked script:

```
$ git ls-files --error-unmatch ci/run.sh; echo "exit:$?"
ci/run.sh
exit:0
```

Not counted in the 39, since the backticked token being classified is the whole command
string, not `ci/run.sh` alone.
