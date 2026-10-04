package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// recordedRequest is what scriptedGitHub saw for one request.
type recordedRequest struct {
	method, path, auth, contentType, body string
}

// scriptedGitHub serves a fixed handler per "METHOD path-prefix" and records
// every request, so a test can assert both the outcome run() reports and
// what it did (or did not) send after a failure.
type scriptedGitHub struct {
	mu   sync.Mutex
	seen []recordedRequest
}

func newScriptedGitHub(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) *scriptedGitHub {
	t.Helper()
	s := &scriptedGitHub{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		s.mu.Lock()
		s.seen = append(s.seen, recordedRequest{r.Method, r.URL.Path, r.Header.Get("Authorization"), r.Header.Get("Content-Type"), string(b)})
		s.mu.Unlock()
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	withAPIBase(t, srv.URL)
	return s
}

func (s *scriptedGitHub) requests() []recordedRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]recordedRequest(nil), s.seen...)
}

var validArgs = []string{"-repo", "o/r", "-pr", "7", "-link", "https://x/report", "-sha", "abc123"}

func TestRunRejectsMissingRequiredFlagsWithoutTouchingTheNetwork(t *testing.T) {
	gh := newScriptedGitHub(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(500) })
	for _, missing := range []string{"-repo", "-pr", "-link", "-sha"} {
		var args []string
		for i := 0; i < len(validArgs); i += 2 {
			if validArgs[i] != missing {
				args = append(args, validArgs[i], validArgs[i+1])
			}
		}
		err := run(args, noTokenEnv)
		if err == nil || !strings.Contains(err.Error(), "-repo, -pr, -link and -sha are all required") {
			t.Fatalf("missing %s: want the all-required error, got %v", missing, err)
		}
	}
	if n := len(gh.requests()); n != 0 {
		t.Fatalf("a rejected invocation must not call GitHub; saw %d requests", n)
	}
}

func TestRunReportsUnknownFlag(t *testing.T) {
	if err := run([]string{"-nonsense"}, noTokenEnv); err == nil {
		t.Fatal("an unknown flag must make run fail")
	}
}

func TestRunSendsBearerTokenPreferringGitHubTokenOverGhToken(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"GITHUB_TOKEN wins", map[string]string{"GITHUB_TOKEN": "primary", "GH_TOKEN": "fallback"}, "Bearer primary"},
		{"GH_TOKEN fallback", map[string]string{"GH_TOKEN": "fallback"}, "Bearer fallback"},
		{"no token sends no Authorization", nil, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gh := newScriptedGitHub(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					_, _ = w.Write([]byte("[]"))
					return
				}
				w.WriteHeader(201)
			})
			if err := run(validArgs, func(k string) string { return tc.env[k] }); err != nil {
				t.Fatalf("run: %v", err)
			}
			reqs := gh.requests()
			if len(reqs) != 2 {
				t.Fatalf("want list + create, got %+v", reqs)
			}
			for _, r := range reqs {
				if r.auth != tc.want {
					t.Errorf("%s %s: Authorization = %q, want %q", r.method, r.path, r.auth, tc.want)
				}
			}
			if reqs[1].contentType != "application/json" {
				t.Errorf("the create must declare a JSON body, got Content-Type %q", reqs[1].contentType)
			}
			if reqs[0].contentType != "" {
				t.Errorf("a body-less GET must not declare a Content-Type, got %q", reqs[0].contentType)
			}
		})
	}
}

func TestPublishSurfacesListFailureAndNeverWrites(t *testing.T) {
	gh := newScriptedGitHub(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "rate limited", http.StatusForbidden)
	})
	err := run(validArgs, noTokenEnv)
	if err == nil {
		t.Fatal("a 403 on the listing must fail the publication")
	}
	for _, want := range []string{"GET", "403", "rate limited"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q should name %q", err, want)
		}
	}
	for _, r := range gh.requests() {
		if r.method != http.MethodGet {
			t.Errorf("after a failed listing run must not %s anything", r.method)
		}
	}
}

func TestPublishRejectsUnparseableListingAndNeverWrites(t *testing.T) {
	gh := newScriptedGitHub(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("<html>not json</html>"))
	})
	err := run(validArgs, noTokenEnv)
	if err == nil || !strings.Contains(err.Error(), "issue 7: could not parse comments response") {
		t.Fatalf("want a parse error naming the issue, got %v", err)
	}
	if n := len(gh.requests()); n != 1 {
		t.Fatalf("only the listing may be attempted, saw %d requests", n)
	}
}

