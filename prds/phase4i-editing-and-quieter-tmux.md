# Phase 4i — editing in fields and in the preview, an honest `Y`, and quieter tmux

## What the operator gets

Five backlog issues, each filed after the operator raised it:

1. **Real text editing in every field** (#58). One line editor with a caret everywhere deck takes
   typed text: move, edit mid-line, delete a word or to either end, paste at the caret, and copy
   the field's text out.
2. **The pane's cursor is drawn in the interactive preview** (#60), so a prompt can be edited
   mid-line without guessing where the caret is.
3. **The wheel reaches full-screen programs** (#59). Over a pane whose program tracks the mouse,
   a wheel notch goes to the program, as it does under tmux's `mouse on`. Today it scrolls an
   empty grid scrollback and does nothing.
4. **`Y` is offered only when there is something to acknowledge** (#53).
5. **deck's steady tmux cost no longer grows with the number of sessions** (#49). Each recurring
   tick costs a constant number of `tmux` processes.

Each issue body is the detailed design and the evidence. **Read the issue before starting its
requirement.** Where an issue and this PRD disagree, this PRD wins. Where either disagrees with
`SPEC.md`, SPEC wins, and the disagreement is a finding.

## Why now

Phase 4h made the Actions board green (#57, closed). The backlog left is feature work and #49.
**CI must stay green through this phase**: a red or rerun-rescued run is a defect, not a
residual.

**`SPEC.md` has already been amended for this phase**, in an operator commit that lands
*before* this PRD:

- §2: every recurring tmux read costs a constant number of `tmux` processes;
- §7: the reconcile is one `list-panes -a -F`; `Y` on an acknowledged row does nothing;
- §11.3: `Y` joins the absent-when-ineligible examples;
- §11.4 and §11.5: on a text field, `←`/`→` move the caret;
- §11.7: the offered `cwd`, and completion only at the end of the field;
- §11.8 and §11.9: the wheel is forwarded to a program that tracks the mouse, and the pane's
  cursor is drawn;
- §11.10: the filter query is a text field;
- **the new §11.11, Text fields**, the authoritative statement of R178-R181.

Nothing here is blocked on the operator.

## Ground rules

- **These paths are read-only to this job:** `SPEC.md`, `prds/`, `ci/Dockerfile`, `ci/SPIKE.md`,
  `.github/workflows/release.yml` and `ci/releasegate/`.

  ```sh
  BASE=$(git log --format=%H -1 -- prds/phase4i-editing-and-quieter-tmux.md)   # the operator's PRD commit
  git log --oneline "$BASE..HEAD" -- SPEC.md prds/ ci/Dockerfile ci/SPIKE.md \
    .github/workflows/release.yml ci/releasegate/                              # must print nothing
  ```

  The SPEC amendment lands before `$BASE`, so it is outside the audit range by construction. It
  is not this run's commit, and it must not be re-made, extended or "cured".
- **No schema change, no migration.** None of this phase needs a column.
- **No tmux control mode.** §2 keeps "No control mode in v1". #49's option 1 is out of scope.
  This phase takes its options 2 and 3: batching and dropping redundant reads.
- **This is partly a defect phase, so a test that passes against today's code has proven
  nothing.**
  - Each requirement below names assertions that must **fail on the pre-phase tree
    `eb4a963058` and pass after**.
  - Run the new tests against an export of that tree. The evidence (the command and the failure
    it produced) goes to the run's artifacts, never into the repo.
- **Never touch the operator's live state.** This covers:
  - `~/.local/share/deck/`, `~/.config/deck/` and `~/.local/state/deck/`. The job never opens,
    reads, copies or creates anything there;
  - the live `tmux -L deck` server, and any `tmux -L deck-*` server. Never touch them, not even
    read-only. The #49 measurements run on a private socket.

  Every test uses its own temp `HOME`/`XDG_*`/`DECK_HOME` and a private socket.
- **Flakes are defects, and CI stays green.** Every new `features/` scenario waits on durable
  facts, never on a transient frame or an unsettled count. Phase 4h's rules apply unchanged. None
  of these is a fix, and each is a finding:
  - adding or extending retries;
  - adding a sleep;
  - skipping, tagging-out or deleting a scenario;
  - narrowing an assertion until it no longer checks its requirement;
  - widening a deadline without a measurement.

  If this phase's change makes an existing scenario flaky, fix the cause.
- **Docker is for `ci/run.sh` and `ci/stability.sh`, and nothing else.**
  - Never remove or kill containers by label: a previous ralphd job on this host SIGKILLed itself
    by sweeping `label=ralphd.run`.
  - No `docker prune`, and no wildcard `rm`/`rmi`.
  - Never signal by pattern: resolve a pid, verify it, then signal that pid.
  - Other runs and the two `deck-ws-*` self-hosted runners live on this host. Never stop, restart,
    reconfigure or re-register a runner.
- **Code references are pointers, not criteria.** The `file:line` references below were correct
  at `eb4a963058`. If one has drifted, find the current site by function name and carry on.
  Drift is never a finding.
- **No paperwork.** Do not add report, findings, audit, close-out or "retake" files to the repo,
  and do not add a `docs/DELIVERY-LOG.md` row.
  - Run evidence goes to `/run/ralphd/artifacts`. That covers unfixed-tree failures, the #49
    measurements, sweep logs and CI run ids.
  - Do not close GitHub issues: the operator closes them.
- **The CI container has no agent binaries.** Every scenario runs against shell sessions or the
  `cmd/fake-*` stubs on a fixture `PATH`. A fake may gain a fixture or a mode that emits the
  terminal modes R182/R183 need (alternate screen, mouse reporting, DECTCEM off).
- **GitHub scope.** The git credentials are the operator's PAT. The job may:
  - push to `main`;
  - read the Actions API, including run logs and artifacts;
  - start `ci.yml` by `workflow_dispatch` on `main`.

  Nothing else:
  - no other branches, no PRs, no tags, no releases, and no direct push to `gh-pages`;
  - no cancelling, re-running or deleting a workflow run;
  - no change to repository settings, environments, Pages, runners or secrets.

## Scope

- **R176:** `Y` only where there is something to acknowledge (#53).
- **R177-R181:** the shared line editor (#58).
- **R182:** the pane's cursor in the interactive preview (#60).
- **R183:** the wheel forwarded to a program that tracks the mouse (#59).
- **R184-R185:** a constant tmux cost per tick (#49).
- **R186:** the spec and the code agree.

All of them are acceptance.

## Materiality rubric

Reject only on substance:

- a requirement's behaviour is absent or wrong in the live code;
- a test the requirement names:
  - is missing or red;
  - asserts the opposite of the requirement;
  - is named as failing on `eb4a963058` but passes there;
- **any proof run in §Definition of done is red, or has a non-empty flaky record** (any scenario
  or Go test that needed its rerun);
- a fix from §Ground rules' "not fixes" list;
- the spec contradicts the live code;
- a secret is exposed, in the tree, a workflow log or a published CI report;
- a guard in §Ground rules is broken:
  - a schema change, or a tmux control-mode client;
  - any access to the operator's deck directories or any `tmux -L deck`/`deck-*` server;
  - a protected path modified (the audit command prints anything);
  - a branch, PR, tag or release created, or `gh-pages` pushed by hand;
  - a workflow run cancelled, re-run or deleted by the job;
  - a repository setting, environment, runner or secret changed.

Everything else verifies, with the gap recorded as a residual note. That covers wording, form,
provenance and process, and **any number or claim in prose outside the spec** (commit messages,
notes, run artifacts, `docs/`). Prose outside the spec is never a blocking ground. A measured
CPU figure is evidence, never a criterion: R184-R185 are judged by their spawn counts.

There is **no advisory list.** One exception: a run is not judged when it never reached the
`suite` step because GitHub failed to provision a runner, or because of an Actions API 5xx. Such
a run neither counts toward the proof nor resets it. Record it and dispatch another. A red
`suite` step always counts.

## Escape hatch

If the run proves that a requirement contradicts `SPEC.md`, or is impossible as written, it must
not edit the spec or quietly narrow the requirement. Possible causes include a key the terminal
cannot deliver, or a vt-library limitation with no workaround. File a petition with:

- the evidence;
- the smallest change that would resolve it.

Then notify the operator and carry on with everything the petition does not block. The operator
rules by amendment.

## Notifications

The operator is AFK on Telegram, so use the `notify` hat tool in the same iteration as the work.

Send one message for each of:

- anything that blocks work outright, including a petition;
- each requirement as it lands;
- every rejection and cure pass;
- each proof run's result, and whether it reset the count;
- one terminal summary.

Keep each message to a few hundred characters: the notifier refuses a body over 16000 characters
outright. A missed or late send is cured by mentioning it in the next one. It is never a success
criterion and never a finding.

## R176 — `Y` only where there is something to acknowledge (GH #53)

- **Today:** `canAcknowledge` (`internal/tui/tui.go:2433`) returns `true` unconditionally. The
  footer's fixed set therefore offers `Y acknowledge` on every session row
  (`footerRowEligible(m, false, canAcknowledge)`, `tui.go:5853`).
- **After:**
  - `canAcknowledge(session)` is true exactly when `!session.Acknowledged`.
  - Both the footer and the `Y` handler (`tui.go:~4119`) use that one predicate.
  - `Y` on an acknowledged row is a silent no-op: no `acknowledge` call, no store write, no
    event and no message.
  - `Y` stays bound, in the `?` overlay and in §11's keymap.
- **Success, as TUI unit tests:**
  - The footer omits `Y` on an acknowledged row and on a group header.
  - The footer shows `Y` on an unacknowledged `waiting` row and on an unacknowledged `error`
    row.
  - After the row is acknowledged (by attach or by `Y`), the footer no longer shows `Y` for it.
  - `Y` on an acknowledged row makes no `acknowledge` call.
  - `footer_bindings_parity_test.go` stays green.
  - The omission assertions fail on `eb4a963058`.

## R177 — one line editor

- **A new package** (e.g. `internal/tui/lineedit`, or a type inside `internal/tui`) implements
  §11.11's editor as a pure value: text, caret and the offered flag, plus a horizontal scroll
  offset when rendered. It has:
  - key handling for every row of §11.11's table;
  - paste insertion, with control characters dropped;
  - grapheme-cluster stepping and cell widths. `github.com/rivo/uniseg` is already in the
    module graph and may become a direct dependency;
  - a render to a given cell width, with the reverse-video caret, horizontal scrolling that
    keeps the caret visible, and `…`/`...` marks on the clipped sides;
  - §11.11's offered-value rule;
  - a `Value()` for the field's owner.
- **Word rules:**
  - `alt+b`/`alt+f`/`ctrl+←`/`ctrl+→`/`alt+backspace` treat a run of letters and digits as a
    word;
  - `ctrl+w` deletes to whitespace.
- **Success, as table-driven unit tests:**
  - every key at the start, middle and end of the text, including on empty text;
  - word motion and word deletion over a path (`/home/me/proj-a`) and over `foo bar-baz`;
  - insertion and paste in the middle, with a paste containing `\n`, `\r` and `\t`;
  - a combining sequence, a ZWJ emoji and a wide CJK character: each steps as one character,
    is measured in cells and is never split by a scroll edge;
  - scrolling: a 200-character value in a 20-cell field keeps the caret visible after `home`,
    `end` and word jumps, with the clip marks correct on each side;
  - the offered rule: a printable key replaces, a paste replaces, `←` accepts and moves,
    `backspace` accepts and deletes one character;
  - the render under `NO_COLOR` still carries SGR 7 at the caret.

## R178 — every text field uses it

- **Each of the 17 buffers in these handlers moves to the shared editor:**
  - `updateRenameDialog` (`rename.go`);
  - `updateFilter` (`filter.go`);
  - `updateLaunchInputsDialog` (`launch_inputs.go`): its three fields;
  - `updateEnvDialog` (`env_editor.go`);
  - `updateCreate` (`tui.go`): name, cwd, launch args, env, pre-launch, post-destroy;
  - `updateSettingsGroupEditing`, `updateSettingsEnvEditing` (key and value),
    `updateSettingsStringEditing` and `updateSettingsSearch` (`settings.go`).

  Find any field missed here, too.
- **When this is done**, no `case "backspace", "ctrl+h"` and no `+= string(msg.Runes)` remain in
  a text handler, and the byte-trimming `backspaceCreateField`/`backspaceLaunchInputsField` are
  gone. `settingsTextCursor` and every `"_"` caret stand-in go as well.
- **Key routing (§11.4, §11.5):**
  - On a focused text field, `←`/`→`/`space` are the editor's. The dialog contract's `Cycle`
    applies to selection fields only. Today the create modal and launch-inputs set `Cycle`, so
    `left`/`right` are swallowed even on text fields (`dialog_contract.go:94-104`).
  - In settings, `tab`/`←`/`→` switch lists only while no value is being typed.
  - `↑`/`↓`/`↵`/`esc`/`tab` keep their contract meanings.
- **Offered values (§11.11)** are the cwd prefill and the opening value of rename, every
  settings edit, the env editor and the launch-inputs fields. Each follows the one rule. Some of
  these append to their prefill today, so this is a deliberate behaviour change for them.
- **Success:**
  - Each handler's unit tests are updated to the new contract. A test that pinned append-only
    behaviour is rewritten to the new behaviour, not deleted.
  - A test fails if any text handler still has its own backspace or append case. A source scan
    is acceptable.
  - `features/` scenarios on real tmux:
    - In rename, move into the middle of a name with `←`, insert, then `↵`: the renamed session
      carries the edited name, read from the store.
    - In the `/` filter, edit the middle of the query: the list re-narrows to the edited query.
    - In the create modal, `←`/`→` on the name field move the caret and leave the agent
      selection unchanged.
  - Each assertion above that concerns new behaviour fails on `eb4a963058`.

## R179 — the `cwd` field under a caret (§11.7)

- Ghost and `tab` complete only with the caret at the end of the field.
  - With the caret anywhere else, there is no ghost and `tab` does nothing.
  - `→`/`end` accept a ghost only when one is showing. Otherwise they are caret keys.
- `ctrl+p`/`ctrl+n` replace the whole text and put the caret at its end. Cycling back past the
  newest entry restores the snapshot, caret included.
- The prefill is offered. `←` into `/home/me/proj-a` followed by `b` in place of `a` (via
  `backspace` then `b`) yields `/home/me/proj-b`, not `b`.
- Update `create_cwd_ghost.go`'s end-of-field comment and `features/create_cwd_ghost.feature`'s
  description to the new contract.
- **Success:**
  - unit tests for each rule above;
  - `features/` scenarios:
    - edit an offered `cwd` mid-path, then create: the session's `cwd` in the store is the
      edited path;
    - with the caret mid-field, no ghost is painted, and `tab` changes nothing;
    - back at the end, the ghost returns and `→` accepts it.
  - The mid-path edit and the mid-field ghost suppression fail on `eb4a963058`.

## R180 — copy a field's text

- `alt+w` copies the focused field's whole text through `SetSelectionBuffer`
  (`internal/tmux/buffer.go`) and `writeOSCClipboardBestEffort`
  (`internal/tui/interactive_select.go`), with the same on-screen confirmation drag-to-copy
  gives.
- A masked secret copies only while revealed. While masked, `alt+w` copies nothing and says so.
- **Success:**
  - unit tests for the copy, the masked case and the confirmation;
  - a `features/` scenario on real tmux: type into the create modal's name field, press
    `alt+w`, and `show-buffer -b deck-selection` on the private socket holds that text. This
    fails on `eb4a963058`.

## R181 — the caret is visible, and nothing else pretends to be one

- Only the focused field draws the reverse-video caret (§11.11). There is no `_` stand-in.
- An offered value is drawn as a selection.
- **Success:**
  - render tests for one field per handler: the caret cell is SGR 7 at the right column, under
    colour and under `NO_COLOR`;
  - an unfocused field has no caret;
  - an offered value carries the `selection` background;
  - a `features/` scenario uses the `cell at row R column C is reverse` step to see the caret
    move under `←`;
  - each fails on `eb4a963058`.

## R182 — the pane's cursor in the interactive preview (GH #60)

- **Today:** the vt grid knows the cursor (`SafeEmulator.CursorPosition()`), and the seed replays
  `cursor_x`/`cursor_y`/`cursor_flag` (`internal/interactive/seed.go`). Nothing draws it.
  - The vt library has no public visibility getter. Visibility arrives through the
    `Callbacks.CursorVisibility` callback.
  - Grids are rebuilt on every reseed, so the callback must be registered before the seed is
    written (`newGrid`/`newDrainedGrid`).
- **After (§11.9):**
  - Interactive mode draws the grid's cursor cell in reverse video.
  - It is composed under the same lock and frame as `RenderRows`. The stale-frame path must
    never pair a fresh cursor with cached rows.
  - It is not drawn while the cursor is hidden, or while the grid is scrolled back. It returns
    at live.
  - A reverse-video cell the program painted passes through under every `[ui] preview_paint`
    mode.
  - The passive preview is unchanged.
- **Success, as `features/` scenarios on real tmux:**
  - In an interactive preview of a shell, type `abc` and press `←` twice: the cell holding `b`
    is reverse, and the cells holding `a` and `c` are not.
  - A program that hides its cursor (`printf '\e[?25l'` then wait) shows no reverse cell at the
    cursor position. After `\e[?25h`, the cursor shows again.
  - Scrolled back, no cursor is drawn. Back at live, it is drawn.
  - A program-painted reverse cell survives under `preview_paint = fit`, `nofit`, `bg` and `off`.
  - The first three fail on `eb4a963058`.
  - A unit test pins that the cursor and the rows come from one locked snapshot.

## R183 — the wheel reaches a program that tracks the mouse (GH #59)

- **Today:** the interactive wheel over the preview always calls `scrollInteractiveByLines`
  (`internal/tui/tui.go:~4693`).
  - The grid receives mouse modes 1000/1002/1003/1005/1006 from the seed (`seed.go:159-163`) and
    from the live stream, but exposes none of them.
  - `SendMouse` is no use for forwarding, because its reply pipe is drained and discarded
    (`grid.go:140`).
- **After (§11.8, §11.9):**
  - The grid tracks the pane program's mouse-reporting state. Use the vt `EnableMode`/
    `DisableMode` callbacks, registered on every new grid before its seed. Do not add a tmux
    read per notch.
  - Over the preview, while interactive, a notch is forwarded when all of these hold:
    - reporting is on (1000, 1002 or 1003);
    - the grid is at live (offset 0);
    - `Shift` is not held.
  - Otherwise the notch scrolls the grid as today.
  - Forwarding encodes the notch at the pane cell under the pointer, using `previewCellAt`:
    - SGR `CSI < 64|65 ; col+1 ; row+1 M` under 1006;
    - X10 `CSI M` otherwise, and only for a column and row ≤ 223;
    - a notch past 223 in X10, or under 1005/1015, is dropped.
  - The bytes go through the interactive dispatcher's verified-identity `send-keys -H` path.
  - A forwarded notch ends a sidebar drift, as a forwarded key does.
- **Success:**
  - unit tests:
    - the routing table: reporting × offset × `Shift`;
    - both encodings, byte-exact, at several cells including column 223 and 224;
    - the drop cases;
    - the mode tracking surviving a reseed.
  - `features/` scenarios on real tmux, with a shell running a tiny program that turns
    reporting on (`printf '\e[?1000h\e[?1006h'`) and shows its input (`cat -v`, or a fake mode):
    - a notch over the preview reaches the program as the exact SGR report for that cell, and
      the grid's offset stays 0;
    - with reporting turned off (`\e[?1000l`), the next notch scrolls the grid instead;
    - `Shift`+wheel scrolls the grid while reporting is on;
    - over an inline shell with history, the wheel scrolls the grid exactly as today. The
      existing `interactive_scroll.feature` wheel scenario stays green, unmodified.
  - The forwarding assertions fail on `eb4a963058`.

## R184 — reconcile is one `tmux` process per tick (GH #49, §2, §7)

- **Today:** `List` runs `list-sessions` and then one `list-panes -t` per deck session
  (`internal/tmux/tmux.go:366, 382, 724`), so each 500 ms reconcile spawns 1 + N processes.
  `_hook`'s liveness pass does the same.
- **After:**
  - `List` is one `list-panes -a -F` carrying `#{session_name}` and today's pane fields, grouped
    by session in Go. Only `deck_` sessions are kept, as today.
  - "No server" and an empty server read as "no sessions", exactly as now.
  - Every `List` caller keeps its current result shape: reconcile, `_hook`, `PreviewPane` and
    any other caller.
  - §7's status probe is unchanged.
- **Success:**
  - A unit test, through a counting `tmux` shim on a private socket: one reconcile pass over 12
    live shell sessions, with no probe due, spawns **exactly one** `tmux` process. The same
    holds for 1 session and for 0. This fails on `eb4a963058`.
  - `List`'s result for a fixture server (live, dead-with-status and dead-by-signal panes, plus
    a non-deck session) is identical before and after. Take today's output as the oracle.
  - The existing reconcile, crash-capture and status-recovery tests stay green, unmodified.

## R185 — the preview tick and interactive mode are constant-cost too (GH #49, §2)

- **Today:**
  - Each 250 ms preview tick does a full `List` before its `capture-pane`, which is 1 + N + 1
    processes (`CapturePreview`, `tmux.go:542`). It keeps doing so in interactive mode, where
    the grid is what is shown.
  - In interactive mode there are also two more:
    - the displacement backstop: an `@deck_isize_owner` read plus `#{session_attached}`, 2 per
      tick (`interactive_displacement.go`);
    - `pollPaneDead`: a `#{pane_dead}` read every 200 ms (`internal/interactive/grid.go`).
- **After:**
  - A passive preview tick is **one** process. Capture the selected session's pane by its
    target directly. If the target has vanished, the error is the same transient one as today
    (§2), and the next reconcile tick corrects the selection.
  - **No passive capture runs while interactive.**
  - In interactive mode, the displacement backstop and the dead-pane poll are **one**
    `display-message` per preview tick, reading `#{pane_dead}`, `#{session_attached}` and the
    window's `#{@deck_isize_owner}` together. The separate 200 ms `pollPaneDead` loop is gone,
    or folded into it.
  - Detection latency for a dead pane and for displacement is unchanged. Both are one preview
    tick or better.
  - The capture transport's own 200 ms seed loop is the transport, not a backstop, and stays.
- **Success:**
  - Unit tests through the counting shim:
    - one passive preview tick over 12 sessions spawns exactly one process;
    - one interactive preview tick (pipe transport) spawns exactly one process and no
      `capture-pane`.
    - Both fail on `eb4a963058`.
  - A `features/` scenario on a private socket with 12 shell sessions counts spawns through the
    tmux shim over a 10 s window. The window opens once the sessions are created, and once any
    interactive entry has finished, so one-off reads are outside it:
    - list mode spawns at most 75 processes;
    - interactive mode on a shell (pipe transport) spawns at most 75 processes.

    Both are upper bounds, so load can only lower the count. Both fail on `eb4a963058`.
  - Every displacement, pane-dead, entry-refusal and force-attach scenario stays green,
    unmodified.
  - **Measurement, as evidence only:** deck and tmux-server CPU and spawns per second, before
    and after. Use 12 shell sessions on a private socket, in list mode and in interactive mode
    on a busy pane (`top -d 0.25`). The figures go to artifacts and to the terminal
    notification, and never into the repo.

## R186 — the spec and the code agree

- When R176-R185 are done, every amended passage must describe the live code:
  - §2's constant-cost rule;
  - §7's reconcile and `Y` rule;
  - §11.3;
  - §11.4 and §11.5's caret keys;
  - §11.7;
  - §11.8 and §11.9's wheel and cursor;
  - §11.10;
  - §11.11.
- The `?` help overlay and each field's help line name the editor keys and `alt+w` that §11.11
  names, and the wheel-forwarding and `Shift`+wheel behaviour.
- A contradiction that the run cannot resolve in code goes through the §Escape hatch. It is never
  an edit to `SPEC.md`.
- **Success:** the requirement tests above cover each amended passage's behaviour. Where help
  text is itself the behaviour, a render test asserts it.

## Ordering

1. **R176**: small and self-contained.
2. **R184, then R185.** They are independent of the UI work. Land them early, so every later CI
   run measures the quieter deck.
3. **R177**, then R178, R179, R180 and R181. R178 is the largest: migrate one handler per commit.
4. **R182, then R183.** They share the grid callback registration.
5. **R186**, checked last, against the finished code.
6. **The proof (§Definition of done), as the last task, with nothing committed after it.** If a
   proof run is red or flaky, that is a real defect:
   - fix it at its root, with a regression test that fails before the fix;
   - push, and restart the proof from zero.

   That is the only reason to commit after the proof starts.

## Definition of done

- R176-R186's behaviours are in the live code, and each named test exists and is green. Each
  test named as failing on `eb4a963058` does so, with the evidence in artifacts.
- Every new or changed `features/` scenario passes 20 consecutive solo runs under `-race` before
  the proof starts. The logs go to artifacts.
- **At the final pushed sha:**
  - the `push` run of `ci` concluded `success`, and its flaky record is empty;
  - **three consecutive `workflow_dispatch` runs of `ci` concluded `success`, and each one's
    flaky record is empty.** That is the nightly path: `-race`, `ci/stability.sh 3`, report and
    publish. A scheduled nightly at the final sha counts as one of the three.
    - "Flaky record is empty" means `ci/suite.sh` reran no features scenario and no Go test
      (`flaky-features.txt` and `flaky-go.txt` are empty), and the stability loop had no
      failure.
    - Dispatch them **one at a time**. Each one starts after the previous one has finished.
    - None runs while the job's own local `ci/stability.sh` or other heavy local work is
      running.
    - A red or flaky run resets the count to zero, with the exception in §Materiality rubric.
  - `ci/run.sh go test -p=1 -count=1 ./...` is green locally: the whole suite, with no narrowed
    package list and no `-run` filter;
  - a local `ci/stability.sh 5` is green with no failure in any run, and its logs are in
    artifacts. Run it **before** the dispatches, not alongside them;
  - `go build ./...`, `go vet ./...` and `gofmt -l` are clean on every file this run touched;
  - the protected-path audit command prints nothing.
- No secret is exposed.

Nothing else is required. In particular, no report, record or log row goes in the repo.

## Non-goals

- **tmux control mode** (#49 option 1), and any change to the tick intervals
  (`DECK_RECONCILE_MS`/`DECK_PREVIEW_MS`) or to §7's probe cadence.
- **Multi-line editing, undo, a kill ring or `ctrl+y`, and partial selection inside a field.**
- **A cursor in the passive preview.**
- **Mouse reporting other than the wheel:** clicks and drags are not forwarded to the pane. A
  click over the interactive preview stays the drag-to-copy press (§11.8).
- **`Y` over the marked set.**
- The quality gates in #54, Allure work (#55), and any CI workflow, runner or release-gate
  change.
- **Any schema change.**
- **Publishing a release.**

## For the planner

- **R178 is the bulk, and the risk is a regression in a dialog you did not mean to touch.**
  Migrate one handler per commit, and run that handler's unit and feature tests before the
  next. The dialog contract change (`Cycle` only on selection fields) touches every dialog:
  `dialog_contract_test.go` is the guard.
- **The offered-value rule changes four fields' behaviour**: settings group rename, settings
  string, settings env and launch-inputs append to their prefill today. Those tests change on
  purpose.
- **R182 and R183 share one trap:** the grid is rebuilt on every reseed and resize. Anything
  registered on it (the cursor-visibility and mode callbacks) must be registered on every new
  grid before its seed is written, or it silently goes deaf after the first resize. Test across
  a resize.
- **R184's trap is result shape.** `List` feeds reconcile's crash and liveness rules. Pin today's
  output as an oracle **before** changing it.
- **R185's trap is detection latency.** The folded read must still notice a dead pane and a
  displacement within one preview tick. The existing displacement and pane-dead scenarios are
  the guard. Do not slow them to make a count fit.
- **The counting shim:** `ci/tmux-guard.sh` already wraps tmux in CI. The tests'
  `countingTmuxShim` (`internal/tmux/paneseed_history_test.go`) and `newFakeTMuxClient` are
  existing patterns to reuse.
- **Runner time is the scarce resource.** There are two shared `deck-ws-*` slots. A push run takes
  about 12 minutes, and a dispatch run about 50. Batch commits, and get local evidence first.
  Poll the Actions API rather than sleeping in a loop.
- **Budget for one cure pass.** That is the harness working.
