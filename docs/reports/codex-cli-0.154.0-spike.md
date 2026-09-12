<!--
Provenance note. This is the verbatim spike report that established every Codex
CLI fact SPEC §5 and §8.2 assert, captured against a real codex-cli 0.154.0 on
2026-09-12 before any Codex adapter existed. It is kept in the tree because the
phase 4 PRD cites it as the source for payload field names and probe anchors.

Three edits were made to the original for publication: the remote gateway's
hostname, its model name and its token env-var name are replaced with neutral
placeholders (this repository is public). Nothing else is changed.

Only the classifier fixtures were copied into the tree, at
internal/agent/testdata/probes/codex/ (see codex-PROVENANCE.md there). The other
scratch artefacts this report names under /tmp are not in the repository.
-->

# Codex CLI verification spike — for the deck Codex adapter PRD

**Provenance**
- Binary: `codex-cli 0.154.0` (`codex --version`), npm install at
  `$HOME/.local/lib/npm-global/bin/codex`; native binary at
  `.../@openai/codex-linux-x64/vendor/x86_64-unknown-linux-musl/bin/codex` (262 MB, mtime 2026-09-10).
- Host: Linux 7.0.12-201.fc44.x86_64, x86_64.
- All captures taken **2026-09-12, 15:18-15:55 UTC**.
- Every codex invocation ran with `CODEX_HOME` pointed at a scratch dir under
  `/tmp/codex-spike/homes/*`. `~/.codex` was never written to (only `cp`/`grep`-read).
- Terminal for pane captures: `tmux -L codexspike`, 200x50.
- Side effect of putting `CODEX_HOME` under `/tmp`: one stderr line on every run,
  `WARNING: proceeding, even though we could not create PATH aliases: Refusing to create helper
  binaries under temporary dir "/tmp"`. Spike artefact, not a codex property.

---

## Q1. Do inline `-c hooks....` hooks fire WITHOUT `--dangerously-bypass-hook-trust`?

Command (negative case), fresh `CODEX_HOME` with no `hooks.json`:

```
cd /tmp/codex-spike/work
CODEX_HOME=/tmp/codex-spike/homes/q1 timeout 25 codex exec --skip-git-repo-check \
  -c "hooks.SessionStart=[{hooks=[{type=\"command\",command=\"/tmp/codex-spike/dump.sh Q1NoBypass\"}]}]" "x"
```

stdout empty; stderr verbatim (`probes/q1-stderr.txt`):

```
Reading additional input from stdin...
OpenAI Codex v0.154.0
--------
workdir: /tmp/codex-spike/work
model: test-model
provider: bogus
approval: never
sandbox: workspace-write [workdir, /tmp, $TMPDIR]
reasoning effort: none
reasoning summaries: none
session id: 01a095fc-3f7f-70f0-9fb0-12e1b70a2296
--------
user
x
warning: Model metadata for `test-model` not found. ...
ERROR: Reconnecting... waiting for network
```

`ls /tmp/codex-spike/homes/q1/dumps` -> `No such file or directory`.

Positive control, identical command + `--dangerously-bypass-hook-trust` (`probes/q1b-stderr.txt`)
additionally prints:

```
warning: `--dangerously-bypass-hook-trust` is enabled. Enabled hooks may run without review for this invocation.
hook: SessionStart
hook: SessionStart Completed
```

and the dump file appears.

**Answer: No. Untrusted inline hooks are silently skipped - codex prints no warning, no error and
no mention of hooks at all. There is nothing in stdout/stderr for deck to detect the skip from.**

---

## Q2. Can trust be pre-seeded so no bypass flag is needed?

### Q2a. Yes - headline result

Trust state lives in `$CODEX_HOME/config.toml` as

```toml
[hooks.state."<source>:<event_snake_case>:<group_idx>:<hook_idx>"]
trusted_hash = "sha256:<64 hex>"
```

Verified end-to-end (`probes/q2-seeded-stdout.txt`): a **hand-written** trust entry in a brand-new
`CODEX_HOME`, with **no bypass flag**, fires the hook.

```
# $T/config.toml, appended by hand:
[hooks.state."/tmp/codex-spike/homes/qT2/hooks.json:session_start:0:0"]
trusted_hash = "sha256:d61d41b0c824ddd13f419be61913e62acb2e713e663d83f476e1b7db2f63a84e"
# $T/hooks.json:
{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"/tmp/codex-spike/dump.sh SeededSessionStart"}]}]}}

CODEX_HOME=$T timeout 25 codex exec --skip-git-repo-check "x" </dev/null
-> hook: SessionStart
-> hook: SessionStart Completed
-> $T/dumps/SeededSessionStart.log written
```

