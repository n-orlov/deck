# Phase 4c findings (cure-01-08)

Companion to [`phase4c.md`](phase4c.md) (the per-requirement evidence report
for R136-R139): what that report does not carry — the review-raised
behavioural findings this cure wave fixed, where each fix now lives in the
tree (`file:line`), the two known advisory flake classes, and the
iterations 48-54 infrastructure stall. Written at the final code sha
`1ad530b3c63335c6a06ca07950640dafe410d75b`; every commit after it touches
only `docs/` (`da92f63`, `6d861a0`, `8eca5bf`, this file's own commit), so
every `file:line` below resolves identically at `1ad530b` and at this
file's own commit.

## 1. Retained findings — review-raised, cured in place

Each of these was filed as a blocking finding against the original phase
4c landing (`/run/ralphd/review-findings.json`, iteration 64) and cured in
a dedicated `cure-01-0N` commit under operator ruling 001. All are
`curable: true` and none was waived — see `phase4c.md` for the
fail-before test text and probe citations; this section names only where
the fix itself now lives.

### F1 — session-scoped keys were not unconditionally inert on a header

`x`/`dd` bypassed the header guard whenever a mark was already set, and
detail-mode `r`/`l` never asked the shared guard at all (`len(m.sessions)
> 0` says nothing about what the cursor names). Cured in `cdce2641`:

- `internal/tui/rename.go:71` — detail `r` (rename) now calls
  `m.guardSessionScopedKey("r")` before touching a session.
- `internal/tui/rename.go:86` — detail `l` (launch-inputs editor) now
  calls `m.guardSessionScopedKey("l")` the same way.
- `internal/tui/session_scoped_guard.go:69` — `guardSessionScopedKey`
  itself, with the marked-batch exemption removed so `x`/`dd` are inert
  on a header regardless of the mark set.

### F2 — an empty or structural-default group with zero total sessions could not be folded

`c`/left/right on a header required `len(m.sessions) > 0` even though
`cursorGroupID` resolves a header cursor's own id with no session lookup.
Cured in `3529eb90`:

- `internal/tui/tui.go:4063` (`c`), `internal/tui/tui.go:4080` (`left`),
  `internal/tui/tui.go:4089` (`right`) — the stale `len(m.sessions) > 0`
  guard is removed from all three handlers; `!m.help && !m.detail` is
  unchanged.

### F3 — a header cursor painted no visible selection cue

Moving the cursor between two headers left every fully-painted sidebar
line byte-identical (text, gutter, background) — no cue at all, in colour
or monochrome. Cured in `e23bc499`:

- `internal/tui/group.go:650` — `headerSelectionCue` composes a header's
  own gutter/background through the same `sidebarGutterBar`/
  `sidebarSelectionToken` a row's own cue already uses, so a header reads
  as selected via its glyph even under `NO_COLOR`.

### F6 — the re-sort/re-group seam could strand the cursor off-screen or on a hidden stop

Saving `default_group_first` moved the selected row/header cursor's
rendered entry span without re-clamping the scroll offset; a fresh
session load after restart, a rename, or a filter query change could also
normalize onto a hidden or absent stop. Cured in `4e30475c`:

- `internal/tui/settings.go:519` — `settingsApplyLiveFields`'s
  `default_group_first` branch now calls `m.followSelectionViewport()`
  after flipping the flag.
- `internal/tui/tui.go:2594` — `sessionsLoaded`'s reload path calls the
  same `m.followSelectionViewport()` once the preserved/normalized cursor
  is resolved.

### F7 — folding the interactive session's own group dropped its name from the preview border

`previewTitle` resolved the interactive session's name off the CURSOR
(`m.selectedSession()`), and folding that session's own group retargets a
row cursor onto the group's header — silently blanking the border title
while the pane still held the keyboard. Cured in `c864e6b9` (unit
fixture) and `5da35c55` (live-PTY feature scenario):

- `internal/tui/tui.go:5851` — new `interactiveTargetSession`, keyed off
  `m.interactiveWindowTarget` rather than the cursor.
- `internal/tui/tui.go:6290` — `previewTitle` now prefers
  `interactiveTargetSession` over `m.selectedSession()`.

### F8 (residual) — probe-report wording overstated a compile failure and cited the wrong issue

`phase4c-probes/r139.md`'s probe 1 disposition treated a build failure
(the test's 3-arg call needing the `defaultFirst` parameter) as itself
proof the ordering *behaviour* was absent, conflating compile-time API
evidence with runtime behaviour evidence; `r136.md`'s heading cited GH #33
instead of #31. Cured in `2bb61a8b` (docs-only, touches only
`docs/reports/phase4c-probes/`):

- `docs/reports/phase4c-probes/r139.md` — a compiling order-regression
  probe (old default-last body, intact signature) and its quoted runtime
  failure were added alongside the compile-time note.
- `docs/reports/phase4c-probes/r136.md:1` — heading now cites GH #31.

## 2. Evidence findings — closed by dedicated record tasks, not repeated here

F4 (ten-run stability sweep) and F5a/F5b/F5c (phase report, this findings
file plus the DELIVERY-LOG row, and final-code guards) were findings
against *missing evidence*, not product behaviour — `curable: true`,
"curable in place, never grounds for a replan" per ruling 001. Each has
its own carrier task and its own record: `docs/reports/phase4c-stability10/`
(cure-01-06), `docs/reports/phase4c.md` (cure-01-07), this file plus the
DELIVERY-LOG row below (cure-01-08), and `docs/reports/phase4c-guards/`
(cure-01-09). They are not re-litigated here.

## 3. Known flake classes — advisory

The PRD names two flake classes as advisory: a transient-`starting`
assertion and a `SIGWINCH` exact-count assertion. Neither recurred in the
committed sweep: `docs/reports/phase4c-stability10/README.md` records
10/10 PASS across `run-1.log`..`run-10.log`, and `grep -n '^--- FAIL\|^FAIL'`
across all ten committed logs returns nothing — there is no log path to
cite for either class this time, since neither one fired. Both remain
advisory in general per the PRD; a future sweep that does hit one should
publish it with its log path rather than re-run to chase a clean headline.

## 4. Infrastructure — the iterations 48-54 docker-socket stall

Across iterations 48-54 of this run, `/var/run/docker.sock` was absent
from the job container, making `ci/run.sh` (and therefore every CI-lane
command, including validation) unreachable for roughly 73 minutes. This
was a harness/infrastructure condition, not a defect in the product under
test — recorded here per operator ruling 001 with no `file:line`, since
nothing in the tree caused or fixed it. The socket was present again for
every command this cure wave actually ran on the CI lane, including both
the whole-suite gate re-run (`task 022`) and the ten-run stability sweep
(`cure-01-06`): `ls -la /var/run/docker.sock` and `ci/run.sh go version`
were both re-checked live before each, per the standing "re-check, then
proceed" rule, and neither sweep nor gate encountered the outage.

## 5. How to re-check every citation in this file

```
git show --stat --format='' 1ad530b3c63335c6a06ca07950640dafe410d75b
git diff --stat 1ad530b3c63335c6a06ca07950640dafe410d75b..HEAD -- '*.go' '*.feature'   # empty
sed -n '69,90p'  internal/tui/rename.go
sed -n '4055,4095p' internal/tui/tui.go
sed -n '640,660p'  internal/tui/group.go
sed -n '505,520p'  internal/tui/settings.go
sed -n '2585,2596p' internal/tui/tui.go
sed -n '5845,5860p' internal/tui/tui.go
sed -n '6285,6292p' internal/tui/tui.go
grep -n '^--- FAIL\|^FAIL' docs/reports/phase4c-stability10/run-*.log   # empty
```
