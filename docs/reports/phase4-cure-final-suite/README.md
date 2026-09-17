# Full-suite gate sweep — approach 2 (cure-and-reverify)

- **Tail code sha**: `e93a7902582672d799dadfa8bbaa4f1f25eb28dd` (`e93a790`,
  `tui: sidebar row's permission badge follows SPEC §11's non-safe rule
  (task 011)`) — the most recent commit in history touching a `*.go` or
  `*.feature` file at the time this sweep ran. Task 012 (`425c9dc`) landed
  after it but is docs-only, so it does not move the tail:
  `git diff --stat e93a790 425c9dc -- '*.go' '*.feature'` prints nothing.
- **Command as run**:
  `ci/run.sh sh -c 'go test -p=1 -count=1 -timeout=40m ./...'` (the
  `-timeout` bounds the whole invocation per the standing rules' launch
  guidance; it narrows nothing — every package ran, no `-run` filter, no
  package list).
- **Skips in force**:
  - godog's default tag filter, `~@real-agents && ~@nightly` (no
    `DECK_GODOG_TAGS` opt-in), excluding `@real-agents`/`@nightly`
    scenarios.
  - The real-binary skips baked into the fixture PATH/agent probes for
    codex, claude and pi: scenarios and probes needing a genuine `codex`,
    `claude` or `pi` executable are satisfied here by the
    `cmd/fake-codex`/`cmd/fake-claude`/`cmd/fake-pi` fixtures — no real
    agent binaries exist in this container.
- **Wall-clock duration**: 7m16s (436s), from `2026-09-16T23:46:32Z` to
  `2026-09-16T23:53:48Z` (`date -u` markers wrapping the command, captured
  in `suite.log`) — close to the ≈7m expected / 421s measured at `db66965`
  in `docs/reports/phase4-final-suite/README.md`.
- **Full output**: committed verbatim at
  `docs/reports/phase4-cure-final-suite/suite.log` (this is the third of
  three consecutive attempts at this same sha; the first two are
  summarised below under "Not a flake" — all three hit the identical
  deterministic mismatch, so the third's log is representative and is the
  one committed).

## Result: FAIL — one real, deterministic regression found

Per-package result (`go list ./...` at this sha lists 18 packages):

| Package | Result |
| --- | --- |
| `cmd/deck` | ok (7.6s) |
| `cmd/fake-claude` | ok (0.8s) |
| `cmd/fake-codex` | ok (0.1s) |
| `cmd/fake-pi` | ok (0.8s) |
| `features` | **FAIL** (374.1s) |
| `internal/agent` | ok (0.01s) |
| `internal/audit` | ok (0.02s) |
| `internal/config` | ok (0.03s) |
| `internal/hookrecv` | ok (4.4s) |
| `internal/interactive` | ok (11.3s) |
| `internal/notify` | `[no test files]` |
| `internal/search` | `[no test files]` |
| `internal/service` | ok (7.1s) |
| `internal/store` | ok (2.6s) |
| `internal/theme` | ok (0.01s) |
| `internal/tmux` | ok (19.8s) |
| `internal/tui` | ok (3.9s) |
| `internal/unit` | `[no test files]` |

Within the `features` package: godog's own `TestFeatures` run is fully
green — **352 scenarios (352 passed), 4168 steps (4168 passed)**. The two
"1 failed" / "1 undefined" lines in `suite.log` belong to
`TestGodogRejectsUndefinedAndFailedSteps`, a godog-harness self-test that
*deliberately* runs a failing and an undefined scenario in a throwaway
temp fixture to prove godog's own reporting — not a real scenario failure.

The one genuine failure is a plain Go test, not a godog scenario:
`--- FAIL: TestGoldenMinimumFrame` (`features/golden_frame_test.go`), both
`run-1` and `run-2` subtests, in every one of the three attempts run at
this sha.

## Root cause: task 011 regressed the checked-in golden frame, not a flake

`TestGoldenMinimumFrame` compares a rendered frame byte-for-byte against
`features/testdata/golden/side_by_side_80x24.golden`. That golden file's
line 5 still reads:

```
|   just now [safe]                |                                           |
```

Task 011 (`e93a790`) correctly stopped the sidebar row from rendering a
`[safe]` badge for a `safe`-profile session, per SPEC.md:1339 (badge only
for non-`safe` profiles) — but nothing in task 011 regenerated this golden
fixture, which still expects the pre-011 `[safe]` badge. Every rendered
frame now reads `|   just now                       |` (no badge), so the
byte-for-byte comparison fails deterministically:

```
golden_frame_test.go:91: rendered frame does not match golden testdata/golden/side_by_side_80x24.golden
    --- got ---
    ...
    |   just now                       |
    ...
    --- want ---
    ...
    |   just now [safe]                |
    ...
```

**This is a genuine, deterministic regression, not the known-open
transient-`starting` settle-race flake** (`golden_frame_test.go:74`,
`"frame kept changing after the fixture rendered; not settled"`,
disclosed as advisory in `docs/reports/phase4-stability10/README.md`).
The distinction matters and was checked directly: this sweep was run
**three times** at the identical sha before this report was written,
specifically to separate a flake from a real fault:

1. Attempt 1 (`23:26:02Z`–`23:33:28Z`): both `run-1` and `run-2` failed on
   the golden mismatch (`does not match golden`).
2. Attempt 2 (`23:37:05Z`–`23:44:21Z`): `run-1` hit the *other*, known-open
   settle-race flake (`"not settled"`) — load-sensitive, consistent with
   its documented history — while `run-2` again failed on the golden
   mismatch.
3. Attempt 3 (`23:46:32Z`–`23:53:48Z`, committed as `suite.log`): both
   `run-1` and `run-2` failed on the golden mismatch again.

The golden-mismatch failure is 100% reproducible across all three
attempts and six subtest executions; the settle-race flake surfaced once,
consistent with it being separate, pre-existing, load-sensitive noise
riding on top of the real fault, not the cause of it.

## Disposition

Per the standing rules ("a sweep is never narrowed... if a lane is red and
the fix is code, the fix is a NEW task... and 013/014/015 are re-run from
scratch afterwards, never re-run under the task that found the red lane"),
this task's own job is to run the sweep once, unnarrowed, and record
exactly what it found — which this report does. The fix (regenerating
`features/testdata/golden/side_by_side_80x24.golden` via the test's own
documented `UPDATE_GOLDEN=1` regeneration path, reviewing the diff, and
then re-running the gate/guards/stability sweeps fresh at the corrected
tail sha, overwriting this directory and its guard/stability counterparts
with the clean results) is out of this task's scope — task 013 is
record-only past the freeze line and may not touch `features/testdata/`.
That fix is proposed as a new task; this report and `suite.log` stand as
the honest record of the first sweep attempt, which found it.
