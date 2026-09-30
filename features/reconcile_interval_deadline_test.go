package features

import "time"

// reconcileIntervalPollDeadline is the shared poll deadline behind every
// "within one configured reconcile interval ..." step (R164): the fixed
// scenarioReconcileInterval+250ms=500ms bound that
// clientScreenContainsWithinReconcileInterval (assertions_test.go),
// auditHasLaunchRecordCountForSessionWithinReconcileInterval
// (agent_steps_test.go), and clientRowContainsWithinReconcile
// (status_probe_test.go) each poll a released deck binary against.
//
// Unlike hookStoreDurationWithinBudget / sessionsCreatedGapWithinBudget
// (R164's BUDGET treatment, which SKIPS the assertion entirely on a race
// build because it judges a wall-clock performance target that a
// race-instrumented binary cannot meaningfully meet), this bound is a
// DEADLINE: the awaited condition -- a scheduled reconcile tick's
// tmux/SQLite work and PTY render becoming observable -- must still
// actually become true, so a race build only WIDENS how long the poll
// waits for it; it never skips the wait or weakens what is being checked.
//
// CI run 36717507756 showed the fixed 500ms bound missed under `-race`:
// features/agent_session.feature:33 ("R restarts a running claude session
// with the resume argv, preserving its conversation id") failed both in
// the suite job and on a solo rerun with "did not show \"fake-claude
// resume:\" within 500ms". features/launch_hooks.feature:115 drives the
// exact same clientScreenContainsWithinReconcileInterval step wording and
// shares the same exposure. The race build's own instrumentation and the
// host contention it invites push real wall-clock cost past a bound that
// is otherwise generous on a normal build.
func reconcileIntervalPollDeadline(interval time.Duration, raceBuild bool) time.Duration {
	if raceBuild {
		return interval + 20*time.Second
	}
	return interval + 250*time.Millisecond
}
