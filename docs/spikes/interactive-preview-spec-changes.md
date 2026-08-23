# Staged `SPEC.md` edits for Phase 3b (interactive preview)

**Not yet applied.** Written 2026-08-22 while `deck-phase3` held
`~/Projects/agent-sessions-tui` mounted read-write; editing it concurrently risks the edits being
swept into the job's next commit. Apply the moment Phase 3 is terminal, before cutting Phase 3b.

Line numbers are as of `b847069`. Every claim below is measured — see
`~/deck-spikes/interactive-preview/{a,b,c}/REPORT.md`.

---

## 1. §3.2 tmux server contract (line ~155-168) — add `history-limit`

After the `mouse on` item Phase 3 is already adding, add:

> `set -g history-limit <N>`. tmux's default is 2000, and deck has never set it. This matters
> because §11.9's interactive mode narrows the window: output produced while narrow consumes
> history rows roughly **2.7× faster**, and rows evicted past the limit never return — measured at
> 23 of 40 logical lines destroyed at `history-limit 100` where an unnarrowed control lost none.
> A pane's limit is fixed when the pane is created, so this must be set before `new-session`, not
> after.

## 2. §3.3 Attach (line ~186-200) — `Enter` is no longer the attach key

Replace the opening sentence:

> `Enter` attaches the client directly: `tmux -L deck attach -t deck_<slug>`.

with:

> **`a` attaches the client directly**: `tmux -L deck attach -t deck_<slug>`. On detach the client
> returns to the TUI, which resumes its render loop. `Enter` enters §11.9's interactive preview
> instead — the cheaper, reversible half of the same intent — and `a` is the escalation. Both
> resize the shared window under `window-size latest`; that is not new, and §11.9 states the
> difference in what size each picks.

Add, at the end of the geometry paragraph:

> **Detaching does not restore geometry** when the detaching client was the only one. Under
> `window-size latest`, "latest" means the last client to *express* a size, and once it is gone
> nothing re-expresses the old one. Restoring is therefore a positive action, not a consequence —
> §11.9 specifies it for interactive mode, and an ordinary `a` attach/detach leaves the window at
> the size the attaching client had.

## 3. §11 preview (line ~906-935) — the preview gains a second mode

Keep every existing bullet. The passive guarantees are **unchanged** and their assertions stay
green. Add after the "A pane larger than the preview panel is cropped" bullet:

> - **The preview has two modes, and only one of them touches the pane.** Everything above
>   describes **passive** preview, which is the default and which remains exactly as specified: a
>   `capture-pane -e` poll, no attached client, no pipe, no resize, cropped bottom-left, no scroll.
>   §11.9's **interactive** mode is entered deliberately, per session, and is the only thing in
>   deck that fits a pane to the panel. A reader who takes "the preview never resizes a pane" as
>   unconditional is reading the passive mode, which is the one that runs unbidden.

## 4. New §11.9 — interactive preview

Insert after §11.8. Draft:

> ### 11.9 Interactive preview
>
> `Enter` hands the keyboard to the selected session without leaving the list. deck fits the
> session's window to the preview panel, streams the pane into an in-process cell grid, and
> forwards keystrokes to it. `Ctrl+Q` returns. `a` remains the escalation to a real terminal.
>
> The bet is that a **user-initiated, bounded** geometry change is acceptable where a continuous,
> passive one is not — because `a` already resizes the window today. Interactive mode changes which
> size is chosen, not whether a keypress may perturb a pane.
>
> - **Geometry is owned, claimed and restored.** Entering records the window's dimensions and its
>   window-local `window-size` value, claims ownership in a pid-tagged window option, then resizes
>   the **window** (never the pane — on a split window chrome is proportional and a pane-targeting
>   loop cannot converge). Exiting resizes back *only when no client is attached*, then unsets the
>   window-local `window-size`. The order is load-bearing and `set -g window-size latest` does not
>   substitute for it: `resize-window` writes `manual` into the **window** options, which shadow the
>   global. Cost is exactly two `SIGWINCH` per cycle.
> - **Refused rather than degraded, in three cases**, each naming its reason and offering `a`:
>   another client is attached to that session (the squeeze it would inflict on them is not
>   avoidable — one window has one size); the preview box has fewer than **7 inner rows**, which is
>   deck's stacked height floor and leaves no transcript at all; or ownership is held by a live
>   process.
> - **The transport is `pipe-pane -IO` into a `charmbracelet/x/vt` grid**, seeded from
>   `capture-pane -e -N` plus the pane state tmux exposes as formats, and **reseeded on every
>   resize** — resizing the grid alone leaves it wrong for seconds. The pipe is armed before the
>   seed is taken. `pipe-pane` is single-holder per pane, so a second reader displaces the first
>   silently; a reader that sees EOF while `pane_pipe` is still 1 has been displaced, falls back to
>   passive capture, and **says so**. A dead pane never closes the pipe at all, so `pane_dead` is
>   polled rather than inferred from EOF.
> - **Foreign bytes never reach the outer terminal.** Only composed cells are emitted. This is not
>   an optimisation: pane bytes passed through leave the *outer* terminal on the alternate screen
>   and reprogram its scrolling region. Passive preview gets this property free from
>   `capture-pane`, which carries no such sequences; the moment bytes come from a pipe, the grid is
>   the only safe consumer.
> - **Input is dispatched on a verified identity**, re-resolved immediately before every send:
>   socket path, server pid, `pane_id`, **`pane_pid`** and session name. `pane_id` alone is not
>   sufficient — `respawn-pane` keeps it, and everything else tmux reports, unchanged.
> - **The grid keeps its own bounded scrollback, and the wheel scrolls it.** This is the only way
>   to scroll a full-screen agent: the alternate screen has no tmux history, which is why tmux's
>   own wheel binding declines to enter copy-mode for it.
> - **Honesty about what the user cannot see.** When the target has not repainted since the
>   resize, the panel says so; an empty frame otherwise reads as deck being broken rather than the
>   agent being wedged. Help states that entering interactive mode resizes the agent's window and
>   that output produced while narrow consumes scrollback faster.

