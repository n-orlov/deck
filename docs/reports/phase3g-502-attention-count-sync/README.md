# Task 502: `attention_sort.feature:92`'s collapsed-strip attention-count race

## The race

Scenario `the collapsed strip's attention count matches the sort's own notion
of attention` (`features/attention_sort.feature:92`) creates four shell
sessions, then writes three of their statuses straight into the state database
(`cnt-wait`→waiting, `cnt-err`→error, `cnt-idle`→idle), waits for the client to
pick that up, sends `|` three times and asserts the collapsed strip's attention
count is 2 (waiting + error, `internal/tui/attention.go:185-187`'s
`NeedsAttention`, counted by `Model.attentionCount`,
`internal/tui/tui.go:3871-3879`).

### What the assertion was reading before it settled

The three status writes are **three separate raw SQL `UPDATE`s**, one per
Gherkin step — `setSessionStatusSecondsAgo`
(`features/attention_sort_test.go:42`, the `UPDATE sessions SET status = ?…`
at `features/attention_sort_test.go:53-56`). They land at three different
instants, seconds after the four sessions were created, and they bypass
`store.UpdateSessionStatus`' precedence rules entirely.

Deck's client has a **concurrent writer to the same column**. Each
`reconcileTick` (`internal/tui/tui.go:2193-2203`) runs
`internal/service.Service.Reconcile` and then reloads the whole session list
through one `ListSessions` snapshot (`Model.loadSessions`,
`internal/tui/tui.go:1396-1401`). A reconcile pass reads every durable row
**once, at its start** (`internal/service/reconcile.go:48`) and afterwards, for
a shell row that snapshot showed as `starting` with a live pane, issues
`UpdateSessionStatus{Status: "running", Source: "tmux"}`
(`internal/service/reconcile.go:103-113`) — SPEC §7's shell-liveness
promotion. The sessions this scenario creates are exactly that: freshly
created shell rows that sit at `starting` until a pass promotes them.

So when the scenario's `cnt-err`→`error` write lands **between** a pass's
snapshot read and that pass's promotion write, the promotion still applies and
silently overwrites `error` with `running`. It is not rejected as a stale
promotion, because `store.UpdateSessionStatus`' tmux guard
(`internal/store/store.go:685`) admits the write on either of two grounds, and
while `tmuxShellPromotion` (`internal/store/store.go:666`, which does require
`currentStatus == "starting"`) is false by then, `tmuxTerminalRepair`
(`internal/store/store.go:673-674`: current status `error`, agent `shell`,
incoming `running`) is true — the row now looks like SPEC §7's
terminal-row-with-a-live-pane invariant violation, so running-over-error is
accepted. The promotion carries no `ExpectedStatus`/`AllowedCurrentStatuses`,
so nothing else stops it.

The result is **durable, not a stale frame**: `cnt-err` is `running`/`tmux`
from then on, the attention count is 1 for the rest of the scenario, and the
count step's own 5-second poll (`features/attention_sort_test.go:220-276`,
deadline at `:233`) cannot recover — which is exactly the observed
`attention count = 1, want 2`. Direct evidence, dumped from the state DB at
the moment the assertion gave up: `mechanism-diag-prefix-clobber.log`
(`cnt-err|running|tmux|tmux pane is alive`, while `cnt-wait`/`cnt-idle` still
carry their `feature-test` writes).

The step the scenario used to wait on before checking the count was
`within one configured reconcile interval deck client "A" screen contains
"waiting"` → `clientScreenContainsWithinReconcileInterval`
(`features/assertions_test.go:113`, via `clientScreenContainsBefore`
`features/assertions_test.go:156`). That returns as soon as the substring
`waiting` appears **anywhere** in the frame, which a frame rendered from a
snapshot taken by a pass that started *before* the writes already satisfies.
It therefore let the scenario walk on while such a pass was still in flight —
i.e. it did not synchronise on the state the count assertion reads at all.

### The fix

