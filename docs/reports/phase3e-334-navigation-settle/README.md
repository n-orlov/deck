# Task 334: settle-race defect class across `selectRowByName` and `clientOpensDetailForSession`

Per operator steer `3e-002`. `7ebafce` (task 324) fixed the settle race in
`selectSessionByNameThenSend` alone; this task collapses all three navigation helpers
(`selectSessionByNameThenSend`, `selectRowByName`, `clientOpensDetailForSession`) into one
top-anchored helper (`navigateToRowByName` + `sendNavKeySettled`,
`features/navigation_settle_test.go`) that settles every keystroke on an observable consequence
instead of a fixed sleep, and adds a source-scan guard test
(`features/navigation_settle_guard_test.go`, `TestNavigationHelperNeverSleeps`) banning
`time.Sleep` in that file.

## The fix

`sendNavKeySettled` sends one key, then polls (the same `d.updated`-channel idiom
`ScreenDriver.WaitForFrame`/`WaitForFrameGone` use, `features/pty_driver_test.go:444,:467`) until
the sidebar's own selected (`"> "`-prefixed) row line — `selectedSidebarLine`, scoped to the first
6 columns of each line, since the sidebar is always the frame's leftmost panel — differs from its
pre-keystroke value, bounded by a 300ms per-key ceiling (`navKeySettleWindow`) that only matters
for a genuine no-op keystroke (arrow at the top/bottom edge, or `"g"` already at the top).

`selectRowByName`'s stale "there is no bound go-to-top key" doc comment is removed along with the
20-upward-arrow rewalk it caused in two callers (`status_probe_test.go`, and the old standalone
`selectRowByName` itself); all three helpers now reset with `"g"` like
`selectSessionByNameThenSend` always did. `selectSessionByNameThenSend`'s stale `SPEC.md:952`
citation is replaced with the quoted text (`` `g`/`G` top/bottom ``), since amendment `2c56309`
moved that line to ~1062.

## A bug caught by this task's own testing, and its fix

A first version of `sendNavKeySettled` handled "no row currently selected in the visible frame"
(the selection scrolled off-screen after a layout-mode cycle) by falling back to
`client.WaitForFrame(waitCtx, false, "> ")` — wait for *any* `"> "` to appear anywhere in the whole
frame. That is unsound: earlier create-dialog renders leave `"> Name: ..."`/`"> Working
directory: ..."` field-marker text on rows a later, shorter render never overwrites, and an
unscoped `"> "` search can match that stale leftover before deck has processed the keystroke at
all.

This was caught, not theorised: `features/mouse.feature`'s
`@requirement-34-wheel-scrolls-without-selecting` scenario (creates 5 sessions, cycles layout mode
twice until the selected row scrolls off-screen, then calls `selectRowByName` to re-select the
first) went flaky under it.

- `wheel-scroll-unscoped-fallback-red.log`: 5/5 failed (`FAIL`, 7.4-8.0s each) with the unscoped
  fallback in place.
- `wheel-scroll-scoped-fix-green.log`: 5/5 passed (`ok`, ~1.4s each) after replacing the fallback
  with a direct comparison of `selectedSidebarLine`'s scoped snippet to its own prior value (never
  the whole frame) — the fix actually shipped.
- Baseline (pre-334) comparison: 4/4 passed at ~1.9s each (not logged to a file, reproduced
  interactively during triage; the baseline's fixed-sleep implementation does not hit this
  particular race because it never introduced the unscoped-frame check in the first place).

## Guard test red/green

- `guard-test-red.log`: `TestNavigationHelperNeverSleeps` FAILs when a `time.Sleep(time.Millisecond)`
  is temporarily inserted into `navigateToRowByName`.
- `guard-test-green.log`: passes once removed (the shipped state).

## Red-proof attempt per pre-existing site, per steer 3e-002's own escape clause

> "if no deterministic red proof is achievable (flake only under load), say so explicitly in the
> report rather than manufacturing one, and rely on the guard test alone for that site"

**`selectRowByName`** (5 call sites: `status_probe_test.go:301`, `agent_steps_test.go:1672`
backing `lease_race.feature`, `attention_sort_test.go:118`, `status_claude_hooks_test.go:286`, and
`clientSelectsSessionByName`'s callers across `dialogs_test.go`/`env_editor_test.go`/many
`.feature` files): reverted task 334 in place (restoring the pre-334 three-separate-helpers shape,
verified `git diff` against `HEAD` empty on the four touched `_test.go` files before proceeding),
temporarily tagged `features/status_claude_hooks.feature`'s two-session "A pane-fired hook cannot
override a user-terminal verdict" scenario `@debug-tmp-334` (tag removed before this commit, like
`7ebafce`'s `@debug-tmp-verify` precedent), and ran it under induced CPU contention mirroring task
325's discriminating-load method: 16 parallel `ci/run.sh` sibling containers, 3 repeats each (48
total runs) — see `selectRowByName-redproof-attempt/redproof_contention_*.log`. **All 48 passed;
no repro.** Every scenario reaching this helper has too short a walk distance (0-2 hops between
sessions) to reliably expose a settle race even under contention — the old 25ms fixed sleep is
generous relative to that few keystrokes. Not fixed by manufacturing a new, longer-walk scenario
for the sole purpose of the demo (steer 3e-002 forbids "manufacturing" a red proof); relies on the
guard test alone for this site.

**`clientOpensDetailForSession`** (12 call sites: `agent_session.feature`\u00d71, `crash.feature`\u00d72,
`dialogs.feature`\u00d76, `launch_lease.feature`\u00d71, `permission_modes.feature`\u00d71,
`status_claude_hooks.feature`\u00d71): every one of them creates exactly one session before opening
its detail view, so the walk distance is always 0 — the marker is already on-screen before any
down-arrow is ever sent, in every committed scenario. No existing scenario can expose a
multi-keystroke settle race here at all, deterministically or under load; relies on the guard test
alone for this site.

## Verification after landing

- `ci/run.sh go build ./...`, `ci/run.sh go vet ./...`, `ci/run.sh gofmt -l` on tracked `.go`
  files: all clean.
- `ci/run.sh go test -count=1 ./features/ -run TestNavigationHelperNeverSleeps -v`: PASS.
- `ci/run.sh go test -count=1 ./features/` (whole package, unfiltered, serial): `ok` in 289.546s —
  `features-green.log`. (An earlier full run at the intermediate, unscoped-fallback state failed
  exactly the wheel-scroll scenario above and nothing else — not saved to a file, reproduced
  interactively during triage; superseded by the clean run cited here.)

## Findings

See `docs/reports/phase3e-findings.md` §4c for the durable record: mechanism, fix shape, and which
sites got a deterministic red proof (the guard test) vs. guard-test-only coverage (both
pre-existing sites, `selectRowByName` and `clientOpensDetailForSession`).
