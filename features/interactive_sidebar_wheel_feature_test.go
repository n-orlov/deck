// interactive_sidebar_wheel_feature_test.go backs
// features/interactive_sidebar_wheel.feature (task 003, R149/GH #47): the
// end-to-end, real-tmux proof that a wheel notch over the sidebar while
// interactive mode owns the keyboard moves only the sidebar's own
// viewport -- leaving the interactive target, the grid's rendered content,
// and the live tmux window's own size (its ownership, not the deck
// binary's) all untouched -- and that the very next forwarded keystroke
// both ends that drift (the selection lands back in view) and still
// reaches the pane byte-exact. Tasks 001/002 (already landed) are the
// product-side fix this scenario exercises; this file adds only the
// black-box assertions no existing step already covers: a snapshot of the
// PREVIEW panel's own rendered text (distinct from a whole-frame snapshot,
// which the sidebar's own viewport move would always change), and a
// direct real-tmux capture-pane read proving the forwarded byte landed.
package features

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

func registerInteractiveSidebarWheelFeatureSteps(sc *godog.ScenarioContext) {
	sc.Step(`^deck client "([^"]+)" preview content is captured as "([^"]+)"$`, clientPreviewContentIsCapturedAs)
	sc.Step(`^deck client "([^"]+)" preview content still matches the captured "([^"]+)"$`, clientPreviewContentStillMatchesCaptured)
	sc.Step(`^the private tmux pane for session "([^"]+)" received "([^"]+)" byte-exact$`, privateTMuxPaneForSessionReceivedByteExact)
}

// previewPanelText extracts the preview panel's own rendered text from a
// full frame, reusing mouse_bindings_test.go's own previewRegion (shape
// detection shared with every other black-box step in this package,
// rather than importing internal/tui) -- the SAME region locatePreviewText
// searches, joined back into a single string so two captures can be
// compared byte-for-byte.
func previewPanelText(frame string) (string, error) {
	lines := strings.Split(frame, "\n")
	rowStart, rowEnd, colStart, err := previewRegion(frame)
	if err != nil {
		return "", fmt.Errorf("preview panel text: %w", err)
	}
	var b strings.Builder
	for i := rowStart; i <= rowEnd && i < len(lines); i++ {
		line := lines[i]
		if colStart > 0 {
			runes := []rune(line)
			if colStart < len(runes) {
				line = string(runes[colStart:])
			} else {
				line = ""
			}
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String(), nil
}

func clientPreviewContentIsCapturedAs(ctx context.Context, name, label string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(name)
	if err != nil {
		return err
	}
	text, err := previewPanelText(client.Frame(false))
	if err != nil {
		return fmt.Errorf("client %q: %w", name, err)
	}
	if h.previewContentSnapshots == nil {
		h.previewContentSnapshots = make(map[string]string)
	}
	h.previewContentSnapshots[label] = text
	return nil
}

func clientPreviewContentStillMatchesCaptured(ctx context.Context, name, label string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	want, ok := h.previewContentSnapshots[label]
	if !ok {
		return fmt.Errorf("no preview content was captured as %q", label)
	}
	client, err := h.Client(name)
	if err != nil {
		return err
	}
	got, err := previewPanelText(client.Frame(false))
	if err != nil {
		return fmt.Errorf("client %q: %w", name, err)
	}
	if got != want {
		return fmt.Errorf("client %q preview content changed: captured %q as:\n%s\nnow:\n%s", name, label, want, got)
	}
	return nil
}

// privateTMuxPaneForSessionReceivedByteExact reads the interactive
// target's own live tmux pane (real capture-pane -p, the same primitive
// internal/tui's own drift-end tests read the far end of a dispatched
// send with) and asserts its trimmed content ends in exactly key -- proof
// the byte a prior "sends" step wrote to the deck client's pty was
// forwarded through the dispatcher and landed on the real pane, not just
// echoed onto deck's own frame. Polled (never a single read): the send is
// asynchronous from this process's point of view, so tmux's own event
// loop needs a moment to read it off the pty before capture-pane reflects
// it.
func privateTMuxPaneForSessionReceivedByteExact(ctx context.Context, name, key string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	slug, err := sessionSlugByName(h, name)
	if err != nil {
		return err
	}
	target := "deck_" + slug
	deadline := time.Now().Add(5 * time.Second)
	var last string
	for {
		out, capErr := tmuxOutput(ctx, h, "capture-pane", "-p", "-t", target)
		if capErr == nil {
			last = string(out)
			trimmed := strings.TrimRight(last, " \n")
			if strings.HasSuffix(trimmed, key) {
				return nil
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("private tmux pane %q never ended in byte-exact %q within the deadline (err=%v); last capture:\n%s", target, key, capErr, last)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
