# Phase 4d — backlog sweep and real CI

## What the operator gets

Everything open on the backlog except named profiles (#27):

1. **No more false "install tmux" banner** (#36). A pane that vanishes mid-probe is a removal race, not an
   error, and a runtime read error never sticks or gives install advice.
2. **Group headers sit flush left**, level with `socket: deck` (#39).
3. **A wheel scroll stays where you put it** until you press a key that moves or acts on the selection
   (#40). Today the next background reload snaps it back.
4. **A refused attach is impossible to miss** (#38). Every refusal draws one high-visibility banner over
   the preview pane, keys stay live, and the held-elsewhere case points at `F`.
5. **Two mouse shortcuts** (#37): click the passive preview to enter (`↵`), click empty sidebar space to
   leave (`Ctrl+Q`).
6. **Real CI** (#35): the full suite on every trusted PR, every push to `main` and nightly, on two
   self-hosted slots on `int21h-ws`, with an Allure report on GitHub Pages and a release tag gated on
   green.

Each of #35-#40 was filed after the operator discussed it, and each issue body is the detailed design.
**Read the issue before starting its requirement.** Where an issue and this PRD disagree, this PRD wins.
Where either disagrees with `SPEC.md`, SPEC wins, and the disagreement is a finding.

## Why now

v0.2.3 (phase 4c) shipped on 2026-09-25 and #31-#34 are closed. The operator found #36-#40 in field
testing v0.2.3, and wants the backlog cleared before named profiles (#27, deferred to its own phase).
#35 is here because every earlier phase proved its gate only locally: GitHub has no record of whether
any sha is green.

**`SPEC.md` has already been amended for this phase** in an operator commit that lands *before* this
PRD:

- §2's tmux bullet: the install notice is start-up only, a runtime error is transient, and a vanished
  pane is a removal race.
- §11's viewport rule: a wheel drift survives background reloads and ends at the next key that moves
  or acts on the selection.
- §11's header bullet: headers have no gutter, and the header cue is selection background plus reverse
  video.
- §11.3's gutter paragraph: session rows only.
- §11.8: two new table rows and the rewritten passive-preview paragraph.
- §11.9: the refusal banner.
- §13.2: what CI runs.

Nothing here is blocked on the operator, except the §CI prerequisites listed under Tier 3, which are
done before launch.

## Ground rules

- **`SPEC.md` is the authority and is read-only to this job**, as are `prds/`, `ci/Dockerfile` and
  `ci/SPIKE.md`.

  ```sh
  BASE=$(git log --format=%H -1 -- prds/phase4d-backlog-sweep-and-ci.md)   # the operator's last PRD commit
  git log --oneline "$BASE..HEAD" -- SPEC.md prds/ ci/Dockerfile ci/SPIKE.md   # must print nothing
  ```

  The SPEC amendment lands before `$BASE`, so it is outside the audit range by construction. It is
  not this run's commit, and it must not be re-made, extended or "cured".
- **No schema change, no migration.** `schemaV7` is on the operator's live database. A `schemaV8` in
  the tree is blocking.
- **Never touch the operator's live state.** `~/.local/share/deck/state.db` is the operator's real
  session list, and the job never opens, reads or copies it. The same goes for the live `tmux -L deck`
  server, which holds about 25 live agent sessions: never touch it, not even read-only. Every test uses
  its own temp database and a private socket.
- **This is mostly a defect phase, so a test that passes against today's code has proven nothing.**
  - R140-R144 each name an assertion that must **fail on the v0.2.3 tree (`26cdbfd`) and pass after**.
  - Keep phase 4c's probe-audit discipline: run the new tests against the unfixed tree. The evidence
    (the command and the failure it produced) goes to the run's artifacts, never into the repo.
- **Docker is for `ci/run.sh` and for this phase's own CI verification, nothing else.**
  - Never remove or kill containers by label: a previous ralphd job on this host SIGKILLed itself by
    sweeping `label=ralphd.run`.
  - No `docker prune`, and no wildcard `rm`/`rmi`.
  - Never signal by pattern: resolve a pid, verify it, then signal that pid.
  - Other runs live on this host.
- **Code references are pointers, not criteria.** The `file:line` references below were correct at
  `3efce1d`. If one has drifted, find the current site by function name and carry on. Drift is never
  a finding.
- **No paperwork.** Do not add report, findings, audit, close-out or "retake" files to the repo, and
  do not add a `docs/DELIVERY-LOG.md` row. Run evidence (probe audits, sweep logs, CI run URLs, the
  contention measurement) goes to `/run/ralphd/artifacts`. CI evidence is the CI run itself, found by
  its head sha. Do not close GitHub issues either: the operator closes them at release.
- **The CI container has no agent binaries.** Every scenario runs against the `cmd/fake-*` stubs on a
  fixture `PATH`.
- **Scope of GitHub actions.** The git credentials the job holds are the operator's PAT, which has
  `repo` and `workflow` scope. The job may:
  - push to `main`, as always;
  - push throwaway branches named `ci-verify/*` and open and close PRs from them, to verify Tier 3;
  - read the Actions API.

  Every `ci-verify/*` branch, PR, tag and draft release is closed and deleted before the final sweep. The job
  **never changes repository settings**: branch protection, rulesets, the Pages source, Actions
  permissions and runner registration are all operator steps (§Tier 3).

## Scope

- **Tier 1: R140, R141, R142.** Three small, independent fixes with disjoint file sets.
- **Tier 2: R143, then R144.** These are the interactive-entry UX. R144's preview click must show
  R143's banner, so R143 lands first.
- **Tier 3: R145, R146, R147.** CI.
- **R148:** the spec and the code agree.

All of them are acceptance.

## Materiality rubric

Reject only on substance:

- a requirement's behaviour is absent or wrong in the live code;
- a test the requirement names is missing, is red, asserts the opposite of the requirement, or (for
  R140-R144) also passes on the unfixed tree `26cdbfd`;
- the spec contradicts the live code;
- a secret is exposed, in the tree, a workflow log or a published CI report;
- a guard in §Ground rules is broken:
  - a schema change;
  - any access to the operator's `state.db` or `tmux -L deck` server;
  - a protected path modified (the audit command prints anything);
  - a repository setting changed by the job;
  - a self-hosted job reachable by fork-PR code;
  - a real `v*` release tag pushed.

Everything else verifies, with the gap recorded as a residual note. That covers wording, form,
provenance and process, the Allure report's cosmetics (no history on a first run, attachment coverage,
summary layout), and **any number or claim in prose outside the spec** (commit messages, notes, issue
and PR comments, `docs/ci.md`, run artifacts). Prose outside the spec is never a blocking ground.

**Advisory, never blocking:**
- a known flake class recurring in the gate, the sweep or a CI run. The transient-`starting` assertion
  and the `SIGWINCH` exact-count assertion are both known and open;
- a stability sweep below 10/10 whose failures are all known flake classes;
- the CPU-contention measurement (R145) showing that contention matters;
- any operator notification that is missed or late.

## Escape hatch

If the run proves that a requirement contradicts `SPEC.md`, or is impossible as written, it must not
edit the spec or quietly narrow the requirement. File a petition, with the evidence and the smallest
change that would resolve it, notify the operator, and carry on with everything the petition does not
block. The operator rules by amendment. SPEC changes are the operator's commits.

## Notifications

The operator is AFK on Telegram, so use the `notify` hat tool in the same iteration as the work.

Send one message for each of:
- anything that blocks work outright, including a petition;
- each requirement as it lands;
- every rejection and cure pass;
- the first green CI run on `main`, with its Pages link;
- the final sweep result;
- one terminal summary.

Keep each message to a few hundred characters: the notifier refuses a body over 16000 characters
outright. A missed or late send is cured by mentioning it in the next one. It is never a success
criterion and never a finding.

## Tier 1

### R140 — a vanished pane is a race, and a runtime error never sticks (GH #36)

- **Race:** `internal/tmux/tmux.go`'s `sessionDisappeared` (`:691`) / `IsTargetAbsent` (`:704`) match
  `can't find session`/`window` and the no-server family, but not `can't find pane`. Fix that, so the
  probe capture (`internal/service/reconcile.go:138-140`) and the crashed-pane capture (`:174`)
  `continue` as they do for the other removal races.
- **Transient error:**
  - `internal/tui/tui.go:2486` writes any reload error into `m.startupNote`, which nothing ever
    clears.
  - `startupBanner` (`:4701-4706`) prefixes it with `tmux unavailable:` and appends install advice.
  - The fix keeps the start-up tmux note (from `tmuxNote` at construction, `:1729`) separate from
    runtime reload errors:
    - a reload error gets its own short message, `Cannot read sessions: <err>`, with no install line;
    - it clears on the next successful reload.
  - The genuine start-up missing/too-old banner is unchanged (`features/tmux_contract.feature`).
- **Success:**
  - `IsTargetAbsent` is true for `can't find pane: %1` and false for unrelated failures.
  - A real-tmux service test kills a probe-eligible pane between list and capture, and the reconcile
    returns nil with the row's status untouched. This must fail on `26cdbfd`.
  - A TUI test: a reload error, then a successful reload, leaves no banner, and a reload error never
    renders `Install tmux`. Both must fail on `26cdbfd`.
  - The start-up too-old note still renders and persists.

### R141 — group headers start at the first content column (GH #39)

- Headers reserve the rows' 2-column gutter today. `headerSelectionCue` (`internal/tui/group.go:650`)
  supplies it, and `tui.go:5840-5842` builds the header entry with it. After the fix:
  - a header's `sidebarEntry.gutter` is `""`, and `groupHeaderText` gets the full content width;
  - SPEC §11's elision rule is unchanged: the name elides, the chevron and `(n)` never do;
  - session rows are unchanged, gutter included.
- **The header cursor cue** (cure-01-02 / R137's `> ` in the gutter) becomes the `selection`
  background across the header's full inner width **plus reverse video over the header text**, per the
  amended SPEC §11. A header's width and left column must not shift as the cursor moves on or off it.
- Mouse hit-testing follows what is drawn (§11.8: "hit-testing asks the layout what it drew"). A header
  press still folds, in list and interactive mode.
- **Success:**
  - The header chevron is at the panel's first content column, in the same column as `socket:`, in
    colour, `NO_COLOR` and `ascii`. This must fail on `26cdbfd`.
  - The header cursor is visible under `NO_COLOR` (a reverse attribute on its cells) and in colour.
  - Header width is identical with the cursor on and off it.
  - Rows are pixel-identical to today.
  - Rewrite the header-gutter assertions from 4c (`header_selection_cue_test.go`,
    `group_order_test.go` and anything the suite finds) to the new cue. Do not delete them.
  - A header click still folds in both modes.

### R142 — a wheel drift lasts until the user acts (GH #40)

- **Today:**
  - `scrollSidebar` (`internal/tui/mouse.go:193`) moves `m.sidebarScroll`.
  - The sessions reload then calls `followSelectionViewport()` unconditionally (`tui.go:2646`, `:2686`,
    added by cure-01-05), so the wheel only lasts until the next tick.
  - Action keys never follow, so after a drift they would act on an off-screen row.
- **Wheel drift is state.** `scrollSidebar` sets it, and anything that follows the selection clears it.
  - While drifted, a reload, re-sort or re-group keeps `m.sidebarScroll` (clamped to the new list
    length) and does not follow. The selection still tracks its session by id.
  - Without a drift, reloads behave exactly as today. Cure-01-05's follow is intact, and its tests
    stay green unchanged.
- **Every navigation key follows.** These already go through `setSelection` (`tui.go:5918`) and
  `followSelectionViewport`. Prove each one clears the drift.
- **Every session- or header-scoped key follows first, then acts exactly as today.**
  - Any confirmation it raises (`dd`, kill, archive) is raised over a list where the target is
    visible.
  - Hook this into the one shared place that already classifies these keys:
    `internal/tui/session_scoped_guard.go`'s `sessionScopedKeys` / `guardSessionScopedKey` (`:69`).
    Do not add it to each call site. Header-scoped fold keys (`c`, `←`/`→`) are covered too.
  - Keys that name no selection (`?`, `q`, `<`/`>`, settings, filter toggles) leave the drift alone.
- **Operator decision:** an action key acts **on the same press** after bringing the selection back.
  It is not "first press only scrolls".
- **Success:**
  - A wheel drift survives several reload ticks: a unit test, plus a `features/mouse.feature` scenario
    over real reload ticks. This must fail on `26cdbfd`.
  - From a drift, each navigation key and each key in `sessionScopedKeys` brings the selected row
    into view with the usual one-row context. Make this a table-driven test over the map itself, so
    that a key added later is covered automatically.
  - `dd` from a drift shows its confirm with the target row visible.
  - A drifted re-sort keeps the drift and the same selected session.

## Tier 2

### R143 — every entry refusal is a banner over the preview (GH #38)

- **Today** every refusal is a footer line. `m.attachError` is set in
  `internal/tui/interactive.go`'s `enterInteractiveBody` at `:68-296`, including the contention
  refusals at `:127` and `:163`, and by the shrank-below-floor fall-out at `tui.go:2435`.
  `attachErrorLines` (`tui.go:4786`) renders it as wrapped footer text.
- **Build it as a refusal state, not a string.** The state records the refused session's id, a reason
  kind (attached-elsewhere, owned-elsewhere, stopped, row floor, no live pane, shrank, other error)
  and the reason text. It renders as the §11.9 banner:
  - boxed and centred, spanning the preview's width but not its height, with the passive capture
    visible around it;
  - line 1: `NOT ATTACHED: <session> …`;
  - line 2: the reason;
  - line 3: the way out for that kind (`F` and `a` for contention, `a` for the floor, and so on),
    always ending with `keys go to the list`.
- **Render modes:** a warning or error background token in colour; reverse video plus bold under
  `NO_COLOR`; a `+-|` box under `ascii`.
- **Keys are unchanged.** It is not a modal.
- **Lifetime:** the banner clears when:
  - the selection moves;
  - `↵`, `F` or `a` succeeds;
  - a later tick finds the reason gone (the holder left, or the session started);
  - `Esc` is pressed. `Esc` dismisses the banner **before** any other layer it clears, so phase 4c's
    marks-only `Esc` behaviour (2d282e2) keeps its order below it.
- **Non-refusal footer notes** stay footer lines, including copy failures (`interactive_select.go:250`)
  and `Cannot kill`/`archive`/…. They are not entry refusals.
- **Success:**
  - A real-tmux test on a private socket covers both contention holders (another attached client, a
    live foreign ownership claim). `↵` draws the banner with the right reason and the `F` hint.
  - The stopped, row-floor, no-live-pane and shrank cases draw the same banner with their own lines.
  - Golden frames in colour, `NO_COLOR` and `ascii` show the box and attributes.
  - `j` moves the selection and clears the banner. `Esc` clears it first, before marks. The holder
    leaving clears it on the next tick. `F` succeeding clears it.
  - A test asserts that no path setting an entry refusal leaves it footer-only. Make it table-driven
    over the reason kinds.
  - A `features/` two-client scenario: B's `↵` while A holds shows the banner, then `F` enters and the
    banner is gone.
  - Each of these must fail on `26cdbfd`.

### R144 — click the preview to enter, click empty sidebar to leave (GH #37)

- **List mode:** a left press over `hitPanelPreview` (`internal/tui/mouse.go:215`, a deliberate no-op
  today) calls `↵`'s own entry path (`enterInteractive`, `interactive.go:46`) on the **current**
  selection.
  - It never moves the selection.
  - It is inert on a header cursor.
  - It shows R143's banner on refusal.
  - It does not start a drag-to-copy selection. A wheel over the passive preview stays a no-op.
- **Interactive mode:** a press that hit-tests to the sidebar with `hitTargetNone` runs
  `exitInteractive` (`interactive.go:429`, what `Ctrl+Q` runs) and leaves the selection in place.
  - `resolveSidebarPress` (`mouse.go:258`) returns `ok=false` for it today, so the press falls
    through to `beginInteractiveSelection` at `tui.go:4306-4322`.
  - Row, header and strip presses, and presses over the preview or seam, are unchanged.
  - In list mode an empty-sidebar press stays a no-op.
- The help view names both gestures next to `↵` and `Ctrl+Q`.
- **Success:**
  - A preview press in list mode enters, and records the same attachment as `↵` (it answers
    `waiting`).
  - On a stopped or attached-elsewhere session it refuses with R143's banner. It is inert on a header
    and starts no selection.
  - An empty-sidebar press while interactive exits with the geometry restored exactly as `Ctrl+Q`
    restores it.
  - An empty-sidebar press in list mode does nothing.
  - A `features/mouse.feature` scenario: click the preview to enter, then click below the last row to
    return. The DB shows `↵`'s attachment record.
  - Each must fail on `26cdbfd`.

## Tier 3 — CI (GH #35)

**Read #35 in full.** Its design sections 1-8 are the requirement. Summary and deltas:

**Operator prerequisites, already done (2026-09-25):**
- Two runner slots on `int21h-ws`, `deck-ws-1` and `deck-ws-2`, repo-scoped to `n-orlov/deck` and
  labelled `[self-hosted, linux, x64, deck]`.
- GitHub Pages enabled with source "GitHub Actions" (`https://n-orlov.github.io/deck/`).
- Fork-PR approval set to `all_external_contributors`.

The job checks these still hold through the API, and **stops and notifies** if they don't. It never
creates or changes them.

**Operator steps, after the run:** the required-check rule on `main`. The job writes the exact
settings, the check name and a `gh api` command for it into `docs/ci.md` (a how-CI-works page for
maintainers, with no shas or run counts), and does not apply it.
ralphd pushes straight to `main` with the operator's own token, and that must keep working.

### R145 — the suite on trusted PRs, `main` and nightly

- `.github/workflows/ci.yml`. It triggers on:
  - `pull_request` whose head repo is `n-orlov/deck`;
  - `push` to `main`;
  - `schedule` (nightly);
  - `workflow_dispatch`.
- **Fork PRs get no self-hosted job**, through the job-level `if:` in #35 §2. Never use
  `pull_request_target`.
- Token permissions: `contents: read` by default, and `pages: write` / `id-token: write` only on the
  publish job.
- **Lint first, failing fast:** `gofmt -l` prints nothing, then `go vet ./...`, then `go mod tidy`
  leaves the diff empty. The known pre-existing gofmt drift (`internal/theme/quantize_test.go` and
  `.spike-preview/`) is either fixed in this phase or excluded by an explicit, commented path list.
  The gate is never weakened silently.
- **Then the whole suite through `ci/run.sh go test -p=1 -count=1 ./...`**, in the `ci/Dockerfile`
  image built or refreshed on the runner. Tools the image lacks (gotestsum, Allure) are fetched at a
  pinned version by the workflow, and `ci/Dockerfile` is not edited.
- **Retry once, and flakes stay visible.**
  - A test that passes on retry is green, and is marked flaky in the job summary and in Allure.
  - A test that fails twice fails the check.
  - `features/` retries **per scenario**, not per package. Check how `features/godog_test.go` surfaces
    scenarios as subtests, and add a per-scenario rerun if `gotestsum --rerun-fails` can't target
    them.
- `concurrency`: PR runs cancel superseded runs on the same ref, and `main` and nightly never cancel.
- `timeout-minutes` with headroom: about 30 for PR and push runs, and nightly sized to `-race`.
- **A red run on `main` or nightly sends one Telegram message** (`http://100.71.162.65:8090/notify`,
  reachable from `int21h-ws` over the tailnet) with a link to the run.
- **Nightly adds `-race`** (a race report is a failure) and `ci/stability.sh 3`.
- **Success:**
  - A `ci-verify/*` PR runs lint and the suite on a `deck` slot.
  - A deliberately failing test turns it red.
  - A deliberately fail-once test is green and flagged flaky, and a `features/` retry reruns only
    that scenario.
  - The fork guard is shown by the job's `if:` and a run list with no fork jobs.
  - A push to `main` runs green.
  - A `workflow_dispatch` nightly completes `-race` and three stability runs.
  - **Measure contention once:** time one run while ralphd's own CI occupies `int21h-ws`. The duration
    and flake count go to the run's artifacts and the terminal notification. This is advisory
    information (#35 §1).

### R146 — the Allure report and coverage

- **JUnit through gotestsum** for the Go packages.
- **Godog's own JUnit formatter** for `features/` (`pretty,junit:<path>`, with the path from an env var
  and local runs unchanged when it is unset), so each of the ~360 scenarios is its own Allure case.
- **Allure on Pages:** `main` and nightly publish to the site root with history persisted across runs.
  PR runs publish under `/pr/<number>/` and post or update one PR comment with the link.
- **Every run** writes a job summary (pass, fail and flaky counts, the slowest packages, the report
  link) and uploads the raw results plus the rendered report as an artifact.
- **Failure attachments:** attach the pty traces and tmux captures that scenario teardown already
  writes, where practical.
- **Coverage:**
  - `-coverprofile` for the unit packages.
  - For `features/`, the binary built with `go build -cover` and `GOCOVERDIR`, merged with
    `go tool covdata`, so black-box coverage counts.
  - Totals and per-package figures go in the summary and the report. There is no threshold.
- **Success:**
  - The Pages site shows a run from `main` with scenarios as individual cases, and history after two
    or more runs.
  - A `ci-verify/*` PR gets its `/pr/<n>/` link comment.
  - The summary shows coverage including `features/`' black-box figure.

### R147 — a release tag publishes only over a green suite

- `.github/workflows/release.yml`, before building: query `commits/<sha>/check-runs` for the suite
  check on the tagged sha.
  - If there is no run, or it did not conclude `success`, the release **refuses** with a clear message
    naming the sha and what it found.
  - The release job stays on `ubuntu-latest`.
- **Success:** verify it without publishing a real release. Use a throwaway tag, such as
  `v0.0.0-ci-verify-red` on a sha with a red check and `…-green` on a green one, with the release step
  dry-run or immediately deleted, **plus** the refusal branch unit-tested as a script. Delete every
  throwaway tag and draft release.
  - **Never** push a `v0.2.*` or higher tag. Releasing is the operator's call.

## R148 — the spec and the code agree

- The operator's amendment (`03752ca`) states this phase's behaviour. When R140-R147 are done, every
  amended passage must describe the live code: §2's tmux bullet, §11's viewport and header bullets,
  §11.3's gutter paragraph, §11.8's table and passive-preview paragraph, §11.9's refusal banner, and
  §13.2's CI bullet.
- The `?` help view and the footer name the same keys and gestures the spec does.
- A contradiction the run cannot resolve in code goes through the §Escape hatch. It is never an edit to
  `SPEC.md`.
- **Success:** the requirement tests above cover each amended passage's behaviour. Where help text is
  itself the behaviour (R144's gestures), a render test asserts it.

## Ordering

1. **R140**: the smallest change, and a real bug the operator sees today.
2. **R141**: rendering only.
3. **R142**: builds on the shared guard.
4. **R143, then R144.**
5. **R145, R146, R147.** Verify these with `ci-verify/*` branches, then clean them all up.
6. **R148**, checked last, against the finished code.
7. **The final sweep, as the last task, with nothing committed after it.** This is §Definition of done.
   If the sweep turns up a real defect, fix it, push, and run the sweep again. That is the only
   reason to commit after a sweep.

## Definition of done

- R140-R148's behaviours are in the live code, and each requirement's named tests exist and are green.
- For R140-R144, each named test fails on `26cdbfd`. The evidence goes to artifacts.
- **At the final pushed sha:**
  - the GitHub suite check (R145) concluded `success`;
  - `ci/run.sh go test -p=1 -count=1 ./...` is green locally, meaning the whole suite, with no
    narrowed package list and no `-run` filter;
  - `go build ./...`, `go vet ./...` and `gofmt -l` are clean on every file this run touched;
  - the protected-path audit command prints nothing.
- A ten-run stability sweep (`ci/stability.sh 10`) at that sha is complete, with its logs in artifacts.
  A failure in a known flake class is advisory; any other failure is a real defect (see §Ordering 7).
- No `ci-verify/*` branch, PR, tag or draft release remains.
- No secret is exposed.

Nothing else is required. In particular, no report, record or log row goes in the repo.

## Non-goals

- **#27, named profiles.** Not partially, and not "the path groundwork".
- **Phases 5-7** (notifications, shell state, search and health). `internal/notify`, `internal/search`
  and `internal/unit` stay placeholders.
- **Any schema change.**
- **Making the refusal banner modal** or swallowing keys (#38 was explicitly decided against a modal).
- **A wheel that moves the selection.**
- **CI extras:**
  - arm64 or macOS CI jobs;
  - coverage thresholds;
  - changing any test's assertions or deadlines to suit CI hardware (#35's out-of-scope list).
- **Repository settings changes by the job**, of any kind.
- **Publishing a release.**

## For the planner

- **Tier 1 and Tier 2 are close to mechanical, but each has one trap:**
  - R141: the 4c tests that pinned the header gutter must be re-aimed, not deleted.
  - R142: the reload-follow from cure-01-05 must keep working when there is no drift, so the drift is
    a flag, not a removed call.
  - R143: `Esc`'s layering order against 4c's marks-only `Esc`.
  - R144: the entering press must not also start drag-to-copy.
- **R143 before R144, always.** The preview-click refusal is R143's banner.
- **CI verification needs real GitHub runs, and they take about 8 minutes each** on two shared slots.
  Plan for waiting: batch the verification cases (red, flaky, fork-guard, PR comment) into as few
  `ci-verify/*` PRs as possible, and poll the Actions API rather than sleeping in a loop.
- **Your git credentials are the operator's PAT.** Use it for `git push` and `curl` against the API,
  never echo it, and never let a workflow print it. Workflow secrets are not needed: the notifier has
  no auth, and Pages deploys with `GITHUB_TOKEN`.
- **The known-flake classes are not this phase's to fix.** Don't spend iterations on the
  transient-`starting` or `SIGWINCH` assertions. Note them in artifacts and move on.
- **Budget for one cure pass.** Phases 4b and 4c were each rejected once, on curable findings, and one
  pass cleared them. That is the harness working.
