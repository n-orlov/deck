package agent

import (
	"fmt"
	"path/filepath"
	"strings"
)

// CopilotDeckExeEnv names the pane environment variable that carries the deck
// binary the Copilot plugin's hook commands run. The plugin directory is
// static, so the per-launch binary arrives through this variable, exactly as
// the other kinds' hook commands name the launching binary (SPEC §8.1).
const CopilotDeckExeEnv = "DECK_EXE"

// CopilotHookEventEnv names the variable each plugin hook command sets, in
// front of `deck _hook`, to the Copilot event it was registered for. Most
// Copilot payloads carry no event name of their own.
const CopilotHookEventEnv = "DECK_HOOK_EVENT"

// CopilotPluginDir is deck's own Copilot plugin directory under the resolved
// data root deckHome. It is never under ~/.copilot: deck writes nothing into
// Copilot's own home.
func CopilotPluginDir(deckHome string) string {
	return filepath.Join(deckHome, "copilot", "plugin")
}

// CopilotHookEvents lists, in order, the six observational Copilot events the
// plugin subscribes to. permissionRequest and preToolUse are deliberately
// absent: those hooks can decide for the user, and deck only observes.
var CopilotHookEvents = []string{
	"userPromptSubmitted", "sessionStart", "notification", "agentStop", "errorOccurred", "sessionEnd",
}

// CopilotHookTimeoutSec is the timeout every plugin hook entry declares.
const CopilotHookTimeoutSec = 5

// CopilotPluginManifest is the constant plugin.json of the deck plugin.
const CopilotPluginManifest = `{"name":"deck-status","version":"0.0.1","description":"deck status hooks","hooks":"hooks.json"}
`

// copilotHookCommand is the bash command of one hooks.json entry. It prints
// nothing on stdout (a hook can speak to Copilot through it) and always exits
// 0, whether DECK_EXE is unset or the hook itself fails: Copilot hooks are
// synchronous, and deck only observes. stdin (the camelCase payload) reaches
// `deck _hook` untouched.
func copilotHookCommand(event string) string {
	return fmt.Sprintf(`[ -n "$%[1]s" ] && %[2]s=%[3]s "$%[1]s" _hook >/dev/null 2>&1; exit 0`, CopilotDeckExeEnv, CopilotHookEventEnv, event)
}

// CopilotHooksConfig is the constant hooks.json of the deck plugin.
func CopilotHooksConfig() string {
	entries := make([]string, 0, len(CopilotHookEvents))
	for _, event := range CopilotHookEvents {
		entries = append(entries, fmt.Sprintf(`%q:[{"type":"command","bash":%q,"timeoutSec":%d}]`,
			event, copilotHookCommand(event), CopilotHookTimeoutSec))
	}
	return `{"version":1,"hooks":{` + strings.Join(entries, ",") + "}}\n"
}

// InstrumentFiles returns the plugin's two files, or nothing when the launch
// names no data root to keep them in. Writing them is best effort: a launch
// that cannot install them drops --plugin-dir and goes ahead.
func (Copilot) InstrumentFiles(in LaunchInput) []InstrumentFile {
	if in.DeckHome == "" {
		return nil
	}
	dir := CopilotPluginDir(in.DeckHome)
	return []InstrumentFile{
		{Path: filepath.Join(dir, "plugin.json"), Content: []byte(CopilotPluginManifest), DirMode: 0o700, Optional: true, DropArgv: []string{"--plugin-dir", dir}},
		{Path: filepath.Join(dir, "hooks.json"), Content: []byte(CopilotHooksConfig()), DirMode: 0o700, Optional: true, DropArgv: []string{"--plugin-dir", dir}},
	}
}
