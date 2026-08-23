# Spike: scoped interactive preview for deck

Run 2026-08-22 by three parallel agents, orchestrated from the deck oversight session.
**129 MB, 815 evidence files.** This is the durable copy; the working copy was `/tmp/deck-ispike`
and is not to be relied on.

## The question

deck's preview today is a `capture-pane -e` poll at 250 ms: no attached client, no pipe, no
resize, and a pane larger than the panel is **cropped**, never fitted (`SPEC.md` §11, asserted by
`features/preview.feature`'s `@requirement-21-preview-no-side-effects`). The operator asked whether
to switch to **resize** plus **keyboard passthrough** with a magic detach chord — an
`agent-of-empires`-like "feels attached" experience.

The design put to the spikes was **scoped interactive mode**: passive preview unchanged; on an
explicit keypress, fit the window to the preview box, stream via `pipe-pane -IO` into an
in-process `charmbracelet/x/vt` grid, forward keys with `send-keys`, and restore geometry on exit.

## Verdicts

| spike | question | verdict |
|---|---|---|
| **A** | is the geometry restore complete? | **Yes, byte-exact**, both tmux 3.5a and 3.6b, at exactly 2 SIGWINCH per cycle |
| **B** | `pipe-pane` + grid, or just poll faster? | **Take the pipe** if the mode is genuinely interactive. CPU does not decide it; RSS (~53 MiB/pane) and semantics do |
| **C** | is a fitted box usable, and is the input path safe? | **Usable except at deck's stacked height floor.** The input path needs a 5-field identity tuple, and `Ctrl+Enter` is not shippable |

## Read these first

- [`a/REPORT.md`](a/REPORT.md) — geometry ownership and restore. The restore recipe, the
  `set -g window-size latest` trap, chrome arithmetic, contention, bystander cost.
- [`b/REPORT.md`](b/REPORT.md) — transport. Fidelity (0 diverging cells in 1,262 samples), the
  two drift triggers, the ten-step seed, exclusivity, the real cost table, escape containment.
- [`c/REPORT.md`](c/REPORT.md) — input and fit. `Ctrl+Enter`'s three broken gates, the
  wrong-target demonstration, argv rules, the smallest usable box, echo latency.
- [`BRIEF.md`](BRIEF.md) — what the three agents were asked and the fences they worked under.

Both `a/REPORT.md` and `b/REPORT.md`, and `c/REPORT.md`, were transcribed by the orchestrating
session because the agents' harness forbade them writing report files. The **evidence** under each
`*/evidence/` directory is first-hand, and every script is beside it and re-runnable.

## The findings that would change a decision

1. **The alternate screen has no scrollback.** Shrinking an alt-screen pane 40→22 rows destroys
   **19 of 40 rows permanently** — not recovered by widening. Invisible for an agent that repaints
   from its own model; permanent for one that is hung. *No real agent was measured* (all three
   spikes were fenced from launching `claude`), so whether Claude Code repaints its full transcript
   on widening is **the single most valuable follow-up**.
2. **A dead pane never closes the pipe** under `remain-on-exit failed` — no EOF, frozen grid,
   forever. `#{pane_dead}` must be polled. deck's crash detection is built on that option.
3. **`Ctrl+Enter` fails at three independent gates** and cannot be a default binding.
4. **`set -g window-size latest` restores nothing** — `resize-window` writes `window-size manual`
   window-locally, shadowing the global. AoE's code is accidentally right; its comment is wrong,
   and an implementer following the comment ships the broken version.
5. **The identity tuple must include `pane_pid`.** After `respawn-pane`, `pane_id`,
   `session_name`, `pane_dead`, `pane_start_time` and `pane_current_command` are all unchanged —
   verifying anything less delivers keystrokes into a program the user never selected.

## Corrections to prior art

- The earlier spike's `12.488948×` CPU figure was **render-per-read against a 1-second poll**.
  Against deck's shipped 250 ms it is 8.74×; against the 60 ms poll it would have to beat, 2.11×;
  with renders coalesced to 60 ms, **1.06×**.
- The earlier spike's claim that Bubble Tea v1.3.10 cannot coexist with current `x/vt` is
  **refuted**: the blocker was a stale transitive `x/cellbuf`, and `go get
  github.com/charmbracelet/x/cellbuf@latest` clears it.
- AoE's "any missed byte is a permanent divergence" is **shape-dependent**: undetectable on a
  full-screen repainter, wrong for 4.6 s on an append-only pane.
- tmux **un-wraps** its own wrapping on widening, so a shell's hard-wrapped scrollback *does*
  round-trip byte-identically. The orchestrating session predicted the opposite and was wrong.

## Downstream artefacts

`~/deck-spikes/staged/` holds what these findings became: the `SPEC.md` edit list and the
Phase 3b PRD, staged outside the repo because Phase 3 held the tree read-write when they were
written.

---

## Where the evidence lives

The three full reports and all 815 evidence files stay in `~/deck-spikes/interactive-preview/`
(129 MB) — too large for this repo, and the tmux-embedded-preview spike set the precedent of
keeping raw evidence out of it. `docs/spikes/interactive-preview-spec-changes.md` is the staged
`SPEC.md` amendment this spike produced; it was applied in the same commit that added this file,
so it is now a record rather than a to-do.

The PRD cut from these findings is `prds/phase3b-interactive-preview.md`, delivered as Part II of
`prds/phase3c-residual-and-interactive-preview.md`.
