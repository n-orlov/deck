# Task 104 — converge the remaining hard-coded "Create shell session" title waits (F19)

## What F19 was

`internal/tui.createBody` titles the create modal "Create shell session" only
while `shell` is the pre-selected agent (task 024's "(last used)"
pre-selection) and a plain "Create session" otherwise. Every
`WaitForFrame(ctx, _, "Create shell session")` whose real purpose was just
"the create modal is open" therefore hangs for the whole scenario timeout the
moment a non-shell agent was created earlier in the same scenario — the same
class of bug task 101 already fixed for `ensureCreateModalAgent`'s own Agent
wait (dialog-box word-wrap), just triggered a different way here.

## What changed

Every remaining "the create modal is open" wait in `features/*_test.go`
converged onto one of:

- `ensureCreateModalAgent(ctx, client, "shell")` (or another kind) when the
  caller specifically needs an agent selected — this also resets focus back
  onto Name, so no caller's subsequent field navigation had to change.
- a direct `WaitForFrame(ctx, _, "Agent: ")` wait when the caller does not
  care which agent is pre-selected (the Agent row is rendered
  unconditionally by `createFieldRows`, exactly like `ensureCreateModalAgent`
  already relies on).

`clientOpensCreateModalForAgent` (features/agent_steps_test.go:521) is among
the converted sites, as required.

Every site touched carries an inline comment (`// Title-independent (F19): ...`)
naming why the swap is safe/necessary at that call site.

## Surviving literal occurrences

After conversion, `grep -n "Create shell session" features/*_test.go` (full
output in `grep-output.txt`, reproduced below) contains only:

- comments (including the new "Title-independent (F19)" ones explaining each
  conversion, and the pre-existing task-101/025 comments), and
- exactly one real wait: `features/create_blank_name_test.go:85`'s
  `waitForFrameGone(ctx, client, "Create shell session")`. This is a
  `waitForFrameGone`, not an "is the modal open" wait — it waits for the
  modal to *close* on successful submit — and it carries an inline comment
  explaining why the shell title is guaranteed at that point: every scenario
  using `clientSubmitsCreateModalWithBlankName` (`create_session.feature`'s
  blank-name scenarios) submits a blank name without ever touching the Agent
  field, and none of those scenarios ever creates a non-shell session, so the
  pre-selected agent is always shell there.

```
$ grep -n "Create shell session" features/*_test.go
features/agent_steps_test.go:398:	// "Create shell session", which hangs the whole scenario timeout once a
features/agent_steps_test.go:521:	// Title-independent (F19): the modal title is "Create shell session"
features/agent_steps_test.go:551:// "Create shell session" only while the pre-selected agent is shell and a
features/assertions_test.go:779:	// Not a wait on the "Create shell session" title: the modal pre-selects
features/assertions_test.go:815:	// left it at, rather than a literal wait for "Create shell session"
features/assertions_test.go:1057:	// title is "Create shell session" or plain "Create session", so this
features/coalesced_keymsg_test.go:67:	// "Create shell session" title, which only appears while shell is the
features/create_blank_name_test.go:77:	// Literal "Create shell session" kept here (F19): every scenario using
features/create_blank_name_test.go:85:	if err := waitForFrameGone(ctx, client, "Create shell session"); err != nil {
features/create_modal_test.go:46:	// instead of the shell-only "Create shell session" title changes
features/create_reuse_warning_test.go:63:	// "Create shell session" title -- guaranteed here on a fresh
features/create_session_test.go:76:// modal's title, which reads "Create shell session" only while shell is
features/create_session_test.go:151:	// "Create shell session" title, which hangs once a non-shell agent
features/create_tilde_test.go:64:	// "Create shell session" title.
features/create_tilde_test.go:96:	// "Create shell session" title.
features/create_validation_test.go:78:	// "Create shell session" title.
features/determinism_test.go:117:	// "Create shell session" title. The Agent row's own text never
features/determinism_test.go:301:	// "Create shell session" title.
features/dialogs_test.go:228:	// the Agent row instead of the shell-only "Create shell session" title
features/dialogs_test.go:277:	// rather than a literal wait for the shell-only "Create shell session"
features/kill_delete_undo_fingerprint_test.go:54:	// "Create shell session" title.
features/mouse_reenable_after_attach_test.go:31:	// "Create shell session" title.
features/sidebar_width_test.go:101:	// "Create shell session" title.
```

## Proof: the converged path survives a prior non-shell session

`features/create_session.feature`'s new scenario "the create modal converges
on the shell agent even after a non-shell session was created earlier"
(`@requirement-12-title-independent-after-non-shell`) creates a claude
session first, then drives a shell create through
`createShellSessionInLabelledCWD` (one of the converted sites, now using
`ensureCreateModalAgent(ctx, client, "shell")`) and asserts it reaches
"starting" and lands the exact cwd. Before this task's conversion, that
create step's literal wait for "Create shell session" would have hung for
the whole scenario timeout, because the modal's title reads plain "Create
session" right after a claude session was created.

## Validation run

```
$ ci/run.sh env DECK_GODOG_PATHS=event_log.feature,durable_identity.feature,create_session.feature,dialogs.feature \
    go test ./features/ -run TestFeatures -count=1
ok  	github.com/n-orlov/deck/features	35.055s
```

Full log: `godog-run.log`.