## 5. §11.2 (line ~1019) — the height floor gains a second meaning

Add to the **Stacked list 5–12 rows** rationale:

> The 8-row stacked preview floor is also §11.9's refusal threshold: measured against a
> Claude-shaped full-screen program, an inner box of 6 rows renders pure chrome and zero
> transcript, and 7 inner rows is the smallest usable box. A real agent that soft-wraps its input
> box needs more, so 7 is a floor, not a target.

## 6. §11.3 (line ~1046-1054) — the single-focus claim is now false

Replace the **Focus is visible, and the main view has only one place for it** bullet. The sidebar
is no longer the only focusable region. New text:

> - **Focus is visible, and there are exactly two places it can be.** The sidebar is focused by
>   default; §11.9's interactive preview is the second and only other stop, and it is entered by
>   `Enter` rather than by a `tab` cycle, because a cycle would imply stops that do nothing. The
>   focused surface's border uses `border_focus` and the unfocused one uses `border`; the sidebar's
>   selected row uses **`selection_idle`** while focus is elsewhere, which is what that token has
>   always meant. **Colour is not sufficient on its own.** `NO_COLOR` drops deck to monochrome, and
>   deck's own golden frames are captured that way, so a focus indication carried only by a border
>   colour is invisible to the user *and* to the tests. While interactive, the preview's top border
>   therefore carries the target session's name as text — which doubles as the wrong-target
>   safeguard, since a user must be able to see which pane is receiving their keystrokes. Any glyph
>   in it obeys §11's no-East-Asian-Wide rule and has a `DECK_ASCII` fallback.

## 7. §11.3 geometry statement, and `features/preview.feature`'s requirement 23

The panel states `45×22 of 120×40` to say it is showing a window onto a larger pane. Fitted, that
degenerates to `45×22 of 45×22`, which reads as a bug. Interactive mode states the fitted geometry
differently. `@requirement-23-preview-crop-geometry` asserts the pattern `\d+x\d+ of \d+x\d+` and
must be scoped to passive preview.

## 8. §11.8 mouse (line ~1271-1293) — the preview stops being inert

- `wheel over the preview` changes from "does nothing" to "scrolls the grid's own scrollback while
  interactive; nothing while passive".
- `double-click a row` changes from "attach" to "enter interactive mode", matching `Enter`.
- **Single click still only selects.** Keep the existing rationale — a click is an idle gesture and
  under the new model click-to-enter would resize a live agent's window on a stray movement.
- Full attach has no mouse affordance, which is fine: §11.8's rule is that no capability is
  mouse-*only*, not that every key has a gesture.

## 9. §11.1 send-without-attach — superseded

`s`'s narrow protocol exists because typing into a full-screen editor blind is dangerous: it is
refused in `waiting` because "a menu is on screen and the keystrokes would blind-pick an option".
Interactive mode removes that premise — the user can see the menu and the caret. Either delete
§11.1 or reduce it to a pointer at §11.9. This shrinks Phase 7.

## 10. §13.1 — new knobs

- `DECK_INTERACTIVE_MS` — the grid's render-coalescing interval. A duration; render frequency, not
  parsing, dominates the transport's cost.
- `DECK_INTERACTIVE_TRANSPORT=pipe|capture` — pins the render path so a scenario can exercise
  either deterministically. This is a *selector over two implementations of the same contract*, not
  a behaviour switch, and the distinction must be stated because §13.1 otherwise forbids knobs that
  change what the product does.

## 11. §14 open questions

- Item 8 (**shared attach geometry**) is partly answered: living with `window-size latest` remains
  the plan, and §11.9 shows a bounded resize is survivable and reversible. Rewrite it to record
  that the *bystander squeeze* is the residual cost and that deck refuses rather than inflicts it.
- Add: **is ~53 MiB of resident memory per gridded pane acceptable?** Measured for one 120×40
  emulator; deck has no memory budget to judge it against. It is the one axis on which the grid is
  materially worse than polling.
- Add: **do real agents repaint their full transcript on widening?** If not, the alternate screen's
  lack of scrollback makes the fit destroy transcript rows irreversibly. Unmeasured — the spikes
  were fenced from launching a real agent.

## 12. Dependency note for §2 Stack

Bubble Tea stays at **v1.3.10**. Two corrections to what was previously believed:

- v1.3.10 *does* coexist with current `x/vt`; the earlier spike's incompatibility was a stale
  transitive `x/cellbuf`, cleared by `go get github.com/charmbracelet/x/cellbuf@latest`. So the
  grid does **not** force a major upgrade.
- `Ctrl+Enter` is unrepresentable on v1.3.10 — both enhanced encodings arrive as an unexported
  `unknownCSISequenceMsg`, and `KeyEnter == KeyCtrlM` means no `KeyType` value can mean it. It also
  requires `extended-keys always` in the *user's own* tmux config, and `send-keys` cannot emit it in
  either direction. It is therefore not bound; `a` is. A future v2 migration
  (`charm.land/bubbletea/v2`, which needs no opt-in) could offer it as a configurable alias.
