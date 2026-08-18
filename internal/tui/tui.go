// Package tui implements deck's interactive terminal interface.
package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/agent"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/service"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// Model is the base session-list screen. Later modal and action work extends
// this model rather than providing a separate command-line interface.
type Model struct {
	store               *store.Store
	settings            config.Settings
	sessions            []store.Session
	startupNote         string
	help                bool
	creating            bool
	createName          string
	createCWD           string
	createAgent         string
	createProfile       string
	createLaunchArgs    string
	createEnv           string
	createPreLaunch     string
	createLoginShell    bool
	createYoloConfirmed bool
	createField         int
	createError         string
	create              func(context.Context, service.ShellCreateInput) (store.Session, error)
	attach              func(context.Context, string) (*exec.Cmd, error)
	kill                func(context.Context, store.Session) error
	reconcile           func(context.Context) error
	selected            int
	attachError         string
	width               int
	height              int
}

type sessionsLoaded struct {
	sessions []store.Session
	err      error
}

// Tick types are deliberately separate: reconciliation refreshes durable rows,
// while preview is a future-facing wake-up cadence that must not manufacture an
// out-of-scope preview pane. Animation is likewise optional and never runs
// when DECK_ANIM=0, keeping deterministic frames quiet.
type reconcileTick time.Time
type previewTick time.Time
type animationTick time.Time

type shellCreated struct {
	session store.Session
	err     error
}

type attachFinished struct{ err error }

type sessionKilled struct{ err error }

// New creates a list model. tmux failures are intentionally retained as a
// rendered health state: users must be able to read and quit it.
func New(db *store.Store, settings config.Settings, tmuxNote string) Model {
	return Model{store: db, settings: settings, startupNote: tmuxNote}
}

// NewWithShellCreator creates a list model that can create plain shell sessions.
// Keeping creation behind this small function lets the UI remain a renderer and
// makes failures, including slug collisions, visible rather than fatal.
func NewWithShellCreator(db *store.Store, settings config.Settings, tmuxNote string, creator func(context.Context, service.ShellCreateInput) (store.Session, error)) Model {
	return NewWithShellCreatorAndAttacher(db, settings, tmuxNote, creator, nil)
}

// NewWithShellCreatorAndAttacher creates a model with the two Phase 0 shell
// actions. The attacher returns an exec command so Bubble Tea can temporarily
// release the terminal while tmux owns it, then redraw the list on detach.
func NewWithShellCreatorAndAttacher(db *store.Store, settings config.Settings, tmuxNote string, creator func(context.Context, service.ShellCreateInput) (store.Session, error), attacher func(context.Context, string) (*exec.Cmd, error)) Model {
	return NewWithShellCreatorAttacherAndKiller(db, settings, tmuxNote, creator, attacher, nil)
}

// NewWithShellCreatorAttacherAndKiller creates a model with all implemented
// Phase 0 shell actions. Killing only tears down deck's private tmux session;
// the caller's cwd and its durable session row remain intact and resumable.
func NewWithShellCreatorAttacherAndKiller(db *store.Store, settings config.Settings, tmuxNote string, creator func(context.Context, service.ShellCreateInput) (store.Session, error), attacher func(context.Context, string) (*exec.Cmd, error), killer func(context.Context, store.Session) error) Model {
	return NewWithShellCreatorAttacherKillerAndReconciler(db, settings, tmuxNote, creator, attacher, killer, nil)
}

// NewWithShellCreatorAttacherKillerAndReconciler adds the released liveness
// pass to the list refresh. Running it immediately before loading rows means
// an external tmux disappearance reaches the visible client in one configured
// reconciliation cadence, rather than waiting a second refresh interval.
func NewWithShellCreatorAttacherKillerAndReconciler(db *store.Store, settings config.Settings, tmuxNote string, creator func(context.Context, service.ShellCreateInput) (store.Session, error), attacher func(context.Context, string) (*exec.Cmd, error), killer func(context.Context, store.Session) error, reconciler func(context.Context) error) Model {
	m := New(db, settings, tmuxNote)
	m.create, m.attach, m.kill, m.reconcile = creator, attacher, killer, reconciler
	return m
}

