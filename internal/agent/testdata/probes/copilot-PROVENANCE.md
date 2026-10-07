# Provenance of copilot probe fixtures

These are **real captures of a real GitHub Copilot CLI 1.0.93** (`copilot
--version` printed `GitHub Copilot CLI 1.0.93.`), taken before any Copilot
adapter existed, so the copilot rules in `probe.go` are fitted to recorded bytes
and not to invented UI text. It is the same discipline as `codex-PROVENANCE.md`
and `pi-PROVENANCE.md`. Nothing in `copilot/` is hand-written.

## Capture method

- Binary: the native `@github/copilot-linux-x64@1.0.93` npm tarball (the
  package the `@github/copilot` loader pulls in), unpacked into a scratch dir
  outside version control. Linux x86_64 ELF, run on glibc (Debian trixie).
- Host: a throwaway Docker container (the repo's `deck-ci` image, `tmux 3.5a`)
  on Linux 7.0.12. Captured **2026-10-07, 17:50-17:58 UTC**.
- Terminal: a private tmux server (`tmux -L cap4m`, never a `deck*` socket),
  **120x40** for every fixture except `idle-wrapped-80.txt` (**80x30**).
  Panes were captured with `tmux capture-pane -p -t <session>`, the same call
  the probe path in `internal/service/reconcile.go` makes.
- **The copilot TUI uses the alternate screen** (`CSI ? 1049 h` is in
  `stream.raw`; `list-panes -F '#{alternate_on}'` returned `1`), and
  `capture-pane -p` reads the alternate screen while the app is on it.
- Isolation: `HOME` and `COPILOT_HOME` pointed at fresh dirs under `/tmp` for
  every launch, so no operator state (`~/.copilot`) was read or written. The
  sandbox had no GitHub login, so no GitHub model routing was used.
- **Model provider.** The model turns ran against a **local stand-in
  OpenAI-compatible chat-completions server** on `127.0.0.1:18080` (a ~70-line
  Python script, kept out of tree), selected with copilot's own BYOK variables:
  `COPILOT_PROVIDER_BASE_URL`, `COPILOT_PROVIDER_TYPE=openai`,
  `COPILOT_PROVIDER_API_KEY` (a dummy value), `COPILOT_MODEL=stand-in`,
  `COPILOT_OFFLINE=true`. That drives copilot's real UI, real permission and
  question dialogs, real retry and error paths. It is why the footer's right edge
  reads `stand-in` and why some panes carry a `stand-in streaming word N` or
  `Stand-in reply:` transcript line. Both are synthetic decoration of a real
  UI. The model name is not part of any anchor.
- Every launch used `--no-auto-update` and cwd `/tmp/work`, a directory that is
  not trusted.

## How each state was reached

| file | state on screen | how it was driven |
|---|---|---|
| `trust.txt` | folder-trust prompt, nothing answered | fresh launch in the untrusted cwd, captured 6 s after launch |
| `permission.txt` | permission dialog for a shell command | prompt that makes the stand-in call the `bash` tool (`sleep 90`); the dialog stays up until answered |
| `question.txt` | `ask_user` question dialog | prompt that makes the stand-in call `ask_user` with a one-choice schema |
| `working.txt` | Working footer, tail `esc interrupt` | stand-in streams a reply one word per second; captured mid-stream |
| `working-edit-prompt.txt` | Working footer, tail `esc edit prompt` | stand-in answers HTTP 500, captured during copilot's retries |
| `error.txt` | a `✗ ` line above the context line, idle footer | the same 500 once the retries ran out |
| `warning.txt` | a `! ` line, idle footer | launch with `--plugin-dir` naming a missing dir and `--additional-mcp-config` naming a server that cannot spawn |
| `idle.txt` | idle footer, empty input | session after startup, nothing typed |
| `idle-draft.txt` | idle footer, a draft typed | text sent with `send-keys -l`, no Enter |
| `idle-wrapped-80.txt` | idle footer wrapped by an 80-column pane | fresh launch in an 80x30 pane, trust answered with Enter |
| `allow-all-footer.txt` | `Allow All` footer on a launch **without** `--allow-all` | launch with no permission flags and `COPILOT_HOME/settings.json` holding `{"defaultPermissionMode":"allow-all"}` |
| `stream.raw` | raw byte stream of one start, one turn, one exit | see below |

`working-edit-prompt.txt` and `error.txt` come from the same real prompt,
`working-edit-prompt.txt` early and `error.txt` after it gave up. The
trust dialog's `Do you trust the files in this folder?` sentence differs from the
permission dialog's `Do you want to ` stem, so the two never match each other;
they do share the line `↑/↓ to navigate · enter to select · esc to cancel`.

## `stream.raw`

The exact bytes tmux's `pipe-pane -O` delivered to a `cat >>` while copilot
ran one start, one turn and one exit (`say hello – café ✓`, then `Ctrl-C` twice)
in a 120x40 pane. `pipe-pane` was attached before copilot's first byte
(copilot started after a 2 s `sleep` in the pane's command), so the startup
queries are present. The cwd was pre-trusted through `trustedFolders` in the
temp `COPILOT_HOME/config.json` so the stream is not interrupted by the trust
dialog; that file is capture scaffolding, never something deck writes. The
stream holds, in order: modifyOtherKeys, `CSI ? u`, alt screen, bracketed
paste, focus 1004, mouse 1003 + 1006, `OSC 10/11/4 ; ? ST` queries, `CSI ? 996
n`, `CSI > q`, DECRQM 12 and 1007, `OSC 0 ; GitHub Copilot BEL`, a second
BEL-terminated `OSC 0` title carrying the prompt text and non-ASCII characters,
the hide/show-cursor pairs, the reply, and the mode resets and alt-screen leave
at exit. It has no scroll-region sequence and no DCS or APC: a turn this short
never scrolled.

## Edits to the captured bytes

**None.** Every fixture is the verbatim output of `capture-pane -p` (which trims
trailing spaces on a row itself) or, for `stream.raw`, the verbatim pipe-pane
bytes. No hostname, token, account or path needed redacting: the only paths are
`/tmp/work` and temp dirs, the only host is `127.0.0.1`, the session id in the
exit banner is a random UUID, and the provider key was a dummy value that never
appears on screen. A test greps the directory and this file for the GitHub token
prefixes and the HTTP authorization scheme name (spelled in the test, not here).

## What the corpus shows that a rule must not trip on

- **The `✗ ` glyph is not only an API error.** Copilot also starts a line with
  `✗ ` for a tool call the user declined (`✗ Shell … Operation aborted by user`),
  for a failed slash command (`✗ Failed to change directory: …`,
  `✗ Invalid color mode: …`), and for `✗ Asked user …`. All of them sit in the
  transcript above the context line, as the real error line does. A transcript
  that has scrolled such a line into view still reads as an `✗ ` line.
- **A wrapped footer splits words.** At 80 columns the footer prints as
  `← open sidebar· Interactive · Manual` / `Approval … tab` / `stand-in`, so
  `Manual Approval` is split across rows and the space before `·` after `sidebar`
  is gone. `· / commands` survives intact, which is why R219 matches substrings
  and not a full footer line.
- **The footer's mode token.** `Manual Approval` and `Allow All` follow
  `Interactive`. `/allow-all on` inside a session flips it to `Allow All` too.
- **The idle footer lacks `/ commands` once a draft is typed**; it then reads
  `Interactive · Manual Approval · @ files · # issues`.
- **A tab bar (`[Current]  Sessions  Issues  Pull requests  Gists`) heads the
  screen** on a fresh launch; it is part of the UI, not state.
- **Ctrl-C aborts a turn with no hook**: the pane shows `● Operation cancelled
  by user` and the idle footer returns (see the abort-demotion rule).
