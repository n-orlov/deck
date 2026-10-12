package agent

import (
	"reflect"
	"strings"
	"testing"
)

// nameArgvAdapter drives one adapter's Launch with a name and extra args.
type nameArgvAdapter struct {
	kind   string
	launch func(name string, extra []string) ([]string, error)
	resume func(name string, extra []string) ([]string, error) // nil: Resume never carries a name
	// flagPrefix is how the adapter's argv spells the attached name element.
	flagPrefix string
}

func nameArgvAdapters() []nameArgvAdapter {
	return []nameArgvAdapter{
		{
			kind:       "claude",
			flagPrefix: "--name=",
			launch: func(name string, extra []string) ([]string, error) {
				return NewClaude().Launch(LaunchInput{CWD: "/w", ConversationID: nameTestUUID, Profile: "plan", Name: name, ExtraArgs: extra})
			},
			resume: func(name string, extra []string) ([]string, error) {
				return NewClaude().Resume(ResumeInput{CWD: "/w", ConversationID: nameTestUUID, Profile: "plan", Name: name, ExtraArgs: extra})
			},
		},
		{
			kind:       "copilot",
			flagPrefix: "--name=",
			launch: func(name string, extra []string) ([]string, error) {
				return NewCopilot().Launch(LaunchInput{ConversationID: copilotTestID, Profile: "safe", Name: name, ExtraArgs: extra})
			},
			resume: func(_ string, extra []string) ([]string, error) {
				return NewCopilot().Resume(ResumeInput{ConversationID: copilotTestID, Profile: "safe", ExtraArgs: extra})
			},
		},
		{
			kind:       "pi",
			flagPrefix: "--name=",
			launch: func(name string, extra []string) ([]string, error) {
				return NewPi().Launch(LaunchInput{ConversationID: piNameTestID, Profile: "safe", Name: name, ExtraArgs: extra})
			},
			resume: func(name string, extra []string) ([]string, error) {
				return NewPi().Resume(ResumeInput{ConversationID: piNameTestID, Profile: "safe", Name: name, ExtraArgs: extra})
			},
		},
	}
}

// R268: a launch name that begins with `-` rides inside one attached element,
// so no argv element is the name on its own (where the CLI would read it as a
// flag such as `-rm`).
func TestClaudeLaunchNameBeginningWithDashIsAttached(t *testing.T) {
	assertDashNameAttached(t, nameArgvAdapters()[0])
}

func TestCopilotLaunchNameBeginningWithDashIsAttached(t *testing.T) {
	assertDashNameAttached(t, nameArgvAdapters()[1])
}

func TestPiLaunchNameBeginningWithDashIsAttached(t *testing.T) {
	assertDashNameAttached(t, nameArgvAdapters()[2])
}

func assertDashNameAttached(t *testing.T, a nameArgvAdapter) {
	t.Helper()
	calls := map[string]func(string, []string) ([]string, error){"launch": a.launch}
	if a.kind != "copilot" { // Copilot's Resume never carries a name (R259)
		calls["resume"] = a.resume
	}
	for what, call := range calls {
		argv, err := call("-rm", nil)
		if err != nil {
			t.Fatalf("%s %s: %v", a.kind, what, err)
		}
		for _, element := range argv {
			if element == "-rm" {
				t.Errorf("%s %s: argv element %q is the name on its own: %q", a.kind, what, element, argv)
			}
		}
		if !contains(argv, a.flagPrefix+"-rm") {
			t.Errorf("%s %s: argv = %q, want the single attached element %q", a.kind, what, argv, a.flagPrefix+"-rm")
		}
	}
}

func contains(argv []string, want string) bool {
	for _, a := range argv {
		if a == want {
			return true
		}
	}
	return false
}

// R268: Claude's launch name passes through the control-stripping sanitiser
// Pi and Copilot use: an embedded control character, escape sequence or newline
// never reaches the argv.
func TestClaudeLaunchNameStripsControlCharacters(t *testing.T) {
	for _, tc := range []struct{ what, name, want string }{
		{"bell", "my\x07work", "mywork"},
		{"newline", "my\nwork", "my work"},
		{"escape", "my\x1b[31mwork", "mywork"},
		{"del", "my\x7fwork", "mywork"},
		{"c1", "my\u0085work", "mywork"},
		{"zero-width", "my\u200bwork", "mywork"},
	} {
		what, name, want := tc.what, tc.name, tc.want
		for call, argv := range map[string]func() ([]string, error){
			"launch": func() ([]string, error) { return nameArgvAdapters()[0].launch(name, nil) },
			"resume": func() ([]string, error) { return nameArgvAdapters()[0].resume(name, nil) },
		} {
			got, err := argv()
			if err != nil {
				t.Fatalf("%s %s: %v", what, call, err)
			}
			if !contains(got, "--name="+want) {
				t.Errorf("%s %s: argv = %q, want element %q", what, call, got, "--name="+want)
			}
			for _, element := range got {
				if strings.ContainsAny(element, "\x07\x1b\x7f\n\u0085\u200b") {
					t.Errorf("%s %s: element %q still holds a control character", what, call, element)
				}
			}
		}
	}
	// A name that is nothing but control characters passes no name flag.
	got, err := NewClaude().Launch(LaunchInput{ConversationID: nameTestUUID, Profile: "plan", Name: "\x07\n\x1b[0m"})
	if err != nil || hasNameFlag(got) {
		t.Errorf("control-only name: argv = %q err = %v, want no name flag", got, err)
	}
}

// R268 table: SPEC §8 appends an allowed extra argument unchanged as one argv
// element (`-p foo` as two, `--x`, and an argument holding a newline) and
// refuses a name flag in any spelling, `-nFOO` included, with an error that
// names the flag and returns no argv.
func TestClaudeExtraArgsTable(t *testing.T) { assertExtraArgsTable(t, nameArgvAdapters()[0]) }

func TestCopilotExtraArgsTable(t *testing.T) { assertExtraArgsTable(t, nameArgvAdapters()[1]) }

func TestPiExtraArgsTable(t *testing.T) { assertExtraArgsTable(t, nameArgvAdapters()[2]) }

func assertExtraArgsTable(t *testing.T, a nameArgvAdapter) {
	t.Helper()
	for _, row := range []struct {
		what    string
		extra   []string
		refused string // the flag the error names; "" means the extras ride verbatim
	}{
		{"-p foo", []string{"-p", "foo"}, ""},
		{"--x", []string{"--x"}, ""},
		{"-nFOO", []string{"-nFOO"}, "-n"},
		{"embedded newline", []string{"--note", "one\ntwo"}, ""},
	} {
		for call, run := range map[string]func(string, []string) ([]string, error){"launch": a.launch, "resume": a.resume} {
			argv, err := run("", row.extra)
			if row.refused != "" {
				if err == nil || argv != nil || !strings.Contains(err.Error(), row.refused) {
					t.Errorf("%s %s %q: argv = %q err = %v, want nil argv and an error naming %s", a.kind, call, row.what, argv, err, row.refused)
				}
				continue
			}
			if err != nil {
				t.Errorf("%s %s %q: %v", a.kind, call, row.what, err)
				continue
			}
			if tail := argv[len(argv)-len(row.extra):]; !reflect.DeepEqual(tail, row.extra) {
				t.Errorf("%s %s %q: tail = %q, want the extra args verbatim %q", a.kind, call, row.what, tail, row.extra)
			}
		}
	}
}
