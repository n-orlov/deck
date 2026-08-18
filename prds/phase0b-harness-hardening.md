# Phase 0b — harness determinism and evidence

## Goal

Close the two defects that kept Phase 0 from being signed off. Phase 0's code is complete and
its 23 requirements are implemented; what is missing is a suite that is *trustworthy* and a
report whose evidence *persists*.

This is a small, surgical phase. Do not add features, do not refactor, do not touch anything
outside what the requirements below name.

`SPEC.md` is the authoritative product spec and **must not be modified**. The Phase 0 PRD
(`prds/phase0-harness-and-skeleton.md`) still defines requirements R1–R23 and is also
read-only; this PRD only repairs how two of them are met.

## Context

All build and test work happens in the sibling toolchain container — the job container has
no Go and no tmux and cannot install them:

```sh
ci/run.sh go build ./...
ci/run.sh go test ./...
```

Phase 0's own reports are at `docs/reports/phase0.md` and `docs/reports/phase0-findings.md`.
Read both before starting.

## Requirements

### 1. Eliminate the fake-agent timing race

`features/fake_agent.feature`'s success scenario fails roughly **one run in three** with
`fake Claude pane "deck_fake-agent-success" does not contain "Fake Claude Code"`.

The cause is a fixed-duration hold, not a slow machine. `features/fake_agent_feature_test.go`
launches the fixture with `FAKE_CLAUDE_HOLD_MS=1000` (`fakeAgentHold`) and then has to land
`capture-pane` inside that one-second window. The success fixture exits 0, and the session is
created under `remain-on-exit failed`, which retains only *failed* panes — so once the hold
elapses the pane is destroyed and the capture returns text that cannot match.

1. Make the success-pane observation **deterministic: no test may depend on winning a race
   against a fixed sleep.** Either retain the pane after a clean exit so its output is still
   readable when the capture happens, or poll the capture until the expected content appears
   or a generous timeout expires. Do not simply increase `FAKE_CLAUDE_HOLD_MS` — a longer
   sleep is the same defect with a wider window, and it slows every run.
2. Keep the fidelity rules of the Phase 0 PRD intact. The fixture must not gain a deck-visible
   back channel, and the observation must still go through the public tmux CLI and real pane
   text. If `FAKE_CLAUDE_HOLD_MS` is no longer needed, remove it from the fixture and its help
   text rather than leaving a dead knob.
3. If retaining dead panes requires a different `remain-on-exit` setting for the *fixture's own*
   session, that is acceptable and must be commented as a fixture-local choice that does not
   describe deck's own tmux contract (`SPEC.md` §3.2 still governs deck).

### 2. Prove the suite is stable, not merely green twice

Phase 0's R23 ("passes twice") was satisfied while a one-in-three flake was present. Two
passes cannot establish stability.

4. `ci/run.sh go test ./... -count=1` passes **ten consecutive times from a clean state**.
   Record the real, unedited output of the loop that demonstrates this — including a visible
   per-run pass indication and a final count — in the report of R8 below. A summary sentence
   is not evidence.
5. If any run in that sequence fails, the flake is not fixed: diagnose and repeat the ten-run
   sequence from the beginning. Report the number of full sequences attempted.

### 3. Make the evidence report correct and self-contained

6. **Persist the evidence inside the repository.** `docs/reports/phase0.md` currently links
   every capture to `/run/ralphd/artifacts/...`, a path in the ephemeral job run directory;
   those links die when the run directory is cleaned. Copy the captures the report relies on
   into a committed directory under `docs/reports/` and reference them by repository-relative
   path. Do not commit anything else.
7. **State a unit-test count under a defined convention.** The report says "75 countable
   `=== RUN Test…` results", which conflates top-level Go tests, Godog scenario subtests and
   nested subtests, and is therefore not a unit-test count. State the counting convention
   explicitly, then give the number produced by a command whose real output is recorded. For
   reference, `grep -cE '^=== RUN   Test[A-Za-z0-9_]+$'` over a verbose run yields **42**
   top-level Go tests; verify this yourself rather than trusting it, and distinguish the
   Godog runner test from genuine unit tests.
8. **Add the gotchas section R22 requires.** It must record every gotcha actually discovered
   in Phase 0 and 0b, each with its consequence. At minimum: Bubble Tea's OSC 11 / CPR
   start-up probes blocking frame one; `sh -c` rather than `sh -lc` in the sibling (a login
   shell resets `PATH` and Go vanishes); the SQLite initialisation ordering needed before a
   second client starts; `capture-pane` preserving terminal soft-wraps so long lines must have
   newlines stripped before matching; and the fake-agent hold race fixed by requirement 1.
9. **Fix the stale R23 reference.** The R23 row cites cached `go-test-first.txt` /
   `go-test-second.txt` captures that predate the corrective work. Point it at the ten-run
   evidence from requirement 4 instead.
10. Every claim in `docs/reports/phase0.md` must be true of the tree as delivered. Re-check
    the requirement-coverage table against the current code and correct any row that no longer
    holds; say in the report that you re-checked it.

### 4. Findings

11. Record anything you had to decide or invent in `docs/reports/phase0-findings.md`, in the
    style already established there. If you conclude a requirement above is wrong or
    impossible, say so there with evidence rather than silently doing something else.

## Non-goals

Everything in Phase 0's non-goals list still applies. In particular: no agent adapters, no
`deck _hook` or status detection, no notifications, no search, no preview pane, no permission
profiles, no user-facing command line. Do not implement Phase 1 work. Do not "improve" the
TUI, the store, or the tmux layer beyond what requirement 1 needs.

## Constraints

- **Do not run `git commit`, `git push`, or any other git write command.** Leave everything in
  the working tree for operator review.
- Do not modify `SPEC.md`, anything under `prds/`, `ci/Dockerfile`, or `ci/SPIKE.md`.
- Do all building and testing in siblings via `ci/run.sh`. Install nothing into the job
  container.
- No network dependency in the default test suite.
- Every sibling container is `--rm`; leave no stray containers or volumes beyond the shared
  cache.
- Do not weaken a test to make it pass. Deleting, skipping or tag-excluding the flaky scenario
  is an explicit failure of this PRD.
