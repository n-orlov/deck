# Phase 3 findings

## Transcript path/naming convention, per adapter (requirement 4/32/33 provenance)

Neither adapter's transcript path is invented; each was established against a
real, on-disk artifact and the exact convention is recorded below. Today
neither `internal/agent.Adapter` nor `Caps` exposes a `TranscriptPaths`
lookup — no code path in this tree currently searches for or depends on
finding one. When a real deck adapter needs one (SPEC requirement 32/33,
purge/reap targeting an agent's own transcript file), it must degrade the
same way `cmd/fake-claude`'s `transcriptPath`/`cmd/fake-pi`'s
`findExistingTranscript` already do for their fixtures: a missing `HOME`,
missing project directory, or no matching file is treated as "no transcript
found" and returned as an empty/absent result, never as an error that blocks
the caller's own operation (kill/delete/reap must still succeed even when no
transcript exists to also remove).

### claude

**Convention**: `$HOME/.claude/projects/<cwd, each path separator replaced
with "-">/<conversation id>.jsonl`. No wrapping characters (contrast pi's
`--...--`), no leading-slash stripping distinct from the general separator
replacement — the leading `/` is itself a separator and becomes a leading
`-`.

**How established**: real, authenticated Claude Code 2.1.237's own
`SessionStart` and `UserPromptSubmit` hook payloads carry this path in their
`transcript_path` field (task 2's opt-in `@real-agents` conformance run,
`DECK_GODOG_TAGS=@real-agents go test -run TestFeatures -v ./features/...`).
Repository-relative capture:
[`docs/reports/phase2-real-claude-authenticated.log`](phase2-real-claude-authenticated.log),
lines 50 and 54, e.g.:

```
transcript_path":"/auth/.claude/projects/-tmp-deck-scenario-3258814457-agent-session-cwd/20d34654-9462-4781-8b14-680862724dc7.jsonl","cwd":"/tmp/deck-scenario-3258814457/agent-session-cwd"
```

`cwd` = `/tmp/deck-scenario-3258814457/agent-session-cwd`, encoded segment =
`-tmp-deck-scenario-3258814457-agent-session-cwd` (leading `/` and every
internal `/` replaced with `-`), filename =
`20d34654-9462-4781-8b14-680862724dc7.jsonl` (the same string as
`session_id`, no timestamp component — unlike pi). This is a genuine upstream
capture, not a fixture's own claim about itself: it is the real product
telling deck, via its own hook payload, where it just wrote the file.

**Consequence for `cmd/fake-claude`**: `transcriptPath` in
`cmd/fake-claude/main.go:172-186` reproduces exactly this
(`strings.ReplaceAll(cwd, string(filepath.Separator), "-")`, no wrapping, id
as filename, no timestamp), and degrades to "no transcript" (empty path, nil
error) when `$HOME` is unset rather than erroring — see the doc comment at
`cmd/fake-claude/main.go:117-121`.

### pi

