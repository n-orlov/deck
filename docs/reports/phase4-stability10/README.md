# Stability sweep: ten runs at the tail code sha

- **Tail code sha**: `db669658ce20de10ef6aaad311c94f6830538436` (`db66965`), named in
  `docs/reports/phase4-final-suite/README.md` (task 039). Unchanged since that gate: `git diff
  --stat db66965 HEAD -- '*.go' '*.feature'` prints nothing at commit time.
- **Command**: `ci/stability.sh 10`, which runs `ci/run.sh go test -p=1 -count=1 ./...` (every
  package, no `-run` filter, no package list, default godog tag filter `~@real-agents &&
  ~@nightly`) ten times from a clean state (`-count=1` disables the test cache; each run gets its
  own `--rm` sibling container), reading each run's real `go test` exit status (not a piped
  `tee` status — see the script's own header comment on the mislabelled-PASS defect it exists to
  avoid).
- **Wall-clock**: ~74 minutes total (script started 20:10:xx UTC, finished 21:24:xx UTC,
  2026-09-16), each run ~6m10s–6m30s.

## Result: 7/10 passed

| Run | Result | Log |
| --- | --- | --- |
| 1 | PASS | `run-1.log` |
| 2 | PASS | `run-2.log` |
| 3 | PASS | `run-3.log` |
| 4 | **FAIL** | `run-4.log` |
| 5 | PASS | `run-5.log` |
| 6 | PASS | `run-6.log` |
| 7 | PASS | `run-7.log` |
| 8 | **FAIL** | `run-8.log` |
| 9 | **FAIL** | `run-9.log` |
| 10 | PASS | `run-10.log` |

`summary.log` is the combined log the script itself accumulates (all ten runs' output plus its
own `=== RUN N ===` / `=== RUN N: PASS|FAIL (exit S) ===` markers and the final `7/10 passed`
line), kept alongside the per-run logs for cross-reference.

## Every failure named

All three failures are the **same known-open flake class** — advisory, not a new finding:

- **Run 4** (`run-4.log`): `--- FAIL: TestGoldenMinimumFrame (2.09s)` in
  `github.com/n-orlov/deck/features`, both `TestGoldenMinimumFrame/run-1` and
  `TestGoldenMinimumFrame/run-2` subtests report `"frame kept changing after the fixture
  rendered; not settled"` (`features/golden_frame_test.go:74`), with teardown then reporting
  `"surviving deck client: signal: killed"` (`golden_frame_test.go:114`) as a consequence of the
  test failing before its own cleanup path ran. `FAIL github.com/n-orlov/deck/features 377.397s`.
- **Run 8** (`run-8.log`): the same `TestGoldenMinimumFrame` failure, this time only the
  `run-2` subtest (`golden_frame_test.go:74`, `not settled`). `FAIL
  github.com/n-orlov/deck/features 379.482s`.
- **Run 9** (`run-9.log`): the same `TestGoldenMinimumFrame` failure, again only the `run-2`
  subtest (`golden_frame_test.go:74`, `not settled`). `FAIL github.com/n-orlov/deck/features
  378.760s`.

**This is a recurrence of the known-open "transient-`starting` assertion" flake class, marked
advisory per this task's own criteria.** `golden_frame_test.go`'s own doc comments (around line
190) state explicitly that the golden frame settles with the session row reading `"starting"`
(the fake-claude fixture's silent mode never emits a probe marker or hook call, so nothing ever
samples a different status) and that the test's settle-check is a "frame kept changing... not
settled" race between the client's resize reflow and the preview/reconcile ticks (250ms/500ms)
repainting on their own schedule — the same mechanism the test's own history (task 210) already
documents as a quiescence race, not a product regression. No other test, scenario or package
failed in any of the ten runs — in particular, **no occurrence of the other known-open flake
class (the SIGWINCH exact-count assertion, `features/sigwinch_count_test.go`'s
`TestSigwinchCountDistinguishesTwoFromThree`) in this sweep.**

Every other package (`cmd/deck`, `cmd/fake-claude`, `cmd/fake-codex`, `cmd/fake-pi`,
`internal/agent`, `internal/audit`, `internal/config`, `internal/hookrecv`,
`internal/interactive`, `internal/service`, `internal/store`, `internal/theme`, `internal/tmux`,
`internal/tui`; `internal/notify`/`internal/search`/`internal/unit` have no test files) reports
`ok` in every one of the ten runs, including runs 4, 8 and 9 — the failure is isolated to
`TestGoldenMinimumFrame` in the `features` package.

## Advisory, not a blocker

Per this task's own criteria, a recurrence of either named known-open flake class is disclosed
here as advisory: it does not reopen task 039/027 (both already green at this same sha) and does
not block this run's completion. This sweep's own headline is 7/10, stated plainly above the
fold rather than rounded up or buried.
