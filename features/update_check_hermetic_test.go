package features

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/testnet"
)

// Part C: no fixture that starts deck contacts a public release endpoint. The
// scenario launcher turns the default-on check off (a launched deck would
// otherwise ask github.com), a scenario about the check points it at a local
// fake release server, and a rejecting proxy records anything else.
func TestScenarioLaunchesNeverReachPublicReleaseEndpoints(t *testing.T) {
	binary := buildDeckBinary(t)

	launch := func(t *testing.T, socket string, env ...string) *testnet.Guard {
		t.Helper()
		guard, err := testnet.Start()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(guard.Close)
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		t.Cleanup(cancel)
		base := []string{"DECK_HOME=" + t.TempDir(), "DECK_TMUX_SOCKET=" + socket, "DECK_ASCII=1", "DECK_ANIM=0", "NO_COLOR=1"}
		driver, err := StartScreenDriver(ctx, binary, append(append(base, guard.Env()...), env...))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = driver.Stop(time.Second) })
		if err := driver.WaitForFrame(ctx, false, "No sessions"); err != nil {
			t.Fatal(err)
		}
		return guard
	}
	fake := func(t *testing.T) (*httptest.Server, *atomic.Int32) {
		var asked atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			asked.Add(1)
			http.Redirect(w, r, "/releases/tag/v0.0.1", http.StatusFound)
		}))
		t.Cleanup(srv.Close)
		return srv, &asked
	}
	waitFor := func(cond func() bool) bool {
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			if cond() {
				return true
			}
			time.Sleep(25 * time.Millisecond)
		}
		return cond()
	}

	t.Run("a launch with the scenario defaults makes no request at all", func(t *testing.T) {
		srv, asked := fake(t)
		// StartScreenDriver's own defaults, plus a fake endpoint that would
		// answer if the check ran.
		guard := launch(t, "deck_hermetic_default", "DECK_UPDATE_URL="+srv.URL)
		time.Sleep(1500 * time.Millisecond)
		if n := asked.Load(); n != 0 {
			t.Errorf("the default launch asked the release server %d times; the check must be off", n)
		}
		if got := guard.Requests(); len(got) != 0 {
			t.Errorf("the default launch reached %v", got)
		}
	})

	t.Run("a scenario about the check reaches only the local fake server", func(t *testing.T) {
		srv, asked := fake(t)
		guard := launch(t, "deck_hermetic_local", "DECK_UPDATE_CHECK=1", "DECK_UPDATE_URL="+srv.URL)
		if !waitFor(func() bool { return asked.Load() > 0 }) {
			t.Fatal("the fake release server was never asked: the check is off where a scenario turned it on")
		}
		if got := guard.Requests(); len(got) != 0 {
			t.Errorf("the check reached %v besides the local fake server", got)
		}
	})

	t.Run("the monitor records the default endpoint when the check is on without a fake server", func(t *testing.T) {
		guard := launch(t, "deck_hermetic_public", "DECK_UPDATE_CHECK=1")
		if !waitFor(func() bool { return len(guard.Violations()) > 0 }) {
			t.Fatal("the monitor saw nothing for a launch that checks the default release endpoint")
		}
		if got := strings.Join(guard.Violations(), " "); !strings.Contains(got, "github.com") {
			t.Fatalf("recorded %q, want the github.com release endpoint", got)
		}
	})
}

// The scenario harness's environment (what every godog client starts with)
// turns the check off, and a scenario's own entry for it wins.
func TestScenarioHarnessEnvironmentDisablesTheUpdateCheck(t *testing.T) {
	h := &ScenarioHarness{}
	last := func(env []string) string {
		got := ""
		for _, kv := range env {
			if strings.HasPrefix(kv, "DECK_UPDATE_CHECK=") {
				got = kv
			}
		}
		return got
	}
	if got := last(h.Environment()); got != "DECK_UPDATE_CHECK=0" {
		t.Errorf("default harness environment sets %q, want DECK_UPDATE_CHECK=0", got)
	}
	if got := last(h.Environment("DECK_UPDATE_CHECK=1")); got != "DECK_UPDATE_CHECK=1" {
		t.Errorf("a scenario's own DECK_UPDATE_CHECK=1 resolves to %q", got)
	}
}