`features/attention_sort.feature` (this task's diff) replaces that one proxy
wait with three waits on the actual per-row state the count depends on, using
the step already defined for this purpose elsewhere in the suite —
`clientRowContainsWithinReconcile` (`features/status_probe_test.go:238-258`,
registered at `features/status_probe_test.go:28`):

```
Then within one configured reconcile interval deck client "A" row "cnt-wait" contains "waiting"
And within one configured reconcile interval deck client "A" row "cnt-err" contains "error"
And within one configured reconcile interval deck client "A" row "cnt-idle" contains "idle"
```

Each wait polls (10ms) until *that specific* row shows *that specific* status.
Reaching the `|`×3 / count step now requires a rendered frame produced from a
`ListSessions` snapshot taken **after** all three writes — including one that
read `cnt-err` as `error`, which is only possible once the promotion window for
that row has closed (a pass that reads `error` takes no liveness verdict at
all: `terminal` is false for a plain `error` row, `internal/service/reconcile.go:80-82`,
and the `starting`-only promotion at `:103` does not match). The scenario
synchronises on the same observable state the assertion reads instead of on a
proxy that settles early.

No weakening: the scenario still asserts attention count 2, keeps
`@requirement-31-attention-count` and `@requirement-15-collapsed-strip`, no
scenario is deleted, skipped, retagged or tagged out, `features/godog_test.go`'s
`defaultTags` is untouched and no `t.Skip` was added (all visible in the
commit's `git diff`).

## Reproduction narrowing

Per approach 04's note, loading the whole `attention_sort.feature` file under
load hits a different scenario's race first. This scenario is isolated
**before any load is applied**, with Go's `-run` subtest-path filter over the
subtest name godog registers per scenario: godog still parses the file (all 6
scenarios register; the other 5 report as `undefined` because only the
selected subtest runs its steps), and exactly one `Scenario:` line executes —
visible in every captured log.

## Evidence

Every command below was run through `ci/run.sh` (the project's Go+tmux sibling
container) and every exit status was captured in the same shell invocation,
immediately after the command that produced it, never after a pipe. Load, where
used, was 12 concurrent `yes >/dev/null` processes started in the job container
with their PIDs captured at spawn and killed by PID (never by pattern)
afterwards.

### Red-before — unmodified route, narrowed first, then load

Tree: a detached `git worktree` at `19fff8d`, the commit immediately before the
fix `306d57a`; nothing in it is hand-edited.

- `red-before-loop-summary.log` — its header records the tree, the pre-fix wait
  step it ran, the narrowing, the load, and the one exact complete command
  repeated for all 100 runs, then one line per run carrying that same command
  and its captured exit status: **8/100 fail** (runs 13, 14, 37, 41, 75, 79,
  81, 95). All 8 failures are the identical `attention count = 1, want 2`.
- `red-before-failure-transcript.log` — run 13 verbatim (`-v`), with its exact
  complete command and captured exit status (`1`) in the header. Three
  single-line raw PTY byte dumps the harness prints are truncated with an
  explicit inline `[ELIDED: …]` marker naming what was cut and why; nothing
  else is altered. Note the status line in the captured frame: `running - tmux
  pane is alive` — the reconciler's own promotion reason, visible without any
  instrumentation.

### Mechanism diagnostics (instrumented throwaway trees, not part of the red-before route)

To identify *which write* the assertion was reading, the failure paths were
made to dump the state DB's status columns. The instrumentation is test-side
only (`features/`), was applied to throwaway worktrees, and is **not committed**;
the exact diffs are `mechanism-diag-instrumentation.diff`.

- `mechanism-diag-prefix-clobber.log` — pre-fix (`19fff8d`): at the moment the
  count assertion gives up, `cnt-err` is `running`/`tmux`/`tmux pane is alive`.
- `mechanism-diag-postfix-under-load-clobber.log` — post-fix (`306d57a`) under
  the same 12× load: the same durable overwrite, now surfacing at the earlier
  `row "cnt-err" contains "error"` wait.

### Green-after — the required check

Five consecutive runs of the exact deliverable command, unmodified
environment, no synthetic load, at this report's tree:

```
ci/run.sh env DECK_GODOG_PATHS=attention_sort.feature go test ./features/ -run TestFeatures -count=1
```

`green-after-required-5x-summary.log` holds the command and the captured exit
status of each run (**all 5 exit 0**); `green-after-required-5x-run{1..5}.log`
hold each run's full output with its own command/exit-status header.

### Residual, under synthetic load only — a finding, not a fix

`green-after-stress-loop-summary.log` re-ran the narrowed single-scenario
command 80 times at the post-fix tree under the same 12× load: **36/80 failed**
— **0** of them on the count assertion (the pre-fix symptom `collapsed strip
attention count = 1, want 2` matches no run), all 36 at the new `row "cnt-err"
contains "error"` wait, and in all 36 the final rendered `cnt-err` row reads
`running` (per-run signatures are listed in that log).
`mechanism-diag-postfix-under-load-clobber.log` dumps the DB behind one of
them: **the same product-level lost update**, caught one step earlier and
reported as "the row never showed `error`" — because the row no longer said
`error`. They are not a timeout margin in `clientRowContainsWithinReconcile`.
Under this much synthetic load the earlier, tighter wait turns what the pre-fix
scenario could only report 5 seconds later (8/100 there) into an immediate
failure, which is why the loaded failure rate is higher post-fix while the
unloaded deliverable command is green.

Task 502's fix is a synchronisation fix and does not close that product
window; it stops the scenario from *asserting* against state it had not waited
for, and it fails loudly and early when the overwrite does happen. The
remaining gap is `internal/service/reconcile.go:103-113`'s promotion being
issued unconditionally from a snapshot that may already be stale: it carries
neither `ExpectedStatus` nor `AllowedCurrentStatuses`, so
`internal/store/store.go:673-674`'s terminal-repair branch lets it land on a
row that changed under it. Guarding that one write with the status it observed
(`AllowedCurrentStatuses: []string{"starting"}`) is the candidate fix; it needs
its own task, its own red/green evidence and a check against SPEC §7's
self-healing rule, and is recorded here plus in this approach's findings ledger
rather than smuggled into this commit. Under the deliverable command (no
synthetic 12× CPU load) the scenario is green.