**Convention**: directory
`$HOME/.pi/agent/sessions/--<cwd with a single leading "/" or "\" stripped,
then every remaining "/", "\", ":" replaced with "-">--` (note the literal
`--` wrapping, absent from claude's convention); file
`<ISO-8601 UTC timestamp with ":" and "." replaced by "-">_<session id>.jsonl`
(note the timestamp component, absent from claude's convention — a caller
resuming by id alone must glob for `*_<id>.jsonl` rather than recompute the
name; pi also has no separate `--resume` flag, reusing `--session-id` for
both create and resume).

**How established**: full detail, including the installed npm package's
documented layout, the exact compiled-source encoding function
(`getDefaultSessionDirPath`, `dist/core/session-manager.js:242-246`) and an
end-to-end capture against the real, installed `/usr/bin/pi` 0.84.1 binary
with an isolated `$HOME`/cwd, is already recorded in
[`docs/reports/phase3-fake-pi-transcript-provenance.md`](phase3-fake-pi-transcript-provenance.md)
(written for task 004; not duplicated here). That capture's directory
(`--tmp-pi-provenance-work--` for cwd `/tmp/pi-provenance/work`) and filename
(`2026-08-22T15-06-00-661Z_43ac9425-9b54-4c5d-8063-ac52768d0cdb.jsonl`) match
the stated convention exactly.

**Consequence for `cmd/fake-pi`**: `encodeCwd`/`createTranscript`/
`findExistingTranscript` in `cmd/fake-pi/main.go` reproduce this convention
exactly, and `findExistingTranscript` globs for `*_<id>.jsonl` rather than
recomputing the filename, matching pi's own lack of a separate resume flag.

## Requirement 3 under-specified requirement 29's modes/symlink assertions (found by operator steer, 22 Aug 2026 17:45 BST)

Tasks 002/003 implemented `fingerprintDirectory`/`compareFingerprints` exactly to requirement 3's
letter — the four mutation modes requirement 3 names (content change, mtime touch, added file,
removed file) all had a proving subtest, and `compareFingerprints` already compared `Mode` and
fingerprinted a symlink by its own `Readlink` target rather than following it. But requirement 29
goes further than requirement 3 states, asserting the fingerprint proves "same entry list, same
contents, **same modes**, same mtimes" across every destructive path, and neither the `Mode`
comparison branch nor the symlink-not-followed decision had a test that could fail: an operator
reproduction outside this tree, at commit `3896573`, deleted the `Mode != Mode` branch of
`compareFingerprints` entirely and `go test -run TestFingerprint ./features/` stayed green, and
`grep -rn "Symlink\|Readlink" features/*_test.go` found only the instrument's own implementation,
no test exercising it. This was not a compliance miss by the worker — requirement 3 simply never
named "mode change" or "symlink target swap" as mutation modes to prove, even though requirement 29
then relies on both being caught.

**Fix**: `TestFingerprintDetectsMutations` (`features/fingerprint_mutation_test.go`) gained two more
isolated subtests, matching the existing four's discipline of changing exactly one property and
restoring every other:
- `mode change`: chmod a seeded file (0644 → 0444) while restoring its mtime via `os.Chtimes`
  afterwards, leaving content and size untouched. Verified red with the `Mode != Mode` branch
  removed (`compareFingerprints did not detect a mode change`), green restored.
- `symlink target swap`: repoint a symlink at a different file holding byte-identical content,
  then restore the symlink's own `lstat` mtime via `golang.org/x/sys/unix.UtimesNanoAt(...,
  AT_SYMLINK_NOFOLLOW)` (recreating a symlink otherwise bumps its own mtime, which would let the
  existing mtime-touch comparison catch the mutation for the wrong reason and leave the
  not-following decision unpinned). Verified red when `fingerprintDirectory` is changed to
  `filepath.EvalSymlinks` + `os.ReadFile` the resolved target instead of `os.Readlink`
  (`compareFingerprints did not detect a symlink target swap`), green restored.

Also added: `fingerprintDirectory` now records the fingerprinted root directory's own entry under a
reserved key (`"."`), fingerprinting its `Mode` — previously the walk returned early on
`path == root`, so a `chmod` of the cwd root itself (not merely something inside it) was invisible.
The root's own mtime is deliberately **not** fingerprinted: adding or removing any child
legitimately bumps a directory's mtime on every platform this runs on (verified experimentally —
recording it caused the existing `added file`/`removed file`/`symlink target swap` subtests to
report `"." mtime changed` instead of naming the actual differing child, since `compareFingerprints`
returns the first difference in sorted order and `"."` sorts first), which would mask the specific
path every destructive-path scenario in tasks 036/037 needs named. A `root directory mode change`
subtest proves the root's mode is still caught (verified red with the root recording removed).

`features/harness.feature`'s `requirement-29-fingerprint-harness` scenario now seeds a fourth entry
using `seedScratchDirectory`'s existing `mode` column — `readonly.txt` at mode `0444` (readable, not
`0000`, so the fingerprint can still read its bytes) — completing requirement 29's four named entry
kinds (a deck-artifact-shaped name, a dotfile, a directory, and now a file with no write
permission).

## PRD requirement 3/27 vs 29 cross-reference (contradiction, recorded not fixed)

`prds/phase3-sessions-and-lifecycle.md` requirement 3 states: "This is the instrument requirement
**27** is measured with." Requirement 27, however, is `A` archives — an unrelated, non-destructive
flag requirement. The requirement that actually measures the fingerprint instrument is **29** ("The
working directory is sacred... a scenario asserts with requirement 3's fingerprint..."), which this
repository's own code comments and task notes (tasks 002/003/036/037) already cite correctly. This
is a PRD cross-reference slip, not a defect in the tree; per operator instruction it is recorded
here for the operator to correct in the PRD, and is deliberately left unedited in
`prds/phase3-sessions-and-lifecycle.md`.

## DECK_UNDO_MS/DECK_DELETE_GRACE_MS as new determinism knobs (task 102)

Task 001 already parsed both `DECK_UNDO_MS` (default 10000) and `DECK_DELETE_GRACE_MS` (default
60000) into `config.Settings.Undo`/`DeleteGrace`, with `internal/config/config_test.go` proving both
the override and default-when-unset paths. Task 102 is the first consumer: the `x` key still kills
without confirmation and still refuses an already-stopped row, but a successful kill now starts a
`tea.Tick(settings.Undo, ...)` window, keyed by a monotonically-increasing generation counter so a
late-firing tick from a superseded kill can never clear a newer one's toast. While the window is
open, a toast naming "undo" (`Killed "<name>" — press u to undo`) occupies a reserved layout line,
and `u` resumes the named session through the same `resume` plumbing `r` already used, then clears
the window immediately (rather than waiting for the tick) so a second `u` press is a no-op. Once the
tick fires — or once `u` has already consumed the window — `u` does nothing and the toast is gone.
`DECK_UNDO_MS` is a real, user-visible determinism knob: `features/kill_delete_undo.feature`'s third
scenario sets it to 200 ms specifically so the test can wait past expiry on the real wall clock
without a multi-second sleep, and the help overlay's new `DECK_UNDO_MS` line documents the same knob
for operators.

`DECK_DELETE_GRACE_MS` is parsed and defaulted identically but has no consumer yet — no code path
reads `settings.DeleteGrace`, and no help-overlay line or feature scenario names it, matching the
known gap tasks 105/106 (and task 001's blocked-validation handoff) describe: the strip that removed
its help line was deliberate, since a knob with no feature behind it is not yet a real, testable
determinism control. Recorded here as the second half of "both new determinism knobs" so a future
worker on 105/106 has a paper trail for why the two knobs were parsed together in task 001 but only
one is wired to behavior after task 102.

## DECK_DELETE_GRACE_MS gains its consumer (task 106)

The gap the previous section flagged — `DECK_DELETE_GRACE_MS` parsed and defaulted but read by no
code path — is closed by task 106. `dd`'s confirm-dialog submit already killed the pane and
tombstoned the row (task 105/133); task 106 adds the other half of the same shape task 102 gave `x`:
a successful delete now starts its own `tea.Tick(settings.DeleteGrace, ...)` window (a second,
independent generation-counter trio, `deleteUndoSessionID`/`deleteUndoGeneration`, since dd's
internal kill step never emits the `sessionKilled` message the `x`-path trio keys off), a toast
occupies its own reserved layout line for that long, `u` falls back to restoring the tombstoned row
when the kill-undo trio is empty, and once the window is gone `u` does nothing and
`reapSvc` (`Service.Reap`) permanently removes the row. Both windows are scheduled via `tea.Tick`
against the real Go runtime clock, never `settings.Clock`, so `features/kill_delete_undo.feature`'s
`@requirement-2-monotonic-windows` scenario can freeze `DECK_CLOCK` for the whole run and still watch
both the toast disappear and the row get reaped. The help overlay's new `DECK_DELETE_GRACE_MS` line
documents the knob for operators, closing task 001's blocked-validation handoff for this half.

One deliberate asymmetry from `x`'s toast: `deleteUndoNoteLines` never names the deleted session,
unlike `undoNoteLines`'s `Killed "<name>" — press u to undo`. The pre-existing "submitting the
confirm dialog..." scenario asserts the deleted session's name is gone from the *whole* screen the
instant the row disappears (dd hides the row outright, unlike `x`'s row which stays visible with a
"stopped" status) — a toast quoting that same name back would violate that assertion the moment it
renders, so the generic "Deleted — press u to undo" text was chosen instead, and none of the new
scenarios depend on the name appearing in that toast.

## Reap leaves no trace: outbox/notify state has no separate cascade, and the §9.4 history file path is a new convention (task 107)

SPEC §9.2's "reaping deletes the `sessions` row and every deck-owned row hanging off it —
events, notification outbox entries, the `waiting` and `notify_epoch` state — together with
deck's own per-session files" describes four things to clean up, but this schema (still schemaV1
through schemaV4, unchanged by this task) only ever had two of them as actual rows: `sessions`
and `events`. There is no `outbox` table anywhere in the tree, and `waiting`/`notify_epoch` are
columns on `sessions` itself, not rows in a side table — so `ReapSession`'s existing single
`DELETE FROM sessions ...` (which cascades `events` via `ON DELETE CASCADE`, schemaV1) already
removes all four; there was no separate cascade to add. If a later phase introduces a real
outbox table, `internal/store.ReapSession` is the one place that DELETE would need a sibling
statement — recorded here so that phase doesn't have to rediscover this by reading the whole
reap path again.

The other half of requirement 24 — "deck's own per-session files ... §9.4's history file and
captured scrollback" — needed an actual decision this task made explicit: SPEC §9.4 says a
history file and captures live "under the deck data dir" / "under `$DECK_HOME/captures/<session_id>/`"
but never names the history file's own path, because nothing in this tree writes either file yet
(both are Phase 6 deliverables). `config.CapturesDir(home, sessionID)` and
`config.HistoryFile(home, sessionID)` (`internal/config/config.go`) are the single place both
paths are now defined — `$DECK_HOME/captures/<id>/` (matching SPEC's own wording exactly) and
`$DECK_HOME/history/<id>` (SPEC left this one to be decided; a flat file per session id, one
level under the data dir, mirroring captures' own shape) — so that when Phase 6 lands its actual
writer, it has one existing convention to target rather than inventing its own that a stale reap
path would then miss. `internal/service.Service.Reap` removes both if present and tolerates
absence either way (the common case today), which is also the shape SPEC §9.4 already requires
of a *missing* capture file elsewhere ("degrades to no replay, never to an error").

## SPEC.md

Unmodified by this task — `git diff SPEC.md` is empty (verified below).

## Pre-existing dead code confirms requirement 27's own premise (task 111)

`internal/tui.previewPlaceholderLines` (`internal/tui/tui.go`) has had a `case "archived":`
branch since before this task, rendering "Session is archived. No live preview to show." — but
`session.Status` can never actually equal the literal string `"archived"`: requirement 27 (and
this task's own store/service work) makes archiving a flag (`archived_at`), never a status
transition, and nothing anywhere in the tree ever calls `SetStatus`/writes `status="archived"`.
That branch was unreachable before this task and stays unreachable after it — the archived rows
this task adds keep whatever status they had going in (`stopped`, since archiving requires it or
kill-and-archives to it), so they fall into the `case "stopped":` branch instead, same as any
other stopped row. Left as-is rather than rewired to check `ArchivedAt` directly: requirement 27
and task 111's `successCriteria` only asked for the sidebar glyph/list-visibility behavior (both
covered by `internal/tui/archive_test.go` and the two new feature scenarios), not the preview
pane's placeholder copy, and rewiring dead code the task wasn't asked to touch risks a scope
change with no test pinning the new preview text. Recorded here so a later task that does want an
"archived" preview message finds this branch already exists, just gated on the wrong condition.

## Bulk mark/kill/delete design: purge is never offered for a batch (task 112)

Requirement 28's `m` toggles a mark on the *selected row* keyed by session id
(`Model.marked map[string]bool`), not by visual index — `markedSessions()` is
the one place that set is turned back into a session list, and it always
walks `visualOrder()` (never `m.sessions` directly), so a re-sort (attention
rank changing under a marked session) or a re-group (`[ui] group_by_workspace`
flipping, or the sort input changing) can never separate a mark from the
session it was set on, nor silently reorder which sessions a subsequent bulk
action targets. `x` and `dd` both resolve the batch through this same
function; when the mark set is empty, both fall back to their pre-existing
single-selected-row behavior unchanged (`len(m.marked) > 0` is the only
branch either key's handler adds).

One deliberate asymmetry from the single-session `dd` confirm dialog: the
bulk confirm dialog (`bulkDeleteConfirmBody`) never offers a purge choice.
Purge (requirement 26) resolves one session's own declared transcript path
via that session's adapter, at dialog-open time, into one `deletePurgeValue`/
`deletePurgePath` pair — a shape that has no batch equivalent without
inventing either a matching one-purge-decision-for-N-different-adapters UI
(no other dialog in this tree does that) or a second, unrequested set of
per-row purge toggles. Requirement 28's own success criteria asks only that
`dd` "act on the whole mark set" and that undo restore it — not that purge
compose with batching — so the bulk path states plainly ("Purge is not
offered for a bulk delete") and always keeps each marked session's own
transcript, exactly like a single `dd` left at its own default `keep`.

Undo is a single new pair of batch-scoped fields per action
(`batchUndoSessionIDs`/`batchUndoGeneration` for `x`,
`batchDeleteUndoSessionIDs`/`batchDeleteUndoGeneration` for `dd`) rather than
reusing the existing single-session undo trio N times — one `u` press must
restore (or, for delete, un-tombstone) every session the batch action
touched, in one action, which the existing single-session undo fields have
no way to express (they only ever name one session id). `u`'s handler checks
priority order — single-kill undo, then batch-kill undo, then single-delete
undo, then batch-delete undo — so a single-session undo window left open
from an earlier action takes priority over a *later* batch action's own undo
window; this is by construction (the single fields are always set by the
action that opened them and cleared the instant `u` consumes or the window
expires) but means a test (or an operator) exercising both kinds of undo in
quick succession must let the single window expire first, or `u` resumes the
wrong thing. `features/kill_delete_undo.feature`'s
`@requirement-28-mark-bulk-actions` batch-kill scenario does exactly this: it
kills one session alone first (with a short undo window), waits for that
window's own toast to disappear, and only then builds and kills the batch —
otherwise `u` would wrongly resume the earlier single-killed session instead
of the batch.

Bulk kill (`x` with a non-empty mark set) silently skips any marked session
already `stopped` — killing an already-stopped session is a no-op for the
single-session path too (`m.kill` is simply never called), so the batch path
inherits the same behavior rather than treating "some of the batch was
already stopped" as an error or a partial-failure state. The toast text is
always plural (`"Killed %d sessions — press u to undo"`, even when a batch of
one marked session gets killed) — there is no singular/plural branch, which
keeps the toast's wording independent of how many sessions happened to still
be running when `x` was pressed (that count is resolved from the mark set,
not reported back into the message).

Marks clear on the action that consumes them (synchronously, even though the
kill/delete itself completes asynchronously) and on a plain top-level `Esc`,
regardless of whether any other dialog is open — `Esc`'s existing
`applyDialogContract` cancel path is untouched; the mark-clearing is a
separate, unconditional statement in the top-level `case "esc":` branch, so
`Esc` clearing marks holds even when nothing else was open to cancel.

## Task 113: fingerprinting bulk actions needs same-basename scratch cwds

Requirement 29's remaining destructive paths (purge, archive, kill-and-archive,
bulk `x`, bulk `dd`, batch undo) needed their own adversarially-seeded scratch
cwd per session, same as task 108. Two gotchas specific to giving each half of
a *marked pair* its own scratch directory:

- Sidebar grouping (`sessionWorkspace`, `internal/tui/group.go`) keys on
  `store.DefaultWorkspace`, the basename of the session's cwd, whenever the
  row has no explicit `workspace` value (nothing sets one explicitly from the
  create modal). Two scratch directories under different labels
  (`fp-bulk-kill-one`, `fp-bulk-kill-two`) therefore land in two DIFFERENT
  workspace groups, inserting a group header between them. The existing
  `k`,`m`,`j`,`m` two-row marking idiom (task 112's own gotcha) does not need
  to change for this — headers are cosmetic and not part of `m.sessions`
  index/selection — but the fixture must still put both scratch directories
  under the SAME basename, or the two rows visually separate under two
  headers and later readers may reasonably (if incorrectly) suspect that
  changes navigation. `seedScratchDirectory`'s `label` param accepts a
  slash-separated path (it is passed straight to `filepath.Join`), so
  labelling both `fp-bk-1/cwd` and `fp-bk-2/cwd` gives two distinct
  directories that both end in `cwd` — same workspace group, same as any
  other same-workspace two-row marking scenario.
- A grouped row is rendered one indent level deeper than a flat one, which
  eats into the same fixed sidebar column width every row already shares
  (`wrapText`/`padTrunc`, the standing narrow-width truncation gotcha).
  Session names as long as the ones task 108 used for single-session
  scenarios (e.g. `fp-bulk-kill-one-session`, 24 chars) truncate before
  ` running [marked]` fits on screen once a group header is present, so a
  `screen contains "<name> running [marked]"` assertion times out even
  though the mark itself is set correctly (`m.marked` is keyed by session id,
  never by rendered text). Keep bulk/marked-pair fixture session names short
  (`bk-one`/`bk-two`, `bd-one`/`bd-two`, `bu-one`/`bu-two`) whenever a scenario
  needs to assert the `[marked]` badge text on screen under grouping.

`features/kill_delete_undo_fingerprint_test.go` gained
`clientCreatesClaudeSessionWithScratchCWDLabelled`, the claude counterpart of
task 108's shell-only `clientCreatesShellSessionWithScratchCWDLabelled`: it
points the scenario's single shared create-modal working directory
(`h.workingDir`, the same field `positionCreateModalOnProfileField` and
`claudeTranscriptPathForSession` both read) at an already-seeded scratch
directory before delegating to the existing profile+message creation flow, so
the `@requirement-29-purge` scenario can fingerprint-test purge against a
real declared claude transcript rooted in the adversarially seeded directory.

## Task 113 validation follow-up: hardening @requirement-29-bulk-kill's j/m race

The prior validation pass flagged `@requirement-29-bulk-kill` as uniquely
flaky (2/13 failed) relative to its five structurally-identical siblings
(`@requirement-29-bulk-delete`, `@requirement-29-batch-undo`,
`@requirement-28-mark-bulk-actions`'s own batch scenarios), which all passed
5/5–6/6 in that same session. Root cause reproduced directly: the scenario's
`k`,`m`,`j`,`m` idiom sent `j` after only a fixed `100 milliseconds pass`
step, with no confirmation the first `m` had actually rendered before `j`
was written, and no confirmation `j` had actually moved the selection before
the second `m` was sent. Under host CPU contention the bubbletea reader
goroutine can be descheduled long enough that two keys land in one PTY read;
`j` and `m` are different runes so they don't hit the documented same-key
coalescing gotcha (task 112's notes), they hit the *sibling* defect that
task 118 exists to fix (multi-rune `tea.KeyMsg` dispatch only ever acts on
the first rune) — under load, `j` can be silently absorbed with the
following `m`, leaving the selection on the same row and toggling its mark
back off instead of marking the second row.

Fix applied here is test-side hardening, not a product change (task 118
owns the real fix): replaced the fixed-duration sleeps around `m`/`j`/`m`
with polling `Then screen contains "<row-one> running [marked]"` after the
first `m` and `Then screen contains "> <row-two> running"` after `j`, so
each step's own render is confirmed before the next key is written — the
same pattern the notes' Gherkin-chord gotcha already recommends. Applied
identically to `@requirement-29-bulk-delete` and `@requirement-29-batch-undo`
(same idiom, same latent race) for consistency, though only bulk-kill was
the validator's named target.

Evidence: `docs/reports/phase3-task113-bulkkill-fix.log` — 10 consecutive
isolated `DECK_GODOG_TAGS="@requirement-29-bulk-kill"` runs, all green (a
second, unlogged batch of 20 consecutive runs during development was also
100% green). This clears the specific validation bar ("at least 10/10
consecutive runs").

**Important residual finding, discovered while chasing this**: re-running
the hardened bulk-kill scenario *combined* with its five siblings, and
separately combined with task 108's five original requirement-29 scenarios,
still shows occasional failures under sufficiently elevated host load —
and, in one run, even the untouched pre-existing `@requirement-29-delete-undo`
scenario failed the same way. This means the flakiness is a genuine shared,
load-correlated characteristic of every `k`/`m`/`j`/`m` marking idiom in this
suite (and possibly PTY-driven scenarios generally), not something specific
to bulk-kill's original wording — the validator's session simply happened to
land at a moment of lower ambient load. The hardening above measurably
improves bulk-kill's *own* reliability (fixed-sleep → poll-on-render), but
does not and cannot fully eliminate the underlying multi-rune coalescing
drop; that is task 118's job. Do not re-litigate this by adding more sleeps
or checkpoints to other scenarios in this pass — wait for task 118 to land,
then re-run `ci/stability.sh` (task 131) to see whether the shared flake
rate actually drops.

**Separate, unrelated discovery from the same investigation**: running the
`features` package with `DECK_GODOG_TAGS` set to any expression that
*mentions* `@real-agents` (including `~@real-agents`, meant to exclude it)
reliably breaks every scenario in that same run that creates a claude
session (shell-only scenarios are unaffected) with `step error: trust real
Claude scenario cwd: open /.claude.json: permission denied`, because
`trustRealClaudeScenarioWorkingDirectory`'s own gate is a bare substring
check (task 110's notes already flagged this for a single scenario; this
confirms it fires for every claude-session scenario across a combined run,
not just the one it was first noticed on). The standing rule's own
recommended one-scenario invocation
(`DECK_GODOG_TAGS="@tag && ~@real-agents"`) is still safe for any scenario
that never creates a claude session (the gate function is only reachable
from that step); it is specifically a claude-session-creating scenario, or
a multi-scenario tag set where at least one member creates a claude
session, combined with any mention of `@real-agents`, that trips this.
When hand-running a tag set that includes any claude-session scenario, drop
the `@real-agents` mention entirely rather than negating it.

## Task 134: task 113's residual does not close after task 118, and is not confined to two scenarios — decision (b)

Steering 006 asked for the two flaky `@requirement-29-bulk-delete`/`@requirement-29-batch-undo`
scenarios to be re-measured once task 118 (commit 465a7d9, coalesced multi-rune `KeyMsg`
dispatch fix) landed, and for a deliberate (a)/(b) decision afterward. Re-measurement (10
consecutive isolated runs of each of the three marked-set scenarios, full data and command
in `docs/reports/phase3-task134-residual-measurement.log`):

- `@requirement-29-bulk-kill`: 3/10 passed (7/10 failed)
- `@requirement-29-bulk-delete`: 5/10 passed (5/10 failed)
- `@requirement-29-batch-undo`: 8/10 passed (2/10 failed)

Two findings that change the shape of the problem from how task 113 described it:

1. **Task 118 did not close the gap.** All three still fail, at rates as bad as or worse than
   task 113's own validation measured before 118 landed. This is expected once the failure
   is inspected closely: the observed drop is a single, non-coalesced `j` keystroke — it
   arrives alone in its own PTY read (there is no adjacent rune for it to coalesce with in
   this idiom's structure: `m`, *then a checkpoint*, `j`, *then a checkpoint*, `m`), so it was
   never a candidate for task 118's multi-rune-`KeyRunes`-splitting fix in the first place.
   Task 118 fixed exactly the defect it targeted (a dropped-whole-event coalesced `tea.KeyMsg`);
   this is a different failure at a different layer (PTY-level single-keystroke delivery loss),
   consistent with steering 006 point 2's instruction to report the two numbers (coalescing-fixed
   vs. residual) separately — the residual number here is the one that matters, and it is large.
2. **The gap is not confined to bulk-delete/batch-undo — it now reproduces just as severely on
   bulk-kill**, the one scenario task 113's own follow-up (commit 0cc8c4a) evidenced at 10/10
   and used as the template the other two were "hardened identically" to. That 10/10 result does
   not generalize across time/host-load conditions on this shared host; it was a true measurement
   at the moment it was taken, not a property of the scenario. All three scenarios use the
   identical `k`,`m`,checkpoint,`j`,checkpoint,`m` idiom and the identical poll-on-render
   checkpoints (task 113's own hardening); their differing pass rates across any given 10-run
   sample are noise around what is evidently one shared defect, not three separate ones.

**Why this is not closable by more test-side hardening** (steering 006 point 4 forecloses the
usual options, and each was considered and rejected for this reason): the failing step *is* a
poll-on-render checkpoint already (`Then ... screen contains "> bk-two running"`); it is doing
exactly its job of failing loudly and specifically when the keystroke that should have produced
that frame never arrives. Widening its 5s timeout would not fix a keystroke that is never coming;
it would only convert a fast, clear failure into a slow, equally-clear one. There is no known
lower layer in the test harness where this keystroke is provably being dropped (unlike task 114's
read-buffer race, no fixed-size-buffer or racy-first-capture pattern was found on inspection of
`ScreenDriver.Send`/`WaitForFrame`) — this looks like the same family of host-load-correlated
PTY/tmux/bubbletea keystroke-delivery characteristic already on record for
`create_cwd_ghost.feature`'s tilde-expansion timing test (present since well before this phase),
not a bug introduced by this phase's own code.

**Decision: (b).** The residual gap is judged not worth closing inside this plan — closing it
would require harness- or product-level PTY delivery-reliability work with no known root cause
and no known fix shape, which is disproportionate to what task 113 was scoped to do. Per steering
006 point 1, task 113 is relabelled `skipped` (not `completed`, and its `failed` validation
attempts stand as-is) in the same commit that downgrades `docs/reports/phase3.md`'s
requirement-29 row off DONE to PARTIAL, removing the "every path has one [reliable] assertion"
sentence and citing this measurement. The purge/archive/kill-and-archive scenarios task 113 also
added are unaffected (they contain no marked-set `k`/`m`/`j`/`m` idiom at all) and remain fully
reliable; only the three marked-set scenarios are implicated.

## Task 114: the "falls silent#01" flake is a harness read-buffer race, not a product bug

Root-caused and fixed. The scenario is `harness.feature`'s
`@requirement-5-preview-fixtures` outline, example row #01 (`claude |
oversized.txt`) — godog's own `#NN` suffixing for the second (0-indexed)
`Examples:` row. `oversized.txt` is 4880 bytes, larger than the 4096-byte
buffer `ScreenDriver.read()` (`features/pty_driver_test.go`) passes to each
`terminal.Read`, so the fixture's single `io.Copy`/`Write` of the whole file
(`renderThenFallSilent` -> `renderFixture`, `cmd/fake-claude/main.go` and
`cmd/fake-pi/main.go`) legitimately arrives at this harness's pty reader
split across two `Read` calls. The old assertion step
(`theFakeAgentsRenderedPaneIsByteIdenticalAcrossTwoConsecutiveCaptures`,
`features/fake_agent_size_test.go`) took its "first" capture the moment the
frame became merely *non-empty* (`waitForNonEmptyFrame`), which is true as
soon as the first ~4096-byte chunk lands — well before the fixture has
finished rendering. Under low host load the two `Read`s happen microseconds
apart, so by the time the 20ms poll notices non-empty, both chunks are
usually already in; under load (a busy scheduler delaying the second `Read`,
or delaying delivery through the pty), the remainder can land more than the
fixed 150ms later than the first, making the "second" capture (taken exactly
150ms after "first") different from "first" — a genuine harness-side race on
the assertion's own timing budget, unrelated to task 118's coalesced-KeyMsg
defect (this scenario sends no keystrokes at all; it only starts a fixture
and watches its pty output).

