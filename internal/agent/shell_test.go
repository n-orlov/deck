package agent

import (
	"os"
	"testing"
)

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

func TestShellLaunchArgvIsShellOnly(t *testing.T) {
	old, hadOld := os.LookupEnv("SHELL")
	os.Setenv("SHELL", "/bin/zsh")
	defer func() {
		if hadOld {
			os.Setenv("SHELL", old)
		} else {
			os.Unsetenv("SHELL")
		}
	}()

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
	want := []string{"/bin/zsh", "-x"}
	if !equalArgv(argv, want) {
		t.Fatalf("Launch argv = %v, want %v", argv, want)
	}
}

func TestShellResumeArgvIsShellOnly(t *testing.T) {
	old, hadOld := os.LookupEnv("SHELL")
	os.Setenv("SHELL", "/bin/bash")
	defer func() {
		if hadOld {
			os.Setenv("SHELL", old)
		} else {
			os.Unsetenv("SHELL")
		}
	}()

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
	want := []string{"/bin/bash", "-l"}
	if !equalArgv(argv, want) {
		t.Fatalf("Resume argv = %v, want %v", argv, want)
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
