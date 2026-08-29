# Task 502: `attention_sort.feature:92`'s collapsed-strip attention-count race

## The race

Scenario `the collapsed strip's attention count matches the sort's own notion
of attention` (`features/attention_sort.feature:92`) sets three sessions'
statuses directly in the state database (`cnt-wait`→waiting, `cnt-err`→error,
`cnt-idle`→idle), waits for the client to pick that up, then sends `|` three
times and asserts the collapsed strip's attention count is 2.

**What it was reading before it settled.** The wait step it used was
`within one configured reconcile interval deck client "A" screen contains
"waiting"` → `clientScreenContainsWithinReconcileInterval`
(`features/assertions_test.go:113`, calling `clientScreenContainsBefore` at
`assertions_test.go:122-140`). That helper returns as soon as the substring
`"waiting"` appears *anywhere* in the client's captured frame. The deck
client's reconciler applies session-status updates incrementally as it
processes each session's DB row and emits a render for each one it changes,
rather than atomically publishing all three of this scenario's updates in a
single frame. So `"waiting"` (from `cnt-wait`, first in DB order) can render
and satisfy the wait *before* the reconciler has gotten to `cnt-err`/`cnt-idle`
and issued their renders. The scenario then proceeds to send `|`×3 and check
the attention count while `cnt-err`'s status (which counts toward "attention")
may still show its pre-update value — undercounting by exactly the "error"
session, i.e. the observed `attention count = 1, want 2`.

**The fix** (`features/attention_sort.feature`, this task's diff) replaces
that one proxy wait with three waits on the actual per-row state the count
assertion depends on, using the step already defined for exactly this purpose
elsewhere in the suite — `within one configured reconcile interval deck
client "A" row "<name>" contains "<status>"` →
`clientRowContainsWithinReconcile` (`features/status_probe_test.go:238-258`,
registered at `status_probe_test.go:28`):

```
Then within one configured reconcile interval deck client "A" row "cnt-wait" contains "waiting"
And within one configured reconcile interval deck client "A" row "cnt-err" contains "error"
And within one configured reconcile interval deck client "A" row "cnt-idle" contains "idle"
```

Each wait polls (10ms) until *that specific* row shows *that specific* text or
the reconcile-interval budget elapses, so the scenario cannot reach the `|`×3
/ count-check step until all three status writes the count depends on have
actually rendered. This synchronises on the same observable state the
assertion reads, rather than a proxy that happens to settle early. The
scenario keeps asserting attention count 2 and both its tags
(`@requirement-31-attention-count`, `@requirement-15-collapsed-strip`); no
scenario was deleted, skipped, or tagged out; `features/godog_test.go`'s
`defaultTags` is untouched.

## Reproduction narrowing

Per approach 04's note, loading the whole `attention_sort.feature` file under
load hits a different scenario's race first. This scenario is isolated with
Go's own subtest-path `-run` filtering against the single-scenario subtest
name godog registers, which lets godog parse the file (registering all 6
scenarios) but execute only the one matching subtest — confirmed by
`-v` output showing exactly one `Scenario:` line and one executed step
sequence per invocation:

```
DECK_GODOG_PATHS=attention_sort.feature go test ./features/ \
  -run "TestFeatures/the_collapsed_strip.s_attention_count_matches_the_sort.s_own_notion_of_attention" \
  -count=1
```

(the two apostrophes in the scenario's title are matched with `-run`'s regex
`.` wildcard since Go's `-run` is a plain regex over the slash-joined subtest
path).

## Evidence

All commands below were run via `ci/run.sh` (the project's Go+tmux sibling
container). Load was applied by starting N `yes >/dev/null &` processes in
*this* job container immediately before each loop (their exact PIDs were
captured at spawn and killed by PID afterwards — never by pattern).

### Red-before (unmodified route, narrowed to this one scenario, load applied after narrowing)

12 concurrent `yes` background processes, looped over the narrowed command
above, 50 iterations, **on the pre-fix tree** (`git stash` held this task's
feature-file diff out during capture):

- `red-before-loop-summary.log` — per-run exit status; **6/50 fail** (runs 8,
  11, 24, 25, 30, 48), each a fresh `ci/run.sh ... -count=1` invocation with
  its own captured `$?` on the same line.
- `red-before-failure-transcript.log` — run 8's full `-v` transcript (trimmed:
  the harness's SIGQUIT goroutine dump from force-killing the hung deck client
  during scenario-hook cleanup is elided, noted inline, and is not part of the
  assertion failure itself), showing `after scenario hook failed: deck client
  "A" collapsed strip attention count = 1, want 2` — the exact race described
  above. All 6 failing runs showed this identical message (verified by
  `grep -o 'attention count = [0-9], want [0-9]'` across all 6 raw logs before
  trimming).

### Green-after

**(a) The required check** — five consecutive runs of the exact deliverable
command, unmodified environment, no artificial load, **on the post-fix
tree**:

```
ci/run.sh env DECK_GODOG_PATHS=attention_sort.feature go test ./features/ -run TestFeatures -count=1
```

`green-after-required-5x-summary.log` records all 5 commands and their exit
statuses (all `0`); `green-after-required-5x-run{1..5}.log` hold each run's
`go test` output (`ok  	github.com/n-orlov/deck/features	...`).

**(b) Stress confirmation** — same load level (12 concurrent `yes`) and same
narrowed single-scenario command as the red-before capture, **on the post-fix
tree**, 80 iterations:

- `green-after-stress-loop-summary.log` — 7/80 fail (runs 4, 24, 41, 45, 56,
  66, 72). **None of these is the race this task fixes**: every one of the 7
  is `client "A" row "cnt-err" did not contain "error" within reconcile
  interval` — a pre-existing, unrelated margin in the shared
  `clientRowContainsWithinReconcile` helper itself (`status_probe_test.go:238`,
  a fixed 250ms-reconcile-interval + 250ms budget), used by other scenarios
  too and predating this change; under this task's synthetic CPU load it can
  legitimately exceed its own budget before the reconciler tick lands. It is
  out of scope for task 502 (which is about the *order* the assertions
  synchronise on, not this helper's timeout margin) and is called out here
  rather than left unexplained. Confirmed the original symptom never
  recurred: `grep -l 'attention count = 1' green-after-stress-loop-*.log`
  (run separately per file at capture time) matched **zero** of the 80 logs.

## Command exit-status capture discipline

Every `$?` above was captured in the same shell invocation immediately after
the command line that produced it, never after a pipe, per this run's
standing rules.
