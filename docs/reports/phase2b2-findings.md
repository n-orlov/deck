# Phase 2b-2 findings

This file is task 052's deliverable and grows as later tasks in this phase
land findings. It is started early by task 056 below because that task's
own success criteria requires recording provenance here; task 052 will add
the remaining entries (missing `label` token, no-rule-matched copy, the
[26,80] clamp reconciliation, settings discard prompt placement, empty
theme-picker-list preview, failed-theme-load copy, SessionEnd reason
taxonomy, already-running copy, new DECK_* controls, and the explicit
closure of 2b-1's requirement-19 deferral) without needing to create the
file from scratch.

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
