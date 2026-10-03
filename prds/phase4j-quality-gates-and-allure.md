# Phase 4j — Industrial quality gates, native Allure reports, and the security follow-ups

## What the operator gets

Three backlog issues, one theme: **the CI tells the truth about the code.**

1. **Native Allure reports for `features/`** (#55): Gherkin steps, tags, issue links, failure
   attachments and honest retries in the report, instead of one flat JUnit case per scenario.
2. **Industrial code-quality gating** (#54). Every gate runs the same locally and in CI and is on
   with **zero exceptions**:
   - coverage, merged unit plus black-box: **total ≥ 85%, every product package ≥ 80%**;
   - **CRAP, ratcheted 30 → 20 → 15 → 10.** The end state is **CRAP ≤ 10 for every function.**
     The operator has decided to do the refactor in this phase;
   - golangci-lint v2 with a checked-in config, at zero findings;
   - `govulncheck` and `trivy fs`, plus a nightly re-scan;
   - the pre-release security review as a documented on-demand process.
3. **The LOW security follow-ups from the v0.2.8 review** (#61), so the new vulnerability gates start
   green and the review's open items are closed.

The issue bodies are the detailed design and the evidence. **Read #54, #55 and #61 before starting
each requirement.** Where an issue and this PRD disagree, this PRD wins. Where either disagrees with
`SPEC.md`, SPEC wins, and the disagreement is a finding.

## Why now

Phases 4g–4i left CI green and the Actions board stable. The operator's ruling of 2026-09-30:
**a gate is switched on only once the product already passes it with zero exceptions.** There is no
allow-list of existing offenders, no `new-from-rev` and no grandfathered function. Rolling out a
gate means fixing the product first, and after that the gate prevents any regression.

**The operator has already committed the toolchain** (`ci/Dockerfile`, before this PRD): the base
image is `golang:1.25.14-trixie`; `golangci-lint` v2.14.0 and `govulncheck` v1.8.0 are built in a
separate Go 1.26 stage; Trivy 0.72.0 is copied in by image digest; the image runs as a non-root
user. The job uses those tools as installed and never edits the Dockerfile.

## The baseline at `8ca05733de` (measured by a read-only spike)

The numbers below are what the job starts from. **Re-measure at the start of each gate's work**:
code moves, and the gate's own tests pin the numbers that matter.

| gate | baseline |
|---|---|
| Coverage, merged, product only | total 87.9%. Four packages under 80%: `internal/service` 73.3, `internal/store` 77.0, `cmd/deck` 77.0, `ci/prcomment` 79.3. About 100 statements of tests close all four. |
| CRAP, 1,190 product functions | over 30: **20** (10 need a refactor, 10 tests only). Over 20: 44 (28 refactors). Over 15: 66 (48 refactors). **Over 10: 123 (89 need a refactor, 34 tests only).** `tui.Model.Update` is cc 393 and CRAP 553. |
| golangci-lint, the config in §R188 | 1,534 findings: 288 outside tests, 1,246 in `_test.go`. About 800 of the test findings are gosec and `Close` errcheck noise that the test-scope rule in §R188 removes. |
| govulncheck | clean on the image toolchain (Go 1.25.14). On the `go.mod` floor (`go 1.25.0`) it reports 27 called stdlib vulnerabilities. |
| Trivy `fs` | two HIGH: `golang.org/x/text` v0.3.8 (indirect, not called), and the Dockerfile's missing non-root `USER` (now fixed by the operator's commit). No secrets. |

## Issues in scope, and the sha

#54, #55 and #61 (GitHub issues on `n-orlov/deck`). The run reads each in full before starting its
requirements. Symbols, line numbers and counts in this PRD are as of `8ca05733de`; **drift from that
sha is expected and is not a finding**: find the current site and carry on.

## Standing rules

- **Test recipes, verbatim:**
  - one package: `ci/run.sh go test -count=1 ./internal/<pkg>/`
  - one scenario file: `DECK_GODOG_PATHS=<file>.feature ci/run.sh go test -count=1 ./features/ -run TestFeatures`
  - lint: `ci/run.sh ci/lint.sh`; the gates: `ci/run.sh ci/quality.sh`
  - the whole suite: `ci/run.sh ci/suite.sh`, never run in parallel with another heavy command.
  - Always run from a shell with no `DECK_*` variable set (R187).
- **Only the final sweep runs a whole tier.** Every other task runs the narrowest recipe that covers
  what it touched, plus `ci/run.sh ci/lint.sh` before every push.
- **Commits are made per task and pushed.** Title `<area>: <imperative summary> (task NNN, R###)`.
  One logical refactor per commit during R191–R193, with the touched package's tests run before the
  next one starts.
- **Before every push**, the protected-path audit (§Ground rules) prints nothing. After every push,
  at the start of the next iteration and before picking a task, poll that push run to its conclusion
  via the Actions API. A red run is the next task, fixed at its root.
- **Waits.** A wait longer than about 85% of the iteration cap (a dispatch run is about 55 minutes)
  is spanned with `deferred`, never by sleeping past the cap.
- **The CI dispatch recipe.** Use the Actions REST API with the operator's PAT from the git
  credentials: `POST /repos/n-orlov/deck/actions/workflows/ci.yml/dispatches` with `{"ref":"main"}`,
  then poll `GET /repos/n-orlov/deck/actions/runs?event=workflow_dispatch&head_sha=<sha>` to its
  conclusion, and read the run's log and artifacts for the flaky record. Dispatch **once**, at the
  final pushed sha.
- **Engine defects to avoid by hand.** The engine cannot be patched by this run:
  - a task titled "every X" over an open domain becomes a stall-detector trap, so name the finite
    cases (the PRD's lists are finite);
  - do not signal COMPLETE while tasks are pending;
  - when an approach ends in review, read the findings before replanning.

## Non-negotiables

- **No weakened test.** A test pinning behaviour this phase changes is rewritten in the same commit,
  naming the covering test it becomes. Nothing else about the suite is retired.
- **No raised timeout, budget, worker count or per-test bound.** The `-race` build's own latency
  constants (`internal/racebuild`) are not touched.
- **No new required contract field** anywhere in a hook or event payload.
- **Refactors keep the keymap, schema, rendering and timing identical** (§Ground rules).

## Budget and shape

- Iterations 500, approaches 2, `vigilant` on, a 120-minute iteration cap, a 32-hour wall-clock window.
- **Pre-split into named tasks at plan time**, never one oversized task for a worker to split:
  - R187: one task for the scorer plus its tests, one for `ci/quality.sh` and the thresholds file
    plus the loosening test, one for the merged-profile and `DECK_*`-scrub changes to `ci/suite.sh`.
  - R189: one task per package that is under 80%.
  - R188: one task per linter group, then the gate.
  - R191: one task for the `Update` split (itself several commits: the per-message handlers, then the
    key-handler map), one task per remaining cc-over-30 function, one task for the test-fixable ten,
    then the gate.
  - R192 and R193: one task per package (`tui` by file), then the gate for each stage.
  - R197: one task per item group in #61.
- Update the spec inside each task that changes behaviour. The operator makes the SPEC §6.4 and §13
  commits; a SPEC need beyond that is a petition.

## Ground rules

- **These paths are read-only to this job:** `SPEC.md`, `prds/`, `ci/Dockerfile`, `ci/SPIKE.md`,
  `.github/workflows/release.yml` and `ci/releasegate/`.

  ```sh
  BASE=$(git log --format=%H -1 -- prds/phase4j-quality-gates-and-allure.md)  # the operator's PRD commit
  git log --oneline "$BASE..HEAD" -- SPEC.md prds/ ci/Dockerfile ci/SPIKE.md \
    .github/workflows/release.yml ci/releasegate/                              # must print nothing
  ```

  If a requirement needs a change to one of them, petition (§Escape hatch).
- **No exceptions, no baselines of debt.** This is the heart of the phase. A gate goes on only when
  the product passes it as it stands.
  - **Not allowed:** an allow-list of offending functions, packages or files; a baseline file of
    existing findings; a bulk `//nolint`; `new-from-rev`; a path glob that could swallow real code;
    a threshold set to whatever the code currently scores.
  - **Allowed:** a `//nolint:<linter> // <reason>` on a genuine false positive, one finding at a
    time, with the linter named and the reason written. A `.trivyignore` entry, one per line, each
    with a reason and a **review-by date**; the gate fails once a date passes. An explicit,
    commented exclusion for generated or deliberately unmeasurable code, by exact path.
  - A threshold sits in one checked-in config. **A test fails when any threshold is loosened
    relative to the merge-base.** Tightening is always allowed.
- **Fail loudly on no input.** A gate that finds zero packages, an empty coverprofile, zero scored
  functions or zero lint targets **fails**. It never passes vacuously. Each gate has a test that
  proves it, and a test that proves it fails on a seeded violation. A package that legitimately has
  no statements (`internal/racebuild`) is named in an explicit, commented list, never skipped by
  accident.
- **The same locally and in CI.** One entry point, `ci/run.sh ci/quality.sh`. Tools come from the
  `deck-ci` image. **Gate and suite scripts scrub every `DECK_*` variable** before running tests:
  `ci/run.sh` forwards them, and a variable leaked from a live deck session once caused about 30
  spurious failures. Scrub in the script, with a test.
- **Refactors preserve behaviour.** CRAP work is mechanical restructuring: splitting functions,
  table-driving dispatch, extracting handlers.
  - The existing tests and the `features/` suite are the guard, and they stay green after every
    chunk.
  - **A behaviour change is not a refactor.** If a split would change a keymap, a rendering, a
    schema or an observable timing, stop and petition.
  - Do not delete or weaken a test to get a function under the threshold. Do not move code to a
    place the scorer does not measure. The scorer counts every `.go` file in the product and in
    `ci/`, except the explicitly listed `cmd/fake-*` fixtures, which get their own floor and CRAP
    ceiling (R187).
- **CI must stay green.** No red or rerun-rescued run on `main` is acceptable (#57). Land one gate
  at a time, each only after the product passes it **locally under `ci/run.sh`** and the previous
  push run is green.
- **Never touch the operator's live state.** This covers `~/.local/share/deck/`,
  `~/.config/deck/`, `~/.local/state/deck/`, and any `tmux -L deck` or `tmux -L deck-*` server.
  Every test uses its own temp `HOME`/`XDG_*`/`DECK_HOME` and a private socket.
- **Docker is for `ci/run.sh`, `ci/stability.sh` and the gate scripts, and nothing else.**
  - Never remove or kill containers by label: a previous ralphd job SIGKILLed itself by sweeping
    `label=ralphd.run`.
  - No `docker prune`, no wildcard `rm`/`rmi`. Never signal by pattern: resolve a pid, verify it,
    then signal that pid.
  - Other runs and the two `deck-ws-*` self-hosted runners live on this host. Never stop, restart,
    reconfigure or re-register a runner.
- **No paperwork.** No report, findings, audit or close-out files in the repo; no
  `docs/DELIVERY-LOG.md` row. Measurements, offender lists and run ids go to `/run/ralphd/artifacts`.
  Do not close GitHub issues: the operator closes them at release.
- **GitHub scope.** The credentials are the operator's PAT. The job may push to `main`, read the
  Actions API (logs and artifacts included) and start `ci.yml` by `workflow_dispatch` on `main`. It
  may **open GitHub issues** only for §R194's "file, don't fix" cases. Nothing else: no branches,
  PRs, tags or releases, no direct push to `gh-pages`, no cancelling, re-running or deleting a
  workflow run, no change to settings, environments, Pages, runners or secrets.

## Scope

All of the following are acceptance:

- **R187–R189**: the gate infrastructure, and the coverage gate.
- **R190–R193**: the vulnerability gates, lint, and the CRAP ratchet 30 → 20 → 15 → 10.
- **R194**: the release security review process, documented.
- **R195–R196**: native Allure results.
- **R197**: the #61 follow-ups.
- **R198**: `docs/ci.md`.

## Materiality rubric

Reject only on substance:

- a requirement's behaviour is absent or wrong in the live code or workflows;
- a gate that is on **has an exception**: an allow-list, a baseline of existing debt, a grandfathered
  function, a bulk `//nolint`, or a `nolint` or `.trivyignore` entry with no named linter or reason,
  or no review-by date;
- a gate **passes vacuously**, or has no test proving it fails on a seeded violation and on empty
  input;
- a threshold is looser than at the merge-base, or looser than the PRD's schedule at that stage;
- a refactor changes behaviour, or a test was deleted, skipped or weakened to pass a gate;
- a test the requirement names is missing, red, or also passes on the unfixed tree;
- **any proof run in §Definition of done is red, or has a non-empty flaky record**;
- the spec contradicts the live code or workflows;
- a secret is exposed, in the tree, a workflow log or a published report;
- a guard in §Ground rules is broken (a protected path modified, the operator's deck directories or
  a `tmux -L deck*` server touched, a branch/PR/tag/release created, a run cancelled or re-run, a
  setting or runner changed).

The working clauses: file the rule, not the example; read the direction of the evidence (a line
that could support either side supports neither); a review after a cure pass confirms the cure
landed and does not re-audit the product; judge a hat or router by its contract, not its taste.

Everything else verifies, with the gap recorded as a residual note: wording, form, provenance,
process, and **any number or claim in prose outside the spec**. There is **no advisory list**. One
exception: a run that never reached `suite` because GitHub failed to provision a runner, or because
of an Actions API 5xx, is not judged. It neither counts toward the proof nor resets it. Record it
and dispatch another.

## Escape hatch

If the run proves that a requirement contradicts `SPEC.md`, that a refactor cannot preserve behaviour,
or that a gate cannot be satisfied without an exception, it must not edit the spec, delete a test,
add an exception or quietly narrow the gate. File a petition with:

- the evidence;
- the smallest change that would resolve it.

Then notify the operator and carry on with everything the petition does not block. The operator
rules by amendment.

## Notifications

The operator is AFK on Telegram, so use the `notify` hat tool in the same iteration as the work.
Send one message for each of:

- anything that blocks work outright, including a petition;
- each gate as it goes on (name, threshold, and what the product needed to pass it);
- each CRAP stage landed (the threshold and the number of functions refactored);
- every rejection and cure pass;
- the proof run's result;
- one terminal summary.

Keep each message to a few hundred characters. A missed or late send is cured by mentioning it in the
next one. It is never a success criterion and never a finding.

## R187 — the gate infrastructure

- **`ci/quality.sh`** runs every gate that is on, in order, and prints one readable report per gate
  to the job summary: top offenders, and what to do about each. It runs under `ci/run.sh`, scrubs
  `DECK_*`, and exits non-zero if any gate fails. It is wired into `ci.yml` as a step in the
  existing `lint`/`suite` jobs, **not** a new lane that could be skipped.
- **One thresholds file**, checked in (for example `ci/quality.toml` or `.json`). Every gate reads
  its threshold from there and from nowhere else.
- **A loosening test.** A Go test compares the thresholds file with its merge-base version
  (`git merge-base HEAD origin/main`, falling back to `HEAD~1` on a shallow clone, with a clear
  message) and fails if any threshold is looser. Tightening passes. It is skipped nowhere: if the
  merge-base cannot be resolved, the test **fails**.
- **`ci/crapgate`**, a stdlib-only scorer:
  - `go/ast` McCabe counting with the gocyclo rules spelled out and locked by tests: `1 + if / for /
    range / case / comm-case / && / ||`, closures counted into the enclosing function;
  - per-function coverage from the **merged** profile's blocks (unit plus `features/`);
  - `CRAP = cc² × (1 − cov)³ + cc`, with a zero-statement function scored at 100% coverage;
  - zero scored functions fails, an empty or missing profile fails, and a function present in the
    source but absent from the profile is scored at 0% coverage, not skipped.
- **The merged profile.** `ci/suite.sh` already runs both passes. Add `-coverpkg=./...` to the unit
  pass and a `GOCOVERDIR` to `features/`, and merge with `go tool covdata textfmt`. A run whose
  merged profile is empty fails. **The suite's pass/fail/flaky counts and its runtime budget are
  unchanged beyond the instrumentation cost**; say what that cost measures.
- **Fixture floors.** `cmd/fake-*` are fixtures: they get their own, lower coverage floor and the
  same CRAP ceiling as product (their baseline is 18 functions over 10, four over 30, all fixable
  by tests).
- **Tests** prove, for the scorer and for the gate scripts: a seeded high-CRAP function fails, a
  seeded low-coverage package fails, a seeded empty input fails, and the report names the offender.

## R188 — golangci-lint v2

- A checked-in **`.golangci.yml`**: the `standard` set (errcheck, govet, ineffassign, staticcheck,
  unused) plus gosec, revive, gocritic, errorlint, misspell, unconvert, unparam, bodyclose,
  copyloopvar, and `nolintlint` with `require-specific` and `require-explanation`.
  - **`gocognit` is not enabled.** It cannot be report-only inside golangci, and CRAP is the
    complexity gate.
  - The existing `gofmt`, `go vet` and `go mod tidy` checks stay in `ci/lint.sh`, or move into
    golangci's formatters. Either way, one place, still green.
- **Test-file scope is an explicit, commented rule, not a suppression.** In `_test.go` files only,
  gosec's G204, G304, G301, G302 and G306 (test fixtures run subprocesses and write temp files by
  design), and errcheck on `Close` of a test-owned handle, are excluded. Each exclusion carries its
  reason in the config. **Production code has no such exclusions.** Everything else applies to tests
  too.
- **Zero findings on the whole tree, tests included**, then the gate goes on in the `lint` job,
  before the suite. Fix, do not suppress:
  - errcheck and `%w` in production code: handle or wrap the error;
  - revive `unused-parameter`: rename to `_` or use the parameter; exported symbols without doc
    comments get a real doc comment, not a boilerplate one;
  - gosec G204/G304 in production: validate or document where the value comes from, in code. A
    `//nolint:gosec // reason` is allowed only for a genuine false positive.
- The pinned image runs the gate. `ci/lint.sh` and `ci/quality.sh` use the same invocation.
- **Tests:** a seeded violation (an unchecked error, an unexplained `//nolint`) fails; an empty
  package list fails; a `//nolint` with no linter name or no reason fails.

## R189 — the coverage gate

Gate on the **merged** profile:

- **total ≥ 85%** and **every product package ≥ 80%**. `ci/*` Go tools **count as product**.
- `cmd/fake-*` are excluded from product, with their own floor (R187).
- Write the tests first. The baseline gap is small: `internal/service` about +55 statements,
  `internal/store` about +34, `cmd/deck` about +8, `ci/prcomment` +1. Test behaviour, not lines:
  error paths, the `cmd/deck` startup and hook paths, store migrations and lease edges. A test that
  executes a line and asserts nothing is not coverage.
- Anything excluded because it is generated or deliberately unmeasurable is an **explicit, commented
  list by exact path**. Never a glob that could swallow real code. A package that has zero
  statements is named in that list.
- Once on, the gate prints when a package beats its floor by 1 pp or more, as a prompt to tighten.
  Floors never go down.
- **Tests:** a seeded 79% package fails; a seeded 84% total fails; an empty profile fails.

## R190 — vulnerability scanning

- **`govulncheck ./...`** gates on vulnerabilities in code deck actually **calls**. It runs on the
  pinned image's toolchain with `GOTOOLCHAIN=local`. The gate reads the `toolchain` line that R197
  adds to `go.mod` and **fails when the running `go version` is older than it**. A test proves it, so
  a stale image cannot pass quietly.
- **`trivy fs`** over the repo with `vuln,secret,misconfig`, `HIGH,CRITICAL`, `--ignore-unfixed`:
  - fix what can be fixed: bump `golang.org/x/text` to its fixed version and tidy;
  - exceptions live in `.trivyignore`, one per line, each with a reason and a **review-by date**; a
    test fails once a date has passed;
  - skip the 11 MB `docs/reports/.../summary.log` by exact path with a comment, or leave the file;
  - the Trivy DB cache lives under the existing `/go-cache` volume so a warm run is fast.
- **Nightly re-scan.** New CVEs appear without code changes. The existing nightly (schedule) path
  re-runs both scanners on the same tree. A newly-disclosed vulnerability turns the nightly red and
  the existing notify step tells Telegram. **Do not change the nightly's cron slot or the release
  gate** (`ci/releasegate` keeps counting push `suite` runs only).
- **Tests:** a vulnerable-module fixture fails govulncheck's gate; an expired `.trivyignore` entry
  fails; an empty scan target fails.

## R191 — CRAP ratchet stage 1: ≤ 30

Switch the gate on at **30**, only after every function passes. Baseline: 20 over, 10 needing a
refactor.

- **Split `tui.Model.Update`** (cc 393, 29 closures) into per-message and per-key handlers. It is a
  mechanical split and the biggest single lever. `features/` protects it.
- Split `store.UpdateSessionStatus` (68), `service.Resume` (48), `service.reconcile` (45),
  `tui.interactiveBareNamedKey` (44), `config.LoadFromProfile` (35), `tui.repaintForeignDefaults`
  (35), `tui.settingsGroupsViewLines` (33), `config.setField` (33), `tui.updateSettings` (33), and
  `service.CreateAgent` (30).
- **Cover** the ten test-fixable offenders, among them `styledMoveGroupBody` (0%), `SetSessionGroup`
  (0%), `ansiEscapeLen` (39%), `CreateProfile` (42%), and the `cmd/fake-*` recorders.
- Prefer table-driven dispatch (a map of key to handler) over a deeper `switch`, so cc falls for
  real instead of moving to another function.

## R192 — CRAP ratchet stages 2–3: ≤ 20, then ≤ 15

Each stage is its own gate commit, switched on only after the product passes it, with the previous
run green. Baseline: **44** functions over 20 (28 refactors), **66** over 15 (48 refactors). `tui`
holds most of them.

## R193 — CRAP ratchet stage 4: ≤ 10

The end state, as the operator decided. Baseline: **123** over 10, **89** needing a refactor, of which
`internal/tui` has 42, `service` 13, `store` 10, `config` 7, `tmux` 5.

- **Switch the gate to 10 only when every function passes.** Each earlier stage's threshold stays
  checked in, and the final commit leaves **only 10**.
- A function that is 98–100% covered and still fails on cc alone (dispatch tables, long switches) is
  fixed by a split, not by a test. If a function genuinely cannot reach cc ≤ 10 without a behaviour
  change or making the code worse, **petition** with the function and the evidence. Do not raise the
  threshold and do not exempt it.
- **Budget.** This is the largest chunk of the phase. If runner time or the iteration budget makes
  stage 4 unreachable, **stop at the last stage that is fully clean and on**, tell the operator in
  the terminal summary what remains (count and list), and leave the gate at that stage. A half-done
  stage 4 is never committed as "on".

## R194 — the pre-release security review (a process, not a gate)

Document it in `docs/ci.md`; **nothing about a review is tracked in the repo** otherwise.

- **When:** before a release tag is cut, the operator asks Claude Code for a security review of
  everything since the last reviewed tag.
- **How:** separate review agents cover the hook input (`deck _hook` stdin), tmux command
  construction and quoting, the filesystem and permissions under `DECK_HOME`, env and secret
  handling, the workflows and the self-hosted runner trust boundary, and dependencies. Each finding
  is adversarially verified before it is reported.
- **Output:** each confirmed finding becomes a GitHub issue, a short summary goes to Telegram, and
  the release's tag message or notes carry "security review: <range>, N findings, all
  resolved/filed".
- **Blocking:** a confirmed HIGH or CRITICAL finding blocks the release. Lower findings are filed and
  do not block.
- **Rejected alternatives**, with the reason each time: the scheduled `claude-code-security-review`
  Action (PR-oriented, needs a model API key as a repo secret), a periodic ralphd security job
  (costlier), and a session cron (fires only while a session is alive).

## R195 — native Allure results for `features/`

A Godog formatter writes `allure-results/` (`*-result.json`, `*-container.json`, attachments; the
Allure 2 schema) **alongside** the existing JUnit output. Prefer a small in-repo formatter under
`features/` over a third-party dependency, unless one fits and pins cleanly without entering the
product's `go.mod`.

- **Hierarchy.** The feature file is the Allure `feature`, the scenario is the test, and every
  Gherkin step is an Allure step with its own status and duration. Step text is kept verbatim.
- **Tags become labels.**
  - `@gh-NN` becomes an issue link to `https://github.com/n-orlov/deck/issues/NN`.
  - `@multiclient`, `@slow` and `@nightly` become `tag` labels.
  - An agent tag (`@claude`, `@pi`, `@codex`) becomes a `suite`/`parentSuite` label.
- **Attachments on the failing step**, reusing what the harness already writes on failure: the last
  normalized pty frame (text), the tmux capture(s), the deck-log slice, and the store dump if the
  harness has one. **Collect nothing new.**
- **Retries.** A scenario retried by the suite's one-retry rule shows as Allure retries of the same
  test (same `historyId`), so Allure's own flaky marker applies. It never shows up as two tests.
- **Environment and executor.** `environment.properties` (Go version, tmux version, sha, `-race` or
  not) and `executor.json` (the Actions run link).
- **`ci/summary.sh`'s counts are unchanged**, or derived from the same source, so the job summary and
  the report never disagree.

## R196 — one report, with trends intact

- Go unit tests keep JUnit; both kinds land in one report, grouped `unit` and `features`.
- **Trends keep working.** The `ci/allure-report.sh` history dir carries over on root publishes.
  A one-time trend reset is acceptable and **must be said in the commit message**.
- **Out of scope:** what Pages publishing does (#50 owns it) and any test-management integration.
- **Tests:**
  - A unit test runs the formatter over a fixture Godog run with a pass, a fail with an attachment,
    a skip and a retried-then-passed scenario, and asserts the emitted JSON: statuses, steps, labels
    (the `@gh-NN` link included), that the attachment file exists and is referenced, and that the
    retry shares its `historyId`.
  - `allure generate` over that fixture succeeds. This is a CI-side smoke check in `ci/`, with
    Allure pinned as today.
  - The existing suite and summary tests stay green.

## R197 — the #61 follow-ups

Fix each item in #61, each with a test where the behaviour is testable. Where an item is a SPEC
statement (the `PRAGMA secure_delete` sentence in §6.4), the operator has made that SPEC commit
before this PRD. Where an item needs a decision the issue does not give, petition.

- **Env / data at rest:** `PRAGMA secure_delete=ON`. **Do not** use `journal_size_limit=0`: a reset
  WAL brings back the fsync the hook-latency fix removed, and a test or measurement shows the hook
  path does not regress. The `Env` field in the create modal is masked (§6.4). The config directory
  is created `0700`. A new `state.db` is created `0600` from the start (umask set before `open`, not a
  chmod after).
- **TUI / tmux:** control characters are stripped from directory names and prefilled field values
  before they are drawn: the cwd ghost, completion, the candidate list, and name/args/env prefills.
  `lineedit` strips on `New`/`NewOffered`, not only on Insert/Paste.
- **Hooks:** `deck _hook` caps the stdin it reads. The cap is generous (it must never truncate a
  real payload) and a test shows an oversize payload is rejected without unbounded growth.
- **Build / supply chain:**
  - release binaries build on a pinned, current Go 1.25.x patch (`toolchain` line in `go.mod`, and
    `setup-go` set so it cannot fall back to 1.25.0);
  - `${{ github.ref_name }}` in the `ci.yml` notify step goes through `env:`, not into `run:`;
  - Actions are pinned by commit SHA, with the tag in a comment;
  - `ci/allure-report.sh` verifies the Allure download against a checksum;
  - the `GITHUB_TOKEN` extraheader is not left in `site/.git/config` in the self-hosted workspace;
  - `install.sh` adds `--proto =https` to its curl calls; its checksum limitation (same-release
    `checksums.txt`) is **stated in a comment**, since signing is out of scope;
  - `ci/releasegate` accepting a "suite" check from any workflow is a **protected path**: petition,
    do not edit it.

## R198 — docs

`docs/ci.md` documents every gate: its threshold, where the thresholds file is, what the report
looks like, how to run it locally, how to read a failure, how to adopt or tighten a ratchet, and the
`.trivyignore` and `//nolint` policy. It states the CRAP staging and which stage is on. It documents
the Allure results (R195–R196) and the security review process (R194). SPEC §13 naming the gates is
the operator's separate commit, not this job's.

## Ordering

Each numbered step lands, goes green on push, then the next begins.

1. **R187**, the infrastructure and its honesty tests, with no gate on yet.
2. **R195–R196**, Allure. Self-contained and low-risk, and it improves the report that everything
   after it is judged by.
3. **R197**, the #61 items. They clear the way for the vulnerability gates (the toolchain pin
   especially) and for the lint and coverage work.
4. **R190**, the vulnerability gates: cheapest, already near-clean.
5. **R189**, tests and then the coverage gate.
6. **R188**, lint onboarding to zero, then the gate.
7. **R191**, **R192**, **R193**, the CRAP ratchet, one stage at a time. Every function that a stage
   would fail is fixed first. The `Update` split comes first within R191.
8. **R198.**
9. **The proof (§Definition of done), as the last task, marked `sweep: true`, with nothing committed after it.**
   `origin/main`'s head when the run ends is the swept sha. A stale sweep is never process: if main
   moved past the swept sha, the sweep re-runs. Issue closure is a comment, not a tracked file. If the
   proof is red or flaky, that is a real defect: fix it at its root with a failing-first regression
   test, push, and restart the proof from zero.

## Definition of done

- R187–R198's behaviours are in the live code, and each named test exists and is green. Every
  gate's seeded-violation and empty-input tests fail on the unfixed condition.
- **Every gate is on, with zero exceptions**, at the final pushed sha: coverage (total 85, package
  80), CRAP at the **final stage reached** (target 10), golangci-lint, govulncheck, Trivy. No
  offender allow-list, no baseline file of debt; every `//nolint` and `.trivyignore` entry is
  individually justified and dated.
- A test proves that loosening any threshold against the merge-base fails.
- **At the final pushed sha, the proof is one push plus ONE dispatch**, never a multi-dispatch gate:
  - the `push` run of `ci` concluded `success` with an empty flaky record;
  - **one** `workflow_dispatch` run of `ci` (the nightly path: `-race`, `ci/stability.sh 3`, vuln
    re-scan, report and publish) concluded `success` with an empty flaky record;
  - "empty flaky record" means `ci/suite.sh` reran no scenario and no Go test and the stability loop
    had no failure, read from the run's log and artifacts. A red or flaky run resets the proof, apart
    from the exception in §Materiality rubric;
  - `ci/run.sh ci/quality.sh` is green locally and prints the same verdict as CI;
  - `ci/run.sh go test -p=1 -count=1 ./...` is green locally, with no narrowed package list and no
    `-run` filter;
  - `go build ./...`, `go vet ./...` and `gofmt -l` are clean on every file this run touched;
  - the protected-path audit command prints nothing.
- The Allure report at the dispatch run shows `features` steps, labels, the `@gh-NN` links and an
  attachment on a seeded or real failure.
- No secret is exposed.

No report, record or log row goes in the repo.

## Non-goals

- Changing the release gate, `release.yml`, the workflows' triggers or concurrency, the runners or
  the nightly's cron slot.
- Changing what Pages publishing does (#50), or a test-management integration.
- Release signing, SBOMs, or fixing the `install.sh` same-release-checksum limitation beyond a
  comment.
- Feature work: #62, #63 and #65 wait for the next wave. The CRAP refactor must not collide with
  them.
- Publishing a release or cutting a tag.

## For the planner

- **Runner time is scarce.** There are two shared `deck-ws-*` slots. A push run takes about 8
  minutes, a dispatch about 55. Spend the single dispatch at the end, after local evidence.
- **The unit pass is 167 s and the features pass 458 s on a 28-core host.** Run the narrowest
  package-level tests while refactoring, the whole `ci/run.sh ci/suite.sh` before each push.
- **The `Update` split is the risk in R191.** Do it in small, behaviour-preserving steps, each
  checked against `features/`, and measure CRAP after each. A handler map beats a nested switch.
- **Measure, don't guess.** Re-run the CRAP scorer after every chunk and record the offender list in
  `/run/ralphd/artifacts`, so a later iteration can resume from it.
- **Plan R193 as many small chunks** (per package, `tui` split by file), each pushed green, rather
  than one giant diff.
- **Budget for one cure pass.** That is the harness working.
