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
type Scope string

const (
	// ScopeGlobal: the value lives only in config.toml; there is no
	// per-session override, and nothing about applying it depends on a
	// session's lifecycle.
	ScopeGlobal Scope = "global"
	// ScopeSessionOverride: config.toml supplies a default that an
	// individual session's own state can override (§6.1).
	ScopeSessionOverride Scope = "per-session override"
	// ScopeRestartToApply: the edit is written immediately, but per §6.2
	// (env reaches only new processes) and P (§5, a profile switch
	// restarts the pane), it has no effect on an already-running pane
	// until that pane (or session launch) restarts.
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
			"quiet hook stream; higher values trust a stale hook verdict longer.",
		Scope: ScopeGlobal,
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
			"last-known pane on an unattended session.",
		Scope: ScopeGlobal,
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
		Scope: ScopeGlobal,
	},
	{
		Section:     "ui",
		Key:         "ascii",
		Kind:        KindToggle,
		Default:     false,
		Description: "Forces box-drawing chrome to render with plain ASCII characters instead of Unicode line-drawing glyphs (SPEC §11), for terminals or fonts that render Unicode drawing characters incorrectly.",
		Scope:       ScopeGlobal,
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
			"how many entries are kept/offered.",
		Scope: ScopeGlobal,
	},
	{
		Section:     "env",
		Key:         "",
		Kind:        KindListOfStrings,
		Default:     []string{},
		Description: "The middle PATH/env layer (SPEC §6.1): KEY=VALUE entries that sit between the tmux server's inherited environment (and captured_path) and a session's own env map override. A session's own env map, set via its own editor, always wins over this table for a key both define. Editable here: enter opens the entries list (up/down select, enter edits the highlighted entry or adds a new one on the trailing \"+ add entry\" row, - removes the highlighted entry); while typing an entry, tab switches between its key and value and enter on the value stages it. Nothing here is saved to config.toml until ctrl+s.",
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
