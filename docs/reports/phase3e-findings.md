# Phase 3e — findings

Per `prds/phase3e-list-ergonomics-and-chrome.md`'s deliverables list and task 327's own success
criteria: every PRD claim found wrong, every SPEC ambiguity hit, and every defect found but not
fixed (with the reason), so none of it is left only in a commit message.

## 1. PRD claim found wrong

### R56's named "exactly one dark and one light" trap does not exist

`prds/phase3e-list-ergonomics-and-chrome.md:255-258` states, as "the trap, named in advance":

> **`internal/theme/theme_test.go:61` `TestBuiltinsHaveOneDarkOneLight`** asserts *exactly* one
> of each. It must be **relaxed to "at least one of each"** — which is what the amended spec now
> says — and the diff must be exactly that.

This is wrong as written. Reading the actual test (`internal/theme/theme_test.go:61-83`) before
task 316 touched anything: it checks `len(all) < 2` (fails under 2), then walks every built-in
and sets a `dark`/`light` boolean per appearance seen, then fails only if `!dark` or `!light` —
i.e. it already asserts "at least one dark, at least one light", not "exactly one of each". A
third or fourth built-in of either appearance does not touch it. Only
`TestBuiltinQuantizationPinned` (`internal/theme/quantize_test.go:58`), which pins every
built-in's exact quantisation, needed a new entry per new theme.

This was caught before task 316 started (recorded in `notes.md`'s "PRD spot-check corrections"
section as it stood at the time) and confirmed again in task 316's own work: "`git diff` proof:
`TestBuiltinsHaveOneDarkOneLight` needed **no change**" (`docs/reports/phase3e.md`, R56 section,
task 316). No test was weakened, relaxed, or touched for this reason — the PRD's own instruction
("the diff must be exactly that") describes a diff that was never made because the premise for
making it was false.

**Why this matters beyond "the PRD was imprecise":** the PRD frames this as a trap a worker will
"fail the moment you add a third theme" unless the relaxation is made. Following that instruction
literally — weakening a test's assertion in response to a claim about its current behaviour,
without first reading the test — would have been a real regression: it would have hidden a
regression toward zero-dark or zero-light built-ins as `>= 2` themes, an assertion strictly
*weaker* than what the test already enforced, for no reason, since the test's actual behaviour
already satisfied the amended spec's "at least one of each" requirement with no change at all.

## 2. SPEC gap found before the amendment (steer 3e-001 §2b) — recorded per the operator's explicit instruction to record it "regardless of the fix"

**`probe.miss` (an events row) and `sessions.last_probe_at` (a column) were never part of
`SPEC.md`'s `events.kind` vocabulary before amendment `6584299`.** `SPEC.md:289`'s `kind` comment
enumerated `started|prompt|waiting|idle|error|ended|resumed|killed|env|note` — ten kinds,
`probe.miss` among none of them. Both were introduced by an earlier phase's task 009, entirely
without SPEC backing, specifically so the `i` detail dialog could distinguish "sampled, no rule
matched" from "never sampled". The column (`last_probe_at`) is legitimate and is read
(`internal/tui/tui.go:4037`, `session.LastProbeAt > session.StatusAt`); the event
(`probe.miss`) has no reader anywhere in the tree except `eventLogView` itself, which the write
path then goes on to make unusable.

On the operator's live database this unspecified event kind grew to **3,738,363 of 3,741,571
rows (99.91%)** of the entire audit trail, at a sustained ~13.57 rows/second across ten `claude`
sessions with no terminating condition (a miss is recorded on every reconcile tick, forever, for
any stale-eligible session no §7 rule matches) — and, compounded with no index on `events` and a
store read inside `eventLogView`'s render path, wedged the operator's client completely (Esc
included, because the Esc keystroke queued behind an unbounded backlog of tick-driven renders
each demanding a 1.834s unindexed scan+sort of the resulting table).

Amendment `6584299` (§3 of `steer 3e-001`, applied verbatim, commit `2c56309`) regularises the
legitimate half (declares `last_probe_at` in the `sessions` DDL) and forbids the illegitimate
half (a new §7 paragraph: *"A diagnostic sampling result is a column, not an event... MUST NOT
append to `events`"*, plus the general rule *"a kind is only written if something reads that
kind, and the reader is named where the kind is introduced"*). Tasks 329-332 (commits `9f73996`,
`faba630`, `b448d19`, `8d63481`) implement the four requirements this gap produced (R59-R62); see
`docs/reports/phase3e.md`'s R59-R62 sections for the full evidence. This finding is recorded here
as the operator's steer required, independent of whether the fix is judged adequate: an
unspecified event kind reaching 99.91% of a supposedly append-only-for-a-reason audit trail is a
process gap (no SPEC review caught it when task 009 introduced it), not merely a bug in the code
that resulted.

