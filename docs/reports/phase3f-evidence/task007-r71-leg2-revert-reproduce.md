# Task 007 (R71 leg 2) — would these tests go red if the fix were reverted?

Commit under test: `63d4189` ("tui: unarchive an archived row with U from the filter
results (task 007)"). Both experiments mutate `internal/tui/tui.go` in the live
workspace, run, then restore from a pre-mutation copy in `/tmp` and prove the tree came
back byte-identical (`diff` silent, `git status --short` empty).

## Green baseline (at 63d4189, loadavg 3.24/2.51)

    ci/run.sh go test -count=1 ./internal/tui/ ./internal/store/ ./internal/service/
    ok  github.com/n-orlov/deck/internal/tui      0.709s
    ok  github.com/n-orlov/deck/internal/store    2.411s
    ok  github.com/n-orlov/deck/internal/service  3.653s

    ci/run.sh go test -count=1 ./cmd/...
    ok  github.com/n-orlov/deck/cmd/deck  5.121s   (the PTY help test, window raised to 260 rows)

    ci/run.sh env DECK_GODOG_TAGS='@requirement-33-filter-reaches-archived-row,
      @requirement-33-unarchive-from-filter-results,@requirement-33-filter-by-name,
      @requirement-33-filter-by-cwd-and-workspace' go test -count=1 -v ./features/
    4 scenarios (4 passed) / 47 steps (47 passed)
    — including `@requirement-33-filter-reaches-archived-row`, unmodified by this task.
    Full output: artifacts/task007-r71-leg2-feature-green.log

## Mutation A — the naive `U`: act on the unfiltered base list

`m.sessions[m.selected]` (the DISPLAYED, possibly filtered list) → `m.baseSessions[m.selected]`.
This is the plausible wrong implementation: it compiles, `U` still exists, help parity still
passes, and it still works for any row you can see without a filter — but an archived row is
never in `baseSessions`, so the one row `U` exists for becomes unreachable.

    ci/run.sh go test -count=1 -run TestUnarchive ./internal/tui/     (loadavg 2.65)
    --- FAIL: TestUnarchiveKeyActsOnAnArchivedRowFoundThroughTheFilter (0.00s)
        unarchive_test.go:64: U inside the filter results issued no command at all
    FAIL  github.com/n-orlov/deck/internal/tui  0.003s

    ci/run.sh env DECK_GODOG_TAGS='@requirement-33-unarchive-from-filter-results' \
      go test -count=1 ./features/                                    (loadavg 2.51)
    ... rendered frame shows:
        Cannot unarchive: session is not archived
        Filter "unarchive-target" in force (1 matching) — / to change, Esc to clear
    godog_test.go:38: godog feature suite failed
    FAIL  github.com/n-orlov/deck/features  7.293s

So the scenario is not vacuous: it fails on the real narrowed list with the real keypress,
and the failure names the exact mechanism.

## Mutation B — reload only the default list

`return m, tea.Batch(m.loadSessions, m.loadArchivedSessions)` → `return m, m.loadSessions`.

    ci/run.sh go test -count=1 -run TestUnarchive ./internal/tui/     (loadavg 3.63)
    --- FAIL: TestUnarchiveResultReloadsBothTheDefaultListAndTheArchivedPool (0.00s)
        unarchive_test.go:110: a successful unarchive returned tui.sessionsLoaded,
            want a tea.Batch of both reloads
    FAIL  github.com/n-orlov/deck/internal/tui  0.004s

## Restore

    cp /tmp/tui.go.pristine internal/tui/tui.go
    git status --short   -> (empty)
    diff /tmp/tui.go.pristine internal/tui/tui.go -> (no output)
    ci/run.sh go test -count=1 ./internal/tui/ ./internal/store/   (loadavg 3.58)
    ok  github.com/n-orlov/deck/internal/tui    0.764s
    ok  github.com/n-orlov/deck/internal/store  2.012s
