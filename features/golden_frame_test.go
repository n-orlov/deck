package features

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// goldenFramePath is the checked-in byte-exact record of SPEC §11.2's golden
// minimum-size frame: side-by-side, sidebar 35 / preview 45, at 80×24 (task
// 031, requirement 42). It is generated, never hand-edited -- see
// TestGoldenMinimumFrame's doc comment for the regeneration command.
const goldenFramePath = "testdata/golden/side_by_side_80x24.golden"

// TestGoldenMinimumFrame asserts the released binary's own rendered frame,
// byte-for-byte, at the exact size and mode SPEC §11.2 names as the golden
// minimum frame: 80×24, auto layout choosing side-by-side (sidebar 35 total
// columns, preview 45 total columns). Every source of nondeterminism this
// repository already knows how to pin is pinned: DECK_ASCII/NO_COLOR/
// DECK_ANIM=0 (harness defaults), DECK_MOUSE=0 (no SGR enable/disable bytes
// in the frame), DECK_CLOCK+DECK_CLOCK_STEP (frozen wall clock), and
// DECK_ID_SEED (reproducible generated ids). The one live session's preview
// content is task 008's deterministic "fitting" fixture, rendered by a real
// fake-claude process (via FAKE_CLAUDE_FIXTURE) through deck's own real
// preview-capture engine -- the preview region is exercised, not excluded.
//
// Regenerate the checked-in golden after a deliberate, reviewed rendering
// change with:
//
//	UPDATE_GOLDEN=1 ci/run.sh go test -run TestGoldenMinimumFrame ./features/...
//
// This test itself runs the whole render-and-compare twice, from two
// independent, freshly created scenario homes, so "running the assertion
// twice from clean state passes both times" is exercised on every run, not
// just claimed.
func TestGoldenMinimumFrame(t *testing.T) {
	for i := 0; i < 2; i++ {
		t.Run(fmt.Sprintf("run-%d", i+1), func(t *testing.T) {
			frame := renderGoldenMinimumFrame(t)
			if os.Getenv("UPDATE_GOLDEN") == "1" {
				if err := os.MkdirAll(filepath.Dir(goldenFramePath), 0o700); err != nil {
					t.Fatalf("create golden directory: %v", err)
				}
				if err := os.WriteFile(goldenFramePath, []byte(frame), 0o600); err != nil {
					t.Fatalf("write golden frame: %v", err)
				}
				t.Logf("wrote %s (%d bytes)", goldenFramePath, len(frame))
				return
			}
			want, err := os.ReadFile(goldenFramePath)
			if err != nil {
				t.Fatalf("read golden frame %s (regenerate with UPDATE_GOLDEN=1): %v", goldenFramePath, err)
			}
			if frame != string(want) {
				t.Fatalf("rendered frame does not match golden %s\n--- got ---\n%s\n--- want ---\n%s", goldenFramePath, frame, string(want))
			}
		})
	}
}

