package features

// pendingKeySend (cure-01-01-2, B4) is a one-shot record of a select-then-
// send this task's own review finding showed can silently miss its target:
// a background reload racing navigateToRowByName's own pre-send
// confirmation (or an earlier, not-yet-drained keystroke coalescing with
// this one -- see review-findings.md's B4 and CI run 36774969847,
// agent_session.feature:33) can move the sidebar's selection off the
// intended row moments after Send returns with no error at all, so the
// keystroke acts on nothing and whatever it was meant to trigger never
// happens.
//
// clientPressesResumeOnNamedSession/clientPressesRestartOnNamedSession
// record one of these (via ScenarioHarness.recordPendingKeySend)
// immediately after their own select-then-send succeeds; the very next
// "within one configured reconcile interval ... screen contains" step
// (clientScreenContainsWithinReconcileInterval, features/assertions_test.go)
// consumes it (via takePendingKeySend) if -- and only if -- the awaited
// text never actually appears, redoing that exact select-then-send once
// before giving the reconcile-interval deadline one more full chance
// rather than only ever concluding "missing" after a keystroke that, per
// B4, may never have landed in the first place.
//
// This is deliberately narrow: it does not change what
// clientScreenContainsWithinReconcileInterval requires (the text must
// still actually appear, within the exact same unweakened deadline, or
// the step still fails), and it is a no-op for every scenario that never
// records one -- the vast majority of "within one configured reconcile
// interval" call sites, none of which follow a resume/restart keypress.
type pendingKeySend struct {
	want string
	key  string
}

// recordPendingKeySend stores want/key as the most recent select-then-send
// clientScreenContainsWithinReconcileInterval may redo for clientName, per
// pendingKeySend's own doc comment. A second call for the same clientName
// (e.g. a scenario that presses r/R on more than one session in sequence)
// simply overwrites the previous record -- only ever the LATEST
// select-then-send is a plausible culprit for the very next reconcile-
// interval wait that follows it.
func (h *ScenarioHarness) recordPendingKeySend(clientName, want, key string) {
	if h.pendingKeySends == nil {
		h.pendingKeySends = make(map[string]pendingKeySend)
	}
	h.pendingKeySends[clientName] = pendingKeySend{want: want, key: key}
}

// takePendingKeySend removes and returns clientName's pending record, if
// any -- "take", not "peek", so the retry it authorises can only ever
// fire once per recorded select-then-send: a second, unrelated reconcile-
// interval wait for the same client later in the same scenario (there is
// no shortage of these once a scenario chains several such steps) must
// not keep redoing a stale keystroke.
func (h *ScenarioHarness) takePendingKeySend(clientName string) (pendingKeySend, bool) {
	pending, ok := h.pendingKeySends[clientName]
	if ok {
		delete(h.pendingKeySends, clientName)
	}
	return pending, ok
}
