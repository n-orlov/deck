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
func (throwawayAdapter) TranscriptPaths(TranscriptInput) (string, bool)       { return "", false }

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
func (f fakeAdapter) TranscriptPaths(TranscriptInput) (string, bool)       { return "", false }

// TestCaps_ExecutablePinnedToLaunchArgv0 pins R111's declared-executable
// requirement: the Executable each adapter reports from Capabilities() must
// never drift from what its own Launch actually puts in argv[0]. For claude
// and pi, Launch hardcodes a literal ("claude", "pi") as argv[0], so the
// declared value is checked against that literal directly. Shell has
// nothing to probe (its Capabilities().Executable is "" -- R111: "shell
// declares the empty executable: nothing to probe"): its own Launch argv[0]
// is the user's resolved $SHELL, not a fixed binary name, so there is no
// literal for the declared value to be pinned against; the assertion for
// shell is simply that the declared value is the empty string R111 calls
// for.
func TestCaps_ExecutablePinnedToLaunchArgv0(t *testing.T) {
	cases := []struct {
		name     string
		adapter  Adapter
		in       LaunchInput
		wantExec string
	}{
		{name: "shell", adapter: NewShell(), in: LaunchInput{}, wantExec: ""},
		{name: "claude", adapter: NewClaude(), in: LaunchInput{ConversationID: "conv-1", Profile: "safe"}, wantExec: "claude"},
		{name: "pi", adapter: NewPi(), in: LaunchInput{ConversationID: "conv-1", Profile: "safe"}, wantExec: "pi"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotExec := tc.adapter.Capabilities().Executable
			if gotExec != tc.wantExec {
				t.Fatalf("Capabilities().Executable = %q, want %q", gotExec, tc.wantExec)
			}

			argv, err := tc.adapter.Launch(tc.in)
			if err != nil {
				t.Fatalf("Launch() error = %v", err)
			}
			if len(argv) == 0 {
				t.Fatalf("Launch() returned empty argv")
			}

			if tc.wantExec == "" {
				// Nothing to pin for shell: its Launch argv[0] is the
				// user's resolved shell, never the empty string, so the
				// declared-empty case is exempted from the argv[0]
				// equality check by design (see doc comment above).
				return
			}
			if argv[0] != tc.wantExec {
				t.Fatalf("Launch() argv[0] = %q, want declared executable %q", argv[0], tc.wantExec)
			}
			if argv[0] != gotExec {
				t.Fatalf("Launch() argv[0] = %q, does not match Capabilities().Executable = %q", argv[0], gotExec)
			}
		})
	}
}
