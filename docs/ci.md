# CI: how it works, and what a maintainer needs to touch

deck's CI is the [`Jenkinsfile`](../Jenkinsfile), run by the multibranch job
`deck/ci` on the operator's homelab Jenkins
(`https://jenkins.wisent-owl.ts.net/`, tailnet only); releases are
[`Jenkinsfile.release`](../Jenkinsfile.release), the job `deck/release`. It replaced GitHub Actions in October 2026: there are no workflows, no
GitHub Pages site. For the design and
rationale read GH #35 (CI/CD); for the acceptance criteria,
`prds/phase4d-backlog-sweep-and-ci.md`'s "Tier 3 -- CI" section (written for
the Actions version; the rules carried over, the mechanics did not). This page
never cites a commit sha or a build's pass/fail counts -- look at the Jenkins
build itself.

## The jobs

Both live in the Jenkins folder `deck/`:

- **`deck/ci`**: multibranch, GitHub branch source `n-orlov/deck`, credential
  `github-n-orlov` (a GitHub token kept in OpenBao, read by Jenkins at use
  time). Every branch is discovered by polling (no webhook); tags are not
  built, and there are no pull requests to build, the project takes none.
- **`deck/release`**: a parameterized pipeline job running
  `Jenkinsfile.release` from `main`, started by hand. See "Release" below.
- **Agent:** label `linux && amd64 && docker`, a host where the agent user can
  run Docker. That makes the agent root-equivalent on its host, which is fine
  only because every job on it is trusted code.
- Every build starts from a clean checkout (`git clean -ffdx`); builds of one
  branch never overlap (`disableConcurrentBuilds`).

## Stages

| stage | what it does |
|---|---|
| CI image | `ci/jenkins-image.sh`: `docker build -t deck-ci:local -f ci/Dockerfile ci`, then makes sure the Go cache volume `deck-go-cache-jenkins` belongs to the agent's uid (the image owns `/go-cache` as 1000:1000; the volume is chowned once). Decides whether this is an extended build. |
| Lint | `ci/run.sh ci/lint.sh`: `gofmt -l`, `go vet ./...`, `go mod tidy` drift, then golangci-lint (R188). A red lint stops the build before the suite. |
| Suite | `ci/run.sh ci/suite.sh` -- the whole `go test -p=1 -count=1 ./...` matrix plus `features/`, retry-once (below), JUnit, Allure results and coverage into `ci-results/`. 45 minutes, 180 on an extended build. A failure marks the build red but the later stages still run. |
| Allure smoke | Runs the Godog Allure formatter fixture in the CI image and `ci/allure-smoke.sh` over it (R196): the pinned Allure CLI must still render its steps and `@gh-61` link. |
| Quality | Extended only: drops the cached trivy and govulncheck databases first (R190). Then `ci/run.sh ci/quality.sh ci-results` (R187; every gate in `ci/quality.json` is on). |
| Stability | Extended only: `ci/stability.sh 3`, three clean-state repetitions (R145), on the agent itself because the script starts its own `ci/run.sh` siblings. |

**Extended lane (nightly):** `main` is built every day at 03:17 (`cron`, on
`main` only) in the extended lane, and any build started with the `EXTENDED`
parameter (off by default) is too: it sets `DECK_CI_GO_EXTRA_FLAGS=-race`,
drops the vulnerability databases and runs the Stability stage. Its build
description says "extended".

**Release candidate:** a `main` build that ends `SUCCESS` (not unstable, not
failed) writes and archives `ci-results/release-candidate.json` --
`{"commit": …, "build_url": …, "build_number": …}` -- in `post { success }`,
which runs after the reports, so nothing can turn the build unstable after
the file is written. Only `deck/release` reads it (`copyArtifactPermission`
allows `/deck/release`, and `/deck/ci/*` for the Allure history).

**Telegram:** every build posts one message to the homelab notifier
(`http://100.71.162.65:8090/notify`) through `notifyTelegram` from the homelab
shared library (`@Library('homelab')`, github.com/n-orlov/jenkins-library), in
`post { cleanup }` so it carries the final result. The message has the
result (FIXED after a red build), branch, build link, failed stage, commit,
test counts and duration. Only a red (failed, unstable or aborted) build of
`main`, or a red nightly, makes a sound; other branches and green builds
arrive silently. `deck/release` posts every result, with sound.

