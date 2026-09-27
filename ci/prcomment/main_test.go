package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// commentServer is a deterministic, in-memory stand-in for the GitHub
// issue-comments API this command's production entry path (run) talks
// to: it serves the real API's own default paging (30/page unless
// per_page is given, capped at 100), a rel="next" Link header exactly
// like GitHub's, and implements create (POST) and update (PATCH) against
// its own in-memory comment list so a test can drive two sequential
// "publications" and observe the resulting comment set.
type commentServer struct {
	mu       sync.Mutex
	comments []comment
	nextID   int64
	requests []string
	baseURL  string
}

func newCommentServer(t *testing.T, seed []comment) *commentServer {
	s := &commentServer{comments: append([]comment{}, seed...)}
	for _, c := range seed {
		if c.ID >= s.nextID {
			s.nextID = c.ID + 1
		}
	}
	if s.nextID == 0 {
		s.nextID = 1
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/o/r/issues/1/comments", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.requests = append(s.requests, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery)
		switch r.Method {
		case http.MethodGet:
			perPage := 30
			if v := r.URL.Query().Get("per_page"); v != "" {
				if n, err := strconv.Atoi(v); err == nil && n > 0 {
					perPage = n
				}
			}
			if perPage > 100 {
				perPage = 100
			}
			page := 1
			if v := r.URL.Query().Get("page"); v != "" {
				if n, err := strconv.Atoi(v); err == nil && n > 0 {
					page = n
				}
			}
			start := (page - 1) * perPage
			if start > len(s.comments) {
				start = len(s.comments)
			}
			end := start + perPage
			if end > len(s.comments) {
				end = len(s.comments)
			}
			slice := s.comments[start:end]
			if end < len(s.comments) {
				next := fmt.Sprintf("<%s/repos/o/r/issues/1/comments?per_page=%d&page=%d>; rel=\"next\"", s.baseURL, perPage, page+1)
				w.Header().Set("Link", next)
			}
			w.Header().Set("Content-Type", "application/json")
			out, _ := json.Marshal(slice)
			w.Write(out)
		case http.MethodPost:
			raw, _ := io.ReadAll(r.Body)
			var payload struct {
				Body string `json:"body"`
			}
			_ = json.Unmarshal(raw, &payload)
			c := comment{ID: s.nextID, Body: payload.Body}
			s.nextID++
			s.comments = append(s.comments, c)
			w.Header().Set("Content-Type", "application/json")
			out, _ := json.Marshal(c)
			w.Write(out)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/repos/o/r/issues/comments/", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.requests = append(s.requests, r.Method+" "+r.URL.Path)
		if r.Method != http.MethodPatch {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		idStr := strings.TrimPrefix(r.URL.Path, "/repos/o/r/issues/comments/")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			http.Error(w, "bad comment id", http.StatusBadRequest)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		var payload struct {
			Body string `json:"body"`
		}
		_ = json.Unmarshal(raw, &payload)
		found := false
		for i := range s.comments {
			if s.comments[i].ID == id {
				s.comments[i].Body = payload.Body
				found = true
			}
		}
		if !found {
			http.Error(w, "no such comment", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		out, _ := json.Marshal(comment{ID: id, Body: payload.Body})
		w.Write(out)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	s.baseURL = srv.URL
	return s
}

func (s *commentServer) snapshot() []comment {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]comment, len(s.comments))
	copy(out, s.comments)
	return out
}

func (s *commentServer) seenRequests() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.requests))
	copy(out, s.requests)
	return out
}

// withAPIBase points githubAPIBase at the given test server for the life
// of the calling test, restoring the production default on cleanup -- so
// run(), the production entry path, talks to the fake server exactly as
// it would talk to api.github.com.
func withAPIBase(t *testing.T, base string) {
	prev := githubAPIBase
	githubAPIBase = base
	t.Cleanup(func() { githubAPIBase = prev })
}

func noTokenEnv(string) string { return "" }

func countRequests(reqs []string, method, pathContains string) int {
	n := 0
	for _, r := range reqs {
		if strings.HasPrefix(r, method+" ") && strings.Contains(r, pathContains) {
			n++
		}
	}
	return n
}

