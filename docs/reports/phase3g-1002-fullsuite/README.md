# Phase 3g — task 1002: one whole-suite sweep at the approach's final code sha, every skip named

## Sha this sweep ran at

```
$ git rev-parse HEAD
a5f8f6b5288e7c5cda07704074c1b346cc6a3b42
$ git status --short
(clean)
```

`a5f8f6b` is task 1001's commit — the only `*.go` change this approach may
land (standing rules, "No code changes after task 1001") — and is also the
last commit to touch any code/build path:

```
$ git log --format=%H -1 -- '*.go' '*.feature' '*.sh' '*.toml' go.mod go.sum
a5f8f6b5288e7c5cda07704074c1b346cc6a3b42
$ git diff --stat a5f8f6b5288e7c5cda07704074c1b346cc6a3b42..HEAD -- '*.go' '*.feature' '*.sh' '*.toml' go.mod go.sum
(empty)
```

So both sweeps below exercised, and this report's own commit still exercises,
exactly the approach's final code tree.

## Launch — the mandated, unnarrowed launcher

Launched exactly as the standing rules mandate, backgrounded and never
blocked on:

```
$ nohup sh -c 'timeout 2400 ci/run.sh go test -p=1 -count=1 ./... > docs/reports/phase3g-1002-fullsuite/suite.log 2>&1; printf "%s\n" "$?" > docs/reports/phase3g-1002-fullsuite/suite.log.exitstatus' &
```

No `-run`, no `DECK_GODOG_PATHS`, no `DECK_GODOG_TAGS`; `features/godog_test.go`'s
`defaultTags` was not touched before, during or after this run.

Polled, never blocked on, with `sleep N; tail -5` only:

```
$ sleep 300; tail -5 docs/reports/phase3g-1002-fullsuite/suite.log     # cmd/ packages done, features still running
$ sleep 300; tail -8 docs/reports/phase3g-1002-fullsuite/suite.log     # log complete through internal/unit; then read suite.log.exitstatus
```

Launch timestamp `2026-08-30 02:26:12Z`; the log was complete and the exit
status written by `02:36:26Z`, giving a wall time of **≈9m50s** for this
particular run (the two sweeps in this report shared the docker host and the
`deck-go-cache` build cache concurrently with the `-v` companion below, which
is why this run is slower than the isolated 6m20s baseline measured at
planning — nothing in the command line or the code changed).

**Captured exit status: `0`**, read from
[`suite.log.exitstatus`](suite.log.exitstatus), written by the same `sh -c`
that ran the command — never inferred from the absence of `FAIL`.

Full unedited log, 17 lines (one per package): [`suite.log`](suite.log).
14 `ok`, 3 `[no test files]`, 17/17 packages accounted for, no `FAIL` anywhere
(`grep -c '^FAIL\|--- FAIL' suite.log` = 0):

```
ok  	github.com/n-orlov/deck/cmd/deck	7.567s
ok  	github.com/n-orlov/deck/cmd/fake-claude	0.794s
ok  	github.com/n-orlov/deck/cmd/fake-pi	0.775s
ok  	github.com/n-orlov/deck/features	320.137s
ok  	github.com/n-orlov/deck/internal/agent	0.005s
ok  	github.com/n-orlov/deck/internal/audit	0.018s
ok  	github.com/n-orlov/deck/internal/config	0.033s
ok  	github.com/n-orlov/deck/internal/hookrecv	4.817s
ok  	github.com/n-orlov/deck/internal/interactive	11.157s
?   	github.com/n-orlov/deck/internal/notify	[no test files]
?   	github.com/n-orlov/deck/internal/search	[no test files]
ok  	github.com/n-orlov/deck/internal/service	4.935s
ok  	github.com/n-orlov/deck/internal/store	2.342s
ok  	github.com/n-orlov/deck/internal/theme	0.005s
ok  	github.com/n-orlov/deck/internal/tmux	19.314s
ok  	github.com/n-orlov/deck/internal/tui	1.218s
?   	github.com/n-orlov/deck/internal/unit	[no test files]
```

## The `-v` companion, for the Gherkin tally only

The mandated launcher above never narrows to a subset and cannot be asked to
print godog's own summary line either: `go test`'s package-list mode
discards a **passing** package's stdout/stderr/`t.Log` output unless `-v` is
given (finding F34, first derived in task 508 and re-cited since — the
buffering is a property of `go test` itself, not of this suite or of
godog). Printing the tally therefore requires a second invocation whose
*only* difference from the mandated one is `-v`; that is a companion, not a
narrowing, because it still runs `./...` with no `-run`, no
`DECK_GODOG_PATHS` and no `DECK_GODOG_TAGS`:

