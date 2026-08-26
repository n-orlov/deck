# Task 004 — R69 legs 2+3 evidence (issue #6)

Tip before: `b8f2513` (task 003, leg 1). Toolchain: `ci/run.sh` (`deck-ci:local`), go1.25.13.
`/proc/loadavg` at the start of the package run: `10.36 5.00 3.73`; at the end: `10.09 5.32 3.86`.

## Shape chosen: a separate live-pane-aware accessor (not a change to TMux.Exists)

`tmux.Client.HasLivePane(ctx, slug)` (`internal/tmux/tmux.go`) is new: `list-panes -t deck_<slug>
-F '#{pane_dead}'`, true iff at least one pane reads `0`. An absent target is a normal `false`
(as in `Exists`); any other tmux failure is returned. `Exists` is **unchanged**, so no existing
caller's meaning moved.

### Caller sweep of `TMux.Exists` (`grep -rn "Exists(" --include=*.go .`), all five call sites

| call site | what it asks | verdict |
|---|---|---|
| `internal/service/resume.go:69` | requirement 46's already-running adoption | **changed to `HasLivePane`** — this is the bug: `has-session` succeeds on a corpse, so `r` adopted a pane with nothing in it |
| `internal/service/kill.go` (new guard) | "is there still a tmux session to remove?" | **deliberately `Exists`** — a corpse is exactly what kill must be able to remove |
| `internal/service/env.go:36` | should `set-environment -t` be mirrored into the tmux session table? | left on `Exists`: the session's env table exists (and is inherited by a future pane) whether or not the current pane is alive |
| `internal/service/restart.go:46` | is there a tmux session to tear down before relaunching? | left on `Exists`: `R` wants to kill whatever is there, corpse included; a `HasLivePane` here would *skip* killing a corpse and reintroduce the duplicate-session refusal |
| `internal/service/inject.go:51` | can `send-keys` reach a live shell process? | left on `Exists` — arguably it should be `HasLivePane` (injecting into a dead pane cannot work), but that is a *different* defect from #6, is not in R69's scope, and is recorded as a finding candidate rather than changed here |
| tests: `internal/service/rename_test.go:25,44,51` | session-name identity after rename | unaffected (they assert the *name* moved, not liveness) |

## Legs delivered

- **Leg 2** — `resume.go`'s pre-lease check is now `HasLivePane`; a corpse no longer returns
  `ResumeAlreadyRunning`. Because `new-session` would be refused as `duplicate session: deck_<name>`
  while the corpse holds the name, resume now also collects the retained corpse (`Exists` → `Kill`)
  after it owns the launch lease and immediately before `TMux.Create` — SPEC.md:547's "collected on
  sight, never retained", applied where resume cannot order a reconcile pass.
- **Leg 3** — `kill.go`'s guard is now "already stopped **AND** no tmux session exists". This is
  deliberate belt-and-braces: with leg 1 (`b8f2513`) collecting corpses on sight, the branch should
  be unreachable in practice; it is kept because the cost of being wrong is a permanently
  unrecoverable session and the cost of being right is one `has-session` round trip on a refusal.
- **Reachability from the UI** — `internal/tui/tui.go`'s single-row `x` no longer refuses on
  `Status == "stopped"` locally; it delegates the verdict to `service.Kill`, whose refusal text is
  the same string, so a genuinely stopped row still renders `Cannot kill: session is already
  stopped` (via the `sessionKilled` branch). Without this the leg-3 guard would be dead code from
  every real keypress. The bulk `m`+`x` skip is untouched (`mark_test.go` pins it).

## Tests (all new, all discriminating — reverted and reproduced)

| test | fixed | reverted |
|---|---|---|
| `internal/tmux` `TestHasLivePaneSeparatesALiveSessionFromARetainedCorpse` | PASS | with `HasLivePane` delegating to `Exists`: `haslivepane_test.go:92: HasLivePane reported true for a session whose only pane is dead` FAIL |
| `internal/service` `TestResumeRelaunchesARowWhoseOnlyPaneIsARetainedCorpse` | PASS (0.17s) | with `resume.go`/`kill.go` stashed: `retained_corpse_recovery_test.go:109: resume reported ResumeAlreadyRunning for a session whose only pane is a corpse (#6)` FAIL |
| `internal/service` `TestKillRemovesARetainedCorpseAndStillRefusesAGenuinelyGoneSession` | PASS (0.11s) | same stash: `retained_corpse_recovery_test.go:165: kill a stopped row that still has a retained corpse: session is already stopped` FAIL |

Both reverts were `git stash push -- <files>` / run / `git stash pop` (service) and a
`cp` of the pre-edit file back (tmux); `diff` against the pre-revert copies is empty, i.e. the tree
came back byte-identical.

The fixture (`retainedCorpseFixture`) refuses to proceed unless it is the trap #6 described: row
`stopped` from a **hook** with `pane_exit_status` NULL, `Exists` **true** and `HasLivePane`
**false** at the same instant. It also records the corpse's pane id, so the resume test proves a
**new** pane (`%N` differs) rather than adoption.

`TestResumeAdoptsAlreadyRunningTMuxSessionInsteadOfDuplicateError` (pre-existing, `resume_test.go:212`)
is the negative control for requirement 46 and still passes: a genuinely live pane still reports
`ResumeAlreadyRunning`, creates no second tmux session, and records no second launch.

## Commands

```
ci/run.sh go build ./...                                        rc=0
ci/run.sh go vet ./internal/service/ ./internal/tmux/ ./internal/tui/   rc=0
ci/run.sh go test -count=1 ./internal/service/ ./internal/tmux/ ./internal/tui/
  ok internal/service 3.677s   ok internal/tmux 19.959s   ok internal/tui 0.653s
ci/run.sh env DECK_GODOG_TAGS='@requirement-46' go test -count=1 ./features/          ok 17.816s
ci/run.sh env DECK_GODOG_TAGS='@requirement-46-interactive-fitted-geometry' ...        ok 18.107s
ci/run.sh env DECK_GODOG_TAGS='@requirement-22-undo-toast' go test -count=1 ./features/ ok 47.722s
```

**Honesty note on the tag as the task worded it:** `DECK_GODOG_TAGS='@requirement-46'` selects
**no scenarios at all** (`No scenarios / No steps`, see
`task004-godog-requirement46-exact.log`) — godog matches tags exactly and the real tag is
`@requirement-46-interactive-fitted-geometry`. Its green is therefore vacuous; the run that carries
evidence is the suffixed tag (1 scenario, 9 steps, passed —
`task004-godog-requirement46-suffixed.log`) plus `@requirement-22-undo-toast` (11 scenarios, all
passed — `task004-godog-requirement22-undo-toast.log`), which drives the real `x` refusal wording
through a real keypress and so covers the `tui.go` delegation end to end. loadavg at that run:
`9.69 7.14 4.85`.

## Follow-up recorded, not done

`internal/service/inject.go:51` uses `Exists` to decide whether a shell pane can receive
`send-keys`; a retained dead pane passes that check and the injection cannot work. Out of R69's
scope — candidate finding for task 024.
