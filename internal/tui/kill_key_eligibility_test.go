package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// TestKillKeyHandlerConsultsCanKillForTheSingleSelectedRow is task 807's
// regression for review finding 2: the single-row `x` handler must consult
// the same canKill the footer's x slot already used, dispatching NO kill
// command at all for an already-stopped row instead of deferring the
// refusal to the service's own verdict (m.kill). This is the direct proof
// that removing the canKill consultation breaks the contract: with the
// consultation in place, a stopped row's cmd() never calls m.kill and still
// carries the same "already stopped" wording a live row's genuine service
// refusal would; a live row's cmd() calls m.kill exactly once.
func TestKillKeyHandlerConsultsCanKillForTheSingleSelectedRow(t *testing.T) {
	cases := []struct {
		name         string
		status       string
		wantDispatch bool
	}{
		{name: "stopped row dispatches no kill command", status: "stopped", wantDispatch: false},
		{name: "live row dispatches a kill command", status: "running", wantDispatch: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			model := NewWithShellCreator(nil, config.Settings{}, "", nil)
			model.kill = func(_ context.Context, s store.Session) error {
				called = true
				return nil
			}
			model.sessions = []store.Session{{ID: "s1", Name: "alpha", Status: tc.status}}
			model.selected = 0

			got, cmd := model.Update(key("x"))
			model = got.(Model)
			if cmd == nil {
				t.Fatal("x returned no command")
			}

			msg := cmd()

			if called != tc.wantDispatch {
				t.Fatalf("kill dispatched = %v, want %v", called, tc.wantDispatch)
			}

			killedMsg, ok := msg.(sessionKilled)
			if !ok {
				t.Fatalf("cmd() returned %T, want sessionKilled", msg)
			}
			if !tc.wantDispatch {
				if killedMsg.err == nil || !strings.Contains(killedMsg.err.Error(), "already stopped") {
					t.Fatalf("stopped row refusal = %v, want an error containing %q", killedMsg.err, "already stopped")
				}
			} else if killedMsg.err != nil {
				t.Fatalf("live row cmd() returned err %v, want nil (the fake kill returns nil)", killedMsg.err)
			}
		})
	}
}
