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

// Elapsed time alone never authorizes a second answer: one unchanged trust
// frame that stays visible far longer than any repaint is still the prompt that
// was already answered. The pane clock is injected, so no real waiting is needed.
func TestRealCopilotAnsweredTrustFrameIsNeverReAnsweredByElapsedTime(t *testing.T) {
	scenario := &realCopilotScenario{keys: func(context.Context, string, ...string) error { return nil }}
	if err := scenario.answerTrust(context.Background(), "s"); err != nil {
		t.Fatal(err)
	}
	for range 5 {
		time.Sleep(20 * time.Millisecond)
		if scenario.trustNeedsAnswer("s", true) {
			t.Fatal("the same answered frame must not be answered again")
		}
	}
	// Another session's prompt is independent of this session's answer.
	if !scenario.trustNeedsAnswer("other", true) {
		t.Fatal("an unanswered prompt in another session must be answered")
	}
}

// A same-frame prompt that stays visible for several seconds (past the former
// grace) still receives exactly one Down Enter through the real helpers.
func TestRealCopilotLongLivedSameFrameIsAnsweredOnce(t *testing.T) {
	pane := &repaintingTrustPane{repaintDelay: 3500 * time.Millisecond}
	scenario := &realCopilotScenario{capture: pane.capture, keys: pane.send}
	ctx := context.Background()
	if err := scenario.answerTrust(ctx, "s"); err != nil {
		t.Fatal(err)
	}
	if err := scenario.reachIdle(ctx, "s"); err != nil {
		t.Fatalf("reachIdle: %v", err)
	}
	if len(pane.sequences) != 1 {
		t.Fatalf("keys sent = %q, want exactly one Down Enter sequence", pane.sequences)
	}
}

// A genuinely new prompt, observed after the answered prompt went away, is
// answered again; so is one in a different session, and one after the pane was
// seen without a trust frame in between.
func TestRealCopilotNewTrustPromptAfterATransitionIsAnswered(t *testing.T) {
	scenario := &realCopilotScenario{keys: func(context.Context, string, ...string) error { return nil }}
	if err := scenario.answerTrust(context.Background(), "s"); err != nil {
		t.Fatal(err)
	}
	if scenario.trustNeedsAnswer("s", true) {
		t.Fatal("the answered frame must not be answered again")
	}
	if scenario.trustNeedsAnswer("s", false) {
		t.Fatal("a pane without a trust frame needs no answer")
	}
	if !scenario.trustNeedsAnswer("s", true) {
		t.Fatal("a trust frame after the prompt went away is a new prompt")
	}
}

// End to end through reachIdle: the first prompt is answered and goes away, a
// second prompt then appears and is answered once more, then the pane is idle.
func TestRealCopilotReachIdleAnswersASecondPromptAfterTheFirstWentAway(t *testing.T) {
	frames := []string{"Confirm folder trust\nDo you trust the files in this folder?\n", "Confirm folder trust\nDo you trust the files in this folder?\n", "starting\n", "Confirm folder trust\nDo you trust the files in this folder?\n", "Confirm folder trust\nDo you trust the files in this folder?\n", "❯\n· / commands\n"}
	var mu sync.Mutex
	next := 0
	var sent [][]string
	scenario := &realCopilotScenario{
		capture: func(context.Context, string) (string, error) {
			mu.Lock()
			defer mu.Unlock()
			f := frames[min(next, len(frames)-1)]
			next++
			return f, nil
		},
		keys: func(_ context.Context, _ string, args ...string) error {
			mu.Lock()
			defer mu.Unlock()
			sent = append(sent, args)
			return nil
		},
	}
	if err := scenario.reachIdle(context.Background(), "s"); err != nil {
		t.Fatalf("reachIdle: %v", err)
	}
	if len(sent) != 2 {
		t.Fatalf("keys sent = %q, want one answer per distinct prompt (2)", sent)
	}
}
