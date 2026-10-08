package features

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

// repaintingTrustPane is a pane that shows the folder-trust prompt until
// repaintDelay after the first Down Enter was sent, and the idle footer after.
type repaintingTrustPane struct {
	mu           sync.Mutex
	repaintDelay time.Duration
	answeredAt   time.Time
	sequences    [][]string
}

func (p *repaintingTrustPane) capture(context.Context, string) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.answeredAt.IsZero() && time.Since(p.answeredAt) >= p.repaintDelay {
		return "❯\n· / commands\n", nil
	}
	return "Confirm folder trust\nDo you trust the files in this folder?\n", nil
}

func (p *repaintingTrustPane) send(_ context.Context, _ string, args ...string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sequences = append(p.sequences, args)
	if p.answeredAt.IsZero() {
		p.answeredAt = time.Now()
	}
	return nil
}

// R223: an explicit trust answer is shared with the readiness wait. The pane
// keeps showing the trust frame for a delayed repaint after the answer; the
// wait must not read it as a second prompt and send a second Down Enter. The
// previous reachIdle kept its own zero timestamp and answered again at once.
func TestRealCopilotTrustAnswerIsSentOnceAcrossAnswerAndReachIdle(t *testing.T) {
	pane := &repaintingTrustPane{repaintDelay: 700 * time.Millisecond}
	scenario := &realCopilotScenario{capture: pane.capture, keys: pane.send}
	ctx := context.Background()
	if err := scenario.answerTrust(ctx, "s"); err != nil {
		t.Fatal(err)
	}
	if err := scenario.reachIdle(ctx, "s"); err != nil {
		t.Fatalf("reachIdle: %v", err)
	}
	if len(pane.sequences) != 1 || strings.Join(pane.sequences[0], " ") != "Down Enter" {
		t.Fatalf("keys sent = %q, want exactly one Down Enter sequence", pane.sequences)
	}
}

// A trust prompt nobody answered is still answered by the readiness wait, and
// only once while it repaints.
func TestRealCopilotReachIdleAnswersAnUnansweredTrustPromptOnce(t *testing.T) {
	pane := &repaintingTrustPane{repaintDelay: 700 * time.Millisecond}
	scenario := &realCopilotScenario{capture: pane.capture, keys: pane.send}
	if err := scenario.reachIdle(context.Background(), "s"); err != nil {
		t.Fatalf("reachIdle: %v", err)
	}
	if len(pane.sequences) != 1 || strings.Join(pane.sequences[0], " ") != "Down Enter" {
		t.Fatalf("keys sent = %q, want exactly one Down Enter sequence", pane.sequences)
	}
}

// An answer older than the grace that still shows the prompt is asked again.
func TestRealCopilotTrustNeedsAnswerAgainAfterTheGrace(t *testing.T) {
	scenario := &realCopilotScenario{trustAnswered: map[string]time.Time{"s": time.Now().Add(-realCopilotTrustGrace - time.Second)}}
	if !scenario.trustNeedsAnswer("s") {
		t.Fatal("a stale answer must not suppress a new one")
	}
	scenario.trustAnswered["s"] = time.Now()
	if scenario.trustNeedsAnswer("s") {
		t.Fatal("a fresh answer must suppress a second one")
	}
}
