// Package tui implements deck's interactive terminal interface.
package tui

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/service"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// Model is the base session-list screen. Later modal and action work extends
// this model rather than providing a separate command-line interface.
type Model struct {
	store       *store.Store
	settings    config.Settings
	sessions    []store.Session
	startupNote string
	help        bool
	creating    bool
	createName  string
	createCWD   string
	createField int
	createError string
	create      func(context.Context, service.ShellCreateInput) (store.Session, error)
	attach      func(context.Context, string) (*exec.Cmd, error)
	kill        func(context.Context, store.Session) error
	reconcile   func(context.Context) error
	selected    int
	attachError string
	width       int
	height      int
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

func (m Model) updateCreate(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.creating, m.createError = false, ""
		return m, nil
	case "tab", "shift+tab", "up", "down":
		m.createField = 1 - m.createField
		return m, nil
	case "enter":
		if m.createField == 0 {
			m.createField = 1
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
		if m.createField == 0 && len(m.createName) > 0 {
			m.createName = m.createName[:len(m.createName)-1]
		} else if m.createField == 1 && len(m.createCWD) > 0 {
			m.createCWD = m.createCWD[:len(m.createCWD)-1]
		}
		return m, nil
	}
	if runes := msg.Runes; len(runes) > 0 {
		if m.createField == 0 {
			m.createName += string(runes)
		} else {
			m.createCWD += string(runes)
		}
	}
	return m, nil
}

func (m Model) createView() string {
	marker := func(field int) string {
		if m.createField == field {
			return "> "
		}
		return "  "
	}
	var b strings.Builder
	b.WriteString("Create shell session\n\n")
	fmt.Fprintf(&b, "%sName: %s\n", marker(0), m.createName)
	fmt.Fprintf(&b, "%sWorking directory: %s\n", marker(1), m.createCWD)
	b.WriteString("\nTab moves fields · Enter advances/submits · Esc cancels\n")
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