**Reproduced deterministically** by adding 28 background CPU-bound
processes (`yes > /dev/null &`, spawned and later killed by this worker's
own captured PIDs — never a pattern-based kill) on this 28-core host, then
running `DECK_GODOG_TAGS="@requirement-5-preview-fixtures" go test -count=1
-v -run TestFeatures ./features/` against the pre-fix code: failed on the
very first attempt, on exactly `#01` (`claude | oversized.txt`), with `fake
"claude" agent pane changed between captures`. Full raw failure output
captured at `docs/reports/phase3-task114-repro.log`.

**Fix** (`features/fake_agent_size_test.go`, `features/lifecycle_test.go`):
the step that starts the fixture (`aFakeAgentRendersThePreviewFixtureAndFallsSilentAt`)
now `os.Stat`s the fixture file first and records its exact on-disk byte
length on the harness (`ScenarioHarness.fakeAgentFixtureBytes`, keyed by
agent kind). The assertion step's "first capture" wait
(`waitForFixtureFullyRendered`, replacing `waitForNonEmptyFrame`) now polls
until the driver's *accumulated raw byte count* (`ScreenDriver.Raw()`, which
for these dumb fixtures contains nothing but the fixture's own writes — no
OSC/CPR negotiation to conflate it with) reaches that exact known length,
not merely "non-empty". This is a deterministic proxy for "the whole write
has arrived", with no dependency on any fixed sleep or guessed timing
budget, and it changes only the harness's own polling condition — nothing
about the product, the fixture binaries, or any scenario's outcome-relevant
behaviour.

