# Spike: in-screen tmux embedding for deck's preview pane

## Goal

`SPEC.md` §11 specifies deck's preview as **"pane capture with escapes preserved, 1 s tick,
selected row only. No embedded PTY emulator in v1 — the preview is a capture, so it is never
interactive."** This spike decides whether that is the right choice or a limitation worth
lifting: **can deck render a live, continuously-updating view of a tmux pane inside its own
bordered panel, without perturbing the real agent pane, without corrupting its own chrome,
at acceptable cost, and while remaining deterministically assertable?**

The answer changes the architecture of the next phase, so it is being settled before that
phase's PRD is written.

This is a **throwaway spike**. Its only lasting deliverable is a written report. If a
requirement cannot be met, **say so explicitly with the exact error or measurement** — a
documented "not viable" is a successful spike and is the outcome the operator most needs to
be able to trust. Do not silently work around a blocked requirement, and do not soften a
bad number.

## Background the agent needs

- **Sibling containers.** This job container has the docker CLI and the host docker socket;
  containers you start are siblings on the host daemon, so every bind source must be a
  **host** path. `$RALPHD_HOST_WORKSPACE` is the host path of the directory mounted here at
  `/workspace`. `ci/run.sh` already encapsulates this correctly — use it for all Go and
  tmux work: `ci/run.sh go test ./...`. If the image is missing, build it with
  `docker build -t deck-ci:local -f ci/Dockerfile ci`. The image carries Go 1.25.13 and
  tmux 3.5a.
- **What deck already does to tmux.** `internal/tmux/tmux.go:423-426` sets, on server
  creation: `exit-empty off`, `remain-on-exit failed`, **`window-size latest`**, and
  **`aggressive-resize on`**. `detach-on-destroy` is deliberately left at its default
  (`on`). Sessions live on a private socket (`tmux -L deck`), one window and one pane each.
- **Why this is not obviously feasible.** `SPEC.md` §3.3 documents the consequence of those
  options honestly: two clients attached to the same session at different terminal sizes
  *share* one view, and `window-size latest` makes the most recently active client govern
  the size. An embedded preview is a **second attached client at roughly 45×22**. Under
  `window-size latest` it may reflow the real agent pane to the preview's dimensions, which
  would corrupt the very thing it is previewing. The current capture-based design attaches
  **no client at all** and therefore has exactly zero influence on pane geometry. Embedding
  trades that guarantee away, and this spike exists to find out what it buys.
- **`internal/tmux` already supports capture with escapes** via the
  `IncludeEscapeSequences` option (`capture-pane -e`), so the baseline being compared
  against is real, already-shipped behaviour.
- The relevant spec text is §3.2 (tmux contract), §3.3 (attach and geometry), §11 (the
  sidebar/preview shape and its keymap), §11.2 (layout modes, floors, and the golden
  80×24 frame), §11.3 (panel chrome). Read them before starting. **You may not change
  them** — see Constraints.

## Requirements

1. **Throwaway module and a deterministic load generator.** Create a Go module under
   `.spike-preview/` (gitignored) containing a **alt-screen load generator**: a program
   that enters the alternate screen and repeatedly draws a full-screen frame using SGR
   colour, box-drawing characters, a moving spinner and a scrolling region, at a redraw
   rate set by an environment variable, and that can also run in a **fixed-frame mode**
   that draws one deterministic frame and stops. It must need no network and no
   credentials. This stands in for a real agent TUI throughout the spike.

2. **Control measurement.** Start a session on a private socket running the load generator
   in a pane of a known size (for example a client attached at 120×40, or
   `new-session -x 120 -y 40`), with deck's own four options set as in the Background.
   With **no second client attached**, record `display -p '#{window_width}x#{window_height}'`
   and a hash of `capture-pane -e` output. Every later measurement is compared against
   this control.

3. **Does a read-only preview client perturb the real pane?** This is the spike's central
   question. Attach a second client **read-only** (`attach -r`) at 45×22 in its own pty,
   while the first client stays attached at 120×40. Record the window geometry and the
   capture hash *during* the read-only attach and again after detaching it. Do this for
   every combination of `window-size` ∈ {`latest`, `manual`} × `aggressive-resize` ∈
   {`on`, `off`}, and additionally with `refresh-client -C 45x22` issued on the preview
   client. **Deliver a table with one row per combination**, giving the measured window
   size, whether the 120×40 client's rendering changed, and whether the previewed program
   itself observed a resize (SIGWINCH). Say plainly which combinations, if any, leave the
   real pane untouched.

4. **The pinned-window option and what it costs.** With `window-size manual` plus
   `resize-window -x 120 -y 40`, establish: (a) the real client at 120×40 renders
   unchanged; (b) what the 45×22 preview client actually sees — cropped, letterboxed, or
   something else; and (c) what a **real** `↵` attach at a *different* terminal size then
   sees. (c) matters because pinning the window changes the behaviour §3.3 currently
   documents for ordinary attaches, so the report must state the new behaviour exactly
   rather than only the preview's gain.

