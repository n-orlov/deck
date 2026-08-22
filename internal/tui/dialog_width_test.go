package tui

import (
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
)

// TestDialogWidthClampedToViewport pins SPEC.md:1070's contract directly on
// dialogWidth (task 030): 80% of the viewport, clamped to [26, 80] columns.
// Covers a narrow terminal below the lower clamp (the take-the-full-
// viewport fallback), a viewport that lands exactly on the lower clamp, a
// mid-range viewport inside the open interval, and both a viewport that
// lands exactly on the upper clamp and one well past it — the "both clamp
// ends" the task's success criteria calls out by name, not just the middle.
func TestDialogWidthClampedToViewport(t *testing.T) {
	cases := []struct {
		name     string
		viewport int
		want     int
	}{
		{"below lower clamp falls back to the full viewport", 20, 20},
		{"lands exactly on the lower clamp", 30, 26},
		{"mid-range viewport stays at 80% uncapped", 60, 48},
		{"lands exactly on the upper clamp", 100, 80},
		{"well past the upper clamp saturates at 80", 220, 80},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			model := New(nil, config.Settings{}, "")
			model.width, model.height = c.viewport, 30
			got := model.dialogWidth()
			if got != c.want {
				t.Fatalf("dialogWidth() at viewport %d = %d, want %d", c.viewport, got, c.want)
			}
		})
	}
}

// TestFramedDialogBoxWidthMatchesDialogWidth proves framedDialog actually
// renders at dialogWidth's fixed width — not grown to fit its widest
// content line — at a narrow, a middle and a wide terminal, so the box
// itself (not just the helper function in isolation) is asserted at every
// clamp band.
func TestFramedDialogBoxWidthMatchesDialogWidth(t *testing.T) {
	shortBody := "one\ntwo\n"
	for _, viewport := range []int{30, 60, 100, 220} {
		model := New(nil, config.Settings{}, "")
		model.width, model.height = viewport, 30
		wantWidth := model.dialogWidth()
		framed := model.framedDialog(shortBody)
		lines := strings.Split(framed, "\n")
		if len(lines) == 0 {
			t.Fatalf("viewport %d: framedDialog produced no lines", viewport)
		}
		gotWidth := stringWidth(lines[0])
		if gotWidth != wantWidth {
			t.Fatalf("viewport %d: framedDialog top border width = %d, want dialogWidth() = %d\nframed:\n%s", viewport, gotWidth, wantWidth, framed)
		}
		for i, line := range lines {
			if w := stringWidth(line); w != wantWidth {
				t.Fatalf("viewport %d: framedDialog line %d width = %d, want %d (every line must share the box width)\nline: %q", viewport, i, w, wantWidth, line)
			}
		}
	}
}

// TestFramedDialogWrapsOverlongContentInsteadOfGrowingOrTruncating proves
// the other half of task 030: a content line wider than the box's inner
// budget wraps at a word boundary onto additional lines rather than either
// growing the box to fit it (the pre-030 behaviour) or truncating the line
// and losing part of its text. Every word from the original sentence must
// still be present somewhere in the framed output, and the box must stay
// at dialogWidth's fixed width throughout.
func TestFramedDialogWrapsOverlongContentInsteadOfGrowingOrTruncating(t *testing.T) {
	long := "this sentence is deliberately much longer than any reasonable dialog box inner width so it is forced to wrap across more than one physical line without losing a single word of its own text"
	model := New(nil, config.Settings{}, "")
	model.width, model.height = 100, 30 // viewport 100 -> dialogWidth 80, inner 76
	wantWidth := model.dialogWidth()

	framed := model.framedDialog(long + "\n")
	lines := strings.Split(framed, "\n")

	if len(lines) < 5 {
		t.Fatalf("expected the long sentence to wrap across multiple content lines (plus top/bottom border), got %d lines:\n%s", len(lines), framed)
	}
	for i, line := range lines {
		if w := stringWidth(line); w != wantWidth {
			t.Fatalf("line %d width = %d, want fixed dialogWidth() = %d (box must not grow to fit content): %q", i, w, wantWidth, line)
		}
	}
	joined := strings.Join(lines, " ")
	for _, word := range strings.Fields(long) {
		if !strings.Contains(joined, word) {
			t.Fatalf("wrapped output lost word %q entirely (truncation instead of wrapping):\n%s", word, framed)
		}
	}
}
