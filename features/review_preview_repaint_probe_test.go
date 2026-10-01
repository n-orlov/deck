package features

import "testing"

// TestReviewPreviewRepaintCannotAcknowledgeNavigation is R169's reviewer
// probe (task 002, inventory mechanism M2): it exercises
// selectedSidebarLine -- the exact value sendNavKeySettled
// (navigation_settle_test.go) compares before and after a keystroke to
// decide whether that keystroke has been acknowledged -- directly against
// two side-by-side frames apiece, standing in for "before this keystroke's
// repaint" and "after some repaint happened". The two subtests pin the
// property R169 fixed: a change confined to the PREVIEW pane on the
// selected row's own screen line must never look like a sidebar-selection
// change, no matter how the preview's text happens to differ, while an
// actual sidebar-selection change (the "> " marker moving to a different
// row) must always be visible to the comparison.
//
// Before task 002's fix, selectedSidebarLine returned the frame's
// selected row's FULL line -- sidebar text and whatever the preview pane
// happened to render on that same screen row -- so a preview-only repaint
// (the preview re-rendering its own pane output a moment after a
// navigation key, with the sidebar selection never actually moving)
// changed the comparison value and made sendNavKeySettled falsely
// acknowledge a keystroke that never landed, letting a queued follow-up
// key race ahead onto whatever row the selection was still sitting on
// (inventory mechanism M2: a "g" acknowledged by a row repaint, with the
// follow-up key then landing on a group header). This probe demonstrates
// that defect directly: see
// /run/ralphd/artifacts/002/unfixed-preview-only.txt for this test's
// preview_only_change_is_not_acknowledged subtest run against an export
// of a192accf7d (pre-fix), where it fails.
//
// This same M2 mechanism is the inventoried root cause of two more
// scenarios that the inventory assigns to their own tasks rather than to
// 002 itself, because 002 only fixes the predicate -- each still needs its
// own re-verification that this fix actually clears its failure:
// agent_session.feature:33 (task 003, which also removes the dead
// a192accf7d retry) and dialogs.feature:143 (task 009:
// clientOpensProfileSwitchDialogForSession's navigateToRowByName sends
// "g" through this same sendNavKeySettled before sending "i" to open the
// profile dialog; a false ack on "g" lets "i" land on the group header
// instead, and the dialog never opens -- see
// /run/ralphd/artifacts/009/probe-unfixed-a192accf7d.log for this test
// run against the same a192accf7d export, and
// /run/ralphd/artifacts/009/dialogs-20x-{normal,race}.log for
// dialogs.feature:143 itself, 20/20 both ways at HEAD).
//
// The same-row STATUS repaint half of M2 (the selected row's own status
// glyph or badges changing) is covered by
// TestSameRowStatusRepaintCannotAcknowledgeNavigation and
// TestSendNavKeySettledWaitsOutSameRowStatusRepaint (task 012).
func TestReviewPreviewRepaintCannotAcknowledgeNavigation(t *testing.T) {
	// sidebarFrame builds a minimal, but real-shaped, side-by-side frame:
	// a bordered sidebar panel on the left (one group header row, then
	// two session rows -- "alpha" and "beta", at most one carrying the
	// "> " selection marker) and a bordered preview panel on the right,
	// sharing one border per row (detectLayoutMode's "side-by-side"
	// shape, layout_modes_test.go), with previewText rendered on the
	// SAME screen row as the selected session so a preview repaint can
	// only ever be observed by a predicate that fails to scope itself to
	// the sidebar's own column.
	sidebarFrame := func(selected, previewText string) string {
		mark := func(name string) string {
			if name == selected {
				return "> " + name
			}
			return "  " + name
		}
		pad := func(s string, width int) string {
			for len(s) < width {
				s += " "
			}
			return s
		}
		return "+----------+-----------------+\n" +
			"| socket   | " + pad("header", 17) + "|\n" +
			"| " + pad(mark("alpha"), 8) + " | " + pad(previewText, 17) + "|\n" +
			"| " + pad(mark("beta"), 8) + " | " + pad(previewText, 17) + "|\n" +
			"+----------+-----------------+"
	}

	t.Run("selection_change_is_acknowledged", func(t *testing.T) {
		before := sidebarFrame("alpha", "preview line A")
		after := sidebarFrame("beta", "preview line A")

		got1 := selectedSidebarLine(before)
		got2 := selectedSidebarLine(after)
		if got1 == got2 {
			t.Fatalf("selectedSidebarLine did not change when the sidebar's own selection moved from %q to %q: both returned %q\nbefore:\n%s\nafter:\n%s",
				"alpha", "beta", got1, before, after)
		}
	})

	t.Run("preview_only_change_is_not_acknowledged", func(t *testing.T) {
		before := sidebarFrame("alpha", "preview line A")
		after := sidebarFrame("alpha", "preview line B")

		got1 := selectedSidebarLine(before)
		got2 := selectedSidebarLine(after)
		if got1 != got2 {
			t.Fatalf("selectedSidebarLine changed when only the PREVIEW pane's text changed on the selected row's own line (sidebar selection stayed on %q throughout): %q != %q\nbefore:\n%s\nafter:\n%s",
				"alpha", got1, got2, before, after)
		}
	})
}
