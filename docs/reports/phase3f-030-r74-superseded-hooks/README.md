# Task 030 — R74 leg 2: a hook write from a superseded launch generation is dropped, for the whole class

Issue #11 (R74), phase 3f approach 02. Leg 1 (task 029, `a0d4887`) minted the
per-launch generation and got it into the agent's environment. This leg is the
consumer: the hook receiver drops a status write whose generation token is not the
one the row currently names. Releasing the lease of a completed launch is R75
(task 031) and is not in this commit.

## 1. The rule, and where it is enforced

`internal/hookrecv.Receive` now takes the hook pane's own generation token
beside the injected row identity, and decides one thing with it
(`supersededLaunch`):

> if the row names a current launch generation and the hook's token is not that
> one, the hook's **status write never happens**.

The chain end to end:

1. `AcquireLaunchLease` mints the token and stores it in `launch_lease_owner`
   as `pid@boot_id#<token>` (leg 1).
2. `store.Session.LaunchGeneration` surfaces the generation half — and only
   that half — to every reader that already resolves a row, which is what the
   hook receiver does anyway (`sessionColumns` gained
   `COALESCE(launch_lease_owner, '')`; `scanSession` splits it with the existing
   `splitOwnerGeneration`). No new query, and no exported helper that nothing
   calls.
3. `Claude.Instrument` exports the token as `DECK_LAUNCH_GENERATION` (leg 1);
   the name is now the shared constant `agent.LaunchGenerationEnv`, used by both
   the writer and the reader, because a typo on either side would silently look
   like "this hook carries no token".
4. `cmd/deck` `_hook` reads `os.Getenv(agent.LaunchGenerationEnv)` and hands it
   to `Receive`.

**How the drop is implemented matters.** It is not a post-hoc repair and not a
second write path: the receiver sets the existing unsatisfiable
`AllowedCurrentStatuses` sentinel (`noCurrentStatusMatches`, the mechanism
requirement 43 already uses for an in-session `SessionEnd`), so inside
`store.UpdateSessionStatus`'s own transaction the status update is skipped while
the event is still recorded verbatim. Consequences, both intended:

- the row is never even momentarily wrong, so nothing has to notice and fix it;
- the superseded hook is still on the record as an event with its payload, so a
  stale pane's chatter stays diagnosable. `Result.Superseded` reports the
  decision to the caller.

One write that is *not* only a status write is gated on the same rule:
requirement 44's conversation-id move. A replaced pane's `SessionStart` names
the conversation *that* pane was running, so adopting it would point the row at
a conversation that is already over
(`TestSupersededSessionStartDoesNotMoveTheConversationID`).

The rule is applied to the **whole hook class**, not to `SessionEnd` alone.
`SessionEnd`→`stopped` is the one that visibly wrecked a live row, but
`Stop`→`idle`, `Notification`→`waiting`, `SessionStart`/`UserPromptSubmit`→`running`
and `StopFailure`→`error` are the same false statement about the same dead pane.
Revert B in §4 is the evidence that the class breadth is load-bearing rather than
decorative.

## 2. The three token cases, decided deliberately

The interesting cases are the ones where a token is absent. Both are decided on
purpose, and both are pinned by subtests:

| row token | hook token | decision | why |
| --- | --- | --- | --- |
| set | same | **apply** | this is the current launch speaking |
| set | different | **drop** | the token names a launch deck has already replaced |
| set | *absent* | **drop** | see below |
| *absent* | anything | **apply** | nothing to discriminate; pre-R74 behaviour, unchanged |

**Hook carries no token while the row holds one → superseded.** A leased launch
always exports its token, so a hook without one cannot have come from the launch
the row currently names. In deck today it can only have come from the row's
*create-time* pane: `CreateAgent` takes no launch lease and therefore injects no
generation (`internal/service/agent.go`), so the first pane of a row that was
later resumed is exactly the older launch this rule exists to discount. Treating
an absent token as "unknown, therefore allow" would leave every resumed row's
first-launch hooks still able to stop it. Revert C in §4 shows what that costs.

