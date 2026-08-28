# Task 301 — independent-review findings 2, 3, 4 closure at HEAD

Re-derivable evidence, at HEAD, that independent-review findings 2 (R77), 3
(R79) and 4 (R82) are closed. Every sha below is verified to exist with
`git cat-file -e <sha>`; every log below was produced by re-running the named
test at HEAD through `ci/run.sh` and captured with its exit status in the same
shell invocation.

## Finding 2 — R77 rename-path post-commit filesystem cleanup

A rename onto a reaped-but-tombstoned name left that tombstoned session's
files (holder dir, log, etc.) on disk instead of cleaning them up.

- **Fixing sha:** `7e3261c` — "service: clean up a reaped tombstoned holder's
  files on rename too (task 201)" (`git cat-file -e 7e3261c` → present).
  Follow-up `248d257` — "docs: make task 201's rename-reuse evidence logs
  self-contained (task 201)" — made the task's own red/green logs
  self-describing; no further code change (`git cat-file -e 248d257` →
  present).
- **Committed test that pins it:**
  `internal/service/rename_reuse_test.go:22`,
  `TestRenameOntoATombstonedNameCleansUpThatSessionsFiles`.
- **Re-run log at HEAD:** [`finding2-rename-cleanup.log`](finding2-rename-cleanup.log)
  — `ci/run.sh go test -count=1 ./internal/service/`, exit status 0, `ok`.

## Finding 3 — R79 whole expired-tombstone backlog drained within the open/startup cycle

`SweepTombstones` stamped its hourly throttle after a single bounded batch
even when the backlog was bigger than one batch, so a real backlog left over
from store-open took a further real hour per leftover batch instead of
finishing within the same open/startup cycle.

- **Fixing sha:** `dd90a28` — "store+cmd/deck: drain the whole
  expired-tombstone backlog within the same startup cycle (task 202)"
  (`git cat-file -e dd90a28` → present). Follow-up `a46514e` — "docs: show
  task 202's tombstone-drain defect behaviourally, not just as a build break
  (task 202)" — added a signature-neutral revert-and-reproduce probe
  (`git cat-file -e a46514e` → present). `6a01fe7` — "cmd/deck: pin the real
  startup tombstone drain continuation (task 214)" — added the test that
  drives the real `preFrameTombstoneSweep` + `newTUIReconcile` →
  `DrainExpiredTombstones` call sites end-to-end (`git cat-file -e 6a01fe7` →
  present).
- **Committed test that pins it:** `cmd/deck/tombstone_startup_continuation_test.go`.
- **Re-run log at HEAD:** [`finding3-tombstone-drain.log`](finding3-tombstone-drain.log)
  — `ci/run.sh go test -count=1 ./cmd/deck/ ./internal/store/`, exit status 0,
  both `ok`.

## Finding 4 — R82's unchanged-PTY-assertion condition, resolved as the SPEC.md-vs-PRD contradiction F27

Independent review finding 4 identified that R82's requirement that certain
keyboard-only PTY assertions stay "unchanged" collides with `SPEC.md:1357`
once every field's help line renders `dimmed`; task 203 resolved this as a
genuine `SPEC.md`-vs-PRD contradiction, filed as finding F27, per the PRD's
own precedence rule (`SPEC.md` wins, and the disagreement is a finding).

- **Fixing sha:** `3e883d7` — "features,docs: resolve R82's
  unchanged-PTY-assertion condition as the SPEC.md-vs-PRD contradiction it is
  (task 203)" (`git cat-file -e 3e883d7` → present).
- **Committed evidence file that pins it:** `docs/reports/phase3g-findings.md`,
  the F27 row, line 178 (as of this HEAD; see `grep -n F27
  docs/reports/phase3g-findings.md`), plus the two revert-and-reproduce logs
  it cites: `docs/reports/phase3g-203-r82-assertion-conflict/original-assertion-red.log`
  and `docs/reports/phase3g-203-r82-assertion-conflict/positive-control-red.log`.
- **Re-run log at HEAD:** [`finding4-f27-assertion.log`](finding4-f27-assertion.log)
  — `ci/run.sh env DECK_GODOG_PATHS=create_cwd_ghost.feature,create_session.feature go test ./features/ -run TestFeatures -count=1`,
  exit status 0, `ok`.

## Scope note

This directory is verification-only: no product code changed. No full-suite
run was performed for this task (none is required or permitted by task 301's
own criteria); the three targeted commands above are each a single package
(or feature-file subset) run, not the whole suite.
