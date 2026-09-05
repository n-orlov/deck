package agent

import (
	"os"
	"reflect"
	"strings"
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

// pathLookupName is the executable name that PATH resolution would have to
// find for an argv[0] to exec, i.e. exactly what R111's declared Executable
// names for a probe. An argv[0] that contains a path separator (shell's
// resolved $SHELL, "/bin/sh") is used verbatim by exec and is never looked
// up on PATH, so its lookup name is the empty string -- which is precisely
// why R111 has shell declare the empty executable ("nothing to probe"). A
// bare name ("claude", "pi") is its own lookup name.
func pathLookupName(argv0 string) string {
	if strings.ContainsAny(argv0, `/\`) {
		return ""
	}
	return argv0
}

// TestCaps_ExecutablePinnedToLaunchArgv0 pins R111's declared-executable
// requirement for all three registered adapters: the Executable each one
// reports from Capabilities() must never drift from the executable its own
// Launch actually puts in argv[0], as PATH would have to resolve it
// (pathLookupName). claude and pi put a bare literal there, so the
// comparison is against "claude" and "pi" directly; shell puts the user's
// resolved $SHELL there, an absolute path that exec runs without consulting
// PATH, whose lookup name is the empty string shell declares. No adapter is
// exempted from the comparison: every case below runs it, shell included,
// under both an explicitly set absolute $SHELL and userShell()'s /bin/sh
// fallback.
func TestCaps_ExecutablePinnedToLaunchArgv0(t *testing.T) {
	cases := []struct {
		name     string
		adapter  Adapter
		in       LaunchInput
		wantExec string
		// shellEnv, when non-nil, is the $SHELL values to run this
		// adapter's comparison under: "" means unset, exercising
		// userShell()'s /bin/sh fallback.
		shellEnv []string
	}{
		{name: "shell", adapter: NewShell(), in: LaunchInput{}, wantExec: "", shellEnv: []string{"/bin/zsh", ""}},
		{name: "claude", adapter: NewClaude(), in: LaunchInput{ConversationID: "conv-1", Profile: "safe"}, wantExec: "claude"},
		{name: "pi", adapter: NewPi(), in: LaunchInput{ConversationID: "conv-1", Profile: "safe"}, wantExec: "pi"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotExec := tc.adapter.Capabilities().Executable
			if gotExec != tc.wantExec {
				t.Fatalf("Capabilities().Executable = %q, want %q", gotExec, tc.wantExec)
			}

			check := func(t *testing.T) {
				argv, err := tc.adapter.Launch(tc.in)
				if err != nil {
					t.Fatalf("Launch() error = %v", err)
				}
				if len(argv) == 0 {
					t.Fatalf("Launch() returned empty argv")
				}
				if got := pathLookupName(argv[0]); got != gotExec {
					t.Fatalf("Launch() argv[0] = %q resolves on PATH as %q, want declared Capabilities().Executable %q",
						argv[0], got, gotExec)
				}
			}

			if tc.shellEnv == nil {
				check(t)
				return
			}
			for _, sh := range tc.shellEnv {
				name := "SHELL=" + sh
				if sh == "" {
					name = "SHELL unset"
				}
				t.Run(name, func(t *testing.T) {
					t.Setenv("SHELL", sh)
					if sh == "" {
						if err := os.Unsetenv("SHELL"); err != nil {
							t.Fatalf("Unsetenv(SHELL) = %v", err)
						}
					}
					check(t)
				})
			}
		})
	}
}
