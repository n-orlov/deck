# Phase 3g — task 604: one whole-suite sweep at the current sha, every exclusion named

## Launch

Launched exactly as the standing rules mandate, backgrounded and never blocked
on, from a clean tree at the sha this task started from:

```
$ git rev-parse HEAD
2f0ef617c8bac0d8913f572c3610e8f51d96adb9
$ git status --short
(clean)
$ nohup sh -c 'timeout 2400 ci/run.sh go test -p=1 -count=1 ./... > /run/ralphd/artifacts/suite-604.log 2>&1; s=$?; printf "%s\n" "$s" > /run/ralphd/artifacts/suite-604.exitstatus' &
```

Polled only with `sleep N; tail -5 /run/ralphd/artifacts/suite-604.log`, never
blocked on. Wall time ~9m40s from launch to the exit-status file appearing;
`features` alone 314.943s, in line with the approach-06 baseline measurement
(330.3s / 9m45s at `b5a228d`).

**Captured exit status: `0`**, read from
[`full-suite.exitstatus`](full-suite.exitstatus), written by the same `sh -c`
that ran the command — never inferred from the absence of `FAIL`.

Full unedited log, 17 lines (one per package): [`full-suite.log`](full-suite.log).
14 `ok`, 3 `[no test files]`, 17/17 packages accounted for, no `FAIL` anywhere
(`grep -c '^FAIL\|--- FAIL' full-suite.log` = 0):

```
ok  	github.com/n-orlov/deck/cmd/deck	7.639s
ok  	github.com/n-orlov/deck/cmd/fake-claude	0.793s
ok  	github.com/n-orlov/deck/cmd/fake-pi	0.776s
ok  	github.com/n-orlov/deck/features	314.943s
ok  	github.com/n-orlov/deck/internal/agent	0.005s
ok  	github.com/n-orlov/deck/internal/audit	0.018s
ok  	github.com/n-orlov/deck/internal/config	0.030s
ok  	github.com/n-orlov/deck/internal/hookrecv	4.104s
ok  	github.com/n-orlov/deck/internal/interactive	11.139s
?   	github.com/n-orlov/deck/internal/notify	[no test files]
?   	github.com/n-orlov/deck/internal/search	[no test files]
ok  	github.com/n-orlov/deck/internal/service	4.166s
ok  	github.com/n-orlov/deck/internal/store	2.336s
ok  	github.com/n-orlov/deck/internal/theme	0.005s
ok  	github.com/n-orlov/deck/internal/tmux	19.446s
ok  	github.com/n-orlov/deck/internal/tui	1.125s
?   	github.com/n-orlov/deck/internal/unit	[no test files]
```

The launch sha is also committed verbatim as [`launch-sha.txt`](launch-sha.txt).

## The exercised tree equals the phase's final code tree

The launch sha, `2f0ef61`, is a docs-only commit on top of the final *code*
commit `b0a4e7d` (approaches 06's reporting-tail commits 601–603 touch only
`docs/`). Two empty diffs, restricted to product/test/build file patterns,
prove it — both re-run after this report's own commit lands, still empty,
since this report is itself docs-only:

```
$ git diff --stat 2f0ef617c8bac0d8913f572c3610e8f51d96adb9..HEAD -- '*.go' '*.feature' '*.sh' '*.toml' go.mod go.sum
(empty)
$ git diff --stat b0a4e7d..HEAD -- '*.go' '*.feature' '*.sh' '*.toml' go.mod go.sum
(empty)
```

So this sweep compiled and ran exactly the code tree that stands at the end of
the phase — no commit landed between the launch and the log being captured,
and none has landed since that touches a compiled or run path.

## `features/godog_test.go`'s `defaultTags` — the one authority for suite coverage, untouched

```
$ grep -n 'defaultTags = ' features/godog_test.go
16:const defaultTags = "~@real-agents && ~@nightly"
```

Never edited or overridden via `DECK_GODOG_TAGS`/`DECK_GODOG_PATHS` for this
run.

## Every exclusion and skip in this run, named

1. **Tag exclusions (both)** — `defaultTags` above excludes `@real-agents`
   scenarios (they would drive a real `claude`/`pi` binary) and `@nightly`
   scenarios. Neither is selected by this sweep.
2. **The three no-test packages** — `internal/notify`, `internal/search` and
   `internal/unit`, the three `?  ... [no test files]` lines above. None has
   ever carried a test file.
