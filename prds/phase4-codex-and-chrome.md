# Phase 4 — Codex, chrome legibility, and the field backlog

## Goal

Six GitHub issues and the last unbuilt agent. In priority order:

1. **Codex** (SPEC R6, §8, §8.2) — the fourth agent, specified since day one and never built.
   `internal/agent/` has `claude.go`, `pi.go`, `shell.go` and no `codex.go`; `cmd/` has
   `fake-claude` and `fake-pi` and no `fake-codex`; `features/codex_hooks.feature` (§13.3)
   does not exist. R6 says four agents; deck ships three.
2. **#22** — `claude` with the default `safe` profile passes `--permission-mode manual`, which
   Claude Code ≤ 2.1.71 rejects outright, so deck cannot launch a session at all on that
   version. A one-line adapter fix with a SPEC row behind it.
3. **#23** — a resumed session's preview stays 80×24 until the user selects away and back,
   because the passive-fit coalescing latch is keyed to the selection and no relaunch clears
   it. The one row the user is watching is the one row that stays wrong.
4. **#24** — deck never paints `theme.Background`, so the canvas is the *terminal's*
   background: both light built-ins render near-black text on a dark terminal, legible only
   on the rows the `surface` stripe happens to paint. The selection cue is a low-contrast
   background with no gutter.
5. **#26** — row cosmetics: the `created` label costs eight columns on every row to say
   nothing, the permission badge sits first on line 2 instead of last, and the `✓ marked`
   badge is the first thing truncation drops.
6. **#25** — manual session groups, replacing cwd-derived workspace grouping entirely. **This
   is Tier 2**: see Materiality. It is the largest item and the last one attempted.

**#27** (multi-tenant profiles) is explicitly **not** in this phase.

## Read this before anything else

- **`SPEC.md` is already correct and is the authority.** The operator amended it for this
  phase before the run started: §5's permission table and the paragraph above it (the verified
  Claude and Codex flag surfaces, and why `safe` names no mode), §8/§8.1/§8.2 (Codex
  instrumentation and what replaces discovery), §11's passive-fit coalescing rule, §11.3 (deck
  paints its own canvas; the selection gutter), §11.6 (`background` is the canvas; the
  contrast floor gains two pairs), §11's row-composition bullets (two lines, bare age,
  permission badge last, the mark cue in the gutter), and — for Tier 2 — §4's `groups` table,
  §6.3's `DECK_SESSION_GROUP`, §6.5's key list, §10's payload, §11's grouping bullets, §11.5's
  groups section and its lifecycle carve-out, §11.8, §11.10, §12 and §13.3.
  **Where this PRD and `SPEC.md` disagree, `SPEC.md` wins** and the disagreement is a finding
  for `docs/reports/phase4-findings.md`, never an edit.
- **Protected paths — `SPEC.md`, `prds/`, `ci/Dockerfile`, `ci/SPIKE.md` — are read-only for
  this job**, no exception, and steering cannot license one. The audit for this run is exactly
  this command, and it must print nothing:

  ```sh
  BASE=$(git log --format=%H --diff-filter=A -1 -- prds/phase4-codex-and-chrome.md)
  git log --oneline "$BASE..HEAD" -- SPEC.md prds/ ci/Dockerfile ci/SPIKE.md
  ```

  The base is the commit that **added this file**, computed rather than pasted, so that commit
  and the SPEC amendments before it are outside the range by construction. Nothing about
  earlier history is this run's problem and no report should re-litigate it.
- **The CI container has no agent binaries at all** — not `claude`, not `pi`, and not `codex`
  (`ci/Dockerfile` installs none, and it is protected). Every scenario therefore runs against
  the `cmd/fake-*` stubs on a fixture `PATH`, and R114 (phase 3k) already established both the
  step vocabulary and the reason: after availability probing, a kind whose executable does not
  resolve is simply not offered. A `codex` scenario that forgets its fake will not fail with a
  dead pane — the Agent field will not contain `codex` at all, and the scenario will time out.
  That is the harness working.
- **The Codex probe corpus is supplied, not invented.** `internal/agent/testdata/probes/codex/`
  and its provenance file were captured from the real codex-cli 0.154.0 by the operator before
  this run, in the same shape as the `pi` corpus (`pi-PROVENANCE.md`, and
  `docs/reports/phase3-fake-pi-transcript-provenance.md` for how that one was established).
  **Probe rules are derived from those bytes.** Inventing a pattern that no captured frame
  contains, or editing a fixture so a rule matches it, is a fake green — see Materiality.
- **`cmd/fake-codex` must not become the specification.** The fake exists so scenarios are
  deterministic; the *contract* is the real CLI's, as recorded in SPEC §5 and §8 and in the
  spike report cited by this PRD (`docs/reports/codex-cli-0.154.0-spike.md`). Where the fake and the real CLI could drift, the fake is
  what is wrong. `features/fake_agent_drift_test.go` already exists for exactly this class of
  drift on the other agents — extend that idiom rather than inventing a second one.