**Inline (`-c`) hooks can also be pre-seeded.** The `<source>` component for a command-line hook is
the literal string `/<session-flags>/config.toml`. Discovered by trusting an inline hook
interactively and reading what codex wrote:

```
[hooks.state."/<session-flags>/config.toml:session_start:0:0"]
trusted_hash = "sha256:82903ea5f1d8e24884f563a15e3b222403cfc58863b7710c540eb2d8b99d5329"
```

Verified (`probes/q2-inline-seeded.txt`): with that key pre-seeded and **no bypass flag**,

```
CODEX_HOME=$T codex exec --skip-git-repo-check \
  -c "hooks.SessionStart=[{hooks=[{type=\"command\",command=\"/tmp/codex-spike/dump.sh SeededSessionStart\"}]}]" "x"
-> hook: SessionStart / hook: SessionStart Completed, dump written
```

**Answer: Yes. Writing `[hooks.state."<source>:<event>:<g>:<h>"] trusted_hash = "sha256:..."` into
the `CODEX_HOME` `config.toml` makes hooks fire with no flags. `<source>` is the absolute
`hooks.json` path, or the literal `/<session-flags>/config.toml` for inline `-c` hooks.**

### Q2b. What the hash depends on - differential experiments

Each row is a separate `CODEX_HOME`, trusted interactively via the TUI, then grepped out of the
written `config.toml`.

| variation | trust key written | `trusted_hash` |
|---|---|---|
| `SessionStart`, `/bin/true`, home `q2` | `.../q2/hooks.json:session_start:0:0` | `82903ea5...d5329` |
| same hook, **different CODEX_HOME path** (`qA`) | `.../qA/hooks.json:session_start:0:0` | `82903ea5...d5329` (identical) |
| same hook, **inline `-c`** instead of hooks.json | `/<session-flags>/config.toml:session_start:0:0` | `82903ea5...d5329` (identical) |
| **JSON keys reordered** (`command` before `type`) | `.../qF/...:session_start:0:0` | `82903ea5...d5329` (identical) |
| **pretty-printed** hooks.json | `.../qH/...:session_start:0:0` | `82903ea5...d5329` (identical) |
| extra `"timeout":600` field added | `.../qG/...:session_start:0:0` | `82903ea5...d5329` (identical) |
| event changed to `PostToolUse` | `...:post_tool_use:0:0` | `ef95ed3c...5309d` (**differs**) |
| command changed to `/bin/false` | `...:session_start:0:0` | `20c47384...43787` (**differs**) |
| 2 hooks in group 0 + a 2nd group, all SessionStart | `:0:0`=/bin/true, `:0:1`=/bin/false, `:1:0`=/bin/true | `82903ea5...`, `20c47384...`, `82903ea5...` - index does **not** enter the hash |
| `PreToolUse`, `/bin/true`, no matcher | `...:pre_tool_use:0:0` | `8f7a965a...42377` |
| `PreToolUse`, `/bin/true`, `"matcher":"Bash"` | `...:pre_tool_use:0:0` | `48809718...dd0ec` (**differs** -> matcher IS in the hash) |

So **`trusted_hash = f(event, enclosing group's matcher, normalised hook definition)`**, independent
of hooks.json path, of group/hook indices, of JSON key order and of whitespace. That is what makes
hardcoding practical: for a fixed hook command string the hash is a constant, the same on every
machine and in every `CODEX_HOME`.

Known-good constants on 0.154.0 (single hook, no matcher unless stated):

| event | command | `trusted_hash` |
|---|---|---|
| `SessionStart` | `/bin/true` | `sha256:82903ea5f1d8e24884f563a15e3b222403cfc58863b7710c540eb2d8b99d5329` |
| `SessionStart` | `/bin/false` | `sha256:20c4738400c2c2506b424581934059cb54bdc8b5c7d4512dac956f87f7d43787` |
| `SessionStart` | `a` | `sha256:cacbffbe3785e6e556ebb9b1191c8f38ab8d9c74012f17303a5dbb9a461fe8db` |
| `SessionStart` | `b` | `sha256:eeda598232bd3d8be9c72de949d2b2ea74feb41821471d2ab526fa734938a05e` |
| `SessionStart` | `ab` | `sha256:cba019c9e3fdc96a3d322d1f9c1ee0ef2232d0521ac2c400dcd12afa53f404c3` |
| `SessionStart` | `/tmp/codex-spike/dump.sh SeededSessionStart` | `sha256:d61d41b0c824ddd13f419be61913e62acb2e713e663d83f476e1b7db2f63a84e` |
| `PostToolUse` | `/bin/true` | `sha256:ef95ed3c5f028e7e94f5c20a828cc12509a81f1efceaf9a2ccd44aca3225309d` |
| `PreToolUse` | `/bin/true` | `sha256:8f7a965a0a9eaa9814cc7d1b5b13a5bec78abd403b73e7e3818b88280da42377` |
| `PreToolUse` (matcher `Bash`) | `/bin/true` | `sha256:4880971825a929690b01ed332349d657cf8de09551b66732559910db4bedd0ec` |

