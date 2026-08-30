# Task 007 — guard the shell-liveness promotion write with `AllowedCurrentStatuses`

## The guard, and nothing else

```
$ grep -n -B8 'EventKind: "tmux.shell_live"' internal/service/reconcile.go
```

```
118:				if session.Agent == "shell" && session.Status == "starting" {
119:					if err := s.Store.UpdateSessionStatus(ctx, store.StatusUpdateInput{
120:						SessionID: session.ID,
121:						Status:    "running",
122:						Reason:    "tmux pane is alive",
123:						Source:    "tmux",
124:						At:        s.Clock.Now().UnixMilli(),
125:						AllowedCurrentStatuses: []string{"starting"},
126:						EventKind: "tmux.shell_live",
```

`git show --numstat HEAD -- internal/service/reconcile.go`:

```
1	0	internal/service/reconcile.go
```

One line added, zero removed. The product change is exactly one new struct field on the
existing `StatusUpdateInput{...}` literal: `AllowedCurrentStatuses: []string{"starting"}`. No
env knob, no test-only branch, no other line touched (the pre-existing `EventKind` line is
untouched, byte-for-byte, in the diff above).

## Why this is the right guard, and what it enforces

`store.StatusUpdateInput.AllowedCurrentStatuses` (store.go:325-328) is a caller-declared,
exhaustive transition policy: `UpdateSessionStatus` (store.go:642) refuses the write — the
`apply` short-circuits false — whenever the row's live `status` at write time is not in the
list, i.e. it re-checks the *precondition the caller already believed was true when it
decided to write* against what the row now actually holds, atomically with the write itself.

Setting it to `[]string{"starting"}` on the shell-liveness promotion encodes §7's own
transition-table row, word for word:

> `starting` \| pane is alive, **`shell` rows only** \| `running` — see the shell rule below

and its prose gloss:

> For `shell` rows only, tmux liveness therefore promotes `starting → running`... This is the
> one place `tmux` supplies more than liveness, and it is sound precisely because no
> higher-precedence source exists for a shell **that could ever contradict it**.

The reconcile loop already gates entry to this branch on `session.Status == "starting"`
(line 118) — but that read happens before the write, and §7's own precedence rule —
`user-terminal > hook > probe > tmux` — is exactly what can move the row to something else
(e.g. a hook-sourced `error`) in the gap between that read and the `UpdateSessionStatus`
call below it. Without `AllowedCurrentStatuses`, the store has no way to tell "the row is
still `starting`, as tmux believed" from "the row moved on while tmux's write was in
flight", and applies the stale `running` write either way — task 006's
`TestReconcileLosesInterleavedStatusWriteDuringShellPromotion` forces exactly that
interleaving with a rendezvous-blocked `list-sessions` and shows the row clobbered from
`error`/`hook` back to `running`/`tmux`, a `tmux`-sourced overwrite of a `hook` verdict in
direct violation of the precedence rule quoted above. `AllowedCurrentStatuses:
["starting"]` closes that gap by making the *same* precondition the code branch already
checks also the precondition the store enforces atomically at write time: if the row is no
longer `starting` when the write actually lands, the write is refused instead of applied,
and the higher-precedence verdict already on the row stands. No other status needs to be in
the list, because §7 permits this promotion from `starting` alone — nothing else is a valid
source state for it.

## §7 precedence rules re-checked

- **The `starting → running`, shell-only transition row** in §7's transition table
  (`SPEC.md`, "## 7. Status model") — the guard's list, `["starting"]`, is exactly that
  row's `from` column, no more and no less.
- **`user-terminal > hook > probe > tmux`** (§7, "Rules: **Precedence:**") — the guard is
  what makes the *shell* promotion respect this rule under interleaving; it was already
  respected by construction for agent rows, which never enter this branch.
- **"A terminal row that denies a live pane is an invariant violation, and the same pass
  repairs it"** (§7) — re-checked to confirm this guard does not touch that repair path
  (`repairTerminalRowWithLivePane`, reached earlier in the same loop body, lines ~102-105);
  it is a different branch, a different precondition (`stopped` or a `pane_exit_status`/
  `tmux`/`user`-sourced `error`), and is untouched by this change.
- **"A hook- or probe-sourced `error` with no `pane_exit_status` is not a violation, and is
  never repaired"** (§7) — re-checked because this is precisely the row shape task 006's
  test seeds (`error`/`hook`, no `pane_exit_status`) and precisely the row this guard now
  protects from the shell-liveness write; before this task the shell-liveness code path was
  the one place that *did* clobber it, contrary to this rule.

## Green-after

| # | Command | Exit | Log |
|---|---|---|---|
| a | `ci/run.sh go test -count=1 -run TestReconcileLosesInterleavedStatusWriteDuringShellPromotion ./internal/service/` | 0 | `a-targeted.log` / `a-targeted.exitstatus` |
| b | `ci/run.sh go test -count=1 ./internal/service/ ./internal/store/` | 0 | `b-service-store.log` / `b-service-store.exitstatus` |
| c | `ci/run.sh env DECK_GODOG_PATHS=attention_sort.feature go test ./features/ -count=1` | 0 | `c-attention-sort.log` / `c-attention-sort.exitstatus` |

`attention_sort.feature` unedited by this task:

```
$ git diff --exit-code a24ff8d..HEAD -- features/attention_sort.feature
$ echo "exit: $?"
exit: 0
```

## Task 003's three scenarios are unaffected

The guard only changes behaviour when the row's status at write time differs from what the
branch's own `session.Status == "starting"` check observed a moment earlier — i.e. only
under the same forced interleaving task 006's test constructs. None of task 003's three
named scenarios (`status_attach.feature`, `status_claude_hooks.feature`,
`status_probe.feature`) exercise that interleaving; each was re-run individually against
this commit and stayed green:

```
$ ci/run.sh env DECK_GODOG_PATHS=status_attach.feature go test ./features/ -count=1
ok  	github.com/n-orlov/deck/features	19.255s
$ ci/run.sh env DECK_GODOG_PATHS=status_claude_hooks.feature go test ./features/ -count=1
ok  	github.com/n-orlov/deck/features	21.532s
$ ci/run.sh env DECK_GODOG_PATHS=status_probe.feature go test ./features/ -count=1
ok  	github.com/n-orlov/deck/features	20.400s
```

Unaffected because none of those three scenarios drive a `shell`-agent row through a
concurrent `starting → running` promotion racing a competing status write; they exercise
agent-hook, probe and attach paths that never reach the guarded branch at all, or reach it
only in the ordinary, non-interleaved case where the precondition the store now re-checks
is still true.
