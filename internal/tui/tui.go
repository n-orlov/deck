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
	detail              bool
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
	createAgentSession  func(context.Context, service.AgentCreateInput) (store.Session, error)
	attach              func(context.Context, string) (*exec.Cmd, error)
	kill                func(context.Context, store.Session) error
	reconcile           func(context.Context) error
	resume              func(context.Context, string) (store.Session, service.ResumeOutcome, error)
	profileSwitch       func(context.Context, string, string) (store.Session, error)
	selected            int
	attachError         string
	resumeNote          string
	profileSwitching    bool
	profileSwitchValue  string
	profileSwitchNote   string
	profileSwitchYoloOK bool
	resumeMode          func(context.Context, string, string) (store.Session, error)
	pinning             bool
	pinValue            string
	pinNote             string
	width               int
	height              int
	agents              *agent.Registry
}

// defaultAgentRegistry returns the stock shell/claude/pi registry used when a
// caller does not supply one, so every existing constructor keeps working
// unchanged.
func defaultAgentRegistry() *agent.Registry {
	r := agent.NewRegistry()
	r.Register(agent.NewShell())
	r.Register(agent.NewClaude())
	r.Register(agent.NewPi())
	return r
}

// registry returns m.agents, falling back to defaultAgentRegistry() when the
// model was built without one (e.g. via New or any constructor that predates
// registry support).
func (m Model) registry() *agent.Registry {
	if m.agents != nil {
		return m.agents
	}
	return defaultAgentRegistry()
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

type sessionResumed struct {
	session store.Session
	outcome service.ResumeOutcome
	err     error
}

type profileSwitched struct {
	session store.Session
	err     error
}

type resumeModeChanged struct {
	session store.Session
	err     error
}

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
	return NewWithShellCreatorAttacherKillerReconcilerAndResumer(db, settings, tmuxNote, creator, attacher, killer, reconciler, nil)
}

// NewWithShellCreatorAttacherKillerReconcilerAndResumer adds the `r` resume
// action (SPEC §8/§9.3). A resumed row is left `starting`; the TUI never
// renders `running` for it, and a caller that lost the launch-lease race
// (service.ResumeStartingElsewhere) is shown "starting elsewhere", not an
// error.
func NewWithShellCreatorAttacherKillerReconcilerAndResumer(db *store.Store, settings config.Settings, tmuxNote string, creator func(context.Context, service.ShellCreateInput) (store.Session, error), attacher func(context.Context, string) (*exec.Cmd, error), killer func(context.Context, store.Session) error, reconciler func(context.Context) error, resumer func(context.Context, string) (store.Session, service.ResumeOutcome, error)) Model {
	return NewWithShellCreatorAttacherKillerResumerAndProfileSwitcher(db, settings, tmuxNote, creator, attacher, killer, reconciler, resumer, nil)
}

// NewWithShellCreatorAttacherKillerResumerAndProfileSwitcher adds the `P`
// profile-switch action (SPEC §5/§8, task 020). Switching a session's
// permission profile only ever persists the new value; it never touches a
// live pane, and the TUI states plainly that the change applies on the
// session's next launch/restart rather than implying the running agent's
// mode changed.
func NewWithShellCreatorAttacherKillerResumerAndProfileSwitcher(db *store.Store, settings config.Settings, tmuxNote string, creator func(context.Context, service.ShellCreateInput) (store.Session, error), attacher func(context.Context, string) (*exec.Cmd, error), killer func(context.Context, store.Session) error, reconciler func(context.Context) error, resumer func(context.Context, string) (store.Session, service.ResumeOutcome, error), profileSwitcher func(context.Context, string, string) (store.Session, error)) Model {
	return NewWithShellCreatorAttacherKillerResumerProfileSwitcherAndResumeModer(db, settings, tmuxNote, creator, attacher, killer, reconciler, resumer, profileSwitcher, nil)
}