func seedOrdinaryComments(n int) []comment {
	seed := make([]comment, n)
	for i := 0; i < n; i++ {
		seed[i] = comment{ID: int64(i + 1), Body: fmt.Sprintf("ordinary preceding comment number %d, no marker here", i)}
	}
	return seed
}

// TestPRCommentTwoPublicationsCreateThenUpdate is the pagination-move
// probe (task 003, R146, review finding B2): the previous
// actions/github-script step only ever read page one of
// issues.listComments (default per_page=30), so once a PR held more than
// 30 ordinary comments -- or the marker comment itself ended up beyond
// whatever page one held -- every run appended a fresh marker comment
// instead of updating the existing one in place. This drives the
// production run() path (the same one main() calls) against a
// deterministic in-memory GitHub stand-in with 0, 2, 35 (more than one
// default 30-sized page) and 250 (more than one max 100-sized page)
// ordinary comments preceding the marker, and asserts that two
// sequential publications -- as ci.yml's pr-comment job issues on two
// separate workflow runs of the same PR -- yield exactly one create, one
// update, and a marker comment whose body names the second run's sha.
func TestPRCommentTwoPublicationsCreateThenUpdate(t *testing.T) {
	cases := []struct {
		name      string
		preceding int
	}{
		{"no preceding comments", 0},
		{"a couple of preceding comments", 2},
		{"more than one default (30-per-page) page of preceding comments", 35},
		{"more than one max (100-per-page) page of preceding comments", 250},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			seed := seedOrdinaryComments(tc.preceding)
			s := newCommentServer(t, seed)
			withAPIBase(t, s.baseURL)

			const sha1 = "1111111111111111111111111111111111111a"
			const sha2 = "2222222222222222222222222222222222222b"

			if err := run([]string{
				"-repo", "o/r", "-pr", "1",
				"-link", "https://example.invalid/pr/1/", "-sha", sha1,
			}, noTokenEnv); err != nil {
				t.Fatalf("first publication: run: %v", err)
			}

			afterFirst := s.snapshot()
			if len(afterFirst) != tc.preceding+1 {
				t.Fatalf("after first publication: got %d comments, want %d (preceding + 1 marker comment)", len(afterFirst), tc.preceding+1)
			}
			idx1 := findMarkerComment(afterFirst)
			if idx1 < 0 {
				t.Fatalf("after first publication: no comment carries the marker %q", marker)
			}
			if !strings.Contains(afterFirst[idx1].Body, sha1) {
				t.Errorf("after first publication: marker comment body %q does not name sha %q", afterFirst[idx1].Body, sha1)
			}

			if err := run([]string{
				"-repo", "o/r", "-pr", "1",
				"-link", "https://example.invalid/pr/1/", "-sha", sha2,
			}, noTokenEnv); err != nil {
				t.Fatalf("second publication: run: %v", err)
			}

			afterSecond := s.snapshot()
			if len(afterSecond) != tc.preceding+1 {
				t.Fatalf("after second publication: got %d comments, want %d -- a second publication must update, never append", len(afterSecond), tc.preceding+1)
			}
			idx2 := findMarkerComment(afterSecond)
			if idx2 < 0 {
				t.Fatalf("after second publication: no comment carries the marker %q", marker)
			}
			if afterSecond[idx2].ID != afterFirst[idx1].ID {
				t.Errorf("after second publication: marker comment id changed (%d -> %d) -- it must be the same comment, updated in place", afterFirst[idx1].ID, afterSecond[idx2].ID)
			}
			if !strings.Contains(afterSecond[idx2].Body, sha2) {
				t.Errorf("after second publication: marker comment body %q does not name the second sha %q", afterSecond[idx2].Body, sha2)
			}

			seen := s.seenRequests()
			creates := countRequests(seen, "POST", "/comments")
			updates := countRequests(seen, "PATCH", "/comments/")
			if creates != 1 {
				t.Errorf("expected exactly 1 create across both publications, got %d (requests: %v)", creates, seen)
			}
			if updates != 1 {
				t.Errorf("expected exactly 1 update across both publications, got %d (requests: %v)", updates, seen)
			}
		})
	}
}

