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
reliably corrupts every subsequent scenario in the same process with `step
error: trust real Claude scenario cwd: open /.claude.json: permission
denied`, because `trustRealClaudeScenarioWorkingDirectory`'s own gate is a
bare substring check (task 110's notes already flagged this for a single
scenario; this confirms it cascades to an entire run). When hand-running
any subset of scenarios that are not themselves real-agents scenarios, pass
only the specific tags wanted, with no `@real-agents` token anywhere in the
expression, not even negated.
