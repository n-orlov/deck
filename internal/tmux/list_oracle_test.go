// list_oracle_test.go pins what Client.List returns for one fixture server
// BEFORE R184 folds its 1+N tmux processes into one `list-panes -a` call: the
// expected value below is written out literally, never recomputed by List or
// by any helper that shares its parsing, so a rewrite of List has to
// reproduce today's result shape exactly. The fixture holds a live deck pane,
// a deck pane dead with an exit status, a deck pane dead by signal and a
// session whose name is not deck-owned.
package tmux

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestListOracleFixtureServerResultIsPinned(t *testing.T) {
	socket := fmt.Sprintf("priv-listoracle-%d-%d", os.Getpid(), time.Now().UnixNano())
	t.Cleanup(func() { _ = exec.Command("tmux", "-L", socket, "kill-server").Run() })
	client := Client{Socket: socket, Timeout: 10 * time.Second}
	ctx := context.Background()

	// A fixed cwd, so the fixture does not depend on a temp dir.
	const cwd = "/tmp"
	// Session names sort the same by name and by creation order, so the
	// expectation does not depend on how this tmux orders list-sessions.
	launches := []Launch{
		{Slug: "a-live", CWD: cwd, Command: []string{"/bin/sh", "-c", "exec sleep 300"}},
		{Slug: "b-exit", CWD: cwd, Command: []string{"/bin/sh", "-c", "exit 7"}},
	}
	for _, launch := range launches {
		if _, err := client.Create(ctx, launch); err != nil {
			t.Fatalf("create %s: %v", launch.Slug, err)
		}
	}
	// A session deck does not own, created on the same server between the deck ones.
	runTmux(t, socket, "new-session", "-d", "-s", "other_session", "-c", cwd, "/bin/sh", "-c", "exec sleep 300")
	if _, err := client.Create(ctx, Launch{Slug: "c-signal", CWD: cwd, Command: []string{"/bin/sh", "-c", "kill -KILL $$"}}); err != nil {
		t.Fatalf("create c-signal: %v", err)
	}

	// tmux marks a pane dead a moment after its process ends.
	deadline := time.Now().Add(10 * time.Second)
	for {
		dead := strings.Fields(runTmux(t, socket, "list-panes", "-a", "-F", "#{pane_dead}"))
		if len(dead) == 4 && strings.Count(strings.Join(dead, ""), "1") == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the two crashing panes never both went dead: %v", dead)
		}
		time.Sleep(10 * time.Millisecond)
	}

	// pane_pid is the one fact the test cannot write literally; read it from
	// tmux directly, by pane id, rather than from List.
	pid := func(paneID string) int {
		value, err := strconv.Atoi(strings.TrimSpace(runTmux(t, socket, "display-message", "-p", "-t", paneID, "#{pane_pid}")))
		if err != nil {
			t.Fatalf("pane_pid of %s: %v", paneID, err)
		}
		return pid0(t, value)
	}
	// A signal death is 128 + signal (KILL = 9).
	status7, status137 := 7, 137
	want := []Session{
		{Name: "deck_a-live", Panes: []Pane{{ID: "%0", PID: pid("%0"), Dead: false, DeadStatus: nil, Width: 80, Height: 24}}},
		{Name: "deck_b-exit", Panes: []Pane{{ID: "%1", PID: pid("%1"), Dead: true, DeadStatus: &status7, Width: 80, Height: 24}}},
		{Name: "deck_c-signal", Panes: []Pane{{ID: "%3", PID: pid("%3"), Dead: true, DeadStatus: &status137, Width: 80, Height: 24}}},
	}

	got, err := client.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("List result changed.\n got: %s\nwant: %s", describeSessions(got), describeSessions(want))
	}
}

func pid0(t *testing.T, pid int) int {
	t.Helper()
	if pid <= 0 {
		t.Fatalf("pane_pid %d is not a real pid", pid)
	}
	return pid
}

func describeSessions(sessions []Session) string {
	var b strings.Builder
	for _, session := range sessions {
		fmt.Fprintf(&b, "{%s", session.Name)
		for _, pane := range session.Panes {
			status := "nil"
			if pane.DeadStatus != nil {
				status = strconv.Itoa(*pane.DeadStatus)
			}
			fmt.Fprintf(&b, " [%+v status=%s]", pane, status)
		}
		b.WriteString("} ")
	}
	return b.String()
}
