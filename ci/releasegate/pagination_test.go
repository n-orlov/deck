package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// pageServer is a deterministic, in-memory stand-in for the GitHub API
// this gate's production entry path (run) actually calls: it serves
// canned check-runs/actions-runs JSON pages, follows the same rel="next"
// Link-header contract GitHub uses, and records every request path+query
// it receives so a test can assert exactly which pages were fetched.
type pageServer struct {
	mu       sync.Mutex
	requests []string

	checkRunsPages   []string // JSON bodies, in order; index 0 is page 1
	actionsRunsPages []string
	baseURL          string
}

func (s *pageServer) record(r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, r.URL.Path+"?"+r.URL.RawQuery)
}

func (s *pageServer) seenRequests() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.requests))
	copy(out, s.requests)
	return out
}

func newPageServer(t *testing.T, checkRunsPages, actionsRunsPages []string) *pageServer {
	s := &pageServer{checkRunsPages: checkRunsPages, actionsRunsPages: actionsRunsPages}
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/o/r/commits/deadbeef/check-runs", func(w http.ResponseWriter, r *http.Request) {
		s.record(r)
		pageNum := pageParam(r)
		if pageNum < 1 || pageNum > len(s.checkRunsPages) {
			http.Error(w, "no such page", http.StatusNotFound)
			return
		}
		if pageNum < len(s.checkRunsPages) {
			next := fmt.Sprintf("<%s/repos/o/r/commits/deadbeef/check-runs?page=%d>; rel=\"next\"", s.baseURL, pageNum+1)
			w.Header().Set("Link", next)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, s.checkRunsPages[pageNum-1])
	})
	mux.HandleFunc("/repos/o/r/actions/runs", func(w http.ResponseWriter, r *http.Request) {
		s.record(r)
		pageNum := pageParam(r)
		if pageNum < 1 || pageNum > len(s.actionsRunsPages) {
			http.Error(w, "no such page", http.StatusNotFound)
			return
		}
		if pageNum < len(s.actionsRunsPages) {
			next := fmt.Sprintf("<%s/repos/o/r/actions/runs?head_sha=deadbeef&page=%d>; rel=\"next\"", s.baseURL, pageNum+1)
			w.Header().Set("Link", next)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, s.actionsRunsPages[pageNum-1])
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	s.baseURL = srv.URL
	return s
}

func pageParam(r *http.Request) int {
	q := r.URL.Query().Get("page")
	if q == "" {
		return 1
	}
	n := 0
	for _, c := range q {
		if c < '0' || c > '9' {
			return 1
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// withAPIBase points githubAPIBase at the given test server for the
// life of the calling test, restoring the production default on
// cleanup -- so run(), the production entry path, talks to the fake
// server exactly as it would talk to api.github.com.
func withAPIBase(t *testing.T, base string) {
	prev := githubAPIBase
	githubAPIBase = base
	t.Cleanup(func() { githubAPIBase = prev })
}

func noTokenEnv(string) string { return "" }

// dispatchCheckRun renders one "suite" check run belonging to check_suite
// id, as JSON object text (no trailing comma) for embedding in a
// hand-built check_runs page.
func dispatchCheckRun(id int, startedAt string) string {
	return fmt.Sprintf(`{"name":"suite","status":"completed","conclusion":"failure","started_at":%q,"check_suite":{"id":%d}}`, startedAt, id)
}

func dispatchWorkflowRun(checkSuiteID int) string {
	return fmt.Sprintf(`{"event":"workflow_dispatch","check_suite_id":%d,"path":".github/workflows/ci.yml"}`, checkSuiteID)
}

// TestReviewGateDoesNotLosePushSuccessBehindIgnoredRunPages drives the
// production releasegate run/fetch/evaluate path (run, the same
// function main() calls) against a deterministic HTTP server, with
// valid paginated GitHub responses in both subcases below. Before the
// fetchCheckRuns/fetchActionsRuns pagination fix, both subcases refuse
// ("all checks are non-gating" / "no push or pull_request check run
// found") because only the first page of either endpoint was ever
// fetched -- the successful push suite check (or its event association)
// sat on a second page behind 30 ignored-dispatch entries and was never
// seen.
func TestReviewGateDoesNotLosePushSuccessBehindIgnoredRunPages(t *testing.T) {
	t.Run("check-runs pagination: push success on page two", func(t *testing.T) {
		var dispatchRuns []string
		var dispatchChecks []string
		for i := 1; i <= 30; i++ {
			dispatchChecks = append(dispatchChecks, dispatchCheckRun(i, "2026-01-01T00:00:00Z"))
			dispatchRuns = append(dispatchRuns, dispatchWorkflowRun(i))
		}
		page1 := `{"total_count":31,"check_runs":[` + strings.Join(dispatchChecks, ",") + `]}`
		page2 := `{"total_count":31,"check_runs":[{"name":"suite","status":"completed","conclusion":"success","started_at":"2026-01-01T00:05:00Z","check_suite":{"id":31}}]}`
		actionsRuns := `{"workflow_runs":[` + strings.Join(dispatchRuns, ",") + `,{"event":"push","check_suite_id":31,"path":".github/workflows/ci.yml"}]}`

		s := newPageServer(t, []string{page1, page2}, []string{actionsRuns})
		withAPIBase(t, s.baseURL)

		if err := run([]string{"-repo", "o/r", "-sha", "deadbeef"}, noTokenEnv); err != nil {
			t.Fatalf("run: expected acceptance (push success on check-runs page two), got error: %v", err)
		}

		seen := s.seenRequests()
		var sawPage2 bool
		for _, req := range seen {
			if strings.Contains(req, "check-runs") && strings.Contains(req, "page=2") {
				sawPage2 = true
			}
		}
		if !sawPage2 {
			t.Errorf("run: never fetched check-runs page 2; requests seen: %v", seen)
		}
	})

	t.Run("actions-runs pagination: push event association on page two", func(t *testing.T) {
		checkRuns := `{"total_count":1,"check_runs":[{"name":"suite","status":"completed","conclusion":"success","started_at":"2026-01-01T00:00:00Z","check_suite":{"id":99}}]}`

		var dispatchRuns []string
		for i := 1; i <= 30; i++ {
			dispatchRuns = append(dispatchRuns, dispatchWorkflowRun(i))
		}
		actionsPage1 := `{"workflow_runs":[` + strings.Join(dispatchRuns, ",") + `]}`
		actionsPage2 := `{"workflow_runs":[{"event":"push","check_suite_id":99,"path":".github/workflows/ci.yml"}]}`

		s := newPageServer(t, []string{checkRuns}, []string{actionsPage1, actionsPage2})
		withAPIBase(t, s.baseURL)

		if err := run([]string{"-repo", "o/r", "-sha", "deadbeef"}, noTokenEnv); err != nil {
			t.Fatalf("run: expected acceptance (push event association on actions-runs page two), got error: %v", err)
		}

		seen := s.seenRequests()
		var sawPage2 bool
		for _, req := range seen {
			if strings.Contains(req, "actions/runs") && strings.Contains(req, "page=2") {
				sawPage2 = true
			}
		}
		if !sawPage2 {
			t.Errorf("run: never fetched actions-runs page 2; requests seen: %v", seen)
		}
	})

	t.Run("latest counted failure still refuses across pages", func(t *testing.T) {
		page1 := `{"total_count":2,"check_runs":[{"name":"suite","status":"completed","conclusion":"success","started_at":"2026-01-01T00:00:00Z","check_suite":{"id":1}}]}`
		page2 := `{"total_count":2,"check_runs":[{"name":"suite","status":"completed","conclusion":"failure","started_at":"2026-01-01T00:10:00Z","check_suite":{"id":2}}]}`
		actionsRuns := `{"workflow_runs":[{"event":"push","check_suite_id":1,"path":".github/workflows/ci.yml"},{"event":"push","check_suite_id":2,"path":".github/workflows/ci.yml"}]}`

		s := newPageServer(t, []string{page1, page2}, []string{actionsRuns})
		withAPIBase(t, s.baseURL)

		err := run([]string{"-repo", "o/r", "-sha", "deadbeef"}, noTokenEnv)
		if err == nil {
			t.Fatalf("run: expected refusal (latest push check run across pages failed), got nil")
		}
		if !strings.Contains(err.Error(), "failure") {
			t.Errorf("run error %q does not name the failure conclusion it found", err.Error())
		}
	})

	t.Run("no counted run across pages still refuses", func(t *testing.T) {
		var dispatchRuns []string
		var dispatchChecks []string
		for i := 1; i <= 30; i++ {
			dispatchChecks = append(dispatchChecks, dispatchCheckRun(i, "2026-01-01T00:00:00Z"))
			dispatchRuns = append(dispatchRuns, dispatchWorkflowRun(i))
		}
		page1 := `{"total_count":30,"check_runs":[` + strings.Join(dispatchChecks[:15], ",") + `]}`
		page2 := `{"total_count":30,"check_runs":[` + strings.Join(dispatchChecks[15:], ",") + `]}`
		actionsRuns := `{"workflow_runs":[` + strings.Join(dispatchRuns, ",") + `]}`

		s := newPageServer(t, []string{page1, page2}, []string{actionsRuns})
		withAPIBase(t, s.baseURL)

		err := run([]string{"-repo", "o/r", "-sha", "deadbeef"}, noTokenEnv)
		if err == nil {
			t.Fatalf("run: expected refusal (every page's checks are non-gating), got nil")
		}
		if !strings.Contains(err.Error(), "non-gating") {
			t.Errorf("run error %q does not say that only non-gating runs exist", err.Error())
		}
	})
}
