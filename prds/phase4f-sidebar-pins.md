# Phase 4f — sidebar pins, and `P`/resume mode move into `i`

## What the operator gets

One feature, GH #51:

1. **`p` pins and unpins a session.** A pinned row is marked `✦` and sorts to the top of its
   own group, above `waiting` and `error` rows, under every `sort_order`. The pin survives
   restarts.
2. **The two rarely-used per-row choosers move into the `i` detail dialog.** The permission
   profile switch (today's top-level `P`) becomes `P` inside `i`, and the conversation lock
   (today's top-level `p`, "pin conversation") becomes `c` inside `i`. A top-level `P` is
   unbound. Both choosers behave exactly as they do today; only the way in changes.

#51's body is the design discussion. **Read it before starting.** Where the issue and this PRD
disagree, this PRD wins; where either disagrees with `SPEC.md`, SPEC wins, and the disagreement
is a finding. Two deliberate differences from the issue: **there is no new theme token** (the
marker uses `accent`, because every theme token is mandatory in `internal/theme` and a new one
would break every user theme on disk), and the in-`i` keys are fixed as `P`, `c` and `p`.

## Why now

v0.2.5 (phase 4e, `17fe304f11`) shipped on 2026-09-28. #51 is the operator's next ask.

**`SPEC.md` has already been amended for this phase** in an operator commit that lands *before*
this PRD:

- R7's action list;
- §4: the `pinned_at` column;
- §5: `P` is inside `i`;
- §9.1: the conversation lock, and why the UI never calls it a pin;
- §11's sort rule: pinned first within the group;
- §11's row layout: the `✦` marker;
- §11's new pin bullet;
- §11's keymap;
- §11.3's footer paragraph;
- §11.4's dialog inventory;
- §11.5's scope-label sentence.

Nothing here is blocked on the operator.

## Ground rules

- **`SPEC.md` is the authority and is read-only to this job**, as are `prds/`, `ci/Dockerfile` and
  `ci/SPIKE.md`.

  ```sh
  BASE=$(git log --format=%H -1 -- prds/phase4f-sidebar-pins.md)   # the operator's PRD commit
  git log --oneline "$BASE..HEAD" -- SPEC.md prds/ ci/Dockerfile ci/SPIKE.md   # must print nothing
  ```

  The SPEC amendment lands before `$BASE`, so it is outside the audit range by construction. It is
  not this run's commit, and it must not be re-made, extended or "cured".
- **Exactly one schema change: `schemaV8`, one additive column.** It is
  `ALTER TABLE sessions ADD COLUMN pinned_at INTEGER NOT NULL DEFAULT 0`, and nothing else. No
  table rebuild, no column drop, no data rewrite.
  - This migration will run on the operator's live `schemaV7` database, which holds about 25 real
    sessions. It must be additive and idempotent, like V1-V7.
  - Any other schema edit, or a `schemaV9`, is blocking.
- **Never touch the operator's live state.** This covers:
  - `~/.local/share/deck/`, `~/.config/deck/` and `~/.local/state/deck/`, including every
    `profiles/` directory under them. The job never opens, reads, copies or creates anything
    there;
  - the live `tmux -L deck` server, and any `tmux -L deck-*` server. Never touch them, not even
    read-only.

  Every test uses its own temp `HOME`/`XDG_*`/`DECK_HOME` and a private socket.
- **This is a behaviour change, so a test that passes on today's code proves nothing.** R158-R161
  each name assertions that must **fail on the v0.2.5 tree (`17fe304f11`) and pass after**. Run
  the new tests against the unfixed tree. The evidence (the command and the failure) goes to the
  run's artifacts, never into the repo.
- **Docker is for `ci/run.sh` and nothing else.**
  - Never remove or kill containers by label: a previous ralphd job on this host SIGKILLed itself
    by sweeping `label=ralphd.run`.
  - No `docker prune`, and no wildcard `rm`/`rmi`.
  - Never signal by pattern: resolve a pid, verify it, then signal that pid.
  - Other runs live on this host.
- **Code references are pointers, not criteria.** The references below were correct at
  `17fe304f11`. If one has drifted, find the current site by function name and carry on. Drift is
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
- **CI already exists** (`.github/workflows/ci.yml`, `docs/ci.md`). Every push to `main` runs the
  suite on the `deck-ws-*` self-hosted slots in about 8 minutes. Leave the workflows,
  `ci/suite.sh`, `ci/stability.sh` and the release gate alone. CI hygiene is #50, a later phase.
- **Tests may be updated, not deleted.** Existing tests that press a top-level `P` or `p` to reach
  the permission switcher or the conversation lock (for example `internal/tui/profile_switch_test.go`,
  `internal/tui/profile_pin_restart_theme_test.go`, `features/dialogs_test.go`) must be moved to
  the new route (`i` then `P`/`c`). The behaviour they prove must keep being proven.

## Scope

- **R158:** the pin is stored (`schemaV8`, store API).
- **R159:** `p` toggles it, singly or over the marked set.
- **R160:** pinned rows sort first within their group, and are marked.
- **R161:** `P` and the conversation lock live inside `i`.
- **R162:** the spec and the code agree.

All of them are acceptance.

## Materiality rubric

Reject only on substance:

- a requirement's behaviour is absent or wrong in the live code;
- a test the requirement names:
  - is missing or red;
  - asserts the opposite of the requirement;
  - for R158-R161, also passes on the unfixed tree `17fe304f11`;
- the spec contradicts the live code;
- a secret is exposed, in the tree, a workflow log or a published CI report;
- a guard in §Ground rules is broken:
  - a schema change other than `schemaV8`'s one column;
  - any access to the operator's deck directories or any `tmux -L deck`/`deck-*` server;
  - a protected path modified (the audit command prints anything);
  - a branch, PR, tag or release created;
  - a repository setting changed.

Everything else verifies, with the gap recorded as a residual note. That covers wording, form,
provenance and process, the Allure report's cosmetics, and **any number or claim in prose outside
the spec** (commit messages, notes, `docs/ci.md`, run artifacts). Prose outside the spec is never
a blocking ground.

**Advisory, never blocking:**

- any stability-sweep or CI failure in a scenario this phase did not add or change. It is a known
  flake class (#50 tracks the nightly), so name it in artifacts and in the terminal notification,
  and do not cure it;
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

## R158 — the pin is stored

- `schemaV8` adds `sessions.pinned_at INTEGER NOT NULL DEFAULT 0` (SPEC §4). `0` means not pinned;
  anything else is the store clock's time when the session was pinned.
- `store.Session` carries the value. A store method sets or clears it for a list of session ids in
  one transaction, as one targeted `UPDATE … WHERE id = ?` per row (SPEC §4's mutation rule).
  Pinning writes now; unpinning writes 0.
- The value is untouched by everything else that writes the row: kill, resume, restart, rename,
  group move, archive/unarchive, `dd` and its restore, and status transitions. A pin survives all
  of them.
- **Success:**
  - A store test migrates a `schemaV7` fixture database that holds rows to V8: every existing row
    reads `pinned_at = 0`, and no other column changes.
  - Opening an already-V8 database is a no-op.
  - A store test pins and unpins a set of ids and observes both values.
  - A store test runs every mutation listed above against a pinned row, and the pin survives each
    one. A `dd` restore included.

## R159 — `p` toggles the pin

- Top-level `p` on a session row toggles its pin (SPEC §11's pin bullet). There is no dialog and no
  confirm.
- **With a marked set**, `p` acts on the whole set. It unpins them all when every marked row is
  pinned, and pins them all otherwise.
- `p` is session-scoped: the existing guard (`internal/tui/session_scoped_guard.go`) refuses it
  when the cursor has no session, as it does today. A group header is not a session.
- The write happens through a `tea.Cmd`, never in `View` or synchronously in `Update` (SPEC
  §11.4's store rule), and the list re-sorts when the reload lands.
- `p` records no event (SPEC §11).
- Inside the `i` detail dialog, `p` toggles the shown session's pin, and the dialog shows
  `pinned: yes`/`no`.
- **Success:**
  - A TUI unit test: `p` on an unpinned row pins it; `p` again unpins it.
  - A TUI unit test for the mixed marked-set rule (pin all), then the all-pinned rule (unpin all).
  - A TUI unit test: `p` inside `i` toggles, and the dialog's `pinned:` line follows.
  - A `features/` scenario on the real binary. `p` pins a row; it moves to the top of its group
    with `✦`. Restart deck: it is still there. `p` again: it returns to its sorted place.

## R160 — pinned first within the group, and marked

- SPEC §11's sort rule. **In every group, pinned rows sort above unpinned rows under all four
  `sort_order` values**, including above `waiting`/`error` under `attention`. Inside each tier the
  configured order applies unchanged, with its `id` tie-break, so the order stays total.
- Implement it once, as the primary key over every comparator in `internal/tui/sort_order.go`
  and `internal/tui/attention.go`'s stable path, through the one sort entry point
  (`sortSessionsByOrder`). Do not re-derive it per order.
- A pin never moves a row to another group and never changes group order (`groupSortsBefore`).
- The attention walk (`space`) and the collapsed strip's attention count still reach and count a
  `waiting` row that sits below a pinned one.
- **Rendering:** a pinned row's line 1 shows `✦` (ASCII `*` under `DECK_ASCII=1`/`[ui] ascii`)
  between the status glyph and the name, drawn in the `accent` token. An unpinned row reserves no
  column. The name truncation budget accounts for the marker, so a pinned row never overflows the
  sidebar width.
- **Success:**
  - A table-driven unit test over all four orders: one group holding pinned and unpinned rows of
    mixed status. Every pinned row precedes every unpinned one, each tier is in that order's own
    sequence, and a pinned `idle` row precedes an unpinned `waiting` row under `attention`.
  - A test that two groups each keep their own pinned tier, and that group order is unchanged.
  - A test that the attention walk from a pinned row reaches the unpinned `waiting` row.
  - A render test at 80×24 in both glyph modes: a pinned row shows the marker before its name, and
    a long pinned name is truncated within the sidebar width.
  - The `features/` scenario from R159 asserts the order on screen.

## R161 — `P` and the conversation lock live inside `i`

- **A top-level `P` is unbound.** It does nothing, and the `?` overlay does not list it at top
  level.
- **Inside the `i` detail dialog:**
  - `P` opens the permission profile picker, exactly as a top-level `P` does today. That means the
    same eligibility (`canSwitchProfile`), the same refusal message, the same `allow_yolo` gating,
    the same degradation reason and the same durable write;
  - `c` opens the conversation lock chooser, exactly as a top-level `p` does today. That means the
    same eligibility (`canPinResume`), the same refusal message and the same `resume_state` writes.
- Closing either chooser, by submit or by `esc`, returns to the detail dialog, the way rename and
  the launch-inputs editor already do.
- The detail dialog's key line names `P`, `c` and `p` next to `r`, `l` and `g`.
- **User-visible wording:** everywhere the UI says "pin conversation" or "pin" for the resume lock
  (the chooser title, its refusal message, help text), it now says "lock conversation" or "resume
  mode". The stored `resume_state` value `pinned` and the `resume_pin` column keep their names.
- **Success:**
  - The existing permission-switch and resume-lock tests move to the `i` route and stay green.
  - A TUI unit test: a top-level `P` changes nothing, and opens nothing.
  - A TUI unit test: `i`, `P`, pick a profile: the store has it and the dialog is back on detail.
  - A TUI unit test: `i`, `c`, lock: `resume_state = pinned`.
  - A test that the chooser renders the "lock" wording and no "pin" wording.
  - A `features/` scenario on the real binary: `i`, then `P`, switches a session's permission
    profile.

## R162 — the spec and the code agree

- The operator's amendment states this phase's behaviour. When R158-R161 are done, every amended
  passage must describe the live code. The passages are listed under §Why now.
- The `?` help view, the footer and the `i` dialog name the same keys that the spec does. `p` is
  the sidebar pin at top level; `P` and `c` appear only inside `i`.
- A contradiction that the run cannot resolve in code goes through the §Escape hatch. It is never
  an edit to `SPEC.md`.
- **Success:** the requirement tests above cover each amended passage's behaviour. Help text is
  itself the behaviour here, so a render test asserts the `?` overlay's `p` line and the absence
  of a top-level `P`.

## Ordering

1. **R158** first: everything reads the column.
2. **R160**, then **R159**. The sort is a pure function and is easiest to prove before the key
   exists.
3. **R161**. It is independent, and may run in parallel with R159-R160.
4. **R162**, checked last, against the finished code.
5. **The final sweep, as the last task, with nothing committed after it.** This is §Definition of
   done. If the sweep turns up a real defect, fix it, push, and run the sweep again. That is the
   only reason to commit after a sweep.

## Definition of done

- R158-R162's behaviours are in the live code, and each requirement's named tests exist and are
  green.
- For R158-R161, each named test fails on `17fe304f11`. The evidence goes to artifacts.
- **At the final pushed sha:**
  - the GitHub `suite` check, from the push run, concluded `success`;
  - `ci/run.sh go test -p=1 -count=1 ./...` is green locally. That means the whole suite, with no
    narrowed package list and no `-run` filter;
  - `go build ./...`, `go vet ./...` and `gofmt -l` are clean on every file this run touched;
  - the protected-path audit command prints nothing.
- A five-run stability sweep (`ci/stability.sh 5`) at that sha is complete, with its logs in
  artifacts. A failure is judged by §Materiality rubric's advisory list.
- No secret is exposed.

Nothing else is required. In particular, no report, record or log row goes in the repo.

## Non-goals

- **A new theme token**, or any change to `internal/theme`'s token set.
- **Pin reordering:** no manual drag or up/down among pinned rows. The configured order decides
  the order inside the pinned tier.
- **Pinning groups**, or pinning across groups.
- **A footer slot for `p`** (SPEC §11.3 keeps it out).
- **Rebinding the freed top-level `P`** to anything.
- **CI hygiene (#50) and CPU work (#49).**
- **Any schema change beyond `schemaV8`'s one column.**
- **Publishing a release.**

## For the planner

- **The migration runs on the operator's real database.** Pin V1-V7's migration style exactly,
  and prove it with a V7 fixture that holds real-looking rows, not an empty database.
- **The sort's trap is the stable path.** `sortSessionsByAttentionStable` and the three
  `less…Stable` comparators prefer the previous frame's relative order on ties. The pinned key
  must sit **above** that preference, or a freshly pinned row will stay where it was until an
  unrelated reorder. The first test to write: pin a row that has an established previous-frame
  position, and assert that it moves on the very next sort.
- **R161's trap is nesting.** The choosers are opened today from the top-level key switch, and
  their update paths return to the list. Inside `i`, `m.detail` must stay true while a chooser is
  open (mirror `m.renaming` in `internal/tui/rename.go`), or `esc` will drop two levels at once.
- **`internal/tui/help_style.go` and `session_scoped_guard.go` both list `P`.** Remove it from
  both, and keep `p`: it is still session-scoped.
- **CI runs take about 8 minutes** on two shared slots. Poll the Actions API rather than sleeping
  in a loop.
- **Budget for one cure pass.** That is the harness working.
