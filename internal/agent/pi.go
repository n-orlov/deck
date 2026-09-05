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

func (Pi) Kind() string { return "pi" }

func (Pi) Capabilities() Caps {
	return Caps{
		Profiles:              piProfiles,
		AssignsConversationID: true,
		Resumable:             true,
		HasTranscript:         true,
		Executable:            "pi",
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

// Probe is Pi's sampled status source until it has a verified event source.
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
// empty, the directory does not exist, or no entry matches: a missing HOME
// and "no matching file" both degrade to "cannot locate".
func (Pi) TranscriptPaths(in TranscriptInput) (string, bool) {
	if in.Home == "" || in.ConversationID == "" {
		return "", false
	}
	dir := filepath.Join(in.Home, ".pi", "agent", "sessions", piEncodeCwd(in.CWD))
	entries, err := os.ReadDir(dir)
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