**Row holds no token → nothing to discriminate.** No lease has ever been taken
on the row, so deck has no opinion about which of its launches is current, and
every hook applies exactly as it did before R74. This is what keeps pre-R74
rows (and any path that never acquires a lease) working; it is also why none of
the pre-existing hookrecv tests needed a behaviour change.

One pre-existing test fixture *was* updated, and it is worth naming:
`TestResumedRowAcceptsTheNewPanesFirstHook` (issue #9's guard-2 leg) acquires a
real lease and then feeds the new pane's first hook. It now passes
`result.LaunchGeneration` as that hook's token, because that is what a
really-resumed pane exports. Left empty, the fixture would have been testing
R74's drop instead of #9's crash-verdict guard — the fixture was made *more*
faithful, not more permissive, and the assertion it makes is unchanged.

## 3. The discriminator: ordering is the whole test

`internal/service/superseded_launch_hook_test.go`,
`TestKilledLaunchsSessionEndAfterTheLeaseNeverStopsTheRow` — real tmux, real
`Resume`, real `hookrecv.Receive` (nothing about the receiver's decision is
simulated). The fixture is the field story from issue #11:

1. `CreateAgent`, then one `x`+`r` so the pane deck is about to kill is itself a
   **leased** launch — both panes in the story carry a token, so the test cannot
   pass merely because a token was absent. Its token is read from tmux itself
   (`assertTMuxEnvironment ... DECK_LAUNCH_GENERATION`).
2. `x` then `r` again: `AcquireLaunchLease` mints the *current* generation and
   the replacement pane comes up with it. The two tokens are asserted different.
3. **Now** the killed launch's `SessionEnd` arrives — after that
   `AcquireLaunchLease`, after the replacement pane exists.

The three assertions, in the order the criteria ask for them:

- **Immediately** after the hook, the row is not `stopped`: never-applied, not
  repaired-later. This is the assertion that goes red on unfixed code, with
  `status="stopped" reason="other" source="hook"`.
- After `service.Reconcile` with **no further hook**, the row is still not
  `stopped`, and the replacement pane is still live.
- `x` then `r` still recovers the row, with a third distinct generation.

**The same fixture with the hook delivered BEFORE the lease is not the
discriminator, and this is measured, not asserted in prose.**
`TestKilledLaunchsSessionEndBeforeTheLeaseIsNotTheDiscriminator` is the same
story with the `SessionEnd` arriving while the row is still stopped, and it
**passes on unfixed code** — see the verbose log in §4: with the fix disabled,
the after-lease test FAILS and the before-lease test PASSES in the same run.
The reason is that before the lease the row is already `stopped`, so the hook's
`stopped` write is a no-op (and store.go's "a hook cannot resurrect a stopped
row" guard drops it in any case), and the resume that follows leaves the row
running. It is kept in the suite beside the discriminator precisely so a later
change cannot quietly swap the ordering that matters for the ordering that does
not.

One test artefact is called out in the test itself: the helper expires
`launch_lease_until` before each resume, because the test clock never advances
and the previous launch's 30 s lease would otherwise still be held by this very
process. That is R75's subject (task 031), not this leg's.

## 4. Red-on-revert evidence

Each revert was applied to the working tree, the tests were run, and the file was
restored from a copy taken before the revert (`git diff` afterwards shows only the
intended change). Host load is recorded because it is a known confound; all four
runs were taken at `/proc/loadavg` 1-minute values 3.09–5.75.

| revert | change made | result |
| --- | --- | --- |
| A | disable the drop entirely (`if false && supersededLaunch(...)`) — i.e. pre-fix behaviour | [revert-a-no-drop-at-all.log](./revert-a-no-drop-at-all.log): the service discriminator fails with `the killed launch's SessionEnd stopped the row deck had just started: status="stopped" reason="other" source="hook"`, and **12** hookrecv subtests fail with `hook from a superseded launch reached the row` (6 event names × {superseded token, no token}), plus the conversation-id test |
| A (ordering) | same revert, `-v`, service package only | [revert-a-service-ordering-verbose.log](./revert-a-service-ordering-verbose.log): `--- FAIL: TestKilledLaunchsSessionEndAfterTheLeaseNeverStopsTheRow` **and** `--- PASS: TestKilledLaunchsSessionEndBeforeTheLeaseIsNotTheDiscriminator` in one run — the proof that the before-lease ordering is green on unfixed code |
| B | narrow the class to `SessionEnd` only (`p.EventName == "SessionEnd" && supersededLaunch(...)`) | [revert-b-sessionend-only.log](./revert-b-sessionend-only.log): the service test passes, so a `SessionEnd`-only fix would have looked complete — while **10** hookrecv subtests fail (`Stop`, `Notification`, `SessionStart`, `UserPromptSubmit`, `StopFailure`) plus the conversation-id test |
| C | treat a tokenless hook as allowed (`if hookGeneration == "" { return false }`) | [revert-c-tokenless-hook-allowed.log](./revert-c-tokenless-hook-allowed.log): exactly the **6** `hook_carries_no_token` subtests fail, one per event name — the deliberate decision in §2 is enforced, not incidental |

Revert B is the one to read twice: it is green on the very test that reproduces the
field bug, which is why the class matrix exists as well.

## 5. store.go's guards are untouched

`git diff internal/store/store.go` for this commit contains exactly three hunks
— the `Session.LaunchGeneration` field, the `scanSession` scan of
`COALESCE(launch_lease_owner, '')`, and that column in `sessionColumns`. Neither
protected guard appears in the diff at all:

    internal/store/store.go:621  // from, so a hook arriving for one anyway is stale/out-of-order and must
    internal/store/store.go:624  if apply && input.Source == "hook" && currentStatus == "stopped" {
    internal/store/store.go:625      apply = false

    internal/store/store.go:669  pane_exit_status = COALESCE(?, pane_exit_status),

The "a hook cannot resurrect a stopped row" guard and the `pane_exit_status`
`COALESCE` are both still present, unchanged and unweakened. R74's drop is a
layer *above* them (the receiver declines to write at all), which is why neither
had to move: the guards answer "can a hook move THIS row from THIS status", the
new rule answers "is this hook even from the row's current pane".

## 6. Green run, what the whole suite found, and what is not claimed

Required packages, green at this commit:

    ci/run.sh go test -count=1 ./internal/service/ ./internal/hookrecv/ ./internal/store/

[green-service-hookrecv-store.log](./green-service-hookrecv-store.log) — exit 0,
`ok` for all three packages, taken at `/proc/loadavg` `3.09 3.47 3.21`.

**One whole-suite run was taken this iteration and it is published even though it
failed**, because it found something real:
[whole-suite.log](./whole-suite.log) — `--- FAIL:
TestFeatures/a_running_hook_cannot_undo_an_explicit_user_kill`, with the row left
at `status "starting", source "tmux"` where the scenario wants `"running"`,
`"hook"`. Cause: the `the released {running,waiting} hook fires for session "X"`
step (`features/assertions_test.go`, `releasedHookForSession`) runs the real
`deck _hook` binary out of band and exported only `DECK_SESSION_ID`, so after an
`r` the synthetic hook carried no generation and R74 correctly took it for a
replaced pane's hook.

The fixture was corrected, not the assertion: the step now also exports the row's
own current generation (read from `launch_lease_owner`), which is what a hook
running inside the live pane inherits, and exports none when the row holds none.
No scenario, step wording, tag or expected count was changed.
[features-hook-env-fixture.log](./features-hook-env-fixture.log) — the two
affected feature files, `3 scenarios (3 passed)`, `36 steps (36 passed)`, at
`/proc/loadavg` `3.87 3.13 3.07`. A fresh whole-suite run at the final code
commit is task 032's deliverable and is deliberately not chained here.

Not claimed by this leg:

- **The lease itself still is not released early.** Every relaunch in these tests
  has to expire `launch_lease_until` by hand because the test clock is frozen;
  that is R75 (task 031).
- **Nothing is done about `reconcile.go`'s "terminal row + live pane" invariant
  detector**, which remains explicitly out of scope for R74/R75 and is recorded
  as a follow-up.
- **A dropped hook is recorded but not labelled as dropped.** The event carries
  its payload verbatim and the row is untouched, which is the same shape
  requirement 43 uses, so "event present, status unchanged" is the only signal a
  forensic reader gets. Marking such events explicitly would need a new event
  kind or reason and was not in this leg's criteria.