**Verified the fix under the identical load** that reproduced the failure:
the same 28 `yes` processes were left running (same host state) while 10
consecutive `DECK_GODOG_TAGS="@requirement-5-preview-fixtures"` runs went
against the fixed code — all green
(`docs/reports/phase3-task114-fix.log`). The stress processes were then
killed by their captured PIDs (all 28 confirmed gone) before this work was
committed.

**Scope note** (per the task's own remaining-flake-sources requirement):
this task's fix addresses only its named target,
`harness.feature`'s "...falls silent#01". It does **not** address either of
the other two known flake sources already on record: the `k`/`m`/`j`/`m`
marking idiom's coalesced-KeyMsg drop (task 113's follow-up above; task 118's
job) or `create_cwd_ghost.feature`'s tilde-expansion timing test. Both remain
open and are not claimed fixed by this task.

## DECK_TMUX_MOUSE is §6.5's environment layer of a declared key, not an invented behaviour switch (task 115)

`tmux_mouse` is declared exactly once, in `internal/config/schema.go` (top
level, `KindToggle`, default `true`), and resolved in
`internal/config/config.go`'s `LoadFrom` the same way every other boolean
schema key with an environment override already is (`ui.ascii`/`DECK_ASCII`,
`ui.mouse`/`DECK_MOUSE`): `fileCfg.TmuxMouse` is the file's value, and a set
`DECK_TMUX_MOUSE` overrides it and is recorded in
`Settings.EnvOverrides["tmux_mouse"]` — §6.5's "environment always outranks
the file" applied to a key this phase declares, not a new mechanism and not
a knob that gates anything beyond the one tmux server option it names.
`internal/tmux.Client` gained a `Mouse bool` field (zero value `false`,
matching every other `Client` field's own zero-value-is-the-honest-default
convention — nothing implicitly substitutes `config.DefaultSocket`-style
here either); `Bootstrap` sets `mouse on`/`mouse off` in the exact same
single `tmux` invocation as `exit-empty`/`remain-on-exit`/`window-size`/
`aggressive-resize`, still scoped to deck's own `-L` socket only. Only the
one `tmux.Client` cmd/deck/main.go actually uses for session creation
(`sessions.CreateShell`/`CreateAgent`/`Resume`/`Restart`, the client at
`cmd/deck/main.go:72`) is constructed with `Mouse: settings.TmuxMouse`; the
liveness-only client built for hook post-processing (`main.go:187`, which
never calls `Create`/`Bootstrap`) and `TmuxHealth`'s discovery-only client
(`internal/tui/tui.go`, which never calls `Create` either) are deliberately
left at the zero value since nothing they do reads `Client.Mouse`.