func (m Model) Init() tea.Cmd {
	commands := []tea.Cmd{
		m.loadSessions,
		tea.Tick(m.settings.Reconcile, func(t time.Time) tea.Msg { return reconcileTick(t) }),
		tea.Tick(m.settings.Preview, func(t time.Time) tea.Msg { return previewTick(t) }),
	}
	if m.settings.Animation {
		commands = append(commands, tea.Tick(m.settings.Preview, func(t time.Time) tea.Msg { return animationTick(t) }))
	}
	return tea.Batch(commands...)
}

func (m Model) loadSessions() tea.Msg {
	if m.store == nil {
		return sessionsLoaded{}
	}
	rows, err := m.store.ListSessions(context.Background())
	return sessionsLoaded{sessions: rows, err: err}
}

func (m Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case sessionsLoaded:
		if msg.err != nil {
			m.startupNote = "Cannot read sessions: " + msg.err.Error()
		} else {
			m.sessions = msg.sessions
			if m.selected >= len(m.sessions) {
				m.selected = max(0, len(m.sessions)-1)
			}
		}
	case shellCreated:
		if msg.err != nil {
			m.createError = msg.err.Error()
			return m, nil
		}
		// A completed creation is the documented interaction that advances a
		// frozen deterministic wall clock. It happens after the service has
		// persisted the row, so its CreatedAt remains the preceding wall time.
		// Elapsed measurements remain monotonic through Clock.Advance.
		if m.settings.Clock != nil {
			m.settings.Clock.Advance()
		}
		m.creating, m.createError = false, ""
		return m, m.loadSessions
	case attachFinished:
		if msg.err != nil {
			m.attachError = "Cannot attach: " + msg.err.Error()
		}
		return m, m.loadSessions
	case sessionKilled:
		if msg.err != nil {
			m.attachError = "Cannot kill: " + msg.err.Error()
			return m, nil
		}
		m.attachError = ""
		return m, m.loadSessions
	case reconcileTick:
		loadAfterReconcile := m.loadSessions
		if m.reconcile != nil {
			loadAfterReconcile = func() tea.Msg {
				if err := m.reconcile(context.Background()); err != nil {
					return sessionsLoaded{err: err}
				}
				return m.loadSessions()
			}
		}
		return m, tea.Batch(loadAfterReconcile, tea.Tick(m.settings.Reconcile, func(t time.Time) tea.Msg { return reconcileTick(t) }))
	case previewTick:
		// Phase 0 intentionally has no preview pane. Retaining this independent
		// wake-up means DECK_PREVIEW_MS is already honored by the released UI
		// rather than being silently parsed and ignored.
		return m, tea.Tick(m.settings.Preview, func(t time.Time) tea.Msg { return previewTick(t) })
	case animationTick:
		if !m.settings.Animation {
			return m, nil
		}
		return m, tea.Tick(m.settings.Preview, func(t time.Time) tea.Msg { return animationTick(t) })
	case tea.KeyMsg:
		if m.creating {
			return m.updateCreate(msg)
		}
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "?":
			m.help = !m.help
		case "esc":
			m.help = false
		case "n":
			if !m.help {
				m.creating, m.createError, m.createField = true, "", 0
				m.createName, m.createCWD = "", ""
				m.createAgent, m.createProfile = createAgentOptions[0], createProfileOptions[0]
				m.createLaunchArgs, m.createEnv, m.createPreLaunch, m.createLoginShell = "", "", "", false
				m.createYoloConfirmed = false
			}
		case "up", "k":
			if m.selected > 0 {
				m.selected--
			}
		case "down", "j":
			if m.selected+1 < len(m.sessions) {
				m.selected++
			}
		case "x":
			if m.kill == nil || len(m.sessions) == 0 {
				return m, nil
			}
			session := m.sessions[m.selected]
			if session.Status == "stopped" {
				m.attachError = "Cannot kill: session is already stopped"
				return m, nil
			}
			return m, func() tea.Msg {
				return sessionKilled{err: m.kill(context.Background(), session)}
			}
		case "enter":
			if m.attach == nil || len(m.sessions) == 0 {
				return m, nil
			}
			session := m.sessions[m.selected]
			if session.Status == "stopped" {
				m.attachError = "Cannot attach: session is stopped; resume it first"
				return m, nil
			}
			command, err := m.attach(context.Background(), session.Slug)
			if err != nil {
				m.attachError = "Cannot attach: " + err.Error()
				return m, nil
			}
			m.attachError = ""
			return m, tea.ExecProcess(command, func(err error) tea.Msg { return attachFinished{err: err} })
		}
	}
	return m, nil
}