### Q2c. The hash preimage - NOT recovered

**I could not reproduce the hash.** Two brute-force passes over ~760,000 and ~43,000 templates,
constrained by the 7 known (event, matcher, command) -> hash data points, produced zero matches.
Candidate space tried, all with and without a trailing newline, all with separators from
`{"", ":", "\n", "\0", "|", " ", "\x1f", "/", "\t"}`, and both `SessionStart`/`session_start`
spellings:

- command alone; `"command"` + command; command with `type` prefix;
- serde_json-style objects `{"type":"command","command":...}` in compact / spaced / key-sorted
  encodings, with and without `timeout`, `timeout_seconds`, `timeout_ms`, `mode`, `execution_mode`;
- externally-tagged form `{"command":{"command":...}}`;
- the enclosing group `{"hooks":[...]}` and the whole `{"<Event>":[{"hooks":[...]}]}` document;
- prefixes/suffixes: event name, snake event name, hooks.json absolute path, the full trust key,
  `0:0`, the matcher rendered as `null` / `""` / JSON, and domain-separation strings
  `"codex-hook-trust-v1"`, `"hook-trust-v1"`, `"v1"`;
- structured objects `{"event":...,"matcher":...,"hook":...}` in all 6 key orders x 2 separator
  styles x sorted/unsorted x camel/snake event.

I also grepped the stripped 262 MB binary. It yields the source file names
(`hooks/src/registry.rs`, `hooks/src/types.rs`, `config/src/...`) and the TOML key literals
(`hooks.state."`, `".trusted_hash`, `".trust_level`), but the strings are concatenated Rust
`.rodata` blobs and I could not isolate the format string used to build the preimage.
**Not reimplementable in Go from this spike.**

Practical consequence: deck must either (a) use *fixed* hook command strings and hardcode the hashes
as constants (fully verified above - path/index/formatting independent), or (b) harvest the hash once
at install time by driving the trust UI (Q2d) and caching it. deck must NOT put per-session data
(session id, socket path) in the hook *command* string, because that changes the hash. Per-session
data should come from the hook's stdin JSON (`session_id`, `cwd`, `transcript_path` are all there)
and from inherited env vars - env inheritance confirmed: the dump hook saw `DECK_SESSION_ID`,
`DECK_SESSION_NAME`, `DECK_SESSION_PROFILE`, `DECK_HOME`, `DECK_SESSION_CWD`, etc.

### Q2d. The interactive trust prompt, verbatim

Directory trust first (skippable by pre-writing `[projects."<cwd>"] trust_level = "trusted"`):

```
> You are in /tmp/codex-spike/work

  Do you trust the contents of this directory? Working with untrusted contents comes with higher risk of prompt injection. Trusting the directory allows project-local config, hooks, and exec policies
  to load.

> 1. Yes, continue
  2. No, quit

  Press enter to continue
```

Then hook trust (`probes/q2-hooktrust.txt`):

```
  Hooks need review
  1 hook is new or changed.
  Hooks can run outside the sandbox after you trust them.

> 1. Review hooks
  2. Trust all and continue
  3. Continue without trusting (hooks won't run)

  Press enter to confirm or esc to go back
```

Option 2 ("Trust all and continue") **is** the persistent choice - it writes `trusted_hash` to
`config.toml` and never asks again for that hook. The per-hook detail screen
(`probes/q2-hookdetail.txt`) shows Source, Command, `Mode  Sync`, `Timeout  600s`,
`Trust  New hook - review required`, and `Press t to trust`.

Scripting it works and takes ~10 s:
`tmux -L ... new-session -d "CODEX_HOME=$T codex" ; sleep 5 ; send-keys "2" Enter ; sleep 4` then read
`$T/config.toml`. It requires a PTY; there is no non-interactive hook-trust subcommand
(`codex --help`, `codex debug --help`, `codex plugin --help`, `codex features --help` all checked).

---

## Q3. Which events fire, and what is each payload?

### Q3a. The event set is 12, not 6

From the in-TUI hooks screen (`probes/q2-hookreview.txt`), verbatim:

```
  Event                 Installed   Active      Review      Description
  PreToolUse            0           0           0           Before a tool executes
  PermissionRequest     0           0           0           When permission is requested
  PostToolUse           0           0           0           After a tool executes
  PreCompact            0           0           0           Before context compaction
  PostCompact           0           0           0           After context compaction
  SessionStart          1           0           1           When a new session starts
  SessionEnd            0           0           0           Right before a session ends
  UserPromptSubmit      0           0           0           When the user submits a prompt
  SubagentStart         0           0           0           When a subagent is created
  SubagentStop          0           0           0           Right before a subagent ends its turn
  Stop                  0           0           0           Right before Codex ends its turn
  Interrupt             0           0           0           Right before an interrupted turn is aborted
```

