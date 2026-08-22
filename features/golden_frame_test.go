package features

import (
	"context"
	"crypto/sha256"
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

// goldenFrameTheme and goldenFrameColorDepth are the pinned theme name and
// DECK_COLOR_DEPTH this golden is recorded under (task 033, requirement 52:
// "a golden frame that does not pin both is a frame that will move when the
// default theme does"). empire is theme.DefaultName; truecolor is pinned
// (rather than the 16-colour floor) so renderGoldenMinimumFrame's own
// per-cell colour proof can compare a live cell's rendered hex directly
// against the theme's authored hex with no quantisation step to account for.
const (
	goldenFrameTheme      = "empire"
	goldenFrameColorDepth = "truecolor"
)

// TestGoldenMinimumFrame asserts the released binary's own rendered frame,
// byte-for-byte, at the exact size and mode SPEC §11.2 names as the golden
// minimum frame: 80×24, auto layout choosing side-by-side (sidebar 35 total
// columns, preview 45 total columns). Every source of nondeterminism this
// repository already knows how to pin is pinned: DECK_ASCII/DECK_ANIM=0
// (harness defaults), goldenFrameTheme+goldenFrameColorDepth (requirement
// 52 -- colour is ON, not NO_COLOR, so a pinned DECK_COLOR_DEPTH actually
// has something to quantise), DECK_MOUSE=0 (no SGR enable/disable bytes in
// the frame), DECK_CLOCK+DECK_CLOCK_STEP (frozen wall clock), and
// DECK_ID_SEED (reproducible generated ids). The one live session's preview
// content is task 008's deterministic "fitting" fixture, rendered by a real
// fake-claude process (via FAKE_CLAUDE_FIXTURE) through deck's own real
// preview-capture engine -- the preview region is exercised, not excluded.
//
// The checked-in golden file itself stays plain grid text with no colour
// markers: it is read via ScreenDriver.Frame, which -- like every other
// screen-contains-"..." assertion in this suite (PRD requirement 53's
// table) -- extracts the emulator's own grid CHARACTERS, never its byte
// stream, so colour landing on cells that already existed changes no
// character and this file's bytes are untouched by lifting NO_COLOR. What
// actually proves goldenFrameTheme/goldenFrameColorDepth are exercised,
// rather than merely named in an env var nothing reads, is
// assertGoldenFrameIsThemed below: a direct CellAt read (the same
// mechanism every per-cell colour assertion in this suite uses) compared
// against internal/theme's own resolution of the pinned theme -- "no
// coloured region excluded" (task 033).
//
// Regenerate the checked-in golden after a deliberate, reviewed rendering
// change with:
//
//	UPDATE_GOLDEN=1 ci/run.sh go test -run TestGoldenMinimumFrame ./features/...
//
// This test itself runs the whole render-and-compare twice, from two
// independent, freshly created scenario homes, so "running the assertion
// twice from clean state passes both times" is exercised on every run, not
// just claimed. Regeneration reproducing identical bytes twice (requirement
// 52) is the same property observed from the other side: run-1 and run-2
// below log a sha256 of their own rendered frame either way, so a real
// UPDATE_GOLDEN=1 invocation's test log names both runs' checksums.
func TestGoldenMinimumFrame(t *testing.T) {
	for i := 0; i < 2; i++ {
		t.Run(fmt.Sprintf("run-%d", i+1), func(t *testing.T) {
			frame := renderGoldenMinimumFrame(t)
			t.Logf("rendered frame sha256 %x (%d bytes, theme %q, DECK_COLOR_DEPTH %q)", sha256.Sum256([]byte(frame)), len(frame), goldenFrameTheme, goldenFrameColorDepth)
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
	// [ui] theme pins requirement 52's "state which theme the golden frame
	// is pinned to" -- empire is deck's own DefaultName (internal/theme/
	// registry.go), named explicitly rather than left to fall back, so this
	// golden does not silently move the day the default theme does.
	config := fmt.Sprintf("[ui]\ntheme = %q\n\n[env]\nFAKE_AGENT_FIXTURE_DIR = %q\nFAKE_CLAUDE_FIXTURE = %q\n", goldenFrameTheme, previewFixtureDir, "fitting.txt")
	if err := os.WriteFile(filepath.Join(h.Home, "config.toml"), []byte(config), 0o600); err != nil {
		t.Fatalf("write scenario config.toml: %v", err)
	}

	ctx := context.WithValue(context.Background(), scenarioHarnessKey{}, h)
	if err := installFakeClaudeOnPATH(ctx, false); err != nil {
		t.Fatalf("install fake claude fixture on PATH: %v", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	// The create dialog's own content (title, eight fields, their
	// word-wrapped help text and the footer -- task 030) is taller than 24
	// rows once wrapped at dialogWidth's 64-column box, so it does not fit
	// whole in the golden frame's own 80x24: its own title scrolls off the
	// top before a wait on it can observe it (a SPEC.md:1282-anticipated
	// scrolling gap dialogs do not yet close, task 030's commit message
	// already flagged it). The golden frame itself is the SETTLED MAIN LIST
	// view, never the create dialog, so the fix belongs in this test's own
	// setup, not in framedDialog: open at a comfortably taller height so the
	// dialog is fully drivable, then resize down to the golden's own 80x24
	// once the session exists and the dialog has closed. The tmux pane's own
	// geometry ("of 80x24" below) is deck's own fixed default and does not
	// depend on the client's terminal size either way.
	client, err := h.StartNamedClientWithSize(ctx, "golden", 80, 40,
		"NO_COLOR=", "DECK_COLOR_DEPTH="+goldenFrameColorDepth,
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

	if err := client.Resize(80, 24); err != nil {
		t.Fatalf("resize client down to the golden frame's own 80x24: %v", err)
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

	assertGoldenFrameIsThemed(t, ctx, client)

	if err := client.Send("q"); err != nil {
		t.Fatalf("send q: %v", err)
	}
	if err := client.Stop(5 * time.Second); err != nil {
		t.Fatalf("deck client did not exit cleanly: %v", err)
	}

	return settled
}

// assertGoldenFrameIsThemed proves goldenFrameTheme/goldenFrameColorDepth
// (requirement 52) are actually exercised by this settled frame, not just
// named in an unread env var: the sidebar always draws its own top-left
// corner in theme.BorderFocus (internal/tui/panel.go's sidebarTopLine --
// the sidebar is never the unfocused panel, per SPEC, since the preview is
// not focusable), so cell (0,0)'s live rendered foreground, read directly
// off the emulator grid the same way every other per-cell colour assertion
// in this suite does, must equal internal/theme's own resolution of
// goldenFrameTheme's border_focus token -- exactly, since goldenFrameColorDepth
// is truecolor and pins away any quantisation step that could otherwise
// paper over a wrong colour landing close to the right one.
func assertGoldenFrameIsThemed(t *testing.T, ctx context.Context, client *ScreenDriver) {
	t.Helper()
	want, err := resolveScenarioTokenHex(ctx, "border_focus")
	if err != nil {
		t.Fatalf("resolve pinned theme %q token border_focus: %v", goldenFrameTheme, err)
	}
	got, err := cellForegroundHex(client.CellAt(0, 0))
	if err != nil {
		t.Fatalf("golden frame's top-left sidebar corner has no foreground colour under pinned theme %q at DECK_COLOR_DEPTH %q: %v", goldenFrameTheme, goldenFrameColorDepth, err)
	}
	if got != want {
		t.Fatalf("golden frame's top-left sidebar corner (border_focus) has foreground %s, want theme %q's border_focus %s", got, goldenFrameTheme, want)
	}
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
