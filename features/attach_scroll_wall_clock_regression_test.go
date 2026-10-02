package features

import (
	"context"
	"os"
	"regexp"
	"testing"
	"time"
)

// tmuxDefaultStatusClock matches the wall-clock text tmux's default
// status-right paints (`%H:%M %d-%b-%y`, e.g. "00:41 02-Oct-26").
var tmuxDefaultStatusClock = regexp.MustCompile(`\b[0-9]{2}:[0-9]{2} [0-9]{2}-[A-Z][a-z]{2}-[0-9]{2}\b`)

// TestAttachScrollBaselineFrameCarriesNoWallClock drives
// attach_scroll.feature:11's own steps up to the point where it captures its
// byte-exact "before-wheel-scroll" baseline, and requires that baseline to
// contain no wall-clock text. A baseline that carries the host's minute
// cannot stay equal to a frame compared after the next minute boundary, so
// the scenario's later frame comparison failed whenever capture and compare
// fell on either side of one (sweep2 at 4012be8a92, attach_scroll.feature:11
// -race, 1 of 20 reps: only the status-line clock differed).
func TestAttachScrollBaselineFrameCarriesNoWallClock(t *testing.T) {
	h, err := newScenarioHarness(buildDeckBinary(t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(h.Binary) }()
	defer func() {
		if err := h.Close(); err != nil {
			t.Error(err)
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	stepCtx := context.WithValue(ctx, scenarioHarnessKey{}, h)

	steps := []struct {
		name string
		run  func() error
	}{
		{"start client", func() error { return startNamedClient(stepCtx, "A") }},
		{"create shell session", func() error { return clientCreatesShellSession(stepCtx, "A", "attach-scroll-target") }},
		{"row running", func() error {
			return clientRowContainsWithinReconcile(stepCtx, "A", "attach-scroll-target", "running")
		}},
		{"attach", func() error { return clientAttachesToSelectedSession(stepCtx, "A") }},
		{"fill scrollback", func() error { return clientFillsAttachedPaneWithScrollback(stepCtx, "A") }},
		{"capture baseline", func() error { return clientCapturesFrameAs(stepCtx, "A", "before-wheel-scroll") }},
	}
	for _, step := range steps {
		if err := step.run(); err != nil {
			t.Fatalf("%s: %v", step.name, err)
		}
	}
	client, err := mouseSynthesisClient(stepCtx, "A")
	if err != nil {
		t.Fatal(err)
	}
	frame := client.Frame(false)
	// Leave the client the way the scenario does, so the harness's Close
	// finds no hung client whichever way the assertions below go.
	defer func() {
		if err := clientDetaches(stepCtx, "A"); err != nil {
			t.Errorf("detach: %v", err)
			return
		}
		if err := clientScreenContains(stepCtx, "A", "deck - sessions"); err != nil {
			t.Errorf("back on the list after detach: %v", err)
			return
		}
		if err := clientExitsCleanly(stepCtx, "A"); err != nil {
			t.Errorf("exit cleanly: %v", err)
		}
	}()
	if clock := tmuxDefaultStatusClock.FindString(frame); clock != "" {
		t.Fatalf("attached baseline frame carries wall-clock text %q, so a compare after the next minute boundary differs from it:\n%s", clock, frame)
	}
	if now := time.Now().Format("15:04"); regexp.MustCompile(`\b` + now + `\b`).MatchString(frame) {
		t.Fatalf("attached baseline frame carries the current wall-clock minute %q:\n%s", now, frame)
	}
}