```
$ nohup sh -c 'timeout 2400 ci/run.sh go test -p=1 -count=1 -v ./... > docs/reports/phase3g-1002-fullsuite/suite-verbose-tally.log 2>&1; printf "%s\n" "$?" > docs/reports/phase3g-1002-fullsuite/suite-verbose-tally.log.exitstatus' &
```

Launched in the same shell session as the mandated run above, at the same
sha `a5f8f6b`, polled with the same two `sleep N; tail -5` commands quoted
above (both logs were tailed in the same poll calls). **Captured exit
status: `0`**, read from
[`suite-verbose-tally.log.exitstatus`](suite-verbose-tally.log.exitstatus).
Full unedited log (9357 lines): [`suite-verbose-tally.log`](suite-verbose-tally.log).

The Gherkin tally, quoted verbatim from that log:

```
$ grep -n 'scenarios (' docs/reports/phase3g-1002-fullsuite/suite-verbose-tally.log
5445:311 scenarios (311 passed)
5765:1 scenarios (1 undefined)
5790:1 scenarios (1 failed)
```

Line 5445 is `TestFeatures`'s own summary — **311 scenarios, 311 passed, 0
failed**; immediately below it (not shown in the grep above, read from the
same log):

```
3532 steps (3532 passed)
5m0.010698643s
--- PASS: TestFeatures (300.02s)
```

**311 scenarios (311 passed), 3532 steps (3532 passed).** Lines 5765 and 5790
are not part of that tally — they belong to the deliberate harness self-test
named below, which builds two disposable throwaway godog suites of its own
outside `features/`; godog reports them as undefined/failed by design, and
the outer Go test that wraps them still passes.

## Every skip and exclusion in this run, named

1. **The 14 `ok` packages** — `cmd/deck`, `cmd/fake-claude`, `cmd/fake-pi`,
   `features`, `internal/agent`, `internal/audit`, `internal/config`,
   `internal/hookrecv`, `internal/interactive`, `internal/service`,
   `internal/store`, `internal/theme`, `internal/tmux`, `internal/tui` — all
   passed, none skipped at the package level.
2. **The three `[no test files]` packages** — `internal/notify`,
   `internal/search`, `internal/unit` (lines 10, 11, 17 of
   [`suite.log`](suite.log)). None has ever carried a test file.
3. **Both `defaultTags` exclusions** —
   `features/godog_test.go:16`, `const defaultTags = "~@real-agents && ~@nightly"`,
   untouched by this sweep and by every commit in `1cfbd5a..HEAD` that touches
   that file (`git log --oneline 1cfbd5a..HEAD -- features/godog_test.go` shows
   only `6718823` and `904419c`, neither of which edits the `defaultTags`
   line). `~@real-agents` excludes scenarios that would drive a real
   `claude`/`pi` binary; `~@nightly` excludes the nightly-only scenarios.
   Neither is selected by this sweep.