5. **Composition inside deck's chrome.** Write a bubbletea program that renders a bordered
   panel in §11.3's style (rounded borders `╭╮╰╯`, one column of horizontal padding) with a
   live view of the pane inside it at a fixed 45×22, fed through a **terminal emulator**
   that maintains a cell grid — not by passing the pane's bytes through to the outer
   terminal. Run the load generator for at least 50 frames and assert that the panel is
   exactly its declared width and that every rendered line still begins and ends with the
   expected border glyph. **If a pass-through implementation is attempted and the agent's
   escape sequences reach the outer terminal and disturb the frame, report that as this
   requirement failing** — it is the expected result and it is the reason an emulator is
   needed.

6. **Emulator options, evidenced.** Evaluate at least **two** current Go terminal-emulator
   libraries as the cell-grid layer for requirement 5 (candidates to consider, subject to
   what actually builds: `github.com/hinshun/vt10x`, `github.com/charmbracelet/x/vt`, or
   another maintained option you find). For each, report: whether it builds in the CI
   image, whether it correctly handles the **alternate screen**, SGR colour, and
   box-drawing plus East-Asian-Wide characters, its apparent maintenance status, and its
   licence. Name the one you would use and why.

7. **Cost.** Over a fixed 30-second window at the load generator's highest tested redraw
   rate, measure CPU seconds and peak RSS for (a) the 1 s `capture-pane -e` baseline and
   (b) the embedded-emulator approach. Report both absolute numbers and the ratio. Deck is
   a long-running foreground TUI on a developer's machine, so a continuous cost is a real
   product cost, not a benchmark curiosity.

8. **Latency.** Measure the delay from a sentinel string appearing in the pane to it being
   visible in the rendered preview, for both approaches, with at least 10 samples each.
   Report min / median / max for each.

9. **Determinism, against §11.2's golden frame.** Using the load generator's fixed-frame
   mode, establish whether an embedded preview can yield a **byte-identical rendered frame
   across 10 consecutive runs at 80×24** with the preview region included. If it cannot,
   state precisely what would have to change for §11.2's golden minimum-size frame
   assertion (side-by-side, 35/45, at 80×24) to remain meaningful — for example a
   deterministic fake pane, or excluding the preview region from the golden assertion, and
   what each would cost in coverage.

10. **Failure modes.** Record observed behaviour for each of: the previewed session is
    killed while the preview client is attached (note `detach-on-destroy` is at its default
    `on`, per §3.2 — does the preview client's forced detach or death disturb the host
    TUI?); the pane's process exits non-zero under `remain-on-exit failed`; the preview
    target does not exist; the outer terminal is resized while a preview is live; the same
    session is previewed by two deck clients at once.

11. **The interactive question, answered but not built.** State whether an embedded view
    *could* accept keystrokes, and what would have to prevent a keystroke reaching the
    wrong session. Note that §11.1 deliberately narrows send-without-attach (`s`, only
    from `idle`, single-line, no `--force`) and that §11 states `↵` is how you get a real
    terminal, so making the preview interactive is a product decision with spec
    consequences. **Do not implement interactivity.**

12. **Report.** Write `docs/spikes/tmux-embedded-preview.md` containing: the exact commands
    used; requirement 3's table; requirement 4's finding; requirement 6's comparison;
    requirements 7 and 8's numbers; requirement 9's verdict; requirement 10's list;
    requirement 11's answer; every gotcha discovered; and a **single explicit
    recommendation** — either keep `capture-pane -e` polling exactly as §11 specifies, or
    amend §11 to an embedded emulator — with the reasoning, and with the specific `SPEC.md`
    and `docs/PLAN.md` changes each option would require, written as a proposal for the
    operator rather than applied.

**Optional, strictly time-boxed:** if an authenticated `claude` CLI happens to be reachable
from a sibling, add one confirming measurement of requirements 7 and 8 against a real
Claude pane instead of the load generator. Do not spend more than a small fraction of the
budget on it, do not attempt to obtain or install credentials, and never block on it. The
synthetic load generator is the mandatory path.

## Constraints

- **Do not modify `SPEC.md`, `docs/PLAN.md`, anything under `prds/`, or anything under
  `internal/`, `cmd/`, `features/` or `ci/`.** This spike changes no product code and no
  planning document. Its output is a report that *proposes* changes.
- The only files you may create are `.spike-preview/**` and
  `docs/spikes/tmux-embedded-preview.md`, plus one `.gitignore` line for
  `.spike-preview/`.
- **Commit exactly once**, containing only the report and the `.gitignore` line, with a
  message saying what the spike concluded and why. Never force-push, amend, or rewrite
  published history.
- All Go and tmux work happens inside `ci/run.sh`. Do not install a toolchain into this job
  container.
- Every tmux server you start must be on a **private socket** named for this spike, and
  must be killed afterwards. Never touch the `deck` socket or the default socket. Leave no
  running containers behind; siblings are `--rm`.
- Do not implement the preview pane in the real TUI, and do not start on any part of the
  UX phase. Answering the question is the entire job.
- The existing suite must still pass at the end (`ci/run.sh go test ./...`), which it will
  if you have touched no product code — run it once as proof.
