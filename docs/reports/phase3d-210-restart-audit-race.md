# Task 210 (part 1): root cause and fix — "R restarts a claude session ..." audit-count race

## Failure

`features/agent_session.feature`'s two `R`-restart scenarios:

- `R restarts a running claude session with the resume argv, preserving its conversation id` (line 33)
- `R restarts a claude session, recording environment key names in the launch audit but never a value` (line 92)

both failed intermittently (6/10 in task 209's `ci/stability.sh 10` measurement for the
second one — the single dominant failure in that taxonomy) with:

```
after scenario hook failed: audit log has 1 launch records for session "restart audit env", want 2
```

Reproduced directly: isolating either scenario via a scratch `Paths` override in
`features/godog_test.go` (reverted before commit, `git diff` on that file empty) and running
`ci/run.sh sh -c 'go test -count=10 -run TestFeatures -v ./features/'` at 1-min loadavg ~7-9
failed 6/10 times for the "recording environment key names" scenario before this fix — matching
209's own taxonomy almost exactly. See `/tmp/210work/restart-audit-10x.log` (this run's own
scratch reproduction; not committed, container-local — cited by path per this run's existing
convention for scratch diagnostic logs).

## Root cause

Both scenarios' step sequence is:

```
When deck client "A" presses R on session "..."
Then within one configured reconcile interval deck client "A" screen contains "fake-claude resume:"
And the audit log has 2 launch records for session "..."
```

The first `Then` step (`clientScreenContainsWithinReconcileInterval`) polls the **passive
preview panel's rendering of the live tmux pane's own stdout** — a channel that is completely
decoupled from deck's own `Restart`/`Resume` Go call. Once `internal/service/resume.go`'s
`s.TMux.Create(...)` returns, the newly forked `fake-claude --resume ...` process can print its
`fake-claude resume: <uuid>` banner to the pty within microseconds — well before the *same*
`Resume` call reaches its own next line, `s.Audit.Launch(...)`, especially under host CPU
contention (this run's shared host repeatedly showed 1-min loadavg spikes to 80-176 during
whole-suite runs; see task 217's own attempts 2-5). Nothing in SPEC.md promises the audit-log
append happens before the pane's own output becomes externally observable — the preview simply
streams whatever the pane already produced, independent of deck's internal audit bookkeeping.

The very next step, `the audit log has 2 launch records for session "..."`, was a **single,
non-retrying** read of the JSONL file
(`auditHasLaunchRecordCountForSession` in `features/agent_steps_test.go`). It assumed the audit
write is already durable the instant the screen shows the pane's resume banner — an ordering the
product never guarantees. That assumption is what was wrong, not the product: this is a genuine
eventual-consistency gap between two independently-observable channels (pane stdout vs. audit
JSONL file), not a double-launch, a lost record, or a value leak.

Confirms this is **pre-existing** and unrelated to tasks 213-220: `git log --oneline -- 
features/agent_session.feature features/agent_steps_test.go` before this commit shows no
201-220 commit touching either file, and task 209 (measured before 213-220 landed) already
recorded this exact scenario failing 6/10.

## Fix

Added a bounded-polling variant of the audit-record-count assertion,
`auditHasLaunchRecordCountForSessionWithinReconcileInterval`
(`features/agent_steps_test.go`), registered as a new step
`^within one configured reconcile interval the audit log has (\d+) launch records? for session "([^"]+)"$`.
It polls every 20ms up to `scenarioReconcileInterval + 250ms` (the same bound the existing
`clientScreenContainsWithinReconcileInterval` step already uses) and still fails outright, with
no retry beyond that bound, if the count is never reached — it is not a sleep or a widened
checkpoint masking a one-off miss; it models the same "eventually, not lockstep, consistent"
reality the codebase already expresses via `deckSelectionBufferEventuallyContains`
(`features/interactive_selection_test.go`, task 216) for an analogous decoupled-channel race
(deck's own tmux selection buffer vs. the SGR release bytes that populate it).

Both scenarios' second `And the audit log has 2 launch records ...` step (the one immediately
following the pane-content `screen contains` step) now uses the polling variant instead. No
other `the audit log has N launch records` call site in any `.feature` file was changed — every
other occurrence either follows a step that is itself driven by the *same* synchronous call path
that also performs the audit write (so no reordering is possible), or checks the count before any
`R`-style relaunch happens at all.

## Non-vacuousness

The *before* state is already on record: the pre-fix isolated run
(`/tmp/210work/restart-audit-10x.log`) failed 6/10 with exactly the race message above. Post-fix,
both affected scenarios were independently re-isolated (same scratch-`Paths`-override method,
reverted before commit) and run 10 additional times each at comparable host load (1-min loadavg
7-10):

- "R restarts a claude session, recording environment key names ..." (line 92): 10/10 pass,
  `/tmp/210work/restart-audit-10x-fixed.log`.
- "R restarts a running claude session with the resume argv, preserving its conversation id"
  (line 33): 10/10 pass, `/tmp/210work/restart-claude-10x-fixed.log`.

(Scratch logs are container-local per this run's existing convention for isolated diagnostic
reruns cited by path rather than committed in full, matching tasks 209/217/220's own practice for
ad hoc scratch evidence.)

## Scope note

This resolves the **dominant** (6/10) failure class from task 209's taxonomy. Task 210's other
two named priorities — `TestGoldenMinimumFrame`'s quiescence heuristic (3/10) and
`internal/tmux`'s `TestPanePipeReceivesGenuineEOFOnDisplacementWithPanePipeStillOne` (1/10) — plus
the "unique directory match ghosts" failure (2/10) from 209's raw taxonomy, are **not yet**
root-caused. Task 210 remains in-progress; see `notes.md` for the handoff.
