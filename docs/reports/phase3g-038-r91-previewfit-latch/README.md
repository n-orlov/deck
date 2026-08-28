# R91 red/green: `previewFit`'s no-live-pane return must not latch the fitted session

**Task 038 (report assembly), and this is a disclosed deviation from the PRD's
"captured at implementation time" rule — read it as retroactive, not as
implementation-time evidence.**

R91 is one of the eight product-defect requirements the PRD requires a
revert-and-reproduce red/green pair for. Task 035 fixed the defect (`9d6c22a`)
and proved the *count neutrality* the PRD's SIGWINCH licence demands (see
`../phase3g-034-previewfit-derivation/` for the pre-derivation and
`../phase3g-035-previewfit-no-live-pane-latch/` for the observed-post-fix
counts), but it committed **no test that fails without the fix** — so no
red-before-fix existed for the latch itself. Task 038 produced this pair
retroactively, at HEAD `9ee8672`, from the same workspace.

## What was run

A single evidence harness test, preserved verbatim here as
[`zz_r91_latch_evidence_test.go.txt`](zz_r91_latch_evidence_test.go.txt). It was
placed in `internal/tui/` for the two runs below and **removed again in the same
iteration** — it is deliberately *not* part of the suite, and no product code
and no committed test changed for this capture.

It names no field introduced by the fix, so the same source compiles either side
of it. Route, with no tmux and no pty: `previewFitModel` (the pre-existing R63
helper) wires a `tmux.Client` whose socket does not exist, so *running the fit
closure for real* takes `previewFit`'s no-live-pane/read-error return — the exact
branch R91 is about. The resulting `previewFitDone` is handed to `Model.Update`,
and the assertions are that (1) `previewFitSessionID` stays empty and (2) the
next `previewTick`, row still selected, issues a fresh fit.

- **Red**, [`red-before-fix.log`](red-before-fix.log): `internal/tui/tui.go`
  reverted to its pre-fix content (`git show 9d6c22a^:internal/tui/tui.go >
  internal/tui/tui.go`), nothing else touched.
- **Green**, [`green-after-fix.log`](green-after-fix.log): the committed
  `tui.go` at `9ee8672` (identical to `9d6c22a`'s).

Both via `ci/run.sh go test -count=1 -v -run
TestR91NoLivePaneReturnDoesNotLatchTheFittedSession ./internal/tui/`.

## Red (pre-fix `tui.go`)

```
    zz_r91_latch_evidence_test.go:51: previewFitSessionID = "s1" after a no-live-pane
    return, want empty: nothing was resized, so this session must stay eligible for a
    real fit once its pane becomes live again (R91)
--- FAIL: TestR91NoLivePaneReturnDoesNotLatchTheFittedSession (0.00s)
FAIL	github.com/n-orlov/deck/internal/tui	0.005s
```

The failure is the defect in one line: a return that resized nothing latched the
session anyway, spending its one coalesced fit forever.

## Green (`tui.go` at `9d6c22a`/`9ee8672`)

```
--- PASS: TestR91NoLivePaneReturnDoesNotLatchTheFittedSession (0.01s)
ok  	github.com/n-orlov/deck/internal/tui	0.008s
```

## Workspace left clean

`git checkout internal/tui/tui.go` restored the reverted file (`git status`
showed no modification to any `.go` file afterwards) and the harness test file
was deleted from the tree. The only committed additions from this capture are
the files in this directory.

## Follow-up worth a task (not done here)

Promoting `zz_r91_latch_evidence_test.go.txt` into
`internal/tui/preview_fit_overlap_test.go` as a permanent regression test would
close R91's real residual gap: today nothing in the suite fails if the
`noLivePane` guard is removed again. Task 038 is a report-writing task and did
not add suite tests.

The fix's own diff is kept here as
[`fix-9d6c22a-tui.go.diff`](fix-9d6c22a-tui.go.diff) so the red can be
reproduced without hunting the commit.
