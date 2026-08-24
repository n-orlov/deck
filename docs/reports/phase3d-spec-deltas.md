# phase3d SPEC deltas — steer 017/018/019's four operator requirements

Steer 018 §0 withdrew its own earlier instruction to edit `SPEC.md` directly (a protected path)
and asked instead for an apply-ready delta list here whenever the operator's own SPEC push
(`395babf`) got something wrong or left a gap. This file is written for every one of the four
items this run attempts, including any that end up deferred — the delta is the durable half.

## Item 1 — modified-navigation keys (task 213)

**No delta.** `SPEC.md` §11.9 (post-`395babf`) already states:

> Modified navigation keys forward, like the unmodified ones, by tmux key name. ... The
> forwardable set is enumerated and tested key by key, never left to a default branch.

The implementation (task 213) now matches this exactly: 20 new names on
`internal/tmux/key.go`'s `namedKeyAllowlist`, 20 new `case` arms in
`internal/tui/interactive.go`'s `interactiveNamedKey`, both confirmed against a real tmux server
and cross-checked against each other by test. No contradiction found between the operator's SPEC
text and the shipped behaviour.

One thing worth flagging as a possible future delta, not raised as an error: the spec text does
not say what happens to an Alt-modified Ctrl/Shift combination (e.g. Ctrl+Alt+Left). The
implementation's answer — refused, same as any other Alt-modified key, a deliberate and stated
gap because `interactiveNamedKey` refuses `msg.Alt` before reaching the modified-navigation
switch at all — is consistent with "enumerated and tested key by key" (an untestable name is not
added), but the spec prose does not call this out explicitly. Not proposing a change; noting it
in case the operator wants the spec to say so.

## Items 2-4

Not yet attempted this iteration. Will be appended here as each lands, per steer 018's cut order
(yolo de-gate, then preview-fit-on-navigation, then drag-to-copy selection).
