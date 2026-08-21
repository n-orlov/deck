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