func TestPublishSurfacesCreateAndUpdateFailures(t *testing.T) {
	cases := []struct {
		name       string
		listing    string
		failMethod string
	}{
		{"create fails", `[{"id":1,"body":"hello"}]`, http.MethodPost},
		{"update fails", `[{"id":9,"body":"` + marker + ` old"}]`, http.MethodPatch},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gh := newScriptedGitHub(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					_, _ = w.Write([]byte(tc.listing))
					return
				}
				http.Error(w, "boom", http.StatusUnprocessableEntity)
			})
			err := run(validArgs, noTokenEnv)
			if err == nil {
				t.Fatal("a rejected write must fail the publication")
			}
			if !strings.Contains(err.Error(), tc.failMethod) || !strings.Contains(err.Error(), "422") || !strings.Contains(err.Error(), "boom") {
				t.Errorf("error %q should name the method, status and response body", err)
			}
			reqs := gh.requests()
			if len(reqs) != 2 || reqs[1].method != tc.failMethod {
				t.Fatalf("want one listing then one %s, got %+v", tc.failMethod, reqs)
			}
			if tc.failMethod == http.MethodPatch && reqs[1].path != "/repos/o/r/issues/comments/9" {
				t.Errorf("update must target the marker comment's own id, got %s", reqs[1].path)
			}
			if !strings.Contains(reqs[1].body, "abc123") || !strings.Contains(reqs[1].body, "https://x/report") {
				t.Errorf("write body must carry the sha and link, got %q", reqs[1].body)
			}
		})
	}
}

func TestDoGitHubRequestTransportAndURLErrors(t *testing.T) {
	// A server that was started and closed: connecting must fail.
	srv := httptest.NewServer(http.NotFoundHandler())
	dead := srv.URL
	srv.Close()
	withAPIBase(t, dead)
	if _, err := listComments("o/r", 1, ""); err == nil {
		t.Error("listing against an unreachable server must fail")
	}
	if err := createComment("o/r", 1, "b", ""); err == nil {
		t.Error("creating against an unreachable server must fail")
	}
	if err := updateComment("o/r", 1, "b", ""); err == nil {
		t.Error("updating against an unreachable server must fail")
	}
	// An invalid URL cannot even build a request.
	if _, _, err := doGitHubRequest(http.MethodGet, "http://bad host/\x7f", "", nil); err == nil {
		t.Error("a malformed URL must fail request construction")
	}
	if _, _, err := doGitHubRequest("BAD METHOD", "http://localhost/", "", nil); err == nil {
		t.Error("an invalid method must fail request construction")
	}
}

func TestDoGitHubRequestReturnsBodyAndLinkHeader(t *testing.T) {
	newScriptedGitHub(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Link", `<https://next>; rel="next"`)
		_, _ = w.Write([]byte("payload"))
	})
	body, link, err := doGitHubRequest(http.MethodGet, githubAPIBase+"/x", "", nil)
	if err != nil || string(body) != "payload" || link != `<https://next>; rel="next"` {
		t.Fatalf("got body=%q link=%q err=%v", body, link, err)
	}
}

func TestNextPageURL(t *testing.T) {
	cases := []struct {
		name, header, want string
	}{
		{"empty", "", ""},
		{"next among others", `<https://a/p1>; rel="prev", <https://a/p3>; rel="next", <https://a/p9>; rel="last"`, "https://a/p3"},
		{"only last", `<https://a/p9>; rel="last"`, ""},
		{"segment without rel", `<https://a/p2>`, ""},
		{"url not in angle brackets", `https://a/p2; rel="next"`, ""},
		{"missing closing bracket", `<https://a/p2; rel="next"`, ""},
		{"rel among extra params", `<https://a/p2>; title="x"; rel="next"`, "https://a/p2"},
		{"malformed part skipped, later next honoured", `garbage, <https://a/p4>; rel="next"`, "https://a/p4"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := nextPageURL(tc.header); got != tc.want {
				t.Errorf("nextPageURL(%q) = %q, want %q", tc.header, got, tc.want)
			}
		})
	}
}

func TestListCommentsFollowsLinkHeaderAcrossPagesAndStopsAtLast(t *testing.T) {
	var srvURL string
	gh := newScriptedGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("page") {
		case "":
			w.Header().Set("Link", `<`+srvURL+`/repos/o/r/issues/7/comments?page=2>; rel="next"`)
			_, _ = w.Write([]byte(`[{"id":1,"body":"a"}]`))
		default:
			_, _ = w.Write([]byte(`[{"id":2,"body":"b"}]`))
		}
	})
	srvURL = githubAPIBase
	got, err := listComments("o/r", 7, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != 1 || got[1].ID != 2 {
		t.Fatalf("want both pages in order, got %+v", got)
	}
	if n := len(gh.requests()); n != 2 {
		t.Errorf("want exactly 2 page fetches, got %d", n)
	}
}
