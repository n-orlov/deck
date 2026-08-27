# Task 029 — R74 leg 1: a per-launch generation discriminator, injected into the agent environment

Issue #11 (R74), phase 3f approach 02. Leg 1 only: mint the discriminator and get
it into the launched agent's environment. Dropping superseded hook writes is leg 2
(task 030); releasing the lease on a completed launch is R75 (task 031).

## 1. What the discriminator is, and where it lives

`AcquireLaunchLease` (`internal/store/lease.go`) now mints a **per-launch
generation token** on every successful acquisition: 8 bytes of `crypto/rand`,
hex-encoded (`newLaunchGeneration`). It is returned as
`LaunchLeaseResult.LaunchGeneration` and persisted as the second half of the
existing `launch_lease_owner` column, joined by `#`:

    launch_lease_owner = "<pid>@<boot_id>#<16 hex chars>"

Why random rather than a timestamp or a counter: deck's clock is injectable and a
test clock does not advance at all, so two launches of one row can share a clock
reading — and a discriminator that two launches can share is not a discriminator.
The store test proves this by acquiring **twice with the same owner string and the
same `at` value** and demanding different tokens.

The identity half is untouched, and the liveness path strips the suffix before
parsing (`splitOwnerGeneration`, called from `parseLeaseOwner`). That strip is
load-bearing, not cosmetic: left on, the suffix lands in the boot-id component,
every live lease compares as "from a previous boot", and §9.3's double-launch
guard silently stops holding — see revert B in §5.

## 2. Spec note: a change REQUEST for the owner-format sentence, and no new column

**No sessions column was added.** `SPEC.md:243-284` pins the `sessions` DDL and
`SPEC.md` is protected, so a `launch_generation` column of its own was rejected in
favour of the existing `launch_lease_owner`/`launch_lease_until` pair the task text
names. `git diff --name-only` for this commit shows no `SPEC.md`, no `prds/`, no
`ci/Dockerfile` and no `ci/SPIKE.md`.

The pair alone was **not** sufficient, which is exactly why the owner string now
carries a suffix: `launch_lease_owner` was `pid@boot_id`, identical for two
successive launches of one row from the same deck process (same pid, same boot),
and `launch_lease_until` is a timestamp that two launches can share under a frozen
clock and that R75 is about to clear. Neither can answer "which launch is the
current one", which is the only question a late hook write needs answered.

**Spec-change REQUEST (not applied here — `SPEC.md` is protected):** §9.3's
sentence at `SPEC.md:781` and the DDL comment at `SPEC.md:277` describe the owner
as `pid@boot_id`. R74 extends that value to `pid@boot_id#<generation>` while
leaving the identity semantics unchanged. Request: reword both to
`pid@boot_id[#launch_generation]` and state that comparisons of launcher identity
ignore the suffix. Until that request is accepted, the divergence is recorded here
and in the code comments at `internal/store/lease.go` rather than edited into the
spec.

## 3. Wiring: lease → LaunchInput → pane environment

- `internal/agent/agent.go`: `LaunchInput.LaunchGeneration` (new field).
- `internal/agent/claude.go`: `Instrument` exports `DECK_LAUNCH_GENERATION`
  beside `DECK_SESSION_ID`, and **omits the key entirely when the token is
  empty** — an absent variable honestly says "this launch has no token", while
  `DECK_LAUNCH_GENERATION=""` would look like a token that failed to match.
- `internal/service/resume.go`: passes `lease.LaunchGeneration` from the
  acquisition it just won into the `LaunchInput`, so the value in the pane is by
  construction the value in the row.
- `internal/service/agent.go`: create passes no generation, with the reason in a
  comment — a brand-new row's first launch takes no launch lease at all, so no
  earlier launch of it exists to be confused with. The row's first generation is
  written by the `AcquireLaunchLease` of its first resume/restart. Leg 2 therefore
  has to treat "hook carries no token, row has one" as superseded and "row has no
  token" as nothing to discriminate.
- `internal/agent/shell.go` and `internal/agent/pi.go` are untouched: their
  `Instrument` returns no env, so shell and pi panes gain nothing.

## 4. Tests and what each one pins

