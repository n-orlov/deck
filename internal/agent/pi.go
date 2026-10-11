package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

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

// Kind returns the registry name of the Pi adapter.
func (Pi) Kind() string { return "pi" }

// Capabilities declares Pi's profiles and conversation-id behaviour.
func (Pi) Capabilities() Caps {
	return Caps{
		Profiles:              piProfiles,
		AssignsConversationID: true,
		Resumable:             true,
		HasTranscript:         true,
		Executable:            "pi",
		Name:                  NameCaps{LaunchName: NameLaunchFlag, AutoTitle: false},
	}
}

// Launch returns the argv that starts Pi under in.Profile, then `--name <n>`
// for a row whose name deck owns (in.Name is non-blank only for `user` rows,
// R261), followed by in.ExtraArgs.
func (p Pi) Launch(in LaunchInput) ([]string, error) {
	if in.ConversationID == "" {
		return nil, fmt.Errorf("pi: launch requires a caller-assigned conversation id")
	}
	return piArgv(in.ConversationID, in.Profile, in.Name, in.ExtraArgs)
}

// Resume returns the argv that resumes the Pi session named by
// in.ConversationID under in.Profile, then `--name <n>` for a `user` row,
// followed by in.ExtraArgs.
func (p Pi) Resume(in ResumeInput) ([]string, error) {
	if in.ConversationID == "" {
		return nil, fmt.Errorf("pi: resume requires a conversation id")
	}
	return piArgv(in.ConversationID, in.Profile, in.Name, in.ExtraArgs)
}

// piArgv builds the argv Launch and Resume share: Pi reuses --session-id for
// both. -n/--name in extra is refused; a blank name passes no flag.
func piArgv(conversationID, profile, name string, extra []string) ([]string, error) {
	for _, arg := range extra {
		if bad := piForbiddenExtraFlag(arg); bad != "" {
			return nil, fmt.Errorf("pi: extra argument %q is refused: %s is not allowed in launch args (deck passes the session name itself)", arg, bad)
		}
	}
	argv := []string{"pi", "--session-id", conversationID}
	if flag, ok := piProfileFlags[profile]; ok {
		argv = append(argv, flag)
	} else if profile != "safe" && profile != "" {
		return nil, fmt.Errorf("pi: unsupported permission profile %q", profile)
	}
	if clean, ok := SanitisePushName("pi", name); ok {
		argv = append(argv, "--name", clean)
	}
	return append(argv, extra...), nil
}

// piForbiddenExtraFlag returns the name flag arg spells (`-n`, `--name` or
// `--name=<x>`), and "" when arg is allowed.
func piForbiddenExtraFlag(arg string) string {
	flag, _, _ := strings.Cut(arg, "=")
	if flag == "-n" || flag == "--name" {
		return flag
	}
	return ""
}

// Instrument installs deck's Pi extension (PiExtensionSource) with `-e` and
// hands it the launching deck binary's absolute path, verbatim, through
// PiHookExecutableEnv; the extension runs it with `_hook` as a separate argv
// element and no shell (SPEC §8.3). Like Claude and Codex
// it adds DECK_LAUNCH_GENERATION when the launch holds a lease. The extension
// file itself is written by the launcher (InstrumentFiles), never here.
func (Pi) Instrument(in LaunchInput) ([]string, map[string]string) {
	env := map[string]string{PiHookExecutableEnv: in.DeckExecutable}
	if in.LaunchGeneration != "" {
		env[LaunchGenerationEnv] = in.LaunchGeneration
	}
	return []string{"-e", PiExtensionPath(in.DeckHome)}, env
}

// Probe is Pi's sampled status source before its extension's first event and
// for the transitions the extension has no event for (SPEC §8.3).
func (Pi) Probe(pane string) (string, string) { return probe("pi", pane) }

// TranscriptPaths locates pi's on-disk transcript for a conversation,
// following exactly the convention recorded in
// docs/reports/phase3-findings.md's provenance section (established against
// the real, installed pi 0.84.1 binary and its compiled
// getDefaultSessionDirPath — see
// docs/reports/phase3-fake-pi-transcript-provenance.md for the capture):
// directory $HOME/.pi/agent/sessions/--<cwd with a single leading
// separator stripped, then every remaining "/", "\" or ":" replaced with
// "-">--, file "<timestamp>_<conversation id>.jsonl". Because the filename
// carries a creation timestamp a caller who only knows the id cannot
// predict, locating it means globbing the directory for the "_<id>.jsonl"
// suffix, mirroring cmd/fake-pi's findExistingTranscript exactly. It
// returns ok=false -- never an error -- when Home or ConversationID is
// empty, ConversationID is not a single safe path component (a separator,
// "." or ".."), the directory does not exist, or no entry matches: a missing HOME
// and "no matching file" both degrade to "cannot locate".
func (Pi) TranscriptPaths(in TranscriptInput) (string, bool) {
	if in.Home == "" || !safeConversationID(in.ConversationID) {
		return "", false
	}
	// Clean the cwd first so a trailing slash, a doubled slash or a "."/".."
	// element encodes to the same directory pi recorded the transcript
	// under (an empty cwd stays empty, not ".").
	cwd := in.CWD
	if cwd != "" {
		cwd = filepath.Clean(cwd)
	}
	dir := filepath.Join(in.Home, ".pi", "agent", "sessions", piEncodeCwd(cwd))
	entries, err := piReadDir(dir)
	if err != nil {
		return "", false
	}
	suffix := "_" + in.ConversationID + ".jsonl"
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), suffix) {
			return filepath.Join(dir, entry.Name()), true
		}
	}
	return "", false
}

// piReadDir is the directory read Pi's TranscriptPaths performs; a package
// variable so a test can observe that an unsafe id is declined before the
// transcript directory is touched at all.
var piReadDir = os.ReadDir

// piEncodeCwd reproduces pi's own encoding exactly (pi-mono's
// session-manager.ts getDefaultSessionDirPath, and cmd/fake-pi's own
// encodeCwd copy of it): strip a single leading "/" or "\\", then replace
// every remaining "/", "\\" or ":" with "-", and wrap the result in a
// literal "--" prefix/suffix.
func piEncodeCwd(cwd string) string {
	trimmed := cwd
	if len(trimmed) > 0 && (trimmed[0] == '/' || trimmed[0] == '\\') {
		trimmed = trimmed[1:]
	}
	replaced := strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == ':' {
			return '-'
		}
		return r
	}, trimmed)
	return "--" + replaced + "--"
}
