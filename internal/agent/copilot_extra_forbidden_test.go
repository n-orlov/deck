package agent

import "testing"

// TestCopilot_ExtraArgsRefuseForbiddenFlags guards R216: the alternate
// identity, remote and ACP launch modes and --yolo may not arrive through
// ExtraArgs, bare or as --flag=value, on either entrypoint, for any profile.
func TestCopilot_ExtraArgsRefuseForbiddenFlags(t *testing.T) {
	forbidden := []string{"--continue", "--resume", "--connect", "--remote", "--acp", "--yolo"}
	for _, profile := range copilotProfiles {
		for _, flag := range forbidden {
			for _, spelling := range []string{flag, flag + "=x"} {
				extra := []string{"--model", "gpt-5", spelling}
				argv, err := NewCopilot().Launch(LaunchInput{ConversationID: copilotTestID, Profile: profile, ExtraArgs: extra})
				if err == nil || argv != nil {
					t.Errorf("Launch profile=%s extra=%q: argv=%q err=%v, want nil argv and an error", profile, spelling, argv, err)
				}
				argv, err = NewCopilot().Resume(ResumeInput{ConversationID: copilotTestID, Profile: profile, ExtraArgs: extra})
				if err == nil || argv != nil {
					t.Errorf("Resume profile=%s extra=%q: argv=%q err=%v, want nil argv and an error", profile, spelling, argv, err)
				}
			}
		}
	}
}

// TestCopilot_ExtraArgsAllowedStayVerbatim: allowed extra args are still
// appended unchanged, Launch equals Resume, and a non-UUID id is still refused.
func TestCopilot_ExtraArgsAllowedStayVerbatim(t *testing.T) {
	extra := []string{"--model", "gpt-5", "--resume-ish=1"}
	for _, profile := range copilotProfiles {
		l, err := NewCopilot().Launch(LaunchInput{ConversationID: copilotTestID, Profile: profile, ExtraArgs: extra})
		if err != nil {
			t.Fatalf("Launch(%s): %v", profile, err)
		}
		r, err := NewCopilot().Resume(ResumeInput{ConversationID: copilotTestID, Profile: profile, ExtraArgs: extra})
		if err != nil {
			t.Fatalf("Resume(%s): %v", profile, err)
		}
		if len(l) < len(extra) || len(l) != len(r) {
			t.Fatalf("profile %s: launch %q resume %q", profile, l, r)
		}
		for i, a := range l {
			if r[i] != a {
				t.Errorf("profile %s: launch %q != resume %q", profile, l, r)
			}
		}
		tail := l[len(l)-len(extra):]
		for i := range extra {
			if tail[i] != extra[i] {
				t.Errorf("profile %s: tail %q, want %q", profile, tail, extra)
			}
		}
		if _, err := NewCopilot().Launch(LaunchInput{ConversationID: "not-a-uuid", Profile: profile, ExtraArgs: extra}); err == nil {
			t.Errorf("profile %s: non-UUID id accepted", profile)
		}
	}
}