## Reports

Everything is on the Jenkins build page, built in `post { always }` so a red
build has them too:

- **Test Result**: the merged JUnit (`ci-results/junit-merged/*.xml`), with
  per-test history. It never marks a build unstable on its own: the Suite stage
  decides.
- **Allure**: `ci/jenkins-report.sh` runs `ci/allure-report.sh` on the agent
  (Allure needs a JRE, which the agent has and `ci/Dockerfile` does not). The
  report's `history/` is archived as `allure-history/` and copied into the next
  build of the same branch (`copyArtifacts`, last completed build), so trends
  carry over per branch. `ci/allure-report.sh` verifies the Allure CLI archive
  against a pinned SHA-256 (changing `ALLURE_VERSION` needs a new
  `ALLURE_SHA256`), downloads with one `curl` attempt, and caches the verified
  archive in `~/.cache/deck-allure` on the agent (`ALLURE_CACHE_DIR`), re-verified
  before every use.
- **CI summary**: `ci/summary.sh`'s counts, flaky tests, slowest packages and
  coverage table, the nightly stability verdicts and the quality gate report.
- **Build page summary line** (badge plugin `addSummary`): pass/fail/skip/flaky
  counts and unit coverage, linking to both reports.
- **README badges** (`embeddable-build-status`): `build` (the plugin's own),
  and `tests`, `coverage`, `quality`, which `post { always }` declares on every
  build with `addEmbeddableBadgeConfiguration` ("not run" until the build fills
  them in). Jenkins stays on the tailnet; the homelab's public nginx proxies
  `https://int21h.mooo.com/jenkins-static/deck/ci/<branch>/<id>.svg` to the
  badge endpoint, because the `deck` folder grants anonymous `Job/ViewStatus`
  (badges only, no build pages). The README shows `main`'s and links nowhere.
- **Artifacts**: the `ci-results/` text, XML and coverage files, the CI summary
  and `allure-history/`.

The HTML reports need Jenkins' default Content-Security-Policy relaxed (Allure
runs JavaScript); the controller is configured that way.

`install.sh` downloads over `--proto =https` only; its `checksums.txt` comes
from the same release as the binary, so the check catches a corrupt download,
not tampering.

