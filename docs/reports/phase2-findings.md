# Phase 2 design findings

This report records Phase 2 decisions, disagreements, upstream assumptions, and deferrals for later operator review against `SPEC.md`. `SPEC.md` and `prds/` were not changed to make the implementation fit.

## Frozen wall-clock control

- `SPEC.md` §13.1 says `DECK_CLOCK_STEP` advances a frozen clock “on demand”, but it does not define a cross-process, collision-free trigger. The original per-process counter advanced only after shell creation and could not make two running clients and a short-lived `_hook` process agree. An initially implemented `>` binding also conflicted with §11's sidebar-width keymap and was removed.
- The implemented control is `clock.now` under deck's resolved data root: `$DECK_HOME/clock.now` when `DECK_HOME` is set, otherwise the equivalent file under the resolved XDG data directory. With `DECK_CLOCK` set, a valid RFC3339/RFC3339Nano instant in this file overrides the initial frozen value. Writing an absolute instant is atomic from the reader's perspective, works while clients are running, and gives every process sharing that data root the same `now`.
- `DECK_CLOCK_STEP` remains the configured/suggested increment for automation; it does not claim a TUI key. The help overlay documents `DECK_CLOCK`, `DECK_CLOCK_STEP`, and the write-based `clock.now` surface. This is new supported product surface that should be folded into §13.1.
- Wall time controls status timestamps, staleness, and rendered ages only. Hook transaction durations and reconciliation timeouts use Go's monotonic clock and continue advancing while wall time is frozen.
- Phase 2 removed the store's implicit real-time timestamp fallback from status/event APIs it exercises: callers must supply an explicit timestamp. This is an API decision needed to prevent a real-time write from silently poisoning frozen-clock staleness comparisons.

## Claude hook injection mechanism

- Phase 2 chose Claude's inline command-line settings mechanism: each launch/resume appends `--settings <JSON>`. The JSON contains command hooks for `SessionStart`, `UserPromptSubmit`, `Notification`, `Stop`, `StopFailure`, and `SessionEnd`; every command invokes the shell-quoted absolute running deck executable with `_hook`.
- Inline JSON was preferred because `Adapter.Instrument` stays a pure function, there is no settings artifact to synchronize or clean up, and deck writes neither user/project settings nor anything under the session cwd. Deck-owned `DECK_SESSION_ID` and `DECK_HOME` are merged into the pane environment at launch but are not persisted or displayed as user session environment.
- The implementation relies on Claude's documented settings-source merge behavior so deck's hooks are additive to user/project hooks. It does not read or normalize those other settings.
- The command hook currently supplies the upstream `type` and `command` fields and relies on Claude's hook timeout default. The spec's phrase “declares a modest” session-end timeout does not choose a value or pin the current upstream timeout-field shape; that remains an upstream contract to confirm before adding another field.
- If a future real-Claude conformance run rejects inline JSON, the fallback is a deck-owned settings file under the resolved data root, never under the session cwd or user settings. No such fallback was implemented speculatively.

## Upstream Claude contract: confirmed and unconfirmed

The default suite deliberately proves deck's declared contract with `fake-claude`; it cannot prove what an installed upstream release emits. The opt-in command is:

```sh
DECK_GODOG_TAGS=@real-agents go test -run TestFeatures -v ./features/...
```

This environment had no installed/authenticated Claude, so the new real-hook scenario could not be executed here. It is strict by design: no aliases, type coercion, or silent normalization. A changed shape reports the observed keys/types as an unsupported upstream contract.

### Event names and event-specific fields not confirmed here