## 3. SPEC ambiguity hit

**None was hit that required a judgement call beyond what amendment `6584299` already specifies
precisely.** This was checked deliberately, not assumed: the two places in this phase's work that
looked most likely to need one turned out not to.

- **R57's "neither panel focused" state.** `SPEC.md`'s amended §11.3 says only "a dialog that
  opens takes focus and the sidebar's border reverts" — it does not enumerate every UI state that
  counts as "neither panel focused". In practice this needed no interpretation: every overlay in
  this tree other than the theme picker (`t`) and the filter input (`/`) already replaces `View()`
  outright (see `View()`'s existing early-return chain, predating this phase), so `mainView` — and
  therefore the seam — never renders under them at all. Only the theme picker and filter genuinely
  keep `mainView` rendering underneath while owning the keyboard (each by its own pre-existing file
  comment, task 025/123), so `mainViewOverlayActive()` (`internal/tui/panel.go:90-104`, task 318)
  names exactly those two with no ambiguity left over.
- **R53's `activity` order field.** The PRD (`prds/phase3e-list-ergonomics-and-chrome.md:143-146`)
  pre-emptively invites a finding if `StatusAt` seems like the wrong field for "activity" (deck
  records no per-session last-output clock, and adding one is explicitly out of scope for this
  phase). No such finding was filed: `StatusAt` — a status *change* timestamp — is what the
  amended spec itself names, and no task in this phase's work surfaced a case where that reading
  produced a wrong or surprising order.

## 4. Defects found but not fixed, with reasons

### 4a. Task 314's no-op scenario does not independently discriminate task 313's absence

`features/mouse.feature`'s `@requirement-54-sidebar-click-on-interactive-row-is-a-no-op` scenario
(commit `d55f174`) is structured as two sessions: it first retargets interactive mode from session
A to session B (exercising task 313's retarget path as a precondition), then clicks B's own
already-interactive row to exercise the true no-op branch. This makes the scenario's own red proof
against a reverted task 313 (`git revert --no-commit e6e1af3`) depend on running in the **same**
suite invocation as its sibling retargeting scenario (`@requirement-54-sidebar-click-retargets-
interactive-mode`) — the retargeting precondition itself fails first and loudly
(`timed out waiting for frame "> retarget-noop-b"`), which is a real, deterministic red proof, but
only in that combined run. Run alone, with only the no-op scenario's own tag, against reverted code:
the scenario passes, because pre-313 code treats *every* sidebar click while interactive as a
passive no-op regardless of whether the row is the current target, so a scenario that never gets
past B's own already-passing precondition trivially satisfies the no-op assertion too.

This is recorded in task 314's own `validationNotes` in `tasks.json` (two independent validation
passes reproduced it identically) and is not being re-litigated here: task 314's validation
attempts (2) are exhausted, and per this loop's own rule a task standing `completed` despite a
recorded validation gap is left as history rather than reopened without new information. A
follow-up task shape was proposed twice in `tasks.json` (restructure the no-op scenario's
assertion to key off behaviour pre-313 code visibly and differently lacks, rather than a
claim/geometry channel pre-313's blanket no-op leaves untouched either way) but was never picked
up as a numbered task in this plan. `internal/tui`'s three unit tests for the same retarget/no-op
logic (`internal/tui/mouse_interactive_retarget_test.go`) are unaffected by this gap — they test
the production code path directly, not through the two-scenario coupling.

### 4b. Matrix's 16-colour quantised idle/stopped/archived tokens are legible but not pairwise-distinct

Under 16-colour quantisation, `matrix`'s `idle`/`stopped`/`archived` status tokens — plus `hint`
and `badge` — all collapse onto the identical ANSI grey (`#7f7f7f`, four-way collision), per task
315's own report (`docs/reports/phase3e.md`, R56 section): "each individually still clears the
3:1 contrast floor (`TestBuiltinContrastFloor`, `TestSessionRowTokensClearContrastFloorOnSurface`,
both hex and quantised) but is not pairwise distinguishable from the others in that mode."

This does not violate any success criterion as written: the PRD's R56 bullet asks only that "the
16-colour quantised form of each new theme is still **legible**" (`prds/phase3e-list-ergonomics-
and-chrome.md:281-282`) — legibility (clearing the contrast floor against the surface) is
distinct from pairwise distinctness between tokens, and task 317's per-cell distinctness proof
(`TestMatrixStatusTokensRenderAsSevenDistinctColours`, commit `78043ee`) is explicitly scoped to
the real, true-colour production path — it never asserts anything about the 16-colour quantised
form. So this is a residual limitation of the shipped `matrix` palette, not an unmet criterion:
under a 16-colour terminal, a user cannot visually distinguish an idle, stopped, or archived
session by colour alone (though each is still readable against the background, and each status
also carries distinct text/badges). Left as-is because no task in this phase's scope asked for
16-colour pairwise distinctness (task 315 scoped itself to "contrast floor + hex-distinctness
only"; task 317's proving ground was true-colour distinctness only) — closing it would mean
widening `matrix`'s palette again, which is a real design change to a theme already shipped and
reviewed, not a bug fix.

### 4c. The settle-race defect class was confirmed present in two more test helpers and fixed (task 334)

Per operator steer `3e-002` (`/run/ralphd/steering/002-steer-3e-002-navigation-settle.md`): commit
`7ebafce` (task 324) root-caused and fixed a settle race in `selectSessionByNameThenSend`
(`features/agent_steps_test.go`) — `ScreenDriver.Send()` only writes to the pty and does not wait
for the client to repaint, so a burst of navigation keystrokes followed immediately by a frame
check can act on a selection that has not actually finished moving (the still-in-flight remainder,
typically the final arrow key, lands after the action key was fired at what looked like the right
row).

The identical loop shape — `Send(arrow)` → fixed sleep → substring-match the frame → act — is
structurally present, unfixed, in two more helpers in the same file:

- **`selectRowByName`** (`features/agent_steps_test.go:1633`), used by five call sites:
  `features/status_probe_test.go:301`, `features/agent_steps_test.go:1672`
  (`clientsRacePressingResumeOnNamedSession`, which backs `lease_race.feature`),
  `features/attention_sort_test.go:118` (backing R53's sort-order scenarios), and
  `features/status_claude_hooks_test.go:286`. Worse in one respect than the pre-fix
  `selectSessionByNameThenSend`: on a match it returns and the **caller** sends the action key, so
  the in-flight remainder has an even wider window to land in before that keystroke fires.
- **`clientOpensDetailForSession`** (`features/agent_steps_test.go:1341`), partly shielded because
  the caller waits for a `" detail"` render afterward — a mis-positioned client surfaces as a
  timeout rather than a false pass, so this is a flake risk rather than a false-green risk, same as
  `selectRowByName`'s `lease_race.feature` call site (checked explicitly per steer `3e-002` §1: a
  mis-targeted client there would show up in `lease_race.feature`'s own launch-record count
  assertion, which is not vacuous — this is a flake, not a silently-passing bug).

Task 325's stability run (`docs/reports/phase3e-stability/README.md`) confirms none of its ten
runs' failures were resume/restart/detail-open/lease-race scenarios, so this mechanism was never
implicated in the published 7/10 stability rate and 325 did not need a re-run once 334 landed.

**Fix (task 334):** `selectSessionByNameThenSend`, `selectRowByName` and
`clientOpensDetailForSession` were collapsed into one top-anchored helper
(`navigateToRowByName`/`sendNavKeySettled`, `features/navigation_settle_test.go`) that sends one
arrow key at a time and, before sending the next, polls (the same `d.updated`-channel idiom
`ScreenDriver.WaitForFrame`/`WaitForFrameGone` use) for the sidebar's own selected (`"> "`-prefixed)
row line to actually change, bounded by a short per-key ceiling (`navKeySettleWindow`, 300ms) that
only matters for a genuine no-op keystroke (top/bottom edge). No `time.Sleep` remains in the merged
path; `TestNavigationHelperNeverSleeps` (`features/navigation_settle_guard_test.go`) is a
source-scan guard that fails the instant one returns, demonstrated red by temporarily adding one
and restoring.

A first version of the fix substituted "wait for ANY `"> "` anywhere in the whole frame" for the
case where no row is currently selected in view (selection scrolled off-screen after a layout-mode
cycle). That is unsound: earlier create-dialog renders leave `"> Name: ..."`/`"> Working
directory: ..."` field-marker text on rows a later, shorter render never overwrites, and an
unscoped `"> "` search can match that stale leftover before deck has processed the keystroke at
all. This was caught, not theorised: `features/mouse.feature`'s
`@requirement-34-wheel-scrolls-without-selecting` scenario (which cycles layout mode until the
selected row scrolls off-screen, then calls `selectRowByName`) went flaky under it — 4 failures in
5 isolated runs, each after 3-8s instead of the ~1.9s baseline. Comparing the sidebar-scoped
selected line to its own prior value (never the whole frame) removed the false-positive surface;
the same scenario then passed 15/15 in isolation.

**Red-proof coverage per site**, per steer `3e-002`'s own escape clause ("if no deterministic red
proof is achievable ... say so explicitly"):

- **Guard test** (`time.Sleep` banned in the merged helper file): deterministic, demonstrated red
  by temporarily adding `time.Sleep(time.Millisecond)` to `navigateToRowByName` and restoring.
- **`selectRowByName`**: attempted a red proof by reverting to the pre-334 three-separate-helpers
  shape (via `git worktree`/in-place revert, restored afterward) and running
  `features/status_claude_hooks_test.go`'s two-session kill scenario (temporarily tagged
  `@debug-tmp-334`, tag removed before this commit) under induced CPU contention — 16 parallel
  sibling containers, 3 repeats each (48 total runs) — mirroring task 325's discriminating-load
  method. All 48 passed; no repro. Every existing scenario that reaches this helper has too short
  a walk distance (0-2 hops) to reliably expose the race even under contention. Not fixed by
  manufacturing a new scenario for the sole purpose of the demo — relies on the guard test alone
  for this site, as the escape clause allows.
- **`clientOpensDetailForSession`**: every committed scenario that calls it creates exactly one
  session before opening its detail view (checked across all 12 call sites:
  `agent_session.feature`, `crash.feature`×2, `dialogs.feature`×6, `launch_lease.feature`,
  `permission_modes.feature`, `status_claude_hooks.feature`), so the walk distance is always 0 —
  the marker is already present on the first check, before any down-arrow is ever sent. No
  existing scenario can expose a multi-keystroke settle race here at all, deterministically or
  under load; relies on the guard test alone for this site.

See `notes.md`'s "Steering: 3e-002" section for the durable copy of the original instruction.

### 4d. Two SIGWINCH-settle-window scenarios flake under host load — root-caused, explicitly not treated as a product defect

`features/mouse.feature`'s `DECK_MOUSE=0` scenario and `features/preview.feature`'s
`@steer-018-preview-fit-on-navigation` "7-inner-row floor" scenario both assert an exact SIGWINCH
count immediately after a gesture/resize, with a deliberately tight settle window (the former's own
comment documents 100ms). Both failed intermittently across task 324's whole-suite run and task
325's ten-run stability run, and both were reproduced clean (3/3) in low-load isolation each time —
root-caused to host CPU contention slipping the settle window, not a product bug, per non-negotiable
7 (`docs/reports/phase3e-fullsuite/README.md` item 2; `docs/reports/phase3e-stability/README.md`
items 1-2). Listed here for completeness, not as an unfixed defect in the product: no code change
was warranted or made, and neither scenario was weakened, skipped, or tag-excluded.

## Cross-references

- `docs/reports/phase3e.md` — the full per-requirement evidence table (R52-R62), including every
  revert-red proof cited above.
- `docs/reports/phase3e-fullsuite/README.md`, `docs/reports/phase3e-stability/README.md` — the
  whole-suite and ten-run stability reports, including the SIGWINCH-flake root-causing (§4d) and
  the `internal/interactive` goroutine-outlives-test panic (found, root-caused and fixed in the
  same iteration that closed task 325's validation gap — not listed under §4 above because it was
  fixed, with a real `sync.WaitGroup` join, commit `fb9bd71`, reproduced deliberately under induced
  contention).
- `/run/ralphd/steering/001-steer-3e-001-event-log.md`, `/run/ralphd/steering/002-steer-3e-002-
  navigation-settle.md` — the two operator steers this phase actioned/queued; `notes.md`'s
  "Steering" sections carry the durable summaries.
