package service

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

const testApplyPoll = 250 * time.Millisecond

// pollingPause makes the poll wait of ApplyLaunchNameWhenReady observable and
// finite without a clock: the first `limit` polls return at once (running
// onPoll), the next one reports the wait as over. Every other pause (the
// adapter's text/Enter gap) is recorded as the pane's own.
func pollingPause(pane *codexPane, limit int, onPoll func(n int)) func(context.Context, time.Duration) error {
	polls := 0
	return func(ctx context.Context, d time.Duration) error {
		if d != testApplyPoll {
			return pane.pause(ctx, d)
		}
		polls++
		if polls > limit {
			return context.DeadlineExceeded
		}
		if onPoll != nil {
			onPoll(polls)
		}
		return nil
	}
}

// R257: a name given to a detached `deck new` waits through a modal on the
// service's own readiness gate, then goes out as the paced text and Enter.
func TestCodexApplyLaunchNameWaitsThroughModalThenSendsPaced(t *testing.T) {
	svc, db, pane := newCodexRig(t, "Do you trust the contents of this directory?\n 1. Yes, continue\n 2. No, quit\n")
	codexRow(t, db, "w1", "starting", "user")
	svc.Pause = pollingPause(pane, 10, func(n int) {
		if n == 3 {
			pane.screen = codexFixture(t, "starting")
		}
	})
	if err := svc.ApplyLaunchNameWhenReady(context.Background(), "w1", time.Minute, testApplyPoll); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(pane.log, codexWant("w1")) {
		t.Fatalf("log = %v, want nothing typed into the modal, then %v", pane.log, codexWant("w1"))
	}
	if got := eventReasons(t, db, "w1", EventHarnessNameSent); !reflect.DeepEqual(got, []string{"fix login w1"}) {
		t.Errorf("marker = %v", got)
	}
}

// A prompt that never becomes ready is a reported failure: one
// harness_rename_failed naming the refusal, nothing typed, deck's name kept.
func TestCodexApplyLaunchNameReportsAPromptThatNeverBecomesReady(t *testing.T) {
	svc, db, pane := newCodexRig(t, "✨ Update available! 0.154.0 -> 0.156.1\n 1. Update now\n 3. Skip until next version\n")
	codexRow(t, db, "w2", "starting", "user")
	svc.Pause = pollingPause(pane, 3, nil)
	err := svc.ApplyLaunchNameWhenReady(context.Background(), "w2", time.Minute, testApplyPoll)
	if err == nil || !strings.Contains(err.Error(), "dialog") {
		t.Fatalf("err = %v, want the dialog refusal", err)
	}
	if len(pane.log) != 0 {
		t.Errorf("typed into a modal: %v", pane.log)
	}
	if got := eventReasons(t, db, "w2", EventHarnessRenameFailed); len(got) != 1 || !strings.Contains(got[0], "launch name not sent") {
		t.Errorf("failed events = %v", got)
	}
	if got, _ := db.GetSession(context.Background(), "w2"); got.Name != "fix login w2" || got.NameSource != "user" {
		t.Errorf("row = %q/%q", got.Name, got.NameSource)
	}
}

// A send that fails is reported to the caller and recorded once, and the row
// keeps deck's name.
func TestCodexApplyLaunchNameReportsAFailedSend(t *testing.T) {
	svc, db, pane := newCodexRig(t, codexFixture(t, "starting"))
	pane.err = errors.New("tmux gone")
	codexRow(t, db, "w3", "starting", "user")
	err := svc.ApplyLaunchNameWhenReady(context.Background(), "w3", time.Minute, testApplyPoll)
	if err == nil || !strings.Contains(err.Error(), "tmux gone") {
		t.Fatalf("err = %v", err)
	}
	if got := eventReasons(t, db, "w3", EventHarnessRenameFailed); len(got) != 1 {
		t.Errorf("failed events = %v", got)
	}
	if got, _ := db.GetSession(context.Background(), "w3"); got.Name != "fix login w3" {
		t.Errorf("name = %q", got.Name)
	}
}

// A row the launch path does not own (derived name, past starting, gone) is
// finished at once with nothing typed.
func TestCodexApplyLaunchNameLeavesRowsItDoesNotOwn(t *testing.T) {
	svc, db, pane := newCodexRig(t, codexFixture(t, "starting"))
	codexRow(t, db, "w4", "starting", "derived")
	codexRow(t, db, "w5", "running", "user")
	for _, id := range []string{"w4", "w5"} {
		if err := svc.ApplyLaunchNameWhenReady(context.Background(), id, time.Minute, testApplyPoll); err != nil {
			t.Errorf("%s: %v", id, err)
		}
	}
	if len(pane.log) != 0 {
		t.Errorf("typed into rows it does not own: %v", pane.log)
	}
	if err := svc.ApplyLaunchNameWhenReady(context.Background(), "missing", time.Minute, testApplyPoll); err == nil {
		t.Error("a missing row must be an error")
	}
}

// A running TUI's tick and a detached applier looking at the same row at the
// same moment type the name once between them.
func TestCodexApplyLaunchNameRacingTicksTypeOnce(t *testing.T) {
	svc, db, pane := newCodexRig(t, codexFixture(t, "starting"))
	session := codexRow(t, db, "w6", "starting", "user")
	var mu sync.Mutex
	svc.NamePane = lockedPane{pane, &mu}
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); svc.applyLaunchName(context.Background(), session) }()
		go func() {
			defer wg.Done()
			_ = svc.ApplyLaunchNameWhenReady(context.Background(), "w6", time.Minute, testApplyPoll)
		}()
	}
	wg.Wait()
	sends := 0
	for _, entry := range pane.log {
		if strings.HasPrefix(entry, "send:/rename") {
			sends++
		}
	}
	if sends != 1 {
		t.Fatalf("name typed %d times: %v", sends, pane.log)
	}
}

type lockedPane struct {
	*codexPane
	mu *sync.Mutex
}

func (p lockedPane) SendPlan(ctx context.Context, slug string, plan []string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.codexPane.SendPlan(ctx, slug, plan)
}
