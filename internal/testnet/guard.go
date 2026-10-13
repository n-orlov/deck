// Package testnet is the test-side guard that keeps the suite off the public
// network. A Guard is a loopback HTTP proxy that refuses and records every
// request it is asked to carry. Pointed at by the proxy variables, it sees
// anything a test, or a deck binary a test launches, sends to a non-loopback
// host: Go's HTTP client never proxies a loopback destination, so a local
// fake release server (httptest, DECK_UPDATE_URL) is reached directly and is
// not recorded.
package testnet

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

// goInfrastructure are the hosts the Go toolchain may reach while a test
// builds a binary with `go build`. They are not deck traffic.
var goInfrastructure = []string{"proxy.golang.org", "sum.golang.org", "golang.org", "go.dev"}

// Guard is a running rejecting proxy.
type Guard struct {
	addr     string
	listener net.Listener
	server   *http.Server

	mu       sync.Mutex
	requests []string
}

// Start listens on a loopback port and refuses everything it receives.
func Start() (*Guard, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	g := &Guard{addr: listener.Addr().String(), listener: listener}
	g.server = &http.Server{Handler: http.HandlerFunc(g.refuse), ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = g.server.Serve(listener) }()
	return g, nil
}

func (g *Guard) refuse(w http.ResponseWriter, r *http.Request) {
	target := r.Host
	if r.Method != http.MethodConnect && r.URL != nil && r.URL.Host != "" {
		target = r.URL.Host
	}
	g.mu.Lock()
	g.requests = append(g.requests, r.Method+" "+target)
	g.mu.Unlock()
	http.Error(w, "the test suite makes no public network requests", http.StatusForbidden)
}

// Close stops the proxy.
func (g *Guard) Close() { _ = g.server.Close() }

// Env is the environment that routes a child's HTTP and HTTPS traffic through
// the guard and clears every exemption.
func (g *Guard) Env() []string {
	url := "http://" + g.addr
	return []string{
		"HTTP_PROXY=" + url, "HTTPS_PROXY=" + url, "ALL_PROXY=" + url,
		"http_proxy=" + url, "https_proxy=" + url, "all_proxy=" + url,
		"NO_PROXY=", "no_proxy=",
	}
}

// Requests returns every recorded request as "METHOD host:port", including
// Go toolchain traffic. Use Violations for the ones that count.
func (g *Guard) Requests() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]string(nil), g.requests...)
}

// Violations is Requests without the Go toolchain's own module traffic, sorted
// and de-duplicated.
func (g *Guard) Violations() []string {
	seen := map[string]bool{}
	var out []string
	for _, request := range g.Requests() {
		if isGoInfrastructure(request) || seen[request] {
			continue
		}
		seen[request] = true
		out = append(out, request)
	}
	sort.Strings(out)
	return out
}

func isGoInfrastructure(request string) bool {
	_, target, _ := strings.Cut(request, " ")
	host, _, err := net.SplitHostPort(target)
	if err != nil {
		host = target
	}
	for _, known := range goInfrastructure {
		if host == known || strings.HasSuffix(host, "."+known) {
			return true
		}
	}
	return false
}

// Main runs a package's tests behind a process-wide guard: every test, and
// every binary a test launches from os.Environ, is routed through it. The
// package fails if anything asked for a public host. Use it as
//
//	func TestMain(m *testing.M) { os.Exit(testnet.Main(m)) }
func Main(m *testing.M) int { return run(m.Run, os.Stderr) }

// run starts the guard, routes the process through it, runs the tests and
// folds the guard's verdict into their exit code.
func run(runTests func() int, stderr io.Writer) int {
	guard, err := Start()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "testnet: start guard: %v\n", err)
		return 1
	}
	defer guard.Close()
	for _, kv := range guard.Env() {
		name, value, _ := strings.Cut(kv, "=")
		_ = os.Setenv(name, value)
	}
	return finish(guard, runTests(), stderr)
}

// finish is code, or 1 with the public hosts named when the guard recorded any.
func finish(guard *Guard, code int, stderr io.Writer) int {
	violations := guard.Violations()
	if len(violations) == 0 {
		return code
	}
	_, _ = fmt.Fprintf(stderr, "testnet: the tests reached the public network: %s\n"+
		"a fixture that starts deck must use a local fake release server (DECK_UPDATE_URL) or set DECK_UPDATE_CHECK=0\n",
		strings.Join(violations, ", "))
	return 1
}
