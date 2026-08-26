# Phase 3f findings

Companion to [`phase3f.md`](phase3f.md) (the per-requirement evidence report for
R63–R73). This file carries what the requirements themselves do not: what
`prds/phase3f-residuals-and-suite-determinism.md` got wrong and the corrected
reading, where `SPEC.md` is ambiguous, every defect found and deliberately **not**
fixed with the reason, and the re-verification of the PRD's "already closed" table.

Nothing here is a requirement verdict — those live in
[`phase3f.md`'s per-requirement table](phase3f.md#per-requirement-table). Nothing
here was fixed silently: every item below is either cross-referenced to the commit
that fixed it or explicitly recorded as open, with why.

Every sha cited resolves under `git cat-file -e` and every path cited exists in the
tree; the checking command is in [§9](#9-how-to-re-check-every-citation-in-this-report).

- [1. What this PRD got wrong](#1-what-this-prd-got-wrong-with-the-corrected-reading)
- [2. Spec ambiguities](#2-spec-ambiguities)
- [3. R68's leak findings](#3-r68-the-two-leaks-issue-5-also-noticed-plus-two-library-findings)
- [4. R66's other-built-in collision survey](#4-r66-every-other-built-in-has-the-same-quantisation-collision)
- [5. R73's framedDialog overflow check result](#5-r73-the-frameddialog-overflow-check-result)
- [6. Defects found and not fixed](#6-defects-found-and-not-fixed-and-why)
- [7. The five already-closed rows, re-verified](#7-the-five-already-closed-bug-log-rows-re-verified-against-this-tree)
- [8. What the field half says about test coverage](#8-what-the-field-half-says-about-test-coverage)
- [9. Re-checking the citations](#9-how-to-re-check-every-citation-in-this-report)

## 1. What this PRD got wrong, with the corrected reading

Five things. The PRD is a good document and none of these invalidated a
requirement; all five are the kind of drift a document acquires between being
written and being executed, and each is recorded with the reading that is actually
true of the tree.

**1.1 R64 named two unsound `starting` waypoints; there are five.** The PRD points
at two frame-read `starting` assertions on `shell` sessions and calls them the
unsound ones. Applying the PRD's own two discriminators (is the session a `shell`?
is the assertion read from the *frame* rather than the store?) to every `starting`
assertion in `features/` found **five** sites with the identical shape —
`create_cwd_ghost.feature:26`, `:42`, `:74`, `:91` and `create_cwd_tab.feature:42`.
Corrected reading: the defect is a *class*, not two lines; fixing the two named
would have left the same mechanism live in three scenarios. All five were replaced
in `677f5a0`. Full classification of every `starting` occurrence in `features/`:
[`phase3f-016-r64-starting-assertion-sweep.md`](phase3f-016-r64-starting-assertion-sweep.md).

**1.2 The `concurrency.feature:21` assertion the PRD pins no longer exists.** The
PRD names it as a Phase 1 store-reading `starting` assertion that must not be
touched. At `60c2c56` and at the tip there is **no `starting` assertion anywhere in
`concurrency.feature`**: the assertion that stood at line 21 was re-aimed to the
durable-row form `the state database contains session "after crash"` in `bd7b9a2`
("Re-aim shell promotion assertions (R30)"), a phase earlier. Line 21 today is the
first line of that commit's explanatory comment; the re-aimed assertion is
`concurrency.feature:23`. The PRD's claim traces to `docs/DELIVERY-LOG.md:412`,
which recorded it while it was still true. Corrected reading: the site is
store-reading and sound either way, so the instruction's *outcome* (leave it alone)
stands — it was left untouched — but the citation is stale and a future sweep
looking for line 21 will find a comment.

**1.3 R67's own file list in the PRD is pre-rename, and stays that way.** The PRD's
`:467-479` lists the eleven test files under their OLD `*_task<NNN>_test.go` names
and its `:480` quotes the OLD duplicate section title `## Task 014 —`. Both were
correct when written and are stale the moment R67 lands (`b848d28`, `e47cb35`).
`prds/` is a protected path for this job, so they are **disclosed, not edited**: the
committed citation sweep prints the PRD's dangling `Task 014` citation as a
`finding (disclosed, not a failure)` on every run rather than letting it pass unseen
([`…/task020-r67-citation-sweep-AFTER.log`](phase3f-020-r67-report-section-titles/task020-r67-citation-sweep-AFTER.log)).

**1.4 One PRD line citation drifted *because of this phase*.** The already-closed
table cites the coalesced-`KeyMsg` split at `internal/tui/tui.go:1830-1863`. That
was accurate at the PRD's own commit (`60c2c56`: the `if msg.Type ==
tea.KeyRunes && len(msg.Runes) > 1 && !msg.Paste {` line is 1831). At the tip
(`a9ff496`) the same block begins at **`tui.go:1990`** and runs to `:2021`, moved
down by R72's confirm dialog and R73's wheel routing. The mechanism is unchanged and
re-verified verbatim in [§7](#7-the-five-already-closed-bug-log-rows-re-verified-against-this-tree);
only the line numbers moved. Recorded because the next phase will read the PRD, not
this paragraph, and 1830 now lands in unrelated code.

**1.5 The PRD's R63-is-the-root-cause hypothesis did not hold.** The PRD makes R63
first-of-order specifically so the stability run measures a tree where "the suspected
cause" of the SIGWINCH flake is gone, and asks the report to say whether R63 alone
removed it. It did not: R63 landed in `f7b97fe`, and `preview.feature:134`/`:147`
still failed `received 1 SIGWINCH signals, want exactly 0` in one of three
whole-suite runs at `e47cb35`
([`task017-r65-full-suite-run3-RED-trimmed.log`](phase3f-evidence/task017-r65-full-suite-run3-RED-trimmed.log))
and in run 9 of ten at `c12c30e`
([`run-9-FAIL-trimmed.log`](phase3f-022-stability10/run-9-FAIL-trimmed.log)).
Corrected reading: R63's overlapping-fit defect was real and is fixed, but it is a
*different* mechanism from the one behind this flake — a pre-resize fit licensed at
the client's default 100x30 whose `previewFit` no-live-pane early return emits
`previewFitDone` anyway and spends the row's one coalesced fit (§6, item F1). The
ordering was still worth honouring: doing R65 first would have destroyed the
information that says so.

## 2. Spec ambiguities

Three, all read the way the standing rules require (`SPEC.md` wins; a disagreement
is a finding, never a spec edit). None blocked a requirement; each is a place where
two honest implementations could differ and the spec does not choose.

**2.1 §11.6 requires legibility after quantisation, not distinctness.** `SPEC.md:1374`
states the tested property exactly: "for every built-in theme, `text`, `hint`,
`title` and each of the seven status tokens must hold a WCAG contrast ratio ≥ 3:1
against `background`… computed over **both** the hex palette and its quantisation to
the reference palette". Nothing in §11.6 says two *different* status tokens must
quantise to two *different* ANSI slots. So a theme in which `idle`, `stopped` and
`archived` all render ANSI 8 — i.e. a 16-colour terminal cannot tell three statuses
apart — is spec-conforming, which is why R66 is scoped to `matrix` alone and why the
four other built-ins in [§4](#4-r66-every-other-built-in-has-the-same-quantisation-collision)
are a finding rather than a bug list. Generalising distinctness is a **spec change**
(a new §11.6 data obligation plus a per-theme test), and this phase deliberately does
not make it. Reference: `matrix.toml` fixed in `ce8ef91`;
[`phase3f-018-r66-matrix-quantised-distinctness.md`](phase3f-018-r66-matrix-quantised-distinctness.md).

**2.2 §11.4/§11.8 do not say whether the wheel may move a read-only overlay
viewport.** What the spec forbids is specific: `SPEC.md:1250-1251` — "**The mouse can
neither cancel nor confirm.** A click outside a dialog does nothing, and no dialog
action is reachable by mouse alone (§11.8)". Scrolling the help overlay is none of
cancel, confirm, or reaching an action, and §11.8 already licenses a *reading*
gesture over a modal surface (drag-to-select). R73 therefore read wheel-over-overlay
as licensed and implemented it (`4edbfc2`) **without loosening the fifteen-flag
action-suppressing guard by one flag** — the conservative reading of the same
sentence — and pinned clicks and drags as still doing nothing for all fifteen
overlays. The ambiguity is recorded, not resolved: a reader who takes "no dialog
action is reachable by mouse alone" to cover *any* mouse-driven state change would
have declined the feature that issue #7 asks for. Reasoning as filed:
[`task013-r73-wheel-routing.md`](phase3f-evidence/task013-r73-wheel-routing.md).

**2.3 §11.4 specifies dialog *width* behaviour and says nothing about height
overflow.** `SPEC.md:1253-1256` pins the width rule (80% of viewport clamped to
`[26, 80]`, plus the below-minimum best-effort rules) and §11.4's contract covers
keys, validation and confirmation — but no sentence says what a dialog does when its
rendered body is *taller* than the frame. The tree answers it two ways: the three
scrollable overlays clip and scroll (`framedDialogScrollable`), while `framedDialog`
neither clips nor scrolls, so three real dialogs draw past the frame at the supported
minimum 80x24 ([§5](#5-r73-the-frameddialog-overflow-check-result)).
With the spec silent, that is a defect by consistency rather than by rule — which is
exactly why it is here and not a requirement.

## 3. R68: the two leaks issue #5 also noticed, plus two library findings

The PRD marks issue #5's two "also noticed" items as findings, not requirements
("Record what you find; fix only if it is trivial and say so"). Neither was trivial;
neither was fixed. Two library-level findings from the same work, and the answer to
the PRD's explicit end-to-end-scenario question, are here as well.

**3.1 Orphaned `/tmp/deck-interactive-pipe-*` — real, still open, wider than the
issue thought.** `ArmPipePane` (`internal/tmux/pipe.go:94`) creates the dir; only
`closeLocal` (`:280`, via `Close`/`CloseLocal`) removes it. The *normal* paths are
clean: `Session.Close` calls `pipe.Close()` and `exitInteractive` calls
`interactiveGrid.Close()`, so `Ctrl+Q` always cleans up. What has **no** cleanup at
all is abnormal exit — nothing on the `tea.Quit` paths (`internal/tui/tui.go:1934`,
`:5032`, `rename.go:53`, as measured at `7d060cd`; `:2101` and `:5404` at the tip
`a9ff496` after R72/R73 moved them) nor in `cmd/deck/main.go` after `Program.Run()` returns tears
interactive mode down, and there is no SIGTERM/SIGHUP handler. So a killed deck —
exactly issue #5's own reproduction, where the wedged process had to be `SIGKILL`ed
from another terminal — leaks the temp dir and FIFO, leaves `pipe-pane` armed, and
leaves the window fitted and ownership held. Not fixed: the fix is `internal/tmux` +
`cmd/deck` lifecycle work (a startup sweep of stale `deck-interactive-pipe-*` dirs, or
moving the FIFO under `DECK_HOME` where the existing leak scan already looks),
outside R68's lock fix and outside the two commits that carry it (`f3c25d5`,
`7d060cd`). Write-up:
[`task002-r68-write-lock.md`](phase3f-evidence/task002-r68-write-lock.md).

**3.2 Preview-switch teardown — checked, no orphan; the issue's observation has a
different explanation.** `enterInteractive` returns immediately when `m.interactive`
is already true (`internal/tui/interactive.go:38`), and the only retarget path — a
mouse press on a different sidebar row — explicitly calls `exitInteractive()` and
*then* `enterInteractive()` on the new row (`internal/tui/mouse.go:286-289`). Keyboard
navigation cannot switch the preview while interactive, because every key is forwarded
to the pane. So no in-process path holds two live `Session`s or drops one without
`Close`. Issue #5's observation (two FIFOs open, one per pid) was **two separate deck
processes**, which is expected. Corrected reading: only 3.1 is a genuine defect.

**3.3 `vt`'s `Emulator.closed` is unsynchronised (upstream, worked around not
fixed).** `Close` writes the field; every `Read`/`Write` reads it, and `SafeEmulator`
deliberately does not lock around `Read`, so any caller that closes an emulator while
another goroutine reads or writes it races. R68 fix 1 therefore retires a displaced
grid by closing the emulator's reply-pipe **write end**
(`io.PipeWriter.CloseWithError(io.EOF)`) instead of calling `Emulator.Close()` — which
matters because `captureLoop` reseeds every 200ms, i.e. five potentially leaked drain
goroutines a second if retirement did not work. The library detail that makes the
workaround sound is pinned by its own test
(`TestEmulatorInputPipeIsAPipeWriter`), so a `go.mod` bump that changes it fails
loudly instead of silently leaking drains. Not fixed here: it is upstream code.
[`task001-r68-reply-drain.md`](phase3f-evidence/task001-r68-reply-drain.md).

**3.4 The fake agent cannot exercise the R68 path at startup — stated explicitly, as
the PRD demands.** `FAKE_CLAUDE_FIXTURE` + `FAKE_AGENT_FIXTURE_DIR` make
`renderThenFallSilent` copy fixture bytes **verbatim** to stdout, so a fixture holding
`\x1b[c` *is* emitted at startup — but that emission happens before deck arms
`pipe-pane`, and the seed comes from `capture-pane` (escapes already stripped). That
is precisely the note issue #5 itself makes about why previewing a deck-launched agent
normally looks fine, and it means a startup DA1 never traverses the wedge path, so no
end-to-end scenario was added. The route that *would* work is `FAKE_CLAUDE_COMMANDS=1`
plus a `{"command":"fixture","name":"…"}` line typed into the pane while interactive
mode is live (the shape `replydrain_test.go`'s `sendLiteralLine` already uses at the
unit seam); it needs a new feature file, new step registrations and a DA1 fixture dir,
and is recorded as follow-up, not done. The wedge itself is covered at the transport
seam by three real-tmux-pane subtests (DA1, DSR, OSC 11) and three stall tests, with
`-race` green over the package (`ok internal/interactive 87.904s`,
[`task002-race-interactive.log`](phase3f-evidence/task002-race-interactive.log)).

## 4. R66: every other built-in has the same quantisation collision

R66 fixed `matrix` only (`ce8ef91`: three authored hexes in
`internal/theme/builtin/matrix.toml`, plus two reconciled entries of
`TestBuiltinQuantizationPinned`'s `"matrix"` map). Before fixing it, the same
nearest-RGB rule the loader uses (§11.6's declared reference palette, `SPEC.md:1372-1373`)
was computed over **every** built-in's seven §7 status hexes. Result: all four other
built-ins have the same class of collision, three of them worse than `matrix` was.

| theme | distinct ANSI slots for the 7 statuses | collisions |
|---|---|---|
| `cobalt` | 4 | ANSI 6 `#00cdcd`: running, starting · ANSI 8 `#7f7f7f`: idle, stopped, archived |
| `daylight` | 3 | ANSI 1 `#cd0000`: waiting, starting, error · ANSI 8: idle, stopped, archived |
| `empire` | 5 | ANSI 8 `#7f7f7f`: idle, stopped, archived |
| `parchment` | 2 | ANSI 1 `#cd0000`: waiting, error · ANSI 8: running, idle, starting, stopped, archived |
| `matrix` before `ce8ef91` | 5 | ANSI 8 `#7f7f7f`: idle, stopped, archived |
| `matrix` after `ce8ef91` | **7** | none |

`parchment` is the extreme case: on a 16-colour terminal five of its seven statuses
paint the same "bright black", so `running`, `idle`, `starting`, `stopped` and
`archived` are indistinguishable by colour.

**Deliberately not fixed, and the reason is a spec reason, not a scope dodge**
([§2.1](#2-spec-ambiguities)): §11.6 requires *legibility* after quantisation, and
every one of these tokens already clears the 3:1 contrast floor over both palettes and
both backgrounds — `TestBuiltinContrastFloor` is green for all five themes. Pairwise
distinctness is a **new** §11.6 obligation; adding it would mean re-authoring four
palettes and generalising
`TestMatrixStatusTokensQuantiseToSevenDistinctReferenceEntries` into a per-theme
test, i.e. a spec change plus four data changes. The standing rules for this phase
also scope R66 to `matrix.toml` alone and require every other built-in and every
golden/`NO_COLOR` frame to stay byte-identical, which they did.

Two consequences of the fix are stated rather than hidden: `hint` and `badge` are not
§7 statuses, so R66 does not reach them and at 16 colours `hint`, `badge` and
`archived` all still render ANSI 8; `idle` now shares ANSI 2 with `dimmed` and `key`,
again not §7 statuses. Survey and arithmetic:
[`phase3f-018-r66-matrix-quantised-distinctness.md`](phase3f-018-r66-matrix-quantised-distinctness.md);
the collision was proved live by restoring the three hexes
([`task018-r66-revert-RED.log`](phase3f-evidence/task018-r66-revert-RED.log)) while the
naive true-colour test still passed on that defective tree
([`task018-r66-truecolour-passes-with-defect.log`](phase3f-evidence/task018-r66-truecolour-passes-with-defect.log)).

## 5. R73: the `framedDialog` overflow check result

R73's leg 3 had to check a claim `framedDialogScrollable`'s own doc comment made —
that every other §11.4 dialog is "bounded by its own field count" and therefore cannot
overflow. **The claim is false: three dialogs can overflow the frame at 80x24**, the
supported minimum. Measured with a throwaway probe test that rendered each
`framedDialog` caller at 80x24 and compared its line count with the frame height
(probe committed as text, not as a test:
[`task014-framed-overflow-probe_test.go.txt`](phase3f-evidence/task014-framed-overflow-probe_test.go.txt),
full output [`task014-framed-overflow-probe.log`](phase3f-evidence/task014-framed-overflow-probe.log),
loadavg `2.69 3.31 3.53`):

```
env editor, 15 variables                   rendered  24 lines, frame height 24 -> fits
env editor, 16 variables                   rendered  25 lines, frame height 24 -> OVERFLOWS
env editor, 60 variables                   rendered  69 lines, frame height 24 -> OVERFLOWS
create, untouched defaults                 rendered  29 lines, frame height 24 -> OVERFLOWS
create, every field filled plus an error    rendered  31 lines, frame height 24 -> OVERFLOWS
bulk delete confirm, 20 marked             rendered  32 lines, frame height 24 -> OVERFLOWS
rename / restart choice / delete confirm / archive confirm / profile / pin -> fit
settings takeover                          rendered  24 lines, frame height 24 -> fits
```

- **`e` env editor overflows from 16 resolved keys up** — one row per key, threshold
  measured exactly between 15 (24 lines) and 16 (25 lines). The likeliest of the three
  to be hit in real use: a config `[env]` table plus a session env map easily exceeds
  15 keys.
- **`n` create dialog overflows at 80x24 even untouched** (29 lines), i.e. at deck's
  documented minimum terminal the whole create modal cannot be shown at all; filling
  every field and provoking a validation error takes it to 31.
- **bulk `dd` confirm overflows once the mark set is large** (20 marks → 32 lines; it
  prints one line per marked session, so ~13 marks is the threshold at 80x24).
- The `settings` takeover is **not** a `framedDialog` (`settingsView` bounds itself to
  `frameSize()` minus borders and footer) and measured exactly 24 lines; `rename`,
  `restart-choice`, `delete-confirm` (single, purge chosen, longest path) and R72's new
  `archive-confirm` all fit with room to spare.

**Not fixed, and why:** R73's criteria put a fix out of scope unless trivial, and it is
not — each of the three needs a stored scroll offset, key and wheel routing, and (for
the env editor and the create dialog) reconciliation with their own cursor/field
navigation, i.e. leg-1-plus-leg-2-sized work apiece. What `9c2e66a` did do is delete
the false claim: `framedDialogScrollable`'s doc comment is corrected in place and now
points here instead of asserting the opposite. Judgement calls and the full table:
[`task014-r73-help-text-and-decisions.md`](phase3f-evidence/task014-r73-help-text-and-decisions.md).

## 6. Defects found and not fixed, and why

Everything found in this phase that is a real defect and was left standing, with the
reason. Two of them are **open flakes that need their own task** and are explicitly
not claimed fixed anywhere in [`phase3f.md`](phase3f.md).

| # | defect | where | why not fixed here |
|---|---|---|---|
| F1 | pre-resize passive preview fit spends the row's one coalesced fit → `preview.feature:134`/`:147` fails `received 1 SIGWINCH signals, want exactly 0` | `internal/tui` `previewFit` no-live-pane early return | product change outside R65's scope (R65 was forbidden to paper it over); needs a task — see below |
| F2 | `TestGoldenMinimumFrame` "frame kept changing after the fixture rendered; not settled" | `features/golden_frame_test.go:236`, reported at the `:74` call site because the helper calls `t.Helper()` | not reproduced at the final tree; open and **unproven fixed**, not retired |
| F3 | `inject.go:51` decides "can this shell pane receive `send-keys`?" with `Exists`, which a **retained dead pane passes** | `internal/service/inject.go:51` | a different defect from #6/R69 (injection into a corpse, not reconciliation); changing it needs its own test and requirement |
| F4 | abnormal exit leaks `/tmp/deck-interactive-pipe-*`, the FIFO, an armed `pipe-pane` and window ownership | `internal/tmux/pipe.go:94`/`:280`, `cmd/deck/main.go` | `internal/tmux` + `cmd/deck` lifecycle work — [§3.1](#3-r68-the-two-leaks-issue-5-also-noticed-plus-two-library-findings) |
| F5 | `vt`'s `Emulator.closed` is unsynchronised | upstream `vt` | upstream; worked around by never calling `Emulator.Close()` — [§3.3](#3-r68-the-two-leaks-issue-5-also-noticed-plus-two-library-findings) |
| F6 | three `framedDialog` dialogs draw past the frame at 80x24 | `internal/tui` env editor, create, bulk delete confirm | leg-sized work apiece; spec is silent on height overflow — [§5](#5-r73-the-frameddialog-overflow-check-result) |
| F7 | four built-in themes collide 2–5 status tokens onto one ANSI slot at 16 colours | `internal/theme/builtin/{cobalt,daylight,empire,parchment}.toml` | §11.6 requires legibility, not distinctness — [§4](#4-r66-every-other-built-in-has-the-same-quantisation-collision) |
| F8 | `phase2b2.md` cites `Task 034` by title; **five** headings in `phase2b2-findings.md` match (`:770`, `:801`, `:849`, `:870`, `:1048`) | `docs/reports/` | R67 scopes only the `Task 014` pair; the committed sweep prints it as a disclosed finding every run instead of letting it pass |
| F9 | the PRD's `:467-479` pre-rename file list and `:480` pre-retitle quotation | `prds/…phase3f….md` | `prds/` is protected for this job — disclosed, not edited ([§1.3](#1-what-this-prd-got-wrong-with-the-corrected-reading)) |
| F10 | `new_session_selection.feature:12-15`'s comment explains row placement via a `starting` attention rank the assertion no longer depends on | `features/new_session_selection.feature` | comment-only staleness; the assertion is sound either way (the anchor is forced to `waiting`), and R64 changes no line whose behaviour is sound |

**F1 in full, because it cost the phase its 10/10.** `preview.feature:134` creates
`beacon` while the client is still at its default 100x30, where a passive fit *is*
licensed, then shrinks the panel to 100x9 and asserts exactly 0 SIGWINCH. The
resize step returns without waiting for deck to process the `tea.WindowSizeMsg`, so
the next keypress can be handled while `m.width/m.height` are still 100x30;
`previewFit`'s floor check passes and issues one fit. Worse, `previewFit`'s
**no-live-pane early return still emits `previewFitDone`**, spending the row's single
coalesced fit on a failed attempt — so the scenario observes 0 only when it wins that
race. Run 9's own frames carry the signature: the crop line reads `61x6 of 61x27`, the
100x30 geometry, not the 100x9 the scenario resizes to. It flaked the same way
**before** R65's settle (`f7b97fe`-era run:
[`task015-r63-features-exacttags-flake.log`](phase3f-evidence/task015-r63-features-exacttags-flake.log)),
so the settle did not cause it — it raises its detection rate, which is what R65 is
for. Evidence: [`task017-r65-full-suite-run3-RED-trimmed.log`](phase3f-evidence/task017-r65-full-suite-run3-RED-trimmed.log)
(1 red in 3 whole-suite runs at `e47cb35`),
[`run-9-FAIL-trimmed.log`](phase3f-022-stability10/run-9-FAIL-trimmed.log) (1 red in 10
at `c12c30e`, at the **lowest** start loadavg of the ten, `1.49` — host load ruled out,
not merely doubted), root cause in
[`phase3f-017-r65-settled-sigwinch-count.md`](phase3f-017-r65-settled-sigwinch-count.md).
**The fix is to restructure the scenario's prefix so no fit is licensed at the larger
size, or to bound the no-live-pane return so it cannot spend the coalesced fit.
Re-baselining the expected counts is forbidden.** This is why R65 is recorded FAILED in
[`phase3f.md`](phase3f.md#per-requirement-table): its assertion criteria are met, its
field-symptom claim is not.

**F2 in full.** Seen once during task 016 at a `-count=10` run of the golden-frame test
([`task016-goldenframe-count10.log`](phase3f-evidence/task016-goldenframe-count10.log));
the same command at the phase's own HEAD did not reproduce it
([`task016-goldenframe-count10-at-HEAD.log`](phase3f-evidence/task016-goldenframe-count10-at-HEAD.log)).
**The test lives in the `features` package, not `internal/tui`** — `features/golden_frame_test.go`,
which is why task 016's own failing log ends `FAIL github.com/n-orlov/deck/features` — so
`internal/tui`'s stability record says nothing about it and is not cited here. The
non-recurrence evidence is the `features` package's own: `features` is `ok` in nine of
the ten stability runs at `c12c30e`
([`run-pass-logs.log`](phase3f-022-stability10/run-pass-logs.log), nine `ok
github.com/n-orlov/deck/features` lines), and the tenth run's **only** failure is F1's
`preview.feature` fit assertion, not this test
([`run-9-FAIL-trimmed.log`](phase3f-022-stability10/run-9-FAIL-trimmed.log): the single
`--- FAIL: TestFeatures/…7-inner-row_floor…` at `:4905`, package verdict at `:4964`; no
`TestGoldenMinimumFrame` failure appears in any of the ten). That is *absence of
recurrence*, not a fix: no code changed to address it, so it stays open. Whoever picks
it up should expect a settle/quiescence gap in the fixture's render path, not a colour
bug.

## 7. The five already-closed bug-log rows, re-verified against this tree

The PRD's "already closed" table is a claim about the tree, and the PRD says so: "If
any row of it is wrong, that finding belongs here." **No row is wrong.** Two came out
differently in their *citations* rather than their substance, and the count itself
needs one clarification.

*Which five.* The PRD's prose says "Seven items were on the operator's list. Five of
them are already fixed in the tree", while the table under it has **seven** rows. The
reading that reconciles them, and the one `docs/DELIVERY-LOG.md`'s reconciliation
(task 025) should use: the five *already-fixed* rows are `probe.miss`, the coalesced
`KeyMsg`, the `crash.feature` SIGKILL hang, `harness.feature`'s "falls silent#01" and
`TestSessionResizeDuringLiveDrainIsRaceFree`. The remaining two rows are not "fixed"
claims at all: the pasted-`dd` row is dispositioned **never exposed**, and the
`phase3e-findings.md §4a/§4c` row points at report items rather than a field defect.

| row | re-verified how, at `a9ff496` unless stated | outcome |
|---|---|---|
| `probe.miss` grows unboundedly | no code path writes a `probe.miss` **event**: the rule is stated at `internal/store/store.go:797` ("a probe miss overwrites `sessions.last_probe_at` ONLY") and pinned by `internal/service/reconcile_test.go:341-374` ("a probe miss must NOT append an event"); the detail view's miss copy is rendered from the column (`internal/tui/tui.go:4458`) | **confirmed** (R59, [`phase3e.md:741`](phase3e.md)) |
| coalesced `KeyMsg` drops both runes | the split block read verbatim at the tip: `if msg.Type == tea.KeyRunes && len(msg.Runes) > 1 && !msg.Paste {` → one single-rune `tea.KeyMsg` per character dispatched through `Update` in order | **confirmed, citation drifted** — `tui.go:1990-2021` at the tip, not `:1830-1863` ([§1.4](#1-what-this-prd-got-wrong-with-the-corrected-reading)) |
| pasted `"dd"` as a delete chord | the `!msg.Paste` guard is on the same line as the split, and `cmd/deck/main.go:121` still builds `programOptions := []tea.ProgramOption{tea.WithAltScreen()}` with no `WithoutBracketedPaste`, passed at `:129` | **confirmed**, and the PRD's `main.go:121` citation still resolves exactly |
| `crash.feature`'s SIGKILL scenario hangs in its after-scenario hook | ten more whole-suite runs at `c12c30e` — **thirty** since the fix, not twenty. No `crash.feature` scenario failed in any of the ten; the only failure in the ten is F1's `preview.feature` fit assertion ([`run-9-FAIL-trimmed.log`](phase3f-022-stability10/run-9-FAIL-trimmed.log), [`run-pass-logs.log`](phase3f-022-stability10/run-pass-logs.log)) | **confirmed and strengthened** |
| `harness.feature`'s "a fake agent renders a preview fixture once and then falls silent#01" | the scenario ran in every one of the ten runs — it is a `Scenario Outline` at `features/harness.feature:212` today, the `#01` being godog's Examples-row suffix — and passed in all ten (run 9's single failure is elsewhere) | **confirmed** (`17e91ce`) |
| `internal/interactive`'s `TestSessionResizeDuringLiveDrainIsRaceFree` goroutine-outlives-test panic | this is the row R68 put most at risk: `f3c25d5`/`7d060cd` rewrote the very machinery the test drives (drain, grid install/retire, the `s.mu`/`s.writes` split). The `sync.WaitGroup` join of `fb9bd71` is still there (`internal/interactive/resize_test.go:253`), and `-race` over the whole package is green: `ok internal/interactive 87.904s` ([`task002-race-interactive.log`](phase3f-evidence/task002-race-interactive.log)) | **confirmed under a rewritten implementation** |
| `phase3e-findings.md §4a`, `§4c` | **not re-verified.** No work in this phase touched tasks 402/403/334's areas, so there is no reproduction attempt to report either way | untested here — stated rather than implied |

One row deserves emphasis for the next phase: the `crash.feature` row's confidence now
rests on thirty clean whole-suite runs, and the hang has not recurred since the task 118
fix — but the mechanism behind it (a coalesced
keystroke matching no `case`) is prevented by exactly one guarded block in `tui.go`, so
a future refactor that moves or narrows the `!msg.Paste` split re-opens all three of the
keystroke rows at once. The three tests that pin it are named in
`internal/tui/tui.go:1990-2021`'s own comment.

## 8. What the field half says about test coverage

*The note the PRD requires, kept short and kept inside its boundary.*

Six defects in one day of ordinary use, against a suite of 306 scenarios and fourteen
test packages, is itself a finding. Five of the six were closed by product changes in
this phase (`f3c25d5`+`7d060cd` for #5, `b8f2513`+`366dd78` for #6, `0745ced` for #9,
`88742b2`+`63d4189`+`9d43a32` for #8, `10f3970`+`4822484` for #10) and the sixth by
`2714d1b`+`4edbfc2`+`9c2e66a` for #7 — all of them in code the suite already covered
heavily. So the gap is not coverage *volume*; it is a missing **shape**.

**Three of them — #6, #8 and #9 — share one shape: a durable row and tmux disagreeing
about liveness, with each guard consulting a different source of truth.** In #6 the
reconcile pass short-circuited on `status == "stopped"` before tmux was ever consulted,
so a retained dead pane was never collected. In #9 a stale `pane_exit_status` removed a
row from reconciliation permanently, again without asking tmux. In #8 an archived row
could be launched because the launch path never asked whether the row was archived,
while hook resolution asked a *display* query that could not see the row at all. Same
shape three times: two answers to "is this session alive?", each read from a different
place, with no scenario that puts them deliberately out of step.

The suite has no scenario family for that shape. Whether it should get one — a family
that constructs the disagreement on purpose (row says X, tmux says Y) and asserts what
every guard does about it — **is the operator's call, and explicitly not scope taken
here.** This phase added the per-defect regression tests the requirements asked for and
nothing wider. The observation is recorded so it is in a document rather than in
nobody's head.

One adjacent data point for whoever makes that call: [F3](#6-defects-found-and-not-fixed-and-why)
(`inject.go:51` deciding "can this pane receive keys?" with `Exists`, which a retained
dead pane passes) is a **fourth** instance of the same shape, found while fixing #6 and
still open.

## 9. How to re-check every citation in this report

The committed checker verifies both this file and
[`phase3f.md`](phase3f.md): every backticked sha resolves under `git cat-file -e`, every
relative markdown link target exists under `docs/reports/`, and every backticked
repo-relative *source* path (a directory-qualified `.go`, `.toml`, `.feature`, `.sh`,
`.sql` or `.md` file, with an optional `:line` suffix) names a file that exists now or
existed at some commit in history — the third check was added because F2 was first
written up against the `internal/tui` package, which has never held
`golden_frame_test.go` (it lives in `features`), and a sha-and-link checker had nothing
to say about it. Its history escape is why the pre-rename names R67 retired still pass
while that miscitation does not; the checker's own comment records the probe that
confirms it still fails on the wrong package.

```
$ docs/reports/phase3f-evidence/check-citations.sh          # run from the repo root
```

Its output for this commit is quoted in this commit's own message. `phase3f.md`'s
section titles are stable anchors: this file links to
[the per-requirement table](phase3f.md#per-requirement-table) rather than to line
numbers, precisely because of [F8](#6-defects-found-and-not-fixed-and-why) and
[§1.3](#1-what-this-prd-got-wrong-with-the-corrected-reading) — by-line citations into
documents that keep growing are what went stale in the first place.

Where this file cites *code* by line, the sha it was read at is named (`a9ff496` unless
stated), for the same reason.
