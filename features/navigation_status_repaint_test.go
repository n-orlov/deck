package features

import (
	"context"
	"strings"
	"testing"
	"time"
)

// statusRepaintFrame builds a side-by-side frame in the exact shape deck
// rendered in CI run 36758283998's failing dialogs.feature:214 attempt
// (a 34-column bordered sidebar, a seam, then the preview), with
// sidebarRows rendered one per screen row inside the sidebar and preview
// on the first row next to them.
func statusRepaintFrame(preview string, sidebarRows ...string) string {
	pad := func(s string, width int) string {
		n := len([]rune(s))
		if n >= width {
			return s
		}
		return s + strings.Repeat(" ", width-n)
	}
	var b strings.Builder
	b.WriteString("+ deck - sessions -----------------+---------------------------------------------------------------+\n")
	b.WriteString("| " + pad("socket: deck_test_27998_146", 33) + "| " + pad(preview, 62) + "|\n")
	for _, row := range sidebarRows {
		b.WriteString("| " + pad(row, 33) + "| " + pad("", 62) + "|\n")
	}
	b.WriteString("+----------------------------------+---------------------------------------------------------------+")
	return b.String()
}

// TestSameRowStatusRepaintCannotAcknowledgeNavigation is task 012's
// regression test for inventory mechanism M2's same-row case.
// sendNavKeySettled treats a change in selectedSidebarLine as the
// keystroke's acknowledgement. After task 002, that value was cropped to
// the sidebar column but still carried the selected row's status glyph and
// status badges. So the still-selected row's own status repaint
// (starting -> stopped in dialogs.feature:214, an unseen waiting row being
// acknowledged in status_attach.feature:20) acknowledged a "g" deck had
// not read yet. The follow-up key then landed on the group header once
// deck processed the "g".
//
// Each "repaint" case below keeps the same row selected and changes only
// what deck repaints for that row's status. selectedSidebarLine must not
// change. Each "moves" case changes which row is selected, so it must
// change.
func TestSameRowStatusRepaintCannotAcknowledgeNavigation(t *testing.T) {
	header := "v default  (2)"
	repaints := []struct {
		name          string
		before, after string
	}{
		{"ascii_starting_to_stopped", "> . dc-pin2 starting", "> # dc-pin2 stopped"},
		{"unicode_starting_to_stopped", "> \u25cc dc-pin2 starting", "> \u25a0 dc-pin2 stopped"},
		{"ascii_starting_to_waiting_unseen", "> . st-attach starting", "> ? st-attach ! waiting"},
		{"unicode_waiting_unseen_acknowledged", "> \u25cf st-attach \u25cf waiting", "> \u25d0 st-attach running"},
		{"quality_word_appears", "> ~ st-attach running", "> ? st-attach ! live waiting"},
		{"pinned_row_status_change", "> . * dc-pin2 starting", "> # * dc-pin2 stopped"},
		{"archived_badge_kept", "> o dc-arch idle [archived]", "> # dc-arch stopped [archived]"},
		{"status_word_cut_by_width", "> . dc-a-rather-long-name-x sta...", "> # dc-a-rather-long-name-x sto..."},
		{"name_cut_by_width", "> . dc-a-much-much-longer-row-name...", "> # dc-a-much-much-longer-row-name..."},
	}
	for _, tc := range repaints {
		t.Run("repaint_"+tc.name, func(t *testing.T) {
			before := statusRepaintFrame("Session is starting; no pane yet.", header, tc.before, "  just now")
			after := statusRepaintFrame("Session is starting; no pane yet.", header, tc.after, "  just now")
			got1, got2 := selectedSidebarLine(before), selectedSidebarLine(after)
			if got1 != got2 {
				t.Fatalf("selectedSidebarLine changed on a same-row status repaint (the selection never moved), so sendNavKeySettled would acknowledge a key deck has not read: %q != %q\nbefore:\n%s\nafter:\n%s", got1, got2, before, after)
			}
			if got1 == "" {
				t.Fatalf("selectedSidebarLine found no selected row in:\n%s", before)
			}
		})
	}

	moves := []struct {
		name          string
		before, after []string
	}{
		{
			// "g" from the selected session row to the group header: deck
			// renders a selected header in reverse video, with no "> "
			// gutter, so the row below loses its marker.
			"row_to_header",
			[]string{header, "> . dc-pin2 starting", "  just now"},
			[]string{header, "  . dc-pin2 starting", "  just now"},
		},
		{
			"row_to_header_with_status_repaint",
			[]string{header, "> . dc-pin2 starting", "  just now"},
			[]string{header, "  # dc-pin2 stopped", "  just now"},
		},
		{
			"row_to_next_row",
			[]string{header, "> . alpha starting", "  just now", "  . beta starting", "  just now"},
			[]string{header, "  . alpha starting", "  just now", "> . beta starting", "  just now"},
		},
		{
			// A scrolled sidebar can keep the marker on the same screen
			// line while the row under it changes.
			"same_line_different_row",
			[]string{header, "> . alpha starting", "  just now"},
			[]string{header, "> . beta starting", "  just now"},
		},
	}
	for _, tc := range moves {
		t.Run("moves_"+tc.name, func(t *testing.T) {
			before := statusRepaintFrame("preview", tc.before...)
			after := statusRepaintFrame("preview", tc.after...)
			if got1, got2 := selectedSidebarLine(before), selectedSidebarLine(after); got1 == got2 {
				t.Fatalf("selectedSidebarLine did not change when the selection moved: both %q\nbefore:\n%s\nafter:\n%s", got1, before, after)
			}
		})
	}
}

