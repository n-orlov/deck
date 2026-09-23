package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/theme"
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
	ctx := context.Background()
	rows := make([]envRow, 0, len(sorted))
	for _, key := range sorted {
		value, layer := m.resolveEnvKey(ctx, key, session)
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
//
// The server-env layer (R121) is read from the SESSION'S OWN tmux server
// (m.tmuxClient.ServerEnvironment, `show-environment -g`), never from this
// process's own ambient os.Environ: the observing TUI can be a later
// process than the one that started the already-running server, with a
// different ambient value for the same key, and only the server's own
// table answers what a pane it hosts actually inherited. When there is no
// tmux client to ask (m.tmuxClient.Socket == "", e.g. a session-only unit
// test with no real server), the layer is simply absent rather than
// falling back to this process's own environment.
func (m Model) resolveEnvKey(ctx context.Context, key string, session store.Session) (value, layer string) {
	if v, ok := session.Env[key]; ok {
		return v, envLayerSession
	}
	if v, ok := m.settings.Env[key]; ok {
		return v, envLayerConfig
	}
	if key == "PATH" && session.CapturedPath != "" {
		return session.CapturedPath, envLayerCapturedPath
	}
	if m.tmuxClient.Socket != "" {
		if v, ok, err := m.tmuxClient.ServerEnvironment(ctx, key); err == nil && ok {
			return v, envLayerServer
		}
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
//
// An edit in progress is the one case that must not wait for a manual
// PgDn (mirroring createView's own rejection case): the moment enter opens
// a row, features/env_editor_test.go's keyboard-only PTY step reads this
// view's own string for the submit legend ("Enter saves this key") and for
// the runes it just typed, with no scroll keystroke in between. Once the
// resolved-key list overflows an 80x24 frame both of those sit off the
// bottom of the first page, so whenever a row is being edited and the user
// has not scrolled away from the top themselves (m.envScroll == 0, the
// value `e` opened this dialog with), the view renders its LAST page --
// where envBody always puts the edit prompt and the submit legend, back to
// back -- rather than requiring a page-down to see what is being typed.
func (m Model) envView() string {
	scroll := m.envScroll
	if m.envEditKey != "" && scroll == 0 {
		scroll = m.dialogMaxScroll(m.envBody())
	}
	return m.framedDialogScrollable(m.styledEnvBody(), scroll)
}

// envRowLine is the one place that decides a row's marker, label and
// value text -- both envBody (plain, measured) and styledEnvBody (coloured,
// rendered) call this instead of each re-deriving the same row
// independently, so the two can never drift into a different physical
// line count the way createBody/styledCreateBody's shared field-table walk
// avoids for the create modal.
func (m Model) envRowLine(row envRow, focused bool) (label, value string) {
	marker := "  "
	if focused {
		marker = "> "
	}
	label = fmt.Sprintf("%s%-24s", marker, row.Key)
	// SPEC §6.4/requirement 21: a secret-shaped key's value is masked by
	// default via the single maskEnvValue predicate, revealed only while
	// m.envReveal is on (the "r" toggle in updateEnvDialog).
	displayValue := m.maskEnvValue(row.Key, row.Value, m.envReveal)
	value = fmt.Sprintf("%s  [%s]", displayValue, row.Layer)
	return label, value
}

// envEditPromptLine is the one line an edit in progress is typed on --
// deliberately NOT the row's own line in the list above, which keeps
// showing the value that is still in force until enter commits the new
// one. Putting the buffer here, immediately above envHintLine, is what
// makes "what I am typing" and "the key that saves it" a single adjacent
// pair: they can never land on opposite sides of a page boundary the way a
// buffer rendered 20 rows up can (the env editor is scroll-bounded since
// this task, and the cursor row may be anywhere in an overflowing list).
// It returns label/value separately for the same reason envRowLine does:
// styledEnvBody colours the key part in `hint` and the typed value in
// `text`, over the focused row's own `selection` background.
func (m Model) envEditPromptLine() (label, value string) {
	label = fmt.Sprintf("Editing %s: ", m.envEditKey)
	value = m.maskEnvValue(m.envEditKey, m.envEditValue, m.envReveal) + "_"
	return label, value
}

// envBody builds the env editor's PLAIN, unstyled content -- byte for byte
// what envView rendered directly before task 017 added a themed rendering
// pass. It stays the one text wrapDialogLines/dialogMaxScroll/PgUp/PgDn
// measure (mirroring createBody's own role for the create modal) so a
// colour token styledEnvBody adds can never move where a page boundary
// falls.
func (m Model) envBody() string {
	session, _ := m.selectedSession()
	rows := m.sessionEnvRows(session)
	var lines []string
	lines = append(lines, fmt.Sprintf("Environment for %s", session.Name))
	lines = append(lines, "")
	if len(rows) == 0 {
		lines = append(lines, "(no environment keys resolved for this session)")
	} else {
		for i, row := range rows {
			label, value := m.envRowLine(row, i == m.envCursor)
			lines = append(lines, label+value)
		}
	}
	lines = append(lines, "")
	lines = append(lines, "Order, lowest to highest: server env \u2192 captured_path \u2192 config [env] \u2192 session env.")
	if m.envNote != "" {
		lines = append(lines, "")
		lines = append(lines, m.envNote)
	}
	if m.envEditKey != "" {
		lines = append(lines, "")
		label, value := m.envEditPromptLine()
		lines = append(lines, label+value)
	}
	lines = append(lines, m.envHintLine())
	return strings.Join(lines, "\n")
}

// envHintLine is the env editor's closing instructional line -- the one
// line every keyboard-only PTY assertion in features/env_editor_test.go
// waits on ("Enter saves this key", the browse-mode legend) -- shared by
// envBody and styledEnvBody so the two never state it differently.
func (m Model) envHintLine() string {
	if m.envEditKey != "" {
		return "Enter saves this key into the session's own env; Esc cancels this edit."
	}
	revealHint := "r reveals secret-shaped values (masked by default)"
	if m.envReveal {
		revealHint = "r masks secret-shaped values again"
	}
	return fmt.Sprintf("j/k select a key, Enter edits it, %s; Esc closes.", revealHint)
}

// envBrowseLegendKeys/envEditLegendKeys are styledEnvBody's own key
// vocabulary for envHintLine's two variants (task 017), mirroring
// createFooterKeyTokens one file over: the exact words in that sentence
// that name a bound key, so styledEnvBody's colorLegendLine can single
// them out for `key` and leave the surrounding prose `hint` -- the split
// R82 states for a dialog footer legend ("footer keys -> key with the rest
// in hint"), which is also what styledCreateBody's own colorFooterLine
// does one file over.
var envBrowseLegendKeys = map[string]bool{"j/k": true, "Enter": true, "r": true, "Esc": true}
var envEditLegendKeys = map[string]bool{"Enter": true, "Esc": true}

// styledEnvBody re-derives envBody's exact structure -- same title, blank
// line, row loop, order line, optional note and closing hint, in the same
// order -- but colours each finished PHYSICAL line rather than the
// logical one, exactly like styledCreateBody: every helper below wraps a
// plain string via wrap (m.wrapDialogLines) FIRST, so a colour token can
// never straddle a word-wrap boundary wrapDialogLines hasn't drawn yet.
// Token mapping is SPEC.md:1355 verbatim: the title in `title`, a row's
// key in `hint` and its value (with winning layer) in `text`, the order
// line in `dimmed`, a validation/error note in `error`, the bound keys in
// the closing legend in `key` over `hint` prose, and the focused row (m.envCursor, whether
// merely highlighted or actively being edited) carrying the same
// `selection` treatment a selected list row does (renderCreateRowSegments,
// reused verbatim from the create modal -- the composition it performs is
// generic to "one row, several coloured segments, an optional selection
// background," not specific to that dialog).
func (m Model) styledEnvBody() string {
	wrap := m.wrapDialogLines
	var out []string

	colorWhole := func(tok theme.Token, line string) {
		for _, l := range wrap(line) {
			out = append(out, m.colorToken(tok, l))
		}
	}
	colorRow := func(label, value string, focused bool) {
		for _, l := range wrap(label + value) {
			var segs []settingsRowSegment
			rest := l
			if strings.HasPrefix(l, label) {
				segs = append(segs, settingsRowSegment{Text: label, Tok: theme.Hint})
				rest = strings.TrimPrefix(l, label)
			}
			if rest != "" {
				segs = append(segs, settingsRowSegment{Text: rest, Tok: theme.Text})
			}
			if len(segs) == 0 {
				segs = []settingsRowSegment{{Text: l, Tok: theme.Text}}
			}
			out = append(out, m.renderCreateRowSegments(focused, segs))
		}
	}
	// colorLegendLine colours envHintLine's already-wrapped sentence word
	// by word, never before wrap: every key word in keys gets `key`, every
	// other word (punctuation attached and all) gets `hint`.
	colorLegendLine := func(line string, keys map[string]bool) {
		for _, l := range wrap(line) {
			fields := strings.Fields(l)
			for i, f := range fields {
				trimmed := strings.TrimRight(f, ",;.")
				if keys[trimmed] {
					fields[i] = m.colorToken(theme.Key, trimmed) + f[len(trimmed):]
				} else {
					fields[i] = m.colorToken(theme.Hint, f)
				}
			}
			out = append(out, strings.Join(fields, " "))
		}
	}

	session, _ := m.selectedSession()
	rows := m.sessionEnvRows(session)
	colorWhole(theme.Title, fmt.Sprintf("Environment for %s", session.Name))
	out = append(out, "")
	if len(rows) == 0 {
		colorWhole(theme.Dimmed, "(no environment keys resolved for this session)")
	} else {
		for i, row := range rows {
			label, value := m.envRowLine(row, i == m.envCursor)
			colorRow(label, value, i == m.envCursor)
		}
	}
	out = append(out, "")
	colorWhole(theme.Dimmed, "Order, lowest to highest: server env \u2192 captured_path \u2192 config [env] \u2192 session env.")
	if m.envNote != "" {
		out = append(out, "")
		colorWhole(theme.Error, m.envNote)
	}
	if m.envEditKey != "" {
		out = append(out, "")
		label, value := m.envEditPromptLine()
		colorRow(label, value, true)
		colorLegendLine(m.envHintLine(), envEditLegendKeys)
	} else {
		colorLegendLine(m.envHintLine(), envBrowseLegendKeys)
	}
	return strings.Join(out, "\n")
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
		case "pgup", "pgdown":
			// An edit in progress does not consume the paging keys: the
			// list it was opened from is scroll-bounded (this task), so a
			// typist who wants to re-read a row further up must be able to
			// page there without abandoning the edit. Before this, the
			// whole edit-mode branch returned early and PgUp/PgDn were
			// silent no-ops for as long as a row stayed open.
			dir := -1
			if msg.String() == "pgdown" {
				dir = 1
			}
			m.envScroll = m.dialogScrollByPage(m.envScroll, m.envBody(), dir)
			return m, nil
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
	session, _ := m.selectedSession()
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
	case "r":
		// SPEC §6.4/requirement 21: the explicit per-view reveal toggle.
		// Only meaningful while browsing -- while a value is being typed
		// (m.envEditKey != "") this branch is unreachable, since that
		// case returns earlier in this function.
		m.envReveal = !m.envReveal
	case "pgup":
		// Task 017: the env editor moved onto framedDialogScrollable (task
		// 014's height probe found the resolved-key list overflows an
		// 80x24 frame from 16 keys up), so PgUp/PgDn now scroll it exactly
		// like the create modal's own task 016 -- measured off m.envBody(),
		// the plain body, never the coloured one, so a theme change can
		// never move where a page boundary falls.
		m.envScroll = m.dialogScrollByPage(m.envScroll, m.envBody(), -1)
	case "pgdown":
		m.envScroll = m.dialogScrollByPage(m.envScroll, m.envBody(), 1)
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
	session, _ := m.selectedSession()
	sessionID, key, value := session.ID, m.envEditKey, m.envEditValue
	setSessionEnv := m.setSessionEnv
	m.envEditKey, m.envEditValue, m.envEditPrefilled = "", "", false
	return func() tea.Msg {
		updated, err := setSessionEnv(context.Background(), sessionID, key, value)
		return envEdited{session: updated, err: err}
	}
}
