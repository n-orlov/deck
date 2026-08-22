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

## Requirement 3 under-specified requirement 29's modes/symlink assertions (found by operator steer, 22 Aug 2026 17:45 BST)

Tasks 002/003 implemented `fingerprintDirectory`/`compareFingerprints` exactly to requirement 3's
letter — the four mutation modes requirement 3 names (content change, mtime touch, added file,
removed file) all had a proving subtest, and `compareFingerprints` already compared `Mode` and
fingerprinted a symlink by its own `Readlink` target rather than following it. But requirement 29
goes further than requirement 3 states, asserting the fingerprint proves "same entry list, same
contents, **same modes**, same mtimes" across every destructive path, and neither the `Mode`
comparison branch nor the symlink-not-followed decision had a test that could fail: an operator
reproduction outside this tree, at commit `3896573`, deleted the `Mode != Mode` branch of
`compareFingerprints` entirely and `go test -run TestFingerprint ./features/` stayed green, and
`grep -rn "Symlink\|Readlink" features/*_test.go` found only the instrument's own implementation,
no test exercising it. This was not a compliance miss by the worker — requirement 3 simply never
named "mode change" or "symlink target swap" as mutation modes to prove, even though requirement 29
then relies on both being caught.

**Fix**: `TestFingerprintDetectsMutations` (`features/fingerprint_mutation_test.go`) gained two more
isolated subtests, matching the existing four's discipline of changing exactly one property and
restoring every other:
- `mode change`: chmod a seeded file (0644 → 0444) while restoring its mtime via `os.Chtimes`
  afterwards, leaving content and size untouched. Verified red with the `Mode != Mode` branch
  removed (`compareFingerprints did not detect a mode change`), green restored.
- `symlink target swap`: repoint a symlink at a different file holding byte-identical content,
  then restore the symlink's own `lstat` mtime via `golang.org/x/sys/unix.UtimesNanoAt(...,
  AT_SYMLINK_NOFOLLOW)` (recreating a symlink otherwise bumps its own mtime, which would let the
  existing mtime-touch comparison catch the mutation for the wrong reason and leave the
  not-following decision unpinned). Verified red when `fingerprintDirectory` is changed to
  `filepath.EvalSymlinks` + `os.ReadFile` the resolved target instead of `os.Readlink`
  (`compareFingerprints did not detect a symlink target swap`), green restored.

Also added: `fingerprintDirectory` now records the fingerprinted root directory's own entry under a
reserved key (`"."`), fingerprinting its `Mode` — previously the walk returned early on
`path == root`, so a `chmod` of the cwd root itself (not merely something inside it) was invisible.
The root's own mtime is deliberately **not** fingerprinted: adding or removing any child
legitimately bumps a directory's mtime on every platform this runs on (verified experimentally —
recording it caused the existing `added file`/`removed file`/`symlink target swap` subtests to
report `"." mtime changed` instead of naming the actual differing child, since `compareFingerprints`
returns the first difference in sorted order and `"."` sorts first), which would mask the specific
path every destructive-path scenario in tasks 036/037 needs named. A `root directory mode change`
subtest proves the root's mode is still caught (verified red with the root recording removed).

`features/harness.feature`'s `requirement-29-fingerprint-harness` scenario now seeds a fourth entry
using `seedScratchDirectory`'s existing `mode` column — `readonly.txt` at mode `0444` (readable, not
`0000`, so the fingerprint can still read its bytes) — completing requirement 29's four named entry
kinds (a deck-artifact-shaped name, a dotfile, a directory, and now a file with no write
permission).

## PRD requirement 3/27 vs 29 cross-reference (contradiction, recorded not fixed)

`prds/phase3-sessions-and-lifecycle.md` requirement 3 states: "This is the instrument requirement
**27** is measured with." Requirement 27, however, is `A` archives — an unrelated, non-destructive
flag requirement. The requirement that actually measures the fingerprint instrument is **29** ("The
working directory is sacred... a scenario asserts with requirement 3's fingerprint..."), which this
repository's own code comments and task notes (tasks 002/003/036/037) already cite correctly. This
is a PRD cross-reference slip, not a defect in the tree; per operator instruction it is recorded
here for the operator to correct in the PRD, and is deliberately left unedited in
`prds/phase3-sessions-and-lifecycle.md`.

## SPEC.md

Unmodified by this task — `git diff SPEC.md` is empty (verified below).
