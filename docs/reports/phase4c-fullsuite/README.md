# Phase 4c: unnarrowed whole-suite gate (task 022)

Task 017 (the original whole-suite sweep) was declared unsatisfiable and
skipped: its criteria named "18 packages... the three that report [no test
files]", which task 012 (commit 505256d) had already invalidated by adding a
fourth no-test-files package, `internal/tui/sidebarcursor`. 017 also
uncovered a genuinely red `cmd/deck` package (a stale help-text assertion
left over from task 014's header-cursor rework, commit 1804a72). Task 022
carries both the real fix and the real sweep.

## Code sha

Fix commit: `eaf2477f0efe715310f94acf9fbcf1550201cf27` — "cmd/deck: fix
stale c-key help assertion (task 022, #32)". This report's whole-suite run
below was executed AT this sha (`git rev-parse HEAD` immediately before
dispatching the run showed the same value; the working tree was clean).

## What was fixed

`cmd/deck/main_test.go`'s `TestDeckBinaryEmptyHelpAndQuitThroughPTY` asserted
the stale substring `"c toggle the selected row's manual group"`. Task 014
(commit 1804a72) rewrote the `c`/`←`/`→` header-cursor keymap line in
`internal/tui/tui.go`'s `helpText()` (line 9283) to:

```
c / ←/→ toggle (c), fold (←) or unfold (→) the group whose header is
    under the cursor -- from a session row, that row's own group
```

The test's pinned substring never matched that rewrite, and go test
`./cmd/deck/...` failed. The test now pins the two current lines instead
(`"toggle (c), fold (←) or unfold (→) the group whose header is"` and
`"under the cursor -- from a session row, that row's own group"`).

Before this fix, `ci/run.sh go test -count=1 ./cmd/deck/...` failed:
```
--- FAIL: TestDeckBinaryEmptyHelpAndQuitThroughPTY (0.69s)
    main_test.go:541: released help missing "c toggle the selected row's
    manual group" through the real PTY: ...
```
(full text: `/run/ralphd/artifacts/fullsuite-017.log`, captured by task 017).

## The whole-suite run

Command (unnarrowed, no `-run` filter, no tag override, no package list):

```
ci/run.sh go test -p=1 -count=1 ./...
```

Run in the CI sibling container (`deck-ci:local`, warm `deck-go-cache`
volume), in the background, with the test process's own exit status
captured to a file rather than inferred from a pipe or from log prose.

- Log: `fullsuite.log` (this directory)
- Exit status (the `go test` process's own, written by the shell that ran
  it): `fullsuite.exit` (this directory) — contents: `0`
- Wall clock: started `1790169640` (unix time), ended `1790170115`, elapsed
  **475 seconds (~7.9 minutes)** — in line with the ~7.5-minute baseline
  measured at `2752c9e` (18 packages); the 19th package added by task 012
  (`internal/tui/sidebarcursor`, no test files) costs nothing measurable.

### Package accounting (19 packages: 15 `ok` + 4 `[no test files]`)

`ok`:
`cmd/deck`, `cmd/fake-claude`, `cmd/fake-codex`, `cmd/fake-pi`, `features`,
`internal/agent`, `internal/audit`, `internal/config`, `internal/hookrecv`,
`internal/interactive`, `internal/service`, `internal/store`,
`internal/theme`, `internal/tmux`, `internal/tui` (15 packages)

`[no test files]`:
`internal/notify`, `internal/search`, `internal/unit`,
`internal/tui/sidebarcursor` (4 packages)

15 + 4 = 19, matching `go list ./...`'s own package count at this sha. No
package reported `FAIL`; `fullsuite.exit` holds `0`.
