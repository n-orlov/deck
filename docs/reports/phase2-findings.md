# Phase 2 design findings

This report records Phase 2 decisions, disagreements, upstream assumptions, and deferrals for later operator review against `SPEC.md`. `SPEC.md` and `prds/` were not changed to make the implementation fit.

## Final verification and review-gap disposition

The clean fixed-commit default capture at `d562d6fbe53b9f1db296c1f54d8ab1b8bce15baf`
is [`phase2-full-verbose-run.log`](phase2-full-verbose-run.log): **21 features,
48 scenarios, and 546 steps passed**, with **187 top-level Go tests** under the
report's stated counting convention. Its wall time was **55.277 seconds**. Build
and vet output for the transition-fixed source are retained in
[`phase2-build.log`](phase2-build.log) and [`phase2-vet.log`](phase2-vet.log).
A clean `ci/stability.sh 10` run at the final production/test checkpoint
`20b19dd5e30fc1af0801229c2df36ff30447290b` passed **10/10** with final exit 0;
its complete output is [`phase2-stability.log`](phase2-stability.log). Changes
after that checkpoint are evidence/report files only.

The four review gaps have these dispositions:

- Error attachment is proved through the released binary by
  `status_attach.feature: attach acknowledges a live error without replacing
  its verdict`; the error remains intact while acknowledgement clears durably.
- Non-leasable resume messaging is proved by `launch_lease.feature: a stale
  stopped row reports its new non-leasable verdict instead of a lease
  conflict`; the actual reason is shown and `starting elsewhere` is absent.
- The frozen-clock trigger and its released-binary proof are detailed below.
- Authenticated real-Claude hook delivery and strict R36 payload conformance
  both succeeded: `SessionStart` proves initial injection/delivery and
  `UserPromptSubmit` supplies the full required common payload, as detailed
  below.

## Frozen wall-clock control

- `SPEC.md` §13.1 says `DECK_CLOCK_STEP` advances a frozen clock “on demand”, but it does not name the cross-process trigger. Deck now uses `SIGUSR1` sent to any running client: `kill -USR1 <deck-client-pid>`. There is no TUI key binding, avoiding the earlier conflict with §11's sidebar-width keymap.
- The signal handler calls `Clock.AdvanceShared`. It takes an exclusive lock beside `clock.now` under deck's resolved data root, rereads the current shared instant while locked, adds **exactly** `DECK_CLOCK_STEP`, and atomically publishes the result. Concurrent triggers from different clients therefore serialize without lost increments, and every running client or later `_hook` sharing `DECK_HOME` reads the same result.
- The trigger is enabled only when both `DECK_CLOCK` and `DECK_CLOCK_STEP` configure a step-capable frozen clock. Callers never calculate or write an absolute timestamp. The help overlay documents the signal command and exact per-invocation advance; the released-binary `determinism.feature` scenario proves two already-running clients plus a later hook observe it and reaches probe staleness without a 45-second sleep. This supported surface should be folded into §13.1.
- Wall time controls status timestamps, staleness, and rendered ages only. Hook transaction durations and reconciliation timeouts use Go's monotonic clock and continue advancing while wall time is frozen.
- Phase 2 removed the store's implicit real-time timestamp fallback from status/event APIs it exercises: callers must supply an explicit timestamp. This is an API decision needed to prevent a real-time write from silently poisoning frozen-clock staleness comparisons.

## Claude hook injection mechanism

