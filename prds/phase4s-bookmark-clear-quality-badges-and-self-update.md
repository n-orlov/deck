# Phase 4s — clearing bookmarks (#92), quality-gate badges (#91) and self-update (#86)

## What the operator gets

1. A bookmark slot can be emptied: `Alt+M` then `Delete` clears the current session's bookmark, and archiving a
   session clears its bookmarks as deleting already does (#92).
2. The README opens with a quality-gate badge block whose values are real CI gate results, regenerated on every
   CI run on `main` and never on a PR (#91).
3. `deck update` installs the latest release with the same integrity checks as `install.sh`; deck shows an
   unobtrusive update-available indicator; `ctrl+u` installs from the UI; an opt-in `auto_update` setting does it
   in the background (#86).

## Issues in scope

#92, #91 and #86, nothing else. Read each in full, including every comment; this PRD wins wherever it differs.
**Not in scope:** #73 (Kiro), native Windows for `deck update`, pre-release channels, a v2 Bubble Tea migration,
and any change to how bookmarks are assigned or jumped to.

Code to follow (symbols as of `6fa0fbd0d4a`; drift is expected and is not a finding — if a reference has
drifted, find the current site and carry on):

- Part A: `internal/store/bookmark.go` (`ClearBookmark`, the archive and delete writes), the `Alt+M` chord code in
  `internal/tui/` (the pending-chord state, prompt text, cancel-on-any-key rule of R275/R276/R279), the
  interactive-mode key interception next to Ctrl+Q in `internal/tui/interactive.go`, footer and `?` help.
- Part B: `.github/workflows/ci.yml` (`report` and `publish` jobs, the `gh-pages` tree), `README.md`, `ci/`.
- Part C: `install.sh` (the trust model to reuse), `cmd/deck/version.go` (`buildVersion`), the top-level verb
  dispatch in `cmd/deck/` (`--version` is answered before config and tmux checks), `internal/config/schema.go`
  (`settingsApplyLiveFields`, `ui.attach_on_click` as the live-apply pattern), `internal/tui/list_keys.go`
  (`listKeyHandlers`), the footer/status line in `internal/tui/tui.go`, `internal/store/` (`ui_state`),
  SPEC §3, §4, §6.5, §11.4, §11.5.

## Requirements

### Part A — #92 clearing bookmarks

- **R285 (store).** Archiving a session clears all of its bookmark slots in the same write that archives it;
  un-archive and undo do not restore them. `Store.ClearBookmark` on an empty slot is not an error. Tests: archive
  clears a two-slot session in one write and leaves other sessions' slots alone; clearing an empty slot returns
  nil; un-archive leaves the slots empty. Each fails at the parent.
- **R286 (clear chord).** With `bookmarks` on, `Alt+M` then `Delete` clears every slot held by the session the
  chord was started on, and only that session's: the list cursor's session in list mode, the attached session in
  interactive mode. A one-line message names the slot(s) (`N` or `N,M`). With the setting off nothing is bound
  and nothing is mentioned. Finite cases, each a table-test row: list mode, one slot; list mode, two slots;
  interactive mode, one slot; another session's slot left alone; setting off.
- **R287 (conditional prompt and fall-through).** The chord prompt reads `Press Delete to remove bookmark N`
  (`N,M` for two) only when the session the chord started on holds a bookmark; otherwise the prompt is unchanged
  and `Delete` is not offered. `Delete` after `Alt+M` on an unbookmarked session cancels the chord and is handled
  normally (R275/R276/R279); every other non-digit key still cancels. A stray `Delete` outside a chord behaves as
  before. In interactive mode a `Delete` that completes the chord is intercepted and never forwarded to the pane;
  one that does not complete it is forwarded as today. One test per case.
- **R288 (surface and spec).** Footer and `?` list the clear action while `bookmarks` is on. A `features/`
  scenario covers clear and a second covers archive-clears. SPEC §4 (archive clears) and §11 (chord, prompt,
  footer) state this in present tense, in the commits that change the behaviour.

### Part B — #91 quality-gate badges

- **R289 (generator).** A script under `ci/` (shell or Go; the run chooses) emits `badges/<name>.json` files in
  the shields.io `endpoint` schema (`schemaVersion`, `label`, `message`, `color`) from the real gate results CI
  already produces: lint, unit tests, features (e2e), coverage, and vulnerability audit where CI has one. It
  takes the gate results as input files, never as hardcoded values. Finite cases, table-test rows: gate passed
  (green), gate failed (red), result file missing or unparsable (grey, message `unknown`), a coverage number
  below and above each threshold the script defines. A missing or red gate never renders green.
- **R290 (publish only from main).** `ci.yml` runs the generator and writes `badges/*.json` into the `gh-pages`
  tree through the existing `report`/`publish` path, **only** for `push` to `main` (and `workflow_dispatch` on
  `main`), never on `pull_request` runs. A Go test parses `ci.yml` and asserts the badge steps carry that
  condition and that no PR-triggered job writes `badges/`. CI stays green.
- **R291 (README block).** `README.md` opens with a block between `<!-- badges:start -->` / `<!-- badges:end -->`
  generated by a script (`ci/` or `scripts/`): workflow-status badges for `ci.yml` on `main` (and `release.yml`
  if useful), shields.io endpoint badges reading the published `badges/*.json`, and static badges for license,
  Go version and a link to the published report. Each badge links to its workflow or the report. A test runs the
  generator and fails if the committed block differs from its output, and a test asserts the Go-version badge
  matches `go.mod`.
- **R292 (spec).** The one place the repo documents its quality gates (`ci.md` / `docs/ci.md`, whichever exists)
  states where badges come from and that they are regenerated on `main` only. No SPEC change unless behaviour
  of the product changes.

### Part C — #86 self-update

Decisions in the issue are binding: check default on, throttled to once per 24h; latest stable only
(`releases/latest`); `auto_update` default off; first outbound network use, documented as the only one; `dev`
builds never self-update; `ctrl+u` is provisional and is confirmed against the keymap first.

- **R293 (core).** A package (for example `internal/update`) resolves the latest stable release tag, downloads
  `deck_<os>_<arch>.tar.gz` and `checksums.txt`, verifies sha256, extracts the binary, writes it next to the
  current executable (resolved through symlinks) and `rename(2)`s it over the current one. The base URL is
  overridable by `DECK_UPDATE_URL` (documented in SPEC §13.1) so every test points at a local fake release
  server; no test touches the network. HTTPS-only is enforced except for the loopback override. Finite cases,
  each a test: success replaces the binary and a failure at any step leaves the old binary byte-identical;
  asset-name table for linux/darwin x amd64/arm64; an unsupported OS/arch is a clear error; a tampered archive,
  a checksum mismatch and a missing checksums entry each abort with nothing replaced.
- **R294 (attestation).** With an authenticated `gh` present the update runs
  `gh attestation verify --repo n-orlov/deck` on the archive and a failure aborts with nothing replaced;
  without `gh`, it proceeds on the checksum alone unless `DECK_REQUIRE_ATTESTATION=1`, which turns "cannot
  verify" into an abort. Download prefers an authenticated `gh`, otherwise plain HTTPS. Tests use a fake `gh` on
  `PATH` for: verify passes; verify fails (abort); `gh` absent; `gh` absent with the require variable (abort).
- **R295 (refusals).** The update refuses, with a message and no change, when: the version is a `dev` build; the
  binary sits under a managed prefix (a finite list named in SPEC: for example `/usr`, `/opt/homebrew`,
  `/home/linuxbrew`, a Nix store path — the run states the list); the install directory is not writable (the
  message names the path and both fixes: re-run with sudo, or reinstall to `~/.local/bin` via
  `DECK_INSTALL_DIR`). It never escalates privileges. One test per case.
- **R296 (verb).** `deck update` installs the latest release; `deck update --check` reports whether a newer one
  exists and changes nothing (no file, no state row); `deck update --version vX.Y.Z` pins a release and permits a
  downgrade. The verb is answered before the config and tmux checks, like `--version`. It prints old and new
  version and a note that a running deck must be restarted; failure prints a clear error and exits non-zero.
  An offline `--check` is a clear error with no state change. Tests drive the real binary against the fake
  server: install, `--check` newer, `--check` current, `--version` downgrade, offline, each failure above exits
  non-zero. Scenarios in `features/` cover install and `--check`.
- **R297 (settings and cache).** Config keys `update_check` (bool, default true) and `auto_update` (bool,
  default false; true implies `update_check`), global scope, in the schema with env overrides like their
  siblings, applied live on reload with the `attach_on_click` pattern, and a main-menu toggle for each. The check
  is one HTTPS request for the latest tag, run off the UI goroutine, throttled to once per 24h with the result
  and timestamp cached in one place shared by every profile (the run chooses and states it in SPEC §4), so it
  never runs per launch or per profile. A config reload never writes. Tests: throttle (second check within 24h
  makes no request; after 24h makes one), `update_check=false` makes none, offline or a failed check is silent
  (no indicator, no error, no retry within the window), the `auto_update`-implies-check rule, live apply.
- **R298 (indicator).** The footer/status line shows, unobtrusively and in the R280 note slot's neighbourhood
  without adding a row: **Available** (`auto_update` off) `update available: vX.Y.Z — press ctrl+u to install`;
  **Installed** (after an auto or manual update) `update installed: vX.Y.Z — restart deck to apply`, which
  replaces the available text and persists until deck restarts; **Failed** `update failed — see deck update`
  naming the cause. Hidden when disabled, offline, current, or a `dev` build. When the binary is unwritable or
  package-managed the indicator names the reason instead of offering `ctrl+u`. Fits at 80x24 by dropping whole
  legend entries like the other footer text. One table-test row per state, at 80 columns and wide, in list and
  interactive mode; the frame's line count is identical with the indicator shown and hidden.
- **R299 (ctrl+u).** While the indicator reads Available, `ctrl+u` opens a confirmation showing current and
  target versions, then runs the same verified update as the verb in the background with a progress state; the
  UI stays responsive. Success flips the indicator to Installed; failure shows the error and the old binary
  stays. The binding is inert (and unlisted) in every other state and when the install is unwritable or managed.
  `ctrl+u` keeps its line-editing meaning in text fields and is not forwarded or stolen in interactive mode; it
  is documented in the footer/`?` help (R7). It never restarts deck and never touches sessions. Tests: confirm
  then install, cancel does nothing, inert cases, no collision (a test asserts no other list binding uses the
  key), text-field behaviour unchanged.
- **R300 (auto-update).** With `auto_update` on, the daily check that finds a newer release installs it through
  the same verified path with no prompt and flips the indicator to Installed; any failure leaves the old binary
  and shows the Failed notice quietly. Skipped, with the Available indicator shown instead, when the binary is
  unwritable, package-managed or a `dev` build. With it off nothing is installed without `deck update` or
  `ctrl+u`. Tests with the fake server for each branch.
- **R301 (live sessions).** An update does not disturb live agent sessions. Two scenarios or integration tests,
  one per direction: an old TUI running while the hooks of live sessions invoke the new binary (the atomic
  rename means the next hook run is the new one), and a new TUI over sessions whose hooks still embed the old
  one. Both leave every session's status and the state database consistent, using the existing schema-newer
  re-exec and `writer_binary` mechanisms; no new mechanism unless a test proves one is needed.
- **R302 (spec and docs).** SPEC §3/§4/§6.5/§11 state the verb, settings, indicator, `ctrl+u`, `DECK_UPDATE_URL`
  and the refusals in present tense, in the commits that change the behaviour. SPEC and `README.md` state that the
  update check is deck's only network use, what it sends (the request itself, no telemetry) and how to disable it
  (`update_check=false`).

Pre-split into tasks at plan time: R285; R286+R287; R288; R289; R290; R291+R292; R293; R294; R295; R296; R297;
R298; R299; R300; R301; R302 is satisfied inside each task, not as a docs-only task; then the three sweep tasks.

## Materiality rubric

Reject only on substance:

- a requirement's behaviour is absent or wrong in the live code;
- a test the requirement names is missing, red, or asserts the opposite of the requirement;
- the spec contradicts the live code;
- a secret is exposed.

Everything else verifies, with the gap recorded as a residual note: wording, form, provenance, process, and
**any number or claim in prose outside the spec** (commit messages, notes, issue comments, run artifacts). Prose
outside the spec is never a blocking ground.

Working clauses: file the rule, not the example; read the direction of the evidence; a review after a cure pass
confirms the cure landed and does not re-audit the product; judge a hat or router by its contract, not its taste;
a stale sweep is never process.

## Standing rules

- **Test recipes, verbatim:**
  - one package: `ci/run.sh go test -count=1 ./internal/<pkg>/`
  - one scenario file: `DECK_GODOG_PATHS=<file>.feature ci/run.sh go test -count=1 ./features/ -run TestFeatures`
  - lint: `ci/run.sh ci/lint.sh`; the gates: `ci/run.sh ci/quality.sh`
  - the whole suite: `ci/run.sh ci/suite.sh`, never run in parallel with another heavy command.
  - Always run from a shell with no `DECK_*` variable set.
- **Only the final sweep runs a whole tier.** Every other task runs the narrowest recipe that covers what it
  touched, plus `ci/run.sh ci/lint.sh` before every push.
- **Commits are made per task and pushed to `main`.** Title `<area>: <imperative summary> (task NNN, R###)`.
- **After every push**, at the start of the next iteration and before picking a task, poll that push run to its
  conclusion via the Actions API. A red run is the next task, fixed at its root.
- **Waits.** A wait longer than about 85% of the iteration cap is spanned with `deferred`, never by sleeping past
  the cap.
- **The sweep's shape** (the last three tasks, in this order, all at the same `origin/main` head, each marked
  `sweep: true`):
  1. the whole `go test -p=1 -count=1 ./...` tier under `ci/run.sh`, then `go build ./...`, `go vet ./...` and
     `gofmt -l` on the tracked `.go` files;
  2. `ci/run.sh ci/suite.sh ci-results && ci/run.sh ci/quality.sh ci-results`;
  3. confirm the push run at that head is `success`, then dispatch `ci.yml` **once** and read its flaky record.
  Sweep tasks commit nothing. **Nothing is committed after the sweep.** If `origin/main` moved past the swept
  sha, the sweep is stale and re-runs. A stale sweep is never process. Closing #86, #91 and #92 is a comment
  naming the commit and the test node, made after the sweep.
- **The CI dispatch recipe.** Use the Actions REST API with the operator's PAT from the git credentials:
  `POST /repos/n-orlov/deck/actions/workflows/ci.yml/dispatches` with `{"ref":"main"}`, then poll
  `GET /repos/n-orlov/deck/actions/runs?event=workflow_dispatch&head_sha=<sha>` to its conclusion, and read the
  run's log and artifacts. Dispatch **once**, at the final pushed sha. A run that never reached `suite` (runner
  provisioning failure, API 5xx) is not judged: dispatch another.
- **Badges cannot be seen before they publish.** R290's proof is the workflow-parsing test plus the generator
  tests; the published `badges/*.json` appearing on `gh-pages` after the dispatch is read from the run, not
  committed anywhere.
- **Escape hatch.** A requirement the run proves contradictory or impossible is petitioned (not silently
  dropped), with the evidence; review adjudicates the petition.
- **Engine defects to avoid by hand.** The engine cannot be patched by this run:
  - a task titled "every X" over an open domain becomes a stall-detector trap, so this PRD names its finite cases
    and so must the tasks;
  - do not signal COMPLETE while tasks are pending;
  - when an approach ends in review, read the findings before replanning;
  - after any cure pass, check `tasks.json` for a task that depends on itself and repair it by steer.

## Non-negotiables

- **No weakened test.** A test pinning behaviour this phase changes is rewritten in the same commit, naming the
  covering test it becomes. Nothing else about the suite is retired.
- **No raised timeout, budget, worker count or per-test bound.** `internal/racebuild` is untouched.
- **No new required contract field** in a hook or event payload deck *receives* from an agent.
- **No gate loosened and no exception added** (`ci/quality.json` only ever tightens).
- **No network in tests or CI** for Part C: the fake release server only.
- **Existing behaviour is identical** except where a requirement names the change; with `bookmarks` off every key
  reaches the agent exactly as before, and with `update_check` off deck makes no outbound request.
- **Profiles stay isolated** except the single shared update-check cache R297 names.
- **A config reload never writes.**
- **A failed update never leaves a partial binary**, and never replaces one whose checks did not pass.
- **No secret** is written to a badge file, a log line or the update cache.

## Budget

400 iterations, 6 approaches, 40 h wall clock, 120-minute iteration cap. Peer plan review and peer verdict gate
on, vigilant, reflect, router task selection, Docker access for the sibling toolchain.
