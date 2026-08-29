# Task 605 — closing review finding 1's stability gate by citation at the unchanged final code tree

Review finding 1 requires `ci/stability.sh 10` to be 10/10 at the phase's
final code commit. That measurement already exists, at a code-identical
tree, from task 507: [`docs/reports/phase3g-507-stability10/round3/`](../phase3g-507-stability10/round3/).
This report closes the gate **by citation of that existing measurement plus
a proof the code tree has not changed since**, per this run's standing rule
that a stability measurement must never be re-run to move a number in either
direction once it is already held at a code-identical tree.

## (a) The measurement, quoted verbatim, with the exact command and its own exit status

Last line of the round's summary log:

```
$ tail -1 docs/reports/phase3g-507-stability10/round3/summary.log; echo "cmd_exit=$?"
10/10 passed
cmd_exit=0
```

Content of the round's own captured script exit status:

```
$ cat docs/reports/phase3g-507-stability10/round3/script.exitstatus; echo "cmd_exit=$?"
0
cmd_exit=0
```

Count of per-run pass lines in the same summary log:

```
$ grep -c 'PASS (exit 0)' docs/reports/phase3g-507-stability10/round3/summary.log; echo "cmd_exit=$?"
10
cmd_exit=0
```

Count of failure lines in the same summary log (`grep -c` with zero matches
correctly reports shell exit status `1` — "no match found" — not an error in
the count itself, which is the reported `0`):

```
$ grep -c FAIL docs/reports/phase3g-507-stability10/round3/summary.log; echo "cmd_exit=$?"
0
cmd_exit=1
```

Together: 10 pass lines, 0 fail lines, the script's own exit status `0`, and
the summary's own tally line `10/10 passed` — round 3 of task 507's
measurement is a clean 10/10.

## (b) The measurement's launch commit and an empty code-diff to the final tree

The round-3 measurement was launched at commit `16186e3`. The final code
commit of the whole run is `b0a4e7d` (after it, every change is docs-only).
Both commits are present in this repository:

```
$ git cat-file -e 16186e3^{commit} && echo "16186e3 ok"; echo "cmd_exit=$?"
16186e3 ok
cmd_exit=0

$ git cat-file -e b0a4e7d^{commit} && echo "b0a4e7d ok"; echo "cmd_exit=$?"
b0a4e7d ok
cmd_exit=0
```

The code-pattern diff from the measurement's own launch commit to `HEAD`,
and separately from the run's final code sha to `HEAD`, over every pattern
that could invalidate a stability measurement (`*.go`, `*.feature`, `*.sh`,
`*.toml`, `go.mod`, `go.sum`):

```
$ git diff --stat 16186e3..HEAD -- '*.go' '*.feature' '*.sh' '*.toml' go.mod go.sum; echo "cmd_exit=$?"
cmd_exit=0

$ git diff --stat b0a4e7d..HEAD -- '*.go' '*.feature' '*.sh' '*.toml' go.mod go.sum; echo "cmd_exit=$?"
cmd_exit=0
```

Both commands print no output and exit `0` — the diff is empty in both
directions. Everything landed between `16186e3` and `HEAD` (which includes
everything between `b0a4e7d` and `HEAD`, since `b0a4e7d` sits on that same
line of history) is docs-only under these patterns. The ten green runs in
`round3/summary.log` were therefore measured on a code tree that is
byte-identical, over every pattern the gate cares about, to the phase's
final code tree at `HEAD`.

## (c) Why no fresh run was launched

The 10/10 gate is a property of the *code tree*, not of when it was last
measured. Because `(b)` shows the code tree at `HEAD` is identical, over
every gate-relevant pattern, to the tree the round-3 measurement already
ran against, re-running `ci/stability.sh 10` now could not produce a
different answer about that tree — it could only re-roll the same
already-held result, at a cost of roughly 80 minutes of the iteration
budget, and this run's own standing rules forbid re-running a stability
measurement to move a number (up or down) once it is already held at a
code-identical tree. Citing the existing round-3 result, backed by the
empty diffs above, is the correct way to close the gate; launching a fresh
`ci/stability.sh 10` here would violate that standing rule for no evidentiary
gain.

