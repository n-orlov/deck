# Provenance of pi probe fixtures (requirement 38)

These fixtures replace the invented corpus (`pi coding agent`, `Working ·
ctrl-c to stop`, `Allow tool execution?`, `Starting pi…`) that no real `pi`
ever printed, which is why a real `pi` session never left `starting` before
this fix.

## Capture method

Captured against a real `pi 0.84.1` binary (`/usr/bin/pi` in this job's
container; the PRD's own reference captures are from `pi 0.84.2` — the
version delta is recorded here rather than papered over, since 0.84.1 was
the only binary available to drive). Driven with Python's `pty` module at a
fixed **100x30** size (matching the PRD's own idle-footer capture geometry)
through a real conversation against this job's already-configured model
gateway (no mocking of the model). Raw pty bytes were rendered into a plain
80-cell-independent text grid with `pyte.Screen`/`pyte.Stream` (stripping
SGR/cursor escapes, exactly what `tmux capture-pane -p` — which
`internal/service/reconcile.go`'s probe path actually uses — would also
strip) and saved verbatim, with only leading/trailing blank padding rows
trimmed.

## `running.txt`

Captured mid-turn, immediately after sending a prompt that triggers a tool
call (`run: echo hello-from-tool`), while pi's own working indicator was on
screen. The literal marker used by `probe.go` is `Working...` (present
behind every animated spinner frame observed: `⠧`, `⠼`, `⠙`, `⠇`, `⠴`,
`⠹` — Contains-matching makes the spinner glyph itself irrelevant).

## `error.txt`

Captured by launching pi with a deliberately invalid `--model
totally-bogus-model-xyz`, which reaches a real (if generic) agent-level
failure: `Error: Unknown: UnknownError`. Traced against pi's own bundled
source (`dist/modes/interactive/components/assistant-message.js:145`,
`this.contentContainer.addChild(new Text(theme.fg("error", "Error:
${errorMsg}"), ...))`) to confirm `Error: ` is pi's own top-level
assistant-error banner text, not something `probe.go` is inventing.

**Narrowing (the second thing requirement 38 asks for):** a plain
`contains: ["Error:"]` rule is unsafe — verified by asking pi to run `echo
"Error: fake tool error from a script"` inside a *successful* session: the
shell tool's own stdout containing that literal string appeared on screen
with nothing else that would tell a naive substring rule apart from a real
agent error (`dist/modes/interactive/components/tool-execution.js` never
constructs an `"Error: "`-prefixed string itself — the ambiguity is a pure
pane-scraping artifact, exactly as requirement 38 predicted). What *does*
reliably tell the two apart, confirmed across several captures: pi's own
error banner is always the pane's **last non-blank, non-separator content
line** before the footer (nothing follows it — the turn stops there),
whereas a tool's echoed text is always followed by more transcript (a
`Took Ns` line, a closing code fence, or further assistant prose) before
the pane settles. `probe.go`'s pi error rule therefore requires `Error:` to
be the prefix of that tail line (see `lastContentLine` in `probe.go`), not
merely present anywhere in the captured scrollback.

## States left out (captured but without a durable marker, or not captured at all)

- **`idle`** — captured (the pane sitting still with nothing happening:
  pi's startup welcome banner, `pi v0.84.1` / `escape interrupt · …` /
  `Press ctrl+o …`), but that banner is a **one-time** transcript line: it
  scrolls out of `capture-pane`'s `-200`-line window (the range
  `internal/service/reconcile.go` actually samples) after enough
  conversation, and across several multi-turn captures pi printed **no**
  other recurring idle indicator at all — no per-turn box, no "Ready"
  glyph, nothing distinguishing "just finished, waiting for you" from any
  other quiet moment. Using the startup banner as a general `idle` marker
  would silently stop matching partway through an ordinary session while
  looking like a real rule — the same shape of defect this refit exists to
  remove. Left out; a `pi` row that goes quiet keeps its last known
  probed/hook status rather than a fabricated `idle` verdict, which is
  §7-safe (never claims more than the evidence supports).
- **`starting`** — the only pre-idle text observed in this environment
  (`fd not found. Offline mode enabled, skipping download.` / `fd not
  found. Downloading...`) is a helper-binary bootstrap message tied to this
  capture container's missing `fd` binary and `PI_OFFLINE=1`, not a
  property of `pi`'s own status semantics — a normal install with `fd`
  already present would not print it. No general marker was found. Left
  out; a session already starts at `starting` by default, so no rule is
  needed to assert it.
- **`waiting`** (permission prompt) — **not captured at all**. pi has no
  built-in permission-confirmation UI; that is explicitly an
  extension-provided affordance (`ctx.ui.confirm`, see
  `docs/extensions.md`), and pi's own README states plainly: "No permission
  popups. Run in a container." Every tool call this job's pi actually ran
  (including `rm /etc/hostname`) executed with no prompt at all. Reaching a
  real `Allow …?`-style pane would require installing and configuring a
  specific confirmation extension, which is out of scope for a static,
  deterministic fixture capture. Left out per requirement 38's own
  instruction to say so and leave the rule out rather than invent a marker.
