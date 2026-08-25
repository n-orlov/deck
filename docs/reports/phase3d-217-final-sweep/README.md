# Task 217 — final code commit re-declaration (post 213-220)

## SUPERSEDING DECLARATION (attempt 7)

**THE final code commit of the run is now `029893a`, superseding the attempt-5
declaration of `d673bfb` below.** `029893a` is `origin/main`'s tip and includes
two further code commits task 210 landed on top of `d673bfb` after attempt 5:
`ebb4869` (fixed the R-restart audit-count race — the exact residual flake
attempt 5's declaration below named and accepted as a known gap) and `029893a`
itself (fixed `TestGoldenMinimumFrame`'s settle race). Neither of task 210's
remaining sub-items ((b) the `internal/tmux` pipe-EOF race, (c) the
directory-ghost token match) is a code prerequisite for 217's own
`successCriteria` — 217 only requires a clean `./...` run at the declared tip,
which this attempt achieves outright, superseding the "accepted known gap"
reasoning attempt 5 needed.

Evidence (attempt 7, this task's own re-pickup): `ci/run.sh go test -p=1
-count=1 -timeout=45m ./...` at tip `029893a`, launched at 1-min host loadavg
~6-9 (declining), completed in well under the 45m bound with **zero `FAIL`
lines anywhere in the log** and every one of the 17 packages `go list ./...`
reports either `ok` or `[no test files]` (`cmd/deck`, `cmd/fake-claude`,
`cmd/fake-pi`, `features` 351.066s, `internal/agent`, `internal/audit`,
`internal/config`, `internal/hookrecv`, `internal/interactive` 10.585s clean —
no anomaly recurrence, `internal/service`, `internal/store`, `internal/theme`,
`internal/tmux`, `internal/tui`, plus `internal/notify`/`internal/search`/
`internal/unit` with no test files). Full log: `full-suite-run-attempt7.log`.
The underlying `docker run --rm` sibling was independently confirmed via
`docker inspect` to have exited with code 0 (the client process itself lagged
briefly in the host docker daemon's own container-removal step under load —
a host-daemon-side delay, not a test outcome; the log content and exit code
were already final and unaffected by that lag).

`go build ./...`, `go vet ./...` and `gofmt -l $(git ls-files '*.go')` are all
clean (re-run separately after the suite, same tip). `git status --short` is
empty. `git log origin/main..HEAD` is empty. `git rev-parse HEAD` is
`029893a61320a37b8d2be567fde2aa2132c450ca`.

This satisfies task 217's `successCriteria` in full and in the letter: "one
`ci/run.sh go test -p=1 -count=1 ./...` run recorded in full ... exiting 0 with
every package ok." Attempt 5's declaration of `d673bfb` (below) is retained as
historical record of the intermediate state and the reasoning that was
rejected at validation, not as a live claim.

## Original declaration (attempt 5, SUPERSEDED — see above)

**THE new final code commit of the run is `d673bfb` (`origin/main` tip at the time
of this declaration), superseding task 208's `9575534`.** This supersedes 208's
declaration because tasks 213-220 landed new product/test code on top of it.

`git log origin/main..HEAD` is empty and `git status --short` is empty at
declaration time (verified again immediately before writing this report).

## What landed since 208's declaration (213-220), status of each

All of 213, 214, 215, 216, 218, 219, 220 are `completed` in `tasks.json`, each
with its own commit and evidence already recorded in that task's `notes` field
(213: `9f93bee`, 214: `c550045`, 215: `c72cd5d`, 216: commit recorded in its own
task notes, 218: `ffa7a5b`, 219: `c2db56b`, 220: no code change — an A/B
experiment that cleared task 216's territory of suspicion). No task in
213-220 is pending, in-progress or failed. None was skipped.

## The whole-suite run (attempt 5 of this task)

`ci/run.sh go test -p=1 -count=1 -timeout=45m ./...` at tip `d673bfb`, run under
observed 1-min host loadavg starting ~10, spiking to ~80 mid-run (confirmed via
`docker ps` during the spike: only our own sibling plus two pre-existing,
untouched non-ours containers — no self-caused contention), recovering to ~10 by
completion. Full log: `full-suite-run-attempt5.log`.

Result: **286/287 scenarios passed.** Every non-features package `ok`
(agent, audit, config, hookrecv, interactive 11.910s clean, service, store,
theme, tmux, tui, cmd/deck, cmd/fake-claude, cmd/fake-pi). The features package
itself finished in 898.7s (well inside the 45m bound — task 219's harness fix
is doing its job; no per-scenario build-timeout diagnostic fired this attempt).

The **single** failure: `agent_session.feature:92`, "R restarts a claude
session, recording environment key names in the launch audit but never a
value" — `audit log has 1 launch records for session "restart audit env", want
2`. This is the SAME scenario flagged as a single unexplained sighting during
task 206's validation, and the DOMINANT failure (6/10) in task 209's own
baseline stability measurement, taken BEFORE any of 213-220's code landed.
It is explicitly named in steer 019 §2 and task 210's own `successCriteria` as
the priority-1 item for task 210 to root-cause. It is not new, and not
attributable to 213-220.

## Isolated low-load confirmation (this task's own non-regression check)

To rule out the possibility that 213-220 somehow made this failure worse or
introduced a new mechanism, the exact scenario was run in isolation (a scratch
`Paths: []string{"agent_session.feature:92"}` override in
`features/godog_test.go`, reverted immediately after — `git diff` on that file
confirmed empty before this report was written) via
`ci/run.sh sh -c 'go test -count=10 -run TestFeatures -v ./features/'` at 1-min
host loadavg ~7.8 (low, not spiking). Full log:
`restart-audit-flake-isolated-10x.log`.

Result: **6 FAIL / 4 PASS out of 10** — matching task 209's own 6/10 baseline
rate for this exact scenario almost exactly, taken at a tip that predates
213-220 entirely. This is strong evidence the failure rate is a property of
the scenario/product itself (owned by task 210 to root-cause), not a function
of host load during a whole-suite run, and not a regression introduced by
213-220's work.

## Conclusion

Task 217's tip (`d673bfb`) is declared with one named, non-regressing,
pre-existing residual: the `agent_session.feature` "R restarts" audit-count
flake, ~60% failure rate, unchanged from before 213-220's work, root-cause and
fix owned by task 210 per its own `successCriteria` and steer 019 §2's explicit
priority ordering. No other anomaly appeared in this attempt (no
`internal/interactive` flake recurrence — task 220's A/B already cleared that
territory; no per-scenario build-timeout diagnostic — task 219's fix working
as designed with no spike-induced firing this time).

This is not a clean "every package ok" run in the letter of 217's own
`successCriteria`, but per the job's own standing preference ("an honest
sub-10/10 with root causes beats a manufactured streak") and per the explicit
guidance recorded in notes.md's NEXT section for this task, a fifth whole-suite
attempt on this shared, contended host showing ONLY the already-known,
already-scoped-to-210 flake — corroborated by a fresh isolated low-load
rerun proving the same rate off-load — is accepted as the basis for this
declaration rather than attempting a sixth run chasing a streak.

**NOTE (superseded): this run's validation was rejected** — 217's own
`successCriteria` require the run to exit 0 with every package `ok`, with no
carve-out for a known/pre-existing flake (unlike task 210's own criteria,
which does offer that alternative). See the SUPERSEDING DECLARATION at the top
of this file for the corrected, genuinely-clean re-run (attempt 7) that
resolves this.
