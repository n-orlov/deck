# R121 — the session's own launch environment, not the observer's ambient environment

Task **cure-03-02-2** (approach 3's cure pass). Commits `594b0b4` (the fix) and
`bfdb69e` (its override controls pointed at their own distinct roots). Final
tail code sha of this approach: `bfdb69ecd6a00f4dc79475c0fb721453b0bd17b7`.

## The defect

`Model.resolveEnvKey` (`internal/tui/env_editor.go`) answers SPEC §6.1/§6.3's
four-layer question — session env > config `[env]` > `captured_path` (PATH
only) > the environment of the process that started the tmux server — and
`transcriptPathFor` (`internal/tui/tui.go`) resolves every adapter-declared
`Caps().TranscriptEnvKeys` entry through it. Its lowest layer was
`os.LookupEnv(key)`: the **observing TUI process's own ambient environment**.
A tmux server keeps whatever it inherited when it started, for its whole life,
so a later TUI whose own `CODEX_HOME` differs from that server's resolved the
wrong transcript root for a session that server launched — or none at all.

## The fix

`tmux.Client.ServerEnvironment(ctx, key)` (`internal/tmux/geometry.go`) reads
the server's own global environment table (`tmux -L <socket> show-environment
-g <key>`), and `resolveEnvKey`'s server-env branch calls it via
`m.tmuxClient` instead of `os.LookupEnv`. Precedence, default-home behaviour
and not-found behaviour are unchanged; there is no Codex-specific production
branch anywhere (the seam is still purely `Caps().TranscriptEnvKeys`, generic
over any adapter) and the adapter itself still reads no ambient environment of
its own.

## The regression and its controls

`TestTranscriptPathForUsesActualServerEnvironmentNotObserverAmbient`
(`internal/tui/transcript_server_env_test.go`) starts a **real** private tmux
server and pane while this test process's `CODEX_HOME` is root-A, asserts from
the fixture itself that both the server's `show-environment -g` table and the
pane's `/proc/<pid>/environ` really carry root-A, then moves only this
process's own ambient value to root-B (the already-running server cannot
follow it). Four distinct temp roots each hold a copy of the same conversation
id's transcript file, so every layer's verdict is distinguishable:

| sub-test | layer under test | expected root |
|---|---|---|
| `no-override-falls-through-to-the-actual-server-not-the-observer` | server env | serverHome (root-A) |
| `session-override-wins-over-actual-server` | session env | sessionHome |
| `config-override-wins-over-actual-server` | config `[env]` | configHome |
| `session-override-wins-over-config-override` | session over config | sessionHome |

## Failure before repair, success after — the logs in this directory

- `prefix-observer-ambient-red.log` — this test against the **pre-fix** code
  (`internal/tui/env_editor.go`, `internal/tui/tui.go`,
  `internal/tmux/geometry.go` checked out at `594b0b4~1`, this test file kept):
  exit 1, only `no-override-falls-through-to-the-actual-server-not-the-observer`
  FAILs, resolving the observer's own ambient file instead of the server's.
  This is the defect the task names.
- `precedence-inversion-red.log` — this test against the fixed code with
  `resolveEnvKey`'s precedence deliberately **inverted** (the server-env branch
  moved above the session/config branches): exit 1, all three override
  sub-tests FAIL while the divergence sub-test passes. This is what proves the
  positive controls are real controls: before `bfdb69e` both of them assigned
  `CODEX_HOME=serverHome`, so they passed identically whether their own layer
  won or the server-env fallback did, and could not detect this inversion at
  all.
- `green.log` — the same test at `bfdb69e` with no code mutation: exit 0, all
  four sub-tests PASS.

Every log is the verbatim stdout+stderr of

```
ci/run.sh sh -c 'go test -count=1 -run TestTranscriptPathForUsesActualServerEnvironmentNotObserverAmbient -v ./internal/tui/'
```

with its exit status read from the command itself, immediately, never from log
text. Both mutations were made in the live checkout, measured, and reverted;
the tree was clean (`git status --porcelain` empty) before `bfdb69e` was
committed and after.

## Preserved coverage

`transcript_env_layers_test.go`'s three-fixture-tree guard,
`registry_guard_test.go`'s black-box registry-swap/transcript guard, the codex
hook-identity tests and the deletion-path tests that also call
`transcriptPathFor` all still pass unchanged — see
`docs/reports/phase4-a3-final-suite/full-suite.log` (the whole-tree gate at
`bfdb69e`) and `docs/reports/phase4-a3-stability10/` (ten repetitions of it).
