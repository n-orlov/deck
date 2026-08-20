package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
)

// previewFixture reads one of task 008's checked-in deterministic preview
// fixtures (internal/agent/testdata/preview), the same files
// features/fake_agent_size_test.go feeds through a real tmux pane, so this
// crop unit test exercises the exact bytes a live capture would contain.
func previewFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "agent", "testdata", "preview", name))
	if err != nil {
		t.Fatalf("read preview fixture %q: %v", name, err)
	}
	return data
}

// TestCropPreviewBottomLeftFitsWithoutGeometryLine covers the "a pane
// smaller than the panel is not stretched" half of SPEC requirement 23: a
// pane whose real geometry is no larger than the panel in either dimension
// renders exactly as captured, top-anchored with blank padding below, and
// carries no geometry line (there is no window onto a larger pane to name).
func TestCropPreviewBottomLeftFitsWithoutGeometryLine(t *testing.T) {
	m := New(nil, config.Settings{}, "")
	raw := previewFixture(t, "fitting.txt")
	lines := m.cropPreviewBottomLeft(raw, 40, 12, 40, 12)
	if len(lines) != 12 {
		t.Fatalf("len(lines) = %d, want 12", len(lines))
	}
	wantFirst := "PREVIEW FIXTURE: fitting"
	if got := strings.TrimRight(lines[0], " "); got != wantFirst {
		t.Fatalf("lines[0] = %q, want %q (padded)", got, wantFirst)
	}
	for i, line := range lines {
		if got := len([]rune(line)); got != 40 {
			t.Fatalf("lines[%d] has %d runes, want exactly 40 (panel border must land in the same column)", i, got)
		}
	}
	// The fixture is 6 lines; everything from row 6 on is blank padding,
	// never a stretched copy of the real content.
	for i := 6; i < 12; i++ {
		if strings.TrimSpace(lines[i]) != "" {
			t.Fatalf("lines[%d] = %q, want blank padding (pane must not be stretched to fill the panel)", i, lines[i])
		}
	}
}

// TestCropPreviewBottomLeftCropsOversizedPane covers the "cropped, never
// resized" half of SPEC requirement 23 using task 008's oversized (120x40)
// fixture against exactly the panel size SPEC's own example uses (45x22 of
// 120x40): the newest (bottom-most) rows are kept, each cropped to the
// panel's width from column one, and the real geometry is stated on the
// first line.
func TestCropPreviewBottomLeftCropsOversizedPane(t *testing.T) {
	m := New(nil, config.Settings{}, "")
	raw := previewFixture(t, "oversized.txt")
	const contentWidth, contentHeight = 45, 22
	const realWidth, realHeight = 120, 40
	lines := m.cropPreviewBottomLeft(raw, contentWidth, contentHeight, realWidth, realHeight)
	if len(lines) != contentHeight {
		t.Fatalf("len(lines) = %d, want %d", len(lines), contentHeight)
	}
	wantGeometry := "45x22 of 120x40"
	if got := strings.TrimRight(lines[0], " "); got != wantGeometry {
		t.Fatalf("geometry line = %q, want %q", got, wantGeometry)
	}
	for i, line := range lines {
		if got := len([]rune(line)); got != contentWidth {
			t.Fatalf("lines[%d] has %d runes, want exactly %d (panel border must land in the same column)", i, got, contentWidth)
		}
	}
	// avail = contentHeight-1 (21) rows of real content follow the geometry
	// line; the fixture's rows are labelled L002..L040 (39 of them) after
	// its title row, 40 rows total, so the newest 21 rows are L020..L040 --
	// never the oldest ones, and never the title row.
	rawRows := strings.Split(strings.TrimRight(string(raw), "\r\n"), "\r\n")
	if len(rawRows) != 40 {
		t.Fatalf("fixture has %d rows, want 40 (fixture drifted out from under this test)", len(rawRows))
	}
	wantOldestVisible := rawRows[19] // 0-indexed: rawRows[19] is labelled L020
	wantNewest := rawRows[39]        // L040, the pane's last row
	if !strings.HasPrefix(wantOldestVisible, "L020") || !strings.HasPrefix(wantNewest, "L040") {
		t.Fatalf("fixture row labelling assumption broke: rawRows[19]=%q rawRows[39]=%q", wantOldestVisible, wantNewest)
	}
	gotOldestVisible := lines[1]
	gotNewest := lines[contentHeight-1]
	if !strings.HasPrefix(gotOldestVisible, "L020") {
		t.Fatalf("lines[1] = %q, want to start with L020 (the oldest of the newest 21 rows)", gotOldestVisible)
	}
	if !strings.HasPrefix(gotNewest, "L040") {
		t.Fatalf("lines[%d] = %q, want to start with L040 (the pane's newest row)", contentHeight-1, gotNewest)
	}
	// Every content row (not the geometry line) was 120 columns wide in the
	// real pane and is now cropped to 45; its last visible column must be
	// the crop marker, not half of whatever real byte was there, and the
	// marker must always land in the same column so the panel border after
	// it never shears.
	marker := m.cropMarker()
	for i := 1; i < contentHeight; i++ {
		line := lines[i]
		r := []rune(line)
		if string(r[len(r)-1:]) != marker {
			t.Fatalf("lines[%d] last column = %q, want crop marker %q", i, string(r[len(r)-1:]), marker)
		}
		wantPrefix := string([]rune(rawRows[i+18])[:contentWidth-1])
		if gotPrefix := string(r[:len(r)-1]); gotPrefix != wantPrefix {
			t.Fatalf("lines[%d] prefix = %q, want %q (crop must start at column one)", i, gotPrefix, wantPrefix)
		}
	}
}

// TestCropPreviewBottomLeftClampsToShortHistory covers the case a live pane
// is genuinely shorter than declared (e.g. tmux trimmed trailing blank
// rows): the crop must never index past the captured content or panic, and
// still pads rather than stretching.
func TestCropPreviewBottomLeftClampsToShortHistory(t *testing.T) {
	m := New(nil, config.Settings{}, "")
	lines := m.cropPreviewBottomLeft([]byte("only one line"), 20, 5, 20, 5)
	if len(lines) != 5 {
		t.Fatalf("len(lines) = %d, want 5", len(lines))
	}
	if got := strings.TrimRight(lines[0], " "); got != "only one line" {
		t.Fatalf("lines[0] = %q, want %q", got, "only one line")
	}
	for i := 1; i < 5; i++ {
		if strings.TrimSpace(lines[i]) != "" {
			t.Fatalf("lines[%d] = %q, want blank", i, lines[i])
		}
	}
}

// TestCropPreviewBottomLeftZeroSize covers the degenerate case the preview
// panel's own floor check (task 021) will route around at runtime, but
// which this function must still handle without panicking.
func TestCropPreviewBottomLeftZeroSize(t *testing.T) {
	m := New(nil, config.Settings{}, "")
	if got := m.cropPreviewBottomLeft([]byte("x"), 0, 5, 10, 10); got != nil {
		t.Fatalf("cropPreviewBottomLeft with contentWidth=0 = %#v, want nil", got)
	}
	if got := m.cropPreviewBottomLeft([]byte("x"), 5, 0, 10, 10); got != nil {
		t.Fatalf("cropPreviewBottomLeft with contentHeight=0 = %#v, want nil", got)
	}
}
