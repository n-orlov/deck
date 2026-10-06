package main

import "testing"

func TestGatingSuiteIDs_KeepsOnlyGatingEvents(t *testing.T) {
	got := gatingSuiteIDs([]actionsRun{
		{Event: "push", CheckSuiteID: 1, Path: ciWorkflowPath},
		{Event: "pull_request", CheckSuiteID: 2, Path: ciWorkflowPath},
		{Event: "schedule", CheckSuiteID: 3, Path: ciWorkflowPath},
		{Event: "workflow_dispatch", CheckSuiteID: 4, Path: ciWorkflowPath},
		// A re-run on the same suite: any gating run makes the suite gating,
		// and a later non-gating run never un-gates it.
		{Event: "schedule", CheckSuiteID: 1, Path: ciWorkflowPath},
	})
	for id, want := range map[int64]bool{1: true, 2: true, 3: false, 4: false, 5: false} {
		if got[id] != want {
			t.Errorf("suite %d gating = %v, want %v (map %v)", id, got[id], want, got)
		}
	}
	if len(got) != 2 {
		t.Errorf("gating set = %v, want exactly suites 1 and 2", got)
	}
}

func TestGatingSuiteIDs_Empty(t *testing.T) {
	if got := gatingSuiteIDs(nil); len(got) != 0 {
		t.Errorf("no workflow runs gave gating set %v, want empty", got)
	}
}

func cr(name string, suite int64, startedAt string) checkRun {
	c := checkRun{Name: name, StartedAt: startedAt}
	c.CheckSuite.ID = suite
	return c
}

func TestLatestGatingCheckRun_PicksLatestStartedAtAmongGating(t *testing.T) {
	runs := []checkRun{
		cr("suite", 1, "2026-01-01T10:00:00Z"),
		cr("suite", 2, "2026-01-01T12:00:00Z"),
		cr("suite", 1, "2026-01-01T11:00:00Z"),
		// Later but non-gating: must not win.
		cr("suite", 9, "2026-01-01T13:00:00Z"),
		// Other check name: must not be counted or win.
		cr("lint", 2, "2026-01-01T14:00:00Z"),
	}
	latest, total, nonGating := latestGatingCheckRun(runs, map[int64]bool{1: true, 2: true}, "suite")
	if latest == nil || latest.StartedAt != "2026-01-01T12:00:00Z" {
		t.Fatalf("latest = %+v, want the 12:00 run of suite 2", latest)
	}
	if total != 4 || nonGating != 1 {
		t.Errorf("namedTotal=%d nonGating=%d, want 4 and 1", total, nonGating)
	}
}

func TestLatestGatingCheckRun_EarlierRunListedAfterLaterOneDoesNotWin(t *testing.T) {
	runs := []checkRun{
		cr("suite", 1, "2026-01-01T12:00:00Z"),
		cr("suite", 1, "2026-01-01T10:00:00Z"),
	}
	latest, _, _ := latestGatingCheckRun(runs, map[int64]bool{1: true}, "suite")
	if latest == nil || latest.StartedAt != "2026-01-01T12:00:00Z" {
		t.Fatalf("latest = %+v, want the 12:00 run", latest)
	}
}

func TestLatestGatingCheckRun_NoGatingRun(t *testing.T) {
	runs := []checkRun{cr("suite", 9, "2026-01-01T10:00:00Z"), cr("suite", 8, "2026-01-01T11:00:00Z")}
	latest, total, nonGating := latestGatingCheckRun(runs, map[int64]bool{}, "suite")
	if latest != nil || total != 2 || nonGating != 2 {
		t.Errorf("got latest=%+v total=%d nonGating=%d, want nil, 2, 2", latest, total, nonGating)
	}
	latest, total, nonGating = latestGatingCheckRun(nil, nil, "suite")
	if latest != nil || total != 0 || nonGating != 0 {
		t.Errorf("empty input: latest=%+v total=%d nonGating=%d, want nil, 0, 0", latest, total, nonGating)
	}
}