func (m Model) View() string {
	if m.help {
		return helpView(m.settings.ASCII)
	}
	if m.creating {
		return m.createView()
	}
	var b strings.Builder
	b.WriteString(m.color("deck") + m.glyph(" — ", " - ") + "sessions")
	if m.settings.Socket != "" {
		fmt.Fprintf(&b, "  socket: %s", m.settings.Socket)
	}
	b.WriteString("\n\n")
	if m.startupNote != "" {
		fmt.Fprintf(&b, "tmux unavailable: %s\n", m.startupNote)
		b.WriteString("Install tmux 3.2 or newer, then restart deck.\n\n")
	}
	if len(m.sessions) == 0 {
		b.WriteString("No sessions yet. Press n to create a session.\n")
	} else {
		for index, session := range m.sessions {
			status := session.Status
			if status == "stopped" {
				status += m.glyph(" · resumable", " - resumable")
			}
			marker := "  "
			if index == m.selected {
				marker = "> "
			}
			fmt.Fprintf(&b, "%s%s  %-10s %s  created %s\n    %s\n", marker, session.Name, session.Agent, status, m.relativeTime(session.CreatedAt), session.CWD)
		}
	}
	if m.attachError != "" {
		fmt.Fprintf(&b, "\n%s\n", m.attachError)
	}
	b.WriteString("\n" + m.glyph("↑/↓ select · ↵ attach · n new · x kill · ? help · q quit", "up/down select - Enter attach - n new - x kill - ? help - q quit") + "\n")
	return b.String()
}

// glyph selects the documented ASCII fallback for terminals where optional
// Unicode symbols are unwanted or unsuitable.
func (m Model) glyph(unicode, ascii string) string {
	if m.settings.ASCII {
		return ascii
	}
	return unicode
}

// color is the only product styling in Phase 0. Keeping it here makes
// NO_COLOR and DECK_COLOR runtime controls rather than merely parsed settings.
func (m Model) color(text string) string {
	if !m.settings.Color {
		return text
	}
	return "\x1b[1;36m" + text + "\x1b[0m"
}

// relativeTime is intentionally based on the configured wall clock. A frozen
// DECK_CLOCK therefore keeps this rendered value stable while Clock.Elapsed
// remains monotonic for measurements and audit durations.
func (m Model) relativeTime(createdAt int64) string {
	now := time.Now()
	if m.settings.Clock != nil {
		now = m.settings.Clock.Now()
	}
	age := now.Sub(time.UnixMilli(createdAt))
	if age < time.Minute {
		return "just now"
	}
	if age < time.Hour {
		return fmt.Sprintf("%dm ago", int(age/time.Minute))
	}
	if age < 24*time.Hour {
		return fmt.Sprintf("%dh ago", int(age/time.Hour))
	}
	return fmt.Sprintf("%dd ago", int(age/(24*time.Hour)))
}

// createAgentOptions and createProfileOptions are the values the create
// modal's Agent and Permission profile fields cycle through.
// createProfileOptions is the master ordering used only as the initial
// default ("safe") when the modal opens; the actual offered set while the
// modal is open is narrowed per selected agent and per allow_yolo by
// createProfileOptionsFor (SPEC §5, task 017).
var createAgentOptions = []string{"shell", "claude", "pi"}
var createProfileOptions = []string{"safe", "plan", "edits", "yolo"}

