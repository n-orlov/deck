# Task 213 — modified-navigation keys forward in interactive mode (steer 017 item 1)

## What was wrong

`Ctrl+Left`/`Ctrl+Right` (and every other Ctrl/Shift/Ctrl+Shift-modified arrow, Home, End
and page key) were **silently dropped** in interactive preview mode, not mistranslated.

Bubbletea (`charmbracelet/bubbletea@v1.3.10`) decodes each of these to its **own distinct
`KeyType`** (`KeyCtrlLeft`, `KeyShiftHome`, `KeyCtrlShiftEnd`, ...) — never `KeyLeft` with a
modifier flag set. `internal/tui/interactive.go`'s `interactiveNamedKey` switched only on the
bare types (`KeyUp`/`KeyLeft`/...), so an unmodified arrow matched and forwarded; a modified one
matched nothing and fell through to `return "", false`. `interactiveLiteralPayload` then also
refused it (its cases are `KeyRunes`/`KeySpace`, `0<=Type<=31`, `127`; these enum values are
large negative constants in none of those ranges), so the keystroke vanished with no bytes
written to the pane and no record anywhere of the drop.

## Fix

- `internal/tui/interactive.go`: `interactiveNamedKey` now has an explicit `case` for each of
  the 18 arrow/Home/End combinations plus `KeyCtrlPgUp`/`KeyCtrlPgDown` (bubbletea has no
  Shift/Ctrl+Shift PgUp/PgDown variants — confirmed by grepping its `key.go` enum), mapping each
  to a tmux key name (`C-Left`, `S-Home`, `C-S-End`, ...).
- `internal/tmux/key.go`: `namedKeyAllowlist` gained those same 20 names. Every one was confirmed
  the way the existing entries were — sent (raw, no deck code) via `send-keys` to a pane running
  `cat` on a real tmux server, capture-pane output inspected byte for byte. **This repo's
  `ci/Dockerfile` (a protected path) pins `golang:1.25-trixie`, whose apt `tmux` package is
  `3.5a`**, not the `3.6b` the steer referenced as the host's own tmux — deck has no way to test
  against a newer tmux directly from this repo's own CI without editing a protected path, so
  this task's own confirmation stands at 3.5a, the same version the pre-existing entries in this
  allowlist were confirmed against.

  **Update (steer 020 §3):** the operator independently ran the identical survey method (private
  `tmux -L specver36` socket, pane running `sh -c "stty -echo; cat -v > file"` so the bytes are
  recorded rather than re-interpreted) against a real tmux `3.6b` server on their own host,
  outside this workspace, and reports all 20 names above pass, byte for byte identical to the
  3.5a survey's own sequences (also cross-checked against `charmbracelet/bubbletea@v1.3.10`'s own
  key table, including the two page keys). This is second-hand evidence, source named: it is not
  independently re-verified from inside this repo, and the `ci/Dockerfile`-pins-3.5a-only caveat
  above still stands for what *this suite* can exercise — the allowlist's own comment
  (`internal/tmux/key.go`) now states both facts side by side.
- Alt-modified variants of these (e.g. Ctrl+Alt+Left) are a **known, stated gap, not a silent
  one**: `interactiveNamedKey` refuses any `msg.Alt` key before reaching this switch at all
  (pre-existing behaviour, unchanged here), so no caller can ever produce one of these names for
  an Alt-modified key. Adding the name to the allowlist without a caller that could exercise it
  would be untested surface in a map whose entire point is "nothing here is untested." Recorded
  in both files' comments as a deliberate exclusion, not an oversight.

## Evidence the drop was real (non-vacuousness)

Reverted `interactiveNamedKey`'s new `case` arms only (mechanical removal via a scratch script,
not by hand) and reran `TestInteractiveNamedKeyMapsOnlyModeDependentKeys`:

```
interactive_test.go:318: interactiveNamedKey(ctrl+up) = ("", false), want ("C-Up", true)
... (all 20 cases fail the same way)
--- FAIL: TestInteractiveNamedKeyMapsOnlyModeDependentKeys (0.00s)
```

Then restored the file from a copy and reran clean:

```
--- PASS: TestInteractiveNamedKeyMapsOnlyModeDependentKeys (0.00s)
--- PASS: TestInteractiveLiteralPayloadCoversEveryFixedByteKey (0.00s)
```

`git diff --stat` showed only the intended four files changed before and after this exercise —
the revert-and-restore cycle touched nothing else.

## Tests added

- `internal/tui/interactive_test.go`: `TestInteractiveNamedKeyMapsOnlyModeDependentKeys` extended
  with all 20 new cases, PLUS a new assertion inside the same loop that every name
  `interactiveNamedKey` can produce is on `internal/tmux.IsNamedKeyAllowed` — closing the loop the
  steer's own hazard warns about (a name typed by `interactiveNamedKey` that is not on the
  allowlist would be typed into the agent as literal text by tmux, exit 0, no error).
- `internal/tmux/key_test.go`: new
  `TestSendNamedKeyDeliversModifiedNavigationKeysByTmuxsOwnTranslation`, one subtest per new
  allowlist entry, each asserting the real tmux-3.5a-produced bytes (captured via `capture-pane`
  into a `cat` pane) match exactly what bubbletea's own `sequences` table decodes back to the
  corresponding `KeyType` — so tmux's translation and bubbletea's decoding are proven to agree,
  not merely assumed to.

## What this does NOT cover (recorded, not silently dropped)

- Alt-modified Ctrl/Shift navigation combinations (Ctrl+Alt+Left etc.) — see above.
- Function keys F13-F20 (bubbletea has `KeyType` constants for them but `interactiveNamedKey`
  never mapped F1-F12 either... actually it does map F1-F12; F13-F20 were already unmapped before
  this task and are out of this task's scope — they are function keys, not navigation, and the
  operator's report and the steer's enumeration were both about navigation/editing keys).

## Verification run

```
ci/run.sh sh -c 'go build ./... && go vet ./...'                     # clean
ci/run.sh sh -c 'gofmt -l internal/tmux/key.go internal/tmux/key_test.go \
  internal/tui/interactive.go internal/tui/interactive_test.go'      # clean, no output
ci/run.sh sh -c "go test -count=1 -run 'TestInteractiveNamedKey|TestInteractiveLiteralPayload' \
  ./internal/tui/... -v"                                             # PASS
ci/run.sh sh -c "go test -count=1 -run 'TestSendNamedKey|TestHomeAndEnd|TestNamedKeySend' \
  ./internal/tmux/... -v"                                            # PASS (26 subtests)
```

Whole-suite `./...` was NOT run in this iteration (budget rule: at most once per iteration, and
this iteration also needed the docker-sibling tmux survey); it is exercised in the eventual
re-declaration sweep (steer 017 §0's "final code commit is re-declared and the ten runs
re-collected after it" step) once all four steer-driven feature tasks land.

## SPEC delta

None. `SPEC.md`'s `§11.9` already states, verbatim, "Modified navigation keys forward, like the
unmodified ones, by tmux key name" and "The forwardable set is enumerated and tested key by key,
never left to a default branch" (`395babf`, the operator's own push). The implementation now
matches that text; no further spec change is needed for this item. See
`docs/reports/phase3d-spec-deltas.md` for the running list across all four steer items.
