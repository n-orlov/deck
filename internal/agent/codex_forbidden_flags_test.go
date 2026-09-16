package agent

import (
	"strings"
	"testing"
)

// TestCodex_NeverEmitsForbiddenFlags guards R121 (SPEC §5 / PRD
// prohibitions): no codex path — Launch, Resume or Instrument — may ever
// compose `--last`, `--full-auto` or
// `--dangerously-bypass-approvals-and-sandbox` into the argv it builds,
// for any of the three declared profiles (safe, edits, yolo). codex-cli's
// own `--full-auto` and `--dangerously-bypass-approvals-and-sandbox` are
// blanket permission-degrade shortcuts distinct from the explicit -a/-s
// pair this adapter composes (codexProfileFlags), and `--last` is the
// "most recent conversation" resume shortcut SPEC R2 bans outright (Resume
// already refuses an empty id and always takes an explicit one instead).
// Instrument is still a deliberate stub (task 013/017) but is exercised
// here too so this guard keeps holding once task 017 fills it in.
func TestCodex_NeverEmitsForbiddenFlags(t *testing.T) {
	forbidden := []string{
		"--last",
		"--full-auto",
		"--dangerously-bypass-approvals-and-sandbox",
	}

	assertClean := func(t *testing.T, label string, argv []string) {
		t.Helper()
		joined := strings.Join(argv, " ")
		for _, bad := range forbidden {
			for _, a := range argv {
				if a == bad {
					t.Fatalf("%s argv %v contains forbidden flag %q", label, argv, bad)
				}
			}
			if strings.Contains(joined, bad) {
				t.Fatalf("%s argv %v contains forbidden substring %q", label, argv, bad)
			}
		}
	}

	codex := Codex{}
	for _, profile := range codexProfiles {
		profile := profile
		t.Run(profile, func(t *testing.T) {
			launchArgv, err := codex.Launch(LaunchInput{CWD: "/tmp/proj", Profile: profile})
			if err != nil {
				t.Fatalf("Launch(%q): %v", profile, err)
			}
			assertClean(t, "Launch("+profile+")", launchArgv)

			resumeArgv, err := codex.Resume(ResumeInput{CWD: "/tmp/proj", ConversationID: "conv-123", Profile: profile})
			if err != nil {
				t.Fatalf("Resume(%q): %v", profile, err)
			}
			assertClean(t, "Resume("+profile+")", resumeArgv)

			instrumentArgv, _ := codex.Instrument(LaunchInput{CWD: "/tmp/proj", Profile: profile})
			assertClean(t, "Instrument("+profile+")", instrumentArgv)
		})
	}
}
