# Phase 3g task 812 — `ci/stability.sh 10` at the approach's final code commit

## Rate, quoted verbatim from `summary.log`

```
0/10 passed
```

(`tail -1 summary.log` in this directory.) Script exit status, captured with
`printf "%s\n" "$?"` in the same shell call that ran it (never through a
pipe): **`1`** — see `stability08.exitstatus` (2 bytes: the digit and its
newline).

**This does not meet task 812's completion bar.** The bar is `10/10 passed`
with the script's own exit status `0`. Neither holds. Per this task's own
criteria, the measured rate is published verbatim below with every failing
run named by log path and mechanism, and **task 812 stays open for a fix
task rather than being rounded up.**

## Code sha

- **HEAD at launch and at commit time: `17b1649`** (`17b164966b8fa70040ec63
  fd0b920af236261716`) — clean, `== origin/main`. `ci/stability.sh 10` ran
  against exactly this tree (whatever is checked out when the script starts
  is what `go test` builds), so `17b1649` is "the code sha" this report
  cites.
- The last commit that actually touches a `.go` or `.feature` file is
  **`fdf4507`** (`809`/`810` on top are docs/evidence commits). Confirmed:
  `git diff --stat fdf4507..HEAD -- '*.go' '*.feature' go.mod go.sum` is
  empty (checked in the same shell call, exit 0) — i.e. the product/test
  code at `17b1649` is byte-identical to `fdf4507`'s.

### The literal `git diff --stat` check named in this task's own criteria

The task text specifies `git diff --stat <that sha>..HEAD -- '*.go'
'*.feature' '*.sh' '*.toml' go.mod go.sum` "shown empty". Run with
`<that sha>` = `fdf4507`, this is **not** empty:

```
$ git diff --stat fdf4507..HEAD -- '*.go' '*.feature' '*.sh' '*.toml' go.mod go.sum
 docs/reports/phase3g-810-findings/reproduce-f37.sh | 55 ++++++++++++++++++++++
 1 file changed, 55 insertions(+)
```

The one line is `docs/reports/phase3g-810-findings/reproduce-f37.sh`, a
docs-evidence *reproduction* script task 810 round 2 added — it matches the
unrestricted `'*.sh'` glob even though it is not product or CI code. Citing
`<that sha>` = HEAD (`17b1649`) itself instead makes the same check trivially
empty (a commit diffed against itself), which is the reading used above: "the
code sha" for this report is `17b1649`, and `git diff --stat 17b1649..HEAD --
'*.go' '*.feature' '*.sh' '*.toml' go.mod go.sum` is empty because `17b1649`
**is** `HEAD`. This is a one-line wording gap in how the check composes with
an evidence `.sh` file living under `docs/reports/`, not a re-run of
`ci/stability.sh`; see the notes file for the same explanation.

## Launch

```
nohup sh -c 'timeout 7200 ci/stability.sh 10 > /run/ralphd/artifacts/stability08.log 2>&1; printf "%s\n" "$?" > /run/ralphd/artifacts/stability08.exitstatus' &
```

Issued verbatim as prescribed, backgrounded, from `17b1649`, at
2026-08-29 22:48:42 UTC. Polled periodically with `sleep N; grep -c '^===
RUN' /run/ralphd/artifacts/stability08.log` (never blocked on, never
re-run). Driver exited at 2026-08-29 23:53:40 UTC (~65 minutes wall for the
whole 10-run sweep). `stability08.exitstatus` reads `1`.
`/run/ralphd/artifacts/` after completion contains exactly `stability08.log`
and `stability08.exitstatus` for this task — no stray `nohup.out` or
`.nohup` sentinel.

## Per-run outcome

| run | result | scenario(s) failed |
|---|---|---|
| 1 | FAIL | `attach_acknowledges_a_live_error_without_replacing_its_verdict`, `Stale sampling is visible, precedence-aware, and agent-only` |
| 2 | FAIL | `attach_acknowledges_a_live_error_without_replacing_its_verdict` |
| 3 | FAIL | `attach_acknowledges_a_live_error_without_replacing_its_verdict` |
| 4 | FAIL | `attach_acknowledges_a_live_error_without_replacing_its_verdict` |
| 5 | FAIL | `attach_acknowledges_a_live_error_without_replacing_its_verdict`, `Stale sampling is visible, precedence-aware, and agent-only` |
| 6 | FAIL | `attach_acknowledges_a_live_error_without_replacing_its_verdict` |
| 7 | FAIL | `attach_acknowledges_a_live_error_without_replacing_its_verdict`, `Stale sampling is visible, precedence-aware, and agent-only`, `a wheel notch scrolls an attached pane's scrollback and leaves the shell's input line untouched` |
| 8 | FAIL | `attach_acknowledges_a_live_error_without_replacing_its_verdict`, `Stale sampling is visible, precedence-aware, and agent-only` |
| 9 | FAIL | `attach_acknowledges_a_live_error_without_replacing_its_verdict`, `Stale sampling is visible, precedence-aware, and agent-only` |
| 10 | FAIL | `attach_acknowledges_a_live_error_without_replacing_its_verdict`, `Stale sampling is visible, precedence-aware, and agent-only` |