// TestPRCommentFindsExistingMarkerBeyondFirstHundredComments places the
// marker comment itself past the first 100-comment page (120 ordinary
// comments ahead of it, 20 more after) so a lookup that requests
// per_page=100 but stops at page one would never see it and would wrongly
// create a duplicate marker comment. Exercises the production run() path
// with a single publication and asserts an update against the existing
// comment (0 creates, 1 update), and that a second results page was in
// fact fetched.
func TestPRCommentFindsExistingMarkerBeyondFirstHundredComments(t *testing.T) {
	seed := seedOrdinaryComments(120)
	const oldSha = "0ld00000000000000000000000000000000000a"
	markerID := int64(len(seed) + 1)
	seed = append(seed, comment{ID: markerID, Body: commentBody("https://example.invalid/pr/1/", oldSha)})
	for i := 0; i < 20; i++ {
		seed = append(seed, comment{ID: int64(len(seed) + 1), Body: fmt.Sprintf("later ordinary comment %d", i)})
	}

	s := newCommentServer(t, seed)
	withAPIBase(t, s.baseURL)

	const newSha = "new00000000000000000000000000000000000b"
	if err := run([]string{
		"-repo", "o/r", "-pr", "1",
		"-link", "https://example.invalid/pr/1/", "-sha", newSha,
	}, noTokenEnv); err != nil {
		t.Fatalf("run: %v", err)
	}

	after := s.snapshot()
	if len(after) != len(seed) {
		t.Fatalf("got %d comments, want %d -- the existing marker comment beyond page one must be updated, never duplicated", len(after), len(seed))
	}
	var found *comment
	for i := range after {
		if after[i].ID == markerID {
			found = &after[i]
		}
	}
	if found == nil {
		t.Fatalf("comment %d (the pre-existing marker comment) vanished", markerID)
	}
	if !strings.Contains(found.Body, newSha) {
		t.Errorf("updated marker comment body %q does not name the new sha %q", found.Body, newSha)
	}

	seen := s.seenRequests()
	if creates := countRequests(seen, "POST", "/comments"); creates != 0 {
		t.Errorf("expected 0 creates (an update was possible), got %d (requests: %v)", creates, seen)
	}
	if updates := countRequests(seen, "PATCH", "/comments/"); updates != 1 {
		t.Errorf("expected exactly 1 update, got %d (requests: %v)", updates, seen)
	}
	sawPage2 := false
	for _, r := range seen {
		if strings.HasPrefix(r, "GET ") && strings.Contains(r, "page=2") {
			sawPage2 = true
		}
	}
	if !sawPage2 {
		t.Errorf("never fetched comments page 2 -- the marker comment beyond page one would have been invisible without it; requests: %v", seen)
	}
}

// TestFindMarkerComment is a small, direct unit test of the marker scan
// itself, independent of any HTTP plumbing.
func TestFindMarkerComment(t *testing.T) {
	comments := []comment{
		{ID: 1, Body: "hello"},
		{ID: 2, Body: marker + "\nold"},
		{ID: 3, Body: "world"},
	}
	if idx := findMarkerComment(comments); idx != 1 {
		t.Fatalf("findMarkerComment: got index %d, want 1", idx)
	}
	if idx := findMarkerComment(comments[:1]); idx != -1 {
		t.Fatalf("findMarkerComment on a marker-free slice: got index %d, want -1", idx)
	}
}

// TestCommentBodyNamesLinkAndSha guards the body-rendering contract every
// other test above relies on: the comment body must carry both the
// report link and the head sha verbatim.
func TestCommentBodyNamesLinkAndSha(t *testing.T) {
	body := commentBody("https://example.invalid/pr/7/", "deadbeefcafef00dfeedface1234567890abcde")
	if !strings.Contains(body, marker) {
		t.Errorf("commentBody %q does not carry the marker %q", body, marker)
	}
	if !strings.Contains(body, "https://example.invalid/pr/7/") {
		t.Errorf("commentBody %q does not name the report link", body)
	}
	if !strings.Contains(body, "deadbeefcafef00dfeedface1234567890abcde") {
		t.Errorf("commentBody %q does not name the head sha", body)
	}
}
