package config

// This file declares the schema task 010 requires: one entry per flat key
// config.toml supports, each carrying enough metadata (kind, bounds,
// default, human description, scope) for both a parser (task 011) and the
// settings takeover (tasks 013-018) to be driven off it instead of hand
// maintaining the key set twice. See SPEC.md §6.5 and §11.5.

// FieldKind names the field kinds §11.5 enumerates: "Field kinds are
// explicit: toggle, integer with bounds, string, path (with a picker), enum
// (cycled), list-of-strings, and link (opens the owning dialog)."
type FieldKind string

const (
	KindToggle        FieldKind = "toggle"
	KindInteger       FieldKind = "integer"
	KindString        FieldKind = "string"
	KindPath          FieldKind = "path"
	KindEnum          FieldKind = "enum"
	KindListOfStrings FieldKind = "list-of-strings"
	KindLink          FieldKind = "link"
)

// Scope names the three labels §11.5 requires on every field: "Scope is
// labelled per field: global (config.toml), or per-session override where
// one exists (§6.1). A field that only takes effect on the next launch says
// restart-to-apply, consistent with §6.2 and P (§5)."
//
// requirement 19 (docs/reports/phase2b2-findings.md, task 005): Scope is a
// claim about *when a save takes effect*, not merely where a value is
// stored. ScopeGlobal on a field means a successful ctrl+s in the settings
// takeover (task 006) is observable in the already-running client without
// any restart; ScopeRestartToApply means it is not, and the takeover's
// on-screen copy must say so (task 008). A field is ScopeGlobal here only
// if a concrete, already-identified code path re-reads the refreshed
// config.Settings (or reacts to it) during the running process's own
// lifetime — see the per-field comment below for that path. Every field
// below was individually decided against the tree as it stands today, not
// assumed from its section.
type Scope string

const (
	// ScopeGlobal: the value lives only in config.toml; there is no
	// per-session override, and nothing about applying it depends on a
	// session's lifecycle. A save takes effect in the running client
	// (see the per-field comment for the exact consumer).
	ScopeGlobal Scope = "global"
	// ScopeSessionOverride: config.toml supplies a default that an
	// individual session's own state can override (§6.1).
	ScopeSessionOverride Scope = "per-session override"
	// ScopeRestartToApply: the edit is written immediately, but the
	// consumer that would need to see it either only reads it once at
	// process start (a captured closure, a tea.ProgramOption applied
	// before Run) or does not exist yet in this phase, so it has no
	// effect on the already-running client until the whole `deck`
	// process (§6.2, §5's P) restarts.
	ScopeRestartToApply Scope = "restart-to-apply"
)

// Bounds gives an integer field's inclusive bounds. A nil Max means no
// documented upper bound; Min is always populated for an integer field.
type Bounds struct {
	Min int
	Max *int // nil: unbounded above
}

// Field is one declared config.toml key (or, for [env], the whole table).
type Field struct {
	// Section is the config.toml table the key lives in ("" for top
	// level, "ui" for [ui], "env" for [env]).
	Section string
	// Key is the key name within Section. Empty only for the [env]
	// table itself, which this schema treats as one field (its members
	// are arbitrary environment variable names, not a fixed key set).
	Key  string
	Kind FieldKind
	// Default is the documented default value for the key: bool for
	// KindToggle, int for KindInteger (in the key's stated unit, see
	// Unit), string for KindString/KindPath/KindEnum, []string for
	// KindListOfStrings, nil for KindLink (nothing to default).
	Default any
	// Unit is a human label for an integer field's value (e.g.
	// "seconds"). Empty for non-integer kinds.
	Unit string
	// IntBounds is populated only for KindInteger.
	IntBounds Bounds
	// EnumValues lists a KindEnum field's fixed choices. Empty when the
	// choices are resolved dynamically at render time (see DynamicEnum).
	EnumValues []string
	// DynamicEnum is true when a KindEnum field's choices cannot be
	// listed statically here because they depend on runtime discovery
	// (the theme picker's built-in-plus-user-theme list, §11.6).
	DynamicEnum bool
	// Description states what the field does and what changes when it
	// changes, per §11.5's "Each field states what it does and what
	// changes when it changes."
	Description string
	Scope       Scope
}

