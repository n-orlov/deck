package tui

import (
	"fmt"
	"os"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/store"
)

// envLayerServer, envLayerCapturedPath, envLayerConfig and envLayerSession
// are the four SPEC §6.1/§6.3 layer names the `e` env editor names on
// screen, lowest to highest priority: "environment of the process that
// started the tmux server" → captured_path (PATH only, §6.3, sitting
// between the server environment and config [env] in that order) →
// config.toml's [env] table → the session's own env map.
const (
	envLayerServer       = "server env"
	envLayerCapturedPath = "captured_path"
	envLayerConfig       = "config [env]"
	envLayerSession      = "session env"
)

// envRow is one line of the `e` env editor: a key, the effective value that
// actually reaches the pane, and the name of the layer that supplied it.
type envRow struct {
	Key, Value, Layer string
}

// sessionEnvRows lists every key deck itself is aware of for this session --
// the union of config.toml's [env] table, the session's own env map, and
// PATH (via captured_path) -- resolved through resolveEnvKey, sorted by key
// for a deterministic, scriptable-to-test frame. It deliberately does not
// enumerate the tmux server's entire inherited environment (arbitrary,
// unbounded, and none of deck's business beyond the keys it itself
// resolves): a key that exists only in the server environment and is never
// mentioned by config.toml or a session's env map is not something deck
// resolved a layer for, so it has nothing to show for it here.
func (m Model) sessionEnvRows(session store.Session) []envRow {
	keys := make(map[string]struct{}, len(m.settings.Env)+len(session.Env)+1)
	if session.CapturedPath != "" {
		keys["PATH"] = struct{}{}
	}
	for key := range m.settings.Env {
		keys[key] = struct{}{}
	}
	for key := range session.Env {
		keys[key] = struct{}{}
	}
	sorted := make([]string, 0, len(keys))
	for key := range keys {
		sorted = append(sorted, key)
	}
	sort.Strings(sorted)
	rows := make([]envRow, 0, len(sorted))
	for _, key := range sorted {
		value, layer := m.resolveEnvKey(key, session)
		rows = append(rows, envRow{Key: key, Value: value, Layer: layer})
	}
	return rows
}

// resolveEnvKey answers the SPEC §6.1/§6.3 layering question for one key:
// which of the four layers (lowest to highest: server env → captured_path
// → config [env] → session env) supplies the value that actually reaches
// the pane, and what that value is. This mirrors
// internal/service.Service.resolveLaunchEnv's own precedence exactly (the
// editor must never disagree with what a launch actually does), generalised
// from that function's PATH/config/session merge to name the winning layer
// rather than only compute the merged map.
func (m Model) resolveEnvKey(key string, session store.Session) (value, layer string) {
	if v, ok := session.Env[key]; ok {
		return v, envLayerSession
	}
	if v, ok := m.settings.Env[key]; ok {
		return v, envLayerConfig
	}
	if key == "PATH" && session.CapturedPath != "" {
		return session.CapturedPath, envLayerCapturedPath
	}
	if v, ok := os.LookupEnv(key); ok {
		return v, envLayerServer
	}
	return "", envLayerServer
}

// envView renders the `e` env editor (task 020, SPEC §6.1/§6.3): read-only
// in this phase -- writing an edit, env_dirty and the tmux mirror are task
// 021's own deliverable -- so its only contract key is esc.
func (m Model) envView() string {
	session := m.sessions[m.selected]
	rows := m.sessionEnvRows(session)
	var b strings.Builder
	fmt.Fprintf(&b, "Environment for %s\n\n", session.Name)
	if len(rows) == 0 {
		b.WriteString("(no environment keys resolved for this session)\n")
	} else {
		for _, row := range rows {
			label := fmt.Sprintf("%-24s", row.Key)
			fmt.Fprintf(&b, "%s\n", m.detailField(label, fmt.Sprintf("%s  [%s]", row.Value, row.Layer)))
		}
	}
	b.WriteString("\nOrder, lowest to highest: server env \u2192 captured_path \u2192 config [env] \u2192 session env.\n")
	b.WriteString("Esc closes; nothing here is editable yet.\n")
	return m.framedDialog(b.String())
}

// updateEnvDialog handles keys while the `e` env editor is open. It has no
// navigable fields (nothing to tab onto, nothing to cycle) so the shared
// §11.4 contract only wires up Cancel; enter, tab and left/right are simply
// not contract keys here and fall through unhandled, exactly as detailView
// and helpView already do.
func (m Model) updateEnvDialog(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	_, handled := applyDialogContract(msg, dialogContract{
		Cancel: func() {
			m.envEditing = false
		},
	})
	if handled {
		return m, nil
	}
	return m, nil
}