- Phase 2 chose Claude's inline command-line settings mechanism: each launch/resume appends `--settings <JSON>`. The JSON contains command hooks for `SessionStart`, `UserPromptSubmit`, `Notification`, `Stop`, `StopFailure`, and `SessionEnd`; every command invokes the shell-quoted absolute running deck executable with `_hook`.
- Inline JSON was preferred because `Adapter.Instrument` stays a pure function, there is no settings artifact to synchronize or clean up, and deck writes neither user/project settings nor anything under the session cwd. Deck-owned `DECK_SESSION_ID` and `DECK_HOME` are merged into the pane environment at launch but are not persisted or displayed as user session environment.
- The implementation relies on Claude's documented settings-source merge behavior so deck's hooks are additive to user/project hooks. It does not read or normalize those other settings.
- The command hook currently supplies the upstream `type` and `command` fields and relies on Claude's hook timeout default. The spec's phrase “declares a modest” session-end timeout does not choose a value or pin the current upstream timeout-field shape; that remains an upstream contract to confirm before adding another field.
- If a future real-Claude conformance run rejects inline JSON, the fallback is a deck-owned settings file under the resolved data root, never under the session cwd or user settings. No such fallback was implemented speculatively.

## Upstream Claude contract: authenticated conformance and SessionStart finding

The default suite deliberately proves deck's declared contract with `fake-claude`; upstream conformance remains in the opt-in suite:

```sh
DECK_GODOG_TAGS=@real-agents go test -run TestFeatures -v ./features/...
```

That command exited 0 against installed, authenticated genuine **Claude Code
2.1.237**. Claude accepted deck's inline `--settings` instrumentation. Its
initial **`SessionStart`** reached the released `deck _hook` and promoted the
created row from `starting` to `live running`, independently proving injection
and delivery against genuine Claude rather than a mock. After the scenario sent
an actual prompt, genuine Claude emitted **`UserPromptSubmit`**; that event is
the passing R36 payload evidence. Its unedited upstream JSON contains non-empty
string `session_id`, `cwd`, `transcript_path`, and `permission_mode` (`default`),
and the released `_hook` persisted it before the strict assertion passed. The
scenario neither creates aliases nor synthesizes or coerces `permission_mode`.
All three opt-in scenarios and all 27 steps passed.

The earlier `SessionStart` omission remains a relevant upstream finding and
does not affect the passing R36 evidence. In 2.1.237 its observed key/type set was `cwd:string`,
`hook_event_name:string`, `model:string`, `session_id:string`, `source:string`,
and `transcript_path:string`; `permission_mode` was absent. The same omission
was observed for `SessionStart` in genuine Claude versions 1.0.128, 2.0.77,
2.1.0, 2.1.50, 2.1.100, 2.1.150, and 2.1.200. R36 does not require that the
full common payload come specifically from `SessionStart`, so the conforming
genuine `UserPromptSubmit` event resolves the requirement while preserving the
SessionStart behavior as diagnosed evidence.

Retained repository-relative, unedited evidence is:

- [`phase2-real-claude-authenticated.log`](phase2-real-claude-authenticated.log)
  — SHA256 `53f6e2f384fbac5ea5a81a4a3e95b070afe31220f3f44b40cd86e40cf0fea2a8`
  (Claude 2.1.237 version, exact genuine `SessionStart` and
  `UserPromptSubmit` payloads, released-hook steps, 3/3 scenarios and 27/27
  steps passing, and command exit 0).
- [`phase2-real-claude-version-matrix.log`](phase2-real-claude-version-matrix.log)
  — SHA256 `897976a0fc4adf1978e8228fac7cfda4880de1f900df8e6441457eb702e529de`
  (unedited `SessionStart` payloads and required-field checks for versions
  1.0.128 through 2.1.200).

**R36 is met** by the authenticated 2.1.237 `UserPromptSubmit` capture. The
strict `@real-agents` assertion still requires all four fields as non-empty
strings and remains opt-in so the default suite stays network-free.

### Event names and event-specific fields still unconfirmed