- All six subscribed names remain upstream assumptions: `SessionStart`, `UserPromptSubmit`, `Notification`, `Stop`, `StopFailure`, and `SessionEnd`. The fake agent emits exactly these names; deck does not rename its subscription set to match an anecdotal CLI observation.
- The `SessionStart.source` field and its vocabulary are unconfirmed. The mapping preserves the supplied string as `status_reason`; tests cover the expected fresh/resumed/compacted meanings without normalizing variants such as `startup`, `resume`, or `compact` in handler code.
- `Notification.notification_type` and the expected permission-prompt, question, needs-input, and idle-prompt values are unconfirmed. Values are preserved verbatim as `status_reason`, including unknown future strings.
- `Stop.last_assistant_message`, `StopFailure.error_type`, and `SessionEnd.reason` are unconfirmed as names, presence rules, and types. The declared string field is consumed when present; raw JSON is retained in the event so drift is diagnosable.
- Whether every event includes the common `session_id`, `cwd`, `transcript_path`, and `permission_mode` fields with non-empty string types is unconfirmed in this environment. The opt-in scenario currently checks those four fields on an actual `SessionStart`, checks the exact conversation id and cwd, and fails explicitly if the installed CLI omits, renames, or changes their types.
- The opt-in scenario also checks that the installed CLI accepts inline `--settings`, fires the command hook, and reaches the exact released `deck _hook`. Until it is run in an installed/authenticated environment, those facts remain conformance assumptions rather than claims from this run.
- Real emission and field coverage for the other five events should be added to `@real-agents` when stable, non-destructive ways to provoke each event are known. Probe fallback remains the deliberate degradation path in the meantime.

## Hook mapping and persistence decisions

- §8.1's prose names user-facing events while §4's `events.kind` comment lists a different vocabulary (`started`, `prompt`, `waiting`, and so on) and does not define the translation. Phase 2 stores stable upstream-derived snake-case kinds: `session_start`, `user_prompt_submitted`, `notification`, `stop`, `stop_failure`, and `session_end`. Status, reason-field, message-field, and event kind are reviewable in one mapping table.
- Session resolution follows the specified order: a unique payload conversation id, then injected deck row id. A duplicate conversation id is not guessed; the injected row id may disambiguate it. An unresolved payload is preserved as an event with `NULL session_id` and returned as a documented diagnostic.
- The spec calls the highest precedence `user-terminal`, while schema and existing UI use `status_source = user`. Phase 2 keeps `user` as the durable source value and represents terminality separately with `killed_by_user=1`; badges therefore key on evidence quality (`hook`/`probe`), not on a new source spelling.
- `last_message` is truncated at 2 KiB at the store boundary using a valid UTF-8 prefix. Raw hook payload preservation remains independently bounded by the event API rather than presenting transcript text as current status.
- Waiting/error transitions clear acknowledgement. Leaving either attention state increments `notify_epoch` in the same transaction, including attach-driven `waiting -> running`; `Y` changes only acknowledgement and does not invent a status transition or epoch.

## Status badge, age, and copy choices

`SPEC.md` requires honest live/sampled quality but leaves exact list/detail copy and glyphs open. Phase 2 chose:

- The list uses the plain words `live` and `sampled` in a fixed source-quality column, without brackets or an icon. Quality derives from `status_source`, not agent kind: `hook` is `live`; `probe` is `sampled`, including Claude fallback and Pi; `tmux` and `user` are blank because neither claims an agent verdict.
- Detail renders `Verdict source: hook (live)` or `probe (sampled)`. Tmux/user sources show only the source. `Verdict age` uses the shared frozen wall clock and coarse stable copy: `just now`, then whole minutes, hours, or days. A future timestamp renders `just now` rather than a misleading negative age.
- Agent rows in `starting` render `starting · awaiting signal` (`starting - awaiting signal` in ASCII mode). Shell rows never show that suffix; a live shell is promoted to `running`. This intentionally does not infer running for a live but unsignalled agent.
- Unacknowledged waiting/error rows use `●` (`!` in ASCII mode). `Y` is named `acknowledge` in the footer/help so it is not confused with changing the status.
- Detail shows the hook `last_message` under `Last message`. A stored crash tail may contain 200 lines, but the non-scrolling detail view shows its first four and last four lines with an explicit omitted-line count and exit status. Storage remains the complete sanitized bounded tail.

## Probe decisions and a spec disagreement