4. **The deliberate injected step failure inside
   `TestGodogRejectsUndefinedAndFailedSteps`** (`features/godog_test.go:145`)
   — visible in the `-v` companion at
   `suite-verbose-tally.log:5760-5790`: this harness self-test builds two
   throwaway godog suites, one with an unregistered step and one with a step
   that returns `errors.New("deliberate step failure")`, and asserts godog
   reports both as undefined ("1 scenarios (1 undefined)") and failed ("1
   scenarios (1 failed)"). It executes on every invocation of the `features`
   package regardless of verbosity — not visible in the non-verbose
   `suite.log` for the same output-buffering reason as the tally (finding
   F34) — and it is not a product failure: the outer Go test **passes**
   because it is asserting godog's own rejection behaviour.
5. **The opt-in `TestI1KeystrokeDropReproduction` skip**
   (`features/i1_repro_test.go:35`, `t.Skip("opt-in reproduction driver for
   I-1 (task 004); set DECK_I1_REPRO=1 to run. See
   docs/reports/phase3d-i1-repro.log for a committed run.")`) — gated on
   `DECK_I1_REPRO=1`, not set here; a committed run of its own exists at
   `docs/reports/phase3d-i1-repro.log`. The only skipped Go test in this
   sweep, confirmed present in the `-v` companion:
   `grep -n 'SKIP: TestI1KeystrokeDropReproduction' suite-verbose-tally.log`
   locates it among the `internal/interactive`/root-level test output.
6. **Every conditional theme-colour `t.Skip` vacuity guard, by name** — each
   guards a theme whose two named tokens happen to resolve to the same
   colour, which would make the distinctness assertion immediately below it
   vacuously true; none skips a scenario or weakens a requirement:
   - `internal/tui/theme_color_test.go:107` (running/error)
   - `internal/tui/detail_theme_test.go:46` (hint/text)
   - `internal/tui/sidebar_hierarchy_test.go:36` (title/text)
   - `internal/tui/sidebar_hierarchy_test.go:63` (dimmed/title)
   - `internal/tui/sidebar_hierarchy_test.go:89` (dimmed/text)
   - `internal/tui/sidebar_hierarchy_test.go:113` (dimmed/badge)
   - `internal/tui/sidebar_stripe_test.go:97` (selection/surface)
   - `internal/tui/help_style_test.go:63` (title/text)
   - `internal/tui/help_style_test.go:91` (key/text)
   - `internal/tui/help_style_test.go:151` (dimmed/text)
   - `internal/tui/main_view_theme_test.go:38` (title/text)
   - `internal/tui/main_view_theme_test.go:69` (border_focus/border)
   - `internal/tui/main_view_theme_test.go:156` (key/hint)
   - `internal/tui/main_view_theme_test.go:194` (border_focus/border)
   - `internal/tui/main_view_theme_test.go:233` (selection/selection_idle)
   - `internal/tui/seam_border_test.go:101` (border_focus/border)
   - `internal/tui/seam_border_test.go:122` (border_focus/border)
   - `internal/tui/seam_border_test.go:152` (border_focus/border)
   - `internal/tui/seam_border_test.go:176` (border_focus/border)
   - `internal/tui/seam_border_test.go:197` (border_focus/border)
   - `internal/tui/seam_border_test.go:233` (border_focus/border)
   - `internal/tui/seam_border_test.go:263` (border_focus/border)
   - `internal/tui/seam_border_test.go:302` (border_focus/border)
   - `internal/tui/settings_color_cues_test.go:137` (hint/text)
   - `internal/tui/settings_color_cues_test.go:198` (border_focus/border)
   - `internal/tui/settings_color_cues_test.go:248` (selection/selection_idle)
   - `internal/tui/env_editor_theme_test.go:279` (key/hint)
   - `internal/tui/bulk_delete_theme_test.go:305` (key/hint, prose-vs-key wording)
   - `internal/tui/archive_delete_confirm_theme_test.go:159` (key/hint, prose-vs-key wording)
   - `internal/tui/archive_delete_confirm_theme_test.go:229` (key/hint, prose-vs-key wording, second guard in the same file)
   - `internal/tui/profile_pin_restart_theme_test.go:202` (key/hint, prose-vs-key wording)

   One further, unrelated `t.Skip` exists in the tree —
   `internal/tui/settings_field_display_and_edit_test.go:70`,
   `t.Skip("no home directory in this environment")` — a home-directory
   guard, not a theme-colour vacuity guard; named here for completeness even
   though it is outside this task's named categories, and it did not fire in
   this sandbox (`ci/run.sh`'s sibling has `$HOME` set) — `suite.log` shows
   `internal/tui` as a plain `ok` with no skip count reported (non-verbose
   `go test` does not print per-test skip counts at all; the `-v` companion's
   `internal/tui` section shows no `--- SKIP:` line for this test, confirming
   it ran rather than skipped).

## Toolchain

Same image and cache as every other sweep this phase, `deck-ci:local` /
`deck-go-cache` (`go1.25.13 linux/amd64`, queried previously and not
re-queried here since nothing about the toolchain changed —
`docs/reports/phase3g-508-fullsuite/README.md:215,254`).

## Net

The whole suite is green (exit `0`, 17/17 packages accounted for, no `FAIL`)
at `a5f8f6b`, the approach's final code sha (empty `git diff --stat` against
itself for every code/build file pattern, shown above). The `-v` companion,
launched and polled alongside it at the identical sha, supplies the Gherkin
tally the mandated non-verbose launcher cannot print (F34) and is committed
here unedited. Every skip and exclusion in the run is named above by file and
line.
