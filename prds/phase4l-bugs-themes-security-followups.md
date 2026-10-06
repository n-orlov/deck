# Phase 4l — Restart and attach bugs, the OSC title leak, themes, v0.2.9 security follow-ups

## What the operator gets

Four GitHub issues on `n-orlov/deck` (#65, the event-hook rewrite, is **out of scope**):

1. **A Claude session restarted before its first message starts again** (#70, bug 1), and **a pane that
   simply died is no longer reported as "Lost attach"** (#70, bug 2).
2. **A Claude Code session title never leaks into the pane grid** (#69): the `✳` (U+2733) title no longer
   draws text over the input field or leaves a phantom character under a typed space.
3. **Themes** (#68): `cobalt` and `empire` become visibly different, and four new built-ins ship:
   `gruvbox-dark`, `solarized-dark`, `amber`, `high-contrast`.
4. **The code-level LOW follow-ups of the v0.2.9 security review** (#71): items 1, 2 (reachability half),
   4, 5, 6 and 7.

The issue bodies **and their comments** are the detailed design. **Read each issue in full, comments
included, before starting its requirement.** Where an issue and this PRD disagree, this PRD wins.
Where either disagrees with `SPEC.md`, fix whichever is wrong in the same commit (R215).

## Why now

Phase 4j put every quality gate on with zero exceptions: coverage (total ≥ 85%, every package ≥ 80%),
CRAP ≤ 10 for every function, golangci-lint at zero findings, govulncheck, Trivy. **All new code in
this phase meets those gates from its first commit.** A feature that needs a function over CRAP 10 is
split in the same commit; a new file needs its tests in the same commit. No gate is loosened, no
exception is added.

## Issues in scope, and the sha

#68, #69, #70, #71 (the items named in R214 only). **Not in scope: #65.**

Symbols and file names in this PRD are as of `8dfdbc88a9`. **Drift from that sha is expected and is
not a finding**: find the current site and carry on.

## Standing rules

- **Test recipes, verbatim:**
  - one package: `ci/run.sh go test -count=1 ./internal/<pkg>/`
  - one scenario file: `DECK_GODOG_PATHS=<file>.feature ci/run.sh go test -count=1 ./features/ -run TestFeatures`
  - lint: `ci/run.sh ci/lint.sh`; the gates: `ci/run.sh ci/quality.sh`
  - the whole suite: `ci/run.sh ci/suite.sh`, never run in parallel with another heavy command.
  - Always run from a shell with no `DECK_*` variable set.
- **Only the final sweep runs a whole tier.** Every other task runs the narrowest recipe that covers
  what it touched, plus `ci/run.sh ci/lint.sh` before every push.
- **Commits are made per task and pushed to `main`.** Title `<area>: <imperative summary> (task NNN, R###)`.
- **After every push**, at the start of the next iteration and before picking a task, poll that push
  run to its conclusion via the Actions API. A red run is the next task, fixed at its root.
- **Waits.** A wait longer than about 85% of the iteration cap is spanned with `deferred`, never by
  sleeping past the cap.
- **The sweep's shape** (the last three tasks, in this order, all at the same `origin/main` head):
  1. the whole `go test -p=1 -count=1 ./...` tier under `ci/run.sh`, then `go build ./...`,
     `go vet ./...` and `gofmt -l` on the tracked `.go` files;
  2. `ci/run.sh ci/suite.sh ci-results && ci/run.sh ci/quality.sh ci-results`;
  3. confirm the push run at that head is `success`, then dispatch `ci.yml` **once** and read its
     flaky record.
  Sweep tasks commit nothing. **Nothing is committed after the sweep.** If `origin/main` moved past
  the swept sha, the sweep is stale and re-runs. A stale sweep is never process.
- **The CI dispatch recipe.** Use the Actions REST API with the operator's PAT from the git
  credentials: `POST /repos/n-orlov/deck/actions/workflows/ci.yml/dispatches` with `{"ref":"main"}`,
  then poll `GET /repos/n-orlov/deck/actions/runs?event=workflow_dispatch&head_sha=<sha>` to its
  conclusion, and read the run's log and artifacts. Dispatch **once**, at the final pushed sha. A run
  that never reached `suite` (runner provisioning failure, API 5xx) is not judged: dispatch another.
- **Engine defects to avoid by hand.** The engine cannot be patched by this run:
  - a task titled "every X" over an open domain becomes a stall-detector trap, so this PRD names its
    finite cases and so must the tasks;
  - do not signal COMPLETE while tasks are pending;
  - when an approach ends in review, read the findings before replanning.

## Non-negotiables

- **No weakened test.** A test pinning behaviour this phase changes is rewritten in the same commit,
  naming the covering test it becomes. Nothing else about the suite is retired.
- **No raised timeout, budget, worker count or per-test bound.** `internal/racebuild` is untouched.
  (R214's new writer timeout is a new bound, not a raised one.)
- **No new required contract field** in a hook or event payload.
- **No gate loosened and no exception added** (`ci/quality.json` only ever tightens; no allow-list,
  baseline, bulk `//nolint`).
- **Refactors keep the keymap, schema, rendering and timing identical** except where a requirement
  below names the change.
- **Terminal bytes outside an escape string are never altered** by R209's filter: ordinary text,
  including every non-ASCII character, reaches the emulator byte-for-byte.

## Ground rules

- **No path is protected in this phase.** The operator explicitly authorises the job to edit any
  file it needs, including `SPEC.md`, `prds/`, `ci/Dockerfile`, `.github/workflows/release.yml` and
  `ci/releasegate/`. Edits stay within what a requirement needs: this is not licence to loosen a gate,
  weaken a test or change the release gate's rule beyond R214. `SPEC.md` is updated inside each task
  that changes behaviour (R215).
- **Never touch the operator's live state:** `~/.local/share/deck/`, `~/.config/deck/`,
  `~/.local/state/deck/`, `~/.claude/`, any `tmux -L deck` or `tmux -L deck-*` server. Tests use temp
  `HOME`/`XDG_*`/`DECK_HOME`/`CLAUDE_CONFIG_DIR` and private sockets.
- **Docker is for `ci/run.sh`, `ci/stability.sh` and the gate scripts, and nothing else** (plus the
  read-only registry lookup R214.6 needs). No `docker prune`, no wildcard or label-filtered
  `rm`/`rmi`, never signal by pattern (resolve a pid, verify it, then signal that pid). Never touch
  the `deck-ws-*` runners.
- **No paperwork.** No report, findings, audit or close-out file in the repo, no
  `docs/DELIVERY-LOG.md` row. Measurements and run ids go to `/run/ralphd/artifacts`.
- **GitHub scope.** The job may push to `main`, read the Actions API, dispatch `ci.yml` on `main`,
  and **comment** on the issues in scope with the commit and the test node that closes each. It does
  **not close issues**: the operator does. No branches, PRs, tags or releases; no cancelling,
  re-running or deleting runs; no change to settings, rulesets, environments, Pages, runners or
  secrets.
- **CI must stay green.** No red or rerun-rescued run on `main` is acceptable.

## Requirements

Order of work: R207, R208, R209 (wave 1, bugs), R210, R211 (wave 2, themes), R212, R213, R214
(wave 3, hardening), then the sweep. Each requirement is a behaviour plus the test that fails without
it. `features/` scenarios are named by what they assert.

### R207 — restart before the first message starts the session again (#70, bug 1)

Deck launches Claude with `claude --session-id <uuid>`; Claude writes no transcript until the first
message, so a relaunch with `claude --resume <uuid>` fails ("No conversation found") and every later
`r`/`R` fails the same way.

The relaunch decision, over this **finite** set of cases (the resume argv choice, `resumeArgv` in
`internal/service/resume.go`, and `Resume`/`Launch` in `internal/agent/claude.go`):

1. Claude, `HasTranscript`, conversation **not pinned**, the home directory known, no
   `CLAUDE_CONFIG_DIR` override, and **no transcript file** at the expected path for the row's
   conversation id → relaunch with `Launch` on the **same** id (`--session-id <same uuid>`), not
   `Resume`. The conversation id is unchanged.
2. A transcript file exists → `--resume`, as today.
3. The conversation is pinned → `--resume`, as today.
4. `CLAUDE_CONFIG_DIR` is set → `--resume` (the transcript location cannot be known).
5. The home directory is unknown → `--resume`.
6. Codex and Pi → unchanged, whatever the transcript state.
- The existing `resume_state = 'fresh-once'` arming is unchanged.
- **Tests:** one unit test per case 1-6 in `internal/service` and `internal/agent` (the argv, and for
  case 1 that the id is reused); a test that a Claude session restarted twice before any message ends
  `running`, not `error`. A `features/` scenario with the fake Claude: create, restart before any
  message, and the session is running on the same conversation id.

### R208 — a dead pane is not "Lost attach" (#70, bug 2)

- `checkInteractiveDisplacementBackstop` (`internal/tui/interactive_displacement.go`): when the
  interactive read fails because the target no longer resolves, or the tick reports `PaneDead`, it
  sets `paneDead` only and leaves `displaced` **false**. `PaneDead` is excluded from the claim and
  attach checks. `raiseLostAttach` is not called, so the "Another client took over this session's
  window." dialog does not appear; the existing dead-pane path (`NotePaneDead`) is what the operator
  sees.
- A genuine takeover (another client holds the window, the pane alive) still raises "Lost attach".
- **Tests:** unit: a failed read sets `paneDead` and not `displaced`; `PaneDead` skips the claim and
  attach checks; a genuine takeover still sets `displaced` and raises the dialog; the dead-pane
  message path is shown. `features/`: the pane's agent exits while interactive, and no "Lost attach"
  dialog appears.

### R209 — an OSC title never leaks into the grid (#69)

The OSC state of `charmbracelet/x/ansi@v0.11.7` is byte-based: byte `0x9C` inside the UTF-8 `✳`
(`E2 9C B3`) ends the OSC early and the rest of the title prints at the cursor.

- A **pre-filter** sits in front of `Grid.Write` and is **the one place** shared by the live drain,
  the seed and `captureLoop` (`internal/interactive/grid.go`: `Session.drain`, `Grid.Write`). While
  inside an escape **string** — exactly these five kinds: OSC (`ESC ]`), DCS (`ESC P`), SOS (`ESC X`),
  PM (`ESC ^`), APC (`ESC _`) — it drops bytes `0x80-0x9F`. The string still ends on `BEL` (OSC only)
  or `ESC \`. Outside a string the bytes pass through unchanged (non-negotiables).
- It **carries state across writes**: a read split anywhere, including between `E2` and `9C B3`, gives
  the same grid as an unsplit one. It does not buffer output: bytes are forwarded as they arrive.
- The pane never needs the title, so nothing else changes: the emulator still sees the string's
  start and end.
- **Tests** (`internal/interactive/grid_test.go`): `ESC ]0;✳ name BEL` at a cursor leaves the row
  unchanged; each of the titles `✳ ✶ ✻ ✽ ✓` with each of `BEL` and `ESC \` leaves the grid blank; each
  of those cases split at **every** byte boundary gives the same grid; the typed-space sequence from
  the issue yields `❯ x y`; the same title leak test for one DCS, one SOS, one PM and one APC string
  carrying `✳`; ordinary text containing `✳`, `日本`, `é`, a box-drawing character and an emoji is
  byte-identical to a no-filter write; the seed and `captureLoop` paths each have one test through the
  shared filter. A `features/` scenario replays the Claude title sequence into an interactive pane
  and asserts the input row is unchanged.

### R210 — `cobalt` and `empire` are visibly different (#68, part 1)

- **`cobalt`** keeps its saturated cobalt background; its title, key, search-match and waiting accents
  move to cool tones (ice, cyan, white), with one warm colour reserved for warnings.
- **`empire`** moves off navy: a near-black charcoal / graphite background with amber and brick-red
  accents and gold titles.
- **A pairwise distance test** in `internal/theme`: for **every unordered pair of built-in themes**,
  the mean CIEDE2000 ΔE over this fixed token set — `background`, `surface`, `title`, `text`, the
  key token, `border_focus`, and the seven §7 status colours — is **at least 15**. The test names
  the closest pair when it fails. It iterates the registry, so a later built-in is covered with no
  new test. The machinery in `contrast_test.go` is reused.
- Both themes still meet the contrast floor and the quantisation pinning.
- **Tests:** the pairwise test (fails on the pre-change `cobalt`/`empire`); the existing contrast and
  quantisation tests green for both.

### R211 — four new built-in themes (#68, part 2)

- Add exactly these built-ins, each one TOML file plus one registry entry, no per-theme code:
  - `gruvbox-dark` — warm brown dark; `solarized-dark` — teal-grey dark; `amber` — black with
    monochrome amber phosphor; `high-contrast` — black and white with saturated primaries, clearing
    WCAG AAA (7:1) for `text` on `background`.
- **Decisions on the issue's open questions:** palettes of gruvbox and solarized follow the upstream
  colours by name, and each such file carries a header comment crediting the upstream project and its
  MIT licence; pastel low-contrast themes (rose-pine, catppuccin) and the other candidates are not
  shipped; no light/dark pairing setting is added.
- Each new theme supplies the seven §7 status colours as **pairwise distinguishable** tokens within
  the theme (for `amber`, by tone and brightness, not hue alone). A test asserts, for every built-in,
  that its seven status colours are pairwise ΔE ≥ 10.
- Each new theme meets the contrast floor and has its quantisation pinned the way `matrix` does
  (`matrix_status_quantization_test.go`); the pairwise distance of R210 holds with all nine.
- The theme list in `SPEC.md` §11.6 and in `README.md` names all nine.
- **Tests:** the registry resolves each of the four names; each loads without fallback; each passes
  contrast, quantisation, the status-distinguishability test and R210's pairwise test; `high-contrast`
  clears 7:1; an `features/` scenario selects one new theme in Settings and the screen paints it.

### R212 — the writer re-exec cannot wedge a hook (#71 item 1)

- `runWriter` in `cmd/deck/hook_reexec.go` runs the recorded writer under a context with a **10 s**
  timeout (a package variable so tests shorten it). On timeout the writer process is killed and the
  hook falls back to the R204 "restart the session" message and its usual exit, exactly as for a
  writer that is not usable.
- **Tests:** unit: a writer fake that sleeps past the shortened timeout is killed, the hook prints the
  restart message and returns; a fast writer still heals. The 3 s `_schema` probe is unchanged.

### R213 — path and process hardening (#71 items 4, 5, 6)

- **Pi hook (item 4).** `internal/agent/pi_hook.go`: the Pi extension runs the hook as **separate argv
  elements** (executable path, then `_hook`), never through `sh -c` with a joined string. A path
  containing spaces, quotes or `;` is passed intact.
- **Transcript path (item 5).** Wherever a transcript path is built from a conversation id supplied by
  a hook payload (`internal/agent/claude.go` and the same pattern in the Codex and Pi code), the id is
  validated first: an id that is empty, contains a path separator, or is `.` / `..` is rejected, and
  no path outside the transcript directory is ever opened.
- **Pipe reclaim (item 6).** `ReclaimLeakedInteractivePipes` (`internal/tmux/reclaim.go`) removes a
  `/tmp/deck-interactive-pipe-*` entry only when it is a real directory (not a symlink), owned by the
  current uid, and has mode no looser than 0700. Anything else is left alone.
- **Tests:** unit: the Pi hook argv for a path with a space, a quote and a `;`, asserting no shell is
  involved; one rejection test per invalid id form (empty, `/`, `..`, `a/b`, `.`) per harness that
  builds a path, and a valid id still resolves; reclaim removes an owned 0700 directory and skips a
  symlink, a directory with a different owner (via an injectable owner check), and one with mode
  0755.

### R214 — CI and release hardening (#71 items 2 and 7)

- **Release gate reachability (item 2).** `ci/releasegate` also requires that the tagged sha is
  reachable from `origin/main` (`git merge-base --is-ancestor <sha> origin/main` or the equivalent
  compare API call, behind an injectable function). A green `suite` run for a sha that is not an
  ancestor of `main` does not satisfy the gate. `release.yml` fetches what the check needs. The tag
  protection ruleset half of the item is a GitHub setting and is **out of scope**.
- **Base images by digest (item 7).** Every `FROM` line in `ci/Dockerfile` pins its image by
  `@sha256:<digest>` keeping the tag as a trailing comment. Digests are read from the registry
  (`docker buildx imagetools inspect` or `docker manifest inspect`, read-only) for the exact tags
  already in the file; no tag or version changes.
- **Tests:** releasegate: an ancestor sha passes, a non-ancestor sha fails, the check function erroring
  fails the gate, and the existing gate tests pass unchanged; the coverage floor and CRAP ceiling hold
  for the package. `ci/workflowcheck`: a `FROM` line without a digest in `ci/Dockerfile` fails the
  check; the real file passes.
- **Out of scope:** item 3 (a published signature or attestation for `install.sh` to verify) needs a
  release to exercise and a product decision; it stays open in #71 and is not started.

### R215 — the spec matches the code

`SPEC.md` is updated in the same commit as each behaviour above, in present tense: the restart and
resume decision and the interactive displacement/dead-pane behaviour (R207, R208), §11.9 for the
escape-string filter (R209), §11.6 for the theme set and the pairwise-distance rule (R210, R211), the
hook, transcript and pipe sections for R212 and R213, and the release gate wherever it is specified
(R214). No history, no "shipped in" lines. The sweep's reviewer finds no sentence in the spec that
contradicts the live code for these behaviours.

## Definition of done

Behaviours R207–R214 shipped, each with its named tests green; the spec consistent with the code
(R215); no secret exposed; **all lanes green at the final pushed sha**, proven by the push run at
that sha plus **one** `workflow_dispatch` with an empty flaky record; the three sweep tasks done at
the same `origin/main` head with nothing committed after them. Nothing else.

## Out of scope

#65 (the event-hook rewrite of SPEC §10), #71 item 3 and the tag-protection half of item 2, themes
other than the four named in R211, and anything not listed above. Do not start #65, and do not leave
`internal/notify` changes behind.

## Escape hatch

If the run proves that a requirement contradicts `SPEC.md` beyond what R215 lets it fix, that a
requirement cannot be met without a gate exception (for example a named theme that cannot reach both
its identity and R210's distance of 15), or that a named test cannot be written, it must not delete a
test, add an exception or quietly narrow the requirement. File a petition with the evidence and the
smallest change that would resolve it, notify the operator, and carry on with everything the petition
does not block. The operator rules by amendment.

## Notifications

Use the `notify` hat tool in the same iteration as the work, a few hundred characters, never a
secret: anything that blocks work outright (including a petition), each wave finished, every
rejection and cure pass, the proof run's result, and one terminal summary. A missed or late send is
cured by mentioning it in the next one; it is never a success criterion and never a finding.

## Budget and shape

Iterations 200, approaches 2, `vigilant` on, a 120-minute iteration cap, a 12-hour wall-clock
window. Every requirement is one implementation task (code, tests and spec together), then the three
sweep tasks; R211 is pre-split into (a) the four theme files and registry entries with their
per-theme tests, (b) the status-distinguishability test and the SPEC/README lists; R213 is pre-split
into its three bullets. The final sweep task is marked `sweep: true`.

## Materiality rubric

Reject only on substance:

- a requirement's behaviour is absent or wrong in the live code or workflows;
- a test the requirement names is missing, red, or also passes on the unfixed tree;
- a gate is loosened, an exception is added, or a test was deleted, skipped or weakened to pass;
- any proof run in the Definition of done is red or has a non-empty flaky record;
- the spec contradicts the live code for a behaviour in this PRD;
- a secret is exposed, in the tree, a workflow log or a published report;
- a guard in the Ground rules is broken (the operator's deck directories or a `tmux -L deck*`
  server touched, a branch/PR/tag/release created, a run cancelled or re-run, an issue closed).
  Editing a formerly protected path is **not** a finding.

The working clauses: file the rule, not the example; read the direction of the evidence (a line that
could support either side supports neither); a review after a cure pass confirms the cure landed and
does not re-audit the product; judge a hat or router by its contract, not its taste (a theme's
palette taste is not a ground: the contrast, distance and distinguishability tests are); a stale
sweep is never process.

Everything else verifies, with the gap recorded as a residual note: wording, form, provenance,
process, and **any number or claim in prose outside the spec** (commit messages, issue comments,
notes, run artifacts). There is no advisory list. One exception: a run that never reached `suite`
because GitHub failed to provision a runner, or because of an Actions API 5xx, is not judged.
