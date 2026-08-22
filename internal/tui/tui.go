// Package tui implements deck's interactive terminal interface.
package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/agent"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/service"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/theme"
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
	prepareAttach       func(context.Context, string) error
	kill                func(context.Context, store.Session) error
	acknowledge         func(context.Context, string) error
	reconcile           func(context.Context) error
	resume              func(context.Context, string) (store.Session, service.ResumeOutcome, error)
	profileSwitch       func(context.Context, string, string) (store.Session, error)
	selected            int
	collapsedGroups     map[string]bool
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
	// settingsOpen is task 013's `,` full-screen takeover (SPEC §11.5): a
	// category list and the selected category's field list, both walking
	// config.Schema rather than a hand-written field set. It is not a
	// §11.4 dialog (no framedDialog, no shared dialog-contract retrofit --
	// see task 029) and replaces the whole frame the same way m.creating
	// etc already do, so it is checked in the same tea.KeyMsg/View()
	// early-return chain as those, never layered atop mainView.
	settingsOpen bool
	// settingsCategoryIndex/settingsFieldIndex select the left list's
	// category and, within it, the right list's field. Both default to 0
	// ("General", its first field) on open; task 014 adds the tab/left/
	// right/up/down navigation that moves them and the `/` search that
	// can jump either. This task only renders whichever index the state
	// holds, never hand-picks what to render.
	settingsCategoryIndex int
	settingsFieldIndex    int
	// settingsFocus is task 014's tab/left/right target: settingsFocus
	// Categories while the category list drives up/down, settingsFocus
	// Fields once tab/left/right has moved it to the field list — see
	// SPEC §11.5: "tab/left/right switch between the category list and the
	// field list, up/down move within the focused list."
	settingsFocus int
	// settingsSearchActive/settingsSearchQuery/settingsSearchIndex are
	// task 014's `/` fuzzy search: settingsSearchActive is true while the
	// query is being typed and results browsed, settingsSearchQuery holds
	// the typed text, settingsSearchIndex selects among the matches
	// settingsSearchMatches(settingsSearchQuery) returns (in schema order,
	// searched across every category, per §11.5's "search every field by
	// label and description").
	settingsSearchActive bool
	settingsSearchQuery  string
	settingsSearchIndex  int
	// settingsEdits is task 015's staged working copy of every schema field's
	// value: a config.FileConfig seeded from m.settings when the takeover
	// opens (settingsEditsFromSettings) and mutated in place as toggle/
	// integer/enum fields are edited. Task 016 hands this to
	// config.WriteConfigFile on save; nothing here writes config.toml or
	// changes a live pane on its own -- editing only ever changes this
	// staged copy, never m.settings itself, so a field labelled
	// restart-to-apply genuinely cannot affect an already-running pane
	// through this path alone.
	settingsEdits config.FileConfig
	// settingsSavedEdits is task 016's "what is actually on disk right
	// now" snapshot: seeded from the same settingsEditsFromSettings call
	// that seeds settingsEdits when `,` opens, and replaced with a copy of
	// settingsEdits only after a successful config.WriteConfigFile. esc
	// compares settingsEdits against THIS, not against m.settings itself
	// (m.settings is only ever refreshed by a restart -- see field kind's
	// own "restart-to-apply" scope, req 19 -- so it would still disagree
	// with settingsEdits right after a save that changed nothing else,
	// wrongly re-prompting to discard a change that is already safely on
	// disk).
	settingsSavedEdits config.FileConfig
	// settingsDiscardConfirm is true while esc's "you have unsaved
	// changes" prompt (SPEC requirement 14/20) is showing: y/enter
	// discards (closes the takeover, leaving config.toml exactly as
	// settingsSavedEdits already has it -- never written to), n/esc
	// dismisses the prompt and returns to editing.
	settingsDiscardConfirm bool
	// settingsNote surfaces ctrl+s's outcome (task 012's atomic writer can
	// fail -- e.g. an unwritable config directory -- and that failure must
	// be visible, not swallowed) and the discard prompt's own text.
	// Mirrors profileSwitchNote/pinNote's existing shape in this package.
	settingsNote string
	// settingsEnvOpen/settingsEnvIndex/settingsEnvEditing/
	// settingsEnvEditingKeyPart/settingsEnvEditKey/settingsEnvEditValue/
	// settingsEnvEditOriginalKey are task 003's [env] entry editor (SPEC
	// requirement 17: the global [env] table is genuinely editable in the
	// takeover, not display-only). settingsEnvOpen is true while the
	// entries list (one row per settingsEdits.Env key, plus a trailing
	// "add entry" row) has taken over the field panel; settingsEnvIndex
	// selects a row in that list. settingsEnvEditing is true while a
	// single entry's key or value is being typed; settingsEnvEditingKeyPart
	// says which of the two free-text buffers (settingsEnvEditKey/
	// settingsEnvEditValue) is currently receiving typed runes.
	// settingsEnvEditOriginalKey holds the key being edited (empty when
	// adding a new entry), so committing a renamed key removes the old one
	// rather than leaving both. All of this only ever mutates
	// settingsEdits.Env, the same staged copy every other field kind edits
	// -- ctrl+s/esc still govern when (or whether) it reaches config.toml.
	settingsEnvOpen            bool
	settingsEnvIndex           int
	settingsEnvEditing         bool
	settingsEnvEditingKeyPart  bool
	settingsEnvEditKey         string
	settingsEnvEditValue       string
	settingsEnvEditOriginalKey string
	// themePicking is task 025's `t` picker (SPEC §11.6, requirement 27): it
	// does NOT replace the whole frame the way m.creating/m.settingsOpen do
	// -- the point of the picker is that the REAL session list stays on
	// screen and is coloured with whatever theme is currently highlighted
	// (activeTheme() consults themePickerValue while this is true), so
	// mainView keeps rendering and simply grows a themePickerLines() banner
	// (mirroring themeBanner/attachErrorLines' own "extra reserved rows"
	// shape) rather than gaining a second, separate full-screen view.
	themePicking bool
	// themePickerValue is the theme NAME currently highlighted in the
	// picker (never the *theme.Theme itself, so "not found" -- an empty
	// list, or a name that raced a user-theme file being deleted mid-pick
	// -- is representable and handled rather than a nil pointer). esc never
	// writes this anywhere durable; only themePickerConfirm (enter) does,
	// and only after resolving it back to a concrete theme.
	themePickerValue string
	// themePickerNote surfaces enter's outcome (a failed config.toml write,
	// or an empty list making enter a no-op), mirroring profileSwitchNote/
	// pinNote/settingsNote's identical shape elsewhere in this package.
	themePickerNote string
	width           int
	height          int
	agents          *agent.Registry
	// layoutMode and sidebarWidth are the §11.2 layout pin and the
	// persisted sidebar width, both persisted to state.db's ui_state table
	// by persistLayoutMode/persistSidebarWidth (task 016). "" and 0 both
	// mean "use the default", which ComputeLayout already treats as
	// auto/35 respectively.
	layoutMode   string
	sidebarWidth int
	// preCollapseLayoutMode and layoutCycleActive track requirement 33's
	// "click the collapsed strip -> restores the previous non-collapsed
	// mode": preCollapseLayoutMode is the pin the user actually had in
	// force before they started the *current* `|` excursion (not merely
	// the mode one hop back -- side-by-side -> stacked -> collapsed still
	// remembers side-by-side, the mode the excursion began from, not the
	// stacked stop it passed through on the way). layoutCycleActive is
	// true for as long as that excursion is still under way (it clears
	// once `|` wraps back to auto, or the strip is clicked, so the next
	// press anchors fresh from wherever the pin then stands). Both are
	// only read/written by cycleLayoutMode and restoreFromCollapsedStrip
	// -- unset (""/false) means "nothing recorded yet, restore to auto",
	// the same default every other unset pin uses.
	preCollapseLayoutMode string
	layoutCycleActive     bool
	// previewCapture is the read-only preview capture engine (SPEC
	// requirements 21, 22): given the selected row's slug, it returns
	// exactly one tmux.PreviewCapture per call, never attaching a client,
	// spawning a control-mode server, using pipe-pane, or resizing a pane.
	// nil (the zero value, and every constructor below New) leaves the
	// preview exactly as inert as before this task landed.
	previewCapture func(context.Context, string) (tmux.PreviewCapture, error)
	// previewSessionID, previewLive and previewBytes hold the most recent
	// successful capture, keyed by the session it was captured for so a
	// stale reply arriving after the selection moved on is never rendered
	// against the wrong row. Rendering these (crop, geometry line, cell-aware
	// truncation) is tasks 018-021; this task only maintains them.
	previewSessionID string
	previewLive      bool
	previewBytes     []byte
	// previewPaneWidth/previewPaneHeight are the real tmux pane geometry at
	// the moment previewBytes was captured (SPEC requirement 23's "45x22 of
	// 120x40" line, task 018) — independent of previewBytes' own line count,
	// since tmux trims trailing blank lines/columns from a capture.
	previewPaneWidth  int
	previewPaneHeight int
	// sidebarScroll is the wheel-scroll offset into the sidebar's own
	// content lines (SPEC §11.8, task 028): it moves which lines the
	// panel shows without ever touching m.selected, and is clamped at
	// render/scroll time (never negative, never past the point where the
	// last line would leave the panel's bottom empty).
	sidebarScroll int
	// draggingSeam is true between a mouse press on the side-by-side
	// seam column and its matching release (SPEC §11.8): while true, a
	// motion event live-adjusts sidebarWidth the same way `<`/`>` do.
	draggingSeam bool
	// lastClickAt/lastClickIndex track the previous sidebar-row press so
	// a second press on the same row shortly after is resolved as a
	// double-click (attach) rather than two independent single clicks
	// (select). Neither field is persisted or read anywhere else.
	lastClickAt    time.Time
	lastClickIndex int
}

// doubleClickWindow is the maximum gap between two presses on the same
// sidebar row that still counts as a double-click (SPEC §11.8).
const doubleClickWindow = 500 * time.Millisecond

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

