package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// releaseAPI is a fake GitHub release API: it answers
// /repos/n-orlov/deck/releases/tags/<tagV> with status and body, and counts
// every request and any that carried credentials.
type releaseAPI struct {
	srv       *httptest.Server
	hits      atomic.Int32
	withAuth  atomic.Int32
	wrongPath atomic.Int32
}

func serveReleaseAPI(t *testing.T, status int, body string) *releaseAPI {
	t.Helper()
	api := &releaseAPI{}
	api.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		api.hits.Add(1)
		if r.Header.Get("Authorization") != "" {
			api.withAuth.Add(1)
		}
		if r.URL.Path != "/repos/"+DefaultRepo+"/releases/tags/"+tagV {
			api.wrongPath.Add(1)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(api.srv.Close)
	return api
}

// downloadCounter wraps a release server's client transport so a test can
// see whether anything was downloaded.
func downloadCounter(srv *httptest.Server) (*http.Client, map[string]int) {
	hits := map[string]int{}
	c := srv.Client()
	c.Transport = countingTransport{base: http.DefaultTransport, hits: hits}
	return c, hits
}

func downloads(hits map[string]int) int {
	n := 0
	for p, c := range hits {
		if strings.Contains(p, "/releases/download/") {
			n += c
		}
	}
	return n
}

var oversizeImmutableBody = `{"immutable":true,"pad":"` + strings.Repeat("a", maxReleaseAPIBytes) + `"}`

// immutabilityAnswers are the release API answers Run must classify, each
// with the state it maps to. Only a JSON boolean true is immutable.
var immutabilityAnswers = []struct {
	name   string
	status int
	body   string
	want   string
}{
	{"immutable true", 200, `{"tag_name":"v1.2.3","immutable":true}`, ImmutableVerified},
	{"immutable false", 200, `{"tag_name":"v1.2.3","immutable":false}`, ImmutableNot},
	{"field absent", 200, `{"tag_name":"v1.2.3"}`, ImmutableUnknown},
	{"string true", 200, `{"immutable":"true"}`, ImmutableUnknown},
	{"number one", 200, `{"immutable":1}`, ImmutableUnknown},
	{"null", 200, `{"immutable":null}`, ImmutableUnknown},
	{"wrong case key", 200, `{"Immutable":true}`, ImmutableUnknown},
	{"malformed JSON", 200, `{"immutable":tru`, ImmutableUnknown},
	{"JSON array", 200, `[{"immutable":true}]`, ImmutableUnknown},
	{"JSON null body", 200, `null`, ImmutableUnknown},
	{"not found", 404, `{"message":"Not Found"}`, ImmutableUnknown},
	{"rate limited", 403, `{"message":"API rate limit exceeded","immutable":true}`, ImmutableUnknown},
	{"oversize body", 200, oversizeImmutableBody, ImmutableUnknown},
}

// setImmutableEnv sets DECK_REQUIRE_IMMUTABLE for one test; immutableUnset
// removes it altogether (the default). Either is restored afterwards.
func setImmutableEnv(t *testing.T, value string) {
	t.Helper()
	if value != immutableUnset {
		t.Setenv(EnvRequireImmutable, value)
		return
	}
	t.Setenv(EnvRequireImmutable, "") // registers the restore
	_ = os.Unsetenv(EnvRequireImmutable)
}

// immutableUnset stands for "DECK_REQUIRE_IMMUTABLE is not in the
// environment" in setImmutableEnv; no real value contains a NUL.
const immutableUnset = "\x00unset"

// requiringValues are the DECK_REQUIRE_IMMUTABLE settings that require an
// immutable release: unset (the default), empty, "1", and any other value
// that is not exactly "0".
var requiringValues = []struct{ name, value string }{
	{"unset", immutableUnset}, {"empty", ""}, {"one", "1"}, {"true", "true"}, {"zero padded", " 0"}, {"false", "false"},
}

// TestImmutabilityReportedWhenOptedOut: with DECK_REQUIRE_IMMUTABLE=0 every
// answer installs the release, and Result.Immutable carries the state the
// answer maps to. The request is unauthenticated and names the pinned
// repository and the tag.
func TestImmutabilityReportedWhenOptedOut(t *testing.T) {
	for _, c := range immutabilityAnswers {
		t.Run(c.name, func(t *testing.T) {
			setImmutableEnv(t, "0")
			rel := newRelease(t)
			srv := rel.serve(t)
			api := serveReleaseAPI(t, c.status, c.body)
			_, exe := installOld(t)
			res, err := Run(context.Background(), Options{BaseURL: srv.URL, Exe: exe, GOOS: "linux", GOARCH: "amd64", apiBase: api.srv.URL})
			if err != nil {
				t.Fatal(err)
			}
			if res.Immutable != c.want {
				t.Errorf("Immutable = %q, want %q", res.Immutable, c.want)
			}
			if fileSum(t, exe) != sum([]byte(newBody)) {
				t.Error("the release was not installed")
			}
			if api.hits.Load() != 1 || api.wrongPath.Load() != 0 || api.withAuth.Load() != 0 {
				t.Errorf("API hits %d, wrong path %d, with auth %d; want one unauthenticated request for the tag", api.hits.Load(), api.wrongPath.Load(), api.withAuth.Load())
			}
		})
	}
}

// strictRefused runs one update with immutability required and asserts it was
// refused, as a RefusedNotImmutable Refusal naming the cause and the
// DECK_REQUIRE_IMMUTABLE=0 opt-out and carrying no server text, with nothing
// downloaded, the old binary and its directory byte-identical and no scratch
// directory left behind.
func strictRefused(t *testing.T, o Options, rel *release, srv *httptest.Server, wantInErr string) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	client, hits := downloadCounter(srv)
	o.Client = client
	dir, exe := installOld(t)
	o.Exe = exe
	beforeSum, beforeList := fileSum(t, exe), listing(t, dir)
	res, err := Run(context.Background(), o)
	if err == nil || !strings.Contains(err.Error(), "set "+EnvRequireImmutable+"=0 to install it anyway") || !strings.Contains(err.Error(), wantInErr) {
		t.Fatalf("err = %v (res %+v), want a refusal naming the %s=0 opt-out and %q", err, res, EnvRequireImmutable, wantInErr)
	}
	if r, ok := AsRefusal(err); !ok || r.Reason != RefusedNotImmutable {
		t.Errorf("err = %#v, want a %s Refusal", err, RefusedNotImmutable)
	}
	for _, server := range []string{"rate limit", "Not Found", "JSON", "boolean", "immutable field"} {
		if strings.Contains(err.Error(), server) {
			t.Errorf("err %q quotes %q; the refusal is deck's own wording", err, server)
		}
	}
	if res != (Result{}) {
		t.Errorf("res = %+v, want none on a refused update", res)
	}
	if got := fileSum(t, exe); got != beforeSum {
		t.Errorf("old binary sha256 = %s, want %s unchanged", got, beforeSum)
	}
	if got := listing(t, dir); !reflect.DeepEqual(got, beforeList) {
		t.Errorf("install dir = %v, want %v", got, beforeList)
	}
	if got := listing(t, tmp); len(got) != 0 {
		t.Errorf("temporary directory holds %v, want nothing", got)
	}
	if n := downloads(hits); n != 0 {
		t.Errorf("%d release downloads before the refusal, want none (%v)", n, rel.asset)
	}
}

