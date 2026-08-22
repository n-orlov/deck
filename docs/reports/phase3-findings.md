# Phase 3 findings

## Transcript path/naming convention, per adapter (requirement 4/32/33 provenance)

Neither adapter's transcript path is invented; each was established against a
real, on-disk artifact and the exact convention is recorded below. Today
neither `internal/agent.Adapter` nor `Caps` exposes a `TranscriptPaths`
lookup — no code path in this tree currently searches for or depends on
finding one. When a real deck adapter needs one (SPEC requirement 32/33,
purge/reap targeting an agent's own transcript file), it must degrade the
same way `cmd/fake-claude`'s `transcriptPath`/`cmd/fake-pi`'s
`findExistingTranscript` already do for their fixtures: a missing `HOME`,
missing project directory, or no matching file is treated as "no transcript
found" and returned as an empty/absent result, never as an error that blocks
the caller's own operation (kill/delete/reap must still succeed even when no
transcript exists to also remove).

### claude

**Convention**: `$HOME/.claude/projects/<cwd, each path separator replaced
with "-">/<conversation id>.jsonl`. No wrapping characters (contrast pi's
`--...--`), no leading-slash stripping distinct from the general separator
replacement — the leading `/` is itself a separator and becomes a leading
`-`.

**How established**: real, authenticated Claude Code 2.1.237's own
`SessionStart` and `UserPromptSubmit` hook payloads carry this path in their
`transcript_path` field (task 2's opt-in `@real-agents` conformance run,
`DECK_GODOG_TAGS=@real-agents go test -run TestFeatures -v ./features/...`).
Repository-relative capture:
[`docs/reports/phase2-real-claude-authenticated.log`](phase2-real-claude-authenticated.log),
lines 50 and 54, e.g.:

```
transcript_path":"/auth/.claude/projects/-tmp-deck-scenario-3258814457-agent-session-cwd/20d34654-9462-4781-8b14-680862724dc7.jsonl","cwd":"/tmp/deck-scenario-3258814457/agent-session-cwd"
```

`cwd` = `/tmp/deck-scenario-3258814457/agent-session-cwd`, encoded segment =
`-tmp-deck-scenario-3258814457-agent-session-cwd` (leading `/` and every
internal `/` replaced with `-`), filename =
`20d34654-9462-4781-8b14-680862724dc7.jsonl` (the same string as
`session_id`, no timestamp component — unlike pi). This is a genuine upstream
capture, not a fixture's own claim about itself: it is the real product
telling deck, via its own hook payload, where it just wrote the file.

**Consequence for `cmd/fake-claude`**: `transcriptPath` in
`cmd/fake-claude/main.go:172-186` reproduces exactly this
(`strings.ReplaceAll(cwd, string(filepath.Separator), "-")`, no wrapping, id
as filename, no timestamp), and degrades to "no transcript" (empty path, nil
error) when `$HOME` is unset rather than erroring — see the doc comment at
`cmd/fake-claude/main.go:117-121`.

### pi

**Convention**: directory
`$HOME/.pi/agent/sessions/--<cwd with a single leading "/" or "\" stripped,
then every remaining "/", "\", ":" replaced with "-">--` (note the literal
`--` wrapping, absent from claude's convention); file
`<ISO-8601 UTC timestamp with ":" and "." replaced by "-">_<session id>.jsonl`
(note the timestamp component, absent from claude's convention — a caller
resuming by id alone must glob for `*_<id>.jsonl` rather than recompute the
name; pi also has no separate `--resume` flag, reusing `--session-id` for
both create and resume).

**How established**: full detail, including the installed npm package's
documented layout, the exact compiled-source encoding function
(`getDefaultSessionDirPath`, `dist/core/session-manager.js:242-246`) and an
end-to-end capture against the real, installed `/usr/bin/pi` 0.84.1 binary
with an isolated `$HOME`/cwd, is already recorded in
[`docs/reports/phase3-fake-pi-transcript-provenance.md`](phase3-fake-pi-transcript-provenance.md)
(written for task 004; not duplicated here). That capture's directory
(`--tmp-pi-provenance-work--` for cwd `/tmp/pi-provenance/work`) and filename
(`2026-08-22T15-06-00-661Z_43ac9425-9b54-4c5d-8063-ac52768d0cdb.jsonl`) match
the stated convention exactly.

**Consequence for `cmd/fake-pi`**: `encodeCwd`/`createTranscript`/
`findExistingTranscript` in `cmd/fake-pi/main.go` reproduce this convention
exactly, and `findExistingTranscript` globs for `*_<id>.jsonl` rather than
recomputing the filename, matching pi's own lack of a separate resume flag.

## SPEC.md

Unmodified by this task — `git diff SPEC.md` is empty (verified below).
