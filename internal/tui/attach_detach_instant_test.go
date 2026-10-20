// attach_detach_instant_test.go is R410 for the detach caller: the detach
// repaint's discount span starts at the instant deck saw the attach client
// end, never at the later instant its exec callback runs. Between the two
// lie the watcher's output drain (up to attachOutputDrain), bubbletea's
// terminal restore and the scheduling of the callback; an instant taken
// there can land in the second after the repaint was stamped, which leaves
// that second uncovered.
package tui

import (
	"context"
	"io"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/service"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// slowTerminal is the user's terminal as a slow consumer: the first write
// blocks for delay, so the watcher's output drain outlasts the client.
type slowTerminal struct {
	mu         sync.Mutex
	delay      time.Duration
	firstAt    time.Time
	returnedAt time.Time
}

func (s *slowTerminal) Write(b []byte) (int, error) {
	s.mu.Lock()
	first := s.firstAt.IsZero()
	if first {
		s.firstAt = time.Now()
	}
	s.mu.Unlock()
	if first {
		time.Sleep(s.delay)
		s.mu.Lock()
		s.returnedAt = time.Now()
		s.mu.Unlock()
	}
	return len(b), nil
}

func (s *slowTerminal) times() (first, returned time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.firstAt, s.returnedAt
}

// instantRecorder is an OutputDiscounter that keeps the instants it was given.
type instantRecorder struct {
	mu      sync.Mutex
	started []time.Time
	ended   []time.Time
}

func (r *instantRecorder) Discount(string, service.DiscountCause, time.Time) {}
func (r *instantRecorder) AttachStarted(_ string, at time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.started = append(r.started, at)
}

func (r *instantRecorder) AttachEnded(_ string, at time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ended = append(r.ended, at)
}

// clientOf runs `a` through attachSelected and returns the attach client it
// hands to tea.Exec, with the callback bubbletea calls after it.
func clientOf(t *testing.T, m Model, client *exec.Cmd) (Model, tea.ExecCommand, tea.ExecCallback) {
	t.Helper()
	next, cmd := m.attachSelected()
	m = next.(Model)
	if cmd == nil || m.attachingSessionID == "" {
		t.Fatalf("attachSelected did not hand the terminal to a client (error %q)", m.attachError)
	}
	session, _ := m.selectedSession()
	run, fn := m.attachClient(session, client)
	return m, run, fn
}

// detachInstantModel is a Model with one live row whose `a` builds an attach
// command; the client the test then runs is clientOf's.
func detachInstantModel(rec *instantRecorder) Model {
	m := New(nil, config.Settings{}, "").WithOutputDiscounter(rec)
	m.width, m.height = 100, 30
	m.sessions = []store.Session{{ID: "s-detach", Slug: "s-detach", Name: "d", Status: "running"}}
	m.selected = rowCursor(0)
	m.attach = func(context.Context, string) (*exec.Cmd, error) { return exec.Command("true"), nil }
	return m
}

func TestDetachDiscountStartsWhenTheClientEndsNotWhenItsCallbackRuns(t *testing.T) {
	// The callback runs this long after the command returned: bubbletea's
	// terminal restore and the message's scheduling, made long enough to
	// cross a second.
	const callbackLag = 1100 * time.Millisecond
	t.Run("watched attach with an output drain", func(t *testing.T) {
		rec := &instantRecorder{}
		m := detachInstantModel(rec)
		m.attachSample = func(context.Context, string, int) (tmux.AttachSample, error) {
			return attachSampleAt(80, 24, "prompt"), nil
		}
		m, run, fn := clientOf(t, m, exec.Command("sh", "-c", "printf redraw; exit 0"))
		if _, ok := run.(*watchedAttach); !ok {
			t.Fatalf("the watched attach ran %T, want *watchedAttach", run)
		}
		term := &slowTerminal{delay: 1300 * time.Millisecond}
		run.SetStdin(eofReader{})
		run.SetStdout(term)
		run.SetStderr(io.Discard)
		err := run.Run()
		time.Sleep(callbackLag)
		finished, ok := fn(err).(attachFinished)
		if !ok {
			t.Fatal("the callback did not return attachFinished")
		}
		next, _ := m.Update(finished)
		_ = next.(Model)
		redrawAt, drained := term.times()
		if redrawAt.IsZero() || drained.IsZero() {
			t.Fatalf("the client's output never reached the terminal (first %v, drained %v)", redrawAt, drained)
		}
		rec.mu.Lock()
		ended := append([]time.Time(nil), rec.ended...)
		rec.mu.Unlock()
		if len(ended) != 1 {
			t.Fatalf("AttachEnded was called %d times, want 1", len(ended))
		}
		at := ended[0]
		if at.IsZero() || at.Before(redrawAt) {
			t.Fatalf("detach span start %v is before the client's own output at %v: the client had not ended", at, redrawAt)
		}
		if !at.Before(drained) {
			t.Fatalf("detach span start %v is not before the output drain finished at %v: it was taken after the drain (R410)", at, drained)
		}
		if lag := at.Sub(redrawAt); lag >= time.Second {
			t.Fatalf("detach span start is %v after the client's last output; a span starting there can miss the second the repaint was stamped in", lag)
		}
	})
	t.Run("plain attach", func(t *testing.T) {
		rec := &instantRecorder{}
		m := detachInstantModel(rec)
		m, run, fn := clientOf(t, m, exec.Command("true"))
		devNull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = devNull.Close() }()
		run.SetStdin(devNull)
		run.SetStdout(devNull)
		run.SetStderr(devNull)
		runErr := run.Run()
		returned := time.Now()
		time.Sleep(callbackLag)
		next, _ := m.Update(fn(runErr))
		_ = next.(Model)
		rec.mu.Lock()
		ended := append([]time.Time(nil), rec.ended...)
		rec.mu.Unlock()
		if len(ended) != 1 {
			t.Fatalf("AttachEnded was called %d times, want 1", len(ended))
		}
		if at := ended[0]; at.IsZero() || at.After(returned) {
			t.Fatalf("detach span start %v is after the client's run returned at %v: it was taken in the callback (R410)", at, returned)
		}
	})
}

// eofReader is a user who types nothing.
type eofReader struct{}

func (eofReader) Read([]byte) (int, error) { return 0, io.EOF }
