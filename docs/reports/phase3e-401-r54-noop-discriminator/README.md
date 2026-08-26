# Task 401 — root-causing why the R54 no-op scenario passes with the guard deleted

## 1. The mutation

`mutation.diff` (applied and reverted below; `git diff HEAD -- internal/` is
empty at the end of this task):

```diff
--- a/internal/tui/mouse.go
+++ b/internal/tui/mouse.go
@@ -285,7 +285,7 @@ func (m Model) clickSidebarRow(index int, e tea.MouseMsg) (tea.Model, tea.Cmd)
 func (m Model) retargetInteractiveSidebarClick(index int) (tea.Model, tea.Cmd) {
-	if !m.interactive || index == m.selected {
+	if !m.interactive {
 		return m, nil
 	}
```

This deletes exactly `index == m.selected` (task 313/R54's own no-op
discriminator, `internal/tui/mouse.go:285`) and nothing else.

## 2. Reproducing the review's PASS

```
ci/run.sh sh -c 'DECK_GODOG_TAGS=@requirement-54-sidebar-click-on-interactive-row-is-a-no-op go test -count=1 -v ./features/'
```

run under the mutation above, exit 0, 3 consecutive times with `-run
TestFeatures` (no `-v`, to avoid the extra I/O `-v` itself adds — see §5's
gotcha about instrumentation overhead): `mutant-pass-run1.log`,
`mutant-pass-run2.log`, `mutant-pass-run3.log`. A fourth run,
`mutant-pass-cited-command.log`, uses the literal `-v` command form from
this task's own success criteria verbatim (`git apply mutation.diff`, run,
then `git checkout -- internal/tui/mouse.go`) and also exits 0. Each ends:

```
ok  	github.com/n-orlov/deck/features	...
```

with the `@requirement-54-sidebar-click-on-interactive-row-is-a-no-op`
scenario reported passed (godog scenario/step counts in the full `-v` log
captured earlier show `1 scenarios (1 passed)`, `21 steps (21 passed)`).

## 3. The mechanism (direct observation, not reasoning)

**Instrumentation used** (`instrumented-mutation-plus-debug.diff`, applied on
top of `mutation.diff`, then fully reverted): every left-button press inside
interactive mode logs its raw `(x, y)` and `hitTest` result
(`internal/tui/tui.go:2409` in the clean tree — the site immediately before
the `retargetInteractiveSidebarClick` call), and every entry into
`retargetInteractiveSidebarClick` logs its `index`/`m.selected`, both to a
file (`/w/401-debug.log`, i.e. `401-debug.log` at the repo root as seen from
inside the `ci/run.sh` sibling) rather than to stdout/stderr — a subprocess's
stderr is swallowed by the godog/PTY harness on a passing scenario and only
surfaces (mid-frame, unusably) inside a *failing* scenario's raw-frame dump,
which is itself a gotcha worth recording (§5).

Running the tagged scenario once with this instrumentation
(`instrumented-run.log`, exit 0 — the scenario still passes with this much
instrumentation) produced `instrumented-debug-events.log` in full:

```
press x=4 y=5 panel=1 target=1 idx=1
retarget index=1 selected=0
press x=38 y=0 panel=2 target=0 idx=0
```

(`hitPanel`/`hitTarget` enums, `internal/tui/mouse.go:22-37`: panel 0 =
`hitPanelNone`, 1 = `hitPanelSidebar`, 2 = `hitPanelPreview`, 3 =
`hitPanelSeam`; target 0 = `hitTargetNone`, 1 = `hitTargetRow`.)

Reading the three lines against the scenario's own steps:

1. **First click** ("clicks on the row containing `retarget-noop-b`",
   scenario's *retarget* step): `press x=4 y=5 panel=1 target=1 idx=1` — a
   real sidebar-row hit, session index 1 (`retarget-noop-b`). This calls
   `retargetInteractiveSidebarClick(1)` with `m.selected == 0`
   (`retarget-noop-a`), confirmed by the very next line `retarget index=1
   selected=0`. The retarget genuinely happens, session B becomes the
   interactive target, and — SPEC's own named safeguard — the preview's top
   border now reads `retarget-noop-b interactive 61x27 fitted ...`
   (`clientPreviewTopBorderContains`, `features/interactive_retarget_test.go`).

2. **Second click** ("clicks on the row containing `retarget-noop-b`" a
   *second* time, the scenario's own no-op step): `press x=38 y=0 panel=2
   target=0 idx=0`. This is **not** a sidebar hit at all — `panel=2` is
   `hitPanelPreview`, at `y=0`, the frame's very first row: the preview's
   own top border. `retargetInteractiveSidebarClick` is **never called a
   second time** — not because its no-op guard stood down, but because the
   press event never reaches the `hit.panel == hitPanelSidebar &&
   hit.target == hitTargetRow` branch (`internal/tui/tui.go:2410`) at all.

**Why the click lands there**: `clientClicksOnRowContaining`
(`features/mouse_bindings_test.go:39`) calls `locateText`
(`features/mouse_bindings_test.go:29`), which does
`strings.Index(line, text)` over `strings.Split(frame, "\n")` **in row
order** and returns the **first** line containing the text. Once session B
is the interactive target (after click 1), the preview's own top border
(frame row 0) *also* contains the literal substring `retarget-noop-b` — the
exact fact `clientPreviewTopBorderContains`'s own doc comment names as
SPEC's safeguard ("the preview's top border therefore carries the target
session's name as text") — and row 0 comes before the sidebar's row for
session B in the split frame. So `locateText` returns the top border's
coordinates, not the sidebar row's, and the synthesized click lands on the
preview panel's border instead.

A press on the preview panel while interactive falls through to the
drag-to-copy switch (`internal/tui/tui.go`'s `case tea.MouseButtonLeft`
block below the sidebar-retarget check): `beginInteractiveSelection` at
`y=0` (the border row, not inside the content box) is a no-op by that
existing, unrelated mechanism — the same one the scenario's own comment
already names as capable of producing a false-negative-proof no-op
("pre-313 code forwards every press ... already a no-op by that unrelated,
pre-existing mechanism"), except here it is not pre-313 REVERTED code doing
it, it is the **test harness's own text-search click helper** doing it
against the fully-313-present, mutated tree.

**Conclusion**: the `@deck_isize_owner` ownership-claim assertion
(`features/interactive_retarget_test.go`) is not being defeated by a hidden
product-side "stand down" in `ClaimWindowOwnership` (the planner's
`r54MutantHypothesis` in `tasks.json`'s `discovered` block is **refuted** —
`ClaimWindowOwnership` writes a fresh random tag on every real claim, exactly
as documented, and would change the option's value if it ran again). The
assertion is defeated one level up: `retargetInteractiveSidebarClick` simply
never runs a second time in this scenario, mutant or not, because the
scenario's second click never reaches the sidebar row it is meant to
target. **The guard being tested is unreachable dead code from this
scenario's own second click, independent of the mutation.**

## 4. Candidate observable channels for task 402

| Channel | Verdict | Why |
|---|---|---|
| `@deck_isize_owner` ownership claim (current) | Sound in principle, defeated by harness | Would discriminate correctly *if* `retargetInteractiveSidebarClick` actually ran a second time; it is the click-targeting, not the discriminator, that is broken. Keep it as the assertion; fix what feeds it. |
| tmux window geometry (`RestoreWindowGeometry`/`FitWindowToPane` size) | Rejected (already, by the scenario's own comment) | A leave-then-re-enter to an unchanged preview size can restore-then-refit back to a byte-identical value with no observable resize; not a real discriminator. |
| A product-side call counter / instrumentation hook | Rejected | Task's own standing rules and SPEC discipline: no test-only instrumentation shipped in product code; this task's own debug prints were fully reverted for that reason. |
| Fix the click's *targeting* so it reliably lands on the sidebar row (scope the text search to the sidebar's own columns/rows, e.g. via the same `seamColumn`/layout-mode helpers `previewTopBorderText` already uses, `features/interactive_retarget_test.go:140`, `features/layout_modes_test.go:46,146`) | **Chosen for 402** | Restores the precondition the existing ownership-claim assertion already assumes: a press that genuinely hits `hitPanelSidebar`/`hitTargetRow` on the already-interactive session. No new assertion type needed — the existing `@deck_isize_owner` check becomes meaningful once the click actually reaches the code path it exists to guard. |

Task 402 should add a sidebar-scoped click step (or scope `locateText`'s
search to rows/columns inside the sidebar panel, excluding the shared
top-border row) so that "clicks on the row containing X" is guaranteed to
hit the sidebar's own row for X even when X's name has also started
appearing in the preview's top border.

## 5. Cleanup

`instrumented-mutation-plus-debug.diff` and `mutation.diff` were both fully
reverted (`git checkout -- internal/tui/mouse.go internal/tui/tui.go`) after
capturing the logs above; `401-debug.log` (a scratch file under the repo
root, git-ignored... actually untracked) was deleted. At the end of this
task:

```
$ git status --short
$ git diff HEAD -- internal/
```

both produce no output (verified below, in the task record).

## Gotchas for later tasks

- A subprocess's stderr is invisible in a **passing** godog run under this
  harness (only a failing scenario's raw-frame dump surfaces it, and even
  then mid-frame and unusably) — use a file under the sibling's `/w`
  (the workspace bind mount) for any throwaway instrumentation, not
  stderr/stdout.
- Debug I/O (opening/writing a file on *every* mouse event, plus `-v`) is
  enough overhead to change this scenario's pass/fail outcome by itself —
  confirmed here: adding `os.OpenFile`+`fmt.Fprintf` to four call sites
  (`ClaimWindowOwnership`, `Release`, `enterInteractive`, `exitInteractive`)
  turned a clean 5/5 pass (task's own `mutant-pass-run*.log`) into a
  consistent fail (`context deadline exceeded` waiting for the frame after
  the *first* retarget click). This is itself evidence for task 403's job
  (the scenario's fixed `200 milliseconds pass` steps are load-sensitive)
  and was kept to the minimum instrumentation (two call sites, file-only,
  no `-v`) needed to get a clean read.
