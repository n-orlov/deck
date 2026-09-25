Feature: The preview capture engine and its visible behaviour
  §11 requirement 21's capture-pane -e engine never attaches a client, never
  spawns a control-mode client, uses no pipe-pane, and never resizes a pane
  it reads; §23-27 govern what the panel shows once a capture lands.

  Steer 018 item 4 / task 215's amendment (SPEC.md §11, post-395babf "The
  preview fits the selected session's window to the panel"): passive
  preview now ALSO fits the selected session's tmux window to the panel's
  content box as the list selection settles -- `[ui] preview_fit`, default
  true. This is a deliberate, narrowly-scoped inversion of what this file's
  own no-side-effect scenario used to assert for the selection axis alone
  (composite-prd.md:747-748's "Do not weaken features/preview.feature's
  passive guarantees ... If they conflict, the interactive design is
  wrong" is about a DIFFERENT axis of conflict -- an interactive-mode
  change reaching backward to weaken passive's own guarantee to make
  itself pass -- and does not apply here either by its letter or in its
  spirit: this change is SPEC's own text, authored by the operator's own
  spec push, not code reaching back to relax a test it was failing, and
  every OTHER passive-preview guarantee -- no attached client, no pipe, no
  scroll, best-effort, floor-respecting, cost-stated, reversible via
  `preview_fit = false` -- is left completely intact by it, proven by the
  scenarios below). The capture engine itself (`capture-pane -e`, this
  file's own title) is unchanged and still never resizes anything; the fit
  is a separate, additional, coalesced `resize-window` issued alongside
  the same 250ms tick.

  @requirement-21-preview-no-side-effects
  Scenario: capturing the preview never attaches a tmux client, resizes a pane, or triggers a SIGWINCH, across selection, mode, sidebar-width and outer-terminal changes
    # DECK_PREVIEW_FIT=0 isolates the capture-pane engine's own read-only
    # guarantee (requirement 21, this scenario's whole point) from the
    # orthogonal preview_fit feature (steer 018 item 4, see this file's
    # header comment): with the fit turned off, passive preview is
    # exactly the pre-215 capture-pane poll, and every assertion below is
    # unweakened and unchanged from before that feature existed.
    Given the deck config disables preview fit
    And a long-running fake "claude" binary is on PATH for future deck clients
    And deck client "solo" is started
    And deck client "solo" creates shell session "alpha"
    And deck client "solo" creates claude session "beacon" with permission profile "safe"
    And the private tmux window for session "beacon" is captured as "before"
    And the fake claude agent's size log is captured as "before"
    When deck client "solo" selects the next session
    And deck client "solo" selects the next session
    And deck client "solo" sends "|"
    And deck client "solo" sends "|"
    And deck client "solo" sends ">"
    And deck client "solo" sends "<"
    And deck client "solo" terminal is resized to 120x40
    And deck client "solo" terminal is resized to 100x30
    Then the private tmux server reports no attached clients
    And the private tmux window for session "beacon" still matches "before"
    And the fake claude agent's size log still matches "before"
    And deck client "solo" exits cleanly

  @steer-018-preview-fit-on-navigation
  Scenario: preview_fit does not resize on a mode switch, sidebar-width change or outer-terminal resize -- only on a settled selection change
    # The amendment this file's header describes is scoped to the
    # SELECTION axis only. This is the same scenario as requirement 21
    # above minus the two selection-change steps and WITHOUT disabling
    # preview_fit (left at its true default), proving the coalescing
    # guard (previewFitSessionID) really does confine the new resize to
    # "the selection settled on something new", not to any layout change
    # that happens to reach the same tick.
    Given a long-running fake "claude" binary is on PATH for future deck clients
    And deck client "solo" is started
    And deck client "solo" creates shell session "alpha"
    And deck client "solo" creates claude session "beacon" with permission profile "safe"
    And within one configured reconcile interval deck client "solo" screen contains "running"
    And deck client "solo" selects session "beacon"
    # Let the settled selection's own fit land and coalesce before the
    # "before" snapshot, so what follows is proven against the STEADY
    # state a real user would already be looking at, not a race with the
    # very first fit this selection ever gets.
    And 200 milliseconds pass
    And the private tmux window for session "beacon" is captured as "axis-before"
    And the fake claude agent's size log is captured as "axis-before"
    When deck client "solo" sends "|"
    And deck client "solo" sends "|"
    And deck client "solo" sends ">"
    And deck client "solo" sends "<"
    And deck client "solo" terminal is resized to 120x40
    And deck client "solo" terminal is resized to 100x30
    Then the private tmux window for session "beacon" still matches "axis-before"
    And the fake claude agent's size log still matches "axis-before"
    And deck client "solo" exits cleanly

  @steer-018-preview-fit-on-navigation
  Scenario: a settled selection fits the session's window to exactly the panel's own content box, coalesced across repeated revisits
    Given a long-running fake "claude" binary is on PATH for future deck clients
    And deck client "solo" is started
    And deck client "solo" creates shell session "alpha"
    And deck client "solo" creates claude session "beacon" with permission profile "safe"
    And within one configured reconcile interval deck client "solo" screen contains "running"
    When deck client "solo" selects session "beacon"
    Then the fake "claude" agent received exactly 1 SIGWINCH signals
    # Revisiting the same settled session repeatedly -- bouncing away and
    # back, exactly what "holding an arrow key" and then coming back to it
    # produces -- costs nothing further: previewFitSessionID's own
    # coalescing means the session already sits at the size deck chose,
    # and FitWindowToPane is itself idempotent when a target already
    # matches, so no further SIGWINCH lands no matter how many times this
    # settles on beacon again.
    When deck client "solo" selects session "alpha"
    And deck client "solo" selects session "beacon"
    And deck client "solo" selects session "alpha"
    And deck client "solo" selects session "beacon"
    Then the fake "claude" agent received exactly 1 SIGWINCH signals
    And the private tmux window for session "beacon" is captured as "passively-fitted"
    # Entering interactive mode computes and fits to the exact same
    # (previewContentSize) box passive fit already used -- if passive fit
    # got it right, entering costs no further resize at all, proving
    # passive fit converged on precisely the panel's content box rather
    # than merely "a" different size.
    When deck client "solo" enters interactive mode
    Then the private tmux window for session "beacon" still matches "passively-fitted"
    And deck client "solo" leaves interactive mode
    And deck client "solo" exits cleanly

  @steer-018-preview-fit-on-navigation
  Scenario: preview_fit = false restores the wholly passive preview, and settling never resizes
    Given the deck config disables preview fit
    And a long-running fake "claude" binary is on PATH for future deck clients
    And deck client "solo" is started
    And deck client "solo" creates shell session "alpha"
    And deck client "solo" creates claude session "beacon" with permission profile "safe"
    And within one configured reconcile interval deck client "solo" screen contains "running"
    When deck client "solo" selects session "beacon"
    And deck client "solo" selects session "alpha"
    And deck client "solo" selects session "beacon"
    Then the fake "claude" agent received exactly 0 SIGWINCH signals
    And deck client "solo" exits cleanly

  @steer-018-preview-fit-on-navigation
  Scenario: a fit is skipped below the 7-inner-row floor, and retried once the panel grows back above it
    Given a long-running fake "claude" binary is on PATH for future deck clients
    # The creating client and the observing client are deliberately
    # separate, and this shape is load-bearing rather than cosmetic (task
    # 028, finding F1). Requirement 52 auto-selects a freshly created row,
    # so a claude session created by a client at the default 100x30 is the
    # selected row with a 61x27 content box -- well above
    # interactiveMinInnerRows -- and the very next previewTick
    # (DECK_PREVIEW_MS=50) licenses the one passive fit this scenario
    # asserts never happens. Shrinking afterwards does not retract it:
    # "terminal is resized to" waits for a render marker
    # (ScreenDriver.ResizeAndAwaitRender), which proves only that SOME
    # repaint happened after the resize, never that Update has already
    # taken m.width/m.height to the smaller size, and the window between
    # the create and that WindowSizeMsg is tens to hundreds of
    # milliseconds wide -- several previewTicks. That race cost this
    # assertion roughly one run in ten with `received 1 SIGWINCH signals,
    # want exactly 0`. Here "maker" runs with DECK_PREVIEW_FIT=0, so it
    # can never fit anything, and "solo" -- the only client with fit
    # enabled -- is born at 100x9 and so has no frame above the floor in
    # its whole life before the assertion. No fit is licensed at a larger
    # size by construction, not by winning a race.
    # features/preview_test.go's clientHasNeverBeenTallerThan pins that
    # shape deterministically, on every run.
    And deck client "maker" is started with preview fit disabled
    And deck client "maker" creates shell session "alpha"
    And deck client "maker" creates claude session "beacon" with permission profile "safe"
    And within one configured reconcile interval deck client "maker" screen contains "running"
    And deck client "maker" exits cleanly
    And deck client "solo" is started with terminal size 100x9
    When deck client "solo" selects session "beacon"
    # 100 columns keeps auto layout side-by-side (>= 80), whose preview is
    # always shown (LayoutResult.PreviewShown, layout.go) regardless of
    # rows, so this is the interactive floor biting on its own -- a
    # content box below interactiveMinInnerRows -- never requirement 27's
    # separate stacked-mode suppression floor.
    Then deck client "solo" has never been taller than 9 rows
    And the fake "claude" agent received exactly 0 SIGWINCH signals
    When deck client "solo" terminal is resized to 100x30
    Then the fake "claude" agent received exactly 1 SIGWINCH signals
    And deck client "solo" exits cleanly

  @requirement-23-preview-crop-geometry
  Scenario: in passive preview, a live pane larger than the panel is cropped with its real geometry stated
    # Scoped to passive preview (PRD Part II item 2 under "Assertions this
    # phase must deliberately change"): this scenario never enters
    # interactive mode, so the \d+x\d+ of \d+x\d+ crop statement it asserts
    # is never the fitted, non-cropping case II-46 gives its own scenario
    # below.
    Given deck client "solo" is started
    And deck client "solo" creates shell session "alpha"
    Then deck client "solo" screen matches the pattern "\d+x\d+ of \d+x\d+"
    And deck client "solo" exits cleanly

  @requirement-46-interactive-fitted-geometry
  Scenario: interactive mode states the fitted geometry, never the crop form
    # PRD Part II requirement 46: entering interactive mode fits the tmux
    # window to exactly the panel's own content box, so the real pane size
    # and the panel's content size are always equal. Stating that in the
    # crop's own "WxH of realWxrealH" form would degenerate to the
    # misleading "45x22 of 45x22" -- a crop statement about a pane that was
    # never cropped. The panel states it differently instead: "WxH fitted".
    Given deck client "solo" is started
    And deck client "solo" creates shell session "alpha"
    And within one configured reconcile interval deck client "solo" screen contains "running"
    And deck client "solo" selects session "alpha"
    When deck client "solo" enters interactive mode
    Then deck client "solo" screen matches the pattern "\d+x\d+ fitted"
    And deck client "solo" screen does not contain " of "
    And deck client "solo" leaves interactive mode
    And deck client "solo" exits cleanly

  @requirement-23-preview-crop-geometry
  @requirement-46-interactive-fitted-geometry
  Scenario: r resumes the selected stopped row and passive preview re-fits away from tmux's unfit 80x24 default
    # Task 117/002's fix (internal/tui/tui.go's sessionResumed/
    # sessionRestarted branch): a resume that actually creates a pane
    # clears previewFitSessionID for that same session id, so the very
    # next previewTick's previewFit (steer 018 item 4) re-fits the fresh
    # pane to the panel's own content box instead of leaving it wedged at
    # tmux new-session's own unfit 80x24 default (no -x/-y is ever passed,
    # internal/tmux/tmux.go's Create) for the rest of the model's
    # lifetime. Reverting that clear makes previewFit's own eligibility
    # guard (`session.ID == m.previewFitSessionID`) refuse every fit
    # attempt forever once this exact session id has settled once before
    # -- which it already did, below, the moment "alpha" was created and
    # auto-selected -- so the crop line's second, real-geometry pair below
    # would never stop reading "80x24" after the resume. Checked by hand:
    # reverting f9de4a5/815f2ea and rerunning this scenario times out on
    # "deck client \"solo\" screen stops containing \"of 80x24\"" after the
    # resume instead of passing.
    Given deck client "solo" is started
    And deck client "solo" creates shell session "alpha"
    Then within one configured reconcile interval deck client "solo" screen contains "running"
    # "alpha" is the only row: it was auto-selected on creation and stays
    # selected through everything below, so nothing in this scenario ever
    # changes the selection -- the one condition the criterion this
    # scenario proves requires. Waiting out the FIRST passive fit here (it
    # settles "alpha" away from its own newly-created-pane 80x24) gives
    # this scenario a real, already-fitted baseline rather than racing the
    # very first fit any selected row ever gets.
    And deck client "solo" screen stops containing "of 80x24"
    And the private tmux window for session "alpha" is captured as "fitted-before-stop"
    When shell session "alpha" exits with status zero
    Then within one configured reconcile interval deck client "solo" screen contains "stopped"
    When deck client "solo" presses r on session "alpha"
    Then within one configured reconcile interval deck client "solo" screen contains "running"
    # The resume above created a brand-new tmux window (task 117/002's own
    # scope: a relaunch's new pane), which starts life back at tmux's own
    # unfit 80x24 default -- exactly the state task 002's latch clear
    # exists to escape a second time for this same still-selected session.
    Then deck client "solo" screen stops containing "of 80x24"
    And the private tmux window for session "alpha" still matches "fitted-before-stop"
    And the private tmux window for session "alpha" does not report geometry "80x24"
    And deck client "solo" exits cleanly

  @requirement-24-preview-wide-cell-boundary
  Scenario: wide glyphs in a cropped pane never shear the preview's border
    Given deck client "solo" is started
    And deck client "solo" creates shell session "widepane"
    When deck client "solo" fills the selected session's pane with wide characters
    Then deck client "solo" screen contains "界"
    And deck client "solo" every full-width row is bordered on both edges
    And deck client "solo" exits cleanly

  @requirement-22-24-preview-colour-border-integrity
  Scenario: a coloured pane's SGR escapes never shear the preview's border
    Given deck client "solo" is started
    And deck client "solo" creates shell session "colourpane"
    When the private tmux pane for session "colourpane" prints red-coloured text "REDLINE"
    Then deck client "solo" screen contains "REDLINE"
    And deck client "solo" every full-width row is bordered on both edges
    And deck client "solo" the row containing "REDLINE" is bordered on both edges at the full grid width
    And deck client "solo" exits cleanly

  @requirement-25-preview-gesture-no-ops
  Scenario: scrolling over the passive preview panel does nothing
    # task 010 (R144, GH #37) made a left press over the passive preview
    # enter interactive mode on the current selection (SPEC.md's SS11.8
    # table, "click the passive preview | enter ... interactive preview"),
    # so this scenario's own click/double-click assertions -- true when it
    # was written, false since that commit -- were dropped rather than
    # left pinned to a claim the product no longer makes; the wheel is the
    # only gesture the passive preview still turns into a no-op. The
    # click's own new behaviour is covered by features/mouse.feature's
    # @requirement-37-preview-click-enters-and-empty-sidebar-click-leaves
    # scenario and internal/tui/mouse_preview_enter_test.go's unit tests,
    # not re-asserted here.
    Given deck client "solo" is started
    And deck client "solo" creates shell session "alpha"
    And within one configured reconcile interval deck client "solo" screen contains "running"
    And deck client "solo" captures its frame as "before-preview-gesture"
    When deck client "solo" scrolls the wheel up at column 70 row 15
    And deck client "solo" scrolls the wheel down at column 70 row 15
    Then deck client "solo" frame still matches the captured "before-preview-gesture" frame
    And deck client "solo" exits cleanly

  @requirement-26-preview-crash-tail
  Scenario: an error row's preview shows the durable crash tail, headed by copy stating it is not live
    Given a crash-tail fixture and long-running fake Claude are configured
    And deck client "A" is started
    When deck client "A" creates claude session "doomed" with permission profile "safe"
    And fake Claude session "doomed" renders the colored crash-tail fixture
    And the agent process "fake-claude-real" in private tmux session "deck_doomed" is killed with SIGKILL
    Then within one configured reconcile interval deck client "A" screen contains "error"
    And deck client "A" screen contains "Last output before exit - not live:"
    And deck client "A" screen contains "crash final line"
    When deck client "A" exits cleanly

  @requirement-26-preview-placeholder
  Scenario: a stopped session's preview names its own state instead of showing stale bytes
    Given deck client "A" is started
    When deck client "A" creates shell session "retiring"
    And shell session "retiring" exits with status zero
    Then within one configured reconcile interval deck client "A" screen contains "resumable"
    And deck client "A" screen contains "Session is stopped. No live preview to show."
    When deck client "A" exits cleanly

  @requirement-27-preview-suppressed-below-floor
  Scenario: the preview is suppressed below its floor and the sidebar takes the space
    Given deck client "solo" is started
    And deck client "solo" creates shell session "alpha"
    And the private tmux pane for session "alpha" prints "PREVIEWMARKERXYZ"
    Then deck client "solo" screen contains "PREVIEWMARKERXYZ"
    When deck client "solo" terminal is resized to 60x12
    Then deck client "solo" screen stops containing "PREVIEWMARKERXYZ"
    When deck client "solo" terminal is resized to 100x30
    Then deck client "solo" screen contains "PREVIEWMARKERXYZ"
    And deck client "solo" exits cleanly