- Probe classification is one ordered table. Exact fixture bytes and expected status/reason are shared by golden tests and released-binary pane scenarios. Unknown text yields no verdict rather than guessing.
- `SPEC.md` §8's adapter matrix labels shell status “probe (sampled)”, while §7 explicitly says a shell has nothing meaningful to probe, is never probed, and derives running solely from tmux liveness. Phase 2 follows §7, the authoritative state machine: shell `Probe` always declines, shell rows never receive a sampled badge, and only live-shell `starting -> running` is permitted.
- Probe reasons (`startup`, `working indicator`, `permission prompt`, `idle prompt`, and agent/API error descriptions) are deck fixture vocabulary, not claims of an upstream structured API. Upstream screen redesign is expected to require fixture/table updates.
- A probe that loses to a fresh hook is still recorded as an event. This was chosen so precedence tests prove the probe actually ran instead of passing vacuously because no probe happened.

## Liveness and crash decisions left implicit by the spec

- Some tmux versions report signal death through `pane_dead_signal` while leaving `pane_dead_status` empty. Phase 2 records the conventional shell status `128 + signal` (therefore SIGKILL is 137), so `pane_exit_status` remains a single comparable integer across normal nonzero exits and signals.
- Crash capture strips ANSI/control sequences and retains the last 200 plain-text lines before collection. The first writer of crash metadata wins; subsequent reconcilers may still idempotently kill an already-disappeared tmux session.
- A hook-triggered bounded liveness pass reconciles the whole session set, not only the hook target. This is required for the specified unattended scenario where a hook from one live session discovers another session's crash, even though §7's shorthand says “the next `_hook` invocation for that session”. It never probes and never relaunches.
- The bounded pass currently has a short service deadline so a stalled tmux command cannot hold the agent hook indefinitely. Its timeout is operational protection, not frozen wall time and not part of the measured `<20 ms` store-write span.

## Session-end no-enqueue deferral to Phase 5

- `SPEC.md`'s conceptual data model includes an `outbox` and §8.1 says session-end is enqueue-only. The repository's actual schema version 1 has no `outbox`, and this phase forbids a schema migration or notification implementation. Enqueue-only is therefore impossible in Phase 2.
- The implemented Phase 2 contract is deliberately narrower and pinned by `features/hook_contract.feature`: `SessionEnd` performs exactly one atomic status/event store write, then exits with no liveness, probe, dispatch, or enqueue attempt. The audit log, not a comment, proves those absences.
- This is temporary negative evidence, not the final product contract. **Phase 5 must add the outbox, enqueue session-end notification work, and re-aim the no-enqueue assertion** while retaining no inline dispatch and no liveness/probe work on the session-end path.

## Feature-layout disagreement

- `SPEC.md` §13.3's enumerated feature layout names the principal Phase 2 files (`status_claude_hooks.feature`, `status_probe.feature`, and `crash.feature`) but does not list every behavior-area file needed by its own requirements.
- `features/shell_liveness.feature` holds the isolated shell-promotion/unsignalled-agent invariant, and `features/status_user_kill.feature` holds kill precedence and resume-clears-kill behavior. Both follow §13.3's behavior naming rule and avoid a phase-named file, but are absent from the literal enumerated layout. The spec was not edited to hide that discrepancy.

## Explicit deferrals and boundaries

- Notification channels, rules, quiet hours, redaction, outbox/retry, dispatch, and epoch dedupe consumption remain Phase 5. Phase 2 only maintains `notify_epoch` and the temporary session-end absence described above.
- Pi and Codex event sources remain deferred as §8.1 states; Pi is sampled in Phase 2. Codex adapter/id discovery remains Phase 4 work.
- Sidebar/preview/layout/theme/settings chrome remains Phase 2b. Phase 2 adds source words and truthful detail copy to the existing list/detail views only.
- Scrollback replay, history files, and `last_cwd` remain Phase 6. The reusable tmux capture primitive landed now for crash tails but no replay behavior was added.
- The `@real-agents` suite is excluded from the default network-free run. Its inability to run without an installed/authenticated CLI is reported, never converted into a skipped/default-green claim.

## Protected source status

This findings task changes only `docs/reports/phase2-findings.md`. It does not modify `SPEC.md`, `prds/`, `ci/Dockerfile`, or `ci/SPIKE.md`.
