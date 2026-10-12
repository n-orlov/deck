package service

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/n-orlov/deck/internal/agent"
	"github.com/n-orlov/deck/internal/hookrecv"
)

// piControlRig is a Pi row on a service whose deck home is home. open=true
// plays the extension's session_start: it creates the row's control file.
func piControlRig(t *testing.T, home string, open bool) (Service, *sendRecorder, string, string) {
	t.Helper()
	svc, db, _, _ := newAgentTestService(t, nil, "pi-control")
	svc.DeckHome = home
	rec := &sendRecorder{}
	svc.NamePane = rec
	row := adoptRow(t, db, "row-a", "old name", "derived")
	if _, err := db.DB().Exec(`UPDATE sessions SET agent = 'pi' WHERE id = ?`, row.ID); err != nil {
		t.Fatal(err)
	}
	file, _ := agent.PiControlFile(home, row.ConversationID)
	if open {
		if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, agent.EncodePiControlPid(os.Getpid()), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return svc, rec, row.ID, file
}

// TestPiControlFilePushWritesTheNameAndTypesNothing: with the extension's
// control file open the deck push appends the name to it and sends no
// keystroke; the name is held pending for its echo.
func TestPiControlFilePushWritesTheNameAndTypesNothing(t *testing.T) {
	ctx := context.Background()
	svc, rec, id, file := piControlRig(t, t.TempDir(), true)
	_, result, err := svc.RenameAndPush(ctx, id, "pushed  name")
	if err != nil {
		t.Fatal(err)
	}
	written, _ := os.ReadFile(file)
	if want := string(agent.EncodePiControlPid(os.Getpid())) + string(agent.EncodePiControlName("pushed name")); string(written) != want {
		t.Fatalf("control file = %q, want %q", written, want)
	}
	if len(rec.plans) != 0 || !result.Sent {
		t.Fatalf("keystrokes %v, result %#v: the control file must carry the rename alone", rec.plans, result)
	}
	got, _ := svc.Store.GetSession(ctx, id)
	if got.PendingHarnessName != "pushed name" {
		t.Fatalf("pending = %q, want the pushed name", got.PendingHarnessName)
	}
}

// TestPiControlFileUnavailableFallsBackToSendKeys: no control file (the
// extension is not running) means the /name keystroke plan, and nothing is
// created under the deck home by the push.
func TestPiControlFileUnavailableFallsBackToSendKeys(t *testing.T) {
	svc, rec, id, file := piControlRig(t, t.TempDir(), false)
	if _, _, err := svc.RenameAndPush(context.Background(), id, "typed name"); err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"\x15", "/name typed name", "Enter"}}
	if !reflect.DeepEqual(rec.plans, want) {
		t.Fatalf("plans = %q, want %q", rec.plans, want)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("the push created the control file: %v", err)
	}
}

// TestPiControlFileLeftByADeadPiFallsBackToSendKeys (R269): a control file a
// dead Pi left behind is removed and the rename is sent by the keystroke plan,
// never reported as sent through the file.
func TestPiControlFileLeftByADeadPiFallsBackToSendKeys(t *testing.T) {
	svc, rec, id, file := piControlRig(t, t.TempDir(), false)
	dead := exec.Command("true")
	if err := dead.Run(); err != nil {
		t.Fatal(err)
	}
	stale := agent.EncodePiControlPid(dead.ProcessState.Pid())
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, stale, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.RenameAndPush(context.Background(), id, "after death"); err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"\x15", "/name after death", "Enter"}}
	if !reflect.DeepEqual(rec.plans, want) {
		t.Fatalf("plans = %q, want the keystroke fallback %q", rec.plans, want)
	}
	if _, err := os.Lstat(file); !os.IsNotExist(err) {
		t.Fatalf("the stale control file was not removed: %v", err)
	}
}

// TestPiControlFileIsPerProfile: a control file under another profile's deck
// home is never written; this profile falls back to keystrokes.
func TestPiControlFileIsPerProfile(t *testing.T) {
	other := t.TempDir()
	conv := "conv-row-a"
	otherFile, _ := agent.PiControlFile(other, conv)
	if err := os.MkdirAll(filepath.Dir(otherFile), 0o700); err != nil {
		t.Fatal(err)
	}
	ownerLine := agent.EncodePiControlPid(os.Getpid())
	if err := os.WriteFile(otherFile, ownerLine, 0o600); err != nil {
		t.Fatal(err)
	}
	mine := t.TempDir()
	svc, rec, id, file := piControlRig(t, mine, false)
	if _, _, err := svc.RenameAndPush(context.Background(), id, "mine"); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(otherFile); string(data) != string(ownerLine) {
		t.Fatalf("another profile's control file was written: %q", data)
	}
	if len(rec.plans) != 1 || filepath.Dir(file) != filepath.Join(mine, "pi", "control") {
		t.Fatalf("plans %v, file %s", rec.plans, file)
	}
}

// echoHook delivers a Pi session_info_changed payload through the real receiver
// and adoption, as `deck _hook` does.
func echoHook(t *testing.T, svc Service, id, name string) {
	t.Helper()
	row, err := svc.Store.GetSession(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	origin := hookrecv.Origin{SessionID: id, Adopt: func(ctx context.Context, sid string, h hookrecv.AdoptHint) {
		_, _ = svc.AdoptNameWith(ctx, sid, NameHint{SessionTitle: h.SessionTitle})
	}}
	payload := `{"session_id":"` + row.ConversationID + `","hook_event_name":"session_info_changed","name":"` + name + `"}`
	if _, err := hookrecv.ReceiveFrom(context.Background(), svc.Store, []byte(payload), origin, 200); err != nil {
		t.Fatal(err)
	}
}

// TestPiControlFileEchoIsNotAdoptedAsAUserRename (R252): the session_info_changed
// that setSessionName fires for a deck push is recognised as deck's own. A row
// with no push pending adopts the same payload as a user rename, which is what
// the pending name prevents.
func TestPiControlFileEchoIsNotAdoptedAsAUserRename(t *testing.T) {
	ctx := context.Background()
	t.Run("echo of a push", func(t *testing.T) {
		svc, _, id, _ := piControlRig(t, t.TempDir(), true)
		if err := svc.Store.SetPendingHarnessName(ctx, id, "pushed name", svc.Clock.Now().UnixMilli()); err != nil {
			t.Fatal(err)
		}
		echoHook(t, svc, id, "pushed name")
		got, _ := svc.Store.GetSession(ctx, id)
		if got.Name != "old name" || got.NameSource != "derived" || got.PendingHarnessName != "" {
			t.Fatalf("echo adopted or left pending: name=%q source=%q pending=%q", got.Name, got.NameSource, got.PendingHarnessName)
		}
	})
	t.Run("the same payload without a push is a user rename", func(t *testing.T) {
		svc, _, id, _ := piControlRig(t, t.TempDir(), true)
		echoHook(t, svc, id, "pushed name")
		got, _ := svc.Store.GetSession(ctx, id)
		if got.Name != "pushed name" || got.NameSource != "user" {
			t.Fatalf("a Pi-side rename was not adopted: name=%q source=%q", got.Name, got.NameSource)
		}
	})
}
