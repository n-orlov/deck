# Claude probe goldens: provenance (R310)

`claude/idle-scrolled.txt`, `idle-draft.txt`, `idle-altscreen.txt` and `running-footer.txt`
**reproduce the pane shapes observed in the field** (a scrolled pane whose startup banner is gone; the same with an
unsent draft in the composer; the alternate screen with no scrollback; a turn in flight with `esc to interrupt`).
They are hand-written reproductions of those shapes, **not byte captures** of a real Claude binary: no real capture
of Claude's footer exists in this repository.

Footer text: only the bypass-permissions mode text, `⏵⏵ bypass permissions on (shift+tab to cycle)`, as quoted by
the phase 4t PRD (R310), is used. The default-permission-mode footer text was **not verified** from repo evidence and
no golden carries it. The rule keys on `⏵⏵` plus `(shift+tab to cycle)`, so it matches any mode text that keeps those two markers.

## Waiting dialog goldens (R312)

`claude/waiting-edit.txt`, `waiting-create.txt` and `waiting-trust.txt` are hand-written reproductions, not byte
captures: no real capture of a Claude dialog exists in this repository (`claude/waiting.txt`, the
`Do you want to proceed?` permission dialog, is hand-written too). Their markers are the ones `claude_gate.go`'s
dialog list already names (`Do you want to make this edit`, `Do you want to create`, `Do you trust the files in
this folder`); the goldens add only those marker lines and a `Yes`/`No` option list, nothing else about the
layout is claimed.

Not covered, no golden and no rule, because the repository holds no evidence of the marker text: an
**AskUserQuestion** choice list (question/elicitation), **plan approval**, and a **write**-file confirmation
wording. The searches that found nothing are recorded in the petition filed for them (SPEC §7).
