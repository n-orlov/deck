# Provenance of codex probe fixtures

These are **real captures of a real `codex-cli 0.154.0`**, taken before any
Codex adapter existed, so `probe.go`'s codex rules are fitted to recorded
bytes rather than to invented UI text. (The pi corpus exists because the
first pi rule set was invented and never matched a real pi — see
`pi-PROVENANCE.md`. This corpus is the same discipline applied up front.)

## Capture method

- Binary: `codex-cli 0.154.0`, npm install at
  `~/.local/lib/npm-global/bin/codex`; native musl binary underneath.
- Host: Linux 7.0.12-201.fc44, x86_64. Captured **2026-09-12, 15:18–15:55 UTC**.
- Terminal: `tmux -L codexspike`, **200x50**, captured with
  `tmux capture-pane -p -t 0` — the exact call
  `internal/service/reconcile.go`'s probe path makes. Escape-preserving twins
  (`capture-pane -pe`) of `starting`, `running`, `waiting` and the error state
  were also taken and are kept out of tree; nothing in the classifier needs
  them.
- **The codex TUI does not use the alternate screen.**
  `list-panes -F '#{alternate_on}'` returned `0` at every capture point in
  every session, so plain `capture-pane -p` sees the live UI and no
  alternate-screen handling is required anywhere.
- Every invocation ran with `CODEX_HOME` pointed at a scratch dir under
  `/tmp`. The operator's `~/.codex` was never written to.

`running`, `waiting`, `waiting-patch` and `idle` were driven against a
**local stand-in Responses API** on `127.0.0.1` (a ~60-line script answering
`POST /v1/responses` with a hand-built SSE stream), because the available
model gateway returned HTTP 401. That drives codex's *real* code paths — real
sandbox, real approval UI, real hook dispatch — at zero API cost, and is why
those four fixtures name `fake-model` and carry a
`⚠ Model metadata for 'fake-model' not found` line. Both are synthetic
decoration: `error.txt` and `retrying.txt` come from a genuine remote
provider and show what a real model status line looks like.

## Two edits to the captured bytes, and nothing else

Everything else is verbatim, including trailing blank rows.

1. The internal gateway hostname in `error.txt` and `retrying.txt` was
   replaced with `https://api.example.invalid/v1/responses`. This repository
   is public; the host is not.
2. The gateway's model name in the same two files was replaced with
   `local-model`, chosen to be **exactly as long** as the original so the
   banner box and status line stay byte-aligned.

The first line of every fixture,
`WARNING: proceeding, even though we could not create PATH aliases: Refusing
to create helper binaries under temporary dir "/tmp"`, is an artefact of
putting `CODEX_HOME` under `/tmp` for the capture. It is kept rather than
scrubbed (verbatim beats tidy), and no rule may key on it: deck's own
sessions will not print it.

## The files

| file | state on screen | anchor that classifies it |
|---|---|---|
| `starting.txt` | Launched, never prompted. Banner box, a rotating `Tip:` line, empty composer, **no transcript**. | `>_ OpenAI Codex (v` present and no transcript cell |
| `running.txt` | Mid-turn. | **`esc to interrupt`** |
| `waiting.txt` | Shell-command approval prompt. | **`Would you like to run the following command?`** + `Press enter to confirm or esc to cancel` |
| `waiting-patch.txt` | File-edit approval — the *other* approval shape, via `apply_patch`. | **`Would you like to make the following edits?`** |
| `idle.txt` | Turn complete: user message, assistant cell, composer back, no spinner. | composer present **and** `esc to interrupt` absent |
| `error.txt` | Retries exhausted — the terminal error state. | leading **`■`** on the error line, `esc to interrupt` gone |
| `retrying.txt` | Same session mid-retry (`Reconnecting... 4/5`). | classifies as **running** — see the trap below |

Glyph convention, consistent across every capture: `›` (U+203A) prefixes both
a user message in the transcript and the input composer; `•` (U+2022)
prefixes an in-progress or completed agent cell; `■` (U+25A0) prefixes a
terminal error or an interruption; `✔` prefixes an approval outcome; `⚠`
prefixes warnings.

## Two traps this corpus exists to pin

- **A retrying network error is textually indistinguishable from
  "running".** `retrying.txt` shows `esc to interrupt` exactly as
  `running.txt` does, so the honest verdict for it is `running`, not `error`.
  Only the exhausted state (`error.txt`: leading `■`, no `esc to interrupt`)
  is safely classifiable as an error from the pane. `retrying.txt` is in the
  corpus specifically so that stays asserted rather than rediscovered.
- **`starting` and `idle` differ only by the presence of a transcript**, not
  by the banner: `starting.txt` and `idle.txt` both still show the banner box
  and the `Tip:` line, and both show `› Ask Codex to do anything`. A
  `starting` rule keyed on the banner would therefore mis-classify a
  just-finished turn, and one keyed on "composer present, no transcript" will
  re-fire after `/clear`. The banner also scrolls away entirely on a long
  session — the same reason pi's banner was rejected as a marker.

## Why the probe is the fallback and not the source

Codex is hook-instrumented (SPEC §8.1), so these rules only run when no hook
verdict exists or the last one is stale. That matters for the
`starting`/`idle` ambiguity above: the window where the probe is the *only*
source is precisely the pre-first-prompt window, because
**codex's `SessionStart` hook does not fire at launch — it fires when the
first prompt is submitted.** In that window the true state is `starting`, and
once a turn has happened the live hook verdict outranks the probe anyway.
