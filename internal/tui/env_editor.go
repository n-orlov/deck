package tui

import (
	"context"
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

// envView renders the `e` env editor (SPEC §6.1/§6.3): j/k (or up/down)
// move the cursor across sessionEnvRows, enter on the highlighted row opens
// it for editing (task 021), and typing plus enter commits the new value
// through m.setSessionEnv -- session.SetSessionEnv persists it, marks the
// row env_dirty and mirrors the key into tmux's own environment table for
// future panes, never into the pane that is already running. Esc cancels
// an edit in progress without touching anything, or closes the whole
// dialog when nothing is being edited.
func (m Model) envView() string {
	session := m.sessions[m.selected]
	rows := m.sessionEnvRows(session)
	var b strings.Builder
	fmt.Fprintf(&b, "Environment for %s\n\n", session.Name)
	if len(rows) == 0 {
		b.WriteString("(no environment keys resolved for this session)\n")
	} else {
		for i, row := range rows {
			marker := "  "
			if i == m.envCursor {
				marker = "> "
			}
			label := fmt.Sprintf("%s%-24s", marker, row.Key)
			value := fmt.Sprintf("%s  [%s]", row.Value, row.Layer)
			if m.envEditKey != "" && m.envEditKey == row.Key {
				value = m.envEditValue + "_"
			}
			fmt.Fprintf(&b, "%s\n", m.detailField(label, value))
		}
	}
	b.WriteString("\nOrder, lowest to highest: server env \u2192 captured_path \u2192 config [env] \u2192 session env.\n")
	if m.envNote != "" {
		fmt.Fprintf(&b, "\n%s\n", m.envNote)
	}
	if m.envEditKey != "" {
		b.WriteString("Enter saves this key into the session's own env; Esc cancels this edit.\n")
	} else {
		b.WriteString("j/k select a key, Enter edits it; Esc closes.\n")
	}
	return m.framedDialog(b.String())
}

// updateEnvDialog handles keys while the `e` env editor is open (task 021).
// It is two nested modes rather than one contract: browsing (navigate rows,
// esc closes the dialog per §11.4) and, once enter opens a row, editing
// (free-text typing/backspace, esc cancels only the edit, enter commits it)
// -- the same distinction createView's own cwd tab-completion list makes
// between "esc closes the list" and "esc closes the whole modal".
func (m Model) updateEnvDialog(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.envEditKey != "" {
		switch msg.String() {
		case "esc":
			m.envEditKey, m.envEditValue, m.envEditPrefilled, m.envNote = "", "", false, ""
			return m, nil
		case "enter":
			cmd := m.submitEnvEdit()
			return m, cmd
		case "backspace", "ctrl+h":
			// Backspace is also an edit (SPEC §11.7's "typing replaces it
			// wholesale" pattern, already used by createView's cwd field):
			// while the buffer still holds nothing but the row's untouched
			// current value, backspace clears it wholesale rather than
			// trimming one rune off the end of it.
			if m.envEditPrefilled {
				m.envEditValue, m.envEditPrefilled = "", false
				return m, nil
			}
			if m.envEditValue != "" {
				runs := []rune(m.envEditValue)
				m.envEditValue = string(runs[:len(runs)-1])
			}
			return m, nil
		}
		if runes := msg.Runes; len(runes) > 0 {
			if m.envEditPrefilled {
				m.envEditValue, m.envEditPrefilled = "", false
			}
			m.envEditValue += string(runes)
		}
		return m, nil
	}
	if cmd, handled := applyDialogContract(msg, dialogContract{
		Cancel: func() {
			m.envEditing = false
			m.envNote = ""
		},
	}); handled {
		return m, cmd
	}
	session := m.sessions[m.selected]
	rows := m.sessionEnvRows(session)
	switch msg.String() {
	case "up", "k":
		if len(rows) > 0 {
			m.envCursor = (m.envCursor - 1 + len(rows)) % len(rows)
		}
	case "down", "j":
		if len(rows) > 0 {
			m.envCursor = (m.envCursor + 1) % len(rows)
		}
	case "enter":
		if m.envCursor < 0 || m.envCursor >= len(rows) {
			return m, nil
		}
		row := rows[m.envCursor]
		// The buffer opens preloaded with the row's current value rather
		// than empty, so a small correction to a long value never requires
		// retyping it in full -- envEditPrefilled marks it untouched, so
		// the very first keystroke (typed or backspace) replaces it
		// wholesale instead of editing within it, exactly like createView's
		// own cwd field prefill.
		m.envEditKey, m.envEditValue, m.envEditPrefilled, m.envNote = row.Key, row.Value, true, ""
	}
	return m, nil
}

// submitEnvEdit dispatches a committed edit through m.setSessionEnv (nil
// when no envSetter is wired, e.g. an internal/tui-only test model), and
// clears the local typing state immediately -- the pending edit is either
// already applied by the store/tmux by the time envEdited arrives, or it
// failed and envEdited's error becomes m.envNote, exactly like every other
// §11.4 dialog's own submit-then-reply pattern. Like submitCreate, it
// mutates the caller's local Model in place and returns only the tea.Cmd
// (never a tea.Model of its own) so updateEnvDialog's own return still
// carries the one Model value the rest of this package's Update chain
// expects.
func (m *Model) submitEnvEdit() tea.Cmd {
	if m.setSessionEnv == nil {
		m.envNote = "editing the environment is unavailable"
		m.envEditKey, m.envEditValue, m.envEditPrefilled = "", "", false
		return nil
	}
	session := m.sessions[m.selected]
	sessionID, key, value := session.ID, m.envEditKey, m.envEditValue
	setSessionEnv := m.setSessionEnv
	m.envEditKey, m.envEditValue, m.envEditPrefilled = "", "", false
	return func() tea.Msg {
		updated, err := setSessionEnv(context.Background(), sessionID, key, value)
		return envEdited{session: updated, err: err}
	}
}
