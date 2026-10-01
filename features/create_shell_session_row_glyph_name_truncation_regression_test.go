package features

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/vt"
)

// TestWaitForSettledSessionRowSettlesOnATruncatedName is task 012's own
// regression guard (inventory mechanism M1's row-identification instance,
// flagged while re-verifying task 008's fix: see panel_background_rectangle
// .feature:82/112/130, confirmed red at HEAD before this fix,
// /run/ralphd/artifacts/012/panel_background_rectangle/head-before-fix.log).
//
// Task 006 fixed waitForSettledSessionRow's first M1 race by accepting the
// status WORD on either side of a transient starting->running promotion;
// task 008 fixed the word being pushed off the row entirely by a name that
// fills the sidebar's whole content width, by reading the row's own
// leading status GLYPH instead. Both fixes still located the right ROW by
// requiring rest (the rendered text after the glyph) to start with the
// FULL rowName. panel_background_rectangle.feature's own fixture --
// "rec-bbb-selected-session-with-a-name-far-too-long-to-fit-in-the-sidebar
// -at-all" (79 characters) against a sidebar content width of 32 columns
// -- is long enough that padTrunc truncates the NAME itself, not just the
// trailing badge run (SPEC §11.3's fixed glyph/name/badge order means the
// glyph survives; the name does not): the rendered row reads
// "rec-bbb-selected-session-wi..." and `rest` can never contain rowName in
// full, so frameSidebarRowGlyph's old prefix check could never match this
// row, however long it waited -- not a race, every single run.
//
// This test scripts exactly that rendered shape against a synthetic
// ScreenDriver (no real deck binary) and proves frameSidebarRowGlyph now
// reads it as rowName's own truncation (longest common run immediately
// followed by padTrunc's ellipsis, nothing else) while a genuinely
// unrelated, short, non-truncated row that happens to share rowName's
// leading characters is still correctly rejected.
func TestWaitForSettledSessionRowSettlesOnATruncatedName(t *testing.T) {
	const (
		cols = int(terminalColumns)
		rows = int(terminalRows)
		name = "rec-bbb-selected-session-with-a-name-far-too-long-to-fit-in-the-sidebar-at-all"
		// truncated is exactly what padTrunc renders for name at this
		// fixture's sidebar width: a 28-character prefix plus a
		// 3-character ellipsis (panel_background_rectangle.feature's own
		// comment: "rec-bbb-selected-session-wi...", before the status
		// glyph takes its own two columns).
		truncated = "rec-bbb-selected-session-wi..."
	)
	d := &ScreenDriver{
		screen:  vt.NewEmulator(cols, rows),
		done:    make(chan struct{}),
		updated: make(chan struct{}, 1),
	}
	go d.drainScreenInput()

	frameBeforeSettle := strings.Join([]string{
		"│ Create shell session              │ $                                                     │",
		"│ Name: " + name[:20] + "   │                                                       │",
		"│ Agent: shell (left/right cycles)  │                                                       │",
	}, "\r\n")
	// frameAfterSettle's sidebar also carries an unrelated short row
	// ("rec-aaa") that happens to share rowName's own leading "rec-"
	// characters -- the fixture's own control, proving a coincidental
	// shared prefix on a row that is NOT rowName's truncation (it renders
	// its own full word, not an ellipsis) is still rejected.
	frameAfterSettle := strings.Join([]string{
		"│ ▾ default  (2)                    │ $                                                     │",
		"│   . rec-aaa starting              │                                                       │",
		"│   just now                        │                                                       │",
		fmt.Sprintf("│ > ~ %s │                                                       │", truncated),
		"│   just now                        │                                                       │",
	}, "\r\n")

	var written strings.Builder
	write := func(frame string) {
		raw := "\x1b[H\x1b[2J" + frame
		written.WriteString(raw)
		d.mu.Lock()
		_, _ = d.screen.Write([]byte(raw))
		d.mu.Unlock()
		select {
		case d.updated <- struct{}{}:
		default:
		}
	}

	write(frameBeforeSettle)

	type result struct {
		err error
	}
	done := make(chan result, 1)
	start := time.Now()
	go func() {
		done <- result{err: waitForSettledSessionRow(context.Background(), d, name)}
	}()

	time.Sleep(75 * time.Millisecond)
	select {
	case r := <-done:
		t.Fatalf("waitForSettledSessionRow returned (err=%v) before the settled frame was ever painted -- it must actually wait", r.err)
	default:
	}

	write(frameAfterSettle)

	// Fixture control: rowName must genuinely not appear verbatim in the
	// settled frame (the old prefix match's own failure mode), and the
	// unrelated rec-aaa row must not be mistaken for rowName's
	// truncation.
	settled := d.Frame(false)
	if strings.Contains(settled, name) {
		t.Fatal("scripted settled frame contains the full untruncated rowName -- fixture does not reproduce the name-truncation defect this test means to prove")
	}
	if glyph, ok := frameSidebarRowGlyph(settled, "rec-aaa"); ok && glyph != "." {
		t.Fatalf("frameSidebarRowGlyph(settled, %q) = (%q, %v), want its own \".\" (starting) glyph, not the long rowName's row's \"~\" -- names must not cross-match", "rec-aaa", glyph, ok)
	}
	if glyph, ok := frameSidebarRowGlyph(settled, name); !ok || glyph != "~" {
		t.Fatalf("frameSidebarRowGlyph(settled, name) = (%q, %v), want (\"~\", true) -- it must identify rowName's own truncated row, not the unrelated rec-aaa row", glyph, ok)
	}

	select {
	case r := <-done:
		if r.err != nil {
			t.Fatalf("waitForSettledSessionRow returned %v, want nil: the row's own leading status glyph settled even though the name itself was truncated past recognition", r.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("waitForSettledSessionRow never returned after the settled (name-truncated) frame was painted -- it is still relying on matching rowName in full")
	}
	if elapsed := time.Since(start); elapsed < 70*time.Millisecond {
		t.Fatalf("waitForSettledSessionRow returned after %s, suspiciously fast -- want it to have actually blocked until the second write landed", elapsed)
	}
	if !strings.Contains(written.String(), "rec-aaa") {
		t.Fatal("regression test's own control row never got painted -- fixture is wrong")
	}
}
