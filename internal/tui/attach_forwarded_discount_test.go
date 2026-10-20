// attach_forwarded_discount_test.go is R305 for the keys and resizes deck
// forwards during a full attach (`a`). deck sits in the attach client's key
// path, so the pane's answer to a forwarded key or a relayed terminal resize
// is deck-caused: each opens one bounded discount window, and the agent's own
// output after it counts while the client is still attached.
package tui

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/creack/pty"

	"github.com/n-orlov/deck/internal/service"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// orderedPty records, in order, the discounts opened and the chunks written
// to the client, so a test can see the discount precede the write.
type orderedPty struct {
	mu   sync.Mutex
	log  []string
	fail bool
}

func (p *orderedPty) note(s string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.log = append(p.log, s)
}

func (p *orderedPty) Write(b []byte) (int, error) {
	p.note("write")
	if p.fail {
		return 0, errors.New("client gone")
	}
	return len(b), nil
}

func (p *orderedPty) discount(cause service.DiscountCause, _ time.Time) {
	p.note("discount:" + string(cause))
}

func (p *orderedPty) entries() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.log...)
}

func TestFullAttachForwardedInputOpensAKeyDiscountBeforeTheWrite(t *testing.T) {
	for _, tc := range []struct {
		name     string
		chunk    string
		discount bool
		counted  bool
	}{
		{"a typed key", "x", true, true},
		{"a typed line", "ls -la\r", true, true},
		{"enter alone", "\r", true, true},
		{"a paste", "\x1b[200~pasted text\x1b[201~", true, true},
		// tmux's prefix and the key after it run a tmux command (zoom,
		// copy mode, a window switch) that can make the pane redraw:
		// deck-caused output, though not an engagement key.
		{"prefix and a command key", "\x02z", true, false},
		// Not keys: the pane does not answer them with an echo.
		{"a mouse report", "\x1b[<0;10;5M", false, false},
		{"a focus change", "\x1b[I", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &orderedPty{}
			w := &watchedAttach{discount: client.discount}
			proxy := newAttachKeyProxy("C-b", "None")
			w.forward(client, proxy, []byte(tc.chunk))
			got := client.entries()
			want := []string{"write"}
			if tc.discount {
				want = []string{"discount:" + string(service.DiscountKey), "write"}
			}
			if strings.Join(got, ",") != strings.Join(want, ",") {
				t.Fatalf("forwarding %q: %v, want %v (the discount must open before the pane can echo)", tc.chunk, got, want)
			}
			if n := proxy.keys.Load(); (n > 0) != tc.counted {
				t.Fatalf("forwarding %q counted %d engagement keys, want a key: %v", tc.chunk, n, tc.counted)
			}
		})
	}
}

// A chunk the client never received still opened its discount (harmless: a
// bounded window), but is no engagement key.
func TestFullAttachUnwrittenKeyIsNotCounted(t *testing.T) {
	client := &orderedPty{fail: true}
	w := &watchedAttach{discount: client.discount}
	proxy := newAttachKeyProxy("C-b", "None")
	w.forward(client, proxy, []byte("x"))
	if n := proxy.keys.Load(); n != 0 {
		t.Fatalf("a key that never reached the client counted %d", n)
	}
}

// A watcher with no clock wired forwards exactly as before.
func TestFullAttachForwardWithoutAClockStillForwards(t *testing.T) {
	client := &orderedPty{}
	w := &watchedAttach{}
	proxy := newAttachKeyProxy("C-b", "None")
	w.forward(client, proxy, []byte("x"))
	if got := client.entries(); len(got) != 1 || got[0] != "write" || proxy.keys.Load() != 1 {
		t.Fatalf("forward without a clock: %v, keys %d", got, proxy.keys.Load())
	}
}

// A SIGWINCH deck relays resizes the client's pty, which resizes the tmux
// window and makes the pane redraw: the relay opens a resize discount first.
func TestFullAttachRelayedResizeOpensAResizeDiscountBeforeTheResize(t *testing.T) {
	ptmx, tty, err := pty.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ptmx.Close(); _ = tty.Close() }()
	userPtmx, userTTY, err := pty.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = userPtmx.Close(); _ = userTTY.Close() }()
	if err := pty.Setsize(userPtmx, &pty.Winsize{Rows: 40, Cols: 132}); err != nil {
		t.Fatal(err)
	}
	var sizeAtDiscount [2]int
	var causes []service.DiscountCause
	w := &watchedAttach{discount: func(cause service.DiscountCause, _ time.Time) {
		causes = append(causes, cause)
		rows, cols, _ := pty.Getsize(ptmx)
		sizeAtDiscount = [2]int{rows, cols}
	}}
	w.relayedResize(ptmx, userTTY, userTTY)()
	if len(causes) != 1 || causes[0] != service.DiscountResize {
		t.Fatalf("relayed resize opened %v, want one %q", causes, service.DiscountResize)
	}
	if sizeAtDiscount == [2]int{40, 132} {
		t.Fatal("the discount opened only after the client's pty was resized")
	}
	if rows, cols, _ := pty.Getsize(ptmx); rows != 40 || cols != 132 {
		t.Fatalf("client pty is %dx%d after the relay, want 132x40", cols, rows)
	}
}

