package agent

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// copilotWorkspaceBytes bounds the read of workspace.yaml. The file is a few
// hundred bytes; anything larger is not the file Copilot writes.
const copilotWorkspaceBytes = 64 * 1024

// copilotWorkspace is the part of workspace.yaml ReadName understands.
type copilotWorkspace struct {
	ID        string `yaml:"id"`
	Name      string `yaml:"name"`
	UserNamed *bool  `yaml:"user_named"`
}

// copilotRoot is the Copilot home to read: the resolved COPILOT_HOME, else
// <Home>/.copilot; empty when neither is known.
func copilotRoot(env map[string]string, home string) string {
	if root := env["COPILOT_HOME"]; root != "" {
		return root
	}
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".copilot")
}

// ReadName returns the name in <root>/session-state/<id>/workspace.yaml (R260).
// The file is parsed with a real YAML parser, from a bounded read, so quoted
// names holding `:`, `'` or Unicode round-trip. explicit is `user_named`: true
// means a human chose the name (`-n` or /rename), false is Copilot's own
// generated title. The marker must be present and a boolean: an absent, null
// or non-boolean user_named is an upstream contract that changed, so there is
// no name and the row keeps its dummy one (R265). A missing or unreadable file, a file whose id is another
// conversation's, and a blank name are all no name.
func (Copilot) ReadName(in NameProbeInput) (string, bool, bool) {
	if !safeConversationID(in.ConversationID) {
		return "", false, false
	}
	root := copilotRoot(in.Env, in.Home)
	if root == "" {
		return "", false, false
	}
	ws, ok := readCopilotWorkspace(filepath.Join(root, "session-state", in.ConversationID, "workspace.yaml"))
	if !ok || (ws.ID != "" && ws.ID != in.ConversationID) {
		return "", false, false
	}
	if ws.UserNamed == nil {
		return "", false, false
	}
	name := strings.TrimSpace(ws.Name)
	return name, *ws.UserNamed, name != ""
}

// readCopilotWorkspace reads and parses path, at most copilotWorkspaceBytes.
func readCopilotWorkspace(path string) (copilotWorkspace, bool) {
	f, err := os.Open(path) //nolint:gosec // G304: read-only; the path is built from a validated conversation id under the Copilot home
	if err != nil {
		return copilotWorkspace{}, false
	}
	defer func() { _ = f.Close() }()
	if info, err := f.Stat(); err != nil || !info.Mode().IsRegular() {
		return copilotWorkspace{}, false
	}
	data, err := io.ReadAll(io.LimitReader(f, copilotWorkspaceBytes))
	if err != nil {
		return copilotWorkspace{}, false
	}
	var ws copilotWorkspace
	if yaml.Unmarshal(data, &ws) != nil {
		return copilotWorkspace{}, false
	}
	return ws, true
}

// PushName is Copilot's rename keystroke plan (R260): `/rename <name>`, then a
// lone Enter, each its own send-keys call. The name is cleaned for Copilot
// again here (no `"`, at most 100 runes, control characters removed), and a
// name with nothing left is declined.
func (Copilot) PushName(in NamePushInput) (keys []string, ok bool) {
	name, ok := SanitisePushName("copilot", in.Name)
	if !ok {
		return nil, false
	}
	return []string{"/rename " + name, "Enter"}, true
}

// copilotDialogReasons are the probe reasons that mean one of Copilot's modal
// prompts has replaced the composer: typed keys would answer it, not rename.
var copilotDialogReasons = map[string]bool{"folder trust": true, "permission prompt": true, "question": true}

// DialogFocused reports whether the folder-trust prompt, a permission dialog
// or a question dialog is on the screen, read with the same rules the pane
// probe uses (copilotProbeRules), so a push is refused while one is up.
func (Copilot) DialogFocused(screen string) bool {
	status, reason := probe("copilot", screen)
	return status == "waiting" && copilotDialogReasons[reason]
}