`internal/store/launch_generation_test.go`
- `TestAcquireLaunchLeaseMintsAFreshGenerationPerLaunch` — two acquisitions of one
  row with the **same owner and the same `at`**: both tokens non-empty, different
  from each other, neither containing the launcher identity nor the acquisition
  timestamp, `launch_lease_owner` equal to `owner#token` after each, and the
  `launch_lease_acquired` event payload naming the second token.
- `TestLiveOwnerWithGenerationIsStillNotBreakable` — a stopped row whose lease is
  held, in-TTL, by *this live process* with a generation suffix stays
  `LaunchLeaseHeldElsewhere` and untouched; a losing result reports no generation.
- `TestSplitOwnerGenerationHandlesPreR74Owners` — a pre-R74 owner with no suffix
  still parses as pure identity, and `parseLeaseOwner` returns `boot-b`, not
  `boot-b#ff00`, for the new shape.

`internal/service/launch_generation_test.go` (real tmux, values read back out of
tmux itself via `show-environment`, which is the environment the agent's hooks
inherit)
- `TestResumeExportsCurrentLaunchGenerationToThePane` — the create-launch pane has
  `DECK_SESSION_ID` and **no** `DECK_LAUNCH_GENERATION`; two successive resumes each
  export a token *equal to the row's current token*, and the two tokens differ.
- `TestResumeShellSessionCarriesNoLaunchGeneration` — a resumed shell row does get
  a generation in the row (its resume takes the lease) but its pane has neither
  `DECK_LAUNCH_GENERATION` nor `DECK_SESSION_ID`, and no launch audit record
  carries a `DECK_`-prefixed key.

Between the two resumes the test ages the lease out of its TTL
(`expireLaunchLease` sets `launch_lease_until = 0`, owner kept). That stands in for
the wall-clock 30 s a frozen test clock never delivers, and it is deliberately the
shape R75 will produce: lease over, row still naming the current launch. It is
also the reason R75 must clear the *deadline*, not the owner — clearing the owner
would destroy R74's discriminator.

## 5. Red-on-revert evidence

Each revert was applied to the working tree, the tests were run, and the file was
restored (`git status --short` clean afterwards, verified before committing).

| revert | change made | result |
| --- | --- | --- |
| A | `storedOwner := composeLeaseOwner(owner, generation)` → `storedOwner := owner` (token minted but never persisted) | [revert-a-no-generation-persisted.log](./revert-a-no-generation-persisted.log) — store test fails on `stored launch_lease_owner = "12345@boot-a"; want "12345@boot-a#eb5e…"`, both service tests fail on `carries no generation` |
| B | drop the `splitOwnerGeneration` strip from `parseLeaseOwner` | [revert-b-identity-parse-keeps-suffix.log](./revert-b-identity-parse-keeps-suffix.log) — `TestLiveOwnerWithGenerationIsStillNotBreakable` fails with `outcome = 0` (the live lease was **broken**: the §9.3 guard is off), plus the parse assertion |
| C | drop the `DECK_LAUNCH_GENERATION` export from `Claude.Instrument` | [revert-c-no-env-injection.log](./revert-c-no-env-injection.log) — `TestResumeExportsCurrentLaunchGenerationToThePane` fails with tmux's own `unknown variable: DECK_LAUNCH_GENERATION` |

Revert B is the one worth reading twice: the naive owner-format change passes every
generation assertion while quietly disabling the double-launch guard, and only that
test catches it.

## 6. Green run

    ci/run.sh go test -count=1 ./internal/store/ ./internal/agent/ ./internal/service/

exit 0, all three `ok`:
[green-store-agent-service.log](./green-store-agent-service.log) (2026-08-27T00:15:57Z,
`/proc/loadavg` 4.78 before / 4.64 after, 1-minute figure — host load is a known
confound for timing claims, not for pass/fail).

`internal/hookrecv/` also runs green ([hookrecv.log](./hookrecv.log)); it is the
package leg 2 will change and it exercises `AcquireLaunchLease` in
`internal/hookrecv/resumed_crash_verdict_test.go`. The whole-suite run at the final
code commit is task 032's deliverable and is not claimed here.

Starting point for this task: `2b39124` (task 028, `origin/main`).
