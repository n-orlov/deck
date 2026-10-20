package agent

import (
	"strings"
	"testing"
)

// TestTranscriptRegionSharesLastContentLinesBoundary: the region ends where
// lastContentLine's footer chrome begins (the earlier of the last two
// separator lines), so footer text never enters it and the last line of the
// region is exactly lastContentLine.
func TestTranscriptRegionSharesLastContentLinesBoundary(t *testing.T) {
	rule := strings.Repeat("─", 30)
	pane := "one\ntwo  \n\n" + rule + "\n❯ \n" + rule + "\n~/p main ctx:9% opus\n"
	region := TranscriptRegion(pane)
	if region != "one\ntwo" {
		t.Fatalf("TranscriptRegion = %q, want the transcript above the two separators", region)
	}
	if got := lastContentLine(pane); got != "two" {
		t.Fatalf("lastContentLine = %q, want two", got)
	}
	footerChanged := strings.Replace(pane, "ctx:9%", "ctx:90%", 1)
	if TranscriptRegion(footerChanged) != region {
		t.Fatal("a footer-only change altered the transcript region")
	}
	// Fewer than two separators: nothing is footer, the whole pane is transcript.
	if got := TranscriptRegion("a\nb\n" + rule + "\nc\n"); got != "a\nb\n"+rule+"\nc" {
		t.Fatalf("one-separator pane region = %q", got)
	}
}