- **Read these before writing any code:**
  - Codex: `internal/agent/claude.go` (the closest adapter — instrumentation, argv, caps) and
    `internal/agent/pi.go` (transcript-path convention with a documented provenance);
    `internal/agent/agent.go` (`Caps`, `Adapter`, `Registry`, `Executable`, `ResolveProfile`);
    `cmd/deck/main.go`'s `_hook` path and `cmd/deck/hook_test.go`; `internal/service/agent.go`
    and `resume.go`; `cmd/fake-claude` for the fake's shape.
  - Chrome: `internal/tui/panel.go` (every chrome builder, `truncateToWidth`, `padTrunc`),
    `internal/tui/tui.go`'s `sidebarRowLines` / `sidebarRowBackground` / `sidebarEntries`,
    `internal/tui/settings.go`'s `settingsRenderRowOpen` and the six settings line builders,
    `internal/theme/theme_color.go`'s `foregroundSGR`/`backgroundSGR` doc comments (they
    already warn about the exact reset-clears-background gotcha R118 turns on), and
    `features/panel_background_rectangle.feature`.
  - Fit: `internal/tui/tui.go`'s `previewFit`, `previewFitDone`, `previewFitSessionID`,
    `previewFitInFlight`, and the `sessionResumed`/`sessionRestarted` handlers.
  - Groups: `internal/tui/group.go` in full, `internal/store/store.go`'s `DefaultWorkspace`
    and `scanSession`, `internal/config/schema.go`'s `group_by_workspace` row, and
    `internal/tui/registry_guard_test.go` for the black-box idiom.
- **The docker socket is for `ci/run.sh` and nothing else.** The gate runs the suite in a
  sibling container (`ci/run.sh go test -p=1 -count=1 ./...`), which is why this job has the
  socket at all. **Never remove or kill containers by label** — a previous ralphd job on this
  host SIGKILLed itself by sweeping containers matching `label=ralphd.run`, which is its own
  engine container. Never `docker prune`, never a wildcard `docker rm`/`rmi`, and never signal
  by pattern (`pkill -f`, `killall`): resolve a pid, verify what it is, signal that pid. Other
  runs' containers and content-hashed images live on this host and are not this job's to tidy.
- **Adding an agent kind must still need no edit under `internal/tui`.** That is the
  registry's reason to exist and `TestBlackBoxRegistrySwapNeedsNoTUIEdit` pins it. Codex is
  the first new kind since that guard was written: it is the proof, not the exception. If
  Codex appears to need a TUI edit, the missing thing is data the service layer should be
  handing the TUI, exactly as availability was in phase 3k.

## Materiality

Review has one rubric for this phase, and it is this section.

### Tiers

**Tier 1 is the phase.** Codex (every requirement below marked T1), #22, #23, #24, #26.
Acceptance requires all of it.

**Tier 2 is #25, manual session groups, and its absence is NOT a rejection.** It is attempted
only after every Tier 1 requirement is green, and it is **all-or-nothing**: either the whole
group model lands (schema, migration, render, create/move/settings/delete, features) or it is
not started at all. A half-migrated store is a defect; an unstarted tier is a budget outcome.

Because the operator amended `SPEC.md` for Tier 2 up front, an unstarted Tier 2 leaves SPEC
describing manual groups while the code still groups by `workspace`. **That gap is expected,
accepted, and explicitly NOT a finding** — neither a blocking one nor a curable one. It is
recorded in one paragraph of the phase report naming what SPEC says and what the code does,
and the operator reverts or re-lands the SPEC section afterwards. A run that "cures" the gap
by editing SPEC has modified a protected path, which *is* blocking.

### Classes

**Blocking** — a Tier 1 requirement's stated behaviour is absent, contradicted, or asserted by
a test that passes against a product that does not have it (a fake green); a probe rule that
matches no captured frame, or a fixture edited to fit a rule; a gate not run at the final code
sha; a protected path modified by this run; a secret or real credential anywhere in the tree;
`--dangerously-bypass-hook-trust` or any `--dangerously-*` flag in a launch argv deck composes
for a profile other than the one SPEC §5 names it for; a Codex session that records a
conversation id deck did not learn from an authoritative source (a guess, a "most recent", or
a scan whose result was not claimed transactionally); a Tier 2 store migration left partially
applied; a test-only env knob in product code (R8); a narrowed sweep.

**Curable in place** — a wrong or missing citation, a stale sha or count, a thin report
section, a scenario title, a findings-file omission, a doc-comment wording, a probe rule that
is correct but under-commented. Cure it with a docs-only commit; it does not invalidate the
gates and does not force a replan. **Phase 3j lost five of its seven approaches to this class
being treated as blocking; it is not.**

**Advisory, never blocking** — the Tier 2 SPEC gap above; a carried-forward finding from an
earlier phase recurring in the gate (report it with its log path); a stability measurement
below 10/10 published honestly with every failure named; anything the codex spike left
unverified and this PRD marks as such; style, altitude and naming opinions; **any operator
notification not sent, sent late or not logged** (see Operator notifications, which says the same
thing from the other side).

### Termination

The **final code sha** is the last commit that touches `*.go` or `*.feature`. Both gates are
run at it and the record cites it. A **docs-only tail commit** — the report, the findings file,
`docs/DELIVERY-LOG.md`, or a citation cure — does not invalidate either gate and is itself
exempt from re-verification. The record is written **once**, at that sha; it does not embed
self-counting scripts, does not publish citation totals, and is not re-audited by a later
task. There is no fixed point to chase: cite the code sha, then write the documents.

## Operator notifications

**The operator is AFK and reading Telegram.** The engine's `notify` hat tool is configured
(`RALPHD_NOTIFIER_URL`) and records every send as a `notification.sent` event. **Use it, never a
direct POST**, and send in the **same iteration** as the work being reported.

**A send is advisory: never a success criterion, never a task's failure condition, and never a
finding.** A missed or late send is cured by mentioning it in the next send, naming what went
unreported. This is stated so a notification duty can never become a reason to reject an
approach — the point of these sends is that the operator can act while asleep, not that a ledger
balances.

Send, at least:

1. **Immediately, ahead of anything else — anything that blocks work outright**: a plan with no
   selectable task, a requirement that collides with a protected path, a fake agent binary that
   is missing from the CI container, a gate that cannot run. Name what is stuck and the decision
   needed.