**Scope**: `ScopeRestartToApply`, by the same reasoning already applied to
`stale_after` — `Bootstrap` re-reads `Client.Mouse` on every `Create` call,
but `cmd/deck/main.go` builds that one `Client` from a `settings` local
captured once before `tui.New*` builds the Model, with no path back into a
refreshed `config.Settings`. A save through the settings takeover writes
`config.toml` immediately; the already-running process's own value does not
change until deck restarts.

**Verified against real tmux**
(`internal/tmux/tmux_test.go`,`TestBootstrapConfiguresOnlyPrivateServer` now
asserts `mouse on` with `Client.Mouse: true` alongside its four pre-existing
option checks; the new
`TestBootstrapMouseOffLeavesOtherServerOptionsUnchanged` asserts `mouse off`
with `Client.Mouse` left at its zero value and re-asserts the same four
other options are unchanged from today). `internal/config`'s schema/settings
parity tests (`TestSchemaPinsKeySet`, `TestSchemaScopes`,
`TestSettingsCategoriesGroupEveryFlatKeyExactlyOnce`) all pass with the one
new key added — the settings takeover surfaces it with no second
declaration since `settingsCategories()` walks `config.Schema` directly.

## Task 117 — what capture-pane returns while a pane is scrolled back in copy-mode (requirement 49)

**Question**: could a user scrolling an attached pane's copy-mode viewport
change what `tmux capture-pane -p` reports for that pane, and therefore flip
a session's badge (its store-derived status) to something reflecting
whatever the user happens to be looking at rather than the pane's real,
live state?