// createProfileOptionsFor returns exactly the permission profiles the
// selected adapter declares (SPEC §5), narrowed further to exclude "yolo"
// when allowYolo is false so the config gate is honoured before any
// per-launch confirm gate is even reachable. shell has no notion of
// permission profiles at all (createAgentCapabilities reports
// !applicable): its field stays a single cosmetic "safe" value, per the
// existing shell-is-inert-to-profiles rule.
func createProfileOptionsFor(kind string, allowYolo bool) []string {
	caps, applicable := createAgentCapabilities(kind)
	if !applicable {
		return []string{"safe"}
	}
	options := make([]string, 0, len(caps.Profiles))
	for _, profile := range caps.Profiles {
		if profile == "yolo" && !allowYolo {
			continue
		}
		options = append(options, profile)
	}
	if len(options) == 0 {
		options = []string{"safe"}
	}
	return options
}

const createFieldCount = 8

// createFieldIsText reports whether field accepts free-typed runes, as
// opposed to being a cycled selection (agent, permission profile, login
// shell) that only left/right/space change.
func createFieldIsText(field int) bool {
	switch field {
	case 0, 1, 4, 5, 6:
		return true
	default:
		return false
	}
}

func cycleOption(options []string, current string, delta int) string {
	index := 0
	for i, option := range options {
		if option == current {
			index = i
			break
		}
	}
	index = (index + delta + len(options)) % len(options)
	return options[index]
}

// createAgentCapabilities returns the declared capabilities for kind and
// whether a permission profile is even applicable to it. shell has no
// notion of permission profiles at all (SPEC §5/§8): its create field is
// present for consistency but never validated, so a shell session is never
// rejected for an "unsupported" profile that simply does not apply to it.
func createAgentCapabilities(kind string) (agent.Caps, bool) {
	switch kind {
	case "claude":
		return agent.NewClaude().Capabilities(), true
	case "pi":
		return agent.NewPi().Capabilities(), true
	default:
		return agent.Caps{}, false
	}
}

// validateCreateFields checks the create modal's free-form fields (cwd,
// launch_args JSON, env pairs, and the selected permission profile) before
// any create call is attempted, returning a specific message for the first
// problem found and "" when the fields are acceptable. Name uniqueness and
// slug collisions are deliberately NOT checked here: those can only be
// known by the store at create time, and their specific messages already
// surface through the shellCreated error path (see createView).
//
// Retaining whatever the user typed is automatic here: this function never
// mutates m, so a non-empty result leaves every field exactly as typed.
func (m Model) validateCreateFields() string {
	if strings.TrimSpace(m.createCWD) == "" {
		return "working directory is required"
	}
	info, err := os.Stat(m.createCWD)
	if err != nil {
		return fmt.Sprintf("working directory %q does not exist", m.createCWD)
	}
	if !info.IsDir() {
		return fmt.Sprintf("working directory %q is not a directory", m.createCWD)
	}
	if strings.TrimSpace(m.createLaunchArgs) != "" {
		var args []string
		if err := json.Unmarshal([]byte(m.createLaunchArgs), &args); err != nil {
			return "launch_args must be a JSON array of strings: " + err.Error()
		}
	}
	if strings.TrimSpace(m.createEnv) != "" {
		for _, entry := range strings.Split(m.createEnv, ",") {
			entry = strings.TrimSpace(entry)
			if entry == "" {
				continue
			}
			key, _, ok := strings.Cut(entry, "=")
			if !ok || strings.TrimSpace(key) == "" {
				return fmt.Sprintf("env entry %q must be key=value", entry)
			}
		}
	}
	if caps, applicable := createAgentCapabilities(m.createAgent); applicable {
		if !caps.SupportsProfile(m.createProfile) {
			_, _, reason := caps.ResolveProfile(m.createAgent, m.createProfile)
			return "unsupported permission profile: " + reason
		}
	}
	if m.createProfile == "yolo" {
		if !m.settings.AllowYolo {
			return "yolo permission profile is not available: enable allow_yolo in config.toml"
		}
		if !m.createYoloConfirmed {
			return "yolo requires confirmation: press y on the Permission profile field, then Enter to create"
		}
	}
	return ""
}

