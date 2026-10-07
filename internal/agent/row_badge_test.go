package agent

import "testing"

// R220.2: only copilot declares a row badge; claude, codex, pi and shell keep
// the empty value, so their session rows render exactly as before.
func TestOnlyCopilotDeclaresARowBadge(t *testing.T) {
	want := map[string]string{"copilot": "copilot", "claude": "", "codex": "", "pi": "", "shell": ""}
	adapters := []Adapter{NewCopilot(), NewClaude(), NewCodex(), NewPi(), NewShell()}
	for _, a := range adapters {
		if got := a.Capabilities().RowBadge; got != want[a.Kind()] {
			t.Errorf("%s RowBadge = %q, want %q", a.Kind(), got, want[a.Kind()])
		}
	}
}

func TestRegistryRequireReportsTheSharedUnknownKindError(t *testing.T) {
	r := NewRegistry()
	r.Register(NewCopilot())
	if a, err := r.Require("copilot"); err != nil || a.Kind() != "copilot" {
		t.Fatalf("Require(copilot) = %v, %v", a, err)
	}
	for _, kind := range []string{"ghost", "", "COPILOT"} {
		_, err := r.Require(kind)
		if err == nil || err.Error() != UnknownKindError(kind).Error() {
			t.Errorf("Require(%q) error = %v", kind, err)
		}
	}
	if got := UnknownKindError("ghost").Error(); got != `unknown agent kind "ghost"` {
		t.Fatalf("UnknownKindError = %q", got)
	}
}
