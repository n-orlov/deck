package agent

import "fmt"

// claudeProfileFlags maps SPEC §5 permission profile names to the exact
// `--permission-mode` value Claude Code accepts. Only structured mode flags
// are ever used — never a `--dangerously-*` flag (SPEC §8 table, ~line 262).
var claudeProfileFlags = map[string]string{
	"safe":  "manual",
	"plan":  "plan",
	"edits": "acceptEdits",
	"yolo":  "bypassPermissions",
}

// claudeProfiles is the stable, declared list of profiles the Claude
// adapter supports, in SPEC §5 order.
var claudeProfiles = []string{"safe", "plan", "edits", "yolo"}

// Claude is the adapter for Claude Code. It assigns its own conversation id
// via `--session-id` (deck generates the UUID) and resumes by
// `--resume <uuid>` only — never `--continue` or any "most recent" form
// (SPEC R2).
type Claude struct{}

// NewClaude returns the Claude adapter.
func NewClaude() Claude { return Claude{} }

func (Claude) Kind() string { return "claude" }

func (Claude) Capabilities() Caps {
	return Caps{
		Profiles:              claudeProfiles,
		AssignsConversationID: true,
		Resumable:             true,
	}
}

func (c Claude) Launch(in LaunchInput) ([]string, error) {
	if in.ConversationID == "" {
		return nil, fmt.Errorf("claude: launch requires a caller-assigned conversation id")
	}
	flag, err := claudePermissionFlag(in.Profile)
	if err != nil {
		return nil, err
	}
	argv := []string{"claude", "--session-id", in.ConversationID, "--permission-mode", flag}
	return append(argv, in.ExtraArgs...), nil
}

func (c Claude) Resume(in ResumeInput) ([]string, error) {
	if in.ConversationID == "" {
		return nil, fmt.Errorf("claude: resume requires a conversation id")
	}
	flag, err := claudePermissionFlag(in.Profile)
	if err != nil {
		return nil, err
	}
	argv := []string{"claude", "--resume", in.ConversationID, "--permission-mode", flag}
	return append(argv, in.ExtraArgs...), nil
}

func claudePermissionFlag(profile string) (string, error) {
	flag, ok := claudeProfileFlags[profile]
	if !ok {
		return "", fmt.Errorf("claude: unsupported permission profile %q", profile)
	}
	return flag, nil
}
