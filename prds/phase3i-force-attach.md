# Phase 3i — force-attach: stealing the interactive preview

## Goal

GH issue #19. One operator working one set of sessions from more than one place is the
ordinary case, not an anomaly. Today the deck client that does *not* hold a session's
interactive preview has no way to move the keyboard to it: `↵` refuses ("another client is
attached to this session" / "a live process holds ownership of this window") and offers only
`a`'s full attach. The operator has to find the other client and press `Ctrl+Q` there.

This phase adds `F` — force-attach — and the concurrency and honesty that make it safe: the
claim is decided so exactly one client wins, the window's *original* geometry survives an
arbitrary chain of steals, and the client that loses the pane is told so in a dialog that
stops its keyboard rather than typing into somewhere the user cannot see.

It is a small phase, five product requirements and one documentation one. It is not an
invitation to revisit interactive mode. Do not grow it.

## Read this before anything else

- **`SPEC.md` is already correct and is the authority.** The operator amended it for this
  phase in commit `6197b53` (`spec: F force-attaches the interactive preview over any holder
  (operator)`): §11.9 gained four bullets (`F` forces entry; exactly one client wins; the
  original geometry outlives every steal; a displaced client is told and its keyboard is
  stopped), §7's "cleared by attaching" now covers a forced entry and states that being
  displaced records nothing, §11's passive fit now stands down under a foreign live claim, and
  §11.3's keymap plus §11.4's dialog inventory gained their entries. Read all of that before
  planning. **Where this PRD and `SPEC.md` disagree, `SPEC.md` wins** and the disagreement is
  a finding for `docs/reports/phase3i-findings.md`, never an edit.
- **`F`, not `f`.** `f` is §12's cross-session search and stays that way — the operator ruled
  on this explicitly. Every message, help entry and footer entry says `F`.
- **Protected paths — `SPEC.md`, `prds/`, `ci/Dockerfile`, `ci/SPIKE.md` — are read-only for
  this job**, no exception, and steering cannot license one. The audit range for this run is
  `6197b53..HEAD` and it must show **zero** protected-path commits. Full-history context so
  no report re-litigates it: the only shas that have ever legitimately touched a protected
  path are operator commits (`2eed8de`, `a03527c`, `de90a5c`, `2b0a215`, `6197b53`, and the
  `prds/`-only ones) plus the disclosed pair `b69b5ba`/`2d61993` (phase 3g finding F41, net
  diff empty, already ruled on). None of that is this run's problem.
- **Almost nothing here needs a new mechanism.** Read these four files before writing any
  code; each already contains most of what a requirement below asks for:
  - `internal/tmux/ownership.go` — `ClaimWindowOwnership` is already read → write →
    **confirm-read**, and `Release` already refuses to unset an option that no longer holds
    its own claim. R99 is a variant of the first and needs no change to the second.
  - `internal/tmux/geometry.go` — `WindowGeometry`, `CaptureWindowGeometry`,
    `RestoreWindowGeometry`, `SessionAttachedCount`, `PanePipe`, and `readBuiltinWindowOption`
    (which encodes the distinction R100 depends on: an unset **builtin** window option prints
    an empty line and exits 0, an unset `@`-prefixed **user** option errors with "invalid
    option").
  - `internal/tmux/reclaim.go` — `InteractiveClaimRecord` / `SaveInteractiveClaimRecord` /
    `ReclaimLeakedInteractivePipes`, including the stand-down-for-a-live-owner guard R100
    extends.
  - `internal/interactive/grid.go` — `Session.Status()` and `StatusLive`/`StatusDisplaced`/
    `StatusDisabled`, already derived from an EOF with `#{pane_pipe}` still 1. That is R101's
    fast path; it exists.
- **The feature harness already has the vocabulary these scenarios need.** Do not invent
  steps that duplicate one of these: `deck client "X" is started` / `exits cleanly`,
  `deck client "X" selects session "S"`, `deck client "X" enters interactive mode` /
  `leaves interactive mode`, `a real tmux client attaches to deck session "S" at 80x24`
  (and its detach), `another live process holds ownership of deck session "S"'s window`,
  `the private tmux window for session "S" is captured as "N"` / `still matches "N"`,
  `tmux window "deck_S" option "@deck_isize_owner" is unset in the window scope`, and
  `the state database session "S" is "T" from "SRC" with acknowledged=N, notify_epoch=N, and
  N attached events`. `features/interactive_refusals.feature` and
  `features/status_attach.feature` are the two files to read for the idiom.

## Requirements

### R98 — `F` force-enters the interactive preview, and every refusal that could be forced says so

- Bind `F` in `internal/tui`'s bare-key switch (`internal/tui/tui.go`, beside
  `case "enter"` / `case "a"`) to a force variant of `enterInteractive`. `F` differs from `↵`
  in exactly two ways, both named in §11.9: it does not refuse for
  `SessionAttachedCount > 0`, and it takes ownership over a live holder (R99) instead of
  standing down. **Every other refusal is unchanged and still applies**: a stopped session
  (`canReachPane`), a `tmux.SessionName` failure, `width <= 0`, `height <
  interactiveMinInnerRows`, no live pane, and every fallible tmux step after the claim.
- The two contention refusals `↵` can produce must name `F` as well as `a` — the attached-client
  refusal and the live-ownership refusal. The 7-row-floor refusal and the no-width refusal
  must **not** offer `F` (deck's own limit, not contention; §11.9 says so explicitly).
  `features/interactive_refusals.feature`'s existing `press a to attach` assertions must keep
  passing, so extend the wording rather than replacing it.
- One implementation, not two copies: `enterInteractive` and the force path must share the
  body. A duplicated ladder that drifts is a defect against this requirement even if both
  copies pass.
- Help and footer: `F` appears wherever `↵`/`a` do, so `help_keymap_parity_test.go`,
  `footer_bindings_parity_test.go` and `footer_handler_agreement_test.go` all agree with the
  amended §11.3 keymap. A key bound with no legend entry, or a legend entry with no binding,
  is the §11.3 defect those tests exist to catch.
- A successful forced entry is an attachment in §7's sense: it goes through the same
  `m.prepareAttach` (`store.RecordAttachment`) transaction `↵` uses, once, after every
  refusal and every fallible step has passed — exactly as `d02e166` established for `↵`.
  A refused or failed force entry records nothing.
- With nothing to steal, `F` and `↵` must be indistinguishable in effect: the same claim, the
  same fit, the same one attachment, the same SIGWINCH count.

### R99 — a contested window has exactly one winner

- Add the force claim to `internal/tmux/ownership.go`, beside `ClaimWindowOwnership` and
  sharing its `<tag>:<pid>` format, `ownershipClaimTag`, and `WindowOwnership` type. Its
  semantics, per §11.9: write this process's fresh claim **over whatever is there** — live
  owner included, so no `pidAlive` respect branch — then the ordinary confirm-read. **Single
  shot: no retry loop.** A confirm-read that returns exactly the value written is the claim;
  anything else means a concurrent writer won, and the call stands down with `acquired=false`
  and **no error**, exactly the way losing to a live owner already does.
- A caller that loses the force claim lands in the same refusal state R98 describes, with the
  `F` hint intact, so a second press is the retry. Deck itself must not loop.
- `Release` needs no change and must not get one: it already reads before unsetting and leaves
  an option alone once it no longer holds its own claim. Prove that property still holds for a
  stolen claim rather than assuming it.
- Unit evidence, in `internal/tmux`, against a real tmux server on a private socket the way
  the existing ownership tests do: a force claim over a live owner acquires; a force claim
  whose confirm-read is taken over by another writer stands down with no error; the loser's
  `Release` does not unset the winner's claim; two force claims against one window leave
  exactly one holder and one option value.

### R100 — the window's original geometry outlives an arbitrary chain of steals

The hazard, stated so it is not rediscovered: a stealer's `CaptureWindowGeometry` reads the
**previous holder's fitted** size, not the size the window had before any deck touched it. A
naive steal therefore destroys the original at the first steal and drifts further at every one
after, and no later exit can put the window back.

- Record the geometry to restore in a second **window-scoped user option** next to
  `OwnershipOption`, named `@deck_isize_geometry`. Encoding, fixed here so nothing drifts:
  `<width>x<height>:<window-size-value>`, with an empty final field when
  `WindowGeometry.WindowSizeSet` is false. A value that does not parse is treated as absent.
- **Written by whoever finds none there** (the first claimant of a fresh window), from its own
  `CaptureWindowGeometry`. **Read, never rewritten, by a claimant that finds one** — that read
  value, not its own capture, is what it will restore. This holds for `↵`, for `F`, and for a
  steal of a steal.
- **Restored and cleared by the last holder to let go legitimately, and by nobody else.**
  Every teardown path must first establish that the claim on the window is still this
  process's own: `exitInteractive`, `teardownInteractive`, `ShutdownInteractive` (SIGTERM and
  the panic route from `cmd/deck`), and R98's unwind ladder on a failed entry. A holder whose
  claim was stolen closes only its own transport — no `RestoreWindowGeometry`, no unset of
  either option, no `resize-window` of any kind.
- The existing attached-client gating on restore (unset the window-local `window-size` and let
  `window-size latest` follow, rather than an explicit resize) is unchanged.
- `InteractiveClaimRecord` (`internal/tmux/reclaim.go`) carries the **resolved original**
  geometry — the value that came out of the option, not the capture — so the SIGKILL-reclaim
  pass restores the right size. `reclaimOne`'s existing stand-down-for-a-live-owner guard
  stays, and must now also leave `@deck_isize_geometry` alone in that case: a crashed
  process's cleanup must never resize a window out from under the client that took over from
  it. A reclaim that *does* proceed clears both options.
- Unit evidence in `internal/tmux` and `internal/tui`: two sequential steals of one window
  restore the pre-first-entry size byte-exactly (width, height, and the `window-size` value
  including its unset shape); a stolen-from holder's teardown issues zero `resize-window`; a
  reclaim over a stolen claim touches neither option.

### R101 — the displaced client is told, and its keyboard is stopped

- **Detection** rides the existing preview/interactive tick — never a per-keystroke check
  (§11.9 states that trade-off as accepted, and a tmux round-trip per key is what it is
  refusing). While `m.interactive` is true the tick establishes two things about the window:
  the ownership option still holds this process's claim, and `SessionAttachedCount` is still
  0. The pipe transport's `StatusDisplaced` is the fast path and must be honoured where it is
  available; the poll is the backstop that also covers `TransportCapture`.
- **Two flavours, and they tear down differently.** Claim no longer ours (stolen by `F`):
  drop out having restored nothing and released nothing — R100's rule. Claim still ours but a
  client has attached: run the ordinary exit teardown (attached-gated restore, clear
  `@deck_isize_geometry`, release ownership).
- **Either way, raise the lost-attach dialog** (§11.4's inventory now lists it): it names the
  session, says another client took over, and dismisses on `↵`. Build it through
  `applyDialogContract` (`internal/tui/dialog_contract.go`) like every other modal — add it to
  `Update`'s overlay interceptor chain and to the mouse-suppression condition every other
  overlay is already in. **It swallows every key while it is up**; that is its whole purpose,
  so a key that leaks past it to list navigation is a defect against this requirement.
- Falling out records **nothing**: no `RecordAttachment`, no acknowledge, no `notify_epoch`
  bump. §7 now says so directly. Assert it on the durable row, not on the frame.
- The SIGWINCH budget must not regress. `features/interactive_sigwinch_budget.feature` pins
  §11.9's two-SIGWINCH-per-cycle cost; a steal costs the winner's own fit and the loser must
  add nothing, which is R100's "restores nothing" seen from the other side. If the accounting
  genuinely changes, that is a finding to report, not a scenario to re-point.

### R102 — passive preview fit stands down under a foreign live claim

`previewFit` (`internal/tui/tui.go`) already declines to fit while **this** client is
interactive, and its doc comment explains why. It has no such guard for another client's
claim, so today a second deck merely *selecting* a row resizes a window the first deck is
typing into — and after R101's fall-out the loser's own passive preview would start fighting
the winner immediately. §11's passive-fit bullet now states the rule: issue no
`resize-window` at all while another live process holds §11.9's claim on that window.

- The capture-based preview still renders, at whatever size the owner is holding. This
  requirement adds a refusal to *fit*, never a refusal to *show*.
- Do not disturb the rest of `previewFit`'s contract while you are in there: the
  `previewFitSessionID` / `previewFitInFlight` coalescing, the `noLivePane: true` non-latching
  return (phase 3g task 035), and the guarantee that **every** early return inside the closure
  owes a `previewFitDone` for the same session ID. A path that returns nil wedges passive fit
  for the model's lifetime.
- Decide deliberately whether standing down latches `previewFitSessionID`, state the choice in
  the doc comment, and pin it with a test: a foreign claim is transient, so latching would
  cost the session its fit for the rest of the model's lifetime — the same defect task 035
  fixed for `noLivePane`.
- `features/preview.feature`'s existing coalesced-fit and floor-skip scenarios pass unedited.

### R103 — the record matches the tree

At the final code sha, in `docs/`:

- `docs/reports/phase3i.md` — a requirement table in the phase 3h style, one row per
  R98–R103, each citing the commits and the evidence path that discharge it.
- `docs/reports/phase3i-findings.md` — anything found on the way, plus the disposition of
  both gates.
- `docs/DELIVERY-LOG.md` gains this phase's paragraph.
- GH issue #19 is the source of this phase and its design section is now SPEC-backed; the
  report says which requirement discharges which numbered section of the issue, so closing it
  is a reading rather than an argument.

## Feature scenarios

New file `features/interactive_force_attach.feature`, tagged `@multiclient`. The behaviour is
inherently two-client, so this is where the phase is actually proven:

1. **`↵` refused, `F` wins.** A holds the preview; B's `↵` is refused and the screen names
   `F`; B's `F` enters; A raises the lost-attach dialog; `↵` at A returns it to the list. The
   durable row shows B's entry recorded one attachment and A's fall-out recorded none.
2. **A forced entry over a `waiting` row answers it**, exactly like an attach (the
   `status_attach.feature` assertion shape, at the forcing client).
3. **Geometry survives the chain.** Capture the window before any deck enters; A enters, B
   forces, A dismisses, B leaves — the window matches the pre-entry capture and both window
   options are unset.
4. **Full attach displaces the holder.** A is interactive; a real tmux client attaches; A
   raises the dialog and leaves by the ordinary teardown; the window is restored per the
   attached-client gating and ownership is released.
5. **The dialog swallows keys.** With the dialog up, a key that would otherwise navigate or
   act (`j`, `dd`, `x`) does nothing observable to the list or the store.
6. **Passive fit stands down** (R102): B selects the row A is interactive on and B's tick
   issues no resize — the window still matches A's held size.

## Ordering

R99 → R98 → R100 → R101 → R102 (the claim before its caller, the caller before the geometry
it hands on, the geometry before the fall-out that must not restore it), then R103 against the
final code sha. The feature scenarios land with the requirement each one proves, not in a
batch at the end. The final *code* sha is whatever commit last touches `*.go`/`*.feature`;
every R103 document cites it, and a later code fix invalidates both gates and forces both to
re-run at the new sha.

## Green when

- `ci/run.sh go test -p=1 -count=1 ./...` exits **0** at the final code sha. Background it and
  poll. Per phase 3g finding F34 the non-verbose launcher cannot print the Gherkin tally —
  take the tally from a companion `-v` run on a docs-only descendant.
- `ci/stability.sh 10` is **10/10 from a clean state at the final code sha**, published
  verbatim from `summary.log` with the script's own captured exit status. A 9/10 is published
  as 9/10 with every failure named and its per-run log path — never rounded up, never re-run
  merely to improve the number.
- The R103 documents exist and cite only shas that resolve and paths that are tracked.

## Non-goals

- **Detaching the other client.** A steal resizes the window under a full-attach client and
  tells a displaced deck client; it never detaches anybody. §11.9 states this.
- **Per-keystroke ownership verification.** Explicitly refused in §11.9; the keystrokes
  between a steal and the tick that notices are an accepted, documented loss.
- **Any priority scheme between a full attach and a preview.** The operator ruled there is
  none.
- §12's cross-session search — `f` is not touched, and nothing in this phase builds search.
- The carried-forward findings that are out of scope and never claimed fixed: F2
  (golden-frame settle), F20 (`status_recovery` dup-pane), F22 (`ByteArrivalPattern`), F37
  (the `sort_order` latent race), F7 quantisation collisions, the `filter.feature` dd/undo
  race, and the OSC 52 clipboard-reliability question. A recurrence in the gate is reported
  with its log path.
- No theme palette / `internal/theme/builtin/*.toml` change, no contrast-floor allowlist, no
  test-only branch or env knob in product code (R8).
- No Phase 4 work, no Codex adapter, no re-litigation of anything R76–R97 already landed.
- Do not touch `features/godog_test.go`'s `defaultTags`, and never narrow a deliverable sweep
  with `-run` or `DECK_GODOG_PATHS`.

## For the planner

Phase 3g lost four approaches to the harness's own stall rule, not to review, and phase 3h
lost approaches the same way. Plan accordingly:

- **One unit of work per task.** One requirement is usually two or three tasks (the product
  change; its unit evidence; its scenario). One feature file is one task. The whole-suite
  sweep is its own task and the stability measurement is its own task and owns its whole
  iteration.
- **No task whose title contains "every".** A task that cannot flip its status within about
  one iteration is two tasks.
- R101's dialog and R99's claim are independent and can be planned in either order relative
  to each other; R100 depends on R99 landing first.

Known tooling gotchas, inherited verbatim from 3g/3h's notes:

- **No Go and no tmux in the container** — everything runs through `ci/run.sh`, which starts a
  sibling container. The docker socket is mounted for this run precisely so that works.
- **Never clean up docker by label, filter or wildcard.** `docker prune`, `docker rm`/`kill`/
  `stop` filtered on `label=ralphd.run=`, and any wildcard `rm`/`rmi` include **this job's own
  container**: a sweep like that SIGKILLs the run mid-iteration and loses the iteration's work.
  Remove a container or image only by exact name and exact tag, and only one you created. The
  same rule for processes: never `pkill -f` or `killall` — resolve a pid, verify what it is,
  signal that pid. Other checkouts on this host are other runs' live workspaces; do not edit,
  stash or clean any tree but `/workspace`.
- `edit`/`write` corrupt `\r` in `.feature` files — `od -c` first.
- Each bash call is a fresh shell; capture `$?` in the same call.
- At most one whole-suite run per iteration.
- Commit subjects `<area>: <why> (task NNN)` with this phase's own task ids; push every
  completed task's commit; never force-push or rewrite history.
- A deck-managed tmux session is named `deck_<slug>`; the `internal/tui` real-tmux unit
  harness is `selectionTestSocket(name)` + `newQuietSelectionPane(t, socket, session, w, h)`.
