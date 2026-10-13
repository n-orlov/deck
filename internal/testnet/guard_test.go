package testnet

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"
)

func clientVia(t *testing.T, g *Guard) *http.Client {
	t.Helper()
	proxy, err := url.Parse("http://" + g.addr)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxy)}}
}

// A request for a public host is refused and recorded; the same client reaches
// nothing, so a fixture that forgot to disable the check cannot succeed
// silently.
func TestGuardRefusesAndRecordsPublicRequests(t *testing.T) {
	g, err := Start()
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	client := clientVia(t, g)
	resp, err := client.Get("http://releases.example.test/latest")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("plain request status = %d, want 403", resp.StatusCode)
	}
	if resp, err := client.Get("https://github.com/n-orlov/deck/releases/latest"); err == nil {
		_ = resp.Body.Close()
		t.Fatal("an https request through the guard succeeded")
	}
	want := []string{"CONNECT github.com:443", "GET releases.example.test"}
	if got := g.Violations(); !reflect.DeepEqual(got, want) {
		t.Fatalf("violations = %v, want %v", got, want)
	}
}

// The Go toolchain's module traffic is recorded but is not a violation.
func TestGuardIgnoresGoToolchainHosts(t *testing.T) {
	for _, row := range []struct {
		request string
		counts  bool
	}{
		{"CONNECT proxy.golang.org:443", false},
		{"CONNECT sum.golang.org:443", false},
		{"CONNECT api.github.com:443", true},
		{"CONNECT objects.githubusercontent.com:443", true},
		{"CONNECT notgolang.org:443", true},
	} {
		if got := !isGoInfrastructure(row.request); got != row.counts {
			t.Errorf("%s counts as a violation = %v, want %v", row.request, got, row.counts)
		}
	}
}

// A loopback fake server is never proxied, so it is reachable and unrecorded.
func TestGuardLeavesLoopbackFakeServersAlone(t *testing.T) {
	g, err := Start()
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	defer fake.Close()
	t.Setenv("HTTP_PROXY", "http://"+g.addr)
	t.Setenv("NO_PROXY", "")
	resp, err := (&http.Client{Transport: &http.Transport{Proxy: http.ProxyFromEnvironment}}).Get(fake.URL)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("loopback request status = %d", resp.StatusCode)
	}
	if got := g.Requests(); len(got) != 0 {
		t.Fatalf("the guard saw %v for a loopback server", got)
	}
}

// finish keeps the tests' own exit code when nothing reached a public host,
// and turns a clean run into a failure naming the host when something did.
func TestFinishFailsAPackageThatReachedAPublicHost(t *testing.T) {
	g, err := Start()
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	var out strings.Builder
	if got := finish(g, 0, &out); got != 0 || out.Len() != 0 {
		t.Fatalf("clean guard: finish = %d, output %q; want 0 and silence", got, out.String())
	}
	if got := finish(g, 3, &out); got != 3 {
		t.Fatalf("clean guard keeps a failing code: finish = %d, want 3", got)
	}
	resp, err := clientVia(t, g).Get("http://updates.example.test/releases/latest")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if got := finish(g, 0, &out); got != 1 {
		t.Fatalf("finish = %d after a public request, want 1", got)
	}
	if !strings.Contains(out.String(), "updates.example.test") || !strings.Contains(out.String(), "DECK_UPDATE_CHECK=0") {
		t.Fatalf("report = %q, want the host and the remedy", out.String())
	}
}

// run routes the process's proxy variables through the guard for the duration
// of the tests and returns their code.
func TestRunRoutesTheProcessThroughTheGuard(t *testing.T) {
	for _, name := range []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy", "NO_PROXY", "no_proxy"} {
		t.Setenv(name, "") // restored when the test ends
	}
	var seen string
	var out strings.Builder
	code := run(func() int { seen = os.Getenv("HTTPS_PROXY"); return 7 }, &out)
	if code != 7 || !strings.HasPrefix(seen, "http://127.0.0.1:") || out.Len() != 0 {
		t.Fatalf("run = %d, HTTPS_PROXY during tests = %q, output %q", code, seen, out.String())
	}
}
