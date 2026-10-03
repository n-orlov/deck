// list_forged_pane_test.go pins that a pane's working directory cannot forge
// pane facts for another deck session. List reads every pane in ONE
// `list-panes -a` and splits it on newline and `|`; tmux prints free-text
// fields such as pane_current_path raw, so a directory name carrying a
// newline and a fake line once added a dead (status 137) pane to a victim
// session, and reconcile then marked the victim crashed and killed it.
package tmux

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestListIgnoresPaneFactsForgedByADirectoryName(t *testing.T) {
	socket := fmt.Sprintf("priv-listforge-%d-%d", os.Getpid(), time.Now().UnixNano())
	t.Cleanup(func() { _ = exec.Command("tmux", "-L", socket, "kill-server").Run() })
	client := Client{Socket: socket, Timeout: 10 * time.Second}
	ctx := context.Background()

	// Closes the attacker's own line, then writes a whole dead pane line for
	// deck_victim, then opens a non-deck line that swallows the real tail.
	payload := "x|1|0|||bash|80|24\ndeck_victim|%9|x|1|1|137||bash|80|24\nz"
	forged := filepath.Join(t.TempDir(), payload)
	if err := os.Mkdir(forged, 0o755); err != nil {
		t.Fatalf("mkdir forged directory: %v", err)
	}
	if _, err := client.Create(ctx, Launch{Slug: "victim", CWD: t.TempDir(), Command: []string{"/bin/sh", "-c", "exec sleep 300"}}); err != nil {
		t.Fatalf("create victim: %v", err)
	}
	// Started by tmux directly, as a deck pane that later cd's into the
	// directory would be: List is what is under test, not Create.
	runTmux(t, socket, "new-session", "-d", "-s", "deck_attacker", "-c", forged, "/bin/sh", "-c", "exec sleep 300")
	// Wait until tmux itself reports the forged directory as the attacker's
	// current path, so the payload is really in a list-panes -a read. tmux
	// 3.6b prints it raw; 3.5a prints the newlines as _ but the | raw, which
	// broke List for every session rather than forging a pane.
	deadline := time.Now().Add(10 * time.Second)
	for !strings.HasPrefix(runTmux(t, socket, "display-message", "-p", "-t", "deck_attacker", "#{pane_current_path}"), filepath.Dir(forged)+"/x|1|0|") {
		if time.Now().After(deadline) {
			t.Fatalf("tmux never reported the forged directory as the attacker's pane_current_path")
		}
		time.Sleep(10 * time.Millisecond)
	}

	sessions, err := client.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	panes := map[string][]Pane{}
	for _, session := range sessions {
		panes[session.Name] = session.Panes
	}
	if len(sessions) != 2 || len(panes["deck_victim"]) != 1 || len(panes["deck_attacker"]) != 1 {
		t.Fatalf("List = %s, want deck_victim and deck_attacker with one pane each", describeSessions(sessions))
	}
	for name, list := range panes {
		for _, pane := range list {
			if pane.Dead || pane.DeadStatus != nil {
				t.Fatalf("%s reports a dead pane %+v; a directory name forged it: %s", name, pane, describeSessions(sessions))
			}
		}
	}
}

func TestParsePaneFactsRejectsMalformedPaneID(t *testing.T) {
	for _, line := range []string{
		"x|1|0|||80|24",
		"%|1|0|||80|24",
		"%1x|1|0|||80|24",
		"%1|1|0|||80",
		"%1|1|0||||80|24",
	} {
		if pane, err := parsePaneFacts("deck_a", line); err == nil {
			t.Errorf("parsePaneFacts(%q) = %+v, want an error", line, pane)
		}
	}
	pane, err := parsePaneFacts("deck_a", "%12|4242|1||9|80|24")
	if err != nil {
		t.Fatalf("parsePaneFacts of a well-formed line: %v", err)
	}
	if pane.ID != "%12" || pane.PID != 4242 || !pane.Dead || pane.DeadStatus == nil || *pane.DeadStatus != 137 || pane.Width != 80 || pane.Height != 24 {
		t.Fatalf("parsePaneFacts = %+v, want %%12 pid 4242 dead by signal 9 (137) at 80x24", pane)
	}
}
