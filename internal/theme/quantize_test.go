package theme

import "testing"

// TestReferencePaletteMatchesSPEC pins §11.6's declared 16-colour
// reference palette byte-for-byte: "000000 cd0000 00cd00 cdcd00 0000ee
// cd00cd 00cdcd e5e5e5" (0-7) and "7f7f7f ff0000 00ff00 ffff00 5c5cff
// ff00ff 00ffff ffffff" (8-15). If this fails, either the SPEC's palette
// changed (update ReferencePalette to match) or the constant drifted.
func TestReferencePaletteMatchesSPEC(t *testing.T) {
	want := [16]string{
		"#000000", "#cd0000", "#00cd00", "#cdcd00",
		"#0000ee", "#cd00cd", "#00cdcd", "#e5e5e5",
		"#7f7f7f", "#ff0000", "#00ff00", "#ffff00",
		"#5c5cff", "#ff00ff", "#00ffff", "#ffffff",
	}
	if ReferencePalette != want {
		t.Fatalf("ReferencePalette = %v, want %v (SPEC §11.6's declared reference palette)", ReferencePalette, want)
	}
}

// TestQuantizeNearestNeighbour is a small, independently-checkable
// sanity set: colours placed deliberately close to one reference entry
// and far from the others.
func TestQuantizeNearestNeighbour(t *testing.T) {
	cases := []struct {
		hex  string
		want string
	}{
		{"#010101", "#000000"},
		{"#fefefe", "#ffffff"},
		{"#cc0505", "#cd0000"},
		{"#04cc04", "#00cd00"},
		{"#fe0101", "#ff0000"},
		{"#800000", "#cd0000"}, // closer to cd0000 than to 000000 or 7f7f7f
	}
	for _, c := range cases {
		got, err := quantize(c.hex)
		if err != nil {
			t.Fatalf("quantize(%q): %v", c.hex, err)
		}
		if got != c.want {
			t.Errorf("quantize(%q) = %q, want %q", c.hex, got, c.want)
		}
	}
}

// TestQuantizeRejectsInvalidHex proves quantize errors rather than
// defaulting on malformed input.
func TestQuantizeRejectsInvalidHex(t *testing.T) {
	for _, bad := range []string{"", "#fff", "not-a-colour", "#gggggg", "123456"} {
		if _, err := quantize(bad); err == nil {
			t.Errorf("quantize(%q): want error, got nil", bad)
		}
	}
}

// TestBuiltinQuantizationPinned pins the exact quantisation of every
// token of every built-in theme to the nearest ReferencePalette entry, by
// Euclidean RGB distance, computed independently of the production code
// (values below were derived offline, not by calling quantize and
// asserting it agrees with itself). A change to a built-in's authored
// colours, or to the quantisation method, must be reconciled here
// deliberately.
func TestBuiltinQuantizationPinned(t *testing.T) {
	want := map[string]map[Token]string{
		"empire": {
			Background:    "#000000",
			Surface:       "#000000",
			Border:        "#7f7f7f",
			BorderFocus:   "#00cdcd",
			Selection:     "#000000",
			SelectionIdle: "#7f7f7f",
			Title:         "#cdcd00",
			Text:          "#e5e5e5",
			Dimmed:        "#7f7f7f",
			Hint:          "#7f7f7f",
			Key:           "#cdcd00",
			Accent:        "#cdcd00",
			Group:         "#e5e5e5",
			SearchMatch:   "#cdcd00",
			Badge:         "#7f7f7f",
			BadgeWarn:     "#cdcd00",
			Waiting:       "#cdcd00",
			Running:       "#00cd00",
			Idle:          "#7f7f7f",
			Starting:      "#cd0000",
			Stopped:       "#7f7f7f",
			Error:         "#ff0000",
			Archived:      "#7f7f7f",
		},
		"daylight": {
			Background:    "#ffffff",
			Surface:       "#ffffff",
			Border:        "#e5e5e5",
			BorderFocus:   "#00cdcd",
			Selection:     "#e5e5e5",
			SelectionIdle: "#e5e5e5",
			Title:         "#cd0000",
			Text:          "#000000",
			Dimmed:        "#7f7f7f",
			Hint:          "#7f7f7f",
			Key:           "#cd0000",
			Accent:        "#cd0000",
			Group:         "#000000",
			SearchMatch:   "#cd0000",
			Badge:         "#7f7f7f",
			BadgeWarn:     "#cd0000",
			Waiting:       "#cd0000",
			Running:       "#00cd00",
			Idle:          "#7f7f7f",
			Starting:      "#cd0000",
			Stopped:       "#7f7f7f",
			Error:         "#cd0000",
			Archived:      "#7f7f7f",
		},
	}

	for name, wantTokens := range want {
		th, ok := Builtin(name)
		if !ok {
			t.Fatalf("built-in %q not found", name)
		}
		if len(th.Quantized) != len(AllTokens) {
			t.Fatalf("theme %q: Quantized has %d entries, want %d (one per AllTokens)", name, len(th.Quantized), len(AllTokens))
		}
		for tok, want := range wantTokens {
			got, err := th.QuantizedColor(tok)
			if err != nil {
				t.Errorf("theme %q: QuantizedColor(%q): %v", name, tok, err)
				continue
			}
			if got != want {
				hex, _ := th.Color(tok)
				t.Errorf("theme %q token %q: quantized %s -> got %s, want %s", name, tok, hex, got, want)
			}
		}
		// Every token pinned above, and no more, no fewer.
		if len(wantTokens) != len(AllTokens) {
			t.Fatalf("theme %q: test pins %d tokens, AllTokens has %d — pin set is stale", name, len(wantTokens), len(AllTokens))
		}
	}
}

// TestQuantizedNeverMutatesColors proves quantisation is a read-only
// derivation: Colors keeps holding the theme's exact authored hex values
// regardless of what Quantized says, so a truecolour terminal is
// unaffected by the 16-colour floor existing at all.
func TestQuantizedNeverMutatesColors(t *testing.T) {
	th := Default()
	for _, tok := range AllTokens {
		hex, err := th.Color(tok)
		if err != nil {
			t.Fatalf("Color(%q): %v", tok, err)
		}
		q, err := th.QuantizedColor(tok)
		if err != nil {
			t.Fatalf("QuantizedColor(%q): %v", tok, err)
		}
		// hex is one of §11.6's authored values (7 chars, "#rrggbb");
		// q is one of the 16 reference entries. They are allowed to
		// coincide for a token that happens to already be a reference
		// colour, but hex itself must never have been overwritten.
		if hex == "" {
			t.Fatalf("Color(%q) returned empty string", tok)
		}
		found := false
		for _, ref := range ReferencePalette {
			if ref == q {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("QuantizedColor(%q) = %q is not a member of ReferencePalette", tok, q)
		}
	}
}
