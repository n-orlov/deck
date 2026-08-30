# Phase 3h — suite reconciliation after the §7 ruling

## Goal

Phase 3g ended with every requirement (R76–R93) substantively landed but the tree in a
deliberately disclosed split: `SPEC.md` §7 contradicted itself about an `error` row under a
live pane (findings F36/F40 in `docs/reports/phase3g-findings.md`), and the run — which may
not edit `SPEC.md` — implemented each reading in alternating approaches. The operator has now
ruled, in commit `de90a5c` (`spec: resolve §7's error-under-live-pane self-contradiction and
land §11.8's R93 wording (operator)`): **the transition table is the design.** The repair
covers a `stopped` row whatever its source, and an `error` row that carries a
`pane_exit_status` or a `tmux`/`user` source; a hook- or probe-sourced `error` with no
pane-exit verdict is `running → error`'s own live state and is never repaired.

This phase brings the tree to that ruling and closes the suite: three forward-reverts, three
one-token scenario assertions, one licensed product fix (F31), a documentation refresh, and
the stability gate held at one final sha. It is small on purpose. Do not grow it.

## Read this before anything else

- **`SPEC.md` is already correct.** §7 as amended at `de90a5c` is the authority for every
  status question in this phase. There is nothing left to interpret: where this PRD and
  `SPEC.md` disagree, `SPEC.md` wins and the disagreement is a finding for
  `docs/reports/phase3h-findings.md`, never an edit.
- **The protected-path audit for this run is the range `de90a5c..HEAD`.** Protected paths:
  `SPEC.md`, `prds/`, `ci/Dockerfile`, `ci/SPIKE.md` — read-only for this job, no exception,
  and steering cannot license an exception. The full-history audit context, so no report
  re-litigates it: the only shas that have ever legitimately touched protected paths are the
  operator commits `2eed8de`, `a03527c`, `de90a5c`, plus the disclosed pair
  `b69b5ba`/`2d61993` (phase 3g finding F41 — net diff empty, history stands, already ruled
  on). None of that is this run's problem; this run's own range must show **zero** protected-
  path commits.
- **The reverts in R94 are verified to apply cleanly** (tested by the operator on
  `de90a5c` in a scratch worktree: `git revert --no-commit a53146a c176751 0b7dce5`, no
  conflicts, 29 files). If they conflict for you, something else changed — stop and report
  rather than resolving by hand.

## Requirements

### R94 — the reconcile repair matches §7 as ruled

Forward-revert, in this order, citing `de90a5c`'s ruling in each commit message:

1. `a53146a` — un-re-points `features/status_probe.feature`'s stale-sampling scenario.
2. `c176751` — un-re-points `features/status_claude_hooks.feature`'s `StopFailure` block.
3. `0b7dce5` — restores the narrow repair in `internal/service/reconcile.go`: this brings
   back `internal/service/reconcile_live_error_precedence_test.go` (the tests pinning that a
   hook-/probe-sourced live-pane `error` is never repaired) and removes
   `reconcile_bare_error_repair_test.go` (which pinned the opposite).

Done means: `features/status_attach.feature:18` ("attach acknowledges a live error without
replacing its verdict"), `features/status_claude_hooks.feature:6` and
`status_probe.feature`'s stale-sampling scenario all pass **in their pre-revert-range form,
unedited beyond the reverts themselves** — exactly the shape that measured 10/10 at 3g's
`a5f8f6b`. The reverts change `.feature` files and delete a test; that is licensed here, for
these three commits only, because it is the ruling landing — it is not the forbidden
"weaken a scenario to make something agree".

### R95 — the cwd-ghost scenarios track the shipped `hint` token

Phase 3g's `abe160c` (task 1203) moved the create modal's cwd ghost from `theme.Dimmed` to
`theme.Hint` to clear R84's 3.0:1 contrast floor (ratio table in
`docs/reports/phase3g-1203-selection-token/`), and only ran the theme package afterwards.
Three scenarios in `features/create_cwd_ghost.feature` still assert the old token and fail
deterministically (`cell 0 has foreground #94a3b8, want #64748b`):

- `create_cwd_ghost.feature:14` — "a unique directory match ghosts in the dimmed token…"
- `create_cwd_ghost.feature:62` — "a hidden directory is a candidate only when…"
- `create_cwd_ghost.feature:80` — "a leading tilde expands for scanning…"

Change the asserted token from `dimmed` to `hint` in those three `Then` steps (and retitle
the `:14` scenario so it no longer names the dimmed token). Nothing else in the file moves;
the product is already correct. This one-token re-point is licensed by this requirement —
it tracks an intentional, already-reviewed product change.