## Lint

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
from the repository root, tests included), run by `ci/lint.sh` (the Lint stage,
before the suite) and by `ci/quality.sh`'s `golangci` gate, so the two cannot
disagree. `gofmt`, `go vet` and `go mod tidy` stay only in `ci/lint.sh`
(golangci's formatters are off). `ci/repocheck/lintgate_test.go` fails if
the Lint stage stops reaching the linter, the quality gate is off, or those three
checks appear in a second place; `ci/repocheck/jenkinsfile_test.go` pins the
stage order, the extended lane, the quality gates, the reports, the release
candidate and `Jenkinsfile.release`'s rules.

`go.mod` carries a `toolchain go1.N.P` line matching the `deck-ci` image's Go
(`ci/Dockerfile`), so `go install` and local builds use the patched release
rather than the `go 1.25.0` floor. CI and release builds use the image's Go.
`ci/repocheck` fails if the line is removed or names a floor release; bump it
together with the image's Go. Both `FROM` lines of `ci/Dockerfile` are pinned
by `@sha256` digest (`ci/repocheck/dockerfiledigest_test.go`).

## Retry-once / flaky rule

A test (or, for `features/`, a scenario) that fails once and then passes on
an immediate retry is recorded as **flaky**, not failed: the check stays
green, but the flaky result surfaces in the CI summary and in Allure. A
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
  asserts sub-second tmux/SQLite deadlines, so CPU contention on the agent
  host can also turn into a retry-recovered flake; that contention is
  measured, not eliminated.

## Race builds and wall-clock budgets

SPEC.md §3.1 gives `_hook` a wall-clock budget (store write < 20 ms,
uncontended) that is asserted "for a normal build ... not [on] a `-race`
build", and §13.2 repeats it as one of the things that keeps a red `main`
run a true positive: "a wall-clock budget ... is asserted on normal builds
only, never on the `-race` build". The extended (nightly or `EXTENDED`) build
is the only one that ever runs with `-race` (`DECK_CI_GO_EXTRA_FLAGS`
is set to `-race` only for those; `ci/suite.sh`
switches its `-covermode` to `atomic` automatically whenever that variable
mentions `-race`, and hands the same mode to the features pass as `DECK_FEATURES_COVERMODE` so the black-box deck binary is built with a matching `-covermode`; mixed `set`/`atomic` covdata cannot be merged into `coverage-merged.out`) -- so any test asserting a wall-clock budget has to know,
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
every other build, which never sets `-race`. A *deadline* (a
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
  surfaced in two places: the build's "CI summary" page (`ci/summary.sh`
  cats it under a `#### coverage` heading; its unit `TOTAL` is also on the
  build page's summary line) and the build's archived artifacts
  (`coverage-summary.txt`, both per-pass legacy-format profiles and
  `coverage-merged.out`). The
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
the caller's shell can never reach a gate subprocess. It is the
Jenkinsfile's Quality stage.

`ci/quality.json` is the single thresholds file -- every gate reads its
threshold from here and nowhere else:

```json
{
    "coverage": {
        "enabled": true, "total_floor": 85, "package_floor": 80, "fixture_floor": 50
    },
    "crap": {
        "enabled": true, "ceiling": 10, "fixture_ceiling": 10
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
- `crap` (R191; 30, then 20 under R192, now 10 under R193) is on at ceiling 10 for
  product and `cmd/fake-*` alike, with no allow-list and no exempted function: `ci/quality.sh` runs `go run ./ci/crapgate
  -profile <outdir>/coverage-merged.out -max 10` over the whole module. The gate
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
  (the suite's own generated output, not repo content). Also skipped, by
  exact (glob-escaped) path: whatever git itself reports in the scan target
  as untracked AND ignored (`git ls-files --others --ignored
  --exclude-standard --directory`) -- a checkout's local leftovers such as an
  agent's `.claude/` scratch tree, a `.spike-preview/` clone or `bin/`, which
  a clean CI checkout never has, so the gate's verdict is the same on a
  developer's checkout as in CI. A tracked file is never skipped (even one
  matching an ignore pattern), nor an untracked file git would commit; outside
  a git work tree, or if git cannot run, nothing extra is skipped (the
  stricter scan). The gate fails on an
  empty or wrong scan target (no files, or no `go.mod`) without running
  trivy, because trivy itself exits 0 on an empty directory.
  When trivy's output names a failed vulnerability-database download (a
  mirror 404 or registry outage, which says nothing about the tree), the gate
  tries again, three attempts in all with a 20 s pause; a finding or any other
  failure is never retried, and a database that never arrives still fails the
  gate.
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
- Every gate in the file is on. R187 shipped them all `enabled: false` ("no
  gate on yet"); each was switched on only once the product passed it
  locally, and switching one back off is a loosening the threshold test
  rejects.

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

## Gate reference (R198)

Every gate is run by `ci/quality.sh` (locally: `ci/run.sh ci/quality.sh`,
after `ci/run.sh ci/suite.sh ci-results` has written the merged profile the
coverage and CRAP gates read), and every threshold lives in one file,
`ci/quality.json`. Each on gate prints one `=== <gate> gate ===` section: the
tool's own report (top offenders, each named by `file:line` or module), then a
single `what to do:` line (`nothing -- ...` when it passed). The run exits `1`
if any on gate failed. To read a failure: find the first `=== ... gate ===`
section whose body is not `what to do: nothing`, fix what it names, and re-run
that one tool (the commands below) rather than the whole suite.

Where the report appears: `ci/quality.sh` prints the report on stdout (so a
local run shows exactly the text CI shows) and also writes the same text to
`<suite-outdir>/quality-report.txt` (`ci-results/quality-report.txt`). That file
is what crosses the `ci/run.sh` sibling boundary: the sibling bind-mounts the
checkout, so `ci/jenkins-report.sh` (in the Jenkinsfile's `post { always }`)
reads it back and puts it on the build's "CI summary" page, whether the gates
passed or failed. The Quality stage wraps the gates in `catchError`, so a red
gate fails the build without stopping the later stages or the reports. A run
that died before printing any section leaves a one-line report saying so. The
file is also archived with the build. `ci/qualitycheck`'s
`TestQualityShWritesReport*` fail if the script stops writing it.

| Gate | Threshold (`ci/quality.json`) | A failure names | Run it alone |
| --- | --- | --- | --- |
| coverage | `coverage.total_floor` 85, `package_floor` 80, `fixture_floor` 50 | each package or total under its floor, with percentage and statement counts | `go run ./ci/covgate -config ci/quality.json -profile ci-results/coverage-merged.out` |
| CRAP | `crap.ceiling` and `crap.fixture_ceiling` | each function over the ceiling: `file:line`, `cc`, `cov`, `CRAP` | `go run ./ci/crapgate -profile ci-results/coverage-merged.out -max 10` |
| golangci-lint | none: zero findings | each finding, as golangci prints it | `ci/golangci.sh` |
| govulncheck | none: no reachable vulnerability | the vulnerable module/standard-library symbol and its fixed version | `go run ./ci/qualitycheck -config ci/quality.json -profile ci-results/coverage-merged.out` with only `govulncheck` on |
| trivy | `trivy.severity` `HIGH,CRITICAL` | each vulnerability, secret or misconfiguration with its fixed version, or a bad `.trivyignore` line | same, with only `trivy` on |

All five run through `ci/run.sh` in the `deck-ci` image so local and CI results
agree; the coverage and CRAP gates are only meaningful on the merged unit +
features profile, never a unit-only one.

**The CRAP stage that is on.** The ceiling is ratcheted 30 -> 20 -> 15 -> 10
(R191-R193), one stage per commit, each only once the whole tree was clean at it.
The ceiling `ci/quality.json` holds at this commit: **CRAP ceiling 10** (the
fixture ceiling is 10 as well). `TestDocsCiMdStatesTheCheckedInCrapCeiling`
(`ci/qualitycheck`) fails if this paragraph's number differs from
`ci/quality.json`, so changing one means changing the other in the same commit.

**Adopting or tightening a ratchet.**

- Tightening: measure first (run the gate alone at the lower number over the
  merged profile of a green build, or the `tighten:` prompt for a coverage
  floor), cure every named offender at its root (lift a helper, add a behaviour
  test), and only then lower the number in `ci/quality.json`, in the same
  commit that updates this file and renames the ceiling test. If the next stage
  cannot be reached cleanly, stay at the last clean stage and list what is left;
  a half-done stage is never committed as on.
- Adopting a new gate: add it to `ci/quality.json` with `enabled: false`, make
  the product pass it locally, then switch it on. A seeded-failure test proves
  it fails when it should.
- Loosening is refused: `TestThresholdsNotLoosened` fails the build when a floor
  drops, a ceiling rises or a gate goes from on to off against the base branch.
  There is no override; a ratchet only moves toward stricter.

**Exceptions policy.**

- `.trivyignore` is the only place a trivy finding may be waived: one line per
  exception, `<ID> review-by:YYYY-MM-DD # <reason>`. A missing reason or date,
  or a date already passed, fails the gate; waivers are for findings with no
  fixed version, and the date forces a re-review. There are none at present.
- `//nolint` is allowed only as `//nolint:<linter> // <reason>` (nolintlint
  enforces both parts) on the single line concerned, for a case where the lint
  is wrong for that line (for example `gosec` G304 on a path that is a
  command-line argument by design). No file- or package-wide exclusion is
  added for production code, and no function is exempted from the CRAP or
  coverage gates.

**Native Allure results (R195-R196).** `features/` writes native Allure 2
results when `DECK_GODOG_ALLURE=<dir>` is set (feature, scenario and one step
per Gherkin step, with status, duration and attachments); `ci/suite.sh` sets it
for the `TestFeatures` run and every solo rerun into one
`<outdir>/allure-results/`, the unit tests are converted into the same
directory by `ci/junit2allure`, and `ci/allure-report.sh` builds the single
report with its history carried forward. The Allure smoke check and where the
report lives are described under "Reports" above.

## The pre-release security review (R194)

This is a process, not a gate, and nothing about a review is tracked in the
repository beyond this section.

- **When:** before a release tag is cut, the operator asks Claude Code for a
  security review of everything since the last reviewed tag.
- **How:** separate review agents cover the hook input (`deck _hook` stdin),
  tmux command construction and quoting, the filesystem and permissions under
  `DECK_HOME`, env and secret handling, the Jenkinsfile and the build
  agent's trust boundary, and dependencies. Each finding is adversarially
  verified before it is reported.
- **Output:** each confirmed finding becomes a GitHub issue, a short summary
  goes to Telegram, and the release's tag message or notes carry "security
  review: <range>, N findings, all resolved/filed".
- **Blocking:** a confirmed HIGH or CRITICAL finding blocks the release. Lower
  findings are filed and do not block.
- **Rejected alternatives:** the scheduled `claude-code-security-review` Action
  (it is PR-oriented and needs a model API key stored as a repo secret); a
  periodic ralphd security job (costlier than asking when a release is near);
  and a session cron (it fires only while a session is alive).

## Release

Run `deck/release` with **Build with Parameters**:

| parameter | meaning |
|---|---|
| `VERSION` | required, `vMAJOR.MINOR.PATCH` with an optional `-pre` suffix (`^v\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$`); the tag must not exist yet |
| `CI_BUILD` | the `deck/ci` `main` build to release; empty means the last successful one |
| `DRY_RUN` | build and archive everything, but do not publish |

Stages (`Jenkinsfile.release`):

1. **Candidate** -- copies `ci-results/release-candidate.json` from
   `deck/ci/main` (build `CI_BUILD`, or the last stable one). A build without
   that file was not fully green, and the release stops there.
2. **Checkout** -- checks out `main`, makes sure the candidate commit is in its
   history, and detaches at that commit. Everything after this runs that
   commit's own scripts and `ci/Dockerfile`.
3. **Version check** -- `git ls-remote --exit-code --tags` on GitHub (with the
   `github-n-orlov` token, never echoed): the tag must not exist, and a lookup
   that fails stops the release rather than passing it.
4. **Build** -- `ci/run.sh ci/release-build.sh "$VERSION"` in the CI image
   builds the four archives (`deck_{linux,darwin}_{amd64,arm64}.tar.gz`,
   `CGO_ENABLED=0`, `-trimpath`, the version stamped into `main.version`) and
   `checksums.txt`, and smoke-checks that `deck --version` prints `deck
   <version> linux/amd64`.
5. **Publish** (skipped on `DRY_RUN`) -- `gh release create "$VERSION" dist/*
   --repo n-orlov/deck --target <commit> --title "$VERSION" --generate-notes`.
   GitHub creates the tag at that commit together with the release; no tag is
   pushed from the agent.

The build is named after the version, archives `dist/*`, and its summary
names the version, the commit, the `deck/ci` build and the release link.
Because the job runs `Jenkinsfile.release` from `main` but builds an older
commit, that commit must already have `ci/release-build.sh` and
`ci/jenkins-image.sh` (true for anything built by `deck/ci`).

**Integrity check.** Releases carry no build attestation (GitHub's needs an
Actions OIDC token). `install.sh` and `deck update` verify the archive's
SHA-256 checksum against the release's `checksums.txt` and nothing else; a
mismatch aborts with nothing installed (SPEC.md, "Update download").

**Operator step -- protect the `v*` tags.** A published release's tag must
never move or disappear. The release job never applies this; the maintainer runs it once with an account
that administers the repository. It blocks deleting, force-moving or updating
any `v*` tag, with the repository admin role allowed to bypass:

```sh
gh api \
  --method POST \
  -H "Accept: application/vnd.github+json" \
  -H "X-GitHub-Api-Version: 2022-11-28" \
  repos/n-orlov/deck/rulesets \
  --input - <<'JSON'
{
  "name": "protect-release-tags",
  "target": "tag",
  "enforcement": "active",
  "conditions": {
    "ref_name": { "include": ["refs/tags/v*"], "exclude": [] }
  },
  "rules": [
    { "type": "deletion" },
    { "type": "non_fast_forward" },
    { "type": "update" }
  ],
  "bypass_actors": [
    { "actor_id": 5, "actor_type": "RepositoryRole", "bypass_mode": "always" }
  ]
}
JSON
```

`actor_id` 5 is the built-in repository admin role. Check the result with `gh api
repos/n-orlov/deck/rulesets`.
