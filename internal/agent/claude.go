package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// claudeProfileFlags maps SPEC §5 permission profile names to the exact
// `--permission-mode` value Claude Code accepts. Only structured mode flags
// are ever used — never a `--dangerously-*` flag (SPEC §8 table, ~line 262).
// "safe" carries no entry here (R116): Claude Code renamed this mode's own
// value from "default" to "manual" somewhere between 2.1.71 and 2.1.259, so
// naming either spelling in argv is only ever correct against one side of
// that version line. Omitting the flag entirely is the version-independent
// fix: Claude Code's own unflagged default already behaves as deck's
// "safe" profile, on either side of the rename. claudePermissionArgs below
// treats "safe" as a valid profile that simply contributes no flag.
var claudeProfileFlags = map[string]string{
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
		HasTranscript:         true,
		Executable:            "claude",
	}
}

// TranscriptPaths locates Claude's on-disk transcript for a conversation,
// following exactly the convention recorded in
// docs/reports/phase3-findings.md's provenance section (established against
// a real, authenticated Claude Code 2.1.237's own hook payload):
// $HOME/.claude/projects/<cwd, every path separator replaced with "-">/
// <conversation id>.jsonl. It returns ok=false -- never an error -- when
// Home or ConversationID is empty, or when the computed path does not
// exist: a missing HOME and "no matching file" both degrade to "cannot
// locate", exactly as cmd/fake-claude's transcriptPath already does for its
// fixture.
func (Claude) TranscriptPaths(in TranscriptInput) (string, bool) {
	if in.Home == "" || in.ConversationID == "" {
		return "", false
	}
	project := strings.ReplaceAll(in.CWD, string(filepath.Separator), "-")
	path := filepath.Join(in.Home, ".claude", "projects", project, in.ConversationID+".jsonl")
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return "", false
	}
	return path, true
}

func (c Claude) Launch(in LaunchInput) ([]string, error) {
	if in.ConversationID == "" {
		return nil, fmt.Errorf("claude: launch requires a caller-assigned conversation id")
	}
	permArgs, err := claudePermissionArgs(in.Profile)
	if err != nil {
		return nil, err
	}
	argv := append([]string{"claude", "--session-id", in.ConversationID}, permArgs...)
	return append(argv, in.ExtraArgs...), nil
}

func (c Claude) Resume(in ResumeInput) ([]string, error) {
	if in.ConversationID == "" {
		return nil, fmt.Errorf("claude: resume requires a conversation id")
	}
	permArgs, err := claudePermissionArgs(in.Profile)
	if err != nil {
		return nil, err
	}
	argv := append([]string{"claude", "--resume", in.ConversationID}, permArgs...)
	return append(argv, in.ExtraArgs...), nil
}

var claudeHookEvents = []string{
	"SessionStart",
	"UserPromptSubmit",
	"Notification",
	"Stop",
	"StopFailure",
	"SessionEnd",
}

type claudeHookSettings struct {
	Hooks map[string][]claudeHookGroup `json:"hooks"`
}

type claudeHookGroup struct {
	Hooks []claudeHook `json:"hooks"`
}

type claudeHook struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

// Instrument supplies Claude's per-process --settings JSON. Claude merges
// this settings source with user and project settings, so deck adds its hooks
// without reading or modifying either source. Marshal cannot fail for these
// concrete string-only structures.
//
// DECK_SESSION_ID and DECK_HOME are no longer set here (SPEC §6.1, R104):
// they moved to internal/service's own deck-owned session-context map,
// which every adapter now carries -- including pi and shell, which never
// had them before. Their VALUES are unchanged, since the new source reads
// the identical row facts (session id, DeckHome) this adapter used to be
// handed directly. What Claude keeps is what it alone owns: the --settings
// hook JSON and DECK_LAUNCH_GENERATION, whose absent-when-no-lease
// semantics (see below) are a Claude-specific hook-routing fact, not a
// session property, and so are not part of that shared map.
// Probe provides the sampled fallback used when Claude's live hook verdict is stale.
func (Claude) Probe(pane string) (string, string) { return probe("claude", pane) }

func (Claude) Instrument(in LaunchInput) ([]string, map[string]string) {
	command := shellQuote(in.DeckExecutable) + " _hook"
	hooks := make(map[string][]claudeHookGroup, len(claudeHookEvents))
	for _, event := range claudeHookEvents {
		hooks[event] = []claudeHookGroup{{Hooks: []claudeHook{{
			Type: "command", Command: command,
		}}}}
	}
	settings, _ := json.Marshal(claudeHookSettings{Hooks: hooks})
	// The launch generation lets every hook this pane's agent runs say WHICH
	// launch of that row it belongs to (issue #11, R74): the session id
	// alone cannot distinguish a hook from the pane deck just started from a
	// late hook from the pane it replaced. Omitted when the launch holds no
	// lease-minted token, so a hook never sees an empty token it would have
	// to interpret, and there is otherwise nothing left for Claude to
	// instrument into the environment at all.
	var env map[string]string
	if in.LaunchGeneration != "" {
		env = map[string]string{LaunchGenerationEnv: in.LaunchGeneration}
	}
	return []string{"--settings", string(settings)}, env
}

// shellQuote quotes one argv path for Claude's command-hook shell. Always
// quoting also prevents a path beginning with '-' from becoming an option.
func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

// claudePermissionArgs returns the `--permission-mode` argument pair to
// append for profile, or nil for "safe" (R116): "safe" is a supported
// profile (see claudeProfiles) that simply composes no --permission-mode
// flag at all, rather than naming a mode value whose own spelling Claude
// Code renamed from "default" to "manual" between 2.1.71 and 2.1.259 --
// omitting the flag is the fix that holds on either side of that rename.
func claudePermissionArgs(profile string) ([]string, error) {
	if profile == "safe" {
		return nil, nil
	}
	flag, ok := claudeProfileFlags[profile]
	if !ok {
		return nil, fmt.Errorf("claude: unsupported permission profile %q", profile)
	}
	return []string{"--permission-mode", flag}, nil
}
