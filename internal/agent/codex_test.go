package agent

import "testing"

func TestCodex_Capabilities(t *testing.T) {
	caps := Codex{}.Capabilities()
	if caps.AssignsConversationID {
		t.Fatalf("codex Caps.AssignsConversationID = true, want false (codex mints its own id)")
	}
	if !caps.Resumable {
		t.Fatalf("codex Caps.Resumable = false, want true")
	}
	if !caps.HasTranscript {
		t.Fatalf("codex Caps.HasTranscript = false, want true")
	}
	if caps.Executable != "codex" {
		t.Fatalf("codex Caps.Executable = %q, want %q", caps.Executable, "codex")
	}
	for _, profile := range []string{"safe", "edits", "yolo"} {
		if !caps.SupportsProfile(profile) {
			t.Fatalf("codex Caps.SupportsProfile(%q) = false, want true", profile)
		}
	}
	if caps.SupportsProfile("plan") {
		t.Fatalf("codex Caps.SupportsProfile(%q) = true, want false — codex has no plan mode", "plan")
	}
	if len(caps.Profiles) != 3 {
		t.Fatalf("codex Caps.Profiles = %v, want exactly [safe edits yolo]", caps.Profiles)
	}
}

func TestCodex_LaunchArgv(t *testing.T) {
	cases := []struct {
		profile string
		want    []string
	}{
		{"safe", []string{"codex", "-a", "on-request", "-s", "workspace-write"}},
		{"edits", []string{"codex", "-a", "never", "-s", "workspace-write"}},
		{"yolo", []string{"codex", "-a", "never", "-s", "danger-full-access"}},
	}

	codex := Codex{}
	for _, tc := range cases {
		t.Run(tc.profile, func(t *testing.T) {
			argv, err := codex.Launch(LaunchInput{
				CWD:       "/tmp/proj",
				Profile:   tc.profile,
				ExtraArgs: []string{"--extra"},
			})
			if err != nil {
				t.Fatalf("Launch: %v", err)
			}
			want := append(append([]string{}, tc.want...), "--extra")
			if !equalArgv(argv, want) {
				t.Fatalf("Launch(%q) argv = %v, want %v", tc.profile, argv, want)
			}
			for _, a := range argv {
				if a == "resume" {
					t.Fatalf("Launch(%q) argv %v contains a resume/id argument, want none", tc.profile, argv)
				}
			}
		})
	}
}

func TestCodex_ResumeArgv(t *testing.T) {
	cases := []struct {
		profile string
		want    []string
	}{
		{"safe", []string{"codex", "resume", "conv-123", "-a", "on-request", "-s", "workspace-write"}},
		{"edits", []string{"codex", "resume", "conv-123", "-a", "never", "-s", "workspace-write"}},
		{"yolo", []string{"codex", "resume", "conv-123", "-a", "never", "-s", "danger-full-access"}},
	}

	codex := Codex{}
	for _, tc := range cases {
		t.Run(tc.profile, func(t *testing.T) {
			argv, err := codex.Resume(ResumeInput{
				CWD:            "/tmp/proj",
				ConversationID: "conv-123",
				Profile:        tc.profile,
				ExtraArgs:      []string{"--extra"},
			})
			if err != nil {
				t.Fatalf("Resume: %v", err)
			}
			want := append(append([]string{}, tc.want...), "--extra")
			if !equalArgv(argv, want) {
				t.Fatalf("Resume(%q) argv = %v, want %v", tc.profile, argv, want)
			}
		})
	}
}

func TestCodex_UnknownProfile(t *testing.T) {
	codex := Codex{}
	if _, err := codex.Launch(LaunchInput{Profile: "plan"}); err == nil {
		t.Fatalf("Launch with profile %q: want error, got nil", "plan")
	}
	if _, err := codex.Resume(ResumeInput{ConversationID: "conv-123", Profile: "bogus"}); err == nil {
		t.Fatalf("Resume with profile %q: want error, got nil", "bogus")
	}
}

func TestCodex_LaunchSucceedsWithEmptyConversationID(t *testing.T) {
	codex := Codex{}
	argv, err := codex.Launch(LaunchInput{CWD: "/tmp/proj", Profile: "safe", ConversationID: ""})
	if err != nil {
		t.Fatalf("Launch with empty ConversationID: want success, got error: %v", err)
	}
	for _, a := range argv {
		if a == "" {
			t.Fatalf("Launch argv %v contains an empty argument", argv)
		}
	}
}

func TestCodex_ResumeRefusesEmptyConversationID(t *testing.T) {
	codex := Codex{}
	if _, err := codex.Resume(ResumeInput{CWD: "/tmp/proj", Profile: "safe", ConversationID: ""}); err == nil {
		t.Fatalf("Resume with empty ConversationID: want error, got nil")
	}
}
