# Claude probe goldens: provenance (R310)

`claude/idle-scrolled.txt`, `idle-draft.txt`, `idle-altscreen.txt` and `running-footer.txt`
**reproduce the pane shapes observed in the field** (a scrolled pane whose startup banner is gone; the same with an
unsent draft in the composer; the alternate screen with no scrollback; a turn in flight with `esc to interrupt`).
They are hand-written reproductions of those shapes, **not byte captures** of a real Claude binary: no real capture
of Claude's footer exists in this repository.

Footer text: only the bypass-permissions mode text, `⏵⏵ bypass permissions on (shift+tab to cycle)`, as quoted by
the phase 4t PRD (R310), is used. The default-permission-mode footer text was **not verified** from repo evidence and
no golden carries it. The rule keys on `⏵⏵` plus `(shift+tab to cycle)`, so it matches any mode text that keeps those two markers.
