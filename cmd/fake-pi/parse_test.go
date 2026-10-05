package main

import "testing"

func TestParseJoinsPositionalPromptWordsWithSingleSpaces(t *testing.T) {
	for name, tc := range map[string]struct {
		args []string
		want options
	}{
		"no prompt":             {nil, options{}},
		"one word":              {[]string{"hello"}, options{message: "hello"}},
		"several words":         {[]string{"write", "the", "tests"}, options{message: "write the tests"}},
		"after the separator":   {[]string{"--approve", "--", "--not-a-flag", "x"}, options{approve: true, message: "--not-a-flag x"}},
		"flags around a prompt": {[]string{"a", "--session-id", "s1", "b"}, options{sessionID: "s1", message: "a b"}},
	} {
		got, err := parse(tc.args)
		if err != nil || got != tc.want {
			t.Errorf("%s: parse(%q) = (%+v, %v), want %+v", name, tc.args, got, err, tc.want)
		}
	}
}

func TestParseRejectsAnUnknownFlagAndAMissingSessionIDValue(t *testing.T) {
	for name, args := range map[string][]string{
		"unknown flag":     {"--nope"},
		"missing value":    {"--session-id"},
		"unknown after ok": {"word", "-x"},
	} {
		if _, err := parse(args); err == nil {
			t.Errorf("%s: parse(%q) succeeded, want an error", name, args)
		}
	}
}
