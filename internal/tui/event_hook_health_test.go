package tui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/notify"
	"github.com/n-orlov/deck/internal/store"
)

// hookSurfaceModel is a model with one selected session, an event_hook
// configured, and the given hook facts loaded as the reload and the `i` open
// would load them.
func hookSurfaceModel(script string, health hookHealth, detail *store.EventHookRun) Model {
	m := New(nil, config.Settings{EventHook: script}, "")
	m.sessions = []store.Session{{ID: "s1", Name: "worker", Agent: "claude", Status: "idle"}}
	m.selected = rowCursor(0)
	m.hookHealth = health
	if detail != nil {
		m.detailHookSessionID, m.detailHookFound, m.detailHookRun = "s1", true, *detail
	}
	return m
}

func hookRun(res store.EventHookResult) store.EventHookRun {
	return store.EventHookRun{EventHookResult: res, SessionID: "s1"}
}

// TestEventHookSurfaceRendersEachState covers SPEC §10.3/§11.4: the last hook
// result is shown in the session detail and the health lines, and a non-zero
// exit, a timeout and a script that is missing each carry their own visible
// marker. Every state asserts its own marker AND that the other states'
// markers are absent, so removing a marker fails exactly its state.
func TestEventHookSurfaceRendersEachState(t *testing.T) {
	const script = "/opt/hooks/notify.sh"
	cases := []struct {
		name       string
		health     hookHealth
		detail     store.EventHookRun
		want       []string // in both surfaces unless detailOnly / healthOnly
		healthOnly []string
		forbid     []string
	}{
		{
			name:   "exit 0",
			health: hookHealth{run: hookRun(store.EventHookResult{Kind: "idle", Output: "sent ok"}), found: true},
			detail: hookRun(store.EventHookResult{Kind: "idle", Output: "sent ok"}),
			want:   []string{"ok · exit status 0 · for idle", "sent ok"},
			forbid: []string{"FAILED", "TIMED OUT", "does not exist", "not executable"},
		},
		{
			name:   "non-zero exit",
			health: hookHealth{run: hookRun(store.EventHookResult{Kind: "error", ExitCode: 3, Output: "first\ncurl: (7) refused"}), found: true},
			detail: hookRun(store.EventHookResult{Kind: "error", ExitCode: 3, Output: "first\ncurl: (7) refused"}),
			want:   []string{"FAILED", "exit status 3", "for error", "curl: (7) refused"},
			forbid: []string{"TIMED OUT", "does not exist"},
		},
		{
			name:   "timeout",
			health: hookHealth{run: hookRun(store.EventHookResult{Kind: "waiting", ExitCode: -1, TimedOut: true, Output: "still sleeping"}), found: true},
			detail: hookRun(store.EventHookResult{Kind: "waiting", ExitCode: -1, TimedOut: true, Output: "still sleeping"}),
			want:   []string{"TIMED OUT", "event_hook_timeout", "still sleeping"},
			forbid: []string{"exit status", "does not exist"},
		},
		{
			name: "script missing",
			health: hookHealth{probe: notify.ErrScriptMissing, found: true,
				run: hookRun(store.EventHookResult{Kind: "idle", ExitCode: -1, Error: "event hook: fork/exec " + script + ": no such file or directory"})},
			detail:     hookRun(store.EventHookResult{Kind: "idle", ExitCode: -1, Error: "event hook: fork/exec " + script + ": no such file or directory"}),
			want:       []string{"FAILED", "could not start", "no such file or directory"},
			healthOnly: []string{"Event hook FAILED: " + script + ": script does not exist"},
			forbid:     []string{"TIMED OUT", "exit status"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			detail := hookSurfaceModel(script, c.health, &c.detail)
			detail.detail = true
			health := hookSurfaceModel(script, c.health, nil)
			for surface, view := range map[string]string{"detail": detail.View(), "health": health.View()} {
				wants := c.want
				if surface == "health" {
					wants = append(append([]string{}, wants...), c.healthOnly...)
				}
				for _, w := range wants {
					if !strings.Contains(view, w) {
						t.Errorf("%s view lacks %q:\n%s", surface, w, view)
					}
				}
				for _, f := range c.forbid {
					if strings.Contains(view, f) {
						t.Errorf("%s view must not contain %q:\n%s", surface, f, view)
					}
				}
			}
		})
	}
}

