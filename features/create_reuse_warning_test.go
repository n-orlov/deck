package features

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestCreateModalWarnsBeforeReusingADeletedSessionsName is task 006's own
// required black-box proof (PRD R77 / SPEC §11.4's "the dialog says so
// before it happens"): typing, into the create modal's Name field, a name
// whose only holder is a session already soft-deleted via `dd` shows an
// in-dialog, specific warning -- BEFORE Enter is ever pressed -- stating
// that the name belongs to a deleted session and that reusing it discards
// that session's undo. It drives the real released binary through a PTY
// with keyboard input alone: create one shell session, `dd` it to a
// tombstone (never reaping it -- task 010's bounded sweep is out of scope
// here and is not needed, since SoftDeleteSession sets deleted_at
// immediately, with no grace-window gate on the flag itself), then open a
// fresh create modal and retype the exact same name, asserting the warning
// appears with a plain (non-word-wrapped) substring match.
func TestCreateModalWarnsBeforeReusingADeletedSessionsName(t *testing.T) {
	binary := buildDeckBinary(t)
	home := t.TempDir()
	cwd := filepath.Join(home, "reuse-warning-cwd")
	if err := os.MkdirAll(cwd, 0o700); err != nil {
		t.Fatalf("create working directory: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	driver, err := StartScreenDriver(ctx, binary, []string{
		"DECK_HOME=" + home,
		"DECK_TMUX_SOCKET=deck_create_reuse_warning_test",
		"DECK_ASCII=1", "DECK_ANIM=0", "NO_COLOR=1",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := driver.Stop(3 * time.Second); err != nil && !strings.Contains(err.Error(), "hung deck client") {
			t.Logf("deck exit: %v", err)
		}
	}()

	if err := driver.WaitForFrame(ctx, false, "No sessions"); err != nil {
		t.Fatal(err)
	}

	const name = "reuse-warn-target"

	// Before this session ever existed, opening the create modal and
	// typing this exact name must show no warning at all -- the negative
	// case, proving the positive assertion below is actually about the
	// tombstone and not a warning shown unconditionally.
	if err := driver.Send("n"); err != nil {
		t.Fatal(err)
	}
	if err := driver.WaitForFrame(ctx, false, "Create shell session"); err != nil {
		t.Fatal(err)
	}
	if err := driver.Send(name); err != nil {
		t.Fatal(err)
	}
	time.Sleep(75 * time.Millisecond)
	if strings.Contains(driver.Frame(false), "belongs to a deleted session") {
		t.Fatalf("create modal warned about a name with no tombstoned holder at all:\n%s", driver.Frame(false))
	}

	// Submit it as a real shell session.
	if err := driver.Send("\t" + cwd + "\r"); err != nil {
		t.Fatal(err)
	}
	if err := driver.WaitForFrame(ctx, false, "starting"); err != nil {
		t.Fatal(err)
	}

	// `dd`: the pending indicator, then the confirm dialog, then Enter to
	// tombstone it (never reaping it -- see the doc comment above).
	if err := driver.Send("d"); err != nil {
		t.Fatal(err)
	}
	if err := driver.WaitForFrame(ctx, false, "press d again to confirm"); err != nil {
		t.Fatal(err)
	}
	if err := driver.Send("d"); err != nil {
		t.Fatal(err)
	}
	if err := driver.WaitForFrame(ctx, false, "Enter deletes"); err != nil {
		t.Fatal(err)
	}
	if err := driver.Send("\r"); err != nil {
		t.Fatal(err)
	}
	if err := driver.WaitForFrame(ctx, false, "No sessions"); err != nil {
		t.Fatal(err)
	}

	// Re-open the create modal and retype the exact same name: the Name
	// field's own value is what is checked, so nothing else needs typing.
	if err := driver.Send("n"); err != nil {
		t.Fatal(err)
	}
	if err := driver.WaitForFrame(ctx, false, "Create shell session"); err != nil {
		t.Fatal(err)
	}
	if err := driver.Send(name); err != nil {
		t.Fatal(err)
	}

	want := "this name belongs to a deleted session; reuse discards its undo"
	if err := driver.WaitForFrame(ctx, false, want); err != nil {
		t.Fatalf("create modal did not warn, before submit, that %q belongs to a deleted session: %v", name, err)
	}
	frame := driver.Frame(false)
	if !strings.Contains(frame, want) {
		t.Fatalf("create modal frame missing the plain reuse-warning substring %q:\n%s", want, frame)
	}

	// esc abandons the modal without submitting, so the tombstoned row's
	// undo (irrelevant to this test either way, since it was never
	// pressed) is never actually exercised by this scenario.
	if err := driver.Send("\x1b"); err != nil {
		t.Fatal(err)
	}
	if err := driver.WaitForFrame(ctx, false, "No sessions"); err != nil {
		t.Fatal(err)
	}
	if err := driver.Send("q"); err != nil {
		t.Fatal(err)
	}
	if err := driver.Stop(3 * time.Second); err != nil {
		t.Fatalf("deck did not quit cleanly: %v", err)
	}
}