- Genuine Claude confirmed `SessionStart`, its `source` field (`startup` in the retained captures), and `UserPromptSubmit` with the full R36 common payload. Real emission of `Notification`, `Stop`, `StopFailure`, and `SessionEnd` remains unconfirmed; the fake agent emits the declared names, and deck does not rename its subscription set from anecdotal observations.
- The wider `SessionStart.source` vocabulary is unconfirmed. The mapping preserves the supplied string as `status_reason`; tests cover the expected fresh/resumed/compacted meanings without normalizing variants such as `startup`, `resume`, or `compact` in handler code.
- `Notification.notification_type` and the expected permission-prompt, question, needs-input, and idle-prompt values are unconfirmed. Values are preserved verbatim as `status_reason`, including unknown future strings.
- `Stop.last_assistant_message`, `StopFailure.error_type`, and `SessionEnd.reason` are unconfirmed as names, presence rules, and types. The declared string field is consumed when present; raw JSON is retained in the event so drift is diagnosable.
- Real emission and field coverage for the remaining four events should be added to `@real-agents` when stable, non-destructive ways to provoke each event are known. Probe fallback remains the deliberate degradation path in the meantime.

## Hook mapping and persistence decisions

### Exhaustive transition-policy review resolution

Review found that the original §8.1 mapping treated every resolved hook as an
unconditional target-status assignment. Store precedence protected a row killed
by the user, but there was no exhaustive current-state check: for example, a
late `SessionStart` could change a clean `stopped` row back to `running`. A
similar stale running hook could replace the terminal `error` verdict and crash
metadata of a dead pane. This contradicted §7's statement that its transition
table is exhaustive.

The resolution makes legal source states declarative beside each hook mapping
in [`internal/hookrecv/receiver.go`](../../internal/hookrecv/receiver.go):
`SessionStart` accepts only `starting`; `UserPromptSubmit` accepts `idle` or
recoverable `error`; `Notification`, `Stop`, and `StopFailure` accept only
`running`; and `SessionEnd` accepts every state and moves it to `stopped`. The
receiver passes that policy into
[`Store.UpdateSessionStatus`](../../internal/store/store.go), which reads the
current state, decides whether the transition is legal, updates accepted status
metadata, and inserts the event in one database transaction. It also rejects a
hook-to-running update when `pane_exit_status` marks the current error as a
process-crash terminal row. Thus a rejected hook leaves status, reason, source,
timestamp, acknowledgement, epoch, last message, and crash evidence unchanged,
but its original payload is still committed to the session's event audit trail.
Retention is intentional: it proves the stale event arrived and keeps upstream
drift diagnosable without letting the event resurrect a terminal session.

The ambiguity was the apparent tension between §8.1's event-to-status prose and
§7's exhaustive edges, plus §7's general recoverable `error → running` edge
versus an `error` row carrying terminal pane-death evidence. Phase 2 treats §8.1
as a mapping constrained by §7, permits `UserPromptSubmit` to recover an
agent/API error, and treats `pane_exit_status` as the discriminator that makes a
process-crash error non-recoverable by hooks. It does not change `SPEC.md`.

Evidence is layered:

- [`internal/hookrecv/receiver_test.go`](../../internal/hookrecv/receiver_test.go)
  exhaustively crosses all six hook events with all six current states, checks
  accepted metadata, and proves every rejected payload is still audited; its
  terminal regressions cover both clean stop and process crash.
- [`internal/store/store_test.go`](../../internal/store/store_test.go) proves the
  allow-list check and event insertion share the transaction and that a
  pane-exit error cannot be revived.
- [`features/status_claude_hooks.feature`](../../features/status_claude_hooks.feature)
  fires the released pane-instrumented `_hook` path, preserves the legal return
  edges, and proves stale hooks cannot revive a clean stop, user-terminal row,
  or process-crash row while their payloads remain queryable.
- The passing cases and exact unit-test names are retained in the clean
  [`phase2-full-verbose-run.log`](phase2-full-verbose-run.log).

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
- The `@real-agents` suite remains excluded from the default network-free run. It was separately executed with installed/authenticated genuine Claude Code 2.1.237; the successful `SessionStart` delivery, conforming `UserPromptSubmit` payload, and passing strict assertion are retained as separate opt-in evidence.

## Protected source status

The report update changes only files under `docs/reports/`. It does not modify `SPEC.md`, `prds/`, `ci/Dockerfile`, or `ci/SPIKE.md`.
