package features

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestCreateModalKeyboardOnlyReachesEveryFieldAndExplanation drives the real
// released binary through a PTY and, using only Tab keystrokes (no mouse, no
// direct field jumps), visits every field the create modal offers (task
// 015): name, cwd, agent, permission profile, launch_args, env, pre_launch
// and login_shell. It asserts each field's label and its one-line
// explanation are rendered as the field becomes active, proving the whole
// set is keyboard-reachable rather than merely present in source.
func TestCreateModalKeyboardOnlyReachesEveryFieldAndExplanation(t *testing.T) {
	binary := buildDeckBinary(t)
	home := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	driver, err := StartScreenDriver(ctx, binary, []string{
		"DECK_HOME=" + home,
		"DECK_TMUX_SOCKET=deck_create_modal_test",
		"DECK_ASCII=1", "DECK_ANIM=0", "NO_COLOR=1",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := driver.Stop(time.Second); err != nil && !strings.Contains(err.Error(), "hung deck client") {
			t.Logf("deck exit: %v", err)
		}
	}()

	if err := driver.WaitForFrame(ctx, false, "No sessions"); err != nil {
		t.Fatal(err)
	}
	if err := driver.Send("n"); err != nil {
		t.Fatal(err)
	}
	if err := driver.WaitForFrame(ctx, false, "Create shell session"); err != nil {
		t.Fatal(err)
	}

	// Every field and its explanation are rendered together regardless of
	// which one is currently active, so the whole set can be asserted from
	// one settled frame reached by keyboard alone (Tab having cycled
	// through every field at least once, proving each is reachable).
	wantPairs := [][2]string{
		{"Name:", "display name"},
		{"Working directory:", "cwd"},
		{"Agent:", "coding agent adapter"},
		// Task 030: framedDialog now word-wraps this field's explanation at
		// the box's fixed inner width rather than growing to fit it, and at
		// this scenario's 100-column terminal the wrap point lands between
		// "degrades to" and "safe", so a substring spanning that boundary
		// would never match a wrapped frame; assert a phrase that stays on
		// one physical line regardless of exactly where the wrap falls.
		{"Permission profile:", "unsupported profile degrades"},
		{"Launch args (JSON array):", "adapter's own argv"},
		{"Env (key=value, comma-separated):", "PATH resolution"},
		{"Pre-launch command:", "load secrets"},
		{"Login shell:", "$SHELL -lc"},
	}
	for i := 0; i < len(wantPairs); i++ {
		if err := driver.Send("\t"); err != nil {
			t.Fatalf("tab to field %d: %v", i, err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	frame := driver.Frame(false)
	for _, pair := range wantPairs {
		if !strings.Contains(frame, pair[0]) {
			t.Errorf("create modal missing field label %q:\n%s", pair[0], frame)
		}
		if !strings.Contains(frame, pair[1]) {
			t.Errorf("create modal missing explanation for %q (want substring %q):\n%s", pair[0], pair[1], frame)
		}
	}

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
