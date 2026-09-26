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
| `lint` | self-hosted `[self-hosted, linux, x64, deck]` | `gofmt -l`, `go vet ./...`, `go mod tidy` drift check, in that order, via `ci/lint.sh`. Fails fast, before the suite. |
| `suite` | self-hosted `[self-hosted, linux, x64, deck]`, `needs: lint` | Builds/refreshes the `deck-ci:local` image, runs `ci/run.sh ci/suite.sh` (the whole `go test -p=1 -count=1 ./...` matrix), and on `schedule`/`workflow_dispatch` also runs `ci/stability.sh 3` (three clean-state repetitions) with `-race` enabled. Writes the job summary and uploads raw results as a workflow artifact. |
| `report (Allure)` | self-hosted, `needs: [lint, suite]` | Turns the JUnit `suite` produced into an Allure report, merges it into the persisted `gh-pages` site tree, uploads the rendered report as a workflow artifact, and stages the merged tree as a Pages artifact. Holds no `pages:`/`id-token:` permission -- it stages, it never deploys. |
| `publish (Pages)` | `ubuntu-latest`, `needs: report` | Deploys the root-triggered (`push`/`schedule`/`workflow_dispatch`) Pages artifact. Holds `pages: write`/`id-token: write`. Gated to `push`/`schedule`/`workflow_dispatch` only -- never `pull_request` (a PR-branch deploy is rejected server-side by the repo's `github-pages` environment's branch policy regardless; see "Publishing a PR's own report" below for how a PR's own report still gets published). |
| `pr-comment` | `ubuntu-latest`, `needs: report` | Creates or updates one PR comment (keyed on an HTML marker) carrying the PR's own `/pr/<n>/` Allure link. Only for `pull_request` events whose head repo is this repository. |
| `notify` | self-hosted, `needs: [lint, suite]` | Posts exactly one message to the Telegram notifier when `lint` or `suite` failed/was cancelled, and only for `push` to `main`, `schedule`, or `workflow_dispatch` (never for a PR). |

`release.yml` adds one more gate, described under "The release gate" below.
`pages-pr-publish.yml`, a separate `workflow_run`-triggered workflow, is
described under "Publishing a PR's own report" below.

## Triggers

- `pull_request` -- every job above still runs, but every self-hosted job
  carries the fork guard (next section). Runs on the same ref cancel a
  superseded run (`concurrency.cancel-in-progress` is true only for
  `pull_request`).
- `push` to `main` -- lint, suite, Allure report, and a Pages publish. A red
  run notifies. Never cancelled by another `main`/nightly run (they queue).
- `schedule` (cron `"17 3 * * *"`, an arbitrary off-hour UTC slot chosen to
  avoid GitHub's own top-of-hour cron congestion) -- the nightly lane: the
  same suite, plus `-race` and `ci/stability.sh`, plus a Pages publish. A red
  run notifies.
- `workflow_dispatch` -- manually triggerable, and takes the same nightly
  path (`-race` + stability) as `schedule`.

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
- `main`/`schedule`/`workflow_dispatch` runs publish to the Pages **site
  root**, with Allure's own history/trend carried forward across runs. A
  `pull_request` run instead publishes under `/pr/<number>/`, with no shared
  history, and gets one PR comment (created once, then updated in place)
  carrying that link.
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
- `report` never holds `pages: write`/`id-token: write`; only `publish`
  does, and `publish` runs on `ubuntu-latest` (no self-hosted slot needed to
  make one API call), gated to non-PR events.
- Every run writes a job summary (pass/fail/flaky counts, the slowest
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

## Coverage

- The ordinary Go matrix uses `-coverprofile` directly (legacy text format).
- `features/` drives a built `deck` binary out-of-process, so
  `-coverprofile` alone would report nothing for the code those scenarios
  exercise. The binary is instead built with `go build -cover`, run with
  `GOCOVERDIR` set, and the resulting counters are merged with
  `go tool covdata textfmt` into the same legacy text format, so black-box
  coverage from `features/` counts too.
- `ci/suite.sh` prints a per-package coverage table (`unit` column vs
  `features/ (black-box)` column) plus a `TOTAL` row, and the job summary
  and Allure report both surface the totals. There is no coverage
  threshold gating anything.

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