func TestEventHookHealthNamesANonExecutableScriptAndStaysQuietWhenInert(t *testing.T) {
	m := hookSurfaceModel("/opt/hooks/notify.sh --quiet", hookHealth{probe: notify.ErrScriptNotExecutable}, nil)
	if view := m.View(); !strings.Contains(view, "/opt/hooks/notify.sh: script is not executable") {
		t.Fatalf("health lines must name the non-executable script:\n%s", view)
	}
	inert := hookSurfaceModel("", hookHealth{probe: notify.ErrScriptMissing, found: true, run: hookRun(store.EventHookResult{ExitCode: 9})}, nil)
	if lines := inert.eventHookHealthLines(80); len(lines) != 0 {
		t.Fatalf("no event_hook is configured, yet the health lines say %q", lines)
	}
	if view := inert.View(); strings.Contains(view, "Event hook") {
		t.Fatalf("an inert hook must not show on the main view:\n%s", view)
	}
}

func TestEventHookDetailIgnoresAnotherSessionsResultAndAnUnrunHook(t *testing.T) {
	m := hookSurfaceModel("/opt/hooks/notify.sh", hookHealth{}, nil)
	m.detail = true
	if strings.Contains(m.View(), "Last event hook") {
		t.Fatalf("a session no hook ran for must show no hook field:\n%s", m.View())
	}
	m.detailHookSessionID, m.detailHookFound = "other", true
	if strings.Contains(m.View(), "Last event hook") {
		t.Fatalf("a result loaded for another session must not render:\n%s", m.View())
	}
}

// TestEventHookFactsLoadFromTheStore drives the real loaders end to end: a
// result recorded against an event is what `i` and the reload read back, and a
// script that is not on disk is probed missing on the reload.
func TestEventHookFactsLoadFromTheStore(t *testing.T) {
	home := t.TempDir()
	db, err := store.OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	if _, err := db.CreateSession(ctx, store.CreateSessionInput{ID: "s1", Name: "worker", Agent: "shell", CWD: home, CapturedPath: "/usr/bin", Status: "idle", StatusAt: 1, CreatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	var seq int64
	if err := db.UpdateSessionStatus(ctx, store.StatusUpdateInput{SessionID: "s1", Status: "idle", Reason: "r",
		Source: "hook", At: 5, EventKind: "stop", EventSeq: &seq}); err != nil {
		t.Fatal(err)
	}
	if err := db.RecordEventHookResult(ctx, seq, store.EventHookResult{Kind: "idle", ExitCode: 4, Output: "bad"}); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(home, "gone.sh")
	m := New(db, config.Settings{EventHook: missing}, "")

	loaded, ok := m.loadDetailDroppedHook("s1")().(detailDroppedHookLoaded)
	if !ok || !loaded.hookFound || loaded.err != nil || loaded.hookRun.ExitCode != 4 || loaded.hookRun.Output != "bad" {
		t.Fatalf("detail load = %+v", loaded)
	}
	m.detailDroppedHookSessionID = "s1"
	next, _ := m.onDetailDroppedHookLoaded(loaded)
	if got := next.(Model); !got.detailHookFound || got.detailHookRun.ExitCode != 4 {
		t.Fatalf("detail state after load: %+v", got.detailHookRun)
	}
	health := m.loadHookHealth()
	if !health.found || health.run.ExitCode != 4 || !errors.Is(health.probe, notify.ErrScriptMissing) {
		t.Fatalf("health = %+v", health)
	}
	if err := os.WriteFile(missing, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := m.loadHookHealth(); got.probe != nil {
		t.Fatalf("an executable script must probe clean: %v", got.probe)
	}
	if got := New(db, config.Settings{}, "").loadHookHealth(); got.found || got.probe != nil {
		t.Fatalf("an inert hook loads no facts: %+v", got)
	}
}