// TestSendNavKeySettledWaitsOutSameRowStatusRepaint drives the real
// sendNavKeySettled through a real ScreenDriver PTY. The program on the
// other end is a small sh script that behaves like deck in the M2 race:
// on reading the "g" byte it first repaints the selected row's status only
// (starting -> stopped, selection unchanged), the repaint that raced deck's
// key handling in CI. Only when the test releases it, by sending "x", does
// it paint the frame that shows "g" processed (the group header selected,
// the row's marker gone). The test releases it once the status repaint is
// on screen and sendNavKeySettled has either returned or had 50 ms to.
//
// sendNavKeySettled must not return before the processed frame. If it
// returns on the status repaint, the frame it hands back still shows the
// target row selected, and navigateToRowByName would send the follow-up
// key while deck's "g" is still pending.
//
// navKeySettleWindow is raised to 10 s for this test so the pass side does
// not depend on scheduling. The window only bounds a genuine no-op key,
// and this key is never a no-op.
func TestSendNavKeySettledWaitsOutSameRowStatusRepaint(t *testing.T) {
	saved := navKeySettleWindow
	navKeySettleWindow = 10 * time.Second
	t.Cleanup(func() { navKeySettleWindow = saved })

	header := "v default  (1)"
	toScreen := func(frame string) string {
		return "\x1b[H\x1b[2J" + strings.ReplaceAll(frame, "\n", "\r\n")
	}
	before := statusRepaintFrame("Session is starting; no pane yet.", header, "> . dc-pin2 starting", "  just now")
	repaint := statusRepaintFrame("Session is starting; no pane yet.", header, "> # dc-pin2 stopped", "  just now")
	processed := statusRepaintFrame("Select or create a session to preview it here.", header, "  # dc-pin2 stopped", "  just now")
	script := `stty raw -echo
printf '%s' "$NAV_FRAME_BEFORE"
dd bs=1 count=1 >/dev/null 2>&1
printf '%s' "$NAV_FRAME_REPAINT"
dd bs=1 count=1 >/dev/null 2>&1
printf '%s' "$NAV_FRAME_PROCESSED"
dd bs=1 count=1 >/dev/null 2>&1
`
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, err := StartScreenDriverWithArgs(ctx, "/bin/sh", []string{
		"NAV_FRAME_BEFORE=" + toScreen(before),
		"NAV_FRAME_REPAINT=" + toScreen(repaint),
		"NAV_FRAME_PROCESSED=" + toScreen(processed),
	}, []string{"-c", script}, terminalColumns, terminalRows)
	if err != nil {
		t.Fatalf("start sh in pty: %v", err)
	}
	defer func() {
		_ = client.Send("qqq")
		if err := client.Stop(3 * time.Second); err != nil {
			t.Errorf("script did not exit: %v", err)
		}
	}()
	if err := client.WaitForFrame(ctx, false, "> . dc-pin2 starting"); err != nil {
		t.Fatalf("initial frame: %v", err)
	}

	type result struct {
		frame string
		err   error
	}
	returned := make(chan result, 1)
	go func() {
		err := sendNavKeySettled(ctx, client, "g")
		returned <- result{client.Frame(false), err}
	}()

	// Poll the frame on a ticker rather than through client.updated,
	// which has a single consumer: sendNavKeySettled.
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for !strings.Contains(client.Frame(false), "> # dc-pin2 stopped") {
		select {
		case <-tick.C:
		case <-ctx.Done():
			t.Fatalf("status repaint never appeared:\n%s", client.Frame(false))
		}
	}

	var got result
	select {
	case got = <-returned:
	case <-time.After(50 * time.Millisecond):
		if err := client.Send("x"); err != nil {
			t.Fatalf("release processed frame: %v", err)
		}
		got = <-returned
	}
	if got.err != nil {
		t.Fatalf("sendNavKeySettled: %v", got.err)
	}
	if frameHasSelectedRowNamed(got.frame, "dc-pin2") || !strings.Contains(got.frame, "Select or create a session") {
		t.Fatalf("sendNavKeySettled returned on the selected row's own status repaint, before the frame showing \"g\" processed; a follow-up key sent now would race the pending \"g\"\nframe at return:\n%s", got.frame)
	}
}
