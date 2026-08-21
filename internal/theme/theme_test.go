package theme

import (
	"sort"
	"testing"
)

// TestAllTokensMatchesSPEC pins §11.6's exact [colors] token list. If this
// fails, either the SPEC's token set changed (update AllTokens to match)
// or a token was invented/dropped in code (fix the code).
func TestAllTokensMatchesSPEC(t *testing.T) {
	want := []string{
		"background", "surface", "border", "border_focus",
		"selection", "selection_idle", "title", "text", "dimmed", "hint",
		"key", "accent", "group", "search_match", "badge", "badge_warn",
		"waiting", "running", "idle", "starting", "stopped", "error", "archived",
	}
	got := make([]string, len(AllTokens))
	for i, tok := range AllTokens {
		got[i] = string(tok)
	}
	if len(got) != len(want) {
		t.Fatalf("AllTokens has %d tokens, SPEC §11.6 has %d: got=%v want=%v", len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("AllTokens[%d] = %q, want %q (order per SPEC §11.6's [colors] table): got=%v want=%v", i, got[i], want[i], got, want)
		}
	}
}

// TestStatusTokensMatchSevenStatuses pins §11.6's "the seven status tokens
// are exactly the seven statuses in §7" rule.
func TestStatusTokensMatchSevenStatuses(t *testing.T) {
	wantStatuses := []string{"starting", "running", "waiting", "idle", "error", "stopped", "archived"}
	gotStatuses := make([]string, len(StatusTokens))
	for i, tok := range StatusTokens {
		gotStatuses[i] = string(tok)
	}
	sortedWant := append([]string{}, wantStatuses...)
	sortedGot := append([]string{}, gotStatuses...)
	sort.Strings(sortedWant)
	sort.Strings(sortedGot)
	if len(sortedGot) != len(sortedWant) {
		t.Fatalf("StatusTokens has %d tokens, §7 has %d statuses: got=%v want=%v", len(sortedGot), len(sortedWant), gotStatuses, wantStatuses)
	}
	for i := range sortedWant {
		if sortedGot[i] != sortedWant[i] {
			t.Fatalf("StatusTokens set mismatch: got=%v want=%v (set comparison)", gotStatuses, wantStatuses)
		}
	}
	// Every status token must also be a member of AllTokens (no separate
	// status vocabulary from the colour vocabulary).
	for _, tok := range StatusTokens {
		if !isKnownToken(tok) {
			t.Fatalf("status token %q is not in AllTokens", tok)
		}
	}
}

func TestBuiltinsHaveOneDarkOneLight(t *testing.T) {
	all := Builtins()
	if len(all) < 2 {
		t.Fatalf("expected at least 2 built-ins, got %d: %v", len(all), all)
	}
	var dark, light bool
	for _, th := range all {
		switch th.Appearance {
		case "dark":
			dark = true
		case "light":
			light = true
		default:
			t.Fatalf("built-in %q has unexpected appearance %q", th.Name, th.Appearance)
		}
	}
	if !dark {
		t.Fatal("no built-in with appearance \"dark\"")
	}
	if !light {
		t.Fatal("no built-in with appearance \"light\"")
	}
}

func TestBuiltinsAreComplete(t *testing.T) {
	for _, th := range Builtins() {
		if err := th.Validate(); err != nil {
			t.Fatalf("built-in %q: %v", th.Name, err)
		}
		for _, tok := range AllTokens {
			if _, ok := th.Colors[tok]; !ok {
				t.Fatalf("built-in %q missing token %q", th.Name, tok)
			}
		}
	}
}

func TestDefaultIsEmbedded(t *testing.T) {
	d := Default()
	if d.Name != DefaultName {
		t.Fatalf("Default().Name = %q, want %q", d.Name, DefaultName)
	}
}

func validThemeSource() string {
	src := "name = \"probe\"\nappearance = \"dark\"\n\n[colors]\n"
	for _, tok := range AllTokens {
		src += string(tok) + " = \"#123456\"\n"
	}
	return src
}

func TestParseRejectsMissingToken(t *testing.T) {
	// Drop the last token line from an otherwise-complete document.
	full := validThemeSource()
	// Remove the "archived" line.
	trimmed := full[:len(full)-len("archived = \"#123456\"\n")]
	_, err := Parse([]byte(trimmed), "test")
	if err == nil {
		t.Fatal("Parse accepted a theme missing a token")
	}
}

func TestParseRejectsUnknownToken(t *testing.T) {
	src := validThemeSource() + "invented_token = \"#000000\"\n"
	_, err := Parse([]byte(src), "test")
	if err == nil {
		t.Fatal("Parse accepted a theme with an unknown token")
	}
}

func TestParseRejectsUnparseableHex(t *testing.T) {
	src := "name = \"probe\"\nappearance = \"dark\"\n\n[colors]\n"
	for _, tok := range AllTokens {
		if tok == Background {
			src += "background = \"not-a-colour\"\n"
			continue
		}
		src += string(tok) + " = \"#123456\"\n"
	}
	_, err := Parse([]byte(src), "test")
	if err == nil {
		t.Fatal("Parse accepted a non-hex colour value")
	}
}

func TestParseRoundTrips(t *testing.T) {
	src := validThemeSource()
	th, err := Parse([]byte(src), "test")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if th.Name != "probe" {
		t.Fatalf("Name = %q, want probe", th.Name)
	}
	if th.Appearance != "dark" {
		t.Fatalf("Appearance = %q, want dark", th.Appearance)
	}
	for _, tok := range AllTokens {
		if th.Colors[tok] != "#123456" {
			t.Fatalf("Colors[%q] = %q, want #123456", tok, th.Colors[tok])
		}
	}
}
