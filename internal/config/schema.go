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
