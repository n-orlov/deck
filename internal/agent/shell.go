package agent

// Shell is the adapter for a plain interactive login shell. Unlike the
// agent adapters, shell has no notion of a conversation: it declares no
// permission profiles and does not accept a caller-assigned conversation
// id (SPEC §5/§8). Its launch and resume argv are simply any extra
// launch_args behind an empty executable slot, ignoring Profile and
// ConversationID entirely.
//
// Shell declares no executable at all (SPEC §5: "`shell` declares no
// executable and is always offered: it is the floor, the one session kind a
// fresh host can always create"), and its argv[0] is therefore the empty
// string, never a binary: resolving which shell a pane runs ($SHELL,
// /bin/sh, or an embedded caller's override) belongs to the launcher, which
// already owned that one resolution for created shell panes. That is what
// keeps declaration and argv honest for every adapter without exception --
// an adapter's declared executable IS the first element of the argv it
// produces, "claude", "pi", and "" for shell, whose empty slot
// internal/service fills in one place for create and resume alike.
type Shell struct{}

// NewShell returns the Shell adapter.
func NewShell() Shell { return Shell{} }

func (Shell) Kind() string { return "shell" }

func (Shell) Capabilities() Caps {
	return Caps{
		Profiles:              nil,
		AssignsConversationID: false,
		Resumable:             false,
		HasTranscript:         false,
		Executable:            "",
	}
}

// Launch returns an argv whose first element is this adapter's declared
// executable -- the empty string, because shell declares none -- followed by
// any ExtraArgs. That leading slot is the launcher's to fill with the shell
// it resolves for the pane (see the type comment). It ignores Profile and
// ConversationID: shell has no notion of either.
func (Shell) Launch(in LaunchInput) ([]string, error) {
	return append([]string{""}, in.ExtraArgs...), nil
}

// Resume returns the same argv Launch does, empty executable slot included.
// Callers must not call this when Capabilities().Resumable is false; shell
// implements it anyway as a harmless pass-through equivalent to Launch.
func (Shell) Resume(in ResumeInput) ([]string, error) {
	return append([]string{""}, in.ExtraArgs...), nil
}

// Instrument deliberately returns nothing: a shell has no agent hook source.
func (Shell) Instrument(LaunchInput) ([]string, map[string]string) { return nil, nil }

// Probe always declines: shell pane text has no meaningful agent verdict.
func (Shell) Probe(string) (string, string) { return "", "" }

// TranscriptPaths always declines: a shell has no notion of a transcript at
// all (Capabilities().HasTranscript is false), so this is never a
// missing-file degradation, just an adapter that has nothing to look for.
func (Shell) TranscriptPaths(TranscriptInput) (string, bool) { return "", false }
