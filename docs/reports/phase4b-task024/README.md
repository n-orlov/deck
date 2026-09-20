# Phase 4b — task 024's own freshness checks for `docs/reports/phase4b.md`

Task 024 is the per-requirement record (`docs/reports/phase4b.md`). This directory
holds the measurements that record's own freshness claims rest on, taken in this
same iteration so nothing in it is a count or a status copied forward.

## Sha

- HEAD at measurement time for the guards and targeted-package sections below:
  **`0563dab`** (`0563dab728ebdae4303f6ddabad1f6aef28f5333`),
  i.e. the commit immediately before this task's first docs-only commit
  (`20ac5a6`). The R133 test-name section at the end was measured one commit
  later, at `20ac5a6` — both are code-identical to `f13c848`.
- Final code sha: **`f13c848`**. `git diff --stat f13c848..HEAD -- . ':(exclude)docs/'`
  prints nothing, so the code measured here is byte-identical to `f13c848` — the
  seven commits between them (`0d966e4`, `257133e`, `c6a0876`, `b502044`,
  `93f5790`, `0e1d6e8`, `0563dab`) are all docs-only.
- Task 021's gate sha: **`baf92ed`** (RED, exit 1 — see
  [`../phase4b-final-suite/README.md`](../phase4b-final-suite/README.md)).

## Guards re-measured at the shipped code (not at `baf92ed`)

`docs/reports/phase4b-guards/README.md` (task 022, commit `0e1d6e8`) is pinned to
`baf92ed` **by its own criteria** ("at the same code sha task 021 gated") and was
measured there in a scratch worktree. That leaves the *shipped* code sha
(`f13c848`) unmeasured by that report, so the same three guards were run here, at
HEAD, through the CI container:

| Guard | Invocation | Exit | Log |
| ----- | ---------- | ---- | --- |
| build | `ci/run.sh go build ./...` | `0` | [build.log](build.log) (empty) |
| vet | `ci/run.sh go vet ./...` | `0` | [vet.log](vet.log) (empty) |
| gofmt | `ci/run.sh gofmt -l .` | `0` | [gofmt.log](gofmt.log) |

`gofmt -l` lists exactly the three untracked `.spike-preview/` files
(`cmd/conformance/main.go`, `conformance/conformance.go`,
`conformance/conformance_test.go`) — the pre-existing drift this run's standing
rules explicitly leave alone and do not treat as a finding. No tracked file is
listed.

## Targeted tests re-run at the shipped code

`ci/run.sh go test -count=1 ./internal/tui/ ./internal/store/ ./internal/config/ ./internal/service/`
→ exit `0` ([targeted-tests.log](targeted-tests.log)):

```
ok  	github.com/n-orlov/deck/internal/tui	7.719s
ok  	github.com/n-orlov/deck/internal/store	4.715s
ok  	github.com/n-orlov/deck/internal/config	0.035s
ok  	github.com/n-orlov/deck/internal/service	7.994s
```

These are the four packages every R128–R131 and R133–R135 unit/behaviour test
named in `docs/reports/phase4b.md` lives in. The whole-suite, gate-strength
evidence for this same code is the ten-run stability sweep at `c6a0876`
(10/10, [`../phase4b-stability10/README.md`](../phase4b-stability10/README.md));
this directory does not claim a single-run gate beyond it.

Real exit statuses for all four invocations: [exit-status.txt](exit-status.txt).

## R133's shipped test names, re-read at this commit

Task 024's second validation attempt rejected the record for naming `8d934d1`'s
regression test `TestInteractiveScrollOffsetHealsAfterVisibleOnlyReseedOnTheNextUpdate`.
That name is `4a1e352`'s; `8d934d1` rewrote
`internal/tui/interactive_scroll_render_heal_test.go` and replaced it with two
names. Re-derived here, not carried forward:

- `git show 4a1e352:internal/tui/interactive_scroll_render_heal_test.go | grep '^func Test'`
  → `TestInteractiveScrollOffsetHealsAfterVisibleOnlyReseedOnTheNextUpdate`,
  `TestInteractiveScrollHealAtUpdateTopLeavesNonInteractiveModelsUntouched`.
- `git show 8d934d1:internal/tui/interactive_scroll_render_heal_test.go | grep '^func Test'`
  → `TestInteractiveScrollOffsetHealsFromTheRenderAfterVisibleOnlyReseed`,
  `TestInteractiveScrollOffsetHealsOnTheNextUpdateWithNoRenderInBetween`,
  `TestInteractiveScrollHealAtUpdateTopLeavesNonInteractiveModelsUntouched`.
  The same three are the names in the tree today (`f13c848`'s code).
- All three PASS at HEAD `20ac5a6` (code-identical to `f13c848`; the commits
  between them are docs-only): `ci/run.sh go test -count=1 -v -run '<the three names>'
  ./internal/tui/` → exit `0` ([r133-test-names.log](r133-test-names.log)).
