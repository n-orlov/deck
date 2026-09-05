package agent

import "testing"

func TestShellCapsNoProfilesNoAssignedID(t *testing.T) {
	s := NewShell()
	if s.Kind() != "shell" {
		t.Fatalf("Kind() = %q, want shell", s.Kind())
	}
	caps := s.Capabilities()
	if len(caps.Profiles) != 0 {
		t.Fatalf("Profiles = %v, want none", caps.Profiles)
	}
	if caps.AssignsConversationID {
		t.Fatal("AssignsConversationID = true, want false")
	}
	if caps.Resumable {
		t.Fatal("Resumable = true, want false")
	}
}

// TestShellLaunchArgvIsTheEmptyExecutableAndArgs pins that shell's Launch
// names no binary of its own whatever $SHELL says: argv[0] is the empty
// executable it declares (SPEC 5) and the launcher fills that slot
// (internal/service's paneArgv), so there is no second copy of shell
// resolution to drift from the declaration (R111).
func TestShellLaunchArgvIsTheEmptyExecutableAndArgs(t *testing.T) {
	t.Setenv("SHELL", "/bin/zsh")

	s := NewShell()
	argv, err := s.Launch(LaunchInput{
		CWD:            "/tmp/whatever",
		ConversationID: "should-be-ignored",
		Profile:        "yolo",
		ExtraArgs:      []string{"-x"},
	})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	want := []string{"", "-x"}
	if !equalArgv(argv, want) {
		t.Fatalf("Launch argv = %q, want %q", argv, want)
	}
}

func TestShellInstrumentIsEmpty(t *testing.T) {
	argv, env := (Shell{}).Instrument(LaunchInput{
		DeckExecutable: "/absolute/deck",
		DeckSessionID:  "row-id",
		DeckHome:       "/deck-home",
	})
	if argv != nil || env != nil {
		t.Fatalf("shell Instrument = %#v, %#v; want no instrumentation", argv, env)
	}
}

func TestShellResumeArgvIsTheEmptyExecutableAndArgs(t *testing.T) {
	t.Setenv("SHELL", "/bin/bash")

	s := NewShell()
	argv, err := s.Resume(ResumeInput{
		CWD:            "/tmp/whatever",
		ConversationID: "should-be-ignored",
		Profile:        "plan",
		ExtraArgs:      []string{"-l"},
	})
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	want := []string{"", "-l"}
	if !equalArgv(argv, want) {
		t.Fatalf("Resume argv = %q, want %q", argv, want)
	}
}

func equalArgv(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
