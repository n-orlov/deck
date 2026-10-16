// output_discount_wiring_test.go is R305's TUI half: every deck-caused
// resize, attach and forwarded key must reach the output clock's discount,
// because the clock itself (internal/service/output_clock_discount_test.go)
// can only discount what it is told about.
package tui

import (
	"context"
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

type discountRecorder struct {
	mu     sync.Mutex
	events []string
}

func (r *discountRecorder) add(e string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, e)
}
func (r *discountRecorder) Discount(id string, cause service.DiscountCause) {
	r.add(string(cause) + ":" + id)
}
func (r *discountRecorder) AttachStarted(id string) { r.add("attach-start:" + id) }
func (r *discountRecorder) AttachEnded(id string)   { r.add("attach-end:" + id) }
func (r *discountRecorder) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.events...)
}

func wantEvents(t *testing.T, r *discountRecorder, want ...string) {
	t.Helper()
	got := r.snapshot()
	if len(got) != len(want) {
		t.Fatalf("discount events = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("discount events = %v, want %v", got, want)
		}
	}
}

func TestAttachDiscountsTheHandOffAndTheDetachRepaint(t *testing.T) {
	rec := &discountRecorder{}
	m := New(nil, config.Settings{}, "").WithOutputDiscounter(rec)
	m.width, m.height = 100, 30
	m.sessions = []store.Session{{ID: "s-attach", Slug: "s-attach", Name: "a", Status: "running"}}
	m.selected = rowCursor(0)
	m.attach = func(context.Context, string) (*exec.Cmd, error) { return exec.Command("true"), nil }

	m = pressKey(t, m, "a")
	wantEvents(t, rec, "attach-start:s-attach")
	updated, _ := m.Update(attachFinished{})
	_ = updated.(Model)
	wantEvents(t, rec, "attach-start:s-attach", "attach-end:s-attach")
}

func TestRefusedAttachDiscountsNothing(t *testing.T) {
	rec := &discountRecorder{}
	m := New(nil, config.Settings{}, "").WithOutputDiscounter(rec)
	m.width, m.height = 100, 30
	m.sessions = []store.Session{{ID: "s-refuse", Slug: "s-refuse", Name: "a", Status: "running"}}
	m.selected = rowCursor(0)
	m.attach = func(context.Context, string) (*exec.Cmd, error) { return nil, context.Canceled }
	_ = pressKey(t, m, "a")
	wantEvents(t, rec)
}

func TestForwardedKeyDiscountsOutputAndAnUnforwardedKeyDoesNot(t *testing.T) {
	socket := selectionTestSocket("discount-key")
	m := forwardGateTestModel(t, socket, "discount_key", 0)
	rec := &discountRecorder{}
	m.outputDiscount = rec

	next, _ := m.updateInteractive(tea.KeyMsg(tea.Key{Type: tea.KeyInsert, Alt: true}))
	m = next.(Model)
	wantEvents(t, rec)

	next, _ = m.updateInteractive(tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune("x")}))
	m = next.(Model)
	wantEvents(t, rec, "key:sess-discount_key")
	next, _ = m.updateInteractive(tea.KeyMsg(tea.Key{Type: tea.KeyEnter}))
	_ = next
	wantEvents(t, rec, "key:sess-discount_key", "key:sess-discount_key")
}

func TestPreviewFitResizeDiscountsOutput(t *testing.T) {
	socket := selectionTestSocket("discount-fit")
	const slug = "discountfit"
	newQuietSelectionPane(t, socket, "deck_"+slug, 80, 24)
	rec := &discountRecorder{}
	m := New(nil, config.Settings{PreviewFit: true, Preview: time.Millisecond}, "").WithOutputDiscounter(rec)
	m.width, m.height = 100, 30
	m.tmuxClient = tmux.Client{Socket: socket}
	m.sessions = []store.Session{{ID: "sess-discountfit", Slug: slug, Name: slug, Status: "running"}}
	m.selected = rowCursor(0)
	if w, h := m.previewContentSize(); w == 80 && h == 24 {
		t.Fatal("test setup: the panel box equals the pane size, so the fit would not resize")
	}

	updated, cmd := m.Update(previewTick(time.Now()))
	_ = updated
	cmds := previewTickCmds(t, cmd)
	if len(cmds) != 2 {
		t.Fatalf("previewTick returned %d commands, want 2", len(cmds))
	}
	if _, ok := cmds[1]().(previewFitDone); !ok {
		t.Fatal("the fit command did not produce previewFitDone")
	}
	wantEvents(t, rec, "resize:sess-discountfit")

	// A fit that finds the window already the right size resizes nothing and
	// so discounts nothing.
	m2 := New(nil, config.Settings{PreviewFit: true, Preview: time.Millisecond}, "").WithOutputDiscounter(rec)
	m2.width, m2.height = 100, 30
	m2.tmuxClient = tmux.Client{Socket: socket}
	m2.sessions = m.sessions
	m2.selected = rowCursor(0)
	_, cmd = m2.Update(previewTick(time.Now()))
	cmds = previewTickCmds(t, cmd)
	cmds[1]()
	wantEvents(t, rec, "resize:sess-discountfit")
}
