package theme

import "testing"

// newBuiltinNames are the four built-ins R211 adds, each one TOML file plus
// one registry entry and no per-theme code. The tests below are the
// per-theme data obligations SPEC §11.6 puts on every built-in.
var newBuiltinNames = []string{"gruvbox-dark", "solarized-dark", "amber", "high-contrast"}

// newBuiltinStatusQuantised pins, per new built-in, which ReferencePalette
// entry each of the seven §7 status tokens quantises to -- the sibling of
// matrix_status_quantization_test.go's table. The values were derived offline
// by nearest-neighbour Euclidean RGB distance against ReferencePalette, not by
// asking Quantize. gruvbox-dark, solarized-dark and amber are muted palettes
// and share reference slots on a 16-colour terminal (status is then carried by
// the glyph column alone, §11.6); high-contrast is built from the saturated
// reference primaries and keeps all seven apart.
var newBuiltinStatusQuantised = map[string]map[Token]string{
	"gruvbox-dark": {
		Waiting: "#cdcd00", Running: "#cdcd00", Idle: "#7f7f7f", Starting: "#7f7f7f",
		Stopped: "#7f7f7f", Error: "#ff0000", Archived: "#7f7f7f",
	},
	"solarized-dark": {
		Waiting: "#cdcd00", Running: "#cdcd00", Idle: "#00cdcd", Starting: "#00cdcd",
		Stopped: "#7f7f7f", Error: "#cd0000", Archived: "#5c5cff",
	},
	"amber": {
		Waiting: "#e5e5e5", Running: "#cdcd00", Idle: "#cdcd00", Starting: "#cd0000",
		Stopped: "#7f7f7f", Error: "#ff0000", Archived: "#7f7f7f",
	},
	"high-contrast": {
		Waiting: "#ffff00", Running: "#00ff00", Idle: "#00ffff", Starting: "#5c5cff",
		Stopped: "#ffffff", Error: "#ff0000", Archived: "#e5e5e5",
	},
}

// highContrastMinTextRatio is WCAG AAA for body text.
const highContrastMinTextRatio = 7.0

func TestNewBuiltinsResolveThroughRegistryWithoutFallback(t *testing.T) {
	for _, name := range newBuiltinNames {
		t.Run(name, func(t *testing.T) {
			th, ok := Builtin(name)
			if !ok {
				t.Fatalf("registry has no built-in %q", name)
			}
			if th.Name != name {
				t.Fatalf("Builtin(%q).Name = %q", name, th.Name)
			}
			if th.Appearance != "dark" {
				t.Errorf("%s appearance = %q, want dark", name, th.Appearance)
			}
			if err := th.Validate(); err != nil {
				t.Fatalf("%s does not validate: %v", name, err)
			}
			got, reason := Resolve(nil, nil, name)
			if reason != "" {
				t.Fatalf("Resolve(%q) fell back: %s", name, reason)
			}
			if got != th {
				t.Fatalf("Resolve(%q) returned %q, want the registry's own theme", name, got.Name)
			}
		})
	}
}

func TestNewBuiltinsClearTheContrastFloor(t *testing.T) {
	checks := append(contrastChecks(), sessionRowSurfaceChecks()...)
	checks = append(checks, dialogSurfaceChecks()...)
	checks = append(checks, dialogSelectionChecks()...)
	checks = append(checks, gutterBarChecks()...)
	for _, name := range newBuiltinNames {
		th, ok := Builtin(name)
		if !ok {
			t.Fatalf("registry has no built-in %q", name)
		}
		for _, chk := range checks {
			fg, _ := th.Color(chk.fg)
			bg, _ := th.Color(chk.bg)
			fgQ, _ := th.QuantizedColor(chk.fg)
			bgQ, _ := th.QuantizedColor(chk.bg)
			hexRatio, err := contrastRatio(fg, bg)
			if err != nil {
				t.Fatalf("%s %s: %v", name, chk.label, err)
			}
			quantRatio, err := contrastRatio(fgQ, bgQ)
			if err != nil {
				t.Fatalf("%s %s: %v", name, chk.label, err)
			}
			if hexRatio < minContrastRatio {
				t.Errorf("%s %s: hex contrast %.2f:1 < %.1f:1", name, chk.label, hexRatio, minContrastRatio)
			}
			if quantRatio < minContrastRatio {
				t.Errorf("%s %s: quantised contrast %.2f:1 < %.1f:1", name, chk.label, quantRatio, minContrastRatio)
			}
		}
	}
}

func TestNewBuiltinsStatusTokensQuantisationIsPinned(t *testing.T) {
	for _, name := range newBuiltinNames {
		t.Run(name, func(t *testing.T) {
			th, ok := Builtin(name)
			if !ok {
				t.Fatalf("registry has no built-in %q", name)
			}
			want := newBuiltinStatusQuantised[name]
			if len(want) != len(StatusTokens) {
				t.Fatalf("%s: table pins %d status tokens, StatusTokens has %d", name, len(want), len(StatusTokens))
			}
			distinct := map[string]bool{}
			for _, tok := range StatusTokens {
				got, err := th.QuantizedColor(tok)
				if err != nil {
					t.Fatalf("%s: QuantizedColor(%q): %v", name, tok, err)
				}
				hex, _ := th.Color(tok)
				t.Logf("%-9s authored %s -> quantised %s", tok, hex, got)
				if got != want[tok] {
					t.Errorf("%s %s: authored %s quantises to %s, pinned %s", name, tok, hex, got, want[tok])
				}
				distinct[got] = true
			}
			if name == "high-contrast" && len(distinct) != len(StatusTokens) {
				t.Errorf("high-contrast: seven status tokens quantise onto %d reference entries, want %d", len(distinct), len(StatusTokens))
			}
		})
	}
}

func TestHighContrastTextOnBackgroundClearsAAA(t *testing.T) {
	th, ok := Builtin("high-contrast")
	if !ok {
		t.Fatal("registry has no built-in high-contrast")
	}
	for _, q := range []bool{false, true} {
		fg, bg := "", ""
		var err error
		if q {
			fg, err = th.QuantizedColor(Text)
		} else {
			fg, err = th.Color(Text)
		}
		if err != nil {
			t.Fatal(err)
		}
		if q {
			bg, err = th.QuantizedColor(Background)
		} else {
			bg, err = th.Color(Background)
		}
		if err != nil {
			t.Fatal(err)
		}
		ratio, err := contrastRatio(fg, bg)
		if err != nil {
			t.Fatal(err)
		}
		if ratio < highContrastMinTextRatio {
			t.Errorf("high-contrast text %s on background %s (quantised=%v) = %.2f:1, want >= %.0f:1", fg, bg, q, ratio, highContrastMinTextRatio)
		}
	}
}
