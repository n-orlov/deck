# Phase 0 findings

`SPEC.md` remained the source of truth during Phase 0. No protected specification
or toolchain file was edited by this task.

## Phase-scope conflict: fake-agent launch

- **Specification / PRD:** `SPEC.md` requires a TUI-only product, and the Phase 0 PRD
  limits creation to `shell` sessions. The same PRD's fake-agent requirement asks for a
  scenario that launches the fake as a *session command* and observes the argv reaching it.
- **Decision:** The fixture scenario launches the repository-built fake directly with the
  private tmux CLI rather than adding an agent/session-command surface to the released
  TUI.
- **Consequence:** This proves the fake fixture's flag and pane-output contract, but does
  not prove a deck-managed fake-agent launch or its launch audit. That integration belongs
  to the future agent-adapter phase.

## Stepped-clock trigger: chosen Phase 0 interaction

- **Specification:** `SPEC.md` §13.1 says `DECK_CLOCK_STEP` advances a frozen clock “on
  demand,” but does not prescribe the demand mechanism.
- **Decision:** Phase 0 defines one normal released-binary trigger: completing a successful
  shell-session creation advances a frozen clock by exactly one `DECK_CLOCK_STEP`. The
  created row and its launch audit record retain the pre-step time; subsequent rendering and
  lifecycle audit records use the advanced wall time. Failed creation does not advance it.
- **Consequence:** This makes stepped time externally testable without adding a test-only
  key or command. `Clock.Elapsed` continues to use real monotonic elapsed time, independent
  of frozen or stepped wall time. A later phase may choose additional triggers, but must
  document them rather than changing this interaction implicitly.

## Timer controls: released Phase 0 meanings

- **`DECK_RECONCILE_MS`:** controls the list-refresh cadence. Each wake-up runs the
  production reconciliation pass before reloading durable rows, so externally removed
  private tmux sessions become stopped/resumable without recreating tmux.
- **`DECK_PREVIEW_MS`:** controls a separate UI wake-up cadence. Phase 0 intentionally has
  no preview pane; the wake-up is retained as the runtime consumer of the configured preview
  interval rather than silently ignoring it.
- **`DECK_ANIM=0`:** suppresses the optional animation tick. With it disabled, the reconcile
  and preview wake-ups do not manufacture frame changes, allowing byte-stable deterministic
  frames.
- **Decision:** The preview cadence is deliberately observable as a scheduler control, not
  as an out-of-scope preview feature. The two timer values may differ and are not collapsed
  into one interval.

## Extra colour override

- **Specification:** The documented deterministic colour controls are `NO_COLOR` and
  `DECK_ASCII`; `SPEC.md` §13.1 does not define `DECK_COLOR`.
- **Decision:** Phase 0 additionally accepts `DECK_COLOR` as an explicit boolean override
  and lists it in help.
- **Consequence:** It is a harmless, documented-by-the-binary extension, but it is an
  invented environment control rather than a specification requirement. Future work should
  either adopt it in `SPEC.md` or remove it to keep the supported configuration surface exact.

## Fixture-local clean-exit pane retention

- **Specification:** `SPEC.md`'s tmux contract sets `remain-on-exit failed` for
  deck-managed sessions, so tmux itself retains a pane only when the pane's
  command exits non-zero; a cleanly-exited (status 0) pane is torn down
  immediately and its content becomes unobservable once the pane closes. This
  contract governs deck's own product server and is exercised elsewhere; it is
  unrelated to the fixture's private tmux server discussed below.
- **Decision:** Phase 0b's fake-agent fixture scenario runs its own private tmux
  server (started by `launch` in `features/fake_agent_feature_test.go`) and sets
  that server's *global* `remain-on-exit` option to `on`, independently of deck's
  product contract. This retains both clean- and nonzero-exit panes on the
  fixture's server, so the pane the fake agent ran in survives after the process
  exits regardless of its exit status. With the pane retained, the fixture then
  uses the public `capture-pane -p -S -` command (full scrollback, not just the
  visible screen) to read the fake's banner, permission-mode line, and argv echo,
  polling on a short interval up to a deadline because writing and rendering that
  output is not instantaneous. Separately, `list-panes -F
  "#{pane_dead}|#{pane_dead_status}"` polls until the pane is reported dead and
  confirms its exit status is 0, rather than assuming a clean exit from the mere
  absence of a failure capture.
- **Consequence:** The released `remain-on-exit failed` behavior on deck's
  product server is unmodified and still matches `SPEC.md` exactly; the
  `remain-on-exit on` setting exists only on the fixture's own private tmux
  server and is fixture-local test scaffolding. Because the pane is durably
  retained after exit, the fixture's `capture-pane` and `list-panes` reads can
  run at any point after the process finishes, not only while it is still
  alive; the only remaining timing sensitivity is the ordinary delay between a
  command producing output and that output becoming visible to a reader, which
  the polling loops account for.

## Read-before-any-output pane race (discovered during ten-run stability testing)

- **Specification:** `SPEC.md` does not specify a minimum delay between a pane's
  command starting and its first output becoming visible to an external `capture-pane`
  reader; it only defines the retained content once the command has run.
- **Decision:** Phase 0b's fixture scenario does not assume the fake-agent's banner,
  permission-mode line, and argv echo are already written by the time the very next
  test step runs. `outputContains` in `features/fake_agent_feature_test.go` polls
  `capture-pane` on a short interval (25ms) up to a fixed deadline (3s) until all
  expected lines are present, mirroring the pre-existing `waitForPaneDeadStatus` poll
  pattern used later in the same scenario for exit-status observation, rather than
  reading the pane exactly once immediately after launch.
- **Consequence:** This closes a second, distinct race from the original
  `FAKE_CLAUDE_HOLD_MS`-based hold race removed in tasks 001/002: even with the hold
  removed and full-scrollback capture in place, a single unpolled read could still
  run before the fixture process had written anything at all, intermittently failing
  the scenario. The fix is fixture-local test scaffolding, not a change to any
  released binary's timing or output contract.

## No further Phase 0 contradiction found

The remaining Phase 0 work is intentionally a subset of the full-product design; deferred
features (agent adapters, hooks, notifications, and the complete lifecycle) are explicitly
listed as Phase 0 non-goals rather than treated as contradictions.