func (m Model) updateCreate(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.creating, m.createError = false, ""
		return m, nil
	case "tab", "down":
		m.createField = (m.createField + 1) % createFieldCount
		return m, nil
	case "shift+tab", "up":
		m.createField = (m.createField - 1 + createFieldCount) % createFieldCount
		return m, nil
	case "left":
		m.cycleCreateField(-1)
		return m, nil
	case "right":
		m.cycleCreateField(1)
		return m, nil
	case " ":
		// Space toggles/cycles only the selection fields (agent, permission
		// profile, login shell); on text fields it is an ordinary typed
		// character (e.g. a name containing a space) and must fall through
		// to the append logic below, not be swallowed as a keybinding.
		if !createFieldIsText(m.createField) {
			m.cycleCreateField(1)
			return m, nil
		}
	case "y":
		// The yolo double-gate's explicit confirm keystroke; everywhere
		// else "y" is just a typed character and must fall through to the
		// append logic below (e.g. typing a name starting with "y").
		if m.createField == 3 && m.createProfile == "yolo" && m.settings.AllowYolo && !m.createYoloConfirmed {
			m.createYoloConfirmed = true
			return m, nil
		}
	case "enter":
		if msg := m.validateCreateFields(); msg != "" {
			m.createError = msg
			return m, nil
		}
		if m.createAgent != "shell" {
			m.createError = "creating " + m.createAgent + " sessions is not available yet"
			return m, nil
		}
		if m.create == nil {
			m.createError = "shell creation is unavailable"
			return m, nil
		}
		name, cwd := m.createName, m.createCWD
		return m, func() tea.Msg {
			session, err := m.create(context.Background(), service.ShellCreateInput{Name: name, CWD: cwd})
			return shellCreated{session: session, err: err}
		}
	case "backspace", "ctrl+h":
		m.backspaceCreateField()
		return m, nil
	}
	if runes := msg.Runes; len(runes) > 0 && createFieldIsText(m.createField) {
		switch m.createField {
		case 0:
			m.createName += string(runes)
		case 1:
			m.createCWD += string(runes)
		case 4:
			m.createLaunchArgs += string(runes)
		case 5:
			m.createEnv += string(runes)
		case 6:
			m.createPreLaunch += string(runes)
		}
	}
	return m, nil
}

// cycleCreateField advances a selection-type field's value by delta; it is a
// no-op on the free-text fields.
func (m *Model) cycleCreateField(delta int) {
	switch m.createField {
	case 2:
		m.createAgent = cycleOption(createAgentOptions, m.createAgent, delta)
		options := createProfileOptionsFor(m.createAgent, m.settings.AllowYolo)
		if !contains(options, m.createProfile) {
			m.createProfile = options[0]
			m.createYoloConfirmed = false
		}
	case 3:
		options := createProfileOptionsFor(m.createAgent, m.settings.AllowYolo)
		next := cycleOption(options, m.createProfile, delta)
		if next != m.createProfile {
			m.createYoloConfirmed = false
		}
		m.createProfile = next
	case 7:
		m.createLoginShell = !m.createLoginShell
	}
}

// contains reports whether options includes value.
func contains(options []string, value string) bool {
	for _, option := range options {
		if option == value {
			return true
		}
	}
	return false
}

func (m *Model) backspaceCreateField() {
	switch m.createField {
	case 0:
		if len(m.createName) > 0 {
			m.createName = m.createName[:len(m.createName)-1]
		}
	case 1:
		if len(m.createCWD) > 0 {
			m.createCWD = m.createCWD[:len(m.createCWD)-1]
		}
	case 4:
		if len(m.createLaunchArgs) > 0 {
			m.createLaunchArgs = m.createLaunchArgs[:len(m.createLaunchArgs)-1]
		}
	case 5:
		if len(m.createEnv) > 0 {
			m.createEnv = m.createEnv[:len(m.createEnv)-1]
		}
	case 6:
		if len(m.createPreLaunch) > 0 {
			m.createPreLaunch = m.createPreLaunch[:len(m.createPreLaunch)-1]
		}
	}
}

