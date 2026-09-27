# Phase 4e — named profiles, the interactive wheel, and the flake classes

## What the operator gets

The last two open backlog issues, plus the flake classes that keep the nightly red:

1. **Named profiles** (#27). `deck work` opens a separate deck with its own config, state, log
   and tmux socket (`deck-work`), created the first time it is used. Plain `deck` is unchanged,
   byte for byte. Hooks always write to the profile that created their pane.
2. **The wheel works over the list while interactive** (#47). A wheel over the sidebar scrolls
   the list without leaving interactive mode. The next keystroke sent to the pane brings the
   selection back into view.
3. **The two known flake classes are fixed** (R151). One is a frame assertion on a transient
   `starting` status; the other is an exact SIGWINCH count with no settle. The nightly run at
   `d40a0552b` failed on the first class twice in a row: `mouse.feature:10`, retry included.

#27 and #47 were each filed after the operator discussed them. Each issue body is the detailed
design. **Read the issue before starting its requirement.** Where an issue and this PRD disagree,
this PRD wins. Where either disagrees with `SPEC.md`, SPEC wins, and the disagreement is a finding.

## Why now

v0.2.4 (phase 4d) shipped on 2026-09-27. #35-#40 are done, which leaves #27 and #47 as the whole
open backlog. #27 was deferred from 4d on purpose, so that it could have a phase of its own.

**`SPEC.md` has already been amended for this phase** in an operator commit that lands *before*
this PRD:

- the interaction-model paragraph and the non-goals: the launch arguments;
- §2 Paths;
- §3.2: the socket name;
- **the new §3.4, Profiles**, which is the authoritative statement of R152-R156;
- §6.1: `DECK_PROFILE` in the pane env;
- §6.5: where the config file lives;
- §9.5: a note that the unit is templated per profile;
- §11's drift rule: input to the live pane counts as acting;
- §11.8 and §11.9: the wheel is decided by the panel under the pointer;
- §13.1: `DECK_HOME` and `DECK_TMUX_SOCKET` under a profile.

Nothing here is blocked on the operator.

## Ground rules

- **`SPEC.md` is the authority and is read-only to this job**, as are `prds/`, `ci/Dockerfile` and
  `ci/SPIKE.md`.

  ```sh
  BASE=$(git log --format=%H -1 -- prds/phase4e-named-profiles-and-interactive-wheel.md)   # the operator's last PRD commit
  git log --oneline "$BASE..HEAD" -- SPEC.md prds/ ci/Dockerfile ci/SPIKE.md   # must print nothing
  ```

  The SPEC amendment lands before `$BASE`, so it is outside the audit range by construction. It is
  not this run's commit, and it must not be re-made, extended or "cured".
- **No schema change, no migration.** `schemaV7` is on the operator's live database, and profiles
  need no column. A `schemaV8` in the tree is blocking.
- **Never touch the operator's live state.** This covers:
  - `~/.local/share/deck/`, `~/.config/deck/` and `~/.local/state/deck/`, which hold the
    operator's real session list and config. The job never opens, reads, copies or creates
    anything under them, and that includes a `profiles/` directory;
  - the live `tmux -L deck` server, which holds about 25 live agent sessions, and any
    `tmux -L deck-*` server. Never touch them, not even read-only.

  Every test uses its own temp `HOME`/`XDG_*`/`DECK_HOME` and a private socket. **A test that
  resolves paths with the real user home is a defect**, because profiles make that mistake
  easy: a default-profile test that forgot to set `HOME` would write the operator's
  `last_used` marker.
- **This is partly a defect phase, so a test that passes against today's code has proven
  nothing.**
  - R149-R155 each name assertions that must **fail on the v0.2.4 tree (`d40a0552b`) and pass
    after**.
  - Keep the probe-audit discipline: run the new tests against the unfixed tree. The evidence
    (the command and the failure it produced) goes to the run's artifacts, never into the repo.
- **Docker is for `ci/run.sh` and nothing else.**
  - Never remove or kill containers by label: a previous ralphd job on this host SIGKILLed itself
    by sweeping `label=ralphd.run`.
  - No `docker prune`, and no wildcard `rm`/`rmi`.
  - Never signal by pattern: resolve a pid, verify it, then signal that pid.
  - Other runs live on this host.
- **Code references are pointers, not criteria.** The `file:line` references below were correct at
  `d40a0552b`. If one has drifted, find the current site by function name and carry on. Drift is
  never a finding.
- **No paperwork.** Do not add report, findings, audit, close-out or "retake" files to the repo,
  and do not add a `docs/DELIVERY-LOG.md` row.
  - Run evidence (probe audits, sweep logs, CI run URLs) goes to `/run/ralphd/artifacts`.
  - CI evidence is the CI run itself, found by its head sha.
  - Do not close GitHub issues: the operator closes them at release.
- **The CI container has no agent binaries.** Every scenario runs against the `cmd/fake-*` stubs on
  a fixture `PATH`.
- **GitHub scope.** The git credentials are the operator's PAT. The job pushes to `main` and reads
  the Actions API, and nothing else:
  - no branches other than `main`;
  - no PRs, no tags, no releases;
  - no change to repository settings.
- **CI already exists** (phase 4d: `.github/workflows/ci.yml`, `docs/ci.md`). Every push to `main`
  runs the suite on the `deck-ws-*` self-hosted slots in about 8 minutes. Leave the workflows,
  `ci/suite.sh` and the release gate alone, unless a requirement below needs a change.

## Scope

- **Tier 1: R149**, the interactive wheel (#47).
- **Tier 2: R150 and R151**, the flake classes.
- **Tier 3: R152-R156**, named profiles (#27). R152 lands first, because everything else in the
  tier resolves paths through it.
- **R157:** the spec and the code agree.

All of them are acceptance.

## Materiality rubric

Reject only on substance:

- a requirement's behaviour is absent or wrong in the live code;
- a test the requirement names:
  - is missing or red;
  - asserts the opposite of the requirement;
  - for R149 and R152-R155, also passes on the unfixed tree `d40a0552b`;
- the spec contradicts the live code;
- a secret is exposed, in the tree, a workflow log or a published CI report;
- a guard in §Ground rules is broken:
  - a schema change;
  - any access to the operator's deck directories or any `tmux -L deck`/`deck-*` server;
  - a protected path modified (the audit command prints anything);
  - a branch, PR, tag or release created;
  - a repository setting changed.

Everything else verifies, with the gap recorded as a residual note. That covers wording, form,
provenance and process, the Allure report's cosmetics, and **any number or claim in prose outside
the spec** (commit messages, notes, `docs/ci.md`, run artifacts). Prose outside the spec is never
a blocking ground.

**Advisory, never blocking:**

- a stability-sweep or CI failure in a flake class **other than** the two R150/R151 fix. Name it in
  artifacts and in the terminal notification.
- a failure in one of R150/R151's own classes after the fix, provided that R150/R151's code
  criteria hold: no remaining frame-read waypoint on a transient, and no unsettled exact count.
  Such a failure is recorded as a residual note with the scenario and log. It is a new mechanism,
  not a reason to keep curing;
- any operator notification that is missed or late.

## Escape hatch

If the run proves that a requirement contradicts `SPEC.md`, or is impossible as written, it must
not edit the spec or quietly narrow the requirement. File a petition, with the evidence and the
smallest change that would resolve it, notify the operator, and carry on with everything the
petition does not block. The operator rules by amendment. SPEC changes are the operator's commits.

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

## Tier 1

### R149 — the wheel over the sidebar scrolls the list while interactive (GH #47)

- **Today:** `Update`'s `tea.MouseMsg` branch for `m.interactive` (`internal/tui/tui.go:4531-4541`)
  honours a wheel only when `hitTest` lands on `hitPanelPreview`, which calls
  `scrollInteractiveByLines`. A wheel over any other panel returns `m, nil`.
- **After:**
  - A wheel over the sidebar while interactive does what list mode's `scrollSidebar`
    (`internal/tui/mouse.go:208`) does. It moves the viewport only and sets the same drift state
    that R142 introduced. It never changes the selection, the interactive target or the grid, and
    it neither leaves interactive mode nor resizes anything.
  - A wheel over the preview is unchanged.
- **Input to the live pane ends the drift.**
  - In `updateInteractive` (`internal/tui/interactive.go:478`), a key or paste that is actually
    forwarded brings the target's row back into view first. It then forwards exactly as today, on
    the same press, with nothing swallowed. Reuse the viewport follow that R142's drift-ending path
    uses; do not write a second one.
  - `Ctrl+Q` (`exitInteractive`) also brings it back, and so does R144's empty-sidebar click,
    which runs the same exit.
  - A key that no forwarding helper recognises writes no bytes. It leaves the drift alone, just
    as it leaves the scrollback alone today (see the comment at `interactive.go:486-500`).
- **Reading is not input.** None of the following ends a drift:
  - a wheel over the preview (scrollback);
  - a drag-to-copy;
  - a background reload, re-sort or re-group.
- **Sidebar clicks while interactive keep their current meaning:** re-target a row, toggle a
  header, restore the strip, and `Ctrl+Q` on empty space.
- **Success:**
  - In interactive mode, on a list longer than the sidebar, a wheel over the sidebar changes the
    sidebar viewport. The interactive target, the grid's content and scroll offset, and the tmux
    window size are all unchanged. This is a unit test, plus a `features/` scenario on real tmux.
  - After that drift, one forwarded printable keystroke brings the selected row into view on the
    same press, and the pane receives that keystroke byte-exact (checked with `capture-pane` or the
    fake agent's input log). The same holds for a forwarded named key (`Enter`) and a bracketed
    paste.
  - After a drift, `Ctrl+Q` returns to the list with the selection in view.
  - A wheel over the preview, a drag-to-copy and several reload ticks each leave the drift in
    place.
  - The existing interactive-scrollback wheel tests stay green, unmodified.
  - Each assertion above that concerns new behaviour fails on `d40a0552b`.

## Tier 2 — the flake classes

These are harness defects, not product defects. **The fix is always a settle-bounded wait on a
durable fact, never a longer sleep, a widened timeout, a skipped scenario, a retry, or a deleted
assertion.** Every scenario keeps asserting what it asserted before.

### R150 — no frame assertion rests on a transient status

- **The mechanism.** A shell session goes from `starting` to `running` on the next reconcile tick
  (SPEC §7's shell fast-forward, `internal/service/reconcile.go`). Under load, a frame showing
  `starting` may never be painted. The same is true of a frame captured *while* a row still reads
  `starting`, then compared later against a frame where it reads `running`.
- **The instance that failed.** The nightly run at `d40a0552b` failed `mouse.feature:10` twice in
  a row, retry included. The scenario:
  - waits for `screen contains "running"`, which alpha's row already satisfies;
  - captures `click-entered-bravo` while bravo's row still reads `starting`;
  - later fails `frame still matches` because bravo now reads `running`.
- **Fix the class, not the instance.**
  - Find every `features/` step that waits on, captures or compares a frame containing a
    transient status (`starting`, or any status the reconcile loop promotes on its own).
  - Make each one wait for the **named** session's settled status before it captures or asserts.
    The SPEC's §7 statuses, read from the store or from that session's own row, are the durable
    facts.
  - An assertion that genuinely needs the store's transient `starting` is sound and stays as it
    is (`features/concurrency.feature:21` reads the store, not a frame).
- **Success:**
  - A test fails if any `features/*.feature` step still waits on a frame reading `starting` for a
    shell session, or captures a frame for later comparison without first settling the sessions
    that frame shows. This can be a harness-level check, such as a step-definition guard or a
    scan over the feature files.
  - `mouse.feature:10`, and every other scenario the audit changed, passes 20 consecutive solo runs
    under `-race` at the final sha. Logs go to artifacts.

### R151 — no exact count is sampled without a settle

- **The mechanism.** Exact SIGWINCH-count assertions (`features/preview.feature`,
  `features/interactive_sigwinch_budget.feature`, and any others the audit finds) sample an async
  counter with no settle, so they fail in both directions under load.
- **Fix:** each exact count is asserted only after the counter is quiescent. That means no change
  across a bounded settle window measured on the monotonic clock, with a deadline that fails
  loudly. It also still asserts the exact value the scenario asserts today. Never a `>=`, and never
  a range.
- **Success:**
  - A unit test of the settle helper:
    - a counter still moving at the deadline fails;
    - a counter that settles at the expected value passes;
    - a counter that settles one off the expected value fails.
  - Every exact-count step goes through that helper.
  - The affected scenarios pass 20 consecutive solo runs under `-race` at the final sha. Logs go
    to artifacts.

## Tier 3 — named profiles (GH #27)

**Read #27 in full, and read SPEC §3.4.** The issue's design sections 1-10 are the requirement,
and §3.4 is their authoritative statement. Where they differ, SPEC wins. The requirements below
split the work and name the tests.

### R152 — profile resolution, validation and layout

- **In `internal/config`:**
  - `ValidateProfileName(name) error`, the **only** validator. Its messages follow §3.4 and #27 §4
    exactly.
  - Profile resolution: positional argument, then `DECK_PROFILE`, then `default`. An empty
    `DECK_PROFILE` counts as unset.
  - `resolvePaths` (`config.go:354`) gains the `profiles/<name>/` branch, **for named profiles
    only**, in both XDG and `DECK_HOME` mode.
  - The derived socket `deck-<name>`, with `DECK_TMUX_SOCKET` still winning.
  - The frozen clock's shared path (`config.go:200-202`) follows the profile root.
  - User themes (`internal/theme` `ThemesDir`) resolve to the **default** profile's `themes/`
    directory for every profile.
  - `Paths` keeps its shape. The settings expose the resolved profile name.
- **In `cmd/deck/main.go` `run()`:**
  - flags are parsed before the positional argument;
  - `deck -x` gives `error: unknown flag -x`;
  - `deck a b` is an error;
  - an invalid name exits **2**, before anything touches disk or tmux;
  - `--version` and `_hook` dispatch are unchanged.
- **Success, as config unit tests:**
  - precedence, with an empty env counting as unset;
  - every validation rule and its exact message, including 16 versus 17 characters, uppercase
    with its `did you mean`, `.`, a leading `_` or `-`, `default`, and a `DECK_PROFILE`-prefixed
    message;
  - the named layout in XDG and in `DECK_HOME` mode;
  - **default resolves byte-for-byte to today's paths and socket**. Take a table of today's
    resolution for several env combinations as the oracle;
  - the `DECK_TMUX_SOCKET` override wins;
  - the clock path follows the profile;
  - the themes dir is shared.
- **Success, as cmd tests:** `deck -x`, `deck a b` and each invalid name exit 2 and leave the temp
  home empty.

### R153 — lazy creation behind a typo guard

- Launch order: validate, then confirm if the profile is unknown, then create, then launch. A
  profile exists when its directory is present under the data root's `profiles/`.
- An unknown valid name prompts once on the terminal with §3.4's exact message, listing the known
  profiles. Only `y` creates it.
  - Anything else exits 1 and creates nothing.
  - **A non-terminal stdin refuses without prompting**: exit 1, and nothing is created.
- Creation:
  - makes the profile's config, data and log directories;
  - copies the default profile's `config.toml` once, if it exists. If default has no config
    file, the new profile gets none either;
  - leaves `state.db` and `log/` to the normal first-launch path.
- **Success:**
  - cmd tests:
    - a non-TTY unknown name exits 1 and creates nothing;
    - `n` on a pty creates nothing;
    - `y` on a pty creates the profile with a byte-identical config copy;
    - `y` with no default config creates no config file;
    - after that, editing one profile's config never changes the other's.
  - A `features/` scenario runs the creation prompt through the real pty and then shows the named
    profile's deck.

### R154 — hooks write to the profile that created the pane

- **The pane environment** (`internal/service/session_context.go:47`, next to `DECK_HOME`) gains
  `DECK_PROFILE=<name>` for every launch, `default` included.
- **`_hook`** resolves its paths from the pane's `DECK_PROFILE`, using R152's resolver. With none
  set, it resolves to default.
- With an invalid `DECK_PROFILE`, or one naming a profile whose directory is gone, `_hook`:
  - writes nothing;
  - creates nothing;
  - never opens default's database;
  - prints one stderr line naming the pane (`TMUX_PANE`) and the profile;
  - exits **exactly 1**.

  Note that `run()` today exits 1 on any hook configuration error (`main.go:36-41`). Keep that
  code, and make sure no profile path reaches 2 or 0.
- **Success:**
  - cmd tests: the bad-name and missing-dir cases exit exactly 1, with no file created anywhere
    under the temp home and default's `state.db` untouched (mtime and content);
  - the launch-env test asserts `DECK_PROFILE` for both default and named launches;
  - a `features/` scenario with two profiles running at once: a hook fired from profile A's pane
    updates A's `state.db`, and B's is untouched while a deck on B is running.

### R155 — `deck --profiles`, the marker, the wall, the display

- **`deck --profiles`** prints one line per profile, default first, in §3.4's shape, and exits 0:
  - it never opens a `state.db`;
  - a directory with an invalid name is listed as `(invalid name: not selectable)`;
  - "last used" is the mtime of the `last_used` marker that the TUI touches in the profile's data
    root at launch.
- **Settings** (`,`) write the running profile's `config.toml`.
- **Display:**
  - For a named profile, the sidebar header line (`internal/tui/tui.go:6100`, today
    `socket: %s`) reads `profile: <name> · socket: deck-<name>`, with the socket half eliding
    first. For default it stays exactly `socket: deck`.
  - A named profile sets the terminal title `deck: <name>`. Default sets none.
- **Help** gives §3.4's manual deletion steps.
- **Success:**
  - cmd tests for `--profiles`: ordering, the invalid-name flag, exit 0, and that no `state.db` is
    opened. Prove the last one by making each profile's `state.db` unreadable (mode `0000`) and
    still getting a listing.
  - A TUI render test of the header for a named profile, including the elision at the narrowest
    sidebar width, and for default, byte-identical to today's.
  - A test that the title sequence is emitted for a named profile and not for default.
  - A settings save under a named profile changes only that profile's file.

### R156 — two profiles side by side, and default unchanged

- **Success, as `features/` scenarios:**
  - Two profiles, each with a session of the same name, running at the same time. Each deck lists
    only its own session. The sessions live on `deck-<a>` and `deck-<b>` respectively, and each
    `state.db` holds only its own row.
  - `deck` with no argument on an existing default install behaves exactly as before:
    - the same paths;
    - socket `deck`;
    - header `socket: deck`;
    - no `profiles/` directory created.

    The existing scenarios, `features/no_leak_scan.feature` included, stay green unmodified.
  - The named-profile header line on screen.

## R157 — the spec and the code agree

- The operator's amendment states this phase's behaviour. When R149-R156 are done, every amended
  passage must describe the live code:
  - the interaction-model paragraph;
  - §2 Paths;
  - §3.2;
  - §3.4;
  - §6.1's `DECK_PROFILE`;
  - §6.5;
  - §11's drift rule;
  - §11.8 and §11.9's wheel;
  - §13.1.
- The `?` help view and the footer name the same keys, gestures and `DECK_*` variables that the
  spec does, `DECK_PROFILE` included.
- A contradiction that the run cannot resolve in code goes through the §Escape hatch. It is never
  an edit to `SPEC.md`.
- **Success:** the requirement tests above cover each amended passage's behaviour. Where help text
  is itself the behaviour (the `DECK_PROFILE` listing and the deletion steps), a render test
  asserts it.

## Ordering

1. **R149**: small and self-contained.
2. **R150, then R151.** Land these early, so that every later CI run on `main` is less noisy.
3. **R152**, then R153, R154 and R155 (in any order), then R156.
4. **R157**, checked last, against the finished code.
5. **The final sweep, as the last task, with nothing committed after it.** This is §Definition of
   done. If the sweep turns up a real defect, fix it, push, and run the sweep again. That is the
   only reason to commit after a sweep.

## Definition of done

- R149-R157's behaviours are in the live code, and each requirement's named tests exist and are
  green.
- For R149 and R152-R155, each named test fails on `d40a0552b`. The evidence goes to artifacts.
- **At the final pushed sha:**
  - the GitHub `suite` check, from the push run, concluded `success`;
  - `ci/run.sh go test -p=1 -count=1 ./...` is green locally. That means the whole suite, with no
    narrowed package list and no `-run` filter;
  - `go build ./...`, `go vet ./...` and `gofmt -l` are clean on every file this run touched;
  - the protected-path audit command prints nothing.
- A ten-run stability sweep (`ci/stability.sh 10`) at that sha is complete, with its logs in
  artifacts. A failure is judged by §Materiality rubric's advisory list.
- R150/R151's 20-run `-race` solo loops are complete at that sha, with their logs in artifacts.
- No secret is exposed.

Nothing else is required. In particular, no report, record or log row goes in the repo.

## Non-goals

- **Phases 5-7** (notifications, shell state, search and health). `internal/notify`,
  `internal/search` and `internal/unit` stay placeholders.
- **Any in-TUI profile management:**
  - a switcher, an `s` binding, or in-place switching;
  - `deck profile new`, or a `-p` flag;
  - any deletion of a profile by deck;
  - any cross-profile view or count.
- **Moving the default profile** into `profiles/default/`, renaming its socket, or any migration.
- **Any schema change.**
- **Allure display work.** Phase 4d's residuals are out of scope: the coverage table and the named
  attachments in the report.
- **CI changes beyond what a requirement needs**, and any repository setting.
- **Publishing a release.**

## For the planner

- **Tier 3's trap is the real user home.** Every config, cmd and TUI test that resolves paths
  must inject `HOME`/`XDG_*` or `DECK_HOME`. Consider a test helper that fails if resolution ever
  reaches the process's real home. The operator's `~/.config/deck/` must never gain a `profiles/`
  directory from this run.
- **R152's byte-for-byte default** is the regression that would hurt the operator most: about 25
  live sessions sit on `tmux -L deck` and the flat `state.db`. Pin today's resolution as a table
  **before** changing `resolvePaths`.
- **R154 has two traps.** The exit code must be exactly 1: Claude reads 2 as "block". And there is
  no fallback to default: a silent fallback is the one failure that corrupts another profile's
  data.
- **R149's trap is the unrecognised key.** Only a key that is actually forwarded ends the drift.
  Mirror the existing scrollback snap's condition at `interactive.go:486-500`, not the top of
  `updateInteractive`.
- **R150 is an audit.** List every frame-reading transient waypoint first, then fix them. Do not
  stop at `mouse.feature:10`.
- **CI runs take about 8 minutes** on two shared slots. Poll the Actions API rather than sleeping
  in a loop.
- **Budget for one cure pass.** Phases 4b, 4c and 4d were each rejected at least once on curable
  findings. That is the harness working.
