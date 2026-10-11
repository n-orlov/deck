package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/agent"
	"github.com/n-orlov/deck/internal/store"
)

// codexPane is a recorder for the pane: the screen it shows and an ordered log
// of every send and every pause, so the gap between the text and Enter is
// asserted from the order and the injected pause, never from wall time.
type codexPane struct {
	t      *testing.T
	screen string
	log    []string
	err    error
}

func (p *codexPane) Screen(context.Context, string) (string, error) { return p.screen, nil }

func (p *codexPane) SendPlan(_ context.Context, _ string, plan []string) error {
	p.log = append(p.log, "send:"+strings.Join(plan, "|"))
	return p.err
}

func (p *codexPane) pause(_ context.Context, d time.Duration) error {
	p.log = append(p.log, "pause:"+d.String())
	return nil
}

func codexFixture(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "agent", "testdata", "probes", "codex", name+".txt"))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func newCodexRig(t *testing.T, screen string) (Service, *store.Store, *codexPane) {
	t.Helper()
	svc, db, _, _ := newAgentTestService(t, nil, "codex-rename")
	pane := &codexPane{t: t, screen: screen}
	svc.NamePane, svc.Pause = pane, pane.pause
	return svc, db, pane
}

func codexRow(t *testing.T, db *store.Store, id, status, nameSource string) store.Session {
	t.Helper()
	session, err := db.CreateSession(context.Background(), store.CreateSessionInput{
		ID: id, Name: "fix login " + id, CWD: t.TempDir(), Agent: "codex", CapturedPath: "/bin",
		Status: status, StatusSource: "probe", StatusAt: 1, CreatedAt: 1, NameSource: nameSource,
	})
	if err != nil {
		t.Fatal(err)
	}
	return session
}

func eventReasons(t *testing.T, db *store.Store, id, kind string) []string {
	t.Helper()
	rows, err := db.DB().Query(`SELECT reason FROM events WHERE session_id = ? AND kind = ? ORDER BY seq`, id, kind)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var reason string
		if err := rows.Scan(&reason); err != nil {
			t.Fatal(err)
		}
		out = append(out, reason)
	}
	return out
}

func codexWant(id string) []string {
	return []string{"send:/rename fix login " + id, "pause:700ms", "send:Enter"}
}

// TestCodexRenameLaunchSendsAfterReady: a fresh user row is typed into only
// once the probe reports the composer ready; the text, the injected 700 ms
// pause and Enter go out in that order.
func TestCodexRenameLaunchSendsAfterReady(t *testing.T) {
	svc, db, pane := newCodexRig(t, codexFixture(t, "starting"))
	session := codexRow(t, db, "c1", "starting", "user")
	svc.applyLaunchName(context.Background(), session)
	if !reflect.DeepEqual(pane.log, codexWant("c1")) {
		t.Fatalf("log = %v, want %v", pane.log, codexWant("c1"))
	}
	if got := eventReasons(t, db, "c1", EventHarnessNameSent); !reflect.DeepEqual(got, []string{"fix login c1"}) {
		t.Errorf("marker = %v", got)
	}
	svc.applyLaunchName(context.Background(), session)
	if len(pane.log) != len(codexWant("c1")) {
		t.Errorf("the name was typed twice: %v", pane.log)
	}
}

// TestCodexRenameLaunchWaitsForReady: while the screen is not ready nothing is
// sent and nothing is recorded, so a later tick still applies the name. There
// is no startup sleep: readiness is the screen on this tick.
func TestCodexRenameLaunchWaitsForReady(t *testing.T) {
	svc, db, pane := newCodexRig(t, "")
	session := codexRow(t, db, "c2", "starting", "user")
	svc.applyLaunchName(context.Background(), session)
	if len(pane.log) != 0 || len(eventReasons(t, db, "c2", EventHarnessNameSent)) != 0 {
		t.Fatalf("typed into a blank screen: %v", pane.log)
	}
	pane.screen = codexFixture(t, "starting")
	svc.applyLaunchName(context.Background(), session)
	if !reflect.DeepEqual(pane.log, codexWant("c2")) {
		t.Errorf("log = %v, want %v", pane.log, codexWant("c2"))
	}
}

