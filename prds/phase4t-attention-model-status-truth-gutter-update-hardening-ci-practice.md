# Phase 4t — the attention model: status truth, output clock, seen/unseen, gutter click, update hardening, CI practice

## What the operator gets

1. A session's status says what it is doing. A finished Claude session reads `idle`, not `running` or `waiting`; a
   `waiting` row means a human decision is pending; a `running` row whose probe keeps missing is marked unverified
   instead of asserting a stale verdict (#98).
2. Looking at a row never moves it or changes its status. `activity` sort follows real pane output, bucketed, not
   the last status change (#97).
3. "Have I seen this?" is one rule: `seen_status_at == status_at` replaces the `acknowledged` bit, and a repeat
   probe no longer un-acknowledges a row (#101). A separate unseen marker and `Alt+U` / `Alt+Shift+U` jump to
   panes that changed since they were last viewed (#100).
4. A click in the cursor gutter of a row detaches or navigates without attaching (#99).
5. `deck update` error text cannot carry terminal control sequences, and the ctrl+u dialog and indicator say
   honestly when an update installs on checksum only (#94, #95).
6. CI costs nothing for draft PRs and runs on every push to `main`; the working practice (branch, PR, squash,
   final dispatch) is written down (#93).

## Issues in scope

#93, #94, #95, #97, #98, #99, #100 and #101, nothing else. Read each in full, including every comment; this PRD
wins wherever it differs from an issue. Where two issues disagree the order of authority is #101, then #98, then
#97, #100 and #99. In particular: **engagement sets "seen" and does not change status or move a row** (#101 over
#97's "engagement releases the row"); status changes only on evidence (#98).
**Not in scope:** #73 (Kiro), a new agent, native Windows, a v2 Bubble Tea migration, any change to how bookmarks
work, and applying GitHub branch protection (an operator action, see R339).

Code to follow (symbols as of `3d0a97f1a95`; drift is expected and is not a finding — if a reference has drifted,
find the current site and carry on):

- Store: `internal/store/store.go` (`writeStatusUpdate`, `applyAttachmentToStatusTx`, `AcknowledgeSession`,
  `StatusUpdateInput`, the schema and the column-drop precedent `schemaV10`), `hook_newer_schema_test.go`.
- Service: `internal/service/reconcile.go` (`recordProbe`, `probeEligible`, the probe capture window).
- Agents: `internal/agent/probe.go` and its fixtures/goldens, `claude_gate.go` (`PromptReady`), the per-agent hook
  tables (SPEC §8.1), `lastContentLine`.
- TUI: `internal/tui/attention.go` (`attentionRank`, `NeedsAttention`), `tui.go` (`canAcknowledge`,
  `sidebarBadgeParts`, the footer), `list_keys.go` (`listKeyHandlers`, `Y`, `space`), `interactive.go` (keys
  intercepted next to Ctrl+Q and the bookmark keys), the mouse code (SPEC §11.8).
- Update: `internal/update/`, `updateFailedText`, the ctrl+u confirmation, `cmd/deck/` update verb.
- CI: `.github/workflows/ci.yml`, `docs/ci.md`, SPEC §13.2, the Go test(s) that parse `ci.yml`.
- SPEC §7 (status, acknowledge), §8.1, §11 (sort, keymap, glyphs, §11.8 mouse, §11.9 intercepted keys), §13.

## Requirements

### Part A — foundations (#97, #101)

- **R303 (a repeat write is inert).** A status write that leaves the status unchanged does not touch `status_at`,
  `status_source` precedence outcome, `notify_epoch`, `hook_fired` or the seen state. `last_probe_at` may still
  advance. Finite cases, each a store or service test that fails at the parent: probe repeats `waiting`; probe
  repeats `running`; probe repeats `error`; hook repeats the same status with the same reason; hook repeats the
  same status with a different reason (reason updates, `status_at` does not); a user-source repeat. A row cleared
  with `Y`, then re-probed `waiting`, keeps its position in `attention` and `activity` sort.
- **R304 (output clock).** New column `sessions.last_output_at`, fed at reconcile from tmux `window_activity` and
  confirmed against the pane: the **transcript region** (above the same boundary `lastContentLine` uses, i.e. above
  the last two separator lines) is captured and hashed when the bucket advances, and only a changed hash counts.
  Footer/statusline churn (`ctx:%`, branch, model) never counts. Written only when the bucket changes (a column,
  never an event; same rule as R59 probe misses). Finite cases, table tests: transcript change counts; footer-only
  change does not; no change does not; two changes inside one bucket write once; the column is not written on a
  tick with no bucket change.
- **R305 (deck-caused output discounted).** Output within a short window (the run chooses and states the length
  in SPEC; at most one bucket) after a deck-caused resize (preview fit/SIGWINCH), an attach, or a forwarded key
  is not counted. Cases: after resize, after attach, after a forwarded key, and output after the window counts.
- **R306 (bucket setting and activity sort).** `[ui] output_bucket` (default `10s`, a quoted duration or whole
  seconds, minimum 1s, `DECK_OUTPUT_BUCKET` overrides like its siblings), in the schema, applied live, and a
  settings-menu entry. `activity` sort orders by `last_output_at` descending with an `id` tie-break inside a
  bucket, so running sessions do not reorder under the cursor. SPEC §11 `activity` is redefined (it currently says
  "status change, not pane output"). Tests: sort by bucket, tie-break stable across ticks, live apply, the config
  key round-trips. A config reload never writes.

### Part B — status truth (#98)

- **R307 (SessionStart lands on idle).** `SessionStart`, including `--resume` and `--continue`, sets `idle`, never
  `running`, for the three agents whose hook table maps it: Claude, Codex, Copilot. `UserPromptSubmit` (or a
  working indicator seen by the probe) is what enters `running`. One test per agent; a resumed session with no
  prompt stays `idle` across a probe tick that matches no rule.
- **R308 (idle_prompt changes no status).** Claude's `idle_prompt` notification changes no status and writes no
  attention transition; it may stamp `idle_since` metadata (a column or field the run names in SPEC). Cases:
  `idle_prompt` on an `idle` row; on a `running` row (stays `running`); on a `waiting` row raised by
  `permission_prompt` (stays `waiting`). `Stop` then `idle_prompt` leaves `idle` (no flip-flop).
- **R309 (waiting is a human decision).** `waiting` is produced only by `permission_prompt` /
  `elicitation_dialog` notifications and by the probe rules for permission dialog, question/elicitation, plan
  approval and trust prompt. Tests: each producer yields `waiting`; no other notification kind does.
- **R310 (Claude idle probe without the banner).** Claude probes `idle` when the footer
  (`⏵⏵ … (shift+tab to cycle)`) is present and no `esc to interrupt` working indicator, **regardless of composer
  content** (an unsent draft does not change status; `Claude.PromptReady` already detects drafts). The old
  `Claude Code` + `❯` banner rule is gone. Goldens, each a probe test: scrolled pane with footer and empty
  composer; same with a draft; same on the alternate screen with no scrollback; pane with `esc to interrupt`
  (`running`). Goldens carry a provenance note stating they reproduce the observed pane shapes; the footer is
  checked against both bypass and default permission mode text when the repo evidence allows, and the SPEC states
  what was and was not verified.
- **R311 (live-region anchoring).** For Claude, `API Error:` and the permission-dialog markers count only in the
  live region (below the last transcript line, above the composer), not anywhere in the capture. Cases: an old
  `API Error:` above a live composer is not `error`; an answered `Do you want to proceed?` above a live composer
  is not `waiting`; a live dialog is `waiting`; a live error is `error`.
- **R312 (Claude waiting shapes).** The probe recognises these dialog shapes as `waiting`, each a golden: the
  `Do you want to proceed?` permission dialog (existing); edit/create/write file confirmation; an AskUserQuestion
  choice list; plan approval. The markers are stated in SPEC §7. A shape whose marker cannot be established from
  repo evidence is petitioned, not guessed.
- **R313 (Codex ■ anchored).** Codex `■` is `error` only on the last content line, the way pi's `Error:` is
  anchored. Cases: `■` in a markdown body, in a progress bar and in tool output is not `error`; a trailing `■`
  error line is.
- **R314 (idle rows are probed).** `idle` rows are stale-eligible at a slower cadence, stated in SPEC (4 x
  `stale_after`); `error` rows are not. Cases: an `idle` row is not probed before the cadence and is after;
  a probe that sees the working indicator moves it to `running` with source `probe`; a probe that matches nothing
  leaves it `idle`. No existing bound is raised.
- **R315 (unverified).** A `running` row with no event for 3 x `stale_after` whose probe has missed 3 times in a
  row is shown as unverified (glyph decoration, with `ascii` form; the run states it in SPEC §11). The miss streak
  is a column, never an event. Cases: 2 misses, 3 misses, a probe hit clears it, a hook verdict clears it, the
  status text asserts nothing new, the sort position is unchanged.
- **R316 (movement fallback).** An `idle` row with fresh, non-deck transcript-region output (the R304/R305 clock)
  becomes `running` and returns to `idle` after a quiet period. Hysteresis: output in two consecutive buckets to
  enter, one quiet bucket to leave. Pi (no hooks) uses movement directly; Claude, Codex and Copilot use it only
  as the stale fallback when no fresh hook verdict exists, and a fresh hook verdict always wins. Cases: pi enters
  on two buckets; pi does not enter on one; footer-only churn never enters; deck-caused output never enters; a
  hooked agent with a fresh hook ignores movement; a hooked agent with a stale hook uses it; leave after one quiet
  bucket.
- **R317 (spec).** SPEC §7 is rewritten in present tense with the agreed definitions (`running`, `waiting`,
  `idle`), "evidence, not time", `SessionStart` landing on `idle`, `idle_prompt` changing no status, the draft rule,
  the unverified marker and the movement fallback; §8.1 tables match R307-R309. This is done inside the tasks
  above, not as a docs-only task.

### Part C — seen_status_at replaces acknowledged (#101, #97)

- **R318 (schema and migration).** `sessions.acknowledged` is dropped and `sessions.seen_status_at` added
  (precedent: `schemaV10`). Backfill `seen_status_at = status_at` where `acknowledged = 1`, else 0.
  `StatusUpdateInput.Acknowledged` goes. Tests: migration of a database holding both values; an older `_hook`
  binary against the newer schema stays green (`hook_newer_schema_test.go`).
- **R319 (the predicate).** A row's status is seen iff `seen_status_at == status_at`, equality and never an
  ordering. `NeedsAttention = waiting || (error && unseen)`, and sort tier, collapsed-strip count and `space`
  all use that one predicate; the invariant in `attention.go` that they never disagree is kept and tested. A
  seen `waiting` stays counted and a `space` target. A seen `error` drops out of the count and `space` and sorts
  just above `stopped`. `Y` marks seen without attaching, is offered only where something is unseen, and is a
  no-op otherwise. Finite cases, each a test: waiting unseen; waiting seen; error unseen; error seen (count,
  `space`, tier, `Alt+U` reachability); `Y` on each; a clock stepped back after seen, then a later prompt is
  still unseen; two stamps in the same millisecond cannot hide a prompt.
- **R320 (attach and preview are inert).** Attach (full or interactive), click and passive preview never change
  `status` or the seen state, and the row does not move, in `attention` and in `activity`. The old
  `waiting` -> `running` flip on attach is removed. Cases: attach on `waiting`, `error`, `running`, `idle`, in both
  sort modes; click; passive preview. The passive-preview non-perturbation guarantee stays green unmodified.
- **R321 (engagement sets seen).** In attach or interactive preview, the first forwarded key, the first scroll
  (including passive-preview scroll) and the first screen-content change on an unseen `waiting`/`error` row set
  `seen_status_at = status_at` through one code path. Engagement sets seen only; it changes no status and moves
  no row. One test per trigger, plus: a second engagement writes nothing.
- **R322 (dwell).** A dwell of 1.5 s with the pane in view sets seen only when the prompt was already on screen
  at the start of the dwell. A prompt that appears mid-dwell stays unseen until engagement. Cases: prompt present
  at start; prompt appears mid-dwell; pane collapsed; deck unfocused; arrowing past a row.
- **R323 (surface and spec).** The `●` marker and the `Y` slot read the new predicate; the ~35 feature assertions
  that name `acknowledged=N` are rewritten to the seen vocabulary, each in the commit that changes the behaviour
  and naming the covering test it becomes. SPEC §7 (acknowledge), the schema listing and §11 are updated in the
  same commits.

### Part D — unseen marker and jump keys (#100)

- **R324 (last_seen_at and marker).** New column `sessions.last_seen_at`, written only on a view change or bucket
  change, never per tick. A row is unseen iff `last_output_at > last_seen_at` OR (attention status AND status not
  seen per R319). One marker, drawn with a glyph distinct from the `waiting` status glyph (the `●` collision in
  SPEC ~2591 is resolved; `ascii` form stated), plus an "N unseen" count in the header. A `running` row shows no
  output-unseen marker. Setting or clearing the marker never moves a row, in any sort order. Cases: output after
  view sets it; view clears it; attention-unseen sets it; running row shows none; statusline-only churn does not
  set it; no row moves when it flips, in each of the four sort orders.
- **R325 (what counts as viewed).** Viewed = the passive preview visible on the row for >= 1.5 s, or interactive
  mode on it. While a row stays selected with a live preview `last_seen_at` keeps advancing, so a watched session
  never goes unseen. A collapsed preview or an unfocused deck views nothing; arrowing down the list does not
  clear every row. One test per case.
- **R326 (Alt+U / Alt+Shift+U in the list).** `Alt+U` jumps to the next unseen session in list order from the
  current row, wrapping; `Alt+Shift+U` the previous. With nothing unseen a one-line message. Landing on a row
  views it but moves no row and acknowledges nothing. `space` is unchanged (blocked on me, by status). The
  decoder test proves `Alt+Shift+U` is distinguishable from `Alt+U` through tmux, or the SPEC states it as a gap
  (as for `Alt+Shift+Tab`) and a test pins the decoder behaviour as it is. Cases: next, previous, wrap, none,
  seen `error` reachable until seen, `space` unaffected.
- **R327 (Alt+U in interactive mode).** In interactive mode both chords are intercepted next to `Ctrl+Q` and the
  bookmark keys, send zero bytes to the pane, and switch the attached view while staying interactive (as a
  bookmark jump does). Footer hint, `?` help and SPEC §11 keymap and §11.9 intercepted-key list state them.
  Cases: forward jump, backward jump, none unseen, zero bytes forwarded, stays interactive.

### Part E — gutter click (#99)

The cursor **gutter** of a row is the cell(s) at the left edge that hold the `>` marker (the same cells on every
other row). Decision on the issue's open question: a gutter click while **not** interactive is select-only,
independent of `attach_on_click`.

- **R328 (behaviour).** While interactive: gutter of the attached row leaves interactive mode (same as empty
  sidebar space / `Ctrl+Q`); gutter of any other row selects it and does **not** enter interactive mode on it;
  main area of a row is unchanged (select and enter interactive preview); empty sidebar space is unchanged.
  Gutter clicks are inert on group headers (header clicks keep their collapse behaviour). While not interactive: a
  gutter click selects only. A gutter navigate is not an engagement: the row it lands on does not move and its
  seen state is untouched (R320). One scenario or table test per row of this list.
- **R329 (geometry and spec).** The gutter geometry is tested at the 80x24 minimum and with `ascii` glyphs, with a
  scenario in the mouse feature. SPEC §11.8 table gains the new rows in the same commit.

### Part F — update hardening (#94, #95)

- **R330 (control characters).** Server-derived text (redirect path, HTTP reason, `checksums.txt` field, `gh`
  stderr) is stripped of C0 and C1 control characters (U+0000-U+001F except none retained, U+007F, U+0080-U+009F)
  before it reaches `updateFailedText`, the footer, the ctrl+u dialog and `deck update` stderr; printable and
  multi-byte UTF-8 text is preserved. Credential redaction still applies. Cases: ESC, BEL, a C1 CSI byte, an OSC
  title sequence, a newline, a normal string, a multi-byte string; each at the three sinks.
- **R331 (checksum-only is said).** The TUI keeps `Result.Attestation` and says so when an update installed on
  checksum only: the Installed indicator names it, and the ctrl+u confirmation tells the truth before it runs
  (attestation is verified only with an authenticated `gh`; otherwise the checksum alone, which catches
  corruption only). With `DECK_REQUIRE_ATTESTATION=1` and no `gh` the dialog says the update will be refused.
  Cases: attested; checksum-only; require variable with no `gh`; wording tests assert the state, not the prose.
  SPEC and README state the checksum-only case.

### Part G — CI working practice (#93)

- **R332 (draft PRs and push filter).** `ci.yml` triggers on `pull_request` types `opened`, `synchronize`,
  `reopened`, `ready_for_review`, with a job-level `github.event.pull_request.draft == false` guard (kept next to
  the existing fork guard) so a draft PR starts no jobs. The `push` to `main` trigger has **no** `paths-ignore`:
  `main` runs CI on every change. `schedule` and `workflow_dispatch` are unchanged; the concurrency group is
  unchanged. A Go test parses `ci.yml` and asserts the types list, the draft guard on every job that has the fork
  guard, and the absence of a push `paths-ignore`; the test that currently pins the docs-only filter is rewritten
  to this one, named in the commit. `release.yml` and `ci/releasegate` are untouched.
- **R333 (practice documented).** `docs/ci.md` gains a "Working practice" section and SPEC §13.2 a pointer:
  agent runs work on a feature branch (branch pushes start no CI); the PR is opened or marked ready, and merged
  by squash, only when the whole scope is done; the merge gate is the full CI gate with no admin bypass; the
  final verification is a `workflow_dispatch` on the merged sha and nothing is committed to `main` after it;
  release is cut from `main`. This PRD's own run still pushes to `main`, as the standing rules say. No test
  asserts the prose.

Pre-split into tasks at plan time: R303; R304+R305; R306; R307; R308+R309; R310; R311; R312; R313; R314; R315;
R316; R317 is satisfied inside each Part B task; R318; R319; R320; R321; R322; R323 inside the tasks that change
the behaviour (the `acknowledged=N` assertion rewrite is split by feature file, one task per group the run names);
R324; R325; R326; R327; R328+R329; R330; R331; R332+R333; then the three sweep tasks. Order: Part A before B and C;
C before D; D's keys after R324; E, F and G are independent and may go anywhere after A.

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
- **Commits are made per task and pushed to `main`** (this phase only; R333 describes the practice for later
  phases). Title `<area>: <imperative summary> (task NNN, R###)`.
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
  sha, the sweep is stale and re-runs. A stale sweep is never process. Closing #93, #94, #95, #97, #98, #99, #100
  and #101 is a comment naming the commit and the test node, made after the sweep.
- **The CI dispatch recipe.** Use the Actions REST API with the operator's PAT from the git credentials:
  `POST /repos/n-orlov/deck/actions/workflows/ci.yml/dispatches` with `{"ref":"main"}`, then poll
  `GET /repos/n-orlov/deck/actions/runs?event=workflow_dispatch&head_sha=<sha>` to its conclusion, and read the
  run's log and artifacts. Dispatch **once**, at the final pushed sha. A run that never reached `suite` (runner
  provisioning failure, API 5xx) is not judged: dispatch another.
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
  covering test it becomes. Nothing else about the suite is retired. The passive-preview non-perturbation
  guarantee stays green unmodified.
- **No raised timeout, budget, worker count or per-test bound.** `internal/racebuild` is untouched.
- **No new required contract field** in a hook or event payload deck *receives* from an agent.
- **No gate loosened and no exception added** (`ci/quality.json` only ever tightens).
- **Existing behaviour is identical** except where a requirement names the change; keys other than `Alt+U` /
  `Alt+Shift+U` reach the agent exactly as before.
- **Status is never changed by looking.** No attach, click, preview or gutter navigation writes a status.
- **Deck-caused output is never counted as activity.**
- **A config reload never writes.** Profiles stay isolated.
- **No secret** is written to a log line, an event, the update cache or a golden file.

## Budget

400 iterations, 6 approaches, 40 h wall clock, 120-minute iteration cap. Peer plan review and peer verdict gate
on, vigilant, reflect, router task selection, Docker access for the sibling toolchain.
