package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// copilotProfileFlags maps SPEC §5 permission profile names to the flags
// GitHub Copilot CLI accepts for them. Only these three entries exist: no
// "plan" entry (Copilot's plan and autopilot modes are not deck profiles) and
// no aliasing of an unsupported profile onto "safe" -- that resolution is the
// caller's, through Caps.ResolveProfile, before Launch or Resume sees it.
//
// "safe" carries no flag: Copilot's own default asks before every tool use.
var copilotProfileFlags = map[string][]string{
	"safe":  nil,
	"edits": {"--allow-tool=write"},
	"yolo":  {"--allow-all"},
}

// copilotProfiles is the stable, declared list of profiles the Copilot adapter
// honestly supports (SPEC §5).
var copilotProfiles = []string{"safe", "edits", "yolo"}

// copilotUUID is the only conversation id shape `copilot --session-id`
// accepts; Copilot exits 1 on any other.
var copilotUUID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// Copilot is the adapter for GitHub Copilot CLI. Deck assigns the
// conversation id (a UUID) at launch with `--session-id`, which creates the
// session when it is new and resumes it when it exists, so Resume is the same
// argv as Launch and the adapter needs no FreshRelauncher.
type Copilot struct{}

// NewCopilot returns the Copilot adapter.
func NewCopilot() Copilot { return Copilot{} }

// Kind returns the registry name of the Copilot adapter.
func (Copilot) Kind() string { return "copilot" }

// Capabilities declares what Copilot can honestly do: deck assigns its
// conversation id, it can be resumed and it writes a transcript under its home
// directory, which a session-level COPILOT_HOME override relocates.
func (Copilot) Capabilities() Caps {
	return Caps{
		Profiles:              copilotProfiles,
		AssignsConversationID: true,
		Resumable:             true,
		HasTranscript:         true,
		Executable:            "copilot",
		TranscriptEnvKeys:     []string{"COPILOT_HOME"},
		RowBadge:              "copilot",
	}
}

// Launch returns `copilot --session-id <id>` plus the profile's flags, then
// --no-auto-update, then ExtraArgs verbatim. A conversation id that is not a
// UUID is refused here, because Copilot would exit 1 on it and the failure
// would otherwise surface as a dead pane.
func (Copilot) Launch(in LaunchInput) ([]string, error) {
	return copilotArgv(in.ConversationID, in.Profile, in.ExtraArgs)
}

// Resume returns exactly the argv Launch returns for the same conversation id,
// profile and ExtraArgs. `--session-id` resumes an existing session, whereas
// `--resume` fails for one killed before its first message, so Resume never
// emits `--resume` (nor --continue, --connect, --remote or --acp).
func (Copilot) Resume(in ResumeInput) ([]string, error) {
	return copilotArgv(in.ConversationID, in.Profile, in.ExtraArgs)
}

func copilotArgv(conversationID, profile string, extra []string) ([]string, error) {
	flags, ok := copilotProfileFlags[profile]
	if !ok {
		return nil, fmt.Errorf("copilot: unsupported permission profile %q", profile)
	}
	if !copilotUUID.MatchString(conversationID) {
		return nil, fmt.Errorf("copilot: conversation id %q is not a UUID", conversationID)
	}
	argv := []string{"copilot", "--session-id", conversationID}
	argv = append(argv, flags...)
	argv = append(argv, "--no-auto-update")
	return append(argv, extra...), nil
}

// Instrument loads deck's static plugin directory (CopilotPluginDir) with
// --plugin-dir and names the deck binary its hooks run in DECK_EXE. Only the
// yolo profile also sets COPILOT_ALLOW_ALL=true, which suppresses the
// folder-trust screen and grants nothing yolo (--allow-all) does not already
// grant. The launch generation, when the launch holds one, rides along as for
// the other kinds. It is pure: the plugin files are written by the launcher
// (InstrumentFiles). The deck row identity reaches the hooks through the pane
// environment the launch already exports.
func (Copilot) Instrument(in LaunchInput) ([]string, map[string]string) {
	env := map[string]string{CopilotDeckExeEnv: in.DeckExecutable}
	if in.LaunchGeneration != "" {
		env[LaunchGenerationEnv] = in.LaunchGeneration
	}
	if in.Profile == "yolo" {
		env["COPILOT_ALLOW_ALL"] = "true"
	}
	return []string{"--plugin-dir", CopilotPluginDir(in.DeckHome)}, env
}

// CopilotElevatedReason is the profile-honesty reason stored when a session
// launched as safe or edits shows Copilot's Allow All footer.
const CopilotElevatedReason = "Copilot's own settings elevated the profile to Allow All; deck launched it with this profile"

// AuditProfile reports a safe or edits launch whose footer shows "Allow All"
// (Copilot's own user settings can turn it on, which deck cannot override). A
// yolo launch is already Allow All, so nothing is reported for it, and a
// pane with no footer makes no claim. Only the footer below the composer is
// read, so "Allow All" quoted in the transcript never counts.
func (Copilot) AuditProfile(profile, pane string) string {
	if profile != "safe" && profile != "edits" {
		return ""
	}
	for _, line := range copilotFooterLines(pane) {
		// "Allow All" can wrap at 80 columns, so the stable prefix is matched.
		if strings.Contains(line, "Interactive · Allow") {
			return CopilotElevatedReason
		}
	}
	return ""
}

// Probe classifies a pane with copilot's own rule table (copilotProbeRules).
func (Copilot) Probe(pane string) (string, string) { return probe("copilot", pane) }

// TranscriptPaths returns <root>/session-state/<id>/events.jsonl, where root is
// in.Env["COPILOT_HOME"] when the caller resolved a non-empty value for that
// declared key and in.Home + "/.copilot" otherwise. It never reads the ambient
// environment. ok=false -- never an error, never a guess -- when the id is not
// one safe path component, when neither a COPILOT_HOME nor a Home is known, or
// when the file does not exist.
func (Copilot) TranscriptPaths(in TranscriptInput) (string, bool) {
	if !safeConversationID(in.ConversationID) {
		return "", false
	}
	root := in.Env["COPILOT_HOME"]
	if root == "" {
		if in.Home == "" {
			return "", false
		}
		root = filepath.Join(in.Home, ".copilot")
	}
	path := filepath.Join(root, "session-state", in.ConversationID, "events.jsonl")
	if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
		return "", false
	}
	return path, true
}
