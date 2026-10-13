package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/n-orlov/deck/internal/config"
)

// R297/R302: the production update source carries no copy of the startup
// settings. Settings that started with the check off still reach the server
// once the model (which owns the live settings, SPEC §6.5) asks for a check;
// the model-level gate is covered by the TestLive* tests in internal/tui.
func TestUpdateIndicatorSourceHoldsNoStartupSettings(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.Redirect(w, r, "/releases/tag/v9.9.9", http.StatusFound)
	}))
	defer srv.Close()
	t.Setenv("DECK_UPDATE_URL", srv.URL)
	withVersion(t, "v1.0.0")
	settings := config.Settings{Paths: config.Paths{UpdateCache: filepath.Join(t.TempDir(), "update-check.json")}}
	check, _ := updateIndicatorSource(settings) // update_check and auto_update both off at startup
	if st := check(context.Background()); st.Latest != "v9.9.9" || !st.Newer || requests.Load() != 1 {
		t.Fatalf("check = %+v after %d requests, want the release asked once", st, requests.Load())
	}
}