2. The plan's acceptance, plus any plan-gate objection and how it was answered.
3. **Each Tier 1 requirement as it goes green** — R116 through R127 — naming the requirement, the
   test that proves it and the sha.
4. **The Tier 1 → Tier 2 decision**, explicitly: whether Tier 2 is being started, and the
   remaining iteration and wall-clock budget that decided it. If Tier 2 is not started, say so
   plainly; that is an expected outcome, not a failure to apologise for.
5. Each Tier 2 requirement (R128–R131) as it goes green, if the tier was started.
6. Every petition, in the petition's own iteration.
7. Every approach rejection and every cure pass, with what review actually found.
8. The first full `ci/run.sh` gate result, and any later transition from green to red.
9. The stability run's result, with every failure named even when the headline is clean.
10. **One terminal summary**: the final code sha, both gate results, which requirements landed,
    Tier 2's fate, every petition with its adjudication, every steer and ruling, and anything cut
    with its reason.

## Requirements

### R116 — `safe` names no permission mode (T1, GH #22)

- `internal/agent/claude.go`: the `safe` profile contributes **no** `--permission-mode` flag to
  either `Launch` or `Resume` argv. `plan`, `edits` and `yolo` keep `plan`, `acceptEdits` and
  `bypassPermissions` exactly as now. SPEC §5's table is the authority.
- `cmd/fake-claude`: a missing `--permission-mode` is **valid** and means the default mode. An
  unknown *value* is still rejected — the fake's job is to catch deck composing nonsense, and
  weakening that is not part of this.
- The scenarios and unit tests that assert `manual` in argv are updated to assert its absence.
  `features/permission_modes.feature` is the main one; `internal/agent/claude_test.go` covers
  the argv itself.
- **Success:** a `safe` claude session's launch and resume argv contain no `--permission-mode`
  at any position; the other three profiles' argv are byte-identical to today's; no test still
  asserts `manual`.
- **Why it matters, stated so it is not "simplified" back:** the flag was not wrong, it was
  *version-dependent*. Claude renamed the mode `default` → `manual` between 2.1.71 and 2.1.259,
  so the fix is to stop naming a value whose name churns, not to name the other one.

### R117 — a relaunch re-fits the preview (T1, GH #23)

- `internal/tui/tui.go`'s passive fit coalesces on `previewFitSessionID`, which latches the
  selection identity when an attempt completes. `r`, `R` and the `u` that undoes a kill each
  replace the pane with a new tmux window at tmux's own default 80×24 **without moving the
  selection**, so the latch is still shut when the new window appears and the fit never runs.
  Clear the latch on the relaunch outcomes that actually created a pane.
- **Only those outcomes.** `ResumeAlreadyRunning`, `ResumeStartingElsewhere` and
  `ResumeNotLeasable` created nothing and must not clear it. `previewFitInFlight` is a
  different guard (one attempt at a time) and is **not** touched by this requirement.
- Batch `u` clears the latch for every session it restored, not just the selected one.
- **Success:** a unit test that resumes the *selected* session and shows the next preview tick
  issues a fit for it, with no selection change in between; a test that a no-op resume outcome
  does not clear the latch; a feature scenario asserting the preview panel reports the panel's
  geometry rather than 80×24 after `r` on the selected row.
- **Do not** fix this by passing `-x`/`-y` to `new-session` in `internal/tmux`. The service
  layer has no idea what the panel's geometry is, and hard-coding one would make every
  headless create carry a TUI's dimensions. The stale latch is the bug.

### R118 — deck paints its own canvas (T1, GH #24)

- Every line deck renders opens the `background` token (SPEC §11.3, §11.6) and closes it, so
  the canvas is deck's, not the terminal's: sidebar and preview borders, padding columns, row
  bodies and their pad-fill, group/section headers, the socket line, the empty state, banners
  and notes, the footer, and all six settings line builders plus every dialog frame.
- **A reset clears the background too**, so painting means re-opening it after every inner
  `\x1b[0m` — a single prefix per line stops at the first coloured glyph. `internal/theme`'s
  `foregroundSGR`/`backgroundSGR` doc comments already describe this trap; the composition
  helper belongs next to them or next to `panel.go`'s other builders, in **one** place.
- **Captured pane content is never repainted.** deck paints the frame and the padding around a
  capture and emits a reset after it so the pane's colours cannot leak into deck's frame. A
  light canvas composed under an agent's bright-on-dark output destroys it.
- **Success:** on every built-in theme, a rendered frame's every cell that deck owns carries
  the theme's `background` (assert over the emulator's cell attributes, the way
  `features/panel_background_rectangle.feature` and `cell_attributes_token_test.go` already
  do), including the columns outside a short row's text; a captured-pane cell carries the
  pane's own attributes and not deck's; `NO_COLOR` and `DECK_COLOR_DEPTH=16` both still render
  a frame whose borders and text are in the right cells.
- The two light built-ins are the reason this exists: `daylight` and `parchment` currently
  render near-black text on whatever the terminal's background is. A frame test that only ever
  runs a dark theme cannot see this, which is why the assertion is per-built-in.

### R119 — the selection gutter, and the mark cue in it (T1, GH #24 + #26)

- The selected row's leftmost columns are a bar painted in `accent`, with `>` on the row's
  first line drawn in the `background` token as its foreground. A **marked** row (SPEC §11's
  `m`) carries `✓` on the second line of the same bar — `*` under `DECK_ASCII` — so a row that
  is both selected and marked shows both cues. A marked-but-unselected row's bar is `badge`.
- The `✓ marked` **text badge leaves line 1's badge run entirely.** It was the last segment
  there and therefore the first thing `padTrunc` dropped, which lost the cue exactly when the
  sidebar was too narrow to count marks by eye.