// defaultCreateAgent picks which registered kind the create modal opens on.
// "shell" is the only adapter that currently supports real session creation
// (see the createAgent != "shell" guard below), so it is preferred whenever
// present; otherwise the first kind in the registry's stable order is used.
// This keeps the modal's opening selection independent of where "shell"
// happens to sort alphabetically among registered kinds.
func defaultCreateAgent(kinds []string) string {
	for _, k := range kinds {
		if k == "shell" {
			return k
		}
	}
	if len(kinds) > 0 {
		return kinds[0]
	}
	return ""
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

type sessionAcknowledged struct{ err error }

// previewCaptured reports one previewCapture tick's result for the session
// selected at the moment the capture was issued (SPEC requirements 21, 22).
// err is only ever a real tmux/transport failure; a session with no live
// pane is reported as capture.Live == false with err == nil, never as an
// error, so a stopped/starting/archived selection does not spam attachError
// every tick.
type previewCaptured struct {
	sessionID string
	capture   tmux.PreviewCapture
	err       error
}

// uiStatePersisted reports the outcome of persisting layout_mode or
// sidebar_width to state.db's ui_state table after a `|`/`<`/`>` keypress
// (task 016, SPEC §11.2). It never touches config.toml.
type uiStatePersisted struct{ err error }

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
	m := Model{store: db, settings: settings, startupNote: tmuxNote}
	if db != nil {
		m.acknowledge = db.AcknowledgeSession
		m.prepareAttach = func(ctx context.Context, sessionID string) error {
			if settings.Clock == nil {
				return fmt.Errorf("attach status clock is unavailable")
			}
			return db.RecordAttachment(ctx, sessionID, settings.Clock.Now().UnixMilli())
		}
		// Task 016: layout_mode and sidebar_width (SPEC §11.2) live only in
		// state.db's ui_state table, read once here so a restarted client
		// (a fresh New(db, ...) call, exactly as acknowledge_test.go's
		// "restarted" model simulates it) renders the persisted pin/width
		// rather than falling back to the in-memory zero values. A read
		// failure is not load-bearing: ui_state degrades to the Go zero
		// value (""/0), which ComputeLayout already treats as auto/default.
		ctx := context.Background()
		if mode, err := db.GetLayoutMode(ctx); err == nil {
			m.layoutMode = mode
		}
		if width, err := db.GetSidebarWidth(ctx); err == nil {
			m.sidebarWidth = width
		}
	}
	return m
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
// renders `running` for it, a caller that lost the launch-lease race
// (service.ResumeStartingElsewhere) is shown "starting elsewhere", and a
// caller whose tmux session already exists (service.ResumeAlreadyRunning,
// requirement 46) is shown "already running" — neither is an error.
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

// NewWithShellCreatorAttacherKillerResumerProfileSwitcherResumeModerAgentCreatorRegistryAndPreviewCapturer
// additionally wires the read-only preview capture engine (SPEC
// requirements 21, 22, task 017). previewCapturer is called with the
// selected row's slug once per DECK_PREVIEW_MS tick; tmux.Client.CapturePreview
// is the released implementation — a single capture-pane per tick, no
// client attach, no control-mode server, no pipe-pane, no resize. nil keeps
// the preview exactly as inert as every constructor above this one leaves
// it.
func NewWithShellCreatorAttacherKillerResumerProfileSwitcherResumeModerAgentCreatorRegistryAndPreviewCapturer(db *store.Store, settings config.Settings, tmuxNote string, creator func(context.Context, service.ShellCreateInput) (store.Session, error), attacher func(context.Context, string) (*exec.Cmd, error), killer func(context.Context, store.Session) error, reconciler func(context.Context) error, resumer func(context.Context, string) (store.Session, service.ResumeOutcome, error), profileSwitcher func(context.Context, string, string) (store.Session, error), resumeModer func(context.Context, string, string) (store.Session, error), agentCreator func(context.Context, service.AgentCreateInput) (store.Session, error), registry *agent.Registry, previewCapturer func(context.Context, string) (tmux.PreviewCapture, error)) Model {
	m := NewWithShellCreatorAttacherKillerResumerProfileSwitcherResumeModerAgentCreatorAndRegistry(db, settings, tmuxNote, creator, attacher, killer, reconciler, resumer, profileSwitcher, resumeModer, agentCreator, registry)
	m.previewCapture = previewCapturer
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

// capturePreview issues exactly one read-only capture-pane for the
// currently selected row (SPEC requirement 21, task 017), or nil when there
// is no engine wired, no row to capture, or the preview panel is not shown
// this frame (requirement 27: no capture tick runs while the preview is
// below its floor and the sidebar has taken its space — task 021). The
// command captures the slug selected at the moment the tick fired, not
// whatever is selected when the reply arrives, so a selection change
// mid-flight cannot mislabel a frame.
func (m Model) capturePreview() tea.Cmd {
	if m.previewCapture == nil || len(m.sessions) == 0 || m.selected < 0 || m.selected >= len(m.sessions) {
		return nil
	}
	if !m.computeLayout().PreviewShown {
		return nil
	}
	session := m.sessions[m.selected]
	capture := m.previewCapture
	return func() tea.Msg {
		result, err := capture(context.Background(), session.Slug)
		return previewCaptured{sessionID: session.ID, capture: result, err: err}
	}
}

func (m Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case sessionsLoaded:
		if msg.err != nil {
			m.startupNote = "Cannot read sessions: " + msg.err.Error()
		} else {
			// SPEC requirements 28/29/30 (task 023's sort, task 024's
			// grouping): every load renders in attention order, not
			// store order, so the sidebar's group order itself follows
			// each group's most urgent member (see SPEC §11's
			// illustration: "service-a" leads with two waiting rows,
			// "infra" follows with only an error) exactly as
			// groupSessions' own "first appearance in m.sessions"
			// bucketing already promises once m.sessions is in this
			// order.
			var selectedID string
			if m.selected >= 0 && m.selected < len(m.sessions) {
				selectedID = m.sessions[m.selected].ID
			}
			m.sessions = sortSessionsByAttention(msg.sessions)
			if idx := indexOfSessionID(m.sessions, selectedID); idx >= 0 {
				m.selected = idx
			} else if m.selected >= len(m.sessions) {
				m.selected = max(0, len(m.sessions)-1)
			}
		}
	case shellCreated:
		if msg.err != nil {
			m.createError = msg.err.Error()
			return m, nil
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
	case sessionAcknowledged:
		if msg.err != nil {
			m.attachError = "Cannot acknowledge: " + msg.err.Error()
			return m, nil
		}
		m.attachError = ""
		return m, m.loadSessions
	case uiStatePersisted:
		// A failed write to ui_state is not load-bearing (SPEC §11.2): the
		// pin/width already changed in memory and keeps rendering; only the
		// error note surfaces so a persistent failure is still visible.
		if msg.err != nil {
			m.attachError = "Cannot persist layout: " + msg.err.Error()
		}
		return m, nil
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
		if msg.outcome == service.ResumeAlreadyRunning {
			// Requirement 46: deck already owns this pane. Adopting it as an
			// honest no-op means refreshing the row from whatever the service
			// returned (untouched) rather than pretending a launch happened.
			m.resumeNote = "already running"
			for i := range m.sessions {
				if m.sessions[i].ID == msg.session.ID {
					m.sessions[i] = msg.session
					break
				}
			}
			return m, nil
		}
		m.resumeNote = ""
		if msg.outcome == service.ResumeNotLeasable {
			// The resume command was dispatched from a stale stopped frame.
			// Render the durable status/reason returned by the service rather
			// than describing it as a launch in another client.
			for i := range m.sessions {
				if m.sessions[i].ID == msg.session.ID {
					m.sessions[i] = msg.session
					break
				}
			}
			return m, nil
		}
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
		// The capture engine (task 017, SPEC requirements 21, 22) samples the
		// selected row's live pane once per tick; rendering it (crop, geometry
		// line, placeholders) is tasks 018-021, so DECK_PREVIEW_MS's cadence is
		// already honoured end-to-end even though the panel still shows its
		// pre-capture placeholder.
		cmds := []tea.Cmd{tea.Tick(m.settings.Preview, func(t time.Time) tea.Msg { return previewTick(t) })}
		if cmd := m.capturePreview(); cmd != nil {
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...)
	case previewCaptured:
		// A session with no live pane reports capture.Live == false and a nil
		// err (see tmux.CapturePreview); only a genuine tmux/transport failure
		// reaches err, and even that is not load-bearing — the previous frame
		// (or, before the first successful capture, the placeholder) keeps
		// rendering rather than the tick disrupting the view.
		if msg.err == nil {
			m.previewSessionID = msg.sessionID
			m.previewLive = msg.capture.Live
			m.previewBytes = msg.capture.Bytes
			m.previewPaneWidth = msg.capture.Width
			m.previewPaneHeight = msg.capture.Height
		}
		return m, nil
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
		if m.settingsOpen {
			return m.updateSettings(msg)
		}
		if m.themePicking {
			return m.updateThemePicker(msg)
		}
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "?":
			m.help = !m.help
		case "esc":
			// detailView and helpView have no fields to submit or cycle, so
			// the only §11.4 contract key either binds is esc — shared here
			// through the same applyDialogContract implementation createView,
			// profileSwitchView and pinView defer to, rather than a sixth
			// hand-written cancel.
			_, _ = applyDialogContract(msg, dialogContract{Cancel: func() {
				m.help = false
				m.detail = false
			}})
		case "i":
			if !m.help && len(m.sessions) > 0 {
				m.detail = !m.detail
			}
		case ",":
			if !m.help {
				m.settingsOpen = true
				m.settingsCategoryIndex = 0
				m.settingsFieldIndex = 0
				m.settingsFocus = settingsFocusCategories
				m.settingsSearchActive = false
				m.settingsSearchQuery = ""
				m.settingsSearchIndex = 0
				m.settingsEdits = settingsEditsFromSettings(m.settings)
				m.settingsSavedEdits = settingsEditsFromSettings(m.settings)
				m.settingsDiscardConfirm = false
				m.settingsNote = ""
				m.settingsEnvOpen = false
				m.settingsEnvEditing = false
				m.settingsEnvIndex = 0
			}
		case "t":
			if !m.help {
				m = m.openThemePicker()
			}
		case "n":
			if !m.help {
				m.creating, m.createError, m.createField = true, "", 0
				m.createName, m.createCWD = "", ""
				m.createAgent, m.createProfile = defaultCreateAgent(m.registry().Kinds()), createProfileOptions[0]
				m.createLaunchArgs, m.createEnv, m.createPreLaunch, m.createLoginShell = "", "", "", false
				m.createYoloConfirmed = false
			}
		case "up", "k":
			if next, ok := m.prevVisibleSelection(m.selected); ok {
				m.selected = next
			}
		case "down", "j":
			if next, ok := m.nextVisibleSelection(m.selected); ok {
				m.selected = next
			}
		case "pgup":
			// ·11.3 requirement 19: PgUp/PgDn always drive the list, since the
			// sidebar is the only focusable region and there is no tab panel
			// cycle to move the page keys onto instead. pageSelection walks
			// VISUAL rows (002-steering.md), not raw m.sessions index
			// arithmetic, so a page of hidden/non-adjacent rows can't skew it.
			m.selected = m.pageSelection(-m.sidebarRowsPerPage())
		case "pgdown":
			m.selected = m.pageSelection(m.sidebarRowsPerPage())
		case "Y":
			if m.acknowledge == nil || len(m.sessions) == 0 {
				return m, nil
			}
			sessionID := m.sessions[m.selected].ID
			return m, func() tea.Msg {
				return sessionAcknowledged{err: m.acknowledge(context.Background(), sessionID)}
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
			if _, applicable := m.agentCapabilities(session.Agent); !applicable {
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
			caps, applicable := m.agentCapabilities(session.Agent)
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
		case " ":
			// SPEC requirements 31, 32: move to the next session needing
			// attention, wrapping, via the one shared NeedsAttention answer
			// (internal/tui/attention.go). Nothing needing attention (ok
			// false) leaves selection — and every session's status —
			// untouched: this must never behave like §7's attach, which
			// clears "waiting" on the attached session.
			if !m.help && len(m.sessions) > 0 {
				if next, ok := m.nextAttentionSelection(m.selected); ok {
					m.selected = next
				}
			}
		case "g":
			// SPEC §11.8 gap (requirement 30's collapsible headers had no key):
			// toggle the selected row's own workspace group collapsed/expanded,
			// via the identical helper task 028's mouse header click will call
			// (toggleGroupCollapse), so neither path is ever the only way to
			// reach this capability. A no-op, like every other bare-letter
			// binding, while help or the `i` detail overlay covers the sidebar,
			// or when there is no row to resolve a group from.
			if !m.help && !m.detail && len(m.sessions) > 0 {
				m.toggleGroupCollapse(sessionWorkspace(m.sessions[m.selected]))
			}
		case "|":
			if !m.help {
				return m.cycleLayoutMode()
			}
		case "<":
			if !m.help {
				m.sidebarWidth = m.adjustSidebarWidth(-1)
				return m, m.persistSidebarWidth()
			}
		case ">":
			if !m.help {
				m.sidebarWidth = m.adjustSidebarWidth(1)
				return m, m.persistSidebarWidth()
			}
		case "enter":
			return m.attachSelected()
		}
	case tea.MouseMsg:
		// [ui] mouse / DECK_MOUSE (requirement 3, 37): bubbletea's own input
		// reader decodes an SGR/X10 mouse report from raw input bytes
		// unconditionally, regardless of whether tea.WithMouseCellMotion
		// was passed at startup (that option only controls whether the
		// *enable* escape sequence is written to the terminal in the first
		// place) -- a real, compliant terminal simply never emits a mouse
		// report deck did not ask for, but this second, product-side gate
		// makes the opt-out authoritative even if one arrives anyway (a
		// terminal that ignores the missing enable sequence, or a replayed
		// byte stream), rather than relying solely on a well-behaved
		// terminal's cooperation.
		if !m.settings.Mouse {
			return m, nil
		}
		// SPEC §11.4/§11.8: the mouse can neither cancel nor confirm a dialog,
		// and no dialog action is reachable by mouse alone, so every overlay
		// that already makes the bare-letter keymap a no-op ignores the mouse
		// exactly the same way.
		if m.help || m.creating || m.profileSwitching || m.pinning || m.detail || m.themePicking || m.settingsOpen || m.settingsDiscardConfirm {
			return m, nil
		}
		return m.handleMouse(msg)
	}
	return m, nil
}

// attachSelected is the shared attach path behind both `↵` (SPEC §11) and
// a sidebar double-click (SPEC §11.8): the mouse binding must duplicate the
// key's exact behaviour, not a variant of it.
func (m Model) attachSelected() (tea.Model, tea.Cmd) {
	if m.attach == nil || len(m.sessions) == 0 || m.selected < 0 || m.selected >= len(m.sessions) {
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
	// Consult the durable row on every attachment. The list frame may lag a
	// hook that just made it waiting or error; the store transaction applies
	// only that row's current status and leaves raced resolutions untouched.
	if m.prepareAttach != nil {
		if err := m.prepareAttach(context.Background(), session.ID); err != nil {
			m.attachError = "Cannot attach: " + err.Error()
			return m, nil
		}
	}
	m.attachError = ""
	return m, tea.ExecProcess(command, func(err error) tea.Msg { return attachFinished{err: err} })
}

// cycleLayoutMode is `|`'s own path (SPEC §11.2/§11.8 requirement 9): auto
// -> side-by-side -> stacked -> collapsed -> auto. The collapsed strip's
// click does *not* call this any more (see restoreFromCollapsedStrip) --
// requirement 33 names its own, different behaviour. The first press of a
// fresh excursion (layoutCycleActive false) anchors preCollapseLayoutMode
// to the mode it is leaving and marks the excursion active; every
// subsequent press in the same excursion leaves that anchor untouched, so
// a run of presses that passes through an intermediate mode on its way to
// collapsed still remembers the mode the run started from, not the stop it
// passed through. Wrapping back to auto ends the excursion so the next
// press anchors fresh.
func (m Model) cycleLayoutMode() (tea.Model, tea.Cmd) {
	next := nextLayoutMode(m.layoutMode)
	if !m.layoutCycleActive {
		m.preCollapseLayoutMode = m.layoutMode
		m.layoutCycleActive = true
	}
	if next == LayoutAuto {
		m.layoutCycleActive = false
	}
	m.layoutMode = next
	return m, m.persistLayoutMode()
}

// restoreFromCollapsedStrip is the collapsed strip's own click path (SPEC
// §11.8 requirement 33: "click the collapsed strip -> restores the
// previous non-collapsed mode"), deliberately not cycleLayoutMode's `|`
// cycle: from collapsed, `|` always advances to auto, but the strip must
// return to whatever was pinned before the current `|` excursion began.
// No such mode was ever recorded (e.g. collapsed was reached by directly
// setting the pin rather than via `|`) falls back to auto, the same
// default every other unset pin uses. The restore itself ends the
// excursion, same as wrapping to auto would, so the next `|` press anchors
// fresh from the restored mode.
func (m Model) restoreFromCollapsedStrip() (tea.Model, tea.Cmd) {
	mode := m.preCollapseLayoutMode
	if mode == "" {
		mode = LayoutAuto
	}
	m.layoutMode = mode
	m.preCollapseLayoutMode = ""
	m.layoutCycleActive = false
	return m, m.persistLayoutMode()
}

// persistLayoutMode returns a command that writes the just-updated
// layoutMode pin to state.db's ui_state table (task 016, SPEC §11.2), never
// to config.toml. With no store attached (most unit tests) it is a no-op so
// callers can always chain it after mutating m.layoutMode.
func (m Model) persistLayoutMode() tea.Cmd {
	if m.store == nil {
		return nil
	}
	mode := m.layoutMode
	return func() tea.Msg {
		return uiStatePersisted{err: m.store.SetLayoutMode(context.Background(), mode)}
	}
}

// persistSidebarWidth is persistLayoutMode's sidebar_width counterpart.
func (m Model) persistSidebarWidth() tea.Cmd {
	if m.store == nil {
		return nil
	}
	width := m.sidebarWidth
	return func() tea.Msg {
		return uiStatePersisted{err: m.store.SetSidebarWidth(context.Background(), width)}
	}
}

func (m Model) View() string {
	if m.help {
		return m.helpView()
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
	if m.settingsOpen {
		return m.settingsView()
	}
	if m.detail && len(m.sessions) > 0 {
		return m.detailView()
	}
	return m.mainView()
}

// frameSize is the terminal size mainView renders at. A zero width/height
// (every Model built without ever receiving a tea.WindowSizeMsg — which is
// most of this package's unit tests) defaults to deck's own 80×24 supported
// minimum rather than to a degenerate empty frame, so those tests keep
// exercising a real, legible layout.
func (m Model) frameSize() (width, height int) {
	width, height = m.width, m.height
	if width <= 0 {
		width = AutoSideBySideWidth
	}
	if height <= 0 {
		height = MinRows
	}
	return width, height
}

// startupBanner is the tmux-unavailable startup note (SPEC requirement: the
// install guidance stays reachable), rendered across the full terminal
// width above the panels rather than wrapped inside the sidebar's 33-column
// budget, which is too narrow to hold features/tmux_contract.feature's
// "tmux 3.1c is too old" on one line. It returns no lines at all once tmux
// is fine, so it costs nothing in the common case.
func (m Model) startupBanner(width int) []string {
	if m.startupNote == "" {
		return nil
	}
	var lines []string
	lines = append(lines, wrapText("tmux unavailable: "+m.startupNote, width)...)
	lines = append(lines, wrapText("Install tmux 3.2 or newer, then restart deck.", width)...)
	lines = append(lines, "")
	return lines
}

// themeBanner is requirement 28's fallback notice for the [ui] theme key:
// theme.Resolve (invoked by config.LoadFrom) computed ThemeReason once, at
// load time, whenever the configured name could not be resolved -- unknown
// name, or a user theme file that failed to parse -- and fell back to
// theme.Default(). The very first painted frame must say so; it must never
// silently render the default as though the requested theme had applied
// (this is the theme system's own version of a fabricated status). Since
// ThemeReason cannot change for the lifetime of one config load, this
// banner is simply always present once set -- there is no later frame at
// which showing it would stop being honest -- which trivially satisfies
// "on first paint" without needing a one-shot flag threaded through Update.
// It returns no lines at all when ThemeReason is "" (nothing was
// configured, or the configured name resolved cleanly), so it costs
// nothing in the common case, matching startupBanner's own convention.
func (m Model) themeBanner(width int) []string {
	if m.settings.ThemeReason == "" {
		return nil
	}
	var lines []string
	lines = append(lines, wrapText(m.settings.ThemeReason, width)...)
	lines = append(lines, "")
	return lines
}

// attachErrorLines is requirement 37's wrapped attachError line set, shared
// between computeLayout's reservation and mainView's actual render so the
// two never disagree about how many rows the message costs. It returns no
// lines at all when there is no error, so it costs nothing in the common
// case — computeLayout must not unconditionally shrink the panels by this
// message's worst case when none is present.
func (m Model) attachErrorLines(width int) []string {
	if m.attachError == "" {
		return nil
	}
	return wrapText(m.attachError, width)
}

// resumeNoteLines is requirement 37's wrapped resumeNote line set, the
// resumeNote counterpart to attachErrorLines above.
func (m Model) resumeNoteLines(width int) []string {
	if m.resumeNote == "" {
		return nil
	}
	return wrapText(m.resumeNote, width)
}

// computeLayout is the one place mainView and the page-size math below call
// ComputeLayout, reserving exactly one row for the footer (SPEC §11.3: "the
// footer is one line, outside both panels") plus the startup banner's own
// rows, if any, plus attachError's and resumeNote's own wrapped row counts,
// if either is set (requirement 37), before handing the rest to the §11.2
// geometry function, which knows about none of them. Skipping any of these
// reservations would let that message's lines push the whole frame past the
// terminal's actual row count, which does not shrink the frame to fit — it
// scrolls, carrying the banner (and the sidebar's own top border) off the
// top of the visible screen (features/tmux_contract.feature's "old tmux is
// actionable" scenario would never see either again). attachError and
// resumeNote are mutually exclusive in practice (tui.go's Update clears one
// whenever it sets the other) but both are budgeted here regardless, so a
// future caller that sets both together still gets a frame that fits.
func (m Model) computeLayout() LayoutResult {
	width, height := m.frameSize()
	reserved := 1 + len(m.startupBanner(width)) + len(m.themeBanner(width)) + len(m.themePickerLines(width)) + len(m.attachErrorLines(width)) + len(m.resumeNoteLines(width))
	result := ComputeLayout(width, height-reserved, m.layoutMode, m.sidebarWidth)
	// ComputeLayout's own BelowMinimum reads its rows argument as the full
	// terminal height (its doc comment says so, and its direct unit tests
	// call it that way), but the reserved rows above already subtract the
	// footer/banner before rows ever reaches it here, so an exact 80x24
	// terminal would otherwise report BelowMinimum=true (23 content rows
	// < MinRows) even though 24 is deck's own supported minimum, not below
	// it. Recompute the flag against the real, unreserved terminal size so
	// SPEC requirement 14's footer notice (footerLine) only ever fires for
	// a genuinely below-minimum terminal.
	result.BelowMinimum = width < AutoSideBySideWidth || height < MinRows
	return result
}

// adjustSidebarWidth is `<`/`>`'s one-column step (SPEC requirement 15),
// clamped through the same ClampSidebarWidth bound ComputeLayout itself
// uses so the persisted value and the rendered geometry never disagree.
// m.sidebarWidth's zero-means-default convention (§11.2) is resolved to
// its concrete default before stepping, so `<` from an unset width steps
// down from 35 rather than from 0.
func (m Model) adjustSidebarWidth(delta int) int {
	width, _ := m.frameSize()
	current := m.sidebarWidth
	if current <= 0 {
		current = store.DefaultSidebarWidth
	}
	return ClampSidebarWidth(width, current+delta)
}

// sidebarRowsPerPage is PgUp/PgDn's page size (SPEC requirement 19): the
// number of two-line session rows that actually fit in the sidebar's
// current content height, never fewer than one.
func (m Model) sidebarRowsPerPage() int {
	layout := m.computeLayout()
	rows := layout.Sidebar.Height - 2
	const linesPerRow = 2
	perPage := rows / linesPerRow
	if perPage < 1 {
		perPage = 1
	}
	return perPage
}

// mainView renders the §11.3 chrome: a sidebar and a preview sharing one
// seam in side-by-side/collapsed layouts, or two independently-bordered
// panels stacked vertically in the below-minimum stacked layout, followed by
// the one-line footer outside both panels.
func (m Model) mainView() string {
	width, _ := m.frameSize()
	layout := m.computeLayout()
	lines := m.startupBanner(width)
	lines = append(lines, m.themeBanner(width)...)
	lines = append(lines, m.themePickerLines(width)...)
	if layout.Effective == LayoutStacked {
		lines = append(lines, m.renderStackedFrame(layout)...)
	} else {
		lines = append(lines, m.renderSideBySideFrame(layout)...)
	}
	lines = append(lines, m.attachErrorLines(width)...)
	lines = append(lines, m.resumeNoteLines(width)...)
	lines = append(lines, m.footerLine())
	return strings.Join(lines, "\n")
}

// footerLine is SPEC requirement 20's single footer line: the key legend,
// preceded by the selected row's status reason (task 012) when it has one.
// It never lists a key that is not bound. When the terminal is below
// deck's supported 80x24 minimum (SPEC requirement 14), the footer states
// that instead — renderStackedFrame no longer draws that notice above the
// panels, where it could scroll the sidebar's own top border off screen;
// the footer is the one line SPEC §11.3 guarantees stays on screen. At a
// width narrower than the notice's own 87 columns, truncateToWidth elides
// its tail rather than letting it wrap and push the frame's line count
// past the terminal's actual height (task 010) — a below-minimum terminal
// is, by definition, exactly the case where that budget is tight.
func (m Model) footerLine() string {
	if m.computeLayout().BelowMinimum {
		width, _ := m.frameSize()
		return truncateToWidth(belowMinimumNotice, width)
	}
	keys := m.footerKeyLegend()
	if reason := m.selectedRowReason(); reason != "" {
		return reason + "    " + keys
	}
	return keys
}

// belowMinimumNotice is SPEC requirement 14's exact below-minimum copy,
// shared between the footer (footerLine) and the mouse hit-tester so the
// two never disagree about whether it is on screen.
const belowMinimumNotice = "Terminal is below deck's supported minimum of 80x24; showing stacked as far as it fits."

// footerKeyHint pairs one footer key legend entry's key glyph (in both its
// Unicode and DECK_ASCII forms, mirroring m.glyph's own two-string calls
// elsewhere) with the hint word after it. The very first entry (↑/↓) has
// no hint -- it names the up/down keys without a verb, exactly as the
// legend always has -- so hint is left "" there rather than inventing one.
type footerKeyHint struct {
	unicodeKey, asciiKey, hint string
}

// footerLegend is SPEC requirement 20's key legend, SPEC requirement 35's
// `key`/`hint` tokens applied structurally (task 021) rather than as one
// undifferentiated string: every entry's key glyph and hint word are
// tracked separately so footerKeyLegend can colour them apart, while the
// concatenated visible text (key legend and Unicode fallback) is byte-for-
// byte what it always was, and pending tests that grep for a plain
// substring like "up/down" or "Enter attach" never see the joins move.
var footerLegend = []footerKeyHint{
	{"↑/↓", "up/down", ""},
	{"↵", "Enter", "attach"},
	{"Y", "Y", "acknowledge"},
	{"n", "n", "new"},
	{"x", "x", "kill"},
	{"r", "r", "resume"},
	{"P", "P", "profile"},
	{"p", "p", "pin"},
	{"i", "i", "detail"},
	{"?", "?", "help"},
	{"q", "q", "quit"},
}

// footerKeyLegend renders footerLegend into the footer's key legend line,
// each entry's key glyph in the `key` token and its hint word in the
// `hint` token (SPEC requirement 35), joined by the same " · "/" - "
// separator the legend has always used -- never coloured itself, so it
// reads as neutral punctuation between differently-coloured runs rather
// than borrowing either token.
func (m Model) footerKeyLegend() string {
	sep := m.glyph(" · ", " - ")
	parts := make([]string, len(footerLegend))
	for i, e := range footerLegend {
		key := m.glyph(e.unicodeKey, e.asciiKey)
		seg := m.colorToken(theme.Key, key)
		if e.hint != "" {
			seg += " " + m.colorToken(theme.Hint, e.hint)
		}
		parts[i] = seg
	}
	return strings.Join(parts, sep)
}

// renderSideBySideFrame draws two panels sharing one seam (SPEC requirement
// 18): the sidebar draws its top/left/bottom borders only, and the
// preview's own left border is the seam, so there is exactly one vertical
// bar between them, never "││". This same shape also draws the collapsed
// strip (a narrower "sidebar" panel beside a wider preview); task 015 fills
// in the strip's own content.
func (m Model) renderSideBySideFrame(layout LayoutResult) []string {
	sw, pw := layout.Sidebar.Width, layout.Preview.Width
	height := layout.Sidebar.Height
	contentRows := height - 2
	if contentRows < 0 {
		contentRows = 0
	}
	collapsed := layout.Effective == LayoutCollapsed
	sidebarTop := m.sidebarTopLine(sw, m.sidebarTitleText())
	var sidebar []string
	if collapsed {
		// The 3-column strip has no room for the "deck — sessions" title
		// and draws its own attention-count content instead of session
		// rows (task 015, SPEC requirement 15).
		sidebarTop = m.sidebarTopLine(sw, "")
		sidebar = fitLines(m.collapsedStripLines(), contentRows)
	} else {
		// sidebarVisibleEntries applies the wheel-scroll offset (task 028)
		// through the identical entries hitTest resolves clicks against.
		visible := m.sidebarVisibleEntries(max(sw-2, 0), contentRows)
		sidebar = make([]string, len(visible))
		for i, e := range visible {
			sidebar[i] = e.text
		}
	}
	preview := m.previewBodyLines(max(pw-4, 0), contentRows)
	lines := make([]string, 0, height)
	lines = append(lines, sidebarTop+m.previewTopLine(pw, m.previewTitle(), true))
	for i := 0; i < contentRows; i++ {
		var sidebarLine string
		if collapsed {
			sidebarLine = m.collapsedStripContentLine(sw, sidebar[i])
		} else {
			sidebarLine = m.sidebarContentLine(sw, sidebar[i])
		}
		lines = append(lines, sidebarLine+m.previewContentLine(pw, preview[i]))
	}
	lines = append(lines, m.sidebarBottomLine(sw)+m.previewBottomLine(pw, true))
	return lines
}

// collapsedStripLines is the 3-column collapsed strip's own content (SPEC
// requirement 15): the `»` glyph, then the attention count's digits each
// on their own line so a multi-digit count still reads inside the strip's
// single content column. attentionCount goes through the shared
// NeedsAttention answer (internal/tui/attention.go, task 025), so this
// count always agrees with the sort and `space`.
func (m Model) collapsedStripLines() []string {
	lines := []string{m.glyph("»", ">")}
	for _, r := range strconv.Itoa(m.attentionCount()) {
		lines = append(lines, string(r))
	}
	return lines
}

// attentionCount counts sessions that need attention (SPEC requirements 31,
// 32), via the single shared NeedsAttention answer also used by the
// attention sort and `space` (Model.nextAttentionSelection) — the three can
// never disagree about what counts.
func (m Model) attentionCount() int {
	n := 0
	for _, s := range m.sessions {
		if NeedsAttention(s) {
			n++
		}
	}
	return n
}

// nextAttentionSelection is `space`'s own step (SPEC requirements 31, 32):
// the index of the next *visible* session needing attention, searching
// forward from just after from's VISUAL position and wrapping around the
// whole painted list back through from itself, so a lone session needing
// attention (including the one already selected) is still found rather
// than treated as "nothing needs attention". Wrapping is done over
// visualOrder (painted order), not m.sessions index order, for the same
// reason ↑/↓ and PgUp/PgDn do (002-steering.md): otherwise `space` could
// jump backwards or skip a visible row whenever a workspace's sessions
// are non-adjacent in m.sessions. ok is false only when no visible
// session needs attention at all, in which case the caller must leave
// selection (and everything else) untouched — this function never
// mutates m.sessions or any session's status, only answers where to move.
func (m Model) nextAttentionSelection(from int) (int, bool) {
	order := m.visualOrder()
	n := len(order)
	if n == 0 {
		return from, false
	}
	pos := -1
	for i, idx := range order {
		if idx == from {
			pos = i
			break
		}
	}
	for step := 1; step <= n; step++ {
		i := (pos + step) % n
		idx := order[i]
		if m.isSessionVisible(idx) && NeedsAttention(m.sessions[idx]) {
			return idx, true
		}
	}
	return from, false
}

// renderStackedFrame draws the below-80-column fallback (SPEC §11.2): the
// list and the preview stack vertically with no seam between them, so each
// keeps all four of its own borders.
func (m Model) renderStackedFrame(layout LayoutResult) []string {
	lw, lh := layout.Sidebar.Width, layout.Sidebar.Height
	pw, ph := layout.Preview.Width, layout.Preview.Height
	var lines []string
	if lh >= 2 {
		listRows := lh - 2
		visible := m.sidebarVisibleEntries(max(lw-4, 0), listRows)
		body := make([]string, len(visible))
		for i, e := range visible {
			body[i] = e.text
		}
		lines = append(lines, m.fullBoxTop(lw, m.sidebarTitleText(), true))
		for i := 0; i < listRows; i++ {
			lines = append(lines, m.fullBoxContentLine(lw, body[i], true))
		}
		lines = append(lines, m.fullBoxBottom(lw, true))
	}
	if ph >= 2 {
		previewRows := ph - 2
		body := m.previewBodyLines(max(pw-4, 0), previewRows)
		lines = append(lines, m.fullBoxTop(pw, m.previewTitle(), false))
		for i := 0; i < previewRows; i++ {
			lines = append(lines, m.fullBoxContentLine(pw, body[i], false))
		}
		lines = append(lines, m.fullBoxBottom(pw, false))
	}
	return lines
}

// sidebarTitleText is the plain (uncoloured) form of the sidebar's title,
// used by the stacked layout's fullBoxTop, which has no special-cased
// "deck" colour run the way sidebarTitleLine does.
func (m Model) sidebarTitleText() string {
	return "deck" + m.glyph(" — ", " - ") + "sessions"
}

// sidebarBodyLines is the sidebar's content, before it is fit to the
// panel's actual content height: an optional socket line, the empty state,
// or every session's two-line row (SPEC §11.3: "the empty state and Press n
// copy now live inside the sidebar"). The tmux-unavailable startup note is
// not part of this body — see mainView's full-width banner.
func (m Model) sidebarBodyLines(contentWidth int) []string {
	entries := m.sidebarEntries(contentWidth)
	lines := make([]string, len(entries))
	for i, e := range entries {
		lines[i] = e.text
	}
	return lines
}

// sidebarLineKind distinguishes what a sidebarEntry line actually is, so
// hit-testing (SPEC §11.8, task 028) can resolve a clicked line back to a
// group header or a session row through the exact same content this
// function feeds the renderer -- "exactly one geometry implementation", per
// the task's own success criterion.
type sidebarLineKind int

const (
	sidebarLineOther sidebarLineKind = iota
	sidebarLineHeader
	sidebarLineRow
)

// sidebarEntry is one rendered line of the sidebar body, tagged with enough
// to resolve a click: sidebarLineHeader carries Workspace, sidebarLineRow
// carries SessionIndex (an index into m.sessions), and sidebarLineOther
// (the socket line, the empty state) carries neither.
type sidebarEntry struct {
	text         string
	kind         sidebarLineKind
	workspace    string
	sessionIndex int
}

// sidebarEntries is sidebarBodyLines' one real implementation: every line
// the sidebar can render, each tagged with what it is. sidebarBodyLines
// (the renderer's own text-only view) and hitTest (task 028's click
// resolution) both build on this rather than keeping two descriptions of
// the same content in sync by hand.
func (m Model) sidebarEntries(contentWidth int) []sidebarEntry {
	var entries []sidebarEntry
	if m.settings.Socket != "" {
		for _, line := range wrapText(fmt.Sprintf("socket: %s", m.settings.Socket), contentWidth) {
			entries = append(entries, sidebarEntry{text: line})
		}
	}
	if len(m.sessions) == 0 {
		for _, line := range wrapText("No sessions yet. Press n to create a session.", contentWidth) {
			entries = append(entries, sidebarEntry{text: line})
		}
		return entries
	}
	for _, group := range m.groupSessions() {
		entries = append(entries, sidebarEntry{text: m.groupHeaderText(group), kind: sidebarLineHeader, workspace: group.Workspace})
		if m.isGroupCollapsed(group.Workspace) {
			continue
		}
		for _, is := range group.Sessions {
			for _, line := range m.sidebarRowLines(is.Index, is.Session) {
				entries = append(entries, sidebarEntry{text: line, kind: sidebarLineRow, sessionIndex: is.Index})
			}
		}
	}
	return entries
}

// clampSidebarScroll bounds a raw scroll offset into [0, max(0,
// total-contentHeight)] (SPEC §11.8's wheel scroll, task 028): never
// negative, and never past the point where the last line would leave the
// panel's bottom empty while there is still content to show.
func clampSidebarScroll(raw, total, contentHeight int) int {
	maxScroll := total - contentHeight
	if maxScroll < 0 {
		maxScroll = 0
	}
	if raw < 0 {
		return 0
	}
	if raw > maxScroll {
		return maxScroll
	}
	return raw
}

// sidebarVisibleEntries returns exactly contentHeight sidebarEntry values
// currently shown in the sidebar's content area, applying the wheel-scroll
// offset (task 028). Both renderSideBySideFrame/renderStackedFrame's actual
// drawing and hitTest's click resolution call this same function, so a
// scrolled frame and a click against it can never disagree about which
// entry is on which line.
func (m Model) sidebarVisibleEntries(contentWidth, contentHeight int) []sidebarEntry {
	entries := m.sidebarEntries(contentWidth)
	offset := clampSidebarScroll(m.sidebarScroll, len(entries), contentHeight)
	var visible []sidebarEntry
	if offset < len(entries) {
		visible = entries[offset:]
	}
	if len(visible) > contentHeight {
		visible = visible[:contentHeight]
	}
	out := make([]sidebarEntry, contentHeight)
	copy(out, visible)
	return out
}

// sidebarRowLines is one session's two-line row: glyph/marker, name, its
// unseen glyph and status/quality badges on the first line (SPEC §11.3,
// task 012 — no reason text), and its permission-profile badge plus
// creation time on the second. It intentionally omits the row's cwd and
// agent kind that the pre-chrome list used to print: at the sidebar's
// default 35-column width (33 content columns) there is no room for both a
// name and a full path on one line, and a wrapped second cwd line would
// halve how many sessions fit on screen. Both remain one keystroke away in
// the `i` detail dialog; a fuller compact row (matching SPEC §11's
// illustrated `● api-refactor claude live waiting 2m`) is Phase 2b-1's
// attention-sort/grouping work (tasks 023/024), which already has to touch
// this row's shape for the status glyph and workspace headers.
//
// The unseen glyph and quality badge are only added to the first line's
// badge run when each actually has something to say, rather than reserving
// a fixed-width placeholder column for each, and the profile badge moves to
// the second line entirely: features/status_probe.feature and
// features/concurrency.feature both require a session's real name plus its
// quality word ("live"/"sampled") or status word on the very same rendered
// line (clientRowContainsWithinReconcile, features/status_probe_test.go),
// and a realistic name (10-22 columns seen across features/*.feature) plus
// a profile badge plus a quality badge plus a status word does not fit
// inside 33 columns; nothing tested requires the profile badge on that same
// line, so it is the one dropped rather than risk ellipsis-truncating a
// word an assertion depends on.
func (m Model) sidebarRowLines(index int, session store.Session) []string {
	marker := "  "
	selected := index == m.selected
	if selected {
		marker = "> "
	}
	// SPEC requirement 35's `dimmed` covers a starting row (task 021): it
	// carries no signal yet beyond its own liveness, so everything but the
	// status word itself -- coloured in its own starting token below, which
	// must keep reading as "starting" rather than fade to grey -- is dimmed
	// so the eye is not drawn to it the way a row with real news is.
	nameTok := theme.Text
	if session.Status == "starting" {
		nameTok = theme.Dimmed
	}
	// Every run of text on this row is composed as a settingsRowSegment
	// (the same generic label/value/selection-background helper task 019
	// built for the settings takeover, reused here rather than duplicated)
	// so a selected row's `selection` BACKGROUND (SPEC requirement 42) can
	// be opened once and stay live under every foreground-coloured segment
	// -- composing self-resetting m.colorToken calls into one string here,
	// the way this row used to, would have the FIRST inner segment's own
	// trailing \x1b[0m clear that background again immediately (the
	// gotcha theme_color.go's foregroundSGR/backgroundSGR doc already
	// warns about).
	segs := []settingsRowSegment{{Text: marker + session.Name + " ", Tok: nameTok}}
	var parts []settingsRowSegment
	if !session.Acknowledged && (session.Status == "waiting" || session.Status == "error") {
		unseen := m.glyph("●", "!")
		tok := theme.Text
		if t, ok := statusToken(session.Status); ok {
			tok = t
		}
		parts = append(parts, settingsRowSegment{Text: unseen, Tok: tok})
	}
	if quality := statusSourceQuality(session.StatusSource); quality != "" {
		parts = append(parts, settingsRowSegment{Text: quality, Tok: theme.Badge})
	}
	statusTok := theme.Text
	if t, ok := statusToken(session.Status); ok {
		statusTok = t
	}
	parts = append(parts, settingsRowSegment{Text: session.Status, Tok: statusTok})
	for i, p := range parts {
		if i > 0 {
			segs = append(segs, settingsRowSegment{Text: " ", Tok: theme.Text})
		}
		segs = append(segs, p)
	}
	line1 := m.settingsRenderRow(segs, theme.Selection, selected)

	line2Tok := theme.Text
	if session.Status == "starting" {
		line2Tok = theme.Dimmed
	}
	line2Segs := []settingsRowSegment{{Text: "  ", Tok: theme.Text}}
	if text, tok, ok := m.profileBadgeSegment(session); ok {
		line2Segs = append(line2Segs, settingsRowSegment{Text: text, Tok: tok}, settingsRowSegment{Text: " ", Tok: theme.Text})
	}
	line2Segs = append(line2Segs, settingsRowSegment{Text: "created " + m.relativeTime(session.CreatedAt), Tok: line2Tok})
	line2 := m.settingsRenderRow(line2Segs, theme.Selection, selected)
	return []string{line1, line2}
}

// previewTitle is the preview panel's border title. It intentionally never
// embeds the selected session's name: every helper across this package and
// features/ that locates "a session's row" does so by finding the first
// screen line containing that name, and the preview's top border shares a
// screen line with the sidebar's own top border (row 0) — embedding the name
// there would make it the *first* match for the selected session, ahead of
// its real row, and silently break every such lookup. SPEC's §11 figure
// shows a named preview title as an illustration, not a tested requirement;
// this returns "" until a design exists that cannot collide with a row
// name.
func (m Model) previewTitle() string {
	return ""
}

// previewBodyLines is the preview panel's content: the real cropped pane
// capture (task 018, SPEC requirement 23) when the selected session has a
// live capture on file, otherwise a placeholder naming exactly which
// no-live-pane state applies (task 020, SPEC requirement 26) -- an `error`
// row's stored crash tail, or a one-line placeholder for `stopped`,
// `archived` and a `starting` row with no pane yet. Stale bytes are never
// presented as live: the placeholder branch is only reached when the most
// recent capture for this exact session id found no live pane (or none has
// been attempted yet). It always returns exactly contentHeight lines, each
// exactly contentWidth runes, so callers no longer need their own fitLines
// pass for the preview panel.
func (m Model) previewBodyLines(contentWidth, contentHeight int) []string {
	if len(m.sessions) == 0 || m.selected < 0 || m.selected >= len(m.sessions) {
		return fitLines(wrapText("Select or create a session to preview it here.", contentWidth), contentHeight)
	}
	session := m.sessions[m.selected]
	if m.previewLive && m.previewSessionID == session.ID {
		return m.cropPreviewBottomLeft(m.previewBytes, contentWidth, contentHeight, m.previewPaneWidth, m.previewPaneHeight)
	}
	return fitLines(m.previewPlaceholderLines(session, contentWidth, contentHeight), contentHeight)
}

// previewPlaceholderLines names, rather than papers over, why the preview
// has nothing live to show for the selected session (task 020, SPEC
// requirement 26). `error` gets the durable crash tail headed by copy
// stating it is the last output before the exit and is not live; the other
// three named states get a single line naming the state and nothing else
// that could be mistaken for live pane content. Any other status reaching
// here (e.g. a row not yet captured on the very first tick after
// selection) keeps the original CWD-plus-notice copy.
func (m Model) previewPlaceholderLines(session store.Session, contentWidth, contentHeight int) []string {
	switch session.Status {
	case "error":
		return m.crashTailPreviewLines(session.CrashTail, contentWidth, contentHeight)
	case "stopped":
		return wrapText("Session is stopped. No live preview to show.", contentWidth)
	case "archived":
		return wrapText("Session is archived. No live preview to show.", contentWidth)
	case "starting":
		return wrapText("Session is starting; no pane yet.", contentWidth)
	default:
		var lines []string
		lines = append(lines, wrapText(session.CWD, contentWidth)...)
		lines = append(lines, "")
		lines = append(lines, wrapText("No live preview captured for this row yet.", contentWidth)...)
		return lines
	}
}

// crashTailPreviewLines renders an `error` row's durable §7 crash tail into
// the preview panel (task 020, SPEC requirement 26), headed by copy that
// states plainly it is the last output before the process exited and is
// not live -- never letting a stale capture be mistaken for a live pane.
// The tail is anchored bottom, mirroring requirement 23's own bottom-left
// crop anchoring: when the stored tail has more lines than the panel has
// room for, the newest (last) lines win.
func (m Model) crashTailPreviewLines(tail string, contentWidth, contentHeight int) []string {
	header := wrapText(m.glyph("Last output before exit \u2014 not live:", "Last output before exit - not live:"), contentWidth)
	if tail == "" {
		return append(header, wrapText("No crash output was captured.", contentWidth)...)
	}
	var body []string
	for _, raw := range strings.Split(strings.Trim(tail, "\n"), "\n") {
		body = append(body, wrapText(raw, contentWidth)...)
	}
	budget := contentHeight - len(header)
	if budget < 0 {
		budget = 0
	}
	if len(body) > budget {
		body = body[len(body)-budget:]
	}
	return append(header, body...)
}

// selectedRowReason returns the reason text belonging to whichever row is
// currently selected (SPEC §11.3, task 012): the row itself only ever shows
// the bare status word now, so the one piece of "why" a user is actually
// looking at gets a stable home on the footer's left, clearly separated from
// the key legend, rather than jittering every row's width as sessions move
// between statuses. It returns "" when the selected session has no reason to
// show (e.g. running, waiting, or a starting shell whose only signal is its
// own liveness).
func (m Model) selectedRowReason() string {
	if len(m.sessions) == 0 || m.selected < 0 || m.selected >= len(m.sessions) {
		return ""
	}
	session := m.sessions[m.selected]
	switch {
	case session.Status == "stopped":
		return session.Status + m.glyph(" · resumable", " - resumable")
	case session.Status == "starting" && session.Agent != "shell":
		return "starting" + m.glyph(" · awaiting signal", " - awaiting signal")
	default:
		return ""
	}
}

// profileBadge renders the bracketed permission-profile badge shown next to
// an agent row in the session list. Shell sessions have no notion of a
// permission profile at all (SPEC §5/§8): agentCapabilities reports
// them as not applicable, so they render no badge rather than a meaningless
// cosmetic "safe".
func (m Model) profileBadgeSegment(session store.Session) (text string, tok theme.Token, ok bool) {
	if _, applicable := m.agentCapabilities(session.Agent); !applicable {
		return "", "", false
	}
	// SPEC's builtin themes comment badge_warn as "non-safe permission
	// profiles, yolo" (task 021): `safe` is merely informational (`badge`),
	// every other profile -- plan, edits, yolo -- is the thing worth a
	// warning colour.
	tok = theme.Badge
	if session.PermissionProfile != "safe" {
		tok = theme.BadgeWarn
	}
	return "[" + session.PermissionProfile + "]", tok, true
}

// profileBadge self-colours profileBadgeSegment's text for a caller that
// just wants a ready-to-print badge; sidebarRowLines instead composes the
// segment directly so a selected row's `selection` background stays live
// underneath it (see settingsRenderRow's own doc on why a self-resetting
// colorToken call must never be nested inside one).
func (m Model) profileBadge(session store.Session) string {
	text, tok, ok := m.profileBadgeSegment(session)
	if !ok {
		return ""
	}
	return m.colorToken(tok, text)
}

// statusSourceQuality describes only the quality of an agent verdict. Hook
// events are live and pane probes are sampled; user and tmux transitions make
// no claim about what an agent is doing and therefore receive no badge.
func statusSourceQuality(source string) string {
	switch source {
	case "hook":
		return "live"
	case "probe":
		return "sampled"
	default:
		return ""
	}
}

// updateProfileSwitch handles keys while the `P` permission-profile switch
// dialog is open (task 020). It only ever cycles a locally-held candidate
// value and, on confirmation, persists it through m.profileSwitch; it never
// issues any argv to the selected session's pane, live or otherwise.
func (m Model) updateProfileSwitch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	session := m.sessions[m.selected]
	options := m.createProfileOptionsFor(session.Agent, m.settings.AllowYolo)
	cycle := func(delta int) {
		if m.profileSwitchValue != "yolo" {
			m.profileSwitchYoloOK = false
		}
		m.profileSwitchValue = cycleOption(options, m.profileSwitchValue, delta)
	}
	if cmd, handled := applyDialogContract(msg, dialogContract{
		Fields: dialogFields{Cycle: cycle},
		Cancel: func() {
			m.profileSwitching = false
			m.profileSwitchNote = ""
		},
		Submit: func() tea.Cmd {
			if m.profileSwitch == nil {
				m.profileSwitchNote = "changing the permission profile is unavailable"
				return nil
			}
			if m.profileSwitchValue == "yolo" && !m.profileSwitchYoloOK {
				m.profileSwitchNote = "yolo requires confirmation: press y, then Enter to switch"
				return nil
			}
			sessionID, profile := session.ID, m.profileSwitchValue
			return func() tea.Msg {
				updated, err := m.profileSwitch(context.Background(), sessionID, profile)
				return profileSwitched{session: updated, err: err}
			}
		},
	}); handled {
		return m, cmd
	}
	// The yolo double-gate's explicit confirm keystroke, mirroring the
	// create modal's "y" (SPEC §5: switching to yolo requires the same
	// explicit confirm as creating with yolo). It is an additional
	// load-bearing key the §11.4 contract does not itself define, stated
	// inline in profileSwitchView.
	if msg.String() == "y" && m.profileSwitchValue == "yolo" && m.settings.AllowYolo && !m.profileSwitchYoloOK {
		m.profileSwitchYoloOK = true
		m.profileSwitchNote = ""
	}
	return m, nil
}

// profileSwitchView renders the `P` permission-profile switch dialog. It
// states plainly that the change only applies on the session's next
// launch/restart; it never claims the live pane's mode changed (SPEC §5).
func (m Model) profileSwitchView() string {
	session := m.sessions[m.selected]
	options := m.createProfileOptionsFor(session.Agent, m.settings.AllowYolo)
	var b strings.Builder
	fmt.Fprintf(&b, "Change permission profile for %s\n\n", session.Name)
	fmt.Fprintf(&b, "%s\n", m.detailField("Current:   ", session.PermissionProfile))
	fmt.Fprintf(&b, "%s\n", m.detailField("New:       ", fmt.Sprintf("%s (left/right cycles: %s)", m.profileSwitchValue, strings.Join(options, ", "))))
	b.WriteString("\nThis applies on the session's next launch/restart; it does not change a\nrunning pane's mode.\n")
	if m.profileSwitchValue == "yolo" && m.settings.AllowYolo && !m.profileSwitchYoloOK {
		b.WriteString("\nyolo requires confirmation: press y, then Enter to switch\n")
	}
	b.WriteString("\nLeft/Right cycles · Enter confirms · Esc cancels\n")
	if m.profileSwitchNote != "" {
		fmt.Fprintf(&b, "\n%s\n", m.profileSwitchNote)
	}
	return m.framedDialog(b.String())
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
	cmd, handled := applyDialogContract(msg, dialogContract{
		Fields: dialogFields{Cycle: func(delta int) {
			m.pinValue = cycleOption(resumeModeOptions, m.pinValue, delta)
		}},
		Cancel: func() {
			m.pinning = false
			m.pinNote = ""
		},
		Submit: func() tea.Cmd {
			if m.resumeMode == nil {
				m.pinNote = "changing the resume mode is unavailable"
				return nil
			}
			sessionID, mode := session.ID, m.pinValue
			return func() tea.Msg {
				updated, err := m.resumeMode(context.Background(), sessionID, mode)
				return resumeModeChanged{session: updated, err: err}
			}
		},
	})
	if handled {
		return m, cmd
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
	fmt.Fprintf(&b, "%s\n", m.detailField("Current:   ", state))
	fmt.Fprintf(&b, "%s\n", m.detailField("New:       ", fmt.Sprintf("%s (left/right cycles: %s)", m.pinValue, strings.Join(resumeModeOptions, ", "))))
	b.WriteString("\npinned always resumes this session's own current conversation id, sticky\nacross a deck restart. fresh-once starts a brand-new conversation exactly\nonce, then reverts to auto. Neither changes a running pane.\n")
	b.WriteString("\nLeft/Right cycles · Enter confirms · Esc cancels\n")
	if m.pinNote != "" {
		fmt.Fprintf(&b, "\n%s\n", m.pinNote)
	}
	return m.framedDialog(b.String())
}

// detailView renders the selected session's full detail, including an
// explicit degradation sentence when the adapter could not honour the
// originally requested permission profile (SPEC §5: "say so in the row
// detail rather than silently lying").
func (m Model) detailView() string {
	session := m.sessions[m.selected]
	var b strings.Builder
	fmt.Fprintf(&b, "%s detail\n\n", session.Name)
	fmt.Fprintf(&b, "%s\n", m.detailField("Agent:              ", session.Agent))
	fmt.Fprintf(&b, "%s\n", m.detailField("Working directory:  ", session.CWD))
	status := session.Status
	if status == "stopped" {
		status += m.glyph(" · resumable", " - resumable")
	}
	if status == "starting" && session.Agent != "shell" {
		status = "starting" + m.glyph(" · awaiting signal", " - awaiting signal")
	}
	fmt.Fprintf(&b, "%s\n", m.detailField("Status:             ", status))
	if session.StatusReason != "" {
		fmt.Fprintf(&b, "%s\n", m.detailField("Status reason:      ", session.StatusReason))
	}
	source := session.StatusSource
	if source == "" {
		source = "unknown"
	}
	if quality := statusSourceQuality(session.StatusSource); quality != "" {
		fmt.Fprintf(&b, "%s\n", m.detailField("Verdict source:     ", fmt.Sprintf("%s (%s)", source, quality)))
	} else {
		fmt.Fprintf(&b, "%s\n", m.detailField("Verdict source:     ", source))
	}
	if session.StatusAt > 0 {
		fmt.Fprintf(&b, "%s\n", m.detailField("Verdict age:        ", m.relativeAge(session.StatusAt)))
	}
	// A total probe miss (no probeRule matched the sampled pane at all) never
	// changes Status/StatusSource/StatusAt (SPEC §7 is untouched), so it is
	// only surfaced here, distinct from "never sampled": a miss strictly newer
	// than the row's current verdict is the freshest evidence deck has, and is
	// superseded (stops rendering) the instant any later verdict lands.
	if session.LastProbeAt > session.StatusAt {
		fmt.Fprintf(&b, "%s\n", m.detailField("Probe:              ", fmt.Sprintf("sampled, no rule matched (%s)", m.relativeAge(session.LastProbeAt))))
	}
	if _, applicable := m.agentCapabilities(session.Agent); applicable {
		fmt.Fprintf(&b, "%s\n", m.detailField("Permission profile: ", session.PermissionProfile))
		if session.PermissionProfileReason != "" {
			fmt.Fprintf(&b, "%s\n", m.detailField("  degraded: ", session.PermissionProfileReason))
		}
	} else {
		fmt.Fprintf(&b, "%s\n", m.detailField("Permission profile: ", "n/a (shell has no permission profile)"))
	}
	if session.ConversationID != "" {
		fmt.Fprintf(&b, "%s\n", m.detailField("Conversation id:    ", session.ConversationID))
	}
	if session.LastMessage != "" {
		fmt.Fprintf(&b, "\nLast message:\n%s\n", session.LastMessage)
	}
	if session.CrashTail != "" {
		crashTail := m.renderCrashTail(session.CrashTail)
		if session.PaneExitStatus != nil {
			fmt.Fprintf(&b, "\nCrash tail (exit status %d):\n%s\n", *session.PaneExitStatus, crashTail)
		} else {
			fmt.Fprintf(&b, "\nCrash tail:\n%s\n", crashTail)
		}
	}
	b.WriteString("\n" + m.glyph("i or Esc closes detail", "i or Esc closes detail") + "\n")
	return m.framedDialog(b.String())
}

// renderCrashTail keeps the non-scrolling detail screen usable even when the
// durable 200-line capture contains a full terminal of blank or noisy output.
// The stored artifact is unchanged; detail shows both ends and says exactly
// how much was omitted.
func (m Model) renderCrashTail(tail string) string {
	lines := strings.Split(strings.Trim(tail, "\n"), "\n")
	const maxLines = 8
	if len(lines) <= maxLines {
		return strings.Join(lines, "\n")
	}
	omitted := len(lines) - maxLines
	visible := append([]string(nil), lines[:maxLines/2]...)
	visible = append(visible, m.colorToken(theme.Dimmed, fmt.Sprintf(m.glyph("… %d lines omitted …", "... %d lines omitted ..."), omitted)))
	visible = append(visible, lines[len(lines)-maxLines/2:]...)
	return strings.Join(visible, "\n")
}

// detailField renders one `label: value` line shared by detailView,
// pinView and profileSwitchView (task 022, SPEC requirement 34): the label
// (including its trailing alignment spaces, exactly as it always rendered)
// in the `hint` token, the value in `text` — mirroring settings.go's own
// settingsRowSegment label/value split rather than inventing a second
// convention. Each half self-resets via colorToken, so plain concatenation
// is safe here: neither dialog composes these lines under a shared
// selection background the way the sidebar/settings rows do.
func (m Model) detailField(label, value string) string {
	return m.colorToken(theme.Hint, label) + m.colorToken(theme.Text, value)
}

// glyph selects the documented ASCII fallback for terminals where optional
// Unicode symbols are unwanted or unsuitable.
func (m Model) glyph(unicode, ascii string) string {
	if m.settings.ASCII {
		return ascii
	}
	return unicode
}

// relativeTime is intentionally based on the configured wall clock. A frozen
// DECK_CLOCK therefore keeps this rendered value stable while Clock.Elapsed
// remains monotonic for measurements and audit durations.
func (m Model) relativeTime(createdAt int64) string {
	age := m.relativeAge(createdAt)
	if age == "just now" {
		return age
	}
	return age + " ago"
}

// relativeAge is wall-clock-derived so verdict freshness remains honest and
// deterministic under DECK_CLOCK. A timestamp in the future is treated as new
// rather than rendering a misleading negative age.
func (m Model) relativeAge(at int64) string {
	now := time.Now()
	if m.settings.Clock != nil {
		now = m.settings.Clock.Now()
	}
	age := now.Sub(time.UnixMilli(at))
	if age < time.Minute {
		return "just now"
	}
	if age < time.Hour {
		return fmt.Sprintf("%dm", int(age/time.Minute))
	}
	if age < 24*time.Hour {
		return fmt.Sprintf("%dh", int(age/time.Hour))
	}
	return fmt.Sprintf("%dd", int(age/(24*time.Hour)))
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
// permission profiles at all (agentCapabilities reports
// !applicable): its field stays a single cosmetic "safe" value, per the
// existing shell-is-inert-to-profiles rule.
func (m Model) createProfileOptionsFor(kind string, allowYolo bool) []string {
	caps, applicable := m.agentCapabilities(kind)
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

// agentCapabilities returns the declared capabilities for kind, looked up
// in the model's agent registry, and whether a permission profile is even
// applicable to it. shell has no notion of permission profiles at all
// (SPEC §5/§8): its create field is present for consistency but never
// validated, so a shell session is never rejected for an "unsupported"
// profile that simply does not apply to it. An adapter that declares no
// profiles at all (shell) is treated as not applicable, exactly as the old
// hardcoded switch did.
func (m Model) agentCapabilities(kind string) (agent.Caps, bool) {
	adapter, ok := m.registry().Lookup(kind)
	if !ok {
		return agent.Caps{}, false
	}
	caps := adapter.Capabilities()
	return caps, len(caps.Profiles) > 0
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
	resolvedCWD, err := expandCreateCWD(m.createCWD)
	if err != nil {
		return err.Error()
	}
	info, err := os.Stat(resolvedCWD)
	if err != nil {
		return fmt.Sprintf("working directory %q does not exist", resolvedCWD)
	}
	if !info.IsDir() {
		return fmt.Sprintf("working directory %q is not a directory", resolvedCWD)
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
	if caps, applicable := m.agentCapabilities(m.createAgent); applicable {
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

// expandCreateCWD expands a leading `~` or `~/...` in raw to the current
// user's home directory and returns the resolved absolute path, per SPEC
// §11.7's tilde-expansion rule (the minimum slice of it Phase 2b-2 owns:
// no recent_cwds, prefill, history cycling, ghost completion or tab —
// those stay in Phase 3). A bare `~otheruser` form is rejected with a
// stated reason rather than half-expanded, since resolving another user's
// home directory is out of scope. Inputs with no leading `~` are returned
// unchanged, preserving every existing relative/absolute-path behaviour.
func expandCreateCWD(raw string) (string, error) {
	if !strings.HasPrefix(raw, "~") {
		return raw, nil
	}
	if raw != "~" && !strings.HasPrefix(raw, "~/") {
		return "", fmt.Errorf("cannot expand %q: only your own home directory (~ or ~/...) is supported, not another user's", raw)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot expand %q: %w", raw, err)
	}
	if raw == "~" {
		return filepath.Abs(home)
	}
	return filepath.Abs(filepath.Join(home, strings.TrimPrefix(raw, "~/")))
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
	if cmd, handled := applyDialogContract(msg, dialogContract{
		Fields: dialogFields{
			Count:          createFieldCount,
			Index:          &m.createField,
			Cycle:          m.cycleCreateField,
			SpaceTypesText: func() bool { return createFieldIsText(m.createField) },
		},
		Cancel: func() { m.creating, m.createError = false, "" },
		Submit: m.submitCreate,
	}); handled {
		return m, cmd
	}
	switch msg.String() {
	case "y":
		// The yolo double-gate's explicit confirm keystroke; everywhere
		// else "y" is just a typed character and must fall through to the
		// append logic below (e.g. typing a name starting with "y"). This is
		// an additional load-bearing key the §11.4 contract does not itself
		// define, stated inline in createView (SPEC §5).
		if m.createField == 3 && m.createProfile == "yolo" && m.settings.AllowYolo && !m.createYoloConfirmed {
			m.createYoloConfirmed = true
			return m, nil
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

// submitCreate implements the create modal's enter (SPEC §11.4 submit): it
// validates, resolves the cwd, dispatches the right create call for the
// chosen agent, and reports every rejection in-dialog via m.createError,
// exactly as the dialog's own hand-written "enter" case used to before the
// shared §11.4 contract (dialogContract.Submit) took over dispatching the
// key itself.
func (m *Model) submitCreate() tea.Cmd {
	if msg := m.validateCreateFields(); msg != "" {
		m.createError = msg
		return nil
	}
	// validateCreateFields already proved this succeeds; the resolved,
	// absolute path (never the typed tilde) is what gets stored.
	resolvedCWD, err := expandCreateCWD(m.createCWD)
	if err != nil {
		m.createError = err.Error()
		return nil
	}
	if m.createAgent != "shell" {
		if m.createAgentSession == nil {
			m.createError = "creating " + m.createAgent + " sessions is not available yet"
			return nil
		}
		launchArgs, err := parseCreateLaunchArgs(m.createLaunchArgs)
		if err != nil {
			m.createError = err.Error()
			return nil
		}
		env, err := parseCreateEnv(m.createEnv)
		if err != nil {
			m.createError = err.Error()
			return nil
		}
		input := service.AgentCreateInput{
			Name: m.createName, CWD: resolvedCWD, Agent: m.createAgent,
			PermissionProfile: m.createProfile, LaunchArgs: launchArgs, Env: env,
			PreLaunch: m.createPreLaunch, LoginShell: m.createLoginShell,
		}
		createAgentSession := m.createAgentSession
		return func() tea.Msg {
			session, err := createAgentSession(context.Background(), input)
			return shellCreated{session: session, err: err}
		}
	}
	if m.create == nil {
		m.createError = "shell creation is unavailable"
		return nil
	}
	name, cwd, create := m.createName, resolvedCWD, m.create
	return func() tea.Msg {
		session, err := create(context.Background(), service.ShellCreateInput{Name: name, CWD: cwd})
		return shellCreated{session: session, err: err}
	}
}

// cycleCreateField advances a selection-type field's value by delta; it is a
// no-op on the free-text fields.
func (m *Model) cycleCreateField(delta int) {
	switch m.createField {
	case 2:
		m.createAgent = cycleOption(m.registry().Kinds(), m.createAgent, delta)
		options := m.createProfileOptionsFor(m.createAgent, m.settings.AllowYolo)
		if !contains(options, m.createProfile) {
			m.createProfile = options[0]
			m.createYoloConfirmed = false
		}
	case 3:
		options := m.createProfileOptionsFor(m.createAgent, m.settings.AllowYolo)
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
	profileOptions := m.createProfileOptionsFor(m.createAgent, m.settings.AllowYolo)
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
		{"Login shell", loginShell + " (space toggles)", "makes captured_path advisory only (not applied): runs via $SHELL -lc instead of the agent argv, so the login shell sets PATH"},
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
	b.WriteString(title + "\n")
	for field, row := range m.createFieldRows() {
		fmt.Fprintf(&b, "%s%s: %s\n    %s\n", marker(field), row.label, row.value, row.help)
	}
	b.WriteString("Tab/Shift+Tab field · Left/Right/Space cycles · Enter submits · Esc cancels\n")
	if m.createError != "" {
		if strings.Contains(m.createError, "collides with existing slug") {
			b.WriteString("\nCannot create session: name collides with existing slug.\n")
		} else {
			fmt.Fprintf(&b, "\nCannot create session: %s\n", m.createError)
		}
	}
	return m.framedDialog(b.String())
}

// helpView renders the `?` help overlay (SPEC requirement 16: bordered like
// every other panel/dialog/overlay). It is a Model method rather than a
// free function so it can reuse framedDialog's box-drawing without
// duplicating boxGlyphs/ASCII-fallback logic here.
func (m Model) helpView() string {
	return m.framedDialog(helpText(m.settings.ASCII))
}

func helpText(ascii bool) string {
	text := `deck help

Keys
  ↑/↓ or j/k select a session
  ↵ attach the selected running session
  Y acknowledge the selected waiting/error session, clear its unseen marker
  n create a session (shell, or an agent: claude or pi)
  x kill the selected running session
  r resume the selected stopped session with its own agent argv (never
    --continue or "most recent"); resumed agents read "starting · awaiting
    signal" until a hook or sampled probe reports ready, while live shells
    become "running" on reconciliation; a client that loses the launch-lease
    race sees "starting elsewhere" instead of an error
  P switch the permission profile of the selected session; only takes
    effect on the next launch or resume ("restart to apply"), never the
    live pane
  p pin the selected session's conversation id so future resumes always
    reuse it, or launch a one-shot fresh conversation (reverts to normal
    auto-resume afterward, it does not stay pinned or cleared)
  i toggle detail view for the selected session
  space move to the next session needing attention (waiting or error),
    wrapping around; does nothing when nothing needs attention and never
    changes any session's status
  g toggle the selected row's workspace group collapsed/expanded
  , open/close settings (edit config.toml's keys); Esc closes, prompting to
    discard if there are unsaved changes
  t open/close the theme picker; previews live on the real session list,
    Enter selects, Esc reverts byte-for-byte
  | cycle the layout mode: auto → side-by-side → stacked → collapsed →
    auto; a chosen mode is pinned regardless of terminal width until you
    cycle again or the terminal cannot hold its floors, when deck falls
    back to auto without forgetting the pin
  < / > shrink/grow the sidebar by one column
  ? open/close help; Esc closes help
  q or Ctrl+C quit deck

Create dialog fields
  Name                the display name; also the source of the tmux slug
  Working directory    the session's cwd; must exist and be a directory
  Agent               shell, claude, or pi; which adapter launches it
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

Settings takeover (opened with ,)
  Tab or Left/Right      switch focus between the category list and the
                         field list
  Up/Down or j/k         move within the focused list
  Enter or Space         toggle/cycle the focused field's value
  + / -                  adjust a bounded-integer field
  /                      fuzzy-search every field by label or description
  Ctrl+S                 save staged edits to config.toml
  Esc                    close; with unsaved changes, prompts to discard
                         (y/Enter discards, any other key keeps editing)

Theme picker (opened with t)
  Left/Right or Up/Down or Space   change the previewed theme
  Enter                  select the previewed theme and save it
  Esc                    revert to the theme active before the picker opened

Runtime controls
  DECK_HOME             isolated data/config/state root
  DECK_TMUX_SOCKET      private tmux socket (default: deck)
  DECK_CLOCK            freeze wall clock (RFC3339); clock.now overrides it
  DECK_CLOCK_STEP       exact amount advanced by each on-demand trigger
  Trigger             kill -USR1 <deck-client-pid>; each invocation advances
                        the shared clock by exactly DECK_CLOCK_STEP
  clock.now             shared RFC3339 state under the resolved data root;
                        the trigger updates it and every process reads it
  DECK_ID_SEED          deterministic generated UUIDs
  DECK_RECONCILE_MS     list/reconciliation interval in milliseconds
  DECK_PREVIEW_MS       pane-preview interval in milliseconds
  DECK_UNDO_MS          undo-toast window after x in milliseconds
  DECK_DELETE_GRACE_MS  dd tombstone grace period before reap in milliseconds
  DECK_ASCII=1          use ASCII instead of optional glyphs
  DECK_ANIM=0           disable animation
  DECK_COLOR            explicitly enable or disable colour
  DECK_COLOR_DEPTH      force truecolor or 16-colour (quantised) rendering
                         instead of terminal auto-detection
  NO_COLOR              disable colour

Sessions use a private tmux server. Inspect it with:
  tmux -L deck ls
(or swap deck with DECK_TMUX_SOCKET). Plain tmux attach does not find deck
sessions. Attach clears TMUX because nested tmux is unsupported. Attached
clients share one pane geometry; the latest active client controls it.

Mouse (every binding duplicates a key above; nothing here is mouse-only)
  click a sidebar row       select it (like ↑/↓); the preview follows on
                            its next tick
  double-click a row        attach (like ↵)
  click a group header      toggle that group's collapse (like g)
  wheel over the sidebar    scroll the list without changing selection
                            (like ↑/↓/PgUp/PgDn)
  drag the seam             adjust sidebar_width live (like </>)
  click the collapsed strip restore the previous layout mode (like |)
  click or wheel over the preview does nothing; a click outside a dialog
  does nothing (Esc cancels)
  DECK_MOUSE=0 (or [ui] mouse = false) disables all mouse reporting and
  every mouse binding above; every keyboard path keeps working, only the
  shortcuts are lost
  Mouse reporting takes over the terminal's own click-drag text selection;
  hold your terminal's override modifier (usually shift) to select and
  copy text while deck is running

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