**Experiment** (full raw transcript at
`docs/reports/phase3-task117-capture-pane-copy-mode-experiment.log`): a real
tmux 3.5a server, one pane filled with 200 numbered lines so the live
bottom is trivially distinguishable from history. `capture-pane -p -S 0
-E -` (the exact range `internal/tmux.Client.CapturePreview` already uses)
and `capture-pane -p -S - -E -` (the same shape as
`internal/service/reconcile.go`'s wider `-S "-200" -E "-"` probe range) were
each taken before any client scrolled, twice while a client sat scrolled to
the very top of history inside copy-mode, and once after that client
cancelled copy-mode.

**Answer**: identical in all four conditions — the pane's live tail
(the last few lines actually at the bottom of the pane's real screen).
Copy-mode scroll position is purely local UI state inside the attached
client's own rendering of the pane; `capture-pane` is a server-side,
target-pane-ID operation that has no notion of "which client asked" or
"what that client currently has scrolled to" at all. Its `-S`/`-E` line
addressing is always relative to the pane's own screen+history buffer, the
same whether zero, one, or many clients are attached, and the same whichever
of those clients (if any) happens to be sitting in copy-mode.

**Decision**: no pinning, no explicit-range change, and no other change to
Phase 2's hook/probe machinery. Both existing capture call sites
(`internal/tmux.Client.CapturePreview`'s `-S 0 -E -` live-preview capture,
task 018/021, and `internal/service/reconcile.go`'s `-S "-200" -E "-"` probe
capture, task 009) were already immune to this by construction, before this
task ever ran. Task 117's diff is therefore a proof, not a fix: one new
scenario (`features/attach_scroll.feature`'s
`@requirement-49-scrolling-does-not-flip-the-badge`) that drives the real
product end-to-end — a real attached, wheel-scrolled, copy-mode pane sitting
on stale fixture text while the pane's actual live bottom already carries a
newer one — and asserts both the durable probe verdict and a second, never-
attached client's own sidebar row reflect the live content, never the
scrolled-to one, plus this experiment log and the two docs above citing it.

**Gotcha carried forward for any future scenario in this vein**: a session
name long enough to make the sidebar truncate its trailing status/badge text
(e.g. "scroll probe claude ! sampl...") will make a `row "<name>" contains
"sampled"`-style assertion fail even though the badge is correct — same
class of truncation already on record for task 113's `[marked]` badge.
Keep any fixture/session name this kind of assertion depends on short (this
task uses `sp-claude`).

## Task 118: coalesced multi-rune KeyMsg — decision on what a paste does

**Root cause**: bubbletea's own PTY reader (`readAnsiInputs`/`detectOneMsg` in
its `key.go`) groups every consecutive run of printable, non-control runes it
finds within one read buffer into a single `tea.KeyMsg{Type: KeyRunes}`,
regardless of whether those runes came from one keystroke or several typed in
quick succession (e.g. two fast presses of `j` then `x` landing in the same
read arrive as one `KeyMsg{Runes: []rune("jx")}`). `internal/tui`'s dispatch
switches on `msg.String()`, which for a `KeyRunes` message is
`string(k.Runes)` verbatim — `"jx"` matches no case at all, so the whole
event was silently dropped: neither `j` nor `x` took effect. This is
fail-safe (nothing destructive happens) but not the documented per-key
contract.

**Fix**: `Model.Update`'s `tea.KeyMsg` case now detects `msg.Type ==
tea.KeyRunes && len(msg.Runes) > 1 && !msg.Paste` and, for exactly that case,
splits the message back into one single-rune `tea.KeyMsg` per character and
replays them through `Update` in order — indistinguishable from the same
runes arriving as separate keystrokes. A single keypress (`len(Runes)==1`)
and every other `tea.KeyMsg` `Type` (arrow keys, Enter, Tab, Backspace, …,
all of which bubbletea gives a dedicated non-`KeyRunes` `Type` and therefore
never trigger this path at all) are untouched.

**Decision on paste**: a bracketed-paste `KeyMsg` (`msg.Paste == true`) is
deliberately exempted from the split and dispatched (or rather, *not*
dispatched — see below) exactly as before task 118. bubbletea's own
`Key.String()` already wraps a paste's runes in `"[...]"` specifically so a
paste can never accidentally match a single-letter shortcut by string
comparison; that bracketed string still matches no case in the switch, so a
paste into the session list is silently ignored outright — no character of
it is dispatched as a keystroke, and it edits no field (the list itself has
no text field for a paste to land in). This is a deliberate decision, not
the same silent-drop defect the rest of task 118 fixes: splitting a paste
rune-by-rune would let a pasted `"dd"` or `"x"` fire a real destructive
action from clipboard content the user never typed as a keystroke, which
would be strictly worse than doing nothing. If a later phase adds a
paste-target view (e.g. a text field the list itself owns), that view's own
`Runes`-reading append path (the same shape `env_editor.go`/`settings.go`/
`createCWD`'s already have) is the right place to accept it, not this
dispatch switch.

**Test-suite gotcha this fix exposed**: `internal/tui/tui_test.go`'s `key()`
helper predates task 118 and built every non-`"esc"` name — including real
special keys like `"backspace"`, `"up"`, `"down"`, `"enter"`, `"tab"`,
`"shift+tab"`, `"ctrl+s"` — as a `tea.KeyRunes` message whose `Runes` spelled
the name out literally, relying on the switch matching `msg.String()`
(which happens to equal the same string for the real `tea.KeyBackspace` etc.)
rather than on `Type`. That coincidence broke the moment multi-rune
`KeyRunes` messages started being split: `key("backspace")` stopped being
one keystroke and became nine (`'b','a','c','k',...`), typing literal text
instead of backspacing. Fixed by giving `key()` a `specialKeyTypes` map from
name to the real `tea.KeyType` bubbletea would actually send for it (`up`→
`tea.KeyUp`, `enter`→`tea.KeyEnter`, etc.); only names with no dedicated
`KeyType` (ordinary typed text, including deliberately multi-rune coalescing
probes like `"jx"`/`"dd"`) still fall through to `tea.KeyRunes`. This also
means the fix is *exactly* what a live PTY sends — the old hack was papering
over a mismatch between the test's synthetic messages and reality.

**PTY-level proof gotcha**: `features/coalesced_keymsg_test.go`'s black-box
test creates a real session and waits for `"starting"` before sending the
unpaced `"d"`,`"d"` pair — waiting on the session's own name instead (its
first, wrong version) raced ahead of the create modal's own `Enter`
submission, because the modal's Name field keeps showing the typed name on
screen the whole time the user is still on a later field, well before Enter
is even sent; the very next `Send` then lands inside the still-open cwd
text field instead of the main view. `clientCreatesShellSession` in
`assertions_test.go` already avoids this by waiting on `"starting"`, not the
name — any new black-box test driving session creation directly (bypassing
that helper) needs the same unambiguous wait target.

## Task 119: rebinding group collapse off `g`, and picking `c` (requirement 30's key gap)

SPEC.md:952's own keymap line reads "`g`/`G` top/bottom", but no code in this
tree ever wired `g`/`G` to a top/bottom jump — `g` had instead been bound
(since an earlier phase, per group.go's own "task 028"/"task 039" comments)
to toggle the selected row's workspace group collapsed/expanded, a capability
SPEC.md's keymap line never lists at all. Every press of `g` was silently
doing collapse instead of the documented jump; `G` did nothing.

**Fix**: `g` and `G` are now `Model.Update`'s SPEC.md:952 top/bottom jump,
implemented via the same `visibleSessionIndices()` (visual order, collapsed
rows skipped) that `↑`/`↓` already walk one step at a time — `g` selects
`visible[0]`, `G` selects `visible[len(visible)-1]`. Group collapse moved to
`c` (mnemonic: collapse), a letter that appears nowhere in SPEC.md §11's
keymap list (checked against every key `↵ space Y n r R x s i e P p E f / m
z A u g G , t | < > ? q` plus the two-key `dd`) so it introduces no new
collision either with an implemented key or a reserved-but-not-yet-built one
(`E`, `f`, `s`, `/`, `z` are real gaps other tasks in this plan still owe —
`c` is not one of them).

**What changed**: `internal/tui/tui.go`'s key switch (`case "c"` for
collapse, `case "g"`/`case "G"` for top/bottom); the released help text's
`g toggle the selected row's workspace group...` line became `c toggle the
selected row's workspace group collapsed/expanded` plus a new `g / G jump
to the first / last visible row` line, with both phrases mirrored into the
guard-word "present" lists in `internal/tui/tui_test.go` and
`cmd/deck/main_test.go` (same commit). `internal/tui/group_test.go`'s
`TestGKeyTogglesOnlySelectedRowsGroup`/`TestGKeyNoopUnderOverlaysAndWithNoSessions`
now drive `key("c")` instead of `key("g")`; a new
`TestGGKeysJumpToFirstAndLastVisibleRow` proves `g`/`G` land on the first/
last visible row including across a collapsed group's hidden rows, and are
no-ops under `help`/with no sessions. `features/attention_sort.feature`'s
two collapse scenarios now send `"c"`; a new `@requirement-30-top-bottom`
scenario drives `g`/`G` (including collapsing `gg-second-workspace` first
and confirming `G` still lands correctly once one group's rows are hidden)
and asserts via the pre-existing `has session "<name>" selected` step
(`clientHasSessionSelected`) rather than inventing new assertion wording.
`features/mouse.feature`'s group-header-click scenario needed no change —
it drives `toggleGroupCollapse` directly via a mouse click, never through
the `g`/`c` key at all.

**Gotcha for whoever touches this next**: collapsing a group only hides its
*member* rows from `visibleSessionIndices()` — the group's own header line
(rendered by `groupHeaderText`) stays on screen with its collapse-state
marker flipped from `▾`/`v` to `▸`/`>`, and still names the workspace and a
representative cwd. A "screen stops containing `<workspace-name>`" assertion
after collapsing is therefore always wrong; assert against a *member
session's own name* instead (the pattern the pre-existing `grp-b-1`/`solo-a`
scenarios already use, and the one this task's new scenario had to be
corrected to match after a first draft asserted on the workspace name and
failed — collapsing does not remove the header).

**Not folded into SPEC.md**: per the standing rule, `SPEC.md` is never edited
by this run; the operator should fold `c` (group collapse) into §11's keymap
list as a new entry the next time SPEC.md itself is revised — it was never
in that document at all, is unrelated to the `g`/`G` fix, but is being
newly documented here since this task is what surfaced the gap.

## Task 010: pre-existing off-by-one in `@requirement-17-clear-recent-cwds-history` (unrelated to secret masking)

While verifying task 010 (secret-shaped env value masking) did not collide
with any `.feature` scenario, `DECK_GODOG_TAGS="@requirement-17-clear-recent-cwds-history"`
failed: after `,` → `j` → Tab → four `j`s, the scenario asserts the screen
shows `cleared recent directory history` once Enter is pressed, but the
selection is actually still one row short, on `Group By Workspace`, not
`Clear Recent Cwds` — Enter toggles that field (`Off`) instead. The UI
category's field order is `Theme, Ascii, Mouse, Recent Cwd Limit, Group By
Workspace, Clear Recent Cwds` (`config.Schema` order plus the appended
`settingsClearRecentCwdsEntry`, `internal/tui/settings.go`), six entries
needing five `j`s from `Theme` to reach `Clear Recent Cwds`; the scenario
sends only four.

**Confirmed pre-existing, not caused by this task's diff**: `git stash` (
reverting every task 010 change) and re-running the same tagged scenario
reproduces the identical failure on `main`'s HEAD before this task's commit.
Not fixed here — out of task 010's scope (masking predicate + reveal
toggle), and the standing rule against loosening assertions cuts the other
way too: the fix belongs to whichever task owns `features/settings.feature`
next (adding the missing fifth `j`), not this one. The rest of `@settings`
(every other scenario in that tag group) and `@environment` both pass
cleanly with task 010's changes in place.

**Not a masking-related false failure**: this scenario asserts on UI-
category navigation and the `ui.recent_cwd_limit`/`clear_recent_cwds`
fields, none of which are `[env]` entries or otherwise touched by
`maskEnvValue`.

## Task 013 (I-8): rename does not touch the tmux session, despite SPEC.md:187's "Rename renames both"

`prds/phase3-sessions-and-lifecycle.md`'s own "Findings, not spec edits" list
names this exact question as an expected finding ("Whether a rename should
touch the tmux session (requirement 31)"), and
`prds/phase3c-residual-and-interactive-preview.md`'s I-8 answers it
explicitly: "The tmux session name does not change — deck's name and tmux's
name are decoupled — and the dialog states that on screen". That is what
task 013 implemented: `store.RenameSession` writes only the `name` column,
never `slug`, so the live tmux session (always `deck_<original slug>`)
survives a rename completely unaffected — proven directly in
`internal/service/rename_test.go`'s
`TestRenameChangesNameLeavesSlugAndTmuxSessionUntouched` and end-to-end in
`features/dialogs.feature`'s three new `rename dialog --` scenarios (esc,
submit, and mouse-guard cases all assert the original `deck_<slug>` tmux
session survives, and submit additionally asserts no second tmux session
under a new slug is ever created).

This appears to contradict `SPEC.md:187`: "Session naming: `deck_<slug>`,
slug `[a-z0-9_-]+` derived from the name, uniqueness enforced in SQLite.
**Rename renames both.**" Read in isolation, "renames both" says a rename
changes both `name` and `slug` together — which would mean renaming an
agent session moves/recreates its live tmux session too.

Resolving the apparent conflict in the PRD's favor (SPEC.md is not edited by
this run, per the standing rule): `SPEC.md:187`'s sentence sits inside a
paragraph describing the create-time naming *scheme* (how `slug` is first
derived from `name`, and that both columns are enforced unique) — it reads
most naturally as "a rename supplies a new name, and slug is (freshly)
derived from it the same way it always is," not as a promise that a later
rename also *migrates* the live tmux session. `SPEC.md:194` ("rename [§11.4]
treats [a default-derived name] like any other name") and `SPEC.md:982`
("`i` session detail (§11.4 — **rename is an action inside it**, not a
top-level key)") both point at §11.4 for rename's actual contract, and
§11.4/I-8 is unambiguous that the tmux session itself is not part of what a
rename changes. Filed here, per the PRD's own explicit request, rather than
editing SPEC.md.

## Task 002: three files were gofmt-dirty at HEAD before Part I (I-21 precondition)

Before any Part I task ran, `ci/run.sh sh -c 'gofmt -l $(git ls-files "*.go")'` named three
already-committed files as unformatted: `features/cell_attributes_test.go`,
`features/layout_modes_test.go`, `internal/tui/group_visual_order_test.go`. This was not
introduced by any task in this run — it predates task 001's own commit and had gone unnoticed
because no earlier phase's own workflow ran a repo-wide `gofmt -l` sweep, only `gofmt` on the
files each task itself touched. Task 002 fixed it with `gofmt -w` applied to exactly those three
files, confirmed whitespace/alignment-only via `git show HEAD | grep -v '^[-+][[:space:]]*$'`
showing no semantic line changed (commit `f8487fb`). Recorded here as the concrete instance of
a durable process gap for whoever plans the next phase: `ci/run.sh sh -c 'gofmt -l $(git ls-files
"*.go")'` catching zero files is worth checking repo-wide periodically, not only on the files a
given task's own diff touches — per-task `gofmt` on touched files alone does not catch a
pre-existing dirty file elsewhere in the tree.