// NewWithShellCreatorAttacherKillerResumerProfileSwitcherAndResumeModer adds
// the `p` pin/start-fresh dialog (SPEC §8/§9.3, task 021). resumeModer picks
// resume_state ("pinned" pins to the session's own current conversation id,
// sticky across a deck restart; "fresh-once" arms exactly one fresh
// conversation launch and reverts to "auto" once that launch has actually
// happened; "auto" clears any pin). None of these ever touch a live pane;
// they only take effect on the session's next resume.
func NewWithShellCreatorAttacherKillerResumerProfileSwitcherAndResumeModer(db *store.Store, settings config.Settings, tmuxNote string, creator func(context.Context, service.ShellCreateInput) (store.Session, error), attacher func(context.Context, string) (*exec.Cmd, error), killer func(context.Context, store.Session) error, reconciler func(context.Context) error, resumer func(context.Context, string) (store.Session, service.ResumeOutcome, error), profileSwitcher func(context.Context, string, string) (store.Session, error), resumeModer func(context.Context, string, string) (store.Session, error)) Model {
	return NewWithShellCreatorAttacherKillerResumerProfileSwitcherResumeModerAndAgentCreator(db, settings, tmuxNote, creator, attacher, killer, reconciler, resumer, profileSwitcher, resumeModer, nil)
}

// NewWithShellCreatorAttacherKillerResumerProfileSwitcherResumeModerAndAgentCreator
// adds real coding-agent creation (task 022) to the create modal's Enter
// handler. Until agentCreator is wired, choosing claude/pi in the create
// modal keeps the pre-task-022 "not available yet" refusal (agentCreator ==
// nil); once wired, Enter on a non-shell Agent field calls
// service.CreateAgent with every other create-modal field (permission
// profile, launch_args, env, pre_launch, login_shell) exactly as typed.
func NewWithShellCreatorAttacherKillerResumerProfileSwitcherResumeModerAndAgentCreator(db *store.Store, settings config.Settings, tmuxNote string, creator func(context.Context, service.ShellCreateInput) (store.Session, error), attacher func(context.Context, string) (*exec.Cmd, error), killer func(context.Context, store.Session) error, reconciler func(context.Context) error, resumer func(context.Context, string) (store.Session, service.ResumeOutcome, error), profileSwitcher func(context.Context, string, string) (store.Session, error), resumeModer func(context.Context, string, string) (store.Session, error), agentCreator func(context.Context, service.AgentCreateInput) (store.Session, error)) Model {
	m := New(db, settings, tmuxNote)
	m.create, m.attach, m.kill, m.reconcile, m.resume, m.profileSwitch, m.resumeMode, m.createAgentSession = creator, attacher, killer, reconciler, resumer, profileSwitcher, resumeModer, agentCreator
	return m
}

