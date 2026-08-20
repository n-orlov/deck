package tui

import (
	"context"
	"errors"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// TestPreviewTickCapturesOnlyTheSelectedRow covers task 017 (SPEC
// requirements 21, 22): the preview capture engine issues exactly one call
// per previewTick, addressed at the row selected when the tick fired, and
// its result lands in the model keyed by that session's id.
func TestPreviewTickCapturesOnlyTheSelectedRow(t *testing.T) {
	var gotSlugs []string
	model := New(nil, config.Settings{}, "")
	model.sessions = []store.Session{
		{ID: "s1", Slug: "one", Name: "one", Status: "running"},
		{ID: "s2", Slug: "two", Name: "two", Status: "running"},
	}
	model.selected = 1
	model.previewCapture = func(ctx context.Context, slug string) (tmux.PreviewCapture, error) {
		gotSlugs = append(gotSlugs, slug)
		return tmux.PreviewCapture{Live: true, Bytes: []byte("frame-for-" + slug)}, nil
	}

	updated, cmd := model.Update(previewTick(time.Now()))
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("previewTick did not return a command")
	}
	// Two commands are batched (the reschedule and the capture), so
	// tea.Batch's compactCmds wraps them in a tea.BatchMsg rather than
	// returning either directly; run each sub-command exactly as the real
	// event loop would and feed any resulting message back into Update.
	batch, ok := cmd().(tea.BatchMsg)
	if !ok {
		t.Fatalf("previewTick command = %T, want tea.BatchMsg with the capture alongside the reschedule", cmd())
	}
	for _, sub := range batch {
		if sub == nil {
			continue
		}
		if msg := sub(); msg != nil {
			updated, _ = model.Update(msg)
			model = updated.(Model)
		}
	}

	if len(gotSlugs) != 1 || gotSlugs[0] != "two" {
		t.Fatalf("previewCapture calls = %v, want exactly one call for the selected row's slug %q", gotSlugs, "two")
	}
	if model.previewSessionID != "s2" || !model.previewLive || string(model.previewBytes) != "frame-for-two" {
		t.Fatalf("model preview state = (id=%q live=%v bytes=%q), want the selected row's capture",
			model.previewSessionID, model.previewLive, model.previewBytes)
	}
}

// TestPreviewTickWithoutEngineOnlyReschedules proves a model with no
// previewCapture wired (every constructor above New, and every existing
// test) behaves exactly as before this task: previewTick still reschedules
// itself and touches no preview state.
func TestPreviewTickWithoutEngineOnlyReschedules(t *testing.T) {
	model := New(nil, config.Settings{}, "")
	model.sessions = []store.Session{{ID: "s1", Slug: "one", Status: "running"}}

	updated, cmd := model.Update(previewTick(time.Now()))
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("previewTick did not return a reschedule command")
	}
	if model.previewSessionID != "" || model.previewLive || model.previewBytes != nil {
		t.Fatalf("previewTick with no engine wired mutated preview state: %#v", model)
	}
}

// TestPreviewCaptureIgnoresErrorsWithoutSurfacingAttachError proves a real
// tmux/transport failure (as opposed to a session with no live pane, which
// tmux.CapturePreview itself reports as Live: false with a nil error) does
// not disrupt the render: the previous frame is kept and the footer's
// attachError never fires on a preview tick, which would otherwise be noisy
// on every DECK_PREVIEW_MS interval for a merely-stopped session.
func TestPreviewCaptureIgnoresErrorsWithoutSurfacingAttachError(t *testing.T) {
	model := New(nil, config.Settings{}, "")
	model.sessions = []store.Session{{ID: "s1", Slug: "one", Status: "running"}}
	model.previewSessionID, model.previewLive, model.previewBytes = "s1", true, []byte("stale-but-good")
	model.previewCapture = func(context.Context, string) (tmux.PreviewCapture, error) {
		return tmux.PreviewCapture{}, errors.New("tmux transport failure")
	}

	cmd := model.capturePreview()
	if cmd == nil {
		t.Fatal("capturePreview returned nil with a session selected and an engine wired")
	}
	updated, _ := model.Update(cmd())
	model = updated.(Model)

	if model.attachError != "" {
		t.Fatalf("attachError = %q after a preview capture failure, want unset", model.attachError)
	}
	if model.previewSessionID != "s1" || !model.previewLive || string(model.previewBytes) != "stale-but-good" {
		t.Fatalf("preview state changed after a failed capture: %#v", model)
	}
}

// TestCapturePreviewNoSessionsReturnsNilCommand proves the engine issues no
// tmux call at all when there is nothing to preview (empty list, or the
// selection index out of range mid-transition), rather than capturing a
// stale or zero-value slug.
func TestCapturePreviewNoSessionsReturnsNilCommand(t *testing.T) {
	called := false
	model := New(nil, config.Settings{}, "")
	model.previewCapture = func(context.Context, string) (tmux.PreviewCapture, error) {
		called = true
		return tmux.PreviewCapture{}, nil
	}
	if cmd := model.capturePreview(); cmd != nil {
		cmd()
	}
	if called {
		t.Fatal("capturePreview invoked the engine with no sessions present")
	}
}
