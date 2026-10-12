package agent

import (
	"strings"
	"testing"
)

func countNameFlag(argv []string) int {
	n := 0
	for _, a := range argv {
		if strings.HasPrefix(a, "--name=") {
			n++
		}
	}
	return n
}

func nameValue(argv []string) string {
	for _, a := range argv {
		if v, ok := strings.CutPrefix(a, "--name="); ok {
			return v
		}
	}
	return ""
}

// TestClaudeNameArgvPerRowKind asserts the --name argv for each row kind: a
// user row carries `--name=<n>` exactly once on launch, on resume and on the
// fresh `--session-id` relaunch of R207; derived and harness rows carry none.
func TestClaudeNameArgvPerRowKind(t *testing.T) {
	c := NewClaude()
	const name = "my work"
	// The fresh relaunch is Launch on the same id, chosen when RelaunchFresh
	// positively knows the transcript is absent.
	home := t.TempDir()
	if !c.RelaunchFresh(TranscriptInput{Home: home, CWD: "/w", ConversationID: nameTestUUID}) {
		t.Fatal("RelaunchFresh = false for an absent transcript")
	}
	for _, kind := range []struct {
		row  string // the Name the service hands the adapter for that row kind
		want int
	}{{name, 1}, {"", 0}} {
		launch, err := c.Launch(LaunchInput{CWD: "/w", ConversationID: nameTestUUID, Profile: "plan", Name: kind.row, ExtraArgs: []string{"--model", "x"}})
		if err != nil {
			t.Fatal(err)
		}
		resume, err := c.Resume(ResumeInput{CWD: "/w", ConversationID: nameTestUUID, Profile: "plan", Name: kind.row, ExtraArgs: []string{"--model", "x"}})
		if err != nil {
			t.Fatal(err)
		}
		for what, argv := range map[string][]string{"user launch": launch, "user resume": resume, "fresh relaunch": launch} {
			if got := countNameFlag(argv); got != kind.want {
				t.Errorf("%s row=%q: --name count = %d, want %d: %v", what, kind.row, got, kind.want, argv)
			}
			if kind.want == 1 && nameValue(argv) != name {
				t.Errorf("%s: --name value = %q, want %q", what, nameValue(argv), name)
			}
			if kind.want == 0 && hasNameFlag(argv) {
				t.Errorf("%s: blank name still carries a name flag: %v", what, argv)
			}
		}
		if launch[1] != "--session-id" || resume[1] != "--resume" {
			t.Errorf("identity flags changed: %v %v", launch, resume)
		}
	}
}

// TestClaudeForbiddenNameLaunchArgs: -n, --name and --name=<x> in launch_args
// are refused on launch and resume with a reason naming the flag.
func TestClaudeForbiddenNameLaunchArgs(t *testing.T) {
	c := NewClaude()
	for _, tc := range []struct{ arg, flag string }{{"-n", "-n"}, {"--name", "--name"}, {"--name=x", "--name"}} {
		extra := []string{"--model", "x", tc.arg}
		argv, err := c.Launch(LaunchInput{ConversationID: nameTestUUID, Profile: "safe", ExtraArgs: extra})
		if err == nil || argv != nil || !strings.Contains(err.Error(), tc.flag) {
			t.Errorf("Launch %q: argv=%v err=%v, want nil argv and an error naming %s", tc.arg, argv, err, tc.flag)
		}
		argv, err = c.Resume(ResumeInput{ConversationID: nameTestUUID, Profile: "safe", ExtraArgs: extra})
		if err == nil || argv != nil || !strings.Contains(err.Error(), tc.flag) {
			t.Errorf("Resume %q: argv=%v err=%v, want nil argv and an error naming %s", tc.arg, argv, err, tc.flag)
		}
	}
	if _, err := c.Launch(LaunchInput{ConversationID: nameTestUUID, Profile: "safe", ExtraArgs: []string{"--model", "x", "--names-ish"}}); err != nil {
		t.Errorf("an unrelated flag was refused: %v", err)
	}
}
