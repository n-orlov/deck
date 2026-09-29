package features

import "testing"

// TestStripSidebarRowLeadSkipsStatusGlyphAndPinMarker pins the harness's
// reading of SPEC §11's line-1 order (gutter, status glyph, pin marker,
// name) in both glyph modes, so every row parser (sidebarRowBadges'
// callers, frameHasSelectedRowNamed) sees "<name> <badge run>".
func TestStripSidebarRowLeadSkipsStatusGlyphAndPinMarker(t *testing.T) {
	cases := map[string]string{
		"\u25d0 alpha running":        "alpha running",
		"\u25cb \u2726 alpha idle":    "alpha idle",
		"~ alpha running":             "alpha running",
		"o * alpha idle":              "alpha idle",
		"x beta error":                "beta error",
		"alpha idle":                  "alpha idle",
		"20725d ago [default]":        "20725d ago [default]",
		"\u25cf \u2726 w ! waiting":   "w ! waiting",
		"? * pinned-long-name-trunc…": "pinned-long-name-trunc…",
	}
	for in, want := range cases {
		if got := stripSidebarRowLead(in); got != want {
			t.Errorf("stripSidebarRowLead(%q) = %q, want %q", in, got, want)
		}
	}
	frame := "+ deck - sessions ---+----+\n| v default  (2)     | x  |\n|   ~ alpha running  |    |\n| > o * beta idle    | >  |\n"
	if !frameHasSelectedRowNamed(frame, "beta") {
		t.Fatalf("selected pinned row beta not found:\n%s", frame)
	}
	if frameHasSelectedRowNamed(frame, "alpha") {
		t.Fatalf("unselected row alpha reported selected:\n%s", frame)
	}
}