// NewWithShellCreatorAttacherKillerResumerProfileSwitcherResumeModerAgentCreatorAndRegistry
// is identical to
// NewWithShellCreatorAttacherKillerResumerProfileSwitcherResumeModerAndAgentCreator
// but additionally accepts the agent registry that drives the create
// modal's Agent field and capability lookups (SPEC requirement: adding an
// adapter must not require touching internal/tui). When registry is nil the
// model falls back to defaultAgentRegistry() (shell/claude/pi), so this is a
// pure superset of the existing constructor.
func NewWithShellCreatorAttacherKillerResumerProfileSwitcherResumeModerAgentCreatorAndRegistry(db *store.Store, settings config.Settings, tmuxNote string, creator func(context.Context, service.ShellCreateInput) (store.Session, error), attacher func(context.Context, string) (*exec.Cmd, error), killer func(context.Context, store.Session) error, reconciler func(context.Context) error, resumer func(context.Context, string) (store.Session, service.ResumeOutcome, error), profileSwitcher func(context.Context, string, string) (store.Session, error), resumeModer func(context.Context, string, string) (store.Session, error), agentCreator func(context.Context, service.AgentCreateInput) (store.Session, error), registry *agent.Registry) Model {
	m := NewWithShellCreatorAttacherKillerResumerProfileSwitcherResumeModerAndAgentCreator(db, settings, tmuxNote, creator, attacher, killer, reconciler, resumer, profileSwitcher, resumeModer, agentCreator)
	m.agents = registry
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
	case sessionResumed:
		if msg.err != nil {
			m.attachError = "Cannot resume: " + msg.err.Error()
			m.resumeNote = ""
			return m, nil
		}
		m.attachError = ""
		if msg.outcome == service.ResumeStartingElsewhere {
			m.resumeNote = "starting elsewhere"
			return m, nil
		}
		m.resumeNote = ""
		return m, m.loadSessions
	case profileSwitched:
		if msg.err != nil {
			m.profileSwitchNote = "Cannot change permission profile: " + msg.err.Error()
			return m, nil
		}
		m.profileSwitching = false
		m.profileSwitchNote = ""
		return m, m.loadSessions
	case resumeModeChanged:
		if msg.err != nil {
			m.pinNote = "Cannot change resume mode: " + msg.err.Error()
			return m, nil
		}
		m.pinning = false
		m.pinNote = ""
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
		if m.profileSwitching {
			return m.updateProfileSwitch(msg)
		}
		if m.pinning {
			return m.updatePinDialog(msg)
		}
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "?":
			m.help = !m.help
		case "esc":
			m.help = false
			m.detail = false
		case "i":
			if !m.help && len(m.sessions) > 0 {
				m.detail = !m.detail
			}
		case "n":
			if !m.help {
				m.creating, m.createError, m.createField = true, "", 0
				m.createName, m.createCWD = "", ""
				m.createAgent, m.createProfile = m.registry().Kinds()[0], createProfileOptions[0]
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
		case "r":
			if m.resume == nil || len(m.sessions) == 0 {
				return m, nil
			}
			session := m.sessions[m.selected]
			if session.Status != "stopped" {
				m.attachError = "Cannot resume: session is not stopped"
				return m, nil
			}
			sessionID := session.ID
			return m, func() tea.Msg {
				resumed, outcome, err := m.resume(context.Background(), sessionID)
				return sessionResumed{session: resumed, outcome: outcome, err: err}
			}
		case "P":
			if m.profileSwitch == nil || len(m.sessions) == 0 {
				return m, nil
			}
			session := m.sessions[m.selected]
			if _, applicable := createAgentCapabilities(session.Agent); !applicable {
				m.attachError = "Cannot change permission profile: " + session.Agent + " has no permission profile"
				return m, nil
			}
			m.profileSwitching = true
			m.profileSwitchValue = session.PermissionProfile
			m.profileSwitchNote = ""
			m.profileSwitchYoloOK = false
			return m, nil
		case "p":
			if m.resumeMode == nil || len(m.sessions) == 0 {
				return m, nil
			}
			session := m.sessions[m.selected]
			caps, applicable := createAgentCapabilities(session.Agent)
			if !applicable || !caps.AssignsConversationID {
				m.attachError = "Cannot change resume mode: " + session.Agent + " has no conversation id to pin or restart fresh"
				return m, nil
			}
			m.pinning = true
			m.pinValue = session.ResumeState
			if m.pinValue == "" {
				m.pinValue = "auto"
			}
			m.pinNote = ""
			return m, nil
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
	if m.profileSwitching && len(m.sessions) > 0 {
		return m.profileSwitchView()
	}
	if m.pinning && len(m.sessions) > 0 {
		return m.pinView()
	}
	if m.detail && len(m.sessions) > 0 {
		return m.detailView()
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
			if status == "starting" {
				status = "starting" + m.glyph(" · awaiting signal", " - awaiting signal")
			}
			marker := "  "
			if index == m.selected {
				marker = "> "
			}
			badge := profileBadge(session)
			fmt.Fprintf(&b, "%s%s  %-10s %-8s %s  created %s\n    %s\n", marker, session.Name, session.Agent, badge, status, m.relativeTime(session.CreatedAt), session.CWD)
		}
	}
	if m.attachError != "" {
		fmt.Fprintf(&b, "\n%s\n", m.attachError)
	}
	if m.resumeNote != "" {
		fmt.Fprintf(&b, "\n%s\n", m.resumeNote)
	}
	b.WriteString("\n" + m.glyph("↑/↓ · ↵ attach · n new · x kill · r resume · P profile · p pin · i detail · ? help · q quit", "up/down - Enter attach - n new - x kill - r resume - P profile - p pin - i detail - ? help - q quit") + "\n")
	return b.String()
}

