package features

import "time"

// r150FailingReconcileWaits is how many rejected reconcile waits the two live
// R150 tests burn in sequence (the base and rename_only_control cases).
const r150FailingReconcileWaits = 2

// r150LiveOuterDeadline (R164 cure, cure-01-01) computes the outer
// context.WithTimeout budget for an R150 live scenario test that drives
// r150FailingReconcileWaits sequential calls to clientRowContainsWithinReconcile
// that are EXPECTED TO REJECT (a negative/rename-only control): each such
// call burns its own full reconcileIntervalPollDeadline before returning,
// because the awaited condition never becomes true. On a normal build that
// per-call deadline is the historical fixed 500ms bound, already inside
// `base`, so this returns `base` unchanged -- product behavior and
// normal-build timing are untouched. On a race build, task 016
// (reconcileIntervalPollDeadline) widened that per-call deadline to
// interval+20s so the awaited reconcile tick still has time to actually
// happen under -race's slowdown; this function widens the OUTER context by
// the same per-call amount, once per expected-failing call, so the outer
// context does not expire out from under those still-legitimate inner
// waits (the R150 review finding cure-01-01 fixes: commit 1b37ad6116 widened
// the inner deadline but left these two live tests' outer contexts fixed at
// 15s/30s, so two sequential 20.25s rejections under -race blew straight
// through them with "context deadline exceeded"/"signal: killed").
func r150LiveOuterDeadline(base time.Duration, raceBuild bool) time.Duration {
	if !raceBuild {
		return base
	}
	perFailingWait := reconcileIntervalPollDeadline(true)
	return base + time.Duration(r150FailingReconcileWaits)*perFailingWait
}