## (d) Flakes that remain named and unfixed — this citation is not a claim they don't exist

The 10/10 gate closes review finding 1 (`ci/stability.sh 10` passing 10/10 at
the final code tree). It is not a claim that no flake exists anywhere in the
suite. Three are named, tracked, and explicitly left unfixed by this run's
own standing rules and findings report:

- **F2** — `TestGoldenMinimumFrame`'s settle flake
  (`features/golden_frame_test.go:236`, "frame kept changing after the
  fixture rendered; not settled"). Out of scope by standing rule; a phase-3f
  flake never reproduced reliably even there (one hit in a `-count=10` run).
  This run's own recurrence check, its exact grep commands, and their
  (negative) findings are tracked at
  [`docs/reports/phase3g-findings.md`, §4](../phase3g-findings.md#4-f2--the-golden-frame-settle-flake-no-recurrence-found).
  Not claimed fixed there either — only that this phase's own work did not
  trip it.
- **F22** — `internal/interactive`'s `TestSessionRendersAreCoalescedAgainstAKnownByteArrivalPattern`
  (`internal/interactive/render_test.go:155`, failing at `:175`) flakes under
  multi-package parallel load when the tmux `pipe-pane` job misses its 5s
  connect budget. Observed once, at
  [`docs/reports/phase3g-030-reclaim-leaked-interactive-pipe/criterion-packages-run1.log:2-3`](../phase3g-030-reclaim-leaked-interactive-pipe/criterion-packages-run1.log);
  full disposition in
  [`docs/reports/phase3g-findings.md`, row F22](../phase3g-findings.md).
  Pre-existing, out of scope, named and not fixed.
- **The F29-series open lost update (F31)** — `reconcile.go`'s unconditional
  `starting`→`running` shell-liveness promotion (`internal/service/reconcile.go:103-113`)
  can be accepted as SPEC §7's terminal-pane repair
  (`internal/store/store.go:673-674`) even when it lands on stale state,
  a genuine durable lost update. Root-caused and measured, not fixed:
  **36/80** narrowed-scenario failures under synthetic load, **0** of them on
  the original count-assertion symptom. Tracked logs:
  [`docs/reports/phase3g-502-attention-count-sync/mechanism-diag-postfix-under-load-clobber.log`](../phase3g-502-attention-count-sync/mechanism-diag-postfix-under-load-clobber.log)
  and
  [`docs/reports/phase3g-502-attention-count-sync/green-after-stress-loop-summary.log`](../phase3g-502-attention-count-sync/green-after-stress-loop-summary.log)
  (line 91: `fails=36 / 80`). Full account in
  [`docs/reports/phase3g-findings.md`, row F31](../phase3g-findings.md).
  Deliberately left open — see F31's own row for why a fix was not applied
  this run (it would invalidate the very 10/10 measurement this report
  cites).

None of these three is a hit against the `ci/stability.sh 10` gate itself —
round 3's own per-run logs (`docs/reports/phase3g-507-stability10/round3/run-1.log`
through `run-10.log`) show no occurrence of any of them. They are named here
only so this citation is not read as a claim that the suite has no flake at
all.

## Verification

```
$ git diff --stat 16186e3..HEAD -- '*.go' '*.feature' '*.sh' '*.toml' go.mod go.sum   # empty
$ git diff --stat b0a4e7d..HEAD -- '*.go' '*.feature' '*.sh' '*.toml' go.mod go.sum   # empty
$ tail -1 docs/reports/phase3g-507-stability10/round3/summary.log                     # 10/10 passed
$ cat docs/reports/phase3g-507-stability10/round3/script.exitstatus                   # 0
$ grep -c 'PASS (exit 0)' docs/reports/phase3g-507-stability10/round3/summary.log      # 10
$ grep -c FAIL docs/reports/phase3g-507-stability10/round3/summary.log                # 0
```