// renderGoldenMinimumFrame drives one real deck client through session
// creation and returns its settled, byte-exact rendered frame.
func renderGoldenMinimumFrame(t *testing.T) string {
	t.Helper()
	binary := buildDeckBinary(t)
	h, err := newScenarioHarness(binary)
	if err != nil {
		t.Fatalf("create scenario harness: %v", err)
	}
	// The default socket name (deck_test_<pid>_<sequence>) is itself visible
	// in the rendered frame (the sidebar's "socket: ..." line), and would
	// make the golden depend on the test process's pid and how many other
	// scenarios ran before it in this binary -- neither of which is stable
	// across regenerations. Pin it.
	h.Socket = "deck_golden_frame_test"
	t.Cleanup(func() {
		if err := h.Close(); err != nil {
			t.Errorf("scenario harness teardown: %v", err)
		}
		_ = os.Remove(h.Binary)
	})

	root, err := repositoryRoot()
	if err != nil {
		t.Fatalf("locate repository root: %v", err)
	}
	previewFixtureDir := filepath.Join(root, "internal", "agent", "testdata", "preview")
	config := fmt.Sprintf("[env]\nFAKE_AGENT_FIXTURE_DIR = %q\nFAKE_CLAUDE_FIXTURE = %q\n", previewFixtureDir, "fitting.txt")
	if err := os.WriteFile(filepath.Join(h.Home, "config.toml"), []byte(config), 0o600); err != nil {
		t.Fatalf("write scenario config.toml: %v", err)
	}

	ctx := context.WithValue(context.Background(), scenarioHarnessKey{}, h)
	if err := installFakeClaudeOnPATH(ctx, false); err != nil {
		t.Fatalf("install fake claude fixture on PATH: %v", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	client, err := h.StartNamedClientWithSize(ctx, "golden", 80, 24,
		"DECK_MOUSE=0", "DECK_CLOCK="+frozenClock, "DECK_CLOCK_STEP=2m", "DECK_ID_SEED=golden-frame-seed")
	if err != nil {
		t.Fatalf("start deck client: %v", err)
	}
	if err := client.WaitForFrame(ctx, true, "No sessions"); err != nil {
		t.Fatalf("deck did not render its empty state: %v", err)
	}

	if err := clientCreatesAgentSessionWithProfile(ctx, "golden", "claude", "golden", "safe"); err != nil {
		t.Fatalf("create golden claude session: %v", err)
	}

	// The row settles at "starting": the fake claude fixture's silent-fixture
	// mode never emits any of internal/agent's probe markers, and it makes no
	// hook calls, so nothing ever samples a different status. That is
	// deliberate and deterministic, not a race to wait out.
	//
	// The pane's real geometry (80x24, tmux's own server default -- deck
	// never resizes it) is taller than the preview panel's content height at
	// 80x24 side-by-side, so requirement 23's bottom-left anchoring keeps the
	// *newest* rows: the fixture's own last line ("row 5 of 5"), not its
	// first, is what a settled capture actually shows. Waiting on the
	// geometry line deck itself prints ("of 80x24") is what proves a live
	// capture landed at all.
	if err := client.WaitForFrame(ctx, true, "of 80x24"); err != nil {
		t.Fatalf("preview never captured the fixture's live pane: %v", err)
	}
	if err := client.WaitForFrame(ctx, true, "row 5 of 5"); err != nil {
		t.Fatalf("preview never showed the fixture's own last line: %v", err)
	}

	// Give one more preview/reconcile tick's worth of headroom, then confirm
	// the frame is still identical -- a torn or still-settling frame must
	// never be mistaken for the golden frame (features/layout_modes.feature's
	// gotcha #3 applies here too).
	settled := client.Frame(true)
	time.Sleep(150 * time.Millisecond)
	if got := client.Frame(true); got != settled {
		t.Fatalf("frame kept changing after the fixture rendered; not settled\nbefore:\n%s\nafter:\n%s", settled, got)
	}

	if err := client.Send("q"); err != nil {
		t.Fatalf("send q: %v", err)
	}
	if err := client.Stop(5 * time.Second); err != nil {
		t.Fatalf("deck client did not exit cleanly: %v", err)
	}

	return settled
}

// TestGoldenMinimumFrameRowCount is a guard against the checked-in golden
// silently drifting to a height other than the one SPEC §11.2 actually
// names: 24 rows. NormalizeFrame strips right-hand padding per row (see
// features/pty_driver_test.go), so column count is not asserted here --
// the golden file's content itself (borders included) is what proves width.
func TestGoldenMinimumFrameRowCount(t *testing.T) {
	data, err := os.ReadFile(goldenFramePath)
	if err != nil {
		t.Fatalf("read golden frame: %v", err)
	}
	rows := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(rows) != 24 {
		t.Fatalf("golden frame has %d rows, want 24", len(rows))
	}
}