### Q3b. Authoritative payload schemas, extracted from the binary

The binary embeds 23 draft-07 JSON Schemas (`....command.input` / `....command.output` per event).
Extracted by brace-matching from raw `.rodata`; saved under `/tmp/codex-spike/schemas/*.json`.
`permission_mode` is everywhere an enum of exactly
`["default","acceptEdits","plan","dontAsk","bypassPermissions"]`; `source` on SessionStart is
`["startup","resume","clear","compact"]`; `trigger` on Pre/PostCompact is `["manual","auto"]`.
`session-end` has an **input schema only** - no output schema, i.e. fire-and-forget, it cannot
influence codex.

### Q3c. Observed payloads (real session, `-a on-request -s workspace-write`)

Driven against a local stand-in Responses API (see "How a real session was obtained"), all 12 events
instrumented inline with `--dangerously-bypass-hook-trust`. Raw logs in
`/tmp/codex-spike/homes/q3/dumps-session1/*.log` and `/tmp/codex-spike/dumps-archive/`.

`SessionStart`:
```json
{"session_id":"01a09616-150d-7252-959c-d72a289dae41","transcript_path":"/tmp/codex-spike/homes/q3/sessions/2026/09/12/rollout-2026-09-12T15-47-04-01a09616-150d-7252-959c-d72a289dae41.jsonl","cwd":"/tmp/codex-spike/work","hook_event_name":"SessionStart","model":"fake-model","permission_mode":"default","source":"startup"}
```

`UserPromptSubmit` (adds `turn_id`, `prompt`):
```json
{"session_id":"01a09616-...","turn_id":"01a09616-2b40-72e0-99b1-38b5ee3d5d6f","transcript_path":"...","cwd":"/tmp/codex-spike/work","hook_event_name":"UserPromptSubmit","model":"fake-model","permission_mode":"default","prompt":"please write the marker file"}
```

**`PermissionRequest` - shell command case:**
```json
{"session_id":"01a09616-...","turn_id":"01a09616-2b40-...","transcript_path":"...","cwd":"/tmp/codex-spike/work","hook_event_name":"PermissionRequest","model":"fake-model","permission_mode":"default","tool_name":"Bash","tool_input":{"command":"touch $HOME/codex-spike-escalation-test","description":"Write a marker file outside the workspace"}}
```

