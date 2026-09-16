package agent

import (
	"fmt"
	"path/filepath"
	"strings"
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

// codexHookEvents is the subscribed event set (SPEC §8.2's table), a
// deliberate 5 of the 12 codex hook events actually exist (verified,
// docs/reports/codex-cli-0.154.0-spike.md Q3a). The other seven are
// excluded on purpose, not by omission:
//   - PreToolUse, PostToolUse fire once per tool call and carry the
//     tool's whole input/output — SPEC §8.2 explicitly excludes them, the
//     same reason §8.1 never subscribes Claude's analogous pair.
//   - PreCompact, PostCompact, SubagentStart, SubagentStop have no SPEC
//     §8.2 status mapping at all: nothing deck's status model does with a
//     compaction or subagent boundary, so subscribing them would be a
//     hook deck receives and immediately discards.
//   - Interrupt fires when an interrupted turn is aborted — a transient,
//     mid-turn event with no status of its own; the eventual SessionEnd
//     or the probe fallback already covers the row once the process
//     actually stops.
var codexHookEvents = []string{
	"SessionStart",
	"UserPromptSubmit",
	"PermissionRequest",
	"Stop",
	"SessionEnd",
}

// codexHookCommand builds the constant `deck _hook` command string embedded
// in every codex inline hook. Per-session facts never go here (SPEC §8.2):
// codex's trust hash is a function of the command string itself, so a
// session-specific command would mean a fresh, unhashable/untrusted
// command every launch. Per-session facts arrive at the hook process via
// its payload and inherited environment instead (deck's own env, verified
// to reach the hook unchanged).
//
// A `type="command"` hook value is a shell COMMAND LINE, not an argv pair
// (the spike drove one carrying a trailing argument,
// docs/reports/codex-cli-0.154.0-spike.md Q1), so the executable is
// shell-quoted exactly the way Claude's own command hook quotes it
// (shellQuote, claude.go): an install path carrying a space, a single quote
// or a backslash still executes, and a path beginning with '-' cannot turn
// into an option. Quoting is a function of the path alone, so the command
// string stays constant per install — all codex's trust hash needs.
func codexHookCommand(deckExecutable string) string {
	return shellQuote(deckExecutable) + " _hook"
}

// codexHookOverride encodes the -c override VALUE (never the "-c" flag
// itself) for one codex hook event: a single hook group containing exactly
// one hook of type "command" (SPEC §8.2). This is a narrow, single-purpose
// TOML encoder, not a general one — it exists only to produce this one
// shape, and only escapes what a TOML basic string needs escaped
// (backslash, double quote) so a command string carrying either (or a
// literal space, which needs no escaping at all) still round-trips.
func codexHookOverride(event, command string) string {
	return "hooks." + event + `=[{hooks=[{type="command",command="` + codexTOMLEscapeString(command) + `"}]}]`
}

// codexTOMLEscapeString escapes s for use inside a TOML basic string
// ("..."): backslash and double-quote are the only bytes that are both
// possible in an absolute executable path and meaningful to a TOML
// string. Everything else, including spaces, passes through unescaped.
func codexTOMLEscapeString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}

// Instrument returns codex's inline hook injection (SPEC §8.2): one -c
// override per codexHookEvents entry, plus the trailing
// --dangerously-bypass-hook-trust every profile needs to make an untrusted
// inline hook run at all (SPEC §8.2's "trust" section; unconditional here
// because Instrument's argv never depends on in.Profile). Nothing is
// written to disk — every value returned is a plain string this function
// builds and hands back; $CODEX_HOME is never opened, let alone written.
func (Codex) Instrument(in LaunchInput) ([]string, map[string]string) {
	command := codexHookCommand(in.DeckExecutable)
	argv := make([]string, 0, len(codexHookEvents)*2+1)
	for _, event := range codexHookEvents {
		argv = append(argv, "-c", codexHookOverride(event, command))
	}
	argv = append(argv, "--dangerously-bypass-hook-trust")

	// Same absent-when-no-lease semantics as Claude's (task 013/claude.go):
	// omitted entirely when this launch holds no lease-minted generation
	// token, so a hook never sees an empty token it would have to
	// interpret.
	var env map[string]string
	if in.LaunchGeneration != "" {
		env = map[string]string{LaunchGenerationEnv: in.LaunchGeneration}
	}
	return argv, env
}

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
