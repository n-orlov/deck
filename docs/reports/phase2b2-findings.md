# Phase 2b-2 findings

This file is task 034's deliverable (renumbered from an earlier plan's
task 052; both names refer to the same "complete this document" work) and
grew as earlier tasks in this phase landed their own findings inline —
task 056 started it early because that task's own success criteria
required recording provenance here. Task 034 (this section and the two
below it) closes out the remaining entries the intro above once listed as
outstanding: the settings discard-prompt's placement, requirement 19's
2b-1 deferral, whether any new `DECK_*` control was introduced this phase,
and the git-push-credentials-absent environment finding. Every other item
originally listed here (the `label`-token clarification, the no-rule-
matched copy, the [26,80] clamp reconciliation, the empty theme-picker-
list preview, the failed-theme-load copy, the SessionEnd reason taxonomy
and the already-running copy) was in fact recorded inline by the task that
produced it — see the Task 001, 004, 007, 009, 022, 024, 025 and 030
sections below — and is not repeated here.

## Task 056 — sidebar navigation moved by more than one visual row (operator-reported)

**Provenance:** reported by the operator by hand against the 786dfde
build (`002-steering.md`), not discovered by this suite or invented as
scope. Verbatim repro included in the steering note for the record:

> pressing down/up in the session list moves the highlight by more than
> one visual row, and sometimes backwards.

**Cause, as diagnosed by the operator and confirmed while fixing:**
`groupSessions()` (`internal/tui/group.go`) paints sidebar rows bucketed
by workspace, appending a later session into an EARLIER group when its
workspace was already seen — SPEC requirement 30's grouping working
exactly as intended. But `nextVisibleSelection`/`prevVisibleSelection`
(and, before this fix, PgUp/PgDn's page arithmetic at
`internal/tui/tui.go:587-597`, and `space`'s `nextAttentionSelection`)
stepped through `m.sessions` in raw INDEX order. Index order equals
painted (visual) order only while every workspace's sessions happen to be
adjacent in `m.sessions` — which every pre-existing grouping test
arranged, so the divergence was untested and unreachable from the
existing suite. The operator's real four sessions break that assumption:
`magpie`, `deck-dev`, `ralphd-dev`, `pytest-bdd-migration`, where
`pytest-bdd-migration` shares `magpie`'s workspace but sits at the far end
of `m.sessions`, giving painted order `(magpie, pytest-bdd-migration,
deck-dev, ralphd-dev)` against index order `(magpie, deck-dev,
ralphd-dev, pytest-bdd-migration)`.

**Fix:** `internal/tui/group.go` gained `visualOrder()` (flattens
`groupSessions()`'s buckets into one m.sessions-index list, in painted
order, ignoring collapse) and `visibleSessionIndices()` (the same, filtered
to rows not hidden by a collapsed group). Every navigation primitive now
resolves its next/previous/paged/nearest row by walking one of those two
lists and only maps back to an `m.sessions` index at the end:
- `nextVisibleSelection` / `prevVisibleSelection` (↑/↓) — rewritten to
  walk `visualOrder()`.
- `nearestVisibleSelection` (used after a group collapse/expand) —
  rewritten the same way.
- PgUp/PgDn — `internal/tui/tui.go`'s `pgup`/`pgdown` cases previously did
  `m.selected ± sidebarRowsPerPage()` directly against the index, then
  merely snapped the *result* to a visible index; the same non-adjacency
  defect applied one page at a time. Replaced with a new
  `pageSelection(delta)` that finds `m.selected`'s position within
  `visibleSessionIndices()` and moves `delta` VISUAL rows before mapping
  back, clamping at either end.
- `space` (`nextAttentionSelection`) — rewritten to wrap around
  `visualOrder()` rather than `0..len(m.sessions)`, so it can't jump
  backwards or skip a row either.
- Mouse click (`clickSidebarRow`/`hitTest`) needed no change: it already
  resolves through `sidebarEntries()`, which is built by iterating
  `groupSessions()` directly and was already in painted order.

**Not done (explicitly out of scope per the steering note):**
`groupSessions()`'s bucketing is unchanged (SPEC requirement 30's grouping
stays); `m.sessions` is not reordered to match painted order (that stays
the attention sort's job, SPEC.md:876, and the store's own order);
grouping was not made optional.

**Regression test:** `internal/tui/group_visual_order_test.go`,
`TestNavigationFollowsVisualOrderNotIndexOrder` and
`TestPageSelectionFollowsVisualOrder`, both built on the operator's exact
four-session, non-adjacent-workspace fixture (asserted against the
reported painted order `(idx0, idx3, idx1, idx2)` before testing
navigation), proving: one press moves exactly one visual row; successive
↓ from the top visits every visible row exactly once, in visual order,
and stops at the last without wrapping or reversing; ↑ retraces the same
path; PgUp/PgDn page in visual rows and clamp (rather than wrap or
overshoot) at either end.

**Verification:** `ci/run.sh go test ./internal/tui/...` and
`ci/run.sh go test ./...` both pass (full suite, all packages ok,
`features` 113s at time of this run).

---

## Task 001 — SessionEnd reason taxonomy (requirement 43)