- **The marker lives in its own columns, outside the row's text run.** This is not a style
  preference: if the marker is composed inside the text and the row background re-opened after
  it, `truncateToWidth` can cut inside that background span on a long name and synthesise a
  reset there, after which the canvas takes over the ellipsis and the trailing pad columns —
  breaking the highlight rectangle SPEC §11.3 requires, on exactly the truncating row that
  `features/panel_background_rectangle.feature` exists to catch. The content budget shrinks by
  the gutter's width; the row's text is handed to `padTrunc` carrying no background span at all.
- Selection wins the bar's colour on a row that is both. The mark is never signalled by colour
  alone, and neither cue is lost under `NO_COLOR`, because both are text in a fixed column.
- `internal/theme`'s contrast floor gains `background`-on-`accent` and `background`-on-`badge`,
  over both the hex palette and its 16-colour quantisation, with no allowlist. Measured before
  this PRD was written, every built-in clears 3:1 on both pairs; the thinnest is `parchment`
  quantised at 3.18:1. **A pair that fails is a finding, never a licence to recolour a theme.**
  One honest note for the comment at the paint site: `parchment` quantises `accent` and `badge`
  to the same reference colour, so on a 16-colour terminal its two bars are the same colour —
  acceptable, because the glyphs differ and sit on different lines.
- **Success:** unit tests for all four states (plain / selected / marked / both) at the
  sidebar's minimum width; a feature scenario asserting the gutter's cells by background token;
  the contrast test covering both new pairs on every built-in.

### R120 — the row says less and means more (T1, GH #26)

- Line 2 renders the age **bare** — `2m ago`, `just now` — with no `created` label, and the
  permission badge (`[safe]`, `[yolo]`, …) moves to the **end** of that line, after the age.
  `env↻` and `launch↻` keep their place ahead of the age. SPEC §11's row-composition bullets
  are the authority.
- The literal assertions that break are known and few: `internal/tui/tui_test.go`,
  `features/sidebar_width_test.go` and `features/determinism_test.go` assert `created …`
  strings; `features/i1_repro_test.go` asserts `[marked]` in four places (R119 moves that cue).
  **The harness's frame scrubber is safe** — `features/pty_driver_test.go`'s `relativeTime`
  regex matches `just now|\d+[mhd] ago` without the `created ` prefix, so normalisation across
  every scenario is unaffected. Do not "fix" the scrubber.
- **Success:** no rendered frame contains the string `created `; the permission badge is line
  2's last segment; the age still normalises under a frozen `DECK_CLOCK`.

### R121 — the Codex adapter's argv and declared capabilities (T1)

- `internal/agent/codex.go` implements `Adapter` and is registered in the registry. SPEC §5's
  table and §8's are the authority for every string below.
- Declared caps: `Executable: "codex"`, `Resumable: true`, `HasTranscript: true`, and
  **`AssignsConversationID: false` — codex is the first adapter with that**, so the create path
  must not mint a uuid for it, must not pass one, and must store none. `LaunchInput`'s own doc
  comment already says the field is "Empty otherwise"; prove it rather than assume it.
- Profiles: `safe`, `edits`, `yolo`. **`plan` is deliberately absent**, exactly as it is absent
  from `piProfiles` — codex has no plan mode, so requesting it degrades through the caller's
  existing `ResolveProfile` and says so in the UI. Do not add a codex-specific degrade path and
  do not alias `plan` to `safe` inside the adapter.
- Flags, always explicit and never inherited from the user's `config.toml`: `safe` →
  `-a on-request -s workspace-write`; `edits` → `-a never -s workspace-write`; `yolo` →
  `-a never -s danger-full-access`. **`--dangerously-bypass-approvals-and-sandbox` is never
  composed** (SPEC §5 prefers the structured flags), and neither is `--full-auto`, which does
  not exist in 0.154.0 at all.
- `Launch` argv is `codex` + those flags + `ExtraArgs`, with **no id argument of any kind**.
  `Resume` argv is `codex resume <conversation id>` + those flags + `ExtraArgs`, and errors when
  the id is empty. `--last` is never composed, in any code path (R2).
- `TranscriptPaths` locates
  `<codex home>/sessions/<yyyy>/<mm>/<dd>/rollout-<ISO>-<conversation id>.jsonl` by globbing the
  date directories, and returns not-ok rather than an error when nothing matches — pi's and
  Claude's convention. The codex home is `$CODEX_HOME` when the **session's own env layering**
  (§6.1) sets it and `<home>/.codex` otherwise; `TranscriptInput` has no way to express that
  today, so extend it and let the caller fill it in. Do not read the ambient environment from
  inside the adapter: a session that overrides `CODEX_HOME` is exactly the case that would then
  silently search the wrong tree.
- **Success:** argv tests for all three profiles on both launch and resume, and for an unknown
  profile; a test that `Launch` succeeds with an empty `ConversationID` and that `Resume` refuses
  one; a `TranscriptPaths` test over a fixture tree including a session-env `CODEX_HOME` override
  and a miss; a grep-style guard that no codex code path can emit `--last`,
  `--dangerously-bypass-approvals-and-sandbox` or `--full-auto`;
  `TestBlackBoxRegistrySwapNeedsNoTUIEdit` still green with codex registered.

### R122 — Codex instrumentation: inline hooks, one constant command (T1)

- `Instrument` returns, for each of SPEC §8.2's five events, one `-c` argument of the shape
  `hooks.<Event>=[{hooks=[{type="command",command="<deck>  _hook"}]}]`, plus
  `--dangerously-bypass-hook-trust`, plus `DECK_LAUNCH_GENERATION` in the env map on exactly the
  same absent-when-no-lease terms Claude's does. **Nothing is written to any file**: no
  `hooks.json`, no `config.toml`, nothing under `$CODEX_HOME`, not even a temp file.