// TestCodexRenameNoSendWhileModalUp: an update or trust modal swallows the
// keys, so nothing is sent on the launch path or the push path.
func TestCodexRenameNoSendWhileModalUp(t *testing.T) {
	update := "✨ Update available! 0.154.0 -> 0.156.1\n 1. Update now\n 3. Skip until next version\n" + codexFixture(t, "starting")
	trust := "Do you trust the contents of this directory?\n 1. Yes, continue\n 2. No, quit\n"
	for name, screen := range map[string]string{"update": update, "trust": trust} {
		t.Run(name, func(t *testing.T) {
			svc, db, pane := newCodexRig(t, screen)
			session := codexRow(t, db, "m-"+name, "starting", "user")
			svc.applyLaunchName(context.Background(), session)
			result := svc.pushRename(context.Background(), session)
			if len(pane.log) != 0 || result.Sent {
				t.Fatalf("sent into a modal: log %v, result %+v", pane.log, result)
			}
			if got := eventReasons(t, db, session.ID, EventHarnessRenameRefused); len(got) != 1 || !strings.Contains(got[0], "dialog") {
				t.Errorf("refusals = %v", got)
			}
		})
	}
}

// TestCodexRenameFailureKeepsDecksName: a send that fails records
// harness_rename_failed once, is not retried, and the row keeps its name.
func TestCodexRenameFailureKeepsDecksName(t *testing.T) {
	svc, db, pane := newCodexRig(t, codexFixture(t, "starting"))
	pane.err = errors.New("tmux gone")
	session := codexRow(t, db, "c3", "starting", "user")
	svc.applyLaunchName(context.Background(), session)
	svc.applyLaunchName(context.Background(), session)
	if got := eventReasons(t, db, "c3", EventHarnessRenameFailed); len(got) != 1 || !strings.Contains(got[0], "tmux gone") {
		t.Errorf("failed events = %v", got)
	}
	if len(pane.log) != 1 {
		t.Errorf("retried after a failure: %v", pane.log)
	}
	if got, _ := db.GetSession(context.Background(), "c3"); got.Name != "fix login c3" || got.NameSource != "user" {
		t.Errorf("row = %q/%q", got.Name, got.NameSource)
	}
}

// TestCodexRenameLaunchSkipsRowsItDoesNotOwn: a derived row, a row past its
// first prompt and a running turn are never typed into by the launch path.
func TestCodexRenameLaunchSkipsRowsItDoesNotOwn(t *testing.T) {
	svc, db, pane := newCodexRig(t, codexFixture(t, "starting"))
	for id, row := range map[string]store.Session{
		"d": codexRow(t, db, "d", "starting", "derived"),
		"i": codexRow(t, db, "i", "idle", "user"),
	} {
		svc.applyLaunchName(context.Background(), row)
		if len(pane.log) != 0 {
			t.Fatalf("%s: sent %v", id, pane.log)
		}
	}
	running := codexRow(t, db, "r", "running", "user")
	pane.screen = codexFixture(t, "running")
	if result := svc.pushRename(context.Background(), running); result.Sent || len(pane.log) != 0 {
		t.Errorf("pushed into a working turn: %+v %v", result, pane.log)
	}
}

// TestCodexRenamePushWhenIdle: a rename of a live user row pushes the same
// paced plan.
func TestCodexRenamePushWhenIdle(t *testing.T) {
	svc, db, pane := newCodexRig(t, codexFixture(t, "idle"))
	session := codexRow(t, db, "p1", "idle", "user")
	if result := svc.pushRename(context.Background(), session); !result.Sent {
		t.Fatalf("not sent: %+v", result)
	}
	if !reflect.DeepEqual(pane.log, codexWant("p1")) {
		t.Errorf("log = %v, want %v", pane.log, codexWant("p1"))
	}
	if _, ok := svc.Agents.Lookup("codex"); !ok || !agent.IsPromptGate(agent.NewCodex()) {
		t.Fatal("codex is not a prompt gate")
	}
}

// TestCodexRenameLaunchTickInsideTheGapDoesNotRetype: the reconcile tick keeps
// running while the 700 ms gap between the text and Enter is held, and a tick
// inside the gap must not type the name into the same input line again (the
// pane would receive "/rename x/rename x" and one Enter).
func TestCodexRenameLaunchTickInsideTheGapDoesNotRetype(t *testing.T) {
	svc, db, pane := newCodexRig(t, codexFixture(t, "starting"))
	session := codexRow(t, db, "c5", "starting", "user")
	svc.Pause = func(ctx context.Context, d time.Duration) error {
		svc.applyLaunchName(ctx, session) // the next tick, mid-gap
		return pane.pause(ctx, d)
	}
	svc.applyLaunchName(context.Background(), session)
	if !reflect.DeepEqual(pane.log, codexWant("c5")) {
		t.Fatalf("log = %v, want the name typed once: %v", pane.log, codexWant("c5"))
	}
}
