# Stability sweep: ten runs at the corrected tail code sha (task 011b)

- **Tail code sha**: `e93a7902582672d799dadfa8bbaa4f1f25eb28dd` (`e93a790`,
  task 011) — the same tail code sha named in
  `docs/reports/phase4-cure-final-suite/README.md` and
  `docs/reports/phase4-cure-guards/README.md`. Unchanged at commit time:
  `git diff --stat e93a790 HEAD -- '*.go' '*.feature'` prints nothing.
  `HEAD` at commit time is `1f38195` (task 011b's golden-fixture
  regeneration) plus this report commit — neither touches a `*.go`/
  `*.feature` file.
- **Command**: `ci/stability.sh 10`, which runs `ci/run.sh go test -p=1
  -count=1 ./...` (every package, no `-run` filter, no package list,
  default godog tag filter `~@real-agents && ~@nightly`) ten times from a
  clean state (`-count=1` disables the test cache; each run gets its own
  `--rm` sibling container), reading each run's real `go test` exit status
  (not a piped `tee` status — see the script's own header comment on the
  mislabelled-PASS defect it exists to avoid).
- **Wall-clock**: 1h19m53s total (`2026-09-17T00:27:12Z` to
  `2026-09-17T01:47:05Z`), each run ~6m10s–6m30s — consistent with the
  standing rules' ~75 min estimate and approach 1's `phase4-stability10`
  measurement at `db66965`.

## Result: 7/10 passed

| Run | Result | Log |
| --- | --- | --- |
| 1 | PASS | `run-1.log` |
| 2 | **FAIL** | `run-2.log` |
| 3 | **FAIL** | `run-3.log` |
| 4 | PASS | `run-4.log` |
| 5 | PASS | `run-5.log` |
| 6 | PASS | `run-6.log` |
| 7 | PASS | `run-7.log` |
| 8 | PASS | `run-8.log` |
| 9 | PASS | `run-9.log` |
| 10 | **FAIL** | `run-10.log` |

`summary.log` is the combined log the script itself accumulates (all ten
runs' output plus its own `=== RUN N ===` / `=== RUN N: PASS|FAIL (exit
S)===` markers and the final `7/10 passed` line), kept alongside the
per-run logs for cross-reference.

## Every failure named

**Runs 2 and 3 are the known-open transient-`starting`-assertion flake
class**, unchanged from every prior sweep in this repository's history
(`docs/reports/phase4-stability10/README.md` and earlier phases) — advisory,
not a new finding:

- **Run 2** (`run-2.log`): `--- FAIL: TestGoldenMinimumFrame` /
  `TestGoldenMinimumFrame/run-2`, `golden_frame_test.go:74`, `"frame kept
  changing after the fixture rendered; not settled"`. `FAIL
  github.com/n-orlov/deck/features 378.262s`.
- **Run 3** (`run-3.log`): the same failure, `TestGoldenMinimumFrame/run-1`
  this time. `FAIL github.com/n-orlov/deck/features 372.793s`.

Both are the documented settle-race between the client's resize reflow and
the preview/reconcile ticks (250ms/500ms) — a pre-existing, load-sensitive
quiescence race in the test's own settle-check, not a product regression,
and not touched by task 011b's golden-fixture regeneration. In both cases
the failure is the *settle-check* itself, never a byte mismatch against the
regenerated golden — the fixture fix holds under this flake class exactly
as it held in every clean run.

**Run 10 is a new, previously undisclosed flake, not the golden-frame
class**:

- **Run 10** (`run-10.log`): `features` package is fully green
  (`ok github.com/n-orlov/deck/features 379.245s`, including
  `TestGoldenMinimumFrame` — the fixture fix holds). The one failure is in
  a different package:

  ```
  --- FAIL: TestSendKeysInvalidHexByteIsSilentlyDiscarded (0.02s)
      literal_send_test.go:134: pane capture = "", want only the bare
      prompt (nothing delivered) -- PRD II-38
  FAIL
  FAIL	github.com/n-orlov/deck/internal/tmux	19.804s
  ```

  `TestSendKeysInvalidHexByteIsSilentlyDiscarded`
  (`internal/tmux/literal_send_test.go:123`) sends `send-keys -H zz` to a
  real tmux pane and asserts the capture is still the bare `$` prompt
  (nothing delivered). In this one run, `capture-pane` returned an empty
  string instead — a real-tmux timing read, not a byte-comparison against
  any fixture task 011b touched. Re-run in isolation three times
  immediately after discovery (`ci/run.sh go test -count=1 -run
  '^TestSendKeysInvalidHexByteIsSilentlyDiscarded$' -v ./internal/tmux/`),
  it passed all three times — not reproducible on demand, consistent with
  a load-sensitive real-tmux capture race surfacing once across 10×18
  package runs (this sweep's own ~70 minutes of prior sequential sibling
  containers on the same host by the time run 10 started).

  FINDING: internal/tmux/literal_send_test.go:123
  TestSendKeysInvalidHexByteIsSilentlyDiscarded read an empty
  `capture-pane` once in run 10/10 of this stability sweep (not
  reproduced in three immediate isolated re-runs) — a new, previously
  undisclosed real-tmux timing flake, load-sensitive, disclosed here as
  advisory per this task's own criteria; not a product regression and
  not caused by task 011b's golden-fixture change (unrelated package, no
  test in this file reads the golden fixture).

No other test, scenario or package failed in any of the ten runs. In
particular: no occurrence of the other previously named known-open flake
class (`features/sigwinch_count_test.go`'s
`TestSigwinchCountDistinguishesTwoFromThree`) in this sweep, and the
golden-fixture fix (task 011b) held in every run that reached
`TestGoldenMinimumFrame`'s byte comparison (all ten — the two golden
failures above are both the settle-check, never the byte compare).

Every other package (`cmd/deck`, `cmd/fake-claude`, `cmd/fake-codex`,
`cmd/fake-pi`, `internal/agent`, `internal/audit`, `internal/config`,
`internal/hookrecv`, `internal/interactive`, `internal/service`,
`internal/store`, `internal/theme`, `internal/tui`; `internal/notify`/
`internal/search`/`internal/unit` have no test files) reports `ok` in
every one of the ten runs, including runs 2, 3 and 10 (`internal/tmux`
itself is `ok` in the other nine runs).

## Advisory, not a blocker

Per this task's own criteria and the standing rules' precedent for the two
already-known flake classes, this new one is disclosed here as advisory in
exactly the same shape: it does not reopen task 011/011b (the golden fix is
green in every run's byte-compare) and does not block this run's
completion. This sweep's own headline is 7/10, stated plainly above the
fold rather than rounded up or buried.