- The events are `SessionStart`, `UserPromptSubmit`, `PermissionRequest`, `Stop`, `SessionEnd`.
  `PreToolUse` and `PostToolUse` are **excluded on purpose** — they fire once per tool call and
  `PostToolUse` carries the tool's entire output — and so are `PreCompact`, `PostCompact`,
  `SubagentStart`, `SubagentStop` and `Interrupt`, which map to no §7 status. A comment says so,
  because the next reader's instinct will be to subscribe to everything available.
- **The hook command is a constant** — deck's own executable path and the literal `_hook`, the
  same string Claude's `Instrument` builds via `shellQuote`. Nothing per-session may enter it:
  codex's hook-*trust* hash covers the command string (SPEC §8.2), so a command carrying a
  session id would need a different trust entry per session. Per-session identity travels in the
  payload and in the pane environment, which the hook process inherits — verified against the
  real CLI: a hook launched by codex sees `DECK_SESSION_ID`.
- **The `-c` value must be generated, not concatenated.** It is TOML embedded in a shell argument
  embedded in the deck executable's own path: a path containing a quote, a backslash or a space
  must round-trip. Write the encoder narrow and test it against such a path rather than trusting
  a format string.
- `--dangerously-bypass-hook-trust` is the **one** `--dangerously-*` flag deck ever composes and
  it is composed for **every** codex launch regardless of profile, because it governs hooks only
  and not approvals or the sandbox. SPEC §8.2 records why the alternatives (harvesting the trust
  hash from codex's interactive UI; a deck-owned `$CODEX_HOME`) are worse. Do not re-litigate
  that choice, and do not make it a config knob.
- **Success:** an `Instrument` test asserting the exact argument list, one `-c` per event, no
  file created anywhere (assert on a temp `$CODEX_HOME` that stays empty), the flag present, and
  the encoder surviving an adversarial executable path; a test that the command string contains
  no session-specific substring.

### R123 — the receiver understands Codex's one new event (T1)

- `internal/hookrecv`'s `Mappings` gains **`PermissionRequest` → `waiting`**, with `tool_name` as
  the reason field so a row reads `waiting · Bash` or `waiting · apply_patch`. That is the whole
  payload change: codex reuses Claude's field names (`session_id`, `cwd`, `transcript_path`,
  `hook_event_name`, `last_assistant_message`, `permission_mode`, `source`) and three of its
  event names, so `SessionStart`, `UserPromptSubmit`, `Stop` and `SessionEnd` already map
  correctly. **Do not build a second receiver, a per-agent mapping table, or a codex payload
  struct.**
- Pin the behaviour deck already has, with codex payloads captured from the real CLI, so a later
  refactor cannot quietly lose it: a codex `SessionStart` sets the row's conversation id
  (requirement 44's existing path — the row is resolved by `DECK_SESSION_ID` because deck does
  not yet know the id); a codex `Stop` records `last_assistant_message`; a codex `SessionEnd`
  with `reason: "other"` **does** stop the row, because `other` is not one of requirement 43's
  in-session reasons.
- `DECK_LAUNCH_GENERATION`'s superseded-launch guard applies to codex hooks unchanged, including
  the rule that a superseded `SessionStart` must **not** move the row's id onto a conversation
  that is already over.
- Real payload text for the tests is in `docs/reports/codex-cli-0.154.0-spike.md` (Q3c) and must
  be copied, not paraphrased.
- **Success:** receiver tests driven by the captured codex payloads for all five events, incl.
  both `PermissionRequest` shapes (`Bash` and `apply_patch`), a `null`
  `last_assistant_message`, and a superseded `SessionStart`.

### R124 — Codex probe rules, fitted to the supplied corpus (T1)

- `internal/agent/probe.go` gains codex rules and `probeGoldens` gains an expectation for
  **every** file in `internal/agent/testdata/probes/codex/` — the golden test already fails if a
  corpus file has no expectation, and that is the mechanism, not an obstacle.
- The verdicts the corpus asserts: `starting.txt` → `starting`; `running.txt` → `running`;
  `waiting.txt` and `waiting-patch.txt` → `waiting` (both approval shapes — a rule that catches
  the command prompt but not the edit prompt is half a rule); `idle.txt` → `idle`;
  `error.txt` → `error`; and **`retrying.txt` → `running`**, which is the point of that file: a
  codex retrying a network error shows `esc to interrupt` exactly as a working one does, so
  `running` is the honest verdict and only the exhausted state (leading `■`, no
  `esc to interrupt`) is classifiable as an error from the pane. Reason strings are the
  planner's, and they appear in the UI, so choose them the way pi's were.
- **`starting` and `idle` are not distinguished by the banner.** Both fixtures still show the
  banner box and the `Tip:` line; they differ by whether a transcript cell is present. A rule
  keyed on the banner is wrong on both counts — it also scrolls away on a long session, which is
  precisely why pi's banner was rejected as a marker.