**`PermissionRequest` - file-write case** (model called `apply_patch`; TUI said "Would you like to
make the following edits?"):
```json
{"session_id":"01a0961b-...","turn_id":"01a0961b-...","transcript_path":"...","cwd":"/tmp/codex-spike/work","hook_event_name":"PermissionRequest","model":"fake-model","permission_mode":"default","tool_name":"apply_patch","tool_input":{"command":"*** Begin Patch\n*** Update File: hello.txt\n@@\n+added by codex spike\n*** End Patch"}}
```

Fields identifying *what* is being asked, for deck's UI reason string:
- `tool_name` - observed `"Bash"` (any shell/exec approval) and `"apply_patch"` (file edits). codex
  normalises its own tool names to Claude-style: the wire tool is `exec_command`, the hook reports
  `Bash`.
- `tool_input.command` - the shell command string, or the whole patch text for `apply_patch`.
- `tool_input.description` - present only when the model supplied a `justification`; same text the
  TUI shows after `Reason:`.
- No `tool_use_id` on `PermissionRequest` (it IS present on Pre/PostToolUse), and no explicit "kind"
  field - deck must branch on `tool_name`.
- `agent_id` / `agent_type` are optional and appear only for subagent-originated requests.

**`Stop` - carries the assistant's last message:**
```json
{"session_id":"01a09616-...","turn_id":"01a09616-2b40-...","transcript_path":"...","cwd":"/tmp/codex-spike/work","hook_event_name":"Stop","model":"fake-model","permission_mode":"default","stop_hook_active":false,"last_assistant_message":"All done. The directory listing is above."}
```
Key is **`last_assistant_message`**, nullable. Verified null when the turn produced no assistant text
(a local model that emitted nothing gave `"last_assistant_message":null`).

`PreToolUse` / `PostToolUse` - **once per tool call**, distinct `tool_use_id`:
```json
{"...","hook_event_name":"PreToolUse","model":"fake-model","permission_mode":"default","tool_name":"Bash","tool_input":{"command":"ls -la"},"tool_use_id":"call_22d7f1ed"}
{"...","hook_event_name":"PostToolUse",...,"tool_name":"Bash","tool_input":{"command":"ls -la"},"tool_response":"total 0\ndrwxr-xr-x. 5 user user 120 Sep 12 15:46 .\n...","tool_use_id":"call_22d7f1ed"}
```
A turn with two tool calls produced exactly 2 `PreToolUse` + 2 `PostToolUse` with `tool_use_id`
`call_08a46e3f` and `call_ca0b5ec4`. `PostToolUse.tool_response` carries the full command output, so
these are per-call AND potentially large. **Confirmed too chatty for deck.**

`Interrupt` (Esc during a turn):
```json
{"session_id":"01a09616-...","turn_id":"01a09616-d036-78b3-abc9-59dcf20b1d8e","transcript_path":"...","cwd":"...","hook_event_name":"Interrupt","model":"fake-model","permission_mode":"default"}
```

**`SessionEnd` - there IS a session-end event.**
```json
{"session_id":"01a09616-150d-7252-959c-d72a289dae41","transcript_path":"...","cwd":"/tmp/codex-spike/work","hook_event_name":"SessionEnd","reason":"other"}
```
Fired on `/quit` and again on double `Ctrl+C`; `reason` was `"other"` both times. It has **no
`model`, no `permission_mode`, no `turn_id`** - only `session_id`, `transcript_path`, `cwd`,
`hook_event_name`, `reason`.

`PreCompact`, `PostCompact`, `SubagentStart`, `SubagentStop` were instrumented but not triggered
(no compaction, no subagents). Their schemas are in `/tmp/codex-spike/schemas/`.

**Answer: 12 events exist. `PermissionRequest` gives `tool_name` + `tool_input.command`
(+ optional `tool_input.description`) and is exactly the "waiting for you" signal deck wants.
`Stop` carries the final assistant text under `last_assistant_message`. `PreToolUse`/`PostToolUse`
fire once per tool call with full `tool_response` bodies - correctly excluded. A session-END event
DOES exist: `SessionEnd`, with a `reason` field and no output contract.**

### Q3d. Timing gotcha: in the TUI, SessionStart fires on the first prompt, not at launch

Measured twice (`/tmp/codex-spike/homes/q5c`, `/tmp/codex-spike/homes/q3`): after launching the
interactive TUI and waiting 6 s, `$CODEX_HOME/dumps` did not exist. It appeared only after the first
prompt was submitted. In `codex exec` the hook fires before the first model call. Same on resume.
**A freshly launched, never-prompted codex TUI is completely uninstrumented.**

---

## Q4. Resume behaviour

Session created above: `01a09616-150d-7252-959c-d72a289dae41`, rollout at
`$CODEX_HOME/sessions/2026/09/12/rollout-2026-09-12T15-47-04-01a09616-150d-7252-959c-d72a289dae41.jsonl`.
On `/quit` codex prints:

```
To continue this session, run:
  codex resume 01a09616-150d-7252-959c-d72a289dae41
```

`codex resume <uuid>` in the same scratch `CODEX_HOME`, inline hooks again, then one prompt:

```json
{"session_id":"01a09616-150d-7252-959c-d72a289dae41","transcript_path":"/tmp/codex-spike/homes/q3/sessions/2026/09/12/rollout-2026-09-12T15-47-04-01a09616-150d-7252-959c-d72a289dae41.jsonl","cwd":"/tmp/codex-spike/work","hook_event_name":"SessionStart","model":"fake-model","permission_mode":"default","source":"resume"}
```

- `source` is **`"resume"`** (vs `"startup"`).
- `session_id` **unchanged**, `transcript_path` the **same rollout file** (appended, not rotated).
- Again fires on first prompt submission, not at launch.

Resume needs a TTY: with stdin a pipe, `codex resume <uuid>` exits `Error: stdin is not a terminal`.

`CODEX_HOME` scoping: resuming the same uuid from a *different* scratch home fails with
```
ERROR: No saved session found with ID 01a09616-150d-7252-959c-d72a289dae41. Run `codex resume` without an ID to choose from existing sessions.
```
Copying **only** `sessions/**/rollout-*.jsonl` into an otherwise-empty third home made the same uuid
resume successfully, full transcript replayed on screen. So the rollout JSONL is what makes a session
resumable; `thread_history_1.sqlite`, `history.jsonl` and `session_index.jsonl` are not required.

**Answer: `codex resume <uuid>` works only within the `CODEX_HOME` that owns the rollout file (or one
you copy `sessions/` into). `SessionStart` fires with `source:"resume"`, the same `session_id` and
the same `transcript_path`. The `sessions/.../rollout-<ISO>-<uuid>.jsonl` file alone is sufficient
for resumability. A TTY is required.**

---

## Q5. Permission flag -> `permission_mode` mapping

Measured in the **interactive TUI** (what deck launches), one scratch `CODEX_HOME` per row, config
containing no `approval_policy`/`sandbox_mode` so only flags matter, SessionStart hook pre-trusted,
prompt submitted to make the hook fire. Scripts: `/tmp/codex-spike/q5tui.sh`, `/tmp/codex-spike/q5exec.sh`.

| launch flags | TUI `permission_mode` | `codex exec` `permission_mode` |
|---|---|---|
| *(none)* | `default` | `bypassPermissions` |
| `-a on-request -s workspace-write` | `default` | `bypassPermissions` |
| `-a never -s workspace-write` | `bypassPermissions` | `bypassPermissions` |
| `-a never -s danger-full-access` | `bypassPermissions` | `bypassPermissions` |
| `-a on-request -s read-only` | `default` | `bypassPermissions` |
| `--dangerously-bypass-approvals-and-sandbox` | `bypassPermissions` | `bypassPermissions` |

`permission_mode` tracks **only the approval policy**, not the sandbox. `-a never` (or the bypass
flag) => `bypassPermissions`; anything that can still ask => `default`. `-s read-only` vs
`workspace-write` vs `danger-full-access` is **invisible** in the hook payload. `codex exec` defaults
approval to `never`, so exec sessions always report `bypassPermissions`.
The schema allows `acceptEdits`, `plan`, `dontAsk` too, but no flag combination I tried produced
them; I did not find a launch flag that does (untested: `--approve-for-me`, config
`permission_profile` / `default_permissions` / `[permissions]`).

Accepted flag values, from `codex --help` and from deliberately invalid values:

```
-a, --ask-for-approval <APPROVAL_POLICY>   [possible values: on-request, never]
-s, --sandbox <SANDBOX_MODE>               [possible values: read-only, workspace-write, danger-full-access]
```
`codex -a auto-edit ...` -> `error: invalid value 'auto-edit' for '--ask-for-approval <APPROVAL_POLICY>'
  [possible values: on-request, never]`
`codex -s full-access ...` -> `error: invalid value 'full-access' for '--sandbox <SANDBOX_MODE>'
  [possible values: read-only, workspace-write, danger-full-access]`

`--full-auto`: **does not exist in 0.154.0.**
`codex --full-auto -h` -> `error: unexpected argument '--full-auto' found`.

Related flags that do exist: `--approve-for-me`, `--dangerously-bypass-approvals-and-sandbox`,
`--dangerously-bypass-hook-trust`, `--no-alt-screen`, `--worktree`, `--add-dir`, `-C/--cd`.

**Answer: the TUI reports only `default` (approval `on-request`) or `bypassPermissions` (approval
`never`, or the bypass flag). Sandbox mode does not appear in the payload. `codex exec` is always
`bypassPermissions`. `--full-auto` does not exist; `-a` takes `on-request|never`, `-s` takes
`read-only|workspace-write|danger-full-access`.**

---

## Q6. Probe corpus

**Alternate screen: NO.** `tmux -L codexspike list-panes -F '#{alternate_on}'` returned `0` at every
capture point (starting, running, waiting, idle, error) in every session. The codex TUI 0.154.0
renders inline in the normal screen buffer, so `capture-pane -p` sees the live UI with no special
handling. (`--no-alt-screen` exists as a flag but was never passed; the default already behaved as
non-alt.)

Captures via `tmux -L codexspike capture-pane -p -t 0`, 200x50, 2026-09-12. Escape-preserving twins
(`capture-pane -pe`) exist for `starting`, `running`, `waiting`, `error`. Sessions used a stand-in
Responses API for `running`/`waiting`/`idle`, so the model name reads `fake-model` and a
`WARN Model metadata for 'fake-model' not found` banner is present - both synthetic decoration; see
`error-401.txt` for a real-model status line.

| file | what is on screen | stable anchors | volatile decoration |
|---|---|---|---|
| `probes/starting.txt` | Boxed banner, a rotating "Tip:", empty composer. No turn yet. | `>_ OpenAI Codex (v`, `model:`, `directory:`, `> Ask Codex to do anything`, status line `<model> - <cwd>` | version number; the `Tip: ...` line rotates between runs (saw two different tips); box width tracks longest line |
| `probes/running.txt` | Prompt echoed as `> ...`, then a live spinner line. | **`esc to interrupt`** (best single anchor), leading `- Working (` | elapsed `(5s ...)`; braille spinner glyphs; trailing `renaming...` (one-shot session auto-naming, first turn only); wording `Working` vs `Reconnecting...` vs `Explored` varies with activity |
| `probes/waiting.txt` | Shell-command approval. | **`Would you like to run the following command?`**, `Press enter to confirm or esc to cancel`, `1. Yes, proceed (y)`, `Environment: local`, `Reason:` line, `$ <cmd>` line | option 2's text embeds the command; `Reason:` present only when the model gave a justification |
| `probes/waiting-patch.txt` | File-edit approval (the other approval shape). | **`Would you like to make the following edits?`**, `Description:`, `Destination: <abs path>`, `Press enter to confirm or esc to cancel`, `1. Yes, proceed (y)` | the diff preview; `- Edited hello.txt (+1 -0)` counts |
| `probes/idle.txt` | Completed turn, assistant text, empty composer, no spinner. | `> Ask Codex to do anything` **and** absence of `esc to interrupt`; status line with no spinner | the assistant text; `Tip:` line |
| `probes/error.txt` | Unreachable provider, still retrying. | `esc to interrupt` IS present (so this classifies as *running*, not error), `Connection failed: error sending request`, `- Reconnecting... waiting for network` | elapsed seconds, spinner |
| `probes/error-401.txt` | Real provider, HTTP 401, mid-retry. Genuine model status line. | `Unexpected status 401 Unauthorized`, `Reconnecting... 4/5`, still has `esc to interrupt`; status line `local-model low - <cwd> - local-model - Context 0% used` | retry counter `4/5`; URL and JSON detail |
| `probes/error-401-final.txt` | Same session after retries exhausted - the terminal error state. | **leading black-square glyph** on the error line (`[sq] unexpected status 401 Unauthorized: ...`); `esc to interrupt` gone; composer back to `> Ask Codex to do anything` | the error text/URL |

Glyph convention worth encoding in deck's classifier (consistent across every capture):
U+203A `>` prefixes both user messages in the transcript and the input composer; U+2022 `.` prefixes
an in-progress or completed agent cell; U+25A0 (black square) prefixes a terminal error or an
interruption (`[sq] Conversation interrupted - tell the model what to do differently.` uses the same
marker); U+2714 check prefixes an approval outcome (`You approved codex to run ... this time`);
U+26A0 warning-sign prefixes warnings and the `N startup issue - ctrl + t for details` line.

Classification warnings for deck:
- **The retrying-error state is textually indistinguishable from "running"** - both show
  `esc to interrupt`. Only the exhausted/terminal error (black-square prefix, no `esc to interrupt`)
  is safely classifiable as error from the pane.
- `starting` and `idle` differ only by the presence of a transcript above the composer, so a
  `starting` classifier keyed on "composer present and no transcript" will re-fire after `/clear`.

Additional captures kept for reference (each carrying the instrumentation banner
`--dangerously-bypass-hook-trust is enabled...`, so **not** suitable as clean fixtures):
`probes/starting-q3.txt`, `probes/running-q3.txt`, `probes/running-q3b.txt`, `probes/waiting-q3.txt`,
`probes/idle-q3.txt`. Trust-UI captures: `probes/q2-trustprompt.txt`, `probes/q2-hooktrust.txt`,
`probes/q2-hookreview.txt`, `probes/q2-hookdetail.txt`, `probes/q2-after-trust.txt`.

---

## How a real session was obtained (and what failed)

The remote provider available at capture time **did not authenticate**. `~/.codex/config.toml` names
`env_key = "GATEWAY_TOKEN"`; after `set -a; source the gateway's env file; set +a` that variable is
set (49 chars), but:

```
$ curl -s -w 'http=%{http_code}\n' -H "Authorization: Bearer $GATEWAY_TOKEN" \
    https://api.example.invalid/v1/models
http=401
{"detail":"Invalid bearer token"}
```
and codex itself: `ERROR: unexpected status 401 Unauthorized: {"detail":"Invalid bearer token"}, url:
https://api.example.invalid/v1/responses`. The token appears expired. I did
not try to refresh it.

Fallbacks tried:
1. **Local ollama** (`http://127.0.0.1:11434`, 9 models present). `codex exec --oss
   --local-provider ollama -m qwen3.6:27b "reply with the single word ok"` **worked** (33 s, local,
   free). But the local models never emitted a tool call under codex's real system prompt - three
   attempts ended each turn with `last_assistant_message: null` - so they could not drive
   `PermissionRequest`. (The same models DO emit tool calls when called directly through ollama's
   `/v1/chat/completions` with a small hand-written tool list, so this is a prompt-complexity
   problem, not a capability one.) Note also: the interactive TUI shows a sign-in gate on a virgin
   `CODEX_HOME`; writing `{"OPENAI_API_KEY":"sk-local-dummy","tokens":null,"last_refresh":null}` to
   `$CODEX_HOME/auth.json` skips it.
2. **A local stand-in Responses API** (`/tmp/codex-spike/fakeapi.py`, ~60 lines, `127.0.0.1:8931`,
   `wire_api = "responses"`) answering `POST /v1/responses` with a hand-built SSE stream
   (`response.created` -> `response.output_item.done` -> `response.completed`). This drove real codex
   code paths - real sandbox, real approval UI, real hook dispatch - at zero API cost, and produced
   all of Q3, Q4 and the running/waiting/idle captures. The real tool codex exposes is
   `exec_command` (params `cmd`, `sandbox_permissions: use_default|require_escalated`,
   `justification`, `prefix_rule`, `workdir`, `tty`, ...); emitting a `function_call` named `shell`
   is rejected with `unsupported call: shell`. `sandbox_permissions: "require_escalated"` is the
   reliable way to force an approval prompt. Full tool list codex sent: `exec_command`,
   `write_stdin`, `request_user_input`, `view_image`, `multi_agent_v1`, `get_goal`, `create_goal`,
   `update_goal`, `web_search`.

Also confirmed: **`wire_api = "chat"` is rejected outright** in 0.154.0 -
`Error loading config.toml: 'wire_api = "chat"' is no longer supported. How to fix: set
'wire_api = "responses"' in your provider config.` Any custom provider deck documents must be a
Responses-API endpoint.

---

## Implications for a deck Codex adapter

### Known to be possible
- **Hook-based instrumentation with no DANGEROUS flag.** Write `hooks.json` (or pass `-c hooks....`)
  into a deck-owned `CODEX_HOME`, plus a `[hooks.state."<source>:<event>:<g>:<h>"] trusted_hash`
  entry. Verified firing with no flags for both the file and the inline form.
- Hashes are **portable constants** for a fixed hook command: independent of hooks.json path,
  group/hook indices, and JSON formatting/key order. deck can ship a constant per (event, command).
- `PermissionRequest` is a clean "waiting for you" signal with enough detail for a UI reason:
  `tool_name` (`Bash` | `apply_patch`) + `tool_input.command` + optional `tool_input.description`.
- `Stop` gives the final assistant message (`last_assistant_message`).
- `SessionEnd` exists - deck can detect exit without polling. Carries `reason` (observed `"other"`)
  and notably no `model`/`permission_mode`.
- Resume is stable: same `session_id`, same rollout path, `source:"resume"`.
- deck env vars reach hooks unchanged (`DECK_SESSION_ID` etc. visible in the hook process env), so
  hooks can be parameter-free and still know which deck session they belong to.
- Pane scraping works with plain `capture-pane -p`: the TUI does not use the alternate screen.
- The sign-in gate on a fresh `CODEX_HOME` can be pre-satisfied with an `auth.json`; directory trust
  with `[projects."<cwd>"] trust_level = "trusted"`.

### Known to be impossible / blocked
- **Instrumenting an inline hook without pre-seeded trust or the bypass flag.** Untrusted hooks are
  skipped *silently* - no stderr, no exit code, nothing. deck cannot detect the failure from codex's
  output; it must verify by checking that its own hook actually ran.
- **Computing `trusted_hash` from first principles.** Preimage not recovered (Q2c). Any hook command
  that varies per session is unusable unless deck harvests the hash via the PTY trust flow first.
- **Distinguishing sandbox modes from the hook payload.** `permission_mode` collapses to
  `default` / `bypassPermissions` and ignores `-s` entirely.
- **`--full-auto`** - does not exist in 0.154.0; `-a` accepts only `on-request` and `never`.
- **Classifying the retrying-network-error state from the pane** - textually identical to "running".
- **Resuming a session from a different `CODEX_HOME`** unless deck copies the rollout JSONL over.
- **Instrumenting a TUI session before its first prompt** - `SessionStart` does not fire at launch.

### Unverified / open
- The exact `trusted_hash` algorithm (Q2c lists everything ruled out).
- Whether `acceptEdits`, `plan` or `dontAsk` are reachable - likely via `--approve-for-me` or the
  `permission_profile` / `[permissions]` config keys visible in the binary's config field list; not
  tested.
- `SessionEnd.reason` values other than `"other"` (both `/quit` and double-`Ctrl+C` gave `"other"`;
  a crash or SIGKILL was not tested, and a SIGKILLed codex presumably fires nothing at all).
- `PreCompact`, `PostCompact`, `SubagentStart`, `SubagentStop` were instrumented but never triggered;
  only their extracted schemas are known.
- Whether the trust hash is stable **across codex versions** - all measurements are 0.154.0 only.
  This is the main risk in hardcoding constants; deck should treat a non-firing hook as recoverable
  (re-harvest) rather than fatal.
- Whether `[hooks.state]` also accepts an `enabled` key that could disable a trusted hook - the
  binary's `HookStateToml` mentions `enabled` alongside `trusted`, and the TUI offers a per-hook
  on/off toggle, but I did not exercise it.
- Real-provider behaviour: only the 401 error path was observed. Payload
  shapes were confirmed against a stand-in Responses API, not against OpenAI's.