// FullKey renders the field's config.toml address, e.g. "allow_yolo",
// "ui.theme", or "[env]" for the whole table.
func (f Field) FullKey() string {
	if f.Section == "" {
		return f.Key
	}
	if f.Key == "" {
		return "[" + f.Section + "]"
	}
	return f.Section + "." + f.Key
}

// intBound is a small helper for building a Bounds with an upper limit,
// since Go has no literal syntax for "pointer to this int constant".
func intBound(max int) *int { return &max }

// Schema is the ordered, canonical declaration of every flat config.toml
// key deck supports, per SPEC.md §6.5's table. [notify] is deliberately
// absent: §6.5 and §11.5 both name it the stated structural exception,
// edited via its own dialog rather than flattened into fields here.
var Schema = []Field{
	{
		Section: "",
		Key:     "allow_yolo",
		Kind:    KindToggle,
		Default: false,
		Description: "Gates whether the yolo permission profile can be selected " +
			"when creating or switching a session (SPEC §5). Off by default: a " +
			"session cannot run in yolo, skipping every permission prompt, until " +
			"an operator opts in here explicitly.",
		// requirement 19: every read of this field (internal/tui/tui.go's
		// createProfileOptionsFor/updateProfileSwitch/updateCreate/
		// updateHelp call sites) is `m.settings.AllowYolo`, checked fresh on
		// each keystroke against the running Model's own settings, not a
		// value captured once at startup. Task 006 refreshing
		// config.Settings on save is therefore sufficient to make a ctrl+s
		// here take effect on the very next keystroke, live.
		Scope: ScopeGlobal,
	},
	{
		Section: "",
		Key:     "yolo_default",
		Kind:    KindToggle,
		Default: false,
		Description: "When true (and only once allow_yolo is also true), the create " +
			"modal opens already on the yolo permission profile instead of safe " +
			"(SPEC \u00a75). Off by default. Inert while allow_yolo is false -- " +
			"turning this on with allow_yolo still off is a stated, visible " +
			"inconsistency in this row's own text, never a silent override in " +
			"either direction (allow_yolo does not flip on because of this, and " +
			"this does not get silently ignored/cleared either).",
		// requirement 19 (steer 017 item 2): the create modal's "n" key handler
		// (internal/tui/tui.go) reads m.settings.YoloDefault fresh, at the exact
		// moment the modal opens, against the running Model's own settings --
		// the same live-read shape allow_yolo's createProfileOptionsFor call
		// sites already have. Task 006 refreshing config.Settings on save is
		// therefore sufficient to make a ctrl+s here take effect the very next
		// time `n` is pressed, live, with no restart.
		Scope: ScopeGlobal,
	},
	{
		Section: "",
		Key:     "stale_after",
		Kind:    KindInteger,
		Default: int(DefaultStaleAfter.Seconds()),
		Unit:    "seconds",
		IntBounds: Bounds{
			Min: 1, // toml.go's existing parser already rejects <= 0.
		},
		Description: "The wall-clock age a hook-derived status verdict may reach " +
			"before it becomes eligible for probing the agent's pane instead of " +
			"being trusted as-is (SPEC §7). Lower values probe sooner after a " +
			"quiet hook stream; higher values trust a stale hook verdict longer. " +
			"Restart-to-apply: saving here writes config.toml immediately, but the " +
			"already-running client keeps probing on its old interval until deck " +
			"restarts.",
		// requirement 19: the only consumer is cmd/deck/main.go's
		// tuiReconcile closure, `sessions.ReconcileWithProbes(ctx,
		// settings.StaleAfter)`, which closes over run()'s own local
		// `settings` variable captured once before tui.New* builds the
		// Model. The Model's own config.Settings (task 006 refreshes on
		// save) is a separate copy the closure never reads, and the
		// reconciler func(context.Context) error signature the Model holds
		// has no way to pass a current value back in. A save changes
		// config.toml immediately but the running client keeps probing on
		// the old interval until the whole process restarts.
		Scope: ScopeRestartToApply,
	},
	{
		Section: "",
		Key:     "capture_min_interval",
		// SPEC §9.4 documents the key but not a numeric default; five
		// seconds is chosen here as a conservative floor between
		// opportunistic scrollback captures triggered by hook traffic.
		Default: 5,
		Kind:    KindInteger,
		Unit:    "seconds",
		IntBounds: Bounds{
			Min: 1,
		},
		Description: "The minimum spacing between opportunistic scrollback " +
			"captures of a session's pane triggered by _hook invocations " +
			"(SPEC §9.4). Lower values capture more often at the cost of more " +
			"tmux calls; higher values capture less often and risk a staler " +
			"last-known pane on an unattended session. Restart-to-apply: saving " +
			"here writes config.toml immediately, but nothing in the already-" +
			"running client reads it again until deck restarts.",
		// requirement 19: §9.4's opportunistic-capture throttle has no
		// consumer anywhere in this tree yet (`grep -rn CaptureMinInterval`
		// outside config/settings plumbing finds nothing in internal/tui,
		// internal/service or cmd/deck) — it is schema/parse/write/edit
		// plumbing only, deferred to whichever future phase wires the
		// _hook-triggered capture SPEC §9.4 describes. There is nothing a
		// running client could apply live even in principle today, so this
		// is labelled restart-to-apply as the honest, conservative default
		// pending that consumer, rather than ScopeGlobal implying a live
		// effect this tree cannot demonstrate.
		Scope: ScopeRestartToApply,
	},
	{
		Section: "",
		Key:     "event_retention_days",
		Kind:    KindInteger,
		Default: 30,
		Unit:    "days",
		IntBounds: Bounds{
			Min: 1,
		},
		Description: "How many days of events (SPEC \u00a712) the store keeps before " +
			"deleting them, oldest first, in bounded batches -- on store open and " +
			"thereafter at most once an hour (SPEC \u00a76.5/\u00a712, steer 3e-001 \u00a76.4). " +
			"A floor on history, not a cap on row count: a burst inside the window " +
			"is kept whole. VACUUM never runs automatically. Lower values reclaim " +
			"space sooner at the cost of less history for \u00a712 search and the " +
			"event log; higher values keep more history at the cost of database " +
			"size. Restart-to-apply: saving here writes config.toml immediately, " +
			"but nothing in the already-running client reads it again until deck " +
			"restarts.",
		// requirement 19: the sole consumer is cmd/deck/main.go's tuiReconcile
		// closure, which calls Store.EnforceEventRetention with a `settings`
		// local captured once before the Model exists -- the same shape as
		// stale_after/tmux_mouse above, and for the same reason: there is no
		// path back into a refreshed config.Settings for that closure to read
		// a saved value from, so a save changes config.toml immediately but
		// the running client keeps purging on the old window until deck
		// restarts.
		Scope: ScopeRestartToApply,
	},
	{
		Section: "",
		Key:     "tmux_mouse",
		Kind:    KindToggle,
		Default: true,
		Description: "Enables tmux's own `mouse on` server option on deck's " +
			"private -L socket (SPEC §6.5/§11.8), so a wheel notch scrolls an " +
			"attached pane's scrollback instead of tmux ignoring it. Independent " +
			"of [ui] mouse above, which is the terminal-side SGR reporting toggle " +
			"that lets deck's own client navigate the sidebar/dialogs with the " +
			"mouse; this key is tmux's own server-side mouse option on deck's " +
			"private socket only -- it never touches the user's default tmux " +
			"socket or ~/.tmux.conf. DECK_TMUX_MOUSE overrides the file when set. " +
			"On by default.",
		// requirement 19: internal/tmux.Client.Bootstrap sets this option every
		// time a session is created (Client.Create calls Bootstrap
		// unconditionally), but cmd/deck/main.go builds the one tmux.Client used
		// for session creation once, from settings.TmuxMouse captured before
		// tui.New* builds the Model -- a save through the settings takeover
		// writes config.toml immediately, but the already-running process's own
		// client value does not change until deck restarts.
		Scope: ScopeRestartToApply,
	},
	{
		Section: "",
		Key:     "interactive_ms",
		// SPEC §13.1 documents DECK_INTERACTIVE_MS as "a duration, like the
		// two ticks above" (DECK_RECONCILE_MS/DECK_PREVIEW_MS) but states no
		// numeric default; 60ms is chosen here because it is the exact
		// coalescing interval the Part II spike measured render cost
		// against (per-read rendering costs 1.99x coalescing to 60ms --
		// II-27/the spike report), not an arbitrary round number.
		Default: 60,
		Kind:    KindInteger,
		Unit:    "milliseconds",
		IntBounds: Bounds{
			Min: 1,
		},
		Description: "The grid render-coalescing interval for §11.9's interactive " +
			"preview: pane bytes arriving faster than this are batched into one " +
			"repaint rather than one repaint per read (SPEC §13.1). Lower values " +
			"repaint more often at higher CPU cost; higher values coalesce more " +
			"aggressively at the cost of a laggier-feeling terminal. " +
			"DECK_INTERACTIVE_MS overrides the file when set, exactly like " +
			"DECK_RECONCILE_MS/DECK_PREVIEW_MS override their own knobs (those " +
			"two have no config.toml counterpart at all; this one does, per " +
			"§6.5's \"declared in the schema with its DECK_ override like every " +
			"other key\"). Restart-to-apply: saving here writes config.toml " +
			"immediately, but nothing in the already-running client reads it " +
			"again until deck restarts.",
		// requirement 19: task 049/II-27 landed the render-coalescing loop
		// itself (internal/interactive.RenderCoalescer, driven by that
		// package's own renderCoalesceInterval var, currently 60ms to match
		// this key's default) but nothing in internal/tui constructs an
		// interactive.Session yet -- that wiring, whenever it lands, is
		// what would read InteractiveMS out of Settings and set the var
		// from it. Until then this key still has no live consumer to
		// apply changes to, the same honest-pending-consumer reasoning as
		// capture_min_interval and ui.recent_cwd_limit above -- only the
		// reason has narrowed from "the mechanism doesn't exist" to "the
		// mechanism exists but nothing hands it this setting yet".
		Scope: ScopeRestartToApply,
	},
	{
		Section: "",
		Key:     "interactive_transport",
		// II-5 (SPEC §6.5/§13.1): this is a selector between two
		// implementations of the SAME §11.9 interactive-preview contract
		// (pipe-pane -IO streaming into a grid, or a poll-and-capture-pane
		// fallback), not the kind of user-visible behaviour switch §13.1
		// forbids -- but §11.9's contract does not extend to scrollback
		// depth or history accumulation, and the two transports are NOT
		// equivalent there: TransportCapture's design (capture-pane -p on
		// each poll tick, never -S, replacing the grid wholesale) has no
		// meaningful scrollback at all, since anything that scrolls off
		// between two polls is gone rather than merely delayed. That is
		// why the four features/interactive_scroll.feature scenarios are
		// pipe-only (task 089); it is a third exclusion class, distinct
		// from and not named by requirement II-33 (peeling a trailing `;`
		// off a send-keys payload, meaningless to a transport that never
		// runs send-keys) or II-24. "pipe" is the default because it is
		// the one measured in the Part II spikes
		// (docs/spikes/interactive-preview.md); this task only declares
		// the knob, it does not implement either path -- see
		// features/interactive_*.feature and
		// docs/reports/phase3b-findings.md's II-5 section for the parity
		// sweep task 070/088/089 owed and ran.
		Kind:       KindEnum,
		Default:    "pipe",
		EnumValues: []string{"pipe", "capture"},
		Description: "Selects which of the two §11.9 interactive-preview " +
			"transports deck uses: \"pipe\" arms tmux's pipe-pane -IO into a " +
			"long-lived grid; \"capture\" polls and re-captures the pane " +
			"instead. Both must satisfy the same scenarios except the " +
			"pipe-only ones (peeling a trailing semicolon off a literal " +
			"send-keys payload) that have no meaning under capture. " +
			"DECK_INTERACTIVE_TRANSPORT overrides the file when set; any " +
			"value other than pipe or capture is a stated error naming the " +
			"variable, never a silent fallback.",
		// requirement 19: interactive mode itself (II-7 onward) does not
		// exist in this tree yet, so there is nothing a running client
		// could apply live even in principle today -- the same honest-
		// pending-consumer reasoning as capture_min_interval/interactive_ms
		// above.
		Scope: ScopeRestartToApply,
	},
	{
		Section:     "ui",
		Key:         "theme",
		Kind:        KindEnum,
		Default:     "",
		DynamicEnum: true, // resolved from theme.Builtins() plus any
		// discovered user theme (SPEC §11.6); the schema cannot list a
		// fixed choice set because a dropped-in user theme file changes
		// it without a code change.
		Description: "Selects the colour theme by name (SPEC §11.6). Empty or " +
			"an unknown/unparseable name falls back to the built-in default and " +
			"says so on first paint; a known name selects a built-in or a " +
			"discovered user theme under $XDG_CONFIG_HOME/deck/themes/*.toml.",
		// requirement 19: internal/tui/theme_picker.go's `t` picker already
		// proves this is live today — themePickerConfirm writes config.toml
		// and then sets `m.settings.Theme = candidate` in the same
		// keystroke, and internal/tui/theme_color.go's activeTheme() reads
		// m.settings.Theme on every render. Task 006 makes the general `,`
		// settings takeover's ctrl+s refresh m.settings.Theme the same way,
		// so the same key no longer has two different behaviours depending
		// on which of the two editors was used to change it.
		Scope: ScopeGlobal,
	},
	{
		Section:     "ui",
		Key:         "ascii",
		Kind:        KindToggle,
		Default:     false,
		Description: "Forces box-drawing chrome to render with plain ASCII characters instead of Unicode line-drawing glyphs (SPEC §11), for terminals or fonts that render Unicode drawing characters incorrectly.",
		// requirement 19: every glyph choice reads `m.settings.ASCII`
		// directly at render time (internal/tui/panel.go, tui.go's helpText
		// and other View()-time call sites) against the running Model's own
		// settings, so refreshing config.Settings on save (task 006) makes
		// the very next frame draw with the new glyph set.
		Scope: ScopeGlobal,
	},
	{
		Section: "ui",
		Key:     "mouse",
		Kind:    KindToggle,
		Default: true,
		Description: "Enables SGR mouse reporting so the mouse can navigate the " +
			"session list and dialogs (SPEC §11.8). On by default; off is an " +
			"explicit opt-out for terminals or tools where mouse-reporting " +
			"escape sequences interfere with normal terminal use.",
		// requirement 19: internal/tui/tui.go's tea.MouseMsg branch checks
		// `if !m.settings.Mouse { return m, nil }` fresh on every incoming
		// mouse report against the running Model's own settings (already
		// live today, independent of task 006), so turning the field off
		// takes effect on the very next mouse event once config.Settings is
		// refreshed on save. Turning it on also needs the terminal itself
		// told to start emitting SGR reports — today that is only
		// `tea.WithMouseCellMotion()`, a ProgramOption applied once before
		// tea.NewProgram(...).Run() in cmd/deck/main.go. bubbletea also
		// exposes this as a runtime command (screen.go's
		// EnableMouseCellMotion()/DisableMouse(), and Program.
		// EnableMouseCellMotion/DisableMouseCellMotion), so task 006 makes
		// settingsSave return the matching tea.Cmd when this field's value
		// changes, closing the gap in both directions rather than only the
		// off-direction the existing gate already covers.
		Scope: ScopeGlobal,
	},
	{
		Section: "ui",
		Key:     "preview_fit",
		Kind:    KindToggle,
		Default: true,
		Description: "When true, the passive preview fits the selected session's window " +
			"to the preview panel as the list selection settles -- coalesced " +
			"against the 250ms preview tick, skipped below the 7-inner-row " +
			"interactive floor, and best-effort (owning and restoring nothing; " +
			"an attaching client's own size simply wins) -- rather than leaving " +
			"a session cropped bottom-left at whatever size it last had (SPEC " +
			"§11). The cost: a fit sends the agent SIGWINCH and reflows its " +
			"output, so output produced while narrow consumes scrollback rows " +
			"faster and evicted rows never return. false restores a wholly " +
			"passive preview -- capture-pane -e poll, no resize, cropped " +
			"bottom-left. On by default. DECK_PREVIEW_FIT overrides the file " +
			"when set.",
		// requirement 19 (steer 018 item 4): tui.go's previewFit, called every
		// previewTick alongside capturePreview, reads m.settings.PreviewFit
		// fresh against the running Model's own settings on every tick -- the
		// same live-read shape ui.mouse's tea.MouseMsg gate already has -- so
		// task 006 refreshing config.Settings on save is sufficient to make a
		// ctrl+s here take effect on the very next tick, live, with no
		// restart.
		Scope: ScopeGlobal,
	},
	{
		Section: "ui",
		Key:     "recent_cwd_limit",
		Kind:    KindInteger,
		Default: 5,
		Unit:    "entries",
		IntBounds: Bounds{
			Min: 0,
			Max: intBound(50),
		},
		Description: "The number of recently used working directories offered " +
			"when creating a session (SPEC §11.7). The recent-directory list " +
			"itself lives in state.db, not config.toml — this key only bounds " +
			"how many entries are kept/offered. Restart-to-apply: saving here " +
			"writes config.toml immediately, but nothing in the already-running " +
			"client reads it again until deck restarts.",
		// requirement 19: §11.7's recent-cwd picker that would read this
		// bound has no consumer anywhere in this tree yet (`grep -rn
		// RecentCwdLimit` outside config/settings plumbing finds nothing in
		// internal/tui, internal/service or cmd/deck) — same situation as
		// capture_min_interval above: schema/parse/write/edit plumbing
		// only, so there is nothing a running client could apply live even
		// in principle today. Labelled restart-to-apply pending that
		// consumer, for the same honesty reason.
		Scope: ScopeRestartToApply,
	},
	{
		Section: "ui",
		Key:     "group_by_workspace",
		Kind:    KindToggle,
		Default: true,
		Description: "Groups the sidebar by sessions.workspace, defaulting to " +
			"the basename of cwd, with each group's own collapsed/expanded state " +
			"(`c`) preserved (SPEC §11/requirement 30). On by default; off is an " +
			"explicit opt-out for a flat, ungrouped sidebar.",
		// I-5 update (task 008): internal/tui/group.go's groupingEnabled()
		// is now this key's consumer -- sidebarEntries, visualOrder and
		// isSessionVisible all read m.settings.GroupByWorkspace, so
		// grouping is genuinely conditional, not unconditional, at render
		// time. The label stays ScopeRestartToApply for a narrower reason
		// than before: settingsApplyLiveFields (internal/tui/settings.go)
		// does not yet copy this field from settingsEdits into the running
		// m.settings the way it does for AllowYolo/ASCII/Mouse, so a save
		// still only reaches a fresh client, not the one that made it.
		// Wiring that live-copy (mirroring Mouse's EnvOverrides-guarded
		// copy, since this key has the identical DECK_GROUP_BY_WORKSPACE
		// override path) is unclaimed follow-up work, not this task's own
		// scope.
		Scope: ScopeRestartToApply,
	},
	{
		Section:    "ui",
		Key:        "sort_order",
		Kind:       KindEnum,
		Default:    "attention",
		EnumValues: []string{"attention", "created", "activity", "name"},
		Description: "Orders the sidebar (SPEC §11, amendment 6584299). " +
			"\"attention\" (default) is the unchanged waiting/error/running/" +
			"starting/idle/stopped tier order -- the order every earlier phase " +
			"shipped, kept default because it answers \"which session needs " +
			"me\". \"created\" is sessions.created_at descending (newest " +
			"first). \"activity\" is sessions.status_at descending -- a status " +
			"CHANGE (§7's own timestamp), never last pane output, which deck " +
			"does not record. \"name\" is case-insensitive ascending. Every " +
			"order falls back to id ascending on a tie (a total order, so a " +
			"re-sort can never swap two rows out from under an in-flight " +
			"keyboard idiom), and a non-attention order never secretly re-ranks " +
			"by status -- choosing one means the user, not deck, decides what " +
			"\"first\" means. An unknown/malformed value falls back to " +
			"attention and says so on the first painted frame, never silently.",
		// requirement 19/task 306: sessionsLoaded (task 305) already reads this
		// live on every reload, and settingsApplyLiveFields
		// (internal/tui/settings.go) now copies a changed value into the
		// running m.settings on save and re-sorts m.baseSessions/m.sessions
		// in place (Model.resortSessionsLive), preserving the selected
		// SESSION by id (never by index) and keeping its row inside the
		// sidebar's visible window via scrollSessionIntoView -- so a save
		// takes effect in the already-running client, matching
		// ui.mouse/ui.preview_fit's own ScopeGlobal reasoning above.
		Scope: ScopeGlobal,
	},
	{
		Section: "",
		Key:     "pre_launch",
		Kind:    KindString,
		Default: "",
		Description: "The global launch hook (task 004, phase3j): a shell command run in " +
			"every session's pane before that session's own agent/shell argv, " +
			"the same way a session's own pre_launch already does, but for EVERY " +
			"session rather than one chosen at create time. Empty by default -- " +
			"nothing runs unless this is set. Restart-to-apply: saving here writes " +
			"config.toml immediately, but the already-running client's launch path " +
			"keeps using whichever value it read at process start until deck " +
			"restarts.",
		// requirement 19: this key has no live consumer -- it is read once, at
		// process start, into service.Service (cmd/deck/main.go), and every
		// launch for the lifetime of that process uses the value captured
		// then. A save changes config.toml immediately but the already-running
		// client keeps launching with the old value until deck restarts.
		Scope: ScopeRestartToApply,
	},
	{
		Section:     "env",
		Key:         "",
		Kind:        KindListOfStrings,
		Default:     []string{},
		Description: "The middle PATH/env layer (SPEC §6.1): KEY=VALUE entries that sit between the tmux server's inherited environment (and captured_path) and a session's own env map override. A session's own env map, set via its own editor, always wins over this table for a key both define. Editable here: enter opens the entries list (up/down select, enter edits the highlighted entry or adds a new one on the trailing \"+ add entry\" row, - removes the highlighted entry); while typing an entry, tab switches between its key and value and enter on the value stages it. Nothing here is saved to config.toml until ctrl+s. Restart-to-apply: a save writes config.toml immediately, but tmux env changes reach only new processes, so a running pane keeps its old environment until it or deck restarts.",
		// §6.2: "tmux env changes reach only new processes, so a
		// mid-flight edit is inherently restart-to-apply." A change here
		// writes immediately but a running pane keeps its old
		// environment until it is restarted (R) or a new session is
		// launched.
		Scope: ScopeRestartToApply,
	},
}

// FieldByFullKey returns the schema Field for a full key as FullKey would
// render it (e.g. "ui.theme", "[env]"), and whether it was found.
func FieldByFullKey(fullKey string) (Field, bool) {
	for _, field := range Schema {
		if field.FullKey() == fullKey {
			return field, true
		}
	}
	return Field{}, false
}
