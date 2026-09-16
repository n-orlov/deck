package agent

import (
	"fmt"
	"path/filepath"
)

// codexProfileFlags maps SPEC §5 permission profile names to the exact
// `-a/--ask-for-approval` and `-s/--sandbox` flag pair codex-cli 0.154.0
// accepts (SPEC §5, verified against that release). Only these three
// entries ever exist here — no "plan" entry (codex has no plan mode) and
// no aliasing of "plan" onto "safe": an unsupported profile is the
// caller's job to resolve/degrade via Caps.ResolveProfile before Launch or
// Resume ever sees it (SPEC §5's "adapter capabilities are declared, not
// assumed"), never this adapter's job to fake.
//
// "safe" carries an explicit entry, unlike Claude's (R116): codex's own
// CLI default is user-configurable (`approval_policy`/`sandbox_mode` in
// `~/.codex/config.toml`), so a "safe" that composed no flags at all could
// silently inherit a permissive config. Naming the flags explicitly is
// what makes "safe" actually safe regardless of the user's own config.
var codexProfileFlags = map[string][]string{
	"safe":  {"-a", "on-request", "-s", "workspace-write"},
	"edits": {"-a", "never", "-s", "workspace-write"},
	"yolo":  {"-a", "never", "-s", "danger-full-access"},
}

// codexProfiles is the stable, declared list of profiles the Codex adapter
// honestly supports (SPEC §5): safe, edits and yolo — deliberately not
// plan.
var codexProfiles = []string{"safe", "edits", "yolo"}

// Codex is the adapter for the Codex CLI. Unlike Claude and Pi, Codex mints
// its own conversation id — deck never assigns one at launch, and adopts it
// later from the agent's own first hook (SPEC §8.2, task 014/017). Launch
// therefore never takes or emits an id argument at all, and Resume takes
// one only because resuming necessarily names an existing conversation.
type Codex struct{}

// NewCodex returns the Codex adapter.
func NewCodex() Codex { return Codex{} }

func (Codex) Kind() string { return "codex" }

func (Codex) Capabilities() Caps {
	return Caps{
		Profiles:              codexProfiles,
		AssignsConversationID: false,
		Resumable:             true,
		HasTranscript:         true,
		Executable:            "codex",
	}
}

// Launch returns `codex` plus the profile's -a/-s flags plus ExtraArgs. It
// never emits an id argument: codex mints its own conversation id, so
// in.ConversationID (always empty for this adapter — Caps declares
// AssignsConversationID false) is not consulted at all.
func (Codex) Launch(in LaunchInput) ([]string, error) {
	flags, ok := codexProfileFlags[in.Profile]
	if !ok {
		return nil, fmt.Errorf("codex: unsupported permission profile %q", in.Profile)
	}
	argv := append([]string{"codex"}, flags...)
	return append(argv, in.ExtraArgs...), nil
}

// Resume returns `codex resume <id>` plus the profile's -a/-s flags plus
// ExtraArgs. It refuses an empty id outright — codex has no "most recent"
// resume form (`resume --last` is banned, SPEC R2) and deck never guesses
// one.
func (Codex) Resume(in ResumeInput) ([]string, error) {
	if in.ConversationID == "" {
		return nil, fmt.Errorf("codex: resume requires a conversation id")
	}
	flags, ok := codexProfileFlags[in.Profile]
	if !ok {
		return nil, fmt.Errorf("codex: unsupported permission profile %q", in.Profile)
	}
	argv := append([]string{"codex", "resume", in.ConversationID}, flags...)
	return append(argv, in.ExtraArgs...), nil
}

// Instrument is left empty by this task: codex's inline hook injection
// (SPEC §8.2, `-c 'hooks...'` plus `--dangerously-bypass-hook-trust`) is
// task 017's own deliverable (R122).
func (Codex) Instrument(LaunchInput) ([]string, map[string]string) { return nil, nil }

// Probe is codex's sampled status source for the pre-hook window (SPEC
// §8.2) and as the fallback once hooks exist. No "codex" rules exist in
// probeRules yet — that corpus is task 019's own deliverable (R124) — so
// this always declines (empty status) until then.
func (Codex) Probe(pane string) (string, string) { return probe("codex", pane) }

// TranscriptPaths locates codex's on-disk transcript for a conversation,
// following exactly the convention SPEC §8.2 and the codex-cli 0.154.0
// spike recorded (docs/reports/codex-cli-0.154.0-spike.md):
// <codex home>/sessions/<yyyy>/<mm>/<dd>/rollout-<ISO>-<conversation
// id>.jsonl. Because the filename carries a creation timestamp a caller
// who only knows the id cannot predict, and the date directories are not
// derivable from the id either, locating it means globbing across both
// (mirroring how Pi's own TranscriptPaths globs its timestamped
// filenames).
//
// The codex home is in.CodexHome when the caller resolved one from the
// session's own §6.1 env layering (a session-level CODEX_HOME override),
// and in.Home + "/.codex" otherwise -- this adapter never reads the
// ambient environment itself; in.Home alone (no CodexHome) is exactly the
// case that must resolve to the default, not to this process's own
// $CODEX_HOME, which could belong to a different session entirely. It
// returns ok=false -- never an error -- when ConversationID is empty, both
// Home and CodexHome are empty, or nothing on disk matches: a miss is
// always "cannot locate", never a guess.
func (Codex) TranscriptPaths(in TranscriptInput) (string, bool) {
	if in.ConversationID == "" {
		return "", false
	}
	codexHome := in.CodexHome
	if codexHome == "" {
		if in.Home == "" {
			return "", false
		}
		codexHome = filepath.Join(in.Home, ".codex")
	}
	pattern := filepath.Join(codexHome, "sessions", "*", "*", "*", "rollout-*-"+in.ConversationID+".jsonl")
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		return "", false
	}
	return matches[0], true
}
