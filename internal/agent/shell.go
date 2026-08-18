package agent

import "os"

// Shell is the adapter for a plain interactive login shell. Unlike the
// agent adapters, shell has no notion of a conversation: it declares no
// permission profiles and does not accept a caller-assigned conversation
// id (SPEC §5/§8). Its launch and resume argv are simply the user's shell
// plus any extra launch_args, ignoring Profile and ConversationID
// entirely.
type Shell struct{}

// NewShell returns the Shell adapter.
func NewShell() Shell { return Shell{} }

func (Shell) Kind() string { return "shell" }

func (Shell) Capabilities() Caps {
	return Caps{
		Profiles:              nil,
		AssignsConversationID: false,
		Resumable:             false,
	}
}

// userShell returns the user's login shell, falling back to /bin/sh when
// $SHELL is unset.
func userShell() string {
	if s := os.Getenv("SHELL"); s != "" {
		return s
	}
	return "/bin/sh"
}

// Launch returns the user shell argv only, plus any ExtraArgs. It ignores
// Profile and ConversationID: shell has no notion of either.
func (Shell) Launch(in LaunchInput) ([]string, error) {
	argv := []string{userShell()}
	return append(argv, in.ExtraArgs...), nil
}

// Resume returns the user shell argv only, plus any ExtraArgs. Callers
// must not call this when Capabilities().Resumable is false; shell
// implements it anyway as a harmless pass-through equivalent to Launch.
func (Shell) Resume(in ResumeInput) ([]string, error) {
	argv := []string{userShell()}
	return append(argv, in.ExtraArgs...), nil
}
