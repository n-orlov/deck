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

## `idle` (added 21 Aug 2026, task 037 / operator steer `003-pi-idle-rule.md`)

The original version of this document claimed pi printed "no other recurring
idle indicator at all" once the one-time startup banner scrolled out. **That
was wrong, and it was contradicted by this document's own two committed
fixtures at the time**: both `running.txt` and `error.txt` already ended with
pi's persistent two-line status footer — a cwd line (`/tmp/pw1`) followed by
a usage-stats line ending `(auto)` … `(amazon-bedrock) <model> • <level>` —
which `lastContentLine`'s own doc comment already described as chrome "pi
and claude both draw at the very bottom of every pane regardless of status".
The reasoning that correctly rejected the startup banner (a one-time line
that ages out of the sampled scrollback) does not apply to a footer that pi
redraws every frame; the original document over-generalised from "the banner
is not durable" to "nothing is durable".

Corrected: `idle.txt` is a real capture, same method and geometry as
`running.txt`/`error.txt`, taken after **four** conversational turns —
enough for the one-time startup banner to have scrolled completely out of
the pane (it is absent from the file). The footer is present, and its
invariant part — the literal substring `(auto)` and the bullet `•`
(U+2022 — distinct from the startup banner's own middle dot `·`, U+00B7, so
the two markers never collide) — is unchanged from `running.txt`/`error.txt`
even though cwd, model name, thinking level and the percentage all differ
between the three captures. `probe.go` now has a `pi`/`idle` rule keyed only
on `(auto)` and `•`, placed **last** among pi's rules (after the `Error:`
tail rule and `Working...`), so it only fires on the *absence* of those two
verdicts — positive evidence (the footer, meaning pi is alive and rendering)
plus negative evidence (no working/error marker matched), never pane
liveness alone (§7 forbids inferring `running`/`idle` from liveness). Rule
ordering is proven, not just inspected: `TestPiIdleRuleStaysLastAmongPiRules`
in `probe_test.go` fails if the idle rule is moved ahead of the error or
running rules (verified by hand: swapping the order made the test fail with
`running.txt` reclassified as `idle`, then reverted).

**The one real false-positive risk was tested and did not materialize.**
`testdata/probes/pi/sleep-midrun.txt` is a real capture taken mid-way through
a real `run: sleep 25` tool call (captured at the 11s mark of a 25s sleep, at
the same geometry). `Working...` was still on screen throughout the entire
call, at every sample point checked (roughly 4s, 9s, 14s, 19s, 26s in). So a
long-running tool call never reaches the idle rule at all — the `Working...`
rule (which runs first) keeps matching for the whole duration. Had this not
held, the idle rule would have been a genuine blocker per the steer's
instruction, and would have been left out with the false-positive reported
instead of shipped; it is shipped because the risk was checked against a real
pi, not assumed.

## States still left out (captured but without a durable marker, or not captured at all)

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
