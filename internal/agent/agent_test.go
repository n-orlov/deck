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

// TestDeclaredExecutableEqualsLaunchArgv0 pins R111's declared-executable
// requirement for all three registered adapters -- shell, claude and pi --
// with no adapter exempted and nothing derived in between: the Executable
// each one reports from Capabilities() is compared directly against the
// first element of the argv its own Launch produces. claude and pi put their
// binary there; shell declares no executable (SPEC 5) and puts the empty
// string there, because a shell pane's binary is resolved by the launcher
// (internal/service's paneArgv/resolveUserShell), never by the adapter.
// Resume is held to the same equality: an adapter whose two argv builders
// disagreed would preflight one binary and exec another.
//
// $SHELL is deliberately set here: shell's argv[0] must stay equal to its
// (empty) declaration whatever the environment says, which is exactly what
// keeping shell resolution out of the adapter buys.
func TestDeclaredExecutableEqualsLaunchArgv0(t *testing.T) {
	t.Setenv("SHELL", "/bin/zsh")
	for _, tc := range []struct {
		name     string
		adapter  Adapter
		wantExec string
	}{
		{name: "shell", adapter: NewShell(), wantExec: ""},
		{name: "claude", adapter: NewClaude(), wantExec: "claude"},
		{name: "pi", adapter: NewPi(), wantExec: "pi"},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			declared := tc.adapter.Capabilities().Executable
			if declared != tc.wantExec {
				t.Fatalf("Capabilities().Executable = %q, want %q", declared, tc.wantExec)
			}
			in := LaunchInput{CWD: "/tmp/wherever", ConversationID: "conv-1", Profile: "safe", ExtraArgs: []string{"-x"}}
			launchArgv, err := tc.adapter.Launch(in)
			if err != nil {
				t.Fatalf("Launch() error = %v", err)
			}
			if len(launchArgv) == 0 {
				t.Fatalf("Launch() produced an argv with no first element; want its first element to be the declared executable %q", declared)
			}
			if launchArgv[0] != declared {
				t.Fatalf("Launch() argv[0] = %q, want declared Capabilities().Executable %q (full argv %q)", launchArgv[0], declared, launchArgv)
			}
			resumeArgv, err := tc.adapter.Resume(ResumeInput{CWD: in.CWD, ConversationID: in.ConversationID, Profile: in.Profile, ExtraArgs: in.ExtraArgs})
			if err != nil {
				t.Fatalf("Resume() error = %v", err)
			}
			if len(resumeArgv) == 0 {
				t.Fatalf("Resume() produced an argv with no first element; want its first element to be the declared executable %q", declared)
			}
			if resumeArgv[0] != declared {
				t.Fatalf("Resume() argv[0] = %q, want declared Capabilities().Executable %q (full argv %q)", resumeArgv[0], declared, resumeArgv)
			}
		})
	}
}

// TestRegisteredAdaptersDeclareTheirLaunchExecutable extends that same
// equality to every kind a registry holds, so a fourth adapter cannot be
// registered with a declaration its own Launch contradicts.
func TestRegisteredAdaptersDeclareTheirLaunchExecutable(t *testing.T) {
	r := NewRegistry()
	r.Register(NewShell())
	r.Register(NewClaude())
	r.Register(NewPi())
	for _, kind := range r.Kinds() {
		adapter, ok := r.Lookup(kind)
		if !ok {
			t.Fatalf("Lookup(%q) missing from the registry that listed it", kind)
		}
		argv, err := adapter.Launch(LaunchInput{CWD: "/tmp/wherever", ConversationID: "conv-1", Profile: "safe"})
		if err != nil {
			t.Fatalf("%s Launch() error = %v", kind, err)
		}
		if len(argv) == 0 || argv[0] != adapter.Capabilities().Executable {
			t.Fatalf("%s: Launch() argv = %q, want argv[0] to be the declared executable %q", kind, argv, adapter.Capabilities().Executable)
		}
	}
}
