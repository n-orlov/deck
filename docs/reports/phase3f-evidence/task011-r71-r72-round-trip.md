# Task 011 — R71/R72 end-to-end round trip (evidence)

One new scenario, `features/filter.feature:58` `@requirement-33-archive-round-trip`:
create two shell sessions, `A` on the auto-selected target, assert the confirm names the
kill and has written nothing, submit it, assert the row left the default frame (and the
pane is gone / `archived_at` set), find it with `/` + Enter, `U`, `r`, then clear the
filter and assert the row is back in the plain default list, live.

No new step definitions: every step already existed (`presses A on its selected session`,
`submits the open dialog`, `opens the list filter`, `types … into the filter field`,
`keeps the filter in force with enter`, `unarchives its selected session`,
`clears the list filter with escape`, `presses r on session`, the tmux/db assertions).
So every leg goes through the real keypress on the real pty; nothing calls
archive/unarchive/resume directly. No `sleep`, no widened timeout, no `@flaky`; the two
waits used (`screen contains` 5 s poll, `state database contains … with status` 5 s poll)
are the package's pre-existing, consequence-bound helpers.

## 1. The scenario is really selected (not a vacuous tag)

The RED run below reports `1 scenarios (1 failed)` / `30 steps (5 passed, 1 failed,
24 skipped)` for `filter.feature:59` — the tag selects exactly this scenario and its
30 steps.

## 2. Revert-and-reproduce: would it go red if the fix were reverted?

Mutation (plausible pre-fix behaviour, not a delete — build stayed green): the `case "A"`
branch of `internal/tui/tui.go` restored to its pre-R72 form from `git show 10f3970`,
i.e. `A` archives on the keypress with no dialog:

```
			session := m.sessions[m.selected]
			return m, func() tea.Msg {
				return sessionArchived{session: session, err: m.archiveSvc(context.Background(), session)}
			}
```

`/proc/loadavg` at start: `2.16 2.18 2.63` (and `2.47 2.20 2.65` for the first,
tail-only run of the same mutation).

Result — RED at the confirm leg (full log: `task011-mutation-red.log`, raw pty frame
dumps elided):

```
--- Failed steps:

  Scenario: the whole round trip: A plus its confirm hides the row, / finds it, U and r bring it back live to the default list # filter.feature:59
    When deck client "A" presses A on its selected session "trip-target" # filter.feature:80
      Error: after scenario hook failed: timed out waiting for frame "Archive trip-target": context deadline exceeded
...
1 scenarios (1 failed)
30 steps (5 passed, 1 failed, 24 skipped)
FAIL	github.com/n-orlov/deck/features	23.531s
```

The failure frame shows the mutation's own damage: the row is gone and the toast reads
`Killed and archived — press u to unarchive (agent stays stopped)` although no confirm was
ever shown — exactly the field defect R72 fixed.

Restore proof (product code byte-identical to `HEAD`, only the feature file changed):

```
$ cp /tmp/tui.go.orig internal/tui/tui.go && diff /tmp/tui.go.orig internal/tui/tui.go
$ git status --short
 M features/filter.feature
$ git diff --stat
 features/filter.feature | 48 ++++++++++++++++++++++++++++++++++++++++++++++++
 1 file changed, 48 insertions(+)
```

Also measured, and NOT used as the proof: dropping `m.loadSessions` from the
`sessionUnarchived` batch (leaving only `m.loadArchivedSessions`) leaves the scenario
green, because the periodic reconcile reload (250 ms in scenarios) restores the default
list on its own. Recorded so a later reader does not mistake that batch for something this
scenario pins.

## 3. Three consecutive green runs

`ci/run.sh env DECK_GODOG_TAGS='@requirement-33-archive-round-trip' go test -count=1 ./features/`

```
=== run 1 loadavg: 2.61 2.41 2.68 1/4133 12510
ok  	github.com/n-orlov/deck/features	20.363s
=== run 2 loadavg: 2.50 2.40 2.67 15/4127 12557
ok  	github.com/n-orlov/deck/features	17.300s
=== run 3 loadavg: 3.65 2.70 2.77 39/4195 12603
ok  	github.com/n-orlov/deck/features	20.285s
```

(The pre-mutation baseline run of the same command was also green:
`loadavg 1.85 2.05 2.64`, `ok … 17.491s`.)

## 4. Whole feature file still green

`ci/run.sh env DECK_GODOG_TAGS='@requirement-33' go test -count=1 ./features/`
— `loadavg 5.56 3.33 2.98` → `ok  github.com/n-orlov/deck/features	20.189s`
(all five `filter.feature` scenarios; the change is additive, no existing scenario edited).

## Finding (recorded, not fixed)

`filterStatusLine`'s held-filter wording says `… — / to change, Esc to clear`, but with the
text field closed a top-level `Esc` does **not** clear `filterQuery` (`updateFilter`'s
`esc` is the only place that clears it, and it is unreachable while `m.filtering ==
false`); the operator must press `/` first and then `Esc`. Both this scenario and the
pre-existing `@requirement-33-unarchive-from-filter-results` scenario therefore reopen the
filter before clearing it. Candidate for task 024's findings report.