3. **The deliberate injected step failure inside
   `TestGodogRejectsUndefinedAndFailedSteps`** (`features/godog_test.go:145`) —
   this harness self-test builds two throwaway godog suites, one with an
   unregistered step and one with a step that returns
   `errors.New("deliberate step failure")`, and asserts godog reports both as
   undefined/failed. It appears, and must appear, in every run; the outer Go
   test **passes** because it is asserting godog's own rejection behaviour. It
   is not visible in this sweep's non-verbose log (Go's `-v`-gated stdout/
   stderr/`t.Log` buffering, see below), but it executes on every invocation of
   the `features` package regardless of verbosity — it is not a product
   failure.
4. **The opt-in `TestI1KeystrokeDropReproduction` skip**
   (`features/i1_repro_test.go:35`) — gated on `DECK_I1_REPRO=1`, not set here;
   a committed run of its own exists at `docs/reports/phase3d-i1-repro.log`.
   The only skipped Go test in the sweep.
5. **The five conditional theme key/hint-distinctness `t.Skip` vacuity
   guards added in the run range (`1cfbd5a..HEAD`)** — each guards a theme
   whose `key` and `hint` (or `key`/`hint`-prose) tokens happen to resolve to
   the same colour, which would make the distinctness assertion immediately
   below it vacuously true rather than a real check; none skips a scenario or
   weakens a requirement, each just declines to assert something meaningless
   for that one theme:
   - `internal/tui/env_editor_theme_test.go:279` (added by task 017,
     `8c3351a`)
   - `internal/tui/bulk_delete_theme_test.go:305` (task 018, `f33d67a`)
   - `internal/tui/archive_delete_confirm_theme_test.go:159` (task 019,
     `103d430`)
   - `internal/tui/archive_delete_confirm_theme_test.go:229` (task 019,
     `103d430`, a second guard in the same file/commit)
   - `internal/tui/profile_pin_restart_theme_test.go:202` (task 020,
     `62c3abe`)

   (`internal/tui/main_view_theme_test.go:156` carries a sixth, textually
   identical, key/hint guard, but it predates the run range — added by
   `c1725ad`, before base `1cfbd5a` — so it is not counted among "the five
   added in the run range"; `git merge-base --is-ancestor 1cfbd5a c1725ad`
   fails, confirming `c1725ad` is not a descendant of the run's base.)

## The Gherkin scenario tally — quoted from task 508's committed `-v` companion log, not re-run here

This sweep's own log cannot carry the tally: with
`features/godog_test.go:35`'s `godog.Options.TestingT` set to the enclosing
`*testing.T`, `go test -p=1 -count=1 ./...` (the launcher this task mandates,
verbatim, non-verbose) discards a **passing** package's stdout, stderr and
`t.Log` output entirely — Go's own `-v`-gated buffering, not anything godog
does. This is finding F34 (the second finding task 602 added, `2058c08`,
`docs/reports/phase3g-findings.md:186`), re-derived from scratch there with a
disposable two-package Go module reproducing the identical non-verbose-vs-`-v`
split on a tree with no godog and no product code at all. The fix is not to
narrow this sweep's command line (that would violate "no `-run` narrowing
anywhere in the evidence") but to cite the tally from the one companion run
that already exists for it, on a code-identical tree:

> `full-suite-verbose.log:5423-5426` of task 508's report
> ([`docs/reports/phase3g-508-fullsuite/README.md`](../phase3g-508-fullsuite/README.md)),
> captured at sha `550a265` (a docs-only descendant of sweep A's `8f8e214`,
> itself a docs-only descendant of the final code commit `b0a4e7d` — so the
> same code tree this sweep exercises):
>
> ```
> 311 scenarios (311 passed)
> 3523 steps (3523 passed)
> 4m50.088026723s
> --- PASS: TestFeatures (290.10s)
> ```

**311 scenarios, 311 passed, 0 failed; 3523 steps, 3523 passed.** Disclosed
here in full: the mandated non-verbose launcher used for *this* sweep cannot
print that tally, for any duration, any tree and any amount of re-running
(F34); the number above is sourced from the sibling `-v` run task 508 already
committed for exactly this reason, not narrowed with `-run` and not re-run in
this task.

## Toolchain

Same image as every other sweep this approach, `deck-ci:local` /
`deck-go-cache`, queried previously as `go1.25.13 linux/amd64` /
`tmux 3.5a` (`docs/reports/phase3g-508-fullsuite/README.md`); not re-queried
here since nothing about the toolchain changed.

## Net

The whole suite is green (exit `0`, 17/17 packages accounted for, no `FAIL`)
at the sha this task started from, and that sha's code tree is proven
identical to the phase's final code tree by two empty `git diff --stat`s. Every
exclusion is named above. Because the run is exit 0, no fresh
`ci/stability.sh 10` is triggered by this task — the final code tree has not
changed.
