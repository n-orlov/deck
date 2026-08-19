package agent

import (
	"reflect"
	"testing"
)

// throwawayAdapter is a minimal test-only adapter used to demonstrate that
// registering a new kind requires no change to this package's exported
// surface, let alone anything under internal/tui.
type throwawayAdapter struct{}

func (throwawayAdapter) Kind() string { return "throwaway" }

func (throwawayAdapter) Capabilities() Caps {
	return Caps{
		Profiles:              []string{"safe"},
		AssignsConversationID: true,
		Resumable:             true,
	}
}

func (throwawayAdapter) Launch(in LaunchInput) (argv []string, err error) {
	return []string{"throwaway", "--session-id", in.ConversationID}, nil
}

func (throwawayAdapter) Resume(in ResumeInput) (argv []string, err error) {
	return []string{"throwaway", "--resume", in.ConversationID}, nil
}
func (throwawayAdapter) Instrument(LaunchInput) ([]string, map[string]string) { return nil, nil }
func (throwawayAdapter) Probe(string) (string, string)                        { return "", "" }

func TestRegistry_RegisterAndLookup(t *testing.T) {
	r := NewRegistry()
	r.Register(throwawayAdapter{})

	a, ok := r.Lookup("throwaway")
	if !ok {
		t.Fatalf("Lookup(%q) not found after Register", "throwaway")
	}
	if a.Kind() != "throwaway" {
		t.Fatalf("Kind() = %q, want %q", a.Kind(), "throwaway")
	}

	if _, ok := r.Lookup("nonexistent"); ok {
		t.Fatalf("Lookup(%q) unexpectedly found", "nonexistent")
	}
}

func TestRegistry_KindsIsStableAndSorted(t *testing.T) {
	r := NewRegistry()
	r.Register(throwawayAdapter{})
	r.Register(fakeAdapter{kind: "zzz"})
	r.Register(fakeAdapter{kind: "aaa"})

	got := r.Kinds()
	want := []string{"aaa", "throwaway", "zzz"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Kinds() = %v, want %v", got, want)
	}

	// Calling again must yield the identical, stable order.
	got2 := r.Kinds()
	if !reflect.DeepEqual(got, got2) {
		t.Fatalf("Kinds() not stable across calls: %v vs %v", got, got2)
	}
}

func TestCaps_SupportsProfile(t *testing.T) {
	c := Caps{Profiles: []string{"safe", "edits"}}
	if !c.SupportsProfile("safe") {
		t.Errorf("SupportsProfile(safe) = false, want true")
	}
	if c.SupportsProfile("plan") {
		t.Errorf("SupportsProfile(plan) = true, want false")
	}

	var empty Caps
	if empty.SupportsProfile("safe") {
		t.Errorf("empty Caps.SupportsProfile(safe) = true, want false")
	}
}

// fakeAdapter is a second throwaway used only to exercise Kinds() ordering.
type fakeAdapter struct{ kind string }

func (f fakeAdapter) Kind() string       { return f.kind }
func (f fakeAdapter) Capabilities() Caps { return Caps{} }
func (f fakeAdapter) Launch(LaunchInput) (argv []string, err error) {
	return nil, nil
}
func (f fakeAdapter) Resume(ResumeInput) (argv []string, err error) {
	return nil, nil
}
func (f fakeAdapter) Instrument(LaunchInput) ([]string, map[string]string) { return nil, nil }
func (f fakeAdapter) Probe(string) (string, string)                        { return "", "" }