// createFieldRows describes the create modal's field set: label, current
// value renderer and a one-line explanation of what the field does. Keeping
// this as one table (rather than scattered Fprintf calls) is what lets a
// keyboard-only PTY test assert every label and its explanation are
// rendered together (task 015).
func (m Model) createFieldRows() []struct{ label, value, help string } {
	loginShell := "off"
	if m.createLoginShell {
		loginShell = "on"
	}
	profileOptions := createProfileOptionsFor(m.createAgent, m.settings.AllowYolo)
	profileValue := m.createProfile + " (left/right cycles: " + strings.Join(profileOptions, ", ") + ")"
	profileHelp := "how much the agent may do without asking; an unsupported profile degrades to safe"
	if !m.settings.AllowYolo {
		profileHelp += "; yolo is not offered because allow_yolo is not enabled in config.toml"
	} else if m.createProfile == "yolo" {
		if m.createYoloConfirmed {
			profileHelp += "; yolo confirmed"
		} else {
			profileHelp += "; press y to confirm yolo before creating"
		}
	}
	return []struct{ label, value, help string }{
		{"Name", m.createName, "the display name; also the source of the session's tmux slug"},
		{"Working directory", m.createCWD, "the session's cwd; must exist and be a directory"},
		{"Agent", m.createAgent + " (left/right cycles: shell, claude, pi)", "which coding agent adapter launches this session"},
		{"Permission profile", profileValue, profileHelp},
		{"Launch args (JSON array)", m.createLaunchArgs, "extra arguments appended verbatim after the adapter's own argv"},
		{"Env (key=value, comma-separated)", m.createEnv, "session-level environment variables, highest priority in PATH resolution"},
		{"Pre-launch command", m.createPreLaunch, "a command run in the pane before the agent starts, e.g. to load secrets"},
		{"Login shell", loginShell + " (space toggles)", "runs the pane through $SHELL -lc instead of execing the agent argv directly"},
	}
}

func (m Model) createView() string {
	marker := func(field int) string {
		if m.createField == field {
			return "> "
		}
		return "  "
	}
	var b strings.Builder
	title := "Create session"
	if m.createAgent == "shell" {
		title = "Create shell session"
	}
	b.WriteString(title + "\n\n")
	for field, row := range m.createFieldRows() {
		fmt.Fprintf(&b, "%s%s: %s\n    %s\n", marker(field), row.label, row.value, row.help)
	}
	b.WriteString("\nTab/Shift+Tab moves fields · Left/Right or Space changes a selection · Enter submits · Esc cancels\n")
	if m.createError != "" {
		if strings.Contains(m.createError, "collides with existing slug") {
			b.WriteString("\nCannot create session: name collides with existing slug.\n")
		} else {
			fmt.Fprintf(&b, "\nCannot create session: %s\n", m.createError)
		}
	}
	return b.String()
}

func helpView(ascii bool) string {
	text := `deck help

Keys
  ↑/↓ or j/k select a session
  ↵ attach the selected running session
  n create a shell session
  x kill the selected running session
  ? open/close help; Esc closes help
  q or Ctrl+C quit deck

Create dialog
  Tab or ↑/↓ changes field; ↵ advances or submits; Esc cancels

Runtime controls
  DECK_HOME             isolated data/config/state root
  DECK_TMUX_SOCKET      private tmux socket (default: deck)
  DECK_CLOCK            freeze wall clock (RFC3339)
  DECK_CLOCK_STEP       advance frozen clock after each successful shell creation
  DECK_ID_SEED          deterministic generated UUIDs
  DECK_RECONCILE_MS     list/reconciliation interval in milliseconds
  DECK_PREVIEW_MS       pane-preview interval in milliseconds
  DECK_ASCII=1          use ASCII instead of optional glyphs
  DECK_ANIM=0           disable animation
  DECK_COLOR            explicitly enable or disable colour
  NO_COLOR              disable colour

Sessions use a private tmux server. Inspect it with:
  tmux -L deck ls
(or replace deck with DECK_TMUX_SOCKET). Plain tmux attach does not find deck
sessions. Attach clears TMUX because nested tmux is unsupported. Attached
clients share one pane geometry; the most recently active client controls it.

? closes help; Esc closes help; q quits deck.
`
	if ascii {
		return strings.NewReplacer("↵", "Enter", "·", "-", "—", "-").Replace(text)
	}
	return text
}

// TmuxHealth verifies the runtime dependency without creating a tmux server.
func TmuxHealth(settings config.Settings) string {
	client := tmux.Client{Socket: settings.Socket}
	if _, err := client.Discover(context.Background()); err != nil {
		return err.Error()
	}
	return ""
}