### R96 — F31, the reconcile lost update, is fixed for real

Finding F31 (`docs/reports/phase3g-findings.md`, root-caused and measured at 36/80 under a
forced interleaving): the shell-liveness promotion at `internal/service/reconcile.go:103-113`
carries neither `ExpectedStatus` nor `AllowedCurrentStatuses`, so a status write landing
between a reconcile pass's snapshot read and its write is silently overwritten — the write is
accepted through `tmuxTerminalRepair`'s branch (`internal/store/store.go:673-674`) even
though the promotion was computed against a row that no longer exists. Candidate fix, named
by 3g's own diagnosis: guard the promotion write with
`AllowedCurrentStatuses: []string{"starting"}` so a stale promotion loses to whatever won
the row in between.

- Evidence bar: red-before/green-after at implementation time under the forced interleaving
  (3g's diagnosis shows how to force it), captured into `docs/reports/phase3h-<task>-f31/`.
- The fix must not disturb §7's precedence rules (`user-terminal` > `hook` > `probe` >
  `tmux`) or the three R94 scenarios; `features/attention_sort.feature` passes unedited.
- If the candidate fix turns out wrong in kind (not merely in detail), stop and record a
  finding rather than inventing a different product change.

### R97 — the record matches the tree

At the final code sha, in `docs/`:

- `docs/reports/phase3g.md`'s R76 row and `phase3g-findings.md`'s F36/F38/F40/F41 rows get a
  one-line disposition update each, pointing at `de90a5c` and at this phase's reverts —
  append, do not rewrite their history.
- `docs/DELIVERY-LOG.md`'s Phase 3g paragraph is brought to the final state of record
  (finding F35 documents exactly how stale it is and what the current facts are).
- A short `docs/reports/phase3h.md` (requirement table in the 3g style) and
  `docs/reports/phase3h-findings.md` (anything found on the way, plus the disposition of the
  gate) close the phase.

## Ordering

R94 → R95 → R96, then one whole-suite sweep, then the stability gate, then R97 against the
final sha. The final *code* sha is whatever commit last touches `*.go`/`*.feature`; every
R97 document cites it, and a later code fix invalidates both gates and forces both to re-run
at the new sha.

## Green when

- `ci/run.sh go test -p=1 -count=1 ./...` exits **0** at the final code sha (background it
  and poll; per 3g finding F34 the non-verbose launcher cannot print the Gherkin tally —
  take the tally from a companion `-v` run on a docs-only descendant, exactly as task 508
  did).
- `ci/stability.sh 10` is **10/10 from a clean state at the final code sha**, published
  verbatim from `summary.log` with the script's own captured exit status. A 9/10 is
  published as 9/10 with every failure named and its per-run log path — never rounded up,
  never re-run merely to improve the number.
- The R97 documents exist and cite only shas that resolve and paths that are tracked.

## Non-goals

- F2 (golden-frame settle), F20 (status_recovery dup-pane), F22 (`ByteArrivalPattern`), F37
  (the sort_order latent race — disclosed, reproducible only under a forced interleaving),
  and the `filter.feature` dd/undo race: out of scope, never claimed fixed; a recurrence in
  the gate is reported with its log path.
- No theme palette / `internal/theme/builtin/*.toml` change, no contrast-floor allowlist,
  no test-only branch or env knob in product code (R8).
- No re-litigation of anything R76–R93 already landed; no Phase 4+ work, no Codex adapter,
  no OSC 52 work, no F7 quantisation work.
- Do not touch `features/godog_test.go`'s `defaultTags`, and never narrow a deliverable
  sweep with `-run` or `DECK_GODOG_PATHS`.

## For the planner

Phase 3g lost four approaches to the harness's own stall rule, not to review. Plan
accordingly: **one unit of work per task** — one revert can be one task, one feature file is
one task, the stability measurement is its own task and owns its whole iteration — and no
task whose title contains "every". A task that cannot flip its status within about one
iteration is two tasks.

Known tooling gotchas, inherited from 3g's notes verbatim: no Go or tmux in the container —
everything runs through `ci/run.sh`; `edit`/`write` corrupt `\r` in `.feature` files
(`od -c` first); each bash call is a fresh shell, capture `$?` in the same call; at most one
whole-suite run per iteration; commit subjects `<area>: <why> (task NNN)` with this phase's
own task ids; push every completed task's commit; never force-push or rewrite history.