func TestModelHandsTheAttachedSessionToTheAttachDiscount(t *testing.T) {
	rec := &discountRecorder{}
	m := Model{}.WithOutputDiscounter(rec)
	discount := m.attachOutputDiscount(store.Session{ID: "attached-1", Slug: "a1"})
	if discount == nil {
		t.Fatal("no attach discount for a Model with a clock wired")
	}
	discount(service.DiscountKey, time.Now())
	discount(service.DiscountResize, time.Now())
	wantEvents(t, rec, string(service.DiscountKey)+":attached-1", string(service.DiscountResize)+":attached-1")
	if (Model{}).attachOutputDiscount(store.Session{ID: "x"}) != nil {
		t.Fatal("a Model with no clock produced an attach discount")
	}
}

// outputWrites records the clock's last_output_at writes.
type outputWrites struct {
	mu sync.Mutex
	at []int64
}

func (o *outputWrites) RecordOutput(_ context.Context, _ string, at int64) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.at = append(o.at, at)
	return nil
}

func (o *outputWrites) count() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return len(o.at)
}

// The whole rule on a real tmux attach with the real clock: the echo of keys
// the user types through deck's attach does not advance last_output_at, and
// the agent's own output a window later does, with the client still attached.
func TestFullAttachForwardedKeyOutputIsDiscountedAndLaterAgentOutputCounts(t *testing.T) {
	const id = "attached-row"
	clock := service.NewOutputClock(time.Second)
	writes := &outputWrites{}
	var reads sync.WaitGroup
	reads.Add(1)
	var once sync.Once
	live := startLiveAttachConfigured(t, 23, func(w *watchedAttach, socket string) {
		client := tmux.Client{Socket: socket}
		w.observeOutput = func(ctx context.Context) {
			pane, ok, err := client.PreviewPane(ctx, w.slug)
			if err != nil || !ok {
				return
			}
			_, _ = clock.Observe(ctx, writes, id, pane.WindowActivity, func(ctx context.Context) (string, error) {
				out, err := client.CapturePane(ctx, pane.ID, tmux.CaptureOptions{StartLine: "-200", EndLine: "-"})
				return string(out), err
			})
			once.Do(reads.Done)
		}
		w.discount = func(cause service.DiscountCause, at time.Time) { clock.Discount(id, cause, at) }
	})
	reads.Wait()
	// Let the quiet pane's baseline settle across a bucket boundary.
	time.Sleep(1200 * time.Millisecond)
	if n := writes.count(); n != 0 {
		t.Fatalf("a quiet attach wrote last_output_at %d times", n)
	}
	// The user types; the pane's tty echoes every key.
	for _, chunk := range []string{"typed", " through", " deck"} {
		if _, err := live.terminal.Write([]byte(chunk)); err != nil {
			t.Fatal(err)
		}
		time.Sleep(150 * time.Millisecond)
	}
	waitForPaneText(t, live, "typed through deck")
	// One key window, plus the second of window_activity resolution the
	// clock compares windows at.
	time.Sleep(clock.DiscountWindow() + 1500*time.Millisecond)
	if n := writes.count(); n != 0 {
		t.Fatalf("the echo of keys forwarded through the attach advanced last_output_at (%d writes)", n)
	}
	// The agent prints on its own, past the key window, still attached.
	paneTTY := strings.TrimSpace(live.output(t, "display-message", "-p", "-t", "="+live.name+":", "#{pane_tty}"))
	f, err := os.OpenFile(paneTTY, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.WriteString("\r\nthe agent printed this on its own\r\n")
	_ = f.Close()
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(4 * time.Second)
	for writes.count() == 0 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if writes.count() == 0 {
		t.Fatal("the agent's own output after the key window did not count while attached")
	}
	if live.detach(t) == "" {
		t.Fatal("the typed keys were not reported as engagement")
	}
}

func (l *liveAttach) output(t *testing.T, args ...string) string {
	t.Helper()
	out, err := exec.Command("tmux", append([]string{"-L", l.socket}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("tmux %v: %v: %s", args, err, out)
	}
	return string(out)
}

func waitForPaneText(t *testing.T, l *liveAttach, text string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		if strings.Contains(l.output(t, "capture-pane", "-p", "-t", "="+l.name+":"), text) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the pane never showed %q", text)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
