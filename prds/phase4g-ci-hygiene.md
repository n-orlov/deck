# Phase 4g — CI hygiene: main's Actions history is green unless the code is broken

## What the operator gets

One issue, GH #50: the Actions tab on `main` becomes a truthful signal. **Every run is green
unless the code under test is genuinely broken.** No harness-caused red, and no grey
`skipped`/`cancelled` noise runs. In short:

1. `pages-pr-publish` never starts for a `main` run, so it leaves no skipped run behind.
2. Wall-clock budgets are not asserted on the `-race` build, so the nightly is not red for a
   timing reason. The nightly still catches real races and real regressions.
3. Nothing on `main` is cancelled. The nightly and manual lane never shares a concurrency group
   with pushes, and every push gets its own complete run.
4. A transient failure of the hosted Pages deploy is retried rather than reported as red.

#50's body is the evidence and the design discussion. **Read it before starting.** Where the issue
and this PRD disagree, this PRD wins; where either disagrees with `SPEC.md`, SPEC wins, and the
disagreement is a finding. Two points were decided since the issue was filed:

- **Push bursts: every sha gets its own run** (option two in #50's "Wanted" §3). Accepting
  superseded pending runs is rejected, because it leaves `cancelled` runs on `main`.
- **Item 4 is new.** It was found after the issue was filed. Run
  [36662439839](https://github.com/n-orlov/deck/actions/runs/36662439839) (push, `579f38d2c3`)
  went red only because `actions/deploy-pages` failed to fetch its OIDC token. The suite and the
  report were green.

## Why now

v0.2.7 (`dc2b6f7ece`) shipped on 2026-09-30. This is the operator's next ask.

**`SPEC.md` has already been amended for this phase** in an operator commit that lands *before*
this PRD:

- §3.1's `_hook` row: the 20 ms budget is a normal-build budget, and a `-race` build does not
  assert it;
- §13.2's "CI runs the same thing" bullet: the new paragraph starting "Main's Actions history is a
  truthful signal".

Nothing here is blocked on the operator.

## Ground rules

- **These paths are read-only to this job:** `SPEC.md`, `prds/`, `ci/Dockerfile`, `ci/SPIKE.md`,
  `.github/workflows/release.yml` and `ci/releasegate/`. The release gate's behaviour must not
  change (#50's acceptance), so this job never edits it.

  ```sh
  BASE=$(git log --format=%H -1 -- prds/phase4g-ci-hygiene.md)   # the operator's PRD commit
  git log --oneline "$BASE..HEAD" -- SPEC.md prds/ ci/Dockerfile ci/SPIKE.md \
    .github/workflows/release.yml ci/releasegate/                # must print nothing
  ```

  The SPEC amendment lands before `$BASE`, so it is outside the audit range by construction. It is
  not this run's commit, and it must not be re-made, extended or "cured".
- **No product behaviour changes.** Code under `cmd/`, `internal/` and `features/` may change
  only as far as R164 needs: a race-build indicator, and budget assertions that consult it. No
  schema change, no keymap change, no rendering change.
- **Never touch the operator's live state.** This covers:
  - `~/.local/share/deck/`, `~/.config/deck/` and `~/.local/state/deck/`. The job never opens,
    reads, copies or creates anything there;
  - the live `tmux -L deck` server, and any `tmux -L deck-*` server. Never touch them, not even
    read-only.

  Every test uses its own temp `HOME`/`XDG_*`/`DECK_HOME` and a private socket.
- **This is a behaviour change, so a test that passes on today's code proves nothing.** R163-R166
  each name assertions that must **fail on the v0.2.7 tree (`dc2b6f7ece`) and pass after**. Run the
  new tests against the unfixed tree. The evidence (the command and the failure) goes to the run's
  artifacts, never into the repo.
- **Docker is for `ci/run.sh` and `ci/stability.sh`, and nothing else.**
  - Never remove or kill containers by label: a previous ralphd job on this host SIGKILLed itself
    by sweeping `label=ralphd.run`.
  - No `docker prune`, and no wildcard `rm`/`rmi`.
  - Never signal by pattern: resolve a pid, verify it, then signal that pid.
  - Other runs and the two `deck-ws-*` self-hosted runners live on this host. Never stop, restart,
    reconfigure or re-register a runner.
- **Code references are pointers, not criteria.** The references below were correct at
  `dc2b6f7ece`. If one has drifted, find the current site by name and carry on. Drift is never a
  finding.
- **No paperwork.** Do not add report, findings, audit, close-out or "retake" files to the repo,
  and do not add a `docs/DELIVERY-LOG.md` row.
  - Run evidence (Actions API listings, sweep logs, the budget audit, CI run URLs) goes to
    `/run/ralphd/artifacts`.
  - CI evidence is the CI run itself, found by its head sha.
  - Do not close GitHub issues: the operator closes them at release.
- **The CI container has no agent binaries.** Every scenario runs against the `cmd/fake-*` stubs on
  a fixture `PATH`.
- **GitHub scope.** The git credentials are the operator's PAT. The job may:
  - push to `main`;
  - read the Actions API;
  - start `ci.yml` by `workflow_dispatch` on `main` (R165 and §Definition of done need it).

  Nothing else:
  - no branches other than `main`, and no direct push to `gh-pages`. Only the workflows write
    `gh-pages`;
  - no PRs, no tags, no releases;
  - no cancelling, re-running or deleting a workflow run;
  - no change to repository settings, environments (including `github-pages`'s "main only"
    branch policy), Pages configuration, runners or secrets.

## Scope

- **R163:** `pages-pr-publish` never starts for a `main` run.
- **R164:** wall-clock budgets are not asserted on the `-race` build.
- **R165:** nothing on `main` is cancelled.
- **R166:** a transient Pages deploy failure is retried.
- **R167:** `docs/ci.md` and the spec agree with the workflows.

All of them are acceptance.

## Materiality rubric

Reject only on substance:

- a requirement's behaviour is absent or wrong in the live workflows or code;
- a test the requirement names:
  - is missing or red;
  - asserts the opposite of the requirement;
  - for R163-R166, also passes on the unfixed tree `dc2b6f7ece`;
- the spec contradicts the live workflows or code;
- the PR preview flow is broken. That means `/pr/<n>/` publishing, the fork guard, or the
  `pr-comment` job no longer follows from a same-repo PR's `ci` run;
- a real budget is weakened on a normal build: a normal build asserts a looser limit, or no limit,
  where `dc2b6f7ece` asserted one;
- a secret is exposed, in the tree, a workflow log or a published CI report;
- a guard in §Ground rules is broken:
  - a protected path is modified (the audit command prints anything);
  - any access to the operator's deck directories or any `tmux -L deck`/`deck-*` server;
  - a branch, PR, tag or release is created, or `gh-pages` is pushed by hand;
  - a workflow run is cancelled, re-run or deleted by the job;
  - a repository setting, environment, runner or secret is changed.

Everything else verifies, with the gap recorded as a residual note. That covers wording, form,
provenance and process, the Allure report's cosmetics, and **any number or claim in prose outside
the spec** (commit messages, notes, run artifacts, and `docs/ci.md` beyond R167's own list). Prose
outside the spec is never a blocking ground.

**Advisory, never blocking:**

- a CI or stability failure in a scenario whose failure this phase does not claim to fix, and
  that is not timing-under-`-race`. Such failures are the known flake classes. Name the failure in
  artifacts and in the terminal notification, and do not cure it;
- a `ci` run on `main` that the job's own red WIP commit made red. That run is the code being
  genuinely broken, which is exactly what should be red;
- a GitHub-side outage outside the retried Pages deploy (runner provisioning, API 5xx);
- any operator notification that is missed or late.

## Escape hatch

If the run proves that a requirement contradicts `SPEC.md`, or is impossible on GitHub Actions as
written, it must not edit the spec or quietly narrow the requirement. File a petition with the
evidence (GitHub docs, a run URL) and the smallest change that would resolve it, notify the
operator, and carry on with everything the petition does not block. The operator rules by
amendment. SPEC changes are the operator's commits.

## Notifications

The operator is AFK on Telegram, so use the `notify` hat tool in the same iteration as the work.

Send one message for each of:

- anything that blocks work outright, including a petition;
- each requirement as it lands;
- every rejection and cure pass;
- the final sweep result;
- one terminal summary.

Keep each message to a few hundred characters: the notifier refuses a body over 16000 characters
outright. A missed or late send is cured by mentioning it in the next one. It is never a success
criterion and never a finding.

## R163 — `pages-pr-publish` never starts for a `main` run

- Today `.github/workflows/pages-pr-publish.yml` triggers on every `ci` completion
  (`workflow_run: workflows: ["ci"], types: [completed]`). Its `gate` job runs only for
  same-repo `pull_request` runs, so every push, nightly and dispatch run on `main` leaves a whole
  skipped run behind.
- **After this phase, a `ci` run whose head branch is `main` starts no `pages-pr-publish` run at
  all.** That covers push, schedule and `workflow_dispatch` runs. The trigger filters on the
  triggering run's head branch (`workflow_run` supports `branches`/`branches-ignore`), or the
  workflow is restructured to the same effect.
- A same-repo PR's `ci` completion still triggers it, and it behaves exactly as today: the `gate`
  job's head-repository and `report (Allure)` checks, the `gh-pages` checkout, and the deploy. The
  in-workflow gate stays as defence in depth.
- **Success:**
  - A `ci/workflowcheck` test parses `pages-pr-publish.yml` and asserts:
    - the trigger excludes a `main` head branch;
    - the trigger still admits a non-`main` head branch;
    - the `gate` job still requires a `pull_request` event and a same-repo head.

    It fails on `dc2b6f7ece`.
  - **Live:** the Actions API lists no `pages-pr-publish` run whose triggering run is a `ci` run
    on `main` that was created after the R163 commit reached `main`. The listing goes to
    artifacts.

## R164 — wall-clock budgets are not asserted on the `-race` build

- The nightly and manual lane builds with `-race`, which is several times slower, and stability
  runs have no retries by design. Run
  [36406261401](https://github.com/n-orlov/deck/actions/runs/36406261401) went red on
  `features/hook_contract.feature:6`: `operation-scoped hook store duration = 20.153ms, want < 20ms`.
- **A race-build indicator.** Add a build-tagged constant: `//go:build race` sets it true and
  `//go:build !race` sets it false. It lives in one small package, under `internal/`, that both
  `features/` and `internal/` tests can import. The Go standard library has no exported
  equivalent.
- **Every wall-clock budget assertion consults it.** On a race build the assertion is not judged:
  the step passes and logs the measured value and the reason. On a normal build it asserts exactly
  the limit it asserts at `dc2b6f7ece`.
  - A **budget** is an assertion that the product did something within a time limit, such as
    `hookStoreDurationBelow` in `features/hook_contract_test.go`.
  - A **deadline** is a poll timeout while waiting for a condition, and it is not a budget.
    Deadlines are out of scope unless the run shows one failing under `-race` on CI.
- **Audit.** Search `features/`, `internal/`, `cmd/` and `ci/` for every budget assertion, and
  put the list with its disposition in artifacts. `hook_contract.feature:6` is the known one.
- The race build still runs every test and every scenario. Only the budget comparison is
  suspended, so real data races and functional regressions still fail the nightly.
- **Success:**
  - A test in the indicator's package, compiled both ways, proves the constant matches the build.
    The `race` file's test expects true and the `!race` file's test expects false, so
    `go test -race` and plain `go test` each run one.
  - A unit test drives the budget step's comparison directly:
    - with the indicator false, 20.153 ms against a 20 ms limit fails, and 19 ms passes;
    - with the indicator true, 20.153 ms passes, and the log names the measured value.

    Make the comparison injectable for this test; do not rebuild with the tag. The test fails on
    `dc2b6f7ece`, where the comparison has no race path.
  - The same treatment covers every other budget the audit finds, each with a test of the same
    shape.

## R165 — nothing on `main` is cancelled

- Today `ci.yml` has one concurrency group, `ci-${{ github.workflow }}-${{ github.ref }}`, with
  `cancel-in-progress` true only for PRs. GitHub keeps only **one pending** run per group and
  cancels the older pending run when a newer one queues. So push bursts cancel queued push runs,
  and a nightly (run 36230437202, 2026-09-26) was cancelled with zero jobs because it shares the
  group with pushes. There were eleven cancelled runs in the 200 before v0.2.5, and four more on
  2026-09-29 alone.
- **After this phase:**
  - `schedule` and `workflow_dispatch` runs never share a concurrency group with `push` runs, so
    no push cancels or supersedes a queued nightly;
  - every push to `main` gets its own complete run, and no push run is cancelled by a later push
    or by any other run;
  - a same-repo PR's superseded run is still cancelled by its successor, as today. That is the
    only cancellation the workflow asks for.
- **The trap is the shared publishing state that the one group used to serialise.** Two root runs
  may now overlap on the two `deck-ws-*` slots, so the design must cover both of these:
  - **`report` pushes `gh-pages`.** A concurrent push must not red a run or drop a report. For
    example, on a non-fast-forward rejection re-fetch `gh-pages`, re-merge the new report with
    `ci/allure-site.sh`, and push again, with a bounded retry. Every PR subtree already in the tree
    survives, and the root report is the one from whichever run pushed last.
  - **`actions/deploy-pages`** refuses to start while another Pages deployment is in progress.
    Two overlapping deploys must not red a run: wait-and-retry, bounded. A job-level
    `concurrency` group on `publish` does **not** solve it, because a pending job in a group is
    cancelled the same way a pending run is, and that turns the run `cancelled`.
  - Allure history carried forward across overlapping root runs may skip one run's entry. That is
    accepted.
- `ci/releasegate` is unchanged, and so is the rule it enforces: only push/PR `suite` runs gate a
  release. It already reads every run for a sha, and per-sha runs keep that true.
- **Success:**
  - `ci/workflowcheck` tests parse `ci.yml` and assert:
    - the concurrency group evaluated for a `schedule` and a `workflow_dispatch` event differs
      from the one for a `push` to `main`;
    - two `push` events with different shas get different groups;
    - `cancel-in-progress` is true for `pull_request` and false otherwise;
    - `publish` has no job-level `concurrency`.

    Evaluate the expressions for each event with a small evaluator or table in the test; do not
    string-match. They fail on `dc2b6f7ece`.
  - A test proves the `gh-pages` persist step's retry path. It runs the step's script (extracted
    into a `ci/` shell script if needed) against two local bare repos: a concurrent push lands
    first, and the step still pushes, keeping both runs' contributions.
  - **Live, at least once during the run:**
    - two pushes to `main` land while the first one's run is still queued or in progress, and
      both runs complete with a non-`cancelled` conclusion;
    - a `workflow_dispatch` run started while a push run is queued or in progress completes, and
      is not cancelled.

    Record the run ids in artifacts. The Actions API lists no `cancelled` `ci` run on `main`
    created after the R165 commit reached `main`.

## R166 — a transient Pages deploy failure is retried

- `publish (Pages)` in `ci.yml` and `publish (Pages, PR)` in `pages-pr-publish.yml` each call
  `actions/deploy-pages` once. A single transient failure, such as run 36662439839's OIDC token
  fetch error, reds a run whose code is fine.
- **After this phase**, each deploy is attempted up to three times, with a back-off between
  attempts, before the job fails. This can be done by chaining `continue-on-error` attempts gated
  on the previous attempt's outcome. The step that finally fails still fails the job, so a Pages
  outage that persists stays red. R165's "another deployment in progress" wait uses the same
  mechanism.
- **Success:** a `ci/workflowcheck` test parses both workflows and asserts:
  - each deploy job has at least three deploy attempts;
  - each later attempt runs only when the previous one failed;
  - the job fails when every attempt fails.

  It fails on `dc2b6f7ece`.

## R167 — `docs/ci.md` and the spec agree with the workflows

- The operator's amendment (§Why now) states the rules. When R163-R166 are done, both amended
  passages must describe the live workflows and code.
- `docs/ci.md` describes the resulting rules. The following list is the obligation, and a missing
  item blocks:
  - which runs exist, per event;
  - that a `main` run starts no `pages-pr-publish` run;
  - the concurrency groups, and that only a superseded PR run is ever cancelled;
  - the `-race` budget rule and the indicator's name;
  - the deploy retries, and the `gh-pages` push retry.

  Its wording is not a finding. Remove or correct every passage that the change makes false, for
  example the line that says main and nightly runs "queue" in one group.
- **Success:** the requirement tests above cover each amended passage's behaviour. No test pins
  `docs/ci.md`'s wording.

## Ordering

1. **R164** first. It is independent of the workflows and is the one that makes the nightly
   green.
2. **R163**, then **R166**, then **R165**. R165's retry mechanism builds on R166's.
3. **R167**, checked last, against the finished workflows.
4. **The final sweep, as the last task, with nothing committed after it.** This is §Definition of
   done. If the sweep turns up a real defect, fix it, push, and run the sweep again. That is the
   only reason to commit after a sweep.

## Definition of done

- R163-R167's behaviours are in the live workflows and code, and each requirement's named tests
  exist and are green.
- For R163-R166, each named test fails on `dc2b6f7ece`. The evidence goes to artifacts.
- **At the final pushed sha:**
  - the `push` run of `ci` concluded `success`, including `report` and `publish (Pages)`;
  - one `workflow_dispatch` run of `ci` concluded `success`. That is the nightly path: `-race`,
    `ci/stability.sh 3`, report and publish. If it fails only on an advisory flake class (see
    §Materiality rubric), dispatch it once more and record both. A second failure is advisory too,
    provided its cause is named. A failure caused by timing under `-race` is not advisory: it is
    R164 work;
  - `ci/run.sh go test -p=1 -count=1 ./...` is green locally. That means the whole suite, with no
    narrowed package list and no `-run` filter;
  - `go build ./...`, `go vet ./...` and `gofmt -l` are clean on every file this run touched;
  - the protected-path audit command prints nothing.
- The live listings for R163 and R165 are in artifacts.
- A five-run stability sweep (`ci/stability.sh 5`) at that sha is complete, with its logs in
  artifacts. A failure is judged by §Materiality rubric's advisory list.
- No secret is exposed.

Nothing else is required. In particular, no report, record or log row goes in the repo. "A week of
green `main`", #50's own acceptance line, is the operator's to observe after release, not this
run's.

## Non-goals

- **Fixing flake classes** that are not timing under `-race`.
- **Changing the release gate** (`ci/releasegate`, `release.yml`) or what counts toward it.
- **Repository settings:** required checks, branch protection, environments, and Pages source.
- **Job-level `skipped` inside a green run**, such as `PR comment` on a push, or `publish` on a
  PR. #50 §4 accepts these.
- **Moving the nightly's cron slot**, or changing the runners.
- **Any product behaviour change**, and the CPU work (#49).
- **Publishing a release.**

## For the planner

- **Read GitHub's docs, don't guess.** You need the `workflow_run` `branches`/`branches-ignore`
  semantics (they match the triggering run's `head_branch`), the concurrency-group pending-run
  rule, and `deploy-pages`'s "deployment in progress" behaviour. Cite the doc in a code comment
  where the workflow depends on it.
- **The live proofs cost runner time.** Two shared slots, about 8 minutes per push run, and about
  50 minutes for a dispatch run with `-race` and `stability 3`. Batch commits. Poll the Actions API
  rather than sleeping in a loop. Do not dispatch more than the §Definition of done allows plus
  R165's one live proof.
- **Your own WIP will make some `main` runs red.** That is correct: the code was broken. It is not
  a finding against R165. R165 is about `cancelled`, not `failure`.
- **The R165 overlap test is real.** After the concurrency change lands, the next two pushes in
  quick succession will overlap on the two slots and exercise the `gh-pages` and deploy retry for
  real. Watch those runs first.
- **Budget for one cure pass.** That is the harness working.
