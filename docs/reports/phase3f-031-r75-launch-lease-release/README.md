# Task 031 — R75: a completed launch stops holding its 30-second launch lease

Issue #11 (R75), phase 3f approach 02. R74 (tasks 029 `a0d4887`, 030 `196e6f4`)
made a late hook from a superseded launch harmless. This task fixes the other
half of the same field story: the row's own finished launch kept the §9.3 launch
lease for the rest of its 30 s TTL, so the next resume of a legitimately stopped
row was answered *starting elsewhere* — a claim about another client, made when
the only "other client" was this process's own concluded launch.

## 1. What was wrong, and what the fix is

`AcquireLaunchLease` writes `launch_lease_owner` + `launch_lease_until = now +
30 s` in the same transaction that flips `stopped → starting`
(`internal/store/lease.go`). Nothing ever ended that hold: it could only expire.
So for 30 s after a launch, `leaseHeld` was true for the row — the owner is this
very process, therefore alive, and the TTL has not elapsed — and any resume of a
row that had legitimately become `stopped` again in that window returned
`ResumeStartingElsewhere` (`internal/service/resume.go`).

That is precisely what SPEC §9.3 forbids: *"'starting elsewhere' is a claim
about another client, so it is only made when one is actually there."* The lease
exists to keep a **second** launcher out **while a launch is in flight**; once
the pane is up, nothing is in flight.

The fix is two small pieces:

- `store.ReleaseLaunchLease(ctx, sessionID, heldOwner)` — sets
  `launch_lease_until = 0` and nothing else, CASed on the exact owner string the
  launch acquired (`UPDATE ... WHERE id = ? AND launch_lease_owner = ? AND
  launch_lease_until != 0`). It reports whether a held lease was actually
  released, so a release arriving after another launcher took the row over is a
  no-op rather than a lease it cuts short.
- `Resume` releases by `defer`, set up immediately after the acquisition
  succeeds, so **every** exit path below it concludes the attempt: the pane came
  up, or the launch failed and the row is `error`. A failure to release is
  recorded in the audit log (`launch_lease.release_failed`) and deliberately does
  not change the verdict the user is given for the launch — the §9.3 TTL is still
  the backstop, i.e. the worst case is exactly the pre-R75 behaviour.