## Tasks 003–005a: I-1's single-keystroke drop was layer 4 (product), not delivery or render lag

The full investigation and fix live in `docs/reports/phase3d-i1-rootcause.md` (excluded layers
1/2/3/5 by direct measurement) and `docs/reports/phase3d-i1-repro.log`/`-counter-poll.log`
(reproduction with task 003's `DECK_INPUT_COUNT_FILE` instrument, `cmd/deck/inputcount_hook.go`,
proving the dropped keystroke *did* reach `Model.Update` — this was not a PTY/Bubble-Tea-reader
drop). Summarised here since a load-correlated intermittent failure is exactly the kind of gotcha
an operator or a later phase needs findable without reading four separate reports:

- **Mechanism**: `internal/tui/attention.go`'s `sortSessionsByAttentionStable` broke ties on rank
  and `StatusAt` (millisecond resolution) by session ID — a random UUID uncorrelated with
  creation order or which row a keystroke targeted. When `internal/service/reconcile.go`
  promotes two sessions from `starting` to `running` in the *same* reconcile pass, both get an
  identical `StatusAt` millisecond stamp, so the tie-break silently reorders the two rows on
  screen between a `k`/`m` mark and a following `j`, landing `j` on a row the test never intended
  to move off of. The keystroke was never dropped; the row it moved from/to changed under it.
- **Fix (task 005a)**: `posKey(s) = prevPos[s.ID]` if present, else `len(previous)`, then
  `a.ID < b.ID` — a lexicographic composite key that keeps the sort a strict weak ordering (no
  intransitive 3-way cycle) and, critically, keeps a previously-visible row's relative order
  stable across a re-sort even when a brand-new row's tie-break ID would otherwise interleave
  between two existing rows.
- **Verification**: each of `@requirement-29-bulk-kill`, `@requirement-29-bulk-delete`,
  `@requirement-29-batch-undo` measured 10/10 consecutive isolated runs post-fix (load 1.96–7.24,
  `docs/reports/phase3d-i1-rootcause-stability-*.log`), spanning and exceeding task 134's own
  6.18–7.28 failure-reproducing load range from before I-1 was root-caused.
- **Not the same defect as task 118's coalesced-`KeyMsg` fix** — that fix targets two runes
  landing in the *same* PTY read; this drop's `j` always arrived alone in its own read. Both are
  real, independently-verified defects that happened to surface through the same `k`/`m`/`j`/`m`
  idiom.

## Part II's `~/deck-spikes/` spike-evidence directory is absent from this container

`prds/phase3b-interactive-preview.md`:17 and `prds/phase3c-residual-and-interactive-preview.md`:397
both instruct: "Every technical claim in this PRD was measured ... on both tmux and screen. Read
`~/deck-spikes/interactive-preview/README.md` first, then `a/REPORT.md` (geometry) ... the spike
evidence is the citation. Do not re-derive them." `~/deck-spikes` does not exist anywhere in this
workspace/container (`ls ~/deck-spikes` → No such file or directory; no `deck-spikes` path found
under `/` at all). Recorded here, not fixed, per the standing instruction that a `prds/` defect is
filed as a finding for the operator rather than silently worked around: whichever task first opens
Part II (task 025, the §11.9 pre-check, or task 026 itself) will hit this the moment it tries to
follow that citation, and needs to either locate the real spike directory outside this container,
ask the operator for it, or explicitly re-derive/measure the claim itself (documenting that
departure per the PRD's own "do not contradict them without recording why" clause at
`phase3c-residual-and-interactive-preview.md`:722) — not invent a citation to a file nobody can
read.


## Standing pre-existing flake list (durable copy — this run's ralphd `notes.md` is not committed and does not survive the run)

Added per operator steer 015 item 2: the run's scratch `notes.md` (in the ralphd run directory,
not this repository) had been carrying the authoritative "open pre-existing flakes" list and
being cited from this very file three times. `notes.md` is deleted when the run ends, so this
section is now the durable copy; the three citations below were rewritten to point here instead.
Each entry states what evidence for "confirmed genuinely intermittent, not deterministically
broken" actually exists on record in `docs/reports/` or a task's own `notes` field in
`tasks.json` — not invented confirmation where none was captured.

**Correction (task 201): the second bullet below is wrong.** It classified
`interactive_refusals.feature`'s 7-row-floor scenario as a load-correlated PTY timeout. It is
neither load-correlated nor pre-existing: task 201 reproduced it on the FIRST TRY at 1-min
loadavg 3.73 (well under the 4.0 this list's own load-correlation claims elsewhere), and the
scenario (`interactive_refusals.feature:29`) was itself created in this run to cover requirement
48, so it cannot be "pre-existing." The failing frame's title bar reads `interactive 41x6
fitted` — deck ENTERED interactive mode at a 6-inner-row box; the refusal never fires, so the
scenario's wait for the "7-row floor" text is waiting for text that literally never gets
composed, which is why it reads as a timeout. Root cause and mechanism (a harness resize step
that returns before deck processes the resize, racing against a floor check that only runs once
at entry with no re-check after a shrink): `docs/reports/phase3d-201-req48-degrade-rootcause.md`,
with the reproduction log at `docs/reports/phase3d-201-req48-repro.log`. The bullet is left below,
struck through in spirit but not in text, as a record of what was previously believed; do not cite
it as a correct classification. The fix for the underlying defect is tracked separately (tasks
202-204); this correction is docs-only.

**Correction (task 206): the first bullet below is also wrong.** It classified
`attach_scroll.feature:11`'s `@requirement-48-wheel-scrolls-attached-pane-without-typing` scenario
as a load-correlated PTY-under-load timeout. It is not: task 206 reproduced it isolated at loadavg
as low as 3.7 (under this list's own 4.0 load-correlation threshold), root-caused it to a genuine
harness pacing race (the copy-mode cancel key sent before tmux finishes draining the preceding
burst of `WheelUp` SGR reports, confirmed against the real tmux server's own `#{pane_in_mode}`,
not the harness's screen emulator), and fixed it (`features/attach_scroll_test.go`'s
`waitForCopyModeQueueToDrain`). Full writeup and 10-consecutive-isolated-run evidence:
`docs/reports/phase3d-206-attach-scroll-hang-rootcause.md`. The bullet is left below, struck
through in spirit but not in text, as a record of what was previously believed.

