// unpin_refit_sites_instant_test.go is R411 cases 4-6: the three remaining
// sites where deck resizes a session's window outside the preview fit and the
// interactive entry/exit (resizeInteractiveToPreview, the unpin the passive
// fit's stand-down issues while a client is attached, and
// releaseForeignPreviewPin). Each takes its discount instant BEFORE its tmux
// call, so the repaint the call provokes falls inside the discount span
// (R410). The tmux the sites talk to is a real tmux behind a wrapper that
// stamps the moment a resize-window or a window-size unset reaches it.
package tui

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// resizeAndUnpinStampingTmux is resizeStampingTmux that also stamps a
// `set-option -u` (the unpin), and with attached set answers the
// session_attached read with 1 as if a client were attached.
func resizeAndUnpinStampingTmux(t *testing.T, attached bool) (string, func() []int64) {
	t.Helper()
	inner, stamps, _ := resizeStampingTmux(t)
	dir := t.TempDir()
	wrapper := filepath.Join(dir, "tmux")
	log := filepath.Join(filepath.Dir(inner), "resizes")
	body := "#!/bin/sh\nfor arg in \"$@\"; do\n\tcase \"$arg\" in\n\t-u) date +%s%N >> '" + log + "' ;;\n"
	if attached {
		body += "\t'#{session_attached}') echo 1; exit 0 ;;\n"
	}
	body += "\tesac\ndone\nexec '" + inner + "' \"$@\"\n"
	if err := os.WriteFile(wrapper, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	return wrapper, stamps
}

func TestUnpinAndRefitSitesDiscountFromBeforeTheCall(t *testing.T) {
	newModel := func(t *testing.T, name string, attached bool) (Model, *instantDiscounter, func() []int64, string, string) {
		t.Helper()
		socket := selectionTestSocket(name)
		slug := "urs" + name
		newQuietSelectionPane(t, socket, "deck_"+slug, 80, 24)
		binary, stamps := resizeAndUnpinStampingTmux(t, attached)
		rec := &instantDiscounter{}
		m := New(nil, config.Settings{Color: true}, "").WithOutputDiscounter(rec)
		m.width, m.height = 100, 30
		m.tmuxClient = tmux.Client{Binary: binary, Socket: socket}
		id := "sess-" + slug
		m.sessions = []store.Session{{ID: id, Slug: slug, Name: slug, Status: "waiting"}}
		m.selected = rowCursor(0)
		return m, rec, stamps, id, slug
	}

	t.Run("resize-to-preview", func(t *testing.T) {
		m, rec, stamps, id, _ := newModel(t, "refit", false)
		next, _ := m.enterInteractive()
		got := next.(Model)
		if !got.interactive {
			t.Fatalf("enterInteractive did not enter interactive mode: %q", got.attachError)
		}
		t.Cleanup(func() { got.exitInteractive() })
		got.width, got.height = 130, 36
		if w, h := got.previewContentSize(); w == got.interactiveWidth && h == got.interactiveHeight {
			t.Fatal("test setup: the preview box did not change, so no re-fit would be issued")
		}
		before := len(stamps())
		rec.reset()
		got.resizeInteractiveToPreview()
		requireInstantBeforeFirstResize(t, rec, id, stamps()[before:])
	})

	t.Run("standdown-unpin", func(t *testing.T) {
		m, rec, stamps, id, slug := newModel(t, "standdown", true)
		// The passive fit's own entry point: a client is attached, so the fit
		// stands down and issues only the unpin.
		msg := fitPreviewPane(m.tmuxClient, slug, id, 90, 20, rec)
		if done, ok := msg.(previewFitDone); !ok || !done.clientAttached {
			t.Fatalf("test setup: fit = %+v, want the attached-client stand-down", msg)
		}
		requireInstantBeforeFirstResize(t, rec, id, stamps())
	})

	t.Run("release-foreign-pin", func(t *testing.T) {
		m, rec, stamps, id, _ := newModel(t, "release", false)
		m.attach = func(context.Context, string) (*exec.Cmd, error) { return exec.Command("true"), nil }
		if _, cmd := m.attachSelected(); cmd == nil {
			t.Fatalf("test setup: attachSelected issued no hand-off: %q", m.attachError)
		}
		requireInstantBeforeFirstResize(t, rec, id, stamps())
	})
}