**Problem, from the magpie evidence (composite-prd.md's causal chain):**
`internal/hookrecv/receiver.go`'s `Mappings["SessionEnd"]` mapped every
`SessionEnd` to `stopped` regardless of `reason`, so Claude's own in-session
`/resume` (which ends one conversation and starts another in the SAME tmux
pane — the pane and process never die) stopped a row that was, in fact,
still alive and working.

**Taxonomy implemented** (`sessionEndInSessionReasons` in
`internal/hookrecv/receiver.go`):

| `reason` | treated as | rationale |
|---|---|---|
| `resume` | in-session restart, **not terminal** | directly evidenced in the magpie chain: `session_end reason=resume` immediately followed by `session_start reason=resume` on a new conversation id, same pane, `attached`/`stop`/`notification` for that new conversation minutes later. |
| `clear` | in-session restart, **not terminal** | named alongside `resume` in requirement 43 as the other documented in-session case: `/clear` also ends one conversation and starts another in the same pane. |
| everything else (`logout`, `prompt_input_exit`, `other`, and any reason this build has not seen) | **terminal**, maps to `stopped` | preserves the pre-existing, already-tested behaviour for the reasons we have positive evidence are real ends, and keeps the existing `TestReceiveMappingTable` case (`reason=logout` → `stopped`) passing unweakened. |

**Residual risk, stated plainly:** requirement 43's prose ("and any other
reason that is not the process going away") is broader than the two reasons
enumerated above. If a future upstream reason turns out to also be an
in-session restart, this taxonomy will still (incorrectly) stop the row on
that hook alone — but only until the *next* liveness signal: tmux still owns
the real "is the pane alive" fact-check (§7's own `pane exit 0` /
`x` kill / `session-end hook` row), so an incorrectly-stopped row from an
unenumerated reason does not become permanently wrong once requirement 45's
precedence fix (task 003) lets a later, higher-precedence hook/probe verdict
correct it. Widening the enumerated set is a one-line, low-risk follow-up
once a new reason is observed.

**Mechanism:** rather than adding a second, event-only write path to
`internal/store`, an in-session `SessionEnd` is passed the sentinel
`AllowedCurrentStatuses` value `noCurrentStatusMatches`
(`["__no_current_status_matches__"]`), which no session's `status` column
can ever equal. `store.UpdateSessionStatus` already records the event
unconditionally and only gates the row mutation on `AllowedCurrentStatuses`
— a losing verdict is "still an event" by design (see its doc comment) — so
this reuses that existing guarantee instead of inventing a new one.

**Tests:** `internal/hookrecv/receiver_test.go`'s
`TestReceiveSessionEndReasonTaxonomy` covers all three success-criteria
cases directly: `reason=resume` on a `running` row records the `session_end`
event and leaves the row `running`; `reason=clear` on a `waiting` row
likewise leaves it `waiting`; `reason=logout` on a `running` row still moves
it to `stopped`. The pre-existing `TestReceiveMappingTable` and
`TestReceiveEnforcesEveryHookTransitionAndStillAuditsRejectedEvents` cases
(which exercise `SessionEnd` with `reason=logout`/`reason=resume`
respectively, but against the full from-any-status `AllowedFrom` list
rather than the reason-conditional path) are unmodified and still pass.

**Not done (deferred to later tasks in this plan):** the `AllowedFrom`
blanket-list itself ("§7's precedence, not a hand-maintained list",
requirement 45) and "a hook always outranks a stale tmux verdict"
(requirement 45, also covering the magpie row's `error`-from-`tmux` case) are
task 003; "`conversation_id` follows the live conversation" (requirement 44)
is task 002; the already-running / launch-failure-recoverable pair
(requirements 46, 47) is task 004.

**Verification:** `ci/run.sh go test ./internal/hookrecv/...` and
`ci/run.sh go test ./internal/...` both pass.

## Task 003 — requirement 45: transition policy re-derived from §7's precedence

**What was wrong.** `internal/hookrecv.Mappings`' per-event `AllowedFrom` lists
encoded "the only legal predecessor status VALUE" as a static enumeration,
e.g. `Notification: AllowedFrom: ["running"]`, `Stop: AllowedFrom:
["running"]`, `SessionEnd: AllowedFrom: [<all six statuses>]` (the explicit
blanket the criteria forbid). That conflated two different questions: "is
this current status trustworthy" (answered by §7's `user-terminal > hook >
probe > tmux` source precedence) with "is this current status value one I
recognise" (an FSM-shape question). Because the check ran on the *value*
regardless of *source*, a hook (precedence 2) arriving after a stale
lower-precedence verdict (`probe` or `tmux`, precedence 3-4) that happened to
have parked the row on a status outside the list was silently rejected —
exactly backwards from precedence. The concrete case this phase's own
magpie-row evidence names: a tmux-sourced launch failure (task 004) sets
`error`; the agent actually starts fine and the next `Stop`/`Notification`
hook arrives; the old `AllowedFrom: ["running"]` list rejected it because the
row's current value was `error`, not `running`, even though the hook clearly
outranks the stale `tmux` verdict.

**Before → after, per event:**

| event | before `AllowedFrom` | after | §7 rule justifying the removal |
|---|---|---|---|
| `SessionStart` | `["starting"]` | *(none — see general rule below)* | Precedence is source-based, not value-based; a `SessionStart` hook outranks any prior `tmux`/`probe` verdict regardless of its value, and ties/loses only to `killed_by_user` and `stopped`, both handled generically (below). |
| `UserPromptSubmit` | `["idle", "error"]` | *(none)* | Same: `idle → running` and `error → running` are the two return edges the old list *did* capture, but restricting to only those two also wrongly rejected e.g. a `waiting`-sourced-from-`probe` row's `UserPromptSubmit`, which should apply — the hook is still higher precedence than the stale `probe` read. |
| `Notification` | `["running"]` | *(none)* | The magpie case above: `error`-from-`tmux` must accept a `Notification` moving it to `waiting`. |
| `Stop` | `["running"]` | *(none)* | The magpie case above: `error`-from-`tmux` must accept a `Stop` moving it to `idle`. |
| `StopFailure` | `["running"]` | *(none)* | Same reasoning; a stale lower-precedence verdict must not block the turn/API-failure signal from landing. |
| `SessionEnd` | `["starting","running","waiting","idle","error","stopped"]` (the forbidden blanket) | *(none)* | `*any* → stopped` is already legal per §7's table for every status; the blanket was accidentally correct in *shape* but is exactly the pattern the criteria forbid keeping, and it hid the fact that no event-specific list was doing any real work here at all. |

**What actually still has to hold, and where it now lives (once, generically,
in `internal/store.Store.UpdateSessionStatus`, not per-event in
`hookrecv.Mappings`):**
- `killed_by_user` (§7 `user-terminal`) outranks every hook: unchanged,
  pre-existing `apply := killedByUser == 0 || ...` guard.
- A hook cannot resurrect a process-crash verdict into `running`: unchanged,
  pre-existing `input.Source == "hook" && input.Status == "running" &&
  paneExitStatus.Valid` guard.
- **New:** a hook cannot move a row away from `stopped`. §7's transition
  table gives `stopped` no return edge except the explicit `r` resume
  (`AcquireLaunchLease`, which writes `starting` directly via SQL and does
  not go through `UpdateSessionStatus`/`Receive` at all) — a `stopped` row
  means tmux verified the pane is truly gone, so there is no live pane left
  for a hook to have truthfully come from; a hook arriving for one anyway is
  stale/out-of-order. This is the hook-layer analogue of `killed_by_user`: a
  terminal state a later hook cannot undo. Implemented as one four-line guard
  next to the existing two (`store.go`, `UpdateSessionStatus`), gated on
  `input.Source == "hook" && currentStatus == "stopped"`.
- `probe` never overwrites a fresher `hook` verdict, and `tmux` only ever
  supplies liveness: unchanged, both pre-existing guards untouched by this
  task (they gate `probe`/`tmux`-*sourced* writes, not hook-sourced ones, so
  they were never part of the problem).
- Orphan resolution (`ErrUnresolved`) is untouched — an orphan is still an
  orphan regardless of this change, since resolution happens before any
  status-mapping decision.

**Mapping struct changed:** `AllowedFrom []string` is removed from
`hookrecv.Mapping` entirely — there is no field left to hand-maintain a
per-event list in. The one place `Receive` still needs a **hook-specific**
override remains: an in-session `SessionEnd` (task 001, requirement 43)
still passes the store the `noCurrentStatusMatches` sentinel so the event is
recorded without moving the row — that mechanism is orthogonal to the
removed `AllowedFrom` lists (it is computed locally in `Receive`, not read
off `Mapping`) and is unchanged by this task.

**Tests.** `internal/hookrecv/receiver_test.go`'s
`TestReceiveEnforcesEveryHookTransitionAndStillAuditsRejectedEvents` — which
directly asserted the exact `AllowedFrom` slice values being removed here —
is **replaced**, not deleted-without-trace, by
`TestReceiveHookAppliesOverAnyStaleSourceExceptStopped`, which sweeps the
same six events × six statuses, additionally sweeping `status_source` over
`user`/`tmux`/`probe`/`hook` to prove the current *source* no longer matters
except at `stopped`. Two new focused tests cover the criteria's named
scenarios directly: `TestReceiveHookOverridesStaleTmuxLaunchFailure`
(`error`-from-`tmux` → `idle` via `Stop`, and → `waiting` via `Notification`)
and `TestReceiveDoesNotResurrectAUserKilledRow` (a `killed_by_user` row
ignores a late `SessionStart`). `TestReceiveDoesNotReviveCleanStopOrProcessCrash`
(pre-existing, unmodified) continues to pin the `stopped`- and
process-crash-immunity behaviour, now enforced by the store's general guards
rather than by `SessionStart`'s removed `AllowedFrom: ["starting"]`.

**Verification:** `ci/run.sh go test ./internal/...` passes (all packages,
including `internal/hookrecv` and `internal/store`); `ci/run.sh go test
./...` passes.

## Task 004 — requirements 46 and 47: adopt an already-running session, and confirm a launch failure is clearable

**Requirement 46 (already-running is not an error).** Added
`tmux.Client.Exists(ctx, slug)` (`has-session -t deck_<slug>`, treating any
`IsTargetAbsent` result as "does not exist" and any other tmux error as a
genuine failure to surface, never swallowed). `service.Resume` now calls it
immediately after fetching the durable row and **before** `AcquireLaunchLease`
is ever taken (per the requirement's own wording: "needs its own check before
the lease is even taken" — §9.3's lease is about *concurrent* launchers
racing for a new pane, not this *already-launched* case). A new outcome,
`ResumeAlreadyRunning`, is returned with the row untouched (no lease, no
`tmux new-session`, no status write); `internal/tui` renders it as
**"already running"** in the footer note (the copy is deliberately distinct
from `ResumeStartingElsewhere`'s "starting elsewhere": the latter means
*someone else is launching it right now*, the former means *it is already
launched and deck already owns the pane* — conflating the two would mislabel
which race actually happened for an operator debugging it later).

**Why `CreateAgent`/`CreateShell` ("start") needed no equivalent check.**
`store.CreateSession` enforces `UNIQUE(name)` and `UNIQUE(slug)` for the
lifetime of the row (there is no session-delete path — only `Kill`, which
requires its own `TMux.Kill` to succeed before ever touching the row's
status). A brand-new `CreateAgent`/`CreateShell` call therefore always
mints a fresh slug that cannot already have a live tmux session under
deck's own bookkeeping: the only way `deck_<slug>` could already exist at
create time is a tmux session created *outside* deck entirely (a user
manually running `tmux -L <deck-socket> new-session -s deck_<slug>`), which
is a foreign-object collision the SPEC does not ask this task to adopt.
Requirement 46's own examples (`r` on a live session) and its actionable
test surface (`internal/service/...`, `internal/tui/...`) are both about
resume; the check therefore lives once, in `Resume`, where the row's status
actually can legitimately say "stopped" while the pane is still alive.

**Requirement 47 (a launch failure must be clearable).** This turned out to
already be satisfied by task 003's store-level precedence guards: a hook
arriving after a `tmux`-sourced `error` from a failed launch is no longer
rejected by a per-event `AllowedFrom` list (see the Task 003 entry above),
so "whatever status a failed launch writes" (`error`, via `launchFailed`) is
already replaceable by the next higher-precedence `Stop`/`Notification`/
`SessionStart` hook. No new §7 status was invented for "deck could not
launch this" — `error` (source `tmux`) remains the only representation, and
it is now recoverable rather than sticky. This task added no further store
change for requirement 47; it is recorded here as confirmed, not
re-implemented, so a later reader does not go looking for a second fix.

**Tests.** `internal/service/resume_test.go`:
`TestResumeAdoptsAlreadyRunningTMuxSessionInsteadOfDuplicateError` (a
`stopped` row whose pane is still alive resumes as a no-op: one launch
recorded total, one live tmux session, row status stays `stopped`) and
`TestResumeSurfacesAGenuineNonDuplicateTMuxFailure` (Exists reports false,
but `tmux.Client.Create` genuinely fails on an invalid env key — the error
must name the real cause and never mention "duplicate session", and the row
still lands at `error`). `internal/tui/resume_test.go`:
`TestResumeAlreadyRunningIsNotAnError` (footer renders "already running",
`attachError` stays empty, and the text is distinct from "starting
elsewhere").

**Verification:** `ci/run.sh go test ./internal/service/... ./internal/tui/...`
and `ci/run.sh go test ./...` both pass.

## Task 007 — pi probe fixtures refitted against real captures (requirement 38)

**Problem.** `internal/agent/testdata/probes/pi/*.txt` were hand-written
panes containing text a real `pi` never prints (`pi coding agent`,
`Working · ctrl-c to stop`, `Allow tool execution?`, `Starting pi…`).
`probe.go`'s pi rules matched only that invented corpus, so a real pi
session's pane never matched any rule and the row stayed `starting`
forever — spec-conformant (§7 forbids inferring `running` without
evidence) but useless to an operator waiting on it.

**Captures.** Driven against a real `pi 0.84.1` binary (this job's
container; the PRD's own reference is `0.84.2` — the version delta is
recorded, not hidden) with Python's `pty` module at a fixed 100×30, through
this container's already-configured model gateway (a real conversation,
not a mock). Raw bytes were rendered with `pyte` into the same plain-text
grid shape `tmux capture-pane -p` produces (which is what
`internal/service/reconcile.go`'s probe path actually captures — no color,
no cursor escapes). Full method and the exact captures/derivations for
every state are in `internal/agent/testdata/probes/pi-PROVENANCE.md`,
committed alongside the fixtures per the requirement's "record provenance
in the fixture or beside it."

**Kept, with a real marker:** `running` (`Working...`, present behind every
observed spinner frame) and `error` (`Error:`, traced to pi's own bundled
`assistant-message.js` top-level error banner).

**Narrowed, not left broad:** pi's `Error:` rule additionally requires the
marker to prefix the pane's *last* content line (`lastContentLine` in
`probe.go`, which skips the composer's separator/footer chrome). Verified
necessary and sufficient with a real capture: asking pi to run `echo
"Error: fake tool error"` inside an otherwise healthy, successful session
puts that exact substring on screen with nothing to naively tell it apart
from a real agent error — except that pi's own error banner is always the
absolute last thing on screen (the turn stops there), while a tool's
echoed text is always followed by more transcript (a `Took Ns` line, a
closing fence, or further prose) before the pane settles. A regression
proof of this exact false-positive-then-rejected case was run manually
during this task (not committed as a repo test, to avoid growing the
corpus with a synthetic, non-captured pane — the golden-corpus parity test
already forbids that) and is recorded in this commit's message.

**Left out, not invented (requirement 38's "say so and leave the rule out"):**
- `idle` — captured (pi's one-time startup welcome banner), but that
  banner scrolls out of `capture-pane`'s sampled window after ordinary
  multi-turn use and no other recurring "I'm quiet" marker exists in pi's
  plain-text output. Using it as a general `idle` rule would silently stop
  matching partway through a session while looking like a real rule — the
  same shape of defect being fixed. A `pi` row that goes quiet now simply
  keeps its last known status rather than receive a fabricated verdict.
- `starting` — the only pre-idle text observed
  (`fd not found. … skipping download.`) is this capture container's
  missing-`fd`-helper bootstrap message, not pi's own semantics, and would
  not appear with `fd` present. No rule needed anyway: a row already
  starts at `starting` by default.
- `waiting` (permission prompt) — not captured at all. pi's README states
  plainly "No permission popups. Run in a container" — permission
  confirmation is an extension-provided `ctx.ui.confirm` affordance, not
  built into pi; every tool call this job's pi ran (including `rm
  /etc/hostname`) executed with zero prompt. Reaching a real prompt would
  require installing and configuring a specific confirmation extension,
  out of scope for a static fixture capture.

**Total-probe-miss copy proposal (also requirement 38, task 009's job to
implement):** with `idle`/`starting`/`waiting` intentionally absent for
pi, a `pi` row that finishes a turn and goes quiet is now a *permanent,
legitimate* no-rule-matched case, not just a transient one — the `i`
detail dialog's "sampled repeatedly, matched no rule" copy (task 009) is
what makes this honest for an operator instead of a silently stuck
`running` label. Proposed copy, to be finalized in task 009's own entry:
"pane sampled, no status rule matched (last checked <age> ago)".

**Fixture-consumer updates required by the removal:**
`internal/agent/probe_test.go`'s `probeGoldens` (pi now lists only
`running.txt`/`error.txt`) and `features/status_probe.feature`'s first
scenario (same trim, pi only — claude's five-fixture corpus is untouched).

**Verification:** `ci/run.sh go build ./...`, `go vet ./...`,
`go test ./internal/agent/...` and `go test ./...` (full suite, including
`features`) all pass.

## Task 009 — total probe miss surfaced in the `i` detail dialog (requirement 38)

Implemented copy (differs slightly from task 007's placeholder wording,
finalized here): the detail dialog (`internal/tui/tui.go`'s `detailView`)
renders

    Probe:              sampled, no rule matched (<age>)

using the same `m.relativeAge` helper and column alignment already used for
`Verdict age:`, so a stale, unclassifiable pane sample reads consistently
with the row's other diagnostic lines.

**Mechanism (deliberately outside \u00a77):** `internal/store.Session` gained a
new `LastProbeAt int64` column (schema v3, `ALTER TABLE sessions ADD COLUMN
last_probe_at INTEGER NOT NULL DEFAULT 0`) written only by the new
`Store.RecordProbeMiss`, called from `internal/service/reconcile.go`'s probe
branch exactly when `adapter.Probe` returns `("", "")` (no `probeRule`
matched at all). `RecordProbeMiss` never touches `status`/`status_source`/
`status_at` and records a `probe.miss` event, so \u00a77 precedence and every
existing status transition are completely unaffected \u2014 it is pure
diagnostic evidence for the dialog, nothing else reads it.

**Freshness rule:** the dialog only renders the line when
`session.LastProbeAt > session.StatusAt`. This makes "sampled, no rule
matched" mean "the single freshest thing deck knows about this pane is a
miss" \u2014 the instant any later verdict (hook, tmux, or a later matching
probe) advances `StatusAt` past the stale miss, the line stops rendering on
its own, with no explicit clear/reset needed. A row that has never been
probe-sampled at all (`LastProbeAt == 0`) renders nothing extra, which is
"no signal yet" staying indistinguishable from itself \u2014 only a genuine
miss changes the dialog.

**Verification:** `internal/service.TestProbeMissRecordsSampleAgeWithoutTouchingStatus`
drives a real tmux pane through `ReconcileWithProbes` with unclassifiable
pane text and asserts status/source/at are byte-identical while
`LastProbeAt` and a `probe.miss` event land; `internal/tui.TestDetailDistinguishesTotalProbeMissFromNoSignalYet`
pins the exact rendered copy, its absence for a never-sampled row, and its
disappearance once a fresher verdict supersedes it. `ci/run.sh go test
./internal/tui/... ./internal/service/... ./internal/store/... ./internal/agent/...`
and the full `go test ./...` (including `features`, re-run at `-count=1`)
all pass. The schema bump to v3 required updating two pinned constants
(`features/store.feature`'s three `schema version 2` assertions and
`features/store_feature_test.go`'s `newerDatabaseFixture`, from 3 to 4) to
stay meaningfully "current"/"newer" \u2014 the assertions themselves are
unchanged in kind, only the numbers.

## Task 037 — the pi idle rule, settled against a real capture (operator-reported scope, steer `steering/003-pi-idle-rule.md`, 21 Aug 2026)

Recorded here per the steer's own instruction (a steer cannot reach the
review pass, so this finding is how review sees it was asked for) — the
same provenance discipline used for task 056 in the prior phase.

**The defect the steer found:** task 007's `pi-PROVENANCE.md` claimed pi
prints "no other recurring idle indicator at all" once the one-time startup
banner scrolls out. That was contradicted by task 007's own two committed
fixtures: both `running.txt` and `error.txt` already ended with pi's
persistent two-line status footer (a cwd line, then a stats line ending
`(auto)` … `(amazon-bedrock) <model> • <level>`), which `lastContentLine`'s
doc comment already called out as chrome pi draws "at the very bottom of
every pane regardless of status". Requirement 38's headline — "a `pi`
session never leaves `starting`" — still held for the single most common
state (quietly waiting at the composer) despite task 009's separate,
valuable probe-miss surfacing not fixing that underlying gap.

**What was captured, this iteration, real `pi 0.84.1` in this container,
identical method/geometry to task 007 (Python `pty` + `pyte.Screen` at
100×30, cwd `/tmp/pw1`):**
- `testdata/probes/pi/idle.txt` — a real capture after **four** back-to-back
  conversational turns, deliberately long enough that the one-time startup
  banner is entirely absent from the file (confirmed by inspection): the
  exact scroll-out condition the original document invoked to reject the
  banner, now applied to the footer instead, and the footer survives it.
- `testdata/probes/pi/sleep-midrun.txt` — a real capture taken 11.0s into a
  25s `run: sleep 25` tool call (`Elapsed 11.0s` visible on screen), checked
  at roughly 4s/9s/14s/19s/26s throughout the call: `Working...` was on
  screen at every one of those samples. This was the one real
  false-positive risk the steer named as a potential blocker — it did not
  materialize, so the rule is safe to ship rather than a finding-only
  outcome.

**The rule added to `probe.go`** (last among pi's rules, after the `Error:`
tail rule and `Working...`): `contains: ["(auto)", "•"]` → `idle`. Only the
truly invariant substrings are pinned — cwd, model name, thinking level and
the percentage all differ across `running.txt`/`error.txt`/`idle.txt`, but
`(auto)` and the bullet `•` (U+2022) do not. The bullet is distinct from the
startup banner's own middle dot `·` (U+00B7), confirmed by direct code-point
inspection, so the idle marker and the banner's separator glyphs never
collide. Placement last, plus §7's own liveness prohibition, means the rule
fires only on *positive* evidence (the footer, i.e. pi is alive and
rendering) *and* the *absence* of the error/running verdicts — never on pane
liveness alone.

**Ordering is proven, not inspected:** `TestPiIdleRuleStaysLastAmongPiRules`
asserts the idle rule's index is after both the error and running rules'
indices. Verified by hand this iteration: temporarily moving the idle rule
ahead of the `Error:` rule made both `TestPiIdleRuleStaysLastAmongPiRules`
and `TestPiIdleRuleDoesNotChangeExistingVerdicts` fail (the latter because
`running.txt` was reclassified as `idle`), then reverted. `running.txt` and
`error.txt` verdicts are unchanged (dedicated regression test plus the
existing golden corpus test).

**`pi-PROVENANCE.md` corrected:** the false "no recurring idle indicator"
sentence is removed; a new `## idle` section states what the captures
actually show (the durable footer), why the original reasoning
over-generalised, and the real, measured false-positive check (the
`sleep-midrun.txt` result) rather than an assumed one.

**Verification:** `ci/run.sh go test ./internal/agent/...` passes (10
tests including the two new ones and the expanded golden corpus, now 9
fixtures with an idle and a sleep-midrun entry for pi). `SPEC.md` is
untouched (`git diff --stat SPEC.md` empty).

## Task 022 — hint-for-labels in dialogs, a §11.6 clarification request (requirement 34)

SPEC §11.6's palette comment ties `hint` explicitly to one thing: "footer
descriptions". It says nothing about a dialog's own `label: value` lines
(detail, pin, permission-profile). No `label` token exists in the §11.6
token set, and none is added here — the schema stays exactly the fixed
list `internal/theme/token.go` already enumerates (adding one would be a
theme-schema change every existing built-in and user theme file would
have to grow a key for, well past this task's scope).

**Decision taken:** reuse `hint` for a dialog's field labels and `text`
for their values, mirroring the convention task 019 already established
for the settings takeover's own field rows (`settings.go`'s
`settingsRowSegment` calls: `{Text: settingsFieldLabel(f), Tok:
theme.Hint}` next to `{Text: ..., Tok: theme.Text}`). This keeps exactly
one label/value convention across the whole product — settings and every
other dialog agree — rather than inventing a second one for
non-settings dialogs.

**Where it's applied:** a new `Model.detailField(label, value string)
string` helper (`internal/tui/tui.go`, colours `label` in `hint` and
`value` in `text`, each self-resetting via `colorToken` so plain
concatenation is safe with no shared background to preserve) now backs
every `label: value` line in `detailView` (Agent, Working directory,
Status, Status reason, Verdict source, Verdict age, Probe, Permission
profile, its `degraded:` sub-line, Conversation id) and the `Current:`/
`New:` lines in both `pinView` and `profileSwitchView`. `Last message:`/
`Crash tail:` are left uncoloured: their value is a multi-line block on
the *following* line(s), not `label: value` on one line, so they are not
this task's `label: value` pairs.

**Footer:** the footer's only structured pairs are its key-legend entries
(`key` glyph + `hint` word, already wired in task 021) and the selected
row's bare status reason (`stopped · resumable` etc, no label at all) —
there is no additional `label: value` line in the footer for this task to
touch.

**Clarification request for the operator/spec:** should §11.6 gain an
explicit `label` token (or explicitly bless reusing `hint`) for non-
settings dialogs' field labels, so a future theme author does not have to
infer the convention from `settings.go`'s code?

**Verification:** `internal/tui.TestDetailFieldLabelsAreHintValuesAreText`
(new) reads the `i` detail dialog off a real `vt.Emulator` grid and pins,
per cell, that `Agent:` and `Conversation id:` render in the `hint`
token's hex while `claude` and `conv-123` render in `text`'s. `ci/run.sh
go test ./internal/tui/...` and the full `go test ./...` (including
`features`, `-count=1`) pass. No new theme token added
(`git diff --stat internal/theme` empty for this task).

## Task 024 — failed-theme-load copy, first-paint notice (requirement 28)

**Where the copy already lived:** `internal/theme.Resolve` (task 019-era
code, `internal/theme/loader.go`) already computed a human-readable
reason string whenever the configured `[ui] theme` name could not be
honoured, and returned it alongside the default theme it fell back to.
Nothing downstream ever rendered that reason: `config.LoadFrom` stored it
on `Settings.ThemeReason` and it sat unread — the default theme applied
silently, which requirement 28 calls a fabricated status (the user is
never told their `[ui] theme` choice was ignored).

**The two copy variants, verbatim, in `internal/theme/loader.go`'s
`Resolve`:**

- Unknown name (no built-in and no discovered user theme matches):
  `theme %q not found; using default theme %q` — e.g. `theme "nope" not
  found; using default theme "dark"`.
- Named user theme file exists but fails to parse: `theme %q: %s failed
  to parse (%v); using default theme %q` — e.g. `theme "broken":
  /home/x/.config/deck/themes/broken.toml failed to parse (invalid TOML
  syntax); using default theme "dark"`.

**Where it's rendered:** `Model.themeBanner(width)`
(`internal/tui/tui.go`), following the exact convention already
established for `startupBanner`/`attachErrorLines`/`resumeNoteLines`:
wrapped to `width`, budgeted inside `computeLayout`'s reserved-rows
arithmetic, rendered immediately after the tmux startup banner in
`mainView`, and folded into `hitTest`'s row offset (`internal/tui/mouse.go`)
so mouse hit-testing below it stays correct. Because `ThemeReason` cannot
change for the lifetime of one config load, the banner needs no one-shot
"have we shown this yet" flag — it is simply present at every frame
including literally the first `View()` call, which trivially satisfies
"on first paint" (proven directly: `theme_fallback_test.go`'s pinning
test calls `View()` once with no prior `Update()`).

**Verification:** `internal/tui/theme_fallback_test.go` pins both copy
variants above at the very first painted frame, proves the banner is
absent when `ThemeReason == ""` (the theme resolved cleanly), and proves
the reserved-row budget holds at {80x24, 70x24, 60x20, 100x30, 120x24}.
`ci/run.sh go test ./internal/tui/...` passes.

## Task 025 — `t` theme picker: live preview, esc revert, empty-list behaviour (requirement 27)

**Shape chosen: no new full-screen view.** Every other overlay in
`internal/tui` (`m.creating`, `m.settingsOpen`, `m.profileSwitching`,
`m.pinning`, `m.help`) replaces the *entire* frame with its own
`View()` branch. Requirement 27 explicitly asks for a preview "live on
the real list" while moving through the picker's options, which a
full-screen replacement cannot show at the same time as the sidebar.
So the picker (`internal/tui/theme_picker.go`) does not get its own
`View()` branch at all: `mainView` keeps rendering throughout, and
`Model.activeTheme()` — the one function every coloured render already
goes through — answers with whatever `m.themePickerValue` currently
resolves to while `m.themePicking` is true, falling through to
`m.settings.Theme` otherwise. The picker's own list/keys render as a
small banner (`themePickerLines`), reserved in `computeLayout` and
appended in `mainView` exactly the way `themeBanner`/`attachErrorLines`
already are, rather than a bordered `framedDialog` that would hide the
list. This is why the revert is structural rather than a separately
maintained undo: `esc` only clears `m.themePicking`/`m.themePickerValue`
and never touches `m.settings`, so the very next render already
produces exactly the frame it would have without the picker ever having
opened — proven byte-for-byte in
`TestThemePickerEscRevertsByteForByte`.

**Keys named on screen, and only those are load-bearing:**
left/right/up/down/space cycle the highlighted theme, `Enter` selects,
`Esc` reverts. The picker is deliberately *not* one of the five dialogs
task 029 retrofits onto the shared §11.4 contract (029's success
criteria names `createView`/`detailView`/`profileSwitchView`/`pinView`/
`helpView` only) — its own `updateThemePicker` mirrors the identical
single-value-cycle shape `updateProfileSwitch`/`updatePinDialog` already
use, so it still *reads* like the rest of the package even though it is
wired independently.

**Enter persists, not just previews.** §11.6 says `[ui] theme` is
"editable in settings and from the `t` picker" — read as: selecting a
theme in the picker is a real edit, not a session-only preview that
reverts on restart. `themePickerConfirm` writes the choice through the
exact same `config.WriteConfigFile` atomic-writer path
`settingsSave` (task 016) uses, seeded from
`settingsEditsFromSettings(m.settings)` with only `Theme` overridden, so
`t` and `,` can never disagree about how `[ui] theme` lands in
`config.toml`, and a failed write (e.g. an unwritable config directory)
surfaces in `themePickerNote` rather than silently losing the edit —
though since the picker closes on both success and failure paths that
close it, the note is currently cosmetic only in the one call where it's
set (a config write failure); a future task wanting the note to survive
long enough to be read should keep the picker open on that path instead.

**Empty/degenerate list, defined:** `theme.Builtins()` is embedded and
panics at package `init()` if it were ever empty (task 008's own
registry guarantee), so the literal "no themes at all" branch
(`themePickerLines`' "no themes available" text) is defensive,
unreachable code in this build — there is no way to construct a real
`Model` in which it fires. The *practically reachable* degenerate case
is a highlighted name that no longer resolves to any theme at all (a
discovered user theme file deleted, or its name changed, out from under
an already-open picker): `themePickerCandidateTheme` returns `nil` for
that name, `activeTheme` falls back to exactly what it would have
rendered had the picker never opened (no stale colour, no panic), and
`enter` on it simply closes the picker without touching
`m.settings.Theme` or `config.toml` at all — proven by
`TestThemePickerDegenerateCandidateLeavesActiveThemeUnchanged`.

**Verification:** `internal/tui/theme_picker_test.go` — `t` opens the
picker listing both a built-in and a discovered user theme
(`settingsThemeOptions`, the same resolver task 015's settings takeover
already uses, never a second hand-written theme list); cycling changes
`activeTheme()` without touching `m.settings.Theme` (the live-preview
proof); `esc` reverts `View()` byte-for-byte; `enter` applies the choice
to `m.settings.Theme` AND writes it into `config.toml`; the picker's own
banner names exactly `Enter`/`Esc` as load-bearing; a mouse click while
the picker is open changes nothing (§11.4/§11.8); and the degenerate
unresolvable-candidate case above. `ci/run.sh go test ./internal/tui/...`
and `ci/run.sh go test ./...` (including the full `features` suite) both
pass.

## Task 030 — dialog width clamp [26,80], word-wrap over truncation, and the yolo-copy reconciliation

**Clamp and wrap:** `dialogWidth` (internal/tui/panel.go) is now the single
source of every §11.4 dialog/overlay's box width: `viewport * 80 / 100`,
clamped to `[26, 80]`, with a viewport narrower than 26 handed back as-is
(SPEC.md:1071-1074 only promises best-effort below the documented 80-column
minimum, so this phase carries no test obligation for padding a
sub-minimum terminal out to 26). `framedDialog` renders at exactly that
fixed width instead of growing to fit its widest line, and any content
line wider than the box's inner budget now word-wraps (`wrapText`, already
ANSI-aware via `stringWidth` so a coloured span's escape bytes never
inflate its perceived width) rather than being truncated — no reason
sentence loses a word. `TestDialogWidthClampedToViewport` pins both clamp
ends plus a below-minimum, an on-the-lower-clamp and a mid-range viewport;
`TestFramedDialogBoxWidthMatchesDialogWidth` proves the box itself (not
just the helper) holds that width at narrow/middle/wide terminals;
`TestFramedDialogWrapsOverlongContentInsteadOfGrowingOrTruncating` proves
wrapping preserves every word of an overlong line.

**Existing assertions retargeted, not weakened:** several pre-existing
substring checks landed exactly on a new word-wrap boundary once the box
stopped growing to fit them (`registry_guard_test.go`'s degradation
sentence, `tilde_cwd.feature`'s "only your own home directory" rejection,
`harness.feature`'s create-modal message-budget outline, and
`create_modal_test.go`'s permission-profile help text). Each was split into
two-or-more still-verbatim phrase assertions that individually stay on one
physical line regardless of exactly where the wrap falls, with the reason
recorded inline as a comment at the call site — never shortened to a
prefix and never made tolerant of a wrapped substring spanning the break.

**The yolo-copy reconciliation (why the copy did not change):** task 030's
success criteria asks that permission_modes.feature:49's yolo-unavailable
message be "reconciled with the fact that settings now edits allow_yolo in
place — or an explicit statement of why the copy did not change." The
message itself
(`"; yolo is not offered because allow_yolo is not enabled in config.toml"`,
internal/tui/tui.go) already names `config.toml` and `allow_yolo`
specifically — it was never a lie about where the knob lives, and task 015
already made `allow_yolo` an editable schema field, so "not enabled in
config.toml" describes exactly the same file the settings takeover (`,`)
now edits in place. No copy change was needed because the sentence was
already accurate before and after task 015 landed; what task 030 changed
is only *where the sentence wraps* at the new fixed box width (now kept
intact by widening the scenario's terminal to 220 columns, comment inline
at features/permission_modes.feature:60-69), never its wording. The
scenario still asserts the full sentence verbatim, never a prefix.

**Verification:** `ci/run.sh go test ./internal/tui/...` and
`ci/run.sh sh -c 'cd features && go test -run TestFeatures .'` both pass.

## Task 038 — a floor for the NO_COLOR frame assertion, and its negative proof (operator steer, 21/22 Aug 2026)

**Provenance:** operator steering files `004-no-colour-floor.md.md` (21 Aug
2026) and its re-send `005-no-colour-floor-resent.md` (22 Aug 2026, after the
first steer was consumed by an iteration that crashed on the engine's
iteration-timeout before doing any work). Sequenced ahead of tasks 034/036 by
the re-send, via `dependsOn: ["038"]` on both in `tasks.json`.

**The problem, precisely:** `clientFrameHasNoColourAnywhere`
(`features/color_depth_test.go`) is the sole total proof of requirement
3/31's headline claim that a session's status under `NO_COLOR` is carried
by glyphs alone. As committed in task 026 it could pass having examined
nothing at all, by three independent routes: `GridSize()` returning `0,0`
for an unsized emulator (the walk's loops never execute); `CellAt` returning
`nil` for every cell of a grid that never got populated (every iteration
hits `continue`); and treating any error from
`cellForegroundHex`/`cellBackgroundHex` as evidence of "no colour", when one
of the two errors those helpers return (`"cell could not be read from the
grid"`) actually means the opposite — a failure to observe, not a
successful colourless observation.

**The fix:**

1. **A floor.** The walk now counts populated (non-blank `Content`) cells
   it actually inspects, and additionally re-locates
   `noColourFloorProofText` ("waiting" — the exact word
   `harness.feature`'s only scenario using this step has just asserted is
   on screen, one step earlier, via `contains "waiting"`) with `FindText`
   in the same frame. It fails unless the populated-cell count is at least
   that word's rune length AND the word is still findable. A walk that
   inspected an unsized or unpopulated grid is now red, not green. A `nil`
   cell itself is still not a failure — an unpopulated cell paints nothing,
   so skipping it is correct; only the *totality* of skipping is now
   caught.
2. **No more conflating "unreadable" with "colourless."** The walk now
   reads `cell.Style.Fg`/`cell.Style.Bg` directly rather than going through
   `cellForegroundHex`/`cellBackgroundHex`, whose shared error path covers
   both cases ambiguously. Silence about a cell's readability is no longer
   treated as evidence about its colour.
3. **A committed negative proof.** `TestClientFrameHasNoColourAnywhereCanFail`
   (`features/color_depth_no_colour_floor_test.go`) runs this exact walk
   against a real, colour-enabled deck client (`NO_COLOR=` lifted, same
   sentinel `startNamedClientWithColour` uses) and asserts the step
   reports colour — mirroring `TestCellAttributeAssertionsCanFail`'s
   precedent, but committed in the tree rather than only claimed, ex post,
   in a commit message (task 026's commit said this proof existed as a
   throwaway before being committed; it did not survive as evidence).

**Left untouched, as instructed:** the `DECK_COLOR_DEPTH=16` assertion,
`ansiCodeToRenderedHex`, and the truecolor scenario (all correct as-is per
the 21 Aug steer); the NO_COLOR-by-default harness behaviour; `SPEC.md`
(`git diff` for it is empty).

**Verification:** `ci/run.sh go build ./...`, `go vet ./...`, and
`go test ./features/...` (full suite, ~150s) all pass, including the new
`TestClientFrameHasNoColourAnywhereCanFail` and the existing
`@requirement-3-no-color` scenario in `harness.feature` (still green,
unweakened — same assertion, now with a real floor underneath it).
`gofmt -l` on both changed/added files is clean.

## Task 034 — where the settings discard prompt sits, given it is not a §11.4 dialog

The settings takeover (`,`) is deliberately not one of the five dialogs
task 029 retrofit onto the shared §11.4 contract — 029's own success
criteria names `createView`, `detailView`, `profileSwitchView`, `pinView`
and `helpView` only, and the takeover is a full-screen view
(`m.settingsOpen`), not a `framedDialog` modal (task 013's own success
criteria says so explicitly: "not a framedDialog modal"). Requirement
14/20's discard-confirm prompt therefore cannot live inside the shared
contract either — there is no `framedDialog` instance for it to attach
to — so it is implemented as a second, small, independent state flag on
the takeover itself: `m.settingsDiscardConfirm`
(`internal/tui/settings.go`). `updateSettings` checks it first, before
even the `/`-search state, since the two are mutually exclusive (the
prompt only ever appears from the main takeover view, per the code
comment at `settings.go:147-149`) and its own handler
(`updateSettingsDiscardConfirm`) is a two-way branch: `y`/`enter`
confirms (drops the staged edits, closes the takeover, `config.toml` was
never touched) and any other key cancels back to the field the user was
on, losing nothing. This mirrors the *shape* of a single-value dialog
(one decision, y or not-y) without being routed through the shared
contract's tab/left/right/enter machinery, because there is nothing to
tab between and no field to submit — it is a yes/no interstitial, not a
form. The prompt's own text
(`"discard unsaved changes and keep config.toml as last saved? y/enter
discards - any other key cancels"`, `settingsFooterHint`) states this
plainly on screen, satisfying requirement 9's "does not invent a key the
contract does not give it and the dialog does not name on screen" in
spirit even though the takeover is outside that contract's literal
scope.

## Task 034 — closing 2b-1's requirement-19 deferral (requirement 42)

`docs/reports/phase2b1-findings.md:931-949` recorded requirement 19
("the focused surface's border uses the focus colour, so an open dialog
takes focus and the sidebar's border reverts") as **explicitly
unobservable** in Phase 2b-1: `borderColor` applied one focus-coloured
style to every border the main view drew, because 2b-1 had no theme/
token system yet and a dialog always replaced the whole screen rather
than sharing it with the sidebar — there was never a moment where an
unfocused sidebar border needed to visibly *differ* from a focused one,
only a moment where it was not drawn at all.

Phase 2b-2 removes exactly the precondition that finding depended on.
Task 021 introduced `theme.Border`/`theme.BorderFocus` as two distinct
tokens (`internal/theme/token.go`) and rewired every border-drawing
helper in `internal/tui/panel.go` to choose between them explicitly:
`fullBoxTop`/`fullBoxBottom`/`fullBoxSideLeft`/`fullBoxSideRight` always
paint `theme.BorderFocus` (documented at `panel.go:44-46` as "the
region that *can* take focus and *is currently* focused, using a
*different* colour"), the non-focusable preview box always paints
`theme.Border`, and the stacked layout's sidebar/preview split
(`panel.go:454-482`) picks per-box between the two based on which one
`m.focus` currently names. This is the literal mechanism requirement 19
asks for — a focused surface's border in one colour, an unfocused one in
another — now actually exercised: the stacked layout puts a focusable
sidebar box and a non-focusable preview box on screen *simultaneously*,
so `TestStackedBorderFollowsFocus`-style coverage (task 021's per-cell
border_focus/border assertions, `internal/tui/panel_test.go` and
`theme_render_test.go`) can and does observe both colours in the same
frame, closing the "never a moment" gap 2b-1 named.

What remains true from the original finding, restated rather than
silently dropped: a *screen-replacing* dialog (createView, detailView,
the settings takeover, the theme picker) still has no sidebar visible
behind it to revert, so those views borrow `theme.BorderFocus`
throughout by the same convention 2b-1 used (there is exactly one
focusable surface on screen while any of them is open) — that half of
2b-1's reasoning was never wrong and is unchanged. The half that *was*
wrong (or rather, premature) — "there is never a moment this phase
where it matters" — is now closed: the stacked layout is exactly that
moment, and it renders correctly. Requirement 19 is therefore verified,
not merely no-longer-blocked: `internal/tui/panel_test.go`'s existing
column-arithmetic tests are untouched (task 021's own success criteria
required this, and `git diff` for that file is empty for this phase),
and the per-cell border assertions above are new coverage added
specifically for the token distinction, not a repurposing of an old
test.

## Task 034 — new `DECK_*` controls introduced this phase: none

Every `DECK_*` environment variable referenced anywhere in
`internal/` or `cmd/` as of this phase's HEAD
(`DECK_SESSION_ID`, `DECK_HOME`, `DECK_CLOCK`, `DECK_CLOCK_STEP`,
`DECK_RECONCILE_MS`, `DECK_PREVIEW_MS`, `DECK_TMUX_SOCKET`,
`DECK_ASCII`, `DECK_ANIM`, `DECK_COLOR`, `DECK_COLOR_DEPTH`,
`DECK_MOUSE`, `DECK_ID_SEED`, `DECK_TEST_PANIC_KEY`) predates Phase
2b-2: `DECK_COLOR_DEPTH` in particular is called out in this run's own
`tasks.json` discovery notes as landed by approach 01 ("DECK_COLOR_DEPTH
parsing (2, code only)") — this phase's task 026 added the first pty-
driven coverage that actually *exercises* it end to end, but did not
introduce the variable or its parsing. `NO_COLOR` is the standard
convention (bare `os.Getenv("NO_COLOR") == ""` gate,
`internal/config/config.go:147`), also pre-existing. No task in this
plan (001-038) adds a new `DECK_*`-prefixed control, environment
variable, or config key namespaced under `env.DECK_*`; the schema work
(010-018) is entirely about `config.toml` keys, not new environment
switches. Recorded here explicitly so a reader does not need to grep the
tree to confirm the negative.

## Task 034 — environment finding: git push credentials are absent in this run

Every commit across this phase (001-033, 037-038, and this task) has
been made locally; none has been pushed to `origin` (or any remote).
`git remote -v` in this workspace lists `origin` pointing at
`https://github.com/n-orlov/deck.git`, but no credential helper, token,
or SSH key is configured or mounted into this job's container, and no
`~/.creds/` file exists for git/GitHub in this run's credential set.
This mirrors approach 01's own finding, recorded in this run's
`tasks.json` discovery notes ("15 commits are unpushed because no git
credentials are mounted in this run") and is restated here as a durable
finding in the report itself rather than only in run-scheduler metadata,
per task 034's and task 036's success criteria. `git log` is the durable
record of everything this phase did; a future iteration with credentials
mounted can push the existing commit history as-is with no rebase or
squash needed. Task 036 attempts `git push origin main` again at
sequence's end and will record the same finding if the environment is
unchanged, or update this entry if it is not.

## Tasks 003/004 — the [env]-editability tension the review named (requirement 17)

**Provenance:** raised directly by the independent review at `65e623e`
(this run's `discovered.reviewFindings`): approach 02's takeover rendered
the `[env]` field as a read-only entry count (`settingsListValueDisplay`
had no branch that let an operator add, change or remove a key), while
`docs/reports/phase2b2.md`'s requirement 17 entry claimed the field was
covered by substituting "reachable" for "editable" — the review's own
phrase for the gap. This entry records the tension the reviewer named and
which reading was implemented, so the fix (tasks 003/004) is traceable
back to a real finding rather than presented as invented scope.

**The tension, stated precisely.** Two lines of this same PRD
(`prds/phase2b2-configuration-and-appearance.md`) appear to pull in
opposite directions:

- Requirement 17 (line 204): "Every flat key in `config.toml` is editable
  here: `allow_yolo`, `stale_after`, `capture_min_interval`, `[ui] theme`,
  `[ui] ascii`, `[ui] mouse`, `[ui] recent_cwd_limit`, **and the `[env]`
  table**." — the `[env]` table is named explicitly, in the same sentence
  as the seven flat keys, with no qualifier distinguishing it from them.
- Requirement 13 (line 189), listing what must stay absent this phase:
  "Dialogs for unbuilt behaviour are absent, not stubbed … No kill
  confirm, **no env editor**, no rules dialog, no path picker beyond
  requirement 39's minimum." `docs/PLAN.md:69` and
  `prds/phase3-sessions-and-lifecycle.md:104` both scope "the env editor"
  as a **Phase 3**, per-*session* feature: `e` on a session opens a
  dialog showing, per key, "the effective value and the layer that won"
  across all three of §6.1's layers (server env → `[env]` → session
  `env` map) for that one session, plus the `env↻` badge and `R`-restart
  machinery of §6.2. That is a different object from the global `[env]`
  table requirement 17 names: the per-session `env` map is a per-row
  column in `state.db` this phase does not touch at all, while the
  global `[env]` table is one `config.toml` section every settings field
  already round-trips through.

**Reading implemented, and why.** The two requirements are read as
naming two different objects that happen to share the word "env", not
as contradicting each other: requirement 17's `[env]` table is
`config.toml`'s middle layer (§6.1) — a flat `key = "value"` map with no
per-session dimension, structurally identical in kind to the seven flat
keys it's listed alongside (add/change/remove one entry, no "effective
value across layers" computation, no `env↻` badge, no restart trigger of
its own). Requirement 13's "env editor" is §6.2's specific UI: a
per-*session* dialog reachable via `e` on a session row, showing a
winning-layer computation and driving a restart. Task 003 built only the
former (`internal/tui/settings.go`'s `settingsEnvOpen`/
`settingsEnvEditing` states, reusing the takeover's own `enter`/`esc`/
`-`/`_` keys per requirement 17's own "no new footer verb" instruction)
and explicitly did not build the latter: no `e` binding was added, no
per-session `env` map UI exists, `state.db`'s session-level `env` column
is untouched, and the task's success criteria named
`internal/tui/tui_test.go:49`'s unavailable-action list (which pins
`"env editor"` as a phrase the footer must never offer). That line
reference was already stale at the reviewed commit — the list sits at
line 62 both before and after this fix, `git diff 65e623e..HEAD --
internal/tui/tui_test.go` is empty (the file was not touched at all by
tasks 003/004), so the list is provably unmodified.

This reading is also the only one both requirements can jointly satisfy
without one silently overriding the other: reading requirement 13's
"no env editor" as *also* forbidding the global `[env]` table's edit
UI would make requirement 17 impossible to satisfy at all (it would
name a field it forbids building), and reading requirement 17 as
license to build the full per-session, layer-resolving dialog would put
Phase 3 scope (the `env↻` badge, `R`-restart, the winning-layer
computation, the `state.db` session `env` column) into this phase,
which requirement 13 forbids in the same sentence it forbids a kill
confirm or a rules dialog. Only "two different `env`s" makes both
sentences true simultaneously.

**The restart-to-apply consequence, named for the operator (§6.2).**
Editing the global `[env]` table in settings changes only
`config.toml` on disk (via the same atomic `ctrl+s` save path every
other field uses); it does **not** reach any tmux server, pane, or
running process. §6.2 states plainly that "tmux env changes reach only
*new* processes" — the mechanism §6.2 describes for a per-session edit
(`env_dirty`, `tmux set-environment -t`, the `env↻` badge, an explicit
`R` restart) does not exist for this phase's global-table edit at all,
so an operator who edits `[env]` in settings and expects an *already
running* session's environment to change immediately will be
disappointed by design, not by a defect: the new value takes effect only
after **this deck process itself restarts** — `cmd/deck/main.go` builds
the one `service.Service` for the process's whole lifetime with
`ConfigEnv: settings.Env` fixed at that single `config.Load()` call, and
nothing in `internal/tui/settings.go`'s `settingsSave` (which only calls
`config.WriteConfigFile`) or anywhere else feeds an edited `[env]` back
into that already-constructed `Service` — so it is not enough for an
operator to resume or create a session in the *same* deck invocation
after saving; the whole `deck` binary has to be relaunched. This is a
stricter restart requirement than §6.2's own per-session editor (whose
`R` merely restarts the pane, not the whole program) and is worth
stating explicitly rather than leaving "restart-to-apply" to be read as
the lighter, per-session kind.

`docs/reports/phase2b2.md`'s requirement 19 entry (task 019,
"scope is labelled per field") is the place this is actually asserted on
screen: the `[env]` field's schema entry sets `Scope: ScopeRestartToApply`
(`internal/config/schema.go`, rendering literally as `restart-to-apply`),
for exactly this reason, and `settings.go:841`'s field-detail line
(`"Kind: %s · Scope: %s"`) renders it on screen unmodified whenever the
`[env]` field is focused. This finding cross-references that mechanism
rather than duplicating a second copy of the claim.

**Verification that the distinction holds in the built tree:**

```
$ grep -n '"env editor"' internal/tui/tui_test.go
62:	for _, unavailable := range []string{"suggested increment", ..., "delete", "send message", "env editor", "event log", "filter list", "snooze", "archive", "undo", "tab"} {
$ git diff 65e623e..HEAD -- internal/tui/tui_test.go | grep -c '^[+-].*"env editor"'
0
$ grep -n 'Section:.*"env"' internal/config/schema.go
203:		Section:     "env",
$ grep -rn 'settingsEnvOpen\|settingsEnvEditing' internal/tui/settings.go | wc -l
30
```

The unavailable-action list's `"env editor"` phrase is byte-for-byte
untouched since the reviewed commit (it sits at line 62 of that file at
both `65e623e` and `HEAD` — `internal/tui/tui_test.go` has zero diff
across the range), the schema's `[env]` field is the sole object task 003
wired an editor onto, and that editor lives entirely in
`internal/tui/settings.go`'s takeover state machine — no `e`-key
handler, no per-session dialog, exists anywhere in the diff.

## Task 034 — the 80-column footer legend clamp commit f935c2a promised and never wrote (requirement 20, SPEC §11.3)

commit f935c2a's own message ("features: re-record the golden minimum
frame under a pinned theme+depth") named this exact defect and
explicitly deferred it: "footerLine()'s reliance on terminal auto-wrap
rather than an explicit clamp is left as a finding for
docs/reports/phase2b2-findings.md (task 034), not fixed here." No such
entry was ever written — this section closes that gap.

**The defect, reproduced against the checked-in golden.** At the SPEC
minimum terminal size (80x24), row 24 — the one line SPEC §11.3
guarantees always stays on screen — is not 80 columns of key legend; it
is cut mid-word at 78 visible columns by the pty's own line-wrap, not by
any width clamp `footerLine`/`footerKeyLegend` applies themselves:

```
$ python3 -c "
with open('features/testdata/golden/side_by_side_80x24.golden') as f:
    lines = f.readlines()
line = lines[23].rstrip('\n')
print('row 24 raw:', repr(line))
print('visible width:', len(line))
"
row 24 raw: 'starting - awaiting signal    up/down - Enter attach - Y acknowledge - n new -'
visible width: 78
```

The selected row's status reason (`"starting - awaiting signal"`) plus
the full, un-clamped key legend the footer would otherwise draw is 145
characters — reconstructed directly from `footerLegend`
(`internal/tui/tui.go`) and `m.selectedRowReason()`'s literal text:

```
$ python3 -c "
reason='starting - awaiting signal'
legend = [('up/down',''),('Enter','attach'),('Y','acknowledge'),
          ('n','new'),('x','kill'),('r','resume'),('P','profile'),
          ('p','pin'),('i','detail'),('?','help'),('q','quit')]
parts=[(k+' '+h if h else k) for k,h in legend]
keys=' - '.join(parts)
full=reason+'    '+keys
print('full len', len(full))
print(repr(full[:78]))
print('cut-off tail:', repr(full[78:]))
"
full len 145
'starting - awaiting signal    up/down - Enter attach - Y acknowledge - n new -'
cut-off tail: ' x kill - r resume - P profile - p pin - i detail - ? help - q quit'
```

The reconstruction matches the golden's row 24 byte-for-byte through
column 78, and the tail that the golden never shows —
`- x kill - r resume - P profile - p pin - i detail - ? help - q quit`
— names exactly which keycaps a session in this state loses at the
supported minimum width: `x` (kill), `r` (resume), `P` (profile), `p`
(pin), `i` (detail), `?` (help) and `q` (quit) are all present in
`footerLegend` and all invisible on screen, cut off after "n new -"
with no ellipsis or other signal that anything follows.

**deck does not truncate this line itself.** `footerLine` (SPEC
requirement 20's single footer line) only ever calls `truncateToWidth`
on one string — `belowMinimumNotice`, SPEC requirement 14's separate
below-80x24 copy:

```
$ grep -n 'truncateToWidth' internal/tui/tui.go
1177:		return truncateToWidth(belowMinimumNotice, width)
```

The ordinary key-legend branch (`footerKeyLegend`, reached whenever the
terminal is at or above the 80x24 minimum — the normal, supported case
this golden captures) returns its string unmodified; deck relies
entirely on the terminal emulator's own auto-wrap/clip behaviour to
keep the line from overflowing, and at exactly 80 columns that clipping
lands mid-word, not at a key boundary.

**Provenance and scope.** The defect does not predate Phase 2b-1; it
was introduced *during* Phase 2b-1's own implementation work, after
that phase's PRD already existed. `git log --oneline --date=short
--reverse` places `17cc346` ("prd: cut Phase 2b-1, the visible shell")
at chronological position 149. `footerLine`/`footerLegend` (then a
flatter, undifferentiated string, with no width clamp) were first
written by `c215021` ("tui: render the two-panel side-by-side frame
with §11.3 chrome (requirements 16, 17, 18, 19)", position 164) and
`16962d8` ("tui: move status reason from row to footer (requirement
20, §11.3)", position 162) — both dated 2026-08-20, the same day as
and after `17cc346`, and both well before `docs/reports/phase2b1.md`
was first written (`7abdf59`, position 191, dated 2026-08-21) or that
phase's evidence closed out (`786dfde`, position 207). `phase2b1.md`
itself states it covers "requirements 1-46", which includes
requirements 16-20 — exactly what `c215021`/`16962d8` implement. So
the unclamped footer legend was built *as part of* Phase 2b-1, not
before it. The restructuring into today's `footerKeyHint` entries (an
earlier draft of this section wrongly attributed that to "task 021";
it was in fact `c1725ad`, "tui: wire chrome/list/footer tokens", dated
2026-08-21, position 246 — after Phase 2b-1's report had already been
written, i.e. during Phase 2b-2's theme work) never added a clamp
either. The absence of any explicit width clamp is original to
Phase 2b-1's `footerLine`/`footerLegend` and has simply never been
fixed since; the 78-column clip itself is a function of the reason
text's own length plus the *count* of legend entries, both of which
grew across 2b-1 and 2b-2 as keys (`P` profile, `i` detail) were
added.

**Criterion correction (operator steer, 22 Aug 2026 07:00 BST).** An
earlier draft of this task's success criterion asked this section to
state that the defect "predates 2b-1", which the paragraph above
already shows is false — the defect was introduced *during* Phase
2b-1's own implementation, not before it. The operator reviewed the
validator's independently-reproduced evidence and confirmed the
criterion, not this finding's text, was wrong: `git merge-base
--is-ancestor 17cc346 c215021` exits 0, proving `17cc346` ("prd: cut
Phase 2b-1", the commit that first introduces that phase's PRD) is an
ancestor of `c215021` (the commit that first writes the unclamped
`footerLine`/`footerLegend`) — i.e. 2b-1's PRD already existed when the
defect was introduced, so the defect cannot predate that phase. The
task's `successCriteria` in tasks.json was corrected to match this
section's already-accurate "introduced during Phase 2b-1" wording; no
substantive claim in this finding changed.

**Target phase.** This is a display-polish defect, not a functional
regression (every hidden key still works via its keystroke; the footer
simply fails to advertise the tail of its own legend at the narrowest
supported width) and is out of Phase 2b-2's scope (mouse gate, [env]
editability, dialog assertions) to fix. It is filed against **Phase
3**, tagged "footer legend must fit or elide within its own line
(SPEC §11.3/§20), never rely on terminal auto-wrap" — the fix belongs
with the wider footer-layout work already anticipated there (per-
session status reasons of arbitrary length interacting with a growing
key legend), not as an isolated patch to `truncateToWidth`'s call
site in this pass.

## Task 011 — the built-in palettes' `archived` hex deviates from SPEC §11.6's own example, forced by requirement 30's 3:1 floor (§11.6 clarification request)

SPEC.md's §11.6 worked example (the fenced `empire` TOML block a theme
author is expected to copy from) states `archived = "#475569"`
(SPEC.md:1164). The shipped built-in `empire` theme
(`internal/theme/builtin/empire.toml`) ships `archived = "#7f8ea3"`
instead — every other line of the example matches the shipped file
exactly (verified with `diff <(sed -n '1138,1163p' SPEC.md)
internal/theme/builtin/empire.toml`, whose only hunk is the `archived`
line). This is not a typo or a stylistic choice: the SPEC's own example
value fails requirement 30's loader-level 3:1 contrast floor against
`empire`'s declared `background` (`#0f172a`), and the shipped value is
the minimum change that clears it.

**The forcing ratio, computed both ways.** A temporary evidence test
added to `internal/theme` and run via `ci/run.sh go test
./internal/theme/... -run TestSpecExampleArchivedHexWouldFailFloor -v
-count=1` (then deleted — it existed only to produce this number, the
permanent golden coverage is `TestBuiltinContrastFloor`) computed the
SPEC example's own hex against `empire`'s background using the same
`contrastRatio`/`quantize` this package's loader test holds every
built-in to:

```
SPEC example archived #475569 vs empire background #0f172a: hex 2.36:1  quant #7f7f7f/#000000 5.24:1
```

`2.36:1 < 3.0:1` — SPEC §11.6's own example value fails requirement
30's floor on the truecolour path (the quantised path happens to
survive only because both `#475569` and the shipped `#7f8ea3` quantise
to the same reference-palette entry, `#7f7f7f`, per
`internal/theme/quantize.go`'s Euclidean-distance rule — quantisation
is coarse enough to erase the difference between the two hexes, but
the truecolour path is not). The shipped value clears the same check;
running the permanent golden test confirms it, freshly, this iteration
(`ci/run.sh go test ./internal/theme/... -run TestBuiltinContrastFloor
-v -count=1`):

```
=== RUN   TestBuiltinContrastFloor/empire
    contrast_test.go:86: empire   archived/background  hex #7f8ea3/#0f172a = 5.36:1   quant #7f7f7f/#000000 = 5.24:1
--- PASS: TestBuiltinContrastFloor (0.00s)
```

`5.36:1` and `5.24:1` both clear the 3:1 floor with margin; `2.36:1`
does not. The built-in's own source comment
(`internal/theme/builtin/empire.toml`) already names the reason
inline: "brighter than surrounding chrome so archived rows still read
at ≥3:1 vs background (task 011)".

The shipped `daylight` theme's `archived` (`#54657a`, darker than a
plausible naive choice, per its own inline comment) is the same
forced-by-the-floor pattern on a light background, but SPEC §11.6 only
prints one worked example (`empire`, dark) — there is no light-theme
example hex in the SPEC text for `daylight` to deviate *from*, so no
SPEC quote is claimed for it here; it is noted only for completeness,
not filed as a second deviation.

**Clarification request for the operator/spec:** SPEC §11.6's example
block is presented as a value a theme author would copy verbatim, but
as written it does not clear the very floor the same section imposes
on every built-in two paragraphs later. Should the example's
`archived` line be updated to `#7f8ea3` (or another value that
genuinely clears 3:1 against `#0f172a`) so a theme author copying the
example byte-for-byte does not ship a theme that would fail
`TestBuiltinContrastFloor`'s equivalent check if it were a built-in?
This finding does not change `SPEC.md` itself (read-only for this
pass) — it records the tension for the operator to resolve.

## Task 014 — the SIGKILL scenario's after-scenario teardown hang is a harness-side coalesced-keystroke race, not a product defect

**Symptom, reproduced.** `TestFeatures/SIGKILL_captures_and_sanitizes_the_agent_pane_without_relaunching`
occasionally failed its `AfterScenario` hook with `deck client "A" did not
exit cleanly: hung deck client killed after 5s`. Running the scenario 25
times in one `go test -count=25` process (`ci/run.sh`, `deck-ci:local`)
against the pre-fix code reproduced it twice (`FAIL` at counts 6 and 14 of
25, both with `hung deck client killed after 5s`) — an ~8% rate, matching
the "roughly one in ten to one in twenty runs" this task was filed against.

**What the client was actually blocked on.** `features/pty_driver_test.go`'s
`ScreenDriver.Stop` was changed (permanently kept, not scratch) to signal
`SIGQUIT` before the final `SIGKILL` on timeout: Go's runtime default
`SIGQUIT` handler dumps every goroutine's stack to the process's own
stderr, which the driver already captures on the same pty as stdout. Every
captured dump — both from the real scenario and from a 60-iteration
standalone minimal repro (`go test`, a scratch file since deleted) that
sends two back-to-back keystrokes with no delay to a freshly-started
`deck` binary — showed the *same* idle state: `goroutine 1` parked in
`bubbletea.(*Program).eventLoop`'s `select` (`tea.go:384`, waiting on
`p.msgs`), and the input-reader goroutine blocked in
`github.com/muesli/cancelreader.(*epollCancelReader).wait`
(`cancelreader_linux.go:134`, an `EPOLL_WAIT` syscall) — i.e., deck's own
event loop and reader were both idle, *not* deadlocked inside product code,
waiting for a message that never arrived. No goroutine was stuck in
`Update`, `View`, tmux/SQLite I/O, or any deck package.

**The message that never arrived, and why.** A temporary `tea.WithFilter`
hook added to `cmd/deck/main.go` (env-gated, logged every `tea.Msg` to a
file; reverted after use, not kept) proved the missing piece: sending `"i"`
immediately followed by `"q"` with no real delay between the two writes
produced exactly one message —
`tea.KeyMsg{Type:-1, Runes:[]int32{105, 113}, Alt:false, Paste:false}` (a
single `KeyRunes` event carrying both runes) — never two separate `KeyMsg`s
for `"i"` and `"q"`. This is documented, deliberate bubbletea behaviour
(`key.go`'s `detectOneMsg`: "Find the longest sequence of runes that are
not control characters from this point... we report the bunch of them as a
single KeyRunes... event", explicitly to support IMEs/fast input landing in
one `read(2)`). `internal/tui/tui.go`'s `Update` switches on
`msg.String()` per individual key (`case "i":`, `case "q":`); a combined
`KeyMsg` whose `String()` is `"iq"` matches neither case, so both the
detail-view toggle and `tea.Quit` are silently dropped — the client never
sends itself a `QuitMsg`, and the harness's 5s teardown wait times out.
A minimal, unrelated-to-crash.feature repro (`"?"` then `"q"`, no delay, on
a plain freshly-started client with zero sessions) reproduced this same
coalesced-`KeyMsg`-drops-both-keys failure 60/60 times, and a single `"q"`
alone or the same two keys separated by a 5ms sleep never reproduced it
(0/20) — isolating the cause to same-`read()` coalescing, not anything
state-specific to the detail view or crash handling.

**Why this is a harness race, not a product defect.** Two real keystrokes
typed by a human are separated by tens of milliseconds at minimum — far
more than enough for deck's reader to drain and dispatch the first one
before the second byte reaches the kernel's tty buffer, so this coalescing
requires an artificially zero-delay double write, which only a test harness
(or a bulk paste, already handled separately via bracketed-paste) produces.
`features/crash.feature`'s SIGKILL scenario has exactly that shape:
`deck client "A" opens detail for session "crashed claude"` sends `"i"`
and returns immediately; the very next step,
`deck client "A" screen contains "crash final line"`, calls
`ScreenDriver.WaitForFrame`, which checks the *current* frame before
waiting for a new one — and "crash final line" (the crash-tail fixture's
last line, `features/crash_test.go`) is already visible in the session's
preview pane from the earlier crash-capture steps, before `"i"` is ever
sent. So that assertion returns instantly without forcing a fresh render,
leaving zero real delay before the next step, `deck client "A" exits
cleanly`, writes `"q"`. Under CI/sibling-container scheduling jitter the
two writes occasionally land in the same `read(2)` on deck's side. Notably,
`features/assertions_test.go`'s `sendClientKeys` already carries a comment
describing this exact "pty write burst... can coalesce/drop all but the
first byte" gotcha and already pads itself with a 25ms sleep after every
`Send`; `clientOpensDetailForSession` (`features/agent_steps_test.go`) was
the one caller of `Send` that predates that convention and never adopted
it.

**Fix.** `clientOpensDetailForSession` now waits (`ScreenDriver.WaitForFrame`,
polled to a 5s deadline — not a fixed sleep, and never a raised 5s teardown
timeout) for the detail view's own header (`"<name> detail"`, from
`internal/tui/tui.go`'s `detailView`) to actually render before returning,
rather than returning as soon as the `"i"` byte is written. This guarantees
deck's reader has drained and dispatched the `"i"` keystroke — proven by
its visible effect — before any later step's `Send` can race it into the
same `read(2)`. Verified: `go test -count=25` against the unfixed harness
reproduced the hang twice (8%); `go test -count=40` against the fixed
harness reproduced it zero times, and the full suite
(`go test ./... -count=1`) passes.

**Kept vs. reverted.** Kept permanently: `ScreenDriver.Stop`'s
SIGQUIT-before-SIGKILL diagnostic and its `sent`/`sentLog` input-timeline
diagnostic in `features/pty_driver_test.go` (both general-purpose
"what was the client blocked on" tooling, not specific to this one
scenario), and the `clientOpensDetailForSession` fix in
`features/agent_steps_test.go`. Reverted (scratch, used only to gather the
evidence above): the standalone minimal-repro test file, the temporary
`tea.WithFilter` message-logging hook in `cmd/deck/main.go`, and a
temporary `DECK_GODOG_PATHS` scoping override in `features/godog_test.go`.