1. **`internal/tmux`'s pane-pipe close-race family** —
   `TestPanePipeReceivesGenuineEOFOnDisplacementWithPanePipeStillOne` and
   `TestPanePipeWasClosedIsTrueBeforeAnyBlockedReadCanObserveOurOwnCloseAsEOF`
   (`internal/tmux/pipe_displacement_test.go`). Confirmed-in-isolation evidence: task 060's notes
   record one of the two failing once in a full-package run, re-run 3x with a *different* test
   failing each time (i.e. not a deterministic single culprit); task 075 confirmed 3/3
   isolated-green for `TestPanePipeWasClosedIsTrueBeforeAnyBlockedReadCanObserveOurOwnCloseAsEOF`
   via `ci/run.sh go test -race -count=1 -run TestPanePipeWasClosedIsTrueBeforeAnyBlockedReadCanObserveOurOwnCloseAsEOF ./internal/tmux/...`
   (see this file's requirement-17 section above); task 076's ten-run stability measurement
   (`docs/reports/phase3d-i20-stability.md`) saw it once (run 8 of 10), consistent with genuine
   intermittency rather than a regression.

2. **Load-correlated PTY-under-load timeout class**, with two worked examples both fully
   documented in `docs/reports/phase3d-i20-stability.md`:
   - ~~`attach_scroll.feature:11`'s `@requirement-48-wheel-scrolls-attached-pane-without-typing`
     hangs the whole `TestFeatures` process to Go's hard 10-minute test timeout under concurrent
     host load (task 076's runs 2 and 5; also seen once each by task 091's and task 075's own
     full-suite runs). Confirmed-in-isolation: task 076's report states it "has never been seen
     to fail deterministically at low/idle load in any isolated rerun on record"; task 091 reran
     it isolated at normal load (~1.8) and it passed in 1.4s.~~ **Struck by task 206: this
     classification is wrong too — see the correction note below this numbered list.** Not a
     load artifact; a real harness pacing race between the burst of `WheelUp` SGR reports and the
     copy-mode cancel key sent right after it, reproduced at loadavg as low as 3.7. Root cause,
     fix, and 10-consecutive-isolated-run evidence:
     `docs/reports/phase3d-206-attach-scroll-hang-rootcause.md`.
   - ~~`interactive_refusals.feature`'s 7-row-floor scenario ("entering interactive mode is
     refused while the preview box has fewer than 7 inner rows") times out waiting for the
     "7-row floor" frame under load (task 076's runs 6, 7, 8 and 10 — the majority of that
     measurement's failures). No isolated-rerun citation for this specific scenario is on record
     beyond task 076's report; it is carried here on the strength of that report alone.~~ **Struck
     by task 201: this classification is wrong — see the correction note above this numbered
     list.** This is not a load-correlated timeout and not pre-existing; it is a real
     degrade-instead-of-refuse defect (deck enters interactive mode at `41x6 fitted` instead of
     refusing), reproduced deterministically at loadavg 3.73. Retained here, struck, only so the
     history of what was previously believed is not silently deleted.

3. **`TestSessionResizeDuringLiveDrainIsRaceFree`** (`internal/interactive/resize_test.go:199`) —
   failed once in six whole-package `-race` runs (task 044's/task 085's notes) with a
   `t.Fatalf` from an unjoined background goroutine (`sendLiteralLine`, a real `send-keys`
   subprocess call racing the test's own `defer cleanup()` tearing the tmux server down) — a
   synchronization gap, not a `WARNING: DATA RACE` report. Confirmed-in-isolation: task 085's
   notes record ten isolated reruns of just that test, all green.

4. **`TestDispatcherSendLiteralStreamsOversizedPayloadViaLoadBufferAndArrivesIntact`**
   (`internal/tmux/chunk_test.go`) — observed failing once each in task 060's and task 067's
   full-package `go test ./internal/...` runs. Confirmed-in-isolation: both tasks' notes record
   an isolated rerun passing cleanly immediately after (task 067: "reran isolated, passed").

5. **`TestSigwinchCountDistinguishesTwoFromThree`** (`features/sigwinch_count_test.go`, added by
   task 027) — was carried on the scratch `notes.md` flake list without a separately recorded
   isolated-confirmation citation anywhere in `docs/reports/` or any task's `notes` field. Stated
   honestly rather than inventing confirmation that was never actually captured: this entry's
   only evidentiary basis is this run's own (uncommitted, now-gone) scratch notes having observed
   it once. A future run that sees it fail again should reproduce it isolated 3x at low load
   before trusting this list's classification, per this file's own requirement-17 lesson below.

## `@requirement-17-clear-recent-cwds-history`'s navigation math went stale, causing a deterministic (not merely load-correlated) failure

Discovered while verifying task 075's precondition (whole-suite green before the final commit).
`features/settings.feature`'s `@requirement-17-clear-recent-cwds-history` scenario (task 013,
commit `bb73b04`) navigates the settings takeover's `[ui]` field list with a fixed count of `j`
presses (4, after the initial category `j` + tab) to land on the synthetic "Clear Recent Cwds"
action and press Enter. At the time task 013 wrote it, `internal/config/schema.go`'s `[ui]`
section held exactly 4 fields before that action (`theme`, `ascii`, `mouse`, `recent_cwd_limit`).
Task 007 (commit `a94a0f6`, landed the day after) inserted a 5th field, `group_by_workspace`,
between `recent_cwd_limit` and the append point — shifting the action's index by one without
updating this scenario's `j` count. The result: the 4th `j` now lands on `group_by_workspace`
instead of the action, so the scenario's subsequent Enter toggles `group_by_workspace` off instead
of clearing the recent-cwd history, and the "cleared recent directory history" note never
appears. This reproduced deterministically (3/3 isolated reruns at load ~2.3–3.3, no load
correlation) — it had previously been miscategorised in the standing "open pre-existing
flakes" list (this file's own "Standing pre-existing flake list" section above) as a
load-correlated PTY timeout, which it is not.

**Fix**: add the missing 5th `j` (`features/settings.feature`, one line). Verified 3/3 isolated
green post-fix. The standing flake list (this file's section above) is corrected to drop this
entry (it was never a flake).
No production code changed; `internal/config/schema.go`'s field order is correct and unchanged.

An earlier `docs/reports/phase3d-075-final-suite-run.log` (committed alongside this fix in the
same iteration) actually showed `internal/tmux`'s
`TestPanePipeWasClosedIsTrueBeforeAnyBlockedReadCanObserveOurOwnCloseAsEOF` FAILing (sibling of the
already-recorded `TestPanePipeReceivesGenuineEOFOnDisplacementWithPanePipeStillOne` close-race
family in `pipe_displacement_test.go`) — the log was committed without actually being green, which
validation caught. Confirmed non-reproducing 3/3 in isolation
(`ci/run.sh go test -race -count=1 -run TestPanePipeWasClosedIsTrueBeforeAnyBlockedReadCanObserveOurOwnCloseAsEOF ./internal/tmux/...`).
A subsequent retry attempt (same iteration budget rules, one whole-suite run) hit a different,
also load-correlated failure instead: a 10-minute per-test timeout inside
`attach_scroll.feature:11`'s `@requirement-48-wheel-scrolls-attached-pane-without-typing` scenario,
with a second `ralphd` job (`selfdev-v09-fsm-loop`) confirmed running concurrently on the host via
`docker ps` at the time; that exact scenario passed 3/3 isolated (~1.5–3s each, nowhere near the
timeout) immediately after. Neither failure was fixed by weakening a test — both are the standing
load-correlated PTY/close-race class already on record in this file's "Standing pre-existing
flake list" section above.

A later retry (this iteration) ran the whole suite once more at load ~2.6–3.5 (concurrent job
still present) and it came back fully green, including `internal/tmux` (18.7s) and `features`
(257.7s) with the `@requirement-48` scenario passing inline this time — no FAIL anywhere in the
run. That log now replaces the earlier (failing) one at
`docs/reports/phase3d-075-final-suite-run.log`, and is the log task 075 cites as its green
whole-suite run. Not re-run a second time in the same iteration to chase a repeat clean streak,
per the standing budget rule.

**Follow-up finding (operator steer 015 item 1, recorded not fixed): the fix restores the stale
count but not a landing guard, so the same class of defect recurs on the next inserted `[ui]`
field.** The scenario's only assertion after the `j` presses is
`Then deck client "A" screen contains "Clear Recent Cwds: press enter/space to clear now"`, and
that string renders whenever the Clear Recent Cwds row is drawn at all — `settingsFieldValueDisplay`
(`internal/tui/settings.go:934`, the `KindLink` branch at `:967`) is a pure function of
`config.Field`/`FileConfig` with no selection parameter, so the assertion passes whether or not the
cursor actually landed on that row. Reverting the fix's 5th `j` reproduces this directly: the
cursor sits one row short on `group_by_workspace`, this assertion still passes (the row still
renders its text), and the scenario only goes red two steps later at
`contains "cleared recent directory history"` — which is why the original defect surfaced as an
unrelated-looking failure instead of "the cursor is one row short," and why it was filed as a
flake for a day before this file's own section above corrected that.

The two sibling fixed-count-`j` scenarios in `features/settings.feature` (the `[env]` table's
restart-to-apply-scope scenario at line 152, and requirement 19's restart-to-apply flat-key
scenario at line 163) both already pin the landing with a second assertion on the selection-only
detail line (`"Kind: … · Scope: …"`, emitted only for the selected row —
`internal/tui/settings.go:1420`, `if selected { … settingsFieldDetailLines(…) }`). Requirement-17's
scenario is the one outlier that omits this idiom, which is the file's own established convention,
not a new one being proposed here.

The concrete one-line fix, not landed per the operator's explicit instruction (see below): add
`And deck client "A" screen contains "Kind: link · Scope: global"` immediately after the existing
assertion. Confirmed the literals without landing them: `KindLink = "link"`
(`internal/config/schema.go:21`), `ScopeGlobal = "global"` (`:47`), and
`settingsClearRecentCwdsEntry` declares `Scope: config.ScopeGlobal` (`internal/tui/settings.go:143`)
— so this matches the sibling idiom exactly, and states *why* the row is the right one rather than
only *that* the cursor is there (the alternative, prefixing the existing assertion with the `"> "`
selection marker per `settings.go:1388-1393`, works too but is less self-explanatory; the per-segment
SGR caveat at `settings.go:1409` does not apply to either, since `clientScreenContains` reads the
cell-grid `Frame`, not the escape stream — `features/pty_driver_test.go:317`).

**Deliberately not landed this iteration.** Per operator steer 015: this is latent fragility in an
already-green, already-correct scenario, not a live failure, and landing it now would touch the
tree while task 076's `ci/stability.sh 10` measurement was in flight (that measurement has since
completed and is recorded above) and after task 075 declared itself the last code commit of the
run. Recorded here as a finding for whichever phase next touches `features/settings.feature`, with
the exact one-line assertion to add and the file:line evidence needed to add it without
re-deriving any of the above.