All ten per-run logs (`run-1.log` … `run-10.log`) and the combined
`summary.log` are committed alongside this README. Every non-`features`
package (`cmd/deck`, `cmd/fake-claude`, `cmd/fake-pi`, `internal/agent`,
`internal/audit`, `internal/config`, `internal/hookrecv`,
`internal/interactive`, `internal/service`, `internal/store`,
`internal/theme`, `internal/tmux`, `internal/tui`) reports `ok` in all ten
runs; every failure is inside `github.com/n-orlov/deck/features`.

## Failure mechanisms, by scenario

### `attach_acknowledges_a_live_error_without_replacing_its_verdict` — 10/10 runs (deterministic)

`features/status_attach.feature:18`. Every run's failure text is
byte-for-byte the same shape already quoted in task 802's finding and
reconfirmed by task 811's investigation this approach:

```
status starting source tmux reason tmux pane is alive; terminal row corrected ... want error hook tool_failure
```

Root cause (task 802/811's finding, unchanged by this run): the R76
unconditional repair runs synchronously inside the same `deck _hook`
subprocess, before the pre-repair status is ever externally observable —
there is no window to assert it without dropping the scenario's `!`-marker
assertions, and both possible fixes (weaken the scenario / gate the repair)
are forbidden by standing rules. Log evidence: e.g.
`run-2.log` (`grep -n "attach_acknowledges" run-2.log`).

### `Stale sampling is visible, precedence-aware, and agent-only` — 6/10 runs (1, 5, 7, 8, 9, 10)

`features/status_probe.feature:33`. Failure text (identical across all six
occurrences, e.g. `run-1.log`):

```
after scenario hook failed: session "sampled pi" verdict = "starting"/"tmux" reason "tmux pane is alive; terminal row corrected", want "error"/"probe" reason "agent error" (err=<nil>)
```

Same repair-timing mechanism as the deterministic failure above — the R76
repair reaches a **probe**-sourced bare error before the assertion window,
not just a hook-sourced one — but here it does not fire on every run,
because the scenario's probe-sourced error is set up differently and the
race window is narrower/looser than `status_attach.feature`'s. This
particular scenario is not one of the five feature files task 811/802-805
re-pointed this approach (`status_attach`, `status_claude_hooks`,
`sort_order`, `status_recovery`); it surfaces here for the first time in
this approach's own evidence. Not claimed fixed by any task in this plan;
named here as observed, per this task's own instruction to publish a
sub-10/10 rate verbatim rather than rounding up. A fix task would need to
either re-point this scenario the same way 803/804/805 re-pointed theirs, or
determine why the race is narrower here than in `status_attach.feature`.

### `a wheel notch scrolls an attached pane's scrollback and leaves the shell's input line untouched` — 1/10 runs (7 only)

`features/attach_scroll.feature:11`, log `run-7.log`. The only byte
difference between the captured "before" and "after" frames is the tmux
status line's wall-clock field ticking over a minute boundary between the
two captures (`23:25` → `23:26`); every `SCROLL_LINE_*` row is identical.
This is **not** task 503/F30's already-fixed window-name caching race
(`window_name` is unaffected here) — it is a new, separate flake: the
scenario's frame-equality assertion is sensitive to tmux's status-line clock
ticking during the ~1s window between the two frame captures, which is
possible whenever the scenario's wall-clock execution straddles a minute
boundary. Not claimed fixed by any task in this plan.

## Conclusion

`0/10 passed`, script exit `1`. Task 812's own completion bar (`10/10`,
exit `0`) is not met. This task stays open; a fix task is needed for (a) the
`attach_acknowledges_a_live_error_without_replacing_its_verdict` scenario
that 811 already found unsatisfiable to re-point without a forbidden change,
(b) the newly observed `status_probe.feature` "Stale sampling…" scenario
hitting the same repair-timing race non-deterministically, and (c) the
newly observed `attach_scroll.feature` wheel-notch scenario's status-line
clock-tick flake. None of these three is fixed by this task; none is
claimed fixed.