// profileBadge renders the bracketed permission-profile badge shown next to
// an agent row in the session list. Shell sessions have no notion of a
// permission profile at all (SPEC §5/§8): createAgentCapabilities reports
// them as not applicable, so they render no badge rather than a meaningless
// cosmetic "safe".
func profileBadge(session store.Session) string {
	if _, applicable := createAgentCapabilities(session.Agent); !applicable {
		return ""
	}
	return "[" + session.PermissionProfile + "]"
}

// updateProfileSwitch handles keys while the `P` permission-profile switch
// dialog is open (task 020). It only ever cycles a locally-held candidate
// value and, on confirmation, persists it through m.profileSwitch; it never
// issues any argv to the selected session's pane, live or otherwise.
func (m Model) updateProfileSwitch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	session := m.sessions[m.selected]
	options := createProfileOptionsFor(session.Agent, m.settings.AllowYolo)
	switch msg.String() {
	case "esc":
		m.profileSwitching = false
		m.profileSwitchNote = ""
		return m, nil
	case "left":
		if m.profileSwitchValue != "yolo" {
			m.profileSwitchYoloOK = false
		}
		m.profileSwitchValue = cycleOption(options, m.profileSwitchValue, -1)
		return m, nil
	case "right", " ":
		if m.profileSwitchValue != "yolo" {
			m.profileSwitchYoloOK = false
		}
		m.profileSwitchValue = cycleOption(options, m.profileSwitchValue, 1)
		return m, nil
	case "y":
		// The yolo double-gate's explicit confirm keystroke, mirroring the
		// create modal's "y" (SPEC §5: switching to yolo requires the same
		// explicit confirm as creating with yolo).
		if m.profileSwitchValue == "yolo" && m.settings.AllowYolo && !m.profileSwitchYoloOK {
			m.profileSwitchYoloOK = true
			m.profileSwitchNote = ""
			return m, nil
		}
	case "enter":
		if m.profileSwitch == nil {
			m.profileSwitchNote = "changing the permission profile is unavailable"
			return m, nil
		}
		if m.profileSwitchValue == "yolo" && !m.profileSwitchYoloOK {
			m.profileSwitchNote = "yolo requires confirmation: press y, then Enter to switch"
			return m, nil
		}
		sessionID, profile := session.ID, m.profileSwitchValue
		return m, func() tea.Msg {
			updated, err := m.profileSwitch(context.Background(), sessionID, profile)
			return profileSwitched{session: updated, err: err}
		}
	}
	return m, nil
}

// profileSwitchView renders the `P` permission-profile switch dialog. It
// states plainly that the change only applies on the session's next
// launch/restart; it never claims the live pane's mode changed (SPEC §5).
func (m Model) profileSwitchView() string {
	session := m.sessions[m.selected]
	options := createProfileOptionsFor(session.Agent, m.settings.AllowYolo)
	var b strings.Builder
	fmt.Fprintf(&b, "Change permission profile for %s\n\n", session.Name)
	fmt.Fprintf(&b, "Current:   %s\n", session.PermissionProfile)
	fmt.Fprintf(&b, "New:       %s (left/right cycles: %s)\n", m.profileSwitchValue, strings.Join(options, ", "))
	b.WriteString("\nThis applies on the session's next launch/restart; it does not change a\nrunning pane's mode.\n")
	if m.profileSwitchValue == "yolo" && m.settings.AllowYolo && !m.profileSwitchYoloOK {
		b.WriteString("\nyolo requires confirmation: press y, then Enter to switch\n")
	}
	b.WriteString("\nLeft/Right cycles · Enter confirms · Esc cancels\n")
	if m.profileSwitchNote != "" {
		fmt.Fprintf(&b, "\n%s\n", m.profileSwitchNote)
	}
	return b.String()
}

// resumeModeOptions lists the SPEC §8/§9.3 resume_state choices the `p`
// dialog cycles through: auto (resume the session's own last-known
// conversation), pinned (always resume the session's own current
// conversation id, sticky across a deck restart), and fresh-once (start a
// brand-new conversation exactly once, then revert to auto).
var resumeModeOptions = []string{"auto", "pinned", "fresh-once"}

