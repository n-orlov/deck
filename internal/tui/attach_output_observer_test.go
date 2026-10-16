package tui

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/store"
)

// R305: the TUI's reconcile ticks stop for the length of a full attach, so the
// attach watcher feeds the output clock itself while the client is attached.

func TestAttachWatcherFeedsTheOutputClockWhileAttached(t *testing.T) {
	var reads atomic.Int64
	w := &watchedAttach{outputEvery: time.Millisecond, observeOutput: func(context.Context) { reads.Add(1) }}
	ctx, cancel := context.WithCancel(context.Background())
	done := w.startOutputObserver(ctx)
	deadline := time.Now().Add(5 * time.Second)
	for reads.Load() < 3 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := reads.Load(); got < 3 {
		t.Fatalf("the clock was read %d times during the attach, want repeated reads", got)
	}
	cancel()
	<-done
	stopped := reads.Load()
	time.Sleep(20 * time.Millisecond)
	if got := reads.Load(); got != stopped {
		t.Fatalf("the clock was read %d more times after the attach ended", got-stopped)
	}
}

func TestAttachWatcherWithoutAnOutputObserverStopsAtOnce(t *testing.T) {
	w := &watchedAttach{}
	select {
	case <-w.startOutputObserver(context.Background()):
	case <-time.After(time.Second):
		t.Fatal("an observer-less watcher never reported done")
	}
}

func TestModelHandsTheAttachedSessionToTheOutputObserver(t *testing.T) {
	var got []string
	m := Model{}.WithAttachOutputObserver(func(_ context.Context, s store.Session) error {
		got = append(got, s.ID)
		return nil
	})
	observe := m.attachOutputObserver(store.Session{ID: "attached-1", Slug: "a1"})
	if observe == nil {
		t.Fatal("no observer for a Model with one wired")
	}
	observe(context.Background())
	observe(context.Background())
	if len(got) != 2 || got[0] != "attached-1" || got[1] != "attached-1" {
		t.Fatalf("observer saw %v, want the attached session twice", got)
	}
	if (Model{}).attachOutputObserver(store.Session{ID: "x"}) != nil {
		t.Fatal("a Model with no observer wired produced one")
	}
}