// TestImmutabilityRequiredRefusesEveryOtherState: by default (and with any
// DECK_REQUIRE_IMMUTABLE value but "0") every answer but immutable:true
// refuses the update before anything is downloaded; immutable:true installs.
func TestImmutabilityRequiredRefusesEveryOtherState(t *testing.T) {
	for _, mode := range requiringValues {
		for _, c := range immutabilityAnswers {
			t.Run(mode.name+"/"+c.name, func(t *testing.T) {
				setImmutableEnv(t, mode.value)
				rel := newRelease(t)
				srv := rel.serve(t)
				api := serveReleaseAPI(t, c.status, c.body)
				o := Options{BaseURL: srv.URL, GOOS: "linux", GOARCH: "amd64", apiBase: api.srv.URL}
				if c.want != ImmutableVerified {
					want := "could not be confirmed immutable: the GitHub release API gave no usable answer"
					if c.want == ImmutableNot {
						want = "is not a GitHub immutable release"
					}
					strictRefused(t, o, rel, srv, want)
					return
				}
				_, exe := installOld(t)
				o.Exe = exe
				res, err := Run(context.Background(), o)
				if err != nil || res.Immutable != ImmutableVerified || fileSum(t, exe) != sum([]byte(newBody)) {
					t.Fatalf("res %+v err %v, want the immutable release installed", res, err)
				}
			})
		}
	}
}

// TestImmutabilityRequiredByOption: Options.RequireImmutable requires an
// immutable release even when DECK_REQUIRE_IMMUTABLE=0 opts out.
func TestImmutabilityRequiredByOption(t *testing.T) {
	setImmutableEnv(t, "0")
	rel := newRelease(t)
	srv := rel.serve(t)
	api := serveReleaseAPI(t, 200, `{"immutable":false}`)
	strictRefused(t, Options{BaseURL: srv.URL, GOOS: "linux", GOARCH: "amd64", apiBase: api.srv.URL, RequireImmutable: true}, rel, srv, "is not a GitHub immutable release")
}