// updatePinDialog handles keys while the `p` pin/start-fresh dialog is open
// (task 021). It only ever cycles a locally-held candidate resume_state
// and, on confirmation, persists it through m.resumeMode; it never issues
// any argv to the selected session's pane, live or otherwise, and takes
// effect only on the session's next resume.
func (m Model) updatePinDialog(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	session := m.sessions[m.selected]
	switch msg.String() {
	case "esc":
		m.pinning = false
		m.pinNote = ""
		return m, nil
	case "left":
		m.pinValue = cycleOption(resumeModeOptions, m.pinValue, -1)
		return m, nil
	case "right", " ":
		m.pinValue = cycleOption(resumeModeOptions, m.pinValue, 1)
		return m, nil
	case "enter":
		if m.resumeMode == nil {
			m.pinNote = "changing the resume mode is unavailable"
			return m, nil
		}
		sessionID, mode := session.ID, m.pinValue
		return m, func() tea.Msg {
			updated, err := m.resumeMode(context.Background(), sessionID, mode)
			return resumeModeChanged{session: updated, err: err}
		}
	}
	return m, nil
}

// pinView renders the `p` pin/start-fresh dialog. "pinned" always resumes
// the session's own current conversation id, sticky across a deck restart;
// "fresh-once" starts a brand-new conversation exactly once and then
// reverts to auto; neither ever touches a live pane.
func (m Model) pinView() string {
	session := m.sessions[m.selected]
	state := session.ResumeState
	if state == "" {
		state = "auto"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Change resume mode for %s\n\n", session.Name)
	fmt.Fprintf(&b, "Current:   %s\n", state)
	fmt.Fprintf(&b, "New:       %s (left/right cycles: %s)\n", m.pinValue, strings.Join(resumeModeOptions, ", "))
	b.WriteString("\npinned always resumes this session's own current conversation id, sticky\nacross a deck restart. fresh-once starts a brand-new conversation exactly\nonce, then reverts to auto. Neither changes a running pane.\n")
	b.WriteString("\nLeft/Right cycles · Enter confirms · Esc cancels\n")
	if m.pinNote != "" {
		fmt.Fprintf(&b, "\n%s\n", m.pinNote)
	}
	return b.String()
}

// detailView renders the selected session's full detail, including an
// explicit degradation sentence when the adapter could not honour the
// originally requested permission profile (SPEC §5: "say so in the row
// detail rather than silently lying").
func (m Model) detailView() string {
	session := m.sessions[m.selected]
	var b strings.Builder
	fmt.Fprintf(&b, "%s detail\n\n", session.Name)
	fmt.Fprintf(&b, "Agent:              %s\n", session.Agent)
	fmt.Fprintf(&b, "Working directory:  %s\n", session.CWD)
	status := session.Status
	if status == "stopped" {
		status += m.glyph(" · resumable", " - resumable")
	}
	if status == "starting" {
		status = "starting" + m.glyph(" · awaiting signal", " - awaiting signal")
	}
	fmt.Fprintf(&b, "Status:             %s\n", status)
	if _, applicable := createAgentCapabilities(session.Agent); applicable {
		fmt.Fprintf(&b, "Permission profile: %s\n", session.PermissionProfile)
		if session.PermissionProfileReason != "" {
			fmt.Fprintf(&b, "  degraded: %s\n", session.PermissionProfileReason)
		}
	} else {
		b.WriteString("Permission profile: n/a (shell has no permission profile)\n")
	}
	if session.ConversationID != "" {
		fmt.Fprintf(&b, "Conversation id:    %s\n", session.ConversationID)
	}
	b.WriteString("\n" + m.glyph("i or Esc closes detail", "i or Esc closes detail") + "\n")
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

// createProfileOptions is the values the create modal's Permission profile
// field cycles through. The Agent field instead cycles m.registry().Kinds(),
// so adding an adapter to the registry never requires an internal/tui edit
// (PRD requirement 1). createProfileOptions is the master ordering used
// only as the initial default ("safe") when the modal opens; the actual
// offered set while the modal is open is narrowed per selected agent and
// per allow_yolo by createProfileOptionsFor (SPEC §5, task 017).
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

// parseCreateLaunchArgs parses the create modal's launch_args field into the
// JSON array of strings service.AgentCreateInput expects. An empty field is
// simply no extra args, matching validateCreateFields' own "" is fine rule.
func parseCreateLaunchArgs(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var args []string
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return nil, fmt.Errorf("launch_args must be a JSON array of strings: %w", err)
	}
	return args, nil
}

// parseCreateEnv parses the create modal's comma-separated key=value env
// field into the map service.AgentCreateInput expects, mirroring the same
// splitting rule validateCreateFields already checked.
func parseCreateEnv(raw string) (map[string]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	env := make(map[string]string)
	for _, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		key, value, ok := strings.Cut(entry, "=")
		if !ok || strings.TrimSpace(key) == "" {
			return nil, fmt.Errorf("env entry %q must be key=value", entry)
		}
		env[strings.TrimSpace(key)] = value
	}
	return env, nil
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
			if m.createAgentSession == nil {
				m.createError = "creating " + m.createAgent + " sessions is not available yet"
				return m, nil
			}
			launchArgs, err := parseCreateLaunchArgs(m.createLaunchArgs)
			if err != nil {
				m.createError = err.Error()
				return m, nil
			}
			env, err := parseCreateEnv(m.createEnv)
			if err != nil {
				m.createError = err.Error()
				return m, nil
			}
			input := service.AgentCreateInput{
				Name: m.createName, CWD: m.createCWD, Agent: m.createAgent,
				PermissionProfile: m.createProfile, LaunchArgs: launchArgs, Env: env,
				PreLaunch: m.createPreLaunch, LoginShell: m.createLoginShell,
			}
			return m, func() tea.Msg {
				session, err := m.createAgentSession(context.Background(), input)
				return shellCreated{session: session, err: err}
			}
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
		m.createAgent = cycleOption(m.registry().Kinds(), m.createAgent, delta)
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
		{"Agent", m.createAgent + " (left/right cycles: " + strings.Join(m.registry().Kinds(), ", ") + ")", "which coding agent adapter launches this session"},
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
  n create a session (shell, or an agent: claude or pi)
  x kill the selected running session
  r resume the selected stopped session with its own agent argv (never
    --continue or "most recent"); a resumed row reads "starting · awaiting signal",
    not "running" — deck cannot yet tell when the agent is ready (a Phase 2
    rough edge); a client that loses the launch-lease race sees
    "starting elsewhere" instead of an error
  P switch the permission profile of the selected session; only takes
    effect on the next launch or resume ("restart to apply"), never the
    live pane
  p pin the selected session's conversation id so future resumes always
    reuse it, or launch a one-shot fresh conversation (reverts to normal
    auto-resume afterward, it does not stay pinned or cleared)
  i toggle detail view for the selected session
  ? open/close help; Esc closes help
  q or Ctrl+C quit deck

Create dialog fields
  Name                the display name; also the source of the tmux slug
  Working directory    the session's cwd; must exist and be a directory
  Agent               shell, claude, or pi; which adapter launches the session
  Permission profile  how much the agent may do without asking (safe, plan,
                      edits, yolo); an unsupported profile for the chosen
                      agent degrades visibly instead of silently
  Launch args         extra argv appended after the adapter's own argv
  Env                 session-level environment variables (highest priority
                      in PATH resolution)
  Pre-launch command  runs in the pane before the agent starts; the SPEC's
                      intended use is loading secrets into the pane's
                      environment without deck ever storing or logging them
  Login shell         run the pane via $SHELL -lc instead of execing the
                      agent argv directly
  Tab or ↑/↓ changes field; ↵ advances or submits; Esc cancels

Yolo is gated twice: allow_yolo must be enabled in config.toml, or yolo is
not offered at all (the UI states why); and even when enabled, choosing
yolo — at create time or when switching profile with P — requires an
explicit y confirm keystroke before it takes effect.

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
