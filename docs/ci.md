# CI: how it works, and what a maintainer needs to touch

This is the maintainer-facing walkthrough of `.github/workflows/ci.yml` and
`.github/workflows/release.yml`. For the underlying design and rationale, read
GH #35 (CI/CD) and GH #37/#40 as applicable; for the acceptance criteria, read
`prds/phase4d-backlog-sweep-and-ci.md`'s "Tier 3 -- CI" section. This page
never cites a commit sha or a specific run's pass/fail counts -- for those,
look at the Actions run itself, found by its head sha.

## Lanes (jobs in `ci.yml`)

| job | runs on | purpose |
|---|---|---|
| `lint` | self-hosted `[self-hosted, linux, x64, deck]` | `gofmt -l`, `go vet ./...`, `go mod tidy` drift check, then golangci-lint (R188), in that order, via `ci/lint.sh`. Fails fast, before the suite. |
| `suite` | self-hosted `[self-hosted, linux, x64, deck]`, `needs: lint` | Builds/refreshes the `deck-ci:local` image, runs `ci/run.sh ci/suite.sh` (the whole `go test -p=1 -count=1 ./...` matrix), then, on `schedule`/`workflow_dispatch` only, drops the cached trivy and govulncheck databases (R190's nightly rescan: both scanners then run through `ci/quality.sh` on fresh data, and a newly disclosed vulnerability fails the job, which `notify` reports), then `ci/run.sh ci/quality.sh` (R187's quality gates; coverage, trivy, govulncheck and golangci are on, the rest `enabled: false` until a later task flips them on), and on `schedule`/`workflow_dispatch` also runs `ci/stability.sh 3` (three clean-state repetitions) with `-race` enabled. The job fails if the suite, the quality gates, or (nightly) stability failed. Writes the job summary and uploads raw results as a workflow artifact. |
| `report (Allure)` | self-hosted, `needs: [lint, suite]` | Turns the JUnit `suite` produced into an Allure report, merges it into the persisted `gh-pages` site tree, uploads the rendered report as a workflow artifact, and stages the merged tree as a Pages artifact. Holds no `pages:`/`id-token:` permission -- it stages, it never deploys. |
| `publish (Pages)` | `ubuntu-latest`, `needs: report` | Deploys the root-triggered (`push`/`schedule`/`workflow_dispatch`) Pages artifact. Holds `pages: write`/`id-token: write`. Gated to `push`/`schedule`/`workflow_dispatch` only -- never `pull_request` (a PR-branch deploy is rejected server-side by the repo's `github-pages` environment's branch policy regardless; see "Publishing a PR's own report" below for how a PR's own report still gets published). |
| `pr-comment` | `ubuntu-latest`, `needs: report` | Runs `go run ./ci/prcomment`, which pages through every existing PR comment and creates or updates the one comment (keyed on an HTML marker) carrying the PR's own `/pr/<n>/` Allure link and its head sha. Only for `pull_request` events whose head repo is this repository. |
| `notify` | self-hosted, `needs: [lint, suite]` | Posts exactly one message to the Telegram notifier when `lint` or `suite` failed/was cancelled, and only for `push` to `main`, `schedule`, or `workflow_dispatch` (never for a PR). |

`ci/allure-report.sh` verifies the Allure CLI archive it downloads against a
pinned SHA-256 and aborts on a mismatch (changing `ALLURE_VERSION` needs a new
`ALLURE_SHA256`). `install.sh` downloads over `--proto =https` only; its
`checksums.txt` comes from the same release as the binary, so the check catches
a corrupt download, not tampering.

`.golangci.yml` (golangci-lint v2, the version pinned in `ci/Dockerfile`) is the
lint configuration (R188): the `standard` linters plus gosec, revive, gocritic,
errorlint, misspell, unconvert, unparam, bodyclose, copyloopvar and nolintlint
(`require-specific`, `require-explanation`); gocognit is not enabled, CRAP is the
complexity gate. Its only exclusions are two `_test.go`-scoped rules (gosec
G204/G304/G301/G302/G306, and errcheck on `Close`), each with its reason in the
file; production code has none, and golangci's default skip of files with a
"generated" header is turned off (`exclusions.generated: disable`).
`ci/lintcheck/golangci_test.go` runs the real binary to prove a seeded unchecked
error (also under a generated-file header), a `//nolint` without a linter name or
a reason, and an empty package list each fail. It is a required gate at zero
findings: `ci/golangci.sh` is the one invocation (`golangci-lint run ./...`
from the repository root, tests included), run by `ci/lint.sh` (the `lint` job,
before the suite) and by `ci/quality.sh`'s `golangci` gate, so the two cannot
disagree. `gofmt`, `go vet` and `go mod tidy` stay only in `ci/lint.sh`
(golangci's formatters are off). `ci/workflowcheck/lintgate_test.go` fails if
the lint job stops reaching the linter, the quality gate is off, or those three
checks appear in a second place.

`go.mod` carries a `toolchain go1.25.N` line (matching the `deck-ci` image's
Go, `ci/Dockerfile`). `actions/setup-go` with `go-version-file: go.mod`
prefers that line over the `go 1.25.0` language floor, so the `setup-go` steps
in `ci.yml` and `release.yml` install the patched toolchain instead of 1.25.0
(unless `GOTOOLCHAIN=local` is set in the step). `ci/workflowcheck` fails if
the line is removed or a `ci.yml` `setup-go` step could resolve to 1.25.0;
bump the line together with the image's Go.

Workflow hardening (R197, GH #61): every `uses:` line in `ci.yml` and
`pages-pr-publish.yml` is pinned by a 40-hex commit SHA with the tag in a
trailing comment (bump both together), `ci.yml`'s notify step reads
`github.ref_name` through `env:` rather than interpolating it into the script,
and the `report` job drops the `GITHUB_TOKEN` extraheader from
`site/.git/config` (an `if: always()` step) once the gh-pages push is done.
`ci/workflowcheck/hardening_test.go` fails if any of the three regresses.

`release.yml` adds one more gate, described under "The release gate" below.
`pages-pr-publish.yml`, a separate `workflow_run`-triggered workflow, is
described under "Publishing a PR's own report" below.

## Triggers

`ci.yml` itself runs for four events (`on:` in the workflow file); a fifth
run -- `pages-pr-publish.yml` -- is a *second, separate* workflow reacting to
a `ci` run's own completion, described in full under "Publishing a PR's own
report" below.

- `pull_request` -- every `ci.yml` job above still runs, but every
  self-hosted job carries the fork guard (next section); `publish (Pages)`
  still never runs (gated to `push`/`schedule`/`workflow_dispatch` only).
  On completion of this run, `pages-pr-publish.yml` fires (its
  `on.workflow_run.branches-ignore: [main]` matches, because a
  `pull_request` run's head branch is the PR's own branch, never `main`) and
  does the PR's own deploy.
- `push` to `main` -- lint, suite, Allure report, and a Pages publish. A red
  run notifies. On completion of this run, `pages-pr-publish.yml`'s
  `workflow_run` trigger does *not* fire a publish run: its
  `branches-ignore: [main]` filter matches on the *triggering* `ci` run's own
  head branch, and a `push`-to-`main` run's head branch is `main` -- so a
  `main` run leaves no `pages-pr-publish` run behind at all (not even a
  skipped one; the workflow simply never starts), which is exactly what lets
  `main`'s own Actions history stay a run-for-run record of `ci.yml` alone
  (SPEC.md §13.2, "Main's Actions history is a truthful signal").
- `schedule` (cron `"17 3 * * *"`, an arbitrary off-hour UTC slot chosen to
  avoid GitHub's own top-of-hour cron congestion) -- the nightly lane: the
  same suite, plus `-race` and `ci/stability.sh`, plus a Pages publish. A red
  run notifies. Its head branch is also `main`, so it likewise starts no
  `pages-pr-publish` run.
- `workflow_dispatch` -- manually triggerable, and takes the same nightly
  path (`-race` + stability) as `schedule`. Also runs with `main` as its head
  branch, so it too starts no `pages-pr-publish` run.

### Concurrency groups and what actually gets cancelled

`ci.yml`'s workflow-level `concurrency:` block:

```
group: ci-${{ github.workflow }}-${{ github.event_name }}-${{ github.event_name == 'push' && github.sha || (github.event_name == 'pull_request' && github.ref || github.run_id) }}
cancel-in-progress: ${{ github.event_name == 'pull_request' }}
```

folds in `github.event_name`, then a per-event tiebreaker: `github.sha` for
`push`, `github.ref` for `pull_request`, and `github.run_id` -- unique to
every single workflow run -- for everything else (`schedule` and
`workflow_dispatch`). That makes every non-PR run's own group unique to
that run:

- Every `push` run's group is unique to its own commit sha
  (`ci-ci-push-<sha>`), so two pushes to `main` are always two independent
  groups: pushing again never queues behind, and never cancels, an earlier
  push's own still-running result.
- Every `schedule` run's group, and every `workflow_dispatch` run's group,
  is unique to that one run (`ci-ci-schedule-<run_id>`,
  `ci-ci-workflow_dispatch-<run_id>`) -- **not** shared with any other
  `schedule` or `workflow_dispatch` run, including a second dispatch fired
  against the very same sha as the first. An earlier design keyed those two
  lanes on `github.ref` alone (constant across every run of the same
  event), which put every `schedule` run in one shared group and every
  `workflow_dispatch` run in another: GitHub keeps only **one pending** run
  per group and cancels the older pending run the moment a newer one
  queues into that same group, so two back-to-back nightly runs, or two
  manual dispatches queued close together, could still cancel each
  other's pending run under that design -- it did not, in fact, "queue
  every dispatch to completion". Keying on `github.run_id` instead removes
  the shared group entirely, so no `schedule`/`workflow_dispatch` run can
  ever be the pending run a same-lane sibling supersedes: each one runs to
  completion (or fails outright) independently, exactly like `push`.
  Neither lane's group is ever shared with `push`'s, or with the other's.
- Every `pull_request` run's group is unique to its own PR
  (`github.ref` is that PR's merge ref, the one context this expression
  still keys on a value that repeats across runs), so a second run on the
  *same* PR shares that PR's group with the first -- which is what lets
  `cancel-in-progress` below actually cancel the superseded one.

`cancel-in-progress` is `true` only when `github.event_name == 'pull_request'`
-- so a superseded run on the same PR is the only run this workflow ever
cancels. A `push`, `schedule`, or `workflow_dispatch` run is never cancelled
by another run of any kind -- each one's group is unique to itself, so
there is never another run in the same group to queue behind, wait for, or
be superseded by; it can still fail outright (a red `suite`, or the job's
own `timeout-minutes`), which is a different outcome from `cancelled`.

## The fork guard

deck is a public repository, so a self-hosted runner (`int21h-ws`, slots
`deck-ws-1`/`deck-ws-2`, labelled `[self-hosted, linux, x64, deck]`) would
otherwise execute arbitrary code from any fork's pull request. Every
self-hosted job in `ci.yml` repeats the same job-level `if:`:

```
github.event_name != 'pull_request' || github.event.pull_request.head.repo.full_name == github.repository
```

For any event that is not `pull_request`, `github.event.pull_request` is
null and the right-hand side would evaluate false, so the `!=` short-circuit
is required to avoid wrongly refusing `push`/`schedule`/`workflow_dispatch`
runs. For a `pull_request` event, the check passes only when the PR's head
repository is `n-orlov/deck` itself -- a fork PR's head repo never matches,
so it gets no self-hosted job at all, only the two `ubuntu-latest` jobs
(`publish` never runs for `pull_request` regardless; `pr-comment` carries the
same repo-identity check a second time).

This guard is deliberately a job-level `if:`, not a workflow-level `on:`
filter, because job-level `if:` cannot read the workflow's own top-level
`env:` -- hence it is inlined on every self-hosted job rather than defined
once. `pull_request_target` (which would check out and evaluate workflow YAML
from the base ref while still running against the PR head) is never used
here; the ordinary `pull_request` trigger checks out and evaluates the PR's
own head YAML, which is exactly what keeps a fork's code off the self-hosted
runners in the first place.

As defence in depth, the repository also requires approval for all outside
contributors before their workflow runs at all (Settings -> Actions -> Fork
pull request workflows -> "Require approval for all outside collaborators").
Nothing in the workflow YAML can substitute for that repository setting, and
this job never re-applies it -- it only checks the setting still holds
through the API and stops and notifies if it does not.

## Retry-once / flaky rule

A test (or, for `features/`, a scenario) that fails once and then passes on
an immediate retry is recorded as **flaky**, not failed: the check stays
green, but the flaky result surfaces in the job summary and in Allure. A
test that fails twice in a row fails the check.

- The ordinary Go package matrix (`ci/suite.sh`, pass 1) uses gotestsum's own
  `--rerun-fails=1`, which reruns exactly the failed test function.
- `features/` (pass 2) is the single Go test function `TestFeatures`, which
  fans out into every Godog scenario via `godog.TestSuite`. Handing that to
  `--rerun-fails` would rerun the *entire* package on any one scenario's
  failure, so `ci/suite.sh` instead: runs `TestFeatures` once, parses Godog's
  own pretty-formatter "Failed steps" summary for each failed scenario's
  `<file>:<line>`, and reruns only that scenario
  (`DECK_GODOG_PATHS=<file>:<line>`). A scenario that passes on that solo
  rerun is flaky; one that fails again fails the check.
- `ci/junitflaky` then folds each retried-but-ultimately-passing test into
  one passing JUnit `<testcase>` carrying its earlier failures as
  `<rerunFailure>`/`<rerunError>` -- the Allure JUnit plugin's own flaky
  convention -- so a retry-recovered test reads as flaky in the report
  rather than as an ordinary failure.
- Two known flake classes are advisory, not blocking, and are never cured by
  loosening a test's own assertion or deadline: a transient-`starting`
  timing assertion, and a `SIGWINCH` exact-count assertion. `features/`
  asserts sub-second tmux/SQLite deadlines, so CPU contention on the runner
  host can also turn into a retry-recovered flake; that contention is
  measured, not eliminated.

## Allure report and Pages layout

- Both passes produce JUnit: the ordinary Go matrix through gotestsum
  (`--junitfile`), `features/` through Godog's own JUnit formatter
  (`pretty,junit:<path>`), so each scenario is its own Allure test case
  rather than one opaque `TestFeatures`.
- `features/` can also write native Allure 2 results: with
  `DECK_GODOG_ALLURE=<dir>` set, `godogFormat()` adds the in-repo `allure`
  formatter (`features/allure_formatter_test.go`, standard library plus Godog
  only) beside pretty and JUnit, writing `*-result.json`, one
  `*-container.json` per feature file and attachment files into `<dir>`. The
  feature is the Allure `feature`, the scenario the test and each Gherkin step
  an Allure step (verbatim text, own status and duration). `@gh-NN` becomes an
  issue link to `https://github.com/n-orlov/deck/issues/NN`, `@multiclient`/
  `@slow`/`@nightly` become `tag` labels and `@claude`/`@pi`/`@codex` the
  `suite` (the feature name moving to `subSuite`); every result carries
  `parentSuite` `features`. A failing step carries only what the harness already writes
  into its error text (last normalized pty frame, tmux captures, deck-log
  slice, store dump), cut out by its section headers; nothing is collected
  anew. The `historyId` is a digest of feature file, scenario name (and, for an
  outline, its substituted steps), so the one-retry rerun of a failed
  scenario (`DECK_GODOG_PATHS=<file>:<line>`) into the same directory is a
  retry of the same test, not a second one.
- `ci/suite.sh` sets `DECK_GODOG_ALLURE` on the `TestFeatures` run and on
  every solo scenario rerun, all into one `<outdir>/allure-results/`: a
  rerun carries the failed attempt's `historyId`, so it is a retry of that
  test, never a second one. The same directory gets the Go unit tests
  converted from the merged JUnit by `ci/junit2allure` (`parentSuite` `unit`,
  the `historyId` Allure's JUnit plugin used, so unit trends carry over; an
  earlier failed attempt is a hidden retry and the test is marked flaky),
  `environment.properties` (Go version, tmux version, sha, `race=yes|no`) and
  `executor.json` (the Actions run link; the workflow passes `DECK_CI_SHA` and
  `DECK_CI_RUN_URL` because `ci/run.sh` forwards only `DECK_*`). If no native
  features results exist (the process died before the formatter flushed), the
  merged features JUnit is converted instead, so a failed pass is never
  missing; the synthetic "aborted" marker is converted either way.
  `ci/allure-report.sh` builds the one report from that directory (a results
  dir without it falls back to the merged JUnit). `ci/summary.sh` still reads
  the merged JUnit, but counts it the way the report does: one test per
  (testsuite, classname, name) with its last attempt's outcome, so a test
  that failed on every attempt is one failure and a skipped test is a skip
  (its own row), never a pass. `ci/suitecheck` proves its pass/fail/skip/
  flaky rows equal the Allure results' on a fixture, and fails when they
  differ.
- Allure smoke check (R196): before the real report is built, the `report`
  job runs the formatter's own fixture
  (`TestAllureFormatterWritesResultsForAFixtureRun`, kept with
  `DECK_ALLURE_FIXTURE_OUT`) in the CI image, then `ci/allure-smoke.sh`
  runs the pinned Allure's `allure generate` over it (through
  `ci/allure-report.sh`, same checksum-verified download) and fails if
  generation fails or the generated report lacks the fixture's steps or the
  `@gh-61` issue link. These are steps of the existing `report` job, not a
  job of their own (`ci/workflowcheck` guards that); a bumped Allure version
  or a formatter change that Allure cannot render turns this red before
  anything is published.
- The features results changed `historyId` (the JUnit-derived id to the
  formatter's digest), so the features trend restarted once when this landed;
  unit trends continue. The root-publish history dir still carries over.
- `main`/`schedule`/`workflow_dispatch` runs publish to the Pages **site
  root**, with Allure's own history/trend carried forward across runs. A
  `pull_request` run instead publishes under `/pr/<number>/`, with no shared
  history, and gets one PR comment (created once, then updated in place)
  carrying that link.
- The PR comment itself is created/updated by `go run ./ci/prcomment`
  (`ci/prcomment/main.go`), not an inline `actions/github-script` step: it
  pages through *every* existing issue comment on the PR (`per_page=100`,
  following the `Link: rel="next"` header, the same pattern
  `ci/releasegate` uses for check-runs/actions-runs) before deciding
  whether to update the first comment whose body carries the
  `<!-- deck-allure-report -->` marker or create a fresh one when no page
  holds it. A single-page lookup (GitHub's own default `per_page` is 30)
  would stop seeing an existing marker comment once a PR accumulated
  enough other comments, and would then append a duplicate on every
  subsequent run instead of updating the one it already posted.
- History and the accumulated multi-report tree live on a `gh-pages` branch,
  which the `report` job reads back (for history) and writes forward (the
  merged tree) on every run. This is a plain data branch this workflow reads
  and writes -- it is never the Pages *source*, which stays "GitHub Actions"
  (`build_type=workflow`) throughout.
  `actions/deploy-pages` always replaces the *whole* Pages site with
  whatever artifact it is handed, so a main-triggered deploy would erase a
  live PR preview (and vice versa) unless every deploy's own artifact
  already contains the full merged tree -- which is exactly what the
  `report` job assembles before `publish` ever runs.
- `report` never pushes to `gh-pages` with a plain, unretried `git push`:
  the merge-and-push step (`ci.yml`'s "Merge this run's report into the
  persisted site tree and push it to gh-pages") delegates to
  `ci/pages-persist.sh <site> <report dir> root|pr <n> [max attempts]`
  (added for R165, GH #50 -- see that script's own header comment). Two
  reports can legitimately race the same `gh-pages` tip (a push to `main`
  and an open PR's own preview, or two PRs), so a non-fast-forward push
  rejection is an expected occasional outcome, not a defect: the script
  re-fetches `gh-pages`, re-merges this run's own report on top of the
  newer tree (via `ci/allure-site.sh`, re-staging the report fresh on every
  attempt), and retries, up to a bounded number of attempts (default 5),
  before giving up and failing the step.
- `report` never holds `pages: write`/`id-token: write`; only `publish`
  does, and `publish` runs on `ubuntu-latest` (no self-hosted slot needed to
  make one API call), gated to non-PR events.
- Every run writes a job summary (pass/fail/skip/flaky counts, the slowest
  packages, coverage totals, the report link) and uploads both the raw
  results and the rendered report as workflow artifacts, independent of
  whether the report ever reaches Pages.
- Failure attachments (pty traces, tmux captures already written by
  scenario teardown) are attached to their Allure case where practical.

### Publishing a PR's own report (R146, GH #35 §5)

- `report`'s `pull_request` run merges `/pr/<n>/` into `gh-pages` and pushes
  it, but `ci.yml`'s `publish` job never runs for `pull_request` at all --
  the repository's `github-pages` deployment environment restricts
  deployment to `main` (a pre-existing setting neither workflow is ever
  allowed to change), so a deploy attempted from inside a `pull_request`
  run's own `github.ref` (the PR's head ref) would be rejected server-side
  regardless of anything this job declares.
- `.github/workflows/pages-pr-publish.yml` is a separate workflow, triggered
  by `workflow_run` on `ci`'s own completion, that does the deploy instead.
  `workflow_run` always executes using the default branch's copy of the
  triggered workflow file, with `github.ref` set to that default branch
  regardless of what triggered the run it reacts to -- so it satisfies the
  same "main only" branch policy for free, and it fires automatically the
  moment the PR's own `ci` run completes: caused by, not unrelated to, that
  run, and requiring no later main push, nightly run or manual dispatch.
- `pages-pr-publish.yml`'s own `on.workflow_run.branches-ignore: [main]`
  (R163, GH #50) excludes a `main`-headed `ci` run from ever triggering this
  workflow at all -- a `push`/`schedule`/`workflow_dispatch` run already
  published synchronously via `ci.yml`'s own `publish` job in that same run,
  so a second, redundant `pages-pr-publish` run would otherwise start (and
  immediately no-op past its own `gate` job's `event == 'pull_request'`
  check) on every single `main` run; filtering it at the trigger means no
  such run appears in the Actions history at all, not even a fast
  skipped/no-op one.
- Its `gate` job calls the Jobs API for the completed run and only proceeds
  when that run's own `report (Allure)` job succeeded (not the run's overall
  conclusion, which can be `failure` on a red `suite` even though `report`
  still ran) and the run's head repository is this one (a second, belt-and-
  suspenders fork guard on top of `report`'s own, which already never runs
  -- so never succeeds -- for a fork PR). Its `publish` job then checks out
  `gh-pages` (already carrying the merged tree `report` pushed), re-stages
  it as a fresh Pages artifact, and deploys it -- never touching the PR's
  own head ref/commit, so it carries none of the fork-PR code-execution risk
  the self-hosted fork guard above exists to keep off a fork's head.
- Both this job's own `actions/deploy-pages` call and `ci.yml`'s own
  `publish` job's call retry the same way (R166, widened for R165, GH #50):
  up to **four** attempts, with back-off sleeps of 30s/60s/120s (210s of
  total back-off) between them. Every attempt but the last carries
  `continue-on-error: true`, so a failed attempt's own `outcome` (unlike
  `conclusion`, never overridden by `continue-on-error`) stays readable by
  the next attempt's `if:`; the final attempt omits it, so a persistent
  failure still fails the job. `environment.url` falls back through
  whichever attempt actually produced a `page_url` output
  (`deployment4 || deployment3 || deployment2 || deployment1`). The back-off
  was widened from three attempts/45s to cover GitHub's Pages API "one
  concurrent build per repository" limit specifically: since `ci.yml`'s own
  concurrency groups now let push/schedule/dispatch run side by side (see
  "Concurrency groups" above), and this workflow's own deploy runs in an
  entirely separate workflow from `ci.yml`'s (so `ci.yml`'s concurrency
  group never serializes against it), two `publish`-style jobs calling
  `actions/deploy-pages` against the same `github-pages` environment at the
  same moment is an expected occasional case, not a rare fluke.

## Race builds and wall-clock budgets

SPEC.md §3.1 gives `_hook` a wall-clock budget (store write < 20 ms,
uncontended) that is asserted "for a normal build ... not [on] a `-race`
build", and §13.2 repeats it as one of the things that keeps a red `main`
run a true positive: "a wall-clock budget ... is asserted on normal builds
only, never on the `-race` build". The nightly/`workflow_dispatch` path
above is the only path that ever runs with `-race` (`DECK_CI_GO_EXTRA_FLAGS`
is set to `-race` only for `schedule`/`workflow_dispatch`; `ci/suite.sh`
switches its `-covermode` to `atomic` automatically whenever that variable
mentions `-race`) -- so any test asserting a wall-clock budget has to know,
from inside the test binary itself, whether it is presently running under
`-race`, without CI having to pass it anything extra.

`internal/racebuild` is that signal: a build-tagged package exposing one
constant, `racebuild.Enabled` -- `false` in `racebuild_norace.go` (build tag
`!race`), `true` in `racebuild_race.go` (build tag `race`). A budget
assertion consults it directly (for example, `features/`'s
`hookStoreDurationBelow` and `sessionsCreatedGapWithinBudget` helpers) and
skips the wall-clock comparison whenever it is `true`, rather than loosening
the budget's own numeric threshold -- so the nightly `-race` run, several
times slower by design, never turns a real budget into a flaky or
permanently-red assertion, and the assertion still runs at full strength on
every `push`/`pull_request` run, which never sets `-race`. A *deadline* (a
test waiting on an external, polled event, e.g. a pty frame or a process
exit) is a different kind of assertion from a *budget* (a bound on this
binary's own uncontended work) and is never exempted the budget's way --
skipping the comparison outright would let the awaited condition go
unchecked. Instead a race build only WIDENS how long a deadline polls for
the condition to become true; it never skips the wait or weakens what is
being checked. `features/`'s `reconcileIntervalPollDeadline` and
`r150LiveOuterDeadline` helpers consult `racebuild.Enabled` this way, so
both budget assertions and race-widened deadlines consult
`racebuild.Enabled` -- the two differ in what they do with it (skip the
comparison vs. widen the bound), not in whether they consult it at all.

## Coverage

- The ordinary Go matrix does not use `-coverprofile` directly. gotestsum's
  own `--rerun-fails=1` reruns a failed test with a *second*, separate `go
  test` invocation, and each `go test` call -- retry included -- writes a
  `-coverprofile` path from scratch rather than merging into whatever is
  already there; the moment any test anywhere needed a retry, that retry's
  own invocation would clobber the whole profile with only its own (one
  package, one test) coverage, deleting every other package's data. So the
  unit pass instead runs with `-cover` and `-args -test.gocoverdir=<dir>`:
  every invocation (the initial run and every gotestsum retry) writes its
  own uniquely-named counter files into that one shared directory, so
  counters from every attempt -- including packages/tests that were never
  retried at all -- accumulate instead of clobbering each other.
- `features/` drives a built `deck` binary out-of-process, so neither
  `-coverprofile` nor `-cover` on the `go test` invocation itself would
  report anything for the code those scenarios exercise. The binary is
  instead built with `go build -cover` and run with `GOCOVERDIR` set to its
  own counter directory, for the same retry-safe reason: every scenario's
  spawned deck process, and every solo scenario rerun, gets its own
  uniquely-named counter files in that directory rather than overwriting a
  shared one.
- Both counter directories are converted with `go tool covdata textfmt`
  into the same legacy text format `go tool cover` understands, so one
  small package-by-package aggregator can read both without needing two
  unrelated summarizers.
- The unit pass also runs with `-coverpkg=./...`, so its counters cover
  every package in the module, not only the package each test lives in.
  Both counter directories are then merged by one `go tool covdata textfmt
  -i=<unit>,<features>` call into `<outdir>/coverage-merged.out`, and
  `ci/suite.sh` exits non-zero when that merged profile has no blocks.
- `ci/suite.sh [<outdir>]` takes its output directory as an optional first
  argument (`DECK_CI_OUT` is still honoured when it is absent). After
  reading its own `DECK_CI_*` settings (including `DECK_CI_GO_EXTRA_FLAGS`,
  captured first so `-race` still reaches both passes), it unsets every
  `DECK_*` variable in its environment before launching `go test`, so a
  leaked `DECK_GODOG_PATHS`/`DECK_HOME` from the caller's shell never
  reaches the tests; the `DECK_GODOG_*` values the script sets itself are
  passed per invocation.
- `ci/suite.sh` prints a per-package coverage table (`unit` column vs
  `features/ (black-box)` column) plus a `TOTAL` row. That table is
  surfaced in exactly two places: the job summary (`ci/summary.sh` cats it
  under a `#### coverage` heading) and the uploaded raw results artifact
  (`ci-results-<run>`, which carries the whole results directory --
  `coverage-summary.txt`, both per-pass legacy-format profiles and `coverage-merged.out` included). The
  Allure report does not display it. The coverage gate (`ci/covgate`, R189,
  below) is what gates on it.

## CRAP scoring (`ci/crapgate`, R187)

`go run ./ci/crapgate -profile <coverprofile> -max <ceiling> [-filter
<pkg-dir>[:<file,...>]]` is a stdlib-only (no `golang.org/x/tools/cover`, no
third-party module of any kind) CRAP scorer. `ci/quality.sh`'s crap gate
(below) drives it over the whole module; its own rules are locked by its
own table tests (`ci/run.sh go test -count=1 ./ci/crapgate/`):

- McCabe complexity via `go/ast`: `cc = 1 + if + for + range + case +
  comm-case + && + ||`. A `default:`/`case` with no `List` (the `select`
  equivalent, a `CommClause` with a nil `Comm`) adds nothing. A closure
  (`FuncLit`) defined inside a named function is walked into, not skipped --
  its branches count into the enclosing function's score, and a closure is
  never scored as a function of its own.
- `CRAP = cc² × (1 − cov)³ + cc`. A function with zero top-level statements
  (an empty body) is defined as 100% covered, so its CRAP collapses to its
  own `cc` regardless of what the profile does or does not say about it.
- Coverage per function comes from the profile's own blocks (the same
  `name:startLine.startCol,endLine.endCol numStmt count` format `go test
  -coverprofile`/`go tool covdata textfmt` both emit), summed over whichever
  blocks fall inside that function's line range. A function present in
  source whose file has no matching block anywhere in the profile at all
  (never built under the profile's own `-coverpkg`/package selection) is
  scored at 0% -- never silently skipped.
- `-filter` takes either a bare package directory (every non-test `.go` file
  directly inside it) or `<pkg-dir>:<file,...>` (only the named files); with
  no `-filter` the whole module (found by walking up from the working
  directory for `go.mod`) is scanned.
- Exit codes: `0` nothing over `-max`; `1` one or more functions over it,
  named in the report by `file:line`, `cc`, `cov` and `CRAP`; `2` a
  usage/input error -- a missing or empty profile, or a scan that found zero
  scored functions (a typo'd `-filter` must never read as a clean bill of
  health).

## The coverage gate (`ci/covgate`, R189)

`go run ./ci/covgate -config <ci/quality.json> -profile <merged coverprofile>`
is a stdlib-only scorer over the merged unit + features profile
(`coverage-merged.out`), counting statements (a block listed twice counts once,
covered if any run covered it):

- the **product total** must reach `coverage.total_floor` (85) and **every
  product package** must reach `coverage.package_floor` (80). The `ci/*` Go
  tools count as product; the `cmd/fake-*` fixtures are left out of the product
  total and each must reach `coverage.fixture_floor` (50) instead;
- a package is a directory with a non-test `.go` file (hidden and `_` dirs,
  `vendor`, `testdata`, `ci-results` and nested modules are not part of the
  set). A package with no statements in the profile fails -- a package that
  was never built under the profile must never read as covered -- unless it is
  named, by exact path, in `zeroStatementPackages` in `ci/covgate/main.go`.
  That commented list holds `internal/racebuild` (constants only) and the
  doc-only `internal/notify`, `internal/search`, `internal/unit`. Naming a
  package that does have statements, or whose source is gone, fails too, so
  the list cannot become a hiding place; it is never a glob;
- a package (or the total) that beats its floor by 1 pp or more is named in a
  `tighten:` prompt: raise that floor in `ci/quality.json`;
- exit codes: `0` every floor met, `1` a floor missed (each miss named with its
  percentage and statement counts), `2` bad input -- a missing, empty or
  mode-only profile, a floor outside (0, 100], or a profile with no product
  statements.

`ci/covgate`'s own tests seed a 79% package, an 84% total and an empty profile
and require each to fail; `ci/qualitycheck`'s tests drive the on gate over this
repository's real package set.

## The quality gates (`ci/quality.sh`, `ci/quality.json`, R187)

`ci/quality.sh [<suite-outdir>] [<config-path>]` is the one entry point for
every quality gate (`ci/run.sh ci/quality.sh`, defaulting `<suite-outdir>` to
`ci-results` -- the same directory `ci/suite.sh ci-results` writes
`coverage-merged.out` into -- and `<config-path>` to the checked-in
`ci/quality.json`). Like `ci/suite.sh`, it scrubs every `DECK_*` variable
from its own environment before running anything, so a variable leaked from
the caller's shell can never reach a gate subprocess. It is wired into
`ci.yml` as a step in the existing `lint`/`suite` jobs, not a new lane.

`ci/quality.json` is the single thresholds file -- every gate reads its
threshold from here and nowhere else:

```json
{
    "coverage": {
        "enabled": true, "total_floor": 85, "package_floor": 80, "fixture_floor": 50
    },
    "crap": {
        "enabled": true, "ceiling": 30, "fixture_ceiling": 30
    },
    "trivy": {
        "enabled": true, "severity": "HIGH,CRITICAL"
    },
    "govulncheck": {
        "enabled": true
    },
    "golangci": {
        "enabled": true
    }
}
```

- `coverage` (R189) is on: `ci/quality.sh` runs `go run ./ci/covgate -config
  ci/quality.json -profile <outdir>/coverage-merged.out`. See "The coverage
  gate" below for what it scores. Floors never go down (the loosening test
  covers the three floors and the flag).
- `crap` (R191) is on at ceiling 30 for product and `cmd/fake-*` alike, with no
  allow-list and no exempted function: `ci/quality.sh` runs `go run ./ci/crapgate
  -profile <outdir>/coverage-merged.out -max 30` over the whole module. The gate
  is only valid on the merged unit + features profile (a unit-only profile
  scores every feature-covered function low). Its own tests seed a function
  over the checked-in ceiling and require the gate to fail and name it.
- `crap.ceiling` is the CRAP gate's ceiling (`ci/crapgate -max`), ratcheted
  30 -> 20 -> 15 -> 10 by R191-R193. `crap.fixture_ceiling` is carried as
  its own field even though R187 sets it equal to `ceiling` ("the same CRAP
  ceiling as product") -- a separate field is what lets a loosening test
  catch either one being loosened on its own. The gate itself always scans
  the whole module in one pass (no `-filter`), so the two thresholds only
  diverge in the config, never in enforcement, for as long as they stay
  equal.
- `trivy` (R190) runs `trivy fs` over the repository root with
  `--scanners vuln,secret,misconfig`, `--severity HIGH,CRITICAL`,
  `--ignore-unfixed` and `--exit-code 1`. `severity` must keep both `HIGH`
  and `CRITICAL` (the gate refuses anything else, and the loosening test
  fails if a level is dropped or the gate switched off). The DB cache is
  `/go-cache/trivy` (the existing volume, so a warm run is fast); outside the
  image `ci/quality.sh` falls back to a temp dir, and `TRIVY_CACHE_DIR`
  overrides both. Skipped, by exact path with the reason in
  `ci/qualitycheck/trivy.go`: the 11 MB stability log
  `docs/reports/phase3g-812-stability10/summary.log`, and `ci-results/`
  (the suite's own generated output, not repo content). The gate fails on an
  empty or wrong scan target (no files, or no `go.mod`) without running
  trivy, because trivy itself exits 0 on an empty directory.
  Exceptions live only in `.trivyignore`, one per line, in the form
  `<ID> review-by:YYYY-MM-DD # <reason>`: a line with no reason, no date, or a
  date before today (UTC) fails the gate and
  `TestTrivyIgnore_CheckedInFileIsValid`. `golang.org/x/text` is at v0.39.0
  (fixing CVE-2026-56852), so there are no exceptions at present.
- `govulncheck` (R190) runs `govulncheck ./...` from the repository root with
  `GOTOOLCHAIN=local` (the image's pinned binary, v1.8.0), failing on a
  vulnerability whose vulnerable function the code actually calls (exit 3);
  an imported-but-uncalled finding does not fail it. Two guards keep it from
  passing vacuously: the running `go version` must not be older than
  `go.mod`'s `toolchain` line (the `go` line when there is none), and the
  target must hold a `go.mod` and Go files -- an empty or wrong target fails
  without running govulncheck. Its vulnerability-database cache follows
  `XDG_CACHE_HOME`, which `ci/quality.sh` points at `/go-cache/xdg-cache` in
  the image. Switching it off is a loosening the threshold test rejects. Its
  tests (`ci/qualitycheck/govulncheck_test.go`) run the real govulncheck
  against a vendored vulnerable-module fixture with a local `file://`
  database, so they need no network.
- `golangci` (R188) runs `ci/golangci.sh`, the same single invocation as
  `ci/lint.sh`'s last stage, and has no threshold: any finding, a missing
  script or a linter that cannot run fails it. Switching it off is a loosening
  the threshold test rejects (`ci/qualitycheck/golangci_test.go`).
- The other gates start `enabled: false` (R187: "no gate on yet"). A later
  task flips one on only once the product passes it locally.

`ci/qualitycheck` (`go run ./ci/qualitycheck -config <path> -profile
<path>`) does the actual gate work behind `ci/quality.sh`: it reads
`ci/quality.json`, runs every gate whose flag is on, prints one `=== <gate>
gate ===` report section per on gate (top offenders, then a "what to do"
line), and exits non-zero if any on gate failed -- `0` if every on gate
passed (or none are on), `1` if at least one failed, `2` on a usage/input
error (a missing/malformed config, or a profile a gate needed but did not
get). Its own tests (`ci/run.sh go test -count=1 ./ci/qualitycheck/`) drive
the real `ci/quality.sh` end to end: one seeds a failing crap gate (a
ceiling of 0 against a profile with no real coverage data, so every scanned
function in the whole repo is over it) and asserts the non-zero exit; another
seeds a `DECK_*` variable and a `go` wrapper on `PATH` that records every
`DECK_*` variable any invocation of `go` sees, and fails if that log is ever
non-empty.

**The loosening test (`TestThresholdsNotLoosened`, task 085, R187).** The
same package's `TestThresholdsNotLoosened` compares the checked-in
`ci/quality.json` against the base-branch copy, resolved exactly as R187
specifies: `git merge-base HEAD origin/main`, falling back to `HEAD~1` on a
shallow clone, with a clear error naming both failures if even that
fallback cannot resolve a ref. It fails if any floor dropped, any ceiling
rose, or a gate switched from on to off relative to base; tightening (or an
unchanged copy) always passes. The comparison and the "resolve the base
copy" step are split apart (`looserThresholds`, `checkThresholdsNotLoosened`,
`loadBaseConfig`/`resolveBaseRef`) so a dedicated unit test can stub the
resolver to simulate "no base copy resolves" and assert the result is a
returned error, never a skip -- there is no code path in
`checkThresholdsNotLoosened` that calls `t.Skip`. Other unit tests seed a
loosened copy (a lower floor, a raised ceiling, a gate switched off) against
a fixed base and assert the comparison fails, and a tightened copy against
the same base and assert it passes.

## The release gate (`release.yml`, R147)

Before `release.yml` builds or publishes anything for a pushed `vX.Y.Z` tag,
its "Gate on green CI" step runs `go run ./ci/releasegate -repo
"$GITHUB_REPOSITORY" -sha "$GITHUB_SHA"`. That program queries `GET
/repos/{owner}/{repo}/commits/{sha}/check-runs` for the check run named
`suite` (the default; overridable with `-check`), and `GET
/repos/{owner}/{repo}/actions/runs?head_sha={sha}` to learn, for each of
those check runs, which workflow run -- and hence which triggering event
(`push`, `pull_request`, `schedule`, `workflow_dispatch`, ...) -- produced
it. Per SPEC §13.2/R147, only a `suite` run whose triggering event is
`push` or `pull_request` ever gates a release: a nightly `schedule` run and
a manual `workflow_dispatch` run may alert on red, but neither ever blocks
a release for a sha whose own push (or PR) run went green, and a green
schedule/workflow_dispatch run never substitutes for a missing push/PR one
either way. Among the gating (push/pull_request) `suite` runs, the most
recently started one decides, and the gate refuses -- non-zero exit, a
message naming the sha and exactly what it found -- unless that run's
`status` is `completed` and its `conclusion` is `success`. No gating run at
all (whether because there is no `suite` run yet, or because every `suite`
run on the sha belongs to a non-gating event), a gating run still
queued/in-progress, or a completed gating run that concluded anything other
than `success`, all refuse. This is what makes a tag's own release build
depend on the *tagged commit's own* push/PR CI history, not on `main`'s
current HEAD, and not on any nightly/dispatch run against that same commit
-- tagging a commit that predates the gate itself runs that old commit's
`release.yml`, which has no gate at all, so the gate protects only tags on
commits at or after the commit that introduced it.

## The required-check rule on `main` (an operator step, never applied by this job)

CI (`ci.yml`'s `suite` check) is meant to gate merges into `main`, but this
job **never applies branch protection or a ruleset itself** -- changing
repository settings is out of scope for every task in this phase, and is
the operator's own step to take after this phase lands. This section states
the exact setting and command so the operator does not have to reverse
engineer it, without ever running it in this job's own name.

**The one constraint that shapes the setting:** ralphd pushes directly to
`main` (not through a pull request) using the operator's own personal
access token, and that must keep working. GitHub only enforces "required
status checks must pass" against **merges** (the PR merge button, or the
merge API) when "Require a pull request before merging" is itself enabled;
a required-status-checks rule with no pull-request requirement, and with
"Include administrators" left **off**, never blocks a plain `git push`
by an actor with admin/bypass permission on the repository (the operator's
own account, which is what ralphd's token authenticates as) -- it only
blocks the PR merge path a non-admin contributor would otherwise use to
land a red commit. So the rule below intentionally sets `enforce_admins:
false` and leaves `required_pull_request_reviews`/`restrictions` unset:
turning either of those *on* instead would make an ordinary contributor's
PR wait for a green `suite` check before it can merge, while still leaving
ralphd's own direct push to `main` unaffected.

**Check name, exactly as the Checks API reports it:** `suite` -- `ci.yml`'s
`suite` job carries no separate `name:` override beyond its own job id, so
GitHub reports its check run under that literal string. This is the same
name `ci/releasegate`'s own `-check` flag defaults to, so the release gate
(R147, above) and the required-check rule name the identical check.

**Settings to apply** (classic branch protection, `PUT
/repos/{owner}/{repo}/branches/{branch}/protection`):

- `required_status_checks.checks`: `[{"context": "suite"}]` -- only the
  `suite` check is required; `lint` failing already prevents `suite` from
  ever reaching `success` (a job-level `needs:` skips it), so requiring
  `suite` alone is sufficient without separately naming `lint`.
- `required_status_checks.strict`: `false` -- a PR branch does not have to
  be rebased onto the latest `main` before its own `suite` run counts;
  this only affects PR merges, never direct pushes either way.
- `enforce_admins`: `false` -- see the constraint above: this is what keeps
  ralphd's (and the operator's) own direct pushes to `main` unaffected.
- `required_pull_request_reviews`: `null` -- no review requirement is added
  by this rule; adding one would also gate the merge path, but never a
  direct push.
- `restrictions`: `null` -- no push-restriction allow-list; leaving this
  unset is what keeps a direct push from any current admin/write actor
  working exactly as it does today.

Applied with the GitHub CLI:

```sh
gh api \
  --method PUT \
  -H "Accept: application/vnd.github+json" \
  -H "X-GitHub-Api-Version: 2022-11-28" \
  repos/n-orlov/deck/branches/main/protection \
  --input - <<'JSON'
{
  "required_status_checks": {
    "strict": false,
    "checks": [
      { "context": "suite" }
    ]
  },
  "enforce_admins": false,
  "required_pull_request_reviews": null,
  "restrictions": null
}
JSON
```

This job never runs the command above against the live repository -- it is
documentation for the operator's own step. `GET
/repos/{owner}/{repo}/branches/main/protection` and `GET
/repos/{owner}/{repo}/rulesets` both show no protection and no ruleset on
`main` as of this writing; applying the command turns the first of those
from a 404 ("Branch not protected") into the settings above, and never
touches the second.
