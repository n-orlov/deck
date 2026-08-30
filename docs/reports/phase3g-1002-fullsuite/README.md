# Phase 3g — task 1002: one whole-suite sweep at the approach's final code sha, every skip named

## Why this report was re-run once

The first pair of runs committed under this directory (`69cffd9`) was green,
but its own polling record disclosed a second poll taken with `tail -8`
instead of the mandated `sleep 300; tail -5`. Rather than argue the
difference, both runs were re-executed from scratch — same sha, same
commands, polled with **nothing but `sleep 300; tail -5`** — and the logs and
numbers below are the re-run's, unedited. Nothing about the launcher, the
code tree or the toolchain changed between the two pairs; the earlier pair's
numbers are superseded only because its logs were overwritten in place by
this re-run.

## Sha this sweep ran at

```
$ git rev-parse HEAD
69cffd9db7bb1090ffed5c2caab15085d29edc73
$ git status --short          # before launch
(clean)
$ git log --format=%H -1 -- '*.go' '*.feature' '*.sh' '*.toml' go.mod go.sum
a5f8f6b5288e7c5cda07704074c1b346cc6a3b42
$ git diff --stat a5f8f6b5288e7c5cda07704074c1b346cc6a3b42..HEAD -- '*.go' '*.feature' '*.sh' '*.toml' go.mod go.sum
(empty)
```

`a5f8f6b` is task 1001's commit — the only `*.go` change this approach may
land (standing rules, "No code changes after task 1001") — and is the last
commit to touch any code/build path. The sweep itself ran with `HEAD` at
`69cffd9`, this directory's first commit, whose code/build tree is *identical*
to `a5f8f6b`'s: the `git diff --stat` above, taken at launch time and again
when this README was written, prints nothing. So both runs below exercised —
and this report's own commit still exercises — exactly the approach's final
code tree.

## Launch — the mandated, unnarrowed launcher

Launched exactly as the standing rules mandate, backgrounded and never
blocked on:

```
$ nohup sh -c 'timeout 2400 ci/run.sh go test -p=1 -count=1 ./... > docs/reports/phase3g-1002-fullsuite/suite.log 2>&1; printf "%s\n" "$?" > docs/reports/phase3g-1002-fullsuite/suite.log.exitstatus' &
```

No `-run`, no `DECK_GODOG_PATHS`, no `DECK_GODOG_TAGS`; `features/godog_test.go`'s
`defaultTags` was not touched before, during or after this run.

Polled — never blocked on — with exactly two `sleep 300; tail -5` calls and
nothing else (both logs were tailed inside each of the two poll calls, so the
same two polls cover the `-v` companion below):

```
$ sleep 300; tail -5 docs/reports/phase3g-1002-fullsuite/suite.log; echo ---; tail -5 docs/reports/phase3g-1002-fullsuite/suite-verbose-tally.log; date -u
      # 02:44:56Z: cmd/ packages done in suite.log, features still running in both
$ sleep 300; tail -5 docs/reports/phase3g-1002-fullsuite/suite.log; echo ---; tail -5 docs/reports/phase3g-1002-fullsuite/suite-verbose-tally.log; echo ---; cat docs/reports/phase3g-1002-fullsuite/suite.log.exitstatus docs/reports/phase3g-1002-fullsuite/suite-verbose-tally.log.exitstatus; date -u
      # 02:49:59Z: both logs complete through internal/unit, both exit statuses 0
```

Launch timestamp `2026-08-30 02:39:49Z`; the log and its exit-status file were
both complete at `02:46:06Z` (file mtime), a wall time of **≈6m17s** — in line
with the 6m20s baseline measured at planning even though the `-v` companion
ran concurrently on the same docker host and shared the `deck-go-cache`
volume. The second poll therefore found the run already finished.

**Captured exit status: `0`**, read from
[`suite.log.exitstatus`](suite.log.exitstatus), written by the same `sh -c`
that ran the command — never inferred from the absence of `FAIL`.

Full unedited log, 17 lines (one per package): [`suite.log`](suite.log).
14 `ok`, 3 `[no test files]`, 17/17 packages accounted for, no `FAIL` anywhere
(`grep -c '^FAIL\|--- FAIL' suite.log` = 0):

