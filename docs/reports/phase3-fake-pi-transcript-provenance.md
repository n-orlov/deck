# Provenance of pi's transcript path/naming convention (requirement 4, task 004)

cmd/fake-pi's transcript writer (`transcriptDir`/`encodeCwd`/`createTranscript`/
`findExistingTranscript` in `cmd/fake-pi/main.go`) reproduces the convention below.
It was established two independent ways, and both agree:

## 1. Documented layout

`docs/session-format.md` ("File Location") in the `pi` npm package installed in
this job's container (`/usr/lib/node_modules/@earendil-works/pi-coding-agent`,
`pi --version` = `0.84.1`) states:

```
~/.pi/agent/sessions/--<path>--/<timestamp>_<uuid>.jsonl
```

where `<path>` is the working directory with `/` replaced by `-`.

## 2. Real binary observation

The same package ships the actual compiled source
(`dist/core/session-manager.js`, `dist/config.js`). The exact encoding function
(`getDefaultSessionDirPath`, `dist/core/session-manager.js:242-246`):

```js
function getDefaultSessionDirPath(cwd, agentDir = getDefaultAgentDir()) {
    const resolvedCwd = normalizePath(cwd); // (unrelated resolution, elided)
    const safePath = `--${resolvedCwd.replace(/^[/\\]/, "").replace(/[/\\:]/g, "-")}--`;
    return join(resolvedAgentDir, "sessions", safePath);
}
```

i.e.: strip a single leading `/` or `\`, replace every remaining `/`, `\` or `:`
with `-`, and wrap the whole thing in a literal `--` prefix and suffix (the docs
page's prose omits the wrapping dashes; the source is authoritative). The
filename (`dist/core/session-manager.js:666-667`):

```js
const fileTimestamp = timestamp.replace(/[:.]/g, "-");
this.sessionFile = join(this.getSessionDir(), `${fileTimestamp}_${this.sessionId}.jsonl`);
```

`timestamp` is an ISO-8601 UTC string (`new Date().toISOString()`-shaped,
e.g. `2026-08-22T15:06:00.661Z`); `fileTimestamp` replaces every `:` and `.`
with `-`.

This was then driven end-to-end against the real `/usr/bin/pi` binary
(`pi 0.84.1`) in this job's container, with an isolated `$HOME` and cwd, to
confirm the source matches what actually lands on disk:

```
$ mkdir -p /tmp/pi-provenance/work && cd /tmp/pi-provenance/work
$ export HOME=/tmp/pi-provenance/home && mkdir -p "$HOME"
$ SID=43ac9425-9b54-4c5d-8063-ac52768d0cdb
$ timeout 15 pi --print --session-id "$SID" --offline "hello"
Warning: No project session found with id '43ac9425-9b54-4c5d-8063-ac52768d0cdb'; creating a new session with that id.
AccessDeniedException: Invalid API Key format: Must start with pre-defined prefix
$ find "$HOME/.pi" -type f
/tmp/pi-provenance/home/.pi/agent/sessions/--tmp-pi-provenance-work--/2026-08-22T15-06-00-661Z_43ac9425-9b54-4c5d-8063-ac52768d0cdb.jsonl
/tmp/pi-provenance/home/.pi/agent/auth.json
/tmp/pi-provenance/home/.pi/agent/models-store.json
```

Contents of the created transcript (header line written immediately, before
any model call — the model call itself failed for lack of a real credential,
which is irrelevant to the path convention being proven):

```json
{"type":"session","version":3,"id":"43ac9425-9b54-4c5d-8063-ac52768d0cdb","timestamp":"2026-08-22T15:06:00.661Z","cwd":"/tmp/pi-provenance/work"}
{"type":"model_change","id":"be59e25f","parentId":null,"timestamp":"2026-08-22T15:06:00.680Z","provider":"amazon-bedrock","modelId":"us.anthropic.claude-opus-4-6-v1"}
{"type":"thinking_level_change","id":"31b60963","parentId":"be59e25f","timestamp":"2026-08-22T15:06:00.681Z","thinkingLevel":"medium"}
{"type":"message","id":"9c0208ac","parentId":"31b60963","timestamp":"2026-08-22T15:06:00.685Z","message":{"role":"user","content":[{"type":"text","text":"hello"}],"timestamp":1787411160684}}
{"type":"message","id":"500d2564","parentId":"9c0208ac","timestamp":"2026-08-22T15:06:01.008Z","message":{"role":"assistant","content":[],"api":"bedrock-converse-stream","provider":"amazon-bedrock","model":"us.anthropic.claude-opus-4-6-v1","usage":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"totalTokens":0,"cost":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"total":0}},"stopReason":"error","timestamp":1787411160717,"errorMessage":"AccessDeniedException: Invalid API Key format: Must start with pre-defined prefix","diagnostics":[{"type":"bedrock_response_failure","timestamp":1787411161008,"details":{"status":403,"errorCode":"AccessDeniedException","requestId":"f3e8460e-1183-4e87-a74d-2e48bbbd9134"}}]}}
```

Directory `--tmp-pi-provenance-work--` matches `strip-leading-slash,
replace-remaining-separators-with-dash, wrap-in-double-dash` applied to
`/tmp/pi-provenance/work`; filename
`2026-08-22T15-06-00-661Z_43ac9425-9b54-4c5d-8063-ac52768d0cdb.jsonl` matches
`fileTimestamp_sessionId.jsonl` with `:`/`.` replaced by `-` in the ISO
timestamp. Both independent sources (documented layout, compiled source) and
the live capture agree.

## Consequence for cmd/fake-pi

`cmd/fake-pi` reproduces the directory/filename convention exactly
(`encodeCwd`, `createTranscript` in `cmd/fake-pi/main.go`). It does **not**
reproduce pi's rich session-tree entry format (model changes, thinking-level
changes, per-message roles/usage/etc.) — that level of fidelity is not needed
by anything deck does with a transcript (deck only needs to prove a
per-conversation file exists at the right path so a later purge/reap can
target it; see requirement 32/33's `TranscriptPaths` capability). The fixture
writes a minimal `{"type":"session",...}` header line on creation and a
`{"message": "..."}` line per turn, exactly mirroring the simplification
`cmd/fake-claude` already made for Claude Code's own JSONL convention.

## Consequence for resuming (pi has no separate `--resume` flag)

Unlike Claude Code, pi's CLI reuses a single `--session-id` flag for both
creating and resuming a conversation (`pi --help`, and confirmed in
`dist/main.js`'s `createSessionManager`). The transcript filename embeds a
creation timestamp that a caller who only knows the id cannot predict ahead
of time, so **resuming means globbing the session directory for a file
ending in `_<id>.jsonl`**, not recomputing the exact name. `cmd/fake-pi`'s
`findExistingTranscript` does this; a real deck adapter's future
`TranscriptPaths` lookup (requirement 32) will need to do the same thing
against the real `~/.pi/agent/sessions/--<cwd>--/` directory.