// TestImmutabilityOnAMirrorIsUnknown: a DECK_UPDATE_URL mirror has no release
// API, so nothing is asked. With DECK_REQUIRE_IMMUTABLE=0 the update installs
// with the state unknown; by default, and with any other value, it is refused
// naming the mirror.
func TestImmutabilityOnAMirrorIsUnknown(t *testing.T) {
	rel := newRelease(t)
	srv := rel.serve(t)
	t.Setenv(EnvBaseURL, srv.URL)
	setImmutableEnv(t, "0")
	_, exe := installOld(t)
	res, err := Run(context.Background(), Options{Exe: exe, GOOS: "linux", GOARCH: "amd64"})
	if err != nil || res.Immutable != ImmutableUnknown || fileSum(t, exe) != sum([]byte(newBody)) {
		t.Fatalf("res %+v err %v, want an install with the state unknown", res, err)
	}
	for _, mode := range requiringValues {
		t.Run(mode.name, func(t *testing.T) {
			setImmutableEnv(t, mode.value)
			strictRefused(t, Options{GOOS: "linux", GOARCH: "amd64"}, rel, srv, "could not be confirmed immutable: the release source is a mirror with no GitHub release API")
		})
	}
}

// TestImmutabilityTimeoutIsUnknown: a release API that does not answer in
// time is unknown, and with DECK_REQUIRE_IMMUTABLE=0 (the package baseline)
// the update still installs.
func TestImmutabilityTimeoutIsUnknown(t *testing.T) {
	old := immutableTimeout
	immutableTimeout = 50 * time.Millisecond
	t.Cleanup(func() { immutableTimeout = old })
	release := make(chan struct{})
	api := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	t.Cleanup(api.Close)
	t.Cleanup(func() { close(release) })
	rel := newRelease(t)
	srv := rel.serve(t)
	_, exe := installOld(t)
	res, err := Run(context.Background(), Options{BaseURL: srv.URL, Exe: exe, GOOS: "linux", GOARCH: "amd64", apiBase: api.URL})
	if err != nil || res.Immutable != ImmutableUnknown {
		t.Fatalf("res %+v err %v, want an install with the state unknown", res, err)
	}
}

// TestImmutabilityRefusesPlainHTTP: the release API is held to the same
// https-only rule as every download, so a plain-http API base is never asked.
func TestImmutabilityRefusesPlainHTTP(t *testing.T) {
	o := Options{apiBase: "http://api.example.invalid"}.withDefaults()
	state, why := o.releaseImmutability(context.Background(), DefaultBaseURL, tagV)
	if state != ImmutableUnknown || why == nil || !strings.Contains(why.Error(), "only https") {
		t.Fatalf("state %q why %v, want unknown refused for plain http", state, why)
	}
}

// TestImmutabilityAPIBaseIsPinned: the default release base asks the real
// GitHub API; any other base has none.
func TestImmutabilityAPIBaseIsPinned(t *testing.T) {
	if got := (Options{}).releaseAPIBase(DefaultBaseURL); got != DefaultAPIBase {
		t.Errorf("default base asks %q, want %q", got, DefaultAPIBase)
	}
	if got := (Options{Repo: "someone/else"}).releaseAPIBase("https://mirror.example/deck"); got != "" {
		t.Errorf("a mirror asks %q, want no API", got)
	}
}

// TestPlanImmutable: the plan the ctrl+u confirmation shows, asked without a
// request.
func TestPlanImmutable(t *testing.T) {
	for _, c := range []struct {
		name     string
		env      string
		option   bool
		base     string
		apiBase  string
		wantPlan string
	}{
		{"default", immutableUnset, false, "", "", ImmutableRequired},
		{"default, mirror", immutableUnset, false, "https://mirror.example/deck", "", ImmutableRefused},
		{"empty", "", false, "", "", ImmutableRequired},
		{"opted out", "0", false, "", "", ImmutableReported},
		{"opted out, mirror", "0", false, "https://mirror.example/deck", "", ImmutableReported},
		{"required by env, default base", "1", false, "", "", ImmutableRequired},
		{"required by option despite opt-out", "0", true, "", "", ImmutableRequired},
		{"required, mirror", "1", false, "https://mirror.example/deck", "", ImmutableRefused},
		{"required, mirror with test API", "1", false, "https://mirror.example/deck", "http://127.0.0.1:1", ImmutableRequired},
		{"env not 0", "false", false, "", "", ImmutableRequired},
	} {
		t.Run(c.name, func(t *testing.T) {
			setImmutableEnv(t, c.env)
			t.Setenv(EnvBaseURL, "")
			o := Options{BaseURL: c.base, RequireImmutable: c.option, apiBase: c.apiBase}
			if got := PlanImmutable(o); got != c.wantPlan {
				t.Errorf("PlanImmutable = %q, want %q", got, c.wantPlan)
			}
		})
	}
}