```
ok  	github.com/n-orlov/deck/cmd/deck	7.517s
ok  	github.com/n-orlov/deck/cmd/fake-claude	0.792s
ok  	github.com/n-orlov/deck/cmd/fake-pi	0.777s
ok  	github.com/n-orlov/deck/features	319.684s
ok  	github.com/n-orlov/deck/internal/agent	0.006s
ok  	github.com/n-orlov/deck/internal/audit	0.019s
ok  	github.com/n-orlov/deck/internal/config	0.034s
ok  	github.com/n-orlov/deck/internal/hookrecv	5.911s
ok  	github.com/n-orlov/deck/internal/interactive	11.277s
?   	github.com/n-orlov/deck/internal/notify	[no test files]
?   	github.com/n-orlov/deck/internal/search	[no test files]
ok  	github.com/n-orlov/deck/internal/service	4.489s
ok  	github.com/n-orlov/deck/internal/store	3.265s
ok  	github.com/n-orlov/deck/internal/theme	0.006s
ok  	github.com/n-orlov/deck/internal/tmux	19.385s
ok  	github.com/n-orlov/deck/internal/tui	1.226s
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

Launched at `02:39:52Z` in the same shell session as the mandated run above,
at the same `HEAD` `69cffd9` / code sha `a5f8f6b`, and polled by the same two
`sleep 300; tail -5` calls quoted above (each poll tailed both logs).
Complete at `02:46:09Z`. **Captured exit status: `0`**, read from
[`suite-verbose-tally.log.exitstatus`](suite-verbose-tally.log.exitstatus).
Full unedited log (9357 lines): [`suite-verbose-tally.log`](suite-verbose-tally.log).

The Gherkin tally, quoted verbatim from that log (ANSI colour codes stripped
in the quotes below only):

```
$ grep -n 'scenarios (' docs/reports/phase3g-1002-fullsuite/suite-verbose-tally.log
5445:311 scenarios (311 passed)
5765:1 scenarios (1 undefined)
5790:1 scenarios (1 failed)
$ sed -n '5445,5448p' docs/reports/phase3g-1002-fullsuite/suite-verbose-tally.log
311 scenarios (311 passed)
3532 steps (3532 passed)
5m2.028221047s
--- PASS: TestFeatures (302.04s)
```

**311 scenarios (311 passed), 3532 steps (3532 passed).** Lines 5765 and 5790
are not part of that tally — they belong to the deliberate harness self-test
named below, which builds two disposable throwaway godog suites of its own
outside `features/`; godog reports them as undefined/failed by design, and
the outer Go test that wraps them still passes
(`suite-verbose-tally.log:5793`, `--- PASS: TestGodogRejectsUndefinedAndFailedSteps`).

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
   `TestGodogRejectsUndefinedAndFailedSteps`** (`features/godog_test.go:145`,
   the injected error at `features/godog_test.go:160`,
   `errors.New("deliberate step failure")`) — visible in the `-v` companion at
   `suite-verbose-tally.log:5760-5795`: this harness self-test builds two
   throwaway godog suites, one with an unregistered step and one with the
   failing step above, and asserts godog reports them as undefined
   ("1 scenarios (1 undefined)", line 5765) and failed ("1 scenarios (1
   failed)", line 5790). It executes on every invocation of the `features`
   package regardless of verbosity — not visible in the non-verbose
   `suite.log` for the same output-buffering reason as the tally (finding
   F34) — and it is not a product failure: the outer Go test **passes**
   (lines 5793-5795) because it is asserting godog's own rejection behaviour.
5. **The opt-in `TestI1KeystrokeDropReproduction` skip**
   (`features/i1_repro_test.go:37`, `t.Skip("opt-in reproduction driver for
   I-1 (task 004); set DECK_I1_REPRO=1 to run. See
   docs/reports/phase3d-i1-repro.log for a committed run.")`) — gated on
   `DECK_I1_REPRO=1`, not set here; a committed run of its own exists at
   `docs/reports/phase3d-i1-repro.log`. It is the **only** skipped Go test in
   this sweep: `grep -c '^\s*--- SKIP:' suite-verbose-tally.log` = **1**, the
   single hit being `suite-verbose-tally.log:5808`,
   `--- SKIP: TestI1KeystrokeDropReproduction (0.00s)`, with the skip reason
   echoed one line above it.
6. **Every conditional theme-colour `t.Skip` vacuity guard, by name** — 31
   guards across 12 files, listed exhaustively below. Each guards a theme
   whose two named tokens happen to resolve to the same colour, which would
   make the distinctness assertion immediately below it vacuously true; none
   skips a scenario or weakens a requirement. **None of them fired in this
   run** — the whole `-v` log contains exactly one `--- SKIP:` line (item 5),
   so every one of these guarded subtests ran its assertion:
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
   though it is outside this task's named categories. It did not fire either
   (`ci/run.sh`'s sibling has `$HOME` set, and the single `--- SKIP:` line in
   the `-v` log is item 5's).

   The complete inventory the two lists above are drawn from:
   `grep -rn 't.Skip' --include='*_test.go' .` = 33 hits = the 31 theme
   guards (all 31 match `vacuous`:
   `grep -rn 't.Skip' --include='*_test.go' . | grep -c vacuous` = 31) plus
   exactly two non-theme guards — the home-directory guard and item 5's
   opt-in guard in `features/i1_repro_test.go`.

## Toolchain

Same image and cache as every other sweep this phase, `deck-ci:local` /
`deck-go-cache` (`go1.25.13 linux/amd64`, queried previously and not
re-queried here since nothing about the toolchain changed —
`docs/reports/phase3g-508-fullsuite/README.md:215,254`).

## Net

The whole suite is green (exit `0`, 17/17 packages accounted for, no `FAIL`)
at the approach's final code sha `a5f8f6b` (run with `HEAD` at `69cffd9`,
whose code/build diff against `a5f8f6b` is empty, shown above). The `-v`
companion, launched and polled alongside it at the identical sha, supplies the
Gherkin tally the mandated non-verbose launcher cannot print (F34) and is
committed here unedited. Every skip and exclusion in the run is named above by
file and line, and only one skip actually fired.
