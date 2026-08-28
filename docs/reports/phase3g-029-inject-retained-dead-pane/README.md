# Task 029 (R88, finding F3) — inject refuses a retained dead pane

`internal/service/inject.go`'s liveness guard decided "can this shell pane take
`send-keys`?" with `TMux.Exists` (has-session), which a retained dead pane —
deck's server runs `remain-on-exit failed`, so a pane that exits non-zero stays
on the socket together with its session, issue #6's trap — passes forever. The
guard therefore never fired for a corpse; the call fell through toward the
`SendKeys` loop instead, and only failed deep inside `tmux.Client.SendKeys`
(via `PreviewPane`'s own dead-pane check) with a generic, unhelpful message
that never says a stopped/error row cannot take an injection or names the way
out.

Fix: the guard now calls `TMux.HasLivePane` — R69's "has a live pane" notion,
already used by `Resume` for exactly this corpse shape — instead of `Exists`,
and the refusal message says so explicitly and names SPEC §6.4's
restart-to-apply route.

## Evidence

- `red-before-fix.log` — `TestInjectEnvRefusesARetainedDeadShellPane` against
  the pre-fix `inject.go` (captured via `git stash push -- internal/service/inject.go`,
  test file kept): fails on the message-content assertion, because the refusal
  that does eventually surface is `tmux`'s own internal "no live pane" wrapped
  message, not the crafted one.
- `green-after-fix.log` — the same test against the fixed `inject.go`: passes.

## Full criterion commands, both green after the fix

- `ci/run.sh go test -count=1 ./internal/service/`
- `ci/run.sh env DECK_GODOG_PATHS=environment.feature go test ./features/ -run TestFeatures -count=1`

## Note: the PRD's own `§6.4` citation

`SPEC.md` §6.2 ("Editing while running") is the section that actually states
the restart-to-apply rule ("a mid-flight edit is inherently restart-to-apply");
§6.4 is "Secrets". The task's own success criteria quote the PRD's R88 bullet
verbatim ("§6.4's restart-to-apply path is the route"), so the refusal message
here cites §6.4 to match that wording exactly. This looks like a citation slip
in the PRD (should likely read §6.2), not a SPEC contradiction — recorded here
rather than silently "corrected" in the message, since the task's literal
wording asked for §6.4.