No acquisition logic changed at all. `launch_lease_until = 0` is a value
`AcquireLaunchLease` already reads as breakable ("unset, or its TTL has
elapsed"), which is why the guard needed no edit and no weakening: what changed
is how long a launch *claims* to be in flight, not who may break a claim.

## 2. Why the owner column is kept, and how R74 stays intact

**The release must not blank `launch_lease_owner`, because that column IS R74's
token.** R74 stores the per-launch generation as the second half of the owner
value, `pid@boot_id#<generation>` (`composeLeaseOwner`,
`internal/store/lease.go`), and `store.Session.LaunchGeneration` surfaces that
half to every reader (`internal/store/store.go`). The hook receiver's whole rule
is "if the row names a current generation and this hook's token is not it, the
status write never happens", with the deliberate fall-through "row holds no
token → nothing to discriminate, therefore apply"
(`internal/hookrecv/receiver.go`, report
[phase3f-030-r74-superseded-hooks](../phase3f-030-r74-superseded-hooks/README.md)
§2).

So a release implemented as "clear the lease" — `launch_lease_owner = NULL` —
would silently un-fix R74 30 s into every launch: with no token on the row, every
late hook from a replaced pane becomes "nothing to discriminate" again and can
stop the row deck has just started. And the two lifetimes are genuinely
different: the lease is about a launch that is *in flight* (seconds), the
discriminator is about which pane is *current* (as long as that pane lives, and a
superseded pane can chatter long after 30 s).

Hence the released state is exactly **"the row still names the current launch,
nobody is mid-launch"**:

| column | after acquire | after release | why |
| --- | --- | --- | --- |
| `launch_lease_owner` | `pid@boot_id#gen` | **unchanged** | R74's discriminator; also what a later release CASes on |
| `launch_lease_until` | `now + 30 s` | `0` | the only thing that means "a launch is in flight" |

This is asserted, not merely stated. `TestReleaseLaunchLeaseLetsTheSameOwnerLaunchAgainInsideTheTTL`
(`internal/store/lease_release_test.go`) reads the raw columns after the release
and requires the owner byte-for-byte unchanged and `Session.LaunchGeneration`
still readable; `TestResumeReleasesItsLaunchLeaseWhenTheLaunchCompletes`
(`internal/service/lease_release_test.go`) requires the resumed row to still
carry a generation. Revert C in §4 is the measurement that those assertions bite:
blanking the owner in the release turns R74's own tests red.

**One test fixture became unnecessary and was removed rather than left dead.**
`expireLaunchLease` (added by task 030 in `internal/service/launch_generation_test.go`)
aged the lease out of its TTL before every test relaunch, because the test clock
never advances and the previous launch's lease was still held by the test process
itself. That is exactly the product defect R75 fixes, so the helper and its three
call sites are gone (`internal/service/superseded_launch_hook_test.go`'s
`relaunchForGeneration` included) and those tests now relaunch with no lease
fixture at all — which makes them, as a side effect, additional witnesses that
the product releases the lease. No assertion in them changed.

## 3. The two directions, both asserted

The requirement has a shape that is easy to satisfy dishonestly (delete the
guard), so both directions are pinned, at both layers.

**Direction 1 — the same owner is no longer locked out inside the window.**

- store: `TestReleaseLaunchLeaseLetsTheSameOwnerLaunchAgainInsideTheTTL` —
  acquire, release, put the row back to `stopped`, acquire again **at the same
  timestamp** (`leaseTestNow`, one literal used for every call, so nothing here
  can pass because time passed). The second acquire must succeed and must mint a
  *different* generation. A second release is asserted to be a no-op, not an
  error.
- service, real tmux, real `Resume`:
  `TestResumeReleasesItsLaunchLeaseWhenTheLaunchCompletes` — create an agent,
  then `x`+`r` twice in a row against a frozen clock, with **no lease fixture of
  any kind**. Both resumes must return `ResumeStarted` (the test names
  `ResumeStartingElsewhere` explicitly as the failure it is looking for), both
  must leave a live pane, `launch_lease_until` must be 0 after each, the owner
  must still carry its `#generation`, and the two generations must differ.

**Direction 2 — a lease genuinely held by a different live owner still blocks.**

- store: `TestReleaseLaunchLeaseDoesNotEndADifferentLiveOwnersLease` — a live,
  in-TTL incumbent; a release naming a *different* launch of the same row
  (same identity, different generation) releases nothing and leaves both columns
  untouched; a competing acquire still returns `LaunchLeaseHeldElsewhere` with
  the incumbent as `HeldBy` and the row still `stopped`.
- service: `TestResumeStillRefusesALeaseHeldByADifferentLiveOwner` — the holder
  is a **real live process this test spawned itself** (`sleep 300`; its pid, this
  boot's id, its own generation), not a fabricated pid, so the store's signal-0
  liveness probe genuinely answers "alive". `Resume` must return
  `ResumeStartingElsewhere`, must create no pane (`HasLivePane`), and must leave
  the holder's lease columns and the row's `stopped` status untouched. Then the
  holder is killed **and reaped** (a zombie still answers signal 0) and the very
  same `Resume`, same frozen clock, must succeed — the row is refused, never
  wedged.

The pre-existing §9.3 coverage is unchanged and still green in the same runs:
`TestAcquireLaunchLeaseLiveOwnerInTTLIsNotBreakable`,
`TestAcquireLaunchLeaseNeverWedgesTheRow`,
`TestResumeLosingLeaseCreatesNoTMuxSession`, `TestLiveOwnerWithGenerationIsStillNotBreakable`
(`internal/store/lease_test.go`, `internal/store/launch_generation_test.go`,
`internal/service/resume_test.go`), plus `features/launch_lease.feature` and
`features/lease_race.feature` through the real terminal (§5).

## 4. Red-on-revert evidence

Each revert was applied to the working tree, the tests were run, and the file was
restored from a copy taken before the revert; `git diff --stat` afterwards showed
only the intended change (two files, the fix itself). Every log carries the
`/proc/loadavg` reading at the start of its run, because host load is a known
confound.

| revert | change made | result |
| --- | --- | --- |
| A | `Resume` no longer releases (`if false { ... }` around the deferred release) — i.e. pre-R75 behaviour | [revert-a-no-release.log](./revert-a-no-release.log): `TestResumeReleasesItsLaunchLeaseWhenTheLaunchCompletes` fails twice over — `launch_lease_until = 1735787075000 after the launch completed; want 0` **and** `second resume inside the lease window: outcome = ResumeStartingElsewhere; the only launcher in this test is this process's own concluded launch`. `TestResumeStillRefusesALeaseHeldByADifferentLiveOwner` also fails on its final release check |
| B | the naive "fix": delete the lease-held guard in `AcquireLaunchLease` (`leaseHeld := false`) | [revert-b-guard-deleted.log](./revert-b-guard-deleted.log): **6** tests fail — R75's own two direction-2 tests (`TestReleaseLaunchLeaseDoesNotEndADifferentLiveOwnersLease`, `TestResumeStillRefusesALeaseHeldByADifferentLiveOwner`) plus the pre-existing `TestAcquireLaunchLeaseLiveOwnerInTTLIsNotBreakable`, `TestAcquireLaunchLeaseNeverWedgesTheRow`, `TestLiveOwnerWithGenerationIsStillNotBreakable`, `TestResumeLosingLeaseCreatesNoTMuxSession` |
| C | the release also blanks the owner (`launch_lease_owner = NULL` beside `launch_lease_until = 0`) | [revert-c-release-blanks-the-owner.log](./revert-c-release-blanks-the-owner.log): **5** tests fail, and the interesting ones are R74's, not R75's — `TestResumeExportsCurrentLaunchGenerationToThePane`, `TestKilledLaunchsSessionEndAfterTheLeaseNeverStopsTheRow` and `TestKilledLaunchsSessionEndBeforeTheLeaseIsNotTheDiscriminator` all report `stored launch_lease_owner "" carries no generation` |

Revert A is direction 1's red-on-revert; revert B is direction 2's, and it is the
answer to "was the guard simply deleted?" — deleting it costs six tests,
four of which predate this task. Revert C is the answer to "is the chosen release
mechanism compatible with R74?" — the version of R75 that blanks the owner takes
R74's discriminator down with it, and R74's own tests say so.

## 5. Green runs

Required packages, green at this commit's code:

    ci/run.sh go test -count=1 ./internal/store/ ./internal/service/

[green-store-service.log](./green-store-service.log) — `exit=0`, `ok` for both
packages, at `/proc/loadavg` `5.17 3.82 3.27`.

Because §9.3's user-visible behaviour is the thing being narrowed, the two lease
feature files were also run through the real terminal and real tmux:

    ci/run.sh env DECK_GODOG_PATHS=launch_lease.feature,lease_race.feature go test -count=1 -v -run TestFeatures ./features/

[features-lease.log](./features-lease.log) — `5 scenarios (5 passed)`,
`70 steps (70 passed)`, `exit=0`, at `/proc/loadavg` `5.18 3.19 3.00`. That
includes `lease_race.feature`'s three-client race: exactly 1 tmux session for the
row, 2 launch records, and at least one client still shown "starting elsewhere".
No scenario, step wording, tag or expected count was touched.

## 6. What is NOT claimed

- **No whole-suite run and no stability run was taken this iteration.** Those are
  tasks 032 and 033, at the final code commit, and this commit is a code commit
  that changes `internal/store` and `internal/service` — so the run task 032
  publishes must be taken at or after it.
- **A watch item for task 032/033, stated in advance rather than after a red
  run.** `lease_race.feature` asserts *at least one* of three racing clients is
  shown "starting elsewhere". A loser now only sees that message while the
  winner's launch is genuinely in flight; a loser arriving after the winner's
  release sees the row's own verdict instead (`starting`, via
  `LaunchLeaseNotLeasable` — the row is no longer `stopped`, so no second launch
  can happen either way, which is why exactly-one-launch is not at risk). The
  three clients press within milliseconds of each other and the flight window
  spans a real `tmux new-session`, so all three land inside it; it passed here.
  If that scenario ever reports "no client showed starting elsewhere", this is
  the mechanism to look at first — not a host-load note.
- The release is best-effort by design: if the `UPDATE` fails, the launch verdict
  is unchanged and the row falls back to expiring on the TTL, exactly as before
  R75. `launch_lease.release_failed` in the audit log is the only trace, and
  nothing in this report claims that path has been exercised.
- `reconcile.go`'s "terminal row + live pane" invariant detector remains out of
  scope (follow-up, carried from task 029/030).
