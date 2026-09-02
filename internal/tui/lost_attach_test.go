package tui

import (
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
)

// TestLostAttachViewNamesTheSession proves task 116's dialog names the
// displaced session and says another client took over, rendered through
// the ordinary top-level View() dispatch (m.lostAttach true) rather than
// by calling lostAttachView directly, so the wiring into that dispatch is
// exercised too, not just the body builder.
func TestLostAttachViewNamesTheSession(t *testing.T) {
	m := New(nil, config.Settings{}, "")
	m.width, m.height = 80, 24
	m.lostAttach = true
	m.lostAttachSession = "alpha"

	got := m.View()
	if !strings.Contains(got, "alpha") {
		t.Fatalf("lost-attach view does not name the session %q:\n%s", "alpha", got)
	}
	if !strings.Contains(got, "Another client took over") {
		t.Fatalf("lost-attach view does not say another client took over:\n%s", got)
	}
}

// TestLostAttachSubmitDismisses proves \u21b5 (applyDialogContract's Submit)
// closes the dialog and clears the captured session name, matching SPEC
// \u00a711.9's "dismisses on \u21b5".
func TestLostAttachSubmitDismisses(t *testing.T) {
	m := New(nil, config.Settings{}, "")
	m.width, m.height = 80, 24
	m.lostAttach = true
	m.lostAttachSession = "alpha"

	got, _ := m.updateLostAttachView(key("enter"))
	next := got.(Model)
	if next.lostAttach {
		t.Fatal("enter did not dismiss the lost-attach dialog")
	}
	if next.lostAttachSession != "" {
		t.Fatalf("dismissing left a stale session name: %q", next.lostAttachSession)
	}
}

// TestLostAttachSwallowsOtherKeys proves the dialog swallows a key
// outside the \u00a711.4 contract (SPEC.md: "it swallows every key while it
// is up, because that is its point") rather than leaving it to fall
// through to the list underneath.
func TestLostAttachSwallowsOtherKeys(t *testing.T) {
	m := New(nil, config.Settings{}, "")
	m.width, m.height = 80, 24
	m.lostAttach = true
	m.lostAttachSession = "alpha"

	got, _ := m.updateLostAttachView(key("q"))
	next := got.(Model)
	if !next.lostAttach {
		t.Fatal("an ordinary key closed the lost-attach dialog; only enter should")
	}
}