- No rule may key on the fixtures' first line (`WARNING: proceeding, even though we could not
  create PATH aliases…`): that is an artefact of the capture's `CODEX_HOME` living under `/tmp`,
  documented as such in `codex-PROVENANCE.md`, and deck's own sessions never print it.
- The corpus and its provenance file are **inputs, not deliverables**. Editing a fixture so a
  rule matches it is a fake green (see Materiality). If a fixture genuinely cannot be classified,
  that is a finding with the offending lines quoted.
- **Success:** the golden corpus test green over all seven files; a test that ordinary pane text
  yields no codex verdict; the `retrying`/`running` collision documented at the rule site.

### R125 — no id yet is a state, not an error (T1)

- Codex's `SessionStart` **does not fire at launch** — verified twice against the real TUI, it
  fires when the first prompt is submitted (SPEC §8.2). For that window a codex row therefore has
  no conversation id, and deck must handle it as a *state*:
  - the row's status comes from the probe, and its quality badge reads `sampled` — which needs no
    new code, because `statusSourceQuality` already keys on the last verdict's source rather than
    the agent kind. **Do not add a per-kind badge.** Assert the existing behaviour instead.
  - `r`/`R` refuse with a stated reason — the row has no conversation to resume — instead of
    resuming with an empty id, guessing, or scanning for a "most recent" transcript (R2). The
    refusal is worded for a human and appears wherever refusals already appear.
  - the `i` detail dialog shows the missing id honestly rather than as a blank field.
  - killing and re-creating such a row is an ordinary fresh launch; it is not a resume.
- Once a `SessionStart` arrives, the row shows the id it carried and reads `live`. From then on
  resume behaves like any other adapter's, and a resumed codex session's own `SessionStart`
  arrives with `source: "resume"` and the **same** id — so id adoption must be idempotent and
  must not log a change when nothing changed.
- **Success:** a feature scenario that creates a codex session, asserts no id / `sampled` /
  resume refused, drives one prompt through the fake, then asserts the id, `live`, and resume
  offered; a unit test that a second `SessionStart` with the same id is a no-op.

### R126 — `cmd/fake-codex` honours the shape codex actually has (T1)

- A new fake beside `fake-claude` and `fake-pi`, following their idiom and registered wherever
  they are (the fixture `PATH` builder, `features/fake_agent_drift.feature`'s inventory, the
  drift test).
- It models codex's argv contract, **not** Claude's: it **rejects `--session-id`** (deck must
  never pass one — the fake is where that regression is caught), accepts `resume <id>` as a
  positional subcommand, accepts `-a` with only `on-request|never` and `-s` with only
  `read-only|workspace-write|danger-full-access`, rejects `--full-auto` as an unexpected
  argument, and accepts `-c <key>=<value>` repeatedly.
- **It reproduces codex's silent-skip trust behaviour**: it parses the `-c hooks.…` overrides but
  fires **nothing** unless `--dangerously-bypass-hook-trust` is also present — no error, no
  warning, exactly like the real CLI. That makes "deck forgot the flag" fail a scenario instead
  of passing one.
- It fires the five events on command with the real payload field names, and it fires
  `SessionStart` **on first prompt, not at launch**, so R125's window is reproducible in the
  harness rather than only in prose.
- It writes a transcript at the real rollout path (`sessions/<yyyy>/<mm>/<dd>/rollout-<ISO>-<id>.jsonl`)
  whose first line is a `session_meta` object carrying `session_id` and `cwd`, so R121's
  `TranscriptPaths` and §12's search have something real to find.
- It mints its own uuid per session (codex's behaviour), prints recognisable pane text for each
  probe state, and can be told to hang, crash or exit like the others.
- **The fake is not the specification.** Where it and the real CLI could drift, the fake is what
  is wrong; the contract is SPEC §5 and §8.2 and the spike report.
- **Success:** the fake's own tests for each rejection and for the trust gate; `fake_agent_drift`
  covering codex; every codex scenario running against it with no real binary anywhere.

### R127 — Codex scenarios, and one real-agent conformance scenario (T1)

- `features/codex_hooks.feature` exists per SPEC §13.3 and contains §13.4's `@codex` scenario
  verbatim in behaviour: two codex sessions created in one directory within two seconds each
  have no id, read `sampled` and refuse resume; after one prompt each, each reads `live` with the
  **distinct** id its own `SessionStart` carried, and neither holds the other's transcript; then
  an approval request puts one row in `waiting` with the tool name as its reason.
- Creating a codex session is covered in the ordinary places too — `agent_availability` (the kind
  is offered only when `codex` resolves on `PATH`, R114's mechanism), permission profiles
  (`plan` degrading and saying so), resume argv, and `dd`/transcript survival.
- The `@real-agents` suite gains **one** codex scenario, in the existing idiom
  (`features/real_agent_hooks_test.go`, `real_agent_smoke.feature`): it is excluded by the
  default tag filter, skips with a stated reason when `codex` is not installed, and is never part
  of the CI gate — the CI container has no agent binaries. Its job is to be runnable by the
  operator against a real codex when a release lands, which is what SPEC §8.2's "re-verified by
  `@real-agents` rather than trusted indefinitely" means in practice.
- **Success:** `codex_hooks.feature` green in the gate against the fake; the `@real-agents` codex
  scenario present, tagged, and skipping cleanly on a host with no codex.

## Tier 2 — manual session groups (GH #25)

Attempted only after every Tier 1 requirement above is green, and **all-or-nothing**: the four
requirements below land together or none of them starts. See Materiality for why an unstarted
tier is a budget outcome rather than a finding, and for the SPEC gap it leaves.

### R128 — the group model replaces the workspace label (T2)

- `state.db` gains SPEC §4's `groups` table and `sessions.group_id`; `sessions.workspace` and
  `store.DefaultWorkspace` (the basename-of-`cwd` fallback) are **removed**, not deprecated in
  place. `group_id` carries **no** foreign key deliberately: the delete flow (R131) chooses
  between two branches for a group's members, and an `ON DELETE SET NULL` would pre-empt one of
  them silently.
- **`default` is `group_id IS NULL`.** There is no row for it, so "always exists / always last /
  never renamed / never deleted" need no enforcement, and a `group_id` that does not resolve
  (another client deleted the group between load and render) reads as `default` rather than
  vanishing.
- **Migration: every existing session lands in `default`** (`group_id = NULL`) and the group list
  starts empty. This is a deliberate clean break — seeding a group per distinct old `workspace`
  value would recreate exactly the cwd-derived grouping this replaces.
- Name rules: trimmed, non-empty, no control characters, unique case-insensitively
  (`COLLATE NOCASE`), `default` in any case rejected as reserved, and a length cap chosen against
  the sidebar's minimum content width and stated in a comment.
- `DECK_SESSION_WORKSPACE` becomes `DECK_SESSION_GROUP` (the group's name, empty for `default`)
  and §10's notification payload field `workspace` becomes `group`. **No alias for either** — a
  hook reading the old name sees nothing set, which is the operator's explicit decision.
- **Success:** a migration test over a fixture DB with several distinct old `workspace` values,
  asserting every row reads `default` afterwards and the file still loads; uniqueness and
  reserved-name tests; a pane-env test for the renamed variable; no `workspace` identifier left
  in `internal/store` or `internal/tui`.

### R129 — the sidebar renders manual groups (T2)

- Group order is **alphabetical, case-insensitive, `default` always last**; rows within a group
  follow `[ui] sort_order`. `reorderPreservingGrouping` — the R53 rule that made the group with
  the most urgent member lead — is **deleted**, along with the flat no-header mode and
  `[ui] group_by_workspace` (config key, env override, schema row and settings row). Grouping is
  unconditional now: there is nothing to switch off once the groups are the user's own.
- **Every header carries its member count, `(0)` included**, and a defined-but-empty group still
  renders. Under an active filter only groups with a match render, and the count shown is the
  matching count.
- **Collapse persists** in `ui_state` by group id, so a collapsed group is still collapsed after
  a restart. Every navigation primitive in `internal/tui/group.go` keeps its current contract —
  one keypress moves one visual row, selection never lands on a hidden row — keyed on group id
  rather than a workspace string. `c` and the §11.8 header click both still toggle.
- The header drops the representative-`cwd` suffix it shows today: a manual group's members can
  span any number of directories, so one member's `cwd` is noise.
- `/` matches name, **group** and `cwd`.
- **Success:** order tests including a group named `zzz` and one named `aaa` with `default` still
  last; count rendering for populated and empty groups; collapse surviving a Model rebuild from
  `ui_state`; the navigation-parity tests that currently prove flat/grouped agreement rewritten
  to prove grouped behaviour alone (they are the ones that will notice a hidden row becoming
  selectable); a dangling `group_id` rendering under `default`.

### R130 — membership is set where the session is (T2)

- The create modal gains a `Group` field that cycles the available groups exactly as `Agent`
  cycles kinds — alphabetical, `default` last — and defaults to the **last group created into**,
  persisted in `ui_state` beside the existing last-used-agent key (`GetLastCreateAgent` is the
  precedent to mirror, including its fallback when the remembered value no longer exists).
- The `i` detail dialog shows the session's group and opens a picker to move it. The key is the
  planner's choice from what is free inside that dialog (`r` is already rename there); whatever
  it is, `?` and the dialog's own footer name it.
- **The marked set is not extended to moves.** `x` and `dd` remain the only batch verbs (SPEC
  §11), and this is a scope line the operator drew deliberately, not an omission to fill in
  helpfully.
- **Success:** a create-into-a-group scenario asserting the persisted `group_id`; the remembered
  default surviving a restart and falling back when its group is gone; a move via `i` asserting
  one row changed and no other did.

### R131 — the group list is edited in settings (T2)

- Settings gains a groups section: `n` creates (with inline validation errors), `r` renames, `d`
  deletes. **It is not staged** — groups live in `state.db`, so edits apply immediately, the
  section says so, and `esc` must not offer to discard what it cannot discard (SPEC §11.5).
- Deleting a non-empty group prompts for one of two branches: **move all its members to
  `default`**, or **delete them**. The destructive branch runs through §9.2's existing `dd` batch
  path — its confirm dialog naming what will go, its tombstones, its transcript-purge offer, its
  `DECK_DELETE_GRACE_MS` window and its single-`u` batch restore. Deleting an empty group prompts
  nothing. `default` offers no delete at all.
- **No second implementation of deletion exists after this.** If the `dd` batch path needs a seam
  to be callable from settings, add the seam; do not copy the deletion. SPEC §11.5's lifecycle
  carve-out is written narrowly on purpose and this requirement is the whole of it.
- **Success:** both branches as feature scenarios, the destructive one asserting tombstones and
  that one `u` restores the whole batch; an empty-group delete with no prompt; a rename that
  carries every member; `esc` in the groups section offering no discard prompt.

### R132 — the record matches the tree (T1)

- `docs/reports/phase4-report.md` states, per requirement: what shipped, the commits, the tests
  that prove it, and — for anything that did not ship — what is missing and why. Tier 2's status
  is stated plainly, including the SPEC-versus-code gap in one paragraph if it was not started.
- `docs/reports/phase4-findings.md` carries every finding this run made and chose not to fix,
  each with a file:line and a reason. The two known-unverified codex items belong here by name:
  whether `acceptEdits`/`plan`/`dontAsk` are reachable on codex at all, and whether the trust
  hash is stable across codex versions (all measurements are 0.154.0).
- `docs/DELIVERY-LOG.md` gains this phase's entry in the existing shape.
- Both gates are reported with their commands, their durations and the sha they ran at — the
  final code sha per Materiality's termination rule.
- **Success:** every requirement number R116–R131 appears in the report with a verdict;
  no requirement is reported green whose test does not exist.

## Ordering

Tier 1 first, and within it the cheap correctness fixes before the wide ones, because they are
independent and finishing them early makes the phase partially deliverable at any point:

1. **R116** (`safe` names no mode) and **R117** (relaunch re-fits) — small, local, each with a
   clear test. R116 touches the fake and two feature files; R117 touches one latch.
2. **R118** (deck paints its own canvas) before **R119** (the gutter) and **R120** (the row's
   line 2), because both compose *onto* the canvas R118 establishes and a gutter built first
   would be rebuilt.
3. **R121**–**R127**, the codex block, in that order: the adapter's argv, then instrumentation,
   then the receiver's one mapping, then the probe rules, then the no-id state, then the fake,
   then the scenarios. R126 (the fake) can start earlier if a scenario needs it sooner — but it
   cannot be *finished* before R122, since its trust gate mirrors R122's flag.
4. **Tier 2 (R128–R131)** only once all of the above is green, in its own numbered order:
   model and migration, render, membership, settings.
5. **R132** last, at the final code sha.

## Green when

- `go build ./...` and `go vet ./...` clean.
- **`gofmt -l` clean on every file this run touches.** It is *not* clean on the tree today —
  `internal/theme/quantize_test.go` and three files under `.spike-preview/` have pre-existing
  alignment drift, no gate has ever enforced gofmt, and reformatting them is unrelated churn.
  Leave them; a report line naming them is enough, and their presence is not a finding.
- **`ci/run.sh go test -p=1 -count=1 ./...` green** at the final code sha — the whole suite, in
  the container, `-p=1`, no narrowed package list and no `-run` filter. A sweep narrowed for any
  reason is a blocking finding (Materiality), including "the wide run is slow": it takes about
  six minutes and that is the price of the gate meaning something.
- The features package run for stability the way phase 3k did (`-count=10` or the repeat idiom
  already in the tree), with **every** failure named and its log path published even when the
  headline is 10/10. Two flake classes are known open — the transient-`starting` assertion and a
  `SIGWINCH` exact-count assertion — and recurring instances of those are advisory, reported with
  evidence, not chased.
- The protected-path audit from "Read this before anything else" prints nothing.
- Every requirement in this PRD has at least one test that fails if the behaviour is removed —
  the standing bar, and the one Materiality calls a fake green when it is missing.

## Non-goals

- **#27, multi-tenant profiles.** Not in this phase, not partially, not "the `DECK_HOME`
  groundwork". It reshapes where every path lives and it is the next phase's whole subject.
- **#19, force-attach.** Its design is settled on the issue and it is not in this scope.
- **A codex `plan` profile.** Codex has no plan mode on 0.154.0 and no flag combination reached
  `acceptEdits`/`plan`/`dontAsk` (R132 records this as unverified). `plan` degrades and says so.
- **Seeding codex's hook trust**, harvesting its trust hash, or shipping a deck-owned
  `$CODEX_HOME`. SPEC §8.2 chose the flag; changing that choice is a SPEC amendment, which this
  run cannot make.
- **Subscribing to codex's other seven hook events** (`PreToolUse`, `PostToolUse`, `PreCompact`,
  `PostCompact`, `SubagentStart`, `SubagentStop`, `Interrupt`). None maps to a §7 status.
- **Pi hooks.** Still an open question (§14.2) and still out of scope; pi stays sampled.
- **Recolouring any built-in theme** to pass a contrast pair. A failing pair is a finding.
- **Extending the marked set** to any verb beyond `x` and `dd`.
- **Touching `internal/tui` to add a kind.** If codex seems to need it, the missing thing is data
  the service layer should hand the TUI.

## For the planner

- **The spike report is in the tree: `docs/reports/codex-cli-0.154.0-spike.md`. It is evidence,
  not folklore.** Every codex fact in this PRD was measured
  against a real codex-cli 0.154.0 on 2026-09-12 — the event set, the payload fields, the
  first-prompt timing, the flag values, the silent trust skip, the resume behaviour, the pane
  anchors. Where a requirement says "verified", it was; where it says "unverified", do not
  quietly promote it. If a codex fact this PRD asserts turns out to be false against the real
  CLI, that is a finding with the command you ran, not a licence to invent a workaround.
- **The receiver is the surprise.** Codex's hook payloads use Claude's field names and three of
  its event names, and deck's receiver already adopts a conversation id from `SessionStart`. The
  correct codex adapter is therefore *small* — one new mapping entry, one argv builder, one
  instrument function, one probe rule set. A large codex diff is a sign something is being
  rebuilt that already exists. In particular: SPEC's old `DiscoverID` / discovery-lease design is
  **gone** (§8.2), and building any part of it is out of scope.
- **Two things about codex will feel like bugs and are not**: `SessionStart` arriving only at the
  first prompt (R125), and a retrying network error being indistinguishable from working (R124).
  Both are recorded upstream behaviour with a spec'd response. Do not "fix" either.
- **R118 is the widest change in the phase** and touches nearly every builder in
  `internal/tui/panel.go`. Do it as one composition helper used everywhere, not as a background
  prefix sprinkled per call site; the reset-clears-background trap (`\x1b[0m` inside a line) is
  why the sprinkled version silently half-works.
- **Tier 2 is genuinely optional.** If the budget is thin when Tier 1 goes green, stop and write
  the record. An unstarted Tier 2 costs nothing; a half-migrated `state.db` costs the operator
  their session list.
- **Do not add a config knob to make any of this optional.** Not the canvas, not the gutter, not
  grouping, not the codex trust flag. SPEC removed `group_by_workspace` rather than adding to it.
- Ask via steering if a SPEC sentence and this PRD genuinely contradict each other on something
  that changes the code. SPEC wins; the point of asking is that the operator may want to amend
  SPEC, which only they can do.
