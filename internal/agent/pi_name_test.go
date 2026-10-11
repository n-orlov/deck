package agent

import (
	"strings"
	"testing"
)

const piNameTestID = "9b2d1c64-3e5f-4a6b-8c7d-0e1f2a3b4c5d"

func piNameFlagValues(argv []string) []string {
	var out []string
	for i, a := range argv {
		if a == "--name" && i+1 < len(argv) {
			out = append(out, argv[i+1])
		}
	}
	return out
}

// TestPiNameArgvPerRowKind (R261): a `user` row (Name set by the service) carries
// `--name <n>` once on launch and on resume; a `derived`/`harness` row (Name
// blank) carries none, so Pi keeps its own dummy name.
func TestPiNameArgvPerRowKind(t *testing.T) {
	p := NewPi()
	for _, tc := range []struct {
		row  string
		want []string
	}{{"my work", []string{"my work"}}, {"", nil}, {"   ", nil}} {
		launch, err := p.Launch(LaunchInput{ConversationID: piNameTestID, Profile: "edits", Name: tc.row, ExtraArgs: []string{"--model", "x"}})
		if err != nil {
			t.Fatal(err)
		}
		resume, err := p.Resume(ResumeInput{ConversationID: piNameTestID, Profile: "edits", Name: tc.row, ExtraArgs: []string{"--model", "x"}})
		if err != nil {
			t.Fatal(err)
		}
		for what, argv := range map[string][]string{"launch": launch, "resume": resume} {
			got := piNameFlagValues(argv)
			if strings.Join(got, "|") != strings.Join(tc.want, "|") {
				t.Errorf("%s row=%q: --name values = %q, want %q: %v", what, tc.row, got, tc.want, argv)
			}
			if tc.want == nil && strings.Contains(strings.Join(argv, " "), "--name") {
				t.Errorf("%s row=%q: blank name still carries a flag: %v", what, tc.row, argv)
			}
			if argv[1] != "--session-id" || argv[len(argv)-1] != "x" {
				t.Errorf("%s: identity or launch args moved: %v", what, argv)
			}
		}
	}
}

// TestPiNameRefusesNameLaunchArgs: -n, --name and --name=<x> in launch_args are
// refused on launch and resume with a reason naming the flag.
func TestPiNameRefusesNameLaunchArgs(t *testing.T) {
	p := NewPi()
	for _, tc := range []struct{ arg, flag string }{{"-n", "-n"}, {"--name", "--name"}, {"--name=x", "--name"}} {
		extra := []string{"--model", "x", tc.arg}
		argv, err := p.Launch(LaunchInput{ConversationID: piNameTestID, Profile: "safe", ExtraArgs: extra})
		if err == nil || argv != nil || !strings.Contains(err.Error(), tc.flag) {
			t.Errorf("Launch %q: argv=%v err=%v, want nil argv and an error naming %s", tc.arg, argv, err, tc.flag)
		}
		argv, err = p.Resume(ResumeInput{ConversationID: piNameTestID, Profile: "safe", ExtraArgs: extra})
		if err == nil || argv != nil || !strings.Contains(err.Error(), tc.flag) {
			t.Errorf("Resume %q: argv=%v err=%v, want nil argv and an error naming %s", tc.arg, argv, err, tc.flag)
		}
	}
}

// TestPiNameCapsDeclared: LaunchName=flag and AutoTitle=false.
func TestPiNameCapsDeclared(t *testing.T) {
	got := NewPi().Capabilities().Name
	if got.LaunchName != NameLaunchFlag || got.AutoTitle {
		t.Fatalf("Pi NameCaps = %#v, want LaunchName=flag, AutoTitle=false", got)
	}
}
