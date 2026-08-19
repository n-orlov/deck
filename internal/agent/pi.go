package agent

import "fmt"

// piProfileFlags maps SPEC §5 permission profile names pi actually
// supports to the exact flag pi accepts. pi has no "plan" mode and no
// "safe" flag of its own — "safe" is simply the absence of --approve.
var piProfileFlags = map[string]string{
	"edits": "--approve",
	"yolo":  "--approve",
}

// piProfiles is the stable, declared list of profiles the pi adapter
// honestly supports (SPEC §5). "plan" is deliberately absent: pi has no
// plan mode, so requesting it must degrade rather than silently pretend.
var piProfiles = []string{"safe", "edits", "yolo"}

// Pi is the adapter for the pi coding agent. Unlike Claude, pi uses the
// same `--session-id <id>` flag for both launch and resume: the id is
// caller-assigned and pi creates the conversation if it does not already
// exist (SPEC §8 table).
type Pi struct{}

// NewPi returns the Pi adapter.
func NewPi() Pi { return Pi{} }

func (Pi) Kind() string { return "pi" }

func (Pi) Capabilities() Caps {
	return Caps{
		Profiles:              piProfiles,
		AssignsConversationID: true,
		Resumable:             true,
	}
}

func (p Pi) Launch(in LaunchInput) ([]string, error) {
	if in.ConversationID == "" {
		return nil, fmt.Errorf("pi: launch requires a caller-assigned conversation id")
	}
	argv := []string{"pi", "--session-id", in.ConversationID}
	if flag, ok := piProfileFlags[in.Profile]; ok {
		argv = append(argv, flag)
	} else if in.Profile != "safe" && in.Profile != "" {
		return nil, fmt.Errorf("pi: unsupported permission profile %q", in.Profile)
	}
	return append(argv, in.ExtraArgs...), nil
}

func (p Pi) Resume(in ResumeInput) ([]string, error) {
	if in.ConversationID == "" {
		return nil, fmt.Errorf("pi: resume requires a conversation id")
	}
	argv := []string{"pi", "--session-id", in.ConversationID}
	if flag, ok := piProfileFlags[in.Profile]; ok {
		argv = append(argv, flag)
	} else if in.Profile != "safe" && in.Profile != "" {
		return nil, fmt.Errorf("pi: unsupported permission profile %q", in.Profile)
	}
	return append(argv, in.ExtraArgs...), nil
}

// Instrument is empty until Pi has a verified event source (SPEC §8.1).
func (Pi) Instrument(LaunchInput) ([]string, map[string]string) { return nil, nil }
